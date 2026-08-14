package settings

import (
	"context"
	"encoding/json"
)

// Store persists validated non-secret setting values.
type Store interface {
	Values(context.Context) (map[string]json.RawMessage, error)
	Upsert(context.Context, string, json.RawMessage) error
}

// Value is a safe non-secret setting returned to the API layer.
type Value struct {
	Definition Definition
	Value      json.RawMessage
	Configured bool
}

// Service validates settings against the registry before persistence.
type Service struct {
	registry *Registry
	store    Store
}

func NewService(registry *Registry, store Store) *Service {
	return &Service{registry: registry, store: store}
}

// List returns only non-secret values, applying registered defaults.
func (service *Service) List(ctx context.Context) ([]Value, error) {
	stored, err := service.store.Values(ctx)
	if err != nil {
		return nil, err
	}

	values := make([]Value, 0, len(stored))
	for _, definition := range service.registry.Definitions() {
		if definition.Sensitivity != SensitivityPublic {
			continue
		}
		value, configured := stored[definition.Key]
		if !configured {
			value = append(json.RawMessage(nil), definition.Default...)
		}
		values = append(values, Value{
			Definition: definition,
			Value:      append(json.RawMessage(nil), value...),
			Configured: configured,
		})
	}
	return values, nil
}

// Update validates, canonicalizes, and atomically replaces one setting.
func (service *Service) Update(ctx context.Context, key string, value json.RawMessage) (json.RawMessage, error) {
	canonical, err := service.registry.ValidateSetting(key, value)
	if err != nil {
		return nil, err
	}
	if err := service.store.Upsert(ctx, key, canonical); err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), canonical...), nil
}
