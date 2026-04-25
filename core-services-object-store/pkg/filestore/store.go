package filestore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store manages file lifecycle: upload, download, delete, listing, deduplication,
// and presigned URLs. It composes a StorageBackend with a PostgreSQL pool.
//
// Create one with New; it is safe for concurrent use.
type Store struct {
	backend      StorageBackend
	db           *pgxpool.Pool
	emitter      EventEmitter
	tenantBucket func(uuid.UUID) string
}

// Option configures a Store.
type Option func(*Store)

// WithEventEmitter sets the EventEmitter used for file lifecycle events.
// Defaults to NoOpEmitter.
func WithEventEmitter(e EventEmitter) Option {
	return func(s *Store) { s.emitter = e }
}

// WithTenantBucket overrides the function that maps a tenant UUID to a bucket
// name. The default produces names like "t-<uuid>" which are valid for MinIO
// and S3 (≤63 chars, lowercase alphanumeric + hyphens).
func WithTenantBucket(fn func(uuid.UUID) string) Option {
	return func(s *Store) { s.tenantBucket = fn }
}

// New creates a Store with the given backend and PostgreSQL pool.
func New(backend StorageBackend, db *pgxpool.Pool, opts ...Option) *Store {
	s := &Store{
		backend:      backend,
		db:           db,
		emitter:      NoOpEmitter{},
		tenantBucket: func(id uuid.UUID) string { return "t-" + id.String() },
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Upload streams the file in req to the storage backend, computes a SHA-256
// checksum during streaming, then persists a FileRecord in PostgreSQL.
//
// If req.Size is -1 the backend receives an unbounded stream; the service
// stats the object afterwards to record the actual byte count.
//
// On a PostgreSQL write failure the backend object is deleted best-effort so
// that orphaned blobs do not accumulate.
func (s *Store) Upload(ctx context.Context, req UploadRequest) (*FileRecord, error) {
	bucket := s.tenantBucket(req.TenantID)
	if err := s.backend.EnsureBucket(ctx, bucket); err != nil {
		return nil, fmt.Errorf("filestore: ensure bucket: %w", err)
	}

	fileID := uuid.New()
	key := objectKey(req.Category, fileID, req.OriginalFilename)

	// TeeReader computes SHA-256 while the stream is consumed by the backend.
	hash := sha256.New()
	tee := io.TeeReader(req.Reader, hash)

	if err := s.backend.Upload(ctx, bucket, key, tee, req.Size, req.ContentType); err != nil {
		return nil, fmt.Errorf("filestore: upload to backend: %w", err)
	}

	checksum := hex.EncodeToString(hash.Sum(nil))

	size := req.Size
	if size < 0 {
		info, err := s.backend.Stat(ctx, bucket, key)
		if err != nil {
			_ = s.backend.Delete(ctx, bucket, key)
			return nil, fmt.Errorf("filestore: stat after upload: %w", err)
		}
		size = info.Size
	}

	rec := &FileRecord{
		ID:               fileID,
		TenantID:         req.TenantID,
		OriginalFilename: req.OriginalFilename,
		ContentType:      req.ContentType,
		SizeBytes:        size,
		ChecksumSHA256:   checksum,
		Bucket:           bucket,
		ObjectKey:        key,
		Category:         req.Category,
		Status:           StatusActive,
		ObjectID:         req.ObjectID,
		SourceID:         req.SourceID,
		UploadedBy:       req.UploadedBy,
	}
	if err := s.insertFileRecord(ctx, rec); err != nil {
		_ = s.backend.Delete(ctx, bucket, key)
		return nil, fmt.Errorf("filestore: record upload: %w", err)
	}

	_ = s.emitter.Emit(ctx, Event{
		Type:      EventFileUploaded,
		FileID:    fileID,
		TenantID:  req.TenantID,
		Timestamp: time.Now().UTC(),
		Metadata: map[string]any{
			"category":          string(req.Category),
			"original_filename": req.OriginalFilename,
			"size_bytes":        size,
			"checksum_sha256":   checksum,
		},
	})
	return rec, nil
}

// Download returns a streaming reader for the file identified by fileID.
// Ownership of the tenant is verified via tenantID.
// The caller must close the returned ReadCloser after reading.
func (s *Store) Download(ctx context.Context, fileID, tenantID uuid.UUID) (io.ReadCloser, *FileRecord, error) {
	rec, err := s.getFileRecord(ctx, fileID, tenantID)
	if err != nil {
		return nil, nil, fmt.Errorf("filestore: get record for download: %w", err)
	}
	rc, err := s.backend.Download(ctx, rec.Bucket, rec.ObjectKey)
	if err != nil {
		return nil, nil, fmt.Errorf("filestore: backend download: %w", err)
	}
	return rc, rec, nil
}

// Delete soft-deletes the file: marks is_deleted=true in PostgreSQL and
// deletes the object from the storage backend immediately.
//
// The row is retained in PostgreSQL for audit purposes.
func (s *Store) Delete(ctx context.Context, fileID, tenantID uuid.UUID) error {
	rec, err := s.getFileRecord(ctx, fileID, tenantID)
	if err != nil {
		return fmt.Errorf("filestore: get record for delete: %w", err)
	}

	if err := s.softDeleteFileRecord(ctx, fileID, tenantID); err != nil {
		return fmt.Errorf("filestore: soft delete record: %w", err)
	}

	// Delete from storage backend after the DB is committed.
	if err := s.backend.Delete(ctx, rec.Bucket, rec.ObjectKey); err != nil {
		// Log but don't fail: the record is already soft-deleted.
		// A background worker could re-attempt orphan cleanup if needed.
		_ = err
	}

	_ = s.emitter.Emit(ctx, Event{
		Type:      EventFileDeleted,
		FileID:    fileID,
		TenantID:  tenantID,
		Timestamp: time.Now().UTC(),
	})
	return nil
}

// Get returns metadata for a single file. Returns ErrNotFound if the file does
// not exist or does not belong to tenantID.
func (s *Store) Get(ctx context.Context, fileID, tenantID uuid.UUID) (*FileRecord, error) {
	return s.getFileRecord(ctx, fileID, tenantID)
}

// List returns file records matching req. Results are ordered newest-first.
func (s *Store) List(ctx context.Context, req ListRequest) ([]*FileRecord, error) {
	return s.listFileRecords(ctx, req)
}

// Browse lists objects under prefix in the tenant's storage bucket.
// Use prefix="" to list the bucket root.
// Set recursive=false to see only immediate children (folders appear with IsPrefix=true).
func (s *Store) Browse(ctx context.Context, tenantID uuid.UUID, prefix string, recursive bool) (*BrowseResult, error) {
	bucket := s.tenantBucket(tenantID)
	entries, err := s.backend.List(ctx, bucket, prefix, recursive)
	if err != nil {
		return nil, fmt.Errorf("filestore: browse %s/%s: %w", bucket, prefix, err)
	}
	return &BrowseResult{Bucket: bucket, Prefix: prefix, Entries: entries}, nil
}

// FindDuplicate checks whether a file with the given SHA-256 checksum already
// exists for tenantID. Returns nil, nil if no duplicate is found.
// The caller decides the deduplication policy (skip, version, reject).
func (s *Store) FindDuplicate(ctx context.Context, tenantID uuid.UUID, checksum string) (*FileRecord, error) {
	return s.findDuplicateRecord(ctx, tenantID, checksum)
}

// UpdateStatus updates the lifecycle status of a file record.
// This is the primary way other services signal processing progress.
func (s *Store) UpdateStatus(ctx context.Context, fileID uuid.UUID, status FileStatus) error {
	if err := s.updateFileStatus(ctx, fileID, status); err != nil {
		return fmt.Errorf("filestore: update status: %w", err)
	}

	var evType EventType
	switch status {
	case StatusProcessing:
		evType = EventFileProcessing
	case StatusProcessed:
		evType = EventFileProcessed
	default:
		return nil
	}
	_ = s.emitter.Emit(ctx, Event{
		Type:      evType,
		FileID:    fileID,
		Timestamp: time.Now().UTC(),
	})
	return nil
}

// PresignedURL returns a time-limited download URL for the given file.
// tenantID is verified before generating the URL.
func (s *Store) PresignedURL(ctx context.Context, fileID, tenantID uuid.UUID, expiry time.Duration) (string, error) {
	rec, err := s.getFileRecord(ctx, fileID, tenantID)
	if err != nil {
		return "", fmt.Errorf("filestore: get record for presign: %w", err)
	}
	url, err := s.backend.PresignedGetURL(ctx, rec.Bucket, rec.ObjectKey, expiry)
	if err != nil {
		return "", fmt.Errorf("filestore: presign: %w", err)
	}
	return url, nil
}

// objectKey builds the storage key for a new file within a tenant bucket.
// Format: {category}/{fileID}/{originalFilename}
// Using the fileID as a path component guarantees uniqueness even when
// the same filename is uploaded multiple times.
func objectKey(category FileCategory, fileID uuid.UUID, filename string) string {
	return fmt.Sprintf("%s/%s/%s", string(category), fileID.String(), filename)
}
