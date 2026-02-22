package version

import (
	"testing"
)

func TestVersionDefault(t *testing.T) {
	if Version != "dev" {
		t.Errorf("Version = %q, want %q", Version, "dev")
	}
}
