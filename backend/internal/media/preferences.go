package media

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Preferences struct {
	DefaultLibrarySort string `json:"defaultLibrarySort"`
	PreferredView      string `json:"preferredView"`
	Theme              string `json:"theme"`
	RatingScale        string `json:"ratingScale"`
}

type PreferencesService struct{ pool *pgxpool.Pool }

func NewPreferencesService(pool *pgxpool.Pool) *PreferencesService {
	return &PreferencesService{pool: pool}
}

func (service *PreferencesService) Get(ctx context.Context, userID string) (Preferences, error) {
	var result Preferences
	err := service.pool.QueryRow(ctx, `
		INSERT INTO user_settings(user_id) VALUES($1)
		ON CONFLICT(user_id) DO UPDATE SET user_id=EXCLUDED.user_id
		RETURNING default_library_sort,preferred_view,theme,rating_scale`, userID).Scan(&result.DefaultLibrarySort, &result.PreferredView, &result.Theme, &result.RatingScale)
	if err != nil {
		return Preferences{}, fmt.Errorf("read library preferences: %w", err)
	}
	return result, nil
}

func (service *PreferencesService) Update(ctx context.Context, userID string, value Preferences) (Preferences, error) {
	value.Theme = NormalizeTheme(value.Theme)
	value.RatingScale = NormalizeRatingScale(value.RatingScale)
	if !validSort(value.DefaultLibrarySort) {
		return Preferences{}, errors.New("choose a valid default Library sort")
	}
	if value.PreferredView != "grid" && value.PreferredView != "list" {
		return Preferences{}, errors.New("choose grid or list view")
	}
	if !ValidTheme(value.Theme) {
		return Preferences{}, errors.New("choose Dark, Light, or System appearance")
	}
	if !ValidRatingScale(value.RatingScale) {
		return Preferences{}, errors.New("choose a valid personal rating scale")
	}
	var result Preferences
	err := service.pool.QueryRow(ctx, `
		INSERT INTO user_settings(user_id,default_library_sort,preferred_view,theme,rating_scale)
		VALUES($1,$2,$3,$4,$5)
		ON CONFLICT(user_id) DO UPDATE SET
			default_library_sort=EXCLUDED.default_library_sort,
			preferred_view=EXCLUDED.preferred_view,
			theme=EXCLUDED.theme,
			rating_scale=EXCLUDED.rating_scale,
			updated_at=now()
		RETURNING default_library_sort,preferred_view,theme,rating_scale`, userID, value.DefaultLibrarySort, value.PreferredView, value.Theme, value.RatingScale).Scan(&result.DefaultLibrarySort, &result.PreferredView, &result.Theme, &result.RatingScale)
	if err != nil {
		return Preferences{}, fmt.Errorf("persist library preferences: %w", err)
	}
	return result, nil
}

func ValidTheme(value string) bool {
	switch value {
	case "dark", "light", "system":
		return true
	default:
		return false
	}
}

func NormalizeTheme(value string) string {
	if value == "" {
		return "dark"
	}
	return value
}

func ValidRatingScale(value string) bool {
	switch value {
	case "1_10", "0_5", "minus5_plus5", "0_100":
		return true
	default:
		return false
	}
}

func NormalizeRatingScale(value string) string {
	if value == "" {
		return "1_10"
	}
	return value
}

func validSort(value string) bool {
	switch value {
	case "rating_desc", "rating_asc", "community_desc", "title_asc", "title_desc", "release_desc", "release_asc", "added_desc", "added_asc":
		return true
	default:
		return false
	}
}
