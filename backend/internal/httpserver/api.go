package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	authpackage "github.com/nzagler/gradeium/backend/internal/auth"
	"github.com/nzagler/gradeium/backend/internal/games"
	"github.com/nzagler/gradeium/backend/internal/integrations"
	"github.com/nzagler/gradeium/backend/internal/media"
	"github.com/nzagler/gradeium/backend/internal/movies"
	"github.com/nzagler/gradeium/backend/internal/settings"
	"github.com/nzagler/gradeium/backend/internal/tv"
)

const (
	apiOperationTimeout = 3 * time.Second
	maxSettingsBodySize = 20 * 1024
)

// SetupService is the one-time bootstrap state used by the HTTP layer.
type SetupService interface {
	CompleteStatus(context.Context) (bool, error)
	Complete(context.Context) (bool, error)
}

// SettingsService provides validated non-secret application settings.
type SettingsService interface {
	List(context.Context) ([]settings.Value, error)
	Update(context.Context, string, json.RawMessage) (json.RawMessage, error)
}

// SecretService provides redacted secret-setting operations to the API.
type SecretService interface {
	Configured(context.Context, string) (bool, error)
	Set(context.Context, string, string) error
	Delete(context.Context, string) (bool, error)
}

// AuthenticationService is the OIDC/session boundary consumed by HTTP.
type AuthenticationService interface {
	State(context.Context) (authpackage.State, error)
	Configuration(context.Context) (authpackage.ConfigurationView, error)
	SaveConfiguration(context.Context, authpackage.ConfigurationInput) (authpackage.ConfigurationView, error)
	TestConfiguration(context.Context) (authpackage.ValidationResult, error)
	Activate(context.Context) (authpackage.ConfigurationView, error)
	StartLogin(context.Context, string) (string, error)
	CompleteCallback(context.Context, string, string, string) (authpackage.LoginResult, error)
	Authenticate(context.Context, string) (authpackage.Session, error)
	CSRFToken(string) string
	ValidateCSRF(string, string) bool
	Logout(context.Context, string) (bool, error)
}

type apiHandlers struct {
	logger             *slog.Logger
	setup              SetupService
	settings           SettingsService
	secrets            SecretService
	registry           *settings.Registry
	authentication     AuthenticationService
	integrations       *integrations.Service
	games              *games.Service
	movies             *movies.Service
	tv                 *tv.Service
	preferences        *media.PreferencesService
	masterKeyAvailable bool
}

// NewAPI returns Gradeium's setup, authentication, and admin JSON API.
func NewAPI(
	logger *slog.Logger,
	setupService SetupService,
	settingsService SettingsService,
	secretService SecretService,
	registry *settings.Registry,
	authentication AuthenticationService,
	masterKeyAvailable bool,
) http.Handler {
	return NewAPIWithMedia(logger, setupService, settingsService, secretService, registry, authentication, masterKeyAvailable, nil, nil, nil, nil, nil)
}

