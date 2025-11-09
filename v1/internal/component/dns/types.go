package dns

import (
	"context"

	"github.com/catalystcommunity/foundry/v1/internal/component"
)

// Component implements the component.Component interface for PowerDNS.
type Component struct {
	name string
}

// NewComponent creates a new PowerDNS component instance.
func NewComponent() *Component {
	return &Component{
		name: "dns",
	}
}

// Name returns the component name.
func (c *Component) Name() string {
	return c.name
}

// Install installs PowerDNS as a containerized systemd service.
// Implementation is in install.go

// Upgrade upgrades the PowerDNS installation.
func (c *Component) Upgrade(ctx context.Context, cfg component.ComponentConfig) error {
	// Not implemented in Phase 2
	return nil
}

// Status returns the current status of PowerDNS.
func (c *Component) Status(ctx context.Context) (*component.ComponentStatus, error) {
	// Not implemented in Phase 2
	return &component.ComponentStatus{
		Installed: false,
		Version:   "unknown",
		Healthy:   false,
		Message:   "status check not implemented",
	}, nil
}

// Uninstall removes the PowerDNS installation.
func (c *Component) Uninstall(ctx context.Context) error {
	// Not implemented in Phase 2
	return nil
}

// Dependencies returns the list of components that DNS depends on.
func (c *Component) Dependencies() []string {
	return []string{"openbao"} // DNS depends on OpenBAO for API key storage
}

// Config type is generated from CSIL in types.gen.go
// This file extends the generated type with methods

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		ImageTag:   "49",
		Backend:    "sqlite",
		Forwarders: []string{"8.8.8.8", "1.1.1.1"},
		DataDir:    "/var/lib/powerdns",
		ConfigDir:  "/etc/powerdns",
	}
}

// Zone represents a DNS zone in PowerDNS.
type Zone struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"` // NATIVE, MASTER, SLAVE (PowerDNS uses "kind" not "type")
	Type   string `json:"type"` // For backward compatibility
	Serial uint32 `json:"serial"`
}

// Record represents a DNS record in PowerDNS.
type Record struct {
	Name    string
	Type    string // A, AAAA, CNAME, MX, TXT, NS, SOA, etc.
	Content string
	TTL     int
	Disabled bool
}

// RecordSet represents a set of DNS records (PowerDNS groups records by name+type).
type RecordSet struct {
	Name    string
	Type    string
	TTL     int
	Records []Record
}
