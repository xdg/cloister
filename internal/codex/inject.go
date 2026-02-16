// Package codex provides credential injection for Codex CLI.
package codex

import (
	"errors"

	"github.com/xdg/cloister/internal/config"
)

// EnvOpenAIAPIKey is the environment variable name for credential injection.
const EnvOpenAIAPIKey = "OPENAI_API_KEY" //nolint:gosec // G101: not a credential, just the env var name

// AuthMethodAPIKey is the auth method constant matching config.AgentConfig.AuthMethod value.
const AuthMethodAPIKey = "api_key"

// ErrNoAuthMethod indicates that no authentication method is configured.
var ErrNoAuthMethod = errors.New("no authentication method configured for Codex: run `cloister setup codex`")

// ErrMissingAPIKey indicates that auth_method is "api_key" but no API key is provided.
var ErrMissingAPIKey = errors.New("auth_method is 'api_key' but no API key configured")

// InjectionConfig contains the configuration for injecting Codex credentials
// into a container. This includes environment variables to set and files to create.
type InjectionConfig struct {
	// EnvVars contains environment variables to set in the container.
	// Keys are variable names, values are the variable values.
	EnvVars map[string]string

	// Files contains files to create in the container.
	// Keys are absolute paths inside the container, values are file contents.
	Files map[string]string
}

// Injector handles credential injection for Codex CLI.
type Injector struct{}

// NewInjector creates a new Injector.
func NewInjector() *Injector {
	return &Injector{}
}

// InjectCredentials generates an InjectionConfig based on the agent configuration.
// Codex only supports API key authentication via OPENAI_API_KEY.
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

	// Codex only supports API key authentication
	if cfg.APIKey == "" {
		return nil, ErrMissingAPIKey
	}

	result.EnvVars[EnvOpenAIAPIKey] = cfg.APIKey
	return result, nil
}
