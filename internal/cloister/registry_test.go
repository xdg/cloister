package cloister

import (
	"errors"
	"testing"
	"time"

	"github.com/xdg/cloister/internal/testutil"
)

// mockClock is a test Clock that returns a fixed time.
type mockClock struct {
	now time.Time
}

func (m *mockClock) Now() time.Time {
	return m.now
}

func TestRegistry_RoundTrip(t *testing.T) {
	testutil.IsolateXDGDirs(t)

	clock := &mockClock{now: time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)}
	reg := &Registry{}
	reg.SetClock(clock)

	entries := []RegistryEntry{
		{
			CloisterName: "project-main",
			ProjectName:  "my-project",
			Branch:       "main",
			HostPath:     "/home/user/repos/my-project",
			IsWorktree:   false,
		},
		{
			CloisterName: "project-feature",
			ProjectName:  "my-project",
			Branch:       "feature-x",
			HostPath:     "/home/user/worktrees/feature-x",
			IsWorktree:   true,
		},
	}

	for _, e := range entries {
		if err := reg.Register(e); err != nil {
			t.Fatalf("Register(%q) error = %v", e.CloisterName, err)
		}
	}

	// Save and reload
	if err := SaveRegistry(reg); err != nil {
		t.Fatalf("SaveRegistry() error = %v", err)
	}

	loaded, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v", err)
	}

	if len(loaded.Cloisters) != len(entries) {
		t.Fatalf("expected %d entries, got %d", len(entries), len(loaded.Cloisters))
	}

	for i, want := range entries {
		got := loaded.Cloisters[i]
		if got.CloisterName != want.CloisterName {
			t.Errorf("entry %d: CloisterName = %q, want %q", i, got.CloisterName, want.CloisterName)
		}
		if got.ProjectName != want.ProjectName {
			t.Errorf("entry %d: ProjectName = %q, want %q", i, got.ProjectName, want.ProjectName)
		}
		if got.Branch != want.Branch {
			t.Errorf("entry %d: Branch = %q, want %q", i, got.Branch, want.Branch)
		}
		if got.HostPath != want.HostPath {
			t.Errorf("entry %d: HostPath = %q, want %q", i, got.HostPath, want.HostPath)
		}
		if got.IsWorktree != want.IsWorktree {
			t.Errorf("entry %d: IsWorktree = %v, want %v", i, got.IsWorktree, want.IsWorktree)
		}
		if !got.CreatedAt.Equal(clock.now) {
			t.Errorf("entry %d: CreatedAt = %v, want %v", i, got.CreatedAt, clock.now)
		}
	}
}

func TestRegistry_Upsert(t *testing.T) {
	testutil.IsolateXDGDirs(t)

	t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	clock := &mockClock{now: t1}

	reg := &Registry{}
	reg.SetClock(clock)

	// First registration
	err := reg.Register(RegistryEntry{
		CloisterName: "proj-main",
		ProjectName:  "proj",
		Branch:       "main",
		HostPath:     "/old/path",
		IsWorktree:   false,
	})
	if err != nil {
		t.Fatalf("first Register() error = %v", err)
	}

	if len(reg.Cloisters) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(reg.Cloisters))
	}
	if !reg.Cloisters[0].CreatedAt.Equal(t1) {
		t.Errorf("CreatedAt = %v, want %v", reg.Cloisters[0].CreatedAt, t1)
	}

	// Advance clock and re-register same name + same project with updated fields
	clock.now = t2
	err = reg.Register(RegistryEntry{
		CloisterName: "proj-main",
		ProjectName:  "proj",
		Branch:       "develop",
		HostPath:     "/new/path",
		IsWorktree:   true,
	})
	if err != nil {
		t.Fatalf("second Register() error = %v", err)
	}

	// Should still be 1 entry (upsert, not append)
	if len(reg.Cloisters) != 1 {
		t.Fatalf("expected 1 entry after upsert, got %d", len(reg.Cloisters))
	}

	got := reg.Cloisters[0]

	// Fields should be updated
	if got.Branch != "develop" {
		t.Errorf("Branch = %q, want %q", got.Branch, "develop")
	}
	if got.HostPath != "/new/path" {
		t.Errorf("HostPath = %q, want %q", got.HostPath, "/new/path")
	}
	if !got.IsWorktree {
		t.Errorf("IsWorktree = %v, want true", got.IsWorktree)
	}

	// CreatedAt should be preserved from the original registration
	if !got.CreatedAt.Equal(t1) {
		t.Errorf("CreatedAt = %v, want original %v (should be preserved on upsert)", got.CreatedAt, t1)
	}
}

