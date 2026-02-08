package cmd

import (
	"testing"

	"github.com/xdg/cloister/internal/config"
)

func TestDecisionsToAllowEntries_DomainsOnly(t *testing.T) {
	decisions := &config.Decisions{
		Domains: []string{"example.com", "api.example.com"},
	}

	allow, deny := decisionsToAllowEntries(decisions)

	if len(allow) != 2 {
		t.Fatalf("expected 2 allow entries, got %d", len(allow))
	}

	if allow[0].Domain != "example.com" {
		t.Errorf("allow[0].Domain = %q, want %q", allow[0].Domain, "example.com")
	}
	if allow[0].Pattern != "" {
		t.Errorf("allow[0].Pattern should be empty, got %q", allow[0].Pattern)
	}

	if allow[1].Domain != "api.example.com" {
		t.Errorf("allow[1].Domain = %q, want %q", allow[1].Domain, "api.example.com")
	}
	if allow[1].Pattern != "" {
		t.Errorf("allow[1].Pattern should be empty, got %q", allow[1].Pattern)
	}

	if len(deny) != 0 {
		t.Fatalf("expected 0 deny entries, got %d", len(deny))
	}
}

func TestDecisionsToAllowEntries_PatternsOnly(t *testing.T) {
	decisions := &config.Decisions{
		Patterns: []string{"*.example.com", "*.cdn.example.com"},
	}

	allow, deny := decisionsToAllowEntries(decisions)

	if len(allow) != 2 {
		t.Fatalf("expected 2 allow entries, got %d", len(allow))
	}

	if allow[0].Pattern != "*.example.com" {
		t.Errorf("allow[0].Pattern = %q, want %q", allow[0].Pattern, "*.example.com")
	}
	if allow[0].Domain != "" {
		t.Errorf("allow[0].Domain should be empty, got %q", allow[0].Domain)
	}

	if allow[1].Pattern != "*.cdn.example.com" {
		t.Errorf("allow[1].Pattern = %q, want %q", allow[1].Pattern, "*.cdn.example.com")
	}
	if allow[1].Domain != "" {
		t.Errorf("allow[1].Domain should be empty, got %q", allow[1].Domain)
	}

	if len(deny) != 0 {
		t.Fatalf("expected 0 deny entries, got %d", len(deny))
	}
}

func TestDecisionsToAllowEntries_DomainsAndPatterns(t *testing.T) {
	decisions := &config.Decisions{
		Domains:  []string{"example.com"},
		Patterns: []string{"*.example.com"},
	}

	allow, deny := decisionsToAllowEntries(decisions)

	if len(allow) != 2 {
		t.Fatalf("expected 2 allow entries, got %d", len(allow))
	}

	// Domains come first
	if allow[0].Domain != "example.com" {
		t.Errorf("allow[0].Domain = %q, want %q", allow[0].Domain, "example.com")
	}
	if allow[0].Pattern != "" {
		t.Errorf("allow[0].Pattern should be empty, got %q", allow[0].Pattern)
	}

	// Patterns come after domains
	if allow[1].Pattern != "*.example.com" {
		t.Errorf("allow[1].Pattern = %q, want %q", allow[1].Pattern, "*.example.com")
	}
	if allow[1].Domain != "" {
		t.Errorf("allow[1].Domain should be empty, got %q", allow[1].Domain)
	}

	if len(deny) != 0 {
		t.Fatalf("expected 0 deny entries, got %d", len(deny))
	}
}

func TestDecisionsToAllowEntries_Empty(t *testing.T) {
	decisions := &config.Decisions{
		Domains:  []string{},
		Patterns: []string{},
	}

	allow, deny := decisionsToAllowEntries(decisions)

	if len(allow) != 0 {
		t.Fatalf("expected 0 allow entries, got %d", len(allow))
	}

	// Verify it returns a non-nil empty slice (not nil)
	if allow == nil {
		t.Error("expected non-nil empty allow slice, got nil")
	}

	if len(deny) != 0 {
		t.Fatalf("expected 0 deny entries, got %d", len(deny))
	}

	if deny == nil {
		t.Error("expected non-nil empty deny slice, got nil")
	}
}

func TestDecisionsToAllowEntries_NilSlices(t *testing.T) {
	decisions := &config.Decisions{
		Domains:  nil,
		Patterns: nil,
	}

	allow, deny := decisionsToAllowEntries(decisions)

	if len(allow) != 0 {
		t.Fatalf("expected 0 allow entries, got %d", len(allow))
	}

	// Verify it returns a non-nil empty slice (not nil)
	if allow == nil {
		t.Error("expected non-nil empty allow slice, got nil")
	}

	if len(deny) != 0 {
		t.Fatalf("expected 0 deny entries, got %d", len(deny))
	}

	if deny == nil {
		t.Error("expected non-nil empty deny slice, got nil")
	}
}

func TestDecisionsToAllowEntries_DeniedDomainsOnly(t *testing.T) {
	decisions := &config.Decisions{
		DeniedDomains: []string{"blocked.example.com", "malware.test"},
	}

	allow, deny := decisionsToAllowEntries(decisions)

	if len(allow) != 0 {
		t.Fatalf("expected 0 allow entries, got %d", len(allow))
	}

	if len(deny) != 2 {
		t.Fatalf("expected 2 deny entries, got %d", len(deny))
	}

	if deny[0].Domain != "blocked.example.com" {
		t.Errorf("deny[0].Domain = %q, want %q", deny[0].Domain, "blocked.example.com")
	}
	if deny[0].Pattern != "" {
		t.Errorf("deny[0].Pattern should be empty, got %q", deny[0].Pattern)
	}

	if deny[1].Domain != "malware.test" {
		t.Errorf("deny[1].Domain = %q, want %q", deny[1].Domain, "malware.test")
	}
	if deny[1].Pattern != "" {
		t.Errorf("deny[1].Pattern should be empty, got %q", deny[1].Pattern)
	}
}

