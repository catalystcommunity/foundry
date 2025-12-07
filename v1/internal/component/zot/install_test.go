package zot

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/container"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockExecutor implements ssh.Executor for testing
type mockExecutor struct {
	commands []string
	outputs  map[string]string
	errors   map[string]error
}

func newMockExecutor() *mockExecutor {
	m := &mockExecutor{
		commands: []string{},
		outputs:  make(map[string]string),
		errors:   make(map[string]error),
	}
	// Default response for runtime path detection
	m.outputs["which docker"] = "/usr/bin/docker"
	m.outputs["which podman"] = "/usr/bin/podman"
	return m
}

func (m *mockExecutor) Execute(cmd string) (string, error) {
	m.commands = append(m.commands, cmd)

	// Check for specific error first (exact match)
	for errorCmd, err := range m.errors {
		if strings.Contains(cmd, errorCmd) || cmd == errorCmd {
			return "", err
		}
	}

	// Check for specific output (exact match or contains)
	for outputCmd, output := range m.outputs {
		if cmd == outputCmd || strings.Contains(cmd, outputCmd) {
			return output, nil
		}
	}

	// Handle systemctl status queries (default)
	if strings.Contains(cmd, "systemctl show") {
		return "LoadState=loaded\nActiveState=active\nSubState=running", nil
	}

	return "", nil
}

func (m *mockExecutor) hasCommand(pattern string) bool {
	for _, cmd := range m.commands {
		if strings.Contains(cmd, pattern) {
			return true
		}
	}
	return false
}

// mockRuntime implements container.Runtime for testing
type mockRuntime struct {
	pulledImages []string
	pullError    error
}

func newMockRuntime() *mockRuntime {
	return &mockRuntime{
		pulledImages: []string{},
	}
}

func (m *mockRuntime) Name() string {
	return "docker"
}

func (m *mockRuntime) Pull(image string) error {
	if m.pullError != nil {
		return m.pullError
	}
	m.pulledImages = append(m.pulledImages, image)
	return nil
}

func (m *mockRuntime) Run(config container.RunConfig) (string, error) {
	return "container-id-123", nil
}

func (m *mockRuntime) Stop(containerID string, timeout time.Duration) error {
	return nil
}

func (m *mockRuntime) Remove(containerID string, force bool) error {
	return nil
}

func (m *mockRuntime) Inspect(containerID string) (*container.ContainerInfo, error) {
	return &container.ContainerInfo{
		ID:    containerID,
		Name:  "test-container",
		Image: "test-image",
		State: "running",
	}, nil
}

func (m *mockRuntime) List(all bool) ([]container.ContainerInfo, error) {
	return []container.ContainerInfo{}, nil
}

func (m *mockRuntime) IsAvailable() bool {
	return true
}

func TestInstall_Success(t *testing.T) {
	executor := newMockExecutor()
	runtime := newMockRuntime()
	cfg := DefaultConfig()

	err := Install(executor, runtime, cfg)
	require.NoError(t, err)

	// Verify directories were created
	assert.True(t, executor.hasCommand("mkdir -p /var/lib/foundry-zot"))
	assert.True(t, executor.hasCommand("mkdir -p /etc/foundry-zot"))

	// Verify config file was written
	assert.True(t, executor.hasCommand("sudo tee /etc/foundry-zot/config.json"))

	// Verify image was pulled
	assert.Contains(t, runtime.pulledImages, "ghcr.io/project-zot/zot:latest")

	// Verify systemd service was created
	assert.True(t, executor.hasCommand("sudo tee /etc/systemd/system/foundry-zot.service"))

	// Verify service was enabled and started
	assert.True(t, executor.hasCommand("sudo systemctl daemon-reload"))
	assert.True(t, executor.hasCommand("sudo systemctl enable foundry-zot"))
	assert.True(t, executor.hasCommand("sudo systemctl start foundry-zot"))

	// Verify status was checked
	assert.True(t, executor.hasCommand("systemctl show foundry-zot"))
}

func TestInstall_CustomConfig(t *testing.T) {
	executor := newMockExecutor()
	runtime := newMockRuntime()
	cfg := &Config{
		Version:          "v2.0.0",
		DataDir:          "/custom/data",
		ConfigDir:        "/custom/config",
		Port:             5050,
		PullThroughCache: true,
	}

	err := Install(executor, runtime, cfg)
	require.NoError(t, err)

	// Verify custom directories
	assert.True(t, executor.hasCommand("mkdir -p /custom/data"))
	assert.True(t, executor.hasCommand("mkdir -p /custom/config"))

	// Verify custom config path
	assert.True(t, executor.hasCommand("sudo tee /custom/config/config.json"))

	// Verify custom image version
	assert.Contains(t, runtime.pulledImages, "ghcr.io/project-zot/zot:v2.0.0")

	// Verify custom port in systemd service
	hasCustomPort := false
	for _, cmd := range executor.commands {
		if strings.Contains(cmd, "-p 5050:5050") {
			hasCustomPort = true
			break
		}
	}
	assert.True(t, hasCustomPort, "systemd service should contain custom port mapping")
}

