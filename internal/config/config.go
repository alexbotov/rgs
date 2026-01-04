// Package config provides configuration management for the RGS
package config

import (
	"os"
	"time"
)

// Config holds all configuration for the RGS
type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Auth     AuthConfig
	Game     GameConfig
}

// ServerConfig holds HTTP server configuration
type ServerConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Driver string
	DSN    string
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	JWTSecret         string
	TokenExpiry       time.Duration
	SessionTimeout    time.Duration
	MaxFailedAttempts int
	LockoutDuration   time.Duration
}

// GameConfig holds game-related configuration
type GameConfig struct {
	DefaultCurrency string
	MinRTP          float64
}

// Load loads configuration from environment with defaults
func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         getEnv("RGS_PORT", "8080"),
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
		Database: DatabaseConfig{
			Driver: getEnv("RGS_DB_DRIVER", "postgres"),
			DSN:    getEnv("RGS_DB_DSN", "host=localhost dbname=rgs sslmode=disable"),
		},
		Auth: AuthConfig{
			JWTSecret:         getEnv("RGS_JWT_SECRET", "rgs-dev-secret-change-in-production"),
			TokenExpiry:       24 * time.Hour,
			SessionTimeout:    30 * time.Minute,
			MaxFailedAttempts: 3,
			LockoutDuration:   30 * time.Minute,
		},
		Game: GameConfig{
			DefaultCurrency: getEnv("RGS_CURRENCY", "USD"),
			MinRTP:          0.75, // GLI-19 §4.7.1 - minimum 75%
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
