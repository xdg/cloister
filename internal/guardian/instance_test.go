package guardian

import (
	"testing"
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
	if tokenPort != DefaultTokenAPIPort {
		t.Errorf("tokenPort = %d, want %d", tokenPort, DefaultTokenAPIPort)
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
