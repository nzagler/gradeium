package movies

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/nzagler/gradeium/backend/internal/integrations/tmdb"
)

func (store *PostgresStore) Rematch(ctx context.Context, userID, id string, movie tmdb.Movie) (Detail, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return Detail{}, err
	}
	defer tx.Rollback(ctx)

	var tracked bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_movies WHERE user_id=$1 AND movie_id=$2)`, userID, id).Scan(&tracked); err != nil {
		return Detail{}, err
	}
	if !tracked {
		return Detail{}, ErrNotFound
	}

	var collision string
	err = tx.QueryRow(ctx, `SELECT entity_id::text FROM movies WHERE tmdb_id=$1 AND entity_id<>$2 LIMIT 1`, movie.ProviderID, id).Scan(&collision)
	if err == nil {
		return Detail{}, ErrProviderAlreadyExists
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Detail{}, err
	}

	if _, err = tx.Exec(ctx, `UPDATE movies SET tmdb_id=$1,updated_at=now() WHERE entity_id=$2`, movie.ProviderID, id); err != nil {
		return Detail{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE user_movies SET selected_poster_provider_image_id=NULL,selected_backdrop_provider_image_id=NULL,selected_logo_provider_image_id=NULL,updated_at=now() WHERE user_id=$1 AND movie_id=$2`, userID, id); err != nil {
		return Detail{}, err
	}
	if err = store.persist(ctx, tx, id, movie); err != nil {
		return Detail{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Detail{}, err
	}
	return store.Detail(ctx, userID, id)
}
