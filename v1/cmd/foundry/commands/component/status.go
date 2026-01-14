package component

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/component/statushelpers"
	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/systemd"
	"github.com/urfave/cli/v3"
)

// StatusCommand checks the status of a component
var StatusCommand = &cli.Command{
	Name:      "status",
	Usage:     "Check the status of a component",
	ArgsUsage: "<name>",
	Description: `Checks the installation and health status of a component.

Examples:
  foundry component status openbao
  foundry component status dns
  foundry component status zot
  foundry component status k3s`,
	Action: runStatus,
}

func runStatus(ctx context.Context, cmd *cli.Command) error {
	// Get component name from arguments
	if cmd.Args().Len() == 0 {
		return fmt.Errorf("component name required\n\nUsage: foundry component status <name>")
	}

	name := cmd.Args().Get(0)

	// Get component from registry (just to verify it exists)
	comp := component.Get(name)
	if comp == nil {
		return component.ErrComponentNotFound(name)
	}

	// Load config using --config flag
	configPath, err := config.FindConfig(cmd.String("config"))
	if err != nil {
		return fmt.Errorf("failed to find config: %w", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Printf("Checking status of component: %s\n\n", name)

	// Check status based on component name
	var status *component.ComponentStatus

	switch name {
	case "openbao":
		status, err = CheckOpenBAOStatus(ctx, cfg)
	case "dns", "powerdns":
		status, err = CheckDNSStatus(ctx, cfg)
	case "zot":
		status, err = CheckZotStatus(ctx, cfg)
	case "k3s", "kubernetes":
		status, err = CheckK3sStatus(ctx, cfg)
	default:
		return fmt.Errorf("status checking not implemented for component: %s", name)
	}

	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	// Print status
	fmt.Printf("Component: %s\n", name)
	fmt.Printf("Installed: %v\n", status.Installed)
	if status.Version != "" {
		fmt.Printf("Version:   %s\n", status.Version)
	}
	fmt.Printf("Healthy:   %v\n", status.Healthy)
	if status.Message != "" {
		fmt.Printf("Message:   %s\n", status.Message)
	}

	// Exit with error if not healthy
	if status.Installed && !status.Healthy {
		return fmt.Errorf("component %s is not healthy", name)
	}

	return nil
}

// CheckOpenBAOStatus checks the OpenBAO component status
func CheckOpenBAOStatus(ctx context.Context, cfg *config.Config) (*component.ComponentStatus, error) {
	// Get OpenBAO host
	h, err := cfg.GetPrimaryOpenBAOHost()
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("OpenBAO host not configured: %v", err),
		}, nil
	}

	// Connect to host
	_, err = statushelpers.FindHostByIP(h.Address)
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to find host: %v", err),
		}, nil
	}

	configDir, err := config.GetConfigDir()
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to get config directory: %v", err),
		}, nil
	}

	conn, err := statushelpers.ConnectToHost(h, configDir, cfg.Cluster.Name)
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to connect to host: %v", err),
		}, nil
	}
	defer conn.Close()

	// Check systemd service status
	executor := &statushelpers.SSHExecutorAdapter{Conn: conn}
	svcStatus, err := systemd.GetServiceStatus(executor, "openbao")
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to get service status: %v", err),
		}, nil
	}

	if !svcStatus.Loaded {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   "service not installed",
		}, nil
	}

	// Check health
	healthy := svcStatus.Active && svcStatus.Running
	message := "service running"
	version := ""

	if healthy {
		healthJSON, err := conn.Exec("curl -s http://localhost:8200/v1/sys/health")
		if err == nil && healthJSON.ExitCode == 0 {
			var healthData struct {
				Initialized bool   `json:"initialized"`
				Sealed      bool   `json:"sealed"`
				Version     string `json:"version"`
			}
			if err := json.Unmarshal([]byte(healthJSON.Stdout), &healthData); err == nil {
				version = healthData.Version
				if !healthData.Initialized {
					healthy = false
					message = "not initialized"
				} else if healthData.Sealed {
					healthy = false
					message = "sealed"
				} else {
					message = "healthy (initialized, unsealed)"
				}
			}
		} else {
			healthy = false
			message = "API not responding"
		}
	} else {
		message = fmt.Sprintf("service state: %s, sub-state: %s", svcStatus.ActiveState, svcStatus.SubState)
	}

	return &component.ComponentStatus{
		Installed: true,
		Version:   version,
		Healthy:   healthy,
		Message:   message,
	}, nil
}

