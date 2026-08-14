package backups

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresStore struct {
	pool *pgxpool.Pool
}

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore {
	return &PostgresStore{pool: pool}
}

func (store *PostgresStore) Snapshot(ctx context.Context, applicationVersion string) (Document, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Document{}, fmt.Errorf("begin backup snapshot: %w", err)
	}
	defer tx.Rollback(ctx)

	document := Document{
		Format:             Format,
		Version:            FormatVersion,
		CreatedAt:          utcNow(),
		ApplicationVersion: applicationVersion,
		Users:              []User{},
		Games:              []Game{},
		Movies:             []Movie{},
		TVShows:            []TVShow{},
		EpisodeProgress:    []Progress{},
		Settings:           []Setting{},
	}
	if err := queryJSONRows(ctx, tx, snapshotUsersSQL, &document.Users); err != nil {
		return Document{}, fmt.Errorf("snapshot users: %w", err)
	}
	if err := queryJSONRows(ctx, tx, snapshotGamesSQL, &document.Games); err != nil {
		return Document{}, fmt.Errorf("snapshot games: %w", err)
	}
	if err := queryJSONRows(ctx, tx, snapshotMoviesSQL, &document.Movies); err != nil {
		return Document{}, fmt.Errorf("snapshot movies: %w", err)
	}
	if err := queryJSONRows(ctx, tx, snapshotTVSQL, &document.TVShows); err != nil {
		return Document{}, fmt.Errorf("snapshot TV shows: %w", err)
	}
	if err := queryJSONRows(ctx, tx, snapshotProgressSQL, &document.EpisodeProgress); err != nil {
		return Document{}, fmt.Errorf("snapshot TV progress: %w", err)
	}
	rows, err := tx.Query(ctx, `SELECT key, value FROM app_settings WHERE key = 'general.instance_name' ORDER BY key`)
	if err != nil {
		return Document{}, fmt.Errorf("snapshot portable settings: %w", err)
	}
	for rows.Next() {
		var setting Setting
		if err := rows.Scan(&setting.Key, &setting.Value); err != nil {
			rows.Close()
			return Document{}, err
		}
		document.Settings = append(document.Settings, setting)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Document{}, err
	}
	rows.Close()
	if err := Validate(document); err != nil {
		return Document{}, fmt.Errorf("validate generated backup: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Document{}, fmt.Errorf("finish backup snapshot: %w", err)
	}
	return document, nil
}

func queryJSONRows[T any](ctx context.Context, tx pgx.Tx, query string, destination *[]T) error {
	rows, err := tx.Query(ctx, query)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		var value T
		if err := json.Unmarshal(raw, &value); err != nil {
			return err
		}
		*destination = append(*destination, value)
	}
	return rows.Err()
}

const snapshotUsersSQL = `
SELECT jsonb_build_object(
    'id', u.id,
    'oidcIssuer', u.oidc_issuer,
    'oidcSubject', u.oidc_subject,
    'displayName', u.display_name,
    'email', u.email,
    'preferences', jsonb_build_object(
        'defaultLibrarySort', COALESCE(us.default_library_sort, 'rating_desc'),
        'preferredView', COALESCE(us.preferred_view, 'grid')
    )
)
FROM users u
LEFT JOIN user_settings us ON us.user_id = u.id
ORDER BY u.id`

