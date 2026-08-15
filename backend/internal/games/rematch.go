package games

import (
	"context"
	"errors"

	"github.com/nzagler/gradeium/backend/internal/integrations/igdb"
	"github.com/nzagler/gradeium/backend/internal/media"
)

var ErrProviderAlreadyExists = errors.New("IGDB game is already represented by another Gradeium entity")

type rematchStore interface {
	Rematch(context.Context, string, string, igdb.Game) (Detail, error)
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
	client, err := service.providers.IGDB(ctx)
	if err != nil {
		return Detail{}, media.ProviderError("igdb")
	}
	game, err := client.Game(ctx, providerID)
	if err != nil {
		if errors.Is(err, igdb.ErrNotTrackable) {
			return Detail{}, media.ValidationError(err.Error())
		}
		return Detail{}, media.ProviderError("igdb")
	}
	store, ok := service.store.(rematchStore)
	if !ok {
		return Detail{}, errors.New("game store does not support rematching")
	}
	detail, err := store.Rematch(ctx, userID, id, game)
	if errors.Is(err, ErrProviderAlreadyExists) {
		return Detail{}, &media.SafeError{Code: "already_tracked", Message: "That IGDB game is already represented by another Gradeium item. Remove the duplicate first."}
	}
	return detail, err
}
