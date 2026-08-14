package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	authpackage "github.com/nzagler/gradeium/backend/internal/auth"
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
	records map[string]secretspackage.Record
}

func (store *apiSecretStore) Get(_ context.Context, key string) (secretspackage.Record, error) {
	record, ok := store.records[key]
	if !ok {
		return secretspackage.Record{}, secretspackage.ErrSecretNotFound
	}
	return record, nil
}
func (store *apiSecretStore) Upsert(_ context.Context, record secretspackage.Record) error {
	store.records[record.Key] = record
	return nil
}
func (store *apiSecretStore) Delete(_ context.Context, key string) (bool, error) {
	_, found := store.records[key]
	delete(store.records, key)
	return found, nil
}
func (store *apiSecretStore) List(context.Context) ([]secretspackage.Record, error) {
	return nil, nil
}

type apiAuthenticationService struct {
	activated     bool
	isAdmin       bool
	sessionToken  string
	csrfToken     string
	clientSecret  string
	configuration authpackage.ConfigurationView
	callback      authpackage.LoginResult
	callbackError error
}

func newAPIAuthenticationService() *apiAuthenticationService {
	return &apiAuthenticationService{
		activated: true, isAdmin: true, sessionToken: "test-session", csrfToken: "test-csrf",
		configuration: authpackage.ConfigurationView{
			Configuration: authpackage.Configuration{IssuerURL: "https://id.example", ClientID: "client", PublicURL: "https://gradeium.example"},
			Revision:      1, Activated: true, Validated: true, ClientSecretConfigured: true,
		},
	}
}

func (service *apiAuthenticationService) State(context.Context) (authpackage.State, error) {
	state := authpackage.State{Activated: service.activated}
	if service.activated {
		revision := int64(1)
		state.ActiveRevision = &revision
		state.Active = &service.configuration.Configuration
	}
	return state, nil
}
func (service *apiAuthenticationService) Configuration(context.Context) (authpackage.ConfigurationView, error) {
	view := service.configuration
	view.Activated = service.activated
	return view, nil
}
func (service *apiAuthenticationService) SaveConfiguration(_ context.Context, input authpackage.ConfigurationInput) (authpackage.ConfigurationView, error) {
	service.clientSecret = input.ClientSecret
	service.configuration.Configuration = authpackage.Configuration{IssuerURL: input.IssuerURL, ClientID: input.ClientID, PublicURL: input.PublicURL}
	service.configuration.ClientSecretConfigured = input.ClientSecret != "" && !input.RemoveClientSecret
	return service.configuration, nil
}
func (service *apiAuthenticationService) TestConfiguration(context.Context) (authpackage.ValidationResult, error) {
	return authpackage.ValidationResult{Revision: 1, RedirectURI: "https://gradeium.example/api/auth/callback", Validated: true}, nil
}
func (service *apiAuthenticationService) Activate(context.Context) (authpackage.ConfigurationView, error) {
	service.activated = true
	service.configuration.Activated = true
	return service.configuration, nil
}
func (service *apiAuthenticationService) StartLogin(context.Context, string) (string, error) {
	return "https://id.example/authorize", nil
}
func (service *apiAuthenticationService) CompleteCallback(context.Context, string, string, string) (authpackage.LoginResult, error) {
	return service.callback, service.callbackError
}
func (service *apiAuthenticationService) Authenticate(_ context.Context, token string) (authpackage.Session, error) {
	if token != service.sessionToken {
		return authpackage.Session{}, authpackage.ErrSessionNotFound
	}
	return authpackage.Session{
		User:      authpackage.User{ID: "01900000-0000-7000-8000-000000000001", DisplayName: stringPointer("Admin"), IsAdmin: service.isAdmin},
		ExpiresAt: time.Now().Add(time.Hour), PublicURL: "https://gradeium.example",
	}, nil
}
func (service *apiAuthenticationService) CSRFToken(string) string { return service.csrfToken }
func (service *apiAuthenticationService) ValidateCSRF(_, submitted string) bool {
	return submitted == service.csrfToken
}
func (service *apiAuthenticationService) Logout(context.Context, string) (bool, error) {
	return true, nil
}

