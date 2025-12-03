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

// GetSystemInfo returns TrueNAS system information
func (c *Client) GetSystemInfo() (*SystemInfo, error) {
	respBody, err := c.httpClient.Do("GET", "/api/v2.0/system/info", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get system info: %w", err)
	}

	var info SystemInfo
	if err := json.Unmarshal(respBody, &info); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &info, nil
}

// ListDisks lists all disks
func (c *Client) ListDisks() ([]Disk, error) {
	respBody, err := c.httpClient.Do("GET", "/api/v2.0/disk", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list disks: %w", err)
	}

	var disks []Disk
	if err := json.Unmarshal(respBody, &disks); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return disks, nil
}

// GetUnusedDisks returns disks not assigned to any pool
func (c *Client) GetUnusedDisks() ([]Disk, error) {
	disks, err := c.ListDisks()
	if err != nil {
		return nil, err
	}

	unused := make([]Disk, 0)
	for _, disk := range disks {
		if disk.Pool == "" {
			unused = append(unused, disk)
		}
	}

	return unused, nil
}

// CreatePool creates a new storage pool
func (c *Client) CreatePool(config PoolCreateConfig) (*Pool, error) {
	if config.Name == "" {
		return nil, fmt.Errorf("pool name cannot be empty")
	}
	if len(config.Topology.Data) == 0 {
		return nil, fmt.Errorf("pool must have at least one data vdev")
	}

	respBody, err := c.httpClient.Do("POST", "/api/v2.0/pool", config)
	if err != nil {
		return nil, fmt.Errorf("failed to create pool: %w", err)
	}

	var pool Pool
	if err := json.Unmarshal(respBody, &pool); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &pool, nil
}

