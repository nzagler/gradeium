package backups

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	ErrNotFound      = errors.New("backup not found")
	ErrBusy          = errors.New("another backup or restore operation is already running")
	ErrInvalidBackup = errors.New("backup is invalid")
)

var backupFilenamePattern = regexp.MustCompile(`^gradeium-[0-9]{8}T[0-9]{6}\.[0-9]{9}Z-(manual|automatic|pre-restore)\.json\.gz$`)

type Service struct {
	store              *PostgresStore
	directory          string
	applicationVersion string
	operation          chan struct{}
	now                func() time.Time
}

func NewService(store *PostgresStore, directory, applicationVersion string) *Service {
	return &Service{
		store:              store,
		directory:          directory,
		applicationVersion: applicationVersion,
		operation:          make(chan struct{}, 1),
		now:                utcNow,
	}
}

func (service *Service) Create(ctx context.Context, kind Kind) (Metadata, error) {
	if kind != KindManual && kind != KindAutomatic && kind != KindPreRestore {
		return Metadata{}, errors.New("unsupported backup kind")
	}
	if err := service.acquire(ctx); err != nil {
		return Metadata{}, err
	}
	defer service.release()
	return service.createLocked(ctx, kind)
}

func (service *Service) createLocked(ctx context.Context, kind Kind) (Metadata, error) {
	if err := os.MkdirAll(service.directory, 0o700); err != nil {
		return Metadata{}, fmt.Errorf("prepare backup directory: %w", err)
	}
	document, err := service.store.Snapshot(ctx, service.applicationVersion)
	if err != nil {
		return Metadata{}, err
	}
	filename := backupFilename(document.CreatedAt, kind)
	finalPath, err := service.path(filename)
	if err != nil {
		return Metadata{}, err
	}
	temporary, err := os.CreateTemp(service.directory, ".gradeium-backup-*.tmp")
	if err != nil {
		return Metadata{}, fmt.Errorf("create temporary backup: %w", err)
	}
	temporaryPath := temporary.Name()
	published := false
	defer func() {
		_ = temporary.Close()
		if !published {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return Metadata{}, fmt.Errorf("secure temporary backup: %w", err)
	}
	hash := sha256.New()
	if err := Encode(io.MultiWriter(temporary, hash), document); err != nil {
		return Metadata{}, err
	}
	if err := temporary.Sync(); err != nil {
		return Metadata{}, fmt.Errorf("flush temporary backup: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return Metadata{}, fmt.Errorf("close temporary backup: %w", err)
	}
	validated, checksum, size, err := validateFile(temporaryPath)
	if err != nil {
		return Metadata{}, fmt.Errorf("validate generated backup: %w", err)
	}
	if checksum != hex.EncodeToString(hash.Sum(nil)) || validated.Format != document.Format || validated.Version != document.Version {
		return Metadata{}, errors.New("generated backup validation did not reproduce the written document")
	}
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		return Metadata{}, fmt.Errorf("publish backup atomically: %w", err)
	}
	published = true
	syncDirectory(service.directory)
	metadata, err := service.store.AddMetadata(ctx, Metadata{
		Filename:           filename,
		Kind:               kind,
		CreatedAt:          document.CreatedAt,
		SizeBytes:          size,
		SHA256:             checksum,
		FormatVersion:      document.Version,
		ApplicationVersion: document.ApplicationVersion,
		Valid:              true,
	})
	if err != nil {
		_ = os.Remove(finalPath)
		return Metadata{}, err
	}
	return metadata, nil
}

func (service *Service) List(ctx context.Context) ([]Metadata, error) {
	items, err := service.store.ListMetadata(ctx)
	if err != nil {
		return nil, err
	}
	for index := range items {
		path, pathErr := service.path(items[index].Filename)
		if pathErr != nil {
			items[index].Valid = false
			continue
		}
		stat, statErr := os.Stat(path)
		if statErr != nil || stat.Size() != items[index].SizeBytes {
			items[index].Valid = false
		}
	}
	return items, nil
}

func (service *Service) Open(ctx context.Context, id string) (*os.File, Metadata, error) {
	metadata, err := service.store.Metadata(ctx, id)
	if err != nil {
		return nil, Metadata{}, err
	}
	path, err := service.path(metadata.Filename)
	if err != nil {
		return nil, Metadata{}, ErrNotFound
	}
	document, checksum, size, err := validateFile(path)
	if err != nil || checksum != metadata.SHA256 || size != metadata.SizeBytes || document.Version != metadata.FormatVersion {
		return nil, Metadata{}, fmt.Errorf("%w: file failed integrity validation", ErrInvalidBackup)
	}
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, Metadata{}, ErrNotFound
		}
		return nil, Metadata{}, fmt.Errorf("open backup download: %w", err)
	}
	return file, metadata, nil
}

func (service *Service) Delete(ctx context.Context, id string) error {
	if err := service.acquire(ctx); err != nil {
		return err
	}
	defer service.release()
	return service.deleteLocked(ctx, id)
}

func (service *Service) deleteLocked(ctx context.Context, id string) error {
	metadata, err := service.store.Metadata(ctx, id)
	if err != nil {
		return err
	}
	path, err := service.path(metadata.Filename)
	if err != nil {
		return ErrNotFound
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete backup file: %w", err)
	}
	if err := service.store.DeleteMetadata(ctx, id); err != nil {
		return err
	}
	return nil
}

