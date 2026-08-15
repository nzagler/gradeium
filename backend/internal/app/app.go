package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/nzagler/gradeium/backend/internal/auth"
	"github.com/nzagler/gradeium/backend/internal/backups"
	"github.com/nzagler/gradeium/backend/internal/buildinfo"
	"github.com/nzagler/gradeium/backend/internal/config"
	"github.com/nzagler/gradeium/backend/internal/dashboard"
	"github.com/nzagler/gradeium/backend/internal/database"
	"github.com/nzagler/gradeium/backend/internal/games"
	"github.com/nzagler/gradeium/backend/internal/httpserver"
	"github.com/nzagler/gradeium/backend/internal/integrations"
	"github.com/nzagler/gradeium/backend/internal/jellyfinsync"
	"github.com/nzagler/gradeium/backend/internal/media"
	"github.com/nzagler/gradeium/backend/internal/movies"
	"github.com/nzagler/gradeium/backend/internal/secrets"
	"github.com/nzagler/gradeium/backend/internal/settings"
	"github.com/nzagler/gradeium/backend/internal/setup"
	"github.com/nzagler/gradeium/backend/internal/tv"
)

const (
	migrationTimeout = 2 * time.Minute
	databaseTimeout  = 10 * time.Second
	securityTimeout  = 10 * time.Second
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

	registry := settings.NewRegistry()
	secretStore := secrets.NewPostgresStore(pool)
	securityCtx, cancelSecurity := context.WithTimeout(ctx, securityTimeout)
	secretCipher, err := secrets.InitializeCipher(securityCtx, cfg.ConfigDir, secretStore)
	if err != nil {
		cancelSecurity()
		return fmt.Errorf("initialize persistent master key: %w", err)
	}
	secretService := secrets.NewService(registry, secretStore, secretCipher)
	if err := secretService.ValidateStored(securityCtx); err != nil {
		cancelSecurity()
		return fmt.Errorf("validate encrypted settings: %w", err)
	}
	authService := auth.NewService(
		auth.NewPostgresStore(pool),
		secretService,
		secretCipher,
		auth.NewOIDCProtocol(auth.NewProviderHTTPClient()),
	)
	if err := authService.Cleanup(securityCtx); err != nil {
		cancelSecurity()
		return fmt.Errorf("clean authentication security state: %w", err)
	}
	cancelSecurity()

	setupService := setup.NewService(setup.NewPostgresStore(pool))
	settingsService := settings.NewService(registry, settings.NewPostgresStore(pool))
	integrationService := integrations.NewService(settingsService, secretService, integrations.NewPostgresStatusStore(pool))
	gameService := games.NewService(integrationService, games.NewPostgresStore(pool))
	movieService := movies.NewService(integrationService, movies.NewPostgresStore(pool))
	tvService := tv.NewService(integrationService, tv.NewPostgresStore(pool))
	preferenceService := media.NewPreferencesService(pool)
	backupService := backups.NewService(backups.NewPostgresStore(pool), cfg.BackupsDir, buildinfo.Version)
	dashboardService := dashboard.NewService(pool)
	jellyfinSyncService := jellyfinsync.NewService(
		movieService,
		tvService,
		func(ctx context.Context, userID string, providerID int64, status media.Status) error {
			_, err := movieService.Add(ctx, userID, providerID, status)
			return err
		},
		func(ctx context.Context, userID string, providerID int64, status media.Status) error {
			_, err := tvService.Add(ctx, userID, providerID, status)
			return err
		},
	)
	jellyfinSyncJobs := jellyfinsync.NewJobManager(ctx, jellyfinSyncService, jellyfinsync.DefaultJobTimeout)
	// This defer was registered after pool.Close, so active imports are cancelled
	// and joined before their database dependencies are closed.
	defer jellyfinSyncJobs.Close()
	apiHandler := httpserver.NewAPIWithPhase11(
		logger,
		setupService,
		settingsService,
		secretService,
		registry,
		authService,
		true,
		integrationService,
		gameService,
		movieService,
		tvService,
		preferenceService,
		backupService,
		dashboardService,
		jellyfinSyncJobs,
	)

	schedulerContext, cancelScheduler := context.WithCancel(ctx)
	var schedulerWait sync.WaitGroup
	schedulerWait.Add(1)
	go func() {
		defer schedulerWait.Done()
		backups.NewScheduler(backupService, logger).Run(schedulerContext)
	}()
	defer func() {
		cancelScheduler()
		schedulerWait.Wait()
	}()

	webHandler := http.NotFoundHandler()
	webFS := os.DirFS(cfg.WebDir)
	if _, err := fs.Stat(webFS, "index.html"); err != nil {
		logger.Warn("frontend assets unavailable; serving API only", "web_dir", cfg.WebDir)
	} else {
		webHandler = httpserver.NewSPAHandler(webFS)
	}

	router := httpserver.NewRouter(logger, pool, apiHandler, webHandler)
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
