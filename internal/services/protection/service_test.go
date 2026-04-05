package protection

import "testing"

func TestMatchBranch(t *testing.T) {
	tests := []struct {
		pattern string
		branch  string
		want    bool
	}{
		// Exact matches
		{"main", "main", true},
		{"main", "develop", false},

		// Glob patterns
		{"release/*", "release/v1.0", true},
		{"release/*", "release/v2.0.1", true},
		{"release/*", "main", false},
		{"release/*", "release/", true}, // filepath.Match: * matches empty string

		// Wildcard all
		{"*", "anything", true},
		{"*", "main", true},

		// Single char wildcard
		{"release-?", "release-1", true},
		{"release-?", "release-12", false},

		// Character classes
		{"release/v[0-9]", "release/v1", true},
		{"release/v[0-9]", "release/va", false},

		// No match
		{"feature/*", "hotfix/bug", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.branch, func(t *testing.T) {
			got := matchBranch(tt.pattern, tt.branch)
			if got != tt.want {
				t.Errorf("matchBranch(%q, %q) = %v, want %v", tt.pattern, tt.branch, got, tt.want)
			}
		})
	}
}
