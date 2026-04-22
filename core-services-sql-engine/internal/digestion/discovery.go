package digestion

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/manju-backend/core-services-sql-engine/internal/digestion/store"
)

// Discover connects to the source, introspects its schema, and persists the
// catalog of tables, columns, and foreign keys to the platform DB.
func (e *Engine) Discover(ctx context.Context, sourceID uuid.UUID) error {
	src, err := e.store.GetSource(ctx, sourceID)
	if err != nil {
		return fmt.Errorf("get source: %w", err)
	}

	conn, err := e.connectorForSource(ctx, src)
	if err != nil {
		_ = e.store.UpdateSourceStatus(ctx, sourceID, "error", err.Error())
		return fmt.Errorf("connect to source: %w", err)
	}
	defer conn.Close()

	if err := e.store.UpdateSourceConnected(ctx, sourceID); err != nil {
		slog.Error("update source connected", "err", err)
	}

	// --- Discover tables ---------------------------------------------------
	tables, err := conn.DiscoverTables(ctx)
	if err != nil {
		_ = e.store.UpdateSourceStatus(ctx, sourceID, "error", err.Error())
		return fmt.Errorf("discover tables: %w", err)
	}

	slog.Info("discovered tables", "source", src.Name, "count", len(tables))

	// Build a map of schema.table → platform table ID for FK resolution.
	tableIDMap := make(map[string]uuid.UUID, len(tables))

	for _, t := range tables {
		dt := &store.DiscoveredTable{
			SourceID:          sourceID,
			SchemaName:        t.Schema,
			TableName:         t.Name,
			EstimatedRowCount: t.EstimatedRows,
			SyncEnabled:       true,
			PrimaryKeyColumns: t.PrimaryKeys,
			TimestampColumn:   t.TimestampColumn,
		}
		if err := e.store.UpsertTable(ctx, dt); err != nil {
			return fmt.Errorf("upsert table %s.%s: %w", t.Schema, t.Name, err)
		}
		tableIDMap[t.Schema+"."+t.Name] = dt.ID

		// --- Discover columns for this table --------------------------------
		cols, err := conn.DiscoverColumns(ctx, t.Schema, t.Name)
		if err != nil {
			slog.Warn("discover columns failed", "table", t.Name, "err", err)
			continue
		}
		for _, c := range cols {
			sc := &store.DiscoveredColumn{
				TableID:          dt.ID,
				ColumnName:       c.Name,
				OrdinalPosition:  c.OrdinalPosition,
				DataType:         c.DataType,
				MappedType:       c.MappedType,
				MaxLength:        c.MaxLength,
				NumericPrecision: c.NumericPrecision,
				NumericScale:     c.NumericScale,
				IsNullable:       c.IsNullable,
				IsPrimaryKey:     c.IsPrimaryKey,
				IsUnique:         c.IsUnique,
				HasDefault:       c.HasDefault,
				DefaultValue:     c.DefaultValue,
				SemanticType:     c.SemanticType,
			}
			if err := e.store.InsertColumn(ctx, sc); err != nil {
				slog.Warn("insert column failed", "col", c.Name, "err", err)
			}
		}
	}

	// --- Discover foreign keys ---------------------------------------------
	fks, err := conn.DiscoverForeignKeys(ctx)
	if err != nil {
		slog.Warn("discover foreign keys failed", "err", err)
	} else {
		_ = e.store.DeleteFKsForSource(ctx, sourceID)
		for _, fk := range fks {
			fromID, ok := tableIDMap[fk.FromSchema+"."+fk.FromTable]
			if !ok {
				continue
			}
			toID := tableIDMap[fk.ToSchema+"."+fk.ToTable]
			sfk := &store.DiscoveredForeignKey{
				SourceID:       sourceID,
				FromTableID:    fromID,
				FromColumn:     fk.FromColumn,
				ToTableSchema:  fk.ToSchema,
				ToTableName:    fk.ToTable,
				ToColumn:       fk.ToColumn,
				ConstraintName: fk.ConstraintName,
			}
			if toID != uuid.Nil {
				sfk.ToTableID = &toID
			}
			if err := e.store.InsertFK(ctx, sfk); err != nil {
				slog.Warn("insert FK failed", "err", err)
			}
		}
	}

	_ = e.store.UpdateSourceStatus(ctx, sourceID, "discovered", "")
	slog.Info("discovery complete", "source", src.Name, "tables", len(tables))
	return nil
}
