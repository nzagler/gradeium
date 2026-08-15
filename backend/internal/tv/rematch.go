package tv

import (
	"context"
	"errors"

	"github.com/nzagler/gradeium/backend/internal/integrations/tmdb"
	"github.com/nzagler/gradeium/backend/internal/integrations/tvdb"
	"github.com/nzagler/gradeium/backend/internal/media"
)

var ErrProviderAlreadyExists = errors.New("TVDB show is already represented by another Gradeium entity")

type rematchStore interface {
	Rematch(context.Context, string, string, tvdb.Show, *tmdb.VerifiedTV) (Detail, error)
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
	show, mapping, err := service.metadata(ctx, providerID)
	if err != nil {
		return Detail{}, err
	}
	store, ok := service.store.(rematchStore)
	if !ok {
		return Detail{}, errors.New("TV store does not support rematching")
	}
	detail, err := store.Rematch(ctx, userID, id, show, mapping)
	if errors.Is(err, ErrProviderAlreadyExists) {
		return Detail{}, &media.SafeError{Code: "already_tracked", Message: "That TVDB show is already represented by another Gradeium item. Remove the duplicate first."}
	}
	return detail, err
}
