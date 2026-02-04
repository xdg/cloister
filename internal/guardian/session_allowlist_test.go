package guardian

import (
	"sync"
	"testing"
)

func TestSessionAllowlist_AddAndIsAllowed(t *testing.T) {
	s := NewSessionAllowlist()

	// Initially empty - should deny everything
	if s.IsAllowed("project1", "example.com") {
		t.Error("empty allowlist should deny all domains")
	}

	// Add domain to project1
	if err := s.Add("project1", "example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Should allow the added domain for project1
	if !s.IsAllowed("project1", "example.com") {
		t.Error("project1 should allow example.com after adding")
	}

	// Should still deny for different project
	if s.IsAllowed("project2", "example.com") {
		t.Error("project2 should not allow example.com (isolated from project1)")
	}

	// Should deny different domain for same project
	if s.IsAllowed("project1", "other.com") {
		t.Error("project1 should not allow other.com (not added)")
	}
}

func TestSessionAllowlist_MultipleProjects(t *testing.T) {
	s := NewSessionAllowlist()

	// Add domains to different projects
	if err := s.Add("project1", "example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("project1", "test.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("project2", "api.example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("project3", "github.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Verify project1 domains
	if !s.IsAllowed("project1", "example.com") {
		t.Error("project1 should allow example.com")
	}
	if !s.IsAllowed("project1", "test.com") {
		t.Error("project1 should allow test.com")
	}
	if s.IsAllowed("project1", "api.example.com") {
		t.Error("project1 should not allow api.example.com (belongs to project2)")
	}

	// Verify project2 domains
	if !s.IsAllowed("project2", "api.example.com") {
		t.Error("project2 should allow api.example.com")
	}
	if s.IsAllowed("project2", "example.com") {
		t.Error("project2 should not allow example.com (belongs to project1)")
	}

	// Verify project3 domains
	if !s.IsAllowed("project3", "github.com") {
		t.Error("project3 should allow github.com")
	}
	if s.IsAllowed("project3", "test.com") {
		t.Error("project3 should not allow test.com (belongs to project1)")
	}
}

func TestSessionAllowlist_AddDuplicate(t *testing.T) {
	s := NewSessionAllowlist()

	// Add same domain multiple times
	if err := s.Add("project1", "example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("project1", "example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("project1", "example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Should still be allowed (idempotent)
	if !s.IsAllowed("project1", "example.com") {
		t.Error("project1 should allow example.com after multiple adds")
	}
}

func TestSessionAllowlist_Clear(t *testing.T) {
	s := NewSessionAllowlist()

	// Setup: Add domains to multiple projects
	if err := s.Add("project1", "example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("project1", "test.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("project2", "api.example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("project3", "github.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Clear project1
	s.Clear("project1")

	// project1 domains should be denied
	if s.IsAllowed("project1", "example.com") {
		t.Error("project1 should not allow example.com after clear")
	}
	if s.IsAllowed("project1", "test.com") {
		t.Error("project1 should not allow test.com after clear")
	}

	// Other projects should be unaffected
	if !s.IsAllowed("project2", "api.example.com") {
		t.Error("project2 should still allow api.example.com after project1 clear")
	}
	if !s.IsAllowed("project3", "github.com") {
		t.Error("project3 should still allow github.com after project1 clear")
	}
}

func TestSessionAllowlist_ClearNonexistent(t *testing.T) {
	s := NewSessionAllowlist()

	// Add domain to project1
	if err := s.Add("project1", "example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Clear nonexistent project (should be no-op, not panic)
	s.Clear("nonexistent")

	// project1 should be unaffected
	if !s.IsAllowed("project1", "example.com") {
		t.Error("project1 should still allow example.com after clearing nonexistent project")
	}
}

func TestSessionAllowlist_ClearAll(t *testing.T) {
	s := NewSessionAllowlist()

	// Setup: Add domains to multiple projects
	if err := s.Add("project1", "example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("project1", "test.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("project2", "api.example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("project3", "github.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Clear all
	s.ClearAll()

	// All domains should be denied
	if s.IsAllowed("project1", "example.com") {
		t.Error("project1 should not allow example.com after clear all")
	}
	if s.IsAllowed("project1", "test.com") {
		t.Error("project1 should not allow test.com after clear all")
	}
	if s.IsAllowed("project2", "api.example.com") {
		t.Error("project2 should not allow api.example.com after clear all")
	}
	if s.IsAllowed("project3", "github.com") {
		t.Error("project3 should not allow github.com after clear all")
	}
}

func TestSessionAllowlist_ClearAllEmpty(t *testing.T) {
	s := NewSessionAllowlist()

	// ClearAll on empty allowlist should not panic
	s.ClearAll()

	// Should still deny everything
	if s.IsAllowed("project1", "example.com") {
		t.Error("empty allowlist should deny all domains after clear all")
	}
}

func TestSessionAllowlist_ConcurrentAccess(t *testing.T) {
	s := NewSessionAllowlist()
	const goroutines = 10
	const operations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 3) // Add, IsAllowed, Clear operations

	// Concurrent Add operations
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				_ = s.Add("project1", "example.com")
			}
		}(i)
	}

	// Concurrent IsAllowed operations
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				s.IsAllowed("project1", "example.com")
			}
		}(i)
	}

	// Concurrent Clear operations (on different project to avoid race on project1)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				s.Clear("project2")
			}
		}(i)
	}

	wg.Wait()

	// After all concurrent operations, project1 should allow example.com
	if !s.IsAllowed("project1", "example.com") {
		t.Error("project1 should allow example.com after concurrent adds")
	}
}

