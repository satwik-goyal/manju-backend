-- Object Store Service schema.
-- Run after init-platform.sql (mounted as 02-init-object-store.sql).

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- ─────────────────────────────────────────────────────────────────────────────
-- stored_files
-- One row per file uploaded through the Object Store Service.
-- The actual bytes live in the storage backend (MinIO / S3 / GCS / Azure).
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS stored_files (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Tenant isolation: every query must filter by tenant_id.
    tenant_id         UUID        NOT NULL,

    -- File identity
    original_filename TEXT        NOT NULL,
    content_type      TEXT        NOT NULL,
    size_bytes        BIGINT      NOT NULL,
    checksum_sha256   TEXT        NOT NULL,

    -- Storage location
    bucket            TEXT        NOT NULL,
    object_key        TEXT        NOT NULL,

    -- Classification
    category          TEXT        NOT NULL,
    status            TEXT        NOT NULL DEFAULT 'active',

    -- Optional back-links to other kernel entities
    object_id         UUID,
    source_id         UUID,
    uploaded_by       UUID,

    -- Timestamps
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ,
    is_deleted        BOOLEAN     NOT NULL DEFAULT FALSE,

    CONSTRAINT stored_files_bucket_key_unique
        UNIQUE (bucket, object_key),

    CONSTRAINT stored_files_category_check CHECK (
        category IN ('digestion_source', 'attachment', 'export', 'pipeline_artifact')
    ),

    CONSTRAINT stored_files_status_check CHECK (
        status IN ('active', 'processing', 'processed', 'deleted')
    )
);

-- ─────────────────────────────────────────────────────────────────────────────
-- Indexes
-- ─────────────────────────────────────────────────────────────────────────────

-- Primary lookup: all files for a tenant
CREATE INDEX IF NOT EXISTS idx_stored_files_tenant_id
    ON stored_files (tenant_id);

-- Listing by category within a tenant
CREATE INDEX IF NOT EXISTS idx_stored_files_tenant_category
    ON stored_files (tenant_id, category);

-- Listing by status within a tenant
CREATE INDEX IF NOT EXISTS idx_stored_files_tenant_status
    ON stored_files (tenant_id, status);

-- Deduplication check
CREATE INDEX IF NOT EXISTS idx_stored_files_checksum
    ON stored_files (checksum_sha256);

-- Back-link lookups (sparse — most rows will have NULLs here)
CREATE INDEX IF NOT EXISTS idx_stored_files_source_id
    ON stored_files (source_id)
    WHERE source_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_stored_files_object_id
    ON stored_files (object_id)
    WHERE object_id IS NOT NULL;

-- Time-range queries, defaulting to newest-first
CREATE INDEX IF NOT EXISTS idx_stored_files_created_at
    ON stored_files (created_at DESC);

-- Cheap "list active files" filter
CREATE INDEX IF NOT EXISTS idx_stored_files_active
    ON stored_files (tenant_id, created_at DESC)
    WHERE is_deleted = FALSE;

-- ─────────────────────────────────────────────────────────────────────────────
-- Auto-update trigger for updated_at
-- ─────────────────────────────────────────────────────────────────────────────
CREATE OR REPLACE FUNCTION trigger_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER set_updated_at_stored_files
    BEFORE UPDATE ON stored_files
    FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();
