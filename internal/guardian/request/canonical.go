// Package request defines types and middleware for hostexec command requests
// between cloister containers and the guardian request server.
package request

import (
	"strings"
)

// canonicalCmd reconstructs a canonical command string from an args array.
// This is the authoritative representation used for pattern matching and display.
//
// Quoting rules:
//   - Simple args (alphanumeric, hyphen, underscore, dot, slash, colon): use as-is
//   - Args with special chars or spaces: wrap in single quotes
//   - Embedded single quotes: escape using the POSIX single-quote idiom
//
// The single-quote escape idiom works by ending the current quoted section,
// adding a backslash-escaped literal quote, then starting a new quoted section.
// See the "it's" example below - the output shows the three concatenated parts.
//
// Examples:
//
//	["docker", "ps"]           → "docker ps"
//	["echo", "hello world"]    → "echo 'hello world'"
//	["echo", "it's"]           → "echo 'it'\''s'"
//	["ls", "-la", "/tmp"]      → "ls -la /tmp"
func canonicalCmd(args []string) string {
	if len(args) == 0 {
		return ""
	}

	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuote(arg)
	}
	return strings.Join(quoted, " ")
}

// shellQuote quotes a single argument for shell display.
// Returns the argument unchanged if it contains only safe characters,
// otherwise wraps it in single quotes with proper escaping.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}

	// Check if the string needs quoting
	needsQuote := false
	for _, c := range s {
		if !isSafeChar(c) {
			needsQuote = true
			break
		}
	}

	if !needsQuote {
		return s
	}

	// Quote with single quotes, escaping embedded single quotes as '\''
	var b strings.Builder
	b.WriteByte('\'')
	for _, c := range s {
		if c == '\'' {
			// End current quote, add escaped quote, start new quote
			b.WriteString("'\\''")
		} else {
			b.WriteRune(c)
		}
	}
	b.WriteByte('\'')
	return b.String()
}

// isSafeChar returns true if the character doesn't need quoting.
// Safe characters: alphanumeric, hyphen, underscore, dot, slash, colon, at, plus, equals.
func isSafeChar(c rune) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '_' || c == '.' || c == '/' || c == ':' || c == '@' || c == '+' || c == '='
}

// containsNUL returns true if the string contains a NUL byte.
// NUL bytes cannot be safely represented in shell arguments and must be rejected.
func containsNUL(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == 0 {
			return true
		}
	}
	return false
}
