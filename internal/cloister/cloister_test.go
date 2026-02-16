package cloister

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/xdg/cloister/internal/agent"
	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/term"
	"github.com/xdg/cloister/internal/testutil"
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

// mockManager is a test double for ContainerManager that records calls
// and returns configurable results.
type mockManager struct {
	containerExistsResult bool
	containerExistsError  error
	createCalled          bool
	createConfig          *container.Config
	createResult          string
	createError           error
	startContainerCalled  bool
	startContainerName    string
	startContainerError   error
	stopCalled            bool
	stopContainerName     string
	stopError             error
	attachCalled          bool
	attachContainerName   string
	attachExitCode        int
	attachError           error
}

func (m *mockManager) ContainerExists(_ string) (bool, error) {
	return m.containerExistsResult, m.containerExistsError
}

func (m *mockManager) Create(cfg *container.Config) (string, error) {
	m.createCalled = true
	m.createConfig = cfg
	return m.createResult, m.createError
}

func (m *mockManager) Start(_ *container.Config) (string, error) {
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
	testutil.IsolateXDGDirs(t)
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
	testutil.IsolateXDGDirs(t)
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

	// Verify config loader is set
	if deps.configLoader == nil {
		t.Fatal("applyOptions().configLoader is nil")
	}

	// Verify agent is nil by default (resolved later based on config)
	if deps.agent != nil {
		t.Errorf("applyOptions().agent should be nil by default, got %T", deps.agent)
	}
}

// mockGuardian is a test double for GuardianManager that always succeeds.
type mockGuardian struct {
	ensureRunningErr     error
	registerTokenErr     error
	registerTokenFullErr error
	revokeTokenErr       error
}

func (m *mockGuardian) EnsureRunning() error {
	return m.ensureRunningErr
}

func (m *mockGuardian) RegisterToken(_, _, _ string) error {
	return m.registerTokenErr
}

func (m *mockGuardian) RegisterTokenFull(_, _, _, _ string) error {
	if m.registerTokenFullErr != nil {
		return m.registerTokenFullErr
	}
	return m.registerTokenErr
}

