package config

import (
	"os"
)

type Config struct {
	HTTPPort        string
	JWTSecret       string
	MediaMTXAPI     string
	RecordingsDir   string
	PreviewsDir     string
	PreviewInterval int // seconds
}

func Load() *Config {
	return &Config{
		HTTPPort:        getEnv("HTTP_PORT", "8080"),
		JWTSecret:       getEnv("JWT_SECRET", "change-me-in-production-please"),
		MediaMTXAPI:     getEnv("MEDIAMTX_API", "http://localhost:9997"),
		RecordingsDir:   getEnv("RECORDINGS_DIR", "./data/recordings"),
		PreviewsDir:     getEnv("PREVIEWS_DIR", "./data/previews"),
		PreviewInterval: 10,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
