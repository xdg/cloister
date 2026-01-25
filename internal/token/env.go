package token

import (
	"fmt"
	"os"
)

// DefaultGuardianHost is the default hostname for the guardian container
// on the cloister-net Docker network.
const DefaultGuardianHost = "cloister-guardian"

// DefaultProxyPort is the default port for the guardian proxy server.
const DefaultProxyPort = 3128

// ProxyEnvVars returns environment variables for configuring a container
// to use the guardian proxy with the given token for authentication.
//
// The returned slice contains:
//   - CLOISTER_TOKEN: the authentication token
//   - HTTP_PROXY: proxy URL with embedded credentials
//   - HTTPS_PROXY: same proxy URL (for tools that check HTTPS_PROXY)
//   - http_proxy: lowercase variant for compatibility
//   - https_proxy: lowercase variant for compatibility
//
// The proxy URL format is: http://token:$token@$host:$port
// Using "token" as the username and the actual token as the password.
func ProxyEnvVars(token, guardianHost string) []string {
	if guardianHost == "" {
		guardianHost = DefaultGuardianHost
	}

	proxyURL := fmt.Sprintf("http://token:%s@%s:%d", token, guardianHost, DefaultProxyPort)

	return []string{
		"CLOISTER_TOKEN=" + token,
		"HTTP_PROXY=" + proxyURL,
		"HTTPS_PROXY=" + proxyURL,
		"http_proxy=" + proxyURL,
		"https_proxy=" + proxyURL,
	}
}

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