func newTestAPI(t *testing.T) (http.Handler, *apiSetupService, *apiAuthenticationService, *bytes.Buffer) {
	t.Helper()
	registry := settingspackage.NewRegistry()
	settingsStore := &apiSettingsStore{values: make(map[string]json.RawMessage)}
	secretStore := &apiSecretStore{records: make(map[string]secretspackage.Record)}
	cipher, err := secretspackage.NewCipher(bytes.Repeat([]byte{0x77}, 32))
	if err != nil {
		t.Fatalf("NewCipher returned an error: %v", err)
	}
	setupService := &apiSetupService{}
	authentication := newAPIAuthenticationService()
	logOutput := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logOutput, nil))
	api := NewAPI(
		logger,
		setupService,
		settingspackage.NewService(registry, settingsStore),
		secretspackage.NewService(registry, secretStore, cipher),
		registry,
		authentication,
		true,
	)
	return api, setupService, authentication, logOutput
}

func TestSetupAPIIsOneTimeAndAdminRoutesRequireAuthentication(t *testing.T) {
	api, _, authentication, _ := newTestAPI(t)

	response := performAPIRequest(api, http.MethodGet, "/setup/status", "", false, false)
	if response.Code != http.StatusOK || response.Body.String() != "{\"complete\":false}\n" {
		t.Fatalf("initial status = %d %q", response.Code, response.Body.String())
	}
	response = performAPIRequest(api, http.MethodGet, "/admin/settings", "", false, false)
	if response.Code != http.StatusPreconditionRequired {
		t.Fatalf("pre-setup admin status = %d", response.Code)
	}
	response = performAPIRequest(api, http.MethodPost, "/setup/complete", "", false, false)
	if response.Code != http.StatusOK {
		t.Fatalf("completion = %d %q", response.Code, response.Body.String())
	}
	response = performAPIRequest(api, http.MethodPost, "/setup/complete", "", false, false)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "setup_already_complete") {
		t.Fatalf("second completion = %d %q", response.Code, response.Body.String())
	}
	response = performAPIRequest(api, http.MethodGet, "/admin/settings", "", false, false)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated admin status = %d", response.Code)
	}
	response = performAPIRequest(api, http.MethodGet, "/admin/settings", "", true, false)
	if response.Code != http.StatusOK {
		t.Fatalf("authenticated admin status = %d, body %q", response.Code, response.Body.String())
	}
	authentication.isAdmin = false
	response = performAPIRequest(api, http.MethodGet, "/admin/settings", "", true, false)
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-admin status = %d, want 403", response.Code)
	}
}

