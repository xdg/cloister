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

func TestGenerateContainerName(t *testing.T) {
	// Should match Config.ContainerName behavior
	result := GenerateContainerName("myproject")
	expected := "cloister-myproject"
	if result != expected {
		t.Errorf("GenerateContainerName() = %q, want %q", result, expected)
	}
}

func TestGenerateCloisterName(t *testing.T) {
	tests := []struct {
		name     string
		project  string
		expected string
	}{
		{
			name:     "simple name",
			project:  "myproject",
			expected: "myproject",
		},
		{
			name:     "uppercase project",
			project:  "MyProject",
			expected: "myproject",
		},
		{
			name:     "special chars",
			project:  "my_project.v2",
			expected: "my-project-v2",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := GenerateCloisterName(tc.project)
			if result != tc.expected {
				t.Errorf("GenerateCloisterName() = %q, want %q", result, tc.expected)
			}
		})
	}
}

func TestGenerateWorktreeCloisterName(t *testing.T) {
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
			expected: "myproject-main",
		},
		{
			name:     "feature branch",
			project:  "myproject",
			branch:   "feature/new-feature",
			expected: "myproject-feature-new-feature",
		},
		{
			name:     "uppercase project",
			project:  "MyProject",
			branch:   "Main",
			expected: "myproject-main",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := GenerateWorktreeCloisterName(tc.project, tc.branch)
			if result != tc.expected {
				t.Errorf("GenerateWorktreeCloisterName() = %q, want %q", result, tc.expected)
			}
		})
	}
}

func TestCloisterNameToContainerName(t *testing.T) {
	tests := []struct {
		cloisterName  string
		containerName string
	}{
		{"myproject", "cloister-myproject"},
		{"cloister", "cloister-cloister"},
		{"", "cloister-"},
	}

	for _, tc := range tests {
		t.Run(tc.cloisterName, func(t *testing.T) {
			result := CloisterNameToContainerName(tc.cloisterName)
			if result != tc.containerName {
				t.Errorf("CloisterNameToContainerName(%q) = %q, want %q", tc.cloisterName, result, tc.containerName)
			}
		})
	}
}

func TestNameToCloisterName(t *testing.T) {
	tests := []struct {
		containerName string
		cloisterName  string
	}{
		{"cloister-myproject", "myproject"},
		{"cloister-cloister", "cloister"},
		{"other-container", "other-container"}, // no prefix, returns unchanged
		{"cloister-", ""},                      // prefix only
	}

	for _, tc := range tests {
		t.Run(tc.containerName, func(t *testing.T) {
			result := NameToCloisterName(tc.containerName)
			if result != tc.cloisterName {
				t.Errorf("NameToCloisterName(%q) = %q, want %q", tc.containerName, result, tc.cloisterName)
			}
		})
	}
}

func TestParseCloisterName(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		expectedProject string
		expectedBranch  string
	}{
		{
			name:            "project and branch",
			input:           "foo-main",
			expectedProject: "foo",
			expectedBranch:  "main",
		},
		{
			name:            "multi-hyphen splits on last",
			input:           "foo-bar-feature",
			expectedProject: "foo-bar",
			expectedBranch:  "feature",
		},
		{
			name:            "no hyphen returns project only",
			input:           "foo",
			expectedProject: "foo",
			expectedBranch:  "",
		},
		{
			name:            "empty string",
			input:           "",
			expectedProject: "",
			expectedBranch:  "",
		},
		{
			name:            "trailing hyphen",
			input:           "foo-",
			expectedProject: "foo",
			expectedBranch:  "",
		},
		{
			name:            "leading hyphen",
			input:           "-main",
			expectedProject: "",
			expectedBranch:  "main",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			project, branch := ParseCloisterName(tc.input)
			if project != tc.expectedProject {
				t.Errorf("ParseCloisterName(%q) project = %q, want %q", tc.input, project, tc.expectedProject)
			}
			if branch != tc.expectedBranch {
				t.Errorf("ParseCloisterName(%q) branch = %q, want %q", tc.input, branch, tc.expectedBranch)
			}
		})
	}
}

func TestCloisterContainerNameRoundTrip(t *testing.T) {
	// Test that converting cloister name to container name and back gives the original
	cloisterNames := []string{
		"myproject",
		"cloister",
		"test-project",
	}

	for _, original := range cloisterNames {
		containerName := CloisterNameToContainerName(original)
		result := NameToCloisterName(containerName)
		if result != original {
			t.Errorf("Round trip failed: %q -> %q -> %q", original, containerName, result)
		}
	}
}
