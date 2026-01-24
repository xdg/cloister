package guardian

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestNewProxyServer(t *testing.T) {
	t.Run("default address", func(t *testing.T) {
		p := NewProxyServer("")
		if p.Addr != ":3128" {
			t.Errorf("expected default address :3128, got %s", p.Addr)
		}
	})

	t.Run("custom address", func(t *testing.T) {
		p := NewProxyServer(":8080")
		if p.Addr != ":8080" {
			t.Errorf("expected address :8080, got %s", p.Addr)
		}
	})
}

func TestProxyServer_StartStop(t *testing.T) {
	p := NewProxyServer(":0") // Use random available port

	// Start the server
	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}

	// Verify server is listening
	addr := p.ListenAddr()
	if addr == "" {
		t.Fatal("expected non-empty listen address")
	}

	// Starting again should fail
	if err := p.Start(); err == nil {
		t.Error("expected error when starting already running server")
	}

	// Stop the server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.Stop(ctx); err != nil {
		t.Fatalf("failed to stop proxy server: %v", err)
	}

	// Stopping again should be idempotent
	if err := p.Stop(ctx); err != nil {
		t.Fatalf("expected no error when stopping already stopped server: %v", err)
	}
}

func TestProxyServer_ConnectMethod(t *testing.T) {
	p := NewProxyServer(":0")
	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	addr := p.ListenAddr()

	// CONNECT request should succeed (200 OK)
	t.Run("CONNECT returns 200", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodConnect, fmt.Sprintf("http://%s", addr), nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}
		req.Host = "example.com:443"

		client := &http.Client{
			// Don't follow redirects
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
	})
}

func TestProxyServer_NonConnectMethods(t *testing.T) {
	p := NewProxyServer(":0")
	if err := p.Start(); err != nil {
		t.Fatalf("failed to start proxy server: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	addr := p.ListenAddr()
	baseURL := fmt.Sprintf("http://%s", addr)

	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodPatch,
		http.MethodHead,
		http.MethodOptions,
	}

	for _, method := range methods {
		t.Run(method+" returns 405", func(t *testing.T) {
			req, err := http.NewRequest(method, baseURL, nil)
			if err != nil {
				t.Fatalf("failed to create request: %v", err)
			}

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("expected status 405, got %d", resp.StatusCode)
			}

			// Verify response body contains informative message (except HEAD, which has no body)
			if method != http.MethodHead {
				body, _ := io.ReadAll(resp.Body)
				if len(body) == 0 {
					t.Error("expected non-empty error message in response body")
				}
			}
		})
	}
}

func TestProxyServer_ListenAddr(t *testing.T) {
	p := NewProxyServer(":0")

	// Before starting, should return empty string
	if addr := p.ListenAddr(); addr != "" {
		t.Errorf("expected empty address before start, got %s", addr)
	}

	if err := p.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = p.Stop(ctx)
	}()

	// After starting, should return actual address with port
	addr := p.ListenAddr()
	if addr == "" {
		t.Error("expected non-empty address after start")
	}
}
