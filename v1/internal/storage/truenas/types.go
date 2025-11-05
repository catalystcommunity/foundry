package truenas

import (
	"time"
)

// Client represents a TrueNAS API client
type Client struct {
	baseURL string
	apiKey  string
	// httpClient is exposed for testing
	httpClient HTTPClient
}

// HTTPClient interface for HTTP operations (allows mocking)
type HTTPClient interface {
	Do(method, path string, body interface{}) ([]byte, error)
}

// Dataset represents a TrueNAS dataset
type Dataset struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Pool       string            `json:"pool"`
	Type       string            `json:"type"`
	Mountpoint string            `json:"mountpoint"`
	Available  int64             `json:"available"`
	Used       int64             `json:"used"`
	Comments   string            `json:"comments"`
	Properties map[string]string `json:"properties,omitempty"`
}

// DatasetConfig represents configuration for creating a dataset
type DatasetConfig struct {
	Name       string            `json:"name"`
	Type       string            `json:"type"` // FILESYSTEM or VOLUME
	Comments   string            `json:"comments,omitempty"`
	Properties map[string]string `json:"properties,omitempty"`
}

// NFSShare represents a TrueNAS NFS share
type NFSShare struct {
	ID        int      `json:"id"`
	Path      string   `json:"path"`
	Comment   string   `json:"comment"`
	Networks  []string `json:"networks"`
	Hosts     []string `json:"hosts"`
	Enabled   bool     `json:"enabled"`
	ReadOnly  bool     `json:"ro"`
	MapAllUID int      `json:"mapall_user,omitempty"`
	MapAllGID int      `json:"mapall_group,omitempty"`
}

// NFSConfig represents configuration for creating an NFS share
type NFSConfig struct {
	Path      string   `json:"path"`
	Comment   string   `json:"comment,omitempty"`
	Networks  []string `json:"networks,omitempty"`
	Hosts     []string `json:"hosts,omitempty"`
	ReadOnly  bool     `json:"ro"`
	MapAllUID int      `json:"mapall_user,omitempty"`
	MapAllGID int      `json:"mapall_group,omitempty"`
}

// Pool represents a TrueNAS storage pool
type Pool struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Guid      string    `json:"guid"`
	Status    string    `json:"status"`
	Healthy   bool      `json:"healthy"`
	Size      int64     `json:"size"`
	Allocated int64     `json:"allocated"`
	Free      int64     `json:"free"`
	Topology  Topology  `json:"topology"`
	ScanStats ScanStats `json:"scan"`
}

// Topology represents pool topology
type Topology struct {
	Data   []VDev `json:"data"`
	Cache  []VDev `json:"cache,omitempty"`
	Log    []VDev `json:"log,omitempty"`
	Spare  []VDev `json:"spare,omitempty"`
	Special []VDev `json:"special,omitempty"`
}

// VDev represents a virtual device
type VDev struct {
	Type     string  `json:"type"`
	Children []Disk  `json:"children,omitempty"`
	Status   string  `json:"status"`
	Stats    Stats   `json:"stats"`
}

// Disk represents a physical disk
type Disk struct {
	Type   string `json:"type"`
	Path   string `json:"path"`
	Status string `json:"status"`
	Stats  Stats  `json:"stats"`
}

// Stats represents device statistics
type Stats struct {
	ReadErrors  int64 `json:"read_errors"`
	WriteErrors int64 `json:"write_errors"`
	CheckErrors int64 `json:"checksum_errors"`
}

// ScanStats represents pool scan statistics
type ScanStats struct {
	Function   string    `json:"function"`
	State      string    `json:"state"`
	StartTime  time.Time `json:"start_time,omitempty"`
	EndTime    time.Time `json:"end_time,omitempty"`
	Percentage float64   `json:"percentage"`
	BytesIssued int64    `json:"bytes_issued"`
	BytesToExamine int64  `json:"bytes_to_examine"`
}

// APIError represents a TrueNAS API error response
type APIError struct {
	ErrorMsg  string `json:"error"`
	Message   string `json:"message"`
	Traceback string `json:"traceback,omitempty"`
	ErrorCode int    `json:"errcode,omitempty"`
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.ErrorMsg
}
