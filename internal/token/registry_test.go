package token //nolint:revive // intentional: does not conflict at import path level

import (
	"sync"
	"testing"
)

func TestRegistry_RegisterAndValidate(t *testing.T) {
	r := NewRegistry()

	token := "abc123"
	cloisterName := "project-main"

	// Token should not be valid before registration
	if r.Validate(token) {
		t.Error("token should not be valid before registration")
	}

	// Register the token
	r.Register(token, cloisterName)

	// Token should now be valid
	if !r.Validate(token) {
		t.Error("token should be valid after registration")
	}

	// Lookup should return the correct cloister name
	info, valid := r.Lookup(token)
	if !valid {
		t.Error("Lookup should return valid=true for registered token")
	}
	if info.CloisterName != cloisterName {
		t.Errorf("Lookup returned cloister name %q, want %q", info.CloisterName, cloisterName)
	}
}

func TestRegistry_Lookup(t *testing.T) {
	r := NewRegistry()

	// Lookup non-existent token
	info, valid := r.Lookup("nonexistent")
	if valid {
		t.Error("Lookup should return valid=false for non-existent token")
	}
	if info.CloisterName != "" || info.ProjectName != "" {
		t.Errorf("Lookup should return zero value for non-existent token, got %+v", info)
	}

	// Register and lookup
	r.Register("token1", "cloister-alpha")
	info, valid = r.Lookup("token1")
	if !valid {
		t.Error("Lookup should return valid=true for registered token")
	}
	if info.CloisterName != "cloister-alpha" {
		t.Errorf("Lookup returned %q, want %q", info.CloisterName, "cloister-alpha")
	}
}

func TestRegistry_Revoke(t *testing.T) {
	r := NewRegistry()

	token := "revoke-test"
	cloisterName := "project-feature"

	// Revoking non-existent token should return false
	if r.Revoke(token) {
		t.Error("Revoke should return false for non-existent token")
	}

	// Register and then revoke
	r.Register(token, cloisterName)
	if !r.Validate(token) {
		t.Fatal("token should be valid after registration")
	}

	// Revoke should return true
	if !r.Revoke(token) {
		t.Error("Revoke should return true for existing token")
	}

	// Token should no longer be valid
	if r.Validate(token) {
		t.Error("token should not be valid after revocation")
	}

	// Revoking again should return false
	if r.Revoke(token) {
		t.Error("Revoke should return false for already-revoked token")
	}
}

func TestRegistry_Lifecycle(t *testing.T) {
	r := NewRegistry()

	// Start with empty registry
	if r.Count() != 0 {
		t.Errorf("new registry should have count 0, got %d", r.Count())
	}

	// Register multiple tokens
	r.Register("token1", "cloister-one")
	r.Register("token2", "cloister-two")
	r.Register("token3", "cloister-three")

	if r.Count() != 3 {
		t.Errorf("registry should have count 3, got %d", r.Count())
	}

	// Validate all
	for _, token := range []string{"token1", "token2", "token3"} {
		if !r.Validate(token) {
			t.Errorf("token %q should be valid", token)
		}
	}

	// Revoke one
	r.Revoke("token2")
	if r.Count() != 2 {
		t.Errorf("registry should have count 2 after revoke, got %d", r.Count())
	}

	if r.Validate("token2") {
		t.Error("token2 should not be valid after revoke")
	}
	if !r.Validate("token1") {
		t.Error("token1 should still be valid")
	}
	if !r.Validate("token3") {
		t.Error("token3 should still be valid")
	}

	// Revoke remaining
	r.Revoke("token1")
	r.Revoke("token3")
	if r.Count() != 0 {
		t.Errorf("registry should have count 0 after all revoked, got %d", r.Count())
	}
}

