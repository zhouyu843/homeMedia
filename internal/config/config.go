package config

import (
	"fmt"
	"os"
	"strconv"
)

const uploadRootDir = "/data/uploads"
const listenPort = "8080"

type Config struct {
	ListenPort      string
	DatabaseURL     string
	UploadRootDir   string
	MaxUploadSizeMB int64
	AdminUsername   string
	AdminPassword   string
	SessionSecret   string
	SessionTTLHours int64
}

func Load() (Config, error) {
	cfg := Config{
		ListenPort:      listenPort,
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		UploadRootDir:   uploadRootDir,
		MaxUploadSizeMB: getEnvAsInt64("MAX_UPLOAD_SIZE_MB", 2048),
		AdminUsername:   getEnv("ADMIN_USERNAME", "admin"),
		AdminPassword:   os.Getenv("ADMIN_PASSWORD"),
		SessionSecret:   os.Getenv("SESSION_SECRET"),
		SessionTTLHours: getEnvAsInt64("SESSION_TTL_HOURS", 24),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	if cfg.AdminPassword == "" {
		return Config{}, fmt.Errorf("ADMIN_PASSWORD is required")
	}

	if cfg.SessionSecret == "" {
		return Config{}, fmt.Errorf("SESSION_SECRET is required")
	}

	if cfg.SessionTTLHours <= 0 {
		return Config{}, fmt.Errorf("SESSION_TTL_HOURS must be greater than 0")
	}

	return cfg, nil
}

func getEnv(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}

func getEnvAsInt64(key string, fallback int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}

	return parsed
}
