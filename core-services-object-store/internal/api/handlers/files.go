// Package handlers contains the Echo HTTP handlers for the Object Store Service.
package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/manju-backend/core-services-object-store/pkg/filestore"
)

// Handler wraps the filestore.Store for HTTP delivery.
type Handler struct {
	store *filestore.Store
}

// New creates a Handler.
func New(store *filestore.Store) *Handler {
	return &Handler{store: store}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func ok(c echo.Context, data any) error {
	return c.JSON(http.StatusOK, map[string]any{"data": data})
}

func created(c echo.Context, data any) error {
	return c.JSON(http.StatusCreated, map[string]any{"data": data})
}

func errResp(c echo.Context, status int, msg string) error {
	return c.JSON(status, map[string]any{"error": msg})
}

// tenantID extracts and validates the X-Tenant-ID header.
func tenantID(c echo.Context) (uuid.UUID, error) {
	raw := c.Request().Header.Get("X-Tenant-ID")
	if raw == "" {
		return uuid.UUID{}, echo.NewHTTPError(http.StatusBadRequest, "missing X-Tenant-ID header")
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.UUID{}, echo.NewHTTPError(http.StatusBadRequest, "invalid X-Tenant-ID header")
	}
	return id, nil
}

func parseID(c echo.Context, param string) (uuid.UUID, error) {
	return uuid.Parse(c.Param(param))
}

func optionalUUID(s string) *uuid.UUID {
	if s == "" {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &id
}

// fileResponse is the JSON shape returned for every FileRecord.
type fileResponse struct {
	ID               string     `json:"id"`
	TenantID         string     `json:"tenant_id"`
	OriginalFilename string     `json:"original_filename"`
	ContentType      string     `json:"content_type"`
	SizeBytes        int64      `json:"size_bytes"`
	ChecksumSHA256   string     `json:"checksum_sha256"`
	Bucket           string     `json:"bucket"`
	ObjectKey        string     `json:"object_key"`
	Category         string     `json:"category"`
	Status           string     `json:"status"`
	ObjectID         *string    `json:"object_id,omitempty"`
	SourceID         *string    `json:"source_id,omitempty"`
	UploadedBy       *string    `json:"uploaded_by,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	DeletedAt        *time.Time `json:"deleted_at,omitempty"`
	IsDeleted        bool       `json:"is_deleted"`
}

func toFileResponse(rec *filestore.FileRecord) fileResponse {
	r := fileResponse{
		ID:               rec.ID.String(),
		TenantID:         rec.TenantID.String(),
		OriginalFilename: rec.OriginalFilename,
		ContentType:      rec.ContentType,
		SizeBytes:        rec.SizeBytes,
		ChecksumSHA256:   rec.ChecksumSHA256,
		Bucket:           rec.Bucket,
		ObjectKey:        rec.ObjectKey,
		Category:         string(rec.Category),
		Status:           string(rec.Status),
		CreatedAt:        rec.CreatedAt,
		UpdatedAt:        rec.UpdatedAt,
		DeletedAt:        rec.DeletedAt,
		IsDeleted:        rec.IsDeleted,
	}
	if rec.ObjectID != nil {
		s := rec.ObjectID.String()
		r.ObjectID = &s
	}
	if rec.SourceID != nil {
		s := rec.SourceID.String()
		r.SourceID = &s
	}
	if rec.UploadedBy != nil {
		s := rec.UploadedBy.String()
		r.UploadedBy = &s
	}
	return r
}

// objectEntry is the JSON shape for a single item returned by Browse.
type objectEntry struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	ContentType  string    `json:"content_type,omitempty"`
	ETag         string    `json:"etag,omitempty"`
	LastModified time.Time `json:"last_modified"`
	IsPrefix     bool      `json:"is_prefix"`
}

// ── Health ────────────────────────────────────────────────────────────────────

func (h *Handler) Health(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// ── POST /v1/files ────────────────────────────────────────────────────────────

// UploadFile accepts a multipart/form-data request and streams the file to
// the storage backend.
//
// Form fields:
//   - file      (required) — binary file content
//   - category  (required) — digestion_source | attachment | export | pipeline_artifact
//   - source_id (optional) — UUID linking to a digestion source
//   - object_id (optional) — UUID linking to a kernel object
//
// Headers:
//   - X-Tenant-ID   (required)
//   - X-Uploaded-By (optional) UUID of the acting user
func (h *Handler) UploadFile(c echo.Context) error {
	tenID, err := tenantID(c)
	if err != nil {
		return err
	}

	category := filestore.FileCategory(c.FormValue("category"))
	switch category {
	case filestore.CategoryDigestionSource,
		filestore.CategoryAttachment,
		filestore.CategoryExport,
		filestore.CategoryPipelineArtifact:
	default:
		return errResp(c, http.StatusBadRequest,
			"category must be one of: digestion_source, attachment, export, pipeline_artifact")
	}

	fh, err := c.FormFile("file")
	if err != nil {
		return errResp(c, http.StatusBadRequest, "file field is required")
	}
	f, err := fh.Open()
	if err != nil {
		return errResp(c, http.StatusInternalServerError, "could not open uploaded file")
	}
	defer f.Close()

	contentType := fh.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	req := filestore.UploadRequest{
		TenantID:         tenID,
		OriginalFilename: fh.Filename,
		ContentType:      contentType,
		Size:             fh.Size,
		Category:         category,
		Reader:           f,
		SourceID:         optionalUUID(c.FormValue("source_id")),
		ObjectID:         optionalUUID(c.FormValue("object_id")),
		UploadedBy:       optionalUUID(c.Request().Header.Get("X-Uploaded-By")),
	}

	rec, err := h.store.Upload(c.Request().Context(), req)
	if err != nil {
		return errResp(c, http.StatusInternalServerError, err.Error())
	}
	return created(c, toFileResponse(rec))
}

// ── GET /v1/files ─────────────────────────────────────────────────────────────

// ListFiles returns file records for the tenant, with optional filters.
//
// Query params: category, status, source_id, object_id, limit (default 50), offset.
func (h *Handler) ListFiles(c echo.Context) error {
	tenID, err := tenantID(c)
	if err != nil {
		return err
	}

	req := filestore.ListRequest{TenantID: tenID}

	if v := c.QueryParam("category"); v != "" {
		cat := filestore.FileCategory(v)
		req.Category = &cat
	}
	if v := c.QueryParam("status"); v != "" {
		st := filestore.FileStatus(v)
		req.Status = &st
	}
	req.SourceID = optionalUUID(c.QueryParam("source_id"))
	req.ObjectID = optionalUUID(c.QueryParam("object_id"))

	limit := 50
	if v := c.QueryParam("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}
	req.Limit = limit
	fmt.Sscanf(c.QueryParam("offset"), "%d", &req.Offset)

	records, err := h.store.List(c.Request().Context(), req)
	if err != nil {
		return errResp(c, http.StatusInternalServerError, err.Error())
	}

	out := make([]fileResponse, len(records))
	for i, r := range records {
		out[i] = toFileResponse(r)
	}
	return ok(c, out)
}

// ── GET /v1/files/duplicate ───────────────────────────────────────────────────

// FindDuplicate checks whether a file with the given SHA-256 checksum exists
// for this tenant.
//
// Query params: checksum (required) — hex-encoded SHA-256.
func (h *Handler) FindDuplicate(c echo.Context) error {
	tenID, err := tenantID(c)
	if err != nil {
		return err
	}
	checksum := c.QueryParam("checksum")
	if len(checksum) != 64 {
		return errResp(c, http.StatusBadRequest, "checksum must be a 64-character hex-encoded SHA-256")
	}
	rec, err := h.store.FindDuplicate(c.Request().Context(), tenID, checksum)
	if err != nil {
		return errResp(c, http.StatusInternalServerError, err.Error())
	}
	if rec == nil {
		return ok(c, map[string]bool{"found": false})
	}
	return ok(c, map[string]any{"found": true, "file": toFileResponse(rec)})
}

// ── GET /v1/files/:id ─────────────────────────────────────────────────────────

// GetFile returns metadata for a single file.
func (h *Handler) GetFile(c echo.Context) error {
	tenID, err := tenantID(c)
	if err != nil {
		return err
	}
	id, err := parseID(c, "id")
	if err != nil {
		return errResp(c, http.StatusBadRequest, "invalid file ID")
	}
	rec, err := h.store.Get(c.Request().Context(), id, tenID)
	if err != nil {
		if errors.Is(err, filestore.ErrNotFound) || errors.Is(err, filestore.ErrDeleted) {
			return errResp(c, http.StatusNotFound, "file not found")
		}
		return errResp(c, http.StatusInternalServerError, err.Error())
	}
	return ok(c, toFileResponse(rec))
}

// ── DELETE /v1/files/:id ──────────────────────────────────────────────────────

// DeleteFile soft-deletes the file and removes it from the storage backend.
func (h *Handler) DeleteFile(c echo.Context) error {
	tenID, err := tenantID(c)
	if err != nil {
		return err
	}
	id, err := parseID(c, "id")
	if err != nil {
		return errResp(c, http.StatusBadRequest, "invalid file ID")
	}
	if err := h.store.Delete(c.Request().Context(), id, tenID); err != nil {
		if errors.Is(err, filestore.ErrNotFound) {
			return errResp(c, http.StatusNotFound, "file not found")
		}
		return errResp(c, http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// ── GET /v1/files/:id/download ────────────────────────────────────────────────

// DownloadFile streams the file content directly to the response body.
// Sets Content-Disposition: attachment so browsers prompt a save dialog.
func (h *Handler) DownloadFile(c echo.Context) error {
	tenID, err := tenantID(c)
	if err != nil {
		return err
	}
	id, err := parseID(c, "id")
	if err != nil {
		return errResp(c, http.StatusBadRequest, "invalid file ID")
	}
	rc, rec, err := h.store.Download(c.Request().Context(), id, tenID)
	if err != nil {
		if errors.Is(err, filestore.ErrNotFound) || errors.Is(err, filestore.ErrDeleted) {
			return errResp(c, http.StatusNotFound, "file not found")
		}
		return errResp(c, http.StatusInternalServerError, err.Error())
	}
	defer rc.Close()

	c.Response().Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename=%q`, rec.OriginalFilename))
	c.Response().Header().Set("Content-Length", fmt.Sprintf("%d", rec.SizeBytes))
	c.Response().Header().Set("X-Checksum-SHA256", rec.ChecksumSHA256)
	return c.Stream(http.StatusOK, rec.ContentType, rc)
}

