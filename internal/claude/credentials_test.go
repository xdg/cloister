package claude

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// MockCommandRunner implements CommandRunner for testing.
type MockCommandRunner struct {
	// Output is the stdout to return from Run.
	Output string
	// Err is the error to return from Run.
	Err error
	// Calls records all calls made to Run.
	Calls []MockCommandCall
}

// MockCommandCall records a single call to Run.
type MockCommandCall struct {
	Name string
	Args []string
}

func (m *MockCommandRunner) Run(name string, args ...string) (string, error) {
	m.Calls = append(m.Calls, MockCommandCall{Name: name, Args: args})
	return m.Output, m.Err
}

// MockFileChecker implements FileChecker for testing.
type MockFileChecker struct {
	// ExistingPaths is the set of paths that should return true from Exists.
	ExistingPaths map[string]bool
	// Calls records all paths checked.
	Calls []string
}

func (m *MockFileChecker) Exists(path string) bool {
	m.Calls = append(m.Calls, path)
	return m.ExistingPaths[path]
}

// MockUserLookup implements UserLookup for testing.
type MockUserLookup struct {
	Username    string
	UsernameErr error
	Home        string
	HomeErr     error
}

func (m *MockUserLookup) CurrentUsername() (string, error) {
	return m.Username, m.UsernameErr
}

func (m *MockUserLookup) HomeDir() (string, error) {
	return m.Home, m.HomeErr
}

func TestExtractor_Extract_MacOS_Success(t *testing.T) {
	validJSON := `{"accessToken":"test-token","refreshToken":"refresh","expiresAt":"2025-01-01"}`

	extractor := &Extractor{
		CommandRunner: &MockCommandRunner{
			Output: validJSON + "\n", // Keychain output often has trailing newline
		},
		FileChecker: &MockFileChecker{},
		UserLookup: &MockUserLookup{
			Username: "testuser",
		},
		Platform: "darwin",
	}

	creds, err := extractor.Extract()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if creds.JSON != validJSON {
		t.Errorf("expected JSON %q, got %q", validJSON, creds.JSON)
	}
	if creds.Platform != "darwin" {
		t.Errorf("expected platform darwin, got %q", creds.Platform)
	}
	if creds.FilePath != "" {
		t.Errorf("expected empty FilePath for macOS, got %q", creds.FilePath)
	}
}

func TestExtractor_Extract_MacOS_CommandArgs(t *testing.T) {
	cmdRunner := &MockCommandRunner{
		Output: `{"accessToken":"test"}`,
	}

	extractor := &Extractor{
		CommandRunner: cmdRunner,
		FileChecker:   &MockFileChecker{},
		UserLookup: &MockUserLookup{
			Username: "alice",
		},
		Platform: "darwin",
	}

	_, err := extractor.Extract()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cmdRunner.Calls) != 1 {
		t.Fatalf("expected 1 command call, got %d", len(cmdRunner.Calls))
	}

	call := cmdRunner.Calls[0]
	if call.Name != "security" {
		t.Errorf("expected command 'security', got %q", call.Name)
	}

	expectedArgs := []string{
		"find-generic-password",
		"-s", "Claude Code-credentials",
		"-a", "alice",
		"-w",
	}
	if len(call.Args) != len(expectedArgs) {
		t.Fatalf("expected %d args, got %d: %v", len(expectedArgs), len(call.Args), call.Args)
	}
	for i, arg := range expectedArgs {
		if call.Args[i] != arg {
			t.Errorf("arg[%d]: expected %q, got %q", i, arg, call.Args[i])
		}
	}
}

func TestExtractor_Extract_MacOS_NotFound(t *testing.T) {
	extractor := &Extractor{
		CommandRunner: &MockCommandRunner{
			Err: errors.New("security: SecKeychainSearchCopyNext: The specified item could not be found in the keychain"),
		},
		FileChecker: &MockFileChecker{},
		UserLookup: &MockUserLookup{
			Username: "testuser",
		},
		Platform: "darwin",
	}

	_, err := extractor.Extract()
	if !errors.Is(err, ErrCredentialsNotFound) {
		t.Errorf("expected ErrCredentialsNotFound, got %v", err)
	}
}

