package patterns

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// TestRegexMatcherImplementsMatcher verifies RegexMatcher implements the Matcher interface.
func TestRegexMatcherImplementsMatcher(t *testing.T) {
	var _ Matcher = (*RegexMatcher)(nil)
}

// TestRegexMatcherAutoApprove verifies that auto_approve patterns return AutoApprove.
func TestRegexMatcherAutoApprove(t *testing.T) {
	m := NewRegexMatcher(
		[]string{"^docker compose ps$"},
		[]string{"^docker compose (up|down|restart|build).*$"},
	)

	result := m.Match("docker compose ps")
	if result.Action != AutoApprove {
		t.Errorf("Action = %v, want AutoApprove", result.Action)
	}
	if result.Pattern != "^docker compose ps$" {
		t.Errorf("Pattern = %q, want %q", result.Pattern, "^docker compose ps$")
	}
}

// TestRegexMatcherManualApprove verifies that manual_approve patterns return ManualApprove.
func TestRegexMatcherManualApprove(t *testing.T) {
	m := NewRegexMatcher(
		[]string{"^docker compose ps$"},
		[]string{"^docker compose (up|down|restart|build).*$"},
	)

	result := m.Match("docker compose up -d")
	if result.Action != ManualApprove {
		t.Errorf("Action = %v, want ManualApprove", result.Action)
	}
	if result.Pattern != "^docker compose (up|down|restart|build).*$" {
		t.Errorf("Pattern = %q, want %q", result.Pattern, "^docker compose (up|down|restart|build).*$")
	}
}

// TestRegexMatcherDeny verifies that unmatched commands return Deny.
func TestRegexMatcherDeny(t *testing.T) {
	m := NewRegexMatcher(
		[]string{"^docker compose ps$"},
		[]string{"^docker compose (up|down|restart|build).*$"},
	)

	result := m.Match("rm -rf /")
	if result.Action != Deny {
		t.Errorf("Action = %v, want Deny", result.Action)
	}
	if result.Pattern != "" {
		t.Errorf("Pattern = %q, want empty", result.Pattern)
	}
}

// TestRegexMatcherInvalidPattern verifies that invalid patterns are logged and skipped.
func TestRegexMatcherInvalidPattern(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(nil)

	// Create matcher with an invalid regex pattern
	m := NewRegexMatcher(
		[]string{"[invalid"},        // Invalid regex (unclosed bracket)
		[]string{"^valid pattern$"}, // Valid pattern
	)

	// The invalid pattern should be logged
	logOutput := buf.String()
	if !strings.Contains(logOutput, "WARNING") {
		t.Errorf("expected WARNING in log output, got: %q", logOutput)
	}
	if !strings.Contains(logOutput, "[invalid") {
		t.Errorf("expected pattern '[invalid' in log output, got: %q", logOutput)
	}
	if !strings.Contains(logOutput, "auto_approve") {
		t.Errorf("expected 'auto_approve' category in log output, got: %q", logOutput)
	}
	if !strings.Contains(logOutput, "skipped") {
		t.Errorf("expected 'skipped' in log output, got: %q", logOutput)
	}

	// The invalid pattern should be skipped, so auto_approve should be empty
	if len(m.autoApprove) != 0 {
		t.Errorf("expected 0 auto_approve patterns, got %d", len(m.autoApprove))
	}

	// Valid pattern should still be compiled
	if len(m.manualApprove) != 1 {
		t.Errorf("expected 1 manual_approve pattern, got %d", len(m.manualApprove))
	}

	// The valid pattern should still match
	result := m.Match("valid pattern")
	if result.Action != ManualApprove {
		t.Errorf("Action = %v, want ManualApprove", result.Action)
	}
}

// TestRegexMatcherAutoApproveFirst verifies that auto_approve patterns are checked before manual_approve.
func TestRegexMatcherAutoApproveFirst(t *testing.T) {
	// Both patterns could match the same command
	m := NewRegexMatcher(
		[]string{"^docker.*$"},    // Auto-approve: any docker command
		[]string{"^docker push$"}, // Manual-approve: docker push
	)

	// "docker push" matches both, but auto_approve should win
	result := m.Match("docker push")
	if result.Action != AutoApprove {
		t.Errorf("Action = %v, want AutoApprove (auto_approve should be checked first)", result.Action)
	}
	if result.Pattern != "^docker.*$" {
		t.Errorf("Pattern = %q, want %q", result.Pattern, "^docker.*$")
	}
}

// TestRegexMatcherEmptyPatterns verifies behavior with no patterns configured.
func TestRegexMatcherEmptyPatterns(t *testing.T) {
	m := NewRegexMatcher(nil, nil)

	result := m.Match("any command")
	if result.Action != Deny {
		t.Errorf("Action = %v, want Deny", result.Action)
	}
	if result.Pattern != "" {
		t.Errorf("Pattern = %q, want empty", result.Pattern)
	}
}

// TestRegexMatcherMultiplePatterns verifies matching stops at first hit.
func TestRegexMatcherMultiplePatterns(t *testing.T) {
	m := NewRegexMatcher(
		[]string{
			"^docker compose ps$",
			"^docker compose logs$",
		},
		[]string{
			"^docker compose up$",
			"^docker compose down$",
		},
	)

	// Test first auto_approve pattern
	result := m.Match("docker compose ps")
	if result.Pattern != "^docker compose ps$" {
		t.Errorf("Pattern = %q, want first matching pattern", result.Pattern)
	}

	// Test second auto_approve pattern
	result = m.Match("docker compose logs")
	if result.Pattern != "^docker compose logs$" {
		t.Errorf("Pattern = %q, want second matching pattern", result.Pattern)
	}

	// Test first manual_approve pattern
	result = m.Match("docker compose up")
	if result.Action != ManualApprove {
		t.Errorf("Action = %v, want ManualApprove", result.Action)
	}
	if result.Pattern != "^docker compose up$" {
		t.Errorf("Pattern = %q, want first manual_approve pattern", result.Pattern)
	}
}

// TestRegexMatcherPartialMatch verifies that patterns without anchors can match substrings.
func TestRegexMatcherPartialMatch(t *testing.T) {
	m := NewRegexMatcher(
		[]string{"docker compose ps"}, // No anchors - matches substring
		nil,
	)

	// Without anchors, this should match
	result := m.Match("some prefix docker compose ps suffix")
	if result.Action != AutoApprove {
		t.Errorf("Action = %v, want AutoApprove (pattern without anchors should match substring)", result.Action)
	}
}

// TestRegexMatcherDockerComposeVariants tests the patterns from the TODO spec.
func TestRegexMatcherDockerComposeVariants(t *testing.T) {
	m := NewRegexMatcher(
		[]string{"^docker compose ps$"},
		[]string{"^docker compose (up|down|restart|build).*$"},
	)

	tests := []struct {
		cmd        string
		wantAction Action
	}{
		{"docker compose ps", AutoApprove},
		{"docker compose up", ManualApprove},
		{"docker compose up -d", ManualApprove},
		{"docker compose down", ManualApprove},
		{"docker compose restart", ManualApprove},
		{"docker compose build", ManualApprove},
		{"docker compose build --no-cache", ManualApprove},
		{"docker compose exec web bash", Deny},
		{"docker compose run --rm web echo hello", Deny},
		{"rm -rf /", Deny},
	}

	for _, tt := range tests {
		result := m.Match(tt.cmd)
		if result.Action != tt.wantAction {
			t.Errorf("Match(%q) = %v, want %v", tt.cmd, result.Action, tt.wantAction)
		}
	}
}
