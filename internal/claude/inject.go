package claude

import (
	"errors"
	"fmt"
	"os"

	"github.com/xdg/cloister/internal/config"
)

// Container path where credentials file should be written.
const ContainerCredentialsPath = "/home/cloister/.claude/.credentials.json"

// Environment variable names for credential injection.
const (
	EnvClaudeOAuthToken = "CLAUDE_CODE_OAUTH_TOKEN"
	EnvAnthropicAPIKey  = "ANTHROPIC_API_KEY"
)

// Auth method constants matching config.AgentConfig.AuthMethod values.
const (
	AuthMethodExisting = "existing"
	AuthMethodToken    = "token"
	AuthMethodAPIKey   = "api_key"
)

// ErrNoAuthMethod indicates that no authentication method is configured.
var ErrNoAuthMethod = errors.New("no authentication method configured for Claude: run `cloister setup claude`")

// ErrMissingToken indicates that auth_method is "token" but no token is provided.
var ErrMissingToken = errors.New("auth_method is 'token' but no token configured")

// ErrMissingAPIKey indicates that auth_method is "api_key" but no API key is provided.
var ErrMissingAPIKey = errors.New("auth_method is 'api_key' but no API key configured")

// ErrInvalidAuthMethod indicates an unrecognized auth_method value.
var ErrInvalidAuthMethod = errors.New("invalid auth_method")

// InjectionConfig contains the configuration for injecting Claude credentials
// into a container. This includes environment variables to set and files to create.
type InjectionConfig struct {
	// EnvVars contains environment variables to set in the container.
	// Keys are variable names, values are the variable values.
	EnvVars map[string]string

	// Files contains files to create in the container.
	// Keys are absolute paths inside the container, values are file contents.
	Files map[string]string
}

// Injector handles credential injection for Claude Code.
type Injector struct {
	// Extractor extracts credentials for "existing" auth method.
	// If nil, NewExtractor() will be called.
	Extractor *Extractor

	// FileReader reads files from the filesystem.
	// If nil, os.ReadFile is used.
	FileReader func(path string) ([]byte, error)
}

// NewInjector creates a new Injector with production implementations.
func NewInjector() *Injector {
	return &Injector{
		Extractor:  NewExtractor(),
		FileReader: os.ReadFile,
	}
}

// InjectCredentials generates an InjectionConfig based on the agent configuration.
// The auth_method field determines how credentials are injected:
//   - "existing": Extract credentials from host (Keychain on macOS, file on Linux)
//     and write to container as .credentials.json file
//   - "token": Set CLAUDE_CODE_OAUTH_TOKEN environment variable
//   - "api_key": Set ANTHROPIC_API_KEY environment variable
//
// Returns an error if auth_method is not set or if required credentials are missing.
func (i *Injector) InjectCredentials(cfg *config.AgentConfig) (*InjectionConfig, error) {
	if cfg == nil || cfg.AuthMethod == "" {
		return nil, ErrNoAuthMethod
	}

	result := &InjectionConfig{
		EnvVars: make(map[string]string),
		Files:   make(map[string]string),
	}

	switch cfg.AuthMethod {
	case AuthMethodExisting:
		return i.injectExisting(result)
	case AuthMethodToken:
		return i.injectToken(cfg, result)
	case AuthMethodAPIKey:
		return i.injectAPIKey(cfg, result)
	default:
		return nil, fmt.Errorf("%w: %q", ErrInvalidAuthMethod, cfg.AuthMethod)
	}
}

// injectExisting handles the "existing" auth method by extracting credentials
// from the host system and configuring them to be written as a file in the container.
func (i *Injector) injectExisting(result *InjectionConfig) (*InjectionConfig, error) {
	extractor := i.Extractor
	if extractor == nil {
		extractor = NewExtractor()
	}

	creds, err := extractor.Extract()
	if err != nil {
		return nil, fmt.Errorf("failed to extract credentials: %w", err)
	}

	// Get the credential JSON content
	var credJSON string
	if creds.JSON != "" {
		// macOS: JSON was extracted from Keychain
		credJSON = creds.JSON
	} else if creds.FilePath != "" {
		// Linux: Read from file path
		reader := i.FileReader
		if reader == nil {
			reader = os.ReadFile
		}
		content, err := reader(creds.FilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read credentials file %s: %w", creds.FilePath, err)
		}
		credJSON = string(content)
	} else {
		return nil, errors.New("extracted credentials have neither JSON nor FilePath set")
	}

	result.Files[ContainerCredentialsPath] = credJSON
	return result, nil
}

// injectToken handles the "token" auth method by setting the OAuth token
// as an environment variable.
func (i *Injector) injectToken(cfg *config.AgentConfig, result *InjectionConfig) (*InjectionConfig, error) {
	if cfg.Token == "" {
		return nil, ErrMissingToken
	}

	result.EnvVars[EnvClaudeOAuthToken] = cfg.Token
	return result, nil
}

// injectAPIKey handles the "api_key" auth method by setting the API key
// as an environment variable.
func (i *Injector) injectAPIKey(cfg *config.AgentConfig, result *InjectionConfig) (*InjectionConfig, error) {
	if cfg.APIKey == "" {
		return nil, ErrMissingAPIKey
	}

	result.EnvVars[EnvAnthropicAPIKey] = cfg.APIKey
	return result, nil
}
