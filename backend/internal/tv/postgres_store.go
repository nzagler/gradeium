package tv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nzagler/gradeium/backend/internal/integrations/tmdb"
	"github.com/nzagler/gradeium/backend/internal/integrations/tvdb"
	"github.com/nzagler/gradeium/backend/internal/media"
	"math"
	"time"
)

type PostgresStore struct{ pool *pgxpool.Pool }

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore { return &PostgresStore{pool: pool} }
func (store *PostgresStore) Tracked(ctx context.Context, userID string, ids []int64) (map[int64]Tracked, error) {
	result := map[int64]Tracked{}
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := store.pool.Query(ctx, `SELECT t.tvdb_id,t.entity_id::text,ut.status::text FROM tv_shows t JOIN user_tv_shows ut ON ut.tv_show_id=t.entity_id WHERE ut.user_id=$1 AND t.tvdb_id=ANY($2)`, userID, ids)
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
func (store *PostgresStore) Add(ctx context.Context, userID string, show tvdb.Show, mapping *tmdb.VerifiedTV, status media.Status) (Detail, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return Detail{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, fmt.Sprintf("tv:%d", show.ProviderID)); err != nil {
		return Detail{}, err
	}
	var id string
	err = tx.QueryRow(ctx, `SELECT entity_id::text FROM tv_shows WHERE tvdb_id=$1`, show.ProviderID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = tx.QueryRow(ctx, `INSERT INTO entities(type)VALUES('tv_show')RETURNING id::text`).Scan(&id); err != nil {
			return Detail{}, err
		}
	} else if err != nil {
		return Detail{}, err
	}
	if err = store.persist(ctx, tx, id, show, mapping); err != nil {
		return Detail{}, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO user_tv_shows(user_id,tv_show_id,status)VALUES($1,$2,$3)ON CONFLICT DO NOTHING`, userID, id, status)
	if err != nil {
		return Detail{}, err
	}
	if tag.RowsAffected() != 1 {
		return Detail{}, ErrAlreadyTracked
	}
	if err = tx.Commit(ctx); err != nil {
		return Detail{}, err
	}
	return store.Detail(ctx, userID, id)
}
func (store *PostgresStore) Refresh(ctx context.Context, userID string, show tvdb.Show, mapping *tmdb.VerifiedTV) (Detail, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return Detail{}, err
	}
	defer tx.Rollback(ctx)
	var id string
	if err = tx.QueryRow(ctx, `SELECT entity_id::text FROM tv_shows WHERE tvdb_id=$1 FOR UPDATE`, show.ProviderID).Scan(&id); err != nil {
		return Detail{}, mapNotFound(err)
	}
	if err = store.persist(ctx, tx, id, show, mapping); err != nil {
		return Detail{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Detail{}, err
	}
	return store.Detail(ctx, userID, id)
}
func (store *PostgresStore) persist(ctx context.Context, tx pgx.Tx, id string, show tvdb.Show, mapping *tmdb.VerifiedTV) error {
	cast, _ := json.Marshal(nonNil(show.Cast))
	people, _ := json.Marshal(nonNil(show.KeyPeople))
	var tmdbID any
	var rating *int16
	var count *int32
	var verifiedAt any
	if mapping != nil {
		tmdbID = mapping.TMDBID
		rating = mapping.CommunityRating
		count = mapping.CommunityRatingCount
		verifiedAt = time.Now().UTC()
	}
	_, err := tx.Exec(ctx, `INSERT INTO tv_shows(entity_id,tvdb_id,verified_tmdb_id,english_title,original_title,overview,first_aired,release_year,provider_status,network_name,genres,cast_members,key_people,community_rating,community_rating_count,tmdb_mapping_verified_at,metadata_refreshed_at,community_rating_refreshed_at,updated_at)VALUES($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),$7,$8,NULLIF($9,''),NULLIF($10,''),$11,$12,$13,$14,$15,$16,now(),CASE WHEN $3::bigint IS NULL THEN NULL ELSE now() END,now())ON CONFLICT(entity_id)DO UPDATE SET english_title=EXCLUDED.english_title,original_title=EXCLUDED.original_title,overview=EXCLUDED.overview,first_aired=EXCLUDED.first_aired,release_year=EXCLUDED.release_year,provider_status=EXCLUDED.provider_status,network_name=EXCLUDED.network_name,genres=EXCLUDED.genres,cast_members=EXCLUDED.cast_members,key_people=EXCLUDED.key_people,verified_tmdb_id=COALESCE(EXCLUDED.verified_tmdb_id,tv_shows.verified_tmdb_id),community_rating=CASE WHEN EXCLUDED.verified_tmdb_id IS NULL THEN tv_shows.community_rating ELSE EXCLUDED.community_rating END,community_rating_count=CASE WHEN EXCLUDED.verified_tmdb_id IS NULL THEN tv_shows.community_rating_count ELSE EXCLUDED.community_rating_count END,tmdb_mapping_verified_at=COALESCE(EXCLUDED.tmdb_mapping_verified_at,tv_shows.tmdb_mapping_verified_at),community_rating_refreshed_at=CASE WHEN EXCLUDED.verified_tmdb_id IS NULL THEN tv_shows.community_rating_refreshed_at ELSE now() END,metadata_refreshed_at=now(),updated_at=now()`, id, show.ProviderID, tmdbID, show.Title, show.OriginalTitle, show.Overview, dateValue(show.FirstAired), show.Year, show.ProviderStatus, show.Network, show.Genres, cast, people, rating, count, verifiedAt)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE tv_seasons SET available=false,updated_at=now()WHERE tv_show_id=$1`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE tv_episodes SET available=false,updated_at=now()WHERE tv_show_id=$1`, id); err != nil {
		return err
	}
	for _, season := range show.Seasons {
		var seasonID string
		err = tx.QueryRow(ctx, `INSERT INTO tv_seasons(tv_show_id,tvdb_season_id,season_number,name,is_specials,air_date,poster_url,available,updated_at)VALUES($1,$2,$3,NULLIF($4,''),$5,$6,NULLIF($7,''),true,now())ON CONFLICT(tv_show_id,season_number)DO UPDATE SET tvdb_season_id=EXCLUDED.tvdb_season_id,name=EXCLUDED.name,is_specials=EXCLUDED.is_specials,air_date=EXCLUDED.air_date,poster_url=EXCLUDED.poster_url,available=true,updated_at=now()RETURNING id::text`, id, season.ProviderID, season.Number, season.Name, season.Special, dateValue(season.AirDate), season.PosterURL).Scan(&seasonID)
		if err != nil {
			return err
		}
		for _, episode := range season.Episodes {
			_, err = tx.Exec(ctx, `INSERT INTO tv_episodes(tv_show_id,season_id,tvdb_episode_id,season_number,episode_number,sort_order,english_title,overview,air_date,runtime_minutes,still_url,is_special,available,updated_at)VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),$9,$10,NULLIF($11,''),$12,true,now())ON CONFLICT(tv_show_id,season_number,episode_number)DO UPDATE SET season_id=EXCLUDED.season_id,tvdb_episode_id=EXCLUDED.tvdb_episode_id,sort_order=EXCLUDED.sort_order,english_title=EXCLUDED.english_title,overview=EXCLUDED.overview,air_date=EXCLUDED.air_date,runtime_minutes=EXCLUDED.runtime_minutes,still_url=EXCLUDED.still_url,is_special=EXCLUDED.is_special,available=true,updated_at=now()`, id, seasonID, episode.ProviderID, episode.SeasonNumber, episode.EpisodeNumber, episode.SortOrder, episode.Title, episode.Overview, dateValue(episode.AirDate), episode.RuntimeMinutes, episode.StillURL, episode.Special)
			if err != nil {
				return err
			}
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE media_artwork SET available=false,preferred=false,updated_at=now()WHERE entity_id=$1 AND provider='tvdb'`, id); err != nil {
		return err
	}
	for index, item := range show.Artworks {
		if _, err = tx.Exec(ctx, `INSERT INTO media_artwork(entity_id,provider,provider_image_id,kind,language,image_url,thumbnail_url,width,height,preferred,available,sort_order)VALUES($1,'tvdb',$2,$3,NULLIF($4,''),$5,$6,NULLIF($7,0),NULLIF($8,0),$9,true,$10)ON CONFLICT(entity_id,provider_image_id)DO UPDATE SET kind=EXCLUDED.kind,language=EXCLUDED.language,image_url=EXCLUDED.image_url,thumbnail_url=EXCLUDED.thumbnail_url,width=EXCLUDED.width,height=EXCLUDED.height,preferred=EXCLUDED.preferred,available=true,sort_order=EXCLUDED.sort_order,updated_at=now()`, id, item.ProviderImageID, item.Kind, item.Language, item.ImageURL, item.ThumbnailURL, item.Width, item.Height, item.Preferred, index); err != nil {
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

const tvSelect = `SELECT t.entity_id::text,t.tvdb_id,t.verified_tmdb_id,t.english_title,COALESCE(t.original_title,''),COALESCE(t.overview,''),t.first_aired,t.release_year,COALESCE(t.provider_status,''),COALESCE(t.network_name,''),t.genres,t.cast_members,t.key_people,t.community_rating,t.community_rating_count,t.metadata_refreshed_at,ut.status::text,ut.rating,ut.rating_reason,ut.date_added,ut.selected_poster_provider_image_id,ut.selected_backdrop_provider_image_id,ut.selected_logo_provider_image_id,COALESCE(selected.image_url,preferred.image_url,''),(SELECT count(*) FROM tv_episodes e WHERE e.tv_show_id=t.entity_id AND e.available AND NOT e.is_special),(SELECT count(*) FROM user_episode_progress p JOIN tv_episodes e ON e.id=p.episode_id WHERE p.user_id=ut.user_id AND p.tv_show_id=t.entity_id AND e.available AND NOT e.is_special),(SELECT count(*) FROM tv_episodes e WHERE e.tv_show_id=t.entity_id AND e.available AND e.is_special),(SELECT count(*) FROM user_episode_progress p JOIN tv_episodes e ON e.id=p.episode_id WHERE p.user_id=ut.user_id AND p.tv_show_id=t.entity_id AND e.available AND e.is_special)FROM tv_shows t JOIN user_tv_shows ut ON ut.tv_show_id=t.entity_id LEFT JOIN LATERAL(SELECT image_url FROM media_artwork a WHERE a.entity_id=t.entity_id AND a.available AND a.kind='poster' AND a.provider_image_id=ut.selected_poster_provider_image_id LIMIT 1)selected ON true LEFT JOIN LATERAL(SELECT image_url FROM media_artwork a WHERE a.entity_id=t.entity_id AND a.available AND a.kind='poster' ORDER BY a.preferred DESC,a.sort_order LIMIT 1)preferred ON true `

func (store *PostgresStore) List(ctx context.Context, userID string, backlog bool) ([]Item, error) {
	op := "<>"
	if backlog {
		op = "="
	}
	rows, err := store.pool.Query(ctx, tvSelect+`WHERE ut.user_id=$1 AND ut.status `+op+` 'backlog' ORDER BY ut.rating DESC NULLS LAST,lower(t.english_title)`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Item{}
	for rows.Next() {
		detail, err := scanShow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, detail.Item)
	}
	return result, rows.Err()
}
func (store *PostgresStore) Detail(ctx context.Context, userID, id string) (Detail, error) {
	result, err := scanShow(store.pool.QueryRow(ctx, tvSelect+`WHERE ut.user_id=$1 AND t.entity_id=$2`, userID, id))
	if err != nil {
		return Detail{}, mapNotFound(err)
	}
	if err = store.load(ctx, userID, &result); err != nil {
		return Detail{}, err
	}
	return result, nil
}

type scanner interface{ Scan(...any) error }

func scanShow(row scanner) (Detail, error) {
	var r Detail
	var tmdbID pgtype.Int8
	var first pgtype.Date
	var year, count pgtype.Int4
	var community, rating pgtype.Int2
	var reason, posterPin, backdropPin, logoPin pgtype.Text
	var cast, people []byte
	var status string
	var total, watched, specialTotal, specialWatched int64
	err := row.Scan(&r.ID, &r.ProviderID, &tmdbID, &r.Title, &r.OriginalTitle, &r.Overview, &first, &year, &r.ProviderStatus, &r.Network, &r.Genres, &cast, &people, &community, &count, &r.MetadataRefreshedAt, &status, &rating, &reason, &r.State.DateAdded, &posterPin, &backdropPin, &logoPin, &r.ArtworkURL, &total, &watched, &specialTotal, &specialWatched)
	if err != nil {
		return Detail{}, err
	}
	if tmdbID.Valid {
		value := tmdbID.Int64
		r.VerifiedTMDBID = &value
	}
	r.FirstAired = datePointer(first)
	r.Year = intPointer(year)
	r.CommunityRating = int16Pointer(community)
	r.CommunityRatingCount = intPointer(count)
	r.State.PersonalState = media.PersonalState{Status: media.Status(status), Rating: int16Pointer(rating), RatingReason: textPointer(reason)}
	_ = json.Unmarshal(cast, &r.Cast)
	_ = json.Unmarshal(people, &r.KeyPeople)
	r.ArtworkPins = map[string]string{}
	for kind, value := range map[string]pgtype.Text{"poster": posterPin, "backdrop": backdropPin, "logo": logoPin} {
		if value.Valid {
			r.ArtworkPins[kind] = value.String
		}
	}
	r.Progress = Progress{Watched: int32(watched), Total: int32(total), SpecialsWatched: int32(specialWatched), SpecialsTotal: int32(specialTotal)}
	if total > 0 {
		r.Progress.Percent = int32(math.Round(float64(watched) * 100 / float64(total)))
	}
	return r, nil
}
func (store *PostgresStore) load(ctx context.Context, userID string, r *Detail) error {
	rows, err := store.pool.Query(ctx, `SELECT s.id::text,s.tvdb_season_id,s.season_number,COALESCE(s.name,''),s.is_specials,s.air_date,COALESCE(s.poster_url,''),(SELECT count(*) FROM tv_episodes e WHERE e.season_id=s.id AND e.available),(SELECT count(*) FROM user_episode_progress p JOIN tv_episodes e ON e.id=p.episode_id WHERE p.user_id=$2 AND e.season_id=s.id AND e.available)FROM tv_seasons s WHERE s.tv_show_id=$1 AND s.available ORDER BY s.season_number`, r.ID, userID)
	if err != nil {
		return err
	}
	r.Seasons = []Season{}
	for rows.Next() {
		var s Season
		var date pgtype.Date
		var total, watched int64
		if err := rows.Scan(&s.ID, &s.ProviderID, &s.Number, &s.Name, &s.Special, &date, &s.PosterURL, &total, &watched); err != nil {
			return err
		}
		s.AirDate = datePointer(date)
		s.Total = int32(total)
		s.Watched = int32(watched)
		s.Episodes = []Episode{}
		r.Seasons = append(r.Seasons, s)
	}
	rows.Close()
	for index := range r.Seasons {
		episodeRows, err := store.pool.Query(ctx, `SELECT e.id::text,e.tvdb_episode_id,e.season_number,e.episode_number,e.english_title,COALESCE(e.overview,''),e.air_date,e.runtime_minutes,COALESCE(e.still_url,''),e.is_special,(p.episode_id IS NOT NULL)FROM tv_episodes e LEFT JOIN user_episode_progress p ON p.episode_id=e.id AND p.user_id=$2 WHERE e.season_id=$1 AND e.available ORDER BY e.episode_number`, r.Seasons[index].ID, userID)
		if err != nil {
			return err
		}
		for episodeRows.Next() {
			var e Episode
			var date pgtype.Date
			var runtime pgtype.Int4
			if err := episodeRows.Scan(&e.ID, &e.ProviderID, &e.SeasonNumber, &e.EpisodeNumber, &e.Title, &e.Overview, &date, &runtime, &e.StillURL, &e.Special, &e.Watched); err != nil {
				return err
			}
			e.AirDate = datePointer(date)
			e.RuntimeMinutes = intPointer(runtime)
			r.Seasons[index].Episodes = append(r.Seasons[index].Episodes, e)
			if !e.Special && !e.Watched && r.Progress.NextEpisode == nil {
				copy := e
				r.Progress.NextEpisode = &copy
			}
		}
		episodeRows.Close()
	}
	return store.loadArtwork(ctx, r)
}
func (store *PostgresStore) loadArtwork(ctx context.Context, r *Detail) error {
	rows, err := store.pool.Query(ctx, `SELECT provider_image_id,kind,COALESCE(language,''),image_url,thumbnail_url,width,height,preferred,available FROM media_artwork WHERE entity_id=$1 ORDER BY kind,available DESC,preferred DESC,sort_order`, r.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	r.Artworks = []media.Artwork{}
	r.UnavailablePins = []string{}
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
		r.Artworks = append(r.Artworks, item)
		if r.ArtworkPins[item.Kind] == item.ProviderImageID && !item.Available {
			r.UnavailablePins = append(r.UnavailablePins, item.Kind)
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
	if err = tx.QueryRow(ctx, `SELECT rating,rating_reason FROM user_tv_shows WHERE user_id=$1 AND tv_show_id=$2 FOR UPDATE`, userID, id).Scan(&rating, &reason); err != nil {
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
	if err = tx.QueryRow(ctx, `UPDATE user_tv_shows SET status=$3,rating=$4,rating_reason=$5,updated_at=now()WHERE user_id=$1 AND tv_show_id=$2 RETURNING date_added`, userID, id, state.Status, state.Rating, state.RatingReason).Scan(&added); err != nil {
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
	tag, err := store.pool.Exec(ctx, `UPDATE user_tv_shows SET `+column+`=$3,updated_at=now()WHERE user_id=$1 AND tv_show_id=$2`, userID, id, value)
	if err != nil {
		return Detail{}, err
	}
	if tag.RowsAffected() != 1 {
		return Detail{}, ErrNotFound
	}
	return store.Detail(ctx, userID, id)
}
func (store *PostgresStore) SetEpisode(ctx context.Context, userID, id, episodeID string, watched bool) (Detail, error) {
	if watched {
		tag, err := store.pool.Exec(ctx, `INSERT INTO user_episode_progress(user_id,tv_show_id,episode_id)SELECT $1,$2,e.id FROM tv_episodes e JOIN user_tv_shows u ON u.tv_show_id=e.tv_show_id AND u.user_id=$1 WHERE e.id=$3 AND e.tv_show_id=$2 AND e.available ON CONFLICT DO NOTHING`, userID, id, episodeID)
		if err != nil {
			return Detail{}, err
		}
		if tag.RowsAffected() == 0 {
			var exists bool
			_ = store.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_episode_progress WHERE user_id=$1 AND episode_id=$2)`, userID, episodeID).Scan(&exists)
			if !exists {
				return Detail{}, ErrNotFound
			}
		}
	} else {
		_, err := store.pool.Exec(ctx, `DELETE FROM user_episode_progress WHERE user_id=$1 AND tv_show_id=$2 AND episode_id=$3`, userID, id, episodeID)
		if err != nil {
			return Detail{}, err
		}
	}
	return store.Detail(ctx, userID, id)
}
func (store *PostgresStore) SetSeason(ctx context.Context, userID, id string, season int32, watched bool) (Detail, error) {
	if watched {
		_, err := store.pool.Exec(ctx, `INSERT INTO user_episode_progress(user_id,tv_show_id,episode_id)SELECT $1,$2,e.id FROM tv_episodes e JOIN user_tv_shows u ON u.user_id=$1 AND u.tv_show_id=e.tv_show_id WHERE e.tv_show_id=$2 AND e.season_number=$3 AND e.available ON CONFLICT DO NOTHING`, userID, id, season)
		if err != nil {
			return Detail{}, err
		}
	} else {
		_, err := store.pool.Exec(ctx, `DELETE FROM user_episode_progress p USING tv_episodes e WHERE p.episode_id=e.id AND p.user_id=$1 AND p.tv_show_id=$2 AND e.season_number=$3`, userID, id, season)
		if err != nil {
			return Detail{}, err
		}
	}
	return store.Detail(ctx, userID, id)
}
func (store *PostgresStore) SetThrough(ctx context.Context, userID, id, episodeID string) (Detail, error) {
	tag, err := store.pool.Exec(ctx, `INSERT INTO user_episode_progress(user_id,tv_show_id,episode_id)SELECT $1,$2,e.id FROM tv_episodes e JOIN tv_episodes target ON target.id=$3 AND target.tv_show_id=e.tv_show_id JOIN user_tv_shows u ON u.user_id=$1 AND u.tv_show_id=e.tv_show_id WHERE e.tv_show_id=$2 AND e.available AND NOT e.is_special AND NOT target.is_special AND e.sort_order<=target.sort_order ON CONFLICT DO NOTHING`, userID, id, episodeID)
	if err != nil {
		return Detail{}, err
	}
	if tag.RowsAffected() == 0 {
		var valid bool
		_ = store.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tv_episodes WHERE id=$1 AND tv_show_id=$2 AND NOT is_special)`, episodeID, id).Scan(&valid)
		if !valid {
			return Detail{}, ErrNotFound
		}
	}
	return store.Detail(ctx, userID, id)
}
func (store *PostgresStore) SetAllRegular(ctx context.Context, userID, id string, watched bool) (Detail, error) {
	if watched {
		_, err := store.pool.Exec(ctx, `INSERT INTO user_episode_progress(user_id,tv_show_id,episode_id)SELECT $1,$2,e.id FROM tv_episodes e JOIN user_tv_shows u ON u.user_id=$1 AND u.tv_show_id=e.tv_show_id WHERE e.tv_show_id=$2 AND e.available AND NOT e.is_special ON CONFLICT DO NOTHING`, userID, id)
		if err != nil {
			return Detail{}, err
		}
	} else {
		_, err := store.pool.Exec(ctx, `DELETE FROM user_episode_progress p USING tv_episodes e WHERE p.episode_id=e.id AND p.user_id=$1 AND p.tv_show_id=$2 AND NOT e.is_special`, userID, id)
		if err != nil {
			return Detail{}, err
		}
	}
	return store.Detail(ctx, userID, id)
}
func (store *PostgresStore) Remove(ctx context.Context, userID, id string) (bool, error) {
	tag, err := store.pool.Exec(ctx, `DELETE FROM user_tv_shows WHERE user_id=$1 AND tv_show_id=$2`, userID, id)
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
