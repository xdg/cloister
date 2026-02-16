package patterns

import "testing"

// TestActionConstants verifies the Action constants have expected values.
func TestActionConstants(t *testing.T) {
	// Verify iota ordering matches spec
	if Deny != 0 {
		t.Errorf("Deny = %d, want 0", Deny)
	}
	if AutoApprove != 1 {
		t.Errorf("AutoApprove = %d, want 1", AutoApprove)
	}
	if ManualApprove != 2 {
		t.Errorf("ManualApprove = %d, want 2", ManualApprove)
	}
}

// TestActionString verifies the string representation of Actions.
func TestActionString(t *testing.T) {
	tests := []struct {
		action Action
		want   string
	}{
		{Deny, "deny"},
		{AutoApprove, "auto_approve"},
		{ManualApprove, "manual_approve"},
		{Action(99), "unknown"},
	}

	for _, tt := range tests {
		got := tt.action.String()
		if got != tt.want {
			t.Errorf("Action(%d).String() = %q, want %q", tt.action, got, tt.want)
		}
	}
}

// TestMatchResult verifies MatchResult struct works correctly.
func TestMatchResult(t *testing.T) {
	// Test creating a MatchResult with all fields
	result := MatchResult{
		Action:  AutoApprove,
		Pattern: "^docker compose ps$",
	}

	if result.Action != AutoApprove {
		t.Errorf("Action = %v, want AutoApprove", result.Action)
	}
	if result.Pattern != "^docker compose ps$" {
		t.Errorf("Pattern = %q, want %q", result.Pattern, "^docker compose ps$")
	}

	// Test default values (Deny action, empty pattern)
	var empty MatchResult
	if empty.Action != Deny {
		t.Errorf("zero value Action = %v, want Deny", empty.Action)
	}
	if empty.Pattern != "" {
		t.Errorf("zero value Pattern = %q, want empty", empty.Pattern)
	}
}

// TestMatcherInterface verifies the Matcher interface can be implemented.
func TestMatcherInterface(_ *testing.T) {
	// mockMatcher is a simple implementation to verify the interface compiles
	var _ Matcher = mockMatcher{}
}

// mockMatcher implements Matcher for testing purposes.
type mockMatcher struct {
	result MatchResult
}

func (m mockMatcher) Match(_ string) MatchResult {
	return m.result
}

// TestMockMatcherUsage demonstrates using a mock Matcher.
func TestMockMatcherUsage(t *testing.T) {
	mock := mockMatcher{
		result: MatchResult{
			Action:  ManualApprove,
			Pattern: "^git push.*$",
		},
	}

	result := mock.Match("git push origin main")
	if result.Action != ManualApprove {
		t.Errorf("Action = %v, want ManualApprove", result.Action)
	}
	if result.Pattern != "^git push.*$" {
		t.Errorf("Pattern = %q, want %q", result.Pattern, "^git push.*$")
	}
}
