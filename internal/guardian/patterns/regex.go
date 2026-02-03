package patterns

import (
	"regexp"

	"github.com/xdg/cloister/internal/clog"
)

// compiledPattern holds a compiled regex and its original pattern string.
type compiledPattern struct {
	regex   *regexp.Regexp
	pattern string
}

// RegexMatcher implements Matcher using compiled regular expressions.
// It checks commands against auto_approve patterns first, then manual_approve patterns.
// If no pattern matches, the command is denied.
type RegexMatcher struct {
	autoApprove   []compiledPattern
	manualApprove []compiledPattern
}

// NewRegexMatcher creates a new RegexMatcher from the given pattern slices.
// Invalid patterns are logged and skipped, not fatal.
func NewRegexMatcher(autoApprove, manualApprove []string) *RegexMatcher {
	m := &RegexMatcher{
		autoApprove:   compilePatterns(autoApprove, "auto_approve"),
		manualApprove: compilePatterns(manualApprove, "manual_approve"),
	}
	return m
}

// compilePatterns compiles a slice of regex pattern strings.
// Invalid patterns are logged and skipped.
func compilePatterns(patterns []string, category string) []compiledPattern {
	result := make([]compiledPattern, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			clog.Warn("invalid %s pattern %q: %v (skipped)", category, p, err)
			continue
		}
		result = append(result, compiledPattern{regex: re, pattern: p})
	}
	return result
}

// Match checks a command string against configured patterns.
// It checks auto_approve patterns first, then manual_approve patterns.
// Returns MatchResult indicating the action to take.
func (m *RegexMatcher) Match(cmd string) MatchResult {
	// Check auto_approve patterns first
	for _, cp := range m.autoApprove {
		if cp.regex.MatchString(cmd) {
			return MatchResult{
				Action:  AutoApprove,
				Pattern: cp.pattern,
			}
		}
	}

	// Check manual_approve patterns second
	for _, cp := range m.manualApprove {
		if cp.regex.MatchString(cmd) {
			return MatchResult{
				Action:  ManualApprove,
				Pattern: cp.pattern,
			}
		}
	}

	// No pattern matched - deny
	return MatchResult{
		Action:  Deny,
		Pattern: "",
	}
}