// ── GET /v1/files/:id/presign ─────────────────────────────────────────────────

// PresignFile returns a time-limited URL for direct download from the storage backend.
//
// Query params: expiry — Go duration string (default "15m", e.g. "1h", "30m").
func (h *Handler) PresignFile(c echo.Context) error {
	tenID, err := tenantID(c)
	if err != nil {
		return err
	}
	id, err := parseID(c, "id")
	if err != nil {
		return errResp(c, http.StatusBadRequest, "invalid file ID")
	}

	expiry := 15 * time.Minute
	if v := c.QueryParam("expiry"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			expiry = d
		}
	}

	url, err := h.store.PresignedURL(c.Request().Context(), id, tenID, expiry)
	if err != nil {
		if errors.Is(err, filestore.ErrNotFound) || errors.Is(err, filestore.ErrDeleted) {
			return errResp(c, http.StatusNotFound, "file not found")
		}
		return errResp(c, http.StatusInternalServerError, err.Error())
	}
	return ok(c, map[string]any{
		"url":        url,
		"expires_in": expiry.String(),
	})
}

// ── PATCH /v1/files/:id/status ────────────────────────────────────────────────

// UpdateFileStatus updates the lifecycle status of a file.
//
// Body: {"status": "processing"} — valid values: active, processing, processed, deleted.
func (h *Handler) UpdateFileStatus(c echo.Context) error {
	id, err := parseID(c, "id")
	if err != nil {
		return errResp(c, http.StatusBadRequest, "invalid file ID")
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := c.Bind(&body); err != nil {
		return errResp(c, http.StatusBadRequest, "invalid request body")
	}

	status := filestore.FileStatus(body.Status)
	switch status {
	case filestore.StatusActive,
		filestore.StatusProcessing,
		filestore.StatusProcessed,
		filestore.StatusDeleted:
	default:
		return errResp(c, http.StatusBadRequest,
			"status must be one of: active, processing, processed, deleted")
	}

	if err := h.store.UpdateStatus(c.Request().Context(), id, status); err != nil {
		if errors.Is(err, filestore.ErrNotFound) {
			return errResp(c, http.StatusNotFound, "file not found")
		}
		return errResp(c, http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// ── GET /v1/browse ────────────────────────────────────────────────────────────

// Browse lists objects and folder prefixes within the tenant's storage bucket.
//
// Query params:
//   - prefix    — path prefix to list under (default: bucket root)
//   - recursive — "true" to list all descendants; "false" (default) for immediate children only
func (h *Handler) Browse(c echo.Context) error {
	tenID, err := tenantID(c)
	if err != nil {
		return err
	}

	prefix := c.QueryParam("prefix")
	recursive := c.QueryParam("recursive") == "true"

	result, err := h.store.Browse(c.Request().Context(), tenID, prefix, recursive)
	if err != nil {
		return errResp(c, http.StatusInternalServerError, err.Error())
	}

	entries := make([]objectEntry, len(result.Entries))
	for i, e := range result.Entries {
		entries[i] = objectEntry{
			Key:          e.Key,
			Size:         e.Size,
			ContentType:  e.ContentType,
			ETag:         e.ETag,
			LastModified: e.LastModified,
			IsPrefix:     e.IsPrefix,
		}
	}
	return ok(c, map[string]any{
		"bucket":  result.Bucket,
		"prefix":  result.Prefix,
		"entries": entries,
	})
}
