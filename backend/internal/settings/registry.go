package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	InstanceNameKey               = "general.instance_name"
	FutureAuthenticationSecretKey = "authentication.client_secret"
)

// ValueType is the JSON type accepted for a setting.
type ValueType string

const (
	ValueTypeString ValueType = "string"
)

// Sensitivity controls whether a value may be returned through the API.
type Sensitivity string

const (
	SensitivityPublic Sensitivity = "public"
	SensitivitySecret Sensitivity = "secret"
)

// Definition is the central allowlist and display metadata for one setting.
type Definition struct {
	Key         string
	Section     string
	Label       string
	Description string
	Type        ValueType
	Sensitivity Sensitivity
	Default     json.RawMessage
	Reserved    bool
}

// Registry owns every setting key accepted by the backend.
type Registry struct {
	ordered []Definition
	byKey   map[string]Definition
}

// NewRegistry returns the deliberately small Phase 2 settings allowlist.
func NewRegistry() *Registry {
	registry, err := NewRegistryWithDefinitions([]Definition{
		{
			Key:         InstanceNameKey,
			Section:     "general",
			Label:       "Instance name",
			Description: "The name shown in Gradeium's application shell.",
			Type:        ValueTypeString,
			Sensitivity: SensitivityPublic,
			Default:     json.RawMessage(`"Gradeium"`),
		},
		{
			Key:         FutureAuthenticationSecretKey,
			Section:     "authentication",
			Label:       "Authentication client secret",
			Description: "Reserved for the future external authentication phase.",
			Type:        ValueTypeString,
			Sensitivity: SensitivitySecret,
			Reserved:    true,
		},
	})
	if err != nil {
		panic(err)
	}
	return registry
}

// NewRegistryWithDefinitions is useful for focused service tests.
func NewRegistryWithDefinitions(definitions []Definition) (*Registry, error) {
	registry := &Registry{
		ordered: make([]Definition, 0, len(definitions)),
		byKey:   make(map[string]Definition, len(definitions)),
	}
	for _, definition := range definitions {
		if strings.TrimSpace(definition.Key) == "" {
			return nil, errors.New("setting definition key must not be empty")
		}
		if _, exists := registry.byKey[definition.Key]; exists {
			return nil, fmt.Errorf("duplicate setting definition %q", definition.Key)
		}
		if definition.Type != ValueTypeString {
			return nil, fmt.Errorf("unsupported value type for %q", definition.Key)
		}
		if definition.Sensitivity != SensitivityPublic && definition.Sensitivity != SensitivitySecret {
			return nil, fmt.Errorf("unsupported sensitivity for %q", definition.Key)
		}
		registry.ordered = append(registry.ordered, definition)
		registry.byKey[definition.Key] = definition
	}
	return registry, nil
}

// Definitions returns a copy in stable display order.
func (registry *Registry) Definitions() []Definition {
	return append([]Definition(nil), registry.ordered...)
}

// Definition returns an allowed setting definition.
func (registry *Registry) Definition(key string) (Definition, bool) {
	definition, ok := registry.byKey[key]
	return definition, ok
}

// ValidateSetting validates and canonicalizes a non-secret JSON value.
func (registry *Registry) ValidateSetting(key string, value json.RawMessage) (json.RawMessage, error) {
	definition, ok := registry.byKey[key]
	if !ok || definition.Sensitivity != SensitivityPublic {
		return nil, errors.New("setting key is not allowed")
	}

	switch definition.Type {
	case ValueTypeString:
		var decoded string
		if err := json.Unmarshal(value, &decoded); err != nil {
			return nil, errors.New("value must be a JSON string")
		}
		decoded = strings.TrimSpace(decoded)
		if decoded == "" {
			return nil, errors.New("value must not be empty")
		}
		if utf8.RuneCountInString(decoded) > 80 {
			return nil, errors.New("value must be at most 80 characters")
		}
		canonical, err := json.Marshal(decoded)
		if err != nil {
			return nil, errors.New("encode setting value")
		}
		return canonical, nil
	default:
		return nil, errors.New("setting type is not supported")
	}
}

// AllowsSecret reports whether key is an allowed secret setting.
func (registry *Registry) AllowsSecret(key string) bool {
	definition, ok := registry.byKey[key]
	return ok && definition.Sensitivity == SensitivitySecret
}

// ValidateSecret validates a plaintext secret without transforming it.
func (registry *Registry) ValidateSecret(key, value string) error {
	if !registry.AllowsSecret(key) {
		return errors.New("secret key is not allowed")
	}
	if value == "" {
		return errors.New("secret value must not be empty")
	}
	if len(value) > 16*1024 {
		return errors.New("secret value is too large")
	}
	if !utf8.ValidString(value) {
		return errors.New("secret value must be valid UTF-8")
	}
	return nil
}
