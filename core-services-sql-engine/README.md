# SQL Digestion Engine

I am building the **SQL Digestion Engine** as the first component. This is the 
foundation that enables everything else — before ontology mapping, before plugins, 
before the map, I need data flowing into the platform.

The digestion engine:
1. **DISCOVER** — Connect to a source database, introspect all tables, columns, 
   types, primary keys, foreign keys, detect timestamp columns and semantic types
2. **SNAPSHOT** — Full initial copy of all data into platform DB as JSONB rows
3. **TRACK** — Continuously detect changes via timestamp-based incremental sync, 
   full-table hash comparison, or row-level hash diffing
4. **VERSION** — Every sync creates a versioned snapshot enabling time-travel queries
5. **CATALOG** — Maintain registry of all sources, tables, columns, sync status

The engine is a Go library with three interfaces:
- **HTTP API** (Echo) — for frontend and external integrations
- **Background Scheduler** — runs syncs on configured intervals
- **CLI** (future) — for developer convenience

The connector system is interface-based, starting with PostgreSQL driver, 
with MySQL and MSSQL planned.

Connects to external SQL databases, discovers their schema, ingests all row data into the platform database, and tracks changes over time.

**Capabilities:** DISCOVER · SNAPSHOT · TRACK · VERSION · CATALOG

---

## Running

```bash
# Start platform DB + source DB
cd core-services-database && docker compose up -d

# Run the server (defaults: port 8080, platform DSN = local manju DB)
cd core-services-sql-engine && go run ./cmd/server

# Override via env
PLATFORM_DB_DSN="host=... dbname=manju user=manju password=manju sslmode=disable" \
PORT=9000 go run ./cmd/server
```

---

## API Reference

All responses are JSON. Success responses wrap the payload in a `"data"` key. Errors wrap a message in an `"error"` key.

```json
// success
{ "data": <payload> }

// error
{ "error": "message" }
```

---

### Health

#### `GET /health`

Returns server liveness. No auth required.

**Response `200`**
```json
{
  "data": { "status": "ok" }
}
```

---

### Sources

A **source** is a registered external database. The engine connects to it for discovery and sync.

#### `GET /sources`

List all registered sources.

**Response `200`**
```json
{
  "data": [
    {
      "id": "a1b2c3d4-...",
      "name": "Warehouse DB",
      "description": "Primary warehouse operations database",
      "db_type": "postgres",
      "host": "localhost",
      "port": 5433,
      "database_name": "warehouse",
      "username": "warehouse_user",
      "password": "warehouse",
      "ssl_mode": "disable",
      "sync_enabled": true,
      "sync_interval_seconds": 900,
      "sync_strategy": "full",
      "status": "connected",
      "last_error": "",
      "last_connected": "2026-04-24T10:00:00Z",
      "last_sync_at": "2026-04-24T10:15:00Z",
      "created_at": "2026-04-24T09:00:00Z",
      "updated_at": "2026-04-24T10:15:00Z"
    }
  ]
}
```

Sources are ordered by `created_at DESC`. Returns an empty array `[]` when none exist.

---

#### `POST /sources`

Register a new external database source.

**Request body**
```json
{
  "Name": "Warehouse DB",
  "Description": "Primary warehouse operations database",
  "DBType": "postgres",
  "Host": "localhost",
  "Port": 5433,
  "DatabaseName": "warehouse",
  "Username": "warehouse_user",
  "Password": "warehouse",
  "SSLMode": "disable",
  "SyncEnabled": true,
  "SyncIntervalSeconds": 900,
  "SyncStrategy": "full"
}
```

| Field | Required | Default | Notes |
|---|---|---|---|
| `Name` | yes | — | Human-readable label |
| `DBType` | yes | — | `"postgres"` (only supported type) |
| `Host` | yes | — | Hostname or IP |
| `DatabaseName` | yes | — | Target database name |
| `Description` | no | `""` | |
| `Port` | no | `5432` | |
| `Username` | no | `""` | |
| `Password` | no | `""` | |
| `SSLMode` | no | `"disable"` | `disable` · `require` · `verify-full` |
| `SyncEnabled` | no | `false` | If true, auto-schedules recurring syncs |
| `SyncIntervalSeconds` | no | `900` | Minimum `60`. Ignored if `SyncEnabled=false` |
| `SyncStrategy` | no | `"full"` | Only `"full"` is currently supported |

**Behavior**
- Source is persisted with `status: "pending"`.
- If `SyncEnabled=true`, the background scheduler immediately schedules recurring syncs at `SyncIntervalSeconds` interval.
- No connection is made at registration time — connection happens on first discover/sync.