func TestExtractor_Extract_MacOS_InvalidJSON(t *testing.T) {
	extractor := &Extractor{
		CommandRunner: &MockCommandRunner{
			Output: "not valid json",
		},
		FileChecker: &MockFileChecker{},
		UserLookup: &MockUserLookup{
			Username: "testuser",
		},
		Platform: "darwin",
	}

	_, err := extractor.Extract()
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if errors.Is(err, ErrCredentialsNotFound) {
		t.Error("should not be ErrCredentialsNotFound for invalid JSON")
	}
}

func TestExtractor_Extract_MacOS_UserLookupError(t *testing.T) {
	extractor := &Extractor{
		CommandRunner: &MockCommandRunner{},
		FileChecker:   &MockFileChecker{},
		UserLookup: &MockUserLookup{
			UsernameErr: errors.New("user lookup failed"),
		},
		Platform: "darwin",
	}

	_, err := extractor.Extract()
	if err == nil {
		t.Fatal("expected error")
	}
	if !containsString(err.Error(), "failed to get current user") {
		t.Errorf("expected 'failed to get current user' in error, got %v", err)
	}
}

func TestExtractor_Extract_Linux_Success(t *testing.T) {
	// Create a temp directory with a credentials file
	tmpDir := t.TempDir()
	claudeDir := filepath.Join(tmpDir, ".claude")
	if err := os.Mkdir(claudeDir, 0755); err != nil {
		t.Fatalf("failed to create .claude dir: %v", err)
	}
	credPath := filepath.Join(claudeDir, ".credentials.json")
	if err := os.WriteFile(credPath, []byte(`{"test":"data"}`), 0600); err != nil {
		t.Fatalf("failed to create credentials file: %v", err)
	}

	extractor := &Extractor{
		CommandRunner: &MockCommandRunner{},
		FileChecker: &MockFileChecker{
			ExistingPaths: map[string]bool{
				credPath: true,
			},
		},
		UserLookup: &MockUserLookup{
			Home: tmpDir,
		},
		Platform: "linux",
	}

	creds, err := extractor.Extract()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if creds.FilePath != credPath {
		t.Errorf("expected FilePath %q, got %q", credPath, creds.FilePath)
	}
	if creds.Platform != "linux" {
		t.Errorf("expected platform linux, got %q", creds.Platform)
	}
	if creds.JSON != "" {
		t.Errorf("expected empty JSON for Linux, got %q", creds.JSON)
	}
}

func TestExtractor_Extract_Linux_NotFound(t *testing.T) {
	tmpDir := t.TempDir()

	extractor := &Extractor{
		CommandRunner: &MockCommandRunner{},
		FileChecker: &MockFileChecker{
			ExistingPaths: map[string]bool{}, // No files exist
		},
		UserLookup: &MockUserLookup{
			Home: tmpDir,
		},
		Platform: "linux",
	}

	_, err := extractor.Extract()
	if !errors.Is(err, ErrCredentialsNotFound) {
		t.Errorf("expected ErrCredentialsNotFound, got %v", err)
	}
}

func TestExtractor_Extract_Linux_CorrectPath(t *testing.T) {
	fileChecker := &MockFileChecker{
		ExistingPaths: map[string]bool{
			"/home/alice/.claude/.credentials.json": true,
		},
	}

	extractor := &Extractor{
		CommandRunner: &MockCommandRunner{},
		FileChecker:   fileChecker,
		UserLookup: &MockUserLookup{
			Home: "/home/alice",
		},
		Platform: "linux",
	}

	_, err := extractor.Extract()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(fileChecker.Calls) != 1 {
		t.Fatalf("expected 1 file check, got %d", len(fileChecker.Calls))
	}

	expectedPath := "/home/alice/.claude/.credentials.json"
	if fileChecker.Calls[0] != expectedPath {
		t.Errorf("expected path check for %q, got %q", expectedPath, fileChecker.Calls[0])
	}
}

