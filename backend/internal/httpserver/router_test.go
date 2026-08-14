package httpserver

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestRouterKeepsAPIRoutesOutOfSPAFallback(t *testing.T) {
	webFS := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("spa-index")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log('ok')")},
	}
	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), readinessStub{}, NewSPAHandler(webFS))

	tests := []struct {
		path       string
		wantStatus int
		wantBody   string
	}{
		{path: "/", wantStatus: http.StatusOK, wantBody: "spa-index"},
		{path: "/games", wantStatus: http.StatusOK, wantBody: "spa-index"},
		{path: "/assets/app.js", wantStatus: http.StatusOK, wantBody: "console.log('ok')"},
		{path: "/api/unknown", wantStatus: http.StatusNotFound, wantBody: "{\"error\":\"not_found\"}\n"},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, test.path, nil)
			router.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if body := response.Body.String(); body != test.wantBody {
				t.Fatalf("body = %q, want %q", body, test.wantBody)
			}
		})
	}
}
