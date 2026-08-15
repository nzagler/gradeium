package httpserver

import (
	"context"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/nzagler/gradeium/backend/internal/games"
	"github.com/nzagler/gradeium/backend/internal/integrations"
	"github.com/nzagler/gradeium/backend/internal/integrations/jellyfin"
	"github.com/nzagler/gradeium/backend/internal/media"
	"github.com/nzagler/gradeium/backend/internal/movies"
	"github.com/nzagler/gradeium/backend/internal/tv"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	providerOperationTimeout = 12 * time.Second
	jellyfinSyncTimeout      = 10 * time.Minute
)

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func (handlers *apiHandlers) mountMediaRoutes(router chi.Router) {
	if handlers.preferences != nil {
		router.Route("/preferences", func(r chi.Router) {
			r.Use(handlers.requireUser)
			r.Use(handlers.requireCSRF)
			r.Get("/library", handlers.libraryPreferences)
			r.Put("/library", handlers.updateLibraryPreferences)
		})
	}
	router.Route("/games", func(r chi.Router) {
		r.Use(handlers.requireUser)
		r.Use(handlers.requireCSRF)
		r.Get("/", handlers.listGames)
		r.Get("/search", handlers.searchGames)
		r.Post("/", handlers.addGame)
		r.Get("/{id}", handlers.gameDetail)
		r.Patch("/{id}/state", handlers.updateGameState)
		r.Put("/{id}/artwork/{kind}", handlers.selectGameArtwork)
		r.Post("/{id}/refresh", handlers.refreshGame)
		r.Delete("/{id}", handlers.removeGame)
	})
	router.Route("/movies", func(r chi.Router) {
		r.Use(handlers.requireUser)
		r.Use(handlers.requireCSRF)
		r.Get("/", handlers.listMovies)
		r.Get("/search", handlers.searchMovies)
		r.Post("/", handlers.addMovie)
		r.Get("/{id}", handlers.movieDetail)
		r.Patch("/{id}/state", handlers.updateMovieState)
		r.Put("/{id}/artwork/{kind}", handlers.selectMovieArtwork)
		r.Post("/{id}/refresh", handlers.refreshMovie)
		r.Delete("/{id}", handlers.removeMovie)
	})
	router.Route("/tv", func(r chi.Router) {
		r.Use(handlers.requireUser)
		r.Use(handlers.requireCSRF)
		r.Get("/", handlers.listTV)
		r.Get("/search", handlers.searchTV)
		r.Post("/", handlers.addTV)
		r.Get("/{id}", handlers.tvDetail)
		r.Patch("/{id}/state", handlers.updateTVState)
		r.Put("/{id}/artwork/{kind}", handlers.selectTVArtwork)
		r.Post("/{id}/refresh", handlers.refreshTV)
		r.Put("/{id}/episodes/{episodeID}", handlers.setTVEpisode)
		r.Put("/{id}/seasons/{season}", handlers.setTVSeason)
		r.Post("/{id}/progress/through/{episodeID}", handlers.setTVThrough)
		r.Put("/{id}/progress/regular", handlers.setTVRegular)
		r.Delete("/{id}", handlers.removeTV)
	})
}

