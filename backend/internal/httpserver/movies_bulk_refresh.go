package httpserver

import (
	"context"
	"net/http"
	"sync"

	"github.com/nzagler/gradeium/backend/internal/movies"
)

var movieRefreshManagers sync.Map

func movieRefreshManager(service *movies.Service) *movies.RefreshJobManager {
	if value, ok := movieRefreshManagers.Load(service); ok {
		return value.(*movies.RefreshJobManager)
	}
	manager := movies.NewRefreshJobManager(context.Background(), service, movies.DefaultRefreshJobTimeout)
	actual, loaded := movieRefreshManagers.LoadOrStore(service, manager)
	if loaded {
		manager.Close()
		return actual.(*movies.RefreshJobManager)
	}
	return manager
}

func (handlers *apiHandlers) startMovieBulkRefresh(w http.ResponseWriter, r *http.Request) {
	if !emptyBody(w, r) {
		return
	}
	status, _ := movieRefreshManager(handlers.movies).Start(handlers.identity(r))
	writeJSON(w, http.StatusAccepted, status)
}

func (handlers *apiHandlers) movieBulkRefreshStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, movieRefreshManager(handlers.movies).Status(handlers.identity(r)))
}