func TestRegistry_NameCollision(t *testing.T) {
	testutil.IsolateXDGDirs(t)

	clock := &mockClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	reg := &Registry{}
	reg.SetClock(clock)

	// Register with project A
	err := reg.Register(RegistryEntry{
		CloisterName: "shared-name",
		ProjectName:  "project-a",
		HostPath:     "/path/a",
	})
	if err != nil {
		t.Fatalf("first Register() error = %v", err)
	}

	// Register same cloister name with project B
	err = reg.Register(RegistryEntry{
		CloisterName: "shared-name",
		ProjectName:  "project-b",
		HostPath:     "/path/b",
	})

	if err == nil {
		t.Fatal("expected NameCollisionError, got nil")
	}

	var collisionErr *NameCollisionError
	if !errors.As(err, &collisionErr) {
		t.Fatalf("expected *NameCollisionError, got %T: %v", err, err)
	}

	if collisionErr.CloisterName != "shared-name" {
		t.Errorf("CloisterName = %q, want %q", collisionErr.CloisterName, "shared-name")
	}
	if collisionErr.ExistingProject != "project-a" {
		t.Errorf("ExistingProject = %q, want %q", collisionErr.ExistingProject, "project-a")
	}
	if collisionErr.NewProject != "project-b" {
		t.Errorf("NewProject = %q, want %q", collisionErr.NewProject, "project-b")
	}

	// Registry should still have only the original entry
	if len(reg.Cloisters) != 1 {
		t.Errorf("expected 1 entry (unchanged), got %d", len(reg.Cloisters))
	}
}

func TestRegistry_FindByProject(t *testing.T) {
	testutil.IsolateXDGDirs(t)

	clock := &mockClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	reg := &Registry{}
	reg.SetClock(clock)

	// Register cloisters for two different projects
	for _, e := range []RegistryEntry{
		{CloisterName: "alpha-main", ProjectName: "alpha", Branch: "main", HostPath: "/alpha"},
		{CloisterName: "alpha-dev", ProjectName: "alpha", Branch: "dev", HostPath: "/alpha-dev"},
		{CloisterName: "beta-main", ProjectName: "beta", Branch: "main", HostPath: "/beta"},
	} {
		if err := reg.Register(e); err != nil {
			t.Fatalf("Register(%q) error = %v", e.CloisterName, err)
		}
	}

	// FindByProject for "alpha" should return 2 entries
	alphaEntries := reg.FindByProject("alpha")
	if len(alphaEntries) != 2 {
		t.Fatalf("FindByProject(alpha) returned %d entries, want 2", len(alphaEntries))
	}

	names := map[string]bool{}
	for _, e := range alphaEntries {
		names[e.CloisterName] = true
		if e.ProjectName != "alpha" {
			t.Errorf("FindByProject(alpha) returned entry with ProjectName = %q", e.ProjectName)
		}
	}
	if !names["alpha-main"] || !names["alpha-dev"] {
		t.Errorf("FindByProject(alpha) missing expected entries, got %v", names)
	}

	// FindByProject for "beta" should return 1 entry
	betaEntries := reg.FindByProject("beta")
	if len(betaEntries) != 1 {
		t.Fatalf("FindByProject(beta) returned %d entries, want 1", len(betaEntries))
	}
	if betaEntries[0].CloisterName != "beta-main" {
		t.Errorf("FindByProject(beta)[0].CloisterName = %q, want %q", betaEntries[0].CloisterName, "beta-main")
	}
}

func TestRegistry_FindByProject_Empty(t *testing.T) {
	reg := &Registry{}

	result := reg.FindByProject("nonexistent")

	if result == nil {
		t.Fatal("FindByProject() should return empty slice, not nil")
	}
	if len(result) != 0 {
		t.Errorf("FindByProject() returned %d entries, want 0", len(result))
	}
}

