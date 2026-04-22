// Package store handles all persistence against the platform database.
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// --- Domain types ----------------------------------------------------------

type Source struct {
	ID                  uuid.UUID
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
	Status              string
	LastError           string
	LastConnected       *time.Time
	LastSyncAt          *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type DiscoveredTable struct {
	ID                  uuid.UUID
	SourceID            uuid.UUID
	SchemaName          string
	TableName           string
	EstimatedRowCount   int64
	SyncEnabled         bool
	PrimaryKeyColumns   []string
	TimestampColumn     string
	LastSyncAt          *time.Time
	LastRowCount        int64
	LastSnapshotVersion int64
	DiscoveredAt        time.Time
	UpdatedAt           time.Time
}

type DiscoveredColumn struct {
	ID               uuid.UUID
	TableID          uuid.UUID
	ColumnName       string
	OrdinalPosition  int
	DataType         string
	MappedType       string
	MaxLength        int
	NumericPrecision int
	NumericScale     int
	IsNullable       bool
	IsPrimaryKey     bool
	IsUnique         bool
	HasDefault       bool
	DefaultValue     string
	SemanticType     string
}

type DiscoveredForeignKey struct {
	ID             uuid.UUID
	SourceID       uuid.UUID
	FromTableID    uuid.UUID
	FromColumn     string
	ToTableID      *uuid.UUID
	ToTableSchema  string
	ToTableName    string
	ToColumn       string
	ConstraintName string
}

type SyncJob struct {
	ID              uuid.UUID
	SourceID        uuid.UUID
	TableID         *uuid.UUID
	JobType         string
	Strategy        string
	Status          string
	StartedAt       *time.Time
	CompletedAt     *time.Time
	ErrorMessage    string
	RowsScanned     int64
	RowsInserted    int64
	RowsUpdated     int64
	RowsDeleted     int64
	RowsUnchanged   int64
	SnapshotVersion int64
	CreatedAt       time.Time
}

type IngestedRow struct {
	ID                 uuid.UUID
	TableID            uuid.UUID
	RowKey             string
	Data               map[string]any
	RowHash            string
	FirstSeenVersion   int64
	LastSeenVersion    int64
	CurrentVersion     int64
	SourceUpdatedAt    *time.Time
	IsDeleted          bool
	DeletedAt          *time.Time
}

type Change struct {
	ID              uuid.UUID
	TableID         uuid.UUID
	SyncJobID       *uuid.UUID
	Operation       string
	RowKey          string
	OldData         map[string]any
	NewData         map[string]any
	ChangedColumns  []string
	SnapshotVersion int64
	DetectedAt      time.Time
}

type Snapshot struct {
	ID          uuid.UUID
	TableID     uuid.UUID
	SyncJobID   *uuid.UUID
	Version     int64
	RowCount    int64
	Inserts     int64
	Updates     int64
	Deletes     int64
	CreatedAt   time.Time
}

// --- Store -----------------------------------------------------------------

type Store struct {
	db *pgxpool.Pool
}

func New(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

// === Sources ================================================================

func (s *Store) CreateSource(ctx context.Context, src *Source) error {
	const q = `
		INSERT INTO sources (
			id, name, description, db_type, host, port, database_name, username,
			password_encrypted, ssl_mode, sync_enabled, sync_interval_seconds,
			sync_strategy, status, created_at, updated_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,NOW(),NOW()
		)`
	src.ID = uuid.New()
	_, err := s.db.Exec(ctx, q,
		src.ID, src.Name, src.Description, src.DBType, src.Host, src.Port,
		src.DatabaseName, src.Username, src.Password, src.SSLMode,
		src.SyncEnabled, src.SyncIntervalSeconds, src.SyncStrategy, "pending",
	)
	return err
}

func (s *Store) GetSource(ctx context.Context, id uuid.UUID) (*Source, error) {
	const q = `
		SELECT id, name, description, db_type, host, port, database_name, username,
		       password_encrypted, ssl_mode, sync_enabled, sync_interval_seconds,
		       sync_strategy, status, COALESCE(last_error,''), last_connected, last_sync_at,
		       created_at, updated_at
		FROM sources WHERE id = $1`
	return scanSource(s.db.QueryRow(ctx, q, id))
}

func (s *Store) ListSources(ctx context.Context) ([]*Source, error) {
	const q = `
		SELECT id, name, description, db_type, host, port, database_name, username,
		       password_encrypted, ssl_mode, sync_enabled, sync_interval_seconds,
		       sync_strategy, status, COALESCE(last_error,''), last_connected, last_sync_at,
		       created_at, updated_at
		FROM sources ORDER BY created_at DESC`
	rows, err := s.db.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Source
	for rows.Next() {
		src, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, rows.Err()
}

func (s *Store) DeleteSource(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.Exec(ctx, `DELETE FROM sources WHERE id = $1`, id)
	return err
}

func (s *Store) UpdateSourceStatus(ctx context.Context, id uuid.UUID, status, lastErr string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE sources SET status=$1, last_error=$2, updated_at=NOW() WHERE id=$3`,
		status, lastErr, id,
	)
	return err
}

func (s *Store) UpdateSourceConnected(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`UPDATE sources SET last_connected=NOW(), status='connected', updated_at=NOW() WHERE id=$1`,
		id,
	)
	return err
}

func (s *Store) UpdateSourceSyncTime(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`UPDATE sources SET last_sync_at=NOW(), updated_at=NOW() WHERE id=$1`,
		id,
	)
	return err
}

func scanSource(row pgx.Row) (*Source, error) {
	var src Source
	err := row.Scan(
		&src.ID, &src.Name, &src.Description, &src.DBType, &src.Host, &src.Port,
		&src.DatabaseName, &src.Username, &src.Password, &src.SSLMode,
		&src.SyncEnabled, &src.SyncIntervalSeconds, &src.SyncStrategy,
		&src.Status, &src.LastError, &src.LastConnected, &src.LastSyncAt,
		&src.CreatedAt, &src.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &src, nil
}

// === Discovered Tables ======================================================

func (s *Store) UpsertTable(ctx context.Context, t *DiscoveredTable) error {
	const q = `
		INSERT INTO discovered_tables (
			id, source_id, schema_name, table_name, estimated_row_count,
			sync_enabled, primary_key_columns, timestamp_column,
			discovered_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NOW(),NOW())
		ON CONFLICT (source_id, schema_name, table_name) DO UPDATE SET
			estimated_row_count  = EXCLUDED.estimated_row_count,
			primary_key_columns  = EXCLUDED.primary_key_columns,
			timestamp_column     = EXCLUDED.timestamp_column,
			updated_at           = NOW()
		RETURNING id`
	t.ID = uuid.New()
	err := s.db.QueryRow(ctx, q,
		t.ID, t.SourceID, t.SchemaName, t.TableName, t.EstimatedRowCount,
		t.SyncEnabled, t.PrimaryKeyColumns, t.TimestampColumn,
	).Scan(&t.ID)
	return err
}

func (s *Store) GetTable(ctx context.Context, id uuid.UUID) (*DiscoveredTable, error) {
	const q = `
		SELECT id, source_id, schema_name, table_name, estimated_row_count,
		       sync_enabled, primary_key_columns, COALESCE(timestamp_column,''),
		       last_sync_at, last_row_count, last_snapshot_version,
		       discovered_at, updated_at
		FROM discovered_tables WHERE id = $1`
	return scanTable(s.db.QueryRow(ctx, q, id))
}

func (s *Store) ListTables(ctx context.Context, sourceID uuid.UUID) ([]*DiscoveredTable, error) {
	const q = `
		SELECT id, source_id, schema_name, table_name, estimated_row_count,
		       sync_enabled, primary_key_columns, COALESCE(timestamp_column,''),
		       last_sync_at, last_row_count, last_snapshot_version,
		       discovered_at, updated_at
		FROM discovered_tables WHERE source_id = $1
		ORDER BY schema_name, table_name`
	rows, err := s.db.Query(ctx, q, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*DiscoveredTable
	for rows.Next() {
		t, err := scanTable(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) UpdateTableSyncStats(ctx context.Context, id uuid.UUID, rowCount, snapshotVersion int64) error {
	_, err := s.db.Exec(ctx,
		`UPDATE discovered_tables SET last_sync_at=NOW(), last_row_count=$1, last_snapshot_version=$2, updated_at=NOW() WHERE id=$3`,
		rowCount, snapshotVersion, id,
	)
	return err
}

func scanTable(row pgx.Row) (*DiscoveredTable, error) {
	var t DiscoveredTable
	err := row.Scan(
		&t.ID, &t.SourceID, &t.SchemaName, &t.TableName, &t.EstimatedRowCount,
		&t.SyncEnabled, &t.PrimaryKeyColumns, &t.TimestampColumn,
		&t.LastSyncAt, &t.LastRowCount, &t.LastSnapshotVersion,
		&t.DiscoveredAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// === Discovered Columns =====================================================

func (s *Store) DeleteColumnsForTable(ctx context.Context, tableID uuid.UUID) error {
	_, err := s.db.Exec(ctx, `DELETE FROM discovered_columns WHERE table_id = $1`, tableID)
	return err
}

func (s *Store) InsertColumn(ctx context.Context, col *DiscoveredColumn) error {
	const q = `
		INSERT INTO discovered_columns (
			id, table_id, column_name, ordinal_position, data_type, mapped_type,
			max_length, numeric_precision, numeric_scale, is_nullable,
			is_primary_key, is_unique, has_default, default_value, semantic_type
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT (table_id, column_name) DO UPDATE SET
			ordinal_position  = EXCLUDED.ordinal_position,
			data_type         = EXCLUDED.data_type,
			mapped_type       = EXCLUDED.mapped_type,
			is_nullable       = EXCLUDED.is_nullable,
			is_primary_key    = EXCLUDED.is_primary_key,
			semantic_type     = EXCLUDED.semantic_type`
	col.ID = uuid.New()
	_, err := s.db.Exec(ctx, q,
		col.ID, col.TableID, col.ColumnName, col.OrdinalPosition,
		col.DataType, col.MappedType, col.MaxLength, col.NumericPrecision,
		col.NumericScale, col.IsNullable, col.IsPrimaryKey, col.IsUnique,
		col.HasDefault, col.DefaultValue, col.SemanticType,
	)
	return err
}

func (s *Store) ListColumns(ctx context.Context, tableID uuid.UUID) ([]*DiscoveredColumn, error) {
	const q = `
		SELECT id, table_id, column_name, ordinal_position, data_type, mapped_type,
		       max_length, numeric_precision, numeric_scale, is_nullable,
		       is_primary_key, is_unique, has_default, COALESCE(default_value,''), semantic_type
		FROM discovered_columns WHERE table_id = $1
		ORDER BY ordinal_position`
	rows, err := s.db.Query(ctx, q, tableID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*DiscoveredColumn
	for rows.Next() {
		var c DiscoveredColumn
		if err := rows.Scan(
			&c.ID, &c.TableID, &c.ColumnName, &c.OrdinalPosition,
			&c.DataType, &c.MappedType, &c.MaxLength, &c.NumericPrecision,
			&c.NumericScale, &c.IsNullable, &c.IsPrimaryKey, &c.IsUnique,
			&c.HasDefault, &c.DefaultValue, &c.SemanticType,
		); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

// === Foreign Keys ===========================================================

func (s *Store) DeleteFKsForSource(ctx context.Context, sourceID uuid.UUID) error {
	_, err := s.db.Exec(ctx, `DELETE FROM discovered_foreign_keys WHERE source_id = $1`, sourceID)
	return err
}

func (s *Store) InsertFK(ctx context.Context, fk *DiscoveredForeignKey) error {
	const q = `
		INSERT INTO discovered_foreign_keys (
			id, source_id, from_table_id, from_column, to_table_id,
			to_table_schema, to_table_name, to_column, constraint_name, discovered_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW())`
	fk.ID = uuid.New()
	_, err := s.db.Exec(ctx, q,
		fk.ID, fk.SourceID, fk.FromTableID, fk.FromColumn, fk.ToTableID,
		fk.ToTableSchema, fk.ToTableName, fk.ToColumn, fk.ConstraintName,
	)
	return err
}

// === Sync Jobs ==============================================================

func (s *Store) CreateSyncJob(ctx context.Context, job *SyncJob) error {
	const q = `
		INSERT INTO sync_jobs (id, source_id, table_id, job_type, strategy, status, created_at)
		VALUES ($1,$2,$3,$4,$5,'pending',NOW())`
	job.ID = uuid.New()
	_, err := s.db.Exec(ctx, q, job.ID, job.SourceID, job.TableID, job.JobType, job.Strategy)
	return err
}

func (s *Store) StartSyncJob(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.Exec(ctx,
		`UPDATE sync_jobs SET status='running', started_at=NOW() WHERE id=$1`, id,
	)
	return err
}

func (s *Store) CompleteSyncJob(ctx context.Context, id uuid.UUID, stats SyncJob) error {
	_, err := s.db.Exec(ctx, `
		UPDATE sync_jobs SET
			status='completed', completed_at=NOW(),
			rows_scanned=$1, rows_inserted=$2, rows_updated=$3,
			rows_deleted=$4, rows_unchanged=$5, snapshot_version=$6
		WHERE id=$7`,
		stats.RowsScanned, stats.RowsInserted, stats.RowsUpdated,
		stats.RowsDeleted, stats.RowsUnchanged, stats.SnapshotVersion, id,
	)
	return err
}

func (s *Store) FailSyncJob(ctx context.Context, id uuid.UUID, msg string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE sync_jobs SET status='failed', completed_at=NOW(), error_message=$1 WHERE id=$2`,
		msg, id,
	)
	return err
}

func (s *Store) GetSyncJob(ctx context.Context, id uuid.UUID) (*SyncJob, error) {
	const q = `
		SELECT id, source_id, table_id, job_type, strategy, status,
		       started_at, completed_at, COALESCE(error_message,''),
		       rows_scanned, rows_inserted, rows_updated, rows_deleted,
		       rows_unchanged, COALESCE(snapshot_version,0), created_at
		FROM sync_jobs WHERE id = $1`
	var j SyncJob
	err := s.db.QueryRow(ctx, q, id).Scan(
		&j.ID, &j.SourceID, &j.TableID, &j.JobType, &j.Strategy, &j.Status,
		&j.StartedAt, &j.CompletedAt, &j.ErrorMessage,
		&j.RowsScanned, &j.RowsInserted, &j.RowsUpdated, &j.RowsDeleted,
		&j.RowsUnchanged, &j.SnapshotVersion, &j.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func (s *Store) ListSyncJobs(ctx context.Context, sourceID uuid.UUID, limit int) ([]*SyncJob, error) {
	const q = `
		SELECT id, source_id, table_id, job_type, strategy, status,
		       started_at, completed_at, COALESCE(error_message,''),
		       rows_scanned, rows_inserted, rows_updated, rows_deleted,
		       rows_unchanged, COALESCE(snapshot_version,0), created_at
		FROM sync_jobs WHERE source_id = $1
		ORDER BY created_at DESC LIMIT $2`
	rows, err := s.db.Query(ctx, q, sourceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*SyncJob
	for rows.Next() {
		var j SyncJob
		if err := rows.Scan(
			&j.ID, &j.SourceID, &j.TableID, &j.JobType, &j.Strategy, &j.Status,
			&j.StartedAt, &j.CompletedAt, &j.ErrorMessage,
			&j.RowsScanned, &j.RowsInserted, &j.RowsUpdated, &j.RowsDeleted,
			&j.RowsUnchanged, &j.SnapshotVersion, &j.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, &j)
	}
	return out, rows.Err()
}

// === Ingested Rows ==========================================================

// UpsertRowResult is returned for each row processed during a sync.
type UpsertRowResult struct {
	IsNew     bool
	IsChanged bool
	OldHash   string
	OldData   map[string]any
}

func (s *Store) UpsertRow(ctx context.Context, tableID uuid.UUID, rowKey, rowHash string, data map[string]any, version int64) (UpsertRowResult, error) {
	// First, look up existing row.
	const lookup = `
		SELECT row_hash, data FROM ingested_rows
		WHERE table_id = $1 AND row_key = $2`

	var result UpsertRowResult
	var existingHash string
	var existingDataJSON []byte
	err := s.db.QueryRow(ctx, lookup, tableID, rowKey).Scan(&existingHash, &existingDataJSON)

	if err == pgx.ErrNoRows {
		// New row — INSERT.
		result.IsNew = true
		dataJSON, _ := json.Marshal(data)
		const ins = `
			INSERT INTO ingested_rows (
				id, table_id, row_key, data, row_hash,
				first_seen_version, last_seen_version, current_version,
				ingested_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$6,1,NOW(),NOW())`
		_, err = s.db.Exec(ctx, ins, uuid.New(), tableID, rowKey, dataJSON, rowHash, version)
		return result, err
	}
	if err != nil {
		return result, err
	}

	// Row exists — check if changed.
	if existingHash != rowHash {
		result.IsChanged = true
		result.OldHash = existingHash
		if existingDataJSON != nil {
			_ = json.Unmarshal(existingDataJSON, &result.OldData)
		}
		dataJSON, _ := json.Marshal(data)
		const upd = `
			UPDATE ingested_rows SET
				data=$1, row_hash=$2, last_seen_version=$3,
				current_version=current_version+1, updated_at=NOW(),
				is_deleted=FALSE, deleted_at=NULL
			WHERE table_id=$4 AND row_key=$5`
		_, err = s.db.Exec(ctx, upd, dataJSON, rowHash, version, tableID, rowKey)
	} else {
		// Unchanged — just update the version watermark.
		const touch = `
			UPDATE ingested_rows SET last_seen_version=$1, updated_at=NOW()
			WHERE table_id=$2 AND row_key=$3`
		_, err = s.db.Exec(ctx, touch, version, tableID, rowKey)
	}
	return result, err
}

// MarkDeletedRows marks all rows for tableID not touched in the current sync version as deleted.
// Returns the row keys of newly deleted rows (for change recording).
func (s *Store) MarkDeletedRows(ctx context.Context, tableID uuid.UUID, version int64) ([]string, error) {
	const q = `
		UPDATE ingested_rows
		SET is_deleted=TRUE, deleted_at=NOW(), updated_at=NOW()
		WHERE table_id=$1 AND is_deleted=FALSE AND last_seen_version < $2
		RETURNING row_key`
	rows, err := s.db.Query(ctx, q, tableID, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *Store) GetDeletedRowData(ctx context.Context, tableID uuid.UUID, rowKey string) (map[string]any, error) {
	var dataJSON []byte
	err := s.db.QueryRow(ctx,
		`SELECT data FROM ingested_rows WHERE table_id=$1 AND row_key=$2`,
		tableID, rowKey,
	).Scan(&dataJSON)
	if err != nil {
		return nil, err
	}
	var data map[string]any
	_ = json.Unmarshal(dataJSON, &data)
	return data, nil
}

func (s *Store) QueryRows(ctx context.Context, tableID uuid.UUID, limit, offset int) ([]map[string]any, error) {
	const q = `
		SELECT data FROM ingested_rows
		WHERE table_id=$1 AND is_deleted=FALSE
		ORDER BY ingested_at DESC
		LIMIT $2 OFFSET $3`
	rows, err := s.db.Query(ctx, q, tableID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var dataJSON []byte
		if err := rows.Scan(&dataJSON); err != nil {
			return nil, err
		}
		var row map[string]any
		_ = json.Unmarshal(dataJSON, &row)
		out = append(out, row)
	}
	return out, rows.Err()
}

// === Changes ================================================================

func (s *Store) InsertChange(ctx context.Context, c *Change) error {
	const q = `
		INSERT INTO changes (id, table_id, sync_job_id, operation, row_key,
		                     old_data, new_data, changed_columns, snapshot_version, detected_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NOW())`
	c.ID = uuid.New()
	oldJSON, _ := json.Marshal(c.OldData)
	newJSON, _ := json.Marshal(c.NewData)
	_, err := s.db.Exec(ctx, q,
		c.ID, c.TableID, c.SyncJobID, c.Operation, c.RowKey,
		oldJSON, newJSON, c.ChangedColumns, c.SnapshotVersion,
	)
	return err
}

func (s *Store) ListChanges(ctx context.Context, tableID uuid.UUID, limit int) ([]*Change, error) {
	const q = `
		SELECT id, table_id, sync_job_id, operation, row_key,
		       old_data, new_data, changed_columns, snapshot_version, detected_at
		FROM changes WHERE table_id=$1
		ORDER BY detected_at DESC LIMIT $2`
	rows, err := s.db.Query(ctx, q, tableID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Change
	for rows.Next() {
		var c Change
		var oldJSON, newJSON []byte
		if err := rows.Scan(
			&c.ID, &c.TableID, &c.SyncJobID, &c.Operation, &c.RowKey,
			&oldJSON, &newJSON, &c.ChangedColumns, &c.SnapshotVersion, &c.DetectedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(oldJSON, &c.OldData)
		_ = json.Unmarshal(newJSON, &c.NewData)
		out = append(out, &c)
	}
	return out, rows.Err()
}

// === Snapshots ==============================================================

func (s *Store) CreateSnapshot(ctx context.Context, snap *Snapshot) error {
	const q = `
		INSERT INTO snapshots (id, table_id, sync_job_id, version, row_count, inserts, updates, deletes, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NOW())
		ON CONFLICT (table_id, version) DO NOTHING`
	snap.ID = uuid.New()
	_, err := s.db.Exec(ctx, q,
		snap.ID, snap.TableID, snap.SyncJobID, snap.Version,
		snap.RowCount, snap.Inserts, snap.Updates, snap.Deletes,
	)
	return err
}

func (s *Store) ListSnapshots(ctx context.Context, tableID uuid.UUID, limit int) ([]*Snapshot, error) {
	const q = `
		SELECT id, table_id, sync_job_id, version, row_count, inserts, updates, deletes, created_at
		FROM snapshots WHERE table_id=$1
		ORDER BY version DESC LIMIT $2`
	rows, err := s.db.Query(ctx, q, tableID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Snapshot
	for rows.Next() {
		var snap Snapshot
		if err := rows.Scan(
			&snap.ID, &snap.TableID, &snap.SyncJobID, &snap.Version,
			&snap.RowCount, &snap.Inserts, &snap.Updates, &snap.Deletes, &snap.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, &snap)
	}
	return out, rows.Err()
}

// ListSyncEnabledSources returns sources that have sync enabled.
func (s *Store) ListSyncEnabledSources(ctx context.Context) ([]*Source, error) {
	const q = `
		SELECT id, name, description, db_type, host, port, database_name, username,
		       password_encrypted, ssl_mode, sync_enabled, sync_interval_seconds,
		       sync_strategy, status, COALESCE(last_error,''), last_connected, last_sync_at,
		       created_at, updated_at
		FROM sources WHERE sync_enabled=TRUE ORDER BY created_at`
	rows, err := s.db.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Source
	for rows.Next() {
		src, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, src)
	}
	return out, rows.Err()
}

// TableByName looks up a discovered table by source + schema + name.
func (s *Store) TableByName(ctx context.Context, sourceID uuid.UUID, schema, name string) (*DiscoveredTable, error) {
	const q = `
		SELECT id, source_id, schema_name, table_name, estimated_row_count,
		       sync_enabled, primary_key_columns, COALESCE(timestamp_column,''),
		       last_sync_at, last_row_count, last_snapshot_version,
		       discovered_at, updated_at
		FROM discovered_tables
		WHERE source_id=$1 AND schema_name=$2 AND table_name=$3`
	return scanTable(s.db.QueryRow(ctx, q, sourceID, schema, name))
}

// RowCount returns the number of live (non-deleted) rows for a table.
func (s *Store) RowCount(ctx context.Context, tableID uuid.UUID) (int64, error) {
	var n int64
	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM ingested_rows WHERE table_id=$1 AND is_deleted=FALSE`,
		tableID,
	).Scan(&n)
	return n, err
}

// FKTableID resolves the platform table ID for a given schema+name, or nil if not found.
func (s *Store) FKTableID(ctx context.Context, sourceID uuid.UUID, schema, name string) (*uuid.UUID, error) {
	t, err := s.TableByName(ctx, sourceID, schema, name)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t.ID, nil
}

// lastErrorMessage converts an error to a string, handling nil.
func lastErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// ensure lastErrorMessage is used somewhere (avoids lint warning).
var _ = fmt.Sprintf("%s", lastErrorMessage(nil))
