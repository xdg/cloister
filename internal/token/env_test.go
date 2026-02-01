package token

import (
	"strings"
	"testing"
)

func TestProxyEnvVars_ContainsAllVariables(t *testing.T) {
	token := "abc123"
	envVars := ProxyEnvVars(token, "guardian")

	expected := []string{
		"CLOISTER_TOKEN",
		"CLOISTER_GUARDIAN_HOST",
		"HTTP_PROXY",
		"HTTPS_PROXY",
		"http_proxy",
		"https_proxy",
		"NO_PROXY",
		"no_proxy",
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

func TestProxyEnvVars_GuardianHostEnvVar(t *testing.T) {
	t.Run("default host", func(t *testing.T) {
		envVars := ProxyEnvVars("token123", "")

		var guardianHost string
		for _, env := range envVars {
			if strings.HasPrefix(env, "CLOISTER_GUARDIAN_HOST=") {
				guardianHost = strings.TrimPrefix(env, "CLOISTER_GUARDIAN_HOST=")
				break
			}
		}

		if guardianHost != DefaultGuardianHost {
			t.Errorf("CLOISTER_GUARDIAN_HOST = %q, want %q", guardianHost, DefaultGuardianHost)
		}
	})

	t.Run("custom host", func(t *testing.T) {
		customHost := "custom-host"
		envVars := ProxyEnvVars("token123", customHost)

		var guardianHost string
		for _, env := range envVars {
			if strings.HasPrefix(env, "CLOISTER_GUARDIAN_HOST=") {
				guardianHost = strings.TrimPrefix(env, "CLOISTER_GUARDIAN_HOST=")
				break
			}
		}

		if guardianHost != customHost {
			t.Errorf("CLOISTER_GUARDIAN_HOST = %q, want %q", guardianHost, customHost)
		}
	})
}

func TestProxyEnvVars_NoProxyValue(t *testing.T) {
	t.Run("default host", func(t *testing.T) {
		envVars := ProxyEnvVars("token123", "")
		expected := "cloister-guardian,localhost,127.0.0.1"

		for _, varName := range []string{"NO_PROXY", "no_proxy"} {
			var value string
			for _, env := range envVars {
				if strings.HasPrefix(env, varName+"=") {
					value = strings.TrimPrefix(env, varName+"=")
					break
				}
			}
			if value != expected {
				t.Errorf("%s = %q, want %q", varName, value, expected)
			}
		}
	})

	t.Run("custom host", func(t *testing.T) {
		envVars := ProxyEnvVars("token123", "custom-guardian")
		expected := "custom-guardian,localhost,127.0.0.1"

		var noProxy string
		for _, env := range envVars {
			if strings.HasPrefix(env, "NO_PROXY=") {
				noProxy = strings.TrimPrefix(env, "NO_PROXY=")
				break
			}
		}
		if noProxy != expected {
			t.Errorf("NO_PROXY = %q, want %q", noProxy, expected)
		}
	})
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

func TestGuardianHost_Production(t *testing.T) {
	t.Setenv(InstanceIDEnvVar, "")

	if got := GuardianHost(); got != "cloister-guardian" {
		t.Errorf("GuardianHost() = %q, want %q", got, "cloister-guardian")
	}
}

func TestGuardianHost_TestInstance(t *testing.T) {
	t.Setenv(InstanceIDEnvVar, "abc123")

	if got := GuardianHost(); got != "cloister-guardian-abc123" {
		t.Errorf("GuardianHost() = %q, want %q", got, "cloister-guardian-abc123")
	}
}
