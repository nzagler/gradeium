package backups

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
)

const schedulerPollInterval = time.Minute

type Scheduler struct {
	service *Service
	logger  *slog.Logger
}

func NewScheduler(service *Service, logger *slog.Logger) *Scheduler {
	return &Scheduler{service: service, logger: logger}
}

func (scheduler *Scheduler) Run(ctx context.Context) {
	scheduler.runAndLog(ctx)
	ticker := time.NewTicker(schedulerPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			scheduler.logger.Info("backup scheduler stopped")
			return
		case <-ticker.C:
			scheduler.runAndLog(ctx)
		}
	}
}

func (scheduler *Scheduler) runAndLog(ctx context.Context) {
	if err := scheduler.RunOnce(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrBusy) {
		scheduler.logger.Warn("automatic backup attempt failed", "error", safeSchedulerError(err))
	}
}

func (scheduler *Scheduler) RunOnce(ctx context.Context) error {
	return scheduler.service.store.WithSchedulerLock(ctx, func(ctx context.Context) error {
		state, err := scheduler.service.store.BackupSettings(ctx)
		if err != nil {
			return err
		}
		now := scheduler.service.now()
		if !state.Enabled || state.NextDueAt == nil || state.NextDueAt.After(now) {
			return nil
		}
		metadata, err := scheduler.service.Create(ctx, KindAutomatic)
		if err != nil {
			scheduler.recordFailure(now, "Automatic backup could not be completed.")
			return err
		}
		if err := scheduler.service.store.RecordAutomaticSuccess(ctx, metadata.CreatedAt); err != nil {
			return err
		}
		if err := scheduler.service.ApplyRetention(ctx, state.RetentionCount); err != nil {
			scheduler.recordFailure(now, "Automatic backup completed, but retention cleanup failed.")
			return err
		}
		scheduler.logger.Info("automatic backup completed", "backup_id", metadata.ID, "size_bytes", metadata.SizeBytes)
		return nil
	})
}

func (scheduler *Scheduler) recordFailure(attemptedAt time.Time, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := scheduler.service.store.RecordAutomaticFailure(ctx, attemptedAt, message); err != nil {
		scheduler.logger.Warn("automatic backup failure status could not be recorded", "error", safeSchedulerError(err))
	}
}

func safeSchedulerError(err error) string {
	message := strings.TrimSpace(err.Error())
	if len(message) > 240 {
		return message[:240]
	}
	return message
}
