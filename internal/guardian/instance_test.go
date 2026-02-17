package guardian

import (
	"strings"
	"testing"

	"github.com/xdg/cloister/internal/executor"
)

func TestInstanceID_WhenUnset(t *testing.T) {
	t.Setenv(InstanceIDEnvVar, "")
	if got := InstanceID(); got != "" {
		t.Errorf("InstanceID() = %q, want empty", got)
	}
}

func TestInstanceID_WhenSet(t *testing.T) {
	t.Setenv(InstanceIDEnvVar, "abc123")
	if got := InstanceID(); got != "abc123" {
		t.Errorf("InstanceID() = %q, want %q", got, "abc123")
	}
}

func TestGenerateInstanceID_Returns6CharHex(t *testing.T) {
	id := GenerateInstanceID()
	if len(id) != 6 {
		t.Errorf("GenerateInstanceID() length = %d, want 6", len(id))
	}

	// Should be valid hex
	for _, c := range id {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Errorf("GenerateInstanceID() contains non-hex char %c", c)
		}
	}

	// Should be unique (generate two and compare)
	id2 := GenerateInstanceID()
	if id == id2 {
		t.Error("GenerateInstanceID() returned same value twice")
	}
}

func TestContainerName_Production(t *testing.T) {
	t.Setenv(InstanceIDEnvVar, "")
	if got := ContainerName(); got != "cloister-guardian" {
		t.Errorf("ContainerName() = %q, want %q", got, "cloister-guardian")
	}
}

func TestContainerName_TestInstance(t *testing.T) {
	t.Setenv(InstanceIDEnvVar, "abc123")
	if got := ContainerName(); got != "cloister-guardian-abc123" {
		t.Errorf("ContainerName() = %q, want %q", got, "cloister-guardian-abc123")
	}
}

func TestFindFreePort(t *testing.T) {
	port, err := FindFreePort()
	if err != nil {
		t.Fatalf("FindFreePort() error: %v", err)
	}
	if port <= 0 || port > 65535 {
		t.Errorf("FindFreePort() = %d, want valid port", port)
	}

	// Should return different ports on subsequent calls (most likely)
	port2, err := FindFreePort()
	if err != nil {
		t.Fatalf("FindFreePort() second call error: %v", err)
	}
	if port == port2 {
		// This could occasionally happen but is unlikely
		t.Log("FindFreePort() returned same port twice (unlikely but possible)")
	}
}

func TestPorts_Production(t *testing.T) {
	t.Setenv(InstanceIDEnvVar, "")
	tokenPort, approvalPort, err := Ports()
	if err != nil {
		t.Fatalf("Ports() error: %v", err)
	}
	if tokenPort != DefaultAPIPort {
		t.Errorf("tokenPort = %d, want %d", tokenPort, DefaultAPIPort)
	}
	if approvalPort != DefaultApprovalPort {
		t.Errorf("approvalPort = %d, want %d", approvalPort, DefaultApprovalPort)
	}
}

func TestPorts_TestInstance(t *testing.T) {
	t.Setenv(InstanceIDEnvVar, "abc123")
	tokenPort, approvalPort, err := Ports()
	if err != nil {
		t.Fatalf("Ports() error: %v", err)
	}

	// For test instances, ports should be dynamically allocated (not the defaults)
	// They might happen to be the defaults by chance, but that's extremely unlikely
	if tokenPort <= 0 || tokenPort > 65535 {
		t.Errorf("tokenPort = %d, want valid port", tokenPort)
	}
	if approvalPort <= 0 || approvalPort > 65535 {
		t.Errorf("approvalPort = %d, want valid port", approvalPort)
	}
	if tokenPort == approvalPort {
		t.Error("tokenPort and approvalPort should be different")
	}
}

func TestGuardianHost_MatchesContainerName(t *testing.T) {
	t.Setenv(InstanceIDEnvVar, "")
	if got := Host(); got != ContainerName() {
		t.Errorf("Host() = %q, want %q (ContainerName)", got, ContainerName())
	}

	t.Setenv(InstanceIDEnvVar, "abc123")
	if got := Host(); got != ContainerName() {
		t.Errorf("Host() = %q, want %q (ContainerName)", got, ContainerName())
	}
}

func TestProxyEnvVars_ContainsAllVariables(t *testing.T) {
	envVars := ProxyEnvVars("abc123", "guardian")

	expected := []string{
		"CLOISTER_TOKEN",
		"CLOISTER_GUARDIAN_HOST",
		"CLOISTER_REQUEST_PORT",
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

func TestProxyEnvVars_ProxyURLFormat(t *testing.T) {
	envVars := ProxyEnvVars("testtoken", "guardian-host")
	expectedURL := "http://token:testtoken@guardian-host:3128"

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
	t.Setenv(InstanceIDEnvVar, "")
	envVars := ProxyEnvVars("testtoken", "")
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

func TestProxyEnvVars_NoProxyValue(t *testing.T) {
	t.Setenv(InstanceIDEnvVar, "")
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
}

func TestProxyEnvVars_RequestPortValue(t *testing.T) {
	envVars := ProxyEnvVars("token123", "")

	var requestPort string
	for _, env := range envVars {
		if strings.HasPrefix(env, "CLOISTER_REQUEST_PORT=") {
			requestPort = strings.TrimPrefix(env, "CLOISTER_REQUEST_PORT=")
			break
		}
	}

	if requestPort != "9998" {
		t.Errorf("CLOISTER_REQUEST_PORT = %q, want %q", requestPort, "9998")
	}
}

// TestInstanceIDEnvVarConsistency verifies that the duplicated InstanceIDEnvVar
// constant in the executor package matches the authoritative value in guardian.
// This test exists because we duplicate the constant to avoid import cycles.
func TestInstanceIDEnvVarConsistency(t *testing.T) {
	if executor.InstanceIDEnvVar != InstanceIDEnvVar {
		t.Errorf("executor.InstanceIDEnvVar = %q, want %q (guardian.InstanceIDEnvVar)",
			executor.InstanceIDEnvVar, InstanceIDEnvVar)
	}
}
