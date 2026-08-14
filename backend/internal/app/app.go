package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/nzagler/gradeium/backend/internal/config"
	"github.com/nzagler/gradeium/backend/internal/database"
	"github.com/nzagler/gradeium/backend/internal/httpserver"
)

const (
	migrationTimeout = 2 * time.Minute
	databaseTimeout  = 10 * time.Second
	shutdownTimeout  = 20 * time.Second
)

// Run starts Gradeium and blocks until the server fails or the context is cancelled.
func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	migrationCtx, cancelMigrations := context.WithTimeout(ctx, migrationTimeout)
	if err := database.Migrate(migrationCtx, cfg.DatabaseURL, logger); err != nil {
		cancelMigrations()
		return fmt.Errorf("apply database migrations: %w", err)
	}
	cancelMigrations()

	databaseCtx, cancelDatabase := context.WithTimeout(ctx, databaseTimeout)
	pool, err := database.Open(databaseCtx, cfg.DatabaseURL)
	cancelDatabase()
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	webHandler := http.NotFoundHandler()
	webFS := os.DirFS(cfg.WebDir)
	if _, err := fs.Stat(webFS, "index.html"); err != nil {
		logger.Warn("frontend assets unavailable; serving API only", "web_dir", cfg.WebDir)
	} else {
		webHandler = httpserver.NewSPAHandler(webFS)
	}

	router := httpserver.NewRouter(logger, pool, webHandler)
	server := httpserver.New(cfg.ListenAddress, router)

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("http server starting", "address", cfg.ListenAddress)
		err := server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serverErrors <- err
	}()

	select {
	case err := <-serverErrors:
		if err != nil {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return fmt.Errorf("graceful HTTP shutdown: %w", err)
	}

	if err := <-serverErrors; err != nil {
		return fmt.Errorf("serve HTTP during shutdown: %w", err)
	}

	logger.Info("shutdown complete")
	return nil
}
