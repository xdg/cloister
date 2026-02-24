package cloister

import (
	"errors"
	"testing"

	"github.com/xdg/cloister/internal/agent"
	"github.com/xdg/cloister/internal/config"
	"github.com/xdg/cloister/internal/container"
	"github.com/xdg/cloister/internal/testutil"
)

// mockWorktreeOps is a test double for WorktreeOperations.
type mockWorktreeOps struct {
	resolveBranchCalled bool
	resolveBranchRoot   string
	resolveBranchName   string
	resolveBranchExist  bool
	resolveBranchErr    error

	dirCalled      bool
	dirProjectName string
	dirBranch      string
	dirResult      string
	dirErr         error

	createCalled       bool
	createRepoRoot     string
	createWorktreePath string
	createBranch       string
	createErr          error
}

func (m *mockWorktreeOps) ResolveBranch(repoRoot, branch string) (bool, error) {
	m.resolveBranchCalled = true
	m.resolveBranchRoot = repoRoot
	m.resolveBranchName = branch
	return m.resolveBranchExist, m.resolveBranchErr
}

func (m *mockWorktreeOps) Dir(projectName, branch string) (string, error) {
	m.dirCalled = true
	m.dirProjectName = projectName
	m.dirBranch = branch
	return m.dirResult, m.dirErr
}

func (m *mockWorktreeOps) Create(repoRoot, worktreePath, branch string) error {
	m.createCalled = true
	m.createRepoRoot = repoRoot
	m.createWorktreePath = worktreePath
	m.createBranch = branch
	return m.createErr
}

// TestStartWorktree_FullFlow verifies the complete worktree start orchestration:
// branch resolution, directory lookup, worktree creation, and delegation to Start.
func TestStartWorktree_FullFlow(t *testing.T) {
	mockMgr := &mockManager{createResult: "container-wt-789"}
	mockGuard := &mockGuardian{}
	mockCfgLoader := &mockConfigLoader{
		config: &config.GlobalConfig{
			Agents: map[string]config.AgentConfig{
				"claude": {AuthMethod: "token", Token: "test-token"},
			},
		},
	}
	mockAgt := &mockAgent{
		name:        "claude",
		setupResult: &agent.SetupResult{EnvVars: map[string]string{}},
	}
	mockReg := &mockRegistryStore{}

	// The worktree dir must not exist for Create to be called.
	// Use a path that does not exist on the filesystem.
	worktreeDir := t.TempDir() + "/worktrees/myproject/feature-x"

	mockWt := &mockWorktreeOps{
		resolveBranchExist: true,
		dirResult:          worktreeDir,
	}

	t.Setenv("HOME", t.TempDir())
	testutil.IsolateXDGDirs(t)

	opts := StartOptions{
		ProjectPath: "/path/to/main-checkout",
		ProjectName: "myproject",
		BranchName:  "feature-x",
	}

	containerID, tok, err := StartWorktree(opts,
		WithManager(mockMgr),
		WithGuardian(mockGuard),
		WithConfigLoader(mockCfgLoader),
		WithAgent(mockAgt),
		WithRegistryStore(mockReg),
		WithWorktreeOps(mockWt),
	)
	if err != nil {
		t.Fatalf("StartWorktree() returned error: %v", err)
	}
	if containerID == "" {
		t.Error("containerID should not be empty")
	}
	if tok == "" {
		t.Error("token should not be empty")
	}

	// Verify ResolveBranch was called with repo root and branch
	if !mockWt.resolveBranchCalled {
		t.Error("ResolveBranch was not called")
	}
	if mockWt.resolveBranchRoot != "/path/to/main-checkout" {
		t.Errorf("ResolveBranch repoRoot = %q, want %q", mockWt.resolveBranchRoot, "/path/to/main-checkout")
	}
	if mockWt.resolveBranchName != "feature-x" {
		t.Errorf("ResolveBranch branch = %q, want %q", mockWt.resolveBranchName, "feature-x")
	}

	// Verify Dir was called with project name and branch
	if !mockWt.dirCalled {
		t.Error("Dir was not called")
	}
	if mockWt.dirProjectName != "myproject" {
		t.Errorf("Dir projectName = %q, want %q", mockWt.dirProjectName, "myproject")
	}
	if mockWt.dirBranch != "feature-x" {
		t.Errorf("Dir branch = %q, want %q", mockWt.dirBranch, "feature-x")
	}

	// Verify Create was called with repo root, worktree path, and branch
	if !mockWt.createCalled {
		t.Error("Create was not called")
	}
	if mockWt.createRepoRoot != "/path/to/main-checkout" {
		t.Errorf("Create repoRoot = %q, want %q", mockWt.createRepoRoot, "/path/to/main-checkout")
	}
	if mockWt.createWorktreePath != worktreeDir {
		t.Errorf("Create worktreePath = %q, want %q", mockWt.createWorktreePath, worktreeDir)
	}
	if mockWt.createBranch != "feature-x" {
		t.Errorf("Create branch = %q, want %q", mockWt.createBranch, "feature-x")
	}

	// Verify Start was called with modified opts: ProjectPath is the worktree path
	if mockMgr.createConfig == nil {
		t.Fatal("container Create was not called")
	}
	if mockMgr.createConfig.ProjectPath != worktreeDir {
		t.Errorf("container ProjectPath = %q, want %q", mockMgr.createConfig.ProjectPath, worktreeDir)
	}

	// Verify the container name includes the branch (worktree style)
	expectedContainerName := "cloister-myproject-feature-x"
	if mockMgr.startContainerName != expectedContainerName {
		t.Errorf("container name = %q, want %q", mockMgr.startContainerName, expectedContainerName)
	}

	// Verify registry entry has IsWorktree=true and correct path
	if mockReg.saved == nil {
		t.Fatal("registry was not saved")
	}
	if len(mockReg.saved.Cloisters) != 1 {
		t.Fatalf("expected 1 registry entry, got %d", len(mockReg.saved.Cloisters))
	}
	entry := mockReg.saved.Cloisters[0]
	if !entry.IsWorktree {
		t.Error("registry entry IsWorktree should be true")
	}
	if entry.HostPath != worktreeDir {
		t.Errorf("registry entry HostPath = %q, want %q", entry.HostPath, worktreeDir)
	}
}

