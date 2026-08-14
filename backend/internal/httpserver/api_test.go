package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	secretspackage "github.com/nzagler/gradeium/backend/internal/secrets"
	settingspackage "github.com/nzagler/gradeium/backend/internal/settings"
)

type apiSetupService struct {
	mutex    sync.Mutex
	complete bool
}

func (service *apiSetupService) CompleteStatus(context.Context) (bool, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	return service.complete, nil
}

func (service *apiSetupService) Complete(context.Context) (bool, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()
	if service.complete {
		return false, nil
	}
	service.complete = true
	return true, nil
}

type apiSettingsStore struct {
	values map[string]json.RawMessage
}

func (store *apiSettingsStore) Values(context.Context) (map[string]json.RawMessage, error) {
	result := make(map[string]json.RawMessage, len(store.values))
	for key, value := range store.values {
		result[key] = append(json.RawMessage(nil), value...)
	}
	return result, nil
}

func (store *apiSettingsStore) Upsert(_ context.Context, key string, value json.RawMessage) error {
	store.values[key] = append(json.RawMessage(nil), value...)
	return nil
}

type apiSecretStore struct {
	mutex   sync.Mutex
	records map[string]secretspackage.Record
}

func (store *apiSecretStore) Get(_ context.Context, key string) (secretspackage.Record, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, ok := store.records[key]
	if !ok {
		return secretspackage.Record{}, secretspackage.ErrSecretNotFound
	}
	return cloneAPISecretRecord(record), nil
}

func (store *apiSecretStore) Upsert(_ context.Context, record secretspackage.Record) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.records[record.Key] = cloneAPISecretRecord(record)
	return nil
}

func (store *apiSecretStore) Delete(_ context.Context, key string) (bool, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	_, found := store.records[key]
	delete(store.records, key)
	return found, nil
}

func (store *apiSecretStore) List(context.Context) ([]secretspackage.Record, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	records := make([]secretspackage.Record, 0, len(store.records))
	for _, record := range store.records {
		records = append(records, cloneAPISecretRecord(record))
	}
	return records, nil
}

func cloneAPISecretRecord(record secretspackage.Record) secretspackage.Record {
	record.Nonce = append([]byte(nil), record.Nonce...)
	record.Ciphertext = append([]byte(nil), record.Ciphertext...)
	return record
}

func newTestAPI(t *testing.T) (http.Handler, *bytes.Buffer, *apiSecretStore) {
	t.Helper()
	registry := settingspackage.NewRegistry()
	settingsStore := &apiSettingsStore{values: make(map[string]json.RawMessage)}
	secretStore := &apiSecretStore{records: make(map[string]secretspackage.Record)}
	cipher, err := secretspackage.NewCipher(bytes.Repeat([]byte{0x77}, 32))
	if err != nil {
		t.Fatalf("NewCipher returned an error: %v", err)
	}
	setupService := &apiSetupService{}
	logOutput := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logOutput, nil))
	api := NewAPI(
		logger,
		setupService,
		settingspackage.NewService(registry, settingsStore),
		secretspackage.NewService(registry, secretStore, cipher),
		registry,
		true,
		Phase2AdminAuthorization,
	)
	return api, logOutput, secretStore
}

func TestSetupAPIIsOneTimeAndGatesAdminRoutes(t *testing.T) {
	api, _, _ := newTestAPI(t)

	response := performAPIRequest(api, http.MethodGet, "/setup/status", "")
	if response.Code != http.StatusOK || response.Body.String() != "{\"complete\":false}\n" {
		t.Fatalf("initial status = %d %q", response.Code, response.Body.String())
	}
	response = performAPIRequest(api, http.MethodGet, "/admin/settings", "")
	if response.Code != http.StatusPreconditionRequired {
		t.Fatalf("pre-setup admin status = %d, want %d", response.Code, http.StatusPreconditionRequired)
	}
	response = performAPIRequest(api, http.MethodPost, "/setup/complete", "")
	if response.Code != http.StatusOK || response.Body.String() != "{\"complete\":true}\n" {
		t.Fatalf("completion = %d %q", response.Code, response.Body.String())
	}
	response = performAPIRequest(api, http.MethodPost, "/setup/complete", "")
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "setup_already_complete") {
		t.Fatalf("second completion = %d %q", response.Code, response.Body.String())
	}
	response = performAPIRequest(api, http.MethodGet, "/admin/settings", "")
	if response.Code != http.StatusOK {
		t.Fatalf("post-setup admin status = %d, body %q", response.Code, response.Body.String())
	}
}

