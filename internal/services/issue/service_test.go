package issue

import (
	"testing"
)

// TestIsValidIssueStatus verifies the issue status allowlist.
func TestIsValidIssueStatus(t *testing.T) {
	valid := []string{"open", "closed", "duplicate"}
	invalid := []string{"", "pending", "merged", "Open"}

	for _, s := range valid {
		if !isValidIssueStatus(s) {
			t.Errorf("expected %q to be a valid issue status", s)
		}
	}
	for _, s := range invalid {
		if isValidIssueStatus(s) {
			t.Errorf("expected %q to be an invalid issue status", s)
		}
	}
}

// TestIsValidPriority verifies the priority allowlist.
func TestIsValidPriority(t *testing.T) {
	valid := []string{"critical", "high", "medium", "low", "none"}
	invalid := []string{"", "urgent", "Normal", "HIGH"}

	for _, s := range valid {
		if !isValidPriority(s) {
			t.Errorf("expected %q to be a valid priority", s)
		}
	}
	for _, s := range invalid {
		if isValidPriority(s) {
			t.Errorf("expected %q to be an invalid priority", s)
		}
	}
}
