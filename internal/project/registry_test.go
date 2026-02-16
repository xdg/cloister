package project

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestRegistryPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/test/config")

	path := RegistryPath()

	want := "/test/config/cloister/projects.yaml"
	if path != want {
		t.Errorf("RegistryPath() = %q, want %q", path, want)
	}
}

func TestRegistryPath_Default(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")

	path := RegistryPath()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error = %v", err)
	}

	want := home + "/.config/cloister/projects.yaml"
	if path != want {
		t.Errorf("RegistryPath() = %q, want %q", path, want)
	}
}

func TestLoadRegistry_Missing(t *testing.T) {
	// Use a temp directory where no registry file exists
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	reg, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v", err)
	}

	if reg == nil {
		t.Fatal("expected non-nil registry")
	}

	if len(reg.Projects) != 0 {
		t.Errorf("expected empty registry, got %d projects", len(reg.Projects))
	}
}

func TestLoadRegistry_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create the cloister config directory
	configDir := filepath.Join(tmpDir, "cloister")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Write a test registry file
	testTime := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	content := `projects:
  - name: my-project
    root: /home/user/repos/my-project
    remote: git@github.com:user/my-project.git
    last_used: 2024-06-15T10:30:00Z
  - name: other-project
    root: /home/user/repos/other
    remote: https://github.com/user/other.git
    last_used: 2024-06-15T10:30:00Z
`
	registryPath := filepath.Join(configDir, "projects.yaml")
	if err := os.WriteFile(registryPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write registry file: %v", err)
	}

	reg, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v", err)
	}

	if len(reg.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(reg.Projects))
	}

	// Check first project
	p := reg.Projects[0]
	if p.Name != "my-project" {
		t.Errorf("expected name 'my-project', got %q", p.Name)
	}
	if p.Root != "/home/user/repos/my-project" {
		t.Errorf("expected root '/home/user/repos/my-project', got %q", p.Root)
	}
	if p.Remote != "git@github.com:user/my-project.git" {
		t.Errorf("expected remote 'git@github.com:user/my-project.git', got %q", p.Remote)
	}
	if !p.LastUsed.Equal(testTime) {
		t.Errorf("expected last_used %v, got %v", testTime, p.LastUsed)
	}

	// Check second project
	p2 := reg.Projects[1]
	if p2.Name != "other-project" {
		t.Errorf("expected name 'other-project', got %q", p2.Name)
	}
}

func TestLoadRegistry_ExpandsTilde(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create the cloister config directory
	configDir := filepath.Join(tmpDir, "cloister")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Write a registry file with tilde paths
	content := `projects:
  - name: tilde-project
    root: ~/repos/tilde-project
    remote: git@github.com:user/tilde-project.git
    last_used: 2024-06-15T10:30:00Z
`
	registryPath := filepath.Join(configDir, "projects.yaml")
	if err := os.WriteFile(registryPath, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write registry file: %v", err)
	}

	reg, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v", err)
	}

	if len(reg.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(reg.Projects))
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error = %v", err)
	}

	expectedRoot := filepath.Join(home, "repos", "tilde-project")
	if reg.Projects[0].Root != expectedRoot {
		t.Errorf("expected root %q, got %q", expectedRoot, reg.Projects[0].Root)
	}
}

func TestSaveRegistry(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	testTime := time.Date(2024, 7, 20, 14, 0, 0, 0, time.UTC)
	reg := &Registry{
		Projects: []RegistryEntry{
			{
				Name:     "saved-project",
				Root:     "/home/user/repos/saved-project",
				Remote:   "git@github.com:user/saved-project.git",
				LastUsed: testTime,
			},
		},
	}

	if err := SaveRegistry(reg); err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}

	// Verify file was created
	registryPath := filepath.Join(tmpDir, "cloister", "projects.yaml")
	info, err := os.Stat(registryPath)
	if err != nil {
		t.Fatalf("registry file not created: %v", err)
	}

	// Verify permissions are 0600
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("registry file permissions = %o, want 0600", perm)
	}

	// Verify content can be read back
	data, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("failed to read registry file: %v", err)
	}

	var loaded Registry
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal registry: %v", err)
	}

	if len(loaded.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(loaded.Projects))
	}

	p := loaded.Projects[0]
	if p.Name != "saved-project" {
		t.Errorf("expected name 'saved-project', got %q", p.Name)
	}
}

func TestRegistryRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	testTime := time.Date(2024, 8, 10, 9, 45, 30, 0, time.UTC)
	original := &Registry{
		Projects: []RegistryEntry{
			{
				Name:     "project-alpha",
				Root:     "/home/user/repos/alpha",
				Remote:   "git@github.com:user/alpha.git",
				LastUsed: testTime,
			},
			{
				Name:     "project-beta",
				Root:     "/home/user/repos/beta",
				Remote:   "https://github.com/user/beta.git",
				LastUsed: testTime.Add(time.Hour),
			},
		},
	}

	// Save
	if err := SaveRegistry(original); err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}

	// Load
	loaded, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v", err)
	}

	// Verify all fields are preserved
	if len(loaded.Projects) != len(original.Projects) {
		t.Fatalf("expected %d projects, got %d", len(original.Projects), len(loaded.Projects))
	}

	for i, orig := range original.Projects {
		got := loaded.Projects[i]
		if got.Name != orig.Name {
			t.Errorf("project %d: name = %q, want %q", i, got.Name, orig.Name)
		}
		if got.Root != orig.Root {
			t.Errorf("project %d: root = %q, want %q", i, got.Root, orig.Root)
		}
		if got.Remote != orig.Remote {
			t.Errorf("project %d: remote = %q, want %q", i, got.Remote, orig.Remote)
		}
		if !got.LastUsed.Equal(orig.LastUsed) {
			t.Errorf("project %d: last_used = %v, want %v", i, got.LastUsed, orig.LastUsed)
		}
	}
}

func TestSaveRegistry_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Verify cloister directory doesn't exist yet
	configDir := filepath.Join(tmpDir, "cloister")
	if _, err := os.Stat(configDir); !os.IsNotExist(err) {
		t.Fatalf("config dir should not exist before test")
	}

	reg := &Registry{
		Projects: []RegistryEntry{
			{
				Name:     "test-project",
				Root:     "/home/user/test",
				Remote:   "git@github.com:user/test.git",
				LastUsed: time.Now(),
			},
		},
	}

	if err := SaveRegistry(reg); err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}

	// Verify directory was created
	info, err := os.Stat(configDir)
	if err != nil {
		t.Fatalf("config dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("config dir is not a directory")
	}

	// Verify directory permissions are 0700
	perm := info.Mode().Perm()
	if perm != 0o700 {
		t.Errorf("config dir permissions = %o, want 0700", perm)
	}
}

func TestLoadRegistry_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create the cloister config directory
	configDir := filepath.Join(tmpDir, "cloister")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Write invalid YAML
	registryPath := filepath.Join(configDir, "projects.yaml")
	if err := os.WriteFile(registryPath, []byte("invalid: yaml: content:"), 0o600); err != nil {
		t.Fatalf("failed to write registry file: %v", err)
	}

	_, err := LoadRegistry()
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestRegistry_Register_New(t *testing.T) {
	reg := &Registry{}

	info := &Info{
		Name:   "new-project",
		Root:   "/home/user/repos/new-project",
		Remote: "git@github.com:user/new-project.git",
		Branch: "main",
	}

	err := reg.Register(info)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if len(reg.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(reg.Projects))
	}

	p := reg.Projects[0]
	if p.Name != info.Name {
		t.Errorf("Name = %q, want %q", p.Name, info.Name)
	}
	if p.Root != info.Root {
		t.Errorf("Root = %q, want %q", p.Root, info.Root)
	}
	if p.Remote != info.Remote {
		t.Errorf("Remote = %q, want %q", p.Remote, info.Remote)
	}
	if p.LastUsed.IsZero() {
		t.Error("LastUsed should not be zero")
	}
}

