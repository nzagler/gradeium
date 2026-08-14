package auth

import "testing"

func TestNormalizeConfiguration(t *testing.T) {
	configuration, err := NormalizeConfiguration(ConfigurationInput{
		IssuerURL: "https://ID.Example/oidc/", ClientID: " gradeium ", PublicURL: "https://MEDIA.Example/",
	})
	if err != nil {
		t.Fatalf("NormalizeConfiguration returned an error: %v", err)
	}
	if configuration.IssuerURL != "https://id.example/oidc/" || configuration.ClientID != "gradeium" || configuration.PublicURL != "https://media.example" {
		t.Fatalf("normalized configuration = %#v", configuration)
	}
	if configuration.RedirectURI() != "https://media.example/api/auth/callback" {
		t.Fatalf("redirect URI = %q", configuration.RedirectURI())
	}

	tests := []ConfigurationInput{
		{IssuerURL: "http://id.example", ClientID: "client", PublicURL: "https://gradeium.example"},
		{IssuerURL: "https://id.example?tenant=x", ClientID: "client", PublicURL: "https://gradeium.example"},
		{IssuerURL: "https://id.example", ClientID: "", PublicURL: "https://gradeium.example"},
		{IssuerURL: "https://id.example", ClientID: "client", PublicURL: "http://gradeium.example"},
		{IssuerURL: "https://id.example", ClientID: "client", PublicURL: "https://gradeium.example/subpath"},
	}
	for index, input := range tests {
		if _, err := NormalizeConfiguration(input); err == nil {
			t.Fatalf("invalid configuration %d was accepted", index)
		}
	}
	if _, err := NormalizeConfiguration(ConfigurationInput{
		IssuerURL: "http://127.0.0.1:8081", ClientID: "client", PublicURL: "http://localhost:8080",
	}); err != nil {
		t.Fatalf("loopback development URLs were rejected: %v", err)
	}
}

func TestSafeReturnPathRejectsOpenRedirects(t *testing.T) {
	tests := map[string]string{
		"":                           "/",
		"/settings/authentication":   "/settings/authentication",
		"/settings?section=auth":     "/settings?section=auth",
		"https://evil.example/steal": "/",
		"//evil.example/steal":       "/",
		"/\\evil.example":            "/",
		"/%5cevil.example":           "/",
		"/%2f%2fevil.example":        "/",
		"/safe/../../settings":       "/settings",
		"/safe#https://evil.example": "/",
	}
	for input, expected := range tests {
		if actual := SafeReturnPath(input); actual != expected {
			t.Errorf("SafeReturnPath(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestSameOrigin(t *testing.T) {
	if !SameOrigin("https://gradeium.example", "https://gradeium.example") {
		t.Fatal("matching origin was rejected")
	}
	if SameOrigin("https://gradeium.example", "https://evil.example") {
		t.Fatal("cross-origin request was accepted")
	}
	for _, malformed := range []string{"https://gradeium.example/", "https://user@gradeium.example", "https://gradeium.example?unexpected=1", "null"} {
		if SameOrigin("https://gradeium.example", malformed) {
			t.Fatalf("malformed origin %q was accepted", malformed)
		}
	}
	if !SameOrigin("https://gradeium.example", "") {
		t.Fatal("missing Origin was rejected despite separate CSRF token protection")
	}
}
