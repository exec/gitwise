package middleware

import "testing"

func TestIsHex(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"", true},
		{"0123456789abcdef", true},
		{"ABCDEF", true},
		{"abcDEF012", true},
		{"xyz", false},
		{"0123g", false},
		{"hello world", false},
		{"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", true}, // 64 chars
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isHex(tt.input)
			if got != tt.want {
				t.Errorf("isHex(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
