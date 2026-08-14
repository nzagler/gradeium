package httpserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/nzagler/gradeium/backend/internal/backups"
	"github.com/nzagler/gradeium/backend/internal/dashboard"
)

const backupOperationTimeout = 25 * time.Second

func (handlers *apiHandlers) mountDashboardRoutes(router chi.Router) {
	router.Route("/dashboard", func(r chi.Router) {
		r.Use(handlers.requireUser)
		r.Get("/", handlers.dashboardSummary)
	})
	router.Route("/exports", func(r chi.Router) {
		r.Use(handlers.requireUser)
		r.Get("/ratings.csv", handlers.ratingsCSV)
	})
}

func (handlers *apiHandlers) mountBackupRoutes(router chi.Router) {
	router.Get("/backups", handlers.listBackups)
	router.Get("/backups/settings", handlers.backupSettings)
	router.Put("/backups/settings", handlers.updateBackupSettings)
	router.Post("/backups", handlers.createBackup)
	router.Post("/backups/restore", handlers.restoreUploadedBackup)
	router.Get("/backups/{id}/download", handlers.downloadBackup)
	router.Post("/backups/{id}/restore", handlers.restoreBackup)
	router.Delete("/backups/{id}", handlers.deleteBackup)
}

func (handlers *apiHandlers) dashboardSummary(w http.ResponseWriter, r *http.Request) {
	scope, err := dashboard.ParseScope(r.URL.Query().Get("scope"))
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), apiOperationTimeout)
	defer cancel()
	value, err := handlers.dashboard.Summary(ctx, handlers.identity(r), scope)
	if err != nil {
		handlers.internalError(w, r, "read Dashboard", err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (handlers *apiHandlers) ratingsCSV(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), apiOperationTimeout)
	defer cancel()
	value, err := handlers.dashboard.RatingsCSV(ctx, handlers.identity(r))
	if err != nil {
		handlers.internalError(w, r, "create ratings CSV export", err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="gradeium-ratings.csv"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(value)
}

func (handlers *apiHandlers) listBackups(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), apiOperationTimeout)
	defer cancel()
	items, err := handlers.backups.List(ctx)
	if err != nil {
		handlers.backupError(w, r, "list backups", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"backups": items})
}

func (handlers *apiHandlers) backupSettings(w http.ResponseWriter, r *http.Request) {
	value, err := withTimeoutResult(r.Context(), handlers.backups.Settings)
	if err != nil {
		handlers.backupError(w, r, "read backup settings", err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (handlers *apiHandlers) updateBackupSettings(w http.ResponseWriter, r *http.Request) {
	var request backups.Settings
	if err := decodeJSONBody(w, r, &request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "Provide valid automatic backup settings.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), apiOperationTimeout)
	defer cancel()
	value, err := handlers.backups.UpdateSettings(ctx, request)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func (handlers *apiHandlers) createBackup(w http.ResponseWriter, r *http.Request) {
	if !emptyBody(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), backupOperationTimeout)
	defer cancel()
	metadata, err := handlers.backups.Create(ctx, backups.KindManual)
	if err != nil {
		handlers.backupError(w, r, "create manual backup", err)
		return
	}
	writeJSON(w, http.StatusCreated, metadata)
}

func (handlers *apiHandlers) downloadBackup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if handlers.badID(w, id) {
		return
	}
	file, metadata, err := handlers.backups.Open(r.Context(), id)
	if err != nil {
		handlers.backupError(w, r, "open backup download", err)
		return
	}
	defer file.Close()
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, metadata.Filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", metadata.SizeBytes))
	w.Header().Set("X-Gradeium-Backup-SHA256", metadata.SHA256)
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, file)
}

type confirmationRequest struct {
	Confirmation string `json:"confirmation"`
}

func (handlers *apiHandlers) deleteBackup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if handlers.badID(w, id) {
		return
	}
	var request confirmationRequest
	if err := decodeJSONBody(w, r, &request); err != nil || request.Confirmation != "DELETE" {
		writeAPIError(w, http.StatusBadRequest, "confirmation_required", "Type DELETE to confirm backup deletion.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), backupOperationTimeout)
	defer cancel()
	if err := handlers.backups.Delete(ctx, id); err != nil {
		handlers.backupError(w, r, "delete backup", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (handlers *apiHandlers) restoreBackup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if handlers.badID(w, id) {
		return
	}
	var request confirmationRequest
	if err := decodeJSONBody(w, r, &request); err != nil || request.Confirmation != "RESTORE" {
		writeAPIError(w, http.StatusBadRequest, "confirmation_required", "Type RESTORE to confirm replacement of current portable media state.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), backupOperationTimeout)
	defer cancel()
	safety, err := handlers.backups.Restore(ctx, id)
	if err != nil {
		handlers.backupError(w, r, "restore backup", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"restored": true, "safetyBackup": safety})
}

func (handlers *apiHandlers) restoreUploadedBackup(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Gradeium-Restore-Confirmation") != "RESTORE" {
		writeAPIError(w, http.StatusBadRequest, "confirmation_required", "Type RESTORE to confirm replacement of current portable media state.")
		return
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
	if contentType != "application/gzip" && contentType != "application/octet-stream" {
		writeAPIError(w, http.StatusUnsupportedMediaType, "invalid_backup", "Upload a Gradeium JSON.gz backup file.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, backups.MaxCompressedSize+1)
	ctx, cancel := context.WithTimeout(r.Context(), backupOperationTimeout)
	defer cancel()
	safety, err := handlers.backups.RestoreUpload(ctx, r.Body)
	if err != nil {
		handlers.backupError(w, r, "restore uploaded backup", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"restored": true, "safetyBackup": safety})
}

func (handlers *apiHandlers) backupError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	switch {
	case errors.Is(err, backups.ErrNotFound):
		writeAPIError(w, http.StatusNotFound, "backup_not_found", "That backup is not available.")
	case errors.Is(err, backups.ErrBusy):
		writeAPIError(w, http.StatusConflict, "backup_busy", "Another backup or restore operation is already running.")
	case errors.Is(err, backups.ErrInvalidBackup):
		writeAPIError(w, http.StatusUnprocessableEntity, "invalid_backup", "The backup is malformed, unsupported, or failed integrity validation.")
	default:
		handlers.internalError(w, r, operation, err)
	}
}
