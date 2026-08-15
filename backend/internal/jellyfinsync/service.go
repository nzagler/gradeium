package jellyfinsync

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/nzagler/gradeium/backend/internal/integrations/jellyfin"
	"github.com/nzagler/gradeium/backend/internal/media"
)

type Source interface {
	Items(context.Context, string, jellyfin.MediaType) ([]jellyfin.Item, error)
}

type adder interface {
	TrackedProviderIDs(context.Context, string, []int64) (map[int64]bool, error)
}

type addMovie func(context.Context, string, int64, media.Status) error

type Service struct {
	movies   adder
	tv       adder
	addMovie addMovie
	addTV    addMovie
}

func NewService(
	movies adder,
	tv adder,
	addMovie addMovie,
	addTV addMovie,
) *Service {
	return &Service{movies: movies, tv: tv, addMovie: addMovie, addTV: addTV}
}

type Issue struct {
	LibraryID string `json:"libraryId,omitempty"`
	Title     string `json:"title,omitempty"`
	Reason    string `json:"reason"`
}

type Result struct {
	Scanned        int     `json:"scanned"`
	MoviesAdded    int     `json:"moviesAdded"`
	TVShowsAdded   int     `json:"tvShowsAdded"`
	AlreadyPresent int     `json:"alreadyPresent"`
	Skipped        int     `json:"skipped"`
	Failed         int     `json:"failed"`
	Issues         []Issue `json:"issues"`
}

type candidate struct {
	item   jellyfin.Item
	domain jellyfin.MediaType
}

func (service *Service) Sync(ctx context.Context, userID string, source Source, mappings []jellyfin.LibraryMapping) (Result, error) {
	result := Result{Issues: []Issue{}}
	candidates := map[jellyfin.MediaType]map[int64]candidate{
		jellyfin.Movies:  {},
		jellyfin.TVShows: {},
	}
	for _, mapping := range mappings {
		items, err := source.Items(ctx, mapping.LibraryID, mapping.Domain)
		if err != nil {
			result.Failed++
			result.Issues = append(result.Issues, Issue{LibraryID: mapping.LibraryID, Reason: "Library could not be read; other libraries continued."})
			continue
		}
		for _, item := range items {
			result.Scanned++
			if item.ProviderID <= 0 {
				result.Skipped++
				result.Issues = append(result.Issues, Issue{LibraryID: mapping.LibraryID, Title: item.Title, Reason: missingProviderReason(mapping.Domain)})
				continue
			}
			if _, exists := candidates[mapping.Domain][item.ProviderID]; exists {
				result.Skipped++
				result.Issues = append(result.Issues, Issue{LibraryID: mapping.LibraryID, Title: item.Title, Reason: "Duplicate canonical provider ID in the mapped Jellyfin libraries."})
				continue
			}
			candidates[mapping.Domain][item.ProviderID] = candidate{item: item, domain: mapping.Domain}
		}
	}
	if err := service.addCandidates(ctx, userID, &result, candidates[jellyfin.Movies], service.movies, service.addMovie); err != nil {
		return Result{}, err
	}
	if err := service.addCandidates(ctx, userID, &result, candidates[jellyfin.TVShows], service.tv, service.addTV); err != nil {
		return Result{}, err
	}
	return result, nil
}

func (service *Service) addCandidates(ctx context.Context, userID string, result *Result, candidates map[int64]candidate, domain adder, add addMovie) error {
	ids := make([]int64, 0, len(candidates))
	for id := range candidates {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	tracked, err := domain.TrackedProviderIDs(ctx, userID, ids)
	if err != nil {
		return fmt.Errorf("check existing Jellyfin import items: %w", err)
	}
	for _, id := range ids {
		candidate := candidates[id]
		if tracked[id] {
			result.AlreadyPresent++
			continue
		}
		if err := add(ctx, userID, id, media.StatusBacklog); err != nil {
			result.Failed++
			result.Issues = append(result.Issues, Issue{Title: candidate.item.Title, Reason: "Canonical metadata could not be added; remaining items continued."})
			continue
		}
		if candidate.domain == jellyfin.Movies {
			result.MoviesAdded++
		} else {
			result.TVShowsAdded++
		}
	}
	return nil
}

func missingProviderReason(domain jellyfin.MediaType) string {
	provider := "TMDB"
	if domain == jellyfin.TVShows {
		provider = "TVDB"
	}
	return strings.Join([]string{"Missing a valid", provider, "provider ID."}, " ")
}