func TestRegistry_Register_SameNameSameRemote(t *testing.T) {
	originalTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	reg := &Registry{
		Projects: []RegistryEntry{
			{
				Name:     "existing-project",
				Root:     "/old/path/existing-project",
				Remote:   "git@github.com:user/existing-project.git",
				LastUsed: originalTime,
			},
		},
	}

	// Register with same name and remote but different root
	info := &Info{
		Name:   "existing-project",
		Root:   "/new/path/existing-project",
		Remote: "git@github.com:user/existing-project.git",
		Branch: "main",
	}

	err := reg.Register(info)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Should still have only 1 project
	if len(reg.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(reg.Projects))
	}

	p := reg.Projects[0]

	// Root should be updated
	if p.Root != info.Root {
		t.Errorf("Root = %q, want %q", p.Root, info.Root)
	}

	// LastUsed should be updated (not the original time)
	if p.LastUsed.Equal(originalTime) {
		t.Error("LastUsed should have been updated")
	}
	if p.LastUsed.Before(originalTime) {
		t.Error("LastUsed should be after original time")
	}
}

func TestRegistry_Register_NameCollision(t *testing.T) {
	reg := &Registry{
		Projects: []RegistryEntry{
			{
				Name:     "my-project",
				Root:     "/home/user/repos/my-project",
				Remote:   "git@github.com:user/my-project.git",
				LastUsed: time.Now(),
			},
		},
	}

	// Try to register a project with the same name but different remote
	info := &Info{
		Name:   "my-project",
		Root:   "/home/user/other/my-project",
		Remote: "git@github.com:other-user/my-project.git",
		Branch: "main",
	}

	err := reg.Register(info)
	if err == nil {
		t.Fatal("expected error for name collision")
	}

	var collisionErr *NameCollisionError
	if !errors.As(err, &collisionErr) {
		t.Fatalf("expected NameCollisionError, got %T: %v", err, err)
	}

	if collisionErr.Name != "my-project" {
		t.Errorf("NameCollisionError.Name = %q, want %q", collisionErr.Name, "my-project")
	}
	if collisionErr.ExistingRemote != "git@github.com:user/my-project.git" {
		t.Errorf("NameCollisionError.ExistingRemote = %q, want %q",
			collisionErr.ExistingRemote, "git@github.com:user/my-project.git")
	}
	if collisionErr.NewRemote != "git@github.com:other-user/my-project.git" {
		t.Errorf("NameCollisionError.NewRemote = %q, want %q",
			collisionErr.NewRemote, "git@github.com:other-user/my-project.git")
	}

	// Registry should be unchanged
	if len(reg.Projects) != 1 {
		t.Errorf("expected 1 project (unchanged), got %d", len(reg.Projects))
	}
}

func TestRegistry_FindByName(t *testing.T) {
	reg := &Registry{
		Projects: []RegistryEntry{
			{
				Name:     "project-alpha",
				Root:     "/home/user/repos/alpha",
				Remote:   "git@github.com:user/alpha.git",
				LastUsed: time.Now(),
			},
			{
				Name:     "project-beta",
				Root:     "/home/user/repos/beta",
				Remote:   "git@github.com:user/beta.git",
				LastUsed: time.Now(),
			},
		},
	}

	// Find existing project
	entry := reg.FindByName("project-alpha")
	if entry == nil {
		t.Fatal("expected to find project-alpha")
	}
	if entry.Name != "project-alpha" {
		t.Errorf("Name = %q, want %q", entry.Name, "project-alpha")
	}
	if entry.Root != "/home/user/repos/alpha" {
		t.Errorf("Root = %q, want %q", entry.Root, "/home/user/repos/alpha")
	}

	// Find second project
	entry2 := reg.FindByName("project-beta")
	if entry2 == nil {
		t.Fatal("expected to find project-beta")
	}
	if entry2.Name != "project-beta" {
		t.Errorf("Name = %q, want %q", entry2.Name, "project-beta")
	}

	// Find non-existent project
	missing := reg.FindByName("non-existent")
	if missing != nil {
		t.Errorf("expected nil for non-existent project, got %v", missing)
	}
}

