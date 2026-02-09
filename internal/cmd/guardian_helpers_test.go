package cmd

import (
	"testing"

	"github.com/xdg/cloister/internal/config"
)

func TestMergeStaticAndDecisions_GlobalOnly(t *testing.T) {
	// Simulates the global allowlist merging pattern from runGuardianProxy:
	// globalAllow = append(cfg.Proxy.Allow, globalDecisions.Proxy.Allow...)
	staticAllow := []config.AllowEntry{
		{Domain: "golang.org"},
		{Domain: "api.anthropic.com"},
	}
	decisions := &config.Decisions{
		Proxy: config.DecisionsProxy{
			Allow: []config.AllowEntry{
				{Domain: "approved.example.com"},
				{Pattern: "*.cdn.example.com"},
			},
		},
	}

	merged := append(staticAllow, decisions.Proxy.Allow...)

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
	// merged = append(merged, globalDecisions.Proxy.Allow...)
	// merged = append(merged, projectDecisions.Proxy.Allow...)
	globalStatic := []config.AllowEntry{
		{Domain: "golang.org"},
	}
	projectStatic := []config.AllowEntry{
		{Domain: "custom.project.com"},
	}
	globalDecisions := &config.Decisions{
		Proxy: config.DecisionsProxy{
			Allow: []config.AllowEntry{
				{Domain: "approved-global.com"},
			},
		},
	}
	projectDecisions := &config.Decisions{
		Proxy: config.DecisionsProxy{
			Allow: []config.AllowEntry{
				{Domain: "approved-project.com"},
				{Pattern: "*.internal.corp"},
			},
		},
	}

	merged := config.MergeAllowlists(globalStatic, projectStatic)
	merged = append(merged, globalDecisions.Proxy.Allow...)
	merged = append(merged, projectDecisions.Proxy.Allow...)

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
	decisions := &config.Decisions{}

	merged := append(staticAllow, decisions.Proxy.Allow...)

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
		Proxy: config.DecisionsProxy{
			Allow: []config.AllowEntry{
				{Domain: "approved.example.com"},
				{Pattern: "*.cdn.example.com"},
			},
		},
	}

	merged := append(staticAllow, decisions.Proxy.Allow...)

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

func TestMergeStaticAndDecisions_DenyEntries(t *testing.T) {
	decisions := &config.Decisions{
		Proxy: config.DecisionsProxy{
			Deny: []config.AllowEntry{
				{Domain: "blocked.example.com"},
				{Pattern: "*.evil.com"},
			},
		},
	}

	deny := decisions.Proxy.Deny

	if len(deny) != 2 {
		t.Fatalf("expected 2 deny entries, got %d", len(deny))
	}

	if deny[0].Domain != "blocked.example.com" {
		t.Errorf("deny[0].Domain = %q, want %q", deny[0].Domain, "blocked.example.com")
	}
	if deny[0].Pattern != "" {
		t.Errorf("deny[0].Pattern should be empty, got %q", deny[0].Pattern)
	}

	if deny[1].Pattern != "*.evil.com" {
		t.Errorf("deny[1].Pattern = %q, want %q", deny[1].Pattern, "*.evil.com")
	}
	if deny[1].Domain != "" {
		t.Errorf("deny[1].Domain should be empty, got %q", deny[1].Domain)
	}
}

func TestMergeStaticAndDecisions_AllowAndDeny(t *testing.T) {
	decisions := &config.Decisions{
		Proxy: config.DecisionsProxy{
			Allow: []config.AllowEntry{
				{Domain: "example.com"},
				{Pattern: "*.example.com"},
			},
			Deny: []config.AllowEntry{
				{Domain: "blocked.example.com"},
				{Pattern: "*.evil.com"},
			},
		},
	}

	allow := decisions.Proxy.Allow
	deny := decisions.Proxy.Deny

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
