// Package config loads server settings from environment variables.
package config

import (
	"errors"
	"os"
	"strings"
	"time"
)

type Config struct {
	Addr        string
	DatabaseURL string
	JWTSecret   []byte
	JWTTTL      time.Duration
	// AppBaseURL prefixes invite links, e.g. https://chat.example.com.
	// Empty means invite_url is returned as a path and the client adds its own origin.
	AppBaseURL string
	// AllowedOrigins are extra host patterns accepted on the WebSocket handshake
	// besides the request's own host, e.g. "localhost:3000".
	AllowedOrigins []string
	// TrustedProxies are CIDRs/IPs whose X-Forwarded-For is trusted for client IPs.
	TrustedProxies []string
	// MediaDir is where uploaded images are stored.
	MediaDir string
	// ServiceAPIKey lets trusted backends call /service/v1. Empty turns that API off.
	ServiceAPIKey string
	// ServiceJWTTTL is the lifetime of JWTs issued through /service/v1.
	ServiceJWTTTL time.Duration
	// CORSOrigins are full origins (https://claims.example.com) whose pages may
	// call the REST API from the browser.
	CORSOrigins []string
}

func Load() (Config, error) {
	cfg := Config{
		Addr:           env("HTTP_ADDR", ":8080"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		JWTSecret:      []byte(os.Getenv("JWT_SECRET")),
		JWTTTL:         12 * time.Hour,
		AppBaseURL:     strings.TrimRight(os.Getenv("APP_BASE_URL"), "/"),
		AllowedOrigins: list(os.Getenv("ALLOWED_ORIGINS")),
		TrustedProxies: list(env("TRUSTED_PROXIES", "127.0.0.1,172.16.0.0/12,10.0.0.0/8,192.168.0.0/16")),
		MediaDir:       env("MEDIA_DIR", "./data/images"),
		ServiceAPIKey:  strings.TrimSpace(os.Getenv("SERVICE_API_KEY")),
		CORSOrigins:    list(os.Getenv("CORS_ORIGINS")),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if len(cfg.JWTSecret) < 32 {
		return Config{}, errors.New("JWT_SECRET must be at least 32 bytes")
	}
	if cfg.ServiceAPIKey != "" && len(cfg.ServiceAPIKey) < 32 {
		return Config{}, errors.New("SERVICE_API_KEY must be at least 32 characters")
	}
	ttl, err := time.ParseDuration(env("SERVICE_JWT_TTL", "15m"))
	if err != nil || ttl <= 0 {
		return Config{}, errors.New("SERVICE_JWT_TTL must be a positive duration such as 15m")
	}
	cfg.ServiceJWTTTL = ttl
	return cfg, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func list(s string) []string {
	var out []string
	for part := range strings.SplitSeq(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
