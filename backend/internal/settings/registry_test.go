package settings

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRegistryValidatesAndCanonicalizesSettings(t *testing.T) {
	registry := NewRegistry()
	value, err := registry.ValidateSetting(InstanceNameKey, json.RawMessage(`"  My Gradeium  "`))
	if err != nil {
		t.Fatalf("ValidateSetting returned an error: %v", err)
	}
	if string(value) != `"My Gradeium"` {
		t.Fatalf("canonical value = %s", value)
	}

	tests := []struct {
		name  string
		key   string
		value json.RawMessage
	}{
		{name: "unknown key", key: "unknown", value: json.RawMessage(`"value"`)},
		{name: "secret through public path", key: FutureAuthenticationSecretKey, value: json.RawMessage(`"value"`)},
		{name: "wrong type", key: InstanceNameKey, value: json.RawMessage(`true`)},
		{name: "empty", key: InstanceNameKey, value: json.RawMessage(`"  "`)},
		{name: "too long", key: InstanceNameKey, value: json.RawMessage(`"` + strings.Repeat("a", 81) + `"`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := registry.ValidateSetting(test.key, test.value); err == nil {
				t.Fatal("ValidateSetting unexpectedly accepted invalid input")
			}
		})
	}
}

func TestRegistrySeparatesSecretKeys(t *testing.T) {
	registry := NewRegistry()
	if !registry.AllowsSecret(FutureAuthenticationSecretKey) {
		t.Fatal("registered future authentication secret was not allowed")
	}
	if registry.AllowsSecret(InstanceNameKey) {
		t.Fatal("public setting was accepted as a secret")
	}
	if err := registry.ValidateSecret(FutureAuthenticationSecretKey, ""); err == nil {
		t.Fatal("empty secret was accepted")
	}
}
