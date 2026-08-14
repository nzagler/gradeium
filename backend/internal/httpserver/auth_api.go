package httpserver

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	authpackage "github.com/nzagler/gradeium/backend/internal/auth"
)

type requestAuthentication struct {
	session authpackage.Session
	token   string
}

type requestAuthenticationKey struct{}

func (handlers *apiHandlers) authStatus(w http.ResponseWriter, r *http.Request) {
	complete, err := withTimeoutResult(r.Context(), handlers.setup.CompleteStatus)
	if err != nil {
		handlers.internalError(w, r, "read setup status for authentication", err)
		return
	}
	state, err := withTimeoutResult(r.Context(), handlers.authentication.State)
	if err != nil {
		handlers.internalError(w, r, "read authentication state", err)
		return
	}
	response := map[string]any{
		"setupComplete":    complete,
		"activated":        state.Activated,
		"bootstrapAllowed": complete && !state.Activated,
		"authenticated":    false,
	}
	identity, err := handlers.optionalAuthentication(r)
	if err != nil {
		handlers.internalError(w, r, "read current session", err)
		return
	}
	if identity != nil {
		response["authenticated"] = true
		response["user"] = identity.session.User
		response["csrfToken"] = handlers.authentication.CSRFToken(identity.token)
		response["sessionExpiresAt"] = identity.session.ExpiresAt
	}
	writeJSON(w, http.StatusOK, response)
}

