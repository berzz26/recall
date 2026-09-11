package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Env  string
	Port string
	Addr string
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

	return Config{
		Env:  env,
		Port: port,
		Addr: ":" + port,
	}
}