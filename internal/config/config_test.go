package config

import (
	"strings"
	"testing"
)

const goodKey = "abcdefghijklmnopqrstuvwxyz012345" // 32 chars, not the placeholder

// setBaseEnv sets a valid baseline; individual tests override one variable.
func setBaseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://localhost/laserfeed")
	t.Setenv("CSRF_AUTH_KEY", goodKey)
	t.Setenv("APP_BASE_URL", "https://feeds.example.com")
	t.Setenv("PORT", "")
	t.Setenv("SECURE_COOKIES", "")
}

func TestLoadValidDefaults(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("APP_BASE_URL", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AppBaseURL != "http://localhost:8080" {
		t.Errorf("AppBaseURL default: got %q", cfg.AppBaseURL)
	}
	if cfg.Port != "8080" {
		t.Errorf("Port default: got %q", cfg.Port)
	}
	if !cfg.SecureCookies {
		t.Error("SecureCookies should default to true")
	}
}

func TestLoadErrors(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T)
	}{
		{"missing database url", func(t *testing.T) { t.Setenv("DATABASE_URL", "") }},
		{"missing csrf key", func(t *testing.T) { t.Setenv("CSRF_AUTH_KEY", "") }},
		{"short csrf key", func(t *testing.T) { t.Setenv("CSRF_AUTH_KEY", "tooshort") }},
		{"placeholder csrf key", func(t *testing.T) {
			t.Setenv("CSRF_AUTH_KEY", "change-me-to-a-random-32-char-secret")
		}},
		{"bad secure cookies", func(t *testing.T) { t.Setenv("SECURE_COOKIES", "yes-please") }},
		{"bad base url", func(t *testing.T) { t.Setenv("APP_BASE_URL", "not-a-url") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setBaseEnv(t)
			tc.mutate(t)
			if _, err := Load(); err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

func TestLoadSecureCookiesFalse(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("SECURE_COOKIES", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SecureCookies {
		t.Error("SecureCookies should be false")
	}
}

// Guard against accidentally shipping the example key as an acceptable value.
func TestPlaceholderKeyIsRejectedExactly(t *testing.T) {
	if !strings.EqualFold(placeholderCSRFKey, "change-me-to-a-random-32-char-secret") {
		t.Errorf("placeholder constant drifted from .env.example: %q", placeholderCSRFKey)
	}
}
