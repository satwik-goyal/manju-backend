package digestion

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/manju-backend/core-services-sql-engine/internal/digestion/store"
)

// SyncSource runs a full snapshot sync for all sync-enabled tables of a source.
func (e *Engine) SyncSource(ctx context.Context, sourceID uuid.UUID) error {
	tables, err := e.store.ListTables(ctx, sourceID)
	if err != nil {
		return fmt.Errorf("list tables: %w", err)
	}
	var firstErr error
	for _, t := range tables {
		if !t.SyncEnabled {
			continue
		}
		if _, err := e.SyncTable(ctx, sourceID, t.ID); err != nil {
			slog.Error("sync table failed", "table", t.TableName, "err", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if firstErr == nil {
		_ = e.store.UpdateSourceSyncTime(ctx, sourceID)
	}
	return firstErr
}

// SyncTable runs a full snapshot sync for a single table.
// It returns the completed SyncJob.
func (e *Engine) SyncTable(ctx context.Context, sourceID, tableID uuid.UUID) (*store.SyncJob, error) {
	src, err := e.store.GetSource(ctx, sourceID)
	if err != nil {
		return nil, fmt.Errorf("get source: %w", err)
	}
	table, err := e.store.GetTable(ctx, tableID)
	if err != nil {
		return nil, fmt.Errorf("get table: %w", err)
	}

	job := &store.SyncJob{
		SourceID: sourceID,
		TableID:  &tableID,
		JobType:  "snapshot",
		Strategy: "full",
	}
	if err := e.store.CreateSyncJob(ctx, job); err != nil {
		return nil, fmt.Errorf("create sync job: %w", err)
	}
	if err := e.store.StartSyncJob(ctx, job.ID); err != nil {
		return nil, fmt.Errorf("start sync job: %w", err)
	}

	slog.Info("sync started", "source", src.Name, "table", table.TableName, "job", job.ID)

	stats, err := e.runFullSync(ctx, src, table, job.ID)
	if err != nil {
		_ = e.store.FailSyncJob(ctx, job.ID, err.Error())
		_ = e.store.UpdateSourceStatus(ctx, sourceID, "error", err.Error())
		return nil, fmt.Errorf("run full sync: %w", err)
	}

	if err := e.store.CompleteSyncJob(ctx, job.ID, stats); err != nil {
		slog.Error("complete sync job", "err", err)
	}
	if err := e.store.UpdateTableSyncStats(ctx, tableID, stats.RowsScanned, stats.SnapshotVersion); err != nil {
		slog.Error("update table sync stats", "err", err)
	}

	snap := &store.Snapshot{
		TableID:   tableID,
		SyncJobID: &job.ID,
		Version:   stats.SnapshotVersion,
		RowCount:  stats.RowsScanned,
		Inserts:   stats.RowsInserted,
		Updates:   stats.RowsUpdated,
		Deletes:   stats.RowsDeleted,
	}
	if err := e.store.CreateSnapshot(ctx, snap); err != nil {
		slog.Error("create snapshot", "err", err)
	}

	slog.Info("sync complete",
		"table", table.TableName,
		"inserted", stats.RowsInserted,
		"updated", stats.RowsUpdated,
		"deleted", stats.RowsDeleted,
		"unchanged", stats.RowsUnchanged,
	)
	return e.store.GetSyncJob(ctx, job.ID)
}

func (e *Engine) runFullSync(
	ctx context.Context,
	src *store.Source,
	table *store.DiscoveredTable,
	jobID uuid.UUID,
) (store.SyncJob, error) {
	version := time.Now().Unix()
	stats := store.SyncJob{SnapshotVersion: version}

	conn, err := e.connectorForSource(ctx, src)
	if err != nil {
		return stats, err
	}
	defer conn.Close()

	rowCh, errCh := conn.StreamRows(ctx, table.SchemaName, table.TableName, nil)

	for row := range rowCh {
		rowKey := buildRowKey(row, table.PrimaryKeyColumns)
		rowHash := computeHash(row)

		result, err := e.store.UpsertRow(ctx, table.ID, rowKey, rowHash, row, version)
		if err != nil {
			return stats, fmt.Errorf("upsert row %q: %w", rowKey, err)
		}
		stats.RowsScanned++

		switch {
		case result.IsNew:
			stats.RowsInserted++
			_ = e.store.InsertChange(ctx, &store.Change{
				TableID:         table.ID,
				SyncJobID:       &jobID,
				Operation:       "INSERT",
				RowKey:          rowKey,
				NewData:         row,
				SnapshotVersion: version,
			})

		case result.IsChanged:
			stats.RowsUpdated++
			cols := changedColumns(result.OldData, row)
			_ = e.store.InsertChange(ctx, &store.Change{
				TableID:         table.ID,
				SyncJobID:       &jobID,
				Operation:       "UPDATE",
				RowKey:          rowKey,
				OldData:         result.OldData,
				NewData:         row,
				ChangedColumns:  cols,
				SnapshotVersion: version,
			})

		default:
			stats.RowsUnchanged++
		}
	}

	// Check for streaming errors.
	if err := <-errCh; err != nil {
		return stats, fmt.Errorf("stream rows: %w", err)
	}

	// Mark rows not seen in this sync as deleted.
	deletedKeys, err := e.store.MarkDeletedRows(ctx, table.ID, version)
	if err != nil {
		return stats, fmt.Errorf("mark deleted: %w", err)
	}
	stats.RowsDeleted = int64(len(deletedKeys))

	for _, key := range deletedKeys {
		oldData, _ := e.store.GetDeletedRowData(ctx, table.ID, key)
		_ = e.store.InsertChange(ctx, &store.Change{
			TableID:         table.ID,
			SyncJobID:       &jobID,
			Operation:       "DELETE",
			RowKey:          key,
			OldData:         oldData,
			SnapshotVersion: version,
		})
	}

	return stats, nil
}

// buildRowKey produces a stable string key from the primary key column values.
// Falls back to all column values if no PKs are defined.
func buildRowKey(row map[string]any, pkColumns []string) string {
	cols := pkColumns
	if len(cols) == 0 {
		// No PK: use all columns as the key (expensive but correct).
		cols = make([]string, 0, len(row))
		for k := range row {
			cols = append(cols, k)
		}
	}
	parts := make([]string, len(cols))
	for i, col := range cols {
		parts[i] = fmt.Sprintf("%v", row[col])
	}
	return strings.Join(parts, "|")
}
