package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/nzagler/gradeium/backend/internal/database/sqlc"
	"github.com/nzagler/gradeium/backend/internal/integrations/igdb"
	"github.com/nzagler/gradeium/backend/internal/integrations/jellyfin"
	"github.com/nzagler/gradeium/backend/internal/integrations/tmdb"
	"github.com/nzagler/gradeium/backend/internal/integrations/tvdb"
	"github.com/nzagler/gradeium/backend/internal/secrets"
	"github.com/nzagler/gradeium/backend/internal/settings"
)

type Settings interface {
	List(context.Context) ([]settings.Value, error)
	Update(context.Context, string, json.RawMessage) (json.RawMessage, error)
}

type Secrets interface {
	Configured(context.Context, string) (bool, error)
	Set(context.Context, string, string) error
	Read(context.Context, string) ([]byte, error)
	Delete(context.Context, string) (bool, error)
}

type testStatusStore interface {
	List(context.Context) ([]TestStatus, error)
	Upsert(context.Context, TestStatus) (TestStatus, error)
	Delete(context.Context, string) error
}

type PostgresStatusStore struct{ queries *db.Queries }

func NewPostgresStatusStore(pool *pgxpool.Pool) *PostgresStatusStore {
	return &PostgresStatusStore{queries: db.New(pool)}
}
func (store *PostgresStatusStore) List(ctx context.Context) ([]TestStatus, error) {
	rows, err := store.queries.ListIntegrationTestStatuses(ctx)
	if err != nil {
		return nil, fmt.Errorf("list integration test statuses: %w", err)
	}
	result := make([]TestStatus, 0, len(rows))
	for _, row := range rows {
		result = append(result, TestStatus{Provider: row.Provider, Status: row.Status, Message: row.Message, TestedAt: timestamp(row.TestedAt)})
	}
	return result, nil
}
func (store *PostgresStatusStore) Upsert(ctx context.Context, status TestStatus) (TestStatus, error) {
	row, err := store.queries.UpsertIntegrationTestStatus(ctx, db.UpsertIntegrationTestStatusParams{Provider: status.Provider, Status: status.Status, Message: status.Message})
	if err != nil {
		return TestStatus{}, fmt.Errorf("persist integration test status: %w", err)
	}
	return TestStatus{Provider: row.Provider, Status: row.Status, Message: row.Message, TestedAt: timestamp(row.TestedAt)}, nil
}

func (store *PostgresStatusStore) Delete(ctx context.Context, provider string) error {
	if err := store.queries.DeleteIntegrationTestStatus(ctx, provider); err != nil {
		return fmt.Errorf("delete integration test status: %w", err)
	}
	return nil
}

type TestStatus struct {
	Provider string    `json:"provider"`
	Status   string    `json:"status"`
	Message  string    `json:"message"`
	TestedAt time.Time `json:"testedAt"`
}
type ProviderView struct {
	Provider         string                    `json:"provider"`
	Enabled          bool                      `json:"enabled"`
	Configured       bool                      `json:"configured"`
	State            string                    `json:"state"`
	ClientID         string                    `json:"clientId,omitempty"`
	BaseURL          string                    `json:"baseUrl,omitempty"`
	LibraryMappings  []jellyfin.LibraryMapping `json:"libraryMappings,omitempty"`
	SecretConfigured bool                      `json:"secretConfigured"`
	PINConfigured    bool                      `json:"pinConfigured,omitempty"`
	LastTest         *TestStatus               `json:"lastTest,omitempty"`
}
type ConfigurationInput struct {
	Enabled         bool                      `json:"enabled"`
	ClientID        string                    `json:"clientId"`
	Secret          string                    `json:"secret"`
	RemoveSecret    bool                      `json:"removeSecret"`
	PIN             string                    `json:"pin"`
	RemovePIN       bool                      `json:"removePin"`
	BaseURL         string                    `json:"baseUrl"`
	LibraryMappings []jellyfin.LibraryMapping `json:"libraryMappings"`
}

type Service struct {
	settings Settings
	secrets  Secrets
	statuses testStatusStore
}

func NewService(settingsService Settings, secretsService Secrets, statuses testStatusStore) *Service {
	return &Service{settings: settingsService, secrets: secretsService, statuses: statuses}
}

