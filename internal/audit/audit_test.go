package audit

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Fixed timestamp for deterministic testing
var testTime = time.Date(2024, 1, 15, 14, 32, 5, 0, time.UTC)

func TestEventFormat_Request(t *testing.T) {
	e := &Event{
		Timestamp: testTime,
		Type:      EventRequest,
		Project:   "my-api",
		Branch:    "main",
		Cloister:  "my-api",
		Cmd:       "docker compose up -d",
	}

	got := e.Format()
	want := `2024-01-15T14:32:05Z HOSTEXEC REQUEST project=my-api branch=main cloister=my-api cmd="docker compose up -d"`

	if got != want {
		t.Errorf("Format() =\n  got:  %q\n  want: %q", got, want)
	}
}

func TestEventFormat_AutoApprove(t *testing.T) {
	e := &Event{
		Timestamp: testTime,
		Type:      EventAutoApprove,
		Project:   "my-api",
		Branch:    "feature-auth",
		Cloister:  "my-api-feature-auth",
		Cmd:       "docker compose ps",
		Pattern:   "^docker compose ps$",
	}

	got := e.Format()
	want := `2024-01-15T14:32:05Z HOSTEXEC AUTO_APPROVE project=my-api branch=feature-auth cloister=my-api-feature-auth cmd="docker compose ps" pattern="^docker compose ps$"`

	if got != want {
		t.Errorf("Format() =\n  got:  %q\n  want: %q", got, want)
	}
}

func TestEventFormat_Approve(t *testing.T) {
	e := &Event{
		Timestamp: testTime,
		Type:      EventApprove,
		Project:   "my-api",
		Branch:    "main",
		Cloister:  "my-api",
		Cmd:       "docker compose up -d",
		User:      "david",
	}

	got := e.Format()
	want := `2024-01-15T14:32:05Z HOSTEXEC APPROVE project=my-api branch=main cloister=my-api cmd="docker compose up -d" user="david"`

	if got != want {
		t.Errorf("Format() =\n  got:  %q\n  want: %q", got, want)
	}
}

func TestEventFormat_Deny(t *testing.T) {
	e := &Event{
		Timestamp: testTime,
		Type:      EventDeny,
		Project:   "my-api",
		Branch:    "main",
		Cloister:  "my-api",
		Cmd:       "docker run --privileged alpine",
		Reason:    "pattern denied",
	}

	got := e.Format()
	want := `2024-01-15T14:32:05Z HOSTEXEC DENY project=my-api branch=main cloister=my-api cmd="docker run --privileged alpine" reason="pattern denied"`

	if got != want {
		t.Errorf("Format() =\n  got:  %q\n  want: %q", got, want)
	}
}

func TestEventFormat_Complete(t *testing.T) {
	e := &Event{
		Timestamp: testTime,
		Type:      EventComplete,
		Project:   "my-api",
		Branch:    "main",
		Cloister:  "my-api",
		Cmd:       "docker compose up -d",
		ExitCode:  0,
		Duration:  2300 * time.Millisecond,
	}

	got := e.Format()
	want := `2024-01-15T14:32:05Z HOSTEXEC COMPLETE project=my-api branch=main cloister=my-api cmd="docker compose up -d" exit=0 duration=2.3s`

	if got != want {
		t.Errorf("Format() =\n  got:  %q\n  want: %q", got, want)
	}
}

func TestEventFormat_Timeout(t *testing.T) {
	e := &Event{
		Timestamp: testTime,
		Type:      EventTimeout,
		Project:   "my-api",
		Branch:    "main",
		Cloister:  "my-api",
		Cmd:       "docker compose build",
	}

	got := e.Format()
	want := `2024-01-15T14:32:05Z HOSTEXEC TIMEOUT project=my-api branch=main cloister=my-api cmd="docker compose build"`

	if got != want {
		t.Errorf("Format() =\n  got:  %q\n  want: %q", got, want)
	}
}

