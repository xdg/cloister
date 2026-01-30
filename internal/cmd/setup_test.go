package cmd

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/xdg/cloister/internal/claude"
	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/prompt"
)

func TestSetupCmd_ExistsInRoot(t *testing.T) {
	// Verify setup command is registered with root command
	found := false
	for _, cmd := range rootCmd.Commands() {
		if cmd.Name() == "setup" {
			found = true
			break
		}
	}

	if !found {
		t.Error("setup command not found in root command")
	}
}

func TestSetupCmd_HasClaudeSubcommand(t *testing.T) {
	// Verify setup command has claude subcommand
	subCmds := setupCmd.Commands()
	if len(subCmds) == 0 {
		t.Fatal("setup command should have subcommands")
	}

	found := false
	for _, cmd := range subCmds {
		if cmd.Name() == "claude" {
			found = true
			break
		}
	}

	if !found {
		t.Error("claude subcommand not found in setup command")
	}
}

func TestSetupClaudeCmd_Runs(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Use mock prompter to avoid stdin requirement
	mockPrompter := prompt.NewMockPrompter(0) // Select first option
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock extractor that succeeds
	oldExtractor := setupClaudeExtractor
	setupClaudeExtractor = &claude.Extractor{
		CommandRunner: &mockCommandRunner{output: `{"test":"data"}`},
		FileChecker:   &mockFileChecker{exists: true},
		UserLookup:    &mockUserLookup{username: "testuser", home: "/home/test"},
		Platform:      "darwin",
	}
	defer func() { setupClaudeExtractor = oldExtractor }()

	var stdout bytes.Buffer

	// Reset the command for testing
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err != nil {
		t.Fatalf("setup claude returned error: %v", err)
	}

	// Verify output
	output := stdout.String()
	if output == "" {
		t.Error("setup claude should produce output")
	}
}

func TestSetupClaudeCmd_DefaultSelectsExistingLogin(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Mock prompter that returns default (simulates user pressing Enter)
	mockPrompter := &prompt.MockPrompter{} // No responses = return default
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock extractor that succeeds
	oldExtractor := setupClaudeExtractor
	setupClaudeExtractor = &claude.Extractor{
		CommandRunner: &mockCommandRunner{output: `{"test":"data"}`},
		FileChecker:   &mockFileChecker{exists: true},
		UserLookup:    &mockUserLookup{username: "testuser", home: "/home/test"},
		Platform:      "darwin",
	}
	defer func() { setupClaudeExtractor = oldExtractor }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err != nil {
		t.Fatalf("setup claude returned error: %v", err)
	}

	// Verify the prompter was called with correct parameters
	if len(mockPrompter.Calls) != 1 {
		t.Fatalf("expected 1 prompt call, got %d", len(mockPrompter.Calls))
	}

	call := mockPrompter.Calls[0]
	if call.DefaultIdx != 0 {
		t.Errorf("default index should be 0 (existing login), got %d", call.DefaultIdx)
	}

	// Verify "existing" auth method was selected
	output := stdout.String()
	if !strings.Contains(output, "Auth method: existing") {
		t.Errorf("default should select existing login method, got: %s", output)
	}
}

func TestSetupClaudeCmd_SelectsOAuthToken(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Mock prompter that returns option 2 (index 1)
	mockPrompter := prompt.NewMockPrompter(1)
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock credential reader that returns a token
	mockReader := prompt.NewMockCredentialReader("test-oauth-token")
	oldReader := setupClaudeCredentialReader
	setupClaudeCredentialReader = mockReader
	defer func() { setupClaudeCredentialReader = oldReader }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err != nil {
		t.Fatalf("setup claude returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Auth method: token") {
		t.Errorf("should select token method, got: %s", output)
	}
	if !strings.Contains(output, "OAuth token received") {
		t.Errorf("should confirm token received, got: %s", output)
	}
}

func TestSetupClaudeCmd_SelectsAPIKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Mock prompter that returns option 3 (index 2)
	mockPrompter := prompt.NewMockPrompter(2)
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock credential reader that returns an API key
	mockReader := prompt.NewMockCredentialReader("sk-ant-test-api-key")
	oldReader := setupClaudeCredentialReader
	setupClaudeCredentialReader = mockReader
	defer func() { setupClaudeCredentialReader = oldReader }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err != nil {
		t.Fatalf("setup claude returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Auth method: api_key") {
		t.Errorf("should select api_key method, got: %s", output)
	}
	if !strings.Contains(output, "API key received") {
		t.Errorf("should confirm API key received, got: %s", output)
	}
}

func TestSetupClaudeCmd_PrompterError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Mock prompter that returns an error
	mockPrompter := &prompt.MockPrompter{
		Errors: []error{errTestPrompter},
	}
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err == nil {
		t.Fatal("expected error from prompter")
	}

	if !strings.Contains(err.Error(), "failed to get authentication method") {
		t.Errorf("error should wrap prompter error, got: %v", err)
	}
}

var errTestPrompter = &testError{msg: "test prompter error"}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestAuthMethodOptions_ContainsExpectedChoices(t *testing.T) {
	if len(authMethodOptions) != 3 {
		t.Errorf("expected 3 auth method options, got %d", len(authMethodOptions))
	}

	// Verify first option mentions "existing" and "recommended"
	if !strings.Contains(authMethodOptions[0], "existing") {
		t.Error("first option should mention 'existing'")
	}
	if !strings.Contains(authMethodOptions[0], "recommended") {
		t.Error("first option should be marked as recommended")
	}

	// Verify second option mentions OAuth/token
	if !strings.Contains(strings.ToLower(authMethodOptions[1]), "token") {
		t.Error("second option should mention 'token'")
	}

	// Verify third option mentions API key
	if !strings.Contains(strings.ToLower(authMethodOptions[2]), "api key") {
		t.Error("third option should mention 'API key'")
	}
}