func (handlers *apiHandlers) libraryPreferences(w http.ResponseWriter, r *http.Request) {
	value, err := handlers.preferences.Get(r.Context(), handlers.identity(r))
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (handlers *apiHandlers) updateLibraryPreferences(w http.ResponseWriter, r *http.Request) {
	var request media.Preferences
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "Provide valid Library preferences.")
		return
	}
	value, err := handlers.preferences.Update(r.Context(), handlers.identity(r), request)
	if err != nil {
		handlers.mediaError(w, r, media.ValidationError(err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (handlers *apiHandlers) identity(r *http.Request) string {
	value, _ := r.Context().Value(requestAuthenticationKey{}).(*requestAuthentication)
	if value == nil {
		return ""
	}
	return value.session.User.ID
}
func mediaContext(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), providerOperationTimeout)
}
func parsePage(r *http.Request) (int, error) {
	if r.URL.Query().Get("page") == "" {
		return 1, nil
	}
	return strconv.Atoi(r.URL.Query().Get("page"))
}
func backlogView(r *http.Request) bool { return r.URL.Query().Get("view") == "backlog" }
func validID(id string) bool           { return uuidPattern.MatchString(strings.ToLower(id)) }

type addRequest struct {
	ProviderID int64  `json:"providerId"`
	Status     string `json:"status"`
}
type stateRequest struct {
	Status             string  `json:"status"`
	Rating             *int16  `json:"rating"`
	RatingReason       *string `json:"ratingReason"`
	ConfirmRatingClear bool    `json:"confirmRatingClear"`
}
type artworkRequest struct {
	ProviderImageID string `json:"providerImageId"`
}
type watchedRequest struct {
	Watched bool `json:"watched"`
}

func stateFrom(request stateRequest) (media.PersonalState, error) {
	status, err := media.ParseStatus(request.Status)
	if err != nil {
		return media.PersonalState{}, err
	}
	return media.PersonalState{Status: status, Rating: request.Rating, RatingReason: request.RatingReason}, nil
}
func (handlers *apiHandlers) badID(w http.ResponseWriter, id string) bool {
	if validID(id) {
		return false
	}
	writeAPIError(w, http.StatusBadRequest, "invalid_id", "The Gradeium item ID is invalid.")
	return true
}

func (handlers *apiHandlers) listIntegrations(w http.ResponseWriter, r *http.Request) {
	values, err := withTimeoutResult(r.Context(), handlers.integrations.List)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"integrations": values})
}
func (handlers *apiHandlers) configureIntegration(w http.ResponseWriter, r *http.Request) {
	var request integrations.ConfigurationInput
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "Provide one valid integration configuration.")
		return
	}
	ctx, cancel := mediaContext(r)
	defer cancel()
	value, err := handlers.integrations.Configure(ctx, chi.URLParam(r, "provider"), request)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (handlers *apiHandlers) testIntegration(w http.ResponseWriter, r *http.Request) {
	if !emptyBody(w, r) {
		return
	}
	ctx, cancel := mediaContext(r)
	defer cancel()
	value, err := handlers.integrations.Test(ctx, chi.URLParam(r, "provider"))
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (handlers *apiHandlers) jellyfinLibraries(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := mediaContext(r)
	defer cancel()
	client, err := handlers.integrations.Jellyfin(ctx)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	libraries, err := client.Libraries(ctx)
	if err != nil {
		handlers.mediaError(w, r, &integrations.SafeError{Code: "jellyfin_connection_failed", Message: "Jellyfin libraries could not be loaded.", Cause: err})
		return
	}
	mappings, err := handlers.integrations.JellyfinMappings(ctx)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	byID := make(map[string]string, len(mappings))
	for _, mapping := range mappings {
		byID[mapping.LibraryID] = string(mapping.Domain)
	}
	for index := range libraries {
		libraries[index].Domain = ""
		if domain := byID[libraries[index].ID]; domain != "" {
			libraries[index].Domain = integrationsJellyfinDomain(domain)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"libraries": libraries})
}

func (handlers *apiHandlers) syncJellyfin(w http.ResponseWriter, r *http.Request) {
	if !emptyBody(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), jellyfinSyncTimeout)
	defer cancel()
	client, err := handlers.integrations.Jellyfin(ctx)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	mappings, err := handlers.integrations.JellyfinMappings(ctx)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	if len(mappings) == 0 {
		writeAPIError(w, http.StatusBadRequest, "validation_error", "Map at least one Jellyfin library before importing.")
		return
	}
	result, err := handlers.jellyfinSync.Sync(ctx, handlers.identity(r), client, mappings)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func integrationsJellyfinDomain(value string) jellyfin.MediaType {
	return jellyfin.MediaType(value)
}

func (handlers *apiHandlers) searchGames(w http.ResponseWriter, r *http.Request) {
	page, err := parsePage(r)
	if err != nil {
		handlers.mediaError(w, r, media.ValidationError("search page is invalid"))
		return
	}
	ctx, cancel := mediaContext(r)
	defer cancel()
	value, err := handlers.games.Search(ctx, handlers.identity(r), r.URL.Query().Get("q"), page)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (handlers *apiHandlers) listGames(w http.ResponseWriter, r *http.Request) {
	value, err := handlers.games.List(r.Context(), handlers.identity(r), backlogView(r))
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": value})
}
func (handlers *apiHandlers) addGame(w http.ResponseWriter, r *http.Request) {
	var request addRequest
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "Provide a provider ID and initial status.")
		return
	}
	status, err := media.ParseStatus(request.Status)
	if err != nil {
		handlers.mediaError(w, r, media.ValidationError(err.Error()))
		return
	}
	ctx, cancel := mediaContext(r)
	defer cancel()
	value, err := handlers.games.Add(ctx, handlers.identity(r), request.ProviderID, status)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}
