package cloister

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xdg/cloister/internal/claude"
	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/container"
)

func TestStartOptions_Fields(t *testing.T) {
	// Test that StartOptions has expected fields
	opts := StartOptions{
		ProjectPath: "/path/to/project",
		ProjectName: "myproject",
		BranchName:  "main",
		Image:       "custom:latest",
	}

	if opts.ProjectPath != "/path/to/project" {
		t.Errorf("ProjectPath = %q, want %q", opts.ProjectPath, "/path/to/project")
	}
	if opts.ProjectName != "myproject" {
		t.Errorf("ProjectName = %q, want %q", opts.ProjectName, "myproject")
	}
	if opts.BranchName != "main" {
		t.Errorf("BranchName = %q, want %q", opts.BranchName, "main")
	}
	if opts.Image != "custom:latest" {
		t.Errorf("Image = %q, want %q", opts.Image, "custom:latest")
	}
}

func TestInjectUserSettings_MissingClaudeDir(t *testing.T) {
	// Test that injectUserSettings returns nil when ~/.claude/ doesn't exist.
	// Skip if ~/.claude exists to avoid slow copy operation - integration tests
	// cover the real behavior.
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home dir: %v", err)
	}
	claudeDir := filepath.Join(homeDir, ".claude")
	if _, err := os.Stat(claudeDir); err == nil {
		t.Skip("~/.claude exists - skipping to avoid slow copy; integration tests cover this")
	}

	// ~/.claude doesn't exist, so injectUserSettings should return nil (no-op)
	err = injectUserSettings("nonexistent-container-12345")
	if err != nil {
		t.Errorf("Expected nil when ~/.claude doesn't exist, got: %v", err)
	}
}

// mockManager is a test double for ContainerManager that records calls
// and returns configurable results.
type mockManager struct {
	createCalled         bool
	createConfig         *container.Config
	createResult         string
	createError          error
	startContainerCalled bool
	startContainerName   string
	startContainerError  error
	stopCalled           bool
	stopContainerName    string
	stopError            error
	attachCalled         bool
	attachContainerName  string
	attachExitCode       int
	attachError          error
}

func (m *mockManager) Create(cfg *container.Config) (string, error) {
	m.createCalled = true
	m.createConfig = cfg
	return m.createResult, m.createError
}

func (m *mockManager) Start(cfg *container.Config) (string, error) {
	// Not used by cloister.Start, but required by interface
	return "", nil
}

func (m *mockManager) StartContainer(containerName string) error {
	m.startContainerCalled = true
	m.startContainerName = containerName
	return m.startContainerError
}

func (m *mockManager) Stop(containerName string) error {
	m.stopCalled = true
	m.stopContainerName = containerName
	return m.stopError
}

func (m *mockManager) Attach(containerName string) (int, error) {
	m.attachCalled = true
	m.attachContainerName = containerName
	return m.attachExitCode, m.attachError
}

func TestWithManager_InjectionWorks(t *testing.T) {
	// Test that WithManager properly injects the manager
	mock := &mockManager{
		attachExitCode: 42,
	}

	exitCode, err := Attach("test-container", WithManager(mock))
	if err != nil {
		t.Fatalf("Attach() returned error: %v", err)
	}
	if exitCode != 42 {
		t.Errorf("Attach() exitCode = %d, want 42", exitCode)
	}
	if !mock.attachCalled {
		t.Error("mock.Attach() was not called")
	}
	if mock.attachContainerName != "test-container" {
		t.Errorf("mock.attachContainerName = %q, want %q", mock.attachContainerName, "test-container")
	}
}

func TestAttach_WithMockManager_ReturnsError(t *testing.T) {
	// Test that errors from the manager are propagated
	expectedErr := errors.New("attach failed")
	mock := &mockManager{
		attachError: expectedErr,
	}

	_, err := Attach("test-container", WithManager(mock))
	if err == nil {
		t.Fatal("Attach() should return error from manager")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("Attach() error = %v, want %v", err, expectedErr)
	}
}

