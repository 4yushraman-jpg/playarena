package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment variables.
// Never call os.Getenv directly in application code; always go through Config.
type Config struct {
	// AppEnv identifies the runtime environment (development, staging, production).
	AppEnv string
	// AppPort is the TCP port the HTTP server listens on.
	AppPort int
	// DatabaseURL is the full PostgreSQL connection string including credentials.
	DatabaseURL string
	// JWTSecret is the HMAC key used to sign and verify JWT tokens.
	JWTSecret string

	// ── Storage (Phase 11) ───────────────────────────────────────────────────

	// StorageBackend selects the storage implementation: "local" (default for
	// development) or "s3" (production).
	StorageBackend string
	// StorageLocalPath is the filesystem directory for local storage.
	// Defaults to "./uploads" when empty.
	StorageLocalPath string
	// StorageLocalBaseURL is the URL prefix used to serve local files, e.g.
	// "http://localhost:8080/media/files". Defaults to that value when empty.
	StorageLocalBaseURL string
	// StorageS3Endpoint is the S3-compatible API base URL, e.g.
	// "https://s3.us-east-1.amazonaws.com" or a MinIO / Cloudflare R2 URL.
	// When empty the AWS standard endpoint is derived from StorageS3Region.
	StorageS3Endpoint string
	// StorageS3Region is the AWS region or equivalent (e.g. "us-east-1").
	StorageS3Region string
	// StorageS3Bucket is the S3 bucket name.
	StorageS3Bucket string
	// StorageS3AccessKey is the access key ID for S3 authentication.
	StorageS3AccessKey string
	// StorageS3SecretKey is the secret access key for S3 authentication.
	StorageS3SecretKey string
	// StorageCDNBaseURL is the public CDN URL prefix, e.g.
	// "https://cdn.playarena.com". file_url = StorageCDNBaseURL + "/" + key.
	StorageCDNBaseURL string
}

// Load reads configuration from environment variables and returns a validated Config.
//
// In non-production environments, Load attempts to populate unset variables from
// a .env file in the current working directory before reading any config value.
// Environment variables already set in the process always take precedence over
// .env values — godotenv.Load never overwrites an existing variable.
//
// In production (APP_ENV=production set in the process environment), the .env
// file is intentionally skipped; all variables must be injected by the
// deployment platform (Kubernetes secrets, ECS task definitions, etc.).
func Load() (*Config, error) {
	// Read APP_ENV directly from the process environment before loading .env.
	// Using os.Getenv (not getEnv with a default) means an unset APP_ENV is
	// treated as non-production, so local development works out of the box.
	if !strings.EqualFold(os.Getenv("APP_ENV"), "production") {
		// Silently ignore a missing .env file — expected in CI/CD pipelines
		// and container environments where variables come from the platform.
		_ = godotenv.Load()
	}

	portStr := getEnv("APP_PORT", "8080")
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("config: APP_PORT %q is not a valid port number (1–65535)", portStr)
	}

	cfg := &Config{
		AppEnv:      strings.ToLower(getEnv("APP_ENV", "development")),
		AppPort:     port,
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   os.Getenv("JWT_SECRET"),

		StorageBackend:      getEnv("STORAGE_BACKEND", "local"),
		StorageLocalPath:    getEnv("STORAGE_LOCAL_PATH", "./uploads"),
		StorageLocalBaseURL: getEnv("STORAGE_LOCAL_BASE_URL", ""),
		StorageS3Endpoint:   os.Getenv("STORAGE_S3_ENDPOINT"),
		StorageS3Region:     getEnv("STORAGE_S3_REGION", "us-east-1"),
		StorageS3Bucket:     os.Getenv("STORAGE_S3_BUCKET"),
		StorageS3AccessKey:  os.Getenv("STORAGE_S3_ACCESS_KEY"),
		StorageS3SecretKey:  os.Getenv("STORAGE_S3_SECRET_KEY"),
		StorageCDNBaseURL:   os.Getenv("STORAGE_CDN_BASE_URL"),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// IsDevelopment reports whether the application is running in development mode.
func (c *Config) IsDevelopment() bool { return c.AppEnv == "development" }

// IsProduction reports whether the application is running in production mode.
func (c *Config) IsProduction() bool { return c.AppEnv == "production" }

func (c *Config) validate() error {
	var errs []string

	if c.DatabaseURL == "" {
		errs = append(errs, "DATABASE_URL is required")
	}
	if c.JWTSecret == "" {
		errs = append(errs, "JWT_SECRET is required")
	}
	if c.IsProduction() && c.JWTSecret == "change-me" {
		errs = append(errs, "JWT_SECRET must not be the default value in production")
	}

	if len(errs) > 0 {
		return errors.New("config validation failed: " + strings.Join(errs, "; "))
	}
	return nil
}

// getEnv returns the value of the named environment variable, or defaultValue
// if the variable is not set or is empty.
func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