func TestSettingsAPIValidatesAndPersistsSafeValue(t *testing.T) {
	api, _, _ := newTestAPI(t)
	performAPIRequest(api, http.MethodPost, "/setup/complete", "")

	response := performAPIRequest(api, http.MethodPut, "/admin/settings/general.instance_name", `{"value":"  Home Gradeium  "}`)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"value":"Home Gradeium"`) {
		t.Fatalf("update = %d %q", response.Code, response.Body.String())
	}
	response = performAPIRequest(api, http.MethodGet, "/admin/settings", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"value":"Home Gradeium"`) {
		t.Fatalf("list = %d %q", response.Code, response.Body.String())
	}
	response = performAPIRequest(api, http.MethodPut, "/admin/settings/general.instance_name", `{"value":true}`)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "validation_error") {
		t.Fatalf("invalid update = %d %q", response.Code, response.Body.String())
	}
	response = performAPIRequest(api, http.MethodPut, "/admin/settings/unknown", `{"value":"x"}`)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown setting = %d %q", response.Code, response.Body.String())
	}
}

func TestSecretAPIAlwaysRedactsPlaintext(t *testing.T) {
	api, logOutput, store := newTestAPI(t)
	performAPIRequest(api, http.MethodPost, "/setup/complete", "")
	key := settingspackage.FutureAuthenticationSecretKey
	plaintext := "never-return-this-value"

	response := performAPIRequest(api, http.MethodPut, "/admin/secrets/"+key, `{"value":"`+plaintext+`"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("set secret = %d %q", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), plaintext) {
		t.Fatal("secret write response contained plaintext")
	}
	first, err := store.Get(context.Background(), key)
	if err != nil {
		t.Fatalf("stored secret was missing: %v", err)
	}
	if bytes.Contains(first.Ciphertext, []byte(plaintext)) {
		t.Fatal("stored ciphertext contained plaintext")
	}

	response = performAPIRequest(api, http.MethodGet, "/admin/settings", "")
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"configured":true`) {
		t.Fatalf("settings list = %d %q", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), plaintext) {
		t.Fatal("settings list contained secret plaintext")
	}

	response = performAPIRequest(api, http.MethodPut, "/admin/secrets/"+key, `{"value":"`+plaintext+`"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("replace secret = %d %q", response.Code, response.Body.String())
	}
	second, _ := store.Get(context.Background(), key)
	if bytes.Equal(first.Nonce, second.Nonce) {
		t.Fatal("secret replacement reused a nonce")
	}
	if strings.Contains(logOutput.String(), plaintext) {
		t.Fatal("application log contained secret plaintext")
	}

	response = performAPIRequest(api, http.MethodDelete, "/admin/secrets/"+key, "")
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), plaintext) {
		t.Fatalf("delete secret = %d %q", response.Code, response.Body.String())
	}
}

func TestAPIUnknownRouteReturnsJSON404(t *testing.T) {
	api, _, _ := newTestAPI(t)
	response := performAPIRequest(api, http.MethodGet, "/unknown", "")
	if response.Code != http.StatusNotFound || response.Body.String() != "{\"error\":\"not_found\"}\n" {
		t.Fatalf("unknown route = %d %q", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q", contentType)
	}
}

func performAPIRequest(handler http.Handler, method, path, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