func (service *Service) List(ctx context.Context) ([]ProviderView, error) {
	values, err := service.settings.List(ctx)
	if err != nil {
		return nil, err
	}
	public := map[string]json.RawMessage{}
	for _, value := range values {
		public[value.Definition.Key] = value.Value
	}
	statuses, err := service.statuses.List(ctx)
	if err != nil {
		return nil, err
	}
	statusByProvider := map[string]TestStatus{}
	for _, status := range statuses {
		statusByProvider[status.Provider] = status
	}
	definitions := []struct{ provider, enabledKey, clientIDKey, secretKey, pinKey, baseURLKey string }{{"igdb", settings.IGDBEnabledKey, settings.IGDBClientIDKey, settings.IGDBClientSecretKey, "", ""}, {"tmdb", settings.TMDBEnabledKey, "", settings.TMDBAccessTokenKey, "", ""}, {"tvdb", settings.TVDBEnabledKey, "", settings.TVDBAPIKey, settings.TVDBPINKey, ""}, {"jellyfin", settings.JellyfinEnabledKey, "", settings.JellyfinAPIKey, "", settings.JellyfinBaseURLKey}}
	result := make([]ProviderView, 0, len(definitions))
	for _, definition := range definitions {
		view := ProviderView{Provider: definition.provider, Enabled: decodeBool(public[definition.enabledKey]), ClientID: decodeString(public[definition.clientIDKey]), BaseURL: decodeString(public[definition.baseURLKey])}
		if definition.provider == "jellyfin" {
			view.LibraryMappings = decodeMappings(public[settings.JellyfinLibraryMappingsKey])
		}
		view.SecretConfigured, err = service.secrets.Configured(ctx, definition.secretKey)
		if err != nil {
			return nil, err
		}
		if definition.pinKey != "" {
			view.PINConfigured, err = service.secrets.Configured(ctx, definition.pinKey)
			if err != nil {
				return nil, err
			}
		}
		view.Configured = view.SecretConfigured && (definition.clientIDKey == "" || view.ClientID != "") && (definition.baseURLKey == "" || view.BaseURL != "")
		switch {
		case !view.Configured:
			view.State = "not_configured"
		case !view.Enabled:
			view.State = "disabled"
		default:
			view.State = "configured"
		}
		if status, ok := statusByProvider[definition.provider]; ok {
			copy := status
			view.LastTest = &copy
			if view.Enabled && view.Configured {
				view.State = status.Status
			}
		}
		result = append(result, view)
	}
	return result, nil
}

