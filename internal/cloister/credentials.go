package cloister

import "os"

// credentialEnvVarNames lists the host environment variables to pass through
// to containers for AI agent authentication.
//
// Deprecated: This direct credential passthrough is a fallback mechanism.
// Prefer config-based credential management via agent-specific setup commands
// (e.g., `cloister setup claude`, `cloister setup codex`). Host env var passthrough
// remains available for backward compatibility when no config-based credentials are set.
var credentialEnvVarNames = []string{
	// Claude credentials
	"ANTHROPIC_API_KEY",
	"CLAUDE_CODE_OAUTH_TOKEN",
	// Codex credentials
	"OPENAI_API_KEY",
}

// credentialEnvVars returns environment variables for AI agent credentials
// that are set on the host system. Only non-empty values are included.
//
// Deprecated: This is a fallback mechanism for credential injection.
// Prefer config-based credential management via agent-specific setup commands.
// This function is still used when no config-based credentials are set.
//
// Passes through:
//   - ANTHROPIC_API_KEY: API key for Anthropic API access
//   - CLAUDE_CODE_OAUTH_TOKEN: OAuth token for Claude Code authentication
//   - OPENAI_API_KEY: API key for OpenAI/Codex access
func credentialEnvVars() []string {
	var envVars []string
	for _, name := range credentialEnvVarNames {
		if value := os.Getenv(name); value != "" {
			envVars = append(envVars, name+"="+value)
		}
	}
	return envVars
}

// credentialEnvVarsUsed returns the names of credential environment variables
// that are set on the host system. This is used to generate deprecation warnings
// when falling back to host env vars instead of config-based credentials.
//
// Deprecated: This is a fallback mechanism. Prefer config-based credentials.
func credentialEnvVarsUsed() []string {
	var names []string
	for _, name := range credentialEnvVarNames {
		if value := os.Getenv(name); value != "" {
			names = append(names, name)
		}
	}
	return names
}