func (handlers *apiHandlers) gameDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if handlers.badID(w, id) {
		return
	}
	value, err := handlers.games.Detail(r.Context(), handlers.identity(r), id)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (handlers *apiHandlers) updateGameState(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if handlers.badID(w, id) {
		return
	}
	var request stateRequest
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "Provide a valid status and rating.")
		return
	}
	state, err := stateFrom(request)
	if err != nil {
		handlers.mediaError(w, r, media.ValidationError(err.Error()))
		return
	}
	value, err := handlers.games.UpdateState(r.Context(), handlers.identity(r), id, state, request.ConfirmRatingClear)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (handlers *apiHandlers) selectGameArtwork(w http.ResponseWriter, r *http.Request) {
	handlers.selectArtwork(w, r, func(ctx context.Context, user, id, kind, image string) (any, error) {
		return handlers.games.SelectArtwork(ctx, user, id, kind, image)
	})
}
func (handlers *apiHandlers) refreshGame(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if handlers.badID(w, id) {
		return
	}
	ctx, cancel := mediaContext(r)
	defer cancel()
	value, err := handlers.games.Refresh(ctx, handlers.identity(r), id)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (handlers *apiHandlers) removeGame(w http.ResponseWriter, r *http.Request) {
	handlers.removeMedia(w, r, handlers.games.Remove)
}

func (handlers *apiHandlers) searchMovies(w http.ResponseWriter, r *http.Request) {
	page, err := parsePage(r)
	if err != nil {
		handlers.mediaError(w, r, media.ValidationError("search page is invalid"))
		return
	}
	ctx, cancel := mediaContext(r)
	defer cancel()
	value, err := handlers.movies.Search(ctx, handlers.identity(r), r.URL.Query().Get("q"), page)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (handlers *apiHandlers) listMovies(w http.ResponseWriter, r *http.Request) {
	value, err := handlers.movies.List(r.Context(), handlers.identity(r), backlogView(r))
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": value})
}
func (handlers *apiHandlers) addMovie(w http.ResponseWriter, r *http.Request) {
	var request addRequest
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "Provide a provider ID and initial status.")
		return
	}
	status, err := media.ParseStatus(request.Status)
	if err != nil {
		handlers.mediaError(w, r, media.ValidationError(err.Error()))
		return
	}
	ctx, cancel := mediaContext(r)
	defer cancel()
	value, err := handlers.movies.Add(ctx, handlers.identity(r), request.ProviderID, status)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}
