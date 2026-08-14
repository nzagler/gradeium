package tmdb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nzagler/gradeium/backend/internal/integrations/provider"
)

func TestVerifyTVDBMappingRequiresExactReverseID(t *testing.T) {
	for _, test := range []struct {
		name    string
		reverse int64
		want    bool
	}{{"verified", 81189, true}, {"reverse mismatch", 999, false}} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/find/81189":
					_, _ = w.Write([]byte(`{"tv_results":[{"id":1396}]}`))
				case "/tv/1396/external_ids":
					_, _ = w.Write([]byte(`{"tvdb_id":` + format(test.reverse) + `}`))
				case "/tv/1396":
					_, _ = w.Write([]byte(`{"id":1396,"vote_average":8.75,"vote_count":3000}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			client := NewClientWithEndpoint(provider.NewClient(), "read-token", server.URL)
			mapping, err := client.VerifyTVDBMapping(context.Background(), 81189)
			if err != nil {
				t.Fatal(err)
			}
			if (mapping != nil) != test.want {
				t.Fatalf("unexpected mapping: %#v", mapping)
			}
			if mapping != nil && (mapping.CommunityRating == nil || *mapping.CommunityRating != 88) {
				t.Fatalf("unexpected rating: %#v", mapping)
			}
		})
	}
}

func TestVerifyTVDBMappingRejectsAmbiguousFind(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tv_results":[{"id":1},{"id":2}]}`))
	}))
	defer server.Close()
	client := NewClientWithEndpoint(provider.NewClient(), "token", server.URL)
	mapping, err := client.VerifyTVDBMapping(context.Background(), 44)
	if err != nil {
		t.Fatal(err)
	}
	if mapping != nil {
		t.Fatalf("ambiguous mapping must be omitted: %#v", mapping)
	}
}

func TestMovieParsesAndOrdersCollectionMembers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer read-token" {
			t.Fatalf("unexpected authorization header %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/movie/11":
			_, _ = w.Write([]byte(`{"id":11,"title":"Second","release_date":"2022-03-04","runtime":120,"vote_average":7.94,"vote_count":45,"belongs_to_collection":{"id":77,"name":"Fixture Collection"},"credits":{"cast":[],"crew":[]},"images":{"posters":[],"backdrops":[],"logos":[]},"videos":{"results":[]}}`))
		case "/collection/77":
			_, _ = w.Write([]byte(`{"parts":[{"id":11,"title":"Second","release_date":"2022-03-04","poster_path":"/second.jpg"},{"id":10,"title":"First","release_date":"2020-01-02","poster_path":"/first.jpg"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewClientWithEndpoint(provider.NewClient(), "read-token", server.URL)
	movie, err := client.Movie(context.Background(), 11)
	if err != nil {
		t.Fatal(err)
	}
	if movie.CollectionID == nil || *movie.CollectionID != 77 || movie.CollectionName != "Fixture Collection" || len(movie.Collection) != 2 {
		t.Fatalf("unexpected collection: %#v", movie)
	}
	if movie.Collection[0].ProviderID != 10 || movie.Collection[1].ProviderID != 11 {
		t.Fatalf("collection was not release-date ordered: %#v", movie.Collection)
	}
	if movie.CommunityRating == nil || *movie.CommunityRating != 79 || movie.CommunityRatingCount == nil || *movie.CommunityRatingCount != 45 {
		t.Fatalf("unexpected movie community rating: %#v", movie)
	}
}

func format(value int64) string {
	if value == 0 {
		return "0"
	}
	result := ""
	for value > 0 {
		result = string(rune('0'+value%10)) + result
		value /= 10
	}
	return result
}
