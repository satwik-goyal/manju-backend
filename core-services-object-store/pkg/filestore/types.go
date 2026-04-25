package filestore

import (
	"errors"
	"io"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors returned by Store methods.
var (
	// ErrNotFound is returned when the requested file or object does not exist.
	ErrNotFound = errors.New("filestore: not found")
	// ErrDeleted is returned when a file exists but has been soft-deleted.
	ErrDeleted = errors.New("filestore: file has been deleted")
)

// FileCategory classifies the intended purpose of a stored file.
type FileCategory string

const (
	CategoryDigestionSource  FileCategory = "digestion_source"  // CSV/TSV/TXT for the SQL Digestion Engine
	CategoryAttachment       FileCategory = "attachment"         // PDFs/images attached to kernel objects
	CategoryExport           FileCategory = "export"             // Generated reports and data exports
	CategoryPipelineArtifact FileCategory = "pipeline_artifact"  // Intermediate outputs from data pipelines
)

// FileStatus tracks the lifecycle state of a stored file.
type FileStatus string

const (
	StatusActive     FileStatus = "active"
	StatusProcessing FileStatus = "processing"
	StatusProcessed  FileStatus = "processed"
	StatusDeleted    FileStatus = "deleted"
)

// FileRecord is the platform-DB representation of a stored file.
// The actual bytes live in the storage backend; this holds metadata only.
type FileRecord struct {
	ID               uuid.UUID
	TenantID         uuid.UUID
	OriginalFilename string
	ContentType      string
	SizeBytes        int64
	ChecksumSHA256   string
	Bucket           string
	ObjectKey        string
	Category         FileCategory
	Status           FileStatus

	// Optional back-links to other kernel entities.
	ObjectID   *uuid.UUID
	SourceID   *uuid.UUID
	UploadedBy *uuid.UUID

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
	IsDeleted bool
}

// UploadRequest carries all inputs needed to store a new file.
type UploadRequest struct {
	TenantID         uuid.UUID
	OriginalFilename string
	ContentType      string
	// Size is the content length in bytes. Pass -1 if unknown; the service
	// will stat the object after upload to record the actual size.
	Size     int64
	Category FileCategory
	// Reader is the file content stream. The caller retains ownership and
	// must not close it until Upload returns.
	Reader io.Reader

	// Optional links to other kernel entities.
	ObjectID   *uuid.UUID
	SourceID   *uuid.UUID
	UploadedBy *uuid.UUID
}

// ListRequest filters the file listing query.
// All fields except TenantID are optional.
type ListRequest struct {
	TenantID uuid.UUID
	Category *FileCategory
	Status   *FileStatus
	SourceID *uuid.UUID
	ObjectID *uuid.UUID
	// Limit defaults to 50; maximum is 1000.
	Limit  int
	Offset int
}

// BrowseResult is returned by Store.Browse.
type BrowseResult struct {
	Bucket  string
	Prefix  string
	Entries []ObjectInfo
}
