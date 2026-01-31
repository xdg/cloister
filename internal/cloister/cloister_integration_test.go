//go:build integration

package cloister

import (
	"encoding/json"
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
)

// testProjectName generates a unique test project name.
func testProjectName() string {
	return "test-" + time.Now().Format("20060102-150405")
}

// cleanupTestContainer removes a test container if it exists.
func cleanupTestContainer(name string) {
	// Best effort cleanup - ignore errors
	_, _ = docker.Run("rm", "-f", name)
}

// requireDocker skips the test if Docker is not available.
func requireDocker(t *testing.T) {
	t.Helper()
	if err := docker.CheckDaemon(); err != nil {
		t.Skipf("Docker not available: %v", err)
	}
}

// requireGuardian skips the test if guardian cannot be started.
// Registers cleanup to stop the guardian when test completes.
func requireGuardian(t *testing.T) {
	t.Helper()
	requireDocker(t)
	if err := guardian.EnsureRunning(); err != nil {
		t.Skipf("Guardian not available: %v", err)
	}
	t.Cleanup(func() {
		_ = guardian.Stop()
	})
}

// TestCloisterLifecycle combines integration tests that require the guardian.
// Using subtests allows sharing a single guardian instance across all tests.
func TestCloisterLifecycle(t *testing.T) {
	requireGuardian(t)

	t.Run("Start_Stop", func(t *testing.T) {
		projectName := testProjectName()
		branchName := "main"
		containerName := container.GenerateContainerName(projectName)
		t.Cleanup(func() { cleanupTestContainer(containerName) })

		tmpDir, err := os.MkdirTemp("", "cloister-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(tmpDir) })

		opts := StartOptions{
			ProjectPath: tmpDir,
			ProjectName: projectName,
			BranchName:  branchName,
			Image:       container.DefaultImage,
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
		projectName := testProjectName()
		branchName := "env-test"
		containerName := container.GenerateContainerName(projectName)
		t.Cleanup(func() { cleanupTestContainer(containerName) })

		tmpDir, err := os.MkdirTemp("", "cloister-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(tmpDir) })

		opts := StartOptions{
			ProjectPath: tmpDir,
			ProjectName: projectName,
			BranchName:  branchName,
			Image:       container.DefaultImage,
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

		// Verify proxy env vars
		expectedProxy := "http://token:" + tok + "@cloister-guardian:3128"
		for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
			if val, ok := envMap[key]; !ok {
				t.Errorf("%s not set in container", key)
			} else if val != expectedProxy {
				t.Errorf("%s = %q, want %q", key, val, expectedProxy)
			}
		}
	})

	t.Run("ConnectsToCloisterNet", func(t *testing.T) {
		projectName := testProjectName()
		branchName := "network-test"
		containerName := container.GenerateContainerName(projectName)
		t.Cleanup(func() { cleanupTestContainer(containerName) })

		tmpDir, err := os.MkdirTemp("", "cloister-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(tmpDir) })

		opts := StartOptions{
			ProjectPath: tmpDir,
			ProjectName: projectName,
			BranchName:  branchName,
			Image:       container.DefaultImage,
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
		projectName := testProjectName()
		branchName := "empty-token"
		containerName := container.GenerateContainerName(projectName)
		t.Cleanup(func() { cleanupTestContainer(containerName) })

		tmpDir, err := os.MkdirTemp("", "cloister-test-*")
		if err != nil {
			t.Fatalf("Failed to create temp dir: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(tmpDir) })

		opts := StartOptions{
			ProjectPath: tmpDir,
			ProjectName: projectName,
			BranchName:  branchName,
			Image:       container.DefaultImage,
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
}

func TestStop_NonExistentContainer(t *testing.T) {
	requireDocker(t)

	// Stop a non-existent container - should return error
	err := Stop("cloister-nonexistent-12345", "sometoken")
	if err == nil {
		t.Error("Stop() with non-existent container should return error")
	}
}

func TestWriteFileToContainerWithOwner(t *testing.T) {
	requireDocker(t)

	// Create a test container
	containerName := "cloister-ownership-test-" + time.Now().Format("150405")
	t.Cleanup(func() { cleanupTestContainer(containerName) })

	// Create container using cloister-default image which has /home/cloister
	_, err := docker.Run("create",
		"--name", containerName,
		container.DefaultImage,
		"sleep", "infinity")
	if err != nil {
		t.Skipf("Could not create container with %s: %v", container.DefaultImage, err)
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
	requireDocker(t)

	// Create a test container
	containerName := "cloister-copy-ownership-test-" + time.Now().Format("150405")
	t.Cleanup(func() { cleanupTestContainer(containerName) })

	// Create a temp directory with test files
	tmpDir, err := os.MkdirTemp("", "cloister-copy-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

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
		container.DefaultImage,
		"sleep", "infinity")
	if err != nil {
		t.Skipf("Could not create container with %s: %v", container.DefaultImage, err)
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
	requireDocker(t)

	// Create a mock home directory with test .claude files
	mockHome, err := os.MkdirTemp("", "cloister-mock-home-*")
	if err != nil {
		t.Fatalf("Failed to create mock home: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(mockHome) })

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
	projectName := testProjectName()
	branchName := "settings-test"
	containerName := "cloister-" + projectName + "-" + branchName
	t.Cleanup(func() { cleanupTestContainer(containerName) })

	// Create a temporary directory for the project path
	tmpDir, err := os.MkdirTemp("", "cloister-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	// Create container using docker create directly (not through cloister.Start)
	// to avoid needing the guardian. Uses cloister-default which has /home/cloister.
	_, err = docker.Run("create",
		"--name", containerName,
		"-v", tmpDir+":/work",
		container.DefaultImage,
		"sleep", "infinity")
	if err != nil {
		t.Skipf("Could not create container with %s: %v", container.DefaultImage, err)
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
