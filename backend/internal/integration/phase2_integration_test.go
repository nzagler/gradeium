package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/nzagler/gradeium/backend/internal/database"
	"github.com/nzagler/gradeium/backend/internal/secrets"
	"github.com/nzagler/gradeium/backend/internal/settings"
	"github.com/nzagler/gradeium/backend/internal/setup"
)

func TestPhase2PostgresFoundation(t *testing.T) {
	baseURL := os.Getenv("GRADEIUM_TEST_DATABASE_URL")
	if baseURL == "" {
		t.Skip("GRADEIUM_TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	testURL := createTestDatabase(t, ctx, baseURL)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := database.Migrate(ctx, testURL, logger); err != nil {
		t.Fatalf("Migrate returned an error: %v", err)
	}
	pool, err := database.Open(ctx, testURL)
	if err != nil {
		t.Fatalf("database.Open returned an error: %v", err)
	}
	t.Cleanup(pool.Close)

	var serverVersionText string
	if err := pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&serverVersionText); err != nil {
		t.Fatalf("read PostgreSQL version: %v", err)
	}
	serverVersion, err := strconv.Atoi(serverVersionText)
	if err != nil {
		t.Fatalf("parse PostgreSQL version %q: %v", serverVersionText, err)
	}
	if serverVersion < 180000 {
		t.Fatalf("PostgreSQL version = %d, want 18 or newer", serverVersion)
	}

	var entityVersion, userVersion int
	if err := pool.QueryRow(ctx, `
		INSERT INTO entities (type) VALUES ('game')
		RETURNING uuid_extract_version(id)
	`).Scan(&entityVersion); err != nil {
		t.Fatalf("insert entity: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO users DEFAULT VALUES
		RETURNING uuid_extract_version(id)
	`).Scan(&userVersion); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if entityVersion != 7 || userVersion != 7 {
		t.Fatalf("UUID versions = entity %d, user %d; want 7", entityVersion, userVersion)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO entities (type) VALUES ('unknown')`); err == nil {
		t.Fatal("entities type constraint accepted an unknown domain")
	}

	setupService := setup.NewService(setup.NewPostgresStore(pool))
	complete, err := setupService.CompleteStatus(ctx)
	if err != nil || complete {
		t.Fatalf("fresh setup status = (%v, %v), want (false, nil)", complete, err)
	}
	assertSingleSetupTransition(t, ctx, setupService)

	registry := settings.NewRegistry()
	settingsService := settings.NewService(registry, settings.NewPostgresStore(pool))
	if _, err := settingsService.Update(ctx, settings.InstanceNameKey, []byte(`"Persistent Gradeium"`)); err != nil {
		t.Fatalf("persist application setting: %v", err)
	}
	values, err := settingsService.List(ctx)
	if err != nil || len(values) != 4 || string(values[0].Value) != `"Persistent Gradeium"` {
		t.Fatalf("persisted settings = (%#v, %v)", values, err)
	}

	secretStore := secrets.NewPostgresStore(pool)
	configDirectory := t.TempDir()
	firstCipher, err := secrets.InitializeCipher(ctx, configDirectory, secretStore)
	if err != nil {
		t.Fatalf("initialize first cipher: %v", err)
	}
	secretService := secrets.NewService(registry, secretStore, firstCipher)
	assertConcurrentSecretReplacement(t, ctx, secretService)
	secretValue := "database-must-not-contain-this-plaintext"
	if err := secretService.Set(ctx, settings.FutureAuthenticationSecretKey, secretValue); err != nil {
		t.Fatalf("persist encrypted secret: %v", err)
	}
	assertDatabaseDoesNotContainPlaintext(t, ctx, pool, secretValue)

	// Reloading from the same config directory simulates an application restart.
	secondCipher, err := secrets.InitializeCipher(ctx, configDirectory, secretStore)
	if err != nil {
		t.Fatalf("reload persistent cipher: %v", err)
	}
	restartedService := secrets.NewService(registry, secretStore, secondCipher)
	if err := restartedService.ValidateStored(ctx); err != nil {
		t.Fatalf("validate encrypted secret after restart: %v", err)
	}
	plaintext, err := restartedService.Read(ctx, settings.FutureAuthenticationSecretKey)
	if err != nil {
		t.Fatalf("decrypt after restart: %v", err)
	}
	if string(plaintext) != secretValue {
		t.Fatalf("decrypted value = %q", plaintext)
	}
	for index := range plaintext {
		plaintext[index] = 0
	}
}

type secretSetter interface {
	Set(context.Context, string, string) error
	Read(context.Context, string) ([]byte, error)
}

func assertConcurrentSecretReplacement(t *testing.T, ctx context.Context, service secretSetter) {
	t.Helper()
	const replacements = 12
	var waitGroup sync.WaitGroup
	for index := range replacements {
		waitGroup.Add(1)
		go func(index int) {
			defer waitGroup.Done()
			value := "database-concurrent-secret-" + strconv.Itoa(index)
			if err := service.Set(ctx, settings.FutureAuthenticationSecretKey, value); err != nil {
				t.Errorf("concurrent secret Set %d returned an error: %v", index, err)
			}
		}(index)
	}
	waitGroup.Wait()

	plaintext, err := service.Read(ctx, settings.FutureAuthenticationSecretKey)
	if err != nil {
		t.Fatalf("read concurrent secret replacement: %v", err)
	}
	defer func() {
		for index := range plaintext {
			plaintext[index] = 0
		}
	}()
	value := string(plaintext)
	matched := false
	for index := range replacements {
		if value == "database-concurrent-secret-"+strconv.Itoa(index) {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("database replacement left an incomplete value %q", value)
	}
}

type setupCompleter interface {
	Complete(context.Context) (bool, error)
}

func assertSingleSetupTransition(t *testing.T, ctx context.Context, service setupCompleter) {
	t.Helper()
	const requests = 12
	var transitions atomic.Int32
	var waitGroup sync.WaitGroup
	for range requests {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			transitioned, err := service.Complete(ctx)
			if err != nil {
				t.Errorf("Complete returned an error: %v", err)
				return
			}
			if transitioned {
				transitions.Add(1)
			}
		}()
	}
	waitGroup.Wait()
	if got := transitions.Load(); got != 1 {
		t.Fatalf("database setup transitions = %d, want 1", got)
	}
}

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func assertDatabaseDoesNotContainPlaintext(t *testing.T, ctx context.Context, database queryRower, plaintext string) {
	t.Helper()
	var nonce, ciphertext []byte
	if err := database.QueryRow(ctx, `
		SELECT nonce, ciphertext
		FROM secret_settings
		WHERE key = $1
	`, settings.FutureAuthenticationSecretKey).Scan(&nonce, &ciphertext); err != nil {
		t.Fatalf("read encrypted database row: %v", err)
	}
	if len(nonce) != 12 || len(ciphertext) < 16 {
		t.Fatalf("invalid encrypted envelope lengths: nonce=%d ciphertext=%d", len(nonce), len(ciphertext))
	}
	if bytes.Contains(ciphertext, []byte(plaintext)) {
		t.Fatal("database ciphertext contains secret plaintext")
	}
}

func createTestDatabase(t *testing.T, ctx context.Context, baseURL string) string {
	t.Helper()
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		t.Fatalf("GRADEIUM_TEST_DATABASE_URL must be a PostgreSQL URL")
	}
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		t.Fatalf("generate test database name: %v", err)
	}
	databaseName := "gradeium_phase2_" + hex.EncodeToString(random)

	admin, err := pgx.Connect(ctx, baseURL)
	if err != nil {
		t.Fatalf("connect to test database server: %v", err)
	}
	identifier := pgx.Identifier{databaseName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		admin.Close(ctx)
		t.Fatalf("create isolated test database: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, dropErr := admin.Exec(cleanupCtx, "DROP DATABASE "+identifier+" WITH (FORCE)")
		admin.Close(cleanupCtx)
		if dropErr != nil {
			t.Errorf("drop isolated test database: %v", dropErr)
		}
	})

	parsed.Path = "/" + databaseName
	return parsed.String()
}
