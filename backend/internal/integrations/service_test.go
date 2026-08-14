package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nzagler/gradeium/backend/internal/secrets"
	"github.com/nzagler/gradeium/backend/internal/settings"
)

func TestConfigureStoresSecretsAndReturnsOnlyRedactedState(t *testing.T) {
	ctx := context.Background()
	settingsStore := &memorySettingsStore{values: map[string]json.RawMessage{}}
	secretStore := &memorySecrets{values: map[string]string{}}
	statusStore := &memoryStatuses{values: map[string]TestStatus{}}
	service := NewService(settings.NewService(settings.NewRegistry(), settingsStore), secretStore, statusStore)

	tests := []struct {
		provider string
		input    ConfigurationInput
		secret   string
		pin      string
	}{
		{provider: "igdb", input: ConfigurationInput{Enabled: true, ClientID: "fixture-client", Secret: "igdb-plaintext-secret"}, secret: settings.IGDBClientSecretKey},
		{provider: "tmdb", input: ConfigurationInput{Enabled: true, Secret: "tmdb-plaintext-token"}, secret: settings.TMDBAccessTokenKey},
		{provider: "tvdb", input: ConfigurationInput{Enabled: true, Secret: "tvdb-plaintext-key", PIN: "tvdb-plaintext-pin"}, secret: settings.TVDBAPIKey, pin: settings.TVDBPINKey},
	}
	for _, test := range tests {
		view, err := service.Configure(ctx, test.provider, test.input)
		if err != nil {
			t.Fatalf("configure %s: %v", test.provider, err)
		}
		if !view.Configured || !view.SecretConfigured || view.State != "configured" {
			t.Fatalf("configured %s view = %#v", test.provider, view)
		}
		if secretStore.values[test.secret] != test.input.Secret {
			t.Fatalf("stored %s secret was not replaced", test.provider)
		}
		if test.pin != "" && secretStore.values[test.pin] != test.input.PIN {
			t.Fatalf("stored %s PIN was not replaced", test.provider)
		}
	}

	views, err := service.List(ctx)
	if err != nil {
		t.Fatalf("list provider views: %v", err)
	}
	payload, err := json.Marshal(views)
	if err != nil {
		t.Fatalf("marshal provider views: %v", err)
	}
	for _, plaintext := range []string{"igdb-plaintext-secret", "tmdb-plaintext-token", "tvdb-plaintext-key", "tvdb-plaintext-pin"} {
		if strings.Contains(string(payload), plaintext) {
			t.Fatalf("provider response contains secret plaintext %q", plaintext)
		}
	}

	if _, err := service.Configure(ctx, "tmdb", ConfigurationInput{Enabled: true, Secret: "replacement", RemoveSecret: true}); err == nil {
		t.Fatal("configure accepted simultaneous secret replacement and removal")
	}
	view, err := service.Configure(ctx, "tmdb", ConfigurationInput{Enabled: false, RemoveSecret: true})
	if err != nil {
		t.Fatalf("remove TMDB credential: %v", err)
	}
	if view.Configured || view.SecretConfigured || view.State != "not_configured" {
		t.Fatalf("TMDB view after credential removal = %#v", view)
	}
}

func TestListUsesDurableConnectionStateWithoutExposingCauses(t *testing.T) {
	ctx := context.Background()
	settingsStore := &memorySettingsStore{values: map[string]json.RawMessage{
		settings.TMDBEnabledKey: json.RawMessage(`true`),
	}}
	secretStore := &memorySecrets{values: map[string]string{settings.TMDBAccessTokenKey: "not-for-the-browser"}}
	status := TestStatus{Provider: "tmdb", Status: "error", Message: "Connection failed. Check the saved credentials and try again.", TestedAt: time.Now().UTC()}
	statusStore := &memoryStatuses{values: map[string]TestStatus{"tmdb": status}}
	service := NewService(settings.NewService(settings.NewRegistry(), settingsStore), secretStore, statusStore)

	views, err := service.List(ctx)
	if err != nil {
		t.Fatalf("list connection state: %v", err)
	}
	view := find(views, "tmdb")
	if view.State != "error" || view.LastTest == nil || view.LastTest.Message != status.Message {
		t.Fatalf("durable TMDB connection state = %#v", view)
	}
	view, err = service.Configure(ctx, "tmdb", ConfigurationInput{Enabled: true})
	if err != nil || view.State != "configured" || view.LastTest != nil {
		t.Fatalf("connection status after configuration change = (%#v, %v)", view, err)
	}
	cause := errors.New("upstream response included not-for-the-browser")
	safe := (&SafeError{Code: "tmdb_connection_failed", Message: status.Message, Cause: cause}).Error()
	if strings.Contains(safe, "not-for-the-browser") || safe != status.Message {
		t.Fatalf("safe integration error exposed its cause: %q", safe)
	}
}

type memorySettingsStore struct {
	mu     sync.Mutex
	values map[string]json.RawMessage
}

func (store *memorySettingsStore) Values(context.Context) (map[string]json.RawMessage, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make(map[string]json.RawMessage, len(store.values))
	for key, value := range store.values {
		result[key] = append(json.RawMessage(nil), value...)
	}
	return result, nil
}

func (store *memorySettingsStore) Upsert(_ context.Context, key string, value json.RawMessage) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.values[key] = append(json.RawMessage(nil), value...)
	return nil
}

type memorySecrets struct {
	mu     sync.Mutex
	values map[string]string
}

func (store *memorySecrets) Configured(_ context.Context, key string) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	_, ok := store.values[key]
	return ok, nil
}

func (store *memorySecrets) Set(_ context.Context, key, value string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.values[key] = value
	return nil
}

func (store *memorySecrets) Read(_ context.Context, key string) ([]byte, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	value, ok := store.values[key]
	if !ok {
		return nil, secrets.ErrSecretNotFound
	}
	return []byte(value), nil
}

func (store *memorySecrets) Delete(_ context.Context, key string) (bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	_, ok := store.values[key]
	delete(store.values, key)
	return ok, nil
}

type memoryStatuses struct {
	mu     sync.Mutex
	values map[string]TestStatus
}

func (store *memoryStatuses) List(context.Context) ([]TestStatus, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	result := make([]TestStatus, 0, len(store.values))
	for _, value := range store.values {
		result = append(result, value)
	}
	return result, nil
}

func (store *memoryStatuses) Upsert(_ context.Context, value TestStatus) (TestStatus, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if value.TestedAt.IsZero() {
		value.TestedAt = time.Now().UTC()
	}
	store.values[value.Provider] = value
	return value, nil
}

func (store *memoryStatuses) Delete(_ context.Context, provider string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	delete(store.values, provider)
	return nil
}