// TestStartWorktree_AlreadyExists verifies that when Start returns
// ErrContainerExists, the error propagates unchanged.
func TestStartWorktree_AlreadyExists(t *testing.T) {
	mockMgr := &mockManager{
		containerExistsResult: true, // container already exists
	}
	mockGuard := &mockGuardian{}
	mockCfgLoader := &mockConfigLoader{
		config: &config.GlobalConfig{},
	}
	mockAgt := &mockAgent{
		name:        "claude",
		setupResult: &agent.SetupResult{},
	}

	worktreeDir := t.TempDir() + "/worktrees/myproject/feature-x"

	mockWt := &mockWorktreeOps{
		resolveBranchExist: true,
		dirResult:          worktreeDir,
	}

	t.Setenv("HOME", t.TempDir())
	testutil.IsolateXDGDirs(t)

	opts := StartOptions{
		ProjectPath: "/path/to/main-checkout",
		ProjectName: "myproject",
		BranchName:  "feature-x",
	}

	_, _, err := StartWorktree(opts,
		WithManager(mockMgr),
		WithGuardian(mockGuard),
		WithConfigLoader(mockCfgLoader),
		WithAgent(mockAgt),
		WithWorktreeOps(mockWt),
	)

	if !errors.Is(err, container.ErrContainerExists) {
		t.Fatalf("StartWorktree() error = %v, want ErrContainerExists", err)
	}
}

// TestStartWorktree_SkipsCreateWhenDirExists verifies that worktree Create
// is skipped when the worktree directory already exists (re-entry case).
func TestStartWorktree_SkipsCreateWhenDirExists(t *testing.T) {
	mockMgr := &mockManager{createResult: "container-wt-reentry"}
	mockGuard := &mockGuardian{}
	mockCfgLoader := &mockConfigLoader{
		config: &config.GlobalConfig{
			Agents: map[string]config.AgentConfig{
				"claude": {AuthMethod: "token", Token: "test-token"},
			},
		},
	}
	mockAgt := &mockAgent{
		name:        "claude",
		setupResult: &agent.SetupResult{EnvVars: map[string]string{}},
	}
	mockReg := &mockRegistryStore{}

	// Use an existing directory so that os.Stat succeeds and Create is skipped.
	worktreeDir := t.TempDir()

	mockWt := &mockWorktreeOps{
		resolveBranchExist: true,
		dirResult:          worktreeDir,
	}

	t.Setenv("HOME", t.TempDir())
	testutil.IsolateXDGDirs(t)

	opts := StartOptions{
		ProjectPath: "/path/to/main-checkout",
		ProjectName: "myproject",
		BranchName:  "feature-x",
	}

	_, _, err := StartWorktree(opts,
		WithManager(mockMgr),
		WithGuardian(mockGuard),
		WithConfigLoader(mockCfgLoader),
		WithAgent(mockAgt),
		WithRegistryStore(mockReg),
		WithWorktreeOps(mockWt),
	)
	if err != nil {
		t.Fatalf("StartWorktree() returned error: %v", err)
	}

	// Create should NOT have been called since the directory already exists
	if mockWt.createCalled {
		t.Error("Create should not be called when worktree directory already exists")
	}

	// But ResolveBranch and Dir should still be called
	if !mockWt.resolveBranchCalled {
		t.Error("ResolveBranch was not called")
	}
	if !mockWt.dirCalled {
		t.Error("Dir was not called")
	}

	// Start should still proceed with the existing directory
	if mockMgr.createConfig == nil {
		t.Fatal("container Create was not called")
	}
	if mockMgr.createConfig.ProjectPath != worktreeDir {
		t.Errorf("container ProjectPath = %q, want %q", mockMgr.createConfig.ProjectPath, worktreeDir)
	}
}

