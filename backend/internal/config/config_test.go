package config

import (
	"strings"
	"testing"
)

func TestLoadFromRequiresDatabaseURL(t *testing.T) {
	_, err := LoadFrom(func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), envDatabaseURL) {
		t.Fatalf("expected missing %s error, got %v", envDatabaseURL, err)
	}
}

func TestLoadFromRejectsInvalidDatabaseURLWithoutEchoingIt(t *testing.T) {
	secretValue := "not-a-connection-string-with-secret"
	_, err := LoadFrom(mapEnvironment(map[string]string{envDatabaseURL: secretValue}))
	if err == nil {
		t.Fatal("expected invalid database URL error")
	}
	if strings.Contains(err.Error(), secretValue) {
		t.Fatal("configuration error exposed the configured value")
	}
}

func TestLoadFromAppliesSafeDefaults(t *testing.T) {
	cfg, err := LoadFrom(mapEnvironment(map[string]string{
		envDatabaseURL: "postgres://gradeium:password@localhost:5432/gradeium?sslmode=disable",
	}))
	if err != nil {
		t.Fatalf("LoadFrom returned an error: %v", err)
	}
	if cfg.ListenAddress != defaultListen {
		t.Fatalf("ListenAddress = %q, want %q", cfg.ListenAddress, defaultListen)
	}
	if cfg.ConfigDir != defaultConfig || cfg.BackupsDir != defaultBackups {
		t.Fatalf("unexpected persistent path defaults: config=%q backups=%q", cfg.ConfigDir, cfg.BackupsDir)
	}
	if strings.ToLower(cfg.LogLevel.String()) != defaultLogLevel {
		t.Fatalf("LogLevel = %q, want %q", cfg.LogLevel.String(), defaultLogLevel)
	}
}

func TestLoadFromRejectsInvalidListenAddress(t *testing.T) {
	_, err := LoadFrom(mapEnvironment(map[string]string{
		envDatabaseURL: "postgres://gradeium:password@localhost:5432/gradeium?sslmode=disable",
		envListen:      "8080",
	}))
	if err == nil || !strings.Contains(err.Error(), envListen) {
		t.Fatalf("expected invalid %s error, got %v", envListen, err)
	}
}

func mapEnvironment(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}
