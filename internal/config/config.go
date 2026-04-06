package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port        int
	Host        string
	SSHPort     int
	Database    DatabaseConfig
	Redis       RedisConfig
	Git         GitConfig
	Frontend    FrontendConfig
	Embedding   EmbeddingConfig
	Secret      string
	BaseURL     string
	GitHubOAuth GitHubOAuthConfig
}

type GitHubOAuthConfig struct {
	ClientID     string
	ClientSecret string
	Enabled      bool // true if both ClientID and ClientSecret are non-empty
}

type EmbeddingConfig struct {
	Provider       string
	APIKey         string
	Model          string
	Dimensions     int
	WorkerInterval time.Duration
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

type FrontendConfig struct {
	DistPath string // path to built frontend assets (web/dist)
}

func Load() *Config {
	githubID := envStr("GITWISE_GITHUB_CLIENT_ID", "")
	githubSecret := envStr("GITWISE_GITHUB_CLIENT_SECRET", "")

	return &Config{
		Port:    envInt("GITWISE_PORT", 3000),
		Host:    envStr("GITWISE_HOST", "0.0.0.0"),
		SSHPort: envInt("GITWISE_SSH_PORT", 2222),
		Secret: envStr("GITWISE_SECRET", "change-me-in-production"),
		BaseURL: envStr("GITWISE_BASE_URL", "http://localhost:3000"),
		GitHubOAuth: GitHubOAuthConfig{
			ClientID:     githubID,
			ClientSecret: githubSecret,
			Enabled:      githubID != "" && githubSecret != "",
		},
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
		Frontend: FrontendConfig{
			DistPath: envStr("GITWISE_FRONTEND_DIST", "./web/dist"),
		},
		Embedding: EmbeddingConfig{
			Provider:       envStr("EMBEDDING_PROVIDER", ""),
			APIKey:         envStr("EMBEDDING_API_KEY", ""),
			Model:          envStr("EMBEDDING_MODEL", "text-embedding-3-small"),
			Dimensions:     envInt("EMBEDDING_DIMENSIONS", 1536),
			WorkerInterval: envDuration("EMBEDDING_WORKER_INTERVAL", 5*time.Minute),
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

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
