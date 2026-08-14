package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/nzagler/gradeium/backend/internal/secrets"
	"github.com/nzagler/gradeium/backend/internal/settings"
)

type serviceSecretStore struct {
	records map[string]secrets.Record
}

func (store *serviceSecretStore) Get(_ context.Context, key string) (secrets.Record, error) {
	record, ok := store.records[key]
	if !ok {
		return secrets.Record{}, secrets.ErrSecretNotFound
	}
	return record, nil
}
func (store *serviceSecretStore) Upsert(_ context.Context, record secrets.Record) error {
	store.records[record.Key] = record
	return nil
}
func (store *serviceSecretStore) Delete(_ context.Context, key string) (bool, error) {
	_, found := store.records[key]
	delete(store.records, key)
	return found, nil
}
func (store *serviceSecretStore) List(context.Context) ([]secrets.Record, error) { return nil, nil }

type memoryAuthStore struct {
	mutex    sync.Mutex
	state    State
	flows    map[[32]byte]FlowRecord
	users    map[string]User
	sessions map[[32]byte]Session
	saves    int
}

func (store *memoryAuthStore) State(context.Context) (State, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	return store.state, nil
}
func (store *memoryAuthStore) LoadDraft(context.Context) (Draft, error) { return Draft{}, nil }
func (store *memoryAuthStore) SaveDraft(context.Context, int64, Configuration, SecretMutation, *secrets.Record, bool) (int64, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.saves++
	return 0, nil
}
func (store *memoryAuthStore) MarkValidated(context.Context, int64) (bool, error) { return true, nil }
func (store *memoryAuthStore) Activate(context.Context, int64, Configuration, *secrets.Record) (bool, error) {
	return true, nil
}
func (store *memoryAuthStore) Apply(context.Context, int64, Configuration, *secrets.Record) (bool, error) {
	return true, nil
}
func (store *memoryAuthStore) SaveFlow(_ context.Context, record FlowRecord) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.flows[record.StateHash] = record
	return nil
}
func (store *memoryAuthStore) ConsumeFlow(_ context.Context, hash [32]byte) (FlowRecord, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	record, ok := store.flows[hash]
	if !ok {
		return FlowRecord{}, ErrFlowNotFound
	}
	delete(store.flows, hash)
	return record, nil
}
func (store *memoryAuthStore) CompleteLogin(_ context.Context, identity Identity, hash [32]byte, expires time.Time, previous *[32]byte) (User, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	key := identity.Issuer + "\x00" + identity.Subject
	user, exists := store.users[key]
	if !exists {
		user = User{ID: key, Issuer: identity.Issuer, Subject: identity.Subject, DisplayName: identity.DisplayName, Email: identity.Email, IsAdmin: len(store.users) == 0}
		store.users[key] = user
	}
	if previous != nil {
		delete(store.sessions, *previous)
	}
	store.sessions[hash] = Session{ID: "session", User: user, ExpiresAt: expires, PublicURL: store.state.Active.PublicURL}
	return user, nil
}
func (store *memoryAuthStore) Session(_ context.Context, hash [32]byte) (Session, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	session, ok := store.sessions[hash]
	if !ok || !session.ExpiresAt.After(time.Now()) {
		return Session{}, ErrSessionNotFound
	}
	return session, nil
}
func (store *memoryAuthStore) RevokeSession(_ context.Context, hash [32]byte) (bool, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	_, found := store.sessions[hash]
	delete(store.sessions, hash)
	return found, nil
}
func (store *memoryAuthStore) Cleanup(context.Context, int32) error { return nil }