const snapshotGamesSQL = `
SELECT jsonb_build_object(
    'id', g.entity_id,
    'providerId', g.igdb_id,
    'title', g.english_title,
    'originalTitle', g.original_title,
    'summary', g.summary,
    'releaseDate', CASE WHEN g.first_release_date IS NULL THEN NULL ELSE g.first_release_date::timestamp AT TIME ZONE 'UTC' END,
    'releaseYear', g.release_year,
    'gameType', g.game_type,
    'developer', g.developer,
    'publisher', g.publisher,
    'genres', g.genres,
    'gameModes', g.game_modes,
    'platforms', g.platforms,
    'franchise', g.franchise,
    'communityRating', g.community_rating,
    'communityRatingCount', g.community_rating_count,
    'screenshots', g.screenshots,
    'externalLinks', g.external_links,
    'metadataRefreshedAt', g.metadata_refreshed_at,
    'artwork', COALESCE((
        SELECT jsonb_agg(jsonb_build_object(
            'provider', a.provider, 'providerImageId', a.provider_image_id, 'kind', a.kind,
            'language', a.language, 'imageUrl', a.image_url, 'thumbnailUrl', a.thumbnail_url,
            'width', a.width, 'height', a.height, 'preferred', a.preferred,
            'available', a.available, 'sortOrder', a.sort_order
        ) ORDER BY a.kind, a.sort_order, a.provider_image_id)
        FROM media_artwork a WHERE a.entity_id = g.entity_id
    ), '[]'::jsonb),
    'additionalContent', COALESCE((
        SELECT jsonb_agg(jsonb_build_object(
            'providerId', c.igdb_id, 'title', c.title, 'type', c.content_type,
            'year', c.release_year, 'coverUrl', c.cover_url
        ) ORDER BY c.igdb_id)
        FROM game_additional_content c WHERE c.game_id = g.entity_id
    ), '[]'::jsonb),
    'relatedReleases', COALESCE((
        SELECT jsonb_agg(jsonb_build_object(
            'providerId', r.igdb_id, 'title', r.title, 'relationship', r.relationship,
            'year', r.release_year, 'coverUrl', r.cover_url
        ) ORDER BY r.igdb_id, r.relationship)
        FROM game_related_releases r WHERE r.game_id = g.entity_id
    ), '[]'::jsonb),
    'users', COALESCE((
        SELECT jsonb_agg(jsonb_build_object(
            'userId', ug.user_id, 'status', ug.status, 'rating', ug.rating,
            'ratingReason', ug.rating_reason, 'dateAdded', ug.date_added,
            'selectedCoverId', ug.selected_cover_provider_image_id,
            'selectedBackdropId', ug.selected_backdrop_provider_image_id,
            'selectedLogoId', ug.selected_logo_provider_image_id
        ) ORDER BY ug.user_id)
        FROM user_games ug WHERE ug.game_id = g.entity_id
    ), '[]'::jsonb)
)
FROM games g
WHERE EXISTS (SELECT 1 FROM user_games ug WHERE ug.game_id = g.entity_id)
ORDER BY g.entity_id`

const snapshotMoviesSQL = `
SELECT jsonb_build_object(
    'id', m.entity_id, 'providerId', m.tmdb_id, 'title', m.english_title,
    'originalTitle', m.original_title, 'overview', m.overview,
    'releaseDate', CASE WHEN m.release_date IS NULL THEN NULL ELSE m.release_date::timestamp AT TIME ZONE 'UTC' END,
    'releaseYear', m.release_year, 'runtimeMinutes', m.runtime_minutes,
    'director', m.director, 'genres', m.genres, 'productionCompanies', m.production_companies,
    'cast', m.cast_members, 'crew', m.key_crew, 'trailerKey', m.trailer_key,
    'imdbId', m.imdb_id, 'homepage', m.homepage, 'collectionId', m.collection_tmdb_id,
    'collectionName', m.collection_name, 'communityRating', m.community_rating,
    'communityRatingCount', m.community_rating_count, 'metadataRefreshedAt', m.metadata_refreshed_at,
    'artwork', COALESCE((
        SELECT jsonb_agg(jsonb_build_object(
            'provider', a.provider, 'providerImageId', a.provider_image_id, 'kind', a.kind,
            'language', a.language, 'imageUrl', a.image_url, 'thumbnailUrl', a.thumbnail_url,
            'width', a.width, 'height', a.height, 'preferred', a.preferred,
            'available', a.available, 'sortOrder', a.sort_order
        ) ORDER BY a.kind, a.sort_order, a.provider_image_id)
        FROM media_artwork a WHERE a.entity_id = m.entity_id
    ), '[]'::jsonb),
    'collection', COALESCE((
        SELECT jsonb_agg(jsonb_build_object(
            'providerId', c.tmdb_id, 'title', c.title,
            'releaseDate', CASE WHEN c.release_date IS NULL THEN NULL ELSE c.release_date::timestamp AT TIME ZONE 'UTC' END,
            'posterUrl', c.poster_url
        ) ORDER BY c.tmdb_id)
        FROM movie_collection_members c WHERE c.movie_id = m.entity_id
    ), '[]'::jsonb),
    'users', COALESCE((
        SELECT jsonb_agg(jsonb_build_object(
            'userId', um.user_id, 'status', um.status, 'rating', um.rating,
            'ratingReason', um.rating_reason, 'dateAdded', um.date_added,
            'selectedPosterId', um.selected_poster_provider_image_id,
            'selectedBackdropId', um.selected_backdrop_provider_image_id,
            'selectedLogoId', um.selected_logo_provider_image_id
        ) ORDER BY um.user_id)
        FROM user_movies um WHERE um.movie_id = m.entity_id
    ), '[]'::jsonb)
)
FROM movies m
WHERE EXISTS (SELECT 1 FROM user_movies um WHERE um.movie_id = m.entity_id)
ORDER BY m.entity_id`