// NewAPIWithMedia adds the Phase 4 integration and domain routes while NewAPI
// remains available to focused foundation tests.
func NewAPIWithMedia(
	logger *slog.Logger,
	setupService SetupService,
	settingsService SettingsService,
	secretService SecretService,
	registry *settings.Registry,
	authentication AuthenticationService,
	masterKeyAvailable bool,
	integrationService *integrations.Service,
	gameService *games.Service,
	movieService *movies.Service,
	tvService *tv.Service,
	preferenceService *media.PreferencesService,
) http.Handler {
	handlers := &apiHandlers{
		logger:             logger,
		setup:              setupService,
		settings:           settingsService,
		secrets:            secretService,
		registry:           registry,
		authentication:     authentication,
		integrations:       integrationService,
		games:              gameService,
		movies:             movieService,
		tv:                 tvService,
		preferences:        preferenceService,
		masterKeyAvailable: masterKeyAvailable,
	}

	router := chi.NewRouter()
	router.Use(noStore)
	router.Get("/setup/status", handlers.setupStatus)
	router.Post("/setup/complete", handlers.completeSetup)
	router.Get("/auth/status", handlers.authStatus)
	router.Get("/auth/configuration", handlers.authConfiguration)
	router.Put("/auth/configuration", handlers.saveAuthConfiguration)
	router.Post("/auth/configuration/test", handlers.testAuthConfiguration)
	router.Post("/auth/activate", handlers.activateAuthentication)
	router.Get("/auth/login", handlers.startLogin)
	router.Get("/auth/callback", handlers.authCallback)
	router.Get("/auth/session", handlers.authSession)
	router.Post("/auth/logout", handlers.logout)
	router.Route("/admin", func(admin chi.Router) {
		admin.Use(handlers.requireSetupComplete)
		admin.Use(handlers.requireAdmin)
		admin.Use(handlers.requireCSRF)
		admin.Get("/settings", handlers.listSettings)
		admin.Put("/settings/{key}", handlers.updateSetting)
		admin.Put("/secrets/{key}", handlers.setSecret)
		admin.Delete("/secrets/{key}", handlers.deleteSecret)
		admin.Get("/system/status", handlers.systemStatus)
		if handlers.integrations != nil {
			admin.Get("/integrations", handlers.listIntegrations)
			admin.Put("/integrations/{provider}", handlers.configureIntegration)
			admin.Post("/integrations/{provider}/test", handlers.testIntegration)
		}
	})
	if handlers.games != nil && handlers.movies != nil && handlers.tv != nil {
		handlers.mountMediaRoutes(router)
	}
	router.NotFound(apiNotFoundHandler)
	return router
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (handlers *apiHandlers) setupStatus(w http.ResponseWriter, r *http.Request) {
	complete, err := withTimeoutResult(r.Context(), handlers.setup.CompleteStatus)
	if err != nil {
		handlers.internalError(w, r, "read setup status", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"complete": complete})
}

func (handlers *apiHandlers) completeSetup(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1)
	if body, err := io.ReadAll(r.Body); err != nil || len(body) != 0 {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "This endpoint does not accept a request body.")
		return
	}

	transitioned, err := withTimeoutResult(r.Context(), handlers.setup.Complete)
	if err != nil {
		handlers.internalError(w, r, "complete setup", err)
		return
	}
	if !transitioned {
		writeAPIError(w, http.StatusConflict, "setup_already_complete", "Initial setup has already been completed.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"complete": true})
}

func (handlers *apiHandlers) requireSetupComplete(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		complete, err := withTimeoutResult(r.Context(), handlers.setup.CompleteStatus)
		if err != nil {
			handlers.internalError(w, r, "check setup status", err)
			return
		}
		if !complete {
			writeAPIError(w, http.StatusPreconditionRequired, "setup_required", "Initial setup must be completed first.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type settingResponse struct {
	Key         string               `json:"key"`
	Section     string               `json:"section"`
	Label       string               `json:"label"`
	Description string               `json:"description"`
	Type        settings.ValueType   `json:"type"`
	Sensitivity settings.Sensitivity `json:"sensitivity"`
	Reserved    bool                 `json:"reserved"`
	Configured  bool                 `json:"configured"`
	Value       json.RawMessage      `json:"value,omitempty"`
}

func (handlers *apiHandlers) listSettings(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), apiOperationTimeout)
	defer cancel()

	publicValues, err := handlers.settings.List(ctx)
	if err != nil {
		handlers.internalError(w, r, "list application settings", err)
		return
	}
	responses := make([]settingResponse, 0, len(handlers.registry.Definitions()))
	for _, value := range publicValues {
		definition := value.Definition
		responses = append(responses, settingResponse{
			Key:         definition.Key,
			Section:     definition.Section,
			Label:       definition.Label,
			Description: definition.Description,
			Type:        definition.Type,
			Sensitivity: definition.Sensitivity,
			Reserved:    definition.Reserved,
			Configured:  value.Configured,
			Value:       value.Value,
		})
	}
	for _, definition := range handlers.registry.Definitions() {
		if definition.Sensitivity != settings.SensitivitySecret || definition.Internal {
			continue
		}
		configured, err := handlers.secrets.Configured(ctx, definition.Key)
		if err != nil {
			handlers.internalError(w, r, "read secret configuration status", err)
			return
		}
		responses = append(responses, settingResponse{
			Key:         definition.Key,
			Section:     definition.Section,
			Label:       definition.Label,
			Description: definition.Description,
			Type:        definition.Type,
			Sensitivity: definition.Sensitivity,
			Reserved:    definition.Reserved,
			Configured:  configured,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": responses})
}

func (handlers *apiHandlers) updateSetting(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	definition, allowed := handlers.registry.Definition(key)
	if !allowed || definition.Sensitivity != settings.SensitivityPublic || definition.Section == "authentication" || definition.Internal {
		writeAPIError(w, http.StatusNotFound, "setting_not_found", "That setting is not available.")
		return
	}
	var request struct {
		Value json.RawMessage `json:"value"`
	}
	if err := decodeJSONBody(w, r, &request); err != nil || len(request.Value) == 0 {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "Provide one valid JSON value.")
		return
	}
	if _, err := handlers.registry.ValidateSetting(key, request.Value); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), apiOperationTimeout)
	defer cancel()
	value, err := handlers.settings.Update(ctx, key, request.Value)
	if err != nil {
		handlers.internalError(w, r, "update application setting", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"key":        key,
		"configured": true,
		"value":      value,
	})
}

func (handlers *apiHandlers) setSecret(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	definition, allowed := handlers.registry.Definition(key)
	if !allowed || definition.Section == "authentication" || definition.Internal || !handlers.registry.AllowsSecret(key) {
		writeAPIError(w, http.StatusNotFound, "setting_not_found", "That secret setting is not available.")
		return
	}
	var request struct {
		Value string `json:"value"`
	}
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "Provide one valid secret value.")
		return
	}
	if err := handlers.registry.ValidateSecret(key, request.Value); err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), apiOperationTimeout)
	defer cancel()
	if err := handlers.secrets.Set(ctx, key, request.Value); err != nil {
		handlers.internalError(w, r, "persist secret setting", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"key": key, "configured": true})
}

