package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootCommand_Help(t *testing.T) {
	// Capture output
	var stdout bytes.Buffer

	// Create a fresh command instance for testing
	cmd := rootCmd
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--help"})

	// Execute and verify no error
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("root command --help returned error: %v", err)
	}

	// Verify output contains expected content
	output := stdout.String()

	expectedStrings := []string{
		"cloister",
		"Docker containers",
		"Usage:",
		"Available Commands:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("help output missing expected string %q\nGot: %s", expected, output)
		}
	}
}

func TestRootCommand_Version(t *testing.T) {
	var stdout bytes.Buffer

	cmd := rootCmd
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--version"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("root command --version returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "cloister") {
		t.Errorf("version output missing 'cloister'\nGot: %s", output)
	}
}
