package cmd

import "testing"

func TestShutdownCmd(t *testing.T) {
	if shutdownCmd.Use != "shutdown" {
		t.Errorf("expected Use = %q, got %q", "shutdown", shutdownCmd.Use)
	}
	if shutdownCmd.Short == "" {
		t.Error("expected non-empty Short description")
	}
	if shutdownCmd.RunE == nil {
		t.Error("expected RunE to be set")
	}
}

func TestTokenForContainer(t *testing.T) {
	tokens := map[string]string{
		"tok-abc": "cloister-foo",
		"tok-def": "cloister-bar",
	}

	if got := tokenForContainer(tokens, "cloister-foo"); got != "tok-abc" {
		t.Errorf("expected tok-abc, got %q", got)
	}
	if got := tokenForContainer(tokens, "cloister-baz"); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
	if got := tokenForContainer(nil, "cloister-foo"); got != "" {
		t.Errorf("expected empty for nil map, got %q", got)
	}
}
