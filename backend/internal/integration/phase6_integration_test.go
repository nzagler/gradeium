package integration_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/nzagler/gradeium/backend/internal/backups"
	"github.com/nzagler/gradeium/backend/internal/dashboard"
	"github.com/nzagler/gradeium/backend/internal/database"
	"github.com/nzagler/gradeium/backend/internal/media"
	tvdomain "github.com/nzagler/gradeium/backend/internal/tv"
)

func TestPhase5DatabaseMigratesToPhase6ThemeAndPortablePreferences(t *testing.T) {
	baseURL := os.Getenv("GRADEIUM_TEST_DATABASE_URL")
	if baseURL == "" {
		t.Skip("GRADEIUM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	testURL := createTestDatabase(t, ctx, baseURL)
	applyMigrationsThrough(t, ctx, testURL, 5)
	pool, err := database.Open(ctx, testURL)
	if err != nil {
		t.Fatalf("open Phase 5 database: %v", err)
	}
	var userID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users(oidc_issuer,oidc_subject,display_name,is_admin)
		VALUES('https://id.example','phase6-admin','Phase 6 Admin',true)
		RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatalf("create Phase 5 administrator: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_settings(user_id,default_library_sort,preferred_view) VALUES($1,'title_asc','list')`, userID); err != nil {
		t.Fatalf("create Phase 5 preferences: %v", err)
	}
	pool.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := database.Migrate(ctx, testURL, logger); err != nil {
		t.Fatalf("migrate Phase 5 database to Phase 6: %v", err)
	}
	pool, err = database.Open(ctx, testURL)
	if err != nil {
		t.Fatalf("reopen Phase 6 database: %v", err)
	}
	defer pool.Close()

	preferences, err := media.NewPreferencesService(pool).Get(ctx, userID)
	if err != nil {
		t.Fatalf("read migrated preferences: %v", err)
	}
	if preferences.DefaultLibrarySort != "title_asc" || preferences.PreferredView != "list" || preferences.Theme != "dark" {
		t.Fatalf("migrated preferences = %#v", preferences)
	}

	preferences.Theme = "system"
	if _, err := media.NewPreferencesService(pool).Update(ctx, userID, preferences); err != nil {
		t.Fatalf("save Phase 6 theme: %v", err)
	}
	store := backups.NewPostgresStore(pool)
	document, err := store.Snapshot(ctx, "1.0.0")
	if err != nil {
		t.Fatalf("snapshot Phase 6 preferences: %v", err)
	}
	if len(document.Users) != 1 || document.Users[0].Preferences.Theme != "system" {
		t.Fatalf("portable theme = %#v", document.Users)
	}
	if _, err := pool.Exec(ctx, `UPDATE user_settings SET theme='light' WHERE user_id=$1`, userID); err != nil {
		t.Fatalf("change theme before restore: %v", err)
	}
	if err := store.Restore(ctx, document); err != nil {
		t.Fatalf("restore Phase 6 preferences: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT theme FROM user_settings WHERE user_id=$1`, userID).Scan(&preferences.Theme); err != nil || preferences.Theme != "system" {
		t.Fatalf("restored theme = (%q, %v)", preferences.Theme, err)
	}

	// Phase 5 backups had no theme field. They remain restorable and acquire
	// Gradeium's default-dark preference.
	document.Users[0].Preferences.Theme = ""
	if err := store.Restore(ctx, document); err != nil {
		t.Fatalf("restore legacy Phase 5 preferences: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT theme FROM user_settings WHERE user_id=$1`, userID).Scan(&preferences.Theme); err != nil || preferences.Theme != "dark" {
		t.Fatalf("legacy restored theme = (%q, %v)", preferences.Theme, err)
	}
}

func TestFreshPhase6MigrationsDefaultNewUsersToDark(t *testing.T) {
	baseURL := os.Getenv("GRADEIUM_TEST_DATABASE_URL")
	if baseURL == "" {
		t.Skip("GRADEIUM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	testURL := createTestDatabase(t, ctx, baseURL)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := database.Migrate(ctx, testURL, logger); err != nil {
		t.Fatalf("apply fresh Phase 6 migrations: %v", err)
	}
	pool, err := database.Open(ctx, testURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var theme string
	if err := pool.QueryRow(ctx, `
		WITH new_user AS (INSERT INTO users(display_name) VALUES('Fresh User') RETURNING id)
		INSERT INTO user_settings(user_id) SELECT id FROM new_user RETURNING theme`).Scan(&theme); err != nil {
		t.Fatalf("create fresh user preferences: %v", err)
	}
	if theme != "dark" {
		t.Fatalf("fresh theme = %q, want dark", theme)
	}
}

func TestRealisticPersonalLibraryPerformanceSanity(t *testing.T) {
	baseURL := os.Getenv("GRADEIUM_TEST_DATABASE_URL")
	if baseURL == "" {
		t.Skip("GRADEIUM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	testURL := createTestDatabase(t, ctx, baseURL)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := database.Migrate(ctx, testURL, logger); err != nil {
		t.Fatal(err)
	}
	pool, err := database.Open(ctx, testURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	var userID string
	if err := pool.QueryRow(ctx, `INSERT INTO users(display_name) VALUES('Performance User') RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	seedStarted := time.Now()
	for _, statement := range []string{
		`WITH created AS (
			INSERT INTO entities(type) SELECT 'game' FROM generate_series(1,334) WHERE $1::text <> '' RETURNING id
		), numbered AS (SELECT id,row_number() OVER () AS n FROM created)
		INSERT INTO games(entity_id,igdb_id,english_title,game_type,release_year)
		SELECT id,100000+n,'Game '||n,'main_game',2000+(n%25) FROM numbered`,
		`INSERT INTO user_games(user_id,game_id,status,rating)
		SELECT $1,entity_id,'completed',10+(igdb_id%91)::smallint FROM games`,
		`WITH created AS (
			INSERT INTO entities(type) SELECT 'movie' FROM generate_series(1,333) WHERE $1::text <> '' RETURNING id
		), numbered AS (SELECT id,row_number() OVER () AS n FROM created)
		INSERT INTO movies(entity_id,tmdb_id,english_title,release_year,runtime_minutes)
		SELECT id,200000+n,'Movie '||n,1980+(n%45),80+(n%80) FROM numbered`,
		`INSERT INTO user_movies(user_id,movie_id,status,rating)
		SELECT $1,entity_id,'in_progress',10+(tmdb_id%91)::smallint FROM movies`,
		`WITH created AS (
			INSERT INTO entities(type) SELECT 'tv_show' FROM generate_series(1,333) WHERE $1::text <> '' RETURNING id
		), numbered AS (SELECT id,row_number() OVER () AS n FROM created)
		INSERT INTO tv_shows(entity_id,tvdb_id,english_title,release_year)
		SELECT id,300000+n,'TV Show '||n,1990+(n%35) FROM numbered`,
		`INSERT INTO user_tv_shows(user_id,tv_show_id,status,rating)
		SELECT $1,entity_id,'in_progress',10+(tvdb_id%91)::smallint FROM tv_shows`,
		`WITH numbered AS (SELECT entity_id,row_number() OVER (ORDER BY entity_id) AS n FROM tv_shows)
		INSERT INTO tv_seasons(tv_show_id,tvdb_season_id,season_number,name,is_specials)
		SELECT entity_id,400000+n,1,'Season 1',false FROM numbered WHERE $1::text <> ''`,
		`WITH numbered AS (
			SELECT s.id,s.tv_show_id,row_number() OVER (ORDER BY s.tv_show_id) AS n FROM tv_seasons s
		)
		INSERT INTO tv_episodes(tv_show_id,season_id,tvdb_episode_id,season_number,episode_number,sort_order,english_title,is_special)
		SELECT n.tv_show_id,n.id,500000+(n.n*100)+episode,1,episode,episode,'Episode '||episode,false
		FROM numbered n CROSS JOIN generate_series(1,12) AS episode WHERE $1::text <> ''`,
		`INSERT INTO user_episode_progress(user_id,tv_show_id,episode_id)
		SELECT $1,tv_show_id,id FROM tv_episodes WHERE episode_number <= 6`,
	} {
		if _, err := pool.Exec(ctx, statement, userID); err != nil {
			t.Fatalf("seed realistic fixture: %v", err)
		}
	}
	t.Logf("seeded 1,000 media items and 3,996 episodes in %s", time.Since(seedStarted))

	dashboardStarted := time.Now()
	summary, err := dashboard.NewService(pool).Summary(ctx, userID, dashboard.ScopeAll)
	if err != nil {
		t.Fatalf("dashboard summary: %v", err)
	}
	if summary.Totals["games"].Tracked+summary.Totals["movies"].Tracked+summary.Totals["tv"].Tracked != 1000 {
		t.Fatalf("dashboard tracked totals = %#v", summary.Totals)
	}
	t.Logf("rendered realistic Dashboard aggregate in %s", time.Since(dashboardStarted))

	var showID string
	if err := pool.QueryRow(ctx, `SELECT entity_id::text FROM tv_shows ORDER BY entity_id LIMIT 1`).Scan(&showID); err != nil {
		t.Fatal(err)
	}
	detailStarted := time.Now()
	detail, err := tvdomain.NewPostgresStore(pool).Detail(ctx, userID, showID)
	if err != nil || len(detail.Seasons) != 1 || len(detail.Seasons[0].Episodes) != 12 || detail.Progress.Watched != 6 {
		t.Fatalf("realistic TV detail = (%#v, %v)", detail.Progress, err)
	}
	t.Logf("loaded TV detail/progress in %s", time.Since(detailStarted))

	backupStarted := time.Now()
	document, err := backups.NewPostgresStore(pool).Snapshot(ctx, "1.0.0")
	if err != nil {
		t.Fatalf("snapshot realistic library: %v", err)
	}
	if len(document.Games)+len(document.Movies)+len(document.TVShows) != 1000 || len(document.EpisodeProgress) != 1998 {
		t.Fatalf("realistic backup counts = games %d movies %d TV %d progress %d", len(document.Games), len(document.Movies), len(document.TVShows), len(document.EpisodeProgress))
	}
	if err := backups.Validate(document); err != nil {
		t.Fatalf("validate realistic backup: %v", err)
	}
	t.Logf("snapshotted and validated realistic library in %s", time.Since(backupStarted))
}
