package tv

import (
	"context"
	"errors"
	"github.com/nzagler/gradeium/backend/internal/integrations/tmdb"
	"github.com/nzagler/gradeium/backend/internal/integrations/tvdb"
	"github.com/nzagler/gradeium/backend/internal/media"
	"time"
)

var (
	ErrNotFound             = errors.New("TV show not found")
	ErrAlreadyTracked       = errors.New("TV show is already tracked")
	ErrConfirmationRequired = errors.New("moving this rated TV show to Backlog requires confirmation")
)

type ProviderFactory interface {
	TVDB(context.Context) (*tvdb.Client, error)
	TMDB(context.Context) (*tmdb.Client, error)
}
type State struct {
	media.PersonalState
	DateAdded time.Time `json:"dateAdded"`
}
type Progress struct {
	Watched         int32    `json:"watched"`
	Total           int32    `json:"total"`
	Percent         int32    `json:"percent"`
	SpecialsWatched int32    `json:"specialsWatched"`
	SpecialsTotal   int32    `json:"specialsTotal"`
	NextEpisode     *Episode `json:"nextEpisode,omitempty"`
}
type Item struct {
	ID              string     `json:"id"`
	ProviderID      int64      `json:"providerId"`
	Title           string     `json:"title"`
	Year            *int32     `json:"year,omitempty"`
	FirstAired      *time.Time `json:"firstAired,omitempty"`
	Network         string     `json:"network,omitempty"`
	Genres          []string   `json:"genres"`
	CommunityRating *int16     `json:"communityRating,omitempty"`
	ArtworkURL      string     `json:"artworkUrl,omitempty"`
	State           State      `json:"state"`
	Progress        Progress   `json:"progress"`
}
type Episode struct {
	ID             string     `json:"id"`
	ProviderID     int64      `json:"providerId"`
	SeasonNumber   int32      `json:"seasonNumber"`
	EpisodeNumber  int32      `json:"episodeNumber"`
	Title          string     `json:"title"`
	Overview       string     `json:"overview,omitempty"`
	AirDate        *time.Time `json:"airDate,omitempty"`
	RuntimeMinutes *int32     `json:"runtimeMinutes,omitempty"`
	StillURL       string     `json:"stillUrl,omitempty"`
	Special        bool       `json:"special"`
	Watched        bool       `json:"watched"`
}
type Season struct {
	ID         string     `json:"id"`
	ProviderID int64      `json:"providerId"`
	Number     int32      `json:"number"`
	Name       string     `json:"name,omitempty"`
	Special    bool       `json:"special"`
	AirDate    *time.Time `json:"airDate,omitempty"`
	PosterURL  string     `json:"posterUrl,omitempty"`
	Watched    int32      `json:"watched"`
	Total      int32      `json:"total"`
	Episodes   []Episode  `json:"episodes"`
}
type Detail struct {
	Item
	OriginalTitle        string            `json:"originalTitle,omitempty"`
	Overview             string            `json:"overview,omitempty"`
	ProviderStatus       string            `json:"providerStatus,omitempty"`
	VerifiedTMDBID       *int64            `json:"verifiedTmdbId,omitempty"`
	CommunityRatingCount *int32            `json:"communityRatingCount,omitempty"`
	Cast                 []tvdb.Person     `json:"cast"`
	KeyPeople            []tvdb.Person     `json:"keyPeople"`
	Seasons              []Season          `json:"seasons"`
	Artworks             []media.Artwork   `json:"artworks"`
	ArtworkPins          map[string]string `json:"artworkPins"`
	UnavailablePins      []string          `json:"unavailablePins"`
	MetadataRefreshedAt  time.Time         `json:"metadataRefreshedAt"`
}
type SearchResult struct {
	tvdb.SearchResult
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
	Add(context.Context, string, tvdb.Show, *tmdb.VerifiedTV, media.Status) (Detail, error)
	List(context.Context, string, bool) ([]Item, error)
	Detail(context.Context, string, string) (Detail, error)
	UpdateState(context.Context, string, string, media.PersonalState, bool) (State, error)
	SelectArtwork(context.Context, string, string, string, string) (Detail, error)
	Refresh(context.Context, string, tvdb.Show, *tmdb.VerifiedTV) (Detail, error)
	SetEpisode(context.Context, string, string, string, bool) (Detail, error)
	SetSeason(context.Context, string, string, int32, bool) (Detail, error)
	SetThrough(context.Context, string, string, string) (Detail, error)
	SetAllRegular(context.Context, string, string, bool) (Detail, error)
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
	client, err := service.providers.TVDB(ctx)
	if err != nil {
		return SearchPage{}, media.ProviderError("tvdb")
	}
	upstream, err := client.Search(ctx, query, page)
	if err != nil {
		return SearchPage{}, media.ProviderError("tvdb")
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
func (service *Service) metadata(ctx context.Context, providerID int64) (tvdb.Show, *tmdb.VerifiedTV, error) {
	client, err := service.providers.TVDB(ctx)
	if err != nil {
		return tvdb.Show{}, nil, media.ProviderError("tvdb")
	}
	show, err := client.Show(ctx, providerID)
	if err != nil {
		return tvdb.Show{}, nil, media.ProviderError("tvdb")
	}
	var mapping *tmdb.VerifiedTV
	if tmdbClient, tmdbErr := service.providers.TMDB(ctx); tmdbErr == nil {
		mapping, _ = tmdbClient.VerifyTVDBMapping(ctx, providerID)
	}
	return show, mapping, nil
}
func (service *Service) Add(ctx context.Context, userID string, providerID int64, status media.Status) (Detail, error) {
	if err := media.ValidateProviderID(providerID); err != nil {
		return Detail{}, media.ValidationError(err.Error())
	}
	state, err := media.ValidatePersonalState(media.PersonalState{Status: status})
	if err != nil {
		return Detail{}, media.ValidationError(err.Error())
	}
	show, mapping, err := service.metadata(ctx, providerID)
	if err != nil {
		return Detail{}, err
	}
	detail, err := service.store.Add(ctx, userID, show, mapping, state.Status)
	if errors.Is(err, ErrAlreadyTracked) {
		return Detail{}, &media.SafeError{Code: "already_tracked", Message: "This TV show is already in your library."}
	}
	return detail, err
}

// TrackedProviderIDs is used by add-only imports to avoid touching canonical
// metadata, episode progress, or personal state for existing shows.
func (service *Service) TrackedProviderIDs(ctx context.Context, userID string, providerIDs []int64) (map[int64]bool, error) {
	tracked, err := service.store.Tracked(ctx, userID, providerIDs)
	if err != nil {
		return nil, err
	}
	result := make(map[int64]bool, len(tracked))
	for providerID := range tracked {
		result[providerID] = true
	}
	return result, nil
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
		return Detail{}, media.ValidationError("TV shows use poster artwork")
	}
	return service.store.SelectArtwork(ctx, userID, id, kind, imageID)
}
func (service *Service) Refresh(ctx context.Context, userID, id string) (Detail, error) {
	current, err := service.store.Detail(ctx, userID, id)
	if err != nil {
		return Detail{}, err
	}
	show, mapping, err := service.metadata(ctx, current.ProviderID)
	if err != nil {
		return Detail{}, err
	}
	return service.store.Refresh(ctx, userID, show, mapping)
}
func (service *Service) SetEpisode(ctx context.Context, userID, id, episodeID string, watched bool) (Detail, error) {
	return service.store.SetEpisode(ctx, userID, id, episodeID, watched)
}
func (service *Service) SetSeason(ctx context.Context, userID, id string, season int32, watched bool) (Detail, error) {
	return service.store.SetSeason(ctx, userID, id, season, watched)
}
func (service *Service) SetThrough(ctx context.Context, userID, id, episodeID string) (Detail, error) {
	return service.store.SetThrough(ctx, userID, id, episodeID)
}
func (service *Service) SetAllRegular(ctx context.Context, userID, id string, watched bool) (Detail, error) {
	return service.store.SetAllRegular(ctx, userID, id, watched)
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