func TestStop_WithMockManager(t *testing.T) {
	// Test that Stop calls the injected manager
	mock := &mockManager{}

	err := Stop("test-container", "", WithManager(mock))
	if err != nil {
		t.Fatalf("Stop() returned error: %v", err)
	}
	if !mock.stopCalled {
		t.Error("mock.Stop() was not called")
	}
	if mock.stopContainerName != "test-container" {
		t.Errorf("mock.stopContainerName = %q, want %q", mock.stopContainerName, "test-container")
	}
}

func TestStop_WithMockManager_ReturnsError(t *testing.T) {
	// Test that errors from the manager are propagated
	expectedErr := errors.New("stop failed")
	mock := &mockManager{
		stopError: expectedErr,
	}

	err := Stop("test-container", "", WithManager(mock))
	if err == nil {
		t.Fatal("Stop() should return error from manager")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("Stop() error = %v, want %v", err, expectedErr)
	}
}

func TestApplyOptions_DefaultsToRealImplementations(t *testing.T) {
	// Test that applyOptions returns real implementations when no options are provided
	deps := applyOptions()

	if deps.manager == nil {
		t.Fatal("applyOptions().manager is nil")
	}
	if deps.guardian == nil {
		t.Fatal("applyOptions().guardian is nil")
	}

	// Verify manager is the concrete type
	_, ok := deps.manager.(*container.Manager)
	if !ok {
		t.Errorf("applyOptions().manager is %T, want *container.Manager", deps.manager)
	}

	// Verify guardian is the default implementation
	_, ok = deps.guardian.(defaultGuardianManager)
	if !ok {
		t.Errorf("applyOptions().guardian is %T, want defaultGuardianManager", deps.guardian)
	}

	// Verify new dependencies are also set
	if deps.configLoader == nil {
		t.Fatal("applyOptions().configLoader is nil")
	}
	if deps.credentialInjector == nil {
		t.Fatal("applyOptions().credentialInjector is nil")
	}
	if deps.fileCopier == nil {
		t.Fatal("applyOptions().fileCopier is nil")
	}
}

// mockGuardian is a test double for GuardianManager that always succeeds.
type mockGuardian struct {
	ensureRunningErr error
	registerTokenErr error
	revokeTokenErr   error
}

func (m *mockGuardian) EnsureRunning() error {
	return m.ensureRunningErr
}

func (m *mockGuardian) RegisterToken(token, cloisterName, projectName string) error {
	return m.registerTokenErr
}

func (m *mockGuardian) RevokeToken(token string) error {
	return m.revokeTokenErr
}

// mockConfigLoader is a test double for ConfigLoader.
type mockConfigLoader struct {
	config *config.GlobalConfig
	err    error
}

func (m *mockConfigLoader) LoadGlobalConfig() (*config.GlobalConfig, error) {
	return m.config, m.err
}

// mockCredentialInjector is a test double for CredentialInjector.
type mockCredentialInjector struct {
	called         bool
	receivedConfig *config.AgentConfig
	result         *claude.InjectionConfig
	err            error
}

func (m *mockCredentialInjector) InjectCredentials(cfg *config.AgentConfig) (*claude.InjectionConfig, error) {
	m.called = true
	m.receivedConfig = cfg
	return m.result, m.err
}

// mockFileCopier is a test double for FileCopier.
type mockFileCopier struct {
	calls []fileCopyCall
	err   error
}

type fileCopyCall struct {
	containerName string
	destPath      string
	content       string
	uid           string
	gid           string
}

func (m *mockFileCopier) WriteFileToContainerWithOwner(containerName, destPath, content, uid, gid string) error {
	m.calls = append(m.calls, fileCopyCall{containerName, destPath, content, uid, gid})
	return m.err
}