func TestRegistry_FindByName(t *testing.T) {
	clock := &mockClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	reg := &Registry{}
	reg.SetClock(clock)

	if err := reg.Register(RegistryEntry{
		CloisterName: "my-cloister",
		ProjectName:  "my-project",
		Branch:       "main",
		HostPath:     "/home/user/my-project",
		IsWorktree:   false,
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	t.Run("found", func(t *testing.T) {
		entry := reg.FindByName("my-cloister")
		if entry == nil {
			t.Fatal("expected non-nil entry")
		}
		if entry.CloisterName != "my-cloister" {
			t.Errorf("CloisterName = %q, want %q", entry.CloisterName, "my-cloister")
		}
		if entry.ProjectName != "my-project" {
			t.Errorf("ProjectName = %q, want %q", entry.ProjectName, "my-project")
		}
	})

	t.Run("not found", func(t *testing.T) {
		entry := reg.FindByName("does-not-exist")
		if entry != nil {
			t.Errorf("expected nil for unknown name, got %+v", entry)
		}
	})
}

func TestRegistry_Remove(t *testing.T) {
	clock := &mockClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	reg := &Registry{}
	reg.SetClock(clock)

	for _, e := range []RegistryEntry{
		{CloisterName: "c1", ProjectName: "p1", HostPath: "/p1"},
		{CloisterName: "c2", ProjectName: "p2", HostPath: "/p2"},
		{CloisterName: "c3", ProjectName: "p3", HostPath: "/p3"},
	} {
		if err := reg.Register(e); err != nil {
			t.Fatalf("Register(%q) error = %v", e.CloisterName, err)
		}
	}

	t.Run("remove existing", func(t *testing.T) {
		err := reg.Remove("c2")
		if err != nil {
			t.Fatalf("Remove(c2) error = %v", err)
		}

		if len(reg.Cloisters) != 2 {
			t.Fatalf("expected 2 entries after removal, got %d", len(reg.Cloisters))
		}

		if reg.FindByName("c2") != nil {
			t.Error("c2 should have been removed")
		}

		// Other entries should still exist
		if reg.FindByName("c1") == nil {
			t.Error("c1 should still exist")
		}
		if reg.FindByName("c3") == nil {
			t.Error("c3 should still exist")
		}
	})

	t.Run("remove nonexistent", func(t *testing.T) {
		err := reg.Remove("no-such-cloister")
		if !errors.Is(err, ErrCloisterNotFound) {
			t.Errorf("expected ErrCloisterNotFound, got %v", err)
		}
	})
}

func TestRegistry_List(t *testing.T) {
	clock := &mockClock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}

	t.Run("defensive copy", func(t *testing.T) {
		reg := &Registry{}
		reg.SetClock(clock)

		for _, e := range []RegistryEntry{
			{CloisterName: "c1", ProjectName: "p1", HostPath: "/p1"},
			{CloisterName: "c2", ProjectName: "p2", HostPath: "/p2"},
		} {
			if err := reg.Register(e); err != nil {
				t.Fatalf("Register(%q) error = %v", e.CloisterName, err)
			}
		}

		list := reg.List()
		if len(list) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(list))
		}

		// Modify the returned slice
		list[0].CloisterName = "modified"

		// Original should be unchanged
		if reg.Cloisters[0].CloisterName == "modified" {
			t.Error("List() should return a defensive copy; modifying it affected the registry")
		}
	})

	t.Run("empty registry", func(t *testing.T) {
		reg := &Registry{}
		list := reg.List()

		if list == nil {
			t.Fatal("List() should return empty slice, not nil")
		}
		if len(list) != 0 {
			t.Errorf("expected 0 entries, got %d", len(list))
		}
	})
}

func TestLoadRegistry_MissingFile(t *testing.T) {
	testutil.IsolateXDGDirs(t)

	reg, err := LoadRegistry()
	if err != nil {
		t.Fatalf("LoadRegistry() error = %v, want nil for missing file", err)
	}

	if reg == nil {
		t.Fatal("expected non-nil registry")
	}

	if len(reg.Cloisters) != 0 {
		t.Errorf("expected empty registry, got %d entries", len(reg.Cloisters))
	}
}
