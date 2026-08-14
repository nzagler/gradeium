package auth

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	db "github.com/nzagler/gradeium/backend/internal/database/sqlc"
	"github.com/nzagler/gradeium/backend/internal/secrets"
	"github.com/nzagler/gradeium/backend/internal/settings"
)

type PostgresStore struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	now     func() time.Time
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool, queries: db.New(pool), now: time.Now}
}

func (store *PostgresStore) State(ctx context.Context) (State, error) {
	row, err := store.queries.GetAuthenticationState(ctx)
	if err != nil {
		return State{}, fmt.Errorf("read authentication state: %w", err)
	}
	return mapAuthenticationState(row), nil
}

func (store *PostgresStore) LoadDraft(ctx context.Context) (Draft, error) {
	transaction, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return Draft{}, fmt.Errorf("begin authentication configuration read: %w", err)
	}
	defer transaction.Rollback(ctx)
	queries := db.New(transaction)
	stateRow, err := queries.GetAuthenticationState(ctx)
	if err != nil {
		return Draft{}, fmt.Errorf("read authentication state: %w", err)
	}
	draft := Draft{
		Revision:    stateRow.ConfigurationRevision,
		Validated:   stateRow.ValidatedRevision.Valid && stateRow.ValidatedRevision.Int64 == stateRow.ConfigurationRevision,
		ValidatedAt: timestampPointer(stateRow.ValidatedAt),
	}
	for _, target := range []struct {
		key         string
		destination *string
	}{
		{settings.AuthenticationIssuerURLKey, &draft.IssuerURL},
		{settings.AuthenticationClientIDKey, &draft.ClientID},
		{settings.AuthenticationPublicURLKey, &draft.PublicURL},
	} {
		value, queryErr := queries.GetAppSetting(ctx, target.key)
		if errors.Is(queryErr, pgx.ErrNoRows) {
			continue
		}
		if queryErr != nil {
			return Draft{}, fmt.Errorf("read authentication setting: %w", queryErr)
		}
		if err := json.Unmarshal(value, target.destination); err != nil {
			return Draft{}, errors.New("stored authentication setting is invalid")
		}
	}
	secretRow, err := queries.GetSecretSetting(ctx, settings.AuthenticationClientSecretKey)
	if err == nil {
		record := secrets.Record{
			Key: settings.AuthenticationClientSecretKey,
			Envelope: secrets.Envelope{
				AlgorithmVersion: secretRow.AlgorithmVersion,
				Nonce:            append([]byte(nil), secretRow.Nonce...),
				Ciphertext:       append([]byte(nil), secretRow.Ciphertext...),
			},
		}
		draft.ClientSecretConfigured = true
		draft.ClientSecretRecord = &record
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Draft{}, fmt.Errorf("read encrypted authentication secret: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return Draft{}, fmt.Errorf("finish authentication configuration read: %w", err)
	}
	return draft, nil
}

func (store *PostgresStore) SaveDraft(
	ctx context.Context,
	expectedRevision int64,
	configuration Configuration,
	mutation SecretMutation,
	record *secrets.Record,
	validated bool,
) (int64, error) {
	transaction, err := store.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin authentication configuration update: %w", err)
	}
	defer transaction.Rollback(ctx)
	queries := db.New(transaction)
	state, err := queries.GetAuthenticationStateForUpdate(ctx)
	if err != nil {
		return 0, fmt.Errorf("lock authentication state: %w", err)
	}
	if state.ConfigurationRevision != expectedRevision {
		return 0, ErrConfigurationStale
	}
	for key, value := range map[string]string{
		settings.AuthenticationIssuerURLKey: configuration.IssuerURL,
		settings.AuthenticationClientIDKey:  configuration.ClientID,
		settings.AuthenticationPublicURLKey: configuration.PublicURL,
	} {
		encoded, _ := json.Marshal(value)
		if err := queries.UpsertAppSetting(ctx, db.UpsertAppSettingParams{Key: key, Value: encoded}); err != nil {
			return 0, fmt.Errorf("persist authentication setting: %w", err)
		}
	}
	if err := applySecretMutation(ctx, queries, settings.AuthenticationClientSecretKey, mutation, record); err != nil {
		return 0, err
	}
	revision, err := queries.IncrementAuthenticationConfigurationRevision(ctx)
	if err != nil {
		return 0, fmt.Errorf("advance authentication configuration revision: %w", err)
	}
	if validated {
		rows, err := queries.MarkAuthenticationConfigurationValidated(ctx, nullableInt8(revision))
		if err != nil || rows != 1 {
			return 0, fmt.Errorf("mark authentication configuration validated: %w", err)
		}
	}
	if err := transaction.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit authentication configuration update: %w", err)
	}
	return revision, nil
}

