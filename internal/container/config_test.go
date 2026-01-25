package container

import (
	"strings"
	"testing"
)

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Basic cases
		{
			name:     "simple lowercase",
			input:    "myproject",
			expected: "myproject",
		},
		{
			name:     "mixed case",
			input:    "MyProject",
			expected: "myproject",
		},
		{
			name:     "with hyphens",
			input:    "my-project",
			expected: "my-project",
		},
		{
			name:     "with numbers",
			input:    "project123",
			expected: "project123",
		},

		// Branch name patterns
		{
			name:     "feature branch with slash",
			input:    "feature/new-feature",
			expected: "feature-new-feature",
		},
		{
			name:     "nested branch",
			input:    "user/feature/branch",
			expected: "user-feature-branch",
		},
		{
			name:     "release branch",
			input:    "release/1.0.0",
			expected: "release-1-0-0",
		},

		// Special characters
		{
			name:     "underscores",
			input:    "my_project",
			expected: "my-project",
		},
		{
			name:     "dots",
			input:    "version.1.2.3",
			expected: "version-1-2-3",
		},
		{
			name:     "at sign",
			input:    "user@org",
			expected: "user-org",
		},
		{
			name:     "mixed special chars",
			input:    "my_project.v1@2",
			expected: "my-project-v1-2",
		},

		// Edge cases
		{
			name:     "empty string",
			input:    "",
			expected: "default",
		},
		{
			name:     "only special chars",
			input:    "___",
			expected: "default",
		},
		{
			name:     "leading special chars",
			input:    "---myproject",
			expected: "myproject",
		},
		{
			name:     "trailing special chars",
			input:    "myproject---",
			expected: "myproject",
		},
		{
			name:     "consecutive special chars",
			input:    "my---project",
			expected: "my-project",
		},
		{
			name:     "leading slash",
			input:    "/main",
			expected: "main",
		},

		// Unicode
		{
			name:     "unicode characters",
			input:    "projet-été",
			expected: "projet-t",
		},
		{
			name:     "emoji",
			input:    "project-🚀-launch",
			expected: "project-launch",
		},

		// Long strings
		{
			name:     "exactly 63 chars",
			input:    strings.Repeat("a", 63),
			expected: strings.Repeat("a", 63),
		},
		{
			name:     "over 63 chars",
			input:    strings.Repeat("a", 100),
			expected: strings.Repeat("a", 63),
		},
		{
			name:     "truncation removes trailing hyphen",
			input:    strings.Repeat("a", 62) + "-b",
			expected: strings.Repeat("a", 62),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := SanitizeName(tc.input)
			if result != tc.expected {
				t.Errorf("SanitizeName(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestSanitizeName_ValidDockerName(t *testing.T) {
	// Docker container names must match [a-zA-Z0-9][a-zA-Z0-9_.-]*
	// Our sanitized names use only lowercase alphanumeric and hyphens
	inputs := []string{
		"myproject",
		"MY-PROJECT",
		"feature/branch",
		"release/v1.2.3",
		"user@domain/repo",
		strings.Repeat("x", 100),
		"---leading---",
		"",
	}

	validChar := func(r rune) bool {
		return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-'
	}

	for _, input := range inputs {
		result := SanitizeName(input)

		// Must not be empty
		if result == "" {
			t.Errorf("SanitizeName(%q) returned empty string", input)
			continue
		}

		// Must be <= 63 chars
		if len(result) > 63 {
			t.Errorf("SanitizeName(%q) returned %d chars, want <= 63", input, len(result))
		}

		// Must not start with hyphen
		if result[0] == '-' {
			t.Errorf("SanitizeName(%q) = %q starts with hyphen", input, result)
		}

		// Must not end with hyphen
		if result[len(result)-1] == '-' {
			t.Errorf("SanitizeName(%q) = %q ends with hyphen", input, result)
		}

		// Must contain only valid chars
		for _, r := range result {
			if !validChar(r) {
				t.Errorf("SanitizeName(%q) = %q contains invalid char %q", input, result, r)
			}
		}
	}
}

func TestConfig_ContainerName(t *testing.T) {
	tests := []struct {
		name     string
		project  string
		branch   string
		expected string
	}{
		{
			name:     "simple names",
			project:  "myproject",
			branch:   "main",
			expected: "cloister-myproject-main",
		},
		{
			name:     "feature branch",
			project:  "myproject",
			branch:   "feature/new-feature",
			expected: "cloister-myproject-feature-new-feature",
		},
		{
			name:     "uppercase project",
			project:  "MyProject",
			branch:   "Main",
			expected: "cloister-myproject-main",
		},
		{
			name:     "special chars in both",
			project:  "my_project.v2",
			branch:   "user@org/feature",
			expected: "cloister-my-project-v2-user-org-feature",
		},
		{
			name:     "empty project",
			project:  "",
			branch:   "main",
			expected: "cloister-default-main",
		},
		{
			name:     "empty branch",
			project:  "myproject",
			branch:   "",
			expected: "cloister-myproject-default",
		},
		{
			name:     "both empty",
			project:  "",
			branch:   "",
			expected: "cloister-default-default",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{
				Project: tc.project,
				Branch:  tc.branch,
			}
			result := cfg.ContainerName()
			if result != tc.expected {
				t.Errorf("ContainerName() = %q, want %q", result, tc.expected)
			}
		})
	}
}

func TestGenerateContainerName(t *testing.T) {
	// Should match Config.ContainerName behavior
	result := GenerateContainerName("myproject", "main")
	expected := "cloister-myproject-main"
	if result != expected {
		t.Errorf("GenerateContainerName() = %q, want %q", result, expected)
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
		Image:       "cloister-default:latest",
		EnvVars:     []string{"FOO=bar", "BAZ=qux"},
		Network:     "cloister-net",
	}

	args := cfg.BuildRunArgs()

	// Check expected arguments are present
	expectedPairs := map[string]string{
		"--name":    "cloister-myproject-main",
		"-v":        "/home/user/projects/myproject:/work",
		"-w":        "/work",
		"--network": "cloister-net",
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
	if args[len(args)-1] != "cloister-default:latest" {
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

	// Should have: --name, container-name, -v, mount, -w, /work, image
	if len(args) != 7 {
		t.Errorf("expected 7 args for minimal config, got %d: %v", len(args), args)
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

func TestConstants(t *testing.T) {
	if DefaultImage != "cloister-default:latest" {
		t.Errorf("DefaultImage = %q, want cloister-default:latest", DefaultImage)
	}
	if DefaultWorkDir != "/work" {
		t.Errorf("DefaultWorkDir = %q, want /work", DefaultWorkDir)
	}
	if DefaultUID != 1000 {
		t.Errorf("DefaultUID = %d, want 1000", DefaultUID)
	}
}
