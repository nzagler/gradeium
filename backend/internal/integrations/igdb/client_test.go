package igdb

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nzagler/gradeium/backend/internal/integrations/provider"
)

func TestTrackableGameTypeNormalization(t *testing.T) {
	for _, value := range []string{"Main Game", "standalone_expansion", "Remake", "remaster", "Expanded Game"} {
		if !TrackableGameType(value) {
			t.Fatalf("expected %q to be trackable", value)
		}
	}
	for _, value := range []string{"DLC Addon", "Expansion", "Bundle", "Port", "Pack", "Update"} {
		if TrackableGameType(value) {
			t.Fatalf("expected %q to be nested/non-trackable", value)
		}
	}
}

func TestGameMapsCurrentGameTypeAndCommunityRating(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			_, _ = w.Write([]byte(`{"access_token":"token-value","expires_in":3600}`))
		case "/games":
			if got := r.Header.Get("Authorization"); got != "Bearer token-value" {
				t.Fatalf("unexpected auth header %q", got)
			}
			_, _ = w.Write([]byte(`[{"id":42,"name":"Example","summary":"Overview","first_release_date":946684800,"rating":87.45,"rating_count":321,"game_type":{"type":"Remake"},"cover":{"id":8,"image_id":"cover42","width":600,"height":800},"dlcs":[{"id":43,"name":"Bonus","game_type":{"type":"DLC Addon"}}]}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewClientWithEndpoints(provider.NewClient(), "client-id", "secret-value", server.URL+"/token", server.URL)
	game, err := client.Game(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if game.GameType != "Remake" || game.CommunityRating == nil || *game.CommunityRating != 87 {
		t.Fatalf("unexpected game: %#v", game)
	}
	if len(game.AdditionalContent) != 1 || game.AdditionalContent[0].ProviderID != 43 {
		t.Fatalf("unexpected additional content: %#v", game.AdditionalContent)
	}
}

func TestSearchEscapesApicalypseInput(t *testing.T) {
	gotBody := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_, _ = w.Write([]byte(`{"access_token":"token","expires_in":3600}`))
			return
		}
		payload, _ := io.ReadAll(r.Body)
		gotBody = string(payload)
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()
	client := NewClientWithEndpoints(provider.NewClient(), "id", "secret", server.URL+"/token", server.URL)
	if _, err := client.Search(context.Background(), `Halo"; limit 500;`, 1); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(gotBody, `search "Halo";`) {
		t.Fatalf("query injection was not escaped: %s", gotBody)
	}
}