func applySecretMutation(ctx context.Context, queries *db.Queries, key string, mutation SecretMutation, record *secrets.Record) error {
	switch mutation {
	case KeepSecret:
		return nil
	case RemoveSecret:
		if _, err := queries.DeleteSecretSetting(ctx, key); err != nil {
			return fmt.Errorf("remove encrypted authentication secret: %w", err)
		}
		return nil
	case ReplaceSecret:
		if record == nil || record.Key != key {
			return errors.New("encrypted authentication secret is missing")
		}
		if err := queries.UpsertSecretSetting(ctx, db.UpsertSecretSettingParams{
			Key:              key,
			AlgorithmVersion: record.AlgorithmVersion,
			Nonce:            record.Nonce,
			Ciphertext:       record.Ciphertext,
		}); err != nil {
			return fmt.Errorf("persist encrypted authentication secret: %w", err)
		}
		return nil
	default:
		return errors.New("unknown authentication secret mutation")
	}
}

func (store *PostgresStore) MarkValidated(ctx context.Context, revision int64) (bool, error) {
	rows, err := store.queries.MarkAuthenticationConfigurationValidated(ctx, nullableInt8(revision))
	if err != nil {
		return false, fmt.Errorf("mark authentication configuration validated: %w", err)
	}
	return rows == 1, nil
}

func (store *PostgresStore) Activate(ctx context.Context, revision int64, configuration Configuration, activeSecret *secrets.Record) (bool, error) {
	return store.publishActive(ctx, false, revision, configuration, activeSecret)
}

func (store *PostgresStore) Apply(ctx context.Context, revision int64, configuration Configuration, activeSecret *secrets.Record) (bool, error) {
	return store.publishActive(ctx, true, revision, configuration, activeSecret)
}

func (store *PostgresStore) publishActive(ctx context.Context, apply bool, revision int64, configuration Configuration, activeSecret *secrets.Record) (bool, error) {
	transaction, err := store.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin authentication activation: %w", err)
	}
	defer transaction.Rollback(ctx)
	queries := db.New(transaction)
	state, err := queries.GetAuthenticationStateForUpdate(ctx)
	if err != nil {
		return false, fmt.Errorf("lock authentication state: %w", err)
	}
	if (!apply && state.Activated) || (apply && !state.Activated) {
		return false, nil
	}
	if state.ConfigurationRevision != revision || !state.ValidatedRevision.Valid || state.ValidatedRevision.Int64 != revision {
		return false, ErrNotValidated
	}
	if activeSecret == nil {
		if _, err := queries.DeleteSecretSetting(ctx, settings.AuthenticationActiveClientSecretKey); err != nil {
			return false, fmt.Errorf("remove active encrypted authentication secret: %w", err)
		}
	} else {
		if activeSecret.Key != settings.AuthenticationActiveClientSecretKey {
			return false, errors.New("active encrypted authentication secret has the wrong key")
		}
		if err := queries.UpsertSecretSetting(ctx, db.UpsertSecretSettingParams{
			Key:              activeSecret.Key,
			AlgorithmVersion: activeSecret.AlgorithmVersion,
			Nonce:            activeSecret.Nonce,
			Ciphertext:       activeSecret.Ciphertext,
		}); err != nil {
			return false, fmt.Errorf("persist active encrypted authentication secret: %w", err)
		}
	}
	parameters := db.ActivateAuthenticationParams{
		ActiveRevision:  nullableInt8(revision),
		ActiveIssuerUrl: nullableText(configuration.IssuerURL),
		ActiveClientID:  nullableText(configuration.ClientID),
		ActivePublicUrl: nullableText(configuration.PublicURL),
	}
	var rows int64
	if apply {
		rows, err = queries.ApplyActiveAuthenticationConfiguration(ctx, db.ApplyActiveAuthenticationConfigurationParams(parameters))
	} else {
		rows, err = queries.ActivateAuthentication(ctx, parameters)
	}
	if err != nil {
		return false, fmt.Errorf("publish active authentication configuration: %w", err)
	}
	if rows != 1 {
		return false, nil
	}
	if err := transaction.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit authentication activation: %w", err)
	}
	return true, nil
}

