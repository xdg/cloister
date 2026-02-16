// Package guardian implements the HTTP CONNECT proxy and related services
// that mediate network access for cloister containers.
package guardian

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client provides methods to interact with the guardian API.
type Client struct {
	// BaseURL is the base URL of the guardian API (e.g., "http://localhost:9997").
	BaseURL string

	// HTTPClient is the HTTP client used for requests.
	// If nil, a default client with a 10-second timeout is used.
	HTTPClient *http.Client
}

// NewClient creates a new guardian API client.
// The guardianAddr should be the host:port where the guardian API is listening
// (e.g., "localhost:9997").
func NewClient(guardianAddr string) *Client {
	return &Client{
		BaseURL: fmt.Sprintf("http://%s", guardianAddr),
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// doRequest executes an HTTP request and optionally decodes the response.
// If body is not nil, it's JSON-encoded and sent as the request body.
// If result is not nil, the response body is JSON-decoded into it.
// Returns an error if the response status is not in acceptedStatuses.
func (c *Client) doRequest(method, path string, body, result any, acceptedStatuses ...int) error {
	// Build request body if provided
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	// Create request
	req, err := http.NewRequestWithContext(context.Background(), method, c.BaseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Get HTTP client
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check status
	statusOK := false
	for _, accepted := range acceptedStatuses {
		if resp.StatusCode == accepted {
			statusOK = true
			break
		}
	}
	if !statusOK {
		var errResp errorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Error != "" {
			return fmt.Errorf("%s", errResp.Error)
		}
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	// Decode response if requested
	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// RegisterToken registers a new token with the guardian.
// The token will be associated with the given cloister and project names.
//
// Deprecated: Use RegisterTokenFull to include the worktree path.
func (c *Client) RegisterToken(token, cloisterName, projectName string) error {
	return c.RegisterTokenFull(token, cloisterName, projectName, "")
}

// RegisterTokenFull registers a new token with the guardian.
// The token will be associated with the given cloister, project, and worktree path.
func (c *Client) RegisterTokenFull(token, cloisterName, projectName, worktreePath string) error {
	body := registerTokenRequest{
		Token:    token,
		Cloister: cloisterName,
		Project:  projectName,
		Worktree: worktreePath,
	}

	if err := c.doRequest(http.MethodPost, "/tokens", body, nil, http.StatusCreated); err != nil {
		return fmt.Errorf("failed to register token: %w", err)
	}
	return nil
}

// RevokeToken removes a token from the guardian.
// Returns nil if the token was already revoked or never existed (idempotent).
func (c *Client) RevokeToken(token string) error {
	// Accept both OK and NotFound (token already revoked or never existed)
	if err := c.doRequest(http.MethodDelete, "/tokens/"+token, nil, nil, http.StatusOK, http.StatusNotFound); err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}
	return nil
}

// ListTokens returns a map of all registered tokens to their cloister names.
func (c *Client) ListTokens() (map[string]string, error) {
	var listResp listTokensResponse
	if err := c.doRequest(http.MethodGet, "/tokens", nil, &listResp, http.StatusOK); err != nil {
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}

	result := make(map[string]string, len(listResp.Tokens))
	for _, t := range listResp.Tokens {
		result[t.Token] = t.Cloister
	}

	return result, nil
}
