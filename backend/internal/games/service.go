package games

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nzagler/gradeium/backend/internal/integrations/igdb"
	"github.com/nzagler/gradeium/backend/internal/media"
)

var (
	ErrNotFound             = errors.New("game not found")
	ErrAlreadyTracked       = errors.New("game is already tracked")
	ErrConfirmationRequired = errors.New("moving this rated game to Backlog requires confirmation")
)

type Provider interface {
	Search(context.Context, string, int) (igdb.SearchPage, error)
	Game(context.Context, int64) (igdb.Game, error)
}
type ProviderFactory interface {
	IGDB(context.Context) (*igdb.Client, error)
}

type State struct {
	media.PersonalState
	DateAdded time.Time `json:"dateAdded"`
}
type Item struct {
	ID              string     `json:"id"`
	ProviderID      int64      `json:"providerId"`
	Title           string     `json:"title"`
	Year            *int32     `json:"year,omitempty"`
	ReleaseDate     *time.Time `json:"releaseDate,omitempty"`
	Developer       string     `json:"developer,omitempty"`
	GameType        string     `json:"gameType,omitempty"`
	Genres          []string   `json:"genres"`
	CommunityRating *int16     `json:"communityRating,omitempty"`
	ArtworkURL      string     `json:"artworkUrl,omitempty"`
	State           State      `json:"state"`
}
type RelatedRelease struct {
	igdb.RelatedRelease
	LocalID     string       `json:"localId,omitempty"`
	LocalStatus media.Status `json:"localStatus,omitempty"`
	LocalRating *int16       `json:"localRating,omitempty"`
}
type Detail struct {
	Item
	Summary              string                   `json:"summary,omitempty"`
	Publisher            string                   `json:"publisher,omitempty"`
	GameModes            []string                 `json:"gameModes"`
	Platforms            []string                 `json:"platforms"`
	Franchise            string                   `json:"franchise,omitempty"`
	CommunityRatingCount *int32                   `json:"communityRatingCount,omitempty"`
	Screenshots          []string                 `json:"screenshots"`
	AdditionalContent    []igdb.AdditionalContent `json:"additionalContent"`
	RelatedReleases      []RelatedRelease         `json:"relatedReleases"`
	ExternalLinks        []igdb.ExternalLink      `json:"externalLinks"`
	Artworks             []media.Artwork          `json:"artworks"`
	ArtworkPins          map[string]string        `json:"artworkPins"`
	UnavailablePins      []string                 `json:"unavailablePins"`
	MetadataRefreshedAt  time.Time                `json:"metadataRefreshedAt"`
}
type SearchResult struct {
	igdb.SearchResult
	LocalID    string `json:"localId,omitempty"`
	LocalState string `json:"localState,omitempty"`
}
type SearchPage struct {
	Results []SearchResult `json:"results"`
	Page    int            `json:"page"`
	HasMore bool           `json:"hasMore"`
}

type Store interface {
	Tracked(context.Context, string, []int64) (map[int64]Tracked, error)
	Add(context.Context, string, igdb.Game, media.Status) (Detail, error)
	List(context.Context, string, bool) ([]Item, error)
	Detail(context.Context, string, string) (Detail, error)
	UpdateState(context.Context, string, string, media.PersonalState, bool) (State, error)
	SelectArtwork(context.Context, string, string, string, string) (Detail, error)
	Refresh(context.Context, string, igdb.Game) (Detail, error)
	Remove(context.Context, string, string) (bool, error)
}
type Tracked struct {
	ID     string
	Status media.Status
}
type Service struct {
	providers ProviderFactory
	store     Store
}

func NewService(providers ProviderFactory, store Store) *Service {
	return &Service{providers: providers, store: store}
}