func TestRegistry_FindByRemote(t *testing.T) {
	reg := &Registry{
		Projects: []RegistryEntry{
			{
				Name:     "project-alpha",
				Root:     "/home/user/repos/alpha",
				Remote:   "git@github.com:user/alpha.git",
				LastUsed: time.Now(),
			},
			{
				Name:     "project-beta",
				Root:     "/home/user/repos/beta",
				Remote:   "https://github.com/user/beta.git",
				LastUsed: time.Now(),
			},
		},
	}

	// Find by SSH remote
	entry := reg.FindByRemote("git@github.com:user/alpha.git")
	if entry == nil {
		t.Fatal("expected to find project by SSH remote")
	}
	if entry.Name != "project-alpha" {
		t.Errorf("Name = %q, want %q", entry.Name, "project-alpha")
	}

	// Find by HTTPS remote
	entry2 := reg.FindByRemote("https://github.com/user/beta.git")
	if entry2 == nil {
		t.Fatal("expected to find project by HTTPS remote")
	}
	if entry2.Name != "project-beta" {
		t.Errorf("Name = %q, want %q", entry2.Name, "project-beta")
	}

	// Find non-existent remote
	missing := reg.FindByRemote("git@github.com:user/non-existent.git")
	if missing != nil {
		t.Errorf("expected nil for non-existent remote, got %v", missing)
	}
}

func TestRegistry_UpdateLastUsed(t *testing.T) {
	originalTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	reg := &Registry{
		Projects: []RegistryEntry{
			{
				Name:     "my-project",
				Root:     "/home/user/repos/my-project",
				Remote:   "git@github.com:user/my-project.git",
				LastUsed: originalTime,
			},
		},
	}

	// Update last used time
	err := reg.UpdateLastUsed("my-project")
	if err != nil {
		t.Fatalf("UpdateLastUsed() error = %v", err)
	}

	entry := reg.FindByName("my-project")
	if entry == nil {
		t.Fatal("expected to find my-project")
	}

	// LastUsed should be updated
	if entry.LastUsed.Equal(originalTime) {
		t.Error("LastUsed should have been updated")
	}
	if entry.LastUsed.Before(originalTime) {
		t.Error("LastUsed should be after original time")
	}

	// Update non-existent project
	err = reg.UpdateLastUsed("non-existent")
	if err == nil {
		t.Fatal("expected error for non-existent project")
	}
	if !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("expected ErrProjectNotFound, got %v", err)
	}
}

func TestNameCollisionError_Error(t *testing.T) {
	err := &NameCollisionError{
		Name:           "my-project",
		ExistingRemote: "git@github.com:user/my-project.git",
		NewRemote:      "git@github.com:other/my-project.git",
	}

	msg := err.Error()

	// Check that all key information is in the error message
	if msg == "" {
		t.Fatal("error message should not be empty")
	}
	if !strings.Contains(msg, "my-project") {
		t.Error("error message should contain project name")
	}
	if !strings.Contains(msg, "git@github.com:user/my-project.git") {
		t.Error("error message should contain existing remote")
	}
	if !strings.Contains(msg, "git@github.com:other/my-project.git") {
		t.Error("error message should contain new remote")
	}
}

func TestRegistry_FindByPath_Exact(t *testing.T) {
	reg := &Registry{
		Projects: []RegistryEntry{
			{
				Name:     "project-alpha",
				Root:     "/home/user/repos/alpha",
				Remote:   "git@github.com:user/alpha.git",
				LastUsed: time.Now(),
			},
			{
				Name:     "project-beta",
				Root:     "/home/user/repos/beta",
				Remote:   "git@github.com:user/beta.git",
				LastUsed: time.Now(),
			},
		},
	}

	// Find by exact root path
	entry := reg.FindByPath("/home/user/repos/alpha")
	if entry == nil {
		t.Fatal("expected to find project by exact path")
	}
	if entry.Name != "project-alpha" {
		t.Errorf("Name = %q, want %q", entry.Name, "project-alpha")
	}

	// Find second project by exact path
	entry2 := reg.FindByPath("/home/user/repos/beta")
	if entry2 == nil {
		t.Fatal("expected to find project-beta by exact path")
	}
	if entry2.Name != "project-beta" {
		t.Errorf("Name = %q, want %q", entry2.Name, "project-beta")
	}
}

