package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
)

// placeholderCSRFKey is the example value shipped in .env.example. Refusing it
// stops an instance from launching with a publicly-known signing key.
const placeholderCSRFKey = "change-me-to-a-random-32-char-secret"

type Config struct {
	DatabaseURL   string
	CSRFAuthKey   string
	AppBaseURL    string
	Port          string
	SecureCookies bool // should be true in production (HTTPS); set SECURE_COOKIES=false for local HTTP
}

func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL: os.Getenv("DATABASE_URL"),
		CSRFAuthKey: os.Getenv("CSRF_AUTH_KEY"),
		AppBaseURL:  os.Getenv("APP_BASE_URL"),
		Port:        os.Getenv("PORT"),
	}

	// Default to true; reject typos like "False" or "0" rather than silently
	// flipping to insecure cookies (or vice versa).
	if raw := os.Getenv("SECURE_COOKIES"); raw == "" {
		cfg.SecureCookies = true
	} else {
		v, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("SECURE_COOKIES must be a boolean (true/false), got %q", raw)
		}
		cfg.SecureCookies = v
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.CSRFAuthKey == "" {
		return nil, fmt.Errorf("CSRF_AUTH_KEY is required")
	}
	if len(cfg.CSRFAuthKey) < 32 {
		return nil, fmt.Errorf("CSRF_AUTH_KEY must be at least 32 characters")
	}
	if cfg.CSRFAuthKey == placeholderCSRFKey {
		return nil, fmt.Errorf("CSRF_AUTH_KEY is still the example placeholder; generate a real one (e.g. `openssl rand -base64 32`)")
	}
	if cfg.AppBaseURL == "" {
		cfg.AppBaseURL = "http://localhost:8080"
	}
	if u, err := url.Parse(cfg.AppBaseURL); err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("APP_BASE_URL must be a fully-qualified URL (got %q)", cfg.AppBaseURL)
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	return cfg, nil
}
