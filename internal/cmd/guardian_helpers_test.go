package cmd

import (
	"testing"

	"github.com/xdg/cloister/internal/config"
)

func TestApprovalsToAllowEntries_DomainsOnly(t *testing.T) {
	approvals := &config.Decisions{
		Domains: []string{"example.com", "api.example.com"},
	}

	entries := approvalsToAllowEntries(approvals)

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Domain != "example.com" {
		t.Errorf("entries[0].Domain = %q, want %q", entries[0].Domain, "example.com")
	}
	if entries[0].Pattern != "" {
		t.Errorf("entries[0].Pattern should be empty, got %q", entries[0].Pattern)
	}

	if entries[1].Domain != "api.example.com" {
		t.Errorf("entries[1].Domain = %q, want %q", entries[1].Domain, "api.example.com")
	}
	if entries[1].Pattern != "" {
		t.Errorf("entries[1].Pattern should be empty, got %q", entries[1].Pattern)
	}
}

func TestApprovalsToAllowEntries_PatternsOnly(t *testing.T) {
	approvals := &config.Decisions{
		Patterns: []string{"*.example.com", "*.cdn.example.com"},
	}

	entries := approvalsToAllowEntries(approvals)

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Pattern != "*.example.com" {
		t.Errorf("entries[0].Pattern = %q, want %q", entries[0].Pattern, "*.example.com")
	}
	if entries[0].Domain != "" {
		t.Errorf("entries[0].Domain should be empty, got %q", entries[0].Domain)
	}

	if entries[1].Pattern != "*.cdn.example.com" {
		t.Errorf("entries[1].Pattern = %q, want %q", entries[1].Pattern, "*.cdn.example.com")
	}
	if entries[1].Domain != "" {
		t.Errorf("entries[1].Domain should be empty, got %q", entries[1].Domain)
	}
}

func TestApprovalsToAllowEntries_DomainsAndPatterns(t *testing.T) {
	approvals := &config.Decisions{
		Domains:  []string{"example.com"},
		Patterns: []string{"*.example.com"},
	}

	entries := approvalsToAllowEntries(approvals)

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	// Domains come first
	if entries[0].Domain != "example.com" {
		t.Errorf("entries[0].Domain = %q, want %q", entries[0].Domain, "example.com")
	}
	if entries[0].Pattern != "" {
		t.Errorf("entries[0].Pattern should be empty, got %q", entries[0].Pattern)
	}

	// Patterns come after domains
	if entries[1].Pattern != "*.example.com" {
		t.Errorf("entries[1].Pattern = %q, want %q", entries[1].Pattern, "*.example.com")
	}
	if entries[1].Domain != "" {
		t.Errorf("entries[1].Domain should be empty, got %q", entries[1].Domain)
	}
}

func TestApprovalsToAllowEntries_Empty(t *testing.T) {
	approvals := &config.Decisions{
		Domains:  []string{},
		Patterns: []string{},
	}

	entries := approvalsToAllowEntries(approvals)

	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}

	// Verify it returns a non-nil empty slice (not nil)
	if entries == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
}

func TestApprovalsToAllowEntries_NilSlices(t *testing.T) {
	approvals := &config.Decisions{
		Domains:  nil,
		Patterns: nil,
	}

	entries := approvalsToAllowEntries(approvals)

	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}

	// Verify it returns a non-nil empty slice (not nil)
	if entries == nil {
		t.Error("expected non-nil empty slice, got nil")
	}
}

func TestMergeStaticAndApprovals_GlobalOnly(t *testing.T) {
	// Simulates the global allowlist merging pattern from runGuardianProxy:
	// globalAllow = append(cfg.Proxy.Allow, approvalsToAllowEntries(globalApprovals)...)
	staticAllow := []config.AllowEntry{
		{Domain: "golang.org"},
		{Domain: "api.anthropic.com"},
	}
	approvals := &config.Decisions{
		Domains:  []string{"approved.example.com"},
		Patterns: []string{"*.cdn.example.com"},
	}

	merged := append(staticAllow, approvalsToAllowEntries(approvals)...)

	if len(merged) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(merged))
	}

	// Static entries come first
	if merged[0].Domain != "golang.org" {
		t.Errorf("merged[0].Domain = %q, want %q", merged[0].Domain, "golang.org")
	}
	if merged[1].Domain != "api.anthropic.com" {
		t.Errorf("merged[1].Domain = %q, want %q", merged[1].Domain, "api.anthropic.com")
	}

	// Approved domain entries come next
	if merged[2].Domain != "approved.example.com" {
		t.Errorf("merged[2].Domain = %q, want %q", merged[2].Domain, "approved.example.com")
	}
	if merged[2].Pattern != "" {
		t.Errorf("merged[2].Pattern should be empty, got %q", merged[2].Pattern)
	}

	// Approved pattern entries come last
	if merged[3].Pattern != "*.cdn.example.com" {
		t.Errorf("merged[3].Pattern = %q, want %q", merged[3].Pattern, "*.cdn.example.com")
	}
	if merged[3].Domain != "" {
		t.Errorf("merged[3].Domain should be empty, got %q", merged[3].Domain)
	}
}

