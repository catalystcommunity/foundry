package openbao

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is an HTTP client for interacting with the OpenBAO API
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient creates a new OpenBAO API client
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Health checks the health status of OpenBAO
func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	url := fmt.Sprintf("%s/v1/sys/health", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Health endpoint returns different status codes based on state
	// 200 - initialized, unsealed, and active
	// 429 - unsealed and standby
	// 472 - disaster recovery mode replication secondary and active
	// 473 - performance standby
	// 501 - not initialized
	// 503 - sealed

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var health HealthResponse
	if err := json.Unmarshal(body, &health); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	health.StatusCode = resp.StatusCode
	return &health, nil
}

// HealthResponse represents the response from the health endpoint
type HealthResponse struct {
	Initialized                bool        `json:"initialized"`
	Sealed                     bool        `json:"sealed"`
	Standby                    bool        `json:"standby"`
	Version                    string      `json:"version"`
	ClusterName                string      `json:"cluster_name"`
	ClusterID                  string      `json:"cluster_id"`
	StatusCode                 int         `json:"-"`
	ReplicationDRMode          interface{} `json:"replication_dr_mode"`          // Can be string or object
	ReplicationPerformanceMode interface{} `json:"replication_performance_mode"` // Can be string or object
}

// IsReady returns true if OpenBAO is initialized, unsealed, and ready
func (h *HealthResponse) IsReady() bool {
	return h.Initialized && !h.Sealed && h.StatusCode == 200
}

// IsSealed returns true if OpenBAO is sealed
func (h *HealthResponse) IsSealed() bool {
	return h.Sealed
}

// IsInitialized returns true if OpenBAO has been initialized
func (h *HealthResponse) IsInitialized() bool {
	return h.Initialized
}

// doRequest performs an HTTP request with the configured token
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	url := fmt.Sprintf("%s%s", c.baseURL, path)

	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.token != "" {
		req.Header.Set("X-Vault-Token", c.token)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	return resp, nil
}

// readResponse reads and unmarshals the response body
func readResponse(resp *http.Response, result interface{}) error {
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	if result != nil && len(body) > 0 {
		if err := json.Unmarshal(body, result); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}
	}

	return nil
}

// KVv2Response represents the response structure from OpenBAO KV v2 API
type KVv2Response struct {
	Data struct {
		Data     map[string]interface{} `json:"data"`
		Metadata struct {
			Version int `json:"version"`
		} `json:"metadata"`
	} `json:"data"`
}

// ReadSecretV2 reads a secret from the KV v2 secrets engine
// path should be in the format: "secret/data/path/to/secret"
// For KV v2, the API path is: /v1/<mount>/data/<path>
func (c *Client) ReadSecretV2(ctx context.Context, mount, path string) (map[string]interface{}, error) {
	apiPath := fmt.Sprintf("/v1/%s/data/%s", mount, path)

	resp, err := c.doRequest(ctx, "GET", apiPath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to read secret: %w", err)
	}

	var result KVv2Response
	if err := readResponse(resp, &result); err != nil {
		return nil, err
	}

	if result.Data.Data == nil {
		return nil, fmt.Errorf("secret not found at path: %s", path)
	}

	return result.Data.Data, nil
}

// WriteSecretV2 writes a secret to the KV v2 secrets engine
func (c *Client) WriteSecretV2(ctx context.Context, mount, path string, data map[string]interface{}) error {
	apiPath := fmt.Sprintf("/v1/%s/data/%s", mount, path)

	body := map[string]interface{}{
		"data": data,
	}

	resp, err := c.doRequest(ctx, "POST", apiPath, body)
	if err != nil {
		return fmt.Errorf("failed to write secret: %w", err)
	}

	return readResponse(resp, nil)
}

// DeleteSecretV2 deletes a secret from the KV v2 secrets engine
// This performs a soft delete - the secret can be recovered with undelete
func (c *Client) DeleteSecretV2(ctx context.Context, mount, path string) error {
	apiPath := fmt.Sprintf("/v1/%s/data/%s", mount, path)

	resp, err := c.doRequest(ctx, "DELETE", apiPath, nil)
	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	return readResponse(resp, nil)
}
