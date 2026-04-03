package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	AppPort         string
	DatabaseURL     string
	UploadRootDir   string
	MaxUploadSizeMB int64
}

func Load() (Config, error) {
	cfg := Config{
		AppPort:         getEnv("APP_PORT", "8080"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		UploadRootDir:   getEnv("UPLOAD_ROOT_DIR", "./data/uploads"),
		MaxUploadSizeMB: getEnvAsInt64("MAX_UPLOAD_SIZE_MB", 200),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
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
