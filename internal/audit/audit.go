// Package audit provides structured logging for hostexec events.
// Log entries follow a key=value format suitable for parsing and analysis.
package audit

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

// EventType represents the type of hostexec event.
type EventType string

// Event types for hostexec operations.
const (
	EventRequest     EventType = "REQUEST"
	EventAutoApprove EventType = "AUTO_APPROVE"
	EventApprove     EventType = "APPROVE"
	EventDeny        EventType = "DENY"
	EventComplete    EventType = "COMPLETE"
	EventTimeout     EventType = "TIMEOUT"
)

// Event represents a hostexec audit log entry.
type Event struct {
	// Timestamp is when the event occurred.
	Timestamp time.Time

	// Type is the event type (REQUEST, APPROVE, etc.)
	Type EventType

	// Project is the project name.
	Project string

	// Branch is the git branch name.
	Branch string

	// Cloister is the container name.
	Cloister string

	// Cmd is the command being executed.
	Cmd string

	// Pattern is the matched pattern (for AUTO_APPROVE events).
	Pattern string

	// User is the user who approved/denied (for APPROVE events).
	User string

	// Reason is the denial reason (for DENY events).
	Reason string

	// ExitCode is the command exit code (for COMPLETE events).
	ExitCode int

	// Duration is the execution time (for COMPLETE events).
	Duration time.Duration
}

// Format returns the log entry as a formatted string.
// Format: 2024-01-15T14:32:05Z HOSTEXEC REQUEST project=my-api branch=main cloister=my-api cmd="..."
func (e *Event) Format() string {
	var b strings.Builder

	// Timestamp in RFC3339 format
	b.WriteString(e.Timestamp.UTC().Format(time.RFC3339))
	b.WriteString(" HOSTEXEC ")
	b.WriteString(string(e.Type))

	// Always include project, branch, cloister
	b.WriteString(" project=")
	b.WriteString(e.Project)
	b.WriteString(" branch=")
	b.WriteString(e.Branch)
	b.WriteString(" cloister=")
	b.WriteString(e.Cloister)

	// Include cmd for all event types
	b.WriteString(" cmd=")
	b.WriteString(quoteValue(e.Cmd))

	// Type-specific fields
	switch e.Type {
	case EventAutoApprove:
		if e.Pattern != "" {
			b.WriteString(" pattern=")
			b.WriteString(quoteValue(e.Pattern))
		}
	case EventApprove:
		if e.User != "" {
			b.WriteString(" user=")
			b.WriteString(quoteValue(e.User))
		}
	case EventDeny:
		if e.Reason != "" {
			b.WriteString(" reason=")
			b.WriteString(quoteValue(e.Reason))
		}
	case EventComplete:
		b.WriteString(" exit=")
		b.WriteString(strconv.Itoa(e.ExitCode))
		b.WriteString(" duration=")
		b.WriteString(formatDuration(e.Duration))
	}

	return b.String()
}

// quoteValue returns a quoted string value.
// Values are always quoted for consistency and to handle spaces/special chars.
func quoteValue(s string) string {
	return fmt.Sprintf("%q", s)
}

// formatDuration formats a duration as a human-readable string (e.g., "2.3s", "1m30s").
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%.1fms", float64(d)/float64(time.Millisecond))
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return d.Round(time.Second).String()
}

// Logger writes audit events to an io.Writer.
type Logger struct {
	mu sync.Mutex
	w  io.Writer
}

// NewLogger creates a new audit logger that writes to the given writer.
func NewLogger(w io.Writer) *Logger {
	return &Logger{w: w}
}

// Log writes an event to the audit log.
func (l *Logger) Log(e *Event) error {
	if l == nil || l.w == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	line := e.Format() + "\n"
	_, err := l.w.Write([]byte(line))
	return err
}

// LogRequest logs a HOSTEXEC REQUEST event.
func (l *Logger) LogRequest(project, branch, cloister, cmd string) error {
	return l.Log(&Event{
		Timestamp: time.Now(),
		Type:      EventRequest,
		Project:   project,
		Branch:    branch,
		Cloister:  cloister,
		Cmd:       cmd,
	})
}

// LogAutoApprove logs a HOSTEXEC AUTO_APPROVE event.
func (l *Logger) LogAutoApprove(project, branch, cloister, cmd, pattern string) error {
	return l.Log(&Event{
		Timestamp: time.Now(),
		Type:      EventAutoApprove,
		Project:   project,
		Branch:    branch,
		Cloister:  cloister,
		Cmd:       cmd,
		Pattern:   pattern,
	})
}

// LogApprove logs a HOSTEXEC APPROVE event.
func (l *Logger) LogApprove(project, branch, cloister, cmd, user string) error {
	return l.Log(&Event{
		Timestamp: time.Now(),
		Type:      EventApprove,
		Project:   project,
		Branch:    branch,
		Cloister:  cloister,
		Cmd:       cmd,
		User:      user,
	})
}

// LogDeny logs a HOSTEXEC DENY event.
func (l *Logger) LogDeny(project, branch, cloister, cmd, reason string) error {
	return l.Log(&Event{
		Timestamp: time.Now(),
		Type:      EventDeny,
		Project:   project,
		Branch:    branch,
		Cloister:  cloister,
		Cmd:       cmd,
		Reason:    reason,
	})
}

// LogComplete logs a HOSTEXEC COMPLETE event.
func (l *Logger) LogComplete(project, branch, cloister, cmd string, exitCode int, duration time.Duration) error {
	return l.Log(&Event{
		Timestamp: time.Now(),
		Type:      EventComplete,
		Project:   project,
		Branch:    branch,
		Cloister:  cloister,
		Cmd:       cmd,
		ExitCode:  exitCode,
		Duration:  duration,
	})
}

// LogTimeout logs a HOSTEXEC TIMEOUT event.
func (l *Logger) LogTimeout(project, branch, cloister, cmd string) error {
	return l.Log(&Event{
		Timestamp: time.Now(),
		Type:      EventTimeout,
		Project:   project,
		Branch:    branch,
		Cloister:  cloister,
		Cmd:       cmd,
	})
}