func (store *PostgresStore) SaveFlow(ctx context.Context, record FlowRecord) error {
	err := store.queries.InsertOIDCLoginFlow(ctx, db.InsertOIDCLoginFlowParams{
		StateHash:        record.StateHash[:],
		AlgorithmVersion: record.Envelope.AlgorithmVersion,
		Nonce:            record.Envelope.Nonce,
		Ciphertext:       record.Envelope.Ciphertext,
		ExpiresAt:        nullableTime(record.ExpiresAt),
	})
	if err != nil {
		return fmt.Errorf("persist OIDC login flow: %w", err)
	}
	return nil
}

func (store *PostgresStore) ConsumeFlow(ctx context.Context, stateHash [32]byte) (FlowRecord, error) {
	row, err := store.queries.ConsumeOIDCLoginFlow(ctx, stateHash[:])
	if errors.Is(err, pgx.ErrNoRows) {
		return FlowRecord{}, ErrFlowNotFound
	}
	if err != nil {
		return FlowRecord{}, fmt.Errorf("consume OIDC login flow: %w", err)
	}
	return FlowRecord{
		StateHash: stateHash,
		Envelope: secrets.Envelope{
			AlgorithmVersion: row.AlgorithmVersion,
			Nonce:            row.Nonce,
			Ciphertext:       row.Ciphertext,
		},
		ExpiresAt: row.ExpiresAt.Time,
	}, nil
}

func (store *PostgresStore) CompleteLogin(
	ctx context.Context,
	identity Identity,
	sessionHash [32]byte,
	expiresAt time.Time,
	previousSessionHash *[32]byte,
) (User, error) {
	transaction, err := store.pool.Begin(ctx)
	if err != nil {
		return User{}, fmt.Errorf("begin OIDC identity binding: %w", err)
	}
	defer transaction.Rollback(ctx)
	queries := db.New(transaction)
	state, err := queries.GetAuthenticationStateForUpdate(ctx)
	if err != nil {
		return User{}, fmt.Errorf("lock authentication state: %w", err)
	}
	if !state.Activated {
		return User{}, ErrNotActivated
	}
	identityParameters := db.GetUserByOIDCIdentityParams{
		OidcIssuer:  nullableText(identity.Issuer),
		OidcSubject: nullableText(identity.Subject),
	}
	userRow, err := queries.GetUserByOIDCIdentity(ctx, identityParameters)
	if errors.Is(err, pgx.ErrNoRows) {
		adminExists, queryErr := queries.AnyAdminExists(ctx)
		if queryErr != nil {
			return User{}, fmt.Errorf("check initial administrator: %w", queryErr)
		}
		created, queryErr := queries.CreateOIDCUser(ctx, db.CreateOIDCUserParams{
			OidcIssuer:  identityParameters.OidcIssuer,
			OidcSubject: identityParameters.OidcSubject,
			DisplayName: nullableStringPointer(identity.DisplayName),
			Email:       nullableStringPointer(identity.Email),
			IsAdmin:     !adminExists,
		})
		if queryErr != nil {
			return User{}, fmt.Errorf("create OIDC user: %w", queryErr)
		}
		userRow = db.GetUserByOIDCIdentityRow(created)
	} else if err != nil {
		return User{}, fmt.Errorf("read OIDC user: %w", err)
	} else {
		adminExists, queryErr := queries.AnyAdminExists(ctx)
		if queryErr != nil {
			return User{}, fmt.Errorf("check initial administrator: %w", queryErr)
		}
		if !adminExists && !userRow.IsAdmin {
			promoted, queryErr := queries.PromoteUserToAdmin(ctx, userRow.ID)
			if queryErr != nil {
				return User{}, fmt.Errorf("bind initial administrator: %w", queryErr)
			}
			userRow = db.GetUserByOIDCIdentityRow(promoted)
		}
		updated, queryErr := queries.UpdateOIDCUser(ctx, db.UpdateOIDCUserParams{
			ID:          userRow.ID,
			DisplayName: nullableStringPointer(identity.DisplayName),
			Email:       nullableStringPointer(identity.Email),
		})
		if queryErr != nil {
			return User{}, fmt.Errorf("update OIDC user metadata: %w", queryErr)
		}
		userRow = db.GetUserByOIDCIdentityRow(updated)
	}
	if previousSessionHash != nil {
		if _, err := queries.RevokeSessionByTokenHash(ctx, previousSessionHash[:]); err != nil {
			return User{}, fmt.Errorf("rotate previous session: %w", err)
		}
	}
	if _, err := queries.CreateSession(ctx, db.CreateSessionParams{
		UserID:    userRow.ID,
		TokenHash: sessionHash[:],
		ExpiresAt: nullableTime(expiresAt),
	}); err != nil {
		return User{}, fmt.Errorf("create Gradeium session: %w", err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit OIDC identity binding: %w", err)
	}
	return mapUserRow(userRow), nil
}

func (store *PostgresStore) Session(ctx context.Context, hash [32]byte) (Session, error) {
	row, err := store.queries.GetSessionByTokenHash(ctx, hash[:])
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("read Gradeium session: %w", err)
	}
	if row.RevokedAt.Valid || !row.ExpiresAt.Valid || !row.ExpiresAt.Time.After(store.now()) {
		return Session{}, ErrSessionNotFound
	}
	return Session{
		ID: uuidString(row.ID),
		User: User{
			ID:          uuidString(row.UserID),
			Issuer:      row.OidcIssuer.String,
			Subject:     row.OidcSubject.String,
			DisplayName: textPointer(row.DisplayName),
			Email:       textPointer(row.Email),
			IsAdmin:     row.IsAdmin,
		},
		ExpiresAt: row.ExpiresAt.Time,
		RevokedAt: timestampPointer(row.RevokedAt),
		PublicURL: row.ActivePublicUrl.String,
	}, nil
}

