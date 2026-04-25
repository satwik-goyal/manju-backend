package filestore

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// EventType identifies a file lifecycle event.
type EventType string

const (
	EventFileUploaded   EventType = "file.uploaded"
	EventFileProcessing EventType = "file.processing"
	EventFileProcessed  EventType = "file.processed"
	EventFileDeleted    EventType = "file.deleted"
)

// Event is emitted on file lifecycle changes so that other kernel services
// (e.g. the SQL Digestion Engine) can react without polling.
type Event struct {
	Type      EventType
	FileID    uuid.UUID
	TenantID  uuid.UUID
	Timestamp time.Time
	// Metadata contains event-specific fields (category, size, checksum, etc.).
	Metadata map[string]any
}

// EventEmitter receives file lifecycle events.
// Implementations should be non-blocking or handle back-pressure internally.
// Use NoOpEmitter when event emission is not required.
type EventEmitter interface {
	Emit(ctx context.Context, event Event) error
}

// NoOpEmitter discards all events. This is the default emitter.
type NoOpEmitter struct{}

func (NoOpEmitter) Emit(_ context.Context, _ Event) error { return nil }

// LogEmitter logs events via slog. Useful for development and testing.
type LogEmitter struct {
	Logger *slog.Logger
}

func (e *LogEmitter) Emit(_ context.Context, ev Event) error {
	e.Logger.Info("file event",
		"type", string(ev.Type),
		"file_id", ev.FileID.String(),
		"tenant_id", ev.TenantID.String(),
		"timestamp", ev.Timestamp,
	)
	return nil
}

// ChanEmitter sends events to an unbuffered or buffered channel.
// Useful for unit tests that need to assert event emission.
type ChanEmitter struct {
	Ch chan<- Event
}

func (e *ChanEmitter) Emit(_ context.Context, ev Event) error {
	e.Ch <- ev
	return nil
}
