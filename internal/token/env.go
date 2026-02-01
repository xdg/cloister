package token

import (
	"fmt"
	"os"
)

// DefaultGuardianHost is the default hostname for the guardian container
// on the cloister-net Docker network.
const DefaultGuardianHost = "cloister-guardian"

// InstanceIDEnvVar matches guardian.InstanceIDEnvVar for test isolation.
// We duplicate it here to avoid an import cycle.
// A consistency test in testutil verifies these constants stay in sync.
const InstanceIDEnvVar = "CLOISTER_INSTANCE_ID"

// GuardianHost returns the guardian container hostname for the current instance.
// For production (no instance ID), returns "cloister-guardian".
// For test instances, returns "cloister-guardian-<id>".
func GuardianHost() string {
	if id := os.Getenv(InstanceIDEnvVar); id != "" {
		return "cloister-guardian-" + id
	}
	return DefaultGuardianHost
}

// DefaultProxyPort is the default port for the guardian proxy server.
const DefaultProxyPort = 3128

// ProxyEnvVars returns environment variables for configuring a container
// to use the guardian proxy with the given token for authentication.
//
// The returned slice contains:
//   - CLOISTER_TOKEN: the authentication token
//   - CLOISTER_GUARDIAN_HOST: the guardian hostname (for hostexec and other tools)
//   - HTTP_PROXY: proxy URL with embedded credentials
//   - HTTPS_PROXY: same proxy URL (for tools that check HTTPS_PROXY)
//   - http_proxy: lowercase variant for compatibility
//   - https_proxy: lowercase variant for compatibility
//   - NO_PROXY: hosts that bypass the proxy (guardian, localhost)
//   - no_proxy: lowercase variant for compatibility
//
// The proxy URL format is: http://token:$token@$host:$port
// Using "token" as the username and the actual token as the password.
func ProxyEnvVars(token, guardianHost string) []string {
	if guardianHost == "" {
		guardianHost = GuardianHost()
	}

	proxyURL := fmt.Sprintf("http://token:%s@%s:%d", token, guardianHost, DefaultProxyPort)
	noProxy := fmt.Sprintf("%s,localhost,127.0.0.1", guardianHost)

	return []string{
		"CLOISTER_TOKEN=" + token,
		"CLOISTER_GUARDIAN_HOST=" + guardianHost,
		"HTTP_PROXY=" + proxyURL,
		"HTTPS_PROXY=" + proxyURL,
		"http_proxy=" + proxyURL,
		"https_proxy=" + proxyURL,
		"NO_PROXY=" + noProxy,
		"no_proxy=" + noProxy,
	}
}
