package token

import "os"

// credentialEnvVarNames lists the host environment variables to pass through
// to containers for AI agent authentication.
//
// TEMPORARY: This direct credential passthrough is a Phase 1 workaround.
// In Phase 3, credentials will be managed via `cloister setup claude` and
// stored in the cloister config, removing the dependency on host env vars.
var credentialEnvVarNames = []string{
	"ANTHROPIC_API_KEY",
	"CLAUDE_CODE_OAUTH_TOKEN",
}

// CredentialEnvVars returns environment variables for AI agent credentials
// that are set on the host system. Only non-empty values are included.
//
// TEMPORARY: This is a Phase 1 workaround for credential injection.
// In Phase 3, this will be replaced by credential management via
// `cloister setup claude` which stores credentials in the cloister config.
//
// Currently passes through:
//   - ANTHROPIC_API_KEY: API key for Anthropic API access
//   - CLAUDE_CODE_OAUTH_TOKEN: OAuth token for Claude Code authentication
func CredentialEnvVars() []string {
	var envVars []string
	for _, name := range credentialEnvVarNames {
		if value := os.Getenv(name); value != "" {
			envVars = append(envVars, name+"="+value)
		}
	}
	return envVars
}
