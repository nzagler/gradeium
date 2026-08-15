package tvdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nzagler/gradeium/backend/internal/integrations/provider"
)

func TestShowUsesDefaultAiredOrderAndClassifiesSpecials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			_, _ = w.Write([]byte(`{"status":"success","data":{"token":"tvdb-token"}}`))
		case "/series/100/extended":
			if got := r.URL.Query().Get("short"); got != "" {
				t.Errorf("series extended request unexpectedly set short=%q", got)
			}
			if got := r.URL.Query().Get("meta"); got != "translations" {
				t.Errorf("series extended request meta=%q, want translations", got)
			}
			_, _ = w.Write([]byte(`{"status":"success","data":{"id":100,"name":"Example Show","firstAired":"2020-01-01","status":{"name":"Continuing"},"genres":[{"name":"Drama"}],"artworks":[{"id":20,"type":2,"image":"/banners/v4/series/100/posters/poster.jpg","thumbnail":"http://artworks.thetvdb.com/banners/v4/series/100/posters/poster-thumb.jpg"}],"characters":[{"name":"Lead","personName":"Actor","peopleType":"Actor","image":"banners/v4/actor/200/photo/person.jpg"}],"seasons":[{"id":10,"number":0,"name":"Specials","type":{"type":"Aired Order"}},{"id":11,"number":1,"name":"Season 1","image":"//artworks.thetvdb.com/banners/v4/series/100/seasons/1.jpg","type":{"type":"Aired Order"}}]}}`))
		case "/series/100/episodes/default/eng":
			_, _ = w.Write([]byte(`{"status":"success","data":{"episodes":[{"id":1,"seasonNumber":0,"number":1,"name":"Special"},{"id":2,"seasonNumber":1,"number":1,"name":"Pilot","runtime":45,"image":"http://www.thetvdb.com/banners/episodes/100/2.jpg"}]},"links":{"next":null}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewClientWithEndpoint(provider.NewClient(), "api-key", "", server.URL)
	show, err := client.Show(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(show.Seasons) != 2 || !show.Seasons[0].Special || show.Seasons[1].Special {
		t.Fatalf("unexpected seasons: %#v", show.Seasons)
	}
	if !show.Seasons[0].Episodes[0].Special || show.Seasons[1].Episodes[0].Special {
		t.Fatalf("unexpected episode classification: %#v", show.Seasons)
	}
	if len(show.Artworks) != 1 || show.Artworks[0].ImageURL != "https://artworks.thetvdb.com/banners/v4/series/100/posters/poster.jpg" || show.Artworks[0].ThumbnailURL != "https://artworks.thetvdb.com/banners/v4/series/100/posters/poster-thumb.jpg" {
		t.Fatalf("unexpected normalized artwork: %#v", show.Artworks)
	}
	if len(show.Cast) != 1 || show.Cast[0].ImageURL != "https://artworks.thetvdb.com/banners/v4/actor/200/photo/person.jpg" {
		t.Fatalf("unexpected normalized cast image: %#v", show.Cast)
	}
	if show.Seasons[1].PosterURL != "https://artworks.thetvdb.com/banners/v4/series/100/seasons/1.jpg" || show.Seasons[1].Episodes[0].StillURL != "https://artworks.thetvdb.com/banners/episodes/100/2.jpg" {
		t.Fatalf("unexpected normalized season or episode image: %#v", show.Seasons[1])
	}
}

func TestSearchOmitsInvalidProviderIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			_, _ = w.Write([]byte(`{"data":{"token":"token"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"tvdb_id":"bad","name":"Bad"},{"tvdb_id":"44","name":"Good","year":"2022","image_url":"/banners/v4/series/44/posters/search.jpg"}],"links":{"next":null}}`))
	}))
	defer server.Close()
	client := NewClientWithEndpoint(provider.NewClient(), "key", "", server.URL)
	page, err := client.Search(context.Background(), "good", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Results) != 1 || page.Results[0].ProviderID != 44 {
		t.Fatalf("unexpected results: %#v", page.Results)
	}
	if page.Results[0].ArtworkURL != "https://artworks.thetvdb.com/banners/v4/series/44/posters/search.jpg" {
		t.Fatalf("unexpected normalized search artwork: %q", page.Results[0].ArtworkURL)
	}
}

func TestSafeImageNormalizesOnlyTrustedTVDBArtwork(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "absolute HTTPS", value: "https://artworks.thetvdb.com/banners/posters/100-1.jpg", want: "https://artworks.thetvdb.com/banners/posters/100-1.jpg"},
		{name: "legacy HTTP CDN", value: "http://artworks.thetvdb.com/banners/posters/100-1.jpg", want: "https://artworks.thetvdb.com/banners/posters/100-1.jpg"},
		{name: "legacy TVDB host", value: "http://www.thetvdb.com/banners/posters/100-1.jpg", want: "https://artworks.thetvdb.com/banners/posters/100-1.jpg"},
		{name: "protocol relative", value: "//artworks.thetvdb.com/banners/posters/100-1.jpg", want: "https://artworks.thetvdb.com/banners/posters/100-1.jpg"},
		{name: "root relative", value: "/banners/v4/series/100/posters/one.jpg", want: "https://artworks.thetvdb.com/banners/v4/series/100/posters/one.jpg"},
		{name: "path relative", value: " banners/v4/series/100/posters/one.jpg ", want: "https://artworks.thetvdb.com/banners/v4/series/100/posters/one.jpg"},
		{name: "arbitrary HTTP", value: "http://images.example/poster.jpg"},
		{name: "arbitrary HTTPS", value: "https://images.example/poster.jpg"},
		{name: "spoofed TVDB host", value: "https://artworks.thetvdb.com.example/banners/poster.jpg"},
		{name: "credentials", value: "https://user@artworks.thetvdb.com/banners/poster.jpg"},
		{name: "custom port", value: "https://artworks.thetvdb.com:444/banners/poster.jpg"},
		{name: "non artwork path", value: "/images/poster.jpg"},
		{name: "path traversal", value: "/banners/../private/poster.jpg"},
		{name: "encoded path traversal", value: "/banners/%2e%2e/private/poster.jpg"},
		{name: "javascript URL", value: "javascript:alert(1)"},
		{name: "malformed URL", value: "https://artworks.thetvdb.com/%zz"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := safeImage(test.value); got != test.want {
				t.Fatalf("safeImage(%q) = %q, want %q", test.value, got, test.want)
			}
	}
}
