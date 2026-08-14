package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/nzagler/gradeium/backend/internal/secrets"
	"github.com/nzagler/gradeium/backend/internal/settings"
	"golang.org/x/oauth2"
)

const (
	flowLifetime    = 10 * time.Minute
	sessionLifetime = 12 * time.Hour
	cacheLifetime   = 15 * time.Minute
	cleanupLimit    = 500
)

type cachedRuntime struct {
	revision  int64
	expiresAt time.Time
	runtime   *providerRuntime
}

type Service struct {
	store    Store
	secrets  *secrets.Service
	cipher   *secrets.Cipher
	protocol *OIDCProtocol
	now      func() time.Time

	cacheMutex sync.Mutex
	cache      *cachedRuntime
}

func NewService(store Store, secretService *secrets.Service, cipher *secrets.Cipher, protocol *OIDCProtocol) *Service {
	return &Service{
		store: store, secrets: secretService, cipher: cipher, protocol: protocol, now: time.Now,
	}
}

func (service *Service) State(ctx context.Context) (State, error) {
	return service.store.State(ctx)
}

func (service *Service) Configuration(ctx context.Context) (ConfigurationView, error) {
	state, err := service.store.State(ctx)
	if err != nil {
		return ConfigurationView{}, err
	}
	draft, err := service.store.LoadDraft(ctx)
	if err != nil {
		return ConfigurationView{}, err
	}
	view := ConfigurationView{
		Configuration:          draft.Configuration,
		Revision:               draft.Revision,
		Activated:              state.Activated,
		Validated:              draft.Validated,
		ValidatedAt:            draft.ValidatedAt,
		ClientSecretConfigured: draft.ClientSecretConfigured,
	}
	if draft.PublicURL != "" {
		view.RedirectURI = draft.RedirectURI()
	}
	return view, nil
}

// SaveConfiguration stores an unvalidated draft before activation. After
// activation it validates the candidate first and publishes it only after the
// exact saved revision is marked valid, preserving the last-known-good login.
func (service *Service) SaveConfiguration(ctx context.Context, input ConfigurationInput) (ConfigurationView, error) {
	configuration, err := NormalizeConfiguration(input)
	if err != nil {
		return ConfigurationView{}, &SafeError{Code: "validation_error", Message: err.Error()}
	}
	state, err := service.store.State(ctx)
	if err != nil {
		return ConfigurationView{}, err
	}
	draft, err := service.store.LoadDraft(ctx)
	if err != nil {
		return ConfigurationView{}, err
	}
	mutation, record, plaintext, err := service.prepareDraftSecret(input, draft)
	if err != nil {
		return ConfigurationView{}, err
	}
	if plaintext != nil {
		defer secrets.Clear(plaintext)
	}

	validated := false
	if state.Activated {
		if _, err := service.protocol.Discover(ctx, configuration, plaintext); err != nil {
			return ConfigurationView{}, &SafeError{Code: "oidc_validation_failed", Message: err.Error()}
		}
		validated = true
	}
	revision, err := service.store.SaveDraft(ctx, draft.Revision, configuration, mutation, record, validated)
	if err != nil {
		if errors.Is(err, ErrConfigurationStale) {
			return ConfigurationView{}, &SafeError{Code: "configuration_changed", Message: "Authentication configuration changed in another request. Reload and try again."}
		}
		return ConfigurationView{}, err
	}

	if state.Activated {
		activeSecret, err := service.sealActiveSecret(plaintext)
		if err != nil {
			return ConfigurationView{}, err
		}
		applied, err := service.store.Apply(ctx, revision, configuration, activeSecret)
		if err != nil {
			return ConfigurationView{}, err
		}
		if !applied {
			return ConfigurationView{}, &SafeError{Code: "configuration_changed", Message: "Authentication configuration changed before it could be applied. The previous login configuration remains active."}
		}
		service.clearRuntimeCache()
	}
	return service.Configuration(ctx)
}

