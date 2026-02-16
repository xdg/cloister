package version //nolint:revive // package name matches package under test

import (
	"testing"
)

func TestVersionDefault(t *testing.T) {
	if Version != "dev" {
		t.Errorf("Version = %q, want %q", Version, "dev")
	}
}
