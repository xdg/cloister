package request

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xdg/cloister/internal/token"
)

// mockTokenLookup creates a TokenLookup that validates tokens from a map.
func mockTokenLookup(tokens map[string]token.Info) TokenLookup {
	return func(token string) (token.Info, bool) {
		info, ok := tokens[token]
		return info, ok
	}
}

func TestAuthMiddleware_MissingHeader(t *testing.T) {
	lookup := mockTokenLookup(map[string]token.Info{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})
	middleware := AuthMiddleware(lookup)

	// Create a handler that should not be called
	called := false
	handler := middleware(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodPost, "/request", http.NoBody)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
	if called {
		t.Error("handler should not have been called")
	}
	if body := rr.Body.String(); body == "" {
		t.Error("expected error message in response body")
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	lookup := mockTokenLookup(map[string]token.Info{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})
	middleware := AuthMiddleware(lookup)

	// Create a handler that should not be called
	called := false
	handler := middleware(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodPost, "/request", http.NoBody)
	req.Header.Set(TokenHeader, "invalid-token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
	if called {
		t.Error("handler should not have been called")
	}
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	expectedInfo := token.Info{CloisterName: "test-cloister", ProjectName: "test-project"}
	lookup := mockTokenLookup(map[string]token.Info{
		"valid-token": expectedInfo,
	})
	middleware := AuthMiddleware(lookup)

	// Create a handler that captures the context
	var capturedInfo token.Info
	var infoFound bool
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedInfo, infoFound = CloisterInfo(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/request", http.NoBody)
	req.Header.Set(TokenHeader, "valid-token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if !infoFound {
		t.Fatal("expected token.Info in context, but not found")
	}
	if capturedInfo.CloisterName != expectedInfo.CloisterName {
		t.Errorf("CloisterName = %q, want %q", capturedInfo.CloisterName, expectedInfo.CloisterName)
	}
	if capturedInfo.ProjectName != expectedInfo.ProjectName {
		t.Errorf("ProjectName = %q, want %q", capturedInfo.ProjectName, expectedInfo.ProjectName)
	}
}

func TestAuthMiddleware_EmptyTokenValue(t *testing.T) {
	lookup := mockTokenLookup(map[string]token.Info{
		"valid-token": {CloisterName: "test-cloister", ProjectName: "test-project"},
	})
	middleware := AuthMiddleware(lookup)

	// Create a handler that should not be called
	called := false
	handler := middleware(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodPost, "/request", http.NoBody)
	req.Header.Set(TokenHeader, "") // Empty value is same as missing
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
	}
	if called {
		t.Error("handler should not have been called")
	}
}

func TestCloisterInfo_NoContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	info, ok := CloisterInfo(req.Context())

	if ok {
		t.Error("expected ok=false for context without token.Info")
	}
	if info.CloisterName != "" || info.ProjectName != "" {
		t.Errorf("expected zero value token.Info, got %+v", info)
	}
}

func TestAuthMiddleware_TokenWithOnlyCloisterName(t *testing.T) {
	// Test token with no project name (legacy or standalone usage)
	expectedInfo := token.Info{CloisterName: "standalone-cloister", ProjectName: ""}
	lookup := mockTokenLookup(map[string]token.Info{
		"standalone-token": expectedInfo,
	})
	middleware := AuthMiddleware(lookup)

	var capturedInfo token.Info
	var infoFound bool
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedInfo, infoFound = CloisterInfo(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/request", http.NoBody)
	req.Header.Set(TokenHeader, "standalone-token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if !infoFound {
		t.Fatal("expected token.Info in context, but not found")
	}
	if capturedInfo.CloisterName != expectedInfo.CloisterName {
		t.Errorf("CloisterName = %q, want %q", capturedInfo.CloisterName, expectedInfo.CloisterName)
	}
	if capturedInfo.ProjectName != "" {
		t.Errorf("ProjectName should be empty, got %q", capturedInfo.ProjectName)
	}
}