func TestEventFormat_CompleteNonZeroExit(t *testing.T) {
	e := &Event{
		Timestamp: testTime,
		Type:      EventComplete,
		Project:   "my-api",
		Branch:    "main",
		Cloister:  "my-api",
		Cmd:       "make test",
		ExitCode:  1,
		Duration:  45 * time.Second,
	}

	got := e.Format()
	want := `2024-01-15T14:32:05Z HOSTEXEC COMPLETE project=my-api branch=main cloister=my-api cmd="make test" exit=1 duration=45.0s`

	if got != want {
		t.Errorf("Format() =\n  got:  %q\n  want: %q", got, want)
	}
}

func TestEventFormat_LongDuration(t *testing.T) {
	e := &Event{
		Timestamp: testTime,
		Type:      EventComplete,
		Project:   "my-api",
		Branch:    "main",
		Cloister:  "my-api",
		Cmd:       "npm install",
		ExitCode:  0,
		Duration:  90 * time.Second,
	}

	got := e.Format()
	// For durations >= 1 minute, use standard Go duration format
	want := `2024-01-15T14:32:05Z HOSTEXEC COMPLETE project=my-api branch=main cloister=my-api cmd="npm install" exit=0 duration=1m30s`

	if got != want {
		t.Errorf("Format() =\n  got:  %q\n  want: %q", got, want)
	}
}

func TestEventFormat_MillisecondDuration(t *testing.T) {
	e := &Event{
		Timestamp: testTime,
		Type:      EventComplete,
		Project:   "my-api",
		Branch:    "main",
		Cloister:  "my-api",
		Cmd:       "echo hello",
		ExitCode:  0,
		Duration:  123 * time.Millisecond,
	}

	got := e.Format()
	want := `2024-01-15T14:32:05Z HOSTEXEC COMPLETE project=my-api branch=main cloister=my-api cmd="echo hello" exit=0 duration=123.0ms`

	if got != want {
		t.Errorf("Format() =\n  got:  %q\n  want: %q", got, want)
	}
}

func TestEventFormat_SpecialCharactersInCmd(t *testing.T) {
	e := &Event{
		Timestamp: testTime,
		Type:      EventRequest,
		Project:   "my-api",
		Branch:    "main",
		Cloister:  "my-api",
		Cmd:       `echo "hello world" | grep 'hello'`,
	}

	got := e.Format()
	// Quotes within cmd should be escaped
	want := `2024-01-15T14:32:05Z HOSTEXEC REQUEST project=my-api branch=main cloister=my-api cmd="echo \"hello world\" | grep 'hello'"`

	if got != want {
		t.Errorf("Format() =\n  got:  %q\n  want: %q", got, want)
	}
}

func TestLogger_Log(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	e := &Event{
		Timestamp: testTime,
		Type:      EventRequest,
		Project:   "test-project",
		Branch:    "feature",
		Cloister:  "test-project-feature",
		Cmd:       "make build",
	}

	if err := logger.Log(e); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	got := buf.String()
	want := `2024-01-15T14:32:05Z HOSTEXEC REQUEST project=test-project branch=feature cloister=test-project-feature cmd="make build"` + "\n"

	if got != want {
		t.Errorf("Log() wrote:\n  got:  %q\n  want: %q", got, want)
	}
}

func TestLogger_LogMultipleEvents(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	events := []*Event{
		{Timestamp: testTime, Type: EventRequest, Project: "p", Branch: "b", Cloister: "c", Cmd: "cmd1"},
		{Timestamp: testTime, Type: EventApprove, Project: "p", Branch: "b", Cloister: "c", Cmd: "cmd1", User: "user1"},
		{Timestamp: testTime, Type: EventComplete, Project: "p", Branch: "b", Cloister: "c", Cmd: "cmd1", ExitCode: 0, Duration: time.Second},
	}

	for _, e := range events {
		if err := logger.Log(e); err != nil {
			t.Fatalf("Log() error = %v", err)
		}
	}

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 log lines, got %d", len(lines))
	}

	// Verify each line has the correct event type
	expectedTypes := []string{"REQUEST", "APPROVE", "COMPLETE"}
	for i, line := range lines {
		if !strings.Contains(line, "HOSTEXEC "+expectedTypes[i]) {
			t.Errorf("line %d should contain 'HOSTEXEC %s': %s", i, expectedTypes[i], line)
		}
	}
}

