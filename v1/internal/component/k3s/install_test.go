package k3s

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/stretchr/testify/assert"
)

// mockInstallSSHExecutor is a mock for install tests
type mockInstallSSHExecutor struct {
	execFunc func(command string) (*ssh.ExecResult, error)
}

func (m *mockInstallSSHExecutor) Exec(command string) (*ssh.ExecResult, error) {
	if m.execFunc != nil {
		return m.execFunc(command)
	}
	return &ssh.ExecResult{ExitCode: 0}, nil
}

func TestInstallControlPlane(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		exec    func(command string) (*ssh.ExecResult, error)
		wantErr bool
		errMsg  string
	}{
		{
			name: "successful installation",
			cfg: &Config{
				VIP:         "192.168.1.100",
				Interface:   "eth0",
				ClusterInit: true,
				ClusterToken: "test-cluster-token",
				AgentToken:   "test-agent-token",
			},
			exec: func(command string) (*ssh.ExecResult, error) {
				// All commands succeed
				if strings.Contains(command, "ip route") {
					// This shouldn't be called since interface is provided
					return &ssh.ExecResult{ExitCode: 0, Stdout: "default via 192.168.1.1 dev eth0"}, nil
				}
				if strings.Contains(command, "k3s kubectl get nodes") {
					return &ssh.ExecResult{ExitCode: 0, Stdout: "node1   Ready"}, nil
				}
				if strings.Contains(command, "k3s kubectl apply") {
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				if strings.Contains(command, "ip addr show | grep") {
					return &ssh.ExecResult{ExitCode: 0, Stdout: "192.168.1.100"}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			wantErr: false,
		},
		{
			name: "installation with DNS configuration",
			cfg: &Config{
				VIP:         "192.168.1.100",
				Interface:   "eth0",
				ClusterInit: true,
				DNSServers:  []string{"192.168.1.10", "8.8.8.8"},
			},
			exec: func(command string) (*ssh.ExecResult, error) {
				// Check DNS configuration commands
				if strings.Contains(command, "systemctl is-active systemd-resolved") {
					return &ssh.ExecResult{ExitCode: 0, Stdout: "active"}, nil
				}
				if strings.Contains(command, "systemd/resolved.conf.d") {
					assert.Contains(t, command, "192.168.1.10")
					assert.Contains(t, command, "8.8.8.8")
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				if strings.Contains(command, "systemctl restart systemd-resolved") {
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				if strings.Contains(command, "k3s kubectl") {
					return &ssh.ExecResult{ExitCode: 0, Stdout: "ready"}, nil
				}
				if strings.Contains(command, "ip addr show | grep") {
					return &ssh.ExecResult{ExitCode: 0, Stdout: "192.168.1.100"}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			wantErr: false,
		},
		{
			name: "installation with registry config",
			cfg: &Config{
				VIP:            "192.168.1.100",
				Interface:      "eth0",
				ClusterInit:    true,
				RegistryConfig: "mirrors:\n  docker.io:\n    endpoint:\n      - http://zot:5000",
			},
			exec: func(command string) (*ssh.ExecResult, error) {
				// Check registry configuration
				if strings.Contains(command, "/etc/rancher/k3s/registries.yaml") {
					assert.Contains(t, command, "mirrors")
					assert.Contains(t, command, "docker.io")
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				if strings.Contains(command, "k3s kubectl") {
					return &ssh.ExecResult{ExitCode: 0, Stdout: "ready"}, nil
				}
				if strings.Contains(command, "ip addr show | grep") {
					return &ssh.ExecResult{ExitCode: 0, Stdout: "192.168.1.100"}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			wantErr: false,
		},
		{
			name: "auto-detect interface",
			cfg: &Config{
				VIP:         "192.168.1.100",
				ClusterInit: true,
			},
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "ip route") {
					return &ssh.ExecResult{ExitCode: 0, Stdout: "default via 192.168.1.1 dev eth0"}, nil
				}
				if strings.Contains(command, "k3s kubectl") {
					return &ssh.ExecResult{ExitCode: 0, Stdout: "ready"}, nil
				}
				if strings.Contains(command, "ip addr show | grep") {
					return &ssh.ExecResult{ExitCode: 0, Stdout: "192.168.1.100"}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			wantErr: false,
		},
		{
			name: "invalid config",
			cfg: &Config{
				VIP: "", // Missing VIP
			},
			exec: func(command string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			wantErr: true,
			errMsg:  "config validation failed",
		},
		{
			name: "interface detection fails",
			cfg: &Config{
				VIP:         "192.168.1.100",
				ClusterInit: true,
			},
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "ip route") {
					return &ssh.ExecResult{ExitCode: 1, Stderr: "command not found"}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			wantErr: true,
			errMsg:  "failed to detect network interface",
		},
		{
			name: "K3s installation fails",
			cfg: &Config{
				VIP:         "192.168.1.100",
				Interface:   "eth0",
				ClusterInit: true,
			},
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "curl -sfL https://get.k3s.io") {
					return &ssh.ExecResult{ExitCode: 1, Stderr: "installation failed"}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			wantErr: true,
			errMsg:  "K3s installation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &mockInstallSSHExecutor{execFunc: tt.exec}
			err := InstallControlPlane(context.Background(), executor, tt.cfg)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfigureDNS(t *testing.T) {
	tests := []struct {
		name       string
		dnsServers []string
		exec       func(command string) (*ssh.ExecResult, error)
		wantErr    bool
	}{
		{
			name:       "systemd-resolved active",
			dnsServers: []string{"192.168.1.10", "8.8.8.8"},
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "systemctl is-active systemd-resolved") {
					return &ssh.ExecResult{ExitCode: 0, Stdout: "active"}, nil
				}
				if strings.Contains(command, "systemd/resolved.conf.d") {
					assert.Contains(t, command, "192.168.1.10")
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				if strings.Contains(command, "systemctl restart systemd-resolved") {
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			wantErr: false,
		},
		{
			name:       "systemd-resolved not active - use resolv.conf",
			dnsServers: []string{"192.168.1.10"},
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "systemctl is-active systemd-resolved") {
					return &ssh.ExecResult{ExitCode: 3, Stdout: "inactive"}, nil
				}
				if strings.Contains(command, "/etc/resolv.conf") {
					assert.Contains(t, command, "nameserver 192.168.1.10")
					assert.Contains(t, command, "chattr +i /etc/resolv.conf")
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			wantErr: false,
		},
		{
			name:       "systemd-resolved restart fails",
			dnsServers: []string{"192.168.1.10"},
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "systemctl is-active systemd-resolved") {
					return &ssh.ExecResult{ExitCode: 0, Stdout: "active"}, nil
				}
				if strings.Contains(command, "systemd/resolved.conf.d") {
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				if strings.Contains(command, "systemctl restart systemd-resolved") {
					return &ssh.ExecResult{ExitCode: 1, Stderr: "restart failed"}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &mockInstallSSHExecutor{execFunc: tt.exec}
			err := configureDNS(executor, tt.dnsServers)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateRegistriesConfig(t *testing.T) {
	tests := []struct {
		name    string
		content string
		exec    func(command string) (*ssh.ExecResult, error)
		wantErr bool
	}{
		{
			name:    "successful creation",
			content: "mirrors:\n  docker.io:\n    endpoint:\n      - http://zot:5000",
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "mkdir -p /etc/rancher/k3s") {
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				if strings.Contains(command, "/etc/rancher/k3s/registries.yaml") {
					assert.Contains(t, command, "mirrors")
					assert.Contains(t, command, "docker.io")
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			wantErr: false,
		},
		{
			name:    "mkdir fails",
			content: "mirrors:\n  docker.io:\n    endpoint:\n      - http://zot:5000",
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "mkdir -p /etc/rancher/k3s") {
					return &ssh.ExecResult{ExitCode: 1, Stderr: "permission denied"}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			wantErr: true,
		},
		{
			name:    "write fails",
			content: "mirrors:\n  docker.io:\n    endpoint:\n      - http://zot:5000",
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "mkdir -p /etc/rancher/k3s") {
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				if strings.Contains(command, "/etc/rancher/k3s/registries.yaml") {
					return &ssh.ExecResult{ExitCode: 1, Stderr: "write error"}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &mockInstallSSHExecutor{execFunc: tt.exec}
			err := createRegistriesConfig(executor, tt.content)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWaitForK3sReady(t *testing.T) {
	// Use fast retry config for tests
	fastRetryCfg := RetryConfig{
		MaxRetries: 5,
		RetryDelay: 10 * time.Millisecond,
	}

	tests := []struct {
		name    string
		exec    func(command string) (*ssh.ExecResult, error)
		wantErr bool
	}{
		{
			name: "K3s ready immediately",
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "k3s kubectl get nodes") {
					return &ssh.ExecResult{ExitCode: 0, Stdout: "node1   Ready"}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			wantErr: false,
		},
		{
			name: "K3s ready after retry",
			exec: func() func(command string) (*ssh.ExecResult, error) {
				attempt := 0
				return func(command string) (*ssh.ExecResult, error) {
					if strings.Contains(command, "k3s kubectl get nodes") {
						attempt++
						if attempt < 3 {
							return &ssh.ExecResult{ExitCode: 1, Stderr: "connection refused"}, nil
						}
						return &ssh.ExecResult{ExitCode: 0, Stdout: "node1   Ready"}, nil
					}
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &mockInstallSSHExecutor{execFunc: tt.exec}
			err := waitForK3sReady(executor, fastRetryCfg)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSetupKubeVIP(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		exec    func(command string) (*ssh.ExecResult, error)
		wantErr bool
	}{
		{
			name: "successful setup",
			cfg: &Config{
				VIP:       "192.168.1.100",
				Interface: "eth0",
			},
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "k3s kubectl apply") {
					assert.Contains(t, command, "kind: DaemonSet")
					assert.Contains(t, command, "192.168.1.100")
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			wantErr: false,
		},
		{
			name: "kubectl apply fails",
			cfg: &Config{
				VIP:       "192.168.1.100",
				Interface: "eth0",
			},
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "k3s kubectl apply") {
					return &ssh.ExecResult{ExitCode: 1, Stderr: "apply failed"}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &mockInstallSSHExecutor{execFunc: tt.exec}
			err := setupKubeVIP(context.Background(), executor, tt.cfg)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWaitForKubeVIPReady(t *testing.T) {
	// Use fast retry config for tests
	fastRetryCfg := RetryConfig{
		MaxRetries: 5,
		RetryDelay: 10 * time.Millisecond,
	}

	tests := []struct {
		name    string
		vip     string
		exec    func(command string) (*ssh.ExecResult, error)
		wantErr bool
	}{
		{
			name: "VIP ready immediately",
			vip:  "192.168.1.100",
			exec: func(command string) (*ssh.ExecResult, error) {
				if strings.Contains(command, "ip addr show | grep 192.168.1.100") {
					return &ssh.ExecResult{ExitCode: 0, Stdout: "inet 192.168.1.100/32 scope global eth0"}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			wantErr: false,
		},
		{
			name: "VIP ready after retry",
			vip:  "192.168.1.100",
			exec: func() func(command string) (*ssh.ExecResult, error) {
				attempt := 0
				return func(command string) (*ssh.ExecResult, error) {
					if strings.Contains(command, "ip addr show | grep") {
						attempt++
						if attempt < 3 {
							return &ssh.ExecResult{ExitCode: 1}, nil
						}
						return &ssh.ExecResult{ExitCode: 0, Stdout: "inet 192.168.1.100/32 scope global eth0"}, nil
					}
					return &ssh.ExecResult{ExitCode: 0}, nil
				}
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &mockInstallSSHExecutor{execFunc: tt.exec}
			err := waitForKubeVIPReady(executor, tt.vip, fastRetryCfg)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
