package integration_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nzagler/gradeium/backend/internal/database"
	"github.com/nzagler/gradeium/backend/internal/games"
	"github.com/nzagler/gradeium/backend/internal/integrations/igdb"
	"github.com/nzagler/gradeium/backend/internal/integrations/tmdb"
	"github.com/nzagler/gradeium/backend/internal/integrations/tvdb"
	"github.com/nzagler/gradeium/backend/internal/media"
	"github.com/nzagler/gradeium/backend/internal/movies"
	"github.com/nzagler/gradeium/backend/internal/secrets"
	"github.com/nzagler/gradeium/backend/internal/settings"
	tvdomain "github.com/nzagler/gradeium/backend/internal/tv"
)

func TestPhase3DatabaseMigratesToPhase4FullMediaProduct(t *testing.T) {
	baseURL := os.Getenv("GRADEIUM_TEST_DATABASE_URL")
	if baseURL == "" {
		t.Skip("GRADEIUM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	testURL := createTestDatabase(t, ctx, baseURL)
	applyMigrationsThrough(t, ctx, testURL, 3)
	pool, err := database.Open(ctx, testURL)
	if err != nil {
		t.Fatalf("open Phase 3 database: %v", err)
	}

	var userID, secondUserID string
	if err := pool.QueryRow(ctx, `INSERT INTO users(display_name,is_admin) VALUES('Phase 4 Admin',true) RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatalf("create Phase 3 administrator: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(display_name) VALUES('Second User') RETURNING id::text`).Scan(&secondUserID); err != nil {
		t.Fatalf("create Phase 3 second user: %v", err)
	}
	registry := settings.NewRegistry()
	if _, err := settings.NewService(registry, settings.NewPostgresStore(pool)).Update(ctx, settings.InstanceNameKey, []byte(`"Phase 3 Library"`)); err != nil {
		t.Fatalf("persist Phase 3 setting: %v", err)
	}
	pool.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := database.Migrate(ctx, testURL, logger); err != nil {
		t.Fatalf("migrate Phase 3 database to Phase 4: %v", err)
	}
	pool, err = database.Open(ctx, testURL)
	if err != nil {
		t.Fatalf("open Phase 4 database: %v", err)
	}
	t.Cleanup(pool.Close)

	values, err := settings.NewService(registry, settings.NewPostgresStore(pool)).List(ctx)
	if err != nil || len(values) == 0 || string(values[0].Value) != `"Phase 3 Library"` {
		t.Fatalf("Phase 3 setting after migration = (%#v, %v)", values, err)
	}

	assertPhase4ProviderSecretPersistence(t, ctx, pool, registry)
	assertPhase4GamePersistence(t, ctx, pool, userID, secondUserID)
	assertPhase4MoviePersistence(t, ctx, pool, userID)
	assertPhase4TVPersistence(t, ctx, pool, userID)
	assertPhase4Preferences(t, ctx, pool, userID)
}

func assertPhase4ProviderSecretPersistence(t *testing.T, ctx context.Context, pool *pgxpool.Pool, registry *settings.Registry) {
	t.Helper()
	configDirectory := t.TempDir()
	secretStore := secrets.NewPostgresStore(pool)
	cipher, err := secrets.InitializeCipher(ctx, configDirectory, secretStore)
	if err != nil {
		t.Fatalf("initialize Phase 4 secret cipher: %v", err)
	}
	service := secrets.NewService(registry, secretStore, cipher)
	plaintext := "phase-4-provider-secret-must-remain-encrypted"
	if err := service.Set(ctx, settings.TMDBAccessTokenKey, plaintext); err != nil {
		t.Fatalf("store encrypted provider secret: %v", err)
	}
	var rawPosition int
	if err := pool.QueryRow(ctx, `SELECT position(convert_to($1,'UTF8') in ciphertext) FROM secret_settings WHERE key=$2`, plaintext, settings.TMDBAccessTokenKey).Scan(&rawPosition); err != nil {
		t.Fatalf("inspect encrypted provider secret: %v", err)
	}
	if rawPosition != 0 {
		t.Fatal("provider secret plaintext is present in its database ciphertext")
	}
	reloadedCipher, err := secrets.InitializeCipher(ctx, configDirectory, secretStore)
	if err != nil {
		t.Fatalf("reload Phase 4 secret cipher: %v", err)
	}
	reloaded := secrets.NewService(registry, secretStore, reloadedCipher)
	value, err := reloaded.Read(ctx, settings.TMDBAccessTokenKey)
	if err != nil || string(value) != plaintext {
		t.Fatalf("provider secret after restart = (%q, %v)", value, err)
	}
	secrets.Clear(value)
	removed, err := reloaded.Delete(ctx, settings.TMDBAccessTokenKey)
	if err != nil || !removed {
		t.Fatalf("remove provider secret = (%v, %v)", removed, err)
	}
}

func assertPhase4GamePersistence(t *testing.T, ctx context.Context, databasePool *pgxpool.Pool, userID, secondUserID string) {
	t.Helper()
	store := games.NewPostgresStore(databasePool)
	game := fixtureGame(101, "The Foundation Game")
	detail, err := store.Add(ctx, userID, game, media.StatusInProgress)
	if err != nil {
		t.Fatalf("add game: %v", err)
	}
	assertUUIDv7(t, ctx, databasePool, detail.ID)

	secondDetail, err := store.Add(ctx, secondUserID, game, media.StatusBacklog)
	if err != nil || secondDetail.ID != detail.ID || secondDetail.State.Status != media.StatusBacklog {
		t.Fatalf("second user canonical reuse = (%#v, %v)", secondDetail, err)
	}
	assertConcurrentGameAdd(t, ctx, store, databasePool, userID)

	rating, reason := int16(87), "Tight systems and a memorable finale."
	state, err := store.UpdateState(ctx, userID, detail.ID, media.PersonalState{Status: media.StatusCompleted, Rating: &rating, RatingReason: &reason}, false)
	if err != nil || state.Rating == nil || *state.Rating != 87 {
		t.Fatalf("persist game rating = (%#v, %v)", state, err)
	}
	if _, err := store.UpdateState(ctx, userID, detail.ID, media.PersonalState{Status: media.StatusBacklog}, false); !errors.Is(err, games.ErrConfirmationRequired) {
		t.Fatalf("unconfirmed rated game backlog error = %v", err)
	}

	detail, err = store.SelectArtwork(ctx, userID, detail.ID, "cover", "game-cover-manual")
	if err != nil || detail.ArtworkPins["cover"] != "game-cover-manual" {
		t.Fatalf("pin game cover = (%#v, %v)", detail.ArtworkPins, err)
	}
	refreshed := fixtureGame(101, "The Foundation Game — Refreshed")
	refreshed.Artworks = refreshed.Artworks[:1]
	detail, err = store.Refresh(ctx, userID, refreshed)
	if err != nil {
		t.Fatalf("refresh game metadata: %v", err)
	}
	if detail.Title != refreshed.Title || detail.State.Rating == nil || *detail.State.Rating != 87 || detail.ArtworkPins["cover"] != "game-cover-manual" || !contains(detail.UnavailablePins, "cover") {
		t.Fatalf("game refresh did not preserve user data and unavailable pin: %#v", detail)
	}

	state, err = store.UpdateState(ctx, userID, detail.ID, media.PersonalState{Status: media.StatusBacklog}, true)
	if err != nil || state.Rating != nil || state.RatingReason != nil {
		t.Fatalf("confirmed game backlog transition = (%#v, %v)", state, err)
	}
	backlog, err := store.List(ctx, userID, true)
	if err != nil || len(backlog) != 1 || backlog[0].ID != detail.ID {
		t.Fatalf("game backlog list = (%#v, %v)", backlog, err)
	}
	if _, err := databasePool.Exec(ctx, `UPDATE user_games SET rating=9 WHERE user_id=$1 AND game_id=$2`, userID, detail.ID); err == nil {
		t.Fatal("database accepted a rating below 1.0")
	}
	if removed, err := store.Remove(ctx, userID, detail.ID); err != nil || !removed {
		t.Fatalf("remove user game = (%v, %v)", removed, err)
	}
	if _, err := store.Detail(ctx, secondUserID, detail.ID); err != nil {
		t.Fatalf("removing one user's game removed another user's association: %v", err)
	}
}

func assertConcurrentGameAdd(t *testing.T, ctx context.Context, store *games.PostgresStore, databasePool *pgxpool.Pool, userID string) {
	t.Helper()
	game := fixtureGame(102, "Concurrent Game")
	var successes, duplicates atomic.Int32
	var wait sync.WaitGroup
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := store.Add(ctx, userID, game, media.StatusInProgress)
			switch {
			case err == nil:
				successes.Add(1)
			case errors.Is(err, games.ErrAlreadyTracked):
				duplicates.Add(1)
			default:
				t.Errorf("concurrent game add: %v", err)
			}
		}()
	}
	wait.Wait()
	if successes.Load() != 1 || duplicates.Load() != 7 {
		t.Fatalf("concurrent game adds = %d success, %d duplicates", successes.Load(), duplicates.Load())
	}
	var canonical, personal int
	if err := databasePool.QueryRow(ctx, `SELECT count(*) FROM games WHERE igdb_id=102`).Scan(&canonical); err != nil {
		t.Fatalf("count canonical concurrent game: %v", err)
	}
	if err := databasePool.QueryRow(ctx, `SELECT count(*) FROM user_games ug JOIN games g ON g.entity_id=ug.game_id WHERE ug.user_id=$1 AND g.igdb_id=102`, userID).Scan(&personal); err != nil {
		t.Fatalf("count personal concurrent game: %v", err)
	}
	if canonical != 1 || personal != 1 {
		t.Fatalf("concurrent game rows = %d canonical, %d personal", canonical, personal)
	}
}