// TestStart_WithTokenAuth verifies that token-based credentials are passed as env vars.
func TestStart_WithTokenAuth(t *testing.T) {
	// Set up mocks
	mockMgr := &mockManager{createResult: "container-123"}
	mockGuard := &mockGuardian{}
	mockCfgLoader := &mockConfigLoader{
		config: &config.GlobalConfig{
			Agents: map[string]config.AgentConfig{
				"claude": {
					AuthMethod: "token",
					Token:      "sk-ant-oat01-test-token",
				},
			},
		},
	}
	mockInjector := &mockCredentialInjector{
		result: &claude.InjectionConfig{
			EnvVars: map[string]string{
				"CLAUDE_CODE_OAUTH_TOKEN": "sk-ant-oat01-test-token",
			},
			Files: map[string]string{},
		},
	}
	mockCopier := &mockFileCopier{}

	// Use t.TempDir() to avoid touching real token store
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	opts := StartOptions{
		ProjectPath: "/path/to/project",
		ProjectName: "testproject",
		BranchName:  "main",
	}

	_, _, err := Start(opts,
		WithManager(mockMgr),
		WithGuardian(mockGuard),
		WithConfigLoader(mockCfgLoader),
		WithCredentialInjector(mockInjector),
		WithFileCopier(mockCopier),
	)
	if err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// Verify credential injector was called with correct config
	if !mockInjector.called {
		t.Error("credentialInjector.InjectCredentials() was not called")
	}
	if mockInjector.receivedConfig == nil {
		t.Fatal("credentialInjector received nil config")
	}
	if mockInjector.receivedConfig.AuthMethod != "token" {
		t.Errorf("receivedConfig.AuthMethod = %q, want %q", mockInjector.receivedConfig.AuthMethod, "token")
	}
	if mockInjector.receivedConfig.Token != "sk-ant-oat01-test-token" {
		t.Errorf("receivedConfig.Token = %q, want %q", mockInjector.receivedConfig.Token, "sk-ant-oat01-test-token")
	}

	// Verify env vars were passed to container
	if mockMgr.createConfig == nil {
		t.Fatal("manager.Create() was not called")
	}
	envVars := mockMgr.createConfig.EnvVars
	found := false
	for _, env := range envVars {
		if env == "CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat01-test-token" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("CLAUDE_CODE_OAUTH_TOKEN not found in container env vars: %v", envVars)
	}

	// Verify no files were copied (token auth uses env vars only)
	if len(mockCopier.calls) != 0 {
		t.Errorf("expected no file copies for token auth, got %d", len(mockCopier.calls))
	}
}

// TestStart_WithAPIKeyAuth verifies that API key credentials are passed as env vars.
func TestStart_WithAPIKeyAuth(t *testing.T) {
	mockMgr := &mockManager{createResult: "container-123"}
	mockGuard := &mockGuardian{}
	mockCfgLoader := &mockConfigLoader{
		config: &config.GlobalConfig{
			Agents: map[string]config.AgentConfig{
				"claude": {
					AuthMethod: "api_key",
					APIKey:     "sk-ant-api01-test-key",
				},
			},
		},
	}
	mockInjector := &mockCredentialInjector{
		result: &claude.InjectionConfig{
			EnvVars: map[string]string{
				"ANTHROPIC_API_KEY": "sk-ant-api01-test-key",
			},
			Files: map[string]string{},
		},
	}
	mockCopier := &mockFileCopier{}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	opts := StartOptions{
		ProjectPath: "/path/to/project",
		ProjectName: "testproject",
		BranchName:  "main",
	}

	_, _, err := Start(opts,
		WithManager(mockMgr),
		WithGuardian(mockGuard),
		WithConfigLoader(mockCfgLoader),
		WithCredentialInjector(mockInjector),
		WithFileCopier(mockCopier),
	)
	if err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// Verify env vars were passed to container
	envVars := mockMgr.createConfig.EnvVars
	found := false
	for _, env := range envVars {
		if env == "ANTHROPIC_API_KEY=sk-ant-api01-test-key" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ANTHROPIC_API_KEY not found in container env vars: %v", envVars)
	}
}

