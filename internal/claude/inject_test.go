package claude

import (
	"errors"
	"testing"

	"github.com/xdg/cloister/internal/config"
)

func TestInjector_InjectCredentials_Token(t *testing.T) {
	injector := NewInjector()

	cfg := &config.AgentConfig{
		AuthMethod: "token",
		Token:      "sk-ant-oat01-test-token",
	}

	result, err := injector.InjectCredentials(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have env var set
	if len(result.EnvVars) != 1 {
		t.Errorf("expected 1 env var, got %d", len(result.EnvVars))
	}
	if result.EnvVars[EnvClaudeOAuthToken] != "sk-ant-oat01-test-token" {
		t.Errorf("expected token %q, got %q", "sk-ant-oat01-test-token", result.EnvVars[EnvClaudeOAuthToken])
	}

	// Should have no files
	if len(result.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(result.Files))
	}
}

func TestInjector_InjectCredentials_APIKey(t *testing.T) {
	injector := NewInjector()

	cfg := &config.AgentConfig{
		AuthMethod: "api_key",
		APIKey:     "sk-ant-api01-test-key",
	}

	result, err := injector.InjectCredentials(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have env var set
	if len(result.EnvVars) != 1 {
		t.Errorf("expected 1 env var, got %d", len(result.EnvVars))
	}
	if result.EnvVars[EnvAnthropicAPIKey] != "sk-ant-api01-test-key" {
		t.Errorf("expected API key %q, got %q", "sk-ant-api01-test-key", result.EnvVars[EnvAnthropicAPIKey])
	}

	// Should have no files
	if len(result.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(result.Files))
	}
}

