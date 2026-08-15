package jellyfinsync

import (
	"context"
	"errors"
	"testing"

	"github.com/nzagler/gradeium/backend/internal/integrations/jellyfin"
	"github.com/nzagler/gradeium/backend/internal/media"
)

type fakeSource struct {
	items map[string][]jellyfin.Item
	err   map[string]error
}

func (source fakeSource) Items(_ context.Context, libraryID string, _ jellyfin.MediaType) ([]jellyfin.Item, error) {
	return source.items[libraryID], source.err[libraryID]
}

type fakeDomain struct{ tracked map[int64]bool }

func (domain fakeDomain) TrackedProviderIDs(_ context.Context, _ string, _ []int64) (map[int64]bool, error) {
	return domain.tracked, nil
}

func TestSyncIsAddOnlyUsesCanonicalIDsAndContinuesPerItem(t *testing.T) {
	source := fakeSource{items: map[string][]jellyfin.Item{
		"movies": {{Title: "Existing", ProviderID: 10}, {Title: "New", ProviderID: 11}, {Title: "Missing"}, {Title: "Fails", ProviderID: 12}},
		"tv":     {{Title: "Show", ProviderID: 20}},
	}, err: map[string]error{}}
	addedMovies := []int64{}
	addedTV := []int64{}
	service := NewService(
		fakeDomain{tracked: map[int64]bool{10: true}},
		fakeDomain{tracked: map[int64]bool{}},
		func(_ context.Context, _ string, id int64, status media.Status) error {
			if status != media.StatusBacklog {
				t.Fatalf("movie status = %q, want Backlog", status)
			}
			if id == 12 {
				return errors.New("TMDB unavailable")
			}
			addedMovies = append(addedMovies, id)
			return nil
		},
		func(_ context.Context, _ string, id int64, status media.Status) error {
			addedTV = append(addedTV, id)
			return nil
		},
	)
	result, err := service.Sync(context.Background(), "user", source, []jellyfin.LibraryMapping{{LibraryID: "movies", Domain: jellyfin.Movies}, {LibraryID: "tv", Domain: jellyfin.TVShows}})
	if err != nil {
		t.Fatal(err)
	}
	if len(addedMovies) != 1 || addedMovies[0] != 11 || len(addedTV) != 1 || addedTV[0] != 20 {
		t.Fatalf("added movies/TV = (%v, %v)", addedMovies, addedTV)
	}
	if result.Scanned != 5 || result.MoviesAdded != 1 || result.TVShowsAdded != 1 || result.AlreadyPresent != 1 || result.Skipped != 1 || result.Failed != 1 {
		t.Fatalf("sync result = %#v", result)
	}
}

func TestSyncContinuesAfterLibraryFailure(t *testing.T) {
	service := NewService(fakeDomain{tracked: map[int64]bool{}}, fakeDomain{tracked: map[int64]bool{}}, func(context.Context, string, int64, media.Status) error { return nil }, func(context.Context, string, int64, media.Status) error { return nil })
	result, err := service.Sync(context.Background(), "user", fakeSource{items: map[string][]jellyfin.Item{"ok": {{ProviderID: 1}}}, err: map[string]error{"bad": errors.New("offline")}}, []jellyfin.LibraryMapping{{LibraryID: "bad", Domain: jellyfin.Movies}, {LibraryID: "ok", Domain: jellyfin.TVShows}})
	if err != nil || result.Failed != 1 || result.TVShowsAdded != 1 {
		t.Fatalf("Sync() = (%#v, %v)", result, err)
	}
}
