package dns

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is an HTTP API client for PowerDNS
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a new PowerDNS API client
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// doRequest performs an HTTP request with proper headers
func (c *Client) doRequest(method, path string, body interface{}) ([]byte, error) {
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

	// Set headers
	req.Header.Set("X-API-Key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// CreateZone creates a new DNS zone
func (c *Client) CreateZone(name, zoneType string) error {
	if name == "" {
		return fmt.Errorf("zone name cannot be empty")
	}
	if zoneType == "" {
		zoneType = "Native"
	}

	payload := map[string]interface{}{
		"name":        name,
		"kind":        zoneType,
		"nameservers": []string{},
	}

	_, err := c.doRequest("POST", "/api/v1/servers/localhost/zones", payload)
	return err
}

// DeleteZone deletes a DNS zone
func (c *Client) DeleteZone(name string) error {
	if name == "" {
		return fmt.Errorf("zone name cannot be empty")
	}

	_, err := c.doRequest("DELETE", fmt.Sprintf("/api/v1/servers/localhost/zones/%s", name), nil)
	return err
}

// ListZones lists all DNS zones
func (c *Client) ListZones() ([]Zone, error) {
	respBody, err := c.doRequest("GET", "/api/v1/servers/localhost/zones", nil)
	if err != nil {
		return nil, err
	}

	var zones []Zone
	if err := json.Unmarshal(respBody, &zones); err != nil {
		return nil, fmt.Errorf("failed to unmarshal zones: %w", err)
	}

	return zones, nil
}

// RRSet represents a Resource Record Set in PowerDNS API
type RRSet struct {
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	TTL        int       `json:"ttl"`
	ChangeType string    `json:"changetype"`
	Records    []RRecord `json:"records"`
}

// RRecord represents a single resource record
type RRecord struct {
	Content  string `json:"content"`
	Disabled bool   `json:"disabled"`
}

// AddRecord adds a DNS record to a zone
func (c *Client) AddRecord(zone, name, recordType, content string, ttl int) error {
	if zone == "" || name == "" || recordType == "" || content == "" {
		return fmt.Errorf("zone, name, type, and content are required")
	}
	if ttl <= 0 {
		ttl = 3600 // Default TTL
	}

	rrset := RRSet{
		Name:       name,
		Type:       recordType,
		TTL:        ttl,
		ChangeType: "REPLACE",
		Records: []RRecord{
			{
				Content:  content,
				Disabled: false,
			},
		},
	}

	payload := map[string]interface{}{
		"rrsets": []RRSet{rrset},
	}

	_, err := c.doRequest("PATCH", fmt.Sprintf("/api/v1/servers/localhost/zones/%s", zone), payload)
	return err
}

// DeleteRecord deletes a DNS record from a zone
func (c *Client) DeleteRecord(zone, name, recordType string) error {
	if zone == "" || name == "" || recordType == "" {
		return fmt.Errorf("zone, name, and type are required")
	}

	rrset := RRSet{
		Name:       name,
		Type:       recordType,
		ChangeType: "DELETE",
	}

	payload := map[string]interface{}{
		"rrsets": []RRSet{rrset},
	}

	_, err := c.doRequest("PATCH", fmt.Sprintf("/api/v1/servers/localhost/zones/%s", zone), payload)
	return err
}

// ListRecords lists all records in a zone
func (c *Client) ListRecords(zone string) ([]RecordSet, error) {
	if zone == "" {
		return nil, fmt.Errorf("zone name cannot be empty")
	}

	respBody, err := c.doRequest("GET", fmt.Sprintf("/api/v1/servers/localhost/zones/%s", zone), nil)
	if err != nil {
		return nil, err
	}

	var zoneData struct {
		RRSets []struct {
			Name    string `json:"name"`
			Type    string `json:"type"`
			TTL     int    `json:"ttl"`
			Records []struct {
				Content  string `json:"content"`
				Disabled bool   `json:"disabled"`
			} `json:"records"`
		} `json:"rrsets"`
	}

	if err := json.Unmarshal(respBody, &zoneData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal zone data: %w", err)
	}

	recordSets := make([]RecordSet, 0, len(zoneData.RRSets))
	for _, rrset := range zoneData.RRSets {
		records := make([]Record, 0, len(rrset.Records))
		for _, r := range rrset.Records {
			records = append(records, Record{
				Name:     rrset.Name,
				Type:     rrset.Type,
				Content:  r.Content,
				TTL:      rrset.TTL,
				Disabled: r.Disabled,
			})
		}

		recordSets = append(recordSets, RecordSet{
			Name:    rrset.Name,
			Type:    rrset.Type,
			TTL:     rrset.TTL,
			Records: records,
		})
	}

	return recordSets, nil
}
