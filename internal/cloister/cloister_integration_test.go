//go:build integration

package cloister

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/agent"
	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/docker"
	"github.com/xdg/cloister/internal/guardian"
	"github.com/xdg/cloister/internal/testutil"
)

// TestCloisterLifecycle combines integration tests that require the guardian.
// Using subtests allows sharing a single guardian instance across all tests.
func TestCloisterLifecycle(t *testing.T) {
	testutil.RequireGuardian(t)

	t.Run("Start_Stop", func(t *testing.T) {
		projectName := testutil.TestProjectName()
		branchName := "main"
		containerName := container.GenerateContainerName(projectName)
		t.Cleanup(func() { testutil.CleanupContainer(containerName) })

		tmpDir, err := os.MkdirTemp("", "cloister-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

		opts := StartOptions{
			ProjectPath: tmpDir,
			ProjectName: projectName,
			BranchName:  branchName,
			Image:       container.DefaultImage(),
		}

		containerID, tok, err := Start(opts)
		if err != nil {
			t.Fatalf("Start() failed: %v", err)
		}
		if containerID == "" {
			t.Error("Start() returned empty container ID")
		}
		if tok == "" {
			t.Error("Start() returned empty token")
		}
		if len(tok) != 64 {
			t.Errorf("Start() returned token of length %d, want 64", len(tok))
		}

		// Verify container is running
		mgr := container.NewManager()
		containers, err := mgr.List()
		if err != nil {
			t.Fatalf("List() failed: %v", err)
		}

		found := false
		for _, c := range containers {
			if c.Name == containerName {
				found = true
				if c.State != "running" {
					t.Errorf("Expected container state 'running', got %q", c.State)
				}
				break
			}
		}
		if !found {
			t.Errorf("Container %q not found in List()", containerName)
		}

		// Test Stop
		if err := Stop(containerName, tok); err != nil {
			t.Fatalf("Stop() failed: %v", err)
		}

		// Verify container is removed
		containers, err = mgr.List()
		if err != nil {
			t.Fatalf("List() after stop failed: %v", err)
		}
		for _, c := range containers {
			if c.Name == containerName {
				t.Errorf("Container %q still exists after Stop()", containerName)
			}
		}
	})

	t.Run("InjectsEnvVars", func(t *testing.T) {
		projectName := testutil.TestProjectName()
		branchName := "env-test"
		containerName := container.GenerateContainerName(projectName)
		t.Cleanup(func() { testutil.CleanupContainer(containerName) })

		tmpDir, err := os.MkdirTemp("", "cloister-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

		opts := StartOptions{
			ProjectPath: tmpDir,
			ProjectName: projectName,
			BranchName:  branchName,
			Image:       container.DefaultImage(),
		}

		containerID, tok, err := Start(opts)
		if err != nil {
			t.Fatalf("Start() failed: %v", err)
		}
		t.Cleanup(func() { _ = Stop(containerName, tok) })

		var inspectResult []struct {
			Config struct {
				Env []string `json:"Env"`
			} `json:"Config"`
		}

		output, err := docker.Run("inspect", containerID)
		if err != nil {
			t.Fatalf("docker inspect failed: %v", err)
		}

		if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &inspectResult); err != nil {
			t.Fatalf("Failed to parse inspect output: %v", err)
		}

		if len(inspectResult) == 0 {
			t.Fatal("No inspect results returned")
		}

		envMap := make(map[string]string)
		for _, e := range inspectResult[0].Config.Env {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 {
				envMap[parts[0]] = parts[1]
			}
		}

		// Verify CLOISTER_TOKEN
		if tokenVal, ok := envMap["CLOISTER_TOKEN"]; !ok {
			t.Error("CLOISTER_TOKEN not set in container")
		} else if tokenVal != tok {
			t.Errorf("CLOISTER_TOKEN = %q, want %q", tokenVal, tok)
		}

		// Verify proxy env vars (use dynamic guardian host for test isolation)
		expectedHost := guardian.Host()
		expectedProxy := "http://token:" + tok + "@" + expectedHost + ":3128"
		for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
			if val, ok := envMap[key]; !ok {
				t.Errorf("%s not set in container", key)
			} else if val != expectedProxy {
				t.Errorf("%s = %q, want %q", key, val, expectedProxy)
			}
		}

		// Verify CLOISTER_GUARDIAN_HOST
		if hostVal, ok := envMap["CLOISTER_GUARDIAN_HOST"]; !ok {
			t.Error("CLOISTER_GUARDIAN_HOST not set in container")
		} else if hostVal != expectedHost {
			t.Errorf("CLOISTER_GUARDIAN_HOST = %q, want %q", hostVal, expectedHost)
		}
	})

	t.Run("HostexecAvailable", func(t *testing.T) {
		projectName := testutil.TestProjectName()
		branchName := "hostexec-test"
		containerName := container.GenerateContainerName(projectName)
		t.Cleanup(func() { testutil.CleanupContainer(containerName) })

		tmpDir, err := os.MkdirTemp("", "cloister-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

		opts := StartOptions{
			ProjectPath: tmpDir,
			ProjectName: projectName,
			BranchName:  branchName,
			Image:       container.DefaultImage(),
		}

		_, tok, err := Start(opts)
		if err != nil {
			t.Fatalf("Start() failed: %v", err)
		}
		t.Cleanup(func() { _ = Stop(containerName, tok) })

		// Verify hostexec exists at /usr/local/bin/hostexec
		_, err = docker.Run("exec", containerName, "test", "-f", "/usr/local/bin/hostexec")
		if err != nil {
			t.Errorf("hostexec not found at /usr/local/bin/hostexec: %v", err)
		}

		// Verify hostexec is executable
		_, err = docker.Run("exec", containerName, "test", "-x", "/usr/local/bin/hostexec")
		if err != nil {
			t.Errorf("hostexec is not executable: %v", err)
		}

		// Verify hostexec outputs expected usage message when called with no args
		// Note: hostexec writes usage to stderr and exits with code 1 when no args
		// We need to redirect stderr to stdout to capture it
		output, _ := docker.Run("exec", "--user", "1000", containerName, "bash", "-c", "/usr/local/bin/hostexec 2>&1 || true")
		expectedUsage := "Usage: hostexec <command> [args...]"
		if !strings.Contains(output, expectedUsage) {
			t.Errorf("hostexec usage output = %q, want to contain %q", output, expectedUsage)
		}
	})

	t.Run("ConnectsToCloisterNet", func(t *testing.T) {
		projectName := testutil.TestProjectName()
		branchName := "network-test"
		containerName := container.GenerateContainerName(projectName)
		t.Cleanup(func() { testutil.CleanupContainer(containerName) })

		tmpDir, err := os.MkdirTemp("", "cloister-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

		opts := StartOptions{
			ProjectPath: tmpDir,
			ProjectName: projectName,
			BranchName:  branchName,
			Image:       container.DefaultImage(),
		}

		containerID, tok, err := Start(opts)
		if err != nil {
			t.Fatalf("Start() failed: %v", err)
		}
		t.Cleanup(func() { _ = Stop(containerName, tok) })

		var inspectResult []struct {
			NetworkSettings struct {
				Networks map[string]struct {
					NetworkID string `json:"NetworkID"`
				} `json:"Networks"`
			} `json:"NetworkSettings"`
		}

		output, err := docker.Run("inspect", containerID)
		if err != nil {
			t.Fatalf("docker inspect failed: %v", err)
		}

		if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &inspectResult); err != nil {
			t.Fatalf("Failed to parse inspect output: %v", err)
		}

		if len(inspectResult) == 0 {
			t.Fatal("No inspect results returned")
		}

		networks := inspectResult[0].NetworkSettings.Networks
		if _, ok := networks[docker.CloisterNetworkName]; !ok {
			t.Errorf("Container not connected to %s, connected networks: %v",
				docker.CloisterNetworkName, networks)
		}
	})

	t.Run("StopWithEmptyToken", func(t *testing.T) {
		projectName := testutil.TestProjectName()
		branchName := "empty-token"
		containerName := container.GenerateContainerName(projectName)
		t.Cleanup(func() { testutil.CleanupContainer(containerName) })

		tmpDir, err := os.MkdirTemp("", "cloister-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

		opts := StartOptions{
			ProjectPath: tmpDir,
			ProjectName: projectName,
			BranchName:  branchName,
			Image:       container.DefaultImage(),
		}

		_, _, err = Start(opts)
		if err != nil {
			t.Fatalf("Start() failed: %v", err)
		}

		// Stop with empty token - should still stop the container
		if err := Stop(containerName, ""); err != nil {
			t.Fatalf("Stop() with empty token failed: %v", err)
		}

		// Verify container is removed
		mgr := container.NewManager()
		containers, err := mgr.List()
		if err != nil {
			t.Fatalf("List() after stop failed: %v", err)
		}
		for _, c := range containers {
			if c.Name == containerName {
				t.Errorf("Container %q still exists after Stop()", containerName)
			}
		}
	})

	// Repro for bug: Second Start() on existing container deletes the token
	// When two terminals run 'cloister start' in the same directory:
	// 1. Terminal 1 creates container with token T1
	// 2. Terminal 2 generates token T2, overwrites T1 on disk, then deletes the file on ErrContainerExists
	// 3. Both terminals become non-functional because T1 is gone
	t.Run("SecondStart_PreservesToken", func(t *testing.T) {
		projectName := testutil.TestProjectName()
		branchName := "token-preservation"
		cloisterName := container.GenerateCloisterName(projectName)
		containerName := container.CloisterNameToContainerName(cloisterName)
		t.Cleanup(func() { testutil.CleanupContainer(containerName) })

		tmpDir, err := os.MkdirTemp("", "cloister-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

		opts := StartOptions{
			ProjectPath: tmpDir,
			ProjectName: projectName,
			BranchName:  branchName,
			Image:       container.DefaultImage(),
		}

		// First start - creates container and token T1
		_, token1, err := Start(opts)
		if err != nil {
			t.Fatalf("First Start() failed: %v", err)
		}
		if token1 == "" {
			t.Fatal("First Start() returned empty token")
		}

		// Verify token T1 is saved on disk
		store, err := getTokenStore()
		if err != nil {
			t.Fatalf("getTokenStore() failed: %v", err)
		}
		tokens, err := store.Load()
		if err != nil {
			t.Fatalf("store.Load() failed: %v", err)
		}
		tokenInfo1, exists := tokens[token1]
		if !exists {
			t.Fatalf("Token T1 not found on disk after first Start()")
		}
		if tokenInfo1.CloisterName != cloisterName {
			t.Errorf("Token info has wrong cloister name: got %q, want %q", tokenInfo1.CloisterName, cloisterName)
		}

		// Second start - should detect existing container and return ErrContainerExists
		// BUG: This currently deletes the token file
		_, token2, err := Start(opts)
		if err == nil {
			t.Error("Second Start() should return ErrContainerExists, but got nil error")
		}
		if !errors.Is(err, container.ErrContainerExists) {
			t.Errorf("Second Start() error = %v, want ErrContainerExists", err)
		}

		// ASSERTION: Token T1 should still be on disk
		// If the test fails here, it reproduces the bug
		tokens, err = store.Load()
		if err != nil {
			t.Fatalf("store.Load() after second Start() failed: %v", err)
		}
		if _, exists := tokens[token1]; !exists {
			t.Errorf("BUG REPRODUCED: Token T1 was deleted from disk after second Start()")
			t.Logf("Second Start() returned token: %q (should be empty since it failed)", token2)
			t.Logf("Tokens on disk after second Start(): %v", tokens)
		}

		// Clean up
		if err := Stop(containerName, token1); err != nil {
			t.Logf("Stop() failed during cleanup: %v", err)
		}
	})
}

func TestStop_NonExistentContainer(t *testing.T) {
	testutil.RequireDocker(t)
	testutil.IsolateXDGDirs(t)

	// Stop a non-existent container - should return error
	err := Stop("cloister-nonexistent-12345", "sometoken")
	if err == nil {
		t.Error("Stop() with non-existent container should return error")
	}
}

func TestWriteFileToContainerWithOwner(t *testing.T) {
	testutil.RequireDocker(t)

	// Create a test container
	containerName := "cloister-ownership-test-" + time.Now().Format("150405")
	t.Cleanup(func() { testutil.CleanupContainer(containerName) })

	// Create container using cloister-default image which has /home/cloister
	_, err := docker.Run("create",
		"--name", containerName,
		container.DefaultImage(),
		"sleep", "infinity")
	if err != nil {
		t.Skipf("Could not create container with %s: %v", container.DefaultImage(), err)
	}

	// Start the container first - tar piping requires a running container
	if _, err := docker.Run("start", containerName); err != nil {
		t.Fatalf("Failed to start container: %v", err)
	}

	// Write a file with ownership using tar piping (simulating credential injection)
	testContent := `{"test": "data"}`
	testPath := "/home/cloister/.claude/.credentials.json"
	if err := docker.WriteFileToContainerWithOwner(containerName, testPath, testContent, "1000", "1000"); err != nil {
		t.Fatalf("WriteFileToContainerWithOwner failed: %v", err)
	}

	// Check file ownership - should be cloister user (UID 1000)
	output, err := docker.Run("exec", containerName, "stat", "-c", "%u:%g", testPath)
	if err != nil {
		t.Fatalf("Failed to stat credentials file: %v", err)
	}
	ownership := strings.TrimSpace(output)
	if ownership != "1000:1000" {
		t.Errorf("Expected credentials file owned by 1000:1000, got %s", ownership)
	}

	// Verify the cloister user can actually read the file
	output, err = docker.Run("exec", "--user", "cloister", containerName, "cat", testPath)
	if err != nil {
		t.Errorf("Cloister user cannot read credentials file: %v", err)
	} else if strings.TrimSpace(output) != testContent {
		t.Errorf("File content mismatch: expected %q, got %q", testContent, output)
	}
}

func TestCopyToContainerWithOwner(t *testing.T) {
	testutil.RequireDocker(t)

	// Create a test container
	containerName := "cloister-copy-ownership-test-" + time.Now().Format("150405")
	t.Cleanup(func() { testutil.CleanupContainer(containerName) })

	// Create a temp directory with test files
	tmpDir, err := os.MkdirTemp("", "cloister-copy-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	// Create a .claude directory with some test files
	srcClaudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.MkdirAll(srcClaudeDir, 0o755); err != nil {
		t.Fatalf("Failed to create .claude dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcClaudeDir, "settings.json"), []byte(`{"test": true}`), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create container using cloister-default image
	_, err = docker.Run("create",
		"--name", containerName,
		container.DefaultImage(),
		"sleep", "infinity")
	if err != nil {
		t.Skipf("Could not create container with %s: %v", container.DefaultImage(), err)
	}

	// Start the container first - tar piping requires a running container
	if _, err := docker.Run("start", containerName); err != nil {
		t.Fatalf("Failed to start container: %v", err)
	}

	// Copy the directory with ownership using tar piping
	destDir := "/home/cloister"
	destPath := destDir + "/.claude"
	if err := docker.CopyToContainerWithOwner(srcClaudeDir, containerName, destDir, "1000", "1000"); err != nil {
		t.Fatalf("CopyToContainerWithOwner failed: %v", err)
	}

	// Check directory ownership - should be cloister user (UID 1000)
	output, err := docker.Run("exec", containerName, "stat", "-c", "%u:%g", destPath)
	if err != nil {
		t.Fatalf("Failed to stat .claude directory: %v", err)
	}
	ownership := strings.TrimSpace(output)
	if ownership != "1000:1000" {
		t.Errorf("Expected .claude directory owned by 1000:1000, got %s", ownership)
	}

	// Check file ownership inside the directory
	settingsPath := destPath + "/settings.json"
	output, err = docker.Run("exec", containerName, "stat", "-c", "%u:%g", settingsPath)
	if err != nil {
		t.Fatalf("Failed to stat settings.json: %v", err)
	}

	ownership = strings.TrimSpace(output)
	if ownership != "1000:1000" {
		t.Errorf("Expected settings.json owned by 1000:1000, got %s", ownership)
	}

	// Verify the cloister user can read the files
	_, err = docker.Run("exec", "--user", "cloister", containerName, "cat", settingsPath)
	if err != nil {
		t.Errorf("Cloister user cannot read settings.json: %v", err)
	}
}

func TestInjectUserSettings_IntegrationWithContainer(t *testing.T) {
	testutil.RequireDocker(t)

	// Create a mock home directory with test .claude files
	mockHome, err := os.MkdirTemp("", "cloister-mock-home-*")
	if err != nil {
		t.Fatalf("Failed to create mock home: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(mockHome) })

	// Create mock ~/.claude with test files
	mockClaudeDir := filepath.Join(mockHome, ".claude")
	if err := os.MkdirAll(mockClaudeDir, 0o755); err != nil {
		t.Fatalf("Failed to create mock .claude: %v", err)
	}

	testFiles := []string{"settings.json", "CLAUDE.md", ".credentials.json"}
	for _, f := range testFiles {
		content := fmt.Sprintf(`{"file": %q}`, f)
		if err := os.WriteFile(filepath.Join(mockClaudeDir, f), []byte(content), 0o644); err != nil {
			t.Fatalf("Failed to create %s: %v", f, err)
		}
	}

	// Override home directory for this test
	origHomeDir := agent.UserHomeDirFunc
	agent.UserHomeDirFunc = func() (string, error) { return mockHome, nil }
	t.Cleanup(func() { agent.UserHomeDirFunc = origHomeDir })

	// Create a test container
	projectName := testutil.TestProjectName()
	branchName := "settings-test"
	containerName := "cloister-" + projectName + "-" + branchName
	t.Cleanup(func() { testutil.CleanupContainer(containerName) })

	// Create a temporary directory for the project path
	tmpDir, err := os.MkdirTemp("", "cloister-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	// Create container using docker create directly (not through cloister.Start)
	// to avoid needing the guardian. Uses cloister-default which has /home/cloister.
	_, err = docker.Run("create",
		"--name", containerName,
		"-v", tmpDir+":/work",
		container.DefaultImage(),
		"sleep", "infinity")
	if err != nil {
		t.Skipf("Could not create container with %s: %v", container.DefaultImage(), err)
	}

	// Start the container first - injectUserSettings uses tar piping
	// which requires a running container
	_, err = docker.Run("start", containerName)
	if err != nil {
		t.Fatalf("Failed to start container: %v", err)
	}

	// Inject settings into container (uses tar piping for correct ownership)
	err = agent.CopyDirToContainer(containerName, ".claude", nil)
	if err != nil {
		t.Fatalf("CopyDirToContainer() failed: %v", err)
	}

	// Check each test file exists at correct path with correct ownership
	for _, testFile := range testFiles {
		expectedPath := "/home/cloister/.claude/" + testFile
		nestedPath := "/home/cloister/.claude/.claude/" + testFile

		output, err := docker.Run("exec", "--user", "root", containerName, "stat", "-c", "%u:%g", expectedPath)
		if err != nil {
			// Check if it's at the nested path (nesting bug)
			if _, nestedErr := docker.Run("exec", "--user", "root", containerName, "stat", nestedPath); nestedErr == nil {
				t.Errorf("BUG: %s is nested at %s instead of %s", testFile, nestedPath, expectedPath)
			} else {
				t.Errorf("Failed to stat %s: %v", expectedPath, err)
			}
			continue
		}

		// Verify ownership is cloister user (1000:1000), not host UID
		ownership := strings.TrimSpace(output)
		if ownership != "1000:1000" {
			t.Errorf("Expected %s owned by 1000:1000, got %s", expectedPath, ownership)
		}

		// Verify cloister user can read the file
		_, err = docker.Run("exec", "--user", "cloister", containerName, "cat", expectedPath)
		if err != nil {
			t.Errorf("Cloister user cannot read %s: %v", expectedPath, err)
		}
	}
}