**Response `201`**
```json
{
  "data": {
    "id": "a1b2c3d4-...",
    "name": "Warehouse DB",
    "db_type": "postgres",
    "host": "localhost",
    "port": 5433,
    "database_name": "warehouse",
    "username": "warehouse_user",
    "password": "warehouse",
    "ssl_mode": "disable",
    "sync_enabled": true,
    "sync_interval_seconds": 900,
    "sync_strategy": "full",
    "status": "pending",
    "last_error": "",
    "last_connected": null,
    "last_sync_at": null,
    "created_at": "2026-04-24T09:00:00Z",
    "updated_at": "2026-04-24T09:00:00Z"
  }
}
```

**Error `400`** — missing required fields
```json
{ "error": "name, db_type, host, and database_name are required" }
```

---

#### `GET /sources/:sourceId`

Fetch a single source by UUID.

**Response `200`** — same shape as a single item from `GET /sources`

**Error `400`** — malformed UUID
```json
{ "error": "invalid source ID" }
```

**Error `404`** — source not found
```json
{ "error": "source not found" }
```

---

#### `DELETE /sources/:sourceId`

Remove a source and all its associated data (tables, columns, foreign keys, ingested rows, changes, snapshots, sync jobs) via cascade.

**Behavior**
- Immediately unschedules recurring syncs for this source.
- Deletes the source record. All child records are removed by the platform DB's `ON DELETE CASCADE` constraint.

**Response `204`** — no body

**Error `400`** — malformed UUID
```json
{ "error": "invalid source ID" }
```

---

### Discovery

Discovery connects to the source, introspects the schema, and saves tables, columns, and foreign keys to the platform database. It does **not** ingest row data.

#### `POST /sources/:sourceId/discover`

Run schema discovery for a source.

**Request body** — none

**Behavior**
1. Opens a connection to the source database.
2. Queries `pg_class`/`pg_namespace` to enumerate all user tables.
3. For each table: detects primary key columns and guesses the best timestamp column (`updated_at`, `created_at`, etc.).
4. Upserts each table into `discovered_tables` (idempotent — re-running updates `estimated_row_count` and PK info).
5. Deletes old columns for each table and re-inserts from `information_schema.columns`, with type mapping and semantic type detection.
6. Detects all foreign keys and stores them in `discovered_foreign_keys`, cross-referencing platform table IDs.
7. Returns the full list of discovered tables.

**Response `200`**
```json
{
  "data": {
    "tables": [
      {
        "id": "b2c3d4e5-...",
        "source_id": "a1b2c3d4-...",
        "schema_name": "public",
        "table_name": "warehouses",
        "estimated_row_count": 4,
        "sync_enabled": true,
        "primary_key_columns": ["id"],
        "timestamp_column": "updated_at",
        "last_sync_at": null,
        "last_row_count": 0,
        "last_snapshot_version": 0,
        "discovered_at": "2026-04-24T09:05:00Z",
        "updated_at": "2026-04-24T09:05:00Z"
      }
    ]
  }
}
```

**Error `500`** — connection failure or query error
```json
{ "error": "dial tcp ...: connection refused" }
```

---

### Tables

#### `GET /sources/:sourceId/tables`

List all discovered tables for a source.

**Response `200`**
```json
{
  "data": [
    {
      "id": "b2c3d4e5-...",
      "source_id": "a1b2c3d4-...",
      "schema_name": "public",
      "table_name": "inventory",
      "estimated_row_count": 500,
      "sync_enabled": true,
      "primary_key_columns": ["id"],
      "timestamp_column": "updated_at",
      "last_sync_at": "2026-04-24T10:15:00Z",
      "last_row_count": 498,
      "last_snapshot_version": 1745492100,
      "discovered_at": "2026-04-24T09:05:00Z",
      "updated_at": "2026-04-24T10:15:00Z"
    }
  ]
}
```

Tables are ordered by `schema_name, table_name`. Returns an empty array when none discovered yet.

---

#### `GET /sources/:sourceId/tables/:tableId`

Fetch a single discovered table by UUID.

**Response `200`** — same shape as a single item from `GET /sources/:sourceId/tables`

**Error `404`**
```json
{ "error": "table not found" }
```

---

#### `GET /sources/:sourceId/tables/:tableId/columns`

List all columns for a table.

