package integration_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nzagler/gradeium/backend/internal/backups"
	"github.com/nzagler/gradeium/backend/internal/dashboard"
	"github.com/nzagler/gradeium/backend/internal/database"
	"github.com/nzagler/gradeium/backend/internal/games"
	"github.com/nzagler/gradeium/backend/internal/media"
	"github.com/nzagler/gradeium/backend/internal/movies"
	"github.com/nzagler/gradeium/backend/internal/secrets"
	"github.com/nzagler/gradeium/backend/internal/settings"
	tvdomain "github.com/nzagler/gradeium/backend/internal/tv"
)

func TestPhase4DatabaseMigratesToPhase5DashboardAndPortableBackups(t *testing.T) {
	baseURL := os.Getenv("GRADEIUM_TEST_DATABASE_URL")
	if baseURL == "" {
		t.Skip("GRADEIUM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	testURL := createTestDatabase(t, ctx, baseURL)
	applyMigrationsThrough(t, ctx, testURL, 4)
	pool, err := database.Open(ctx, testURL)
	if err != nil {
		t.Fatalf("open Phase 4 database: %v", err)
	}

	var userID, secondUserID string
	if err := pool.QueryRow(ctx, `INSERT INTO users(oidc_issuer,oidc_subject,display_name,is_admin) VALUES('https://id.example','phase5-admin','Phase 5 Admin',true) RETURNING id::text`).Scan(&userID); err != nil {
		t.Fatalf("create Phase 4 administrator: %v", err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO users(oidc_issuer,oidc_subject,display_name) VALUES('https://id.example','other-user','Other User') RETURNING id::text`).Scan(&secondUserID); err != nil {
		t.Fatalf("create second user: %v", err)
	}
	sessionHash := sha256.Sum256([]byte("phase-5-session-material-must-not-be-backed-up"))
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(user_id,token_hash,expires_at) VALUES($1,$2,now()+interval '1 hour')`, userID, sessionHash[:]); err != nil {
		t.Fatalf("create established session: %v", err)
	}
	registry := settings.NewRegistry()
	settingsService := settings.NewService(registry, settings.NewPostgresStore(pool))
	if _, err := settingsService.Update(ctx, settings.InstanceNameKey, []byte(`"Portable Gradeium"`)); err != nil {
		t.Fatalf("save portable instance name: %v", err)
	}
	preferenceService := media.NewPreferencesService(pool)
	if _, err := preferenceService.Update(ctx, userID, media.Preferences{DefaultLibrarySort: "title_asc", PreferredView: "list"}); err != nil {
		t.Fatalf("save portable Library defaults: %v", err)
	}
	secretDirectory := t.TempDir()
	secretStore := secrets.NewPostgresStore(pool)
	cipher, err := secrets.InitializeCipher(ctx, secretDirectory, secretStore)
	if err != nil {
		t.Fatalf("initialize secret cipher: %v", err)
	}
	secretService := secrets.NewService(registry, secretStore, cipher)
	secretPlaintext := "phase-5-provider-secret-must-never-enter-backup"
	if err := secretService.Set(ctx, settings.TMDBAccessTokenKey, secretPlaintext); err != nil {
		t.Fatalf("save encrypted provider secret: %v", err)
	}
	var ciphertextBefore []byte
	if err := pool.QueryRow(ctx, `SELECT ciphertext FROM secret_settings WHERE key=$1`, settings.TMDBAccessTokenKey).Scan(&ciphertextBefore); err != nil {
		t.Fatalf("read encrypted provider secret: %v", err)
	}

	gameStore := games.NewPostgresStore(pool)
	game := fixtureGame(8101, `Comma, Quote " Game`)
	gameDetail, err := gameStore.Add(ctx, userID, game, media.StatusInProgress)
	if err != nil {
		t.Fatalf("add Phase 4 game: %v", err)
	}
	gameRating, gameReason := int16(87), "Line one, quoted \"reason\"\nLine two"
	if _, err := gameStore.UpdateState(ctx, userID, gameDetail.ID, media.PersonalState{Status: media.StatusCompleted, Rating: &gameRating, RatingReason: &gameReason}, false); err != nil {
		t.Fatalf("rate Phase 4 game: %v", err)
	}
	if _, err := gameStore.SelectArtwork(ctx, userID, gameDetail.ID, "cover", "game-cover-manual"); err != nil {
		t.Fatalf("pin Phase 4 game artwork: %v", err)
	}
	if _, err := gameStore.Add(ctx, secondUserID, fixtureGame(8102, "Other User Game"), media.StatusBacklog); err != nil {
		t.Fatalf("add isolated second-user game: %v", err)
	}

	movieStore := movies.NewPostgresStore(pool)
	movieDetail, err := movieStore.Add(ctx, userID, fixtureMovie(), media.StatusInProgress)
	if err != nil {
		t.Fatalf("add Phase 4 movie: %v", err)
	}
	movieRating := int16(75)
	if _, err := movieStore.UpdateState(ctx, userID, movieDetail.ID, media.PersonalState{Status: media.StatusCompleted, Rating: &movieRating}, false); err != nil {
		t.Fatalf("rate Phase 4 movie: %v", err)
	}

	tvStore := tvdomain.NewPostgresStore(pool)
	tvDetail, err := tvStore.Add(ctx, userID, fixtureShow("Phase 5 TV"), nil, media.StatusInProgress)
	if err != nil {
		t.Fatalf("add Phase 4 TV show: %v", err)
	}
	var regularEpisodeID, specialEpisodeID string
	for _, season := range tvDetail.Seasons {
		for _, episode := range season.Episodes {
			if episode.Special {
				specialEpisodeID = episode.ID
			} else if regularEpisodeID == "" {
				regularEpisodeID = episode.ID
			}
		}
	}
	if _, err := tvStore.SetEpisode(ctx, userID, tvDetail.ID, regularEpisodeID, true); err != nil {
		t.Fatalf("track regular TV progress: %v", err)
	}
	if _, err := tvStore.SetEpisode(ctx, userID, tvDetail.ID, specialEpisodeID, true); err != nil {
		t.Fatalf("track Specials progress: %v", err)
	}
	tvRating := int16(92)
	if _, err := tvStore.UpdateState(ctx, userID, tvDetail.ID, media.PersonalState{Status: media.StatusInProgress, Rating: &tvRating}, false); err != nil {
		t.Fatalf("rate Phase 4 TV show: %v", err)
	}
	pool.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := database.Migrate(ctx, testURL, logger); err != nil {
		t.Fatalf("migrate Phase 4 database to Phase 5: %v", err)
	}
	pool, err = database.Open(ctx, testURL)
	if err != nil {
		t.Fatalf("open Phase 5 database: %v", err)
	}
	t.Cleanup(pool.Close)

	dashboardService := dashboard.NewService(pool)
	summary, err := dashboardService.Summary(ctx, userID, dashboard.ScopeAll)
	if err != nil {
		t.Fatalf("read Phase 5 Dashboard: %v", err)
	}
	if summary.Totals["games"].Tracked != 1 || summary.Totals["movies"].Tracked != 1 || summary.Totals["tv"].Tracked != 1 || summary.Totals["games"].Backlog != 0 {
		t.Fatalf("user-scoped Dashboard totals = %#v", summary.Totals)
	}
	if summary.AverageRating == nil || *summary.AverageRating < 8.4 || *summary.AverageRating > 8.5 || len(summary.TVProgress) != 1 || summary.TVProgress[0].Watched == nil || *summary.TVProgress[0].Watched != 1 {
		t.Fatalf("Dashboard averages/progress = %#v", summary)
	}
	csvBytes, err := dashboardService.RatingsCSV(ctx, userID)
	if err != nil {
		t.Fatalf("create ratings CSV: %v", err)
	}
	records, err := csv.NewReader(bytes.NewReader(csvBytes)).ReadAll()
	if err != nil || len(records) != 4 || records[1][3] != game.Title || records[1][7] != gameReason {
		t.Fatalf("ratings CSV round trip = (%#v, %v)", records, err)
	}

	backupDirectory := t.TempDir()
	backupStore := backups.NewPostgresStore(pool)
	backupService := backups.NewService(backupStore, backupDirectory, "phase5-integration")
	manual, err := backupService.Create(ctx, backups.KindManual)
	if err != nil {
		t.Fatalf("create atomic portable backup: %v", err)
	}
	backupBytes, err := os.ReadFile(filepath.Join(backupDirectory, manual.Filename))
	if err != nil {
		t.Fatalf("read portable backup: %v", err)
	}
	if bytes.Contains(backupBytes, []byte(secretPlaintext)) || bytes.Contains(backupBytes, sessionHash[:]) {
		t.Fatal("compressed portable backup contains secret or session material")
	}
	document, err := backups.Decode(bytes.NewReader(backupBytes))
	if err != nil {
		t.Fatalf("decode created portable backup: %v", err)
	}
	var plain bytes.Buffer
	if err := backups.Encode(&plain, document); err != nil {
		t.Fatalf("re-encode portable backup: %v", err)
	}
	if strings.Contains(string(mustGunzip(t, backupBytes)), secretPlaintext) || strings.Contains(string(mustGunzip(t, backupBytes)), "secret_settings") || strings.Contains(string(mustGunzip(t, backupBytes)), "authentication.active") {
		t.Fatal("portable backup JSON contains forbidden secret/authentication material")
	}

	if _, err := pool.Exec(ctx, `UPDATE user_games SET rating=10,rating_reason=NULL WHERE user_id=$1`, userID); err != nil {
		t.Fatalf("mutate game state before restore: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM user_episode_progress WHERE user_id=$1`, userID); err != nil {
		t.Fatalf("mutate TV progress before restore: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE app_settings SET value='"Mutated"' WHERE key='general.instance_name'`); err != nil {
		t.Fatalf("mutate portable setting before restore: %v", err)
	}
	safety, err := backupService.Restore(ctx, manual.ID)
	if err != nil {
		t.Fatalf("restore portable backup transactionally: %v", err)
	}
	if safety.Kind != backups.KindPreRestore {
		t.Fatalf("restore safety backup kind = %q", safety.Kind)
	}
	restoredGame, err := games.NewPostgresStore(pool).Detail(ctx, userID, gameDetail.ID)
	if err != nil || restoredGame.State.Rating == nil || *restoredGame.State.Rating != gameRating || restoredGame.ArtworkPins["cover"] != "game-cover-manual" {
		t.Fatalf("restored Game state = (%#v, %v)", restoredGame, err)
	}
	restoredMovie, err := movies.NewPostgresStore(pool).Detail(ctx, userID, movieDetail.ID)
	if err != nil || restoredMovie.State.Rating == nil || *restoredMovie.State.Rating != movieRating {
		t.Fatalf("restored Movie state = (%#v, %v)", restoredMovie, err)
	}
	restoredTV, err := tvdomain.NewPostgresStore(pool).Detail(ctx, userID, tvDetail.ID)
	if err != nil || restoredTV.Progress.Watched != 1 || restoredTV.Progress.SpecialsWatched != 1 {
		t.Fatalf("restored TV regular/Specials progress = (%#v, %v)", restoredTV.Progress, err)
	}
	preferences, err := media.NewPreferencesService(pool).Get(ctx, userID)
	if err != nil || preferences.DefaultLibrarySort != "title_asc" || preferences.PreferredView != "list" {
		t.Fatalf("restored Library defaults = (%#v, %v)", preferences, err)
	}
	var administrator bool
	var sessionCount int
	var ciphertextAfter []byte
	if err := pool.QueryRow(ctx, `SELECT is_admin FROM users WHERE id=$1`, userID).Scan(&administrator); err != nil || !administrator {
		t.Fatalf("current administrator after restore = (%v, %v)", administrator, err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE token_hash=$1`, sessionHash[:]).Scan(&sessionCount); err != nil || sessionCount != 1 {
		t.Fatalf("current session after restore = (%d, %v)", sessionCount, err)
	}
	if err := pool.QueryRow(ctx, `SELECT ciphertext FROM secret_settings WHERE key=$1`, settings.TMDBAccessTokenKey).Scan(&ciphertextAfter); err != nil || !bytes.Equal(ciphertextBefore, ciphertextAfter) {
		t.Fatalf("encrypted provider secret changed during restore: %v", err)
	}

	assertRestoreRollback(t, ctx, pool, backupStore, document, userID)
	if _, err := backupService.RestoreUpload(ctx, strings.NewReader("not a gzip backup")); !errors.Is(err, backups.ErrInvalidBackup) {
		t.Fatalf("malformed restore error = %v, want ErrInvalidBackup", err)
	}
	oversized := io.LimitReader(repeatingByteReader{}, backups.MaxCompressedSize+1)
	if _, err := backupService.RestoreUpload(ctx, oversized); !errors.Is(err, backups.ErrInvalidBackup) || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("oversized restore error = %v, want bounded ErrInvalidBackup", err)
	}
	assertSchedulerAndRetention(t, ctx, pool, backupService, backupDirectory, logger)
	assertConcurrentBackupSerialization(t, ctx, backupService)
	if temporary, err := filepath.Glob(filepath.Join(backupDirectory, ".gradeium-*.tmp")); err != nil || len(temporary) != 0 {
		t.Fatalf("temporary backup files after completed operations = (%v, %v)", temporary, err)
	}
}

type repeatingByteReader struct{}

func (repeatingByteReader) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = 'x'
	}
	return len(buffer), nil
}

func TestPhase5FreshMigrations(t *testing.T) {
	baseURL := os.Getenv("GRADEIUM_TEST_DATABASE_URL")
	if baseURL == "" {
		t.Skip("GRADEIUM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	testURL := createTestDatabase(t, ctx, baseURL)
	if err := database.Migrate(ctx, testURL, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("fresh Phase 5 migrations: %v", err)
	}
	pool, err := database.Open(ctx, testURL)
	if err != nil {
		t.Fatalf("open fresh Phase 5 database: %v", err)
	}
	defer pool.Close()
	settings, err := backups.NewPostgresStore(pool).BackupSettings(ctx)
	if err != nil || !settings.Enabled || settings.IntervalDays != 3 || settings.RetentionCount != 30 {
		t.Fatalf("fresh backup settings = (%#v, %v)", settings, err)
	}
}

func assertRestoreRollback(t *testing.T, ctx context.Context, pool *pgxpool.Pool, store *backups.PostgresStore, document backups.Document, userID string) {
	t.Helper()
	var before int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM user_games WHERE user_id=$1`, userID).Scan(&before); err != nil {
		t.Fatalf("count games before forced restore failure: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE FUNCTION gradeium_test_restore_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced restore failure'; END $$`); err != nil {
		t.Fatalf("install forced restore failure: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE TRIGGER gradeium_test_restore_failure BEFORE INSERT ON games FOR EACH ROW EXECUTE FUNCTION gradeium_test_restore_failure()`); err != nil {
		t.Fatalf("install forced restore trigger: %v", err)
	}
	if err := store.Restore(ctx, document); err == nil {
		t.Fatal("Restore() succeeded despite forced database failure")
	}
	var after int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM user_games WHERE user_id=$1`, userID).Scan(&after); err != nil || after != before {
		t.Fatalf("personal state after failed restore = (%d, %v), want %d", after, err, before)
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER gradeium_test_restore_failure ON games`); err != nil {
		t.Fatalf("remove forced restore failure: %v", err)
	}
	if _, err := pool.Exec(ctx, `DROP FUNCTION gradeium_test_restore_failure()`); err != nil {
		t.Fatalf("remove forced restore function: %v", err)
	}
}

func assertSchedulerAndRetention(t *testing.T, ctx context.Context, pool *pgxpool.Pool, service *backups.Service, directory string, logger *slog.Logger) {
	t.Helper()
	if _, err := service.UpdateSettings(ctx, backups.Settings{Enabled: true, IntervalDays: 1, RetentionCount: 1}); err != nil {
		t.Fatalf("persist scheduler settings: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE backup_settings SET schedule_anchor_at=now()-interval '2 days' WHERE singleton`); err != nil {
		t.Fatalf("make automatic backup overdue: %v", err)
	}
	if err := backups.NewScheduler(service, logger).RunOnce(ctx); err != nil {
		t.Fatalf("run overdue automatic backup: %v", err)
	}
	settings, err := service.Settings(ctx)
	if err != nil || settings.LastSuccessfulAutomaticAt == nil || settings.NextDueAt == nil {
		t.Fatalf("scheduler persistence = (%#v, %v)", settings, err)
	}
	if _, err := service.Create(ctx, backups.KindAutomatic); err != nil {
		t.Fatalf("create second automatic backup: %v", err)
	}
	if err := service.ApplyRetention(ctx, 1); err != nil {
		t.Fatalf("apply automatic retention: %v", err)
	}
	items, err := service.List(ctx)
	if err != nil {
		t.Fatalf("list backups after retention: %v", err)
	}
	automatic, protected := 0, 0
	for _, item := range items {
		if item.Kind == backups.KindAutomatic {
			automatic++
		} else {
			protected++
		}
	}
	if automatic != 1 || protected == 0 {
		t.Fatalf("retention inventory = %d automatic, %d protected", automatic, protected)
	}
	_ = directory
}

func assertConcurrentBackupSerialization(t *testing.T, ctx context.Context, service *backups.Service) {
	t.Helper()
	var wait sync.WaitGroup
	errorsChannel := make(chan error, 2)
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := service.Create(ctx, backups.KindManual)
			errorsChannel <- err
		}()
	}
	wait.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		if err != nil {
			t.Fatalf("serialized concurrent backup: %v", err)
		}
	}
}

func mustGunzip(t *testing.T, value []byte) []byte {
	t.Helper()
	reader, err := gzip.NewReader(bytes.NewReader(value))
	if err != nil {
		t.Fatalf("open backup gzip: %v", err)
	}
	defer reader.Close()
	result, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read backup gzip: %v", err)
	}
	return result
}