func TestSessionAllowlist_RejectsEmptyInputs(t *testing.T) {
	s := NewSessionAllowlist()

	// Add with empty project should fail
	err := s.Add("", "example.com")
	if err != ErrEmptyProject {
		t.Errorf("Add with empty project should return ErrEmptyProject, got: %v", err)
	}

	// Add with empty domain should fail
	err = s.Add("project1", "")
	if err != ErrEmptyDomain {
		t.Errorf("Add with empty domain should return ErrEmptyDomain, got: %v", err)
	}

	// Add with both empty should fail with ErrEmptyProject (checked first)
	err = s.Add("", "")
	if err != ErrEmptyProject {
		t.Errorf("Add with empty project and domain should return ErrEmptyProject, got: %v", err)
	}

	// Verify nothing was added
	if s.IsAllowed("", "example.com") {
		t.Error("empty project should not be allowed after rejected add")
	}
	if s.IsAllowed("project1", "") {
		t.Error("empty domain should not be allowed after rejected add")
	}
	if s.IsAllowed("", "") {
		t.Error("empty project/domain should not be allowed after rejected add")
	}

	// Verify that Size reports no data was added
	projects, domains := s.Size()
	if projects != 0 || domains != 0 {
		t.Errorf("Size should be (0, 0) after rejected adds, got: (%d, %d)", projects, domains)
	}
}

func TestSessionAllowlist_Size(t *testing.T) {
	s := NewSessionAllowlist()

	// Initially empty
	projects, domains := s.Size()
	if projects != 0 || domains != 0 {
		t.Errorf("empty allowlist should have Size (0, 0), got: (%d, %d)", projects, domains)
	}

	// Add one domain to one project
	if err := s.Add("project1", "example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	projects, domains = s.Size()
	if projects != 1 || domains != 1 {
		t.Errorf("after adding 1 domain to 1 project, Size should be (1, 1), got: (%d, %d)", projects, domains)
	}

	// Add another domain to the same project
	if err := s.Add("project1", "test.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	projects, domains = s.Size()
	if projects != 1 || domains != 2 {
		t.Errorf("after adding 2 domains to 1 project, Size should be (1, 2), got: (%d, %d)", projects, domains)
	}

	// Add domains to a second project
	if err := s.Add("project2", "api.example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("project2", "cdn.example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("project2", "static.example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	projects, domains = s.Size()
	if projects != 2 || domains != 5 {
		t.Errorf("after adding 5 domains to 2 projects, Size should be (2, 5), got: (%d, %d)", projects, domains)
	}

	// Add duplicate domain (should not increase count)
	if err := s.Add("project1", "example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	projects, domains = s.Size()
	if projects != 2 || domains != 5 {
		t.Errorf("after adding duplicate domain, Size should still be (2, 5), got: (%d, %d)", projects, domains)
	}

	// Clear one project
	s.Clear("project1")
	projects, domains = s.Size()
	if projects != 1 || domains != 3 {
		t.Errorf("after clearing project1, Size should be (1, 3), got: (%d, %d)", projects, domains)
	}

	// Clear all
	s.ClearAll()
	projects, domains = s.Size()
	if projects != 0 || domains != 0 {
		t.Errorf("after ClearAll, Size should be (0, 0), got: (%d, %d)", projects, domains)
	}
}