// TestStartWorktree_ResolveBranchError verifies error handling when
// ResolveBranch fails.
func TestStartWorktree_ResolveBranchError(t *testing.T) {
	mockWt := &mockWorktreeOps{
		resolveBranchErr: errors.New("git error"),
	}

	opts := StartOptions{
		ProjectPath: "/path/to/main-checkout",
		ProjectName: "myproject",
		BranchName:  "bad-branch",
	}

	_, _, err := StartWorktree(opts, WithWorktreeOps(mockWt))
	if err == nil {
		t.Fatal("StartWorktree() should return error when ResolveBranch fails")
	}
	if !mockWt.resolveBranchCalled {
		t.Error("ResolveBranch was not called")
	}
	if mockWt.dirCalled {
		t.Error("Dir should not be called after ResolveBranch error")
	}
}

// TestStartWorktree_DirError verifies error handling when Dir fails.
func TestStartWorktree_DirError(t *testing.T) {
	mockWt := &mockWorktreeOps{
		resolveBranchExist: true,
		dirErr:             errors.New("path error"),
	}

	opts := StartOptions{
		ProjectPath: "/path/to/main-checkout",
		ProjectName: "myproject",
		BranchName:  "feature-x",
	}

	_, _, err := StartWorktree(opts, WithWorktreeOps(mockWt))
	if err == nil {
		t.Fatal("StartWorktree() should return error when Dir fails")
	}
	if mockWt.createCalled {
		t.Error("Create should not be called after Dir error")
	}
}

// TestWithWorktreeOps_InjectionWorks verifies that WithWorktreeOps properly injects.
func TestWithWorktreeOps_InjectionWorks(t *testing.T) {
	mockWt := &mockWorktreeOps{}
	deps := applyOptions(WithWorktreeOps(mockWt))

	if deps.worktreeOps != mockWt {
		t.Errorf("applyOptions().worktreeOps = %T, want *mockWorktreeOps", deps.worktreeOps)
	}
}

// TestStartWorktree_IndependentOfMainCheckout verifies that StartWorktree
// works without any prior main checkout cloister running or registered.
// This is an architectural guarantee: StartWorktree calls Start which does
// not check for a main checkout container or registry entry.
func TestStartWorktree_IndependentOfMainCheckout(t *testing.T) {
	mockMgr := &mockManager{createResult: "container-independent-123"}
	mockGuard := &mockGuardian{}
	mockCfgLoader := &mockConfigLoader{
		config: &config.GlobalConfig{
			Agents: map[string]config.AgentConfig{
				"claude": {AuthMethod: "token", Token: "test-token"},
			},
		},
	}
	mockAgt := &mockAgent{
		name:        "claude",
		setupResult: &agent.SetupResult{EnvVars: map[string]string{}},
	}
	// Empty registry: no prior main checkout or any other cloister registered.
	mockReg := &mockRegistryStore{}

	worktreeDir := t.TempDir() + "/worktrees/fresh-project/new-branch"

	mockWt := &mockWorktreeOps{
		resolveBranchExist: false, // branch is new, not pre-existing
		dirResult:          worktreeDir,
	}

	t.Setenv("HOME", t.TempDir())
	testutil.IsolateXDGDirs(t)

	opts := StartOptions{
		ProjectPath: "/path/to/fresh-project",
		ProjectName: "fresh-project",
		BranchName:  "new-branch",
	}

	containerID, tok, err := StartWorktree(opts,
		WithManager(mockMgr),
		WithGuardian(mockGuard),
		WithConfigLoader(mockCfgLoader),
		WithAgent(mockAgt),
		WithRegistryStore(mockReg),
		WithWorktreeOps(mockWt),
	)
	if err != nil {
		t.Fatalf("StartWorktree() returned error: %v", err)
	}
	if containerID == "" {
		t.Error("containerID should not be empty")
	}
	if tok == "" {
		t.Error("token should not be empty")
	}

	// Verify the cloister was created with the worktree name.
	expectedContainerName := "cloister-fresh-project-new-branch"
	if mockMgr.startContainerName != expectedContainerName {
		t.Errorf("container name = %q, want %q", mockMgr.startContainerName, expectedContainerName)
	}

	// Verify registry has exactly one entry (the worktree, no main checkout).
	if mockReg.saved == nil {
		t.Fatal("registry was not saved")
	}
	if len(mockReg.saved.Cloisters) != 1 {
		t.Fatalf("expected 1 registry entry, got %d", len(mockReg.saved.Cloisters))
	}
	entry := mockReg.saved.Cloisters[0]
	if !entry.IsWorktree {
		t.Error("registry entry IsWorktree should be true")
	}
	if entry.Branch != "new-branch" {
		t.Errorf("registry entry Branch = %q, want %q", entry.Branch, "new-branch")
	}
}

