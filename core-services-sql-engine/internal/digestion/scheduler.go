package digestion

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
)

// Scheduler runs periodic syncs for all sync-enabled sources.
type Scheduler struct {
	engine  *Engine
	cron    *cron.Cron
	entries map[uuid.UUID]cron.EntryID
	mu      sync.Mutex
}

// NewScheduler creates a scheduler backed by the given engine.
func NewScheduler(engine *Engine) *Scheduler {
	return &Scheduler{
		engine:  engine,
		cron:    cron.New(),
		entries: make(map[uuid.UUID]cron.EntryID),
	}
}

// Start loads all sync-enabled sources and schedules them, then starts the cron loop.
func (s *Scheduler) Start(ctx context.Context) error {
	sources, err := s.engine.store.ListSyncEnabledSources(ctx)
	if err != nil {
		return fmt.Errorf("load sync-enabled sources: %w", err)
	}

	for _, src := range sources {
		if err := s.ScheduleSource(src.ID, src.SyncIntervalSeconds); err != nil {
			slog.Warn("schedule source failed", "source", src.Name, "err", err)
		}
	}

	s.cron.Start()
	slog.Info("scheduler started", "sources", len(sources))
	return nil
}

// Stop halts the scheduler gracefully.
func (s *Scheduler) Stop() {
	s.cron.Stop()
}

// ScheduleSource adds (or replaces) the sync schedule for a source.
func (s *Scheduler) ScheduleSource(sourceID uuid.UUID, intervalSeconds int) error {
	if intervalSeconds < 60 {
		intervalSeconds = 60
	}
	spec := fmt.Sprintf("@every %ds", intervalSeconds)

	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove old entry if present.
	if old, ok := s.entries[sourceID]; ok {
		s.cron.Remove(old)
	}

	entryID, err := s.cron.AddFunc(spec, func() {
		ctx := context.Background()
		slog.Info("scheduled sync starting", "source", sourceID)
		if err := s.engine.SyncSource(ctx, sourceID); err != nil {
			slog.Error("scheduled sync failed", "source", sourceID, "err", err)
		}
	})
	if err != nil {
		return fmt.Errorf("add cron func for source %s: %w", sourceID, err)
	}

	s.entries[sourceID] = entryID
	slog.Info("source scheduled", "source", sourceID, "interval_seconds", intervalSeconds)
	return nil
}

// UnscheduleSource removes a source from the scheduler.
func (s *Scheduler) UnscheduleSource(sourceID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.entries[sourceID]; ok {
		s.cron.Remove(id)
		delete(s.entries, sourceID)
	}
}
