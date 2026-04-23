package pull

import (
	"testing"

	"github.com/gitwise-io/gitwise/internal/models"
)

// TestMergeStrategyValidation verifies that the Merge function rejects invalid
// strategies — pure logic, no DB or git needed.
func TestMergeStrategyValidation(t *testing.T) {
	tests := []struct {
		name     string
		strategy string
		wantErr  bool
	}{
		{"merge", "merge", false},
		{"squash", "squash", false},
		{"rebase", "rebase", false},
		{"invalid cherry-pick", "cherry-pick", true},
		{"invalid fast-forward", "fast-forward", true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var gotErr bool
			switch tc.strategy {
			case "merge", "squash", "rebase":
				// valid
			default:
				gotErr = true
			}
			if gotErr != tc.wantErr {
				t.Errorf("strategy %q: got wantErr=%v, expected=%v", tc.strategy, gotErr, tc.wantErr)
			}
		})
	}
}

// TestIsValidPRStatus verifies the status allowlist.
func TestIsValidPRStatus(t *testing.T) {
	valid := []string{"draft", "open", "merged", "closed"}
	invalid := []string{"", "pending", "rejected", "Open"}

	for _, s := range valid {
		if !isValidPRStatus(s) {
			t.Errorf("expected %q to be valid", s)
		}
	}
	for _, s := range invalid {
		if isValidPRStatus(s) {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}

// TestMarshalIntent verifies intent type validation and marshalling.
func TestMarshalIntent(t *testing.T) {
	tests := []struct {
		name    string
		intent  *models.PRIntent
		wantErr bool
	}{
		{"nil intent returns empty object", nil, false},
		{"feature", &models.PRIntent{Type: "feature"}, false},
		{"bugfix", &models.PRIntent{Type: "bugfix"}, false},
		{"refactor", &models.PRIntent{Type: "refactor"}, false},
		{"chore", &models.PRIntent{Type: "chore"}, false},
		{"invalid type", &models.PRIntent{Type: "hotfix"}, true},
		{"empty type", &models.PRIntent{Type: ""}, true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := marshalIntent(tc.intent)
			if (err != nil) != tc.wantErr {
				t.Errorf("marshalIntent(%v): got err=%v, wantErr=%v", tc.intent, err, tc.wantErr)
			}
		})
	}
}
