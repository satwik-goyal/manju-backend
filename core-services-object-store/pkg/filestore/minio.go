package filestore

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinioConfig holds connection settings for a MinIO (or S3-compatible) endpoint.
type MinioConfig struct {
	Endpoint        string // e.g. "localhost:9000" or "s3.amazonaws.com"
	AccessKeyID     string
	SecretAccessKey string
	UseSSL          bool
	// Region defaults to "us-east-1" if empty.
	Region string
}

type minioBackend struct {
	client *minio.Client
	region string
}

// NewMinioBackend returns a StorageBackend backed by MinIO or any S3-compatible store.
// Because MinIO speaks the S3 protocol, pointing this at AWS S3 requires only
// changing Endpoint to "s3.amazonaws.com" and UseSSL to true.
func NewMinioBackend(cfg MinioConfig) (StorageBackend, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("minio: create client: %w", err)
	}
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	return &minioBackend{client: client, region: region}, nil
}

func (m *minioBackend) EnsureBucket(ctx context.Context, bucket string) error {
	exists, err := m.client.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("minio: check bucket %q: %w", bucket, err)
	}
	if exists {
		return nil
	}
	if err := m.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{Region: m.region}); err != nil {
		// Guard against a concurrent EnsureBucket call winning the race.
		if merr := minio.ToErrorResponse(err); merr.Code == "BucketAlreadyOwnedByYou" {
			return nil
		}
		return fmt.Errorf("minio: create bucket %q: %w", bucket, err)
	}
	return nil
}

func (m *minioBackend) BucketExists(ctx context.Context, bucket string) (bool, error) {
	ok, err := m.client.BucketExists(ctx, bucket)
	if err != nil {
		return false, fmt.Errorf("minio: bucket exists %q: %w", bucket, err)
	}
	return ok, nil
}

func (m *minioBackend) Upload(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error {
	_, err := m.client.PutObject(ctx, bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("minio: upload %s/%s: %w", bucket, key, err)
	}
	return nil
}

func (m *minioBackend) Download(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	obj, err := m.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("minio: download %s/%s: %w", bucket, key, err)
	}
	return obj, nil
}

func (m *minioBackend) Delete(ctx context.Context, bucket, key string) error {
	if err := m.client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("minio: delete %s/%s: %w", bucket, key, err)
	}
	return nil
}

func (m *minioBackend) Stat(ctx context.Context, bucket, key string) (*ObjectInfo, error) {
	info, err := m.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		merr := minio.ToErrorResponse(err)
		if merr.Code == "NoSuchKey" {
			return nil, fmt.Errorf("minio: stat %s/%s: %w", bucket, key, ErrNotFound)
		}
		return nil, fmt.Errorf("minio: stat %s/%s: %w", bucket, key, err)
	}
	return &ObjectInfo{
		Key:          info.Key,
		Size:         info.Size,
		ContentType:  info.ContentType,
		ETag:         info.ETag,
		LastModified: info.LastModified,
	}, nil
}

func (m *minioBackend) List(ctx context.Context, bucket, prefix string, recursive bool) ([]ObjectInfo, error) {
	var results []ObjectInfo
	for obj := range m.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: recursive,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("minio: list %s/%s: %w", bucket, prefix, obj.Err)
		}
		results = append(results, ObjectInfo{
			Key:          obj.Key,
			Size:         obj.Size,
			ETag:         obj.ETag,
			LastModified: obj.LastModified,
			// Non-recursive listing returns common prefixes as keys ending in "/".
			IsPrefix: strings.HasSuffix(obj.Key, "/"),
		})
	}
	return results, nil
}

func (m *minioBackend) PresignedGetURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	u, err := m.client.PresignedGetObject(ctx, bucket, key, expiry, nil)
	if err != nil {
		return "", fmt.Errorf("minio: presigned get %s/%s: %w", bucket, key, err)
	}
	return u.String(), nil
}

func (m *minioBackend) PresignedPutURL(ctx context.Context, bucket, key string, expiry time.Duration) (string, error) {
	u, err := m.client.PresignedPutObject(ctx, bucket, key, expiry)
	if err != nil {
		return "", fmt.Errorf("minio: presigned put %s/%s: %w", bucket, key, err)
	}
	return u.String(), nil
}
