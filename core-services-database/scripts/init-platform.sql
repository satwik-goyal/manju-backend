-- scripts/init-platform.sql
-- This sets up extensions and the digestion engine schema
-- in YOUR platform database.

-- ═══════════════════════════════════════════════
-- EXTENSIONS
-- ═══════════════════════════════════════════════

-- PostGIS: spatial queries (distances, areas, containment)
CREATE EXTENSION IF NOT EXISTS postgis;

-- UUID generation
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Trigram matching for fuzzy text search
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Verify extensions loaded
DO 
$$
BEGIN
    RAISE NOTICE 'PostGIS version: %', PostGIS_Version();
    RAISE NOTICE 'Extensions loaded successfully';
END
$$
;

-- ═══════════════════════════════════════════════
-- DIGESTION ENGINE SCHEMA
-- ═══════════════════════════════════════════════

-- Sources: registered external databases
CREATE TABLE IF NOT EXISTS sources (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name            VARCHAR(255) NOT NULL,
    description     TEXT DEFAULT '',
    
    -- Connection details
    db_type         VARCHAR(50) NOT NULL,
    host            VARCHAR(255) NOT NULL,
    port            INTEGER NOT NULL,
    database_name   VARCHAR(255) NOT NULL,
    username        VARCHAR(255) NOT NULL,
    password_encrypted TEXT NOT NULL,
    ssl_mode        VARCHAR(50) DEFAULT 'disable',
    extra_params    JSONB DEFAULT '{}',
    
    -- Sync configuration
    sync_enabled    BOOLEAN DEFAULT TRUE,
    sync_interval_seconds INTEGER DEFAULT 900,
    sync_strategy   VARCHAR(50) DEFAULT 'timestamp',
    
    -- Status
    status          VARCHAR(50) DEFAULT 'pending',
    last_error      TEXT,
    last_connected  TIMESTAMPTZ,
    last_sync_at    TIMESTAMPTZ,
    
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Discovered tables: what we found in the source database
CREATE TABLE IF NOT EXISTS discovered_tables (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_id           UUID NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    
    schema_name         VARCHAR(255) NOT NULL DEFAULT 'public',
    table_name          VARCHAR(255) NOT NULL,
    
    estimated_row_count BIGINT DEFAULT 0,
    size_bytes          BIGINT DEFAULT 0,
    
    sync_enabled        BOOLEAN DEFAULT TRUE,
    primary_key_columns TEXT[] DEFAULT '{}',
    timestamp_column    VARCHAR(255),
    
    last_sync_at        TIMESTAMPTZ,
    last_row_count      BIGINT DEFAULT 0,
    last_snapshot_version BIGINT DEFAULT 0,
    
    discovered_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    UNIQUE(source_id, schema_name, table_name)
);

-- Discovered columns: column-level metadata
CREATE TABLE IF NOT EXISTS discovered_columns (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    table_id        UUID NOT NULL REFERENCES discovered_tables(id) ON DELETE CASCADE,
    
    column_name     VARCHAR(255) NOT NULL,
    ordinal_position INTEGER NOT NULL,
    
    data_type       VARCHAR(100) NOT NULL,
    mapped_type     VARCHAR(50),
    max_length      INTEGER,
    numeric_precision INTEGER,
    numeric_scale   INTEGER,
    
    is_nullable     BOOLEAN DEFAULT TRUE,
    is_primary_key  BOOLEAN DEFAULT FALSE,
    is_unique       BOOLEAN DEFAULT FALSE,
    has_default     BOOLEAN DEFAULT FALSE,
    default_value   TEXT,
    
    semantic_type   VARCHAR(100) DEFAULT '',
    
    null_count      BIGINT DEFAULT 0,
    distinct_count  BIGINT DEFAULT 0,
    sample_values   JSONB DEFAULT '[]',
    
    UNIQUE(table_id, column_name)
);

-- Discovered foreign keys
CREATE TABLE IF NOT EXISTS discovered_foreign_keys (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_id           UUID NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    
    from_table_id       UUID NOT NULL REFERENCES discovered_tables(id) ON DELETE CASCADE,
    from_column         VARCHAR(255) NOT NULL,
    to_table_id         UUID REFERENCES discovered_tables(id) ON DELETE SET NULL,
    to_table_schema     VARCHAR(255) NOT NULL DEFAULT 'public',
    to_table_name       VARCHAR(255) NOT NULL,
    to_column           VARCHAR(255) NOT NULL,
    
    constraint_name     VARCHAR(255),
    
    discovered_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Sync jobs: execution history
CREATE TABLE IF NOT EXISTS sync_jobs (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source_id       UUID NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
    table_id        UUID REFERENCES discovered_tables(id) ON DELETE CASCADE,
    
    job_type        VARCHAR(50) NOT NULL,
    strategy        VARCHAR(50) NOT NULL,
    
    status          VARCHAR(50) NOT NULL DEFAULT 'pending',
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    error_message   TEXT,
    
    rows_scanned    BIGINT DEFAULT 0,
    rows_inserted   BIGINT DEFAULT 0,
    rows_updated    BIGINT DEFAULT 0,
    rows_deleted    BIGINT DEFAULT 0,
    rows_unchanged  BIGINT DEFAULT 0,
    snapshot_version BIGINT,
    
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Ingested rows: the actual data
CREATE TABLE IF NOT EXISTS ingested_rows (
    id                  UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    table_id            UUID NOT NULL REFERENCES discovered_tables(id) ON DELETE CASCADE,
    
    row_key             TEXT NOT NULL,
    data                JSONB NOT NULL,
    row_hash            TEXT NOT NULL,
    
    first_seen_version  BIGINT NOT NULL,
    last_seen_version   BIGINT NOT NULL,
    current_version     BIGINT NOT NULL DEFAULT 1,
    
    source_updated_at   TIMESTAMPTZ,
    ingested_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    is_deleted          BOOLEAN DEFAULT FALSE,
    deleted_at          TIMESTAMPTZ,
    
    UNIQUE(table_id, row_key)
);

-- Changes: detected changes between syncs
CREATE TABLE IF NOT EXISTS changes (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    table_id        UUID NOT NULL REFERENCES discovered_tables(id) ON DELETE CASCADE,
    sync_job_id     UUID REFERENCES sync_jobs(id),
    
    operation       VARCHAR(10) NOT NULL,
    row_key         TEXT NOT NULL,
    
    old_data        JSONB,
    new_data        JSONB,
    changed_columns TEXT[],
    
    snapshot_version BIGINT NOT NULL,
    detected_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Snapshots: point-in-time markers
CREATE TABLE IF NOT EXISTS snapshots (
    id              UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    table_id        UUID NOT NULL REFERENCES discovered_tables(id) ON DELETE CASCADE,
    sync_job_id     UUID REFERENCES sync_jobs(id),
    
    version         BIGINT NOT NULL,
    row_count       BIGINT NOT NULL,
    
    inserts         BIGINT DEFAULT 0,
    updates         BIGINT DEFAULT 0,
    deletes         BIGINT DEFAULT 0,
    
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    UNIQUE(table_id, version)
);

-- ═══════════════════════════════════════════════
-- INDEXES
-- ═══════════════════════════════════════════════

CREATE INDEX IF NOT EXISTS idx_sources_status ON sources(status);
CREATE INDEX IF NOT EXISTS idx_discovered_tables_source ON discovered_tables(source_id);
CREATE INDEX IF NOT EXISTS idx_discovered_columns_table ON discovered_columns(table_id);
CREATE INDEX IF NOT EXISTS idx_sync_jobs_source ON sync_jobs(source_id);
CREATE INDEX IF NOT EXISTS idx_sync_jobs_status ON sync_jobs(status);
CREATE INDEX IF NOT EXISTS idx_sync_jobs_created ON sync_jobs(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ingested_rows_table ON ingested_rows(table_id);
CREATE INDEX IF NOT EXISTS idx_ingested_rows_key ON ingested_rows(table_id, row_key);
CREATE INDEX IF NOT EXISTS idx_ingested_rows_hash ON ingested_rows(table_id, row_hash);
CREATE INDEX IF NOT EXISTS idx_ingested_rows_version ON ingested_rows(table_id, last_seen_version);
CREATE INDEX IF NOT EXISTS idx_ingested_rows_data ON ingested_rows USING GIN(data);
CREATE INDEX IF NOT EXISTS idx_ingested_rows_deleted ON ingested_rows(table_id) WHERE is_deleted = FALSE;
CREATE INDEX IF NOT EXISTS idx_changes_table ON changes(table_id);
CREATE INDEX IF NOT EXISTS idx_changes_detected ON changes(detected_at DESC);
CREATE INDEX IF NOT EXISTS idx_changes_row ON changes(table_id, row_key);
CREATE INDEX IF NOT EXISTS idx_snapshots_table ON snapshots(table_id, version DESC);

-- ═══════════════════════════════════════════════
-- TRIGGER: auto-update updated_at
-- ═══════════════════════════════════════════════

CREATE OR REPLACE FUNCTION trigger_set_updated_at()
RETURNS TRIGGER AS 
$$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$
 LANGUAGE plpgsql;

CREATE TRIGGER set_sources_updated_at
    BEFORE UPDATE ON sources
    FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();

CREATE TRIGGER set_discovered_tables_updated_at
    BEFORE UPDATE ON discovered_tables
    FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();

CREATE TRIGGER set_ingested_rows_updated_at
    BEFORE UPDATE ON ingested_rows
    FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();

-- ═══════════════════════════════════════════════
-- DONE
-- ═══════════════════════════════════════════════

DO 
$$
BEGIN
    RAISE NOTICE '════════════════════════════════════════';
    RAISE NOTICE 'Platform database initialized successfully';
    RAISE NOTICE 'Tables created: sources, discovered_tables,';
    RAISE NOTICE '  discovered_columns, discovered_foreign_keys,';
    RAISE NOTICE '  sync_jobs, ingested_rows, changes, snapshots';
    RAISE NOTICE '════════════════════════════════════════';
END
$$
;