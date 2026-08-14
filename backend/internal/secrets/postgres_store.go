package secrets

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/nzagler/gradeium/backend/internal/database/sqlc"
)

// PostgresStore persists encrypted secret envelopes.
type PostgresStore struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool, queries: db.New(pool)}
}

func (store *PostgresStore) Get(ctx context.Context, key string) (Record, error) {
	row, err := store.queries.GetSecretSetting(ctx, key)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrSecretNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("read encrypted secret: %w", err)
	}
	return Record{
		Key: row.Key,
		Envelope: Envelope{
			AlgorithmVersion: row.AlgorithmVersion,
			Nonce:            row.Nonce,
			Ciphertext:       row.Ciphertext,
		},
	}, nil
}

func (store *PostgresStore) Upsert(ctx context.Context, record Record) error {
	err := store.queries.UpsertSecretSetting(ctx, db.UpsertSecretSettingParams{
		Key:              record.Key,
		AlgorithmVersion: record.AlgorithmVersion,
		Nonce:            record.Nonce,
		Ciphertext:       record.Ciphertext,
	})
	if err != nil {
		return fmt.Errorf("persist encrypted secret: %w", err)
	}
	return nil
}

func (store *PostgresStore) Delete(ctx context.Context, key string) (bool, error) {
	rowsAffected, err := store.queries.DeleteSecretSetting(ctx, key)
	if err != nil {
		return false, fmt.Errorf("delete encrypted secret: %w", err)
	}
	return rowsAffected == 1, nil
}

func (store *PostgresStore) List(ctx context.Context) ([]Record, error) {
	rows, err := store.queries.ListSecretSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("query encrypted secrets: %w", err)
	}
	records := make([]Record, 0, len(rows))
	for _, row := range rows {
		records = append(records, Record{
			Key: row.Key,
			Envelope: Envelope{
				AlgorithmVersion: row.AlgorithmVersion,
				Nonce:            row.Nonce,
				Ciphertext:       row.Ciphertext,
			},
		})
	}
	return records, nil
}

func (store *PostgresStore) State(ctx context.Context) (KeyState, error) {
	secretCount, err := store.queries.CountSecretSettings(ctx)
	if err != nil {
		return KeyState{}, fmt.Errorf("query encryption key state: %w", err)
	}
	state := KeyState{SecretCount: secretCount}
	fingerprint, err := store.queries.GetEncryptionKeyFingerprint(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return state, nil
	}
	if err != nil {
		return KeyState{}, fmt.Errorf("query encryption key fingerprint: %w", err)
	}
	if len(fingerprint) != sha256.Size {
		return KeyState{}, errors.New("stored key fingerprint is invalid")
	}
	state.Registered = true
	copy(state.Fingerprint[:], fingerprint)
	return state, nil
}

func (store *PostgresStore) RegisterFingerprint(ctx context.Context, fingerprint [sha256.Size]byte) error {
	transaction, err := store.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin key fingerprint transaction: %w", err)
	}
	defer transaction.Rollback(ctx)

	queries := db.New(transaction)
	err = queries.RegisterEncryptionKeyFingerprint(ctx, fingerprint[:])
	if err != nil {
		return fmt.Errorf("insert key fingerprint: %w", err)
	}

	persisted, err := queries.GetEncryptionKeyFingerprintForUpdate(ctx)
	if err != nil {
		return fmt.Errorf("verify key fingerprint: %w", err)
	}
	if len(persisted) != sha256.Size || subtle.ConstantTimeCompare(persisted, fingerprint[:]) != 1 {
		return ErrMasterKeyMismatch
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit key fingerprint: %w", err)
	}
	return nil
}
