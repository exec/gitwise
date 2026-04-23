package notification

import (
	"testing"
)

// TestNotifTypeToColumn verifies that all known notification types map to the
// correct preference column and unknown types map to the always-enabled sentinel.
func TestNotifTypeToColumn(t *testing.T) {
	tests := []struct {
		notifType string
		want      string
	}{
		{"pr_review", "pr_review"},
		{"pr_merged", "pr_merged"},
		{"pr_comment", "pr_comment"},
		{"issue_comment", "issue_comment"},
		{"mention", "mention"},
		// Unknown types must default to always-enabled so callers don't need
		// to special-case new notification types before adding a preference column.
		{"unknown_type", "TRUE"},
		{"", "TRUE"},
		{"push", "TRUE"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.notifType, func(t *testing.T) {
			t.Parallel()
			got := notifTypeToColumn(tc.notifType)
			if got != tc.want {
				t.Errorf("notifTypeToColumn(%q) = %q, want %q", tc.notifType, got, tc.want)
			}
		})
	}
}

// TestErrNotificationSkippedIsDistinct verifies that the sentinel error is
// distinct and can be detected by callers using errors.Is.
func TestErrNotificationSkippedIsDistinct(t *testing.T) {
	if ErrNotificationSkipped == nil {
		t.Fatal("ErrNotificationSkipped must not be nil")
	}
	if ErrNotificationSkipped == ErrNotFound {
		t.Fatal("ErrNotificationSkipped must be distinct from ErrNotFound")
	}
}
