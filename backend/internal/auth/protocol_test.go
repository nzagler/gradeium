package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

type testOIDCIssuer struct {
	server   *httptest.Server
	key      *rsa.PrivateKey
	wrongKey *rsa.PrivateKey
	mutex    sync.Mutex
	mode     string
	nonce    string
}

func newTestOIDCIssuer(t *testing.T) *testOIDCIssuer {
	t.Helper()
	issuer := &testOIDCIssuer{}
	var err error
	issuer.key, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate issuer key: %v", err)
	}
	issuer.wrongKey, err = rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate wrong issuer key: %v", err)
	}
	issuer.server = httptest.NewTLSServer(http.HandlerFunc(issuer.serveHTTP))
	t.Cleanup(issuer.server.Close)
	return issuer
}

func (issuer *testOIDCIssuer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/.well-known/openid-configuration":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                issuer.server.URL,
			"authorization_endpoint":                issuer.server.URL + "/authorize",
			"token_endpoint":                        issuer.server.URL + "/token",
			"jwks_uri":                              issuer.server.URL + "/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
			"token_endpoint_auth_methods_supported": []string{"client_secret_basic"},
		})
	case "/jwks":
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
			Key: &issuer.key.PublicKey, KeyID: "gradeium-test", Algorithm: string(jose.RS256), Use: "sig",
		}}})
	case "/token":
		username, password, ok := r.BasicAuth()
		if !ok || username != "gradeium-client" || password != "test-client-secret" {
			http.Error(w, "invalid client", http.StatusUnauthorized)
			return
		}
		if err := r.ParseForm(); err != nil || r.Form.Get("code_verifier") == "" || r.Form.Get("code") == "" {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		raw, err := issuer.idToken()
		if err != nil {
			http.Error(w, "token error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "discarded-access-token", "token_type": "Bearer", "expires_in": 60, "id_token": raw,
		})
	default:
		http.NotFound(w, r)
	}
}

func (issuer *testOIDCIssuer) idToken() (string, error) {
	issuer.mutex.Lock()
	mode, nonce := issuer.mode, issuer.nonce
	issuer.mutex.Unlock()
	now := time.Now()
	claims := map[string]any{
		"iss": issuer.server.URL, "sub": "pocket-id-compatible-subject", "aud": "gradeium-client",
		"iat": now.Unix(), "exp": now.Add(time.Minute).Unix(), "nonce": nonce,
		"name": "Gradeium Admin", "email": "admin@example.test", "email_verified": true,
	}
	key := issuer.key
	switch mode {
	case "wrong_audience":
		claims["aud"] = "another-client"
	case "wrong_issuer":
		claims["iss"] = "https://other-issuer.example"
	case "expired":
		claims["exp"] = now.Add(-time.Minute).Unix()
	case "wrong_nonce":
		claims["nonce"] = "different-nonce"
	case "wrong_signature":
		key = issuer.wrongKey
	}
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "gradeium-test"),
	)
	if err != nil {
		return "", err
	}
	return jwt.Signed(signer).Claims(claims).Serialize()
}

func (issuer *testOIDCIssuer) set(mode, nonce string) {
	issuer.mutex.Lock()
	issuer.mode = mode
	issuer.nonce = nonce
	issuer.mutex.Unlock()
}

func TestOIDCProtocolDiscoveryPKCEAndStrictTokenVerification(t *testing.T) {
	issuer := newTestOIDCIssuer(t)
	protocol := NewOIDCProtocol(issuer.server.Client())
	configuration := Configuration{
		IssuerURL: issuer.server.URL, ClientID: "gradeium-client", PublicURL: "https://gradeium.example",
	}
	runtime, err := protocol.Discover(context.Background(), configuration, []byte("test-client-secret"))
	if err != nil {
		t.Fatalf("Discover returned an error: %v", err)
	}
	state := "state-value"
	nonce := "nonce-value"
	verifier := "0123456789012345678901234567890123456789012345678901234567890123"
	authorizationURL := protocol.AuthorizationURL(runtime, state, nonce, verifier)
	parsed, err := url.Parse(authorizationURL)
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	query := parsed.Query()
	if query.Get("state") != state || query.Get("nonce") != nonce || query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") == "" {
		t.Fatalf("authorization query lacked state/nonce/PKCE: %v", query)
	}
	issuer.set("", nonce)
	identity, err := protocol.Exchange(context.Background(), runtime, "valid-code", verifier, nonce)
	if err != nil {
		t.Fatalf("Exchange returned an error: %v", err)
	}
	if identity.Issuer != issuer.server.URL || identity.Subject != "pocket-id-compatible-subject" || identity.DisplayName == nil || identity.Email == nil {
		t.Fatalf("verified identity = %#v", identity)
	}

	for _, mode := range []string{"wrong_audience", "wrong_issuer", "expired", "wrong_nonce", "wrong_signature"} {
		t.Run(mode, func(t *testing.T) {
			issuer.set(mode, nonce)
			if _, err := protocol.Exchange(context.Background(), runtime, "rejected-code", verifier, nonce); err == nil {
				t.Fatalf("%s token was accepted", mode)
			}
		})
	}
}

func TestOIDCProtocolRejectsDiscoveryIssuerMismatchAndMissingSecret(t *testing.T) {
	issuer := newTestOIDCIssuer(t)
	protocol := NewOIDCProtocol(issuer.server.Client())
	configuration := Configuration{IssuerURL: issuer.server.URL, ClientID: "gradeium-client", PublicURL: "https://gradeium.example"}
	if _, err := protocol.Discover(context.Background(), configuration, nil); err == nil || !strings.Contains(err.Error(), "client secret") {
		t.Fatalf("missing secret error = %v", err)
	}

	mismatch := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": "https://different.example", "authorization_endpoint": "https://different.example/auth",
			"token_endpoint": "https://different.example/token", "jwks_uri": "https://different.example/jwks",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	}))
	defer mismatch.Close()
	if _, err := NewOIDCProtocol(mismatch.Client()).Discover(context.Background(), Configuration{
		IssuerURL: mismatch.URL, ClientID: "client", PublicURL: "https://gradeium.example",
	}, []byte("secret")); err == nil {
		t.Fatal("discovery issuer mismatch was accepted")
	}
}