func TestRegistry_FindByPath_Subdirectory(t *testing.T) {
	reg := &Registry{
		Projects: []RegistryEntry{
			{
				Name:     "project-alpha",
				Root:     "/home/user/repos/alpha",
				Remote:   "git@github.com:user/alpha.git",
				LastUsed: time.Now(),
			},
		},
	}

	// Find by subdirectory path
	entry := reg.FindByPath("/home/user/repos/alpha/src/main")
	if entry == nil {
		t.Fatal("expected to find project by subdirectory path")
	}
	if entry.Name != "project-alpha" {
		t.Errorf("Name = %q, want %q", entry.Name, "project-alpha")
	}

	// Find by immediate subdirectory
	entry2 := reg.FindByPath("/home/user/repos/alpha/README.md")
	if entry2 == nil {
		t.Fatal("expected to find project by file path")
	}
	if entry2.Name != "project-alpha" {
		t.Errorf("Name = %q, want %q", entry2.Name, "project-alpha")
	}
}

func TestRegistry_FindByPath_NotFound(t *testing.T) {
	reg := &Registry{
		Projects: []RegistryEntry{
			{
				Name:     "project-alpha",
				Root:     "/home/user/repos/alpha",
				Remote:   "git@github.com:user/alpha.git",
				LastUsed: time.Now(),
			},
		},
	}

	// Path outside any project
	entry := reg.FindByPath("/home/user/repos/other")
	if entry != nil {
		t.Errorf("expected nil for path outside projects, got %v", entry)
	}

	// Path that starts with project name but is different directory
	entry2 := reg.FindByPath("/home/user/repos/alpha-backup")
	if entry2 != nil {
		t.Errorf("expected nil for similar but different path, got %v", entry2)
	}

	// Empty registry
	emptyReg := &Registry{}
	entry3 := emptyReg.FindByPath("/any/path")
	if entry3 != nil {
		t.Errorf("expected nil for empty registry, got %v", entry3)
	}
}

func TestRegistry_FindByPath_TildePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error = %v", err)
	}

	reg := &Registry{
		Projects: []RegistryEntry{
			{
				Name:     "home-project",
				Root:     filepath.Join(home, "repos", "home-project"),
				Remote:   "git@github.com:user/home-project.git",
				LastUsed: time.Now(),
			},
		},
	}

	// Find using tilde path
	entry := reg.FindByPath("~/repos/home-project")
	if entry == nil {
		t.Fatal("expected to find project by tilde path")
	}
	if entry.Name != "home-project" {
		t.Errorf("Name = %q, want %q", entry.Name, "home-project")
	}

	// Find using tilde path with subdirectory
	entry2 := reg.FindByPath("~/repos/home-project/src")
	if entry2 == nil {
		t.Fatal("expected to find project by tilde subdirectory path")
	}
	if entry2.Name != "home-project" {
		t.Errorf("Name = %q, want %q", entry2.Name, "home-project")
	}
}

func TestRegistry_List(t *testing.T) {
	testTime := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	reg := &Registry{
		Projects: []RegistryEntry{
			{
				Name:     "project-alpha",
				Root:     "/home/user/repos/alpha",
				Remote:   "git@github.com:user/alpha.git",
				LastUsed: testTime,
			},
			{
				Name:     "project-beta",
				Root:     "/home/user/repos/beta",
				Remote:   "git@github.com:user/beta.git",
				LastUsed: testTime.Add(time.Hour),
			},
		},
	}

	list := reg.List()

	// Check count
	if len(list) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(list))
	}

	// Check entries
	if list[0].Name != "project-alpha" {
		t.Errorf("list[0].Name = %q, want %q", list[0].Name, "project-alpha")
	}
	if list[1].Name != "project-beta" {
		t.Errorf("list[1].Name = %q, want %q", list[1].Name, "project-beta")
	}

	// Verify it's a copy by modifying it
	list[0].Name = "modified"
	if reg.Projects[0].Name == "modified" {
		t.Error("List() should return a copy, not the original slice")
	}
}