// TestStart_WithExistingAuth verifies that "existing" auth injects credential files.
func TestStart_WithExistingAuth(t *testing.T) {
	credJSON := `{"claudeAiOauth":{"accessToken":"test-access","refreshToken":"test-refresh"}}`

	mockMgr := &mockManager{createResult: "container-123"}
	mockGuard := &mockGuardian{}
	mockCfgLoader := &mockConfigLoader{
		config: &config.GlobalConfig{
			Agents: map[string]config.AgentConfig{
				"claude": {
					AuthMethod: "existing",
				},
			},
		},
	}
	mockInjector := &mockCredentialInjector{
		result: &claude.InjectionConfig{
			EnvVars: map[string]string{},
			Files: map[string]string{
				"/home/cloister/.claude/.credentials.json": credJSON,
			},
		},
	}
	mockCopier := &mockFileCopier{}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	opts := StartOptions{
		ProjectPath: "/path/to/project",
		ProjectName: "testproject",
		BranchName:  "main",
	}

	_, _, err := Start(opts,
		WithManager(mockMgr),
		WithGuardian(mockGuard),
		WithConfigLoader(mockCfgLoader),
		WithCredentialInjector(mockInjector),
		WithFileCopier(mockCopier),
	)
	if err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// Verify file was copied to container
	if len(mockCopier.calls) != 1 {
		t.Fatalf("expected 1 file copy, got %d", len(mockCopier.calls))
	}
	call := mockCopier.calls[0]
	if call.destPath != "/home/cloister/.claude/.credentials.json" {
		t.Errorf("destPath = %q, want %q", call.destPath, "/home/cloister/.claude/.credentials.json")
	}
	if call.content != credJSON {
		t.Errorf("content = %q, want %q", call.content, credJSON)
	}
	// Container name should match generated name
	if !strings.HasPrefix(call.containerName, "cloister-") {
		t.Errorf("containerName = %q, expected prefix 'cloister-'", call.containerName)
	}
}

// TestStart_NoConfigFallsBackToEnvVars verifies fallback when no config credentials exist.
func TestStart_NoConfigFallsBackToEnvVars(t *testing.T) {
	mockMgr := &mockManager{createResult: "container-123"}
	mockGuard := &mockGuardian{}
	// Config with no auth method set
	mockCfgLoader := &mockConfigLoader{
		config: &config.GlobalConfig{
			Agents: map[string]config.AgentConfig{
				"claude": {
					// No AuthMethod - should fall back to env vars
				},
			},
		},
	}
	mockInjector := &mockCredentialInjector{
		result: &claude.InjectionConfig{
			EnvVars: map[string]string{},
			Files:   map[string]string{},
		},
	}
	mockCopier := &mockFileCopier{}

	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	// Set a host env var that should be passed through
	t.Setenv("ANTHROPIC_API_KEY", "host-api-key")

	opts := StartOptions{
		ProjectPath: "/path/to/project",
		ProjectName: "testproject",
		BranchName:  "main",
	}

	_, _, err := Start(opts,
		WithManager(mockMgr),
		WithGuardian(mockGuard),
		WithConfigLoader(mockCfgLoader),
		WithCredentialInjector(mockInjector),
		WithFileCopier(mockCopier),
	)
	if err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// Credential injector should NOT be called (no auth method configured)
	if mockInjector.called {
		t.Error("credentialInjector.InjectCredentials() should not be called when no auth method configured")
	}

	// Host env var should be passed through (Phase 1 fallback behavior)
	envVars := mockMgr.createConfig.EnvVars
	found := false
	for _, env := range envVars {
		if env == "ANTHROPIC_API_KEY=host-api-key" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ANTHROPIC_API_KEY (from host) not found in container env vars: %v", envVars)
	}
}

// TestStart_CredentialInjectionError verifies error handling when injection fails.
func TestStart_CredentialInjectionError(t *testing.T) {
	mockMgr := &mockManager{createResult: "container-123"}
	mockGuard := &mockGuardian{}
	mockCfgLoader := &mockConfigLoader{
		config: &config.GlobalConfig{
			Agents: map[string]config.AgentConfig{
				"claude": {
					AuthMethod: "existing",
				},
			},
		},
	}
	mockInjector := &mockCredentialInjector{
		err: errors.New("keychain access denied"),
	}
	mockCopier := &mockFileCopier{}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	opts := StartOptions{
		ProjectPath: "/path/to/project",
		ProjectName: "testproject",
		BranchName:  "main",
	}

	_, _, err := Start(opts,
		WithManager(mockMgr),
		WithGuardian(mockGuard),
		WithConfigLoader(mockCfgLoader),
		WithCredentialInjector(mockInjector),
		WithFileCopier(mockCopier),
	)

	if err == nil {
		t.Fatal("Start() should return error when credential injection fails")
	}
	if !strings.Contains(err.Error(), "credential injection failed") {
		t.Errorf("error = %q, expected to contain 'credential injection failed'", err.Error())
	}
	if !strings.Contains(err.Error(), "keychain access denied") {
		t.Errorf("error = %q, expected to contain 'keychain access denied'", err.Error())
	}
}