func TestRegistry_UpdateRegistration(t *testing.T) {
	r := NewRegistry()

	token := "update-test"
	r.Register(token, "original-cloister")

	info, _ := r.Lookup(token)
	if info.CloisterName != "original-cloister" {
		t.Errorf("expected original-cloister, got %q", info.CloisterName)
	}

	// Re-register with different cloister name
	r.Register(token, "updated-cloister")

	info, valid := r.Lookup(token)
	if !valid {
		t.Error("token should still be valid after update")
	}
	if info.CloisterName != "updated-cloister" {
		t.Errorf("expected updated-cloister, got %q", info.CloisterName)
	}

	// Count should still be 1
	if r.Count() != 1 {
		t.Errorf("count should be 1 after update, got %d", r.Count())
	}
}

func TestRegistry_ThreadSafety(_ *testing.T) {
	r := NewRegistry()
	const numGoroutines = 100
	const numOps = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 3) // register, validate, revoke goroutines

	// Register goroutines
	for i := 0; i < numGoroutines; i++ {
		go func(_ int) {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				token := Generate()
				r.Register(token, "cloister")
			}
		}(i)
	}

	// Validate goroutines
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				token := Generate()
				r.Validate(token) // may or may not be registered
			}
		}()
	}

	// Lookup and revoke goroutines
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				token := Generate()
				r.Lookup(token)
				r.Revoke(token) // may or may not be registered
			}
		}()
	}

	wg.Wait()

	// If we got here without deadlock or panic, thread safety is working
	// Final count doesn't matter - the point is no race conditions
}