func assertPhase4MoviePersistence(t *testing.T, ctx context.Context, databasePool *pgxpool.Pool, userID string) {
	t.Helper()
	store := movies.NewPostgresStore(databasePool)
	movie := fixtureMovie()
	detail, err := store.Add(ctx, userID, movie, media.StatusInProgress)
	if err != nil {
		t.Fatalf("add movie: %v", err)
	}
	if detail.CollectionName != "Foundation Collection" || len(detail.Collection) != 2 {
		t.Fatalf("movie collection persistence = %#v", detail.Collection)
	}

	var wait sync.WaitGroup
	for index := range 8 {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			rating := int16(70 + index)
			if _, err := store.UpdateState(ctx, userID, detail.ID, media.PersonalState{Status: media.StatusCompleted, Rating: &rating}, false); err != nil {
				t.Errorf("concurrent movie state update: %v", err)
			}
		}(index)
	}
	wait.Wait()

	detail, err = store.SelectArtwork(ctx, userID, detail.ID, "poster", "movie-poster-manual")
	if err != nil {
		t.Fatalf("pin movie poster: %v", err)
	}
	movie.Title = "Foundation Movie — Refreshed"
	movie.Artworks = movie.Artworks[:1]
	detail, err = store.Refresh(ctx, userID, movie)
	if err != nil {
		t.Fatalf("refresh movie: %v", err)
	}
	if detail.State.Rating == nil || detail.ArtworkPins["poster"] != "movie-poster-manual" || !contains(detail.UnavailablePins, "poster") {
		t.Fatalf("movie refresh did not preserve personal state: %#v", detail)
	}
	if _, err := databasePool.Exec(ctx, `UPDATE user_movies SET rating_reason='reason without rating',rating=NULL WHERE user_id=$1 AND movie_id=$2`, userID, detail.ID); err == nil {
		t.Fatal("database accepted a rating reason without a rating")
	}
	if removed, err := store.Remove(ctx, userID, detail.ID); err != nil || !removed {
		t.Fatalf("remove movie = (%v, %v)", removed, err)
	}
	var personalRows int
	if err := databasePool.QueryRow(ctx, `SELECT count(*) FROM user_movies WHERE user_id=$1 AND movie_id=$2`, userID, detail.ID).Scan(&personalRows); err != nil || personalRows != 0 {
		t.Fatalf("movie personal rows after removal = (%d, %v)", personalRows, err)
	}
}

