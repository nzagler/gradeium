package tv

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/nzagler/gradeium/backend/internal/integrations/tmdb"
	"github.com/nzagler/gradeium/backend/internal/integrations/tvdb"
)

func (store *PostgresStore) Rematch(ctx context.Context, userID, id string, show tvdb.Show, mapping *tmdb.VerifiedTV) (Detail, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return Detail{}, err
	}
	defer tx.Rollback(ctx)

	var tracked bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_tv_shows WHERE user_id=$1 AND tv_show_id=$2)`, userID, id).Scan(&tracked); err != nil {
		return Detail{}, err
	}
	if !tracked {
		return Detail{}, ErrNotFound
	}

	var collision string
	err = tx.QueryRow(ctx, `SELECT entity_id::text FROM tv_shows WHERE tvdb_id=$1 AND entity_id<>$2 LIMIT 1`, show.ProviderID, id).Scan(&collision)
	if err == nil {
		return Detail{}, ErrProviderAlreadyExists
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, err
	}

	if _, err = tx.Exec(ctx, `UPDATE tv_shows SET tvdb_id=$1,verified_tmdb_id=NULL,community_rating=NULL,community_rating_count=NULL,tmdb_mapping_verified_at=NULL,community_rating_refreshed_at=NULL,updated_at=now() WHERE entity_id=$2`, show.ProviderID, id); err != nil {
		return Detail{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE user_tv_shows SET selected_poster_provider_image_id=NULL,selected_backdrop_provider_image_id=NULL,selected_logo_provider_image_id=NULL,updated_at=now() WHERE user_id=$1 AND tv_show_id=$2`, userID, id); err != nil {
		return Detail{}, err
	}
	if err = store.persist(ctx, tx, id, show, mapping); err != nil {
		return Detail{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Detail{}, err
	}
	return store.Detail(ctx, userID, id)
}
