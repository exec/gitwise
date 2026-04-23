package database

import (
	"testing"
)

func TestPoolConfigFromEnv_Defaults(t *testing.T) {
	t.Setenv("GITWISE_DB_MAX_CONNS", "")
	t.Setenv("GITWISE_DB_MIN_CONNS", "")

	pc := poolConfigFromEnv(50, 10)
	if pc.MaxConns != 50 {
		t.Errorf("MaxConns default: want 50, got %d", pc.MaxConns)
	}
	if pc.MinConns != 10 {
		t.Errorf("MinConns default: want 10, got %d", pc.MinConns)
	}
}

func TestPoolConfigFromEnv_EnvOverride(t *testing.T) {
	t.Setenv("GITWISE_DB_MAX_CONNS", "75")
	t.Setenv("GITWISE_DB_MIN_CONNS", "15")

	pc := poolConfigFromEnv(50, 10)
	if pc.MaxConns != 75 {
		t.Errorf("MaxConns env override: want 75, got %d", pc.MaxConns)
	}
	if pc.MinConns != 15 {
		t.Errorf("MinConns env override: want 15, got %d", pc.MinConns)
	}
}

func TestPoolConfigFromEnv_LowValueOverride(t *testing.T) {
	// An explicitly set low value should be respected (the task says "fallback 25
	// only if env is explicitly set low" — we honour whatever the operator sets).
	t.Setenv("GITWISE_DB_MAX_CONNS", "5")
	t.Setenv("GITWISE_DB_MIN_CONNS", "1")

	pc := poolConfigFromEnv(50, 10)
	if pc.MaxConns != 5 {
		t.Errorf("MaxConns low env: want 5, got %d", pc.MaxConns)
	}
	if pc.MinConns != 1 {
		t.Errorf("MinConns low env: want 1, got %d", pc.MinConns)
	}
}

func TestPoolConfigFromEnv_InvalidIgnored(t *testing.T) {
	t.Setenv("GITWISE_DB_MAX_CONNS", "not-a-number")
	t.Setenv("GITWISE_DB_MIN_CONNS", "-5")

	pc := poolConfigFromEnv(50, 10)
	if pc.MaxConns != 50 {
		t.Errorf("MaxConns invalid env: want fallback 50, got %d", pc.MaxConns)
	}
	if pc.MinConns != 10 {
		t.Errorf("MinConns non-positive env: want fallback 10, got %d", pc.MinConns)
	}
}
