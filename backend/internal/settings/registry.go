package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	InstanceNameKey                     = "general.instance_name"
	AuthenticationIssuerURLKey          = "authentication.issuer_url"
	AuthenticationClientIDKey           = "authentication.client_id"
	AuthenticationClientSecretKey       = "authentication.client_secret"
	AuthenticationPublicURLKey          = "authentication.public_url"
	AuthenticationActiveClientSecretKey = "authentication.active_client_secret"

	// FutureAuthenticationSecretKey is retained as a source-compatible alias
	// for the Phase 2 reserved key.
	FutureAuthenticationSecretKey = AuthenticationClientSecretKey
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
	Internal    bool
	MaxLength   int
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
			MaxLength:   80,
		},
		{
			Key:         AuthenticationIssuerURLKey,
			Section:     "authentication",
			Label:       "Issuer URL",
			Description: "The exact generic OIDC issuer used for discovery and token verification.",
			Type:        ValueTypeString,
			Sensitivity: SensitivityPublic,
			MaxLength:   2048,
		},
		{
			Key:         AuthenticationClientIDKey,
			Section:     "authentication",
			Label:       "Client ID",
			Description: "The OIDC client identifier registered for this Gradeium instance.",
			Type:        ValueTypeString,
			Sensitivity: SensitivityPublic,
			MaxLength:   512,
		},
		{
			Key:         AuthenticationClientSecretKey,
			Section:     "authentication",
			Label:       "Authentication client secret",
			Description: "The encrypted OIDC client secret. Saved values are never displayed again.",
			Type:        ValueTypeString,
			Sensitivity: SensitivitySecret,
		},
		{
			Key:         AuthenticationPublicURLKey,
			Section:     "authentication",
			Label:       "Public Gradeium URL",
			Description: "The external base URL used to construct the exact OIDC callback URI.",
			Type:        ValueTypeString,
			Sensitivity: SensitivityPublic,
			MaxLength:   2048,
		},
		{
			Key:         AuthenticationActiveClientSecretKey,
			Section:     "authentication",
			Label:       "Active authentication client secret",
			Description: "Internal last-known-good OIDC secret used by the active login configuration.",
			Type:        ValueTypeString,
			Sensitivity: SensitivitySecret,
			Internal:    true,
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
		if definition.MaxLength < 0 {
			return nil, fmt.Errorf("invalid maximum length for %q", definition.Key)
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
		maximum := definition.MaxLength
		if maximum == 0 {
			maximum = 80
		}
		if utf8.RuneCountInString(decoded) > maximum {
			return nil, fmt.Errorf("value must be at most %d characters", maximum)
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
