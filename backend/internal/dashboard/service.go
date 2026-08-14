package dashboard

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Scope string

const (
	ScopeAll    Scope = "all"
	ScopeGames  Scope = "games"
	ScopeMovies Scope = "movies"
	ScopeTV     Scope = "tv"
)

type Totals struct {
	Tracked int64 `json:"tracked"`
	Library int64 `json:"library"`
	Backlog int64 `json:"backlog"`
}

type Distribution struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int64  `json:"count"`
}

type Item struct {
	Domain      string  `json:"domain"`
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Year        *int32  `json:"year,omitempty"`
	ArtworkURL  string  `json:"artworkUrl,omitempty"`
	Status      string  `json:"status"`
	Rating      *int16  `json:"rating,omitempty"`
	Watched     *int32  `json:"watched,omitempty"`
	Total       *int32  `json:"total,omitempty"`
	Percent     *int32  `json:"percent,omitempty"`
	NextEpisode *string `json:"nextEpisode,omitempty"`
}

type Response struct {
	Scope              Scope               `json:"scope"`
	Totals             map[string]Totals   `json:"totals"`
	AverageRating      *float64            `json:"averageRating,omitempty"`
	AverageByDomain    map[string]*float64 `json:"averageByDomain"`
	RatingDistribution []Distribution      `json:"ratingDistribution"`
	StatusDistribution []Distribution      `json:"statusDistribution"`
	InProgress         []Item              `json:"inProgress"`
	HighestRated       []Item              `json:"highestRated"`
	TVProgress         []Item              `json:"tvProgress"`
}

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func ParseScope(value string) (Scope, error) {
	if value == "" {
		return ScopeAll, nil
	}
	scope := Scope(value)
	switch scope {
	case ScopeAll, ScopeGames, ScopeMovies, ScopeTV:
		return scope, nil
	default:
		return "", errors.New("choose All, Games, Movies, or TV")
	}
}

func (service *Service) Summary(ctx context.Context, userID string, scope Scope) (Response, error) {
	result := Response{
		Scope:              scope,
		Totals:             map[string]Totals{"games": {}, "movies": {}, "tv": {}},
		AverageByDomain:    map[string]*float64{"games": nil, "movies": nil, "tv": nil},
		RatingDistribution: []Distribution{},
		StatusDistribution: []Distribution{},
		InProgress:         []Item{},
		HighestRated:       []Item{},
		TVProgress:         []Item{},
	}
	rows, err := service.pool.Query(ctx, aggregateSQL, userID, scope)
	if err != nil {
		return Response{}, fmt.Errorf("query dashboard totals: %w", err)
	}
	var ratingSum float64
	var ratingCount int64
	for rows.Next() {
		var domain string
		var totals Totals
		var average pgtype.Numeric
		var domainRatingCount int64
		if err := rows.Scan(&domain, &totals.Tracked, &totals.Library, &totals.Backlog, &average, &domainRatingCount); err != nil {
			rows.Close()
			return Response{}, err
		}
		result.Totals[domain] = totals
		if average.Valid && domainRatingCount > 0 {
			value, _ := average.Float64Value()
			display := value.Float64 / 10
			result.AverageByDomain[domain] = &display
			ratingSum += value.Float64 * float64(domainRatingCount)
			ratingCount += domainRatingCount
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Response{}, err
	}
	rows.Close()
	if ratingCount > 0 {
		value := ratingSum / float64(ratingCount) / 10
		result.AverageRating = &value
	}
	if err := service.loadDistribution(ctx, userID, scope, ratingDistributionSQL, &result.RatingDistribution, true); err != nil {
		return Response{}, err
	}
	if err := service.loadDistribution(ctx, userID, scope, statusDistributionSQL, &result.StatusDistribution, false); err != nil {
		return Response{}, err
	}
	result.InProgress, err = service.loadItems(ctx, userID, scope, inProgressSQL)
	if err != nil {
		return Response{}, err
	}
	result.HighestRated, err = service.loadItems(ctx, userID, scope, highestRatedSQL)
	if err != nil {
		return Response{}, err
	}
	if scope == ScopeAll || scope == ScopeTV {
		result.TVProgress, err = service.loadItems(ctx, userID, ScopeTV, tvProgressSQL)
		if err != nil {
			return Response{}, err
		}
	}
	return result, nil
}

func (service *Service) loadDistribution(ctx context.Context, userID string, scope Scope, query string, destination *[]Distribution, rating bool) error {
	rows, err := service.pool.Query(ctx, query, userID, scope)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var count int64
		if err := rows.Scan(&key, &count); err != nil {
			return err
		}
		label := statusLabels[key]
		if rating {
			bucket, _ := strconv.Atoi(key)
			if bucket == 10 {
				label = "10.0"
			} else {
				label = fmt.Sprintf("%d.0–%d.9", bucket, bucket)
			}
		}
		*destination = append(*destination, Distribution{Key: key, Label: label, Count: count})
	}
	return rows.Err()
}

