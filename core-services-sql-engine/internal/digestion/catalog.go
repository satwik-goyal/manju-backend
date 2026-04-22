package digestion

import (
	"context"

	"github.com/google/uuid"
	"github.com/manju-backend/core-services-sql-engine/internal/digestion/store"
)

// GetTables returns all discovered tables for a source.
func (e *Engine) GetTables(ctx context.Context, sourceID uuid.UUID) ([]*store.DiscoveredTable, error) {
	return e.store.ListTables(ctx, sourceID)
}

// GetTable returns a single discovered table.
func (e *Engine) GetTable(ctx context.Context, tableID uuid.UUID) (*store.DiscoveredTable, error) {
	return e.store.GetTable(ctx, tableID)
}

// GetColumns returns all columns for a table.
func (e *Engine) GetColumns(ctx context.Context, tableID uuid.UUID) ([]*store.DiscoveredColumn, error) {
	return e.store.ListColumns(ctx, tableID)
}

// GetData returns ingested rows for a table, with pagination.
func (e *Engine) GetData(ctx context.Context, tableID uuid.UUID, limit, offset int) ([]map[string]any, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	return e.store.QueryRows(ctx, tableID, limit, offset)
}

// GetChanges returns the most recent change events for a table.
func (e *Engine) GetChanges(ctx context.Context, tableID uuid.UUID, limit int) ([]*store.Change, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	return e.store.ListChanges(ctx, tableID, limit)
}

// GetSnapshots returns snapshot history for a table.
func (e *Engine) GetSnapshots(ctx context.Context, tableID uuid.UUID, limit int) ([]*store.Snapshot, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return e.store.ListSnapshots(ctx, tableID, limit)
}

// GetSyncJobs returns recent sync job history for a source.
func (e *Engine) GetSyncJobs(ctx context.Context, sourceID uuid.UUID, limit int) ([]*store.SyncJob, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return e.store.ListSyncJobs(ctx, sourceID, limit)
}

// GetSyncJob returns a single sync job.
func (e *Engine) GetSyncJob(ctx context.Context, jobID uuid.UUID) (*store.SyncJob, error) {
	return e.store.GetSyncJob(ctx, jobID)
}
