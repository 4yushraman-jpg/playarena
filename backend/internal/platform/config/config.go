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

	// ── CORS ────────────────────────────────────────────────────────────────

	// CORSAllowedOrigins is the list of allowed request origins.
	// Parsed from CORS_ALLOWED_ORIGINS (comma-separated). Production must set
	// explicit domain names — never use "*" in production.
	// Defaults to common localhost addresses for development.
	CORSAllowedOrigins []string

	// ── Rate limiting ────────────────────────────────────────────────────────

	// RateLimitEnabled toggles per-IP rate limiting on auth endpoints.
	// Set RATE_LIMIT_ENABLED=false to disable (e.g., in integration tests).
	RateLimitEnabled bool
	// RateLimitAuthRPS is the sustained request rate allowed per IP on auth
	// endpoints (requests per second). Default: 10.
	RateLimitAuthRPS float64
	// RateLimitAuthBurst is the maximum burst size per IP. Default: 20.
	RateLimitAuthBurst int

	// ── Cleanup scheduler ────────────────────────────────────────────────────

	// CleanupIntervalMinutes is how often the background job removes expired
	// tokens (refresh, email verification, password reset). Default: 60.
	CleanupIntervalMinutes int

	// ── Notification email worker ─────────────────────────────────────────

	// NotifWorkerIntervalSeconds is how often the EmailWorker polls for pending
	// email channel notification rows. Default: 30.
	NotifWorkerIntervalSeconds int

	// ── Webhook notification worker ────────────────────────────────────────

	// WebhookSecretKey is the base64-encoded 32-byte AES-256-GCM key used to
	// encrypt webhook secrets at rest. Generate with:
	//   openssl rand -base64 32
	// Required in all environments.
	WebhookSecretKey string

	// WebhookWorkerIntervalSeconds is how often the WebhookWorker polls for
	// pending webhook_deliveries rows. Default: 30.
	WebhookWorkerIntervalSeconds int

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

	// ── Email ────────────────────────────────────────────────────────────────

	// EmailProvider selects the delivery backend.
	// Options: "ses" (production), "smtp" (local dev with MailHog), "log" (dev
	// default — logs email details without sending), "noop" (testing only).
	// Defaults to "log" so the binary starts without any email configuration
	// in development.
	EmailProvider string

	// EmailFromAddress is the sender email address, e.g. "noreply@playarena.com".
	// Required in all environments.
	EmailFromAddress string

	// EmailFromName is the sender display name, e.g. "PlayArena".
	// Defaults to "PlayArena".
	EmailFromName string

	// AppBaseURL is the frontend base URL used to construct verification and
	// password-reset links. Must begin with "https://" in production.
	// Example: "https://app.playarena.com"
	AppBaseURL string

	// EmailSESRegion is the AWS region for SES, e.g. "us-east-1".
	EmailSESRegion string

	// EmailSESAccessKey is the AWS access key ID for SES.
	// Leave empty on ECS/EC2 to use the IAM task/instance role (recommended).
	EmailSESAccessKey string

	// EmailSESSecretKey is the AWS secret access key for SES.
	// Leave empty on ECS/EC2 to use the IAM task/instance role.
	EmailSESSecretKey string

	// EmailSMTPHost is the SMTP server hostname (EMAIL_PROVIDER=smtp only).
	// Defaults to "localhost" for use with MailHog in local development.
	EmailSMTPHost string

	// EmailSMTPPort is the SMTP server port (EMAIL_PROVIDER=smtp only).
	// Defaults to 1025 for MailHog.
	EmailSMTPPort int

	// EmailSMTPUsername is the SMTP authentication username. May be empty.
	EmailSMTPUsername string

	// EmailSMTPPassword is the SMTP authentication password. May be empty.
	EmailSMTPPassword string

	// EmailSMTPTLS enables STARTTLS for the SMTP connection. Default: false.
	// Set to true for real SMTP servers (port 587). Leave false for MailHog.
	EmailSMTPTLS bool

	// ── Rate limiting — domain write and media upload endpoints ─────────────

	// RateLimitWriteRPS is the sustained request rate per IP for domain write
	// endpoints (POST/PATCH/DELETE on organizations, players, teams, matches,
	// etc.). More permissive than auth. Default: 30.
	RateLimitWriteRPS float64

	// RateLimitWriteBurst is the burst size for domain write endpoints. Default: 60.
	RateLimitWriteBurst int

	// RateLimitMediaRPS is the sustained rate per IP for media upload endpoints.
	// Lower than domain writes because uploads trigger S3 writes and image
	// processing. Default: 5.
	RateLimitMediaRPS float64

	// RateLimitMediaBurst is the burst size for media upload endpoints. Default: 10.
	RateLimitMediaBurst int

	// ── Reverse proxy ────────────────────────────────────────────────────────

	// TrustedProxyCIDRs is the list of CIDR ranges for trusted reverse proxies.
	// When non-empty, X-Forwarded-For and X-Real-IP headers are only processed
	// for connections originating from these addresses, preventing rate-limit
	// bypass via spoofed forwarding headers.
	//
	// Parsed from TRUSTED_PROXY_CIDRS (comma-separated, e.g.
	// "10.0.0.0/8,172.16.0.0/12"). When empty, the middleware falls back to
	// unconditional header processing for backward compatibility. In production
	// behind a load balancer, always set this to the load balancer's egress IP
	// range.
	TrustedProxyCIDRs []string

	// ── Observability ────────────────────────────────────────────────────────

	// AppInternalPort is the TCP port for the internal observability server that
	// serves /metrics, /ready, and /live. Must not equal AppPort.
	// Default: 9090. Never expose this port via the public load balancer.
	AppInternalPort int

	// AuditLogRetentionDays is the number of days to retain audit_log rows.
	// Rows older than this are deleted by the cleanup scheduler.
	// Default: 730 (2 years).
	AuditLogRetentionDays int

	// DrainTimeoutSeconds is the maximum wall-clock time allowed for a single
	// DrainOutbox call. The context deadline prevents a slow drain from blocking
	// the request goroutine indefinitely. Default: 5.
	DrainTimeoutSeconds int

	// PprofEnabled controls whether the /debug/pprof/* endpoints are mounted on
	// the internal observability server. Defaults to true in non-production
	// environments.
	PprofEnabled bool
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
	if !strings.EqualFold(os.Getenv("APP_ENV"), "production") {
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

		CORSAllowedOrigins: getEnvStringSlice(
			"CORS_ALLOWED_ORIGINS",
			[]string{"http://localhost:3000", "http://localhost:5173"},
		),

		RateLimitEnabled:   getEnvBool("RATE_LIMIT_ENABLED", true),
		RateLimitAuthRPS:   getEnvFloat("RATE_LIMIT_AUTH_RPS", 10.0),
		RateLimitAuthBurst: getEnvInt("RATE_LIMIT_AUTH_BURST", 20),

		CleanupIntervalMinutes:     getEnvInt("CLEANUP_INTERVAL_MINUTES", 60),
		NotifWorkerIntervalSeconds: getEnvInt("NOTIF_WORKER_INTERVAL_SECONDS", 30),

		WebhookSecretKey:             getEnv("WEBHOOK_SECRET_KEY", ""),
		WebhookWorkerIntervalSeconds: getEnvInt("WEBHOOK_WORKER_INTERVAL_SECONDS", 30),

		StorageBackend:      getEnv("STORAGE_BACKEND", "local"),
		StorageLocalPath:    getEnv("STORAGE_LOCAL_PATH", "./uploads"),
		StorageLocalBaseURL: getEnv("STORAGE_LOCAL_BASE_URL", ""),
		StorageS3Endpoint:   os.Getenv("STORAGE_S3_ENDPOINT"),
		StorageS3Region:     getEnv("STORAGE_S3_REGION", "us-east-1"),
		StorageS3Bucket:     os.Getenv("STORAGE_S3_BUCKET"),
		StorageS3AccessKey:  os.Getenv("STORAGE_S3_ACCESS_KEY"),
		StorageS3SecretKey:  os.Getenv("STORAGE_S3_SECRET_KEY"),
		StorageCDNBaseURL:   os.Getenv("STORAGE_CDN_BASE_URL"),

		EmailProvider:    getEnv("EMAIL_PROVIDER", "log"),
		EmailFromAddress: os.Getenv("EMAIL_FROM_ADDRESS"),
		EmailFromName:    getEnv("EMAIL_FROM_NAME", "PlayArena"),
		AppBaseURL:       os.Getenv("APP_BASE_URL"),

		EmailSESRegion:    getEnv("EMAIL_SES_REGION", "us-east-1"),
		EmailSESAccessKey: os.Getenv("EMAIL_SES_ACCESS_KEY"),
		EmailSESSecretKey: os.Getenv("EMAIL_SES_SECRET_KEY"),

		EmailSMTPHost:     getEnv("EMAIL_SMTP_HOST", "localhost"),
		EmailSMTPPort:     getEnvInt("EMAIL_SMTP_PORT", 1025),
		EmailSMTPUsername: os.Getenv("EMAIL_SMTP_USERNAME"),
		EmailSMTPPassword: os.Getenv("EMAIL_SMTP_PASSWORD"),
		EmailSMTPTLS:      getEnvBool("EMAIL_SMTP_TLS", false),

		RateLimitWriteRPS:   getEnvFloat("RATE_LIMIT_WRITE_RPS", 30.0),
		RateLimitWriteBurst: getEnvInt("RATE_LIMIT_WRITE_BURST", 60),
		RateLimitMediaRPS:   getEnvFloat("RATE_LIMIT_MEDIA_RPS", 5.0),
		RateLimitMediaBurst: getEnvInt("RATE_LIMIT_MEDIA_BURST", 10),

		TrustedProxyCIDRs: getEnvStringSlice("TRUSTED_PROXY_CIDRS", nil),

		AppInternalPort:       getEnvInt("APP_INTERNAL_PORT", 9090),
		AuditLogRetentionDays: getEnvInt("AUDIT_LOG_RETENTION_DAYS", 730),
		DrainTimeoutSeconds:   getEnvInt("DRAIN_TIMEOUT_SECONDS", 5),
		PprofEnabled:          getEnvBool("PPROF_ENABLED", true),
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

// minJWTSecretLength is the minimum acceptable byte length for JWT_SECRET in
// production. NIST SP 800-107 recommends HMAC keys of at least 256 bits (32
// bytes) for HMAC-SHA256.
const minJWTSecretLength = 32

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
	if c.IsProduction() && len(c.JWTSecret) < minJWTSecretLength {
		errs = append(errs, fmt.Sprintf(
			"JWT_SECRET must be at least %d characters in production (current length: %d)",
			minJWTSecretLength, len(c.JWTSecret),
		))
	}
	if c.RateLimitAuthRPS <= 0 {
		errs = append(errs, "RATE_LIMIT_AUTH_RPS must be positive")
	}
	if c.RateLimitAuthBurst <= 0 {
		errs = append(errs, "RATE_LIMIT_AUTH_BURST must be positive")
	}
	if c.CleanupIntervalMinutes <= 0 {
		errs = append(errs, "CLEANUP_INTERVAL_MINUTES must be positive")
	}
	if c.NotifWorkerIntervalSeconds <= 0 {
		errs = append(errs, "NOTIF_WORKER_INTERVAL_SECONDS must be positive")
	}
	if c.WebhookSecretKey == "" {
		errs = append(errs, "WEBHOOK_SECRET_KEY is required")
	}
	if c.WebhookWorkerIntervalSeconds <= 0 {
		errs = append(errs, "WEBHOOK_WORKER_INTERVAL_SECONDS must be positive")
	}

	// Email
	if c.EmailFromAddress == "" {
		errs = append(errs, "EMAIL_FROM_ADDRESS is required")
	}
	if c.AppBaseURL == "" {
		errs = append(errs, "APP_BASE_URL is required")
	}
	if c.IsProduction() {
		if !strings.HasPrefix(c.AppBaseURL, "https://") {
			errs = append(errs, "APP_BASE_URL must begin with https:// in production")
		}
		if c.EmailProvider == "noop" || c.EmailProvider == "log" {
			errs = append(errs, "EMAIL_PROVIDER must not be 'noop' or 'log' in production")
		}
	}

	// Write + media rate limits
	if c.RateLimitWriteRPS <= 0 {
		errs = append(errs, "RATE_LIMIT_WRITE_RPS must be positive")
	}
	if c.RateLimitWriteBurst <= 0 {
		errs = append(errs, "RATE_LIMIT_WRITE_BURST must be positive")
	}
	if c.RateLimitMediaRPS <= 0 {
		errs = append(errs, "RATE_LIMIT_MEDIA_RPS must be positive")
	}
	if c.RateLimitMediaBurst <= 0 {
		errs = append(errs, "RATE_LIMIT_MEDIA_BURST must be positive")
	}

	if c.AppInternalPort == c.AppPort {
		errs = append(errs, "APP_INTERNAL_PORT must differ from APP_PORT")
	}
	if c.AuditLogRetentionDays <= 0 {
		errs = append(errs, "AUDIT_LOG_RETENTION_DAYS must be positive")
	}
	if c.DrainTimeoutSeconds <= 0 {
		errs = append(errs, "DRAIN_TIMEOUT_SECONDS must be positive")
	}

	if len(errs) > 0 {
		return errors.New("config validation failed: " + strings.Join(errs, "; "))
	}
	return nil
}

// ---- environment helpers ----------------------------------------------------

// getEnv returns the value of the named environment variable, or defaultValue
// if the variable is not set or is empty.
func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return defaultValue
	}
	return b
}

func getEnvInt(key string, defaultValue int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return defaultValue
	}
	return i
}

func getEnvFloat(key string, defaultValue float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return defaultValue
	}
	return f
}

// getEnvStringSlice parses a comma-separated environment variable into a slice.
// Whitespace is trimmed from each element. Empty elements are dropped.
func getEnvStringSlice(key string, defaultValue []string) []string {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	parts := strings.Split(v, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			result = append(result, t)
		}
	}
	if len(result) == 0 {
		return defaultValue
	}
	return result
}