func TestLogger_NilLogger(t *testing.T) {
	var logger *Logger = nil

	// Should not panic
	err := logger.Log(&Event{
		Timestamp: testTime,
		Type:      EventRequest,
		Project:   "p",
		Branch:    "b",
		Cloister:  "c",
		Cmd:       "cmd",
	})

	if err != nil {
		t.Errorf("nil logger should return nil error, got %v", err)
	}
}

func TestLogger_NilWriter(t *testing.T) {
	logger := &Logger{w: nil}

	// Should not panic
	err := logger.Log(&Event{
		Timestamp: testTime,
		Type:      EventRequest,
		Project:   "p",
		Branch:    "b",
		Cloister:  "c",
		Cmd:       "cmd",
	})

	if err != nil {
		t.Errorf("nil writer should return nil error, got %v", err)
	}
}

func TestLogger_LogRequest(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	if err := logger.LogRequest("my-api", "main", "my-api", "docker ps"); err != nil {
		t.Fatalf("LogRequest() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "HOSTEXEC REQUEST") {
		t.Errorf("LogRequest() should contain 'HOSTEXEC REQUEST': %s", got)
	}
	if !strings.Contains(got, `cmd="docker ps"`) {
		t.Errorf("LogRequest() should contain cmd: %s", got)
	}
}

func TestLogger_LogAutoApprove(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	if err := logger.LogAutoApprove("my-api", "main", "my-api", "make test", "^make test$"); err != nil {
		t.Fatalf("LogAutoApprove() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "HOSTEXEC AUTO_APPROVE") {
		t.Errorf("LogAutoApprove() should contain 'HOSTEXEC AUTO_APPROVE': %s", got)
	}
	if !strings.Contains(got, `pattern="^make test$"`) {
		t.Errorf("LogAutoApprove() should contain pattern: %s", got)
	}
}

func TestLogger_LogApprove(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	if err := logger.LogApprove("my-api", "main", "my-api", "docker build .", "david"); err != nil {
		t.Fatalf("LogApprove() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "HOSTEXEC APPROVE") {
		t.Errorf("LogApprove() should contain 'HOSTEXEC APPROVE': %s", got)
	}
	if !strings.Contains(got, `user="david"`) {
		t.Errorf("LogApprove() should contain user: %s", got)
	}
}

func TestLogger_LogDeny(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	if err := logger.LogDeny("my-api", "main", "my-api", "rm -rf /", "command not allowed"); err != nil {
		t.Fatalf("LogDeny() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "HOSTEXEC DENY") {
		t.Errorf("LogDeny() should contain 'HOSTEXEC DENY': %s", got)
	}
	if !strings.Contains(got, `reason="command not allowed"`) {
		t.Errorf("LogDeny() should contain reason: %s", got)
	}
}

func TestLogger_LogComplete(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	if err := logger.LogComplete("my-api", "main", "my-api", "make build", 0, 5*time.Second); err != nil {
		t.Fatalf("LogComplete() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "HOSTEXEC COMPLETE") {
		t.Errorf("LogComplete() should contain 'HOSTEXEC COMPLETE': %s", got)
	}
	if !strings.Contains(got, "exit=0") {
		t.Errorf("LogComplete() should contain exit code: %s", got)
	}
	if !strings.Contains(got, "duration=5.0s") {
		t.Errorf("LogComplete() should contain duration: %s", got)
	}
}

func TestLogger_LogTimeout(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	if err := logger.LogTimeout("my-api", "main", "my-api", "long-running-cmd"); err != nil {
		t.Fatalf("LogTimeout() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "HOSTEXEC TIMEOUT") {
		t.Errorf("LogTimeout() should contain 'HOSTEXEC TIMEOUT': %s", got)
	}
}

func TestEventFormat_MatchesConfigReferenceFormat(t *testing.T) {
	// These tests verify the format matches the examples in config-reference.md

	tests := []struct {
		name  string
		event *Event
		// Regex pattern to match - we use regex because timestamps will differ
		pattern string
	}{
		{
			name: "request_format",
			event: &Event{
				Timestamp: testTime,
				Type:      EventRequest,
				Project:   "my-api",
				Branch:    "main",
				Cloister:  "my-api",
				Cmd:       "docker compose up -d",
			},
			pattern: `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z HOSTEXEC REQUEST project=my-api branch=main cloister=my-api cmd="docker compose up -d"$`,
		},
		{
			name: "auto_approve_format",
			event: &Event{
				Timestamp: testTime,
				Type:      EventAutoApprove,
				Project:   "my-api",
				Branch:    "feature-auth",
				Cloister:  "my-api-feature-auth",
				Cmd:       "docker compose ps",
				Pattern:   "^docker compose ps$",
			},
			pattern: `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z HOSTEXEC AUTO_APPROVE project=my-api branch=feature-auth cloister=my-api-feature-auth cmd="docker compose ps" pattern="\^docker compose ps\$"$`,
		},
		{
			name: "approve_format",
			event: &Event{
				Timestamp: testTime,
				Type:      EventApprove,
				Project:   "my-api",
				Branch:    "main",
				Cloister:  "my-api",
				Cmd:       "docker compose up -d",
				User:      "david",
			},
			pattern: `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z HOSTEXEC APPROVE project=my-api branch=main cloister=my-api cmd="docker compose up -d" user="david"$`,
		},
		{
			name: "complete_format",
			event: &Event{
				Timestamp: testTime,
				Type:      EventComplete,
				Project:   "my-api",
				Branch:    "main",
				Cloister:  "my-api",
				Cmd:       "docker compose up -d",
				ExitCode:  0,
				Duration:  2300 * time.Millisecond,
			},
			pattern: `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z HOSTEXEC COMPLETE project=my-api branch=main cloister=my-api cmd="docker compose up -d" exit=0 duration=2\.3s$`,
		},
		{
			name: "deny_format",
			event: &Event{
				Timestamp: testTime,
				Type:      EventDeny,
				Project:   "my-api",
				Branch:    "main",
				Cloister:  "my-api",
				Cmd:       "docker run --privileged alpine",
				Reason:    "pattern denied",
			},
			pattern: `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z HOSTEXEC DENY project=my-api branch=main cloister=my-api cmd="docker run --privileged alpine" reason="pattern denied"$`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.event.Format()
			re := regexp.MustCompile(tc.pattern)
			if !re.MatchString(got) {
				t.Errorf("Format() does not match expected pattern\n  got:     %q\n  pattern: %s", got, tc.pattern)
			}
		})
	}
}

func TestQuoteValue(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", `"simple"`},
		{"with space", `"with space"`},
		{`with "quotes"`, `"with \"quotes\""`},
		{"with\nnewline", `"with\nnewline"`},
		{"with\ttab", `"with\ttab"`},
		{"", `""`},
	}

	for _, tc := range tests {
		got := quoteValue(tc.input)
		if got != tc.want {
			t.Errorf("quoteValue(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{100 * time.Millisecond, "100.0ms"},
		{999 * time.Millisecond, "999.0ms"},
		{1 * time.Second, "1.0s"},
		{2300 * time.Millisecond, "2.3s"},
		{45 * time.Second, "45.0s"},
		{59 * time.Second, "59.0s"},
		{60 * time.Second, "1m0s"},
		{90 * time.Second, "1m30s"},
		{5 * time.Minute, "5m0s"},
		{5*time.Minute + 30*time.Second, "5m30s"},
	}

	for _, tc := range tests {
		got := formatDuration(tc.duration)
		if got != tc.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tc.duration, got, tc.want)
		}
	}
}

func TestEventTypeConstants(t *testing.T) {
	// Verify all event types are defined correctly
	types := []struct {
		eventType EventType
		want      string
	}{
		{EventRequest, "REQUEST"},
		{EventAutoApprove, "AUTO_APPROVE"},
		{EventApprove, "APPROVE"},
		{EventDeny, "DENY"},
		{EventComplete, "COMPLETE"},
		{EventTimeout, "TIMEOUT"},
		{EventDomainRequest, "DOMAIN_REQUEST"},
		{EventDomainApprove, "DOMAIN_APPROVE"},
		{EventDomainDeny, "DOMAIN_DENY"},
		{EventDomainTimeout, "DOMAIN_TIMEOUT"},
	}

	for _, tc := range types {
		if string(tc.eventType) != tc.want {
			t.Errorf("EventType = %q, want %q", tc.eventType, tc.want)
		}
	}
}

// Domain event format tests

func TestEventFormat_DomainRequest(t *testing.T) {
	e := &Event{
		Timestamp: testTime,
		Type:      EventDomainRequest,
		Project:   "my-api",
		Cloister:  "my-api-main",
		Domain:    "api.example.com",
	}

	got := e.Format()
	want := `2024-01-15T14:32:05Z DOMAIN DOMAIN_REQUEST project=my-api cloister=my-api-main domain="api.example.com"`

	if got != want {
		t.Errorf("Format() =\n  got:  %q\n  want: %q", got, want)
	}
}

func TestEventFormat_DomainApprove(t *testing.T) {
	e := &Event{
		Timestamp: testTime,
		Type:      EventDomainApprove,
		Project:   "my-api",
		Cloister:  "my-api-main",
		Domain:    "api.example.com",
		Scope:     "project",
		User:      "user",
	}

	got := e.Format()
	want := `2024-01-15T14:32:05Z DOMAIN DOMAIN_APPROVE project=my-api cloister=my-api-main domain="api.example.com" scope="project" user="user"`

	if got != want {
		t.Errorf("Format() =\n  got:  %q\n  want: %q", got, want)
	}
}

func TestEventFormat_DomainApprove_SessionScope(t *testing.T) {
	e := &Event{
		Timestamp: testTime,
		Type:      EventDomainApprove,
		Project:   "my-api",
		Cloister:  "my-api-main",
		Domain:    "cdn.example.com",
		Scope:     "session",
		User:      "user",
	}

	got := e.Format()
	want := `2024-01-15T14:32:05Z DOMAIN DOMAIN_APPROVE project=my-api cloister=my-api-main domain="cdn.example.com" scope="session" user="user"`

	if got != want {
		t.Errorf("Format() =\n  got:  %q\n  want: %q", got, want)
	}
}

func TestEventFormat_DomainApprove_GlobalScope(t *testing.T) {
	e := &Event{
		Timestamp: testTime,
		Type:      EventDomainApprove,
		Project:   "my-api",
		Cloister:  "my-api-main",
		Domain:    "docs.example.com",
		Scope:     "global",
		User:      "user",
	}

	got := e.Format()
	want := `2024-01-15T14:32:05Z DOMAIN DOMAIN_APPROVE project=my-api cloister=my-api-main domain="docs.example.com" scope="global" user="user"`

	if got != want {
		t.Errorf("Format() =\n  got:  %q\n  want: %q", got, want)
	}
}

func TestEventFormat_DomainDeny(t *testing.T) {
	e := &Event{
		Timestamp: testTime,
		Type:      EventDomainDeny,
		Project:   "my-api",
		Cloister:  "my-api-main",
		Domain:    "malicious.example.com",
		Reason:    "Denied by user",
	}

	got := e.Format()
	want := `2024-01-15T14:32:05Z DOMAIN DOMAIN_DENY project=my-api cloister=my-api-main domain="malicious.example.com" reason="Denied by user"`

	if got != want {
		t.Errorf("Format() =\n  got:  %q\n  want: %q", got, want)
	}
}

func TestEventFormat_DomainDeny_WithScope(t *testing.T) {
	e := &Event{
		Timestamp: testTime,
		Type:      EventDomainDeny,
		Project:   "my-api",
		Cloister:  "my-api-main",
		Domain:    "malicious.example.com",
		Scope:     "session",
	}

	got := e.Format()
	want := `2024-01-15T14:32:05Z DOMAIN DOMAIN_DENY project=my-api cloister=my-api-main domain="malicious.example.com" scope="session"`

	if got != want {
		t.Errorf("Format() =\n  got:  %q\n  want: %q", got, want)
	}
}

func TestEventFormat_DomainDeny_WithScopeAndPattern(t *testing.T) {
	e := &Event{
		Timestamp: testTime,
		Type:      EventDomainDeny,
		Project:   "my-api",
		Cloister:  "my-api-main",
		Domain:    "api.evil.example.com",
		Scope:     "project",
		Pattern:   "*.evil.example.com",
	}

	got := e.Format()
	want := `2024-01-15T14:32:05Z DOMAIN DOMAIN_DENY project=my-api cloister=my-api-main domain="api.evil.example.com" scope="project" pattern="*.evil.example.com"`

	if got != want {
		t.Errorf("Format() =\n  got:  %q\n  want: %q", got, want)
	}
}

func TestEventFormat_DomainDeny_WithAllFields(t *testing.T) {
	e := &Event{
		Timestamp: testTime,
		Type:      EventDomainDeny,
		Project:   "my-api",
		Cloister:  "my-api-main",
		Domain:    "api.evil.example.com",
		Scope:     "global",
		Pattern:   "*.evil.example.com",
		Reason:    "Denied by user",
	}

	got := e.Format()
	want := `2024-01-15T14:32:05Z DOMAIN DOMAIN_DENY project=my-api cloister=my-api-main domain="api.evil.example.com" scope="global" pattern="*.evil.example.com" reason="Denied by user"`

	if got != want {
		t.Errorf("Format() =\n  got:  %q\n  want: %q", got, want)
	}
}

func TestEventFormat_DomainTimeout(t *testing.T) {
	e := &Event{
		Timestamp: testTime,
		Type:      EventDomainTimeout,
		Project:   "my-api",
		Cloister:  "my-api-main",
		Domain:    "slow.example.com",
	}

	got := e.Format()
	want := `2024-01-15T14:32:05Z DOMAIN DOMAIN_TIMEOUT project=my-api cloister=my-api-main domain="slow.example.com"`

	if got != want {
		t.Errorf("Format() =\n  got:  %q\n  want: %q", got, want)
	}
}

func TestEventFormat_DomainWithSpecialCharacters(t *testing.T) {
	e := &Event{
		Timestamp: testTime,
		Type:      EventDomainRequest,
		Project:   "my-api",
		Cloister:  "my-api-main",
		Domain:    "sub.domain-with_chars123.example.com",
	}

	got := e.Format()
	want := `2024-01-15T14:32:05Z DOMAIN DOMAIN_REQUEST project=my-api cloister=my-api-main domain="sub.domain-with_chars123.example.com"`

	if got != want {
		t.Errorf("Format() =\n  got:  %q\n  want: %q", got, want)
	}
}

// Domain logger method tests

func TestLogger_LogDomainRequest(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	if err := logger.LogDomainRequest("my-api", "my-api-main", "api.example.com"); err != nil {
		t.Fatalf("LogDomainRequest() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "DOMAIN DOMAIN_REQUEST") {
		t.Errorf("LogDomainRequest() should contain 'DOMAIN DOMAIN_REQUEST': %s", got)
	}
	if !strings.Contains(got, `domain="api.example.com"`) {
		t.Errorf("LogDomainRequest() should contain domain: %s", got)
	}
	if !strings.Contains(got, "project=my-api") {
		t.Errorf("LogDomainRequest() should contain project: %s", got)
	}
	if !strings.Contains(got, "cloister=my-api-main") {
		t.Errorf("LogDomainRequest() should contain cloister: %s", got)
	}
	// Domain events should NOT include branch
	if strings.Contains(got, "branch=") {
		t.Errorf("LogDomainRequest() should not contain branch field: %s", got)
	}
}

func TestLogger_LogDomainApprove(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	if err := logger.LogDomainApprove("my-api", "my-api-main", "api.example.com", "project", "user"); err != nil {
		t.Fatalf("LogDomainApprove() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "DOMAIN DOMAIN_APPROVE") {
		t.Errorf("LogDomainApprove() should contain 'DOMAIN DOMAIN_APPROVE': %s", got)
	}
	if !strings.Contains(got, `domain="api.example.com"`) {
		t.Errorf("LogDomainApprove() should contain domain: %s", got)
	}
	if !strings.Contains(got, `scope="project"`) {
		t.Errorf("LogDomainApprove() should contain scope: %s", got)
	}
	if !strings.Contains(got, `user="user"`) {
		t.Errorf("LogDomainApprove() should contain user: %s", got)
	}
}

func TestLogger_LogDomainApprove_SessionScope(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	if err := logger.LogDomainApprove("my-api", "my-api-main", "cdn.example.com", "session", "user"); err != nil {
		t.Fatalf("LogDomainApprove() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, `scope="session"`) {
		t.Errorf("LogDomainApprove() should contain session scope: %s", got)
	}
}

func TestLogger_LogDomainApprove_GlobalScope(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	if err := logger.LogDomainApprove("my-api", "my-api-main", "docs.example.com", "global", "user"); err != nil {
		t.Fatalf("LogDomainApprove() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, `scope="global"`) {
		t.Errorf("LogDomainApprove() should contain global scope: %s", got)
	}
}

func TestLogger_LogDomainDeny(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	if err := logger.LogDomainDeny("my-api", "my-api-main", "malicious.example.com", "Denied by user"); err != nil {
		t.Fatalf("LogDomainDeny() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "DOMAIN DOMAIN_DENY") {
		t.Errorf("LogDomainDeny() should contain 'DOMAIN DOMAIN_DENY': %s", got)
	}
	if !strings.Contains(got, `domain="malicious.example.com"`) {
		t.Errorf("LogDomainDeny() should contain domain: %s", got)
	}
	if !strings.Contains(got, `reason="Denied by user"`) {
		t.Errorf("LogDomainDeny() should contain reason: %s", got)
	}
}

func TestLogger_LogDomainDenyWithScope(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	if err := logger.LogDomainDenyWithScope("my-api", "my-api-main", "api.evil.example.com", "project", "*.evil.example.com"); err != nil {
		t.Fatalf("LogDomainDenyWithScope() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "DOMAIN DOMAIN_DENY") {
		t.Errorf("LogDomainDenyWithScope() should contain 'DOMAIN DOMAIN_DENY': %s", got)
	}
	if !strings.Contains(got, `domain="api.evil.example.com"`) {
		t.Errorf("LogDomainDenyWithScope() should contain domain: %s", got)
	}
	if !strings.Contains(got, `scope="project"`) {
		t.Errorf("LogDomainDenyWithScope() should contain scope: %s", got)
	}
	if !strings.Contains(got, `pattern="*.evil.example.com"`) {
		t.Errorf("LogDomainDenyWithScope() should contain pattern: %s", got)
	}
	// Should NOT contain reason (not set by this method)
	if strings.Contains(got, "reason=") {
		t.Errorf("LogDomainDenyWithScope() should not contain reason: %s", got)
	}
}

func TestLogger_LogDomainDenyWithScope_NoPattern(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	if err := logger.LogDomainDenyWithScope("my-api", "my-api-main", "evil.example.com", "session", ""); err != nil {
		t.Fatalf("LogDomainDenyWithScope() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, `scope="session"`) {
		t.Errorf("LogDomainDenyWithScope() should contain scope: %s", got)
	}
	if strings.Contains(got, "pattern=") {
		t.Errorf("LogDomainDenyWithScope() should not contain pattern when empty: %s", got)
	}
}

func TestLogger_LogDomainTimeout(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	if err := logger.LogDomainTimeout("my-api", "my-api-main", "slow.example.com"); err != nil {
		t.Fatalf("LogDomainTimeout() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "DOMAIN DOMAIN_TIMEOUT") {
		t.Errorf("LogDomainTimeout() should contain 'DOMAIN DOMAIN_TIMEOUT': %s", got)
	}
	if !strings.Contains(got, `domain="slow.example.com"`) {
		t.Errorf("LogDomainTimeout() should contain domain: %s", got)
	}
}

func TestLogger_LogMultipleDomainEvents(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(&buf)

	// Simulate a typical domain approval workflow
	_ = logger.LogDomainRequest("my-api", "my-api-main", "api.example.com")
	_ = logger.LogDomainApprove("my-api", "my-api-main", "api.example.com", "project", "user")

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 log lines, got %d", len(lines))
	}

	// Verify each line has the correct event type
	expectedTypes := []string{"DOMAIN_REQUEST", "DOMAIN_APPROVE"}
	for i, line := range lines {
		if !strings.Contains(line, "DOMAIN "+expectedTypes[i]) {
			t.Errorf("line %d should contain 'DOMAIN %s': %s", i, expectedTypes[i], line)
		}
	}
}