**Response `200`**
```json
{
  "data": [
    {
      "id": "c3d4e5f6-...",
      "table_id": "b2c3d4e5-...",
      "column_name": "id",
      "ordinal_position": 1,
      "data_type": "uuid",
      "mapped_type": "uuid",
      "max_length": 0,
      "numeric_precision": 0,
      "numeric_scale": 0,
      "is_nullable": false,
      "is_primary_key": true,
      "is_unique": true,
      "has_default": true,
      "default_value": "gen_random_uuid()",
      "semantic_type": "primary_key"
    },
    {
      "id": "d4e5f6a7-...",
      "table_id": "b2c3d4e5-...",
      "column_name": "quantity",
      "ordinal_position": 5,
      "data_type": "integer",
      "mapped_type": "integer",
      "max_length": 0,
      "numeric_precision": 32,
      "numeric_scale": 0,
      "is_nullable": false,
      "is_primary_key": false,
      "is_unique": false,
      "has_default": false,
      "default_value": "",
      "semantic_type": ""
    }
  ]
}
```

Columns are ordered by `ordinal_position`.

**`mapped_type` values:** `text` · `integer` · `float` · `boolean` · `datetime` · `json` · `uuid` · `geometry`

**`semantic_type` values (heuristic, based on column name):**
| Value | Detected when column name contains |
|---|---|
| `primary_key` | is PK column |
| `foreign_key` | `_id` suffix (non-PK) |
| `updated_at` | `updated_at` / `modified_at` |
| `created_at` | `created_at` |
| `latitude` | `lat` / `latitude` |
| `longitude` | `lon` / `lng` / `longitude` |
| `status_enum` | `status` / `state` / `type` |
| `email` | `email` |
| `phone` | `phone` |
| `name` | `name` |

---

### Sync

Sync ingests rows from a source table into the platform DB (`ingested_rows`), detects inserts/updates/deletes, records a change log, and creates a versioned snapshot.

#### `POST /sources/:sourceId/sync`

Trigger a full sync of **all** tables under a source. Fire-and-forget — returns immediately.

**Request body** — none

**Behavior**
1. Returns `202 Accepted` immediately.
2. In a background goroutine, iterates through all discovered tables for the source and calls the same logic as `POST /sources/:id/tables/:tableId/sync` for each.
3. Progress can be tracked by polling `GET /sources/:sourceId/jobs`.

**Response `202`**
```json
{
  "message": "sync started"
}
```

---

#### `POST /sources/:sourceId/tables/:tableId/sync`

Sync a single table. Runs synchronously — the response is returned only after the sync completes.

**Request body** — none

**Behavior**
1. Creates a `SyncJob` record with `status: "pending"`.
2. Opens a connection to the source DB.
3. Streams all rows from the source table (memory-efficient — no full-table load).
4. For each row:
   - Computes a `row_key` from PK column values joined with `"|"`.
   - Computes a `row_hash` (SHA-256 of JSON-serialised row, Go-sorted keys for determinism).
   - Upserts into `ingested_rows`: records an `INSERT` or `UPDATE` change if the hash differs; touches `last_seen_version` if unchanged.
5. After streaming all rows, marks rows whose `last_seen_version` was not updated as deleted and records `DELETE` changes.
6. Creates a `Snapshot` record with row counts and delta statistics.
7. Updates `SyncJob` to `status: "completed"` with final stats.

**Response `200`** — the completed sync job
```json
{
  "data": {
    "id": "e5f6a7b8-...",
    "source_id": "a1b2c3d4-...",
    "table_id": "b2c3d4e5-...",
    "job_type": "table_sync",
    "strategy": "full",
    "status": "completed",
    "started_at": "2026-04-24T10:15:00Z",
    "completed_at": "2026-04-24T10:15:03Z",
    "error_message": "",
    "rows_scanned": 498,
    "rows_inserted": 2,
    "rows_updated": 5,
    "rows_deleted": 1,
    "rows_unchanged": 490,
    "snapshot_version": 1745492103,
    "created_at": "2026-04-24T10:15:00Z"
  }
}
```

`snapshot_version` is a Unix timestamp (seconds) assigned at the start of the sync. It serves as the version marker for delete detection.

**Error `500`** — connection failure or DB error
```json
{ "error": "dial tcp ...: connection refused" }
```

---

### Sync Jobs

#### `GET /sources/:sourceId/jobs`

List sync jobs for a source.

**Query params**

| Param | Default | Notes |
|---|---|---|
| `limit` | `20` | Max jobs to return |

**Response `200`**
```json
{
  "data": [
    {
      "id": "e5f6a7b8-...",
      "source_id": "a1b2c3d4-...",
      "table_id": "b2c3d4e5-...",
      "job_type": "table_sync",
      "strategy": "full",
      "status": "completed",
      "started_at": "2026-04-24T10:15:00Z",
      "completed_at": "2026-04-24T10:15:03Z",
      "error_message": "",
      "rows_scanned": 498,
      "rows_inserted": 2,
      "rows_updated": 5,
      "rows_deleted": 1,
      "rows_unchanged": 490,
      "snapshot_version": 1745492103,
      "created_at": "2026-04-24T10:15:00Z"
    }
  ]
}
```

