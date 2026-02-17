package guardian

import (
	"fmt"
	"testing"
)

// adapterTestRecorder captures RecordDecision calls for PolicyConfigPersister tests.
type adapterTestRecorder struct {
	calls []RecordDecisionParams
	err   error // if non-nil, RecordDecision returns this error
}

func (m *adapterTestRecorder) RecordDecision(p RecordDecisionParams) error {
	m.calls = append(m.calls, p)
	return m.err
}

func TestPolicyConfigPersister_AddDomainToProject(t *testing.T) {
	rec := &adapterTestRecorder{}
	pcp := &PolicyConfigPersister{Recorder: rec}

	err := pcp.AddDomainToProject("myproject", "example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(rec.calls))
	}

	got := rec.calls[0]
	if got.Project != "myproject" {
		t.Errorf("Project = %q, want %q", got.Project, "myproject")
	}
	if got.Domain != "example.com" {
		t.Errorf("Domain = %q, want %q", got.Domain, "example.com")
	}
	if got.Scope != ScopeProject {
		t.Errorf("Scope = %q, want %q", got.Scope, ScopeProject)
	}
	if !got.Allowed {
		t.Error("Allowed = false, want true")
	}
	if got.IsPattern {
		t.Error("IsPattern = true, want false")
	}
}

func TestPolicyConfigPersister_AddDomainToGlobal(t *testing.T) {
	rec := &adapterTestRecorder{}
	pcp := &PolicyConfigPersister{Recorder: rec}

	err := pcp.AddDomainToGlobal("global.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(rec.calls))
	}

	got := rec.calls[0]
	if got.Project != "" {
		t.Errorf("Project = %q, want empty", got.Project)
	}
	if got.Domain != "global.example.com" {
		t.Errorf("Domain = %q, want %q", got.Domain, "global.example.com")
	}
	if got.Scope != ScopeGlobal {
		t.Errorf("Scope = %q, want %q", got.Scope, ScopeGlobal)
	}
	if !got.Allowed {
		t.Error("Allowed = false, want true")
	}
	if got.IsPattern {
		t.Error("IsPattern = true, want false")
	}
}

func TestPolicyConfigPersister_AddPatternToProject(t *testing.T) {
	rec := &adapterTestRecorder{}
	pcp := &PolicyConfigPersister{Recorder: rec}

	err := pcp.AddPatternToProject("myproject", "*.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(rec.calls))
	}

	got := rec.calls[0]
	if got.Project != "myproject" {
		t.Errorf("Project = %q, want %q", got.Project, "myproject")
	}
	if got.Domain != "*.example.com" {
		t.Errorf("Domain = %q, want %q", got.Domain, "*.example.com")
	}
	if got.Scope != ScopeProject {
		t.Errorf("Scope = %q, want %q", got.Scope, ScopeProject)
	}
	if !got.Allowed {
		t.Error("Allowed = false, want true")
	}
	if !got.IsPattern {
		t.Error("IsPattern = false, want true")
	}
}

func TestPolicyConfigPersister_AddPatternToGlobal(t *testing.T) {
	rec := &adapterTestRecorder{}
	pcp := &PolicyConfigPersister{Recorder: rec}

	err := pcp.AddPatternToGlobal("*.global.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(rec.calls))
	}

	got := rec.calls[0]
	if got.Project != "" {
		t.Errorf("Project = %q, want empty", got.Project)
	}
	if got.Domain != "*.global.example.com" {
		t.Errorf("Domain = %q, want %q", got.Domain, "*.global.example.com")
	}
	if got.Scope != ScopeGlobal {
		t.Errorf("Scope = %q, want %q", got.Scope, ScopeGlobal)
	}
	if !got.Allowed {
		t.Error("Allowed = false, want true")
	}
	if !got.IsPattern {
		t.Error("IsPattern = false, want true")
	}
}

func TestPolicyConfigPersister_PropagatesErrors(t *testing.T) {
	rec := &adapterTestRecorder{err: fmt.Errorf("disk full")}
	pcp := &PolicyConfigPersister{Recorder: rec}

	tests := []struct {
		name string
		fn   func() error
	}{
		{"AddDomainToProject", func() error { return pcp.AddDomainToProject("p", "d") }},
		{"AddDomainToGlobal", func() error { return pcp.AddDomainToGlobal("d") }},
		{"AddPatternToProject", func() error { return pcp.AddPatternToProject("p", "*.d") }},
		{"AddPatternToGlobal", func() error { return pcp.AddPatternToGlobal("*.d") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != "disk full" {
				t.Errorf("error = %q, want %q", err.Error(), "disk full")
			}
		})
	}
}
