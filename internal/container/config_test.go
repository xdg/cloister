package container

import (
	"testing"
)

func TestConfig_ContainerName(t *testing.T) {
	tests := []struct {
		name     string
		project  string
		expected string
	}{
		{
			name:     "simple name",
			project:  "myproject",
			expected: "cloister-myproject",
		},
		{
			name:     "uppercase project",
			project:  "MyProject",
			expected: "cloister-myproject",
		},
		{
			name:     "special chars",
			project:  "my_project.v2",
			expected: "cloister-my-project-v2",
		},
		{
			name:     "empty project",
			project:  "",
			expected: "cloister-default",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				Project: tc.project,
			}
			result := cfg.ContainerName()
			if result != tc.expected {
				t.Errorf("ContainerName() = %q, want %q", result, tc.expected)
			}
		})
	}
}

func TestConfig_ImageName(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		expected string
	}{
		{
			name:     "default when empty",
			image:    "",
			expected: DefaultImage,
		},
		{
			name:     "custom image",
			image:    "my-custom-image:v1",
			expected: "my-custom-image:v1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{Image: tc.image}
			if result := cfg.ImageName(); result != tc.expected {
				t.Errorf("ImageName() = %q, want %q", result, tc.expected)
			}
		})
	}
}

func TestConfig_UserID(t *testing.T) {
	tests := []struct {
		name     string
		uid      int
		expected int
	}{
		{
			name:     "default when zero",
			uid:      0,
			expected: DefaultUID,
		},
		{
			name:     "custom uid",
			uid:      1001,
			expected: 1001,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{UID: tc.uid}
			if result := cfg.UserID(); result != tc.expected {
				t.Errorf("UserID() = %d, want %d", result, tc.expected)
			}
		})
	}
}

func TestConfig_BuildRunArgs(t *testing.T) {
	cfg := &Config{
		Project:     "myproject",
		Branch:      "main",
		ProjectPath: "/home/user/projects/myproject",
		Image:       "cloister:latest",
		EnvVars:     []string{"FOO=bar", "BAZ=qux"},
		Network:     "cloister-net",
	}

	args := cfg.BuildRunArgs()

	// Check expected arguments are present
	expectedPairs := map[string]string{
		"--name":    "cloister-myproject",
		"-v":        "/home/user/projects/myproject:/work",
		"-w":        "/work",
		"--network": "cloister-net",
		"--user":    "1000",
	}

	for flag, value := range expectedPairs {
		found := false
		for i, arg := range args {
			if arg == flag && i+1 < len(args) && args[i+1] == value {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("BuildRunArgs() missing %s %s, got %v", flag, value, args)
		}
	}

	// Check security flags (combined flag=value format)
	expectedFlags := []string{
		"--cap-drop=ALL",
	}
	for _, flag := range expectedFlags {
		found := false
		for _, arg := range args {
			if arg == flag {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("BuildRunArgs() missing %s, got %v", flag, args)
		}
	}

	// Check env vars
	envCount := 0
	for i, arg := range args {
		if arg == "-e" && i+1 < len(args) {
			envCount++
			val := args[i+1]
			if val != "FOO=bar" && val != "BAZ=qux" {
				t.Errorf("unexpected env var: %s", val)
			}
		}
	}
	if envCount != 2 {
		t.Errorf("expected 2 env vars, got %d", envCount)
	}

	// Check image is last
	if args[len(args)-1] != "cloister:latest" {
		t.Errorf("expected image as last arg, got %s", args[len(args)-1])
	}
}

func TestConfig_BuildRunArgs_MinimalConfig(t *testing.T) {
	cfg := &Config{
		Project:     "proj",
		Branch:      "main",
		ProjectPath: "/path/to/project",
	}

	args := cfg.BuildRunArgs()

	// Should have: --name, container-name, -v, mount, -w, /work,
	//              --cap-drop=ALL, --user, 1000, image
	if len(args) != 10 {
		t.Errorf("expected 10 args for minimal config, got %d: %v", len(args), args)
	}

	// Should use default image
	if args[len(args)-1] != DefaultImage {
		t.Errorf("expected default image, got %s", args[len(args)-1])
	}

	// Should not have --network without Network set
	for _, arg := range args {
		if arg == "--network" {
			t.Error("--network should not be present when Network is empty")
		}
	}
}

func TestConfig_BuildRunArgs_SecurityHardening(t *testing.T) {
	cfg := &Config{
		Project:     "myproject",
		Branch:      "main",
		ProjectPath: "/home/user/projects/myproject",
		Network:     "cloister-net",
	}

	args := cfg.BuildRunArgs()

	// Helper to check if a flag exists in args
	containsFlag := func(flag string) bool {
		for _, arg := range args {
			if arg == flag {
				return true
			}
		}
		return false
	}

	// Helper to check if a flag-value pair exists in args
	containsFlagValue := func(flag, value string) bool {
		for i, arg := range args {
			if arg == flag && i+1 < len(args) && args[i+1] == value {
				return true
			}
		}
		return false
	}

	// Test: --cap-drop=ALL is present
	if !containsFlag("--cap-drop=ALL") {
		t.Errorf("BuildRunArgs() missing --cap-drop=ALL, got %v", args)
	}

	// Test: --user with default UID 1000 is present
	if !containsFlagValue("--user", "1000") {
		t.Errorf("BuildRunArgs() missing --user 1000, got %v", args)
	}

	// Test: --network=cloister-net is present (only network, no bridge)
	if !containsFlagValue("--network", "cloister-net") {
		t.Errorf("BuildRunArgs() missing --network cloister-net, got %v", args)
	}
}

func TestConfig_BuildRunArgs_CustomUID(t *testing.T) {
	cfg := &Config{
		Project:     "myproject",
		Branch:      "main",
		ProjectPath: "/home/user/projects/myproject",
		UID:         1001,
	}

	args := cfg.BuildRunArgs()

	// Helper to check if a flag-value pair exists in args
	containsFlagValue := func(flag, value string) bool {
		for i, arg := range args {
			if arg == flag && i+1 < len(args) && args[i+1] == value {
				return true
			}
		}
		return false
	}

	// Test: --user with custom UID is present
	if !containsFlagValue("--user", "1001") {
		t.Errorf("BuildRunArgs() should use custom UID 1001, got %v", args)
	}
}

func TestConstants(t *testing.T) {
	if DefaultImage != "cloister:latest" {
		t.Errorf("DefaultImage = %q, want cloister:latest", DefaultImage)
	}
	if DefaultWorkDir != "/work" {
		t.Errorf("DefaultWorkDir = %q, want /work", DefaultWorkDir)
	}
	if DefaultUID != 1000 {
		t.Errorf("DefaultUID = %d, want 1000", DefaultUID)
	}
}
