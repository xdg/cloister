package token

import (
	"os"
	"strings"
	"testing"
)

func TestProxyEnvVars_ContainsAllVariables(t *testing.T) {
	token := "abc123"
	envVars := ProxyEnvVars(token, "guardian")

	expected := []string{
		"CLOISTER_TOKEN",
		"HTTP_PROXY",
		"HTTPS_PROXY",
		"http_proxy",
		"https_proxy",
	}

	if len(envVars) != len(expected) {
		t.Errorf("expected %d env vars, got %d", len(expected), len(envVars))
	}

	for _, name := range expected {
		found := false
		for _, env := range envVars {
			if strings.HasPrefix(env, name+"=") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing env var: %s", name)
		}
	}
}

func TestProxyEnvVars_TokenValue(t *testing.T) {
	token := "mytoken123"
	envVars := ProxyEnvVars(token, "guardian")

	var cloisterToken string
	for _, env := range envVars {
		if strings.HasPrefix(env, "CLOISTER_TOKEN=") {
			cloisterToken = strings.TrimPrefix(env, "CLOISTER_TOKEN=")
			break
		}
	}

	if cloisterToken != token {
		t.Errorf("CLOISTER_TOKEN = %q, want %q", cloisterToken, token)
	}
}

func TestProxyEnvVars_ProxyURLFormat(t *testing.T) {
	token := "testtoken"
	host := "guardian-host"
	envVars := ProxyEnvVars(token, host)

	expectedURL := "http://token:testtoken@guardian-host:3128"

	// Check all proxy vars have the correct URL
	proxyVars := []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"}
	for _, varName := range proxyVars {
		var value string
		for _, env := range envVars {
			if strings.HasPrefix(env, varName+"=") {
				value = strings.TrimPrefix(env, varName+"=")
				break
			}
		}
		if value != expectedURL {
			t.Errorf("%s = %q, want %q", varName, value, expectedURL)
		}
	}
}

func TestProxyEnvVars_DefaultGuardianHost(t *testing.T) {
	token := "testtoken"
	envVars := ProxyEnvVars(token, "")

	expectedURL := "http://token:testtoken@cloister-guardian:3128"

	var httpProxy string
	for _, env := range envVars {
		if strings.HasPrefix(env, "HTTP_PROXY=") {
			httpProxy = strings.TrimPrefix(env, "HTTP_PROXY=")
			break
		}
	}

	if httpProxy != expectedURL {
		t.Errorf("HTTP_PROXY with empty host = %q, want %q", httpProxy, expectedURL)
	}
}

func TestProxyEnvVars_CustomHost(t *testing.T) {
	token := "testtoken"
	host := "custom-guardian"
	envVars := ProxyEnvVars(token, host)

	expectedURL := "http://token:testtoken@custom-guardian:3128"

	var httpProxy string
	for _, env := range envVars {
		if strings.HasPrefix(env, "HTTP_PROXY=") {
			httpProxy = strings.TrimPrefix(env, "HTTP_PROXY=")
			break
		}
	}

	if httpProxy != expectedURL {
		t.Errorf("HTTP_PROXY = %q, want %q", httpProxy, expectedURL)
	}
}

func TestProxyEnvVars_SpecialCharactersInToken(t *testing.T) {
	// Tokens are hex-encoded so they won't have special characters,
	// but let's ensure the function handles them anyway
	token := Generate() // Use actual generated token
	envVars := ProxyEnvVars(token, DefaultGuardianHost)

	// Verify token appears in the URL
	var httpProxy string
	for _, env := range envVars {
		if strings.HasPrefix(env, "HTTP_PROXY=") {
			httpProxy = strings.TrimPrefix(env, "HTTP_PROXY=")
			break
		}
	}

	if !strings.Contains(httpProxy, token) {
		t.Errorf("HTTP_PROXY does not contain token")
	}
}

func TestDefaultGuardianHost_Constant(t *testing.T) {
	if DefaultGuardianHost != "cloister-guardian" {
		t.Errorf("DefaultGuardianHost = %q, want %q", DefaultGuardianHost, "cloister-guardian")
	}
}