// ListServices lists all services
func (c *Client) ListServices() ([]Service, error) {
	respBody, err := c.httpClient.Do("GET", "/api/v2.0/service", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	var services []Service
	if err := json.Unmarshal(respBody, &services); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return services, nil
}

// GetService gets a specific service by name
func (c *Client) GetService(name string) (*Service, error) {
	services, err := c.ListServices()
	if err != nil {
		return nil, err
	}

	for _, svc := range services {
		if svc.Service == name {
			return &svc, nil
		}
	}

	return nil, fmt.Errorf("service %q not found", name)
}

// StartService starts a service
func (c *Client) StartService(name string) error {
	body := map[string]string{"service": name}
	_, err := c.httpClient.Do("POST", "/api/v2.0/service/start", body)
	if err != nil {
		return fmt.Errorf("failed to start service %s: %w", name, err)
	}
	return nil
}

// StopService stops a service
func (c *Client) StopService(name string) error {
	body := map[string]string{"service": name}
	_, err := c.httpClient.Do("POST", "/api/v2.0/service/stop", body)
	if err != nil {
		return fmt.Errorf("failed to stop service %s: %w", name, err)
	}
	return nil
}

// EnableService enables a service to start on boot
func (c *Client) EnableService(name string) error {
	// First get the service ID
	svc, err := c.GetService(name)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("/api/v2.0/service/id/%d", svc.ID)
	config := ServiceUpdateConfig{Enable: true}
	_, err = c.httpClient.Do("PUT", path, config)
	if err != nil {
		return fmt.Errorf("failed to enable service %s: %w", name, err)
	}
	return nil
}

// EnsureServiceRunning ensures a service is enabled and running
func (c *Client) EnsureServiceRunning(name string) error {
	svc, err := c.GetService(name)
	if err != nil {
		return err
	}

	// Enable if not enabled
	if !svc.Enable {
		if err := c.EnableService(name); err != nil {
			return err
		}
	}

	// Start if not running
	if svc.State != "RUNNING" {
		if err := c.StartService(name); err != nil {
			return err
		}
	}

	return nil
}

// ListISCSIPortals lists all iSCSI portals
func (c *Client) ListISCSIPortals() ([]ISCSIPortal, error) {
	respBody, err := c.httpClient.Do("GET", "/api/v2.0/iscsi/portal", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list iSCSI portals: %w", err)
	}

	var portals []ISCSIPortal
	if err := json.Unmarshal(respBody, &portals); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return portals, nil
}

// CreateISCSIPortal creates an iSCSI portal
func (c *Client) CreateISCSIPortal(config ISCSIPortalConfig) (*ISCSIPortal, error) {
	if len(config.Listen) == 0 {
		return nil, fmt.Errorf("portal must have at least one listen address")
	}

	respBody, err := c.httpClient.Do("POST", "/api/v2.0/iscsi/portal", config)
	if err != nil {
		return nil, fmt.Errorf("failed to create iSCSI portal: %w", err)
	}

	var portal ISCSIPortal
	if err := json.Unmarshal(respBody, &portal); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &portal, nil
}

// ListISCSIInitiators lists all iSCSI initiator groups
func (c *Client) ListISCSIInitiators() ([]ISCSIInitiator, error) {
	respBody, err := c.httpClient.Do("GET", "/api/v2.0/iscsi/initiator", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list iSCSI initiators: %w", err)
	}

	var initiators []ISCSIInitiator
	if err := json.Unmarshal(respBody, &initiators); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return initiators, nil
}

// CreateISCSIInitiator creates an iSCSI initiator group
func (c *Client) CreateISCSIInitiator(config ISCSIInitiatorConfig) (*ISCSIInitiator, error) {
	respBody, err := c.httpClient.Do("POST", "/api/v2.0/iscsi/initiator", config)
	if err != nil {
		return nil, fmt.Errorf("failed to create iSCSI initiator: %w", err)
	}

	var initiator ISCSIInitiator
	if err := json.Unmarshal(respBody, &initiator); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &initiator, nil
}

// ListISCSITargets lists all iSCSI targets
func (c *Client) ListISCSITargets() ([]ISCSITarget, error) {
	respBody, err := c.httpClient.Do("GET", "/api/v2.0/iscsi/target", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list iSCSI targets: %w", err)
	}

	var targets []ISCSITarget
	if err := json.Unmarshal(respBody, &targets); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return targets, nil
}

// CreateISCSITarget creates an iSCSI target
func (c *Client) CreateISCSITarget(config ISCSITargetConfig) (*ISCSITarget, error) {
	if config.Name == "" {
		return nil, fmt.Errorf("target name cannot be empty")
	}

	respBody, err := c.httpClient.Do("POST", "/api/v2.0/iscsi/target", config)
	if err != nil {
		return nil, fmt.Errorf("failed to create iSCSI target: %w", err)
	}

	var target ISCSITarget
	if err := json.Unmarshal(respBody, &target); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &target, nil
}

// DeleteISCSITarget deletes an iSCSI target
func (c *Client) DeleteISCSITarget(id int) error {
	path := fmt.Sprintf("/api/v2.0/iscsi/target/id/%d", id)
	_, err := c.httpClient.Do("DELETE", path, nil)
	if err != nil {
		return fmt.Errorf("failed to delete iSCSI target: %w", err)
	}
	return nil
}

// ListISCSIExtents lists all iSCSI extents
func (c *Client) ListISCSIExtents() ([]ISCSIExtent, error) {
	respBody, err := c.httpClient.Do("GET", "/api/v2.0/iscsi/extent", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list iSCSI extents: %w", err)
	}

	var extents []ISCSIExtent
	if err := json.Unmarshal(respBody, &extents); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return extents, nil
}

// CreateISCSIExtent creates an iSCSI extent
func (c *Client) CreateISCSIExtent(config ISCSIExtentConfig) (*ISCSIExtent, error) {
	if config.Name == "" {
		return nil, fmt.Errorf("extent name cannot be empty")
	}

	respBody, err := c.httpClient.Do("POST", "/api/v2.0/iscsi/extent", config)
	if err != nil {
		return nil, fmt.Errorf("failed to create iSCSI extent: %w", err)
	}

	var extent ISCSIExtent
	if err := json.Unmarshal(respBody, &extent); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &extent, nil
}

// DeleteISCSIExtent deletes an iSCSI extent
func (c *Client) DeleteISCSIExtent(id int, removeFile bool) error {
	path := fmt.Sprintf("/api/v2.0/iscsi/extent/id/%d", id)
	body := map[string]bool{"remove": removeFile}
	_, err := c.httpClient.Do("DELETE", path, body)
	if err != nil {
		return fmt.Errorf("failed to delete iSCSI extent: %w", err)
	}
	return nil
}

// ListISCSITargetExtents lists all target-to-extent mappings
func (c *Client) ListISCSITargetExtents() ([]ISCSITargetExtent, error) {
	respBody, err := c.httpClient.Do("GET", "/api/v2.0/iscsi/targetextent", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list iSCSI target extents: %w", err)
	}

	var mappings []ISCSITargetExtent
	if err := json.Unmarshal(respBody, &mappings); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return mappings, nil
}

// CreateISCSITargetExtent creates a target-to-extent mapping
func (c *Client) CreateISCSITargetExtent(config ISCSITargetExtentConfig) (*ISCSITargetExtent, error) {
	respBody, err := c.httpClient.Do("POST", "/api/v2.0/iscsi/targetextent", config)
	if err != nil {
		return nil, fmt.Errorf("failed to create iSCSI target extent: %w", err)
	}

	var mapping ISCSITargetExtent
	if err := json.Unmarshal(respBody, &mapping); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &mapping, nil
}

// DeleteISCSITargetExtent deletes a target-to-extent mapping
func (c *Client) DeleteISCSITargetExtent(id int) error {
	path := fmt.Sprintf("/api/v2.0/iscsi/targetextent/id/%d", id)
	_, err := c.httpClient.Do("DELETE", path, nil)
	if err != nil {
		return fmt.Errorf("failed to delete iSCSI target extent: %w", err)
	}
	return nil
}

// GetISCSIGlobalConfig gets the iSCSI global configuration
func (c *Client) GetISCSIGlobalConfig() (*ISCSIGlobalConfig, error) {
	respBody, err := c.httpClient.Do("GET", "/api/v2.0/iscsi/global", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get iSCSI global config: %w", err)
	}

	var config ISCSIGlobalConfig
	if err := json.Unmarshal(respBody, &config); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &config, nil
}
