// Package storage provides the provider-agnostic storage backend interface
// for media attachments. Implementations must not leak provider-specific types
// or errors into callers — all provider errors are wrapped before returning.
package storage

import (
	"context"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

// Backend is the provider-agnostic contract for object storage operations.
// Implementations: LocalBackend (development), S3Backend (production).
// All provider-specific details stay inside each implementation.
type Backend interface {
	// Upload writes r to the storage backend under key.
	// size is the exact byte count of r (-1 if unknown; implementation must
	// handle this gracefully, e.g. by reading into a buffer first).
	// contentType is the MIME type of the content (e.g. "image/webp").
	Upload(ctx context.Context, key string, r io.Reader, size int64, contentType string) error

	// Delete removes the object at key. Callers should treat deletion of a
	// non-existent key as a no-op (idempotent).
	Delete(ctx context.Context, key string) error

	// GetPublicURL returns the CDN-accessible URL for key.
	// For S3: cdn_base_url + "/" + key
	// For local: local_base_url + "/" + key
	GetPublicURL(key string) string
}

// New constructs the appropriate backend based on Config.StorageBackend.
// Returns an error if the configuration is invalid or incomplete.
func New(cfg *config.Config) (Backend, error) {
	switch strings.ToLower(cfg.StorageBackend) {
	case "s3":
		return newS3Backend(cfg)
	case "local", "":
		return newLocalBackend(cfg)
	default:
		return nil, fmt.Errorf("storage: unknown backend %q (valid: local, s3)", cfg.StorageBackend)
	}
}

// GenerateKey produces a unique, collision-resistant storage key for an object.
// Keys are namespaced under the org to simplify lifecycle policies.
//
// Pattern: orgs/{orgID}/{entityType}/{entityID}/{fileUUID}{suffix}
//
// suffix should include the variant label and extension, e.g. ".webp",
// "_sm.webp", "_md.webp".
func GenerateKey(orgID, entityType, entityID, fileUUID, suffix string) string {
	return path.Join("orgs", orgID, entityType, entityID, fileUUID+suffix)
}