// TestStart_FileCopyError verifies error handling when file copy fails.
func TestStart_FileCopyError(t *testing.T) {
	mockMgr := &mockManager{createResult: "container-123"}
	mockGuard := &mockGuardian{}
	mockCfgLoader := &mockConfigLoader{
		config: &config.GlobalConfig{
			Agents: map[string]config.AgentConfig{
				"claude": {
					AuthMethod: "existing",
				},
			},
		},
	}
	mockInjector := &mockCredentialInjector{
		result: &claude.InjectionConfig{
			EnvVars: map[string]string{},
			Files: map[string]string{
				"/home/cloister/.claude/.credentials.json": "{}",
			},
		},
	}
	mockCopier := &mockFileCopier{
		err: errors.New("docker exec failed"),
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	opts := StartOptions{
		ProjectPath: "/path/to/project",
		ProjectName: "testproject",
		BranchName:  "main",
	}

	_, _, err := Start(opts,
		WithManager(mockMgr),
		WithGuardian(mockGuard),
		WithConfigLoader(mockCfgLoader),
		WithCredentialInjector(mockInjector),
		WithFileCopier(mockCopier),
	)

	if err == nil {
		t.Fatal("Start() should return error when file copy fails")
	}
	if !strings.Contains(err.Error(), "failed to write credential file") {
		t.Errorf("error = %q, expected to contain 'failed to write credential file'", err.Error())
	}
}

// TestStart_EnvFallback_PrintsDeprecationWarning verifies that using env var fallback prints a warning.
func TestStart_EnvFallback_PrintsDeprecationWarning(t *testing.T) {
	mockMgr := &mockManager{createResult: "container-123"}
	mockGuard := &mockGuardian{}
	// Config with no auth method set - triggers env var fallback
	mockCfgLoader := &mockConfigLoader{
		config: &config.GlobalConfig{
			Agents: map[string]config.AgentConfig{
				"claude": {}, // No AuthMethod
			},
		},
	}
	mockInjector := &mockCredentialInjector{
		result: &claude.InjectionConfig{
			EnvVars: map[string]string{},
			Files:   map[string]string{},
		},
	}
	mockCopier := &mockFileCopier{}

	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	// Set host env var to trigger the fallback
	t.Setenv("ANTHROPIC_API_KEY", "host-api-key")

	// Capture stderr
	var stderrBuf bytes.Buffer

	opts := StartOptions{
		ProjectPath: "/path/to/project",
		ProjectName: "testproject",
		BranchName:  "main",
	}

	_, _, err := Start(opts,
		WithManager(mockMgr),
		WithGuardian(mockGuard),
		WithConfigLoader(mockCfgLoader),
		WithCredentialInjector(mockInjector),
		WithFileCopier(mockCopier),
		WithStderr(&stderrBuf),
	)
	if err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// Verify deprecation warning was printed
	stderrOutput := stderrBuf.String()
	if !strings.Contains(stderrOutput, "Warning: Using ANTHROPIC_API_KEY from environment.") {
		t.Errorf("stderr should contain deprecation warning about ANTHROPIC_API_KEY, got: %q", stderrOutput)
	}
	if !strings.Contains(stderrOutput, "Run 'cloister setup claude' to store credentials in config.") {
		t.Errorf("stderr should contain setup suggestion, got: %q", stderrOutput)
	}
}

// TestStart_EnvFallback_PrintsWarningForOAuthToken verifies warning for CLAUDE_CODE_OAUTH_TOKEN.
func TestStart_EnvFallback_PrintsWarningForOAuthToken(t *testing.T) {
	mockMgr := &mockManager{createResult: "container-123"}
	mockGuard := &mockGuardian{}
	mockCfgLoader := &mockConfigLoader{
		config: &config.GlobalConfig{
			Agents: map[string]config.AgentConfig{
				"claude": {}, // No AuthMethod
			},
		},
	}
	mockInjector := &mockCredentialInjector{
		result: &claude.InjectionConfig{
			EnvVars: map[string]string{},
			Files:   map[string]string{},
		},
	}
	mockCopier := &mockFileCopier{}

	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	// Set OAUTH token only (not API key)
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "host-oauth-token")

	var stderrBuf bytes.Buffer

	opts := StartOptions{
		ProjectPath: "/path/to/project",
		ProjectName: "testproject",
		BranchName:  "main",
	}

	_, _, err := Start(opts,
		WithManager(mockMgr),
		WithGuardian(mockGuard),
		WithConfigLoader(mockCfgLoader),
		WithCredentialInjector(mockInjector),
		WithFileCopier(mockCopier),
		WithStderr(&stderrBuf),
	)
	if err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// Verify deprecation warning was printed for OAuth token
	stderrOutput := stderrBuf.String()
	if !strings.Contains(stderrOutput, "Warning: Using CLAUDE_CODE_OAUTH_TOKEN from environment.") {
		t.Errorf("stderr should contain deprecation warning about CLAUDE_CODE_OAUTH_TOKEN, got: %q", stderrOutput)
	}
}

