package config

import (
	"strings"
	"testing"
	"time"
)

func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://localhost/x")
	t.Setenv("JWT_SECRET", strings.Repeat("s", 32))
	t.Setenv("SERVICE_API_KEY", "")
	t.Setenv("SERVICE_JWT_TTL", "")
	t.Setenv("CORS_ORIGINS", "")
}

func TestLoadServiceDefaults(t *testing.T) {
	setRequired(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServiceAPIKey != "" || cfg.ServiceJWTTTL != 15*time.Minute || cfg.CORSOrigins != nil {
		t.Fatalf("defaults = %q %v %v", cfg.ServiceAPIKey, cfg.ServiceJWTTTL, cfg.CORSOrigins)
	}
}

func TestLoadServiceSettings(t *testing.T) {
	setRequired(t)
	key := strings.Repeat("k", 32)
	t.Setenv("SERVICE_API_KEY", " "+key+"\n") // stray whitespace from .env files is trimmed
	t.Setenv("SERVICE_JWT_TTL", "5m")
	t.Setenv("CORS_ORIGINS", "https://a.test, https://b.test")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ServiceAPIKey != key || cfg.ServiceJWTTTL != 5*time.Minute {
		t.Fatalf("got %q %v", cfg.ServiceAPIKey, cfg.ServiceJWTTTL)
	}
	if len(cfg.CORSOrigins) != 2 || cfg.CORSOrigins[1] != "https://b.test" {
		t.Fatalf("cors = %v", cfg.CORSOrigins)
	}
}

func TestLoadRejectsShortServiceKey(t *testing.T) {
	setRequired(t)
	t.Setenv("SERVICE_API_KEY", "too-short")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for a key under 32 characters")
	}
}

func TestLoadRejectsBadServiceTTL(t *testing.T) {
	for _, v := range []string{"abc", "0s", "-1m"} {
		setRequired(t)
		t.Setenv("SERVICE_JWT_TTL", v)
		if _, err := Load(); err == nil {
			t.Fatalf("SERVICE_JWT_TTL=%q: expected error", v)
		}
	}
}
