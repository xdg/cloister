package token

import (
	"sync"
)

// TokenInfo contains information associated with a registered token.
type TokenInfo struct {
	CloisterName string
	ProjectName  string
}

// Registry is a thread-safe in-memory store mapping tokens to token info.
// It implements the guardian.TokenValidator interface (Validate(string) bool).
type Registry struct {
	mu     sync.RWMutex
	tokens map[string]TokenInfo // token -> TokenInfo
}

// NewRegistry creates a new empty token registry.
func NewRegistry() *Registry {
	return &Registry{
		tokens: make(map[string]TokenInfo),
	}
}

// Register adds a token with its associated cloister name (without project).
// If the token already exists, its info is updated.
// This method is kept for backward compatibility.
func (r *Registry) Register(token, cloisterName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens[token] = TokenInfo{CloisterName: cloisterName}
}

// RegisterWithProject adds a token with its associated cloister and project names.
// If the token already exists, its info is updated.
func (r *Registry) RegisterWithProject(token, cloisterName, projectName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tokens[token] = TokenInfo{
		CloisterName: cloisterName,
		ProjectName:  projectName,
	}
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
	info, ok := r.tokens[token]
	if !ok {
		return "", false
	}
	return info.CloisterName, true
}

// LookupInfo checks if a token is valid and returns the full TokenInfo.
// Returns the TokenInfo and true if valid, zero value and false if invalid.
func (r *Registry) LookupInfo(token string) (TokenInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.tokens[token]
	return info, ok
}

// LookupProject checks if a token is valid and returns the associated project name.
// Returns the project name and true if valid, empty string and false if invalid.
func (r *Registry) LookupProject(token string) (projectName string, valid bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.tokens[token]
	if !ok {
		return "", false
	}
	return info.ProjectName, true
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

// List returns a map of all registered tokens to their cloister names.
// The returned map is a copy and can be safely modified.
// This method is kept for backward compatibility.
func (r *Registry) List() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]string, len(r.tokens))
	for k, v := range r.tokens {
		result[k] = v.CloisterName
	}
	return result
}

// ListInfo returns a map of all registered tokens to their full TokenInfo.
// The returned map is a copy and can be safely modified.
func (r *Registry) ListInfo() map[string]TokenInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]TokenInfo, len(r.tokens))
	for k, v := range r.tokens {
		result[k] = v
	}
	return result
}