func TestExtractor_Extract_Linux_HomeDirError(t *testing.T) {
	extractor := &Extractor{
		CommandRunner: &MockCommandRunner{},
		FileChecker:   &MockFileChecker{},
		UserLookup: &MockUserLookup{
			HomeErr: errors.New("home dir not found"),
		},
		Platform: "linux",
	}

	_, err := extractor.Extract()
	if err == nil {
		t.Fatal("expected error")
	}
	if !containsString(err.Error(), "failed to get home directory") {
		t.Errorf("expected 'failed to get home directory' in error, got %v", err)
	}
}

func TestExtractor_Extract_UnsupportedPlatform(t *testing.T) {
	extractor := &Extractor{
		CommandRunner: &MockCommandRunner{},
		FileChecker:   &MockFileChecker{},
		UserLookup:    &MockUserLookup{},
		Platform:      "windows",
	}

	_, err := extractor.Extract()
	if err == nil {
		t.Fatal("expected error for unsupported platform")
	}
	if !containsString(err.Error(), "unsupported platform") {
		t.Errorf("expected 'unsupported platform' in error, got %v", err)
	}
}

func TestExtractor_Extract_DefaultPlatform(t *testing.T) {
	// Without setting Platform, it should use runtime.GOOS
	extractor := &Extractor{
		CommandRunner: &MockCommandRunner{
			Output: `{"test":"data"}`,
		},
		FileChecker: &MockFileChecker{
			ExistingPaths: map[string]bool{
				"/home/test/.claude/.credentials.json": true,
			},
		},
		UserLookup: &MockUserLookup{
			Username: "testuser",
			Home:     "/home/test",
		},
		// Platform is empty - should use runtime.GOOS
	}

	// This should not panic and should use real platform
	_, _ = extractor.Extract()
	// We can't assert specific behavior since it depends on the actual platform
}

func TestNewExtractor(t *testing.T) {
	extractor := NewExtractor()

	if extractor == nil {
		t.Fatal("NewExtractor returned nil")
	}
	if extractor.CommandRunner == nil {
		t.Error("CommandRunner should not be nil")
	}
	if extractor.FileChecker == nil {
		t.Error("FileChecker should not be nil")
	}
	if extractor.UserLookup == nil {
		t.Error("UserLookup should not be nil")
	}
	if extractor.Platform != "" {
		t.Errorf("Platform should be empty for default, got %q", extractor.Platform)
	}
}

func TestCredentials_Fields(t *testing.T) {
	// Test macOS credentials
	macCreds := &Credentials{
		JSON:     `{"token":"abc"}`,
		Platform: "darwin",
	}
	if macCreds.JSON != `{"token":"abc"}` {
		t.Error("macOS creds should have JSON set")
	}
	if macCreds.FilePath != "" {
		t.Error("macOS creds should not have FilePath set")
	}

	// Test Linux credentials
	linuxCreds := &Credentials{
		FilePath: "/home/user/.claude/.credentials.json",
		Platform: "linux",
	}
	if linuxCreds.FilePath != "/home/user/.claude/.credentials.json" {
		t.Error("Linux creds should have FilePath set")
	}
	if linuxCreds.JSON != "" {
		t.Error("Linux creds should not have JSON set")
	}
}

func TestErrCredentialsNotFound_Message(t *testing.T) {
	err := ErrCredentialsNotFound
	expected := "Claude Code credentials not found: run `claude login` first"
	if err.Error() != expected {
		t.Errorf("expected error message %q, got %q", expected, err.Error())
	}
}

// containsString checks if s contains substr.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
