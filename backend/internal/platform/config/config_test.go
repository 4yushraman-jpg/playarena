package config

import "testing"

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/playarena")
	t.Setenv("JWT_SECRET", "playarena-test-secret-with-enough-length")
	t.Setenv("WEBHOOK_SECRET_KEY", "test-webhook-secret")
	t.Setenv("EMAIL_FROM_ADDRESS", "noreply@test.example.com")
}

func TestLoadUsesFrontendURLForAppBaseURL(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("FRONTEND_URL", "http://localhost:3000")
	t.Setenv("APP_BASE_URL", "http://localhost:8080")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AppBaseURL != "http://localhost:3000" {
		t.Fatalf("AppBaseURL = %q, want frontend URL", cfg.AppBaseURL)
	}
}

func TestPlayerPersonaFlagDefaultsOff(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("FRONTEND_URL", "http://localhost:3000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.PlayerPersonaEnabled {
		t.Fatal("PlayerPersonaEnabled should default to false")
	}
}

func TestPlayerPersonaFlagEnabled(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("FRONTEND_URL", "http://localhost:3000")
	t.Setenv("GP_PLAYER_PERSONA_ENABLED", "true")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.PlayerPersonaEnabled {
		t.Fatal("PlayerPersonaEnabled should be true when GP_PLAYER_PERSONA_ENABLED=true")
	}
}

func TestLoadFallsBackToLegacyAppBaseURL(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("FRONTEND_URL", "")
	t.Setenv("APP_BASE_URL", "http://localhost:3000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AppBaseURL != "http://localhost:3000" {
		t.Fatalf("AppBaseURL = %q, want legacy APP_BASE_URL fallback", cfg.AppBaseURL)
	}
}