func (service *Service) prepareDraftSecret(input ConfigurationInput, draft Draft) (SecretMutation, *secrets.Record, []byte, error) {
	if input.RemoveClientSecret && input.ClientSecret != "" {
		return KeepSecret, nil, nil, &SafeError{Code: "validation_error", Message: "A client secret cannot be replaced and removed in the same request."}
	}
	if input.RemoveClientSecret {
		return RemoveSecret, nil, nil, nil
	}
	if input.ClientSecret != "" {
		record, err := service.secrets.Seal(settings.AuthenticationClientSecretKey, input.ClientSecret)
		if err != nil {
			return KeepSecret, nil, nil, &SafeError{Code: "validation_error", Message: err.Error()}
		}
		return ReplaceSecret, &record, []byte(input.ClientSecret), nil
	}
	if draft.ClientSecretRecord == nil {
		return KeepSecret, nil, nil, nil
	}
	plaintext, err := service.secrets.Open(*draft.ClientSecretRecord)
	if err != nil {
		return KeepSecret, nil, nil, err
	}
	return KeepSecret, nil, plaintext, nil
}

func (service *Service) TestConfiguration(ctx context.Context) (ValidationResult, error) {
	draft, err := service.store.LoadDraft(ctx)
	if err != nil {
		return ValidationResult{}, err
	}
	configuration, err := NormalizeConfiguration(ConfigurationInput{
		IssuerURL: draft.IssuerURL, ClientID: draft.ClientID, PublicURL: draft.PublicURL,
	})
	if err != nil {
		return ValidationResult{}, &SafeError{Code: "validation_error", Message: err.Error()}
	}
	secret, err := service.openDraftSecret(draft)
	if err != nil {
		return ValidationResult{}, err
	}
	if secret != nil {
		defer secrets.Clear(secret)
	}
	if _, err := service.protocol.Discover(ctx, configuration, secret); err != nil {
		return ValidationResult{}, &SafeError{Code: "oidc_validation_failed", Message: err.Error()}
	}
	marked, err := service.store.MarkValidated(ctx, draft.Revision)
	if err != nil {
		return ValidationResult{}, err
	}
	if !marked {
		return ValidationResult{}, &SafeError{Code: "configuration_changed", Message: "Authentication configuration changed during validation. Test it again."}
	}
	return ValidationResult{
		Revision: draft.Revision, RedirectURI: configuration.RedirectURI(), IssuerURL: configuration.IssuerURL, Validated: true,
	}, nil
}

func (service *Service) Activate(ctx context.Context) (ConfigurationView, error) {
	state, err := service.store.State(ctx)
	if err != nil {
		return ConfigurationView{}, err
	}
	if state.Activated {
		return ConfigurationView{}, &SafeError{Code: "authentication_already_activated", Message: "Authentication has already been activated."}
	}
	draft, err := service.store.LoadDraft(ctx)
	if err != nil {
		return ConfigurationView{}, err
	}
	if !draft.Validated {
		return ConfigurationView{}, &SafeError{Code: "configuration_not_validated", Message: "Test the current authentication configuration before enabling it."}
	}
	secret, err := service.openDraftSecret(draft)
	if err != nil {
		return ConfigurationView{}, err
	}
	if secret != nil {
		defer secrets.Clear(secret)
	}
	activeSecret, err := service.sealActiveSecret(secret)
	if err != nil {
		return ConfigurationView{}, err
	}
	activated, err := service.store.Activate(ctx, draft.Revision, draft.Configuration, activeSecret)
	if err != nil {
		return ConfigurationView{}, err
	}
	if !activated {
		return ConfigurationView{}, &SafeError{Code: "authentication_activation_conflict", Message: "Authentication was activated or changed by another request."}
	}
	service.clearRuntimeCache()
	return service.Configuration(ctx)
}

func (service *Service) openDraftSecret(draft Draft) ([]byte, error) {
	if draft.ClientSecretRecord == nil {
		return nil, nil
	}
	return service.secrets.Open(*draft.ClientSecretRecord)
}

