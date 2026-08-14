package auth

import (
	"context"
	"errors"
	"time"

	"github.com/nzagler/gradeium/backend/internal/secrets"
)

const (
	SessionCookieName = "gradeium_session"
	CSRFHeaderName    = "X-CSRF-Token"
	CallbackPath      = "/api/auth/callback"
)

var (
	ErrNotActivated       = errors.New("authentication is not activated")
	ErrAlreadyActivated   = errors.New("authentication is already activated")
	ErrConfigurationStale = errors.New("authentication configuration changed")
	ErrNotValidated       = errors.New("authentication configuration is not validated")
	ErrFlowNotFound       = errors.New("OIDC login flow was not found")
	ErrSessionNotFound    = errors.New("session was not found")
)

// SafeError is intentionally suitable for a browser response. It never wraps
// upstream response bodies, tokens, secrets, or database errors.
type SafeError struct {
	Code    string
	Message string
}

func (err *SafeError) Error() string { return err.Message }

// Configuration is a normalized generic OIDC configuration. Client secrets
// deliberately live outside this value.
type Configuration struct {
	IssuerURL string `json:"issuerUrl"`
	ClientID  string `json:"clientId"`
	PublicURL string `json:"publicUrl"`
}

func (configuration Configuration) RedirectURI() string {
	return configuration.PublicURL + CallbackPath
}

// ConfigurationInput is accepted from the UI. An empty ClientSecret retains
// an existing draft unless RemoveClientSecret is explicitly true.
type ConfigurationInput struct {
	IssuerURL          string `json:"issuerUrl"`
	ClientID           string `json:"clientId"`
	ClientSecret       string `json:"clientSecret"`
	PublicURL          string `json:"publicUrl"`
	RemoveClientSecret bool   `json:"removeClientSecret"`
}

type State struct {
	ConfigurationRevision int64
	ValidatedRevision     *int64
	ValidatedAt           *time.Time
	Activated             bool
	ActivatedAt           *time.Time
	ActiveRevision        *int64
	Active                *Configuration
}

type Draft struct {
	Configuration
	Revision               int64
	Validated              bool
	ValidatedAt            *time.Time
	ClientSecretConfigured bool
	ClientSecretRecord     *secrets.Record
}

type ValidationResult struct {
	Revision    int64  `json:"revision"`
	RedirectURI string `json:"redirectUri"`
	IssuerURL   string `json:"issuerUrl"`
	Validated   bool   `json:"validated"`
}

type ConfigurationView struct {
	Configuration
	Revision               int64      `json:"revision"`
	Activated              bool       `json:"activated"`
	Validated              bool       `json:"validated"`
	ValidatedAt            *time.Time `json:"validatedAt,omitempty"`
	ClientSecretConfigured bool       `json:"clientSecretConfigured"`
	RedirectURI            string     `json:"redirectUri,omitempty"`
}

type SecretMutation int

const (
	KeepSecret SecretMutation = iota
	ReplaceSecret
	RemoveSecret
)

type FlowRecord struct {
	StateHash [32]byte
	Envelope  secrets.Envelope
	ExpiresAt time.Time
}

type FlowPayload struct {
	Nonce          string `json:"nonce"`
	PKCEVerifier   string `json:"pkceVerifier"`
	ReturnPath     string `json:"returnPath"`
	ActiveRevision int64  `json:"activeRevision"`
}

type Identity struct {
	Issuer      string
	Subject     string
	DisplayName *string
	Email       *string
}

type User struct {
	ID          string  `json:"id"`
	Issuer      string  `json:"-"`
	Subject     string  `json:"-"`
	DisplayName *string `json:"displayName"`
	Email       *string `json:"email"`
	IsAdmin     bool    `json:"isAdmin"`
}

type Session struct {
	ID        string
	User      User
	ExpiresAt time.Time
	RevokedAt *time.Time
	PublicURL string
}

type LoginResult struct {
	User         User
	SessionToken string
	ExpiresAt    time.Time
	SecureCookie bool
	ReturnPath   string
}

// Store is the complete persistent boundary used by the auth service. The
// PostgreSQL implementation keeps multi-row security transitions atomic.
type Store interface {
	State(context.Context) (State, error)
	LoadDraft(context.Context) (Draft, error)
	SaveDraft(context.Context, int64, Configuration, SecretMutation, *secrets.Record, bool) (int64, error)
	MarkValidated(context.Context, int64) (bool, error)
	Activate(context.Context, int64, Configuration, *secrets.Record) (bool, error)
	Apply(context.Context, int64, Configuration, *secrets.Record) (bool, error)
	SaveFlow(context.Context, FlowRecord) error
	ConsumeFlow(context.Context, [32]byte) (FlowRecord, error)
	CompleteLogin(context.Context, Identity, [32]byte, time.Time, *[32]byte) (User, error)
	Session(context.Context, [32]byte) (Session, error)
	RevokeSession(context.Context, [32]byte) (bool, error)
	Cleanup(context.Context, int32) error
}