Jobs are ordered by `created_at DESC`.

**`status` values:** `pending` → `running` → `completed` / `failed`

---

#### `GET /jobs/:jobId`

Fetch a single sync job by UUID.

**Response `200`** — same shape as a single item from `GET /sources/:sourceId/jobs`

**Error `404`**
```json
{ "error": "job not found" }
```

---

### Data & Changes

#### `GET /sources/:sourceId/tables/:tableId/data`

Query ingested (live, non-deleted) rows for a table from the platform DB. Does not hit the source database.

**Query params**

| Param | Default | Notes |
|---|---|---|
| `limit` | `100` | Max rows to return |
| `offset` | `0` | Pagination offset |

**Response `200`**
```json
{
  "data": [
    {
      "id": "abc-...",
      "warehouse_code": "WH-001",
      "name": "Main Warehouse",
      "city": "Dubai",
      "total_area_sqm": "12500.00",
      "is_active": true,
      "updated_at": "2026-01-10T08:00:00Z"
    }
  ]
}
```

Each element is the raw row as ingested from the source, with values normalised (UUIDs as strings, timestamps as RFC 3339, JSONB as objects). Rows are ordered by `ingested_at DESC`.

---

#### `GET /sources/:sourceId/tables/:tableId/changes`

List the change log for a table. Each entry represents a detected insert, update, or delete from a sync run.

**Query params**

| Param | Default | Notes |
|---|---|---|
| `limit` | `100` | Max changes to return |

**Response `200`**
```json
{
  "data": [
    {
      "id": "f6a7b8c9-...",
      "table_id": "b2c3d4e5-...",
      "sync_job_id": "e5f6a7b8-...",
      "operation": "UPDATE",
      "row_key": "abc-...",
      "old_data": {
        "quantity": 100,
        "updated_at": "2026-01-09T00:00:00Z"
      },
      "new_data": {
        "quantity": 95,
        "updated_at": "2026-01-10T08:00:00Z"
      },
      "changed_columns": ["quantity", "updated_at"],
      "snapshot_version": 1745492103,
      "detected_at": "2026-04-24T10:15:02Z"
    }
  ]
}
```

**`operation` values:** `INSERT` · `UPDATE` · `DELETE`

- `INSERT`: `old_data` is `null`, `new_data` is the full new row, `changed_columns` is empty.
- `UPDATE`: both `old_data` and `new_data` are set, `changed_columns` lists which fields differ.
- `DELETE`: `old_data` is the last known row state, `new_data` is `null`, `changed_columns` is empty.

Changes are ordered by `detected_at DESC`.

---

#### `GET /sources/:sourceId/tables/:tableId/snapshots`

List snapshot history for a table. Each snapshot corresponds to one completed sync run.

**Query params**

| Param | Default | Notes |
|---|---|---|
| `limit` | `20` | Max snapshots to return |

**Response `200`**
```json
{
  "data": [
    {
      "id": "a7b8c9d0-...",
      "table_id": "b2c3d4e5-...",
      "sync_job_id": "e5f6a7b8-...",
      "version": 1745492103,
      "row_count": 497,
      "inserts": 2,
      "updates": 5,
      "deletes": 1,
      "created_at": "2026-04-24T10:15:03Z"
    }
  ]
}
```

`version` is the Unix timestamp (seconds) of when that sync started. It uniquely identifies the state of the table at that point in time. Snapshots are ordered by `version DESC` (newest first).

---

## Typical Workflow

```
1.  POST /sources                                    # register source
2.  POST /sources/:id/discover                       # introspect schema
3.  GET  /sources/:id/tables                         # confirm tables found
4.  GET  /sources/:id/tables/:tableId/columns        # inspect column metadata
5.  POST /sources/:id/tables/:tableId/sync           # run first sync (blocking)
6.  GET  /sources/:id/tables/:tableId/data           # query ingested rows
7.  POST /sources/:id/tables/:tableId/sync           # run again after source changes
8.  GET  /sources/:id/tables/:tableId/changes        # see what changed
9.  GET  /sources/:id/tables/:tableId/snapshots      # view version history
10. GET  /sources/:id/jobs                           # audit all sync runs
```

## Background Scheduler

When `SyncEnabled=true` on a source, the scheduler runs `POST /sources/:id/sync` logic automatically at `SyncIntervalSeconds` intervals (minimum 60s, default 900s). Scheduled syncs start on server boot for all existing sync-enabled sources, and are added immediately when a new source is registered with `SyncEnabled=true`. Deleting a source unschedules it immediately.
