package config

import (
	"os"
	"testing"
	"time"
)

func TestDatabaseConfig_DSN(t *testing.T) {
	tests := []struct {
		name string
		cfg  DatabaseConfig
		want string
	}{
		{
			name: "default values",
			cfg: DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				User:     "gitwise",
				Password: "secret",
				Name:     "gitwise",
				SSLMode:  "disable",
			},
			want: "postgres://gitwise:secret@localhost:5432/gitwise?sslmode=disable",
		},
		{
			name: "custom port and host",
			cfg: DatabaseConfig{
				Host:     "db.example.com",
				Port:     5433,
				User:     "admin",
				Password: "p@ss",
				Name:     "mydb",
				SSLMode:  "require",
			},
			want: "postgres://admin:p@ss@db.example.com:5433/mydb?sslmode=require",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.DSN()
			if got != tt.want {
				t.Errorf("DSN() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRedisConfig_Addr(t *testing.T) {
	tests := []struct {
		name string
		cfg  RedisConfig
		want string
	}{
		{
			name: "default",
			cfg:  RedisConfig{Host: "localhost", Port: 6379},
			want: "localhost:6379",
		},
		{
			name: "custom",
			cfg:  RedisConfig{Host: "redis.example.com", Port: 6380},
			want: "redis.example.com:6380",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.Addr()
			if got != tt.want {
				t.Errorf("Addr() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoad_Defaults(t *testing.T) {
	// Unset all env vars to test defaults
	envVars := []string{
		"GITWISE_PORT", "GITWISE_HOST", "GITWISE_SECRET",
		"GITWISE_DB_HOST", "GITWISE_DB_PORT", "GITWISE_DB_USER",
		"GITWISE_DB_PASSWORD", "GITWISE_DB_NAME", "GITWISE_DB_SSLMODE",
		"GITWISE_REDIS_HOST", "GITWISE_REDIS_PORT", "GITWISE_REDIS_PASSWORD", "GITWISE_REDIS_DB",
		"GITWISE_REPOS_PATH", "GITWISE_FRONTEND_DIST",
		"EMBEDDING_PROVIDER", "EMBEDDING_API_KEY", "EMBEDDING_MODEL",
		"EMBEDDING_DIMENSIONS", "EMBEDDING_WORKER_INTERVAL",
	}
	for _, key := range envVars {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}

	cfg := Load()

	if cfg.Port != 3000 {
		t.Errorf("Port = %d, want 3000", cfg.Port)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("Host = %q, want %q", cfg.Host, "0.0.0.0")
	}
	if cfg.Database.Host != "localhost" {
		t.Errorf("Database.Host = %q, want %q", cfg.Database.Host, "localhost")
	}
	if cfg.Database.Port != 5432 {
		t.Errorf("Database.Port = %d, want 5432", cfg.Database.Port)
	}
	if cfg.Redis.Host != "localhost" {
		t.Errorf("Redis.Host = %q, want %q", cfg.Redis.Host, "localhost")
	}
	if cfg.Redis.Port != 6379 {
		t.Errorf("Redis.Port = %d, want 6379", cfg.Redis.Port)
	}
	if cfg.Git.ReposPath != "./data/repos" {
		t.Errorf("Git.ReposPath = %q, want %q", cfg.Git.ReposPath, "./data/repos")
	}
	if cfg.Embedding.Model != "text-embedding-3-small" {
		t.Errorf("Embedding.Model = %q, want %q", cfg.Embedding.Model, "text-embedding-3-small")
	}
	if cfg.Embedding.Dimensions != 1536 {
		t.Errorf("Embedding.Dimensions = %d, want 1536", cfg.Embedding.Dimensions)
	}
	if cfg.Embedding.WorkerInterval != 5*time.Minute {
		t.Errorf("Embedding.WorkerInterval = %v, want 5m", cfg.Embedding.WorkerInterval)
	}
}

func TestLoad_FromEnv(t *testing.T) {
	t.Setenv("GITWISE_PORT", "8080")
	t.Setenv("GITWISE_HOST", "127.0.0.1")
	t.Setenv("GITWISE_DB_HOST", "db.local")
	t.Setenv("GITWISE_DB_PORT", "5433")
	t.Setenv("GITWISE_REDIS_PORT", "6380")
	t.Setenv("GITWISE_REPOS_PATH", "/var/repos")
	t.Setenv("EMBEDDING_DIMENSIONS", "768")
	t.Setenv("EMBEDDING_WORKER_INTERVAL", "10m")

	cfg := Load()

	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want %q", cfg.Host, "127.0.0.1")
	}
	if cfg.Database.Host != "db.local" {
		t.Errorf("Database.Host = %q, want %q", cfg.Database.Host, "db.local")
	}
	if cfg.Database.Port != 5433 {
		t.Errorf("Database.Port = %d, want 5433", cfg.Database.Port)
	}
	if cfg.Redis.Port != 6380 {
		t.Errorf("Redis.Port = %d, want 6380", cfg.Redis.Port)
	}
	if cfg.Git.ReposPath != "/var/repos" {
		t.Errorf("Git.ReposPath = %q, want %q", cfg.Git.ReposPath, "/var/repos")
	}
	if cfg.Embedding.Dimensions != 768 {
		t.Errorf("Embedding.Dimensions = %d, want 768", cfg.Embedding.Dimensions)
	}
	if cfg.Embedding.WorkerInterval != 10*time.Minute {
		t.Errorf("Embedding.WorkerInterval = %v, want 10m", cfg.Embedding.WorkerInterval)
	}
}

func TestEnvInt_InvalidValue(t *testing.T) {
	t.Setenv("GITWISE_PORT", "not-a-number")
	cfg := Load()
	// Should fall back to default
	if cfg.Port != 3000 {
		t.Errorf("Port = %d, want 3000 (fallback)", cfg.Port)
	}
}

func TestEnvDuration_InvalidValue(t *testing.T) {
	t.Setenv("EMBEDDING_WORKER_INTERVAL", "not-a-duration")
	cfg := Load()
	if cfg.Embedding.WorkerInterval != 5*time.Minute {
		t.Errorf("WorkerInterval = %v, want 5m (fallback)", cfg.Embedding.WorkerInterval)
	}
}
