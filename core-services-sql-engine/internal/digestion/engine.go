// Package digestion is the core SQL Digestion Engine library.
// It knows nothing about HTTP or CLI — callers wire those up.
package digestion

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/manju-backend/core-services-sql-engine/internal/digestion/connector"
	"github.com/manju-backend/core-services-sql-engine/internal/digestion/store"
)

// Engine orchestrates discovery, sync, and catalog queries.
type Engine struct {
	store *store.Store
}

// New creates an Engine backed by the given platform DB pool.
func New(platformDB *pgxpool.Pool) *Engine {
	return &Engine{store: store.New(platformDB)}
}

// Store exposes the underlying store for use by the scheduler.
func (e *Engine) Store() *store.Store {
	return e.store
}

// RegisterSource persists a new source and returns it.
func (e *Engine) RegisterSource(ctx context.Context, req RegisterSourceRequest) (*store.Source, error) {
	src := &store.Source{
		Name:                req.Name,
		Description:         req.Description,
		DBType:              req.DBType,
		Host:                req.Host,
		Port:                req.Port,
		DatabaseName:        req.DatabaseName,
		Username:            req.Username,
		Password:            req.Password,
		SSLMode:             req.SSLMode,
		SyncEnabled:         req.SyncEnabled,
		SyncIntervalSeconds: req.SyncIntervalSeconds,
		SyncStrategy:        req.SyncStrategy,
	}
	if src.SSLMode == "" {
		src.SSLMode = "disable"
	}
	if src.SyncIntervalSeconds == 0 {
		src.SyncIntervalSeconds = 900
	}
	if src.SyncStrategy == "" {
		src.SyncStrategy = "full"
	}

	if err := e.store.CreateSource(ctx, src); err != nil {
		return nil, fmt.Errorf("register source: %w", err)
	}
	return src, nil
}

// GetSource returns a single source by ID.
func (e *Engine) GetSource(ctx context.Context, id uuid.UUID) (*store.Source, error) {
	return e.store.GetSource(ctx, id)
}

// ListSources returns all registered sources.
func (e *Engine) ListSources(ctx context.Context) ([]*store.Source, error) {
	return e.store.ListSources(ctx)
}

// DeleteSource removes a source and all its discovered data (cascade).
func (e *Engine) DeleteSource(ctx context.Context, id uuid.UUID) error {
	return e.store.DeleteSource(ctx, id)
}

// connectorForSource builds and opens a Connector for the given source.
func (e *Engine) connectorForSource(ctx context.Context, src *store.Source) (connector.Connector, error) {
	cfg := connector.SourceConfig{
		Type:     src.DBType,
		Host:     src.Host,
		Port:     src.Port,
		Database: src.DatabaseName,
		Username: src.Username,
		Password: src.Password,
		SSLMode:  src.SSLMode,
	}
	conn, err := connector.New(cfg)
	if err != nil {
		return nil, err
	}
	if err := conn.Connect(ctx); err != nil {
		return nil, err
	}
	return conn, nil
}

// RegisterSourceRequest is the input DTO for RegisterSource.
type RegisterSourceRequest struct {
	Name                string
	Description         string
	DBType              string
	Host                string
	Port                int
	DatabaseName        string
	Username            string
	Password            string
	SSLMode             string
	SyncEnabled         bool
	SyncIntervalSeconds int
	SyncStrategy        string
}