func TestAuthMethod_String(t *testing.T) {
	tests := []struct {
		method AuthMethod
		want   string
	}{
		{AuthMethodExisting, "existing"},
		{AuthMethodToken, "token"},
		{AuthMethodAPIKey, "api_key"},
		{AuthMethod(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.method.String(); got != tt.want {
				t.Errorf("AuthMethod(%d).String() = %q, want %q", tt.method, got, tt.want)
			}
		})
	}
}

func TestSetupClaudeCmd_ExistingLogin_MacOS_Success(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	mockPrompter := prompt.NewMockPrompter(0) // Select "existing login"
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock extractor that returns macOS credentials
	oldExtractor := setupClaudeExtractor
	setupClaudeExtractor = &claude.Extractor{
		CommandRunner: &mockCommandRunner{output: `{"accessToken":"test-token"}`},
		FileChecker:   &mockFileChecker{},
		UserLookup:    &mockUserLookup{username: "alice", home: "/Users/alice"},
		Platform:      "darwin",
	}
	defer func() { setupClaudeExtractor = oldExtractor }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "macOS Keychain") {
		t.Errorf("should mention macOS Keychain on success, got: %s", output)
	}
}

func TestSetupClaudeCmd_ExistingLogin_Linux_Success(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	mockPrompter := prompt.NewMockPrompter(0) // Select "existing login"
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock extractor that returns Linux credentials
	oldExtractor := setupClaudeExtractor
	setupClaudeExtractor = &claude.Extractor{
		CommandRunner: &mockCommandRunner{},
		FileChecker:   &mockFileChecker{exists: true},
		UserLookup:    &mockUserLookup{username: "alice", home: "/home/alice"},
		Platform:      "linux",
	}
	defer func() { setupClaudeExtractor = oldExtractor }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "credentials file") {
		t.Errorf("should mention credentials file on Linux, got: %s", output)
	}
	if !strings.Contains(output, "/home/alice/.claude/.credentials.json") {
		t.Errorf("should show credentials path, got: %s", output)
	}
}

