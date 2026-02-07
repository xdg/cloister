package testutil

import (
	"net"
	"net/http"
	"time"
)

// NoProxyClient returns an HTTP client that doesn't use any proxy.
// This is necessary for tests running inside the cloister container where
// HTTP_PROXY is set to the guardian proxy.
func NoProxyClient() *http.Client {
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