func (handlers *apiHandlers) authConfiguration(w http.ResponseWriter, r *http.Request) {
	if !handlers.authorizeAuthenticationConfiguration(w, r, false) {
		return
	}
	view, err := withTimeoutResult(r.Context(), handlers.authentication.Configuration)
	if err != nil {
		handlers.authenticationError(w, r, "read authentication configuration", err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (handlers *apiHandlers) saveAuthConfiguration(w http.ResponseWriter, r *http.Request) {
	if !handlers.authorizeAuthenticationConfiguration(w, r, true) {
		return
	}
	var request authpackage.ConfigurationInput
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "Provide one valid authentication configuration.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	view, err := handlers.authentication.SaveConfiguration(ctx, request)
	if err != nil {
		handlers.authenticationError(w, r, "save authentication configuration", err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (handlers *apiHandlers) testAuthConfiguration(w http.ResponseWriter, r *http.Request) {
	if !handlers.authorizeAuthenticationConfiguration(w, r, true) {
		return
	}
	if !emptyBody(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	result, err := handlers.authentication.TestConfiguration(ctx)
	if err != nil {
		handlers.authenticationError(w, r, "test authentication configuration", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (handlers *apiHandlers) activateAuthentication(w http.ResponseWriter, r *http.Request) {
	if !handlers.authorizeAuthenticationConfiguration(w, r, true) {
		return
	}
	if !emptyBody(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), apiOperationTimeout)
	defer cancel()
	view, err := handlers.authentication.Activate(ctx)
	if err != nil {
		handlers.authenticationError(w, r, "activate authentication", err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (handlers *apiHandlers) startLogin(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	authorizationURL, err := handlers.authentication.StartLogin(ctx, r.URL.Query().Get("returnTo"))
	if err != nil {
		handlers.authenticationError(w, r, "start OIDC login", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"authorizationUrl": authorizationURL})
}

func (handlers *apiHandlers) authCallback(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("error") != "" {
		http.Redirect(w, r, "/login?error=oidc_login_failed", http.StatusSeeOther)
		return
	}
	previousToken := ""
	if cookie, err := r.Cookie(authpackage.SessionCookieName); err == nil {
		previousToken = cookie.Value
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	result, err := handlers.authentication.CompleteCallback(
		ctx,
		r.URL.Query().Get("state"),
		r.URL.Query().Get("code"),
		previousToken,
	)
	if err != nil {
		// Callback errors intentionally become a fixed same-origin route. Query
		// values and upstream details are never reflected or logged.
		handlers.logger.Warn("OIDC callback rejected", "reason", "verification_failed")
		http.Redirect(w, r, "/login?error=oidc_login_failed", http.StatusSeeOther)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: authpackage.SessionCookieName, Value: result.SessionToken,
		Path: "/", HttpOnly: true, Secure: result.SecureCookie, SameSite: http.SameSiteLaxMode,
		Expires: result.ExpiresAt, MaxAge: int(time.Until(result.ExpiresAt).Seconds()),
	})
	http.Redirect(w, r, result.ReturnPath, http.StatusSeeOther)
}

func (handlers *apiHandlers) authSession(w http.ResponseWriter, r *http.Request) {
	identity, status, err := handlers.requiredAuthentication(r)
	if err != nil {
		handlers.internalError(w, r, "read current session", err)
		return
	}
	if status != 0 {
		writeAPIError(w, status, "authentication_required", "Sign in to continue.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":      identity.session.User,
		"expiresAt": identity.session.ExpiresAt,
		"csrfToken": handlers.authentication.CSRFToken(identity.token),
	})
}

func (handlers *apiHandlers) logout(w http.ResponseWriter, r *http.Request) {
	identity, status, err := handlers.requiredAuthentication(r)
	if err != nil {
		handlers.internalError(w, r, "read session for logout", err)
		return
	}
	if status != 0 {
		writeAPIError(w, status, "authentication_required", "Sign in to continue.")
		return
	}
	if !handlers.validCSRFRequest(r, identity) {
		writeAPIError(w, http.StatusForbidden, "csrf_rejected", "The request could not be verified.")
		return
	}
	if _, err := handlers.authentication.Logout(r.Context(), identity.token); err != nil && !errors.Is(err, authpackage.ErrSessionNotFound) {
		handlers.internalError(w, r, "revoke current session", err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: authpackage.SessionCookieName, Value: "", Path: "/", HttpOnly: true,
		Secure: strings.HasPrefix(identity.session.PublicURL, "https://"), SameSite: http.SameSiteLaxMode,
		Expires: time.Unix(1, 0), MaxAge: -1,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
}

func (handlers *apiHandlers) authorizeAuthenticationConfiguration(w http.ResponseWriter, r *http.Request, unsafe bool) bool {
	complete, err := withTimeoutResult(r.Context(), handlers.setup.CompleteStatus)
	if err != nil {
		handlers.internalError(w, r, "check setup before authentication configuration", err)
		return false
	}
	if !complete {
		writeAPIError(w, http.StatusPreconditionRequired, "setup_required", "Initial setup must be completed first.")
		return false
	}
	state, err := withTimeoutResult(r.Context(), handlers.authentication.State)
	if err != nil {
		handlers.internalError(w, r, "check authentication activation", err)
		return false
	}
	if !state.Activated {
		return true
	}
	identity, status, err := handlers.requiredAuthentication(r)
	if err != nil {
		handlers.internalError(w, r, "authorize authentication configuration", err)
		return false
	}
	if status != 0 {
		writeAPIError(w, status, "authentication_required", "An administrator session is required.")
		return false
	}
	if !identity.session.User.IsAdmin {
		writeAPIError(w, http.StatusForbidden, "admin_required", "Administrator access is required.")
		return false
	}
	if unsafe && !handlers.validCSRFRequest(r, identity) {
		writeAPIError(w, http.StatusForbidden, "csrf_rejected", "The request could not be verified.")
		return false
	}
	return true
}

func (handlers *apiHandlers) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, status, err := handlers.requiredAuthentication(r)
		if err != nil {
			handlers.internalError(w, r, "authorize admin request", err)
			return
		}
		if status != 0 {
			writeAPIError(w, status, "authentication_required", "Sign in to continue.")
			return
		}
		if !identity.session.User.IsAdmin {
			writeAPIError(w, http.StatusForbidden, "admin_required", "Administrator access is required.")
			return
		}
		contextWithIdentity := context.WithValue(r.Context(), requestAuthenticationKey{}, identity)
		next.ServeHTTP(w, r.WithContext(contextWithIdentity))
	})
}

func (handlers *apiHandlers) requireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		identity, _ := r.Context().Value(requestAuthenticationKey{}).(*requestAuthentication)
		if identity == nil || !handlers.validCSRFRequest(r, identity) {
			writeAPIError(w, http.StatusForbidden, "csrf_rejected", "The request could not be verified.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (handlers *apiHandlers) validCSRFRequest(r *http.Request, identity *requestAuthentication) bool {
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
		return false
	}
	if !authpackage.SameOrigin(identity.session.PublicURL, r.Header.Get("Origin")) {
		return false
	}
	return handlers.authentication.ValidateCSRF(identity.token, r.Header.Get(authpackage.CSRFHeaderName))
}

func (handlers *apiHandlers) optionalAuthentication(r *http.Request) (*requestAuthentication, error) {
	cookie, err := r.Cookie(authpackage.SessionCookieName)
	if errors.Is(err, http.ErrNoCookie) {
		return nil, nil
	}
	if err != nil || cookie.Value == "" {
		return nil, nil
	}
	session, err := handlers.authentication.Authenticate(r.Context(), cookie.Value)
	if errors.Is(err, authpackage.ErrSessionNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &requestAuthentication{session: session, token: cookie.Value}, nil
}

func (handlers *apiHandlers) requiredAuthentication(r *http.Request) (*requestAuthentication, int, error) {
	identity, err := handlers.optionalAuthentication(r)
	if err != nil {
		return nil, 0, err
	}
	if identity == nil {
		return nil, http.StatusUnauthorized, nil
	}
	return identity, 0, nil
}

func (handlers *apiHandlers) authenticationError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	if safe := authpackage.AsSafeError(err); safe != nil {
		status := http.StatusBadRequest
		if strings.Contains(safe.Code, "changed") || strings.Contains(safe.Code, "conflict") || strings.Contains(safe.Code, "already") {
			status = http.StatusConflict
		}
		if safe.Code == "oidc_unavailable" {
			status = http.StatusBadGateway
		}
		writeAPIError(w, status, safe.Code, safe.Message)
		return
	}
	handlers.internalError(w, r, operation, err)
}

func emptyBody(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1)
	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) != 0 {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "This endpoint does not accept a request body.")
		return false
	}
	return true
}