func TestSetupClaudeCmd_ExistingLogin_NotFound(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	mockPrompter := prompt.NewMockPrompter(0) // Select "existing login"
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock extractor that returns credentials not found
	oldExtractor := setupClaudeExtractor
	setupClaudeExtractor = &claude.Extractor{
		CommandRunner: &mockCommandRunner{err: errors.New("keychain error")},
		FileChecker:   &mockFileChecker{exists: false},
		UserLookup:    &mockUserLookup{username: "alice", home: "/home/alice"},
		Platform:      "darwin",
	}
	defer func() { setupClaudeExtractor = oldExtractor }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err == nil {
		t.Fatal("expected error when credentials not found")
	}
	if !errors.Is(err, claude.ErrCredentialsNotFound) {
		t.Errorf("expected ErrCredentialsNotFound, got: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Credentials not found") {
		t.Errorf("should display helpful message, got: %s", output)
	}
	if !strings.Contains(output, "claude login") {
		t.Errorf("should suggest running 'claude login', got: %s", output)
	}
}

// Mock implementations for claude.Extractor dependencies

type mockCommandRunner struct {
	output string
	err    error
}

func (m *mockCommandRunner) Run(name string, args ...string) (string, error) {
	return m.output, m.err
}

type mockFileChecker struct {
	exists bool
}

func (m *mockFileChecker) Exists(path string) bool {
	return m.exists
}

type mockUserLookup struct {
	username    string
	usernameErr error
	home        string
	homeErr     error
}

func (m *mockUserLookup) CurrentUsername() (string, error) {
	return m.username, m.usernameErr
}

func (m *mockUserLookup) HomeDir() (string, error) {
	return m.home, m.homeErr
}

func TestSetupClaudeCmd_OAuthToken_CorrectPrompt(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	mockPrompter := prompt.NewMockPrompter(1) // Select "OAuth token"
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	mockReader := prompt.NewMockCredentialReader("my-token")
	oldReader := setupClaudeCredentialReader
	setupClaudeCredentialReader = mockReader
	defer func() { setupClaudeCredentialReader = oldReader }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the correct prompt was shown
	if len(mockReader.Calls) != 1 {
		t.Fatalf("expected 1 credential read call, got %d", len(mockReader.Calls))
	}
	expectedPrompt := "Paste your OAuth token (from `claude setup-token`): "
	if mockReader.Calls[0] != expectedPrompt {
		t.Errorf("prompt = %q, want %q", mockReader.Calls[0], expectedPrompt)
	}
}

func TestSetupClaudeCmd_APIKey_CorrectPrompt(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	mockPrompter := prompt.NewMockPrompter(2) // Select "API key"
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	mockReader := prompt.NewMockCredentialReader("sk-ant-key")
	oldReader := setupClaudeCredentialReader
	setupClaudeCredentialReader = mockReader
	defer func() { setupClaudeCredentialReader = oldReader }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the correct prompt was shown
	if len(mockReader.Calls) != 1 {
		t.Fatalf("expected 1 credential read call, got %d", len(mockReader.Calls))
	}
	expectedPrompt := "Paste your API key (from console.anthropic.com): "
	if mockReader.Calls[0] != expectedPrompt {
		t.Errorf("prompt = %q, want %q", mockReader.Calls[0], expectedPrompt)
	}
}

func TestSetupClaudeCmd_OAuthToken_EmptyInput_Error(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	mockPrompter := prompt.NewMockPrompter(1) // Select "OAuth token"
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Empty credential
	mockReader := prompt.NewMockCredentialReader("")
	oldReader := setupClaudeCredentialReader
	setupClaudeCredentialReader = mockReader
	defer func() { setupClaudeCredentialReader = oldReader }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err == nil {
		t.Fatal("expected error for empty token")
	}
	if !strings.Contains(err.Error(), "OAuth token cannot be empty") {
		t.Errorf("error should mention empty token, got: %v", err)
	}
}

func TestSetupClaudeCmd_APIKey_EmptyInput_Error(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	mockPrompter := prompt.NewMockPrompter(2) // Select "API key"
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Empty credential
	mockReader := prompt.NewMockCredentialReader("")
	oldReader := setupClaudeCredentialReader
	setupClaudeCredentialReader = mockReader
	defer func() { setupClaudeCredentialReader = oldReader }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err == nil {
		t.Fatal("expected error for empty API key")
	}
	if !strings.Contains(err.Error(), "API key cannot be empty") {
		t.Errorf("error should mention empty API key, got: %v", err)
	}
}

func TestSetupClaudeCmd_OAuthToken_WhitespaceOnly_Error(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	mockPrompter := prompt.NewMockPrompter(1) // Select "OAuth token"
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Whitespace-only credential
	mockReader := prompt.NewMockCredentialReader("   \t\n   ")
	oldReader := setupClaudeCredentialReader
	setupClaudeCredentialReader = mockReader
	defer func() { setupClaudeCredentialReader = oldReader }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err == nil {
		t.Fatal("expected error for whitespace-only token")
	}
	if !strings.Contains(err.Error(), "OAuth token cannot be empty") {
		t.Errorf("error should mention empty token, got: %v", err)
	}
}

func TestSetupClaudeCmd_CredentialReader_Error(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	mockPrompter := prompt.NewMockPrompter(1) // Select "OAuth token"
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock reader that returns an error
	testErr := errors.New("terminal read error")
	mockReader := &prompt.MockCredentialReader{
		Errors: []error{testErr},
	}
	oldReader := setupClaudeCredentialReader
	setupClaudeCredentialReader = mockReader
	defer func() { setupClaudeCredentialReader = oldReader }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err == nil {
		t.Fatal("expected error from credential reader")
	}
	if !strings.Contains(err.Error(), "failed to read OAuth token") {
		t.Errorf("error should wrap credential reader error, got: %v", err)
	}
}

// Skip-permissions prompt tests (Phase 3.2.5)

func TestSetupClaudeCmd_SkipPermissions_DefaultYes(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Mock prompter that returns existing login (index 0)
	mockPrompter := prompt.NewMockPrompter(0)
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock extractor that succeeds
	oldExtractor := setupClaudeExtractor
	setupClaudeExtractor = &claude.Extractor{
		CommandRunner: &mockCommandRunner{output: `{"test":"data"}`},
		FileChecker:   &mockFileChecker{exists: true},
		UserLookup:    &mockUserLookup{username: "testuser", home: "/home/test"},
		Platform:      "darwin",
	}
	defer func() { setupClaudeExtractor = oldExtractor }()

	// Mock yes/no prompter that returns default (empty input -> true)
	mockYesNo := &prompt.MockYesNoPrompter{} // No responses = returns default
	oldYesNo := setupClaudeYesNoPrompter
	setupClaudeYesNoPrompter = mockYesNo
	defer func() { setupClaudeYesNoPrompter = oldYesNo }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the yes/no prompter was called
	if len(mockYesNo.Calls) != 1 {
		t.Fatalf("expected 1 yes/no prompt call, got %d", len(mockYesNo.Calls))
	}

	// Verify the correct prompt was used
	call := mockYesNo.Calls[0]
	expectedPrompt := "Skip Claude's built-in permission prompts? (recommended inside cloister) [Y/n]: "
	if call.Prompt != expectedPrompt {
		t.Errorf("prompt = %q, want %q", call.Prompt, expectedPrompt)
	}

	// Verify default is yes
	if call.DefaultYes != true {
		t.Error("defaultYes should be true")
	}

	// Verify output shows skip_permissions: true (default)
	output := stdout.String()
	if !strings.Contains(output, "Skip permissions: true") {
		t.Errorf("output should show skip permissions true (default), got: %s", output)
	}
}

func TestSetupClaudeCmd_SkipPermissions_ExplicitNo(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Mock prompter that returns existing login (index 0)
	mockPrompter := prompt.NewMockPrompter(0)
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock extractor that succeeds
	oldExtractor := setupClaudeExtractor
	setupClaudeExtractor = &claude.Extractor{
		CommandRunner: &mockCommandRunner{output: `{"test":"data"}`},
		FileChecker:   &mockFileChecker{exists: true},
		UserLookup:    &mockUserLookup{username: "testuser", home: "/home/test"},
		Platform:      "darwin",
	}
	defer func() { setupClaudeExtractor = oldExtractor }()

	// Mock yes/no prompter that returns false (user typed "n")
	mockYesNo := prompt.NewMockYesNoPrompter(false)
	oldYesNo := setupClaudeYesNoPrompter
	setupClaudeYesNoPrompter = mockYesNo
	defer func() { setupClaudeYesNoPrompter = oldYesNo }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify output shows skip_permissions: false
	output := stdout.String()
	if !strings.Contains(output, "Skip permissions: false") {
		t.Errorf("output should show skip permissions false, got: %s", output)
	}
}

func TestSetupClaudeCmd_SkipPermissions_ExplicitYes(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Mock prompter that returns existing login (index 0)
	mockPrompter := prompt.NewMockPrompter(0)
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock extractor that succeeds
	oldExtractor := setupClaudeExtractor
	setupClaudeExtractor = &claude.Extractor{
		CommandRunner: &mockCommandRunner{output: `{"test":"data"}`},
		FileChecker:   &mockFileChecker{exists: true},
		UserLookup:    &mockUserLookup{username: "testuser", home: "/home/test"},
		Platform:      "darwin",
	}
	defer func() { setupClaudeExtractor = oldExtractor }()

	// Mock yes/no prompter that returns true (user typed "y")
	mockYesNo := prompt.NewMockYesNoPrompter(true)
	oldYesNo := setupClaudeYesNoPrompter
	setupClaudeYesNoPrompter = mockYesNo
	defer func() { setupClaudeYesNoPrompter = oldYesNo }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify output shows skip_permissions: true
	output := stdout.String()
	if !strings.Contains(output, "Skip permissions: true") {
		t.Errorf("output should show skip permissions true, got: %s", output)
	}
}

func TestSetupClaudeCmd_SkipPermissions_Error(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Mock prompter that returns existing login (index 0)
	mockPrompter := prompt.NewMockPrompter(0)
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock extractor that succeeds
	oldExtractor := setupClaudeExtractor
	setupClaudeExtractor = &claude.Extractor{
		CommandRunner: &mockCommandRunner{output: `{"test":"data"}`},
		FileChecker:   &mockFileChecker{exists: true},
		UserLookup:    &mockUserLookup{username: "testuser", home: "/home/test"},
		Platform:      "darwin",
	}
	defer func() { setupClaudeExtractor = oldExtractor }()

	// Mock yes/no prompter that returns an error
	testErr := errors.New("input error")
	mockYesNo := &prompt.MockYesNoPrompter{
		Errors: []error{testErr},
	}
	oldYesNo := setupClaudeYesNoPrompter
	setupClaudeYesNoPrompter = mockYesNo
	defer func() { setupClaudeYesNoPrompter = oldYesNo }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err == nil {
		t.Fatal("expected error from yes/no prompter")
	}
	if !strings.Contains(err.Error(), "failed to get skip-permissions setting") {
		t.Errorf("error should wrap yes/no prompter error, got: %v", err)
	}
}

func TestSetupClaudeCmd_SkipPermissions_WithTokenAuth(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Mock prompter that returns OAuth token (index 1)
	mockPrompter := prompt.NewMockPrompter(1)
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock credential reader that returns a token
	mockReader := prompt.NewMockCredentialReader("test-token")
	oldReader := setupClaudeCredentialReader
	setupClaudeCredentialReader = mockReader
	defer func() { setupClaudeCredentialReader = oldReader }()

	// Mock yes/no prompter that returns false (user typed "n")
	mockYesNo := prompt.NewMockYesNoPrompter(false)
	oldYesNo := setupClaudeYesNoPrompter
	setupClaudeYesNoPrompter = mockYesNo
	defer func() { setupClaudeYesNoPrompter = oldYesNo }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify both token auth and skip-permissions were prompted
	output := stdout.String()
	if !strings.Contains(output, "OAuth token received") {
		t.Error("output should show token received")
	}
	if !strings.Contains(output, "Skip permissions: false") {
		t.Errorf("output should show skip permissions false, got: %s", output)
	}
}

func TestSetupClaudeCmd_SkipPermissions_WithAPIKeyAuth(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Mock prompter that returns API key (index 2)
	mockPrompter := prompt.NewMockPrompter(2)
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock credential reader that returns an API key
	mockReader := prompt.NewMockCredentialReader("sk-ant-test-key")
	oldReader := setupClaudeCredentialReader
	setupClaudeCredentialReader = mockReader
	defer func() { setupClaudeCredentialReader = oldReader }()

	// Mock yes/no prompter that returns true (user typed "y")
	mockYesNo := prompt.NewMockYesNoPrompter(true)
	oldYesNo := setupClaudeYesNoPrompter
	setupClaudeYesNoPrompter = mockYesNo
	defer func() { setupClaudeYesNoPrompter = oldYesNo }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify both API key auth and skip-permissions were prompted
	output := stdout.String()
	if !strings.Contains(output, "API key received") {
		t.Error("output should show API key received")
	}
	if !strings.Contains(output, "Skip permissions: true") {
		t.Errorf("output should show skip permissions true, got: %s", output)
	}
}

// Phase 3.2.6 tests - Save credentials to config

func TestSetupClaudeCmd_SavesExistingAuthToConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Mock prompter that returns existing login (index 0)
	mockPrompter := prompt.NewMockPrompter(0)
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock extractor that succeeds
	oldExtractor := setupClaudeExtractor
	setupClaudeExtractor = &claude.Extractor{
		CommandRunner: &mockCommandRunner{output: `{"accessToken":"test-token"}`},
		FileChecker:   &mockFileChecker{exists: true},
		UserLookup:    &mockUserLookup{username: "testuser", home: "/home/test"},
		Platform:      "darwin",
	}
	defer func() { setupClaudeExtractor = oldExtractor }()

	// Mock yes/no prompter that returns true (default)
	mockYesNo := prompt.NewMockYesNoPrompter(true)
	oldYesNo := setupClaudeYesNoPrompter
	setupClaudeYesNoPrompter = mockYesNo
	defer func() { setupClaudeYesNoPrompter = oldYesNo }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify config was saved
	output := stdout.String()
	if !strings.Contains(output, "Configuration saved to:") {
		t.Errorf("output should show config saved message, got: %s", output)
	}

	// Read and verify the config file
	configPath := config.GlobalConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "auth_method: existing") {
		t.Errorf("config should contain auth_method: existing, got:\n%s", content)
	}
	if !strings.Contains(content, "skip_permissions: true") {
		t.Errorf("config should contain skip_permissions: true, got:\n%s", content)
	}
}

