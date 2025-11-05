package truenas

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// NewClient creates a new TrueNAS API client
func NewClient(apiURL string, apiKey string) (*Client, error) {
	if apiURL == "" {
		return nil, fmt.Errorf("API URL cannot be empty")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("API key cannot be empty")
	}

	// Remove trailing slash from URL
	apiURL = strings.TrimSuffix(apiURL, "/")

	httpClient := &defaultHTTPClient{
		baseURL: apiURL,
		apiKey:  apiKey,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	return &Client{
		baseURL:    apiURL,
		apiKey:     apiKey,
		httpClient: httpClient,
	}, nil
}

// defaultHTTPClient implements HTTPClient using net/http
type defaultHTTPClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

// Do performs an HTTP request
func (c *defaultHTTPClient) Do(method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	url := c.baseURL + path
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for API errors
	if resp.StatusCode >= 400 {
		var apiErr APIError
		if err := json.Unmarshal(respBody, &apiErr); err == nil && (apiErr.ErrorMsg != "" || apiErr.Message != "") {
			return nil, &apiErr
		}
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// CreateDataset creates a new dataset
func (c *Client) CreateDataset(config DatasetConfig) (*Dataset, error) {
	if config.Name == "" {
		return nil, fmt.Errorf("dataset name cannot be empty")
	}
	if config.Type == "" {
		config.Type = "FILESYSTEM"
	}

	respBody, err := c.httpClient.Do("POST", "/api/v2.0/pool/dataset", config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dataset: %w", err)
	}

	var dataset Dataset
	if err := json.Unmarshal(respBody, &dataset); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &dataset, nil
}

// DeleteDataset deletes a dataset
func (c *Client) DeleteDataset(name string) error {
	if name == "" {
		return fmt.Errorf("dataset name cannot be empty")
	}

	path := fmt.Sprintf("/api/v2.0/pool/dataset/id/%s", name)
	_, err := c.httpClient.Do("DELETE", path, nil)
	if err != nil {
		return fmt.Errorf("failed to delete dataset: %w", err)
	}

	return nil
}

// ListDatasets lists all datasets
func (c *Client) ListDatasets() ([]Dataset, error) {
	respBody, err := c.httpClient.Do("GET", "/api/v2.0/pool/dataset", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list datasets: %w", err)
	}

	var datasets []Dataset
	if err := json.Unmarshal(respBody, &datasets); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return datasets, nil
}

// GetDataset gets a specific dataset by name
func (c *Client) GetDataset(name string) (*Dataset, error) {
	if name == "" {
		return nil, fmt.Errorf("dataset name cannot be empty")
	}

	path := fmt.Sprintf("/api/v2.0/pool/dataset/id/%s", name)
	respBody, err := c.httpClient.Do("GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get dataset: %w", err)
	}

	var dataset Dataset
	if err := json.Unmarshal(respBody, &dataset); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &dataset, nil
}

// CreateNFSShare creates a new NFS share
func (c *Client) CreateNFSShare(config NFSConfig) (*NFSShare, error) {
	if config.Path == "" {
		return nil, fmt.Errorf("NFS share path cannot be empty")
	}

	respBody, err := c.httpClient.Do("POST", "/api/v2.0/sharing/nfs", config)
	if err != nil {
		return nil, fmt.Errorf("failed to create NFS share: %w", err)
	}

	var share NFSShare
	if err := json.Unmarshal(respBody, &share); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &share, nil
}

// DeleteNFSShare deletes an NFS share
func (c *Client) DeleteNFSShare(id int) error {
	if id <= 0 {
		return fmt.Errorf("invalid NFS share ID: %d", id)
	}

	path := fmt.Sprintf("/api/v2.0/sharing/nfs/id/%d", id)
	_, err := c.httpClient.Do("DELETE", path, nil)
	if err != nil {
		return fmt.Errorf("failed to delete NFS share: %w", err)
	}

	return nil
}

// ListNFSShares lists all NFS shares
func (c *Client) ListNFSShares() ([]NFSShare, error) {
	respBody, err := c.httpClient.Do("GET", "/api/v2.0/sharing/nfs", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list NFS shares: %w", err)
	}

	var shares []NFSShare
	if err := json.Unmarshal(respBody, &shares); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return shares, nil
}

// GetNFSShare gets a specific NFS share by ID
func (c *Client) GetNFSShare(id int) (*NFSShare, error) {
	if id <= 0 {
		return nil, fmt.Errorf("invalid NFS share ID: %d", id)
	}

	path := fmt.Sprintf("/api/v2.0/sharing/nfs/id/%d", id)
	respBody, err := c.httpClient.Do("GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get NFS share: %w", err)
	}

	var share NFSShare
	if err := json.Unmarshal(respBody, &share); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &share, nil
}

// ListPools lists all storage pools
func (c *Client) ListPools() ([]Pool, error) {
	respBody, err := c.httpClient.Do("GET", "/api/v2.0/pool", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list pools: %w", err)
	}

	var pools []Pool
	if err := json.Unmarshal(respBody, &pools); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return pools, nil
}

// GetPool gets a specific pool by ID
func (c *Client) GetPool(id int) (*Pool, error) {
	if id <= 0 {
		return nil, fmt.Errorf("invalid pool ID: %d", id)
	}

	path := fmt.Sprintf("/api/v2.0/pool/id/%d", id)
	respBody, err := c.httpClient.Do("GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get pool: %w", err)
	}

	var pool Pool
	if err := json.Unmarshal(respBody, &pool); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &pool, nil
}

// Ping tests connectivity to the TrueNAS API
func (c *Client) Ping() error {
	_, err := c.httpClient.Do("GET", "/api/v2.0/system/info", nil)
	if err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}
	return nil
}
