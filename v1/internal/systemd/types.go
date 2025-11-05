package systemd

import "time"

// UnitFile represents a systemd unit file configuration
type UnitFile struct {
	// Unit section
	Description string
	After       []string
	Requires    []string
	Wants       []string

	// Service section
	Type            string // simple, forking, oneshot, notify, etc.
	ExecStart       string
	ExecStop        string
	ExecReload      string
	Restart         string // always, on-failure, on-abnormal, etc.
	RestartSec      int    // seconds
	User            string
	Group           string
	Environment     map[string]string
	EnvironmentFile string
	WorkingDirectory string

	// Install section
	WantedBy []string
	RequiredBy []string
}

// ServiceStatus represents the status of a systemd service
type ServiceStatus struct {
	Name        string
	Loaded      bool
	Active      bool
	Running     bool
	Enabled     bool
	MainPID     int
	SubState    string // running, exited, dead, failed, etc.
	LoadState   string // loaded, not-found, bad-setting, error, masked
	ActiveState string // active, inactive, activating, deactivating, failed
	Since       time.Time
	Memory      uint64 // bytes
	Tasks       int
}

// DefaultUnitFile returns a UnitFile with sensible defaults
func DefaultUnitFile() *UnitFile {
	return &UnitFile{
		Type:       "simple",
		Restart:    "on-failure",
		RestartSec: 5,
		WantedBy:   []string{"multi-user.target"},
	}
}

// ContainerUnitFile returns a UnitFile configured for running containers
func ContainerUnitFile(name, description, execStart string) *UnitFile {
	return &UnitFile{
		Description: description,
		After:       []string{"network-online.target"},
		Wants:       []string{"network-online.target"},
		Type:        "simple",
		ExecStart:   execStart,
		Restart:     "always",
		RestartSec:  10,
		WantedBy:    []string{"multi-user.target"},
	}
}
