package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Env                string
	Port               string
	Addr               string
	DatabaseURL        string
	StorageRoot        string
	MaxUploadSize      int64
	StabilitySeconds   int
	StabilityDuration  time.Duration
	PollInterval       time.Duration
}

func Load() Config {
	_ = godotenv.Load()

	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "development"
	}

	port := os.Getenv("APP_PORT")
	if port == "" {
		port = "8080"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://recall:recall@localhost:5436/recall?sslmode=disable"
	}

	storageRoot := os.Getenv("STORAGE_ROOT")
	if storageRoot == "" {
		storageRoot = "./storage"
	}

	maxUploadSize := int64(1 << 30)
	if v := os.Getenv("MAX_UPLOAD_SIZE"); v != "" {
		var parsed int64
		if _, err := fmt.Sscanf(v, "%d", &parsed); err == nil && parsed > 0 {
			maxUploadSize = parsed
		}
	}

	stabilitySeconds := 5
	if v := os.Getenv("LOCAL_FILE_STABILITY_SECONDS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			stabilitySeconds = parsed
		}
	}

	pollInterval := 2 * time.Second
	if v := os.Getenv("PROCESSING_POLL_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			pollInterval = d
		}
	}

	return Config{
		Env:               env,
		Port:              port,
		Addr:              ":" + port,
		DatabaseURL:       dbURL,
		StorageRoot:       storageRoot,
		MaxUploadSize:     maxUploadSize,
		StabilitySeconds:  stabilitySeconds,
		StabilityDuration: time.Duration(stabilitySeconds) * time.Second,
		PollInterval:      pollInterval,
	}
}