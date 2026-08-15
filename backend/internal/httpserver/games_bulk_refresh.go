package httpserver

import (
	"context"
	"net/http"
	"sync"

	"github.com/nzagler/gradeium/backend/internal/games"
)

var gameRefreshManagers sync.Map

func gameRefreshManager(service *games.Service) *games.RefreshJobManager {
	if value, ok := gameRefreshManagers.Load(service); ok {
		return value.(*games.RefreshJobManager)
	}
	manager := games.NewRefreshJobManager(context.Background(), service, games.DefaultRefreshJobTimeout)
	actual, loaded := gameRefreshManagers.LoadOrStore(service, manager)
	if loaded {
		manager.Close()
		return actual.(*games.RefreshJobManager)
	}
	return manager
}

func (handlers *apiHandlers) startGameBulkRefresh(w http.ResponseWriter, r *http.Request) {
	if !emptyBody(w, r) {
		return
	}
	status, _ := gameRefreshManager(handlers.games).Start(handlers.identity(r))
	writeJSON(w, http.StatusAccepted, status)
}

func (handlers *apiHandlers) gameBulkRefreshStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, gameRefreshManager(handlers.games).Status(handlers.identity(r)))
}
