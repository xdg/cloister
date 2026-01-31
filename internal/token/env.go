package token

import "fmt"

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
//   - CLOISTER_GUARDIAN_HOST: the guardian hostname (for hostexec and other tools)
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
		"CLOISTER_GUARDIAN_HOST=" + guardianHost,
		"HTTP_PROXY=" + proxyURL,
		"HTTPS_PROXY=" + proxyURL,
		"http_proxy=" + proxyURL,
		"https_proxy=" + proxyURL,
	}
}
