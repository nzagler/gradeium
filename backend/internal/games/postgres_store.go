package games

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nzagler/gradeium/backend/internal/integrations/igdb"
	"github.com/nzagler/gradeium/backend/internal/media"
)

type PostgresStore struct{ pool *pgxpool.Pool }

func NewPostgresStore(pool *pgxpool.Pool) *PostgresStore { return &PostgresStore{pool: pool} }

func (store *PostgresStore) Tracked(ctx context.Context, userID string, ids []int64) (map[int64]Tracked, error) {
	result := map[int64]Tracked{}
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := store.pool.Query(ctx, `SELECT g.igdb_id,g.entity_id::text,ug.status::text FROM games g JOIN user_games ug ON ug.game_id=g.entity_id WHERE ug.user_id=$1 AND g.igdb_id=ANY($2)`, userID, ids)
	if err != nil {
		return nil, fmt.Errorf("query tracked games: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var providerID int64
		var value Tracked
		if err := rows.Scan(&providerID, &value.ID, &value.Status); err != nil {
			return nil, err
		}
		result[providerID] = value
	}
	return result, rows.Err()
}

func (store *PostgresStore) Add(ctx context.Context, userID string, game igdb.Game, status media.Status) (Detail, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Detail{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, fmt.Sprintf("game:%d", game.ProviderID)); err != nil {
		return Detail{}, err
	}
	var entityID string
	err = tx.QueryRow(ctx, `SELECT entity_id::text FROM games WHERE igdb_id=$1`, game.ProviderID).Scan(&entityID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = tx.QueryRow(ctx, `INSERT INTO entities(type) VALUES('game') RETURNING id::text`).Scan(&entityID); err != nil {
			return Detail{}, err
		}
	} else if err != nil {
		return Detail{}, err
	}
	if err = store.persistCanonical(ctx, tx, entityID, game); err != nil {
		return Detail{}, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO user_games(user_id,game_id,status) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, userID, entityID, status)
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

func (store *PostgresStore) Refresh(ctx context.Context, userID string, game igdb.Game) (Detail, error) {
	tx, err := store.pool.Begin(ctx)
	if err != nil {
		return Detail{}, err
	}
	defer tx.Rollback(ctx)
	var entityID string
	if err = tx.QueryRow(ctx, `SELECT entity_id::text FROM games WHERE igdb_id=$1 FOR UPDATE`, game.ProviderID).Scan(&entityID); err != nil {
		return Detail{}, mapNotFound(err)
	}
	if err = store.persistCanonical(ctx, tx, entityID, game); err != nil {
		return Detail{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Detail{}, err
	}
	return store.Detail(ctx, userID, entityID)
}

func (store *PostgresStore) persistCanonical(ctx context.Context, tx pgx.Tx, entityID string, game igdb.Game) error {
	screenshots, _ := json.Marshal(nonNil(game.Screenshots))
	links, _ := json.Marshal(nonNil(game.ExternalLinks))
	_, err := tx.Exec(ctx, `INSERT INTO games(entity_id,igdb_id,english_title,original_title,summary,first_release_date,release_year,game_type,developer,publisher,genres,game_modes,platforms,franchise,community_rating,community_rating_count,screenshots,external_links,metadata_refreshed_at,updated_at) VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,NULLIF($9,''),NULLIF($10,''),$11,$12,$13,NULLIF($14,''),$15,$16,$17,$18,now(),now()) ON CONFLICT(entity_id) DO UPDATE SET english_title=EXCLUDED.english_title,original_title=EXCLUDED.original_title,summary=EXCLUDED.summary,first_release_date=EXCLUDED.first_release_date,release_year=EXCLUDED.release_year,game_type=EXCLUDED.game_type,developer=EXCLUDED.developer,publisher=EXCLUDED.publisher,genres=EXCLUDED.genres,game_modes=EXCLUDED.game_modes,platforms=EXCLUDED.platforms,franchise=EXCLUDED.franchise,community_rating=EXCLUDED.community_rating,community_rating_count=EXCLUDED.community_rating_count,screenshots=EXCLUDED.screenshots,external_links=EXCLUDED.external_links,metadata_refreshed_at=now(),updated_at=now()`, entityID, game.ProviderID, game.Title, game.OriginalTitle, game.Summary, dateValue(game.ReleaseDate), game.Year, game.GameType, game.Developer, game.Publisher, game.Genres, game.GameModes, game.Platforms, game.Franchise, game.CommunityRating, game.CommunityRatingCount, screenshots, links)
	if err != nil {
		return fmt.Errorf("persist game metadata: %w", err)
	}
	if _, err = tx.Exec(ctx, `DELETE FROM game_additional_content WHERE game_id=$1`, entityID); err != nil {
		return err
	}
	for _, item := range game.AdditionalContent {
		if _, err = tx.Exec(ctx, `INSERT INTO game_additional_content(game_id,igdb_id,title,content_type,release_year,cover_url) VALUES($1,$2,$3,$4,$5,NULLIF($6,''))`, entityID, item.ProviderID, item.Title, item.Type, item.Year, item.CoverURL); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `DELETE FROM game_related_releases WHERE game_id=$1`, entityID); err != nil {
		return err
	}
	for _, item := range game.RelatedReleases {
		if _, err = tx.Exec(ctx, `INSERT INTO game_related_releases(game_id,igdb_id,title,relationship,release_year,cover_url) VALUES($1,$2,$3,$4,$5,NULLIF($6,'')) ON CONFLICT DO NOTHING`, entityID, item.ProviderID, item.Title, item.Relationship, item.Year, item.CoverURL); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE media_artwork SET available=false,preferred=false,updated_at=now() WHERE entity_id=$1 AND provider='igdb'`, entityID); err != nil {
		return err
	}
	for index, item := range game.Artworks {
		if _, err = tx.Exec(ctx, `INSERT INTO media_artwork(entity_id,provider,provider_image_id,kind,language,image_url,thumbnail_url,width,height,preferred,available,sort_order) VALUES($1,'igdb',$2,$3,NULLIF($4,''),$5,$6,NULLIF($7,0),NULLIF($8,0),$9,true,$10) ON CONFLICT(entity_id,provider_image_id) DO UPDATE SET kind=EXCLUDED.kind,language=EXCLUDED.language,image_url=EXCLUDED.image_url,thumbnail_url=EXCLUDED.thumbnail_url,width=EXCLUDED.width,height=EXCLUDED.height,preferred=EXCLUDED.preferred,available=true,sort_order=EXCLUDED.sort_order,updated_at=now()`, entityID, item.ProviderImageID, item.Kind, item.Language, item.ImageURL, item.ThumbnailURL, item.Width, item.Height, item.Preferred, index); err != nil {
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

const gameSelect = `SELECT g.entity_id::text,g.igdb_id,g.english_title,COALESCE(g.summary,''),g.first_release_date,g.release_year,g.game_type,COALESCE(g.developer,''),COALESCE(g.publisher,''),g.genres,g.game_modes,g.platforms,COALESCE(g.franchise,''),g.community_rating,g.community_rating_count,g.screenshots,g.external_links,g.metadata_refreshed_at,ug.status::text,ug.rating,ug.rating_reason,ug.date_added,ug.selected_cover_provider_image_id,ug.selected_backdrop_provider_image_id,ug.selected_logo_provider_image_id,COALESCE(selected.image_url,preferred.image_url,'') FROM games g JOIN user_games ug ON ug.game_id=g.entity_id LEFT JOIN LATERAL(SELECT a.image_url FROM media_artwork a WHERE a.entity_id=g.entity_id AND a.available AND a.provider_image_id=ug.selected_cover_provider_image_id AND a.kind='cover' LIMIT 1)selected ON true LEFT JOIN LATERAL(SELECT a.image_url FROM media_artwork a WHERE a.entity_id=g.entity_id AND a.available AND a.kind='cover' ORDER BY a.preferred DESC,a.sort_order LIMIT 1)preferred ON true `

func (store *PostgresStore) List(ctx context.Context, userID string, backlog bool) ([]Item, error) {
	operator := "<>"
	if backlog {
		operator = "="
	}
	rows, err := store.pool.Query(ctx, gameSelect+`WHERE ug.user_id=$1 AND ug.status `+operator+` 'backlog' ORDER BY ug.rating DESC NULLS LAST,lower(g.english_title)`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		detail, err := scanGame(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, detail.Item)
	}
	return items, rows.Err()
}
func (store *PostgresStore) Detail(ctx context.Context, userID, id string) (Detail, error) {
	row := store.pool.QueryRow(ctx, gameSelect+`WHERE ug.user_id=$1 AND g.entity_id=$2`, userID, id)
	detail, err := scanGame(row)
	if err != nil {
		return Detail{}, mapNotFound(err)
	}
	if err = store.loadDetail(ctx, userID, &detail); err != nil {
		return Detail{}, err
	}
	return detail, nil
}

type rowScanner interface{ Scan(...any) error }

func scanGame(row rowScanner) (Detail, error) {
	var result Detail
	var summary, publisher, gameType, developer, franchise string
	var release pgtype.Date
	var year pgtype.Int4
	var community pgtype.Int2
	var count pgtype.Int4
	var rating pgtype.Int2
	var reason, coverPin, backdropPin, logoPin pgtype.Text
	var screenshots, links []byte
	var status string
	err := row.Scan(&result.ID, &result.ProviderID, &result.Title, &summary, &release, &year, &gameType, &developer, &publisher, &result.Genres, &result.GameModes, &result.Platforms, &franchise, &community, &count, &screenshots, &links, &result.MetadataRefreshedAt, &status, &rating, &reason, &result.State.DateAdded, &coverPin, &backdropPin, &logoPin, &result.ArtworkURL)
	if err != nil {
		return Detail{}, err
	}
	result.Summary = summary
	result.Publisher = publisher
	result.GameType = gameType
	result.Developer = developer
	result.Franchise = franchise
	result.Year = intPointer(year)
	result.ReleaseDate = datePointer(release)
	result.CommunityRating = int16Pointer(community)
	result.CommunityRatingCount = int32Pointer(count)
	result.State.PersonalState = media.PersonalState{Status: media.Status(status), Rating: int16Pointer(rating), RatingReason: textPointer(reason)}
	_ = json.Unmarshal(screenshots, &result.Screenshots)
	_ = json.Unmarshal(links, &result.ExternalLinks)
	result.ArtworkPins = map[string]string{}
	for kind, value := range map[string]pgtype.Text{"cover": coverPin, "backdrop": backdropPin, "logo": logoPin} {
		if value.Valid {
			result.ArtworkPins[kind] = value.String
		}
	}
	return result, nil
}

func (store *PostgresStore) loadDetail(ctx context.Context, userID string, result *Detail) error {
	rows, err := store.pool.Query(ctx, `SELECT igdb_id,title,content_type,release_year,COALESCE(cover_url,'') FROM game_additional_content WHERE game_id=$1 ORDER BY release_year NULLS LAST,title`, result.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	result.AdditionalContent = []igdb.AdditionalContent{}
	for rows.Next() {
		var item igdb.AdditionalContent
		var year pgtype.Int4
		if err := rows.Scan(&item.ProviderID, &item.Title, &item.Type, &year, &item.CoverURL); err != nil {
			return err
		}
		if year.Valid {
			value := int(year.Int32)
			item.Year = &value
		}
		result.AdditionalContent = append(result.AdditionalContent, item)
	}
	rows.Close()
	rows, err = store.pool.Query(ctx, `SELECT r.igdb_id,r.title,r.relationship,r.release_year,COALESCE(r.cover_url,''),g.entity_id::text,ug.status::text,ug.rating FROM game_related_releases r LEFT JOIN games g ON g.igdb_id=r.igdb_id LEFT JOIN user_games ug ON ug.game_id=g.entity_id AND ug.user_id=$2 WHERE r.game_id=$1 ORDER BY r.relationship,r.release_year NULLS LAST`, result.ID, userID)
	if err != nil {
		return err
	}
	result.RelatedReleases = []RelatedRelease{}
	for rows.Next() {
		var item RelatedRelease
		var year pgtype.Int4
		var localID, localStatus pgtype.Text
		var rating pgtype.Int2
		if err := rows.Scan(&item.ProviderID, &item.Title, &item.Relationship, &year, &item.CoverURL, &localID, &localStatus, &rating); err != nil {
			return err
		}
		if year.Valid {
			value := int(year.Int32)
			item.Year = &value
		}
		if localID.Valid {
			item.LocalID = localID.String
			item.LocalStatus = media.Status(localStatus.String)
			item.LocalRating = int16Pointer(rating)
		}
		result.RelatedReleases = append(result.RelatedReleases, item)
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
		if pin := result.ArtworkPins[item.Kind]; pin == item.ProviderImageID && !item.Available {
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
	var currentRating pgtype.Int2
	var currentReason pgtype.Text
	if err = tx.QueryRow(ctx, `SELECT rating,rating_reason FROM user_games WHERE user_id=$1 AND game_id=$2 FOR UPDATE`, userID, id).Scan(&currentRating, &currentReason); err != nil {
		return State{}, mapNotFound(err)
	}
	if state.Status == media.StatusBacklog && (currentRating.Valid || currentReason.Valid) && !confirm {
		return State{}, ErrConfirmationRequired
	}
	if state.Status == media.StatusBacklog {
		state.Rating = nil
		state.RatingReason = nil
	}
	var added time.Time
	if err = tx.QueryRow(ctx, `UPDATE user_games SET status=$3,rating=$4,rating_reason=$5,updated_at=now() WHERE user_id=$1 AND game_id=$2 RETURNING date_added`, userID, id, state.Status, state.Rating, state.RatingReason).Scan(&added); err != nil {
		return State{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return State{}, err
	}
	return State{PersonalState: state, DateAdded: added}, nil
}
func (store *PostgresStore) SelectArtwork(ctx context.Context, userID, id, kind, providerImageID string) (Detail, error) {
	column := map[string]string{"cover": "selected_cover_provider_image_id", "backdrop": "selected_backdrop_provider_image_id", "logo": "selected_logo_provider_image_id"}[kind]
	if column == "" {
		return Detail{}, errors.New("unsupported artwork kind")
	}
	var value any = nil
	if providerImageID != "" {
		var exists bool
		if err := store.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM media_artwork WHERE entity_id=$1 AND provider_image_id=$2 AND kind=$3 AND available)`, id, providerImageID, kind).Scan(&exists); err != nil {
			return Detail{}, err
		}
		if !exists {
			return Detail{}, errors.New("provider artwork is not available")
		}
		value = providerImageID
	}
	tag, err := store.pool.Exec(ctx, `UPDATE user_games SET `+column+`=$3,updated_at=now() WHERE user_id=$1 AND game_id=$2`, userID, id, value)
	if err != nil {
		return Detail{}, err
	}
	if tag.RowsAffected() != 1 {
		return Detail{}, ErrNotFound
	}
	return store.Detail(ctx, userID, id)
}
func (store *PostgresStore) Remove(ctx context.Context, userID, id string) (bool, error) {
	tag, err := store.pool.Exec(ctx, `DELETE FROM user_games WHERE user_id=$1 AND game_id=$2`, userID, id)
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
func int32Pointer(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	copy := value.Int32
	return &copy
}
func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}
