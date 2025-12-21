package zot


import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/container"
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
	// Extract SSH connection from config
	conn, ok := cfg["host"].(container.SSHExecutor)
	if !ok {
		return fmt.Errorf("SSH connection not provided in config\n\nThis is a bug - the install command should provide a connection")
	}

	// Store connection for use by other methods (Status, Uninstall, etc.)
	c.conn = conn

	parsedConfig, err := ParseConfig(cfg)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	// Create container runtime (default to docker, but can be configured)
	var runtime container.Runtime
	if parsedConfig.ContainerRuntime == "podman" {
		runtime = container.NewPodmanRuntime(conn)
	} else {
		runtime = container.NewDockerRuntime(conn)
	}

	// Verify runtime is available
	if !runtime.IsAvailable() {
		return fmt.Errorf("%s runtime is not available on the host", runtime.Name())
	}

	return Install(conn, runtime, parsedConfig)
}

// Upgrade upgrades the Zot registry to a new version
func (c *Component) Upgrade(ctx context.Context, cfg component.ComponentConfig) error {
	return fmt.Errorf("upgrade not yet implemented")
}

// Status returns the current status of the Zot registry
func (c *Component) Status(ctx context.Context) (*component.ComponentStatus, error) {
	// Status checking is implemented in cmd/foundry/commands/component/status.go
	// to avoid import cycles with config/ssh/secrets packages
	return &component.ComponentStatus{
		Installed: false,
		Version:   "",
		Healthy:   false,
		Message:   "use 'foundry component status zot' command",
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

// Config, StorageConfig, and AuthConfig types are generated from CSIL in types.gen.go
// This file extends the generated types with methods

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

// ParsedConfig extends Config with non-generated fields
type ParsedConfig struct {
	*Config
	UpstreamCreds *UpstreamRegistryCredentials
}

// ParseConfig parses a ComponentConfig into a Zot Config
func ParseConfig(cfg component.ComponentConfig) (*ParsedConfig, error) {
	config := DefaultConfig()
	parsed := &ParsedConfig{Config: config}

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
		config.Port = int64(port)
	} else if portFloat, ok := cfg["port"].(float64); ok {
		config.Port = int64(portFloat)
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

	// Parse Docker Hub credentials for pull-through cache (avoids rate limiting)
	dockerHubUser, hasUser := cfg["docker_hub_username"].(string)
	dockerHubPass, hasPass := cfg["docker_hub_password"].(string)
	if hasUser && hasPass && dockerHubUser != "" && dockerHubPass != "" {
		parsed.UpstreamCreds = &UpstreamRegistryCredentials{
			DockerHubUsername: dockerHubUser,
			DockerHubPassword: dockerHubPass,
		}
	}

	return parsed, nil
}