func (service *Service) Search(ctx context.Context, userID, query string, page int) (SearchPage, error) {
	query, err := media.ValidateSearch(query, page)
	if err != nil {
		return SearchPage{}, media.ValidationError(err.Error())
	}
	client, err := service.providers.IGDB(ctx)
	if err != nil {
		return SearchPage{}, media.ProviderError("igdb")
	}
	upstream, err := client.Search(ctx, query, page)
	if err != nil {
		return SearchPage{}, media.ProviderError("igdb")
	}
	ids := make([]int64, 0, len(upstream.Results))
	for _, item := range upstream.Results {
		ids = append(ids, item.ProviderID)
	}
	tracked, err := service.store.Tracked(ctx, userID, ids)
	if err != nil {
		return SearchPage{}, err
	}
	result := SearchPage{Results: make([]SearchResult, 0, len(upstream.Results)), Page: upstream.Page, HasMore: upstream.HasMore}
	for _, item := range upstream.Results {
		value := SearchResult{SearchResult: item}
		if local, ok := tracked[item.ProviderID]; ok {
			value.LocalID = local.ID
			if local.Status == media.StatusBacklog {
				value.LocalState = "Already in Backlog"
			} else {
				value.LocalState = "Already in Library"
			}
		}
		result.Results = append(result.Results, value)
	}
	return result, nil
}
func (service *Service) Add(ctx context.Context, userID string, providerID int64, status media.Status) (Detail, error) {
	if err := media.ValidateProviderID(providerID); err != nil {
		return Detail{}, media.ValidationError(err.Error())
	}
	state, err := media.ValidatePersonalState(media.PersonalState{Status: status})
	if err != nil {
		return Detail{}, media.ValidationError(err.Error())
	}
	client, err := service.providers.IGDB(ctx)
	if err != nil {
		return Detail{}, media.ProviderError("igdb")
	}
	game, err := client.Game(ctx, providerID)
	if err != nil {
		return Detail{}, media.ProviderError("igdb")
	}
	detail, err := service.store.Add(ctx, userID, game, state.Status)
	if errors.Is(err, ErrAlreadyTracked) {
		return Detail{}, &media.SafeError{Code: "already_tracked", Message: "This game is already in your library."}
	}
	return detail, err
}
func (service *Service) List(ctx context.Context, userID string, backlog bool) ([]Item, error) {
	return service.store.List(ctx, userID, backlog)
}
func (service *Service) Detail(ctx context.Context, userID, id string) (Detail, error) {
	return service.store.Detail(ctx, userID, id)
}
func (service *Service) UpdateState(ctx context.Context, userID, id string, state media.PersonalState, confirm bool) (State, error) {
	normalized, err := media.ValidatePersonalState(state)
	if err != nil {
		return State{}, media.ValidationError(err.Error())
	}
	return service.store.UpdateState(ctx, userID, id, normalized, confirm)
}
func (service *Service) SelectArtwork(ctx context.Context, userID, id, kind, providerImageID string) (Detail, error) {
	if err := media.ValidateArtworkKind(kind); err != nil {
		return Detail{}, media.ValidationError(err.Error())
	}
	if kind == "poster" {
		return Detail{}, media.ValidationError("games use cover artwork")
	}
	return service.store.SelectArtwork(ctx, userID, id, kind, providerImageID)
}
func (service *Service) Refresh(ctx context.Context, userID, id string) (Detail, error) {
	current, err := service.store.Detail(ctx, userID, id)
	if err != nil {
		return Detail{}, err
	}
	client, err := service.providers.IGDB(ctx)
	if err != nil {
		return Detail{}, media.ProviderError("igdb")
	}
	game, err := client.Game(ctx, current.ProviderID)
	if err != nil {
		return Detail{}, media.ProviderError("igdb")
	}
	return service.store.Refresh(ctx, userID, game)
}
func (service *Service) Remove(ctx context.Context, userID, id string) error {
	removed, err := service.store.Remove(ctx, userID, id)
	if err != nil {
		return err
	}
	if !removed {
		return fmt.Errorf("%w", ErrNotFound)
	}
	return nil
}
