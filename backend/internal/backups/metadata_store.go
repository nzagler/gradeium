package backups

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type scheduleState struct {
	Settings
	ScheduleAnchor time.Time
}

func (store *PostgresStore) BackupSettings(ctx context.Context) (scheduleState, error) {
	var result scheduleState
	var attempt, successful pgtype.Timestamptz
	var lastError pgtype.Text
	err := store.pool.QueryRow(ctx, `
		SELECT enabled, interval_days, retention_count, schedule_anchor_at,
		       last_attempt_at, last_successful_automatic_at, last_error
		FROM backup_settings WHERE singleton
	`).Scan(
		&result.Enabled,
		&result.IntervalDays,
		&result.RetentionCount,
		&result.ScheduleAnchor,
		&attempt,
		&successful,
		&lastError,
	)
	if err != nil {
		return scheduleState{}, fmt.Errorf("read backup settings: %w", err)
	}
	result.LastAttemptAt = timePointer(attempt)
	result.LastSuccessfulAutomaticAt = timePointer(successful)
	result.LastError = textPointer(lastError)
	if result.Enabled {
		next := result.ScheduleAnchor.Add(time.Duration(result.IntervalDays) * 24 * time.Hour)
		result.NextDueAt = &next
	}
	return result, nil
}

func (store *PostgresStore) UpdateBackupSettings(ctx context.Context, enabled bool, intervalDays, retentionCount int32) (scheduleState, error) {
	_, err := store.pool.Exec(ctx, `
		UPDATE backup_settings
		SET enabled=$1, interval_days=$2, retention_count=$3,
		    schedule_anchor_at=now(), last_error=NULL, updated_at=now()
		WHERE singleton
	`, enabled, intervalDays, retentionCount)
	if err != nil {
		return scheduleState{}, fmt.Errorf("update backup settings: %w", err)
	}
	return store.BackupSettings(ctx)
}

func (store *PostgresStore) RecordAutomaticSuccess(ctx context.Context, at time.Time) error {
	_, err := store.pool.Exec(ctx, `
		UPDATE backup_settings
		SET last_attempt_at=$1, last_successful_automatic_at=$1,
		    schedule_anchor_at=$1, last_error=NULL, updated_at=now()
		WHERE singleton
	`, at)
	if err != nil {
		return fmt.Errorf("record automatic backup success: %w", err)
	}
	return nil
}

func (store *PostgresStore) RecordAutomaticFailure(ctx context.Context, at time.Time, message string) error {
	_, err := store.pool.Exec(ctx, `
		UPDATE backup_settings
		SET last_attempt_at=$1, last_error=$2, updated_at=now()
		WHERE singleton
	`, at, message)
	if err != nil {
		return fmt.Errorf("record automatic backup failure: %w", err)
	}
	return nil
}

func (store *PostgresStore) AddMetadata(ctx context.Context, value Metadata) (Metadata, error) {
	err := store.pool.QueryRow(ctx, `
		INSERT INTO backups(filename,kind,created_at,size_bytes,sha256,format_version,application_version,valid)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id::text
	`, value.Filename, value.Kind, value.CreatedAt, value.SizeBytes, value.SHA256, value.FormatVersion, value.ApplicationVersion, value.Valid).Scan(&value.ID)
	if err != nil {
		return Metadata{}, fmt.Errorf("record backup inventory: %w", err)
	}
	return value, nil
}

func (store *PostgresStore) ListMetadata(ctx context.Context) ([]Metadata, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT id::text, filename, kind, created_at, size_bytes, sha256,
		       format_version, application_version, valid
		FROM backups ORDER BY created_at DESC, id DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list backup inventory: %w", err)
	}
	defer rows.Close()
	result := []Metadata{}
	for rows.Next() {
		var item Metadata
		if err := rows.Scan(&item.ID, &item.Filename, &item.Kind, &item.CreatedAt, &item.SizeBytes, &item.SHA256, &item.FormatVersion, &item.ApplicationVersion, &item.Valid); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (store *PostgresStore) Metadata(ctx context.Context, id string) (Metadata, error) {
	var item Metadata
	err := store.pool.QueryRow(ctx, `
		SELECT id::text, filename, kind, created_at, size_bytes, sha256,
		       format_version, application_version, valid
		FROM backups WHERE id=$1
	`, id).Scan(&item.ID, &item.Filename, &item.Kind, &item.CreatedAt, &item.SizeBytes, &item.SHA256, &item.FormatVersion, &item.ApplicationVersion, &item.Valid)
	if errors.Is(err, pgx.ErrNoRows) {
		return Metadata{}, ErrNotFound
	}
	if err != nil {
		return Metadata{}, fmt.Errorf("read backup inventory: %w", err)
	}
	return item, nil
}

func (store *PostgresStore) DeleteMetadata(ctx context.Context, id string) error {
	tag, err := store.pool.Exec(ctx, `DELETE FROM backups WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete backup inventory: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return nil
}

func (store *PostgresStore) WithSchedulerLock(ctx context.Context, operation func(context.Context) error) error {
	connection, err := store.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire backup scheduler connection: %w", err)
	}
	defer connection.Release()
	var locked bool
	if err := connection.QueryRow(ctx, `SELECT pg_try_advisory_lock(hashtextextended('gradeium-backup-scheduler', 0))`).Scan(&locked); err != nil {
		return fmt.Errorf("acquire backup scheduler lock: %w", err)
	}
	if !locked {
		return ErrBusy
	}
	defer func() {
		unlockContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = connection.Exec(unlockContext, `SELECT pg_advisory_unlock(hashtextextended('gradeium-backup-scheduler', 0))`)
	}()
	return operation(ctx)
}

func timePointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	copy := value.Time
	return &copy
}

func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}