func TestInstall_WithStorageBackend(t *testing.T) {
	executor := newMockExecutor()
	runtime := newMockRuntime()
	cfg := DefaultConfig()
	cfg.StorageBackend = &StorageConfig{
		Type:      "nfs",
		MountPath: "/mnt/nfs/zot",
	}

	err := Install(executor, runtime, cfg)
	require.NoError(t, err)

	// Verify storage backend directory was created
	assert.True(t, executor.hasCommand("mkdir -p /mnt/nfs/zot"))

	// Verify storage backend is mounted in systemd service
	hasStorageMount := false
	for _, cmd := range executor.commands {
		if strings.Contains(cmd, "-v /mnt/nfs/zot:/var/lib/zot") {
			hasStorageMount = true
			break
		}
	}
	assert.True(t, hasStorageMount, "systemd service should mount storage backend")
}

func TestInstall_DirectoryCreationError(t *testing.T) {
	executor := newMockExecutor()
	executor.errors["sudo mkdir -p /var/lib/foundry-zot && sudo chown -R $(id -u):$(id -g) /var/lib/foundry-zot"] = fmt.Errorf("permission denied")
	runtime := newMockRuntime()
	cfg := DefaultConfig()

	err := Install(executor, runtime, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create directories")
}

func TestInstall_ImagePullError(t *testing.T) {
	executor := newMockExecutor()
	runtime := newMockRuntime()
	runtime.pullError = fmt.Errorf("network error")
	cfg := DefaultConfig()

	err := Install(executor, runtime, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pull container image")
}

func TestInstall_ServiceStartError(t *testing.T) {
	executor := newMockExecutor()
	executor.errors["systemctl start foundry-zot"] = fmt.Errorf("service failed to start")
	runtime := newMockRuntime()
	cfg := DefaultConfig()

	err := Install(executor, runtime, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start service")
}

func TestInstall_ServiceNotRunning(t *testing.T) {
	executor := newMockExecutor()
	// Override status check to return non-running state
	executor.outputs["systemctl show foundry-zot"] = "LoadState=loaded\nActiveState=inactive\nSubState=dead"
	runtime := newMockRuntime()
	cfg := DefaultConfig()

	err := Install(executor, runtime, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service failed to start")
}

func TestCreateDirectories_Success(t *testing.T) {
	executor := newMockExecutor()
	cfg := DefaultConfig()

	err := createDirectories(executor, cfg)
	require.NoError(t, err)

	assert.True(t, executor.hasCommand("mkdir -p /var/lib/foundry-zot"))
	assert.True(t, executor.hasCommand("mkdir -p /etc/foundry-zot"))
}

func TestCreateDirectories_WithStorageBackend(t *testing.T) {
	executor := newMockExecutor()
	cfg := DefaultConfig()
	cfg.StorageBackend = &StorageConfig{
		Type:      "nfs",
		MountPath: "/mnt/nfs/zot",
	}

	err := createDirectories(executor, cfg)
	require.NoError(t, err)

	assert.True(t, executor.hasCommand("mkdir -p /mnt/nfs/zot"))
}

func TestWriteConfigFile_Success(t *testing.T) {
	executor := newMockExecutor()
	cfg := DefaultConfig()

	err := writeConfigFile(executor, cfg)
	require.NoError(t, err)

	// Verify config file was written
	assert.True(t, executor.hasCommand("sudo tee /etc/foundry-zot/config.json"))

	// Find the command and verify it contains JSON config
	for _, cmd := range executor.commands {
		if strings.Contains(cmd, "sudo tee /etc/foundry-zot/config.json") {
			assert.Contains(t, cmd, "distSpecVersion")
			assert.Contains(t, cmd, "storage")
			assert.Contains(t, cmd, "http")
			break
		}
	}
}

func TestWriteConfigFile_Error(t *testing.T) {
	executor := newMockExecutor()
	executor.errors["sudo tee /etc/foundry-zot/config.json"] = fmt.Errorf("write error")
	cfg := DefaultConfig()

	err := writeConfigFile(executor, cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write config file")
}

func TestCreateSystemdService_Success(t *testing.T) {
	executor := newMockExecutor()
	runtime := newMockRuntime()
	cfg := DefaultConfig()

	err := createSystemdService(executor, runtime, cfg)
	require.NoError(t, err)

	// Verify systemd service file was created
	assert.True(t, executor.hasCommand("sudo tee /etc/systemd/system/foundry-zot.service"))

	// Verify daemon-reload was called
	assert.True(t, executor.hasCommand("sudo systemctl daemon-reload"))

	// Find the service file content and verify key elements
	for _, cmd := range executor.commands {
		if strings.Contains(cmd, "sudo tee /etc/systemd/system/foundry-zot.service") {
			assert.Contains(t, cmd, "[Unit]")
			assert.Contains(t, cmd, "Description=Foundry Zot Registry")
			assert.Contains(t, cmd, "[Service]")
			assert.Contains(t, cmd, "ExecStart=")
			assert.Contains(t, cmd, "docker run")
			assert.Contains(t, cmd, "--name foundry-zot")
			assert.Contains(t, cmd, "-p 5000:5000")
			assert.Contains(t, cmd, "-v /var/lib/foundry-zot:/var/lib/zot")
			assert.Contains(t, cmd, "-v /etc/foundry-zot/config.json:/etc/zot/config.json")
			assert.Contains(t, cmd, "ghcr.io/project-zot/zot:latest")
			assert.Contains(t, cmd, "ExecStop=/usr/bin/docker stop")
			assert.Contains(t, cmd, "[Install]")
			assert.Contains(t, cmd, "WantedBy=multi-user.target")
			break
		}
	}
}

func TestCreateSystemdService_WithStorageBackend(t *testing.T) {
	executor := newMockExecutor()
	runtime := newMockRuntime()
	cfg := DefaultConfig()
	cfg.StorageBackend = &StorageConfig{
		Type:      "nfs",
		MountPath: "/mnt/nfs/zot",
	}

	err := createSystemdService(executor, runtime, cfg)
	require.NoError(t, err)

	// Verify storage backend mount
	hasStorageMount := false
	for _, cmd := range executor.commands {
		if strings.Contains(cmd, "-v /mnt/nfs/zot:/var/lib/zot") {
			hasStorageMount = true
			break
		}
	}
	assert.True(t, hasStorageMount, "should use storage backend path")
}

func TestComponent_Install_Success(t *testing.T) {
	executor := newMockExecutor()
	comp := NewComponent(executor)

	cfg := component.ComponentConfig{
		"host":               executor,
		"version":            "v2.0.0",
		"port":               5050,
		"pull_through_cache": true,
	}

	err := comp.Install(context.Background(), cfg)
	require.NoError(t, err)
}

func TestComponent_Install_InvalidConfig(t *testing.T) {
	executor := newMockExecutor()
	comp := NewComponent(executor)

	// Test with missing host connection
	cfg := component.ComponentConfig{}

	err := comp.Install(context.Background(), cfg)
	// Should fail without host connection
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SSH connection not provided")
}

func TestComponent_Status_NotInstalled(t *testing.T) {
	executor := newMockExecutor()
	executor.outputs["systemctl show foundry-zot"] = "LoadState=not-found\nActiveState=inactive\nSubState=dead"
	comp := NewComponent(executor)

	status, err := comp.Status(context.Background())
	require.NoError(t, err)

	// Status() now returns a stub that directs users to the command
	assert.False(t, status.Installed)
	assert.False(t, status.Healthy)
	assert.Contains(t, status.Message, "use 'foundry component status zot' command")
}

func TestComponent_Status_Running(t *testing.T) {
	executor := newMockExecutor()
	comp := NewComponent(executor)

	status, err := comp.Status(context.Background())
	require.NoError(t, err)

	// Status() now returns a stub that directs users to the command
	assert.False(t, status.Installed)
	assert.False(t, status.Healthy)
	assert.Contains(t, status.Message, "use 'foundry component status zot' command")
}

func TestComponent_Status_NotRunning(t *testing.T) {
	executor := newMockExecutor()
	executor.outputs["systemctl show foundry-zot"] = "LoadState=loaded\nActiveState=inactive\nSubState=dead"
	comp := NewComponent(executor)

	status, err := comp.Status(context.Background())
	require.NoError(t, err)

	// Status() now returns a stub that directs users to the command
	assert.False(t, status.Installed)
	assert.False(t, status.Healthy)
	assert.Contains(t, status.Message, "use 'foundry component status zot' command")
}

func TestComponent_Status_Error(t *testing.T) {
	executor := newMockExecutor()
	executor.errors["systemctl show foundry-zot"] = fmt.Errorf("command failed")
	comp := NewComponent(executor)

	status, err := comp.Status(context.Background())
	require.NoError(t, err) // Status returns status object, not error

	// Status() now returns a stub that directs users to the command
	assert.False(t, status.Installed)
	assert.False(t, status.Healthy)
	assert.Contains(t, status.Message, "use 'foundry component status zot' command")
}

func TestComponent_Upgrade_NotImplemented(t *testing.T) {
	executor := newMockExecutor()
	comp := NewComponent(executor)

	err := comp.Upgrade(context.Background(), component.ComponentConfig{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}

func TestComponent_Uninstall_NotImplemented(t *testing.T) {
	executor := newMockExecutor()
	comp := NewComponent(executor)

	err := comp.Uninstall(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not yet implemented")
}
