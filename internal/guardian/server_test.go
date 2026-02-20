package guardian

import (
	"testing"

	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/token"
)

func TestNewServer_DefaultConfig(t *testing.T) {
	cfg := config.DefaultGlobalConfig()
	// Disable audit log so setupAuditLogger doesn't try to open a real file.
	cfg.Log.File = ""

	registry := token.NewRegistry()
	decisions := &config.Decisions{}

	srv, err := NewServer(registry, cfg, decisions)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}
	if srv == nil {
		t.Fatal("NewServer returned nil server")
	}
	if srv.registry != registry {
		t.Error("server registry not set correctly")
	}
	if srv.cfg != cfg {
		t.Error("server cfg not set correctly")
	}
	if srv.policyEngine == nil {
		t.Error("policyEngine not initialized")
	}
	if srv.patternCache == nil {
		t.Error("patternCache not initialized")
	}
	if srv.proxy == nil {
		t.Error("proxy not initialized")
	}
	if srv.api == nil {
		t.Error("api not initialized")
	}
	if srv.reqServer == nil {
		t.Error("reqServer not initialized")
	}
	if srv.approvalServer == nil {
		t.Error("approvalServer not initialized")
	}
}

func TestNewServer_PolicyEngineWiredCorrectly(t *testing.T) {
	cfg := config.DefaultGlobalConfig()
	cfg.Log.File = ""

	registry := token.NewRegistry()
	decisions := &config.Decisions{}

	srv, err := NewServer(registry, cfg, decisions)
	if err != nil {
		t.Fatalf("NewServer returned error: %v", err)
	}

	// Default config includes api.anthropic.com in the allow list.
	// The policy engine should allow it via the global policy.
	got := srv.policyEngine.Check("any-token", "any-project", "api.anthropic.com")
	if got != Allow {
		t.Errorf("policyEngine.Check for api.anthropic.com = %v, want Allow", got)
	}

	// A domain not in any list should return AskHuman (default behavior).
	got = srv.policyEngine.Check("any-token", "any-project", "not-in-any-list.example.com")
	if got != AskHuman {
		t.Errorf("policyEngine.Check for not-in-any-list.example.com = %v, want AskHuman", got)
	}
}

func TestExtractPatterns(t *testing.T) {
	tests := []struct {
		name  string
		input []config.CommandPattern
		want  []string
	}{
		{
			name:  "nil input",
			input: nil,
			want:  []string{},
		},
		{
			name:  "empty input",
			input: []config.CommandPattern{},
			want:  []string{},
		},
		{
			name: "single pattern",
			input: []config.CommandPattern{
				{Pattern: "^git push$"},
			},
			want: []string{"^git push$"},
		},
		{
			name: "multiple patterns",
			input: []config.CommandPattern{
				{Pattern: "^git push$"},
				{Pattern: "^make .*$"},
				{Pattern: "^docker build .*$"},
			},
			want: []string{"^git push$", "^make .*$", "^docker build .*$"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPatterns(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("extractPatterns() returned %d elements, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("extractPatterns()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