func assertPhase4TVPersistence(t *testing.T, ctx context.Context, databasePool *pgxpool.Pool, userID string) {
	t.Helper()
	store := tvdomain.NewPostgresStore(databasePool)
	mappingRating, mappingCount := int16(84), int32(1234)
	mapping := &tmdb.VerifiedTV{TMDBID: 9001, CommunityRating: &mappingRating, CommunityRatingCount: &mappingCount}
	detail, err := store.Add(ctx, userID, fixtureShow("Foundation Show"), mapping, media.StatusInProgress)
	if err != nil {
		t.Fatalf("add TV show: %v", err)
	}
	if detail.VerifiedTMDBID == nil || *detail.VerifiedTMDBID != 9001 || detail.Progress.Total != 2 || detail.Progress.SpecialsTotal != 1 {
		t.Fatalf("TV mapping or initial progress = %#v", detail)
	}

	var secondEpisodeID, specialEpisodeID string
	for _, season := range detail.Seasons {
		for _, episode := range season.Episodes {
			if episode.Special {
				specialEpisodeID = episode.ID
			} else if episode.EpisodeNumber == 2 {
				secondEpisodeID = episode.ID
			}
		}
	}
	if secondEpisodeID == "" || specialEpisodeID == "" {
		t.Fatalf("fixture episode IDs were not loaded: %#v", detail.Seasons)
	}
	detail, err = store.SetThrough(ctx, userID, detail.ID, secondEpisodeID)
	if err != nil || detail.Progress.Watched != 2 || detail.Progress.Percent != 100 || detail.Progress.SpecialsWatched != 0 || detail.State.Status != media.StatusInProgress {
		t.Fatalf("mark through regular episode = (%#v, %v)", detail.Progress, err)
	}
	detail, err = store.SetEpisode(ctx, userID, detail.ID, specialEpisodeID, true)
	if err != nil || detail.Progress.Watched != 2 || detail.Progress.SpecialsWatched != 1 {
		t.Fatalf("special progress separation = (%#v, %v)", detail.Progress, err)
	}

	rating, reason := int16(92), "A carefully paced season."
	if _, err := store.UpdateState(ctx, userID, detail.ID, media.PersonalState{Status: media.StatusCompleted, Rating: &rating, RatingReason: &reason}, false); err != nil {
		t.Fatalf("rate TV show: %v", err)
	}
	detail, err = store.SelectArtwork(ctx, userID, detail.ID, "poster", "tv-poster-manual")
	if err != nil {
		t.Fatalf("pin TV poster: %v", err)
	}
	refreshedShow := fixtureShow("Foundation Show — Refreshed")
	refreshedShow.Artworks = refreshedShow.Artworks[:1]
	detail, err = store.Refresh(ctx, userID, refreshedShow, nil)
	if err != nil {
		t.Fatalf("refresh TV show during TMDB outage: %v", err)
	}
	if detail.VerifiedTMDBID == nil || *detail.VerifiedTMDBID != 9001 || detail.CommunityRating == nil || *detail.CommunityRating != 84 || detail.State.Rating == nil || detail.Progress.Watched != 2 || detail.Progress.SpecialsWatched != 1 || !contains(detail.UnavailablePins, "poster") {
		t.Fatalf("TV refresh did not preserve verified mapping and user state: %#v", detail)
	}

	state, err := store.UpdateState(ctx, userID, detail.ID, media.PersonalState{Status: media.StatusBacklog}, true)
	if err != nil || state.Rating != nil {
		t.Fatalf("move TV show to backlog = (%#v, %v)", state, err)
	}
	detail, err = store.Detail(ctx, userID, detail.ID)
	if err != nil || detail.Progress.Watched != 2 || detail.Progress.SpecialsWatched != 1 {
		t.Fatalf("TV progress after backlog transition = (%#v, %v)", detail.Progress, err)
	}
	if removed, err := store.Remove(ctx, userID, detail.ID); err != nil || !removed {
		t.Fatalf("remove TV show: (%v, %v)", removed, err)
	}
	var progressRows int
	if err := databasePool.QueryRow(ctx, `SELECT count(*) FROM user_episode_progress WHERE user_id=$1`, userID).Scan(&progressRows); err != nil || progressRows != 0 {
		t.Fatalf("TV progress after removal = (%d, %v)", progressRows, err)
	}
}

