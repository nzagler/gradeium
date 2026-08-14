package httpserver

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter wires infrastructure endpoints before the SPA fallback.
func NewRouter(logger *slog.Logger, readiness ReadinessChecker, web http.Handler) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(securityHeaders)
	router.Use(requestLogger(logger))
	router.Use(recoverer(logger))

	router.Get("/api/healthz", healthHandler)
	router.Get("/api/readyz", readinessHandler(readiness))
	router.Handle("/api", http.HandlerFunc(apiNotFoundHandler))
	router.Handle("/api/*", http.HandlerFunc(apiNotFoundHandler))
	router.Handle("/", web)
	router.Handle("/*", web)
	return router
}