// CheckDNSStatus checks the DNS component status
func CheckDNSStatus(ctx context.Context, cfg *config.Config) (*component.ComponentStatus, error) {
	// Get DNS host
	h, err := cfg.GetPrimaryDNSHost()
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("DNS host not configured: %v", err),
		}, nil
	}

	// Verify host exists in registry
	_, err = statushelpers.FindHostByIP(h.Address)
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to find host: %v", err),
		}, nil
	}

	configDir, err := config.GetConfigDir()
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to get config directory: %v", err),
		}, nil
	}

	conn, err := statushelpers.ConnectToHost(h, configDir, cfg.Cluster.Name)
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to connect to host: %v", err),
		}, nil
	}
	defer conn.Close()

	// Check both auth and recursor services
	executor := &statushelpers.SSHExecutorAdapter{Conn: conn}

	authStatus, err := systemd.GetServiceStatus(executor, "powerdns-auth")
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to get auth service status: %v", err),
		}, nil
	}

	recursorStatus, err := systemd.GetServiceStatus(executor, "powerdns-recursor")
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to get recursor service status: %v", err),
		}, nil
	}

	if !authStatus.Loaded && !recursorStatus.Loaded {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   "services not installed",
		}, nil
	}

	// Check health of both services
	var messages []string
	allHealthy := true

	if authStatus.Loaded {
		if authStatus.Active && authStatus.Running {
			messages = append(messages, "auth: running")
		} else {
			allHealthy = false
			messages = append(messages, fmt.Sprintf("auth: %s/%s", authStatus.ActiveState, authStatus.SubState))
		}
	} else {
		allHealthy = false
		messages = append(messages, "auth: not installed")
	}

	if recursorStatus.Loaded {
		if recursorStatus.Active && recursorStatus.Running {
			messages = append(messages, "recursor: running")
		} else {
			allHealthy = false
			messages = append(messages, fmt.Sprintf("recursor: %s/%s", recursorStatus.ActiveState, recursorStatus.SubState))
		}
	} else {
		allHealthy = false
		messages = append(messages, "recursor: not installed")
	}

	return &component.ComponentStatus{
		Installed: authStatus.Loaded || recursorStatus.Loaded,
		Version:   "",
		Healthy:   allHealthy,
		Message:   strings.Join(messages, ", "),
	}, nil
}

// CheckZotStatus checks the Zot component status
func CheckZotStatus(ctx context.Context, cfg *config.Config) (*component.ComponentStatus, error) {
	// Get Zot host
	h, err := cfg.GetPrimaryZotHost()
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("Zot host not configured: %v", err),
		}, nil
	}

	// Verify host exists in registry
	_, err = statushelpers.FindHostByIP(h.Address)
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to find host: %v", err),
		}, nil
	}

	configDir, err := config.GetConfigDir()
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to get config directory: %v", err),
		}, nil
	}

	conn, err := statushelpers.ConnectToHost(h, configDir, cfg.Cluster.Name)
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to connect to host: %v", err),
		}, nil
	}
	defer conn.Close()

	// Check systemd service status
	executor := &statushelpers.SSHExecutorAdapter{Conn: conn}
	svcStatus, err := systemd.GetServiceStatus(executor, "foundry-zot")
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to get service status: %v", err),
		}, nil
	}

	if !svcStatus.Loaded {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   "service not installed",
		}, nil
	}

	// Check health
	healthy := svcStatus.Active && svcStatus.Running
	message := "service running"

	if healthy {
		result, err := conn.Exec("curl -s http://localhost:5000/v2/")
		if err == nil && result.ExitCode == 0 {
			message = "healthy (API responding)"
		} else {
			healthy = false
			message = "API not responding"
		}
	} else {
		message = fmt.Sprintf("service state: %s, sub-state: %s", svcStatus.ActiveState, svcStatus.SubState)
	}

	return &component.ComponentStatus{
		Installed: true,
		Version:   "",
		Healthy:   healthy,
		Message:   message,
	}, nil
}

// CheckK3sStatus checks the K3s component status
func CheckK3sStatus(ctx context.Context, cfg *config.Config) (*component.ComponentStatus, error) {
	// Get first cluster host
	clusterHosts := cfg.GetClusterHosts()
	if len(clusterHosts) == 0 {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   "no cluster hosts configured (no hosts with cluster-control-plane or cluster-worker roles)",
		}, nil
	}
	firstHost := clusterHosts[0]

	// Find and connect to host
	h, err := statushelpers.FindHostByHostname(firstHost.Hostname)
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to find host: %v", err),
		}, nil
	}

	configDir, err := config.GetConfigDir()
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to get config directory: %v", err),
		}, nil
	}

	conn, err := statushelpers.ConnectToHost(h, configDir, cfg.Cluster.Name)
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to connect to host: %v", err),
		}, nil
	}
	defer conn.Close()

	// Check systemd service status
	executor := &statushelpers.SSHExecutorAdapter{Conn: conn}
	svcStatus, err := systemd.GetServiceStatus(executor, "k3s")
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to get service status: %v", err),
		}, nil
	}

	if !svcStatus.Loaded {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   "service not installed",
		}, nil
	}

	// Check health
	healthy := svcStatus.Active && svcStatus.Running
	message := "service running"
	version := ""

	if healthy {
		// Get version
		versionResult, err := conn.Exec("k3s --version 2>&1 | head -n 1")
		if err == nil && versionResult.ExitCode == 0 {
			parts := strings.Fields(versionResult.Stdout)
			if len(parts) >= 3 {
				version = parts[2]
			}
		}

		// Try to get node count
		nodesResult, err := conn.Exec("sudo k3s kubectl get nodes --no-headers 2>/dev/null | wc -l")
		if err == nil && nodesResult.ExitCode == 0 {
			nodeCount := strings.TrimSpace(nodesResult.Stdout)
			if nodeCount != "0" && nodeCount != "" {
				message = fmt.Sprintf("healthy (%s nodes)", nodeCount)
			} else {
				message = "running (no nodes ready yet)"
			}
		} else {
			message = "running (kubectl not accessible)"
		}
	} else {
		message = fmt.Sprintf("service state: %s, sub-state: %s", svcStatus.ActiveState, svcStatus.SubState)
	}

	return &component.ComponentStatus{
		Installed: true,
		Version:   version,
		Healthy:   healthy,
		Message:   message,
	}, nil
}