func (store *PostgresStore) RevokeSession(ctx context.Context, hash [32]byte) (bool, error) {
	rows, err := store.queries.RevokeSessionByTokenHash(ctx, hash[:])
	if err != nil {
		return false, fmt.Errorf("revoke Gradeium session: %w", err)
	}
	return rows == 1, nil
}

func (store *PostgresStore) Cleanup(ctx context.Context, limit int32) error {
	if _, err := store.queries.DeleteExpiredOIDCLoginFlows(ctx, limit); err != nil {
		return fmt.Errorf("clean expired OIDC login flows: %w", err)
	}
	if _, err := store.queries.DeleteExpiredSessions(ctx, limit); err != nil {
		return fmt.Errorf("clean expired Gradeium sessions: %w", err)
	}
	return nil
}

func mapAuthenticationState(row db.GetAuthenticationStateRow) State {
	state := State{
		ConfigurationRevision: row.ConfigurationRevision,
		ValidatedRevision:     int8Pointer(row.ValidatedRevision),
		ValidatedAt:           timestampPointer(row.ValidatedAt),
		Activated:             row.Activated,
		ActivatedAt:           timestampPointer(row.ActivatedAt),
		ActiveRevision:        int8Pointer(row.ActiveRevision),
	}
	if row.Activated {
		state.Active = &Configuration{
			IssuerURL: row.ActiveIssuerUrl.String,
			ClientID:  row.ActiveClientID.String,
			PublicURL: row.ActivePublicUrl.String,
		}
	}
	return state
}

func mapUserRow(row db.GetUserByOIDCIdentityRow) User {
	return User{
		ID:          uuidString(row.ID),
		Issuer:      row.OidcIssuer.String,
		Subject:     row.OidcSubject.String,
		DisplayName: textPointer(row.DisplayName),
		Email:       textPointer(row.Email),
		IsAdmin:     row.IsAdmin,
	}
}

func uuidString(value pgtype.UUID) string {
	if !value.Valid {
		return ""
	}
	encoded := make([]byte, 36)
	hex.Encode(encoded[0:8], value.Bytes[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], value.Bytes[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], value.Bytes[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], value.Bytes[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], value.Bytes[10:16])
	return string(encoded)
}

func nullableText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func nullableStringPointer(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func nullableInt8(value int64) pgtype.Int8 {
	return pgtype.Int8{Int64: value, Valid: true}
}

func nullableTime(value time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: value, Valid: true}
}

func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}

func timestampPointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	copy := value.Time
	return &copy
}

func int8Pointer(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	copy := value.Int64
	return &copy
}
