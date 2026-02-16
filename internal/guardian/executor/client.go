// Package executor provides a client for the guardian to communicate
// with the host executor via TCP (or Unix socket for legacy support).
package executor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"

	"github.com/xdg/cloister/internal/clog"
	"github.com/xdg/cloister/internal/executor"
)

// DefaultSocketPath is the default path to the hostexec socket inside the guardian container.
//
// Deprecated: Use TCP mode via NewTCPClient instead.
const DefaultSocketPath = "/var/run/hostexec.sock"

// HostDockerInternal is the hostname that Docker containers use to reach the host.
const HostDockerInternal = "host.docker.internal"

// Client communicates with the host executor via TCP or Unix socket.
type Client struct {
	network string // "tcp" or "unix"
	address string // TCP address or socket path
	secret  string
}

// NewClient creates a new executor client using Unix socket.
// socketPath is the path to the Unix socket (typically /var/run/hostexec.sock).
//
// Deprecated: Use NewTCPClient for Docker compatibility.
// secret is the shared secret for authentication.
func NewClient(socketPath, secret string) *Client {
	return &Client{
		network: "unix",
		address: socketPath,
		secret:  secret,
	}
}

// NewTCPClient creates a new executor client using TCP.
// port is the TCP port on the host that the executor is listening on.
// secret is the shared secret for authentication.
// The client will connect to host.docker.internal:port.
func NewTCPClient(port int, secret string) *Client {
	return &Client{
		network: "tcp",
		address: fmt.Sprintf("%s:%d", HostDockerInternal, port),
		secret:  secret,
	}
}

// Execute sends a command execution request to the host executor and returns the response.
// It opens a new connection for each request, sends the request as newline-delimited JSON,
// reads the response, and closes the connection.
func (c *Client) Execute(req executor.ExecuteRequest) (*executor.ExecuteResponse, error) {
	// Connect to the executor
	conn, err := (&net.Dialer{}).DialContext(context.Background(), c.network, c.address)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to executor (%s): %w", c.address, err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			clog.Warn("failed to close executor connection: %v", err)
		}
	}()

	// Build the socket request with authentication
	socketReq := executor.SocketRequest{
		Secret:  c.secret,
		Request: req,
	}

	// Marshal and send request (newline-delimited JSON)
	reqData, err := json.Marshal(socketReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	reqData = append(reqData, '\n')

	if _, err := conn.Write(reqData); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Read response (newline-delimited JSON)
	reader := bufio.NewReader(conn)
	respLine, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse response
	var socketResp executor.SocketResponse
	if err := json.Unmarshal(respLine, &socketResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for socket-level errors (authentication, validation, etc.)
	if !socketResp.Success {
		return nil, fmt.Errorf("executor error: %s", socketResp.Error)
	}

	return &socketResp.Response, nil
}