func TestAdminUnsafeMethodsRequireSessionBoundCSRF(t *testing.T) {
	api, setup, _, _ := newTestAPI(t)
	setup.complete = true
	response := performAPIRequest(api, http.MethodPut, "/admin/settings/general.instance_name", `{"value":"Home Gradeium"}`, true, false)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), "csrf_rejected") {
		t.Fatalf("missing CSRF = %d %q", response.Code, response.Body.String())
	}
	response = performAPIRequest(api, http.MethodPut, "/admin/settings/general.instance_name", `{"value":"Home Gradeium"}`, true, true)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"value":"Home Gradeium"`) {
		t.Fatalf("valid update = %d %q", response.Code, response.Body.String())
	}
	request := httptest.NewRequest(http.MethodPut, "/admin/settings/general.instance_name", strings.NewReader(`{"value":"Other"}`))
	request.AddCookie(&http.Cookie{Name: authpackage.SessionCookieName, Value: "test-session"})
	request.Header.Set(authpackage.CSRFHeaderName, "test-csrf")
	request.Header.Set("Origin", "https://evil.example")
	response = httptest.NewRecorder()
	api.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("cross-origin request status = %d, want 403", response.Code)
	}
}

func TestAuthenticationBootstrapClosesAfterActivationAndRedactsSecret(t *testing.T) {
	api, setup, authentication, logs := newTestAPI(t)
	setup.complete = true
	authentication.activated = false
	authentication.configuration.Activated = false
	plaintext := "never-return-this-client-secret"
	response := performAPIRequest(api, http.MethodPut, "/auth/configuration", `{"issuerUrl":"https://id.example","clientId":"client","clientSecret":"`+plaintext+`","publicUrl":"https://gradeium.example"}`, false, false)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), plaintext) {
		t.Fatalf("bootstrap save = %d %q", response.Code, response.Body.String())
	}
	if strings.Contains(logs.String(), plaintext) {
		t.Fatal("client secret appeared in logs")
	}
	response = performAPIRequest(api, http.MethodPost, "/auth/configuration/test", "", false, false)
	if response.Code != http.StatusOK {
		t.Fatalf("bootstrap test = %d %q", response.Code, response.Body.String())
	}
	response = performAPIRequest(api, http.MethodPost, "/auth/activate", "", false, false)
	if response.Code != http.StatusOK {
		t.Fatalf("bootstrap activation = %d %q", response.Code, response.Body.String())
	}
	response = performAPIRequest(api, http.MethodGet, "/auth/configuration", "", false, false)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("post-activation anonymous configuration status = %d, want 401", response.Code)
	}
	response = performAPIRequest(api, http.MethodGet, "/auth/configuration", "", true, false)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), plaintext) {
		t.Fatalf("admin configuration read = %d %q", response.Code, response.Body.String())
	}
}

func TestAuthenticationCallbackSetsFiniteSecureCookieAndRedactsFailures(t *testing.T) {
	api, setup, authentication, logs := newTestAPI(t)
	setup.complete = true
	authentication.callback = authpackage.LoginResult{
		SessionToken: "opaque-session-token",
		ExpiresAt:    time.Now().Add(time.Hour),
		SecureCookie: true,
		ReturnPath:   "/settings/authentication",
	}
	response := performAPIRequest(api, http.MethodGet, "/auth/callback?state=private-state&code=private-code", "", false, false)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/settings/authentication" {
		t.Fatalf("successful callback = %d location %q", response.Code, response.Header().Get("Location"))
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("callback cookies = %#v", cookies)
	}
	cookie := cookies[0]
	if cookie.Name != authpackage.SessionCookieName || cookie.Value != "opaque-session-token" || !cookie.HttpOnly || !cookie.Secure ||
		cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.Domain != "" || cookie.MaxAge <= 0 || cookie.Expires.IsZero() {
		t.Fatalf("callback cookie attributes = %#v", cookie)
	}

	authentication.callbackError = errors.New("upstream response contained private-code")
	response = performAPIRequest(api, http.MethodGet, "/auth/callback?state=private-state&code=private-code", "", false, false)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login?error=oidc_login_failed" {
		t.Fatalf("failed callback = %d location %q", response.Code, response.Header().Get("Location"))
	}
	if strings.Contains(logs.String(), "private-state") || strings.Contains(logs.String(), "private-code") {
		t.Fatalf("callback material appeared in logs: %s", logs.String())
	}
}

func TestAPIUnknownRouteReturnsJSON404(t *testing.T) {
	api, _, _, _ := newTestAPI(t)
	response := performAPIRequest(api, http.MethodGet, "/unknown", "", false, false)
	if response.Code != http.StatusNotFound || response.Body.String() != "{\"error\":\"not_found\"}\n" {
		t.Fatalf("unknown route = %d %q", response.Code, response.Body.String())
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q", contentType)
	}
}

func performAPIRequest(handler http.Handler, method, path, body string, authenticated, csrf bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if authenticated {
		request.AddCookie(&http.Cookie{Name: authpackage.SessionCookieName, Value: "test-session"})
	}
	if csrf {
		request.Header.Set(authpackage.CSRFHeaderName, "test-csrf")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func stringPointer(value string) *string { return &value }
