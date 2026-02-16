package testutil

import (
	"testing"

	"github.com/xdg/cloister/internal/executor"
	"github.com/xdg/cloister/internal/guardian"
)

// TestInstanceIDEnvVarConsistency verifies that the duplicated InstanceIDEnvVar
// constant in the executor package matches the authoritative value in guardian.
// This test exists because we duplicate the constant to avoid import cycles.
func TestInstanceIDEnvVarConsistency(t *testing.T) {
	// guardian.InstanceIDEnvVar is the authoritative source
	authoritative := guardian.InstanceIDEnvVar

	if executor.InstanceIDEnvVar != authoritative {
		t.Errorf("executor.InstanceIDEnvVar = %q, want %q (guardian.InstanceIDEnvVar)",
			executor.InstanceIDEnvVar, authoritative)
	}
}
