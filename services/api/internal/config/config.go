package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Env         string
	Port        string
	Addr        string
	DatabaseURL string
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

	return Config{
		Env:         env,
		Port:        port,
		Addr:        ":" + port,
		DatabaseURL: dbURL,
	}
}