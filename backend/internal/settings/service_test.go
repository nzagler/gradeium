package settings

import (
	"context"
	"encoding/json"
	"testing"
)

type memorySettingsStore struct {
	values map[string]json.RawMessage
}

func (store *memorySettingsStore) Values(context.Context) (map[string]json.RawMessage, error) {
	copyValues := make(map[string]json.RawMessage, len(store.values))
	for key, value := range store.values {
		copyValues[key] = append(json.RawMessage(nil), value...)
	}
	return copyValues, nil
}

func (store *memorySettingsStore) Upsert(_ context.Context, key string, value json.RawMessage) error {
	store.values[key] = append(json.RawMessage(nil), value...)
	return nil
}

func TestServiceAppliesDefaultAndPersistsUpdate(t *testing.T) {
	store := &memorySettingsStore{values: make(map[string]json.RawMessage)}
	service := NewService(NewRegistry(), store)
	values, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("List returned an error: %v", err)
	}
	if len(values) != 4 || string(values[0].Value) != `"Gradeium"` || values[0].Configured {
		t.Fatalf("default values = %#v", values)
	}

	updated, err := service.Update(context.Background(), InstanceNameKey, json.RawMessage(`"Home Media"`))
	if err != nil {
		t.Fatalf("Update returned an error: %v", err)
	}
	if string(updated) != `"Home Media"` {
		t.Fatalf("updated value = %s", updated)
	}
	values, err = service.List(context.Background())
	if err != nil {
		t.Fatalf("second List returned an error: %v", err)
	}
	if string(values[0].Value) != `"Home Media"` || !values[0].Configured {
		t.Fatalf("persisted values = %#v", values)
	}
}
