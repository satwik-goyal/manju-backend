// Package filestore provides a provider-agnostic file storage library.
// It composes a StorageBackend (MinIO, S3, GCS, Azure) with PostgreSQL
// metadata tracking and file lifecycle event emission.
//
// Usage:
//
//	backend, _ := filestore.NewMinioBackend(filestore.MinioConfig{...})
//	store := filestore.New(backend, pgPool)
//	rec, err := store.Upload(ctx, filestore.UploadRequest{...})
package filestore

import (
	"context"
	"io"
	"time"
)

// ObjectInfo describes a single object returned by the storage backend.
type ObjectInfo struct {
	Key          string
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
	// IsPrefix is true for directory-like common prefixes returned by a
	// non-recursive listing (i.e., a "folder" entry, not an actual object).
	IsPrefix bool
}

// StorageBackend is the provider-agnostic interface for object storage.
// Each method must honour context cancellation.
//
// Implement this interface to add support for GCS, Azure Blob Storage, etc.
// MinIO and AWS S3 are covered by NewMinioBackend.
type StorageBackend interface {
	// EnsureBucket creates bucket if it does not already exist.
	// Calling it on an existing bucket is a no-op.
	EnsureBucket(ctx context.Context, bucket string) error

	// BucketExists reports whether bucket exists and is accessible.
	BucketExists(ctx context.Context, bucket string) (bool, error)

	// Upload streams r into the backend at bucket/key.
	// size is the content length in bytes; pass -1 if unknown.
	// contentType is the MIME type (e.g. "text/csv", "application/pdf").
	Upload(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error

	// Download returns a streaming reader for the object at bucket/key.
	// The caller is responsible for closing the returned ReadCloser.
	Download(ctx context.Context, bucket, key string) (io.ReadCloser, error)

	// Delete removes the object at bucket/key.
	Delete(ctx context.Context, bucket, key string) error

	// Stat returns metadata for the object at bucket/key.
	// Returns ErrNotFound if the object does not exist.
	Stat(ctx context.Context, bucket, key string) (*ObjectInfo, error)

	// List returns objects (and, when recursive=false, common prefixes) under
	// bucket/prefix. Pass prefix="" to list the bucket root.
	// Non-recursive entries that represent folders have ObjectInfo.IsPrefix=true.
	List(ctx context.Context, bucket, prefix string, recursive bool) ([]ObjectInfo, error)

	// PresignedGetURL returns a time-limited URL for downloading bucket/key.
	PresignedGetURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error)

	// PresignedPutURL returns a time-limited URL for uploading to bucket/key.
	PresignedPutURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error)
}
