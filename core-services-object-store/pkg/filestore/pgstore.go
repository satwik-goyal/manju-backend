package filestore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const selectCols = `
	id, tenant_id, original_filename, content_type, size_bytes,
	checksum_sha256, bucket, object_key, category, status,
	object_id, source_id, uploaded_by,
	created_at, updated_at, deleted_at, is_deleted`

func (s *Store) insertFileRecord(ctx context.Context, rec *FileRecord) error {
	const q = `
		INSERT INTO stored_files (
			id, tenant_id, original_filename, content_type, size_bytes,
			checksum_sha256, bucket, object_key, category, status,
			object_id, source_id, uploaded_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING created_at, updated_at`

	row := s.db.QueryRow(ctx, q,
		rec.ID,
		rec.TenantID,
		rec.OriginalFilename,
		rec.ContentType,
		rec.SizeBytes,
		rec.ChecksumSHA256,
		rec.Bucket,
		rec.ObjectKey,
		string(rec.Category),
		string(rec.Status),
		uuidToPgtype(rec.ObjectID),
		uuidToPgtype(rec.SourceID),
		uuidToPgtype(rec.UploadedBy),
	)
	return row.Scan(&rec.CreatedAt, &rec.UpdatedAt)
}

func (s *Store) getFileRecord(ctx context.Context, fileID, tenantID uuid.UUID) (*FileRecord, error) {
	q := `SELECT` + selectCols + `
		FROM stored_files
		WHERE id = $1 AND tenant_id = $2`

	row := s.db.QueryRow(ctx, q, fileID, tenantID)
	rec, err := scanFileRecord(row)
	if err != nil {
		return nil, err
	}
	if rec.IsDeleted {
		return nil, ErrDeleted
	}
	return rec, nil
}

func (s *Store) listFileRecords(ctx context.Context, req ListRequest) ([]*FileRecord, error) {
	args := []any{req.TenantID}
	where := []string{"tenant_id = $1", "is_deleted = FALSE"}

	if req.Category != nil {
		args = append(args, string(*req.Category))
		where = append(where, fmt.Sprintf("category = $%d", len(args)))
	}
	if req.Status != nil {
		args = append(args, string(*req.Status))
		where = append(where, fmt.Sprintf("status = $%d", len(args)))
	}
	if req.SourceID != nil {
		args = append(args, *req.SourceID)
		where = append(where, fmt.Sprintf("source_id = $%d", len(args)))
	}
	if req.ObjectID != nil {
		args = append(args, *req.ObjectID)
		where = append(where, fmt.Sprintf("object_id = $%d", len(args)))
	}

	limit := req.Limit
	if limit <= 0 || limit > 1000 {
		limit = 50
	}
	args = append(args, limit, req.Offset)

	q := fmt.Sprintf(
		`SELECT`+selectCols+`
		FROM stored_files
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`,
		strings.Join(where, " AND "),
		len(args)-1,
		len(args),
	)

	rows, err := s.db.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("filestore: list query: %w", err)
	}
	defer rows.Close()

	var records []*FileRecord
	for rows.Next() {
		rec, err := scanFileRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
}

func (s *Store) updateFileStatus(ctx context.Context, fileID uuid.UUID, status FileStatus) error {
	const q = `UPDATE stored_files SET status = $1 WHERE id = $2`
	tag, err := s.db.Exec(ctx, q, string(status), fileID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) softDeleteFileRecord(ctx context.Context, fileID, tenantID uuid.UUID) error {
	const q = `
		UPDATE stored_files
		SET is_deleted = TRUE, deleted_at = $1, status = 'deleted'
		WHERE id = $2 AND tenant_id = $3 AND is_deleted = FALSE`
	tag, err := s.db.Exec(ctx, q, time.Now().UTC(), fileID, tenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) findDuplicateRecord(ctx context.Context, tenantID uuid.UUID, checksum string) (*FileRecord, error) {
	q := `SELECT` + selectCols + `
		FROM stored_files
		WHERE tenant_id = $1 AND checksum_sha256 = $2 AND is_deleted = FALSE
		LIMIT 1`

	row := s.db.QueryRow(ctx, q, tenantID, checksum)
	rec, err := scanFileRecord(row)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return rec, nil
}

// scanFileRecord scans a single row from either QueryRow or Rows.Next().
// pgx.Row and pgx.Rows both satisfy this anonymous interface.
func scanFileRecord(row interface {
	Scan(dest ...any) error
}) (*FileRecord, error) {
	rec := &FileRecord{}
	var category, status string
	var objectID, sourceID, uploadedBy pgtype.UUID
	var deletedAt pgtype.Timestamptz

	err := row.Scan(
		&rec.ID,
		&rec.TenantID,
		&rec.OriginalFilename,
		&rec.ContentType,
		&rec.SizeBytes,
		&rec.ChecksumSHA256,
		&rec.Bucket,
		&rec.ObjectKey,
		&category,
		&status,
		&objectID,
		&sourceID,
		&uploadedBy,
		&rec.CreatedAt,
		&rec.UpdatedAt,
		&deletedAt,
		&rec.IsDeleted,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("filestore: scan row: %w", err)
	}

	rec.Category = FileCategory(category)
	rec.Status = FileStatus(status)
	rec.ObjectID = pgtypeToUUID(objectID)
	rec.SourceID = pgtypeToUUID(sourceID)
	rec.UploadedBy = pgtypeToUUID(uploadedBy)
	if deletedAt.Valid {
		t := deletedAt.Time
		rec.DeletedAt = &t
	}
	return rec, nil
}

func uuidToPgtype(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: [16]byte(*id), Valid: true}
}

func pgtypeToUUID(p pgtype.UUID) *uuid.UUID {
	if !p.Valid {
		return nil
	}
	id := uuid.UUID(p.Bytes)
	return &id
}
