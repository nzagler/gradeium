package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	authpackage "github.com/nzagler/gradeium/backend/internal/auth"
	"github.com/nzagler/gradeium/backend/internal/integrations"
	"github.com/nzagler/gradeium/backend/internal/integrations/jellyfin"
	"github.com/nzagler/gradeium/backend/internal/jellyfinsync"
	secretspackage "github.com/nzagler/gradeium/backend/internal/secrets"
	settingspackage "github.com/nzagler/gradeium/backend/internal/settings"
)

type jellyfinSyncAPISyncer struct {
	started chan struct{}
	release chan struct{}
	context chan context.Context
}

func (syncer *jellyfinSyncAPISyncer) Sync(ctx context.Context, _ string, _ jellyfinsync.Source, _ []jellyfin.LibraryMapping) (jellyfinsync.Result, error) {
	syncer.context <- ctx
	close(syncer.started)
	select {
	case <-syncer.release:
		return jellyfinsync.Result{Scanned: 1, MoviesAdded: 1, Issues: []jellyfinsync.Issue{}}, nil
	case <-ctx.Done():
		return jellyfinsync.Result{}, ctx.Err()
	}
}

type jellyfinSyncAPISecrets struct{ values map[string][]byte }

func (store *jellyfinSyncAPISecrets) Configured(_ context.Context, key string) (bool, error) {
	_, ok := store.values[key]
	return ok, nil
}
func (store *jellyfinSyncAPISecrets) Set(_ context.Context, key, value string) error {
	store.values[key] = []byte(value)
	return nil
}
func (store *jellyfinSyncAPISecrets) Read(_ context.Context, key string) ([]byte, error) {
	value, ok := store.values[key]
	if !ok {
		return nil, secretspackage.ErrSecretNotFound
	}
	return append([]byte(nil), value...), nil
}
func (store *jellyfinSyncAPISecrets) Delete(_ context.Context, key string) (bool, error) {
	_, ok := store.values[key]
	delete(store.values, key)
	return ok, nil
}

type jellyfinSyncAPIStatuses struct{}

func (jellyfinSyncAPIStatuses) List(context.Context) ([]integrations.TestStatus, error) {
	return nil, nil
}
func (jellyfinSyncAPIStatuses) Upsert(_ context.Context, status integrations.TestStatus) (integrations.TestStatus, error) {
	return status, nil
}
func (jellyfinSyncAPIStatuses) Delete(context.Context, string) error { return nil }

func TestJellyfinSyncRequestCancellationDoesNotCancelJobAndStatusRetainsResult(t *testing.T) {
	root, cancelRoot := context.WithCancel(context.Background())
	defer cancelRoot()
	syncer := &jellyfinSyncAPISyncer{started: make(chan struct{}), release: make(chan struct{}), context: make(chan context.Context, 1)}
	jobs := jellyfinsync.NewJobManager(root, syncer, time.Minute)
	defer jobs.Close()

	registry := settingspackage.NewRegistry()
	settingsService := settingspackage.NewService(registry, &apiSettingsStore{values: make(map[string]json.RawMessage)})
	integrationService := integrations.NewService(settingsService, &jellyfinSyncAPISecrets{values: make(map[string][]byte)}, jellyfinSyncAPIStatuses{})
	if _, err := integrationService.Configure(context.Background(), "jellyfin", integrations.ConfigurationInput{
		Enabled: true, BaseURL: "http://jellyfin.example", Secret: "test-api-key",
		LibraryMappings: []jellyfin.LibraryMapping{{LibraryID: "movies", Domain: jellyfin.Movies}},
	}); err != nil {
		t.Fatalf("configure Jellyfin: %v", err)
	}

	handlers := &apiHandlers{integrations: integrationService, jellyfinSync: jobs}
	requestContext, cancelRequest := context.WithCancel(context.Background())
	identity := &requestAuthentication{session: authpackage.Session{User: authpackage.User{ID: "user"}}}
	requestContext = context.WithValue(requestContext, requestAuthenticationKey{}, identity)
	request := httptest.NewRequest(http.MethodPost, "/admin/integrations/jellyfin/sync", nil).WithContext(requestContext)
	response := httptest.NewRecorder()
	handlers.syncJellyfin(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("POST status = %d, body %q", response.Code, response.Body.String())
	}
	<-syncer.started
	jobContext := <-syncer.context
	cancelRequest()
	if err := jobContext.Err(); err != nil {
		t.Fatalf("request cancellation propagated to app-owned job: %v", err)
	}
	if status := jobs.Status("user"); status.State != jellyfinsync.JobRunning {
		t.Fatalf("job after request cancellation = %#v", status)
	}
	close(syncer.release)

	deadline := time.Now().Add(time.Second)
	for jobs.Status("user").State != jellyfinsync.JobCompleted && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	statusRequest := httptest.NewRequest(http.MethodGet, "/admin/integrations/jellyfin/sync", nil)
	statusRequest = statusRequest.WithContext(context.WithValue(statusRequest.Context(), requestAuthenticationKey{}, identity))
	statusResponse := httptest.NewRecorder()
	handlers.jellyfinSyncStatus(statusResponse, statusRequest)
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("GET status = %d", statusResponse.Code)
	}
	var status jellyfinsync.JobStatus
	if err := json.NewDecoder(statusResponse.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.State != jellyfinsync.JobCompleted || status.Result == nil || status.Result.MoviesAdded != 1 {
		t.Fatalf("GET status response = %#v", status)
	}
}
