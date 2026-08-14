package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nzagler/gradeium/backend/internal/auth"
	"github.com/nzagler/gradeium/backend/internal/database"
	"github.com/nzagler/gradeium/backend/internal/secrets"
	"github.com/nzagler/gradeium/backend/internal/settings"
	"github.com/nzagler/gradeium/backend/internal/setup"
	"github.com/nzagler/gradeium/backend/migrations"
	"github.com/pressly/goose/v3"
)

func TestPhase2DatabaseMigratesToPhase3Authentication(t *testing.T) {
	baseURL := os.Getenv("GRADEIUM_TEST_DATABASE_URL")
	if baseURL == "" {
		t.Skip("GRADEIUM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	testURL := createTestDatabase(t, ctx, baseURL)
	applyMigrationsThrough(t, ctx, testURL, 2)

	pool, err := database.Open(ctx, testURL)
	if err != nil {
		t.Fatalf("open Phase 2 database: %v", err)
	}
	registry := settings.NewRegistry()
	settingsService := settings.NewService(registry, settings.NewPostgresStore(pool))
	if _, err := settingsService.Update(ctx, settings.InstanceNameKey, []byte(`"Migrated Gradeium"`)); err != nil {
		t.Fatalf("persist Phase 2 setting: %v", err)
	}
	setupService := setup.NewService(setup.NewPostgresStore(pool))
	if transitioned, err := setupService.Complete(ctx); err != nil || !transitioned {
		t.Fatalf("complete Phase 2 setup = (%v, %v)", transitioned, err)
	}
	configDirectory := t.TempDir()
	secretStore := secrets.NewPostgresStore(pool)
	cipher, err := secrets.InitializeCipher(ctx, configDirectory, secretStore)
	if err != nil {
		t.Fatalf("initialize Phase 2 master key: %v", err)
	}
	secretService := secrets.NewService(registry, secretStore, cipher)
	if err := secretService.Set(ctx, settings.AuthenticationClientSecretKey, "phase-2-reserved-secret"); err != nil {
		t.Fatalf("persist Phase 2 reserved secret: %v", err)
	}
	pool.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := database.Migrate(ctx, testURL, logger); err != nil {
		t.Fatalf("migrate Phase 2 database to Phase 3: %v", err)
	}
	pool, err = database.Open(ctx, testURL)
	if err != nil {
		t.Fatalf("open Phase 3 database: %v", err)
	}
	t.Cleanup(pool.Close)

	values, err := settings.NewService(registry, settings.NewPostgresStore(pool)).List(ctx)
	if err != nil || len(values) == 0 || string(values[0].Value) != `"Migrated Gradeium"` {
		t.Fatalf("Phase 2 setting after migration = (%#v, %v)", values, err)
	}
	reloadedCipher, err := secrets.InitializeCipher(ctx, configDirectory, secrets.NewPostgresStore(pool))
	if err != nil {
		t.Fatalf("reload Phase 2 key after migration: %v", err)
	}
	reloadedSecrets := secrets.NewService(registry, secrets.NewPostgresStore(pool), reloadedCipher)
	plaintext, err := reloadedSecrets.Read(ctx, settings.AuthenticationClientSecretKey)
	if err != nil || string(plaintext) != "phase-2-reserved-secret" {
		t.Fatalf("Phase 2 encrypted secret after migration = (%q, %v)", plaintext, err)
	}
	secrets.Clear(plaintext)

	authStore := auth.NewPostgresStore(pool)
	draft, err := authStore.LoadDraft(ctx)
	if err != nil {
		t.Fatalf("load fresh Phase 3 authentication draft: %v", err)
	}
	configuration := auth.Configuration{
		IssuerURL: "https://id.example", ClientID: "gradeium-client", PublicURL: "https://gradeium.example",
	}
	revision, err := authStore.SaveDraft(ctx, draft.Revision, configuration, auth.KeepSecret, nil, true)
	if err != nil {
		t.Fatalf("save validated authentication draft: %v", err)
	}
	activeSecret, err := reloadedSecrets.Seal(settings.AuthenticationActiveClientSecretKey, "phase-2-reserved-secret")
	if err != nil {
		t.Fatalf("seal active OIDC secret: %v", err)
	}
	assertActivationConcurrency(t, ctx, authStore, revision, configuration, &activeSecret)

	assertFirstAdminAndSessionConcurrency(t, ctx, pool, authStore)
	assertSessionPersistenceAndRevocation(t, ctx, pool, authStore)
	assertSessionExpiration(t, ctx, pool, authStore)
	assertOIDCIdentityUniqueness(t, ctx, pool)
}

func assertActivationConcurrency(
	t *testing.T,
	ctx context.Context,
	store *auth.PostgresStore,
	revision int64,
	configuration auth.Configuration,
	activeSecret *secrets.Record,
) {
	t.Helper()
	const attempts = 8
	var activations atomic.Int32
	var waitGroup sync.WaitGroup
	for range attempts {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			activated, err := store.Activate(ctx, revision, configuration, activeSecret)
			if err != nil {
				t.Errorf("concurrent authentication activation: %v", err)
				return
			}
			if activated {
				activations.Add(1)
			}
		}()
	}
	waitGroup.Wait()
	if activations.Load() != 1 {
		t.Fatalf("successful authentication activations = %d, want 1", activations.Load())
	}
	state, err := store.State(ctx)
	if err != nil || !state.Activated || state.ActiveRevision == nil || *state.ActiveRevision != revision {
		t.Fatalf("durable authentication state = (%#v, %v)", state, err)
	}
}

func applyMigrationsThrough(t *testing.T, ctx context.Context, databaseURL string, version int64) {
	t.Helper()
	databaseHandle, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatalf("open migration database: %v", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, databaseHandle, migrations.Files)
	if err != nil {
		_ = databaseHandle.Close()
		t.Fatalf("create migration provider: %v", err)
	}
	if _, err := provider.UpTo(ctx, version); err != nil {
		_ = provider.Close()
		t.Fatalf("apply migrations through %d: %v", version, err)
	}
	if err := provider.Close(); err != nil {
		t.Fatalf("close migration provider: %v", err)
	}
}

func assertFirstAdminAndSessionConcurrency(t *testing.T, ctx context.Context, pool *pgxpool.Pool, store *auth.PostgresStore) {
	t.Helper()
	const logins = 12
	var administrators atomic.Int32
	var waitGroup sync.WaitGroup
	for index := range logins {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			hash := sha256.Sum256([]byte(fmt.Sprintf("session-token-%d", index)))
			name := fmt.Sprintf("User %d", index)
			user, err := store.CompleteLogin(ctx, auth.Identity{
				Issuer: "https://id.example", Subject: fmt.Sprintf("subject-%d", index), DisplayName: &name,
			}, hash, time.Now().Add(time.Hour), nil)
			if err != nil {
				t.Errorf("concurrent CompleteLogin %d: %v", index, err)
				return
			}
			if user.IsAdmin {
				administrators.Add(1)
			}
		}(index)
	}
	waitGroup.Wait()
	if administrators.Load() != 1 {
		t.Fatalf("first-login administrator results = %d, want 1", administrators.Load())
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE is_admin`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("database administrator count = (%d, %v), want 1", count, err)
	}
}

func assertSessionPersistenceAndRevocation(t *testing.T, ctx context.Context, pool *pgxpool.Pool, store *auth.PostgresStore) {
	t.Helper()
	rawToken := "opaque-browser-session-token-not-in-database"
	hash := sha256.Sum256([]byte(rawToken))
	user, err := store.CompleteLogin(ctx, auth.Identity{Issuer: "https://id.example", Subject: "persistent-user"}, hash, time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatalf("create persistent session: %v", err)
	}
	var persistedHash []byte
	var rawPosition int
	if err := pool.QueryRow(ctx, `
		SELECT token_hash, position(convert_to($1, 'UTF8') in token_hash)
		FROM sessions WHERE token_hash = $2
	`, rawToken, hash[:]).Scan(&persistedHash, &rawPosition); err != nil {
		t.Fatalf("inspect persisted session: %v", err)
	}
	if !bytes.Equal(persistedHash, hash[:]) || rawPosition != 0 {
		t.Fatal("session persistence did not remain hash-only")
	}
	restartedStore := auth.NewPostgresStore(pool)
	session, err := restartedStore.Session(ctx, hash)
	if err != nil || session.User.ID != user.ID {
		t.Fatalf("session after store reuse = (%#v, %v)", session, err)
	}
	rotatedHash := sha256.Sum256([]byte("rotated-session-token"))
	rotatedUser, err := store.CompleteLogin(
		ctx,
		auth.Identity{Issuer: "https://id.example", Subject: "persistent-user"},
		rotatedHash,
		time.Now().Add(time.Hour),
		&hash,
	)
	if err != nil || rotatedUser.ID != user.ID {
		t.Fatalf("rotate session = (%#v, %v)", rotatedUser, err)
	}
	if _, err := store.Session(ctx, hash); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("rotated-away session error = %v, want ErrSessionNotFound", err)
	}
	if _, err := store.Session(ctx, rotatedHash); err != nil {
		t.Fatalf("rotated session lookup failed: %v", err)
	}
	revoked, err := store.RevokeSession(ctx, rotatedHash)
	if err != nil || !revoked {
		t.Fatalf("revoke session = (%v, %v)", revoked, err)
	}
	if _, err := store.Session(ctx, rotatedHash); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("revoked session error = %v, want ErrSessionNotFound", err)
	}
}

func assertSessionExpiration(t *testing.T, ctx context.Context, pool *pgxpool.Pool, store *auth.PostgresStore) {
	t.Helper()
	hash := sha256.Sum256([]byte("already-expired-session"))
	if _, err := pool.Exec(ctx, `
		INSERT INTO sessions (user_id, token_hash, created_at, expires_at)
		SELECT id, $1, now() - interval '2 hours', now() - interval '1 hour'
		FROM users ORDER BY created_at LIMIT 1
	`, hash[:]); err != nil {
		t.Fatalf("insert expired session: %v", err)
	}
	if _, err := store.Session(ctx, hash); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("expired session error = %v, want ErrSessionNotFound", err)
	}
	if err := store.Cleanup(ctx, 100); err != nil {
		t.Fatalf("cleanup expired session: %v", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE token_hash = $1`, hash[:]).Scan(&count); err != nil || count != 0 {
		t.Fatalf("expired session count after cleanup = (%d, %v)", count, err)
	}
}

func assertOIDCIdentityUniqueness(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `
		INSERT INTO users (oidc_issuer, oidc_subject)
		VALUES ('https://id.example', 'subject-0')
	`); err == nil {
		t.Fatal("issuer-qualified OIDC identity uniqueness accepted a duplicate")
	}
}