func TestSetupClaudeCmd_SavesTokenAuthToConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Mock prompter that returns OAuth token (index 1)
	mockPrompter := prompt.NewMockPrompter(1)
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock credential reader that returns a token
	mockReader := prompt.NewMockCredentialReader("my-test-oauth-token")
	oldReader := setupClaudeCredentialReader
	setupClaudeCredentialReader = mockReader
	defer func() { setupClaudeCredentialReader = oldReader }()

	// Mock yes/no prompter that returns false
	mockYesNo := prompt.NewMockYesNoPrompter(false)
	oldYesNo := setupClaudeYesNoPrompter
	setupClaudeYesNoPrompter = mockYesNo
	defer func() { setupClaudeYesNoPrompter = oldYesNo }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify config was saved
	output := stdout.String()
	if !strings.Contains(output, "Configuration saved to:") {
		t.Errorf("output should show config saved message, got: %s", output)
	}

	// Read and verify the config file
	configPath := config.GlobalConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "auth_method: token") {
		t.Errorf("config should contain auth_method: token, got:\n%s", content)
	}
	if !strings.Contains(content, "token: my-test-oauth-token") {
		t.Errorf("config should contain the token, got:\n%s", content)
	}
	if !strings.Contains(content, "skip_permissions: false") {
		t.Errorf("config should contain skip_permissions: false, got:\n%s", content)
	}
}

