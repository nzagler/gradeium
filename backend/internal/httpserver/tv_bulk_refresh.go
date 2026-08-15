package httpserver

import (
	"context"
	"net/http"
	"sync"

	"github.com/nzagler/gradeium/backend/internal/tv"
)

var tvRefreshManagers sync.Map

func tvRefreshManager(service *tv.Service) *tv.RefreshJobManager {
	if value, ok := tvRefreshManagers.Load(service); ok {
		return value.(*tv.RefreshJobManager)
	}
	manager := tv.NewRefreshJobManager(context.Background(), service, tv.DefaultRefreshJobTimeout)
	actual, loaded := tvRefreshManagers.LoadOrStore(service, manager)
	if loaded {
		manager.Close()
		return actual.(*tv.RefreshJobManager)
	}
	return manager
}

func (handlers *apiHandlers) startTVBulkRefresh(w http.ResponseWriter, r *http.Request) {
	if !emptyBody(w, r) {
		return
	}
	status, _ := tvRefreshManager(handlers.tv).Start(handlers.identity(r))
	writeJSON(w, http.StatusAccepted, status)
}

func (handlers *apiHandlers) tvBulkRefreshStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, tvRefreshManager(handlers.tv).Status(handlers.identity(r)))
}