const snapshotTVSQL = `
SELECT jsonb_build_object(
    'id', t.entity_id, 'providerId', t.tvdb_id, 'verifiedTmdbId', t.verified_tmdb_id,
    'title', t.english_title, 'originalTitle', t.original_title, 'overview', t.overview,
    'firstAired', CASE WHEN t.first_aired IS NULL THEN NULL ELSE t.first_aired::timestamp AT TIME ZONE 'UTC' END,
    'releaseYear', t.release_year, 'providerStatus', t.provider_status, 'network', t.network_name,
    'genres', t.genres, 'cast', t.cast_members, 'keyPeople', t.key_people,
    'communityRating', t.community_rating, 'communityRatingCount', t.community_rating_count,
    'tmdbMappingVerifiedAt', t.tmdb_mapping_verified_at,
    'metadataRefreshedAt', t.metadata_refreshed_at,
    'communityRatingRefreshedAt', t.community_rating_refreshed_at,
    'artwork', COALESCE((
        SELECT jsonb_agg(jsonb_build_object(
            'provider', a.provider, 'providerImageId', a.provider_image_id, 'kind', a.kind,
            'language', a.language, 'imageUrl', a.image_url, 'thumbnailUrl', a.thumbnail_url,
            'width', a.width, 'height', a.height, 'preferred', a.preferred,
            'available', a.available, 'sortOrder', a.sort_order
        ) ORDER BY a.kind, a.sort_order, a.provider_image_id)
        FROM media_artwork a WHERE a.entity_id = t.entity_id
    ), '[]'::jsonb),
    'seasons', COALESCE((
        SELECT jsonb_agg(jsonb_build_object(
            'id', s.id, 'providerId', s.tvdb_season_id, 'number', s.season_number,
            'name', s.name, 'special', s.is_specials,
            'airDate', CASE WHEN s.air_date IS NULL THEN NULL ELSE s.air_date::timestamp AT TIME ZONE 'UTC' END,
            'posterUrl', s.poster_url, 'available', s.available,
            'episodes', COALESCE((
                SELECT jsonb_agg(jsonb_build_object(
                    'id', e.id, 'providerId', e.tvdb_episode_id,
                    'seasonNumber', e.season_number, 'episodeNumber', e.episode_number,
                    'sortOrder', e.sort_order, 'title', e.english_title, 'overview', e.overview,
                    'airDate', CASE WHEN e.air_date IS NULL THEN NULL ELSE e.air_date::timestamp AT TIME ZONE 'UTC' END,
                    'runtimeMinutes', e.runtime_minutes, 'stillUrl', e.still_url,
                    'special', e.is_special, 'available', e.available
                ) ORDER BY e.episode_number, e.id)
                FROM tv_episodes e WHERE e.season_id = s.id
            ), '[]'::jsonb)
        ) ORDER BY s.season_number, s.id)
        FROM tv_seasons s WHERE s.tv_show_id = t.entity_id
    ), '[]'::jsonb),
    'users', COALESCE((
        SELECT jsonb_agg(jsonb_build_object(
            'userId', ut.user_id, 'status', ut.status, 'rating', ut.rating,
            'ratingReason', ut.rating_reason, 'dateAdded', ut.date_added,
            'selectedPosterId', ut.selected_poster_provider_image_id,
            'selectedBackdropId', ut.selected_backdrop_provider_image_id,
            'selectedLogoId', ut.selected_logo_provider_image_id
        ) ORDER BY ut.user_id)
        FROM user_tv_shows ut WHERE ut.tv_show_id = t.entity_id
    ), '[]'::jsonb)
)
FROM tv_shows t
WHERE EXISTS (SELECT 1 FROM user_tv_shows ut WHERE ut.tv_show_id = t.entity_id)
ORDER BY t.entity_id`

