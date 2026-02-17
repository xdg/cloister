// Package guardian implements the HTTP CONNECT proxy and related services
// that mediate network access for cloister containers.
package guardian

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/xdg/cloister/internal/clog"
	"github.com/xdg/cloister/internal/token"
)

// DefaultAPIPort is the port for the internal management API.
// This API is exposed only to the host, not to cloister containers.
const DefaultAPIPort = 9997

// TokenRegistry defines the interface for token management operations.
// This is implemented by token.Registry.
type TokenRegistry interface {
	TokenValidator
	Register(token, cloisterName string)
	RegisterWithProject(token, cloisterName, projectName string)
	RegisterFull(token, cloisterName, projectName, worktreePath string)
	Revoke(token string) bool
	List() map[string]token.Info
	Count() int
}

// APIServer provides an HTTP API for managing tokens.
// This API is internal and should only be accessible from the host.
type APIServer struct {
	// Addr is the address to listen on (e.g., ":9997").
	Addr string

	// Registry is the token registry to manage.
	Registry TokenRegistry

	// TokenRevoker clears session-level policy state when a token is revoked.
	// If nil, no session cleanup is performed.
	TokenRevoker TokenRevoker

	server   *http.Server
	listener net.Listener
	mu       sync.Mutex
	running  bool
}

// NewAPIServer creates a new API server listening on the specified address.
// If addr is empty, it defaults to ":9997".
func NewAPIServer(addr string, registry TokenRegistry) *APIServer {
	if addr == "" {
		addr = fmt.Sprintf(":%d", DefaultAPIPort)
	}
	return &APIServer{
		Addr:     addr,
		Registry: registry,
	}
}

// Start begins accepting connections on the API server.
// It returns an error if the server is already running or fails to start.
func (a *APIServer) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.running {
		return errors.New("API server already running")
	}

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", a.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", a.Addr, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/tokens", a.handleTokens)
	mux.HandleFunc("/tokens/{token}", a.handleRevokeToken)

	a.listener = listener
	a.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
	}
	a.running = true

	go func() {
		if err := a.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			clog.Warn("API server error: %v", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the API server.
func (a *APIServer) Stop(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.running {
		return nil
	}

	a.running = false
	if err := a.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown API server: %w", err)
	}
	return nil
}

// ListenAddr returns the actual address the server is listening on.
// This is useful when the server was started with port 0 (random port).
// Returns empty string if the server is not running.
func (a *APIServer) ListenAddr() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.listener == nil {
		return ""
	}
	return a.listener.Addr().String()
}

// registerTokenRequest is the request body for POST /tokens.
type registerTokenRequest struct {
	Token    string `json:"token"`
	Cloister string `json:"cloister"`
	Project  string `json:"project,omitempty"`
	Worktree string `json:"worktree,omitempty"`
}

// tokenInfo represents a single token in the list response.
type tokenInfo struct {
	Token    string `json:"token"`
	Cloister string `json:"cloister"`
	Project  string `json:"project,omitempty"`
	Worktree string `json:"worktree,omitempty"`
}

// listTokensResponse is the response body for GET /tokens.
type listTokensResponse struct {
	Tokens []tokenInfo `json:"tokens"`
}

// statusResponse is a generic status response.
type statusResponse struct {
	Status string `json:"status"`
}

// errorResponse is an error response.
type errorResponse struct {
	Error string `json:"error"`
}

// handleTokens routes /tokens requests based on method.
func (a *APIServer) handleTokens(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		a.handleRegisterToken(w, r)
	case http.MethodGet:
		a.handleListTokens(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleRegisterToken handles POST /tokens requests.
func (a *APIServer) handleRegisterToken(w http.ResponseWriter, r *http.Request) {
	var req registerTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Token == "" {
		a.writeError(w, http.StatusBadRequest, "token is required")
		return
	}

	if req.Cloister == "" {
		a.writeError(w, http.StatusBadRequest, "cloister is required")
		return
	}

	a.Registry.RegisterFull(req.Token, req.Cloister, req.Project, req.Worktree)

	a.writeJSON(w, http.StatusCreated, statusResponse{Status: "registered"})
}

// handleRevokeToken handles DELETE /tokens/{token} requests.
func (a *APIServer) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	tok := r.PathValue("token")
	if tok == "" {
		a.writeError(w, http.StatusBadRequest, "token is required")
		return
	}

	// URL decode the token in case it contains special characters
	// (tokens are hex-encoded so this shouldn't be necessary, but be safe)
	tok = strings.TrimSpace(tok)

	if !a.Registry.Revoke(tok) {
		a.writeError(w, http.StatusNotFound, "token not found")
		return
	}

	// Clear session-level policy state for this token to prevent memory leak
	if a.TokenRevoker != nil {
		a.TokenRevoker.RevokeToken(tok)
	}

	a.writeJSON(w, http.StatusOK, statusResponse{Status: "revoked"})
}

// handleListTokens handles GET /tokens requests.
func (a *APIServer) handleListTokens(w http.ResponseWriter, _ *http.Request) {
	tokens := a.Registry.List()

	resp := listTokensResponse{
		Tokens: make([]tokenInfo, 0, len(tokens)),
	}

	for t, info := range tokens {
		resp.Tokens = append(resp.Tokens, tokenInfo{
			Token:    t,
			Cloister: info.CloisterName,
			Project:  info.ProjectName,
			Worktree: info.WorktreePath,
		})
	}

	a.writeJSON(w, http.StatusOK, resp)
}

// writeJSON writes a JSON response with the given status code.
func (a *APIServer) writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		clog.Warn("failed to encode JSON response: %v", err)
	}
}

// writeError writes an error response with the given status code.
func (a *APIServer) writeError(w http.ResponseWriter, status int, message string) {
	a.writeJSON(w, status, errorResponse{Error: message})
}

// LegacyTokenRevoker adapts a SessionAllowlist to the TokenRevoker interface
// by calling Clear on token revocation. This bridges old wiring in guardian.go
// until Phase 5 replaces it with a real PolicyEngine.
type LegacyTokenRevoker struct {
	SessionAllowlist SessionAllowlist
}

// RevokeToken clears session-approved domains for the given token.
func (l *LegacyTokenRevoker) RevokeToken(tok string) {
	if l.SessionAllowlist != nil {
		l.SessionAllowlist.Clear(tok)
	}
}

// TokenLookupFromRegistry returns a TokenLookupFunc that resolves tokens
// via the given registry.
func TokenLookupFromRegistry(registry *token.Registry) TokenLookupFunc {
	return func(tok string) (TokenLookupResult, bool) {
		info, ok := registry.Lookup(tok)
		if !ok {
			return TokenLookupResult{}, false
		}
		return TokenLookupResult{
			ProjectName:  info.ProjectName,
			CloisterName: info.CloisterName,
		}, true
	}
}