func TestRegistry_List_Empty(t *testing.T) {
	reg := &Registry{}

	list := reg.List()

	// Should return empty slice, not nil
	if list == nil {
		t.Fatal("List() should return empty slice, not nil")
	}
	if len(list) != 0 {
		t.Errorf("expected 0 entries, got %d", len(list))
	}
}

func TestLookup(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create registry with test data
	reg := &Registry{
		Projects: []RegistryEntry{
			{
				Name:     "test-project",
				Root:     "/home/user/repos/test-project",
				Remote:   "git@github.com:user/test-project.git",
				LastUsed: time.Now(),
			},
		},
	}
	if err := SaveRegistry(reg); err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}

	// Lookup existing project
	entry, err := Lookup("test-project")
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if entry.Name != "test-project" {
		t.Errorf("Name = %q, want %q", entry.Name, "test-project")
	}
	if entry.Root != "/home/user/repos/test-project" {
		t.Errorf("Root = %q, want %q", entry.Root, "/home/user/repos/test-project")
	}
}

func TestLookup_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create empty registry
	reg := &Registry{}
	if err := SaveRegistry(reg); err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}

	// Lookup non-existent project
	entry, err := Lookup("non-existent")
	if err == nil {
		t.Fatal("expected error for non-existent project")
	}
	if !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("expected ErrProjectNotFound, got %v", err)
	}
	if entry != nil {
		t.Errorf("expected nil entry, got %v", entry)
	}
}

func TestRegistry_Remove(t *testing.T) {
	testTime := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	reg := &Registry{
		Projects: []RegistryEntry{
			{
				Name:     "project-alpha",
				Root:     "/home/user/repos/alpha",
				Remote:   "git@github.com:user/alpha.git",
				LastUsed: testTime,
			},
			{
				Name:     "project-beta",
				Root:     "/home/user/repos/beta",
				Remote:   "git@github.com:user/beta.git",
				LastUsed: testTime.Add(time.Hour),
			},
			{
				Name:     "project-gamma",
				Root:     "/home/user/repos/gamma",
				Remote:   "git@github.com:user/gamma.git",
				LastUsed: testTime.Add(2 * time.Hour),
			},
		},
	}

	// Remove middle project
	err := reg.Remove("project-beta")
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	// Should have 2 projects left
	if len(reg.Projects) != 2 {
		t.Fatalf("expected 2 projects after removal, got %d", len(reg.Projects))
	}

	// project-beta should be gone
	if reg.FindByName("project-beta") != nil {
		t.Error("project-beta should have been removed")
	}

	// Other projects should still exist
	if reg.FindByName("project-alpha") == nil {
		t.Error("project-alpha should still exist")
	}
	if reg.FindByName("project-gamma") == nil {
		t.Error("project-gamma should still exist")
	}
}

func TestRegistry_Remove_NotFound(t *testing.T) {
	reg := &Registry{
		Projects: []RegistryEntry{
			{
				Name:     "existing-project",
				Root:     "/home/user/repos/existing",
				Remote:   "git@github.com:user/existing.git",
				LastUsed: time.Now(),
			},
		},
	}

	err := reg.Remove("non-existent")
	if err == nil {
		t.Fatal("expected error for non-existent project")
	}
	if !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("expected ErrProjectNotFound, got %v", err)
	}

	// Registry should be unchanged
	if len(reg.Projects) != 1 {
		t.Errorf("expected 1 project (unchanged), got %d", len(reg.Projects))
	}
}

func TestRegistry_Remove_LastProject(t *testing.T) {
	reg := &Registry{
		Projects: []RegistryEntry{
			{
				Name:     "only-project",
				Root:     "/home/user/repos/only",
				Remote:   "git@github.com:user/only.git",
				LastUsed: time.Now(),
			},
		},
	}

	err := reg.Remove("only-project")
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	// Should have 0 projects
	if len(reg.Projects) != 0 {
		t.Fatalf("expected 0 projects after removal, got %d", len(reg.Projects))
	}
}

