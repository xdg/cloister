package cloister

import (
	"os"
	"testing"
)

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

	envVars := credentialEnvVars()

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

	envVars := credentialEnvVars()

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

	envVars := credentialEnvVars()

	if len(envVars) != 0 {
		t.Errorf("expected empty slice, got %v", envVars)
	}
}

// getEnvAndUnset gets an environment variable and unsets it, returning the original value.
func getEnvAndUnset(t *testing.T, key string) string {
	t.Helper()
	value, _ := os.LookupEnv(key)
	_ = os.Unsetenv(key)
	return value
}

// restoreEnv restores an environment variable to its original value.
// If origValue is empty, the variable is unset.
func restoreEnv(t *testing.T, key, origValue string) {
	t.Helper()
	if origValue == "" {
		_ = os.Unsetenv(key)
	} else {
		_ = os.Setenv(key, origValue)
	}
}

func TestCredentialEnvVarsUsed_ReturnsNames(t *testing.T) {
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

	names := credentialEnvVarsUsed()

	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d: %v", len(names), names)
	}

	// Check names (not values)
	var foundAPIKey, foundOAuth bool
	for _, name := range names {
		if name == "ANTHROPIC_API_KEY" {
			foundAPIKey = true
		}
		if name == "CLAUDE_CODE_OAUTH_TOKEN" {
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

func TestCredentialEnvVarsUsed_SkipsEmptyVars(t *testing.T) {
	// Save and clear any existing values
	origAPIKey := getEnvAndUnset(t, "ANTHROPIC_API_KEY")
	origOAuthToken := getEnvAndUnset(t, "CLAUDE_CODE_OAUTH_TOKEN")
	defer func() {
		restoreEnv(t, "ANTHROPIC_API_KEY", origAPIKey)
		restoreEnv(t, "CLAUDE_CODE_OAUTH_TOKEN", origOAuthToken)
	}()

	// Set only one value
	t.Setenv("ANTHROPIC_API_KEY", "test-api-key")

	names := credentialEnvVarsUsed()

	if len(names) != 1 {
		t.Fatalf("expected 1 name, got %d: %v", len(names), names)
	}

	if names[0] != "ANTHROPIC_API_KEY" {
		t.Errorf("expected ANTHROPIC_API_KEY, got %s", names[0])
	}
}

func TestCredentialEnvVarsUsed_EmptyWhenNoneSet(t *testing.T) {
	// Save and clear any existing values
	origAPIKey := getEnvAndUnset(t, "ANTHROPIC_API_KEY")
	origOAuthToken := getEnvAndUnset(t, "CLAUDE_CODE_OAUTH_TOKEN")
	defer func() {
		restoreEnv(t, "ANTHROPIC_API_KEY", origAPIKey)
		restoreEnv(t, "CLAUDE_CODE_OAUTH_TOKEN", origOAuthToken)
	}()

	names := credentialEnvVarsUsed()

	if len(names) != 0 {
		t.Errorf("expected empty slice, got %v", names)
	}
}