func TestSetupClaudeCmd_SavesAPIKeyAuthToConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Mock prompter that returns API key (index 2)
	mockPrompter := prompt.NewMockPrompter(2)
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock credential reader that returns an API key
	mockReader := prompt.NewMockCredentialReader("sk-ant-my-test-api-key")
	oldReader := setupClaudeCredentialReader
	setupClaudeCredentialReader = mockReader
	defer func() { setupClaudeCredentialReader = oldReader }()

	// Mock yes/no prompter that returns true
	mockYesNo := prompt.NewMockYesNoPrompter(true)
	oldYesNo := setupClaudeYesNoPrompter
	setupClaudeYesNoPrompter = mockYesNo
	defer func() { setupClaudeYesNoPrompter = oldYesNo }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify config was saved
	output := stdout.String()
	if !strings.Contains(output, "Configuration saved to:") {
		t.Errorf("output should show config saved message, got: %s", output)
	}

	// Read and verify the config file
	configPath := config.GlobalConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "auth_method: api_key") {
		t.Errorf("config should contain auth_method: api_key, got:\n%s", content)
	}
	if !strings.Contains(content, "api_key: sk-ant-my-test-api-key") {
		t.Errorf("config should contain the API key, got:\n%s", content)
	}
	if !strings.Contains(content, "skip_permissions: true") {
		t.Errorf("config should contain skip_permissions: true, got:\n%s", content)
	}
}