func TestRegistry_Remove_FirstProject(t *testing.T) {
	testTime := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	reg := &Registry{
		Projects: []RegistryEntry{
			{
				Name:     "first-project",
				Root:     "/home/user/repos/first",
				Remote:   "git@github.com:user/first.git",
				LastUsed: testTime,
			},
			{
				Name:     "second-project",
				Root:     "/home/user/repos/second",
				Remote:   "git@github.com:user/second.git",
				LastUsed: testTime.Add(time.Hour),
			},
		},
	}

	err := reg.Remove("first-project")
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	// Should have 1 project left
	if len(reg.Projects) != 1 {
		t.Fatalf("expected 1 project after removal, got %d", len(reg.Projects))
	}

	// first-project should be gone
	if reg.FindByName("first-project") != nil {
		t.Error("first-project should have been removed")
	}

	// second-project should still exist
	if reg.FindByName("second-project") == nil {
		t.Error("second-project should still exist")
	}
}

// mockClock is a test Clock that returns a fixed time.
type mockClock struct {
	now time.Time
}

func (m *mockClock) Now() time.Time {
	return m.now
}

func TestRegistry_Register_WithClock(t *testing.T) {
	fixedTime := time.Date(2025, 3, 15, 14, 30, 0, 0, time.UTC)
	clock := &mockClock{now: fixedTime}

	reg := &Registry{}
	reg.SetClock(clock)

	info := &Info{
		Name:   "clock-test-project",
		Root:   "/home/user/repos/clock-test",
		Remote: "git@github.com:user/clock-test.git",
		Branch: "main",
	}

	err := reg.Register(info)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if len(reg.Projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(reg.Projects))
	}

	// Verify that the LastUsed timestamp matches our mock clock
	if !reg.Projects[0].LastUsed.Equal(fixedTime) {
		t.Errorf("LastUsed = %v, want %v", reg.Projects[0].LastUsed, fixedTime)
	}
}

func TestRegistry_UpdateLastUsed_WithClock(t *testing.T) {
	originalTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	updatedTime := time.Date(2025, 6, 20, 10, 0, 0, 0, time.UTC)

	reg := &Registry{
		Projects: []RegistryEntry{
			{
				Name:     "test-project",
				Root:     "/home/user/repos/test",
				Remote:   "git@github.com:user/test.git",
				LastUsed: originalTime,
			},
		},
	}

	// Set mock clock before updating
	clock := &mockClock{now: updatedTime}
	reg.SetClock(clock)

	err := reg.UpdateLastUsed("test-project")
	if err != nil {
		t.Fatalf("UpdateLastUsed() error = %v", err)
	}

	entry := reg.FindByName("test-project")
	if entry == nil {
		t.Fatal("expected to find test-project")
	}

	// Verify that the LastUsed timestamp matches our mock clock exactly
	if !entry.LastUsed.Equal(updatedTime) {
		t.Errorf("LastUsed = %v, want %v", entry.LastUsed, updatedTime)
	}
}

func TestLookupByPath(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	// Create registry with test data
	reg := &Registry{
		Projects: []RegistryEntry{
			{
				Name:     "test-project",
				Root:     "/home/user/repos/test-project",
				Remote:   "git@github.com:user/test-project.git",
				LastUsed: time.Now(),
			},
		},
	}
	if err := SaveRegistry(reg); err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}

	// Lookup by exact path
	entry, err := LookupByPath("/home/user/repos/test-project")
	if err != nil {
		t.Fatalf("LookupByPath() error = %v", err)
	}
	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if entry.Name != "test-project" {
		t.Errorf("Name = %q, want %q", entry.Name, "test-project")
	}

	// Lookup by subdirectory path
	entry2, err := LookupByPath("/home/user/repos/test-project/src/main")
	if err != nil {
		t.Fatalf("LookupByPath() error = %v", err)
	}
	if entry2 == nil {
		t.Fatal("expected non-nil entry for subdirectory")
	}
	if entry2.Name != "test-project" {
		t.Errorf("Name = %q, want %q", entry2.Name, "test-project")
	}

	// Lookup non-existent path
	entry3, err := LookupByPath("/home/user/repos/other")
	if err == nil {
		t.Fatal("expected error for non-existent path")
	}
	if !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("expected ErrProjectNotFound, got %v", err)
	}
	if entry3 != nil {
		t.Errorf("expected nil entry, got %v", entry3)
	}
}
