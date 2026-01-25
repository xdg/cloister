package token

import (
	"sync"

	"github.com/xdg/cloister/internal/guardian"
)

// Compile-time check that Registry implements guardian.TokenValidator.
var _ guardian.TokenValidator = (*Registry)(nil)

// Registry is a thread-safe in-memory store mapping tokens to cloister names.
// It implements the guardian.TokenValidator interface.
type Registry struct {
	mu     sync.RWMutex
	tokens map[string]string // token -> cloisterName
}

// NewRegistry creates a new empty token registry.
func NewRegistry() *Registry {
	return &Registry{
		tokens: make(map[string]string),
	}
}

// Register adds a token with its associated cloister name.
// If the token already exists, its cloister name is updated.
func (r *Registry) Register(token, cloisterName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens[token] = cloisterName
}

// Validate checks if a token is valid.
// This method implements the guardian.TokenValidator interface.
func (r *Registry) Validate(token string) bool {
	_, valid := r.Lookup(token)
	return valid
}

// Lookup checks if a token is valid and returns the associated cloister name.
// Returns the cloister name and true if valid, empty string and false if invalid.
func (r *Registry) Lookup(token string) (cloisterName string, valid bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cloisterName, valid = r.tokens[token]
	return cloisterName, valid
}

// Revoke removes a token from the registry.
// This should be called when a container is stopped.
// Returns true if the token was found and removed, false if it didn't exist.
func (r *Registry) Revoke(token string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tokens[token]; exists {
		delete(r.tokens, token)
		return true
	}
	return false
}

// Count returns the number of registered tokens.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tokens)
}
