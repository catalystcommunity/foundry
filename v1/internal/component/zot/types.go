package zot

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/container"
	"github.com/catalystcommunity/foundry/v1/internal/systemd"
)

// Component implements the component.Component interface for Zot registry
type Component struct {
	conn container.SSHExecutor
}

// NewComponent creates a new Zot component instance
func NewComponent(conn container.SSHExecutor) *Component {
	return &Component{
		conn: conn,
	}
}

// Name returns the component name
func (c *Component) Name() string {
	return "zot"
}

// Install installs the Zot registry as a containerized systemd service
func (c *Component) Install(ctx context.Context, cfg component.ComponentConfig) error {
	config, err := ParseConfig(cfg)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	// Create container runtime (default to docker, but can be configured)
	var runtime container.Runtime
	if config.ContainerRuntime == "podman" {
		runtime = container.NewPodmanRuntime(c.conn)
	} else {
		runtime = container.NewDockerRuntime(c.conn)
	}

	// Verify runtime is available
	if !runtime.IsAvailable() {
		return fmt.Errorf("%s runtime is not available on the host", runtime.Name())
	}

	return Install(c.conn, runtime, config)
}

// Upgrade upgrades the Zot registry to a new version
func (c *Component) Upgrade(ctx context.Context, cfg component.ComponentConfig) error {
	return fmt.Errorf("upgrade not yet implemented")
}

// Status returns the current status of the Zot registry
func (c *Component) Status(ctx context.Context) (*component.ComponentStatus, error) {
	// Check if systemd service exists and is running
	status, err := systemd.GetServiceStatus(c.conn, "foundry-zot")
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to get service status: %v", err),
		}, nil
	}

	if !status.Loaded {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   "service not installed",
		}, nil
	}

	healthy := status.Active && status.Running
	message := "running"
	if !healthy {
		message = fmt.Sprintf("service state: %s, sub-state: %s", status.ActiveState, status.SubState)
	}

	return &component.ComponentStatus{
		Installed: true,
		Version:   "", // Could parse from container image tag
		Healthy:   healthy,
		Message:   message,
	}, nil
}

// Uninstall removes the Zot registry
func (c *Component) Uninstall(ctx context.Context) error {
	return fmt.Errorf("uninstall not yet implemented")
}

// Dependencies returns the list of components that Zot depends on
func (c *Component) Dependencies() []string {
	return []string{"openbao", "dns"} // Zot depends on OpenBAO for secrets and DNS for name resolution
}

// Config represents the Zot registry configuration
type Config struct {
	// Version is the Zot image tag
	Version string

	// DataDir is the path where Zot stores registry data
	DataDir string

	// ConfigDir is the path where Zot config.json is stored
	ConfigDir string

	// Port is the port Zot listens on
	Port int

	// ContainerRuntime is the runtime to use (docker or podman)
	ContainerRuntime string

	// StorageBackend is the storage configuration (optional)
	StorageBackend *StorageConfig

	// PullThroughCache enables pull-through caching for Docker Hub
	PullThroughCache bool

	// Auth configuration (optional, for future use)
	Auth *AuthConfig
}

// StorageConfig represents storage backend configuration
type StorageConfig struct {
	// Type is the storage backend type (e.g., "truenas")
	Type string

	// MountPath is where the storage is mounted on the host
	MountPath string
}

// AuthConfig represents authentication configuration
type AuthConfig struct {
	// Type is the auth type (e.g., "basic", "ldap", "oidc")
	Type string

	// Config is type-specific auth configuration
	Config map[string]interface{}
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Version:          "latest",
		DataDir:          "/var/lib/foundry-zot",
		ConfigDir:        "/etc/foundry-zot",
		Port:             5000,
		ContainerRuntime: "docker",
		PullThroughCache: true,
		Auth:             nil, // No auth by default for Phase 2
	}
}

// ParseConfig parses a ComponentConfig into a Zot Config
func ParseConfig(cfg component.ComponentConfig) (*Config, error) {
	config := DefaultConfig()

	if version, ok := cfg["version"].(string); ok {
		config.Version = version
	}

	if dataDir, ok := cfg["data_dir"].(string); ok {
		config.DataDir = dataDir
	}

	if configDir, ok := cfg["config_dir"].(string); ok {
		config.ConfigDir = configDir
	}

	if port, ok := cfg["port"].(int); ok {
		config.Port = port
	} else if portFloat, ok := cfg["port"].(float64); ok {
		config.Port = int(portFloat)
	}

	if pullThrough, ok := cfg["pull_through_cache"].(bool); ok {
		config.PullThroughCache = pullThrough
	}

	if runtime, ok := cfg["container_runtime"].(string); ok {
		config.ContainerRuntime = runtime
	}

	// Parse storage backend
	if storage, ok := cfg["storage"].(map[string]interface{}); ok {
		config.StorageBackend = &StorageConfig{}
		if storageType, ok := storage["type"].(string); ok {
			config.StorageBackend.Type = storageType
		}
		if mountPath, ok := storage["mount_path"].(string); ok {
			config.StorageBackend.MountPath = mountPath
		}
	}

	// Parse auth config (for future use)
	if auth, ok := cfg["auth"].(map[string]interface{}); ok {
		config.Auth = &AuthConfig{}
		if authType, ok := auth["type"].(string); ok {
			config.Auth.Type = authType
		}
		if authConfig, ok := auth["config"].(map[string]interface{}); ok {
			config.Auth.Config = authConfig
		}
	}

	return config, nil
}
