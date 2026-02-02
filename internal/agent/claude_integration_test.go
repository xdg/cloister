//go:build integration

package agent

import (
	"strings"
	"testing"

	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/testutil"
	"github.com/xdg/cloister/internal/version"
)

func TestClaudeAgent_SkipPerms_False_NoAlias(t *testing.T) {
	testutil.RequireDocker(t)

	containerName := testutil.TestContainerName("skipperms-false")
	t.Cleanup(func() { testutil.CleanupContainer(containerName) })

	// Create container using cloister-default image which has /home/cloister
	_, err := docker.Run("create",
		"--name", containerName,
		version.DefaultImage(),
		"sleep", "infinity")
	if err != nil {
		t.Skipf("Could not create container with %s: %v", version.DefaultImage(), err)
	}

	// Start the container
	if _, err := docker.Run("start", containerName); err != nil {
		t.Fatalf("Failed to start container: %v", err)
	}

	// Create agent with skip_permissions: false
	agent := NewClaudeAgent()
	skipPerms := false
	agentCfg := &config.AgentConfig{
		SkipPerms: &skipPerms,
	}

	// Run Setup (ignore SetupResult - we're testing alias behavior)
	_, err = agent.Setup(containerName, agentCfg)
	if err != nil {
		t.Fatalf("Setup() failed: %v", err)
	}

	// Run `bash -ic 'type claude'` to check if claude is an alias or binary
	// With skip_permissions=false, it should NOT show an alias
	output, err := docker.Run("exec", containerName, "bash", "-ic", "type claude")
	if err != nil {
		// If claude binary is not installed, we can't verify - but that's a different issue
		t.Logf("Note: 'type claude' failed, claude may not be installed: %v", err)
		// Instead, check that the alias is NOT in .bashrc
		bashrcOutput, bashrcErr := docker.Run("exec", containerName, "cat", "/home/cloister/.bashrc")
		if bashrcErr != nil {
			t.Fatalf("Failed to read .bashrc: %v", bashrcErr)
		}
		if strings.Contains(bashrcOutput, "alias claude=") {
			t.Errorf("Expected no claude alias in .bashrc when skip_permissions=false, but found alias")
		}
		return
	}

	// If 'type claude' succeeds, verify it shows the binary path, not an alias
	output = strings.TrimSpace(output)
	if strings.Contains(output, "alias") {
		t.Errorf("Expected claude to be a binary (not alias) when skip_permissions=false, got: %s", output)
	}

	// Double-check by looking at .bashrc directly
	bashrcOutput, err := docker.Run("exec", containerName, "cat", "/home/cloister/.bashrc")
	if err != nil {
		t.Fatalf("Failed to read .bashrc: %v", err)
	}
	if strings.Contains(bashrcOutput, "alias claude=") {
		t.Errorf("Expected no claude alias in .bashrc when skip_permissions=false, but found alias")
	}
}

func TestClaudeAgent_SkipPerms_Nil_HasAlias(t *testing.T) {
	testutil.RequireDocker(t)

	containerName := testutil.TestContainerName("skipperms-nil")
	t.Cleanup(func() { testutil.CleanupContainer(containerName) })

	// Create container using cloister-default image which has /home/cloister
	_, err := docker.Run("create",
		"--name", containerName,
		version.DefaultImage(),
		"sleep", "infinity")
	if err != nil {
		t.Skipf("Could not create container with %s: %v", version.DefaultImage(), err)
	}

	// Start the container
	if _, err := docker.Run("start", containerName); err != nil {
		t.Fatalf("Failed to start container: %v", err)
	}

	// Create agent with skip_permissions: nil (default)
	agent := NewClaudeAgent()
	agentCfg := &config.AgentConfig{
		SkipPerms: nil, // Default - should add alias
	}

	// Run Setup
	_, err = agent.Setup(containerName, agentCfg)
	if err != nil {
		t.Fatalf("Setup() failed: %v", err)
	}

	// Run `bash -ic 'type claude'` to check if claude is an alias
	// With skip_permissions=nil (default), it SHOULD show an alias
	output, err := docker.Run("exec", containerName, "bash", "-ic", "type claude")
	if err != nil {
		// If 'type claude' fails, check .bashrc directly
		t.Logf("Note: 'type claude' failed, checking .bashrc directly: %v", err)
		bashrcOutput, bashrcErr := docker.Run("exec", containerName, "cat", "/home/cloister/.bashrc")
		if bashrcErr != nil {
			t.Fatalf("Failed to read .bashrc: %v", bashrcErr)
		}
		if !strings.Contains(bashrcOutput, "alias claude=") {
			t.Errorf("Expected claude alias in .bashrc when skip_permissions=nil (default), but not found")
		}
		return
	}

	// Verify output indicates an alias
	output = strings.TrimSpace(output)
	if !strings.Contains(output, "alias") {
		t.Errorf("Expected claude to be an alias when skip_permissions=nil (default), got: %s", output)
	}

	// Verify .bashrc contains the alias
	bashrcOutput, err := docker.Run("exec", containerName, "cat", "/home/cloister/.bashrc")
	if err != nil {
		t.Fatalf("Failed to read .bashrc: %v", err)
	}
	expectedAlias := "alias claude='claude --dangerously-skip-permissions'"
	if !strings.Contains(bashrcOutput, expectedAlias) {
		t.Errorf("Expected .bashrc to contain %q, got: %s", expectedAlias, bashrcOutput)
	}
}

func TestClaudeAgent_RulesFile(t *testing.T) {
	testutil.RequireDocker(t)

	containerName := testutil.TestContainerName("rules")
	t.Cleanup(func() { testutil.CleanupContainer(containerName) })

	// Create container using cloister-default image which has /home/cloister
	_, err := docker.Run("create",
		"--name", containerName,
		version.DefaultImage(),
		"sleep", "infinity")
	if err != nil {
		t.Skipf("Could not create container with %s: %v", version.DefaultImage(), err)
	}

	// Start the container
	if _, err := docker.Run("start", containerName); err != nil {
		t.Fatalf("Failed to start container: %v", err)
	}

	// Create agent with default config
	agent := NewClaudeAgent()
	agentCfg := &config.AgentConfig{}

	// Run Setup
	_, err = agent.Setup(containerName, agentCfg)
	if err != nil {
		t.Fatalf("Setup() failed: %v", err)
	}

	// Verify the rules file exists at the expected path
	rulesPath := "/home/cloister/.claude/rules/cloister.md"
	output, err := docker.Run("exec", containerName, "cat", rulesPath)
	if err != nil {
		t.Fatalf("Rules file not found at %s: %v", rulesPath, err)
	}

	// Verify content contains expected key phrases
	expectedPhrases := []string{
		"Cloister Environment",
		"sandbox",
		"/work",
		"hostexec",
		"HTTP proxy",
		"git push",
	}

	for _, phrase := range expectedPhrases {
		if !strings.Contains(output, phrase) {
			t.Errorf("Rules file missing expected phrase %q", phrase)
		}
	}

	// Verify the content matches what we expect (not corrupted)
	if !strings.HasPrefix(output, "# Cloister Environment") {
		t.Errorf("Rules file has unexpected format, starts with: %q", output[:min(50, len(output))])
	}
}
