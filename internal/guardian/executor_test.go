package guardian

import (
	"testing"
)

func TestSharedSecretEnvVar(t *testing.T) {
	if SharedSecretEnvVar != "CLOISTER_SHARED_SECRET" {
		t.Errorf("Expected env var name CLOISTER_SHARED_SECRET, got %q", SharedSecretEnvVar)
	}
}

func TestExecutorPortEnvVar(t *testing.T) {
	if ExecutorPortEnvVar != "CLOISTER_EXECUTOR_PORT" {
		t.Errorf("Expected env var name CLOISTER_EXECUTOR_PORT, got %q", ExecutorPortEnvVar)
	}
}
