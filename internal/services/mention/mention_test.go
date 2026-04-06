package mention

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			name:     "no mentions",
			text:     "this is plain text",
			expected: nil,
		},
		{
			name:     "single mention at start of line",
			text:     "@alice please review",
			expected: []string{"alice"},
		},
		{
			name:     "mention after space",
			text:     "hey @bob what do you think?",
			expected: []string{"bob"},
		},
		{
			name:     "multiple mentions",
			text:     "@alice and @bob should review this",
			expected: []string{"alice", "bob"},
		},
		{
			name:     "duplicate mentions deduped",
			text:     "@alice @bob @alice",
			expected: []string{"alice", "bob"},
		},
		{
			name:     "case insensitive dedup",
			text:     "@Alice @alice @ALICE",
			expected: []string{"alice"},
		},
		{
			name:     "email not matched",
			text:     "send to user@example.com",
			expected: nil,
		},
		{
			name:     "mention in markdown list",
			text:     "- @alice\n- @bob",
			expected: []string{"alice", "bob"},
		},
		{
			name:     "mention after parenthesis",
			text:     "cc (@charlie)",
			expected: []string{"charlie"},
		},
		{
			name:     "mention inside fenced code block ignored",
			text:     "```\n@alice\n```\n@bob",
			expected: []string{"bob"},
		},
		{
			name:     "mention inside inline code ignored",
			text:     "use `@alice` to mention, @bob",
			expected: []string{"bob"},
		},
		{
			name:     "mention with hyphen in username",
			text:     "@my-user is great",
			expected: []string{"my-user"},
		},
		{
			name:     "mention at start of newline",
			text:     "some text\n@dave check this",
			expected: []string{"dave"},
		},
		{
			name:     "mention after colon",
			text:     "reviewer: @eve",
			expected: []string{"eve"},
		},
		{
			name:     "empty string",
			text:     "",
			expected: nil,
		},
		{
			name:     "only code blocks",
			text:     "```\n@alice\n```",
			expected: nil,
		},
		{
			name:     "mention followed by period",
			text:     "thanks @frank.",
			expected: []string{"frank"},
		},
		{
			name:     "no match for bare @",
			text:     "@ nobody",
			expected: nil,
		},
		{
			name:     "fenced code block with language tag",
			text:     "```go\n@alice\n```\n@bob is here",
			expected: []string{"bob"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Parse(tt.text)
			if len(got) != len(tt.expected) {
				t.Errorf("Parse(%q) = %v, want %v", tt.text, got, tt.expected)
				return
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("Parse(%q)[%d] = %q, want %q", tt.text, i, got[i], tt.expected[i])
				}
			}
		})
	}
}
