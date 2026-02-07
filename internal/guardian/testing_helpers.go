package guardian

import (
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

// noProxyClient returns an HTTP client that doesn't use the proxy.
// This is necessary for tests running inside the cloister container where
// HTTP_PROXY is set to the guardian proxy.
func noProxyClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		},
	}
}

// startMockUpstream creates a mock TCP server that echoes data back to the client.
// It returns the server address and a cleanup function.
func startMockUpstream(t *testing.T, handler func(net.Conn)) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock upstream: %v", err)
	}

	var wg sync.WaitGroup
	done := make(chan struct{})

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-done:
					return
				default:
					continue
				}
			}
			wg.Add(1)
			go func(c net.Conn) {
				defer wg.Done()
				defer func() { _ = c.Close() }()
				handler(c)
			}(conn)
		}
	}()

	cleanup := func() {
		close(done)
		_ = listener.Close()
		wg.Wait()
	}

	return listener.Addr().String(), cleanup
}
