// Package guardian implements the HTTP CONNECT proxy and related services
// that mediate network access for cloister containers.
package guardian

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// RegisterToken registers a new token with the guardian.
// The token will be associated with the given cloister and project names.
func (c *Client) RegisterToken(token, cloisterName, projectName string) error {
	body := registerTokenRequest{
		Token:    token,
		Cloister: cloisterName,
		Project:  projectName,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/tokens", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		var errResp errorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Error != "" {
			return fmt.Errorf("failed to register token: %s", errResp.Error)
		}
		return fmt.Errorf("failed to register token: status %d", resp.StatusCode)
	}

	return nil
}

// RevokeToken removes a token from the guardian.
func (c *Client) RevokeToken(token string) error {
	req, err := http.NewRequest(http.MethodDelete, c.BaseURL+"/tokens/"+token, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Token was already revoked or never existed, which is fine
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		var errResp errorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Error != "" {
			return fmt.Errorf("failed to revoke token: %s", errResp.Error)
		}
		return fmt.Errorf("failed to revoke token: status %d", resp.StatusCode)
	}

	return nil
}

// ListTokens returns a map of all registered tokens to their cloister names.
func (c *Client) ListTokens() (map[string]string, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/tokens", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp errorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err == nil && errResp.Error != "" {
			return nil, fmt.Errorf("failed to list tokens: %s", errResp.Error)
		}
		return nil, fmt.Errorf("failed to list tokens: status %d", resp.StatusCode)
	}

	var listResp listTokensResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	result := make(map[string]string, len(listResp.Tokens))
	for _, t := range listResp.Tokens {
		result[t.Token] = t.Cloister
	}

	return result, nil
}