func (service *Service) Configure(ctx context.Context, provider string, input ConfigurationInput) (ProviderView, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	var enabledKey, clientIDKey, secretKey, pinKey, baseURLKey string
	switch provider {
	case "igdb":
		enabledKey, clientIDKey, secretKey = settings.IGDBEnabledKey, settings.IGDBClientIDKey, settings.IGDBClientSecretKey
		input.ClientID = strings.TrimSpace(input.ClientID)
		if input.ClientID == "" {
			return ProviderView{}, validationError("IGDB client ID is required")
		}
	case "tmdb":
		enabledKey, secretKey = settings.TMDBEnabledKey, settings.TMDBAccessTokenKey
	case "tvdb":
		enabledKey, secretKey, pinKey = settings.TVDBEnabledKey, settings.TVDBAPIKey, settings.TVDBPINKey
	case "jellyfin":
		enabledKey, secretKey, baseURLKey = settings.JellyfinEnabledKey, settings.JellyfinAPIKey, settings.JellyfinBaseURLKey
		normalized, err := jellyfin.NormalizeBaseURL(input.BaseURL)
		if err != nil {
			return ProviderView{}, validationError(err.Error())
		}
		input.BaseURL = normalized
		for index := range input.LibraryMappings {
			input.LibraryMappings[index].LibraryID = strings.TrimSpace(input.LibraryMappings[index].LibraryID)
		}
		if err := validateMappings(input.LibraryMappings); err != nil {
			return ProviderView{}, validationError(err.Error())
		}
	default:
		return ProviderView{}, validationError("provider is not supported")
	}
	if input.Secret != "" && input.RemoveSecret {
		return ProviderView{}, validationError("replace and remove cannot be requested together")
	}
	if input.PIN != "" && input.RemovePIN {
		return ProviderView{}, validationError("replace and remove PIN cannot be requested together")
	}
	if input.ClientID != "" && utf8.RuneCountInString(input.ClientID) > 512 {
		return ProviderView{}, validationError("client ID is too long")
	}
	if err := validateCredential(input.Secret); err != nil {
		return ProviderView{}, err
	}
	if err := validateCredential(input.PIN); err != nil {
		return ProviderView{}, err
	}
	if clientIDKey != "" {
		encoded, _ := json.Marshal(input.ClientID)
		if _, err := service.settings.Update(ctx, clientIDKey, encoded); err != nil {
			return ProviderView{}, err
		}
	}
	if baseURLKey != "" {
		encoded, _ := json.Marshal(input.BaseURL)
		if _, err := service.settings.Update(ctx, baseURLKey, encoded); err != nil {
			return ProviderView{}, err
		}
		mappings, _ := json.Marshal(input.LibraryMappings)
		encodedMappings, _ := json.Marshal(string(mappings))
		if _, err := service.settings.Update(ctx, settings.JellyfinLibraryMappingsKey, encodedMappings); err != nil {
			return ProviderView{}, err
		}
	}
	if input.Secret != "" {
		if err := service.secrets.Set(ctx, secretKey, input.Secret); err != nil {
			return ProviderView{}, err
		}
	} else if input.RemoveSecret {
		if _, err := service.secrets.Delete(ctx, secretKey); err != nil {
			return ProviderView{}, err
		}
	}
	if pinKey != "" {
		if input.PIN != "" {
			if err := service.secrets.Set(ctx, pinKey, input.PIN); err != nil {
				return ProviderView{}, err
			}
		} else if input.RemovePIN {
			if _, err := service.secrets.Delete(ctx, pinKey); err != nil {
				return ProviderView{}, err
			}
		}
	}
	enabled, _ := json.Marshal(input.Enabled)
	if _, err := service.settings.Update(ctx, enabledKey, enabled); err != nil {
		return ProviderView{}, err
	}
	if err := service.statuses.Delete(ctx, provider); err != nil {
		return ProviderView{}, err
	}
	views, err := service.List(ctx)
	if err != nil {
		return ProviderView{}, err
	}
	for _, view := range views {
		if view.Provider == provider {
			return view, nil
		}
	}
	return ProviderView{}, errors.New("provider configuration unavailable")
}

func (service *Service) Test(ctx context.Context, providerName string) (TestStatus, error) {
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	var test func(context.Context) error
	switch providerName {
	case "igdb":
		client, err := service.IGDB(ctx)
		if err != nil {
			return service.recordError(ctx, providerName, err)
		}
		test = client.Test
	case "tmdb":
		client, err := service.TMDB(ctx)
		if err != nil {
			return service.recordError(ctx, providerName, err)
		}
		test = client.Test
	case "tvdb":
		client, err := service.TVDB(ctx)
		if err != nil {
			return service.recordError(ctx, providerName, err)
		}
		test = client.Test
	case "jellyfin":
		client, err := service.Jellyfin(ctx)
		if err != nil {
			return service.recordError(ctx, providerName, err)
		}
		test = client.Test
	default:
		return TestStatus{}, validationError("provider is not supported")
	}
	if err := test(ctx); err != nil {
		return service.recordError(ctx, providerName, err)
	}
	return service.statuses.Upsert(ctx, TestStatus{Provider: providerName, Status: "connected", Message: "Connection successful."})
}
func (service *Service) recordError(ctx context.Context, providerName string, cause error) (TestStatus, error) {
	status, err := service.statuses.Upsert(ctx, TestStatus{Provider: providerName, Status: "error", Message: "Connection failed. Check the saved credentials and try again."})
	if err != nil {
		return TestStatus{}, err
	}
	return status, &SafeError{Code: providerName + "_connection_failed", Message: status.Message, Cause: cause}
}

type SafeError struct {
	Code, Message string
	Cause         error
}

func (err *SafeError) Error() string { return err.Message }
func (err *SafeError) Unwrap() error { return err.Cause }

func validationError(message string) error {
	return &SafeError{Code: "validation_error", Message: message}
}

