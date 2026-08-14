package setup

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/nzagler/gradeium/backend/internal/database/sqlc"
)

// PostgresStore persists the explicit singleton setup state.
type PostgresStore struct {
	queries *db.Queries
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{queries: db.New(pool)}
}

func (store *PostgresStore) CompleteStatus(ctx context.Context) (bool, error) {
	complete, err := store.queries.GetSetupComplete(ctx)
	if err != nil {
		return false, fmt.Errorf("read setup state: %w", err)
	}
	return complete, nil
}

func (store *PostgresStore) Complete(ctx context.Context) (bool, error) {
	rowsAffected, err := store.queries.CompleteSetup(ctx)
	if err != nil {
		return false, fmt.Errorf("complete initial setup: %w", err)
	}
	return rowsAffected == 1, nil
}