func (service *Service) Restore(ctx context.Context, id string) (Metadata, error) {
	if err := service.acquire(ctx); err != nil {
		return Metadata{}, err
	}
	defer service.release()
	metadata, err := service.store.Metadata(ctx, id)
	if err != nil {
		return Metadata{}, err
	}
	path, err := service.path(metadata.Filename)
	if err != nil {
		return Metadata{}, ErrNotFound
	}
	document, checksum, size, err := validateFile(path)
	if err != nil || checksum != metadata.SHA256 || size != metadata.SizeBytes {
		return Metadata{}, fmt.Errorf("%w: file failed integrity validation", ErrInvalidBackup)
	}
	safety, err := service.createLocked(ctx, KindPreRestore)
	if err != nil {
		return Metadata{}, fmt.Errorf("create pre-restore safety backup: %w", err)
	}
	if err := service.store.Restore(ctx, document); err != nil {
		return Metadata{}, err
	}
	return safety, nil
}

func (service *Service) RestoreUpload(ctx context.Context, source io.Reader) (Metadata, error) {
	if err := service.acquire(ctx); err != nil {
		return Metadata{}, err
	}
	defer service.release()
	if err := os.MkdirAll(service.directory, 0o700); err != nil {
		return Metadata{}, fmt.Errorf("prepare backup directory: %w", err)
	}
	temporary, err := os.CreateTemp(service.directory, ".gradeium-restore-*.tmp")
	if err != nil {
		return Metadata{}, fmt.Errorf("create restore upload: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return Metadata{}, fmt.Errorf("secure restore upload: %w", err)
	}
	limited := &io.LimitedReader{R: source, N: MaxCompressedSize + 1}
	written, err := io.Copy(temporary, limited)
	if err != nil {
		return Metadata{}, fmt.Errorf("%w: receive restore upload: %v", ErrInvalidBackup, err)
	}
	if written > MaxCompressedSize || limited.N <= 0 {
		return Metadata{}, fmt.Errorf("%w: backup upload exceeds the compressed size limit", ErrInvalidBackup)
	}
	if err := temporary.Sync(); err != nil {
		return Metadata{}, fmt.Errorf("flush restore upload: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return Metadata{}, fmt.Errorf("close restore upload: %w", err)
	}
	document, _, _, err := validateFile(temporaryPath)
	if err != nil {
		return Metadata{}, fmt.Errorf("%w: %v", ErrInvalidBackup, err)
	}
	safety, err := service.createLocked(ctx, KindPreRestore)
	if err != nil {
		return Metadata{}, fmt.Errorf("create pre-restore safety backup: %w", err)
	}
	if err := service.store.Restore(ctx, document); err != nil {
		return Metadata{}, err
	}
	return safety, nil
}

func (service *Service) Settings(ctx context.Context) (Settings, error) {
	value, err := service.store.BackupSettings(ctx)
	return value.Settings, err
}

func (service *Service) UpdateSettings(ctx context.Context, value Settings) (Settings, error) {
	if value.IntervalDays < 1 || value.IntervalDays > 365 {
		return Settings{}, errors.New("backup interval must be between 1 and 365 days")
	}
	if value.RetentionCount < 1 || value.RetentionCount > 365 {
		return Settings{}, errors.New("backup retention must be between 1 and 365")
	}
	updated, err := service.store.UpdateBackupSettings(ctx, value.Enabled, value.IntervalDays, value.RetentionCount)
	return updated.Settings, err
}

func (service *Service) ApplyRetention(ctx context.Context, retention int32) error {
	if err := service.acquire(ctx); err != nil {
		return err
	}
	defer service.release()
	items, err := service.store.ListMetadata(ctx)
	if err != nil {
		return err
	}
	automatic := make([]Metadata, 0)
	for _, item := range items {
		if item.Kind == KindAutomatic {
			automatic = append(automatic, item)
		}
	}
	if len(automatic) <= int(retention) {
		return nil
	}
	for _, item := range automatic[int(retention):] {
		if err := service.deleteLocked(ctx, item.ID); err != nil {
			return err
		}
	}
	return nil
}

func (service *Service) acquire(ctx context.Context) error {
	select {
	case service.operation <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ErrBusy
	}
}

func (service *Service) release() { <-service.operation }

func (service *Service) path(filename string) (string, error) {
	if !backupFilenamePattern.MatchString(filename) || filepath.Base(filename) != filename {
		return "", errors.New("backup filename is invalid")
	}
	directory, err := filepath.Abs(service.directory)
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(filepath.Join(directory, filename))
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(directory, path)
	if err != nil || relative != filename {
		return "", errors.New("backup path escapes the configured directory")
	}
	return path, nil
}

func backupFilename(createdAt time.Time, kind Kind) string {
	label := strings.ReplaceAll(string(kind), "_", "-")
	return "gradeium-" + createdAt.UTC().Format("20060102T150405.000000000Z") + "-" + label + ".json.gz"
}

func validateFile(path string) (Document, string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return Document{}, "", 0, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return Document{}, "", 0, err
	}
	if stat.Size() <= 0 || stat.Size() > MaxCompressedSize {
		return Document{}, "", 0, errors.New("backup compressed size is invalid")
	}
	hash := sha256.New()
	document, err := Decode(io.TeeReader(file, hash))
	if err != nil {
		return Document{}, "", 0, err
	}
	return document, hex.EncodeToString(hash.Sum(nil)), stat.Size(), nil
}

func syncDirectory(directory string) {
	handle, err := os.Open(directory)
	if err == nil {
		_ = handle.Sync()
		_ = handle.Close()
	}
}