func (service *Service) loadItems(ctx context.Context, userID string, scope Scope, query string) ([]Item, error) {
	rows, err := service.pool.Query(ctx, query, userID, scope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Item{}
	for rows.Next() {
		var item Item
		var year, watched, total, percent pgtype.Int4
		var rating pgtype.Int2
		var next pgtype.Text
		if err := rows.Scan(&item.Domain, &item.ID, &item.Title, &year, &item.ArtworkURL, &item.Status, &rating, &watched, &total, &percent, &next); err != nil {
			return nil, err
		}
		item.Year = int32Pointer(year)
		item.Rating = int16Pointer(rating)
		item.Watched = int32Pointer(watched)
		item.Total = int32Pointer(total)
		item.Percent = int32Pointer(percent)
		item.NextEpisode = stringPointer(next)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (service *Service) RatingsCSV(ctx context.Context, userID string) ([]byte, error) {
	rows, err := service.pool.Query(ctx, ratingsExportSQL, userID)
	if err != nil {
		return nil, fmt.Errorf("query ratings export: %w", err)
	}
	defer rows.Close()
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	if err := writer.Write([]string{"domain", "gradeium_id", "provider_id", "title", "year", "status", "personal_rating", "rating_reason"}); err != nil {
		return nil, err
	}
	for rows.Next() {
		var domain, id, providerID, title, status string
		var year pgtype.Int4
		var rating int16
		var reason pgtype.Text
		if err := rows.Scan(&domain, &id, &providerID, &title, &year, &status, &rating, &reason); err != nil {
			return nil, err
		}
		yearValue, reasonValue := "", ""
		if year.Valid {
			yearValue = strconv.Itoa(int(year.Int32))
		}
		if reason.Valid {
			reasonValue = reason.String
		}
		if err := writer.Write([]string{domain, id, providerID, title, yearValue, status, fmt.Sprintf("%.1f", float64(rating)/10), reasonValue}); err != nil {
			return nil, err
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

var statusLabels = map[string]string{
	"backlog": "Backlog", "in_progress": "In Progress", "on_hold": "On Hold",
	"abandoned": "Abandoned", "completed": "Completed",
}

const mediaStateCTE = `
WITH media_state AS (
    SELECT 'games'::text domain, ug.user_id, g.entity_id id, g.igdb_id provider_id,
           g.english_title title, g.release_year, ug.status::text status, ug.rating,
           ug.rating_reason, ug.date_added
    FROM user_games ug JOIN games g ON g.entity_id=ug.game_id
    UNION ALL
    SELECT 'movies', um.user_id, m.entity_id, m.tmdb_id, m.english_title,
           m.release_year, um.status::text, um.rating, um.rating_reason, um.date_added
    FROM user_movies um JOIN movies m ON m.entity_id=um.movie_id
    UNION ALL
    SELECT 'tv', ut.user_id, t.entity_id, t.tvdb_id, t.english_title,
           t.release_year, ut.status::text, ut.rating, ut.rating_reason, ut.date_added
    FROM user_tv_shows ut JOIN tv_shows t ON t.entity_id=ut.tv_show_id
)`

const aggregateSQL = mediaStateCTE + `
SELECT domain, count(*), count(*) FILTER (WHERE status <> 'backlog'),
       count(*) FILTER (WHERE status = 'backlog'),
       avg(rating) FILTER (WHERE rating IS NOT NULL AND status <> 'backlog'),
       count(rating) FILTER (WHERE status <> 'backlog')
FROM media_state
WHERE user_id=$1 AND ($2='all' OR domain=$2)
GROUP BY domain ORDER BY domain`

const ratingDistributionSQL = mediaStateCTE + `
SELECT bucket::text, count FROM (
    SELECT LEAST(10, ((rating - 10) / 10) + 1) bucket, count(*) count
    FROM media_state
    WHERE user_id=$1 AND ($2='all' OR domain=$2) AND status <> 'backlog' AND rating IS NOT NULL
    GROUP BY LEAST(10, ((rating - 10) / 10) + 1)
) distribution ORDER BY bucket`

const statusDistributionSQL = mediaStateCTE + `
SELECT status, count(*) FROM media_state
WHERE user_id=$1 AND ($2='all' OR domain=$2)
GROUP BY status
ORDER BY array_position(ARRAY['backlog','in_progress','on_hold','abandoned','completed'], status)`

const dashboardItemsCTE = `
WITH dashboard_items AS (
    SELECT 'games'::text domain, g.entity_id::text id, g.english_title title, g.release_year,
           COALESCE(selected.image_url,preferred.image_url,'') artwork_url,
           ug.status::text status, ug.rating, ug.date_added,
           NULL::integer watched, NULL::integer total, NULL::integer percent, NULL::text next_episode
    FROM user_games ug JOIN games g ON g.entity_id=ug.game_id
    LEFT JOIN LATERAL(SELECT image_url FROM media_artwork a WHERE a.entity_id=g.entity_id AND a.available AND a.kind='cover' AND a.provider_image_id=ug.selected_cover_provider_image_id LIMIT 1) selected ON true
    LEFT JOIN LATERAL(SELECT image_url FROM media_artwork a WHERE a.entity_id=g.entity_id AND a.available AND a.kind='cover' ORDER BY a.preferred DESC,a.sort_order LIMIT 1) preferred ON true
    WHERE ug.user_id=$1
    UNION ALL
    SELECT 'movies', m.entity_id::text, m.english_title, m.release_year,
           COALESCE(selected.image_url,preferred.image_url,''), um.status::text, um.rating, um.date_added,
           NULL::integer, NULL::integer, NULL::integer, NULL::text
    FROM user_movies um JOIN movies m ON m.entity_id=um.movie_id
    LEFT JOIN LATERAL(SELECT image_url FROM media_artwork a WHERE a.entity_id=m.entity_id AND a.available AND a.kind='poster' AND a.provider_image_id=um.selected_poster_provider_image_id LIMIT 1) selected ON true
    LEFT JOIN LATERAL(SELECT image_url FROM media_artwork a WHERE a.entity_id=m.entity_id AND a.available AND a.kind='poster' ORDER BY a.preferred DESC,a.sort_order LIMIT 1) preferred ON true
    WHERE um.user_id=$1
    UNION ALL
    SELECT 'tv', t.entity_id::text, t.english_title, t.release_year,
           COALESCE(selected.image_url,preferred.image_url,''), ut.status::text, ut.rating, ut.date_added,
           progress.watched::integer, progress.total::integer,
           CASE WHEN progress.total=0 THEN 0 ELSE round(progress.watched*100.0/progress.total)::integer END,
           next_episode.label
    FROM user_tv_shows ut JOIN tv_shows t ON t.entity_id=ut.tv_show_id
    LEFT JOIN LATERAL(SELECT image_url FROM media_artwork a WHERE a.entity_id=t.entity_id AND a.available AND a.kind='poster' AND a.provider_image_id=ut.selected_poster_provider_image_id LIMIT 1) selected ON true
    LEFT JOIN LATERAL(SELECT image_url FROM media_artwork a WHERE a.entity_id=t.entity_id AND a.available AND a.kind='poster' ORDER BY a.preferred DESC,a.sort_order LIMIT 1) preferred ON true
    LEFT JOIN LATERAL(
        SELECT count(*) FILTER (WHERE p.episode_id IS NOT NULL) watched, count(*) total
        FROM tv_episodes e LEFT JOIN user_episode_progress p ON p.episode_id=e.id AND p.user_id=ut.user_id
        WHERE e.tv_show_id=t.entity_id AND e.available AND NOT e.is_special
    ) progress ON true
    LEFT JOIN LATERAL(
        SELECT 'S'||e.season_number||' E'||e.episode_number||' · '||e.english_title label
        FROM tv_episodes e LEFT JOIN user_episode_progress p ON p.episode_id=e.id AND p.user_id=ut.user_id
        WHERE e.tv_show_id=t.entity_id AND e.available AND NOT e.is_special AND p.episode_id IS NULL
        ORDER BY e.sort_order LIMIT 1
    ) next_episode ON true
    WHERE ut.user_id=$1
)`

const itemColumns = `SELECT domain,id,title,release_year,artwork_url,status,rating,watched,total,percent,next_episode FROM dashboard_items `

const inProgressSQL = dashboardItemsCTE + itemColumns + `WHERE ($2='all' OR domain=$2) AND status='in_progress' ORDER BY date_added DESC,lower(title),domain,id LIMIT 12`
const highestRatedSQL = dashboardItemsCTE + itemColumns + `WHERE ($2='all' OR domain=$2) AND status<>'backlog' AND rating IS NOT NULL ORDER BY rating DESC,lower(title),domain,id LIMIT 10`
const tvProgressSQL = dashboardItemsCTE + itemColumns + `WHERE $2='tv' AND domain='tv' AND status<>'backlog' AND total>0 ORDER BY (status='in_progress') DESC,percent DESC,lower(title),id LIMIT 8`

const ratingsExportSQL = mediaStateCTE + `
SELECT domain, id::text, provider_id::text, title, release_year, status, rating, rating_reason
FROM media_state WHERE user_id=$1 AND rating IS NOT NULL AND status<>'backlog'
ORDER BY domain, lower(title), id`

func int32Pointer(value pgtype.Int4) *int32 {
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

func stringPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	copy := value.String
	return &copy
}
