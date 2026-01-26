package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigDir_Default(t *testing.T) {
	// Ensure XDG_CONFIG_HOME is not set
	t.Setenv("XDG_CONFIG_HOME", "")

	dir := ConfigDir()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error = %v", err)
	}

	want := home + "/.config/cloister/"
	if dir != want {
		t.Errorf("ConfigDir() = %q, want %q", dir, want)
	}
}

func TestConfigDir_XDGOverride(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/custom/config")

	dir := ConfigDir()

	want := "/custom/config/cloister/"
	if dir != want {
		t.Errorf("ConfigDir() = %q, want %q", dir, want)
	}
}

func TestConfigDir_XDGWithTilde(t *testing.T) {
	// XDG_CONFIG_HOME can contain ~ which should be expanded
	t.Setenv("XDG_CONFIG_HOME", "~/custom-config")

	dir := ConfigDir()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error = %v", err)
	}

	want := home + "/custom-config/cloister/"
	if dir != want {
		t.Errorf("ConfigDir() = %q, want %q", dir, want)
	}
}

func TestEnsureConfigDir(t *testing.T) {
	// Use a temp directory to avoid modifying real config
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	configDir := ConfigDir()

	// Directory should not exist yet
	if _, err := os.Stat(configDir); !os.IsNotExist(err) {
		t.Fatalf("config dir already exists before test: %v", err)
	}

	// Create it
	if err := EnsureConfigDir(); err != nil {
		t.Fatalf("EnsureConfigDir() error = %v", err)
	}

	// Verify it exists
	info, err := os.Stat(configDir)
	if err != nil {
		t.Fatalf("os.Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Errorf("config dir is not a directory")
	}

	// Verify permissions are 0700
	perm := info.Mode().Perm()
	if perm != 0700 {
		t.Errorf("config dir permissions = %o, want 0700", perm)
	}

	// Calling again should succeed (idempotent)
	if err := EnsureConfigDir(); err != nil {
		t.Errorf("EnsureConfigDir() second call error = %v", err)
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error = %v", err)
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "tilde only",
			input: "~",
			want:  home,
		},
		{
			name:  "tilde with path",
			input: "~/foo/bar",
			want:  filepath.Join(home, "foo/bar"),
		},
		{
			name:  "absolute path unchanged",
			input: "/usr/local/bin",
			want:  "/usr/local/bin",
		},
		{
			name:  "relative path unchanged",
			input: "relative/path",
			want:  "relative/path",
		},
		{
			name:  "tilde in middle unchanged",
			input: "/home/~user",
			want:  "/home/~user",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandHome(tt.input)
			if got != tt.want {
				t.Errorf("expandHome(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGlobalConfigPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/test/config")

	path := GlobalConfigPath()

	want := "/test/config/cloister/config.yaml"
	if path != want {
		t.Errorf("GlobalConfigPath() = %q, want %q", path, want)
	}
}

func TestGlobalConfigPath_Default(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")

	path := GlobalConfigPath()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error = %v", err)
	}

	want := home + "/.config/cloister/config.yaml"
	if path != want {
		t.Errorf("GlobalConfigPath() = %q, want %q", path, want)
	}
}

func TestProjectsDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/test/config")

	path := ProjectsDir()

	want := "/test/config/cloister/projects/"
	if path != want {
		t.Errorf("ProjectsDir() = %q, want %q", path, want)
	}
}

func TestProjectsDir_Default(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")

	path := ProjectsDir()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error = %v", err)
	}

	want := home + "/.config/cloister/projects/"
	if path != want {
		t.Errorf("ProjectsDir() = %q, want %q", path, want)
	}
}

func TestConfigDir_TrailingSlash(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/no-trailing")

	dir := ConfigDir()

	if !strings.HasSuffix(dir, "/") {
		t.Errorf("ConfigDir() = %q, want trailing slash", dir)
	}
}

func TestProjectsDir_TrailingSlash(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/test")

	dir := ProjectsDir()

	if !strings.HasSuffix(dir, "/") {
		t.Errorf("ProjectsDir() = %q, want trailing slash", dir)
	}
}
