package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityHeadersAllowOnlyPhase4MediaEmbeds(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))

	policy := response.Header().Get("Content-Security-Policy")
	for _, directive := range []string{
		"default-src 'self'",
		"frame-ancestors 'none'",
		"frame-src https://www.youtube-nocookie.com",
		"img-src 'self' data: https:",
		"object-src 'none'",
		"script-src 'self'",
	} {
		if !strings.Contains(policy, directive) {
			t.Fatalf("Content-Security-Policy %q does not contain %q", policy, directive)
		}
	}
}