// TestStartWorktree_SlashBranch verifies that branch names with slashes
// (e.g. "feature/auth") produce correct cloister names and container names.
func TestStartWorktree_SlashBranch(t *testing.T) {
	mockMgr := &mockManager{createResult: "container-slash-456"}
	mockGuard := &mockGuardian{}
	mockCfgLoader := &mockConfigLoader{
		config: &config.GlobalConfig{
			Agents: map[string]config.AgentConfig{
				"claude": {AuthMethod: "token", Token: "test-token"},
			},
		},
	}
	mockAgt := &mockAgent{
		name:        "claude",
		setupResult: &agent.SetupResult{EnvVars: map[string]string{}},
	}
	mockReg := &mockRegistryStore{}

	worktreeDir := t.TempDir() + "/worktrees/my-api/feature-auth"

	mockWt := &mockWorktreeOps{
		resolveBranchExist: true,
		dirResult:          worktreeDir,
	}

	t.Setenv("HOME", t.TempDir())
	testutil.IsolateXDGDirs(t)

	opts := StartOptions{
		ProjectPath: "/path/to/my-api",
		ProjectName: "my-api",
		BranchName:  "feature/auth",
	}

	_, _, err := StartWorktree(opts,
		WithManager(mockMgr),
		WithGuardian(mockGuard),
		WithConfigLoader(mockCfgLoader),
		WithAgent(mockAgt),
		WithRegistryStore(mockReg),
		WithWorktreeOps(mockWt),
	)
	if err != nil {
		t.Fatalf("StartWorktree() returned error: %v", err)
	}

	// The cloister name should have slashes replaced with hyphens.
	expectedContainerName := "cloister-my-api-feature-auth"
	if mockMgr.startContainerName != expectedContainerName {
		t.Errorf("container name = %q, want %q", mockMgr.startContainerName, expectedContainerName)
	}

	// Verify the branch is stored as the original name (with slash) in the registry.
	if mockReg.saved == nil {
		t.Fatal("registry was not saved")
	}
	if len(mockReg.saved.Cloisters) != 1 {
		t.Fatalf("expected 1 registry entry, got %d", len(mockReg.saved.Cloisters))
	}
	entry := mockReg.saved.Cloisters[0]
	if entry.Branch != "feature/auth" {
		t.Errorf("registry entry Branch = %q, want %q", entry.Branch, "feature/auth")
	}
	if entry.CloisterName != "my-api-feature-auth" {
		t.Errorf("registry entry CloisterName = %q, want %q", entry.CloisterName, "my-api-feature-auth")
	}
}

// TestApplyOptions_DefaultWorktreeOps verifies default worktree ops is set.
func TestApplyOptions_DefaultWorktreeOps(t *testing.T) {
	deps := applyOptions()

	if deps.worktreeOps == nil {
		t.Fatal("applyOptions().worktreeOps is nil")
	}

	_, ok := deps.worktreeOps.(defaultWorktreeOps)
	if !ok {
		t.Errorf("applyOptions().worktreeOps is %T, want defaultWorktreeOps", deps.worktreeOps)
	}
}