func TestSetupClaudeCmd_ClearsOldCredentialsOnMethodChange(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create a config with existing token auth
	if err := os.MkdirAll(config.ConfigDir(), 0700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	existingConfig := `agents:
  claude:
    auth_method: token
    token: old-token-value
    skip_permissions: true
`
	if err := os.WriteFile(config.GlobalConfigPath(), []byte(existingConfig), 0600); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	// Mock prompter that returns API key (index 2)
	mockPrompter := prompt.NewMockPrompter(2)
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock credential reader that returns an API key
	mockReader := prompt.NewMockCredentialReader("sk-ant-new-api-key")
	oldReader := setupClaudeCredentialReader
	setupClaudeCredentialReader = mockReader
	defer func() { setupClaudeCredentialReader = oldReader }()

	// Mock yes/no prompter
	mockYesNo := prompt.NewMockYesNoPrompter(true)
	oldYesNo := setupClaudeYesNoPrompter
	setupClaudeYesNoPrompter = mockYesNo
	defer func() { setupClaudeYesNoPrompter = oldYesNo }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read and verify the config file
	data, err := os.ReadFile(config.GlobalConfigPath())
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	content := string(data)

	// Should have new auth method
	if !strings.Contains(content, "auth_method: api_key") {
		t.Errorf("config should contain auth_method: api_key, got:\n%s", content)
	}
	if !strings.Contains(content, "api_key: sk-ant-new-api-key") {
		t.Errorf("config should contain the new API key, got:\n%s", content)
	}

	// Should NOT have old token (cleared when switching methods)
	if strings.Contains(content, "old-token-value") {
		t.Errorf("config should not contain old token, got:\n%s", content)
	}
}

func TestSetupClaudeCmd_PreservesOtherConfigSettings(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create a config with custom proxy settings
	if err := os.MkdirAll(config.ConfigDir(), 0700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	existingConfig := `proxy:
  listen: ":9999"
  allow:
    - domain: "custom.example.com"
agents:
  codex:
    command: codex
`
	if err := os.WriteFile(config.GlobalConfigPath(), []byte(existingConfig), 0600); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	// Mock prompter that returns existing login (index 0)
	mockPrompter := prompt.NewMockPrompter(0)
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock extractor that succeeds
	oldExtractor := setupClaudeExtractor
	setupClaudeExtractor = &claude.Extractor{
		CommandRunner: &mockCommandRunner{output: `{"accessToken":"test-token"}`},
		FileChecker:   &mockFileChecker{exists: true},
		UserLookup:    &mockUserLookup{username: "testuser", home: "/home/test"},
		Platform:      "darwin",
	}
	defer func() { setupClaudeExtractor = oldExtractor }()

	// Mock yes/no prompter
	mockYesNo := prompt.NewMockYesNoPrompter(true)
	oldYesNo := setupClaudeYesNoPrompter
	setupClaudeYesNoPrompter = mockYesNo
	defer func() { setupClaudeYesNoPrompter = oldYesNo }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read and verify the config file preserved other settings
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Should have preserved proxy settings
	if cfg.Proxy.Listen != ":9999" {
		t.Errorf("proxy.listen should be preserved, got: %s", cfg.Proxy.Listen)
	}

	// Should have preserved codex agent
	if _, ok := cfg.Agents["codex"]; !ok {
		t.Error("codex agent config should be preserved")
	}

	// Should have added claude agent
	claudeCfg, ok := cfg.Agents["claude"]
	if !ok {
		t.Fatal("claude agent config should exist")
	}
	if claudeCfg.AuthMethod != "existing" {
		t.Errorf("claude auth_method should be 'existing', got: %s", claudeCfg.AuthMethod)
	}
}

func TestSetupClaudeCmd_ConfigLoadError(t *testing.T) {
	// Mock prompter that returns existing login (index 0)
	mockPrompter := prompt.NewMockPrompter(0)
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock extractor that succeeds
	oldExtractor := setupClaudeExtractor
	setupClaudeExtractor = &claude.Extractor{
		CommandRunner: &mockCommandRunner{output: `{"accessToken":"test-token"}`},
		FileChecker:   &mockFileChecker{exists: true},
		UserLookup:    &mockUserLookup{username: "testuser", home: "/home/test"},
		Platform:      "darwin",
	}
	defer func() { setupClaudeExtractor = oldExtractor }()

	// Mock yes/no prompter
	mockYesNo := prompt.NewMockYesNoPrompter(true)
	oldYesNo := setupClaudeYesNoPrompter
	setupClaudeYesNoPrompter = mockYesNo
	defer func() { setupClaudeYesNoPrompter = oldYesNo }()

	// Mock config loader that returns an error
	loadErr := errors.New("config load error")
	oldLoader := setupClaudeConfigLoader
	setupClaudeConfigLoader = func() (*config.GlobalConfig, error) {
		return nil, loadErr
	}
	defer func() { setupClaudeConfigLoader = oldLoader }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err == nil {
		t.Fatal("expected error from config loader")
	}
	if !strings.Contains(err.Error(), "failed to load config") {
		t.Errorf("error should mention config load failure, got: %v", err)
	}
}

func TestSetupClaudeCmd_ConfigWriteError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Mock prompter that returns existing login (index 0)
	mockPrompter := prompt.NewMockPrompter(0)
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock extractor that succeeds
	oldExtractor := setupClaudeExtractor
	setupClaudeExtractor = &claude.Extractor{
		CommandRunner: &mockCommandRunner{output: `{"accessToken":"test-token"}`},
		FileChecker:   &mockFileChecker{exists: true},
		UserLookup:    &mockUserLookup{username: "testuser", home: "/home/test"},
		Platform:      "darwin",
	}
	defer func() { setupClaudeExtractor = oldExtractor }()

	// Mock yes/no prompter
	mockYesNo := prompt.NewMockYesNoPrompter(true)
	oldYesNo := setupClaudeYesNoPrompter
	setupClaudeYesNoPrompter = mockYesNo
	defer func() { setupClaudeYesNoPrompter = oldYesNo }()

	// Mock config writer that returns an error
	writeErr := errors.New("config write error")
	oldWriter := setupClaudeConfigWriter
	setupClaudeConfigWriter = func(*config.GlobalConfig) error {
		return writeErr
	}
	defer func() { setupClaudeConfigWriter = oldWriter }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err == nil {
		t.Fatal("expected error from config writer")
	}
	if !strings.Contains(err.Error(), "failed to write config") {
		t.Errorf("error should mention config write failure, got: %v", err)
	}
}

func TestSetupClaudeCmd_ShowsCorrectConfigPath(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Mock prompter that returns existing login (index 0)
	mockPrompter := prompt.NewMockPrompter(0)
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock extractor that succeeds
	oldExtractor := setupClaudeExtractor
	setupClaudeExtractor = &claude.Extractor{
		CommandRunner: &mockCommandRunner{output: `{"accessToken":"test-token"}`},
		FileChecker:   &mockFileChecker{exists: true},
		UserLookup:    &mockUserLookup{username: "testuser", home: "/home/test"},
		Platform:      "darwin",
	}
	defer func() { setupClaudeExtractor = oldExtractor }()

	// Mock yes/no prompter
	mockYesNo := prompt.NewMockYesNoPrompter(true)
	oldYesNo := setupClaudeYesNoPrompter
	setupClaudeYesNoPrompter = mockYesNo
	defer func() { setupClaudeYesNoPrompter = oldYesNo }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the config path is shown
	output := stdout.String()
	expectedPath := config.GlobalConfigPath()
	if !strings.Contains(output, expectedPath) {
		t.Errorf("output should show config path %q, got: %s", expectedPath, output)
	}
}

// Phase 3.2.7 tests - Handle existing credentials

func TestHasExistingCredentials_NoConfig(t *testing.T) {
	if hasExistingCredentials(nil) {
		t.Error("nil config should not have credentials")
	}
}

func TestHasExistingCredentials_NoAgents(t *testing.T) {
	cfg := &config.GlobalConfig{}
	if hasExistingCredentials(cfg) {
		t.Error("config without agents should not have credentials")
	}
}

func TestHasExistingCredentials_NoClaude(t *testing.T) {
	cfg := &config.GlobalConfig{
		Agents: map[string]config.AgentConfig{
			"codex": {Command: "codex"},
		},
	}
	if hasExistingCredentials(cfg) {
		t.Error("config without claude agent should not have credentials")
	}
}

func TestHasExistingCredentials_EmptyClaudeConfig(t *testing.T) {
	cfg := &config.GlobalConfig{
		Agents: map[string]config.AgentConfig{
			"claude": {},
		},
	}
	if hasExistingCredentials(cfg) {
		t.Error("empty claude config should not count as having credentials")
	}
}

func TestHasExistingCredentials_AuthMethodSet(t *testing.T) {
	cfg := &config.GlobalConfig{
		Agents: map[string]config.AgentConfig{
			"claude": {AuthMethod: "existing"},
		},
	}
	if !hasExistingCredentials(cfg) {
		t.Error("claude config with auth_method should count as having credentials")
	}
}

func TestHasExistingCredentials_TokenSet(t *testing.T) {
	cfg := &config.GlobalConfig{
		Agents: map[string]config.AgentConfig{
			"claude": {Token: "some-token"},
		},
	}
	if !hasExistingCredentials(cfg) {
		t.Error("claude config with token should count as having credentials")
	}
}

func TestHasExistingCredentials_APIKeySet(t *testing.T) {
	cfg := &config.GlobalConfig{
		Agents: map[string]config.AgentConfig{
			"claude": {APIKey: "sk-ant-key"},
		},
	}
	if !hasExistingCredentials(cfg) {
		t.Error("claude config with api_key should count as having credentials")
	}
}

func TestHasExistingCredentials_OnlySkipPerms(t *testing.T) {
	skipPerms := true
	cfg := &config.GlobalConfig{
		Agents: map[string]config.AgentConfig{
			"claude": {SkipPerms: &skipPerms},
		},
	}
	if hasExistingCredentials(cfg) {
		t.Error("claude config with only skip_permissions should not count as having credentials")
	}
}

func TestSetupClaudeCmd_ExistingCredentials_UserDeclinesReplace(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create config with existing credentials
	if err := os.MkdirAll(config.ConfigDir(), 0700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	existingConfig := `agents:
  claude:
    auth_method: token
    token: original-token-value
    skip_permissions: true
`
	if err := os.WriteFile(config.GlobalConfigPath(), []byte(existingConfig), 0600); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	// Mock yes/no prompter that returns false (user pressed Enter for default NO)
	mockYesNo := prompt.NewMockYesNoPrompter(false)
	oldYesNo := setupClaudeYesNoPrompter
	setupClaudeYesNoPrompter = mockYesNo
	defer func() { setupClaudeYesNoPrompter = oldYesNo }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the replacement prompt was shown with correct default
	if len(mockYesNo.Calls) != 1 {
		t.Fatalf("expected 1 yes/no prompt call, got %d", len(mockYesNo.Calls))
	}
	call := mockYesNo.Calls[0]
	expectedPrompt := "Credentials already configured. Replace? [y/N]: "
	if call.Prompt != expectedPrompt {
		t.Errorf("prompt = %q, want %q", call.Prompt, expectedPrompt)
	}
	if call.DefaultYes != false {
		t.Error("defaultYes should be false (N is default)")
	}

	// Verify graceful cancellation message
	output := stdout.String()
	if !strings.Contains(output, "Setup canceled") {
		t.Errorf("output should show cancellation message, got: %s", output)
	}
	if !strings.Contains(output, "Existing credentials unchanged") {
		t.Errorf("output should mention credentials unchanged, got: %s", output)
	}

	// Verify config was NOT modified
	data, err := os.ReadFile(config.GlobalConfigPath())
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "original-token-value") {
		t.Errorf("original token should be preserved, got:\n%s", content)
	}
}

func TestSetupClaudeCmd_ExistingCredentials_UserAcceptsReplace(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create config with existing credentials
	if err := os.MkdirAll(config.ConfigDir(), 0700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	existingConfig := `agents:
  claude:
    auth_method: token
    token: original-token-value
    skip_permissions: false
`
	if err := os.WriteFile(config.GlobalConfigPath(), []byte(existingConfig), 0600); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	// Mock yes/no prompter:
	// - First call: "y" to replace existing credentials
	// - Second call: "y" for skip permissions
	mockYesNo := prompt.NewMockYesNoPrompter(true, true)
	oldYesNo := setupClaudeYesNoPrompter
	setupClaudeYesNoPrompter = mockYesNo
	defer func() { setupClaudeYesNoPrompter = oldYesNo }()

	// Mock prompter that returns API key (index 2)
	mockPrompter := prompt.NewMockPrompter(2)
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock credential reader that returns a new API key
	mockReader := prompt.NewMockCredentialReader("sk-ant-new-api-key")
	oldReader := setupClaudeCredentialReader
	setupClaudeCredentialReader = mockReader
	defer func() { setupClaudeCredentialReader = oldReader }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify two yes/no prompts were made
	if len(mockYesNo.Calls) != 2 {
		t.Fatalf("expected 2 yes/no prompt calls, got %d", len(mockYesNo.Calls))
	}

	// First prompt should be for replacement
	if !strings.Contains(mockYesNo.Calls[0].Prompt, "Replace?") {
		t.Errorf("first prompt should be replacement confirmation, got: %s", mockYesNo.Calls[0].Prompt)
	}

	// Second prompt should be for skip permissions
	if !strings.Contains(mockYesNo.Calls[1].Prompt, "permission prompts") {
		t.Errorf("second prompt should be skip permissions, got: %s", mockYesNo.Calls[1].Prompt)
	}

	// Verify config was updated with new credentials
	data, err := os.ReadFile(config.GlobalConfigPath())
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "auth_method: api_key") {
		t.Errorf("config should have new auth_method, got:\n%s", content)
	}
	if !strings.Contains(content, "sk-ant-new-api-key") {
		t.Errorf("config should have new API key, got:\n%s", content)
	}
	if strings.Contains(content, "original-token-value") {
		t.Errorf("old token should be removed, got:\n%s", content)
	}
}

func TestSetupClaudeCmd_NoExistingCredentials_NoReplacementPrompt(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create config WITHOUT existing credentials (only other settings)
	if err := os.MkdirAll(config.ConfigDir(), 0700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	existingConfig := `proxy:
  listen: ":9999"
`
	if err := os.WriteFile(config.GlobalConfigPath(), []byte(existingConfig), 0600); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	// Mock yes/no prompter - should only be called for skip permissions, not replacement
	mockYesNo := prompt.NewMockYesNoPrompter(true)
	oldYesNo := setupClaudeYesNoPrompter
	setupClaudeYesNoPrompter = mockYesNo
	defer func() { setupClaudeYesNoPrompter = oldYesNo }()

	// Mock prompter that returns existing login (index 0)
	mockPrompter := prompt.NewMockPrompter(0)
	oldPrompter := setupClaudePrompter
	setupClaudePrompter = mockPrompter
	defer func() { setupClaudePrompter = oldPrompter }()

	// Mock extractor that succeeds
	oldExtractor := setupClaudeExtractor
	setupClaudeExtractor = &claude.Extractor{
		CommandRunner: &mockCommandRunner{output: `{"accessToken":"test-token"}`},
		FileChecker:   &mockFileChecker{exists: true},
		UserLookup:    &mockUserLookup{username: "testuser", home: "/home/test"},
		Platform:      "darwin",
	}
	defer func() { setupClaudeExtractor = oldExtractor }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify only ONE yes/no prompt was made (skip permissions, NOT replacement)
	if len(mockYesNo.Calls) != 1 {
		t.Fatalf("expected 1 yes/no prompt call (skip perms only), got %d", len(mockYesNo.Calls))
	}

	// The prompt should be about skip permissions, not replacement
	if strings.Contains(mockYesNo.Calls[0].Prompt, "Replace?") {
		t.Error("should not prompt for replacement when no existing credentials")
	}
	if !strings.Contains(mockYesNo.Calls[0].Prompt, "permission prompts") {
		t.Error("should prompt for skip permissions")
	}

	// Verify config was saved
	output := stdout.String()
	if !strings.Contains(output, "Configuration saved") {
		t.Errorf("should save config without replacement prompt, got: %s", output)
	}
}

func TestSetupClaudeCmd_ExistingCredentials_ConfigLoadError(t *testing.T) {
	// Mock config loader that returns an error
	loadErr := errors.New("config load error")
	oldLoader := setupClaudeConfigLoader
	setupClaudeConfigLoader = func() (*config.GlobalConfig, error) {
		return nil, loadErr
	}
	defer func() { setupClaudeConfigLoader = oldLoader }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err == nil {
		t.Fatal("expected error from config loader")
	}
	if !strings.Contains(err.Error(), "failed to load config") {
		t.Errorf("error should mention config load failure, got: %v", err)
	}
}

func TestSetupClaudeCmd_ExistingCredentials_PromptError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Create config with existing credentials
	if err := os.MkdirAll(config.ConfigDir(), 0700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	existingConfig := `agents:
  claude:
    auth_method: existing
`
	if err := os.WriteFile(config.GlobalConfigPath(), []byte(existingConfig), 0600); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	// Mock yes/no prompter that returns an error
	promptErr := errors.New("input error")
	mockYesNo := &prompt.MockYesNoPrompter{
		Errors: []error{promptErr},
	}
	oldYesNo := setupClaudeYesNoPrompter
	setupClaudeYesNoPrompter = mockYesNo
	defer func() { setupClaudeYesNoPrompter = oldYesNo }()

	var stdout bytes.Buffer
	setupClaudeCmd.SetOut(&stdout)
	setupClaudeCmd.SetErr(&stdout)

	err := setupClaudeCmd.RunE(setupClaudeCmd, nil)
	if err == nil {
		t.Fatal("expected error from yes/no prompter")
	}
	if !strings.Contains(err.Error(), "failed to get confirmation") {
		t.Errorf("error should mention confirmation failure, got: %v", err)
	}
}
