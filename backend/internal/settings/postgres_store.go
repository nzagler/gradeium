package settings

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/nzagler/gradeium/backend/internal/database/sqlc"
)

// PostgresStore persists non-secret application settings as JSONB.
type PostgresStore struct {
	queries *db.Queries
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{queries: db.New(pool)}
}

func (store *PostgresStore) Values(ctx context.Context) (map[string]json.RawMessage, error) {
	rows, err := store.queries.ListAppSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("query application settings: %w", err)
	}

	values := make(map[string]json.RawMessage, len(rows))
	for _, row := range rows {
		values[row.Key] = append(json.RawMessage(nil), row.Value...)
	}
	return values, nil
}

func (store *PostgresStore) Upsert(ctx context.Context, key string, value json.RawMessage) error {
	err := store.queries.UpsertAppSetting(ctx, db.UpsertAppSettingParams{
		Key:   key,
		Value: []byte(value),
	})
	if err != nil {
		return fmt.Errorf("persist application setting: %w", err)
	}
	return nil
}
