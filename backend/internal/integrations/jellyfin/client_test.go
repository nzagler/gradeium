package jellyfin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func TestClientDiscoversLibrariesAndKeepsAPIKeyOutOfURL(t *testing.T) {
	const apiKey = "private-api-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.URL.String(), apiKey) || request.Header.Get("X-Emby-Token") != apiKey {
			t.Fatalf("API key placement was unsafe: url=%q header=%q", request.URL.String(), request.Header.Get("X-Emby-Token"))
		}
		_ = json.NewEncoder(w).Encode([]map[string]string{{"Name": "Films", "CollectionType": "movies", "ItemId": "library-1"}})
	}))
	defer server.Close()
	client, err := NewClient(server.URL, apiKey)
	if err != nil {
		t.Fatal(err)
	}
	libraries, err := client.Libraries(context.Background())
	if err != nil || len(libraries) != 1 || libraries[0].ID != "library-1" {
		t.Fatalf("Libraries() = (%#v, %v)", libraries, err)
	}
}

func TestClientPaginatesItemsAndUsesCanonicalProviderIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		start, _ := strconv.Atoi(request.URL.Query().Get("StartIndex"))
		items := []map[string]any{}
		limit := 100
		if start == 100 {
			limit = 1
		}
		for index := 0; index < limit; index++ {
			id := start + index + 1
			items = append(items, map[string]any{"Id": strconv.Itoa(id), "Name": "Movie " + strconv.Itoa(id), "ProviderIds": map[string]string{"TMDB": strconv.Itoa(1000 + id)}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"Items": items, "TotalRecordCount": 101})
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "key")
	if err != nil {
		t.Fatal(err)
	}
	items, err := client.Items(context.Background(), "library", Movies)
	if err != nil || len(items) != 101 || items[100].ProviderID != 1101 {
		t.Fatalf("Items() count/provider = (%d, %v, %v)", len(items), items[len(items)-1].ProviderID, err)
	}
}

func TestClientRejectsUnsafeURLsAndRedirects(t *testing.T) {
	for _, value := range []string{"ftp://example.com", "https://user:pass@example.com", "https://example.com?token=x", "not a URL"} {
		if _, err := NormalizeBaseURL(value); err == nil {
			t.Errorf("NormalizeBaseURL(%q) accepted unsafe URL", value)
		}
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("redirect target received the Jellyfin credential")
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", target.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	client, err := NewClient(redirect.URL, "key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Libraries(context.Background()); err == nil {
		t.Fatal("Libraries followed a redirect")
	}
	parsed, _ := url.Parse(redirect.URL)
	if parsed.Scheme != "http" {
		t.Fatal("focused test server was not HTTP")
	}
}
