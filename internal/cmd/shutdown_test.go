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
