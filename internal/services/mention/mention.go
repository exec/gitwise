package mention

import (
	"regexp"
	"strings"
)

// usernamePattern matches @username where username is 1-39 alphanumeric chars
// or hyphens (GitHub-style). The match must be preceded by start of line or a
// whitespace/punctuation character, and must NOT be preceded by an alphanumeric
// char (avoids matching email-like patterns such as user@domain).
var (
	usernamePattern = regexp.MustCompile(`(?:^|(?:\s|[^\w]))@([a-zA-Z0-9](?:[a-zA-Z0-9-]{0,37}[a-zA-Z0-9])?)`)
	fencedCodeRe    = regexp.MustCompile("(?s)```.*?```")
	inlineCodeRe    = regexp.MustCompile("`[^`]+`")
)

// Parse extracts unique @mentioned usernames from text. It is
// markdown-aware: mentions inside fenced code blocks (``` ... ```) and
// inline code (` ... `) are ignored.
func Parse(text string) []string {
	cleaned := stripCode(text)

	matches := usernamePattern.FindAllStringSubmatch(cleaned, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool, len(matches))
	var usernames []string
	for _, m := range matches {
		name := strings.ToLower(m[1])
		if !seen[name] {
			seen[name] = true
			usernames = append(usernames, name)
		}
	}
	return usernames
}

// stripCode removes fenced code blocks and inline code spans from text so
// that mentions inside them are not detected.
func stripCode(text string) string {
	text = fencedCodeRe.ReplaceAllString(text, "")
	text = inlineCodeRe.ReplaceAllString(text, "")
	return text
}
