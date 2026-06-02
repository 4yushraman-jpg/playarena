package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/4yushraman-jpg/playarena/internal/platform/config"
)

// LocalBackend stores objects on the local filesystem.
// Intended for development only. In production use S3Backend.
//
// Files are placed under Config.StorageLocalPath using the storage key as a
// relative path. Directory components are created automatically.
// GetPublicURL returns Config.StorageLocalBaseURL + "/" + key so that a static
// file server mounted at StorageLocalBaseURL can serve the files.
type LocalBackend struct {
	basePath string
	baseURL  string
}

func newLocalBackend(cfg *config.Config) (*LocalBackend, error) {
	basePath := cfg.StorageLocalPath
	if basePath == "" {
		basePath = "./uploads"
	}
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return nil, fmt.Errorf("storage: local: cannot create base path %q: %w", basePath, err)
	}
	baseURL := strings.TrimRight(cfg.StorageLocalBaseURL, "/")
	if baseURL == "" {
		baseURL = "http://localhost:8080/media/files"
	}
	return &LocalBackend{basePath: basePath, baseURL: baseURL}, nil
}

func (b *LocalBackend) Upload(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	dest := filepath.Join(b.basePath, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("storage: local: mkdir %q: %w", filepath.Dir(dest), err)
	}
	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("storage: local: create %q: %w", dest, err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("storage: local: write %q: %w", dest, err)
	}
	return nil
}

func (b *LocalBackend) Delete(_ context.Context, key string) error {
	dest := filepath.Join(b.basePath, filepath.FromSlash(key))
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage: local: delete %q: %w", dest, err)
	}
	return nil
}

func (b *LocalBackend) GetPublicURL(key string) string {
	return b.baseURL + "/" + key
}
