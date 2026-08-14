package httpserver

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

type spaHandler struct {
	files      fs.FS
	fileServer http.Handler
}

// NewSPAHandler serves compiled assets and falls back to index.html for client routes.
func NewSPAHandler(files fs.FS) http.Handler {
	return &spaHandler{files: files, fileServer: http.FileServerFS(files)}
}

func (handler *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestedPath := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if requestedPath == "." || requestedPath == "" {
		handler.serveIndex(w, r)
		return
	}

	info, err := fs.Stat(handler.files, requestedPath)
	if err == nil && !info.IsDir() {
		request := r.Clone(r.Context())
		request.URL.Path = "/" + requestedPath
		handler.fileServer.ServeHTTP(w, request)
		return
	}

	handler.serveIndex(w, r)
}

func (handler *spaHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	index, err := fs.ReadFile(handler.files, "index.html")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(index)
}
