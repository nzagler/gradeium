package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nzagler/gradeium/backend/migrations"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// Migrate applies all committed migrations under a cross-process PostgreSQL lock.
func Migrate(ctx context.Context, databaseURL string, logger *slog.Logger) error {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("open migration connection: %w", err)
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	locker, err := lock.NewPostgresTableLocker(
		lock.WithTableLogger(logger),
		lock.WithTableLockTimeout(time.Second, 60),
		lock.WithTableUnlockTimeout(time.Second, 15),
	)
	if err != nil {
		_ = db.Close()
		return fmt.Errorf("configure migration lock: %w", err)
	}

	provider, err := goose.NewProvider(
		goose.DialectPostgres,
		db,
		migrations.Files,
		goose.WithLocker(locker),
		goose.WithSlog(logger),
	)
	if err != nil {
		_ = db.Close()
		return fmt.Errorf("configure migration provider: %w", err)
	}
	defer provider.Close()

	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	logger.Info("database migrations complete", "applied", len(results))
	return nil
}