func (service *Service) sealActiveSecret(plaintext []byte) (*secrets.Record, error) {
	if len(plaintext) == 0 {
		return nil, nil
	}
	record, err := service.secrets.Seal(settings.AuthenticationActiveClientSecretKey, string(plaintext))
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (service *Service) StartLogin(ctx context.Context, returnPath string) (string, error) {
	state, err := service.store.State(ctx)
	if err != nil {
		return "", err
	}
	if !state.Activated || state.Active == nil || state.ActiveRevision == nil {
		return "", &SafeError{Code: "authentication_not_activated", Message: "Authentication has not been activated."}
	}
	// Public login starts also advance bounded expiry cleanup, so abandoned
	// one-time flows do not rely on a successful callback or process restart.
	_ = service.store.Cleanup(ctx, cleanupLimit)
	runtime, err := service.activeRuntime(ctx, state)
	if err != nil {
		return "", &SafeError{Code: "oidc_unavailable", Message: "The OIDC provider could not be reached. Try again later."}
	}
	stateToken, err := randomToken()
	if err != nil {
		return "", err
	}
	nonce, err := randomToken()
	if err != nil {
		return "", err
	}
	pkceVerifier := oauth2.GenerateVerifier()
	stateHash, _ := tokenHash(stateToken)
	payload := FlowPayload{
		Nonce: nonce, PKCEVerifier: pkceVerifier, ReturnPath: SafeReturnPath(returnPath), ActiveRevision: *state.ActiveRevision,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", errors.New("encode OIDC login flow")
	}
	defer secrets.Clear(encoded)
	envelope, err := service.cipher.Encrypt(flowEncryptionKey(stateHash), encoded)
	if err != nil {
		return "", err
	}
	if err := service.store.SaveFlow(ctx, FlowRecord{
		StateHash: stateHash, Envelope: envelope, ExpiresAt: service.now().Add(flowLifetime),
	}); err != nil {
		return "", err
	}
	return service.protocol.AuthorizationURL(runtime, stateToken, nonce, pkceVerifier), nil
}

func (service *Service) CompleteCallback(ctx context.Context, stateToken, code, previousSessionToken string) (LoginResult, error) {
	stateHash, err := tokenHash(stateToken)
	if err != nil {
		return LoginResult{}, &SafeError{Code: "invalid_oidc_callback", Message: "The sign-in request is invalid or expired."}
	}
	flow, err := service.store.ConsumeFlow(ctx, stateHash)
	if err != nil {
		return LoginResult{}, &SafeError{Code: "invalid_oidc_callback", Message: "The sign-in request is invalid or expired."}
	}
	if !flow.ExpiresAt.After(service.now()) {
		return LoginResult{}, &SafeError{Code: "expired_oidc_callback", Message: "The sign-in request expired. Start again."}
	}
	encoded, err := service.cipher.Decrypt(flowEncryptionKey(stateHash), flow.Envelope)
	if err != nil {
		return LoginResult{}, &SafeError{Code: "invalid_oidc_callback", Message: "The sign-in request could not be verified."}
	}
	defer secrets.Clear(encoded)
	var payload FlowPayload
	if err := json.Unmarshal(encoded, &payload); err != nil {
		return LoginResult{}, &SafeError{Code: "invalid_oidc_callback", Message: "The sign-in request could not be verified."}
	}
	state, err := service.store.State(ctx)
	if err != nil {
		return LoginResult{}, err
	}
	if !state.Activated || state.ActiveRevision == nil || *state.ActiveRevision != payload.ActiveRevision {
		return LoginResult{}, &SafeError{Code: "oidc_configuration_changed", Message: "Authentication configuration changed during sign in. Start again."}
	}
	runtime, err := service.activeRuntime(ctx, state)
	if err != nil {
		return LoginResult{}, &SafeError{Code: "oidc_unavailable", Message: "The OIDC provider could not be reached. Try again later."}
	}
	identity, err := service.protocol.Exchange(ctx, runtime, code, payload.PKCEVerifier, payload.Nonce)
	if err != nil {
		return LoginResult{}, &SafeError{Code: "oidc_login_failed", Message: "The OIDC identity could not be verified."}
	}
	sessionToken, err := randomToken()
	if err != nil {
		return LoginResult{}, err
	}
	sessionHash, _ := tokenHash(sessionToken)
	var previousHash *[32]byte
	if previousSessionToken != "" {
		if hash, hashErr := tokenHash(previousSessionToken); hashErr == nil {
			previousHash = &hash
		}
	}
	expiresAt := service.now().Add(sessionLifetime)
	user, err := service.store.CompleteLogin(ctx, identity, sessionHash, expiresAt, previousHash)
	if err != nil {
		return LoginResult{}, err
	}
	_ = service.store.Cleanup(ctx, cleanupLimit)
	return LoginResult{
		User: user, SessionToken: sessionToken, ExpiresAt: expiresAt,
		SecureCookie: strings.HasPrefix(state.Active.PublicURL, "https://"), ReturnPath: SafeReturnPath(payload.ReturnPath),
	}, nil
}

func (service *Service) Authenticate(ctx context.Context, sessionToken string) (Session, error) {
	hash, err := tokenHash(sessionToken)
	if err != nil {
		return Session{}, ErrSessionNotFound
	}
	return service.store.Session(ctx, hash)
}

func (service *Service) CSRFToken(sessionToken string) string {
	return csrfToken(service.cipher, sessionToken)
}

func (service *Service) ValidateCSRF(sessionToken, submitted string) bool {
	return submitted != "" && validCSRFToken(service.cipher, sessionToken, submitted)
}

func (service *Service) Logout(ctx context.Context, sessionToken string) (bool, error) {
	hash, err := tokenHash(sessionToken)
	if err != nil {
		return false, ErrSessionNotFound
	}
	return service.store.RevokeSession(ctx, hash)
}

func (service *Service) Cleanup(ctx context.Context) error {
	return service.store.Cleanup(ctx, cleanupLimit)
}

func (service *Service) activeRuntime(ctx context.Context, state State) (*providerRuntime, error) {
	service.cacheMutex.Lock()
	if service.cache != nil && state.ActiveRevision != nil && service.cache.revision == *state.ActiveRevision && service.cache.expiresAt.After(service.now()) {
		runtime := service.cache.runtime
		service.cacheMutex.Unlock()
		return runtime, nil
	}
	service.cacheMutex.Unlock()

	var clientSecret []byte
	clientSecret, err := service.secrets.Read(ctx, settings.AuthenticationActiveClientSecretKey)
	if errors.Is(err, secrets.ErrSecretNotFound) {
		clientSecret = nil
	} else if err != nil {
		return nil, err
	}
	if clientSecret != nil {
		defer secrets.Clear(clientSecret)
	}
	runtime, err := service.protocol.Discover(ctx, *state.Active, clientSecret)
	if err != nil {
		return nil, err
	}
	service.cacheMutex.Lock()
	service.cache = &cachedRuntime{revision: *state.ActiveRevision, expiresAt: service.now().Add(cacheLifetime), runtime: runtime}
	service.cacheMutex.Unlock()
	return runtime, nil
}

func (service *Service) clearRuntimeCache() {
	service.cacheMutex.Lock()
	service.cache = nil
	service.cacheMutex.Unlock()
}

func flowEncryptionKey(hash [sha256.Size]byte) string {
	return "oidc-login-flow:" + base64.RawURLEncoding.EncodeToString(hash[:])
}

// SameOrigin reports whether an Origin header matches the configured public
// origin. An absent Origin remains acceptable when the session-bound CSRF token
// is valid, which keeps non-browser clients usable.
func SameOrigin(publicURL, origin string) bool {
	if origin == "" {
		return true
	}
	public, err := url.Parse(publicURL)
	if err != nil {
		return false
	}
	submitted, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if !submitted.IsAbs() || submitted.User != nil || submitted.RawQuery != "" || submitted.Fragment != "" || submitted.Path != "" {
		return false
	}
	return submitted.Scheme == public.Scheme && strings.EqualFold(submitted.Host, public.Host) && submitted.Path == ""
}

// AsSafeError extracts an error that may be returned to a browser.
func AsSafeError(err error) *SafeError {
	var result *SafeError
	if errors.As(err, &result) {
		return result
	}
	return nil
}