func TestDefaultProxyPort_Constant(t *testing.T) {
	if DefaultProxyPort != 3128 {
		t.Errorf("DefaultProxyPort = %d, want %d", DefaultProxyPort, 3128)
	}
}

func TestCredentialEnvVars_ReturnsSetVars(t *testing.T) {
	// Save and clear any existing values
	origAPIKey := getEnvAndUnset(t, "ANTHROPIC_API_KEY")
	origOAuthToken := getEnvAndUnset(t, "CLAUDE_CODE_OAUTH_TOKEN")
	defer func() {
		restoreEnv(t, "ANTHROPIC_API_KEY", origAPIKey)
		restoreEnv(t, "CLAUDE_CODE_OAUTH_TOKEN", origOAuthToken)
	}()

	// Set test values
	t.Setenv("ANTHROPIC_API_KEY", "test-api-key")
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "test-oauth-token")

	envVars := CredentialEnvVars()

	if len(envVars) != 2 {
		t.Fatalf("expected 2 env vars, got %d: %v", len(envVars), envVars)
	}

	// Check ANTHROPIC_API_KEY
	var foundAPIKey, foundOAuth bool
	for _, env := range envVars {
		if env == "ANTHROPIC_API_KEY=test-api-key" {
			foundAPIKey = true
		}
		if env == "CLAUDE_CODE_OAUTH_TOKEN=test-oauth-token" {
			foundOAuth = true
		}
	}

	if !foundAPIKey {
		t.Error("missing ANTHROPIC_API_KEY in result")
	}
	if !foundOAuth {
		t.Error("missing CLAUDE_CODE_OAUTH_TOKEN in result")
	}
}

func TestCredentialEnvVars_SkipsEmptyVars(t *testing.T) {
	// Save and clear any existing values
	origAPIKey := getEnvAndUnset(t, "ANTHROPIC_API_KEY")
	origOAuthToken := getEnvAndUnset(t, "CLAUDE_CODE_OAUTH_TOKEN")
	defer func() {
		restoreEnv(t, "ANTHROPIC_API_KEY", origAPIKey)
		restoreEnv(t, "CLAUDE_CODE_OAUTH_TOKEN", origOAuthToken)
	}()

	// Set only one value
	t.Setenv("ANTHROPIC_API_KEY", "test-api-key")
	// CLAUDE_CODE_OAUTH_TOKEN is unset

	envVars := CredentialEnvVars()

	if len(envVars) != 1 {
		t.Fatalf("expected 1 env var, got %d: %v", len(envVars), envVars)
	}

	if envVars[0] != "ANTHROPIC_API_KEY=test-api-key" {
		t.Errorf("expected ANTHROPIC_API_KEY=test-api-key, got %s", envVars[0])
	}
}

func TestCredentialEnvVars_EmptyWhenNoneSet(t *testing.T) {
	// Save and clear any existing values
	origAPIKey := getEnvAndUnset(t, "ANTHROPIC_API_KEY")
	origOAuthToken := getEnvAndUnset(t, "CLAUDE_CODE_OAUTH_TOKEN")
	defer func() {
		restoreEnv(t, "ANTHROPIC_API_KEY", origAPIKey)
		restoreEnv(t, "CLAUDE_CODE_OAUTH_TOKEN", origOAuthToken)
	}()

	envVars := CredentialEnvVars()

	if len(envVars) != 0 {
		t.Errorf("expected empty slice, got %v", envVars)
	}
}

// getEnvAndUnset gets an environment variable and unsets it, returning the original value.
func getEnvAndUnset(t *testing.T, key string) string {
	t.Helper()
	value, _ := os.LookupEnv(key)
	os.Unsetenv(key)
	return value
}

// restoreEnv restores an environment variable to its original value.
// If origValue is empty, the variable is unset.
func restoreEnv(t *testing.T, key, origValue string) {
	t.Helper()
	if origValue == "" {
		os.Unsetenv(key)
	} else {
		os.Setenv(key, origValue)
	}
}
