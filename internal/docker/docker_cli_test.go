package docker

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestRun_Success(t *testing.T) {
	out, err := Run("version", "--format", "{{.Client.Version}}")
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	if out == "" {
		t.Error("expected non-empty output from docker version")
	}
}

func TestRun_InvalidCommand(t *testing.T) {
	_, err := Run("notarealcommand")
	if err == nil {
		t.Fatal("expected error for invalid docker command")
	}

	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		t.Fatalf("expected CommandError, got %T", err)
	}

	if cmdErr.Command != "notarealcommand" {
		t.Errorf("expected command 'notarealcommand', got %q", cmdErr.Command)
	}
}

func TestCheckDaemon_Success(t *testing.T) {
	err := CheckDaemon()
	if err != nil {
		var cmdErr *CommandError
		if errors.As(err, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		if errors.Is(err, ErrDockerNotRunning) {
			t.Skip("Docker daemon not running")
		}
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckDaemon_SentinelErrorUsage(t *testing.T) {
	err := CheckDaemon()

	if err == nil {
		return
	}

	if errors.Is(err, ErrDockerNotRunning) {
		t.Logf("Correctly detected daemon not running: %v", err)
	} else if strings.Contains(err.Error(), "docker CLI not found") {
		t.Skip("Docker CLI not installed")
	} else {
		t.Errorf("unexpected error type: %v", err)
	}
}

// NetworkInfo represents partial docker network ls output for testing.
type NetworkInfo struct {
	Name   string `json:"Name"`
	Driver string `json:"Driver"`
	Scope  string `json:"Scope"`
}

func TestRunJSONLines_NetworkList(t *testing.T) {
	var networks []NetworkInfo
	err := RunJSONLines(&networks, false, "network", "ls")
	if err != nil {
		var cmdErr *CommandError
		if errors.As(err, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		if strings.Contains(err.Error(), "failed to parse JSON") {
			t.Skipf("Docker may not be running or unexpected format: %v", err)
		}
		t.Fatalf("unexpected error: %v", err)
	}

	if len(networks) == 0 {
		t.Skip("No networks found (Docker may not be running)")
	}

	foundBridge := false
	for _, n := range networks {
		if n.Name == "bridge" {
			foundBridge = true
			if n.Driver != "bridge" {
				t.Errorf("expected bridge network driver 'bridge', got %q", n.Driver)
			}
		}
	}
	if !foundBridge {
		t.Log("bridge network not found (may be normal in some Docker configurations)")
	}
}

func TestRunJSONLines_EmptyResult(t *testing.T) {
	var containers []map[string]any
	err := RunJSONLines(&containers, false, "container", "ls", "--filter", "name=cloister-impossible-name-99999")
	if err != nil {
		var cmdErr *CommandError
		if errors.As(err, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		if strings.Contains(err.Error(), "daemon") {
			t.Skip("Docker daemon not running")
		}
		t.Fatalf("unexpected error: %v", err)
	}

	if len(containers) != 0 {
		t.Errorf("expected empty result, got %d containers", len(containers))
	}
}

func TestRunJSONLinesStrict_EmptyResult(t *testing.T) {
	var containers []map[string]any
	err := RunJSONLinesStrict(&containers, "container", "ls", "--filter", "name=cloister-impossible-name-99999")

	if err == nil {
		t.Fatal("expected ErrNoResults for empty result")
	}

	var cmdErr *CommandError
	if errors.As(err, &cmdErr) {
		var execErr *exec.Error
		if errors.As(cmdErr.Err, &execErr) {
			t.Skip("Docker CLI not installed")
		}
		if strings.Contains(cmdErr.Stderr, "daemon") {
			t.Skip("Docker daemon not running")
		}
	}

	if !errors.Is(err, ErrNoResults) {
		t.Fatalf("expected ErrNoResults, got: %v", err)
	}
}

func TestRunJSON_InvalidContainer(t *testing.T) {
	var result map[string]any
	err := RunJSON(&result, false, "inspect", "cloister-nonexistent-container-12345")
	if err == nil {
		t.Fatal("expected error for non-existent container")
	}

	var cmdErr *CommandError
	if !errors.As(err, &cmdErr) {
		var execErr *exec.Error
		if errors.As(err, &execErr) {
			t.Skip("Docker CLI not installed")
		}
		t.Fatalf("expected CommandError, got %T: %v", err, err)
	}
}

func TestRunJSONLinesStrict_WithResults(t *testing.T) {
	var networks []NetworkInfo
	err := RunJSONLinesStrict(&networks, "network", "ls")

	if err != nil {
		var cmdErr *CommandError
		if errors.As(err, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		if errors.Is(err, ErrNoResults) {
			t.Skip("No networks found (unusual Docker configuration)")
		}
		if strings.Contains(err.Error(), "failed to parse JSON") {
			t.Skipf("Docker may not be running: %v", err)
		}
		t.Fatalf("unexpected error: %v", err)
	}

	if len(networks) == 0 {
		t.Error("expected at least one network")
	}
}

func TestRunJSONLines_StrictVsNonStrict(t *testing.T) {
	var containersNonStrict []map[string]any
	errNonStrict := RunJSONLines(&containersNonStrict, false, "container", "ls", "--filter", "name=cloister-impossible-name-strict-test-99999")

	if errNonStrict != nil {
		var cmdErr *CommandError
		if errors.As(errNonStrict, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		if strings.Contains(errNonStrict.Error(), "daemon") {
			t.Skip("Docker daemon not running")
		}
		t.Fatalf("non-strict mode: unexpected error: %v", errNonStrict)
	}

	var containersStrict []map[string]any
	errStrict := RunJSONLines(&containersStrict, true, "container", "ls", "--filter", "name=cloister-impossible-name-strict-test-99999")

	if errStrict == nil {
		t.Fatal("strict mode: expected ErrNoResults for empty result, got nil")
	}
	if !errors.Is(errStrict, ErrNoResults) {
		var cmdErr *CommandError
		if errors.As(errStrict, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		t.Fatalf("strict mode: expected ErrNoResults, got: %v", errStrict)
	}
}

func TestRunJSON_StrictVsNonStrict(t *testing.T) {
	var volumesNonStrict []map[string]any
	errNonStrict := RunJSONLines(&volumesNonStrict, false, "volume", "ls", "--filter", "name=cloister-impossible-volume-99999")

	if errNonStrict != nil {
		var cmdErr *CommandError
		if errors.As(errNonStrict, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		if strings.Contains(errNonStrict.Error(), "daemon") {
			t.Skip("Docker daemon not running")
		}
		t.Fatalf("non-strict mode: unexpected error: %v", errNonStrict)
	}

	var volumesStrict []map[string]any
	errStrict := RunJSONLines(&volumesStrict, true, "volume", "ls", "--filter", "name=cloister-impossible-volume-99999")

	if errStrict == nil {
		t.Fatal("strict mode: expected ErrNoResults for empty result, got nil")
	}
	if !errors.Is(errStrict, ErrNoResults) {
		var cmdErr *CommandError
		if errors.As(errStrict, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		t.Fatalf("strict mode: expected ErrNoResults, got: %v", errStrict)
	}
}

func TestFindContainerByExactName_NotFound(t *testing.T) {
	info, err := FindContainerByExactName("cloister-nonexistent-container-test-exact-12345")
	if err != nil {
		var cmdErr *CommandError
		if errors.As(err, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		if strings.Contains(err.Error(), "daemon") {
			t.Skip("Docker daemon not running")
		}
		t.Fatalf("unexpected error: %v", err)
	}

	if info != nil {
		t.Errorf("expected nil for non-existent container, got: %+v", info)
	}
}

func TestFindContainerByExactName_SubstringMatch(t *testing.T) {
	info, err := FindContainerByExactName("cloister-exact-test")
	if err != nil {
		var cmdErr *CommandError
		if errors.As(err, &cmdErr) {
			var execErr *exec.Error
			if errors.As(cmdErr.Err, &execErr) {
				t.Skip("Docker CLI not installed")
			}
		}
		if strings.Contains(err.Error(), "daemon") {
			t.Skip("Docker daemon not running")
		}
		t.Fatalf("unexpected error: %v", err)
	}

	if info != nil {
		t.Errorf("expected nil, got container: %+v", info)
	}
}