func TestDecisionsToAllowEntries_DeniedPatternsOnly(t *testing.T) {
	decisions := &config.Decisions{
		DeniedPatterns: []string{"*.evil.com", "*.tracking.net"},
	}

	allow, deny := decisionsToAllowEntries(decisions)

	if len(allow) != 0 {
		t.Fatalf("expected 0 allow entries, got %d", len(allow))
	}

	if len(deny) != 2 {
		t.Fatalf("expected 2 deny entries, got %d", len(deny))
	}

	if deny[0].Pattern != "*.evil.com" {
		t.Errorf("deny[0].Pattern = %q, want %q", deny[0].Pattern, "*.evil.com")
	}
	if deny[0].Domain != "" {
		t.Errorf("deny[0].Domain should be empty, got %q", deny[0].Domain)
	}

	if deny[1].Pattern != "*.tracking.net" {
		t.Errorf("deny[1].Pattern = %q, want %q", deny[1].Pattern, "*.tracking.net")
	}
	if deny[1].Domain != "" {
		t.Errorf("deny[1].Domain should be empty, got %q", deny[1].Domain)
	}
}

func TestDecisionsToAllowEntries_AllFourFields(t *testing.T) {
	decisions := &config.Decisions{
		Domains:        []string{"example.com"},
		Patterns:       []string{"*.example.com"},
		DeniedDomains:  []string{"blocked.example.com"},
		DeniedPatterns: []string{"*.evil.com"},
	}

	allow, deny := decisionsToAllowEntries(decisions)

	if len(allow) != 2 {
		t.Fatalf("expected 2 allow entries, got %d", len(allow))
	}

	if allow[0].Domain != "example.com" {
		t.Errorf("allow[0].Domain = %q, want %q", allow[0].Domain, "example.com")
	}
	if allow[1].Pattern != "*.example.com" {
		t.Errorf("allow[1].Pattern = %q, want %q", allow[1].Pattern, "*.example.com")
	}

	if len(deny) != 2 {
		t.Fatalf("expected 2 deny entries, got %d", len(deny))
	}

	if deny[0].Domain != "blocked.example.com" {
		t.Errorf("deny[0].Domain = %q, want %q", deny[0].Domain, "blocked.example.com")
	}
	if deny[1].Pattern != "*.evil.com" {
		t.Errorf("deny[1].Pattern = %q, want %q", deny[1].Pattern, "*.evil.com")
	}
}

func TestMergeStaticAndDecisions_GlobalOnly(t *testing.T) {
	// Simulates the global allowlist merging pattern from runGuardianProxy:
	// allowEntries, _ := decisionsToAllowEntries(globalDecisions)
	// globalAllow = append(cfg.Proxy.Allow, allowEntries...)
	staticAllow := []config.AllowEntry{
		{Domain: "golang.org"},
		{Domain: "api.anthropic.com"},
	}
	decisions := &config.Decisions{
		Domains:  []string{"approved.example.com"},
		Patterns: []string{"*.cdn.example.com"},
	}

	allowEntries, _ := decisionsToAllowEntries(decisions)
	merged := append(staticAllow, allowEntries...)

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

func TestMergeStaticAndDecisions_ProjectWithAllFourSources(t *testing.T) {
	// Simulates the project allowlist merging pattern from loadProjectAllowlist:
	// merged = MergeAllowlists(global, project)
	// allowEntries, _ := decisionsToAllowEntries(globalDecisions)
	// merged = append(merged, allowEntries...)
	// allowEntries, _ = decisionsToAllowEntries(projectDecisions)
	// merged = append(merged, allowEntries...)
	globalStatic := []config.AllowEntry{
		{Domain: "golang.org"},
	}
	projectStatic := []config.AllowEntry{
		{Domain: "custom.project.com"},
	}
	globalDecisions := &config.Decisions{
		Domains: []string{"approved-global.com"},
	}
	projectDecisions := &config.Decisions{
		Domains:  []string{"approved-project.com"},
		Patterns: []string{"*.internal.corp"},
	}

	merged := config.MergeAllowlists(globalStatic, projectStatic)
	globalAllow, _ := decisionsToAllowEntries(globalDecisions)
	merged = append(merged, globalAllow...)
	projectAllow, _ := decisionsToAllowEntries(projectDecisions)
	merged = append(merged, projectAllow...)

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

func TestMergeStaticAndDecisions_EmptyDecisions(t *testing.T) {
	staticAllow := []config.AllowEntry{
		{Domain: "golang.org"},
		{Domain: "api.anthropic.com"},
	}
	decisions := &config.Decisions{
		Domains:  []string{},
		Patterns: []string{},
	}

	allowEntries, _ := decisionsToAllowEntries(decisions)
	merged := append(staticAllow, allowEntries...)

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

func TestMergeStaticAndDecisions_OnlyDecisions(t *testing.T) {
	var staticAllow []config.AllowEntry
	decisions := &config.Decisions{
		Domains:  []string{"approved.example.com"},
		Patterns: []string{"*.cdn.example.com"},
	}

	allowEntries, _ := decisionsToAllowEntries(decisions)
	merged := append(staticAllow, allowEntries...)

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