func (handlers *apiHandlers) deleteSecret(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	definition, allowed := handlers.registry.Definition(key)
	if !allowed || definition.Section == "authentication" || definition.Internal || !handlers.registry.AllowsSecret(key) {
		writeAPIError(w, http.StatusNotFound, "setting_not_found", "That secret setting is not available.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), apiOperationTimeout)
	defer cancel()
	if _, err := handlers.secrets.Delete(ctx, key); err != nil {
		handlers.internalError(w, r, "delete secret setting", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"key": key, "configured": false})
}

func (handlers *apiHandlers) systemStatus(w http.ResponseWriter, r *http.Request) {
	complete, err := withTimeoutResult(r.Context(), handlers.setup.CompleteStatus)
	if err != nil {
		handlers.internalError(w, r, "read system status", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"setupComplete": complete,
		"masterKey": map[string]any{
			"available": handlers.masterKeyAvailable,
			"storage":   "persistent config mount",
		},
	})
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, destination any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxSettingsBodySize)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request must contain one JSON object")
	}
	return nil
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]string{"error": code, "message": message})
}

func (handlers *apiHandlers) internalError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	handlers.logger.Error("api operation failed",
		"operation", operation,
		"method", r.Method,
		"path", r.URL.Path,
		"error", sanitizeError(err),
	)
	writeAPIError(w, http.StatusInternalServerError, "internal_server_error", "The request could not be completed.")
}

func sanitizeError(err error) string {
	message := err.Error()
	if len(message) > 240 {
		return strings.TrimSpace(message[:240])
	}
	return message
}

func withTimeoutResult[T any](ctx context.Context, operation func(context.Context) (T, error)) (T, error) {
	operationContext, cancel := context.WithTimeout(ctx, apiOperationTimeout)
	defer cancel()
	return operation(operationContext)
}
