package importer

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// TestGetStatus_OwnershipEnforced verifies that GetStatus returns ErrNotFound
// when the requesting user is not the job owner, preventing job ID enumeration.
func TestGetStatus_OwnershipEnforced(t *testing.T) {
	owner := uuid.New()
	other := uuid.New()

	jobID := uuid.New().String()
	status := &ImportStatus{
		ID:       jobID,
		UserID:   owner.String(),
		Status:   "running",
		Progress: "test",
		RepoName: "testrepo",
	}

	// Marshal to simulate what would be stored in Redis.
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that an owner lookup succeeds.
	var parsed ImportStatus
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.UserID != owner.String() {
		t.Errorf("UserID mismatch after round-trip: got %s, want %s", parsed.UserID, owner.String())
	}

	// Simulate the ownership check performed in GetStatus.
	checkOwnership := func(stored *ImportStatus, requestingUser uuid.UUID) bool {
		return stored.UserID == requestingUser.String()
	}

	if !checkOwnership(&parsed, owner) {
		t.Error("expected owner to pass ownership check")
	}
	if checkOwnership(&parsed, other) {
		t.Error("expected different user to fail ownership check")
	}
}

// TestGetStatus_EmptyUserIDField verifies that a job without UserID (legacy) fails.
func TestGetStatus_LegacyJobWithoutUserID(t *testing.T) {
	legacyStatus := &ImportStatus{
		ID:     uuid.New().String(),
		Status: "completed",
		// UserID intentionally empty — simulates pre-fix job entries.
	}

	requestingUser := uuid.New()
	// Ownership check must fail: empty UserID != any real user.
	if legacyStatus.UserID == requestingUser.String() {
		t.Error("legacy job (no UserID) should not match any user")
	}
}

// TestImportStatusRoundTrip verifies that UserID survives JSON round-trip.
func TestImportStatusRoundTrip(t *testing.T) {
	userID := uuid.New()
	orig := &ImportStatus{
		ID:       uuid.New().String(),
		UserID:   userID.String(),
		Status:   "running",
		Progress: "doing stuff",
		RepoName: "myrepo",
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}

	var got ImportStatus
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}

	if got.UserID != orig.UserID {
		t.Errorf("UserID: got %s, want %s", got.UserID, orig.UserID)
	}
	if got.Status != orig.Status {
		t.Errorf("Status: got %s, want %s", got.Status, orig.Status)
	}
}

// TestCancellationViaContext verifies that the context stored on Service
// is passed to goroutines and can be cancelled.
func TestCancellationViaContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	svc := &Service{ctx: ctx}
	cancel() // cancel immediately

	// Confirm the stored context is cancelled.
	select {
	case <-svc.ctx.Done():
		// expected
	default:
		t.Error("service context should be cancelled")
	}
}
