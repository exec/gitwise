package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port     int
	Host     string
	Database DatabaseConfig
	Redis    RedisConfig
	Git      GitConfig
	Secret   string
}

type DatabaseConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	SSLMode  string
}

func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.Name, d.SSLMode,
	)
}

type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
}

func (r RedisConfig) Addr() string {
	return fmt.Sprintf("%s:%d", r.Host, r.Port)
}

type GitConfig struct {
	ReposPath string
}

func Load() *Config {
	return &Config{
		Port:   envInt("GITWISE_PORT", 3000),
		Host:   envStr("GITWISE_HOST", "0.0.0.0"),
		Secret: envStr("GITWISE_SECRET", "change-me-in-production"),
		Database: DatabaseConfig{
			Host:     envStr("GITWISE_DB_HOST", "localhost"),
			Port:     envInt("GITWISE_DB_PORT", 5432),
			User:     envStr("GITWISE_DB_USER", "gitwise"),
			Password: envStr("GITWISE_DB_PASSWORD", "gitwise"),
			Name:     envStr("GITWISE_DB_NAME", "gitwise"),
			SSLMode:  envStr("GITWISE_DB_SSLMODE", "disable"),
		},
		Redis: RedisConfig{
			Host:     envStr("GITWISE_REDIS_HOST", "localhost"),
			Port:     envInt("GITWISE_REDIS_PORT", 6379),
			Password: envStr("GITWISE_REDIS_PASSWORD", ""),
			DB:       envInt("GITWISE_REDIS_DB", 0),
		},
		Git: GitConfig{
			ReposPath: envStr("GITWISE_REPOS_PATH", "./data/repos"),
		},
	}
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
