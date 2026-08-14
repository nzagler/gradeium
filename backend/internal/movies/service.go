package movies

import (
	"context"
	"errors"
	"time"

	"github.com/nzagler/gradeium/backend/internal/integrations/tmdb"
	"github.com/nzagler/gradeium/backend/internal/media"
)

var (
	ErrNotFound             = errors.New("movie not found")
	ErrAlreadyTracked       = errors.New("movie is already tracked")
	ErrConfirmationRequired = errors.New("moving this rated movie to Backlog requires confirmation")
)

type ProviderFactory interface {
	TMDB(context.Context) (*tmdb.Client, error)
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
	RuntimeMinutes  *int32     `json:"runtimeMinutes,omitempty"`
	Director        string     `json:"director,omitempty"`
	Genres          []string   `json:"genres"`
	CommunityRating *int16     `json:"communityRating,omitempty"`
	ArtworkURL      string     `json:"artworkUrl,omitempty"`
	State           State      `json:"state"`
}
type CollectionMember struct {
	tmdb.CollectionMember
	LocalID     string       `json:"localId,omitempty"`
	LocalStatus media.Status `json:"localStatus,omitempty"`
	LocalRating *int16       `json:"localRating,omitempty"`
}
type Detail struct {
	Item
	OriginalTitle        string             `json:"originalTitle,omitempty"`
	Overview             string             `json:"overview,omitempty"`
	ProductionCompanies  []string           `json:"productionCompanies"`
	Cast                 []tmdb.Person      `json:"cast"`
	Crew                 []tmdb.Person      `json:"crew"`
	TrailerKey           string             `json:"trailerKey,omitempty"`
	IMDbID               string             `json:"imdbId,omitempty"`
	Homepage             string             `json:"homepage,omitempty"`
	CollectionID         *int64             `json:"collectionId,omitempty"`
	CollectionName       string             `json:"collectionName,omitempty"`
	Collection           []CollectionMember `json:"collection"`
	CommunityRatingCount *int32             `json:"communityRatingCount,omitempty"`
	Artworks             []media.Artwork    `json:"artworks"`
	ArtworkPins          map[string]string  `json:"artworkPins"`
	UnavailablePins      []string           `json:"unavailablePins"`
	MetadataRefreshedAt  time.Time          `json:"metadataRefreshedAt"`
}
type SearchResult struct {
	tmdb.SearchResult
	LocalID    string `json:"localId,omitempty"`
	LocalState string `json:"localState,omitempty"`
}
type SearchPage struct {
	Results []SearchResult `json:"results"`
	Page    int            `json:"page"`
	HasMore bool           `json:"hasMore"`
}
type Tracked struct {
	ID     string
	Status media.Status
}
type Store interface {
	Tracked(context.Context, string, []int64) (map[int64]Tracked, error)
	Add(context.Context, string, tmdb.Movie, media.Status) (Detail, error)
	List(context.Context, string, bool) ([]Item, error)
	Detail(context.Context, string, string) (Detail, error)
	UpdateState(context.Context, string, string, media.PersonalState, bool) (State, error)
	SelectArtwork(context.Context, string, string, string, string) (Detail, error)
	Refresh(context.Context, string, tmdb.Movie) (Detail, error)
	Remove(context.Context, string, string) (bool, error)
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
	client, err := service.providers.TMDB(ctx)
	if err != nil {
		return SearchPage{}, media.ProviderError("tmdb")
	}
	upstream, err := client.SearchMovies(ctx, query, page)
	if err != nil {
		return SearchPage{}, media.ProviderError("tmdb")
	}
	ids := []int64{}
	for _, item := range upstream.Results {
		ids = append(ids, item.ProviderID)
	}
	tracked, err := service.store.Tracked(ctx, userID, ids)
	if err != nil {
		return SearchPage{}, err
	}
	result := SearchPage{Results: []SearchResult{}, Page: upstream.Page, HasMore: upstream.HasMore}
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
	client, err := service.providers.TMDB(ctx)
	if err != nil {
		return Detail{}, media.ProviderError("tmdb")
	}
	movie, err := client.Movie(ctx, providerID)
	if err != nil {
		return Detail{}, media.ProviderError("tmdb")
	}
	detail, err := service.store.Add(ctx, userID, movie, state.Status)
	if errors.Is(err, ErrAlreadyTracked) {
		return Detail{}, &media.SafeError{Code: "already_tracked", Message: "This movie is already in your library."}
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
func (service *Service) SelectArtwork(ctx context.Context, userID, id, kind, imageID string) (Detail, error) {
	if err := media.ValidateArtworkKind(kind); err != nil {
		return Detail{}, media.ValidationError(err.Error())
	}
	if kind == "cover" {
		return Detail{}, media.ValidationError("movies use poster artwork")
	}
	return service.store.SelectArtwork(ctx, userID, id, kind, imageID)
}
func (service *Service) Refresh(ctx context.Context, userID, id string) (Detail, error) {
	current, err := service.store.Detail(ctx, userID, id)
	if err != nil {
		return Detail{}, err
	}
	client, err := service.providers.TMDB(ctx)
	if err != nil {
		return Detail{}, media.ProviderError("tmdb")
	}
	movie, err := client.Movie(ctx, current.ProviderID)
	if err != nil {
		return Detail{}, media.ProviderError("tmdb")
	}
	return service.store.Refresh(ctx, userID, movie)
}
func (service *Service) Remove(ctx context.Context, userID, id string) error {
	removed, err := service.store.Remove(ctx, userID, id)
	if err != nil {
		return err
	}
	if !removed {
		return ErrNotFound
	}
	return nil
}