func (handlers *apiHandlers) movieDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if handlers.badID(w, id) {
		return
	}
	value, err := handlers.movies.Detail(r.Context(), handlers.identity(r), id)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (handlers *apiHandlers) updateMovieState(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if handlers.badID(w, id) {
		return
	}
	var request stateRequest
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "Provide a valid status and rating.")
		return
	}
	state, err := stateFrom(request)
	if err != nil {
		handlers.mediaError(w, r, media.ValidationError(err.Error()))
		return
	}
	value, err := handlers.movies.UpdateState(r.Context(), handlers.identity(r), id, state, request.ConfirmRatingClear)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (handlers *apiHandlers) selectMovieArtwork(w http.ResponseWriter, r *http.Request) {
	handlers.selectArtwork(w, r, func(ctx context.Context, user, id, kind, image string) (any, error) {
		return handlers.movies.SelectArtwork(ctx, user, id, kind, image)
	})
}
func (handlers *apiHandlers) refreshMovie(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if handlers.badID(w, id) {
		return
	}
	ctx, cancel := mediaContext(r)
	defer cancel()
	value, err := handlers.movies.Refresh(ctx, handlers.identity(r), id)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (handlers *apiHandlers) removeMovie(w http.ResponseWriter, r *http.Request) {
	handlers.removeMedia(w, r, handlers.movies.Remove)
}

func (handlers *apiHandlers) searchTV(w http.ResponseWriter, r *http.Request) {
	page, err := parsePage(r)
	if err != nil {
		handlers.mediaError(w, r, media.ValidationError("search page is invalid"))
		return
	}
	ctx, cancel := mediaContext(r)
	defer cancel()
	value, err := handlers.tv.Search(ctx, handlers.identity(r), r.URL.Query().Get("q"), page)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (handlers *apiHandlers) listTV(w http.ResponseWriter, r *http.Request) {
	value, err := handlers.tv.List(r.Context(), handlers.identity(r), backlogView(r))
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": value})
}
func (handlers *apiHandlers) addTV(w http.ResponseWriter, r *http.Request) {
	var request addRequest
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "Provide a provider ID and initial status.")
		return
	}
	status, err := media.ParseStatus(request.Status)
	if err != nil {
		handlers.mediaError(w, r, media.ValidationError(err.Error()))
		return
	}
	ctx, cancel := mediaContext(r)
	defer cancel()
	value, err := handlers.tv.Add(ctx, handlers.identity(r), request.ProviderID, status)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, value)
}
func (handlers *apiHandlers) tvDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if handlers.badID(w, id) {
		return
	}
	value, err := handlers.tv.Detail(r.Context(), handlers.identity(r), id)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (handlers *apiHandlers) updateTVState(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if handlers.badID(w, id) {
		return
	}
	var request stateRequest
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "Provide a valid status and rating.")
		return
	}
	state, err := stateFrom(request)
	if err != nil {
		handlers.mediaError(w, r, media.ValidationError(err.Error()))
		return
	}
	value, err := handlers.tv.UpdateState(r.Context(), handlers.identity(r), id, state, request.ConfirmRatingClear)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (handlers *apiHandlers) selectTVArtwork(w http.ResponseWriter, r *http.Request) {
	handlers.selectArtwork(w, r, func(ctx context.Context, user, id, kind, image string) (any, error) {
		return handlers.tv.SelectArtwork(ctx, user, id, kind, image)
	})
}
func (handlers *apiHandlers) refreshTV(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if handlers.badID(w, id) {
		return
	}
	ctx, cancel := mediaContext(r)
	defer cancel()
	value, err := handlers.tv.Refresh(ctx, handlers.identity(r), id)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (handlers *apiHandlers) removeTV(w http.ResponseWriter, r *http.Request) {
	handlers.removeMedia(w, r, handlers.tv.Remove)
}
func (handlers *apiHandlers) setTVEpisode(w http.ResponseWriter, r *http.Request) {
	id, episodeID := chi.URLParam(r, "id"), chi.URLParam(r, "episodeID")
	if handlers.badID(w, id) || handlers.badID(w, episodeID) {
		return
	}
	var request watchedRequest
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "Provide watched state.")
		return
	}
	value, err := handlers.tv.SetEpisode(r.Context(), handlers.identity(r), id, episodeID, request.Watched)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (handlers *apiHandlers) setTVSeason(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if handlers.badID(w, id) {
		return
	}
	number, err := strconv.ParseInt(chi.URLParam(r, "season"), 10, 32)
	if err != nil || number < 0 {
		writeAPIError(w, http.StatusBadRequest, "invalid_season", "The season number is invalid.")
		return
	}
	var request watchedRequest
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "Provide watched state.")
		return
	}
	value, err := handlers.tv.SetSeason(r.Context(), handlers.identity(r), id, int32(number), request.Watched)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (handlers *apiHandlers) setTVThrough(w http.ResponseWriter, r *http.Request) {
	id, episodeID := chi.URLParam(r, "id"), chi.URLParam(r, "episodeID")
	if handlers.badID(w, id) || handlers.badID(w, episodeID) {
		return
	}
	if !emptyBody(w, r) {
		return
	}
	value, err := handlers.tv.SetThrough(r.Context(), handlers.identity(r), id, episodeID)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
func (handlers *apiHandlers) setTVRegular(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if handlers.badID(w, id) {
		return
	}
	var request watchedRequest
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "Provide watched state.")
		return
	}
	value, err := handlers.tv.SetAllRegular(r.Context(), handlers.identity(r), id, request.Watched)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