func TestRegistry_EmptyToken(t *testing.T) {
	r := NewRegistry()

	// Empty token should be valid if registered (though not recommended)
	r.Register("", "empty-token-cloister")

	if !r.Validate("") {
		t.Error("empty token should be valid if registered")
	}

	info, valid := r.Lookup("")
	if !valid || info.CloisterName != "empty-token-cloister" {
		t.Error("empty token lookup failed")
	}

	r.Revoke("")
	if r.Validate("") {
		t.Error("empty token should not be valid after revoke")
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()

	// Empty registry should return empty map
	tokens := r.List()
	if len(tokens) != 0 {
		t.Errorf("expected empty map for new registry, got %d entries", len(tokens))
	}

	// Register some tokens
	r.Register("token-a", "cloister-a")
	r.Register("token-b", "cloister-b")
	r.Register("token-c", "cloister-c")

	// List should return all tokens
	tokens = r.List()
	if len(tokens) != 3 {
		t.Errorf("expected 3 tokens, got %d", len(tokens))
	}

	if tokens["token-a"].CloisterName != "cloister-a" {
		t.Errorf("expected token-a -> cloister-a, got %s", tokens["token-a"].CloisterName)
	}
	if tokens["token-b"].CloisterName != "cloister-b" {
		t.Errorf("expected token-b -> cloister-b, got %s", tokens["token-b"].CloisterName)
	}
	if tokens["token-c"].CloisterName != "cloister-c" {
		t.Errorf("expected token-c -> cloister-c, got %s", tokens["token-c"].CloisterName)
	}

	// Modifying the returned map should not affect the registry
	tokens["token-d"] = Info{CloisterName: "cloister-d"}
	if r.Count() != 3 {
		t.Errorf("modifying returned map should not affect registry, count is %d", r.Count())
	}

	// Revoke a token and verify List is updated
	r.Revoke("token-b")
	tokens = r.List()
	if len(tokens) != 2 {
		t.Errorf("expected 2 tokens after revoke, got %d", len(tokens))
	}
	if _, ok := tokens["token-b"]; ok {
		t.Error("revoked token should not appear in List")
	}
}

func TestRegistry_RegisterWithProject(t *testing.T) {
	r := NewRegistry()

	token := "proj-token"
	cloisterName := "project-main"
	projectName := "my-project"

	// Register with project
	r.RegisterWithProject(token, cloisterName, projectName)

	// Should be valid
	if !r.Validate(token) {
		t.Error("token should be valid after registration")
	}

	// Lookup should return full Info
	info, valid := r.Lookup(token)
	if !valid {
		t.Error("Lookup should return valid=true")
	}
	if info.CloisterName != cloisterName {
		t.Errorf("Lookup CloisterName = %q, expected %q", info.CloisterName, cloisterName)
	}
	if info.ProjectName != projectName {
		t.Errorf("Lookup ProjectName = %q, expected %q", info.ProjectName, projectName)
	}
}

func TestRegistry_LookupWithProject(t *testing.T) {
	r := NewRegistry()

	// Test Lookup for non-existent token
	info, valid := r.Lookup("nonexistent")
	if valid {
		t.Error("Lookup should return valid=false for non-existent token")
	}
	if info.CloisterName != "" || info.ProjectName != "" {
		t.Error("Lookup should return zero value for non-existent token")
	}

	// Register token without project (using Register for backward compat)
	r.Register("simple-token", "simple-cloister")

	// Lookup should return the info with empty project
	info, valid = r.Lookup("simple-token")
	if !valid {
		t.Error("Lookup should return valid=true for registered token")
	}
	if info.CloisterName != "simple-cloister" {
		t.Errorf("Lookup CloisterName = %q, expected simple-cloister", info.CloisterName)
	}
	if info.ProjectName != "" {
		t.Errorf("Lookup ProjectName should be empty, got %q", info.ProjectName)
	}
}

func TestRegistry_ListWithProjectInfo(t *testing.T) {
	r := NewRegistry()

	// Empty registry
	infos := r.List()
	if len(infos) != 0 {
		t.Errorf("expected empty map for new registry, got %d", len(infos))
	}

	// Register tokens with and without projects
	r.Register("token-a", "cloister-a")
	r.RegisterWithProject("token-b", "cloister-b", "project-b")
	r.RegisterWithProject("token-c", "cloister-c", "project-c")

	infos = r.List()
	if len(infos) != 3 {
		t.Errorf("expected 3 entries, got %d", len(infos))
	}

	// Check token-a (no project)
	if infos["token-a"].CloisterName != "cloister-a" {
		t.Errorf("token-a CloisterName = %q, expected cloister-a", infos["token-a"].CloisterName)
	}
	if infos["token-a"].ProjectName != "" {
		t.Errorf("token-a ProjectName = %q, expected empty", infos["token-a"].ProjectName)
	}

	// Check token-b (with project)
	if infos["token-b"].CloisterName != "cloister-b" {
		t.Errorf("token-b CloisterName = %q, expected cloister-b", infos["token-b"].CloisterName)
	}
	if infos["token-b"].ProjectName != "project-b" {
		t.Errorf("token-b ProjectName = %q, expected project-b", infos["token-b"].ProjectName)
	}

	// Check token-c (with project)
	if infos["token-c"].CloisterName != "cloister-c" {
		t.Errorf("token-c CloisterName = %q, expected cloister-c", infos["token-c"].CloisterName)
	}
	if infos["token-c"].ProjectName != "project-c" {
		t.Errorf("token-c ProjectName = %q, expected project-c", infos["token-c"].ProjectName)
	}

	// Modifying returned map should not affect registry
	infos["token-d"] = Info{CloisterName: "cloister-d", ProjectName: "project-d"}
	if r.Count() != 3 {
		t.Errorf("modifying returned map should not affect registry, count is %d", r.Count())
	}
}

func TestRegistry_UpdateWithProject(t *testing.T) {
	r := NewRegistry()

	// Register without project
	r.Register("token", "original-cloister")

	info, _ := r.Lookup("token")
	if info.CloisterName != "original-cloister" || info.ProjectName != "" {
		t.Errorf("unexpected initial state: %+v", info)
	}

	// Update with project
	r.RegisterWithProject("token", "updated-cloister", "new-project")

	info, valid := r.Lookup("token")
	if !valid {
		t.Error("token should still be valid after update")
	}
	if info.CloisterName != "updated-cloister" {
		t.Errorf("CloisterName = %q, expected updated-cloister", info.CloisterName)
	}
	if info.ProjectName != "new-project" {
		t.Errorf("ProjectName = %q, expected new-project", info.ProjectName)
	}

	// Count should still be 1
	if r.Count() != 1 {
		t.Errorf("count should be 1 after update, got %d", r.Count())
	}
}
