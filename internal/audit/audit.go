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

// EventType represents the type of hostexec or domain event.
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

// Event types for domain approval operations.
const (
	EventDomainRequest EventType = "DOMAIN_REQUEST"
	EventDomainApprove EventType = "DOMAIN_APPROVE"
	EventDomainDeny    EventType = "DOMAIN_DENY"
	EventDomainTimeout EventType = "DOMAIN_TIMEOUT"
)

// Event represents a hostexec or domain approval audit log entry.
type Event struct {
	// Timestamp is when the event occurred.
	Timestamp time.Time

	// Type is the event type (REQUEST, APPROVE, etc.)
	Type EventType

	// Project is the project name.
	Project string

	// Cloister is the cloister name.
	Cloister string

	// Cmd is the command being executed.
	Cmd string

	// Domain is the domain being accessed (for domain approval events).
	Domain string

	// Scope is the approval scope (for domain approval events).
	Scope string

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
// Format: 2024-01-15T14:32:05Z HOSTEXEC REQUEST project=my-api cloister=my-api cmd="..."
// Format: 2024-01-15T14:32:05Z DOMAIN DOMAIN_REQUEST project=my-api cloister=my-api domain="example.com"
func (e *Event) Format() string {
	var b strings.Builder

	b.WriteString(e.Timestamp.UTC().Format(time.RFC3339))

	if e.isDomainEvent() {
		b.WriteString(" DOMAIN ")
	} else {
		b.WriteString(" HOSTEXEC ")
	}
	b.WriteString(string(e.Type))

	b.WriteString(" project=")
	b.WriteString(e.Project)
	b.WriteString(" cloister=")
	b.WriteString(e.Cloister)

	if e.isDomainEvent() {
		b.WriteString(" domain=")
		b.WriteString(quoteValue(e.Domain))
	} else {
		b.WriteString(" cmd=")
		b.WriteString(quoteValue(e.Cmd))
	}

	e.formatTypeSpecificFields(&b)

	return b.String()
}

// isDomainEvent returns true if the event is a domain approval event.
func (e *Event) isDomainEvent() bool {
	return e.Type == EventDomainRequest || e.Type == EventDomainApprove ||
		e.Type == EventDomainDeny || e.Type == EventDomainTimeout
}

// formatTypeSpecificFields appends type-specific key=value pairs to the builder.
func (e *Event) formatTypeSpecificFields(b *strings.Builder) {
	switch e.Type {
	case EventAutoApprove:
		writeOptionalField(b, "pattern", e.Pattern)
	case EventApprove:
		writeOptionalField(b, "user", e.User)
	case EventDeny:
		writeOptionalField(b, "reason", e.Reason)
	case EventComplete:
		b.WriteString(" exit=")
		b.WriteString(strconv.Itoa(e.ExitCode))
		b.WriteString(" duration=")
		b.WriteString(formatDuration(e.Duration))
	case EventDomainApprove:
		writeOptionalField(b, "scope", e.Scope)
		writeOptionalField(b, "user", e.User)
	case EventDomainDeny:
		writeOptionalField(b, "scope", e.Scope)
		writeOptionalField(b, "pattern", e.Pattern)
		writeOptionalField(b, "reason", e.Reason)
	}
}

// writeOptionalField appends " key=quoted_value" to the builder if value is non-empty.
func writeOptionalField(b *strings.Builder, key, value string) {
	if value == "" {
		return
	}
	b.WriteString(" ")
	b.WriteString(key)
	b.WriteString("=")
	b.WriteString(quoteValue(value))
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
	if err != nil {
		return fmt.Errorf("write audit event: %w", err)
	}
	return nil
}

// LogRequest logs a HOSTEXEC REQUEST event.
func (l *Logger) LogRequest(project, cloister, cmd string) error {
	return l.Log(&Event{
		Timestamp: time.Now(),
		Type:      EventRequest,
		Project:   project,
		Cloister:  cloister,
		Cmd:       cmd,
	})
}

// LogAutoApprove logs a HOSTEXEC AUTO_APPROVE event.
func (l *Logger) LogAutoApprove(project, cloister, cmd, pattern string) error {
	return l.Log(&Event{
		Timestamp: time.Now(),
		Type:      EventAutoApprove,
		Project:   project,
		Cloister:  cloister,
		Cmd:       cmd,
		Pattern:   pattern,
	})
}

// LogApprove logs a HOSTEXEC APPROVE event.
func (l *Logger) LogApprove(project, cloister, cmd, user string) error {
	return l.Log(&Event{
		Timestamp: time.Now(),
		Type:      EventApprove,
		Project:   project,
		Cloister:  cloister,
		Cmd:       cmd,
		User:      user,
	})
}

// LogDeny logs a HOSTEXEC DENY event.
func (l *Logger) LogDeny(project, cloister, cmd, reason string) error {
	return l.Log(&Event{
		Timestamp: time.Now(),
		Type:      EventDeny,
		Project:   project,
		Cloister:  cloister,
		Cmd:       cmd,
		Reason:    reason,
	})
}

// LogComplete logs a HOSTEXEC COMPLETE event.
func (l *Logger) LogComplete(project, cloister, cmd string, exitCode int, duration time.Duration) error {
	return l.Log(&Event{
		Timestamp: time.Now(),
		Type:      EventComplete,
		Project:   project,
		Cloister:  cloister,
		Cmd:       cmd,
		ExitCode:  exitCode,
		Duration:  duration,
	})
}

// LogTimeout logs a HOSTEXEC TIMEOUT event.
func (l *Logger) LogTimeout(project, cloister, cmd string) error {
	return l.Log(&Event{
		Timestamp: time.Now(),
		Type:      EventTimeout,
		Project:   project,
		Cloister:  cloister,
		Cmd:       cmd,
	})
}

// LogDomainRequest logs a DOMAIN DOMAIN_REQUEST event.
func (l *Logger) LogDomainRequest(project, cloister, domain string) error {
	return l.Log(&Event{
		Timestamp: time.Now(),
		Type:      EventDomainRequest,
		Project:   project,
		Cloister:  cloister,
		Domain:    domain,
	})
}

// LogDomainApprove logs a DOMAIN DOMAIN_APPROVE event.
func (l *Logger) LogDomainApprove(project, cloister, domain, scope, actor string) error {
	return l.Log(&Event{
		Timestamp: time.Now(),
		Type:      EventDomainApprove,
		Project:   project,
		Cloister:  cloister,
		Domain:    domain,
		Scope:     scope,
		User:      actor,
	})
}

// LogDomainDeny logs a DOMAIN DOMAIN_DENY event.
func (l *Logger) LogDomainDeny(project, cloister, domain, reason string) error {
	return l.Log(&Event{
		Timestamp: time.Now(),
		Type:      EventDomainDeny,
		Project:   project,
		Cloister:  cloister,
		Domain:    domain,
		Reason:    reason,
	})
}

// LogDomainDenyWithScope logs a DOMAIN DOMAIN_DENY event with scope and pattern fields.
// This is used by the domain approver to log processed denials with full context.
func (l *Logger) LogDomainDenyWithScope(project, cloister, domain, scope, pattern string) error {
	return l.Log(&Event{
		Timestamp: time.Now(),
		Type:      EventDomainDeny,
		Project:   project,
		Cloister:  cloister,
		Domain:    domain,
		Scope:     scope,
		Pattern:   pattern,
	})
}

// LogDomainTimeout logs a DOMAIN DOMAIN_TIMEOUT event.
func (l *Logger) LogDomainTimeout(project, cloister, domain string) error {
	return l.Log(&Event{
		Timestamp: time.Now(),
		Type:      EventDomainTimeout,
		Project:   project,
		Cloister:  cloister,
		Domain:    domain,
	})
}
