package config

import (
	"fmt"
	"os"
)

type Config struct {
	DatabaseURL   string
	CSRFAuthKey   string
	AppBaseURL    string
	Port          string
	SecureCookies bool // should be true in production (HTTPS); set SECURE_COOKIES=false for local HTTP
}

func Load() (*Config, error) {
	cfg := &Config{
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		CSRFAuthKey:   os.Getenv("CSRF_AUTH_KEY"),
		AppBaseURL:    os.Getenv("APP_BASE_URL"),
		Port:          os.Getenv("PORT"),
		SecureCookies: os.Getenv("SECURE_COOKIES") != "false",
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
	if cfg.AppBaseURL == "" {
		cfg.AppBaseURL = "http://localhost:8080"
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}
	return cfg, nil
}