type artworkSelector func(context.Context, string, string, string, string) (any, error)

func (handlers *apiHandlers) selectArtwork(w http.ResponseWriter, r *http.Request, selectArtwork artworkSelector) {
	id := chi.URLParam(r, "id")
	if handlers.badID(w, id) {
		return
	}
	var request artworkRequest
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "Provide a provider artwork ID or an empty value to use the provider default.")
		return
	}
	value, err := selectArtwork(r.Context(), handlers.identity(r), id, chi.URLParam(r, "kind"), request.ProviderImageID)
	if err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

type remover func(context.Context, string, string) error

func (handlers *apiHandlers) removeMedia(w http.ResponseWriter, r *http.Request, remove remover) {
	id := chi.URLParam(r, "id")
	if handlers.badID(w, id) {
		return
	}
	if !emptyBody(w, r) {
		return
	}
	if err := remove(r.Context(), handlers.identity(r), id); err != nil {
		handlers.mediaError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (handlers *apiHandlers) mediaError(w http.ResponseWriter, r *http.Request, err error) {
	var safeMedia *media.SafeError
	if errors.As(err, &safeMedia) {
		status := http.StatusBadRequest
		if strings.HasSuffix(safeMedia.Code, "_unavailable") {
			status = http.StatusBadGateway
		} else if safeMedia.Code == "already_tracked" {
			status = http.StatusConflict
		}
		writeAPIError(w, status, safeMedia.Code, safeMedia.Message)
		return
	}
	var safeIntegration *integrations.SafeError
	if errors.As(err, &safeIntegration) {
		status := http.StatusBadRequest
		if strings.HasSuffix(safeIntegration.Code, "_connection_failed") {
			status = http.StatusBadGateway
		}
		writeAPIError(w, status, safeIntegration.Code, safeIntegration.Message)
		return
	}
	switch {
	case errors.Is(err, games.ErrNotFound), errors.Is(err, movies.ErrNotFound), errors.Is(err, tv.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "not_found", "That item is not in your Gradeium library.")
	case errors.Is(err, games.ErrConfirmationRequired), errors.Is(err, movies.ErrConfirmationRequired), errors.Is(err, tv.ErrConfirmationRequired):
		writeAPIError(w, http.StatusConflict, "rating_clear_confirmation_required", err.Error())
	default:
		handlers.internalError(w, r, "media request", err)
	}
}