func TestMergeStaticAndApprovals_ProjectWithAllFourSources(t *testing.T) {
	// Simulates the project allowlist merging pattern from loadProjectAllowlist:
	// merged = MergeAllowlists(global, project)
	// merged = append(merged, approvalsToAllowEntries(globalApprovals)...)
	// merged = append(merged, approvalsToAllowEntries(projectApprovals)...)
	globalStatic := []config.AllowEntry{
		{Domain: "golang.org"},
	}
	projectStatic := []config.AllowEntry{
		{Domain: "custom.project.com"},
	}
	globalApprovals := &config.Decisions{
		Domains: []string{"approved-global.com"},
	}
	projectApprovals := &config.Decisions{
		Domains:  []string{"approved-project.com"},
		Patterns: []string{"*.internal.corp"},
	}

	merged := config.MergeAllowlists(globalStatic, projectStatic)
	merged = append(merged, approvalsToAllowEntries(globalApprovals)...)
	merged = append(merged, approvalsToAllowEntries(projectApprovals)...)

	if len(merged) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(merged))
	}

	// Global static first
	if merged[0].Domain != "golang.org" {
		t.Errorf("merged[0].Domain = %q, want %q", merged[0].Domain, "golang.org")
	}

	// Project static second
	if merged[1].Domain != "custom.project.com" {
		t.Errorf("merged[1].Domain = %q, want %q", merged[1].Domain, "custom.project.com")
	}

	// Global approved third
	if merged[2].Domain != "approved-global.com" {
		t.Errorf("merged[2].Domain = %q, want %q", merged[2].Domain, "approved-global.com")
	}

	// Project approved domain fourth
	if merged[3].Domain != "approved-project.com" {
		t.Errorf("merged[3].Domain = %q, want %q", merged[3].Domain, "approved-project.com")
	}

	// Project approved pattern fifth
	if merged[4].Pattern != "*.internal.corp" {
		t.Errorf("merged[4].Pattern = %q, want %q", merged[4].Pattern, "*.internal.corp")
	}
	if merged[4].Domain != "" {
		t.Errorf("merged[4].Domain should be empty, got %q", merged[4].Domain)
	}
}

func TestMergeStaticAndApprovals_EmptyApprovals(t *testing.T) {
	staticAllow := []config.AllowEntry{
		{Domain: "golang.org"},
		{Domain: "api.anthropic.com"},
	}
	approvals := &config.Decisions{
		Domains:  []string{},
		Patterns: []string{},
	}

	merged := append(staticAllow, approvalsToAllowEntries(approvals)...)

	if len(merged) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(merged))
	}

	if merged[0].Domain != "golang.org" {
		t.Errorf("merged[0].Domain = %q, want %q", merged[0].Domain, "golang.org")
	}
	if merged[1].Domain != "api.anthropic.com" {
		t.Errorf("merged[1].Domain = %q, want %q", merged[1].Domain, "api.anthropic.com")
	}
}

func TestMergeStaticAndApprovals_OnlyApprovals(t *testing.T) {
	var staticAllow []config.AllowEntry
	approvals := &config.Decisions{
		Domains:  []string{"approved.example.com"},
		Patterns: []string{"*.cdn.example.com"},
	}

	merged := append(staticAllow, approvalsToAllowEntries(approvals)...)

	if len(merged) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(merged))
	}

	if merged[0].Domain != "approved.example.com" {
		t.Errorf("merged[0].Domain = %q, want %q", merged[0].Domain, "approved.example.com")
	}
	if merged[0].Pattern != "" {
		t.Errorf("merged[0].Pattern should be empty, got %q", merged[0].Pattern)
	}

	if merged[1].Pattern != "*.cdn.example.com" {
		t.Errorf("merged[1].Pattern = %q, want %q", merged[1].Pattern, "*.cdn.example.com")
	}
	if merged[1].Domain != "" {
		t.Errorf("merged[1].Domain should be empty, got %q", merged[1].Domain)
	}
}