func validateCredential(value string) error {
	if len(value) > 16*1024 || !utf8.ValidString(value) {
		return validationError("credential value is invalid")
	}
	return nil
}

func (service *Service) IGDB(ctx context.Context) (*igdb.Client, error) {
	views, err := service.List(ctx)
	if err != nil {
		return nil, err
	}
	view := find(views, "igdb")
	if !view.Enabled || !view.Configured {
		return nil, errors.New("IGDB is not enabled and configured")
	}
	secret, err := service.secrets.Read(ctx, settings.IGDBClientSecretKey)
	if err != nil {
		return nil, err
	}
	defer clear(secret)
	return igdb.NewClient(view.ClientID, string(secret)), nil
}
func (service *Service) TMDB(ctx context.Context) (*tmdb.Client, error) {
	views, err := service.List(ctx)
	if err != nil {
		return nil, err
	}
	view := find(views, "tmdb")
	if !view.Enabled || !view.Configured {
		return nil, errors.New("TMDB is not enabled and configured")
	}
	secret, err := service.secrets.Read(ctx, settings.TMDBAccessTokenKey)
	if err != nil {
		return nil, err
	}
	defer clear(secret)
	return tmdb.NewClient(string(secret)), nil
}
func (service *Service) TVDB(ctx context.Context) (*tvdb.Client, error) {
	views, err := service.List(ctx)
	if err != nil {
		return nil, err
	}
	view := find(views, "tvdb")
	if !view.Enabled || !view.Configured {
		return nil, errors.New("TVDB is not enabled and configured")
	}
	key, err := service.secrets.Read(ctx, settings.TVDBAPIKey)
	if err != nil {
		return nil, err
	}
	defer clear(key)
	pin, err := service.secrets.Read(ctx, settings.TVDBPINKey)
	if errors.Is(err, secrets.ErrSecretNotFound) {
		pin = nil
	} else if err != nil {
		return nil, err
	}
	defer clear(pin)
	return tvdb.NewClient(string(key), string(pin)), nil
}

func (service *Service) Jellyfin(ctx context.Context) (*jellyfin.Client, error) {
	views, err := service.List(ctx)
	if err != nil {
		return nil, err
	}
	view := find(views, "jellyfin")
	if !view.Enabled || !view.Configured {
		return nil, errors.New("Jellyfin is not enabled and configured")
	}
	key, err := service.secrets.Read(ctx, settings.JellyfinAPIKey)
	if err != nil {
		return nil, err
	}
	defer clear(key)
	return jellyfin.NewClient(view.BaseURL, string(key))
}

func (service *Service) JellyfinMappings(ctx context.Context) ([]jellyfin.LibraryMapping, error) {
	views, err := service.List(ctx)
	if err != nil {
		return nil, err
	}
	return append([]jellyfin.LibraryMapping(nil), find(views, "jellyfin").LibraryMappings...), nil
}

func validateMappings(values []jellyfin.LibraryMapping) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value.LibraryID = strings.TrimSpace(value.LibraryID)
		if value.LibraryID == "" {
			return errors.New("Jellyfin library mappings require a library ID")
		}
		if value.Domain != jellyfin.Movies && value.Domain != jellyfin.TVShows {
			return errors.New("Jellyfin libraries can map only to Movies or TV Shows")
		}
		if _, duplicate := seen[value.LibraryID]; duplicate {
			return errors.New("each Jellyfin library can be mapped only once")
		}
		seen[value.LibraryID] = struct{}{}
	}
	return nil
}

func find(values []ProviderView, provider string) ProviderView {
	for _, value := range values {
		if value.Provider == provider {
			return value
		}
	}
	return ProviderView{Provider: provider}
}
func decodeBool(value json.RawMessage) bool {
	var result bool
	_ = json.Unmarshal(value, &result)
	return result
}
func decodeString(value json.RawMessage) string {
	var result string
	_ = json.Unmarshal(value, &result)
	return result
}

func decodeMappings(value json.RawMessage) []jellyfin.LibraryMapping {
	encoded := decodeString(value)
	result := []jellyfin.LibraryMapping{}
	if json.Unmarshal([]byte(encoded), &result) != nil || validateMappings(result) != nil {
		return []jellyfin.LibraryMapping{}
	}
	return result
}

func timestamp(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}
