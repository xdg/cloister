// Package request defines types and middleware for hostexec command requests
// between cloister containers and the guardian request server.
package request

import (
	"context"
	"net/http"
)

// TokenHeader is the HTTP header used to pass the cloister token.
const TokenHeader = "X-Cloister-Token"

// TokenInfo contains information associated with a registered token.
// This mirrors token.TokenInfo to avoid import cycles.
type TokenInfo struct {
	CloisterName string
	ProjectName  string
}

// TokenLookup validates a token and returns its associated info.
// Returns the TokenInfo and true if valid, zero value and false if invalid.
type TokenLookup func(token string) (TokenInfo, bool)

// contextKey is a type for context keys to avoid collisions.
type contextKey int

const (
	// tokenInfoKey is the context key for storing TokenInfo.
	tokenInfoKey contextKey = iota
)

// CloisterInfo returns the TokenInfo from the request context.
// Returns zero value and false if no TokenInfo is present.
func CloisterInfo(ctx context.Context) (TokenInfo, bool) {
	info, ok := ctx.Value(tokenInfoKey).(TokenInfo)
	return info, ok
}

// AuthMiddleware creates HTTP middleware that validates tokens and attaches
// cloister metadata to the request context.
//
// The middleware:
//   - Extracts the X-Cloister-Token header
//   - Looks up the token using the provided TokenLookup function
//   - Returns 401 Unauthorized if the header is missing or the token is invalid
//   - Attaches the TokenInfo to the request context for valid tokens
func AuthMiddleware(lookup TokenLookup) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get(TokenHeader)
			if token == "" {
				http.Error(w, "Unauthorized: missing token", http.StatusUnauthorized)
				return
			}

			info, valid := lookup(token)
			if !valid {
				http.Error(w, "Unauthorized: invalid token", http.StatusUnauthorized)
				return
			}

			// Attach TokenInfo to context and continue
			ctx := context.WithValue(r.Context(), tokenInfoKey, info)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
