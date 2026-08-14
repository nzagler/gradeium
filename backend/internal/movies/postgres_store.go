package movies

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nzagler/gradeium/backend/internal/integrations/tmdb"
	"github.com/nzagler/gradeium/backend/internal/media"
	"time"
)

type PostgresStore struct{ pool *pgxpool.Pool }

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore { return &PostgresStore{pool: pool} }
func (store *PostgresStore) Tracked(ctx context.Context, userID string, ids []int64) (map[int64]Tracked, error) {
	result := map[int64]Tracked{}
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := store.pool.Query(ctx, `SELECT m.tmdb_id,m.entity_id::text,um.status::text FROM movies m JOIN user_movies um ON um.movie_id=m.entity_id WHERE um.user_id=$1 AND m.tmdb_id=ANY($2)`, userID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var item Tracked
		if err := rows.Scan(&id, &item.ID, &item.Status); err != nil {
			return nil, err
		}
		result[id] = item
	}
	return result, rows.Err()
}
func (store *PostgresStore) Add(ctx context.Context, userID string, movie tmdb.Movie, status media.Status) (Detail, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return Detail{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, fmt.Sprintf("movie:%d", movie.ProviderID)); err != nil {
		return Detail{}, err
	}
	var entityID string
	err = tx.QueryRow(ctx, `SELECT entity_id::text FROM movies WHERE tmdb_id=$1`, movie.ProviderID).Scan(&entityID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = tx.QueryRow(ctx, `INSERT INTO entities(type) VALUES('movie') RETURNING id::text`).Scan(&entityID); err != nil {
			return Detail{}, err
		}
	} else if err != nil {
		return Detail{}, err
	}
	if err = store.persist(ctx, tx, entityID, movie); err != nil {
		return Detail{}, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO user_movies(user_id,movie_id,status)VALUES($1,$2,$3)ON CONFLICT DO NOTHING`, userID, entityID, status)
	if err != nil {
		return Detail{}, err
	}
	if tag.RowsAffected() != 1 {
		return Detail{}, ErrAlreadyTracked
	}
	if err = tx.Commit(ctx); err != nil {
		return Detail{}, err
	}
	return store.Detail(ctx, userID, entityID)
}
func (store *PostgresStore) Refresh(ctx context.Context, userID string, movie tmdb.Movie) (Detail, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return Detail{}, err
	}
	defer tx.Rollback(ctx)
	var id string
	if err = tx.QueryRow(ctx, `SELECT entity_id::text FROM movies WHERE tmdb_id=$1 FOR UPDATE`, movie.ProviderID).Scan(&id); err != nil {
		return Detail{}, mapNotFound(err)
	}
	if err = store.persist(ctx, tx, id, movie); err != nil {
		return Detail{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Detail{}, err
	}
	return store.Detail(ctx, userID, id)
}
func (store *PostgresStore) persist(ctx context.Context, tx pgx.Tx, id string, movie tmdb.Movie) error {
	cast, _ := json.Marshal(nonNil(movie.Cast))
	crew, _ := json.Marshal(nonNil(movie.Crew))
	_, err := tx.Exec(ctx, `INSERT INTO movies(entity_id,tmdb_id,english_title,original_title,overview,release_date,release_year,runtime_minutes,director,genres,production_companies,cast_members,key_crew,trailer_key,imdb_id,homepage,collection_tmdb_id,collection_name,community_rating,community_rating_count,metadata_refreshed_at,updated_at)VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,NULLIF($9,''),$10,$11,$12,$13,NULLIF($14,''),NULLIF($15,''),NULLIF($16,''),$17,NULLIF($18,''),$19,$20,now(),now())ON CONFLICT(entity_id)DO UPDATE SET english_title=EXCLUDED.english_title,original_title=EXCLUDED.original_title,overview=EXCLUDED.overview,release_date=EXCLUDED.release_date,release_year=EXCLUDED.release_year,runtime_minutes=EXCLUDED.runtime_minutes,director=EXCLUDED.director,genres=EXCLUDED.genres,production_companies=EXCLUDED.production_companies,cast_members=EXCLUDED.cast_members,key_crew=EXCLUDED.key_crew,trailer_key=EXCLUDED.trailer_key,imdb_id=EXCLUDED.imdb_id,homepage=EXCLUDED.homepage,collection_tmdb_id=EXCLUDED.collection_tmdb_id,collection_name=EXCLUDED.collection_name,community_rating=EXCLUDED.community_rating,community_rating_count=EXCLUDED.community_rating_count,metadata_refreshed_at=now(),updated_at=now()`, id, movie.ProviderID, movie.Title, movie.OriginalTitle, movie.Overview, dateValue(movie.ReleaseDate), movie.Year, movie.RuntimeMinutes, movie.Director, movie.Genres, movie.ProductionCompanies, cast, crew, movie.TrailerKey, movie.IMDbID, movie.Homepage, movie.CollectionID, movie.CollectionName, movie.CommunityRating, movie.CommunityRatingCount)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM movie_collection_members WHERE movie_id=$1`, id); err != nil {
		return err
	}
	for _, item := range movie.Collection {
		if _, err = tx.Exec(ctx, `INSERT INTO movie_collection_members(movie_id,tmdb_id,title,release_date,poster_url)VALUES($1,$2,$3,$4,NULLIF($5,''))`, id, item.ProviderID, item.Title, dateValue(item.ReleaseDate), item.PosterURL); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE media_artwork SET available=false,preferred=false,updated_at=now() WHERE entity_id=$1 AND provider='tmdb'`, id); err != nil {
		return err
	}
	for index, item := range movie.Artworks {
		if _, err = tx.Exec(ctx, `INSERT INTO media_artwork(entity_id,provider,provider_image_id,kind,language,image_url,thumbnail_url,width,height,preferred,available,sort_order)VALUES($1,'tmdb',$2,$3,NULLIF($4,''),$5,$6,NULLIF($7,0),NULLIF($8,0),$9,true,$10)ON CONFLICT(entity_id,provider_image_id)DO UPDATE SET kind=EXCLUDED.kind,language=EXCLUDED.language,image_url=EXCLUDED.image_url,thumbnail_url=EXCLUDED.thumbnail_url,width=EXCLUDED.width,height=EXCLUDED.height,preferred=EXCLUDED.preferred,available=true,sort_order=EXCLUDED.sort_order,updated_at=now()`, id, item.ProviderImageID, item.Kind, item.Language, item.ImageURL, item.ThumbnailURL, item.Width, item.Height, item.Preferred, index); err != nil {
			return err
		}
	}
	return nil
}

func nonNil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}

const movieSelect = `SELECT m.entity_id::text,m.tmdb_id,m.english_title,COALESCE(m.original_title,''),COALESCE(m.overview,''),m.release_date,m.release_year,m.runtime_minutes,COALESCE(m.director,''),m.genres,m.production_companies,m.cast_members,m.key_crew,COALESCE(m.trailer_key,''),COALESCE(m.imdb_id,''),COALESCE(m.homepage,''),m.collection_tmdb_id,COALESCE(m.collection_name,''),m.community_rating,m.community_rating_count,m.metadata_refreshed_at,um.status::text,um.rating,um.rating_reason,um.date_added,um.selected_poster_provider_image_id,um.selected_backdrop_provider_image_id,um.selected_logo_provider_image_id,COALESCE(selected.image_url,preferred.image_url,'')FROM movies m JOIN user_movies um ON um.movie_id=m.entity_id LEFT JOIN LATERAL(SELECT image_url FROM media_artwork a WHERE a.entity_id=m.entity_id AND a.available AND a.kind='poster' AND a.provider_image_id=um.selected_poster_provider_image_id LIMIT 1)selected ON true LEFT JOIN LATERAL(SELECT image_url FROM media_artwork a WHERE a.entity_id=m.entity_id AND a.available AND a.kind='poster' ORDER BY a.preferred DESC,a.sort_order LIMIT 1)preferred ON true `

func (store *PostgresStore) List(ctx context.Context, userID string, backlog bool) ([]Item, error) {
	operator := "<>"
	if backlog {
		operator = "="
	}
	rows, err := store.pool.Query(ctx, movieSelect+`WHERE um.user_id=$1 AND um.status `+operator+` 'backlog' ORDER BY um.rating DESC NULLS LAST,lower(m.english_title)`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Item{}
	for rows.Next() {
		detail, err := scanMovie(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, detail.Item)
	}
	return result, rows.Err()
}
func (store *PostgresStore) Detail(ctx context.Context, userID, id string) (Detail, error) {
	detail, err := scanMovie(store.pool.QueryRow(ctx, movieSelect+`WHERE um.user_id=$1 AND m.entity_id=$2`, userID, id))
	if err != nil {
		return Detail{}, mapNotFound(err)
	}
	if err = store.load(ctx, userID, &detail); err != nil {
		return Detail{}, err
	}
	return detail, nil
}

type scanner interface{ Scan(...any) error }

func scanMovie(row scanner) (Detail, error) {
	var result Detail
	var release pgtype.Date
	var year, runtime, count pgtype.Int4
	var community, rating pgtype.Int2
	var reason, posterPin, backdropPin, logoPin pgtype.Text
	var cast, crew []byte
	var status string
	err := row.Scan(&result.ID, &result.ProviderID, &result.Title, &result.OriginalTitle, &result.Overview, &release, &year, &runtime, &result.Director, &result.Genres, &result.ProductionCompanies, &cast, &crew, &result.TrailerKey, &result.IMDbID, &result.Homepage, &result.CollectionID, &result.CollectionName, &community, &count, &result.MetadataRefreshedAt, &status, &rating, &reason, &result.State.DateAdded, &posterPin, &backdropPin, &logoPin, &result.ArtworkURL)
	if err != nil {
		return Detail{}, err
	}
	result.ReleaseDate = datePointer(release)
	result.Year = intPointer(year)
	result.RuntimeMinutes = intPointer(runtime)
	result.CommunityRating = int16Pointer(community)
	result.CommunityRatingCount = intPointer(count)
	result.State.PersonalState = media.PersonalState{Status: media.Status(status), Rating: int16Pointer(rating), RatingReason: textPointer(reason)}
	_ = json.Unmarshal(cast, &result.Cast)
	_ = json.Unmarshal(crew, &result.Crew)
	result.ArtworkPins = map[string]string{}
	for kind, value := range map[string]pgtype.Text{"poster": posterPin, "backdrop": backdropPin, "logo": logoPin} {
		if value.Valid {
			result.ArtworkPins[kind] = value.String
		}
	}
	return result, nil
}
func (store *PostgresStore) load(ctx context.Context, userID string, result *Detail) error {
	rows, err := store.pool.Query(ctx, `SELECT c.tmdb_id,c.title,c.release_date,COALESCE(c.poster_url,''),m.entity_id::text,um.status::text,um.rating FROM movie_collection_members c LEFT JOIN movies m ON m.tmdb_id=c.tmdb_id LEFT JOIN user_movies um ON um.movie_id=m.entity_id AND um.user_id=$2 WHERE c.movie_id=$1 ORDER BY c.release_date NULLS LAST,c.title`, result.ID, userID)
	if err != nil {
		return err
	}
	result.Collection = []CollectionMember{}
	for rows.Next() {
		var item CollectionMember
		var date pgtype.Date
		var localID, status pgtype.Text
		var rating pgtype.Int2
		if err := rows.Scan(&item.ProviderID, &item.Title, &date, &item.PosterURL, &localID, &status, &rating); err != nil {
			return err
		}
		item.ReleaseDate = datePointer(date)
		if localID.Valid {
			item.LocalID = localID.String
			item.LocalStatus = media.Status(status.String)
			item.LocalRating = int16Pointer(rating)
		}
		result.Collection = append(result.Collection, item)
	}
	rows.Close()
	return store.loadArtwork(ctx, result)
}
func (store *PostgresStore) loadArtwork(ctx context.Context, result *Detail) error {
	rows, err := store.pool.Query(ctx, `SELECT provider_image_id,kind,COALESCE(language,''),image_url,thumbnail_url,width,height,preferred,available FROM media_artwork WHERE entity_id=$1 ORDER BY kind,available DESC,preferred DESC,sort_order`, result.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	result.Artworks = []media.Artwork{}
	result.UnavailablePins = []string{}
	for rows.Next() {
		var item media.Artwork
		var width, height pgtype.Int4
		if err := rows.Scan(&item.ProviderImageID, &item.Kind, &item.Language, &item.ImageURL, &item.ThumbnailURL, &width, &height, &item.Preferred, &item.Available); err != nil {
			return err
		}
		if width.Valid {
			item.Width = width.Int32
		}
		if height.Valid {
			item.Height = height.Int32
		}
		result.Artworks = append(result.Artworks, item)
		if result.ArtworkPins[item.Kind] == item.ProviderImageID && !item.Available {
			result.UnavailablePins = append(result.UnavailablePins, item.Kind)
		}
	}
	return rows.Err()
}
func (store *PostgresStore) UpdateState(ctx context.Context, userID, id string, state media.PersonalState, confirm bool) (State, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return State{}, err
	}
	defer tx.Rollback(ctx)
	var rating pgtype.Int2
	var reason pgtype.Text
	if err = tx.QueryRow(ctx, `SELECT rating,rating_reason FROM user_movies WHERE user_id=$1 AND movie_id=$2 FOR UPDATE`, userID, id).Scan(&rating, &reason); err != nil {
		return State{}, mapNotFound(err)
	}
	if state.Status == media.StatusBacklog && (rating.Valid || reason.Valid) && !confirm {
		return State{}, ErrConfirmationRequired
	}
	if state.Status == media.StatusBacklog {
		state.Rating = nil
		state.RatingReason = nil
	}
	var added time.Time
	if err = tx.QueryRow(ctx, `UPDATE user_movies SET status=$3,rating=$4,rating_reason=$5,updated_at=now()WHERE user_id=$1 AND movie_id=$2 RETURNING date_added`, userID, id, state.Status, state.Rating, state.RatingReason).Scan(&added); err != nil {
		return State{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return State{}, err
	}
	return State{PersonalState: state, DateAdded: added}, nil
}
func (store *PostgresStore) SelectArtwork(ctx context.Context, userID, id, kind, imageID string) (Detail, error) {
	column := map[string]string{"poster": "selected_poster_provider_image_id", "backdrop": "selected_backdrop_provider_image_id", "logo": "selected_logo_provider_image_id"}[kind]
	if column == "" {
		return Detail{}, errors.New("unsupported artwork kind")
	}
	var value any = nil
	if imageID != "" {
		var exists bool
		if err := store.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_artwork WHERE entity_id=$1 AND provider_image_id=$2 AND kind=$3 AND available)`, id, imageID, kind).Scan(&exists); err != nil {
			return Detail{}, err
		}
		if !exists {
			return Detail{}, errors.New("provider artwork is not available")
		}
		value = imageID
	}
	tag, err := store.pool.Exec(ctx, `UPDATE user_movies SET `+column+`=$3,updated_at=now()WHERE user_id=$1 AND movie_id=$2`, userID, id, value)
	if err != nil {
		return Detail{}, err
	}
	if tag.RowsAffected() != 1 {
		return Detail{}, ErrNotFound
	}
	return store.Detail(ctx, userID, id)
}
func (store *PostgresStore) Remove(ctx context.Context, userID, id string) (bool, error) {
	tag, err := store.pool.Exec(ctx, `DELETE FROM user_movies WHERE user_id=$1 AND movie_id=$2`, userID, id)
	return tag.RowsAffected() == 1, err
}
func mapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
func dateValue(value *time.Time) any {
	if value == nil {
		return nil
	}
	return *value
}
func datePointer(value pgtype.Date) *time.Time {
	if !value.Valid {
		return nil
	}
	copy := value.Time
	return &copy
}
func intPointer(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	copy := value.Int32
	return &copy
}
func int16Pointer(value pgtype.Int2) *int16 {
	if !value.Valid {
		return nil
	}
	copy := value.Int16
	return &copy
}
func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}
