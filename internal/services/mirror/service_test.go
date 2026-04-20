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
