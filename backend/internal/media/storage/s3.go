package storage

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

// S3Backend stores objects in any S3-compatible object storage service.
// It speaks the S3 REST API directly using AWS Signature Version 4 for
// authentication — no vendor SDK is required.
//
// Compatible with:
//   - AWS S3
//   - Cloudflare R2 (set StorageS3Endpoint to your R2 account endpoint)
//   - MinIO (set StorageS3Endpoint to your MinIO server URL)
//   - Any S3-compatible API
//
// Configuration (via environment variables / Config):
//   - STORAGE_S3_ENDPOINT  – base URL, e.g. https://s3.amazonaws.com
//   - STORAGE_S3_REGION    – region, e.g. us-east-1
//   - STORAGE_S3_BUCKET    – bucket name
//   - STORAGE_S3_ACCESS_KEY
//   - STORAGE_S3_SECRET_KEY
//   - STORAGE_CDN_BASE_URL – public URL prefix (e.g. https://cdn.playarena.com)
type S3Backend struct {
	endpoint   string // e.g. https://s3.amazonaws.com  (no trailing slash)
	bucket     string
	cdnBaseURL string // e.g. https://cdn.playarena.com (no trailing slash)
	signer     *sigV4Signer
	client     *http.Client
}

func newS3Backend(cfg *config.Config) (*S3Backend, error) {
	endpoint := strings.TrimRight(cfg.StorageS3Endpoint, "/")
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://s3.%s.amazonaws.com", cfg.StorageS3Region)
	}
	if cfg.StorageS3Bucket == "" {
		return nil, fmt.Errorf("storage: s3: STORAGE_S3_BUCKET is required")
	}
	if cfg.StorageS3AccessKey == "" || cfg.StorageS3SecretKey == "" {
		return nil, fmt.Errorf("storage: s3: STORAGE_S3_ACCESS_KEY and STORAGE_S3_SECRET_KEY are required")
	}
	region := cfg.StorageS3Region
	if region == "" {
		region = "us-east-1"
	}
	cdnBaseURL := strings.TrimRight(cfg.StorageCDNBaseURL, "/")
	if cdnBaseURL == "" {
		// Default: use path-style S3 URL as the public URL.
		cdnBaseURL = endpoint + "/" + cfg.StorageS3Bucket
	}
	return &S3Backend{
		endpoint:   endpoint,
		bucket:     cfg.StorageS3Bucket,
		cdnBaseURL: cdnBaseURL,
		signer: &sigV4Signer{
			accessKey: cfg.StorageS3AccessKey,
			secretKey: cfg.StorageS3SecretKey,
			region:    region,
		},
		client: &http.Client{Timeout: 60 * time.Second},
	}, nil
}

// Upload writes r to the S3 bucket under key using a signed PUT request.
// contentType must be a valid MIME type (e.g. "image/webp").
func (b *S3Backend) Upload(ctx context.Context, key string, r io.Reader, size int64, contentType string) error {
	url := b.objectURL(key)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, r)
	if err != nil {
		return fmt.Errorf("storage: s3: build PUT request: %w", err)
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", contentType)

	if err := b.signer.sign(req); err != nil {
		return fmt.Errorf("storage: s3: sign PUT: %w", err)
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("storage: s3: PUT %q: %w", key, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("storage: s3: PUT %q: status %d: %s", key, resp.StatusCode, body)
	}
	return nil
}

// Delete removes the object at key. Returns nil if the key does not exist
// (S3 DELETE is idempotent: 204 or 404 are both treated as success).
func (b *S3Backend) Delete(ctx context.Context, key string) error {
	url := b.objectURL(key)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, http.NoBody)
	if err != nil {
		return fmt.Errorf("storage: s3: build DELETE request: %w", err)
	}
	if err := b.signer.sign(req); err != nil {
		return fmt.Errorf("storage: s3: sign DELETE: %w", err)
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("storage: s3: DELETE %q: %w", key, err)
	}
	defer resp.Body.Close()
	// 204 No Content = success; 404 = already gone; both are acceptable.
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("storage: s3: DELETE %q: status %d: %s", key, resp.StatusCode, body)
	}
	return nil
}

// GetPublicURL returns the CDN-accessible URL for key.
func (b *S3Backend) GetPublicURL(key string) string {
	return b.cdnBaseURL + "/" + key
}

// objectURL builds the path-style S3 object URL for key.
// Path-style: {endpoint}/{bucket}/{key}
// Works with all S3-compatible services without requiring virtual-hosted-style.
func (b *S3Backend) objectURL(key string) string {
	return b.endpoint + "/" + b.bucket + "/" + key
}