func (m *mockGuardian) RevokeToken(_ string) error {
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

// mockAgent is a test double for agent.Agent and agent.ContainerEnvProvider.
type mockAgent struct {
	name          string
	setupCalled   bool
	setupCfg      *config.AgentConfig
	setupResult   *agent.SetupResult
	setupErr      error
	containerName string

	// containerEnvVars is returned by GetContainerEnvVars if set.
	// If nil, GetContainerEnvVars returns nil, nil.
	containerEnvVars map[string]string
	containerErr     error
}

func (m *mockAgent) Name() string {
	return m.name
}

func (m *mockAgent) Setup(containerName string, agentCfg *config.AgentConfig) (*agent.SetupResult, error) {
	m.setupCalled = true
	m.containerName = containerName
	m.setupCfg = agentCfg
	return m.setupResult, m.setupErr
}

// GetContainerEnvVars implements agent.ContainerEnvProvider.
func (m *mockAgent) GetContainerEnvVars(_ *config.AgentConfig) (map[string]string, error) {
	return m.containerEnvVars, m.containerErr
}

// TestStart_WithTokenAuth verifies that token-based credentials trigger agent setup
// and that credential env vars are passed to the container at creation time.
func TestStart_WithTokenAuth(t *testing.T) {
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
	mockAgt := &mockAgent{
		name: "claude",
		// Container env vars returned by GetContainerEnvVars (before container creation)
		containerEnvVars: map[string]string{
			"CLAUDE_CODE_OAUTH_TOKEN": "sk-ant-oat01-test-token",
		},
		// Setup result (after container creation) - env vars here are redundant
		setupResult: &agent.SetupResult{
			EnvVars: map[string]string{
				"CLAUDE_CODE_OAUTH_TOKEN": "sk-ant-oat01-test-token",
			},
		},
	}

	// Use t.TempDir() to avoid touching real token store
	t.Setenv("HOME", t.TempDir())
	testutil.IsolateXDGDirs(t)

	opts := StartOptions{
		ProjectPath: "/path/to/project",
		ProjectName: "testproject",
		BranchName:  "main",
	}

	_, _, err := Start(opts,
		WithManager(mockMgr),
		WithGuardian(mockGuard),
		WithConfigLoader(mockCfgLoader),
		WithAgent(mockAgt),
	)
	if err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// Verify agent setup was called with correct config
	if !mockAgt.setupCalled {
		t.Error("agent.Setup() was not called")
	}
	if mockAgt.setupCfg == nil {
		t.Fatal("agent.Setup() received nil config")
	}
	if mockAgt.setupCfg.AuthMethod != "token" {
		t.Errorf("setupCfg.AuthMethod = %q, want %q", mockAgt.setupCfg.AuthMethod, "token")
	}
	if mockAgt.setupCfg.Token != "sk-ant-oat01-test-token" {
		t.Errorf("setupCfg.Token = %q, want %q", mockAgt.setupCfg.Token, "sk-ant-oat01-test-token")
	}

	// Verify container was created
	if mockMgr.createConfig == nil {
		t.Fatal("manager.Create() was not called")
	}

	// Verify credential env vars were passed to container at creation time
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
}

// TestStart_WithAPIKeyAuth verifies that API key credentials trigger agent setup
// and that credential env vars are passed to the container at creation time.
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
	mockAgt := &mockAgent{
		name: "claude",
		containerEnvVars: map[string]string{
			"ANTHROPIC_API_KEY": "sk-ant-api01-test-key",
		},
		setupResult: &agent.SetupResult{
			EnvVars: map[string]string{
				"ANTHROPIC_API_KEY": "sk-ant-api01-test-key",
			},
		},
	}

	t.Setenv("HOME", t.TempDir())
	testutil.IsolateXDGDirs(t)

	opts := StartOptions{
		ProjectPath: "/path/to/project",
		ProjectName: "testproject",
		BranchName:  "main",
	}

	_, _, err := Start(opts,
		WithManager(mockMgr),
		WithGuardian(mockGuard),
		WithConfigLoader(mockCfgLoader),
		WithAgent(mockAgt),
	)
	if err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// Verify agent setup was called
	if !mockAgt.setupCalled {
		t.Error("agent.Setup() was not called")
	}
	if mockAgt.setupCfg.AuthMethod != "api_key" {
		t.Errorf("setupCfg.AuthMethod = %q, want %q", mockAgt.setupCfg.AuthMethod, "api_key")
	}

	// Verify credential env vars were passed to container at creation time
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

// TestStart_WithTokenAuthCallsSetup verifies that "token" auth triggers agent setup.
func TestStart_WithTokenAuthCallsSetup(t *testing.T) {
	mockMgr := &mockManager{createResult: "container-123"}
	mockGuard := &mockGuardian{}
	mockCfgLoader := &mockConfigLoader{
		config: &config.GlobalConfig{
			Agents: map[string]config.AgentConfig{
				"claude": {
					AuthMethod: "token",
					Token:      "test-oauth-token",
				},
			},
		},
	}
	mockAgt := &mockAgent{
		name:        "claude",
		setupResult: &agent.SetupResult{EnvVars: map[string]string{}},
	}

	t.Setenv("HOME", t.TempDir())
	testutil.IsolateXDGDirs(t)

	opts := StartOptions{
		ProjectPath: "/path/to/project",
		ProjectName: "testproject",
		BranchName:  "main",
	}

	_, _, err := Start(opts,
		WithManager(mockMgr),
		WithGuardian(mockGuard),
		WithConfigLoader(mockCfgLoader),
		WithAgent(mockAgt),
	)
	if err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// Verify agent setup was called
	if !mockAgt.setupCalled {
		t.Error("agent.Setup() was not called")
	}
	if mockAgt.setupCfg.AuthMethod != "token" {
		t.Errorf("setupCfg.AuthMethod = %q, want %q", mockAgt.setupCfg.AuthMethod, "token")
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
	mockAgt := &mockAgent{
		name:        "claude",
		setupResult: &agent.SetupResult{EnvVars: map[string]string{}},
	}

	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
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
		WithAgent(mockAgt),
	)
	if err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// Agent setup should still be called (with nil auth method)
	if !mockAgt.setupCalled {
		t.Error("agent.Setup() was not called")
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

// TestStart_AgentSetupError verifies error handling when agent setup fails.
func TestStart_AgentSetupError(t *testing.T) {
	mockMgr := &mockManager{createResult: "container-123"}
	mockGuard := &mockGuardian{}
	mockCfgLoader := &mockConfigLoader{
		config: &config.GlobalConfig{
			Agents: map[string]config.AgentConfig{
				"claude": {
					AuthMethod: "token",
					Token:      "test-token",
				},
			},
		},
	}
	mockAgt := &mockAgent{
		name:     "claude",
		setupErr: errors.New("agent setup failed"),
	}

	t.Setenv("HOME", t.TempDir())
	testutil.IsolateXDGDirs(t)

	opts := StartOptions{
		ProjectPath: "/path/to/project",
		ProjectName: "testproject",
		BranchName:  "main",
	}

	_, _, err := Start(opts,
		WithManager(mockMgr),
		WithGuardian(mockGuard),
		WithConfigLoader(mockCfgLoader),
		WithAgent(mockAgt),
	)

	if err == nil {
		t.Fatal("Start() should return error when agent setup fails")
	}
	if !strings.Contains(err.Error(), "agent setup failed") {
		t.Errorf("error = %q, expected to contain 'agent setup failed'", err.Error())
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
	mockAgt := &mockAgent{
		name:        "claude",
		setupResult: &agent.SetupResult{EnvVars: map[string]string{}},
	}

	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	// Set host env var to trigger the fallback
	t.Setenv("ANTHROPIC_API_KEY", "host-api-key")

	// Capture stderr via term package
	var stderrBuf bytes.Buffer
	term.SetErrOutput(&stderrBuf)
	defer term.Reset()

	opts := StartOptions{
		ProjectPath: "/path/to/project",
		ProjectName: "testproject",
		BranchName:  "main",
	}

	_, _, err := Start(opts,
		WithManager(mockMgr),
		WithGuardian(mockGuard),
		WithConfigLoader(mockCfgLoader),
		WithAgent(mockAgt),
	)
	if err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// Verify deprecation warning was printed
	stderrOutput := stderrBuf.String()
	if !strings.Contains(stderrOutput, "Warning: Using ANTHROPIC_API_KEY from environment. Run 'cloister setup claude' to store credentials in config.") {
		t.Errorf("stderr should contain deprecation warning about ANTHROPIC_API_KEY, got: %q", stderrOutput)
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
	mockAgt := &mockAgent{
		name:        "claude",
		setupResult: &agent.SetupResult{EnvVars: map[string]string{}},
	}

	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	// Set OAUTH token only (not API key)
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "host-oauth-token")

	// Capture stderr via term package
	var stderrBuf bytes.Buffer
	term.SetErrOutput(&stderrBuf)
	defer term.Reset()

	opts := StartOptions{
		ProjectPath: "/path/to/project",
		ProjectName: "testproject",
		BranchName:  "main",
	}

	_, _, err := Start(opts,
		WithManager(mockMgr),
		WithGuardian(mockGuard),
		WithConfigLoader(mockCfgLoader),
		WithAgent(mockAgt),
	)
	if err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// Verify deprecation warning was printed for OAuth token
	stderrOutput := stderrBuf.String()
	if !strings.Contains(stderrOutput, "Warning: Using CLAUDE_CODE_OAUTH_TOKEN from environment. Run 'cloister setup claude' to store credentials in config.") {
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
	mockAgt := &mockAgent{
		name:        "claude",
		setupResult: &agent.SetupResult{EnvVars: map[string]string{}},
	}

	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
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
		WithAgent(mockAgt),
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
	mockAgt := &mockAgent{
		name: "claude",
		setupResult: &agent.SetupResult{
			EnvVars: map[string]string{
				"ANTHROPIC_API_KEY": "config-api-key",
			},
		},
	}

	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("XDG_CONFIG_HOME", tempDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())
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
		WithAgent(mockAgt),
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

// TestStart_AgentReceivesContainerName verifies agent receives correct container name.
func TestStart_AgentReceivesContainerName(t *testing.T) {
	mockMgr := &mockManager{createResult: "container-123"}
	mockGuard := &mockGuardian{}
	mockCfgLoader := &mockConfigLoader{
		config: &config.GlobalConfig{
			Agents: map[string]config.AgentConfig{
				"claude": {
					AuthMethod: "token",
					Token:      "test-token",
				},
			},
		},
	}
	mockAgt := &mockAgent{
		name:        "claude",
		setupResult: &agent.SetupResult{EnvVars: map[string]string{}},
	}

	t.Setenv("HOME", t.TempDir())
	testutil.IsolateXDGDirs(t)

	opts := StartOptions{
		ProjectPath: "/path/to/project",
		ProjectName: "testproject",
		BranchName:  "main",
	}

	_, _, err := Start(opts,
		WithManager(mockMgr),
		WithGuardian(mockGuard),
		WithConfigLoader(mockCfgLoader),
		WithAgent(mockAgt),
	)
	if err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// Verify agent received the container name
	if !strings.HasPrefix(mockAgt.containerName, "cloister-") {
		t.Errorf("agent.containerName = %q, expected prefix 'cloister-'", mockAgt.containerName)
	}
}

// TestStart_ExistingContainer_ReturnsEarlyWithoutTokenMutation verifies that
// Start() returns ErrContainerExists immediately when the container already
// exists, without generating a token, writing to disk, or registering with
// the guardian.
func TestStart_ExistingContainer_ReturnsEarlyWithoutTokenMutation(t *testing.T) {
	testutil.IsolateXDGDirs(t)

	mockMgr := &mockManager{
		containerExistsResult: true, // container already exists
		createResult:          "should-not-be-called",
	}
	mockGuard := &mockGuardian{}
	mockAgt := &mockAgent{
		name:        "claude",
		setupResult: &agent.SetupResult{},
	}

	opts := StartOptions{
		ProjectPath: "/path/to/project",
		ProjectName: "testproject",
		BranchName:  "main",
	}

	_, _, err := Start(opts,
		WithManager(mockMgr),
		WithGuardian(mockGuard),
		WithAgent(mockAgt),
	)

	// Should get ErrContainerExists
	if !errors.Is(err, container.ErrContainerExists) {
		t.Fatalf("Start() error = %v, want ErrContainerExists", err)
	}

	// Create should never have been called
	if mockMgr.createCalled {
		t.Error("manager.Create() was called; Start() should return before reaching Create")
	}

	// Agent setup should never have been called
	if mockAgt.setupCalled {
		t.Error("agent.Setup() was called; Start() should return before reaching Setup")
	}

	// Token store should be empty (no token was persisted)
	store, err := getTokenStore()
	if err != nil {
		t.Fatalf("getTokenStore() failed: %v", err)
	}
	tokens, err := store.Load()
	if err != nil {
		t.Fatalf("store.Load() failed: %v", err)
	}
	if len(tokens) != 0 {
		t.Errorf("Token store should be empty, but has %d entries: %v", len(tokens), tokens)
	}
}
