package token

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
	name, valid := r.Lookup(token)
	if !valid {
		t.Error("Lookup should return valid=true for registered token")
	}
	if name != cloisterName {
		t.Errorf("Lookup returned cloister name %q, want %q", name, cloisterName)
	}
}

func TestRegistry_Lookup(t *testing.T) {
	r := NewRegistry()

	// Lookup non-existent token
	name, valid := r.Lookup("nonexistent")
	if valid {
		t.Error("Lookup should return valid=false for non-existent token")
	}
	if name != "" {
		t.Errorf("Lookup should return empty string for non-existent token, got %q", name)
	}

	// Register and lookup
	r.Register("token1", "cloister-alpha")
	name, valid = r.Lookup("token1")
	if !valid {
		t.Error("Lookup should return valid=true for registered token")
	}
	if name != "cloister-alpha" {
		t.Errorf("Lookup returned %q, want %q", name, "cloister-alpha")
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

	name, _ := r.Lookup(token)
	if name != "original-cloister" {
		t.Errorf("expected original-cloister, got %q", name)
	}

	// Re-register with different cloister name
	r.Register(token, "updated-cloister")

	name, valid := r.Lookup(token)
	if !valid {
		t.Error("token should still be valid after update")
	}
	if name != "updated-cloister" {
		t.Errorf("expected updated-cloister, got %q", name)
	}

	// Count should still be 1
	if r.Count() != 1 {
		t.Errorf("count should be 1 after update, got %d", r.Count())
	}
}

func TestRegistry_ThreadSafety(t *testing.T) {
	r := NewRegistry()
	const numGoroutines = 100
	const numOps = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 3) // register, validate, revoke goroutines

	// Register goroutines
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
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

	name, valid := r.Lookup("")
	if !valid || name != "empty-token-cloister" {
		t.Error("empty token lookup failed")
	}

	r.Revoke("")
	if r.Validate("") {
		t.Error("empty token should not be valid after revoke")
	}
}