// TestStart_EnvFallback_NoWarningWhenNoEnvVars verifies no warning when no env vars set.
func TestStart_EnvFallback_NoWarningWhenNoEnvVars(t *testing.T) {
	mockMgr := &mockManager{createResult: "container-123"}
	mockGuard := &mockGuardian{}
	mockCfgLoader := &mockConfigLoader{
		config: &config.GlobalConfig{
			Agents: map[string]config.AgentConfig{
				"claude": {}, // No AuthMethod
			},
		},
	}
	mockInjector := &mockCredentialInjector{
		result: &claude.InjectionConfig{
			EnvVars: map[string]string{},
			Files:   map[string]string{},
		},
	}
	mockCopier := &mockFileCopier{}

	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	// Clear any credential env vars that might be set
	// (t.Setenv will restore after test)

	var stderrBuf bytes.Buffer

	opts := StartOptions{
		ProjectPath: "/path/to/project",
		ProjectName: "testproject",
		BranchName:  "main",
	}

	_, _, err := Start(opts,
		WithManager(mockMgr),
		WithGuardian(mockGuard),
		WithConfigLoader(mockCfgLoader),
		WithCredentialInjector(mockInjector),
		WithFileCopier(mockCopier),
		WithStderr(&stderrBuf),
	)
	if err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// Verify no warning was printed when no env vars are set
	stderrOutput := stderrBuf.String()
	if strings.Contains(stderrOutput, "Warning:") {
		t.Errorf("stderr should not contain warning when no env vars set, got: %q", stderrOutput)
	}
}

// TestStart_ConfigCredentials_NoDeprecationWarning verifies no warning when config has credentials.
func TestStart_ConfigCredentials_NoDeprecationWarning(t *testing.T) {
	mockMgr := &mockManager{createResult: "container-123"}
	mockGuard := &mockGuardian{}
	mockCfgLoader := &mockConfigLoader{
		config: &config.GlobalConfig{
			Agents: map[string]config.AgentConfig{
				"claude": {
					AuthMethod: "api_key",
					APIKey:     "config-api-key",
				},
			},
		},
	}
	mockInjector := &mockCredentialInjector{
		result: &claude.InjectionConfig{
			EnvVars: map[string]string{
				"ANTHROPIC_API_KEY": "config-api-key",
			},
			Files: map[string]string{},
		},
	}
	mockCopier := &mockFileCopier{}

	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	// Set host env var - but it should be ignored since config has credentials
	t.Setenv("ANTHROPIC_API_KEY", "host-api-key")

	var stderrBuf bytes.Buffer

	opts := StartOptions{
		ProjectPath: "/path/to/project",
		ProjectName: "testproject",
		BranchName:  "main",
	}

	_, _, err := Start(opts,
		WithManager(mockMgr),
		WithGuardian(mockGuard),
		WithConfigLoader(mockCfgLoader),
		WithCredentialInjector(mockInjector),
		WithFileCopier(mockCopier),
		WithStderr(&stderrBuf),
	)
	if err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// Verify no deprecation warning was printed (config credentials used)
	stderrOutput := stderrBuf.String()
	if strings.Contains(stderrOutput, "Warning:") {
		t.Errorf("stderr should not contain warning when config has credentials, got: %q", stderrOutput)
	}
}