func TestInjector_InjectCredentials_Existing_MacOS(t *testing.T) {
	credJSON := `{"claudeAiOauth":{"accessToken":"test-access","refreshToken":"test-refresh"}}`

	injector := &Injector{
		Extractor: &Extractor{
			CommandRunner: &MockCommandRunner{
				Output: credJSON,
			},
			FileChecker: &MockFileChecker{},
			UserLookup: &MockUserLookup{
				Username: "testuser",
			},
			Platform: "darwin",
		},
	}

	cfg := &config.AgentConfig{
		AuthMethod: "existing",
	}

	result, err := injector.InjectCredentials(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have no env vars
	if len(result.EnvVars) != 0 {
		t.Errorf("expected 0 env vars, got %d", len(result.EnvVars))
	}

	// Should have credentials file
	if len(result.Files) != 1 {
		t.Errorf("expected 1 file, got %d", len(result.Files))
	}
	if result.Files[ContainerCredentialsPath] != credJSON {
		t.Errorf("expected file content %q, got %q", credJSON, result.Files[ContainerCredentialsPath])
	}
}

func TestInjector_InjectCredentials_Existing_Linux(t *testing.T) {
	credJSON := `{"claudeAiOauth":{"accessToken":"linux-access","refreshToken":"linux-refresh"}}`
	credFilePath := "/home/testuser/.claude/.credentials.json"

	injector := &Injector{
		Extractor: &Extractor{
			CommandRunner: &MockCommandRunner{},
			FileChecker: &MockFileChecker{
				ExistingPaths: map[string]bool{
					credFilePath: true,
				},
			},
			UserLookup: &MockUserLookup{
				Home: "/home/testuser",
			},
			Platform: "linux",
		},
		FileReader: func(path string) ([]byte, error) {
			if path == credFilePath {
				return []byte(credJSON), nil
			}
			return nil, errors.New("file not found")
		},
	}

	cfg := &config.AgentConfig{
		AuthMethod: "existing",
	}

	result, err := injector.InjectCredentials(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have no env vars
	if len(result.EnvVars) != 0 {
		t.Errorf("expected 0 env vars, got %d", len(result.EnvVars))
	}

	// Should have credentials file with content read from host
	if len(result.Files) != 1 {
		t.Errorf("expected 1 file, got %d", len(result.Files))
	}
	if result.Files[ContainerCredentialsPath] != credJSON {
		t.Errorf("expected file content %q, got %q", credJSON, result.Files[ContainerCredentialsPath])
	}
}

func TestInjector_InjectCredentials_NoAuthMethod(t *testing.T) {
	injector := NewInjector()

	// Test with nil config
	_, err := injector.InjectCredentials(nil)
	if !errors.Is(err, ErrNoAuthMethod) {
		t.Errorf("expected ErrNoAuthMethod for nil config, got %v", err)
	}

	// Test with empty auth_method
	cfg := &config.AgentConfig{}
	_, err = injector.InjectCredentials(cfg)
	if !errors.Is(err, ErrNoAuthMethod) {
		t.Errorf("expected ErrNoAuthMethod for empty auth_method, got %v", err)
	}
}

func TestInjector_InjectCredentials_InvalidAuthMethod(t *testing.T) {
	injector := NewInjector()

	cfg := &config.AgentConfig{
		AuthMethod: "invalid_method",
	}

	_, err := injector.InjectCredentials(cfg)
	if !errors.Is(err, ErrInvalidAuthMethod) {
		t.Errorf("expected ErrInvalidAuthMethod, got %v", err)
	}
}

func TestInjector_InjectCredentials_MissingToken(t *testing.T) {
	injector := NewInjector()

	cfg := &config.AgentConfig{
		AuthMethod: "token",
		Token:      "", // missing
	}

	_, err := injector.InjectCredentials(cfg)
	if !errors.Is(err, ErrMissingToken) {
		t.Errorf("expected ErrMissingToken, got %v", err)
	}
}

func TestInjector_InjectCredentials_MissingAPIKey(t *testing.T) {
	injector := NewInjector()

	cfg := &config.AgentConfig{
		AuthMethod: "api_key",
		APIKey:     "", // missing
	}

	_, err := injector.InjectCredentials(cfg)
	if !errors.Is(err, ErrMissingAPIKey) {
		t.Errorf("expected ErrMissingAPIKey, got %v", err)
	}
}

func TestInjector_InjectCredentials_Existing_ExtractError(t *testing.T) {
	injector := &Injector{
		Extractor: &Extractor{
			CommandRunner: &MockCommandRunner{
				Err: errors.New("keychain error"),
			},
			FileChecker: &MockFileChecker{},
			UserLookup: &MockUserLookup{
				Username: "testuser",
			},
			Platform: "darwin",
		},
	}

	cfg := &config.AgentConfig{
		AuthMethod: "existing",
	}

	_, err := injector.InjectCredentials(cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !containsString(err.Error(), "failed to extract credentials") {
		t.Errorf("expected 'failed to extract credentials' in error, got %v", err)
	}
}

func TestInjector_InjectCredentials_Existing_FileReadError(t *testing.T) {
	credFilePath := "/home/testuser/.claude/.credentials.json"

	injector := &Injector{
		Extractor: &Extractor{
			CommandRunner: &MockCommandRunner{},
			FileChecker: &MockFileChecker{
				ExistingPaths: map[string]bool{
					credFilePath: true,
				},
			},
			UserLookup: &MockUserLookup{
				Home: "/home/testuser",
			},
			Platform: "linux",
		},
		FileReader: func(path string) ([]byte, error) {
			return nil, errors.New("permission denied")
		},
	}

	cfg := &config.AgentConfig{
		AuthMethod: "existing",
	}

	_, err := injector.InjectCredentials(cfg)
	if err == nil {
		t.Fatal("expected error")
	}
	if !containsString(err.Error(), "failed to read credentials file") {
		t.Errorf("expected 'failed to read credentials file' in error, got %v", err)
	}
}

func TestNewInjector(t *testing.T) {
	injector := NewInjector()

	if injector == nil {
		t.Fatal("NewInjector returned nil")
	}
	if injector.Extractor == nil {
		t.Error("Extractor should not be nil")
	}
	if injector.FileReader == nil {
		t.Error("FileReader should not be nil")
	}
}

func TestInjectionConfig_Empty(t *testing.T) {
	// Test that an empty InjectionConfig is usable
	cfg := &InjectionConfig{
		EnvVars: make(map[string]string),
		Files:   make(map[string]string),
	}

	if len(cfg.EnvVars) != 0 {
		t.Errorf("expected empty EnvVars, got %d entries", len(cfg.EnvVars))
	}
	if len(cfg.Files) != 0 {
		t.Errorf("expected empty Files, got %d entries", len(cfg.Files))
	}
}

func TestConstants(t *testing.T) {
	// Verify constants have expected values
	if ContainerCredentialsPath != "/home/cloister/.claude/.credentials.json" {
		t.Errorf("unexpected ContainerCredentialsPath: %q", ContainerCredentialsPath)
	}
	if EnvClaudeOAuthToken != "CLAUDE_CODE_OAUTH_TOKEN" {
		t.Errorf("unexpected EnvClaudeOAuthToken: %q", EnvClaudeOAuthToken)
	}
	if EnvAnthropicAPIKey != "ANTHROPIC_API_KEY" {
		t.Errorf("unexpected EnvAnthropicAPIKey: %q", EnvAnthropicAPIKey)
	}
	if AuthMethodExisting != "existing" {
		t.Errorf("unexpected AuthMethodExisting: %q", AuthMethodExisting)
	}
	if AuthMethodToken != "token" {
		t.Errorf("unexpected AuthMethodToken: %q", AuthMethodToken)
	}
	if AuthMethodAPIKey != "api_key" {
		t.Errorf("unexpected AuthMethodAPIKey: %q", AuthMethodAPIKey)
	}
}

// TestInjector_InjectCredentials_Existing_MacOS_NotFound verifies that when macOS
// keychain extraction fails, the error message suggests both recovery options.
func TestInjector_InjectCredentials_Existing_MacOS_NotFound(t *testing.T) {
	injector := &Injector{
		Extractor: &Extractor{
			CommandRunner: &MockCommandRunner{
				// Simulates keychain entry not found
				Err: errors.New("security: SecKeychainSearchCopyNext: The specified item could not be found"),
			},
			FileChecker: &MockFileChecker{},
			UserLookup: &MockUserLookup{
				Username: "testuser",
			},
			Platform: "darwin",
		},
	}

	cfg := &config.AgentConfig{
		AuthMethod: "existing",
	}

	_, err := injector.InjectCredentials(cfg)
	if err == nil {
		t.Fatal("expected error for missing keychain credentials")
	}

	errMsg := err.Error()
	// Error should mention both recovery options
	if !containsString(errMsg, "claude login") {
		t.Errorf("expected error message to suggest 'claude login', got %q", errMsg)
	}
	if !containsString(errMsg, "cloister setup claude") {
		t.Errorf("expected error message to suggest 'cloister setup claude', got %q", errMsg)
	}
}

// TestInjector_InjectCredentials_Existing_Linux_NotFound verifies that when Linux
// credentials file is missing, the error message suggests both recovery options.
func TestInjector_InjectCredentials_Existing_Linux_NotFound(t *testing.T) {
	injector := &Injector{
		Extractor: &Extractor{
			CommandRunner: &MockCommandRunner{},
			FileChecker: &MockFileChecker{
				// Empty map means no files exist
				ExistingPaths: map[string]bool{},
			},
			UserLookup: &MockUserLookup{
				Home: "/home/testuser",
			},
			Platform: "linux",
		},
	}

	cfg := &config.AgentConfig{
		AuthMethod: "existing",
	}

	_, err := injector.InjectCredentials(cfg)
	if err == nil {
		t.Fatal("expected error for missing credentials file")
	}

	errMsg := err.Error()
	// Error should mention both recovery options
	if !containsString(errMsg, "claude login") {
		t.Errorf("expected error message to suggest 'claude login', got %q", errMsg)
	}
	if !containsString(errMsg, "cloister setup claude") {
		t.Errorf("expected error message to suggest 'cloister setup claude', got %q", errMsg)
	}
}
