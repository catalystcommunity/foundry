package openbao

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/container"
	"github.com/catalystcommunity/foundry/v1/internal/systemd"
)

// Install installs OpenBAO as a containerized systemd service
func (c *Component) Install(ctx context.Context, cfg component.ComponentConfig) error {
	// Extract SSH connection from config
	conn, ok := cfg["host"].(container.SSHExecutor)
	if !ok {
		return fmt.Errorf("SSH connection not provided in config\n\nThis is a bug - the install command should provide a connection")
	}

	// Parse OpenBAO-specific config
	openbaoCfg, err := parseConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate config
	if err := openbaoCfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Create container runtime client
	var runtime container.Runtime
	if openbaoCfg.ContainerRuntime == "podman" {
		runtime = container.NewPodmanRuntime(conn)
	} else {
		runtime = container.NewDockerRuntime(conn)
	}

	// Verify runtime is available
	if !runtime.IsAvailable() {
		return fmt.Errorf("%s runtime is not available on the host", runtime.Name())
	}

	// Create data and config directories
	if err := createDirectories(conn, openbaoCfg); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Generate and write OpenBAO config file
	if err := writeConfigFile(conn, openbaoCfg); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// Pull OpenBAO container image
	image := fmt.Sprintf("quay.io/openbao/openbao:%s", openbaoCfg.Version)
	if err := runtime.Pull(image); err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	// Detect container runtime path
	runtimePath, err := detectRuntimePath(conn, runtime.Name())
	if err != nil {
		return fmt.Errorf("failed to detect container runtime path: %w", err)
	}

	// Create systemd service
	if err := createSystemdService(conn, openbaoCfg, runtime.Name(), runtimePath); err != nil {
		return fmt.Errorf("failed to create systemd service: %w", err)
	}

	// Enable and start service
	if err := systemd.EnableService(conn, "openbao"); err != nil {
		return fmt.Errorf("failed to enable service: %w", err)
	}

	if err := systemd.StartService(conn, "openbao"); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	// Wait for service to be ready
	if err := waitForReady(ctx, conn); err != nil {
		return fmt.Errorf("service failed to become ready: %w", err)
	}

	// Initialize and unseal OpenBAO
	// Extract cluster name and keys directory from config
	clusterName, _ := cfg["cluster_name"].(string)
	if clusterName == "" {
		clusterName = "default"
	}

	keysDir, _ := cfg["keys_dir"].(string)
	if keysDir == "" {
		return fmt.Errorf("keys_dir not provided in config - required for saving OpenBAO keys")
	}

	// Get the API URL from config (should be passed by install command)
	apiURL, _ := cfg["api_url"].(string)
	if apiURL == "" {
		// Fallback: construct from address if it's already a full URL
		apiURL = openbaoCfg.Address
		// If it doesn't start with http, prepend it
		if apiURL[:4] != "http" {
			return fmt.Errorf("api_url not provided in config - required for OpenBAO client")
		}
	}

	if err := initializeAndUnseal(ctx, apiURL, keysDir, clusterName); err != nil {
		return fmt.Errorf("failed to initialize and unseal OpenBAO: %w", err)
	}

	return nil
}

// parseConfig converts generic component.ComponentConfig to OpenBAO-specific Config
func parseConfig(cfg component.ComponentConfig) (*Config, error) {
	openbaoCfg := DefaultConfig()

	// Override defaults with provided config
	if version, ok := cfg["version"].(string); ok {
		openbaoCfg.Version = version
	}
	if dataPath, ok := cfg["data_path"].(string); ok {
		openbaoCfg.DataPath = dataPath
	}
	if configPath, ok := cfg["config_path"].(string); ok {
		openbaoCfg.ConfigPath = configPath
	}
	if address, ok := cfg["address"].(string); ok {
		openbaoCfg.Address = address
	}
	if runtime, ok := cfg["container_runtime"].(string); ok {
		openbaoCfg.ContainerRuntime = runtime
	}

	return openbaoCfg, nil
}

