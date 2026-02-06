package guardian

import (
	"errors"
	"sync"
)

// MemorySessionAllowlist tracks domains approved with "session" scope.
// These are per-token (i.e., per-cloister session), ephemeral (lost on guardian
// restart), and checked by the proxy before consulting the persistent allowlist.
// All methods are thread-safe.
//
// Token-based isolation ensures that each cloister session has an independent
// domain cache, even when multiple cloisters belong to the same project. This
// prevents session approval leakage between cloisters and enables natural cleanup
// on token revocation.
//
// Lifecycle contract:
//   - Add() is called when a domain approval request is approved with "session" scope
//   - IsAllowed() is called by the proxy before checking the persistent allowlist
//   - Clear(token) MUST be called when a cloister stops or token is revoked
//   - ClearAll() is called when the guardian restarts to reset all session state
//
// Memory management:
//   - Each token maintains a set of approved domains in memory
//   - Call Size() periodically to monitor memory usage
//   - Clear(token) MUST be called when cloisters stop to prevent unbounded growth
type MemorySessionAllowlist struct {
	mu     sync.RWMutex
	tokens map[string]map[string]struct{} // token -> domain set
}

// NewSessionAllowlist creates an empty MemorySessionAllowlist.
func NewSessionAllowlist() *MemorySessionAllowlist {
	return &MemorySessionAllowlist{
		tokens: make(map[string]map[string]struct{}),
	}
}

var (
	// ErrEmptyToken is returned when Add() is called with an empty token string.
	ErrEmptyToken = errors.New("token cannot be empty")
	// ErrEmptyDomain is returned when Add() is called with an empty domain string.
	ErrEmptyDomain = errors.New("domain cannot be empty")
)

// Add adds a domain to the token's session set.
// If the token doesn't exist yet, it is created.
// Returns an error if token or domain is empty.
func (s *MemorySessionAllowlist) Add(token, domain string) error {
	if token == "" {
		return ErrEmptyToken
	}
	if domain == "" {
		return ErrEmptyDomain
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.tokens[token] == nil {
		s.tokens[token] = make(map[string]struct{})
	}
	s.tokens[token][domain] = struct{}{}
	return nil
}

// IsAllowed checks if a domain is in the token's session set.
// Returns false if the token doesn't exist or the domain is not in the set.
func (s *MemorySessionAllowlist) IsAllowed(token, domain string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	domainSet, ok := s.tokens[token]
	if !ok {
		return false
	}
	_, allowed := domainSet[domain]
	return allowed
}

// Clear removes all session domains for a token.
// This is typically called when a cloister stops or token is revoked.
// If the token doesn't exist, this is a no-op.
func (s *MemorySessionAllowlist) Clear(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tokens, token)
}

// ClearAll removes all session domains for all tokens.
// This is typically called when the guardian restarts.
func (s *MemorySessionAllowlist) ClearAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens = make(map[string]map[string]struct{})
}

// Size returns the number of tracked tokens and total number of domains
// across all tokens for memory monitoring.
func (s *MemorySessionAllowlist) Size() (tokens int, domains int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	tokens = len(s.tokens)
	for _, domainSet := range s.tokens {
		domains += len(domainSet)
	}
	return tokens, domains
}
