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
	Type     string         `json:"type"`
	Children []TopologyDisk `json:"children,omitempty"`
	Status   string         `json:"status"`
	Stats    Stats          `json:"stats"`
}

// TopologyDisk represents a physical disk in pool topology
type TopologyDisk struct {
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

// SystemInfo represents TrueNAS system information
type SystemInfo struct {
	Version       string `json:"version"`
	Hostname      string `json:"hostname"`
	PhysicalMem   int64  `json:"physmem"`
	Model         string `json:"model"`
	Cores         int    `json:"cores"`
	LoadAvg       []float64 `json:"loadavg"`
	Uptime        string `json:"uptime"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	SystemProduct string `json:"system_product"`
	License       *struct {
		ContractType string `json:"contract_type"`
	} `json:"license,omitempty"`
}

// Disk represents a TrueNAS disk
type Disk struct {
	Identifier   string `json:"identifier"`
	Name         string `json:"name"`
	Subsystem    string `json:"subsystem"`
	Number       int    `json:"number"`
	Serial       string `json:"serial"`
	Size         int64  `json:"size"`
	Description  string `json:"description"`
	Model        string `json:"model"`
	Rotationrate *int   `json:"rotationrate,omitempty"`
	Type         string `json:"type"`
	Pool         string `json:"pool,omitempty"`
}

// Service represents a TrueNAS service
type Service struct {
	ID      int    `json:"id"`
	Service string `json:"service"`
	Enable  bool   `json:"enable"`
	State   string `json:"state"`
	Pids    []int  `json:"pids,omitempty"`
}

// ServiceUpdateConfig represents configuration for updating a service
type ServiceUpdateConfig struct {
	Enable bool `json:"enable"`
}

// PoolCreateConfig represents configuration for creating a pool
type PoolCreateConfig struct {
	Name       string              `json:"name"`
	Encryption bool                `json:"encryption"`
	Topology   PoolCreateTopology  `json:"topology"`
}

// PoolCreateTopology represents topology for pool creation
type PoolCreateTopology struct {
	Data   []PoolCreateVDev `json:"data"`
	Cache  []PoolCreateVDev `json:"cache,omitempty"`
	Log    []PoolCreateVDev `json:"log,omitempty"`
	Spare  []PoolCreateVDev `json:"spare,omitempty"`
	Special []PoolCreateVDev `json:"special,omitempty"`
}

// PoolCreateVDev represents a vdev for pool creation
type PoolCreateVDev struct {
	Type  string   `json:"type"` // STRIPE, MIRROR, RAIDZ1, RAIDZ2, RAIDZ3
	Disks []string `json:"disks"`
}

// ISCSIPortal represents an iSCSI portal
type ISCSIPortal struct {
	ID        int                  `json:"id"`
	Tag       int                  `json:"tag"`
	Comment   string               `json:"comment"`
	Listen    []ISCSIPortalListen  `json:"listen"`
}

// ISCSIPortalListen represents a portal listen address
type ISCSIPortalListen struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

// ISCSIPortalConfig represents configuration for creating an iSCSI portal
type ISCSIPortalConfig struct {
	Comment string               `json:"comment,omitempty"`
	Listen  []ISCSIPortalListen  `json:"listen"`
}

// ISCSIInitiator represents an iSCSI initiator group
type ISCSIInitiator struct {
	ID         int      `json:"id"`
	Tag        int      `json:"tag"`
	Initiators []string `json:"initiators"`
	Comment    string   `json:"comment"`
}

// ISCSIInitiatorConfig represents configuration for creating an initiator group
type ISCSIInitiatorConfig struct {
	Initiators []string `json:"initiators,omitempty"` // Empty means allow all
	Comment    string   `json:"comment,omitempty"`
}

// ISCSITarget represents an iSCSI target
type ISCSITarget struct {
	ID     int                  `json:"id"`
	Name   string               `json:"name"`
	Alias  string               `json:"alias,omitempty"`
	Mode   string               `json:"mode"`
	Groups []ISCSITargetGroup   `json:"groups"`
}

// ISCSITargetGroup represents a target group association
type ISCSITargetGroup struct {
	Portal     int  `json:"portal"`
	Initiator  int  `json:"initiator"`
	Auth       *int `json:"auth,omitempty"`
	AuthMethod string `json:"authmethod"`
}

// ISCSITargetConfig represents configuration for creating an iSCSI target
type ISCSITargetConfig struct {
	Name   string               `json:"name"`
	Alias  string               `json:"alias,omitempty"`
	Mode   string               `json:"mode,omitempty"` // ISCSI, FC, BOTH
	Groups []ISCSITargetGroup   `json:"groups,omitempty"`
}

// ISCSIExtent represents an iSCSI extent
type ISCSIExtent struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"` // DISK or FILE
	Disk        string `json:"disk,omitempty"`
	Path        string `json:"path,omitempty"`
	Filesize    int64  `json:"filesize,omitempty"`
	Blocksize   int    `json:"blocksize"`
	RPM         string `json:"rpm"`
	Enabled     bool   `json:"enabled"`
	Comment     string `json:"comment,omitempty"`
}

// ISCSIExtentConfig represents configuration for creating an iSCSI extent
type ISCSIExtentConfig struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // DISK or FILE
	Disk        string `json:"disk,omitempty"`
	Path        string `json:"path,omitempty"`
	Filesize    int64  `json:"filesize,omitempty"`
	Blocksize   int    `json:"blocksize,omitempty"`
	RPM         string `json:"rpm,omitempty"` // UNKNOWN, SSD, 5400, 7200, 10000, 15000
	Comment     string `json:"comment,omitempty"`
}

// ISCSITargetExtent represents a target-to-extent mapping
type ISCSITargetExtent struct {
	ID       int `json:"id"`
	Target   int `json:"target"`
	Extent   int `json:"extent"`
	LUNId    int `json:"lunid"`
}

// ISCSITargetExtentConfig represents configuration for creating a target-extent mapping
type ISCSITargetExtentConfig struct {
	Target int `json:"target"`
	Extent int `json:"extent"`
	LUNId  int `json:"lunid,omitempty"`
}

// ISCSIGlobalConfig represents iSCSI global configuration
type ISCSIGlobalConfig struct {
	Basename       string `json:"basename"`
	ISNSServers    []string `json:"isns_servers"`
	PoolAvailThreshold int `json:"pool_avail_threshold,omitempty"`
}