// createDirectories creates the necessary directories on the remote host
func createDirectories(conn container.SSHExecutor, cfg *Config) error {
	dirs := []string{cfg.DataPath, cfg.ConfigPath}
	for _, dir := range dirs {
		// Create directory, set ownership to foundrysys (374:374), and set permissions
		cmd := fmt.Sprintf("sudo mkdir -p %s && sudo chown 374:374 %s && sudo chmod 755 %s", dir, dir, dir)
		if _, err := conn.Execute(cmd); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

// writeConfigFile writes the OpenBAO configuration to the remote host
func writeConfigFile(conn container.SSHExecutor, cfg *Config) error {
	configContent, err := GenerateConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to generate config: %w", err)
	}

	configPath := fmt.Sprintf("%s/config.hcl", cfg.ConfigPath)

	// Write the config file
	writeCmd := fmt.Sprintf("sudo tee %s > /dev/null << 'EOF'\n%s\nEOF", configPath, configContent)
	if _, err := conn.Execute(writeCmd); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// Set ownership to foundrysys
	chownCmd := fmt.Sprintf("sudo chown 374:374 %s", configPath)
	if _, err := conn.Execute(chownCmd); err != nil {
		return fmt.Errorf("failed to set config file ownership: %w", err)
	}

	return nil
}

// detectRuntimePath finds the actual path to the container runtime executable
func detectRuntimePath(conn container.SSHExecutor, runtimeType string) (string, error) {
	cmd := fmt.Sprintf("which %s", runtimeType)
	output, err := conn.Execute(cmd)
	if err != nil {
		return "", fmt.Errorf("failed to find %s: %w", runtimeType, err)
	}
	return strings.TrimSpace(output), nil
}

// createSystemdService creates and installs the systemd service unit
func createSystemdService(conn container.SSHExecutor, cfg *Config, runtimeType string, runtimePath string) error {
	unit := systemd.ContainerUnitFile(
		"openbao",
		"OpenBAO Secret Management",
		buildExecStart(cfg, runtimePath), // Foreground mode - systemd best practice
	)

	// Clean up any existing container before starting
	unit.ExecStartPre = fmt.Sprintf("-%s rm -f openbao", runtimePath)

	// No ExecStop - systemd sends SIGTERM to docker process which forwards to container
	// Using explicit stop can hit AppArmor issues on Ubuntu/Debian

	// Clean up the container after stopping
	unit.ExecStopPost = fmt.Sprintf("-%s rm -f openbao", runtimePath)

	// Give container time to gracefully shutdown
	unit.TimeoutStopSec = 30

	if err := systemd.CreateService(conn, "openbao", unit); err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	return nil
}

// buildExecStart builds the ExecStart command for the systemd service (foreground mode)
func buildExecStart(cfg *Config, runtimePath string) string {
	image := fmt.Sprintf("quay.io/openbao/openbao:%s", cfg.Version)

	parts := []string{
		fmt.Sprintf("%s run", runtimePath),
		// Note: No --rm flag - systemd manages the container lifecycle
		// Foreground mode (no -d) - systemd best practice
		"--name openbao",
		"--user 374:374", // Run as foundrysys user
		// Disable AppArmor - nerdctl-default profile blocks runc signal operations
		"--security-opt apparmor=unconfined",
		fmt.Sprintf("-p %s:8200", strings.Split(cfg.Address, ":")[1]),
		fmt.Sprintf("-v %s:/vault/data", cfg.DataPath),
		fmt.Sprintf("-v %s:/vault/config", cfg.ConfigPath),
		"--cap-add=IPC_LOCK",
		image,
		"server",
		"-config=/vault/config/config.hcl",
	}

	return strings.Join(parts, " ")
}

// waitForReady waits for OpenBAO to be ready to accept connections
func waitForReady(ctx context.Context, conn container.SSHExecutor) error {
	// Check if service is running
	status, err := systemd.GetServiceStatus(conn, "openbao")
	if err != nil {
		return fmt.Errorf("failed to get service status: %w", err)
	}

	if !status.Active {
		return fmt.Errorf("service is not active: %s", status.ActiveState)
	}

	// TODO: Add health check via API (requires client implementation)
	// For now, just verify the service is active

	return nil
}

// initializeAndUnseal initializes OpenBAO and unseals it
// Uses 5 key shares with a threshold of 3 for proper key rotation capability
func initializeAndUnseal(ctx context.Context, apiURL string, keysDir string, clusterName string) error {
	// Create client (no token needed for initialization/unsealing)
	client := NewClient(apiURL, "")

	// Wait for OpenBAO API to become reachable
	fmt.Printf("Waiting for OpenBAO API to become ready")

	// Start a goroutine to show progress dots
	done := make(chan bool)
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				fmt.Printf(".")
			}
		}
	}()

	err := client.WaitForHealthy(ctx, 20*time.Second)
	done <- true

	if err != nil {
		fmt.Printf(" ✗\n")
		return fmt.Errorf("failed to connect to OpenBAO API: %w", err)
	}
	fmt.Printf(" ✓\n")

	// Check if already initialized
	if KeyMaterialExists(keysDir, clusterName) {
		// Keys already exist, load and unseal
		material, err := LoadKeyMaterial(keysDir, clusterName)
		if err != nil {
			return fmt.Errorf("failed to load existing keys: %w", err)
		}

		if err := client.UnsealWithKeys(ctx, material.UnsealKeys); err != nil {
			return fmt.Errorf("failed to unseal with existing keys: %w", err)
		}

		fmt.Printf("✓ OpenBAO unsealed with existing keys\n")
		return nil
	}

	// Check if already initialized (but keys not saved)
	initialized, err := client.VerifyInitialized(ctx)
	if err != nil {
		return fmt.Errorf("failed to check initialization status: %w", err)
	}

	if initialized {
		return fmt.Errorf("OpenBAO is already initialized but keys not found - manual recovery required")
	}

	// Initialize with 5 shares, threshold 3
	const (
		shares    = 5
		threshold = 3
	)

	fmt.Printf("Initializing OpenBAO (%d shares, threshold %d)...\n", shares, threshold)
	initResp, err := client.Initialize(ctx, shares, threshold)
	if err != nil {
		return fmt.Errorf("failed to initialize: %w", err)
	}

	// Save key material
	material := &KeyMaterial{
		RootToken:  initResp.RootToken,
		UnsealKeys: initResp.Keys,
		Shares:     shares,
		Threshold:  threshold,
	}

	if err := SaveKeyMaterial(keysDir, clusterName, material); err != nil {
		return fmt.Errorf("failed to save keys: %w", err)
	}

	fmt.Printf("✓ OpenBAO initialized\n")
	fmt.Printf("✓ Keys saved to %s/%s/keys.json\n", keysDir, clusterName)
	fmt.Printf("\n⚠ IMPORTANT: Root token and unseal keys have been saved securely!\n")
	fmt.Printf("   Location: %s/%s/keys.json\n", keysDir, clusterName)
	fmt.Printf("   Generated: %d keys (need %d to unseal)\n", shares, threshold)
	fmt.Printf("   Permissions: 600 (owner read/write only)\n")
	fmt.Printf("\n   To view the keys (if needed):\n")
	fmt.Printf("   cat %s/%s/keys.json\n", keysDir, clusterName)

	// Unseal with the keys we just generated
	if err := client.UnsealWithKeys(ctx, initResp.Keys); err != nil {
		return fmt.Errorf("failed to unseal after initialization: %w", err)
	}

	fmt.Printf("✓ OpenBAO unsealed\n")

	// Enable KV v2 secrets engine at foundry-core mount
	// Create authenticated client with root token
	fmt.Printf("Setting up secrets engine...\n")
	authenticatedClient := NewClient(apiURL, initResp.RootToken)
	if err := authenticatedClient.EnableKVv2Engine(ctx, "foundry-core"); err != nil {
		return fmt.Errorf("failed to enable KV v2 secrets engine: %w", err)
	}
	fmt.Printf("✓ Secrets engine configured\n")

	return nil
}
