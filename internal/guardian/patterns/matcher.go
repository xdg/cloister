// Package patterns provides command pattern matching for hostexec auto-approval.
package patterns

// Action represents the result of pattern matching against a command.
type Action int

const (
	// Deny indicates no pattern matched and the command should be denied.
	Deny Action = iota
	// AutoApprove indicates an auto-approve pattern matched.
	AutoApprove
	// ManualApprove indicates a manual-approve pattern matched.
	ManualApprove
)

// String returns the string representation of an Action.
func (a Action) String() string {
	switch a {
	case Deny:
		return "deny"
	case AutoApprove:
		return "auto_approve"
	case ManualApprove:
		return "manual_approve"
	default:
		return "unknown"
	}
}

// MatchResult contains the outcome of matching a command against patterns.
type MatchResult struct {
	Action  Action // The action to take (AutoApprove, ManualApprove, Deny)
	Pattern string // The pattern that matched (empty if Deny)
}

// Matcher defines the interface for command pattern matching.
type Matcher interface {
	// Match checks a command string against configured patterns.
	// Returns MatchResult indicating the action to take.
	Match(cmd string) MatchResult
}
