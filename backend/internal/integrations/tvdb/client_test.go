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
			_, _ = w.Write([]byte(`{"status":"success","data":{"id":100,"name":"Example Show","firstAired":"2020-01-01","status":{"name":"Continuing"},"genres":[{"name":"Drama"}],"seasons":[{"id":10,"number":0,"name":"Specials","type":{"type":"Aired Order"}},{"id":11,"number":1,"name":"Season 1","type":{"type":"Aired Order"}}]}}`))
		case "/series/100/episodes/default/eng":
			_, _ = w.Write([]byte(`{"status":"success","data":{"episodes":[{"id":1,"seasonNumber":0,"number":1,"name":"Special"},{"id":2,"seasonNumber":1,"number":1,"name":"Pilot","runtime":45}]},"links":{"next":null}}`))
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
}

func TestSearchOmitsInvalidProviderIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			_, _ = w.Write([]byte(`{"data":{"token":"token"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"tvdb_id":"bad","name":"Bad"},{"tvdb_id":"44","name":"Good","year":"2022"}],"links":{"next":null}}`))
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
}
