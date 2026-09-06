package k3s

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockWorkerExecutor is a mock for worker joining tests
type mockWorkerExecutor struct {
	execFunc func(command string) (*ssh.ExecResult, error)
}

func (m *mockWorkerExecutor) Exec(command string) (*ssh.ExecResult, error) {
	// Default: the memory cgroup preflight passes (host has the controller)
	if strings.Contains(command, "cgroup.controllers") {
		return &ssh.ExecResult{Stdout: "ok", ExitCode: 0}, nil
	}
	if m.execFunc != nil {
		return m.execFunc(command)
	}
	return &ssh.ExecResult{ExitCode: 0}, nil
}

func TestJoinWorker(t *testing.T) {
	tests := []struct {
		name          string
		serverURL     string
		tokens        *Tokens
		cfg           *Config
		exec          func(command string) (*ssh.ExecResult, error)
		expectError   bool
		errorContains string
	}{
		{
			name:      "successful worker join",
			serverURL: "https://192.168.1.100:6443",
			tokens: &Tokens{
				ClusterToken: "test-cluster-token",
				AgentToken:   "test-agent-token",
			},
			cfg: &Config{
				VIP:        "192.168.1.100",
				DNSServers: []string{"192.168.1.10"},
			},
			exec: func(command string) (*ssh.ExecResult, error) {
				// DNS configuration
				if strings.Contains(command, "systemctl is-active systemd-resolved") {
					return &ssh.ExecResult{Stdout: "active", ExitCode: 0}, nil
				}
				if strings.Contains(command, "resolved.conf.d") || strings.Contains(command, "systemctl restart") {
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				// K3s agent installation
				if strings.Contains(command, "K3S_URL=https://192.168.1.100:6443") {
					assert.Contains(t, command, "K3S_TOKEN=test-agent-token")
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				// Wait for agent ready
				if strings.Contains(command, "systemctl is-active k3s-agent") {
					return &ssh.ExecResult{Stdout: "active", ExitCode: 0}, nil
				}
				// Verify joined
				if strings.Contains(command, "journalctl") {
					return &ssh.ExecResult{Stdout: "successfully registered node", ExitCode: 0}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			expectError: false,
		},
		{
			name:      "missing agent token",
			serverURL: "https://192.168.1.100:6443",
			tokens: &Tokens{
				ClusterToken: "test-cluster-token",
				AgentToken:   "",
			},
			cfg: &Config{
				VIP: "192.168.1.100",
			},
			exec:          nil,
			expectError:   true,
			errorContains: "agent token is required",
		},
		{
			name:      "missing server URL",
			serverURL: "",
			tokens: &Tokens{
				ClusterToken: "test-cluster-token",
				AgentToken:   "test-agent-token",
			},
			cfg: &Config{
				VIP: "192.168.1.100",
			},
			exec:          nil,
			expectError:   true,
			errorContains: "server URL is required",
		},
		{
			name:      "nil tokens",
			serverURL: "https://192.168.1.100:6443",
			tokens:    nil,
			cfg: &Config{
				VIP: "192.168.1.100",
			},
			exec:          nil,
			expectError:   true,
			errorContains: "agent token is required",
		},
		{
			name:      "invalid VIP in config",
			serverURL: "https://192.168.1.100:6443",
			tokens: &Tokens{
				ClusterToken: "test-cluster-token",
				AgentToken:   "test-agent-token",
			},
			cfg: &Config{
				VIP: "invalid-ip",
			},
			exec:          nil,
			expectError:   true,
			errorContains: "VIP validation failed",
		},
		{
			name:      "empty VIP is allowed for worker nodes",
			serverURL: "https://192.168.1.100:6443",
			tokens: &Tokens{
				ClusterToken: "test-cluster-token",
				AgentToken:   "test-agent-token",
			},
			cfg: &Config{
				VIP: "",
			},
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "K3S_URL") {
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				if strings.Contains(command, "systemctl is-active k3s-agent") {
					return &ssh.ExecResult{Stdout: "active", ExitCode: 0}, nil
				}
				if strings.Contains(command, "journalctl") {
					return &ssh.ExecResult{Stdout: "successfully registered", ExitCode: 0}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			expectError: false,
		},
		{
			name:      "K3s agent installation fails",
			serverURL: "https://192.168.1.100:6443",
			tokens: &Tokens{
				ClusterToken: "test-cluster-token",
				AgentToken:   "test-agent-token",
			},
			cfg: &Config{
				VIP: "192.168.1.100",
			},
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "K3S_URL") {
					return &ssh.ExecResult{Stderr: "installation error", ExitCode: 1}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			expectError:   true,
			errorContains: "K3s agent installation failed",
		},
		{
			name:      "with registry config",
			serverURL: "https://192.168.1.100:6443",
			tokens: &Tokens{
				ClusterToken: "test-cluster-token",
				AgentToken:   "test-agent-token",
			},
			cfg: &Config{
				VIP:            "192.168.1.100",
				RegistryConfig: "mirrors:\n  docker.io:\n    endpoint:\n      - http://zot.local:5000",
			},
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "mkdir -p /etc/rancher/k3s") || strings.Contains(command, "tee") {
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				if strings.Contains(command, "K3S_URL") {
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				if strings.Contains(command, "systemctl is-active k3s-agent") {
					return &ssh.ExecResult{Stdout: "active", ExitCode: 0}, nil
				}
				if strings.Contains(command, "journalctl") {
					return &ssh.ExecResult{Stdout: "successfully registered", ExitCode: 0}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			expectError: false,
		},
		{
			name:      "DNS configuration exec error",
			serverURL: "https://192.168.1.100:6443",
			tokens: &Tokens{
				ClusterToken: "test-cluster-token",
				AgentToken:   "test-agent-token",
			},
			cfg: &Config{
				VIP:        "192.168.1.100",
				DNSServers: []string{"192.168.1.10"},
			},
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "systemctl is-active systemd-resolved") {
					return nil, fmt.Errorf("connection error")
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			expectError:   true,
			errorContains: "failed to configure DNS",
		},
		{
			name:      "registry config exec error",
			serverURL: "https://192.168.1.100:6443",
			tokens: &Tokens{
				ClusterToken: "test-cluster-token",
				AgentToken:   "test-agent-token",
			},
			cfg: &Config{
				VIP:            "192.168.1.100",
				RegistryConfig: "mirrors:\n  docker.io:\n    endpoint:\n      - http://zot.local:5000",
			},
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "mkdir -p /etc/rancher/k3s") {
					return nil, fmt.Errorf("permission denied")
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			expectError:   true,
			errorContains: "failed to create registries config",
		},
		{
			name:      "K3s agent exec error (not exit code)",
			serverURL: "https://192.168.1.100:6443",
			tokens: &Tokens{
				ClusterToken: "test-cluster-token",
				AgentToken:   "test-agent-token",
			},
			cfg: &Config{
				VIP: "192.168.1.100",
			},
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "K3S_URL") {
					return nil, fmt.Errorf("network timeout")
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			expectError:   true,
			errorContains: "failed to execute K3s agent install command",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &mockWorkerExecutor{execFunc: tt.exec}
			ctx := context.Background()
			err := JoinWorker(ctx, executor, tt.serverURL, tt.tokens, tt.cfg)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUpgradeWorkerRunsEveryMinorInOrder(t *testing.T) {
	var installers []string
	executor := &mockWorkerExecutor{execFunc: func(command string) (*ssh.ExecResult, error) {
		switch {
		case strings.Contains(command, "systemctl is-active k3s-agent"):
			return &ssh.ExecResult{Stdout: "active", ExitCode: 0}, nil
		case command == "k3s --version":
			return &ssh.ExecResult{Stdout: "k3s version v1.34.3+k3s1", ExitCode: 0}, nil
		case strings.Contains(command, "curl -sfL https://get.k3s.io"):
			installers = append(installers, command)
			return &ssh.ExecResult{ExitCode: 0}, nil
		default:
			return &ssh.ExecResult{ExitCode: 0}, nil
		}
	}}

	err := UpgradeWorker(
		context.Background(),
		executor,
		"https://192.168.1.100:6443",
		&Tokens{AgentToken: "test-agent-token"},
		&Config{Version: "v1.36.3+k3s1"},
	)

	require.NoError(t, err)
	require.Len(t, installers, 2)
	assert.Contains(t, installers[0], "INSTALL_K3S_CHANNEL=v1.35")
	assert.Contains(t, installers[1], "INSTALL_K3S_VERSION=v1.36.3+k3s1")
}

func TestGenerateK3sAgentInstallCommand(t *testing.T) {
	tests := []struct {
		name       string
		serverURL  string
		agentToken string
		expected   string
	}{
		{
			name:       "basic agent install command",
			serverURL:  "https://192.168.1.100:6443",
			agentToken: "test-agent-token",
			expected:   "curl -sfL https://get.k3s.io | K3S_URL=https://192.168.1.100:6443 K3S_TOKEN=test-agent-token sh -",
		},
		{
			name:       "with different server URL",
			serverURL:  "https://10.0.0.1:6443",
			agentToken: "another-token",
			expected:   "curl -sfL https://get.k3s.io | K3S_URL=https://10.0.0.1:6443 K3S_TOKEN=another-token sh -",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateK3sAgentInstallCommand(tt.serverURL, tt.agentToken)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateK3sAgentInstallCommandWithVersionPin(t *testing.T) {
	command := generateK3sAgentInstallCommandForVersion(
		"https://192.168.1.100:6443",
		"agent-token",
		"v1.34.3+k3s1",
	)

	assert.Contains(t, command, "INSTALL_K3S_VERSION=v1.34.3+k3s1")
}

func TestWaitForK3sAgentReady(t *testing.T) {
	// Use fast retry config for tests
	fastRetryCfg := RetryConfig{
		MaxRetries: 5,
		RetryDelay: 10 * time.Millisecond,
	}

	tests := []struct {
		name          string
		exec          func(command string) (*ssh.ExecResult, error)
		expectError   bool
		errorContains string
	}{
		{
			name: "agent becomes ready immediately",
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "systemctl is-active k3s-agent") {
					return &ssh.ExecResult{Stdout: "active", ExitCode: 0}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			expectError: false,
		},
		{
			name: "agent becomes ready after a few attempts",
			exec: func() func(command string) (*ssh.ExecResult, error) {
				attempts := 0
				return func(command string) (*ssh.ExecResult, error) {
					if strings.Contains(command, "systemctl is-active k3s-agent") {
						attempts++
						if attempts < 3 {
							return &ssh.ExecResult{Stdout: "activating", ExitCode: 3}, nil
						}
						return &ssh.ExecResult{Stdout: "active", ExitCode: 0}, nil
					}
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
			}(),
			expectError: false,
		},
		{
			name: "agent never becomes ready reports the host logs",
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "systemctl is-active k3s-agent") {
					return &ssh.ExecResult{Stdout: "activating", ExitCode: 3}, nil
				}
				if strings.Contains(command, "journalctl") {
					return &ssh.ExecResult{Stdout: `level=fatal msg="Error: failed to find memory cgroup (v2)"`, ExitCode: 0}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			expectError:   true,
			errorContains: "memory cgroup controller",
		},
		{
			name: "unreachable host reports that it stayed unreachable",
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "systemctl is-active k3s-agent") {
					return nil, fmt.Errorf("failed to create session: connection lost")
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			expectError:   true,
			errorContains: "stayed unreachable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &mockWorkerExecutor{execFunc: tt.exec}
			err := waitForK3sAgentReady(executor, fastRetryCfg)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestVerifyWorkerNodeJoined(t *testing.T) {
	tests := []struct {
		name          string
		exec          func(command string) (*ssh.ExecResult, error)
		expectError   bool
		errorContains string
	}{
		{
			name: "successful verification with registration message",
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "systemctl is-active k3s-agent") {
					return &ssh.ExecResult{Stdout: "active", ExitCode: 0}, nil
				}
				if strings.Contains(command, "journalctl") {
					return &ssh.ExecResult{Stdout: "successfully registered node worker1", ExitCode: 0}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			expectError: false,
		},
		{
			name: "successful verification without registration message but healthy service",
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "systemctl is-active k3s-agent") {
					return &ssh.ExecResult{Stdout: "active", ExitCode: 0}, nil
				}
				if strings.Contains(command, "journalctl") {
					return &ssh.ExecResult{Stdout: "some other logs", ExitCode: 0}, nil
				}
				if strings.Contains(command, "systemctl status k3s-agent") {
					return &ssh.ExecResult{Stdout: "Active: active (running)", ExitCode: 0}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			expectError: false,
		},
		{
			name: "k3s-agent not active",
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "systemctl is-active k3s-agent") {
					return &ssh.ExecResult{Stdout: "inactive", ExitCode: 3}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			expectError:   true,
			errorContains: "k3s-agent service is not active",
		},
		{
			name: "service status check fails",
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "systemctl is-active k3s-agent") {
					return &ssh.ExecResult{Stdout: "active", ExitCode: 0}, nil
				}
				if strings.Contains(command, "journalctl") {
					return &ssh.ExecResult{Stdout: "no registration message", ExitCode: 0}, nil
				}
				if strings.Contains(command, "systemctl status k3s-agent") {
					return &ssh.ExecResult{Stderr: "service failed", ExitCode: 3}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			expectError:   true,
			errorContains: "k3s-agent service is not healthy",
		},
		{
			name: "exec error on is-active check",
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "systemctl is-active k3s-agent") {
					return nil, fmt.Errorf("ssh connection lost")
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			expectError:   true,
			errorContains: "failed to check k3s-agent status",
		},
		{
			name: "exec error on status check",
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "systemctl is-active k3s-agent") {
					return &ssh.ExecResult{Stdout: "active", ExitCode: 0}, nil
				}
				if strings.Contains(command, "journalctl") {
					return &ssh.ExecResult{Stdout: "no registration", ExitCode: 0}, nil
				}
				if strings.Contains(command, "systemctl status k3s-agent") {
					return nil, fmt.Errorf("timeout")
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			expectError:   true,
			errorContains: "failed to get k3s-agent status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &mockWorkerExecutor{execFunc: tt.exec}
			err := verifyWorkerNodeJoined(executor)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}
