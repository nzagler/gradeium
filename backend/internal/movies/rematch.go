package movies

import (
	"context"
	"errors"

	"github.com/nzagler/gradeium/backend/internal/integrations/tmdb"
	"github.com/nzagler/gradeium/backend/internal/media"
)

var ErrProviderAlreadyExists = errors.New("TMDB movie is already represented by another Gradeium entity")

type rematchStore interface {
	Rematch(context.Context, string, string, tmdb.Movie) (Detail, error)
}

func (service *Service) Rematch(ctx context.Context, userID, id string, providerID int64) (Detail, error) {
	if err := media.ValidateProviderID(providerID); err != nil {
		return Detail{}, media.ValidationError(err.Error())
	}
	current, err := service.store.Detail(ctx, userID, id)
	if err != nil {
		return Detail{}, err
	}
	if current.ProviderID == providerID {
		return service.Refresh(ctx, userID, id)
	}
	client, err := service.providers.TMDB(ctx)
	if err != nil {
		return Detail{}, media.ProviderError("tmdb")
	}
	movie, err := client.Movie(ctx, providerID)
	if err != nil {
		return Detail{}, media.ProviderError("tmdb")
	}
	store, ok := service.store.(rematchStore)
	if !ok {
		return Detail{}, errors.New("movie store does not support rematching")
	}
	detail, err := store.Rematch(ctx, userID, id, movie)
	if errors.Is(err, ErrProviderAlreadyExists) {
		return Detail{}, &media.SafeError{Code: "already_tracked", Message: "That TMDB movie is already represented by another Gradeium item. Remove the duplicate first."}
	}
	return detail, err
}
