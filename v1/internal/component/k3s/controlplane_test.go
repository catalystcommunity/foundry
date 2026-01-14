package k3s

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockControlPlaneExecutor is a mock for control plane joining tests
type mockControlPlaneExecutor struct {
	execFunc func(command string) (*ssh.ExecResult, error)
}

func (m *mockControlPlaneExecutor) Exec(command string) (*ssh.ExecResult, error) {
	if m.execFunc != nil {
		return m.execFunc(command)
	}
	return &ssh.ExecResult{ExitCode: 0}, nil
}

func TestJoinControlPlane(t *testing.T) {
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
			name:      "successful control plane join",
			serverURL: "https://192.168.1.100:6443",
			tokens: &Tokens{
				ClusterToken: "test-cluster-token",
				AgentToken:   "test-agent-token",
			},
			cfg: &Config{
				VIP:         "192.168.1.100",
				DNSServers:  []string{"192.168.1.10"},
				ClusterInit: false,
			},
			exec: func(command string) (*ssh.ExecResult, error) {
				// DNS configuration
				if strings.Contains(command, "systemctl is-active systemd-resolved") {
					return &ssh.ExecResult{Stdout: "active", ExitCode: 0}, nil
				}
				if strings.Contains(command, "resolved.conf.d") || strings.Contains(command, "systemctl restart") {
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				// K3s installation
				if strings.Contains(command, "curl -sfL https://get.k3s.io") {
					assert.Contains(t, command, "--server https://192.168.1.100:6443")
					assert.Contains(t, command, "--token test-cluster-token")
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				// Wait for ready
				if strings.Contains(command, "k3s kubectl get nodes") {
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				// Verify node joined
				if command == "hostname" {
					return &ssh.ExecResult{Stdout: "node2", ExitCode: 0}, nil
				}
				if strings.Contains(command, "get node node2") && !strings.Contains(command, "jsonpath") {
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				if strings.Contains(command, "jsonpath") && strings.Contains(command, "control-plane") {
					return &ssh.ExecResult{Stdout: "true", ExitCode: 0}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			expectError: false,
		},
		{
			name:      "missing cluster token",
			serverURL: "https://192.168.1.100:6443",
			tokens: &Tokens{
				ClusterToken: "",
				AgentToken:   "test-agent-token",
			},
			cfg: &Config{
				VIP: "192.168.1.100",
			},
			exec:          nil,
			expectError:   true,
			errorContains: "cluster token is required",
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
			errorContains: "existing server URL is required",
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
			errorContains: "cluster token is required",
		},
		{
			name:      "invalid VIP",
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
			errorContains: "config validation failed",
		},
		{
			name:      "K3s installation fails",
			serverURL: "https://192.168.1.100:6443",
			tokens: &Tokens{
				ClusterToken: "test-cluster-token",
				AgentToken:   "test-agent-token",
			},
			cfg: &Config{
				VIP: "192.168.1.100",
			},
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "curl -sfL https://get.k3s.io") {
					return &ssh.ExecResult{Stderr: "installation error", ExitCode: 1}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			expectError:   true,
			errorContains: "K3s installation failed",
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
				if strings.Contains(command, "mkdir -p /etc/rancher/k3s") || strings.Contains(command, "tee /etc/rancher/k3s/registries.yaml") {
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				if strings.Contains(command, "curl -sfL https://get.k3s.io") {
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				if strings.Contains(command, "k3s kubectl get nodes") {
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				if command == "hostname" {
					return &ssh.ExecResult{Stdout: "node2", ExitCode: 0}, nil
				}
				if strings.Contains(command, "get node node2") && !strings.Contains(command, "jsonpath") {
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				if strings.Contains(command, "jsonpath") {
					return &ssh.ExecResult{Stdout: "true", ExitCode: 0}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &mockControlPlaneExecutor{execFunc: tt.exec}
			ctx := context.Background()
			err := JoinControlPlane(ctx, executor, tt.serverURL, tt.tokens, tt.cfg)

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

func TestVerifyNodeJoined(t *testing.T) {
	// Use fast retry config for tests
	fastRetryCfg := RetryConfig{
		MaxRetries: 2,
		RetryDelay: 10 * time.Millisecond,
	}

	tests := []struct {
		name          string
		exec          func(command string) (*ssh.ExecResult, error)
		expectError   bool
		errorContains string
	}{
		{
			name: "successful verification",
			exec: func(command string) (*ssh.ExecResult, error) {
				if command == "hostname" {
					return &ssh.ExecResult{Stdout: "node2", ExitCode: 0}, nil
				}
				if strings.Contains(command, "get node node2") && !strings.Contains(command, "jsonpath") {
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				if strings.Contains(command, "jsonpath") && strings.Contains(command, "control-plane") {
					return &ssh.ExecResult{Stdout: "true", ExitCode: 0}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			expectError: false,
		},
		{
			name: "hostname fails",
			exec: func(command string) (*ssh.ExecResult, error) {
				if command == "hostname" {
					return &ssh.ExecResult{ExitCode: 1}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			expectError:   true,
			errorContains: "failed to get hostname",
		},
		{
			name: "node not found in cluster",
			exec: func(command string) (*ssh.ExecResult, error) {
				if command == "hostname" {
					return &ssh.ExecResult{Stdout: "node2", ExitCode: 0}, nil
				}
				if strings.Contains(command, "get node node2") {
					return &ssh.ExecResult{ExitCode: 1}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			expectError:   true,
			errorContains: "not found in cluster",
		},
		{
			name: "node does not have control-plane role",
			exec: func(command string) (*ssh.ExecResult, error) {
				if command == "hostname" {
					return &ssh.ExecResult{Stdout: "node2", ExitCode: 0}, nil
				}
				if strings.Contains(command, "get node node2") {
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				if strings.Contains(command, "jsonpath") {
					return &ssh.ExecResult{Stdout: "", ExitCode: 0}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			expectError:   true,
			errorContains: "does not have control-plane role",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &mockControlPlaneExecutor{execFunc: tt.exec}
			err := verifyNodeJoined(executor, fastRetryCfg)

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