func assertPhase4Preferences(t *testing.T, ctx context.Context, databasePool *pgxpool.Pool, userID string) {
	t.Helper()
	service := media.NewPreferencesService(databasePool)
	preferences, err := service.Update(ctx, userID, media.Preferences{DefaultLibrarySort: "title_asc", PreferredView: "list"})
	if err != nil || preferences.DefaultLibrarySort != "title_asc" || preferences.PreferredView != "list" {
		t.Fatalf("save library preferences = (%#v, %v)", preferences, err)
	}
	restarted := media.NewPreferencesService(databasePool)
	preferences, err = restarted.Get(ctx, userID)
	if err != nil || preferences.DefaultLibrarySort != "title_asc" || preferences.PreferredView != "list" {
		t.Fatalf("library preferences after service restart = (%#v, %v)", preferences, err)
	}
}

func fixtureGame(providerID int64, title string) igdb.Game {
	year := 2026
	rating, count := int16(88), int32(500)
	return igdb.Game{
		ProviderID: providerID, Title: title, Summary: "A deterministic game fixture.", Year: &year,
		GameType: "main_game", Developer: "Fixture Studio", Publisher: "Fixture Publisher",
		Genres: []string{"Adventure"}, GameModes: []string{"Single player"}, Platforms: []string{"PC"},
		CommunityRating: &rating, CommunityRatingCount: &count,
		Artworks: []media.Artwork{
			{ProviderImageID: "game-cover-default", Kind: "cover", ImageURL: "https://images.example/game-default.jpg", ThumbnailURL: "https://images.example/game-default-thumb.jpg", Preferred: true},
			{ProviderImageID: "game-cover-manual", Kind: "cover", ImageURL: "https://images.example/game-manual.jpg", ThumbnailURL: "https://images.example/game-manual-thumb.jpg"},
		},
		AdditionalContent: []igdb.AdditionalContent{{ProviderID: providerID + 1000, Title: "Story Expansion", Type: "expansion"}},
		RelatedReleases:   []igdb.RelatedRelease{{ProviderID: providerID + 2000, Title: "Foundation Remaster", Relationship: "remaster"}},
	}
}

