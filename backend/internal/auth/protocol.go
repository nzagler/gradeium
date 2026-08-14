package auth

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const providerResponseLimit = 2 << 20

type providerRuntime struct {
	provider  *oidc.Provider
	oauth     oauth2.Config
	issuerURL string
}

// OIDCProtocol owns discovery, endpoint validation, PKCE authorization URLs,
// token exchange, and strict ID-token verification.
type OIDCProtocol struct {
	client *http.Client
}

func NewOIDCProtocol(client *http.Client) *OIDCProtocol {
	return &OIDCProtocol{client: client}
}

func NewProviderHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          20,
		MaxIdleConnsPerHost:   4,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   4 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	return &http.Client{
		Transport: limitedRoundTripper{next: transport, limit: providerResponseLimit},
		Timeout:   8 * time.Second,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many provider redirects")
			}
			return nil
		},
	}
}

type limitedRoundTripper struct {
	next  http.RoundTripper
	limit int64
}

func (transport limitedRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := transport.next.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, transport.limit+1))
	closeErr := response.Body.Close()
	if readErr != nil {
		return nil, errors.New("read bounded provider response")
	}
	if int64(len(body)) > transport.limit {
		return nil, errors.New("provider response exceeded the size limit")
	}
	if closeErr != nil {
		return nil, errors.New("close provider response")
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	response.ContentLength = int64(len(body))
	return response, nil
}

func (protocol *OIDCProtocol) Discover(ctx context.Context, configuration Configuration, clientSecret []byte) (*providerRuntime, error) {
	discoveryContext := oidc.ClientContext(ctx, protocol.client)
	provider, err := oidc.NewProvider(discoveryContext, configuration.IssuerURL)
	if err != nil {
		return nil, errors.New("OIDC discovery failed")
	}
	var metadata struct {
		Issuer                string   `json:"issuer"`
		AuthorizationEndpoint string   `json:"authorization_endpoint"`
		TokenEndpoint         string   `json:"token_endpoint"`
		JWKSURI               string   `json:"jwks_uri"`
		TokenAuthMethods      []string `json:"token_endpoint_auth_methods_supported"`
	}
	if err := provider.Claims(&metadata); err != nil {
		return nil, errors.New("OIDC discovery document is invalid")
	}
	if metadata.Issuer != configuration.IssuerURL {
		return nil, errors.New("discovered issuer does not exactly match the configured issuer")
	}
	for _, endpoint := range []string{metadata.AuthorizationEndpoint, metadata.TokenEndpoint, metadata.JWKSURI} {
		if validateEndpoint(endpoint) != nil {
			return nil, errors.New("OIDC discovery returned an insecure or malformed endpoint")
		}
	}

	authStyle, requiresSecret, err := selectAuthStyle(metadata.TokenAuthMethods)
	if err != nil {
		return nil, err
	}
	if requiresSecret && len(clientSecret) == 0 {
		return nil, errors.New("OIDC provider requires a configured client secret")
	}
	endpoint := provider.Endpoint()
	endpoint.AuthStyle = authStyle
	runtime := &providerRuntime{
		provider: provider,
		oauth: oauth2.Config{
			ClientID:     configuration.ClientID,
			ClientSecret: string(clientSecret),
			Endpoint:     endpoint,
			RedirectURL:  configuration.RedirectURI(),
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
		issuerURL: configuration.IssuerURL,
	}
	return runtime, nil
}

func selectAuthStyle(methods []string) (oauth2.AuthStyle, bool, error) {
	if len(methods) == 0 {
		return oauth2.AuthStyleInHeader, true, nil
	}
	for _, method := range methods {
		if method == "client_secret_basic" {
			return oauth2.AuthStyleInHeader, true, nil
		}
	}
	for _, method := range methods {
		if method == "client_secret_post" {
			return oauth2.AuthStyleInParams, true, nil
		}
	}
	for _, method := range methods {
		if method == "none" {
			return oauth2.AuthStyleInParams, false, nil
		}
	}
	return oauth2.AuthStyleAutoDetect, false, errors.New("OIDC provider does not advertise a supported token endpoint authentication method")
}

func (protocol *OIDCProtocol) AuthorizationURL(runtime *providerRuntime, state, nonce, verifier string) string {
	return runtime.oauth.AuthCodeURL(
		state,
		oidc.Nonce(nonce),
		oauth2.S256ChallengeOption(verifier),
	)
}

func (protocol *OIDCProtocol) Exchange(ctx context.Context, runtime *providerRuntime, code, verifier, nonce string) (Identity, error) {
	if strings.TrimSpace(code) == "" {
		return Identity{}, errors.New("authorization code is missing")
	}
	exchangeContext := oidc.ClientContext(ctx, protocol.client)
	token, err := runtime.oauth.Exchange(exchangeContext, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Identity{}, errors.New("OIDC token exchange failed")
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return Identity{}, errors.New("OIDC token response did not contain an ID token")
	}
	verifierContext := oidc.ClientContext(context.Background(), protocol.client)
	idToken, err := runtime.provider.VerifierContext(verifierContext, &oidc.Config{ClientID: runtime.oauth.ClientID}).Verify(exchangeContext, rawIDToken)
	if err != nil {
		return Identity{}, errors.New("OIDC ID token verification failed")
	}
	if nonce == "" || idToken.Nonce == "" || subtle.ConstantTimeCompare([]byte(nonce), []byte(idToken.Nonce)) != 1 {
		return Identity{}, errors.New("OIDC nonce verification failed")
	}
	if strings.TrimSpace(idToken.Subject) == "" || len(idToken.Subject) > 512 {
		return Identity{}, errors.New("OIDC ID token did not contain a stable subject")
	}
	var claims struct {
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
		Email             string `json:"email"`
		EmailVerified     bool   `json:"email_verified"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return Identity{}, errors.New("OIDC identity claims are invalid")
	}
	identity := Identity{Issuer: runtime.issuerURL, Subject: idToken.Subject}
	displayName := strings.TrimSpace(claims.Name)
	if displayName == "" {
		displayName = strings.TrimSpace(claims.PreferredUsername)
	}
	if displayName != "" && len(displayName) <= 512 {
		identity.DisplayName = &displayName
	}
	if claims.EmailVerified {
		email := strings.TrimSpace(claims.Email)
		if email != "" && len(email) <= 512 {
			identity.Email = &email
		}
	}
	return identity, nil
}
