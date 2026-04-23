package review

import (
	"testing"
)

// TestIsValidReviewType verifies the allowlist for review types.
func TestIsValidReviewType(t *testing.T) {
	valid := []string{"approval", "changes_requested", "comment"}
	invalid := []string{"", "approve", "reject", "Approval"}

	for _, s := range valid {
		if !isValidReviewType(s) {
			t.Errorf("expected %q to be a valid review type", s)
		}
	}
	for _, s := range invalid {
		if isValidReviewType(s) {
			t.Errorf("expected %q to be an invalid review type", s)
		}
	}
}