func fixtureMovie() tmdb.Movie {
	year, runtime, rating, count, collectionID := 2024, int32(124), int16(79), int32(900), int64(7001)
	return tmdb.Movie{
		ProviderID: 701, Title: "Foundation Movie", Overview: "A deterministic movie fixture.", Year: &year,
		RuntimeMinutes: &runtime, Director: "Fixture Director", Genres: []string{"Drama"}, ProductionCompanies: []string{"Fixture Films"},
		CollectionID: &collectionID, CollectionName: "Foundation Collection", CommunityRating: &rating, CommunityRatingCount: &count,
		Collection: []tmdb.CollectionMember{{ProviderID: 701, Title: "Foundation Movie"}, {ProviderID: 702, Title: "Foundation Movie II"}},
		Artworks: []media.Artwork{
			{ProviderImageID: "movie-poster-default", Kind: "poster", ImageURL: "https://images.example/movie-default.jpg", ThumbnailURL: "https://images.example/movie-default-thumb.jpg", Preferred: true},
			{ProviderImageID: "movie-poster-manual", Kind: "poster", ImageURL: "https://images.example/movie-manual.jpg", ThumbnailURL: "https://images.example/movie-manual-thumb.jpg"},
		},
	}
}

func fixtureShow(title string) tvdb.Show {
	year := 2025
	return tvdb.Show{
		ProviderID: 501, Title: title, Overview: "A deterministic TV fixture.", Year: &year, ProviderStatus: "Continuing", Network: "Fixture Network", Genres: []string{"Drama"},
		Artworks: []media.Artwork{
			{ProviderImageID: "tv-poster-default", Kind: "poster", ImageURL: "https://images.example/tv-default.jpg", ThumbnailURL: "https://images.example/tv-default-thumb.jpg", Preferred: true},
			{ProviderImageID: "tv-poster-manual", Kind: "poster", ImageURL: "https://images.example/tv-manual.jpg", ThumbnailURL: "https://images.example/tv-manual-thumb.jpg"},
		},
		Seasons: []tvdb.Season{
			{ProviderID: 510, Number: 0, Name: "Specials", Special: true, Episodes: []tvdb.Episode{{ProviderID: 5101, SeasonNumber: 0, EpisodeNumber: 1, SortOrder: 0, Title: "Special One", Special: true}}},
			{ProviderID: 511, Number: 1, Name: "Season 1", Episodes: []tvdb.Episode{
				{ProviderID: 5111, SeasonNumber: 1, EpisodeNumber: 1, SortOrder: 1, Title: "Pilot"},
				{ProviderID: 5112, SeasonNumber: 1, EpisodeNumber: 2, SortOrder: 2, Title: "Second"},
			}},
		},
	}
}

func assertUUIDv7(t *testing.T, ctx context.Context, databasePool *pgxpool.Pool, id string) {
	t.Helper()
	var version int
	if err := databasePool.QueryRow(ctx, `SELECT uuid_extract_version($1::uuid)`, id).Scan(&version); err != nil || version != 7 {
		t.Fatalf("UUID %q version = (%d, %v), want 7", id, version, err)
	}
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
