package guardian

import (
	"sync"
	"testing"
)

func TestSessionAllowlist_AddAndIsAllowed(t *testing.T) {
	s := NewSessionAllowlist()

	// Initially empty - should deny everything
	if s.IsAllowed("token1", "example.com") {
		t.Error("empty allowlist should deny all domains")
	}

	// Add domain to token1
	if err := s.Add("token1", "example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Should allow the added domain for token1
	if !s.IsAllowed("token1", "example.com") {
		t.Error("token1 should allow example.com after adding")
	}

	// Should still deny for different token
	if s.IsAllowed("token2", "example.com") {
		t.Error("token2 should not allow example.com (isolated from token1)")
	}

	// Should deny different domain for same token
	if s.IsAllowed("token1", "other.com") {
		t.Error("token1 should not allow other.com (not added)")
	}
}

func TestSessionAllowlist_MultipleTokens(t *testing.T) {
	s := NewSessionAllowlist()

	// Add domains to different tokens
	if err := s.Add("token1", "example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("token1", "test.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("token2", "api.example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("token3", "github.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Verify token1 domains
	if !s.IsAllowed("token1", "example.com") {
		t.Error("token1 should allow example.com")
	}
	if !s.IsAllowed("token1", "test.com") {
		t.Error("token1 should allow test.com")
	}
	if s.IsAllowed("token1", "api.example.com") {
		t.Error("token1 should not allow api.example.com (belongs to token2)")
	}

	// Verify token2 domains
	if !s.IsAllowed("token2", "api.example.com") {
		t.Error("token2 should allow api.example.com")
	}
	if s.IsAllowed("token2", "example.com") {
		t.Error("token2 should not allow example.com (belongs to token1)")
	}

	// Verify token3 domains
	if !s.IsAllowed("token3", "github.com") {
		t.Error("token3 should allow github.com")
	}
	if s.IsAllowed("token3", "test.com") {
		t.Error("token3 should not allow test.com (belongs to token1)")
	}
}

func TestSessionAllowlist_AddDuplicate(t *testing.T) {
	s := NewSessionAllowlist()

	// Add same domain multiple times
	if err := s.Add("token1", "example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("token1", "example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("token1", "example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Should still be allowed (idempotent)
	if !s.IsAllowed("token1", "example.com") {
		t.Error("token1 should allow example.com after multiple adds")
	}
}

func TestSessionAllowlist_Clear(t *testing.T) {
	s := NewSessionAllowlist()

	// Setup: Add domains to multiple tokens
	if err := s.Add("token1", "example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("token1", "test.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("token2", "api.example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("token3", "github.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Clear token1
	s.Clear("token1")

	// token1 domains should be denied
	if s.IsAllowed("token1", "example.com") {
		t.Error("token1 should not allow example.com after clear")
	}
	if s.IsAllowed("token1", "test.com") {
		t.Error("token1 should not allow test.com after clear")
	}

	// Other tokens should be unaffected
	if !s.IsAllowed("token2", "api.example.com") {
		t.Error("token2 should still allow api.example.com after token1 clear")
	}
	if !s.IsAllowed("token3", "github.com") {
		t.Error("token3 should still allow github.com after token1 clear")
	}
}

func TestSessionAllowlist_ClearNonexistent(t *testing.T) {
	s := NewSessionAllowlist()

	// Add domain to token1
	if err := s.Add("token1", "example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Clear nonexistent token (should be no-op, not panic)
	s.Clear("nonexistent")

	// token1 should be unaffected
	if !s.IsAllowed("token1", "example.com") {
		t.Error("token1 should still allow example.com after clearing nonexistent token")
	}
}

func TestSessionAllowlist_ClearAll(t *testing.T) {
	s := NewSessionAllowlist()

	// Setup: Add domains to multiple tokens
	if err := s.Add("token1", "example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("token1", "test.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("token2", "api.example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("token3", "github.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Clear all
	s.ClearAll()

	// All domains should be denied
	if s.IsAllowed("token1", "example.com") {
		t.Error("token1 should not allow example.com after clear all")
	}
	if s.IsAllowed("token1", "test.com") {
		t.Error("token1 should not allow test.com after clear all")
	}
	if s.IsAllowed("token2", "api.example.com") {
		t.Error("token2 should not allow api.example.com after clear all")
	}
	if s.IsAllowed("token3", "github.com") {
		t.Error("token3 should not allow github.com after clear all")
	}
}

func TestSessionAllowlist_ClearAllEmpty(t *testing.T) {
	s := NewSessionAllowlist()

	// ClearAll on empty allowlist should not panic
	s.ClearAll()

	// Should still deny everything
	if s.IsAllowed("token1", "example.com") {
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
				_ = s.Add("token1", "example.com")
			}
		}(i)
	}

	// Concurrent IsAllowed operations
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				s.IsAllowed("token1", "example.com")
			}
		}(i)
	}

	// Concurrent Clear operations (on different token to avoid race on token1)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				s.Clear("token2")
			}
		}(i)
	}

	wg.Wait()

	// After all concurrent operations, token1 should allow example.com
	if !s.IsAllowed("token1", "example.com") {
		t.Error("token1 should allow example.com after concurrent adds")
	}
}

func TestSessionAllowlist_RejectsEmptyInputs(t *testing.T) {
	s := NewSessionAllowlist()

	// Add with empty token should fail
	err := s.Add("", "example.com")
	if err != ErrEmptyToken {
		t.Errorf("Add with empty token should return ErrEmptyToken, got: %v", err)
	}

	// Add with empty domain should fail
	err = s.Add("token1", "")
	if err != ErrEmptyDomain {
		t.Errorf("Add with empty domain should return ErrEmptyDomain, got: %v", err)
	}

	// Add with both empty should fail with ErrEmptyToken (checked first)
	err = s.Add("", "")
	if err != ErrEmptyToken {
		t.Errorf("Add with empty token and domain should return ErrEmptyToken, got: %v", err)
	}

	// Verify nothing was added
	if s.IsAllowed("", "example.com") {
		t.Error("empty token should not be allowed after rejected add")
	}
	if s.IsAllowed("token1", "") {
		t.Error("empty domain should not be allowed after rejected add")
	}
	if s.IsAllowed("", "") {
		t.Error("empty token/domain should not be allowed after rejected add")
	}

	// Verify that Size reports no data was added
	tokens, domains := s.Size()
	if tokens != 0 || domains != 0 {
		t.Errorf("Size should be (0, 0) after rejected adds, got: (%d, %d)", tokens, domains)
	}
}

func TestSessionAllowlist_Size(t *testing.T) {
	s := NewSessionAllowlist()

	// Initially empty
	tokens, domains := s.Size()
	if tokens != 0 || domains != 0 {
		t.Errorf("empty allowlist should have Size (0, 0), got: (%d, %d)", tokens, domains)
	}

	// Add one domain to one token
	if err := s.Add("token1", "example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	tokens, domains = s.Size()
	if tokens != 1 || domains != 1 {
		t.Errorf("after adding 1 domain to 1 token, Size should be (1, 1), got: (%d, %d)", tokens, domains)
	}

	// Add another domain to the same token
	if err := s.Add("token1", "test.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	tokens, domains = s.Size()
	if tokens != 1 || domains != 2 {
		t.Errorf("after adding 2 domains to 1 token, Size should be (1, 2), got: (%d, %d)", tokens, domains)
	}

	// Add domains to a second token
	if err := s.Add("token2", "api.example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("token2", "cdn.example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if err := s.Add("token2", "static.example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	tokens, domains = s.Size()
	if tokens != 2 || domains != 5 {
		t.Errorf("after adding 5 domains to 2 tokens, Size should be (2, 5), got: (%d, %d)", tokens, domains)
	}

	// Add duplicate domain (should not increase count)
	if err := s.Add("token1", "example.com"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	tokens, domains = s.Size()
	if tokens != 2 || domains != 5 {
		t.Errorf("after adding duplicate domain, Size should still be (2, 5), got: (%d, %d)", tokens, domains)
	}

	// Clear one token
	s.Clear("token1")
	tokens, domains = s.Size()
	if tokens != 1 || domains != 3 {
		t.Errorf("after clearing token1, Size should be (1, 3), got: (%d, %d)", tokens, domains)
	}

	// Clear all
	s.ClearAll()
	tokens, domains = s.Size()
	if tokens != 0 || domains != 0 {
		t.Errorf("after ClearAll, Size should be (0, 0), got: (%d, %d)", tokens, domains)
	}
}