func TestServiceEndToEndFirstLoginCreatesLocalAdminSession(t *testing.T) {
	issuer := newTestOIDCIssuer(t)
	configuration := Configuration{IssuerURL: issuer.server.URL, ClientID: "gradeium-client", PublicURL: "https://gradeium.example"}
	revision := int64(1)
	store := &memoryAuthStore{
		state: State{Activated: true, ActiveRevision: &revision, Active: &configuration},
		flows: make(map[[32]byte]FlowRecord), users: make(map[string]User), sessions: make(map[[32]byte]Session),
	}
	cipher, err := secrets.NewCipher(bytes.Repeat([]byte{0x33}, 32))
	if err != nil {
		t.Fatalf("NewCipher returned an error: %v", err)
	}
	secretStore := &serviceSecretStore{records: make(map[string]secrets.Record)}
	secretService := secrets.NewService(settings.NewRegistry(), secretStore, cipher)
	if err := secretService.Set(context.Background(), settings.AuthenticationActiveClientSecretKey, "test-client-secret"); err != nil {
		t.Fatalf("persist active test secret: %v", err)
	}
	service := NewService(store, secretService, cipher, NewOIDCProtocol(issuer.server.Client()))

	authorizationURL, err := service.StartLogin(context.Background(), "/settings/authentication")
	if err != nil {
		t.Fatalf("StartLogin returned an error: %v", err)
	}
	parsed, err := url.Parse(authorizationURL)
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	stateToken := parsed.Query().Get("state")
	nonce := parsed.Query().Get("nonce")
	if stateToken == "" || nonce == "" || parsed.Query().Get("code_challenge") == "" {
		t.Fatalf("authorization URL lacked security parameters: %s", authorizationURL)
	}
	issuer.set("", nonce)
	result, err := service.CompleteCallback(context.Background(), stateToken, "valid-code", "")
	if err != nil {
		t.Fatalf("CompleteCallback returned an error: %v", err)
	}
	if !result.User.IsAdmin || result.SessionToken == "" || result.ReturnPath != "/settings/authentication" {
		t.Fatalf("login result = %#v", result)
	}
	if _, err := service.Authenticate(context.Background(), result.SessionToken); err != nil {
		t.Fatalf("local session lookup failed: %v", err)
	}

	// Provider availability is not consulted for an existing Gradeium session.
	issuer.server.Close()
	if _, err := service.Authenticate(context.Background(), result.SessionToken); err != nil {
		t.Fatalf("provider outage invalidated local session: %v", err)
	}
	if _, err := service.Logout(context.Background(), result.SessionToken); err != nil {
		t.Fatalf("Logout returned an error: %v", err)
	}
	if _, err := service.Authenticate(context.Background(), result.SessionToken); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("revoked session error = %v, want ErrSessionNotFound", err)
	}
}

func TestActivatedServiceRejectsInvalidReplacementBeforeChangingStoredConfiguration(t *testing.T) {
	mismatch := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": "https://different.example", "authorization_endpoint": "https://different.example/authorize",
			"token_endpoint": "https://different.example/token", "jwks_uri": "https://different.example/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	}))
	defer mismatch.Close()
	revision := int64(4)
	active := Configuration{IssuerURL: "https://working.example", ClientID: "working-client", PublicURL: "https://gradeium.example"}
	store := &memoryAuthStore{
		state: State{Activated: true, ActiveRevision: &revision, Active: &active},
		flows: make(map[[32]byte]FlowRecord), users: make(map[string]User), sessions: make(map[[32]byte]Session),
	}
	cipher, err := secrets.NewCipher(bytes.Repeat([]byte{0x55}, 32))
	if err != nil {
		t.Fatalf("NewCipher returned an error: %v", err)
	}
	secretService := secrets.NewService(
		settings.NewRegistry(),
		&serviceSecretStore{records: make(map[string]secrets.Record)},
		cipher,
	)
	service := NewService(store, secretService, cipher, NewOIDCProtocol(mismatch.Client()))
	_, err = service.SaveConfiguration(context.Background(), ConfigurationInput{
		IssuerURL: mismatch.URL, ClientID: "replacement-client", ClientSecret: "replacement-secret", PublicURL: "https://gradeium.example",
	})
	if err == nil || AsSafeError(err) == nil {
		t.Fatalf("invalid replacement error = %v", err)
	}
	store.mutex.Lock()
	saves := store.saves
	unchanged := store.state.Active == &active && store.state.ActiveRevision != nil && *store.state.ActiveRevision == revision
	store.mutex.Unlock()
	if saves != 0 || !unchanged {
		t.Fatalf("invalid replacement changed stored state: saves=%d state=%#v", saves, store.state)
	}
}
