package mirror

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/gitwise-io/gitwise/internal/models"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	c, err := NewCrypto("test-secret")
	if err != nil {
		t.Fatal(err)
	}
	// DB is nil: validation-only tests must not reach DB.
	return NewService(nil, c, NewRemote(), t.TempDir())
}

func TestConfigureRejectsInvalidDirection(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Configure(context.Background(), uuid.New(), models.ConfigureMirrorRequest{
		Direction:       "sideways",
		GithubOwner:     "o",
		GithubRepo:      "r",
		IntervalSeconds: 900,
	})
	if !errors.Is(err, ErrInvalidDirection) {
		t.Fatalf("err = %v, want ErrInvalidDirection", err)
	}
}

func TestConfigureRejectsEmptyOwnerRepo(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Configure(context.Background(), uuid.New(), models.ConfigureMirrorRequest{
		Direction:       models.MirrorPush,
		GithubOwner:     "",
		GithubRepo:      "r",
		IntervalSeconds: 900,
	})
	if !errors.Is(err, ErrInvalidTarget) {
		t.Fatalf("err = %v, want ErrInvalidTarget", err)
	}
}

func TestConfigureRejectsNegativeInterval(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Configure(context.Background(), uuid.New(), models.ConfigureMirrorRequest{
		Direction:       models.MirrorPush,
		GithubOwner:     "o",
		GithubRepo:      "r",
		IntervalSeconds: -1,
	})
	if !errors.Is(err, ErrInvalidInterval) {
		t.Fatalf("err = %v, want ErrInvalidInterval", err)
	}
}

func TestLockForReturnsSameMutexPerRepo(t *testing.T) {
	svc := newTestService(t)
	id := uuid.New()
	a := svc.lockFor(id)
	b := svc.lockFor(id)
	if a != b {
		t.Fatal("lockFor returned different mutexes for same repo")
	}
	c := svc.lockFor(uuid.New())
	if a == c {
		t.Fatal("lockFor returned same mutex for different repos")
	}
}

// TestRunDue_SkipsIfAlreadyRunning verifies that RunDue returns nil immediately
// when a previous tick is still executing (via the global TryLock guard).
func TestRunDue_SkipsIfAlreadyRunning(t *testing.T) {
	// Acquire the global runDueMu to simulate a running tick.
	if !runDueMu.TryLock() {
		t.Skip("runDueMu already locked — concurrent test interference")
	}
	defer runDueMu.Unlock()

	svc := newTestService(t)
	// RunDue must return nil immediately since runDueMu is held.
	result := svc.RunDue(context.Background())
	if result != nil {
		t.Errorf("RunDue() = %v; want nil (skipped)", result)
	}
}

// TestRunDue_Constants verifies the semaphore and timeout constants are sensible.
func TestRunDue_Constants(t *testing.T) {
	if runDueMaxInflight <= 0 {
		t.Errorf("runDueMaxInflight must be positive, got %d", runDueMaxInflight)
	}
	if runDueSyncTimeout <= 0 {
		t.Error("runDueSyncTimeout must be positive")
	}
}