const snapshotProgressSQL = `
SELECT jsonb_build_object(
    'userId', p.user_id, 'tvShowId', p.tv_show_id,
    'episodeId', p.episode_id, 'watchedAt', p.watched_at
)
FROM user_episode_progress p
ORDER BY p.user_id, p.tv_show_id, p.episode_id`

func (store *PostgresStore) Restore(ctx context.Context, document Document) error {
	if err := Validate(document); err != nil {
		return err
	}
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return fmt.Errorf("begin restore transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('gradeium-portable-restore', 0))`); err != nil {
		return fmt.Errorf("lock restore transaction: %w", err)
	}

	userIDs := make(map[string]string, len(document.Users))
	for _, user := range document.Users {
		currentID, err := restoreUser(ctx, tx, user)
		if err != nil {
			return err
		}
		userIDs[user.ID] = currentID
	}
	if _, err := tx.Exec(ctx, `DELETE FROM user_settings`); err != nil {
		return fmt.Errorf("clear existing user preferences: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM entities`); err != nil {
		return fmt.Errorf("clear existing portable media state: %w", err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM app_settings WHERE key = 'general.instance_name'`); err != nil {
		return fmt.Errorf("clear existing portable settings: %w", err)
	}

	for _, user := range document.Users {
		if _, err := tx.Exec(ctx, `INSERT INTO user_settings(user_id, default_library_sort, preferred_view) VALUES($1,$2,$3)`, userIDs[user.ID], user.Preferences.DefaultLibrarySort, user.Preferences.PreferredView); err != nil {
			return fmt.Errorf("restore user preferences: %w", err)
		}
	}
	for _, setting := range document.Settings {
		if _, err := tx.Exec(ctx, `INSERT INTO app_settings(key, value) VALUES($1,$2)`, setting.Key, []byte(setting.Value)); err != nil {
			return fmt.Errorf("restore portable setting: %w", err)
		}
	}
	for _, game := range document.Games {
		if err := restoreGame(ctx, tx, game, userIDs); err != nil {
			return err
		}
	}
	for _, movie := range document.Movies {
		if err := restoreMovie(ctx, tx, movie, userIDs); err != nil {
			return err
		}
	}
	for _, show := range document.TVShows {
		if err := restoreTVShow(ctx, tx, show, userIDs); err != nil {
			return err
		}
	}
	for _, progress := range document.EpisodeProgress {
		if _, err := tx.Exec(ctx, `INSERT INTO user_episode_progress(user_id, tv_show_id, episode_id, watched_at) VALUES($1,$2,$3,$4)`, userIDs[progress.UserID], progress.TVShowID, progress.EpisodeID, progress.WatchedAt); err != nil {
			return fmt.Errorf("restore TV episode progress: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit restored state: %w", err)
	}
	return nil
}

func restoreUser(ctx context.Context, tx pgx.Tx, user User) (string, error) {
	if user.OIDCIssuer != nil {
		var id string
		err := tx.QueryRow(ctx, `SELECT id::text FROM users WHERE oidc_issuer=$1 AND oidc_subject=$2`, *user.OIDCIssuer, *user.OIDCSubject).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("match restored OIDC identity: %w", err)
		}
	}
	var issuer, subject pgtype.Text
	err := tx.QueryRow(ctx, `SELECT oidc_issuer, oidc_subject FROM users WHERE id=$1`, user.ID).Scan(&issuer, &subject)
	if err == nil {
		if user.OIDCIssuer == nil && !issuer.Valid {
			return user.ID, nil
		}
		if user.OIDCIssuer != nil && issuer.Valid && subject.Valid && issuer.String == *user.OIDCIssuer && subject.String == *user.OIDCSubject {
			return user.ID, nil
		}
		return "", errors.New("backup user ID conflicts with the current authentication identity")
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("inspect restored user identity: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO users(id, oidc_issuer, oidc_subject, display_name, email, is_admin) VALUES($1,$2,$3,$4,$5,false)`, user.ID, user.OIDCIssuer, user.OIDCSubject, user.DisplayName, user.Email); err != nil {
		return "", fmt.Errorf("restore non-administrator user identity: %w", err)
	}
	return user.ID, nil
}

func restoreGame(ctx context.Context, tx pgx.Tx, item Game, users map[string]string) error {
	screenshots, _ := json.Marshal(item.Screenshots)
	links, _ := json.Marshal(item.ExternalLinks)
	if _, err := tx.Exec(ctx, `INSERT INTO entities(id,type) VALUES($1,'game')`, item.ID); err != nil {
		return fmt.Errorf("restore game entity: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO games(entity_id,igdb_id,english_title,original_title,summary,first_release_date,release_year,game_type,developer,publisher,genres,game_modes,platforms,franchise,community_rating,community_rating_count,screenshots,external_links,metadata_refreshed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)`, item.ID, item.ProviderID, item.Title, item.OriginalTitle, item.Summary, item.ReleaseDate, item.ReleaseYear, item.GameType, item.Developer, item.Publisher, item.Genres, item.GameModes, item.Platforms, item.Franchise, item.CommunityRating, item.CommunityRatingCount, screenshots, links, item.MetadataRefreshedAt); err != nil {
		return fmt.Errorf("restore game metadata: %w", err)
	}
	if err := restoreArtwork(ctx, tx, item.ID, item.Artwork); err != nil {
		return err
	}
	for _, child := range item.AdditionalContent {
		if _, err := tx.Exec(ctx, `INSERT INTO game_additional_content(game_id,igdb_id,title,content_type,release_year,cover_url) VALUES($1,$2,$3,$4,$5,$6)`, item.ID, child.ProviderID, child.Title, child.Type, child.Year, child.CoverURL); err != nil {
			return fmt.Errorf("restore game additional content: %w", err)
		}
	}
	for _, related := range item.RelatedReleases {
		if _, err := tx.Exec(ctx, `INSERT INTO game_related_releases(game_id,igdb_id,title,relationship,release_year,cover_url) VALUES($1,$2,$3,$4,$5,$6)`, item.ID, related.ProviderID, related.Title, related.Relationship, related.Year, related.CoverURL); err != nil {
			return fmt.Errorf("restore game relationship: %w", err)
		}
	}
	for _, state := range item.Users {
		if _, err := tx.Exec(ctx, `INSERT INTO user_games(user_id,game_id,status,rating,rating_reason,selected_cover_provider_image_id,selected_backdrop_provider_image_id,selected_logo_provider_image_id,date_added) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, users[state.UserID], item.ID, state.Status, state.Rating, state.RatingReason, state.SelectedCoverID, state.SelectedBackdropID, state.SelectedLogoID, state.DateAdded); err != nil {
			return fmt.Errorf("restore user game state: %w", err)
		}
	}
	return nil
}

func restoreMovie(ctx context.Context, tx pgx.Tx, item Movie, users map[string]string) error {
	cast, _ := json.Marshal(item.Cast)
	crew, _ := json.Marshal(item.Crew)
	if _, err := tx.Exec(ctx, `INSERT INTO entities(id,type) VALUES($1,'movie')`, item.ID); err != nil {
		return fmt.Errorf("restore movie entity: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO movies(entity_id,tmdb_id,english_title,original_title,overview,release_date,release_year,runtime_minutes,director,genres,production_companies,cast_members,key_crew,trailer_key,imdb_id,homepage,collection_tmdb_id,collection_name,community_rating,community_rating_count,metadata_refreshed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)`, item.ID, item.ProviderID, item.Title, item.OriginalTitle, item.Overview, item.ReleaseDate, item.ReleaseYear, item.RuntimeMinutes, item.Director, item.Genres, item.ProductionCompanies, cast, crew, item.TrailerKey, item.IMDbID, item.Homepage, item.CollectionID, item.CollectionName, item.CommunityRating, item.CommunityRatingCount, item.MetadataRefreshedAt); err != nil {
		return fmt.Errorf("restore movie metadata: %w", err)
	}
	if err := restoreArtwork(ctx, tx, item.ID, item.Artwork); err != nil {
		return err
	}
	for _, member := range item.Collection {
		if _, err := tx.Exec(ctx, `INSERT INTO movie_collection_members(movie_id,tmdb_id,title,release_date,poster_url) VALUES($1,$2,$3,$4,$5)`, item.ID, member.ProviderID, member.Title, member.ReleaseDate, member.PosterURL); err != nil {
			return fmt.Errorf("restore movie collection: %w", err)
		}
	}
	for _, state := range item.Users {
		if _, err := tx.Exec(ctx, `INSERT INTO user_movies(user_id,movie_id,status,rating,rating_reason,selected_poster_provider_image_id,selected_backdrop_provider_image_id,selected_logo_provider_image_id,date_added) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, users[state.UserID], item.ID, state.Status, state.Rating, state.RatingReason, state.SelectedPosterID, state.SelectedBackdropID, state.SelectedLogoID, state.DateAdded); err != nil {
			return fmt.Errorf("restore user movie state: %w", err)
		}
	}
	return nil
}

func restoreTVShow(ctx context.Context, tx pgx.Tx, item TVShow, users map[string]string) error {
	cast, _ := json.Marshal(item.Cast)
	people, _ := json.Marshal(item.KeyPeople)
	if _, err := tx.Exec(ctx, `INSERT INTO entities(id,type) VALUES($1,'tv_show')`, item.ID); err != nil {
		return fmt.Errorf("restore TV entity: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO tv_shows(entity_id,tvdb_id,verified_tmdb_id,english_title,original_title,overview,first_aired,release_year,provider_status,network_name,genres,cast_members,key_people,community_rating,community_rating_count,tmdb_mapping_verified_at,metadata_refreshed_at,community_rating_refreshed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`, item.ID, item.ProviderID, item.VerifiedTMDBID, item.Title, item.OriginalTitle, item.Overview, item.FirstAired, item.ReleaseYear, item.ProviderStatus, item.Network, item.Genres, cast, people, item.CommunityRating, item.CommunityRatingCount, item.TMDBMappingVerifiedAt, item.MetadataRefreshedAt, item.CommunityRatingRefreshedAt); err != nil {
		return fmt.Errorf("restore TV metadata: %w", err)
	}
	if err := restoreArtwork(ctx, tx, item.ID, item.Artwork); err != nil {
		return err
	}
	for _, season := range item.Seasons {
		if _, err := tx.Exec(ctx, `INSERT INTO tv_seasons(id,tv_show_id,tvdb_season_id,season_number,name,is_specials,air_date,poster_url,available) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, season.ID, item.ID, season.ProviderID, season.Number, season.Name, season.Special, season.AirDate, season.PosterURL, season.Available); err != nil {
			return fmt.Errorf("restore TV season: %w", err)
		}
		for _, episode := range season.Episodes {
			if _, err := tx.Exec(ctx, `INSERT INTO tv_episodes(id,tv_show_id,season_id,tvdb_episode_id,season_number,episode_number,sort_order,english_title,overview,air_date,runtime_minutes,still_url,is_special,available) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, episode.ID, item.ID, season.ID, episode.ProviderID, episode.SeasonNumber, episode.EpisodeNumber, episode.SortOrder, episode.Title, episode.Overview, episode.AirDate, episode.RuntimeMinutes, episode.StillURL, episode.Special, episode.Available); err != nil {
				return fmt.Errorf("restore TV episode: %w", err)
			}
		}
	}
	for _, state := range item.Users {
		if _, err := tx.Exec(ctx, `INSERT INTO user_tv_shows(user_id,tv_show_id,status,rating,rating_reason,selected_poster_provider_image_id,selected_backdrop_provider_image_id,selected_logo_provider_image_id,date_added) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, users[state.UserID], item.ID, state.Status, state.Rating, state.RatingReason, state.SelectedPosterID, state.SelectedBackdropID, state.SelectedLogoID, state.DateAdded); err != nil {
			return fmt.Errorf("restore user TV state: %w", err)
		}
	}
	return nil
}

func restoreArtwork(ctx context.Context, tx pgx.Tx, entityID string, artwork []Artwork) error {
	for _, item := range artwork {
		if _, err := tx.Exec(ctx, `INSERT INTO media_artwork(entity_id,provider,provider_image_id,kind,language,image_url,thumbnail_url,width,height,preferred,available,sort_order) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, entityID, item.Provider, item.ProviderImageID, item.Kind, item.Language, item.ImageURL, item.ThumbnailURL, item.Width, item.Height, item.Preferred, item.Available, item.SortOrder); err != nil {
			return fmt.Errorf("restore provider artwork: %w", err)
		}
	}
	return nil
}
