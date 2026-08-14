package setup

import "context"

// Store persists the singleton first-run state.
type Store interface {
	CompleteStatus(context.Context) (bool, error)
	Complete(context.Context) (bool, error)
}

// Service owns the one-time setup transition.
type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (service *Service) CompleteStatus(ctx context.Context) (bool, error) {
	return service.store.CompleteStatus(ctx)
}

// Complete returns true only for the single request that transitioned setup.
func (service *Service) Complete(ctx context.Context) (bool, error) {
	return service.store.Complete(ctx)
}
