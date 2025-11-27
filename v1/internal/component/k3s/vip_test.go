package k3s

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock SSH executor for testing
type mockSSHExecutor struct {
	execFunc func(command string) (*ssh.ExecResult, error)
}

func (m *mockSSHExecutor) Exec(command string) (*ssh.ExecResult, error) {
	if m.execFunc != nil {
		return m.execFunc(command)
	}
	return &ssh.ExecResult{
		Stdout:   "",
		Stderr:   "",
		ExitCode: 0,
	}, nil
}

// TestValidateVIP tests VIP validation
func TestValidateVIP(t *testing.T) {
	tests := []struct {
		name    string
		vip     string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid private IP - 192.168.x.x",
			vip:     "192.168.1.100",
			wantErr: false,
		},
		{
			name:    "valid private IP - 10.x.x.x",
			vip:     "10.0.0.50",
			wantErr: false,
		},
		{
			name:    "valid private IP - 172.16-31.x.x",
			vip:     "172.16.0.100",
			wantErr: false,
		},
		{
			name:    "empty VIP",
			vip:     "",
			wantErr: true,
			errMsg:  "cannot be empty",
		},
		{
			name:    "invalid IP format",
			vip:     "not-an-ip",
			wantErr: true,
			errMsg:  "invalid VIP address format",
		},
		{
			name:    "invalid IP format - missing octet",
			vip:     "192.168.1",
			wantErr: true,
			errMsg:  "invalid VIP address format",
		},
		{
			name:    "IPv6 address (not supported yet)",
			vip:     "2001:db8::1",
			wantErr: true,
			errMsg:  "must be an IPv4 address",
		},
		{
			name:    "public IP address",
			vip:     "8.8.8.8",
			wantErr: true,
			errMsg:  "should be a private IP",
		},
		{
			name:    "loopback address",
			vip:     "127.0.0.1",
			wantErr: true,
			errMsg:  "should be a private IP",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVIP(tt.vip)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestDetectNetworkInterface tests network interface detection
func TestDetectNetworkInterface(t *testing.T) {
	tests := []struct {
		name      string
		setupMock func() *mockSSHExecutor
		want      string
		wantErr   bool
		errMsg    string
	}{
		{
			name: "successful detection",
			setupMock: func() *mockSSHExecutor {
				return &mockSSHExecutor{
					execFunc: func(command string) (*ssh.ExecResult, error) {
						if strings.Contains(command, "ip route show default") {
							return &ssh.ExecResult{
								Stdout:   "eth0\n",
								ExitCode: 0,
							}, nil
						}
						return &ssh.ExecResult{ExitCode: 0}, nil
					},
				}
			},
			want:    "eth0",
			wantErr: false,
		},
		{
			name: "SSH execution error",
			setupMock: func() *mockSSHExecutor {
				return &mockSSHExecutor{
					execFunc: func(command string) (*ssh.ExecResult, error) {
						return nil, fmt.Errorf("connection failed")
					},
				}
			},
			wantErr: true,
			errMsg:  "failed to detect network interface",
		},
		{
			name: "no default route",
			setupMock: func() *mockSSHExecutor {
				return &mockSSHExecutor{
					execFunc: func(command string) (*ssh.ExecResult, error) {
						return &ssh.ExecResult{
							Stdout:   "",
							ExitCode: 0,
						}, nil
					},
				}
			},
			wantErr: true,
			errMsg:  "failed to detect network interface",
		},
		{
			name: "command exit code non-zero",
			setupMock: func() *mockSSHExecutor {
				return &mockSSHExecutor{
					execFunc: func(command string) (*ssh.ExecResult, error) {
						return &ssh.ExecResult{
							Stderr:   "command not found",
							ExitCode: 1,
						}, nil
					},
				}
			},
			wantErr: true,
			errMsg:  "failed to detect network interface",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.setupMock()
			got, err := DetectNetworkInterface(mock)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// TestDetermineVIPConfig tests VIP configuration determination
func TestDetermineVIPConfig(t *testing.T) {
	tests := []struct {
		name      string
		vip       string
		setupMock func() *mockSSHExecutor
		want      *VIPConfig
		wantErr   bool
		errMsg    string
	}{
		{
			name: "successful VIP config determination",
			vip:  "192.168.1.100",
			setupMock: func() *mockSSHExecutor {
				return &mockSSHExecutor{
					execFunc: func(command string) (*ssh.ExecResult, error) {
						return &ssh.ExecResult{
							Stdout:   "eth0\n",
							ExitCode: 0,
						}, nil
					},
				}
			},
			want: &VIPConfig{
				VIP:       "192.168.1.100",
				Interface: "eth0",
			},
			wantErr: false,
		},
		{
			name: "invalid VIP",
			vip:  "not-an-ip",
			setupMock: func() *mockSSHExecutor {
				return &mockSSHExecutor{}
			},
			wantErr: true,
			errMsg:  "VIP validation failed",
		},
		{
			name: "interface detection fails",
			vip:  "192.168.1.100",
			setupMock: func() *mockSSHExecutor {
				return &mockSSHExecutor{
					execFunc: func(command string) (*ssh.ExecResult, error) {
						return nil, fmt.Errorf("SSH error")
					},
				}
			},
			wantErr: true,
			errMsg:  "interface detection failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.setupMock()
			got, err := DetermineVIPConfig(tt.vip, mock)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

// TestGenerateKubeVIPManifest tests kube-vip manifest generation
func TestGenerateKubeVIPManifest(t *testing.T) {
	tests := []struct {
		name       string
		cfg        *VIPConfig
		wantErr    bool
		errMsg     string
		checkFuncs []func(t *testing.T, manifest string)
	}{
		{
			name: "valid config generates manifest",
			cfg: &VIPConfig{
				VIP:       "192.168.1.100",
				Interface: "eth0",
			},
			wantErr: false,
			checkFuncs: []func(t *testing.T, manifest string){
				func(t *testing.T, manifest string) {
					assert.Contains(t, manifest, "kind: DaemonSet")
					assert.Contains(t, manifest, "name: kube-vip")
					assert.Contains(t, manifest, "namespace: kube-system")
				},
				func(t *testing.T, manifest string) {
					assert.Contains(t, manifest, "value: \"192.168.1.100\"")
					assert.Contains(t, manifest, "value: \"eth0\"")
				},
				func(t *testing.T, manifest string) {
					assert.Contains(t, manifest, "ghcr.io/kube-vip/kube-vip")
				},
				func(t *testing.T, manifest string) {
					assert.Contains(t, manifest, "vip_interface")
					assert.Contains(t, manifest, "vip_address")
					assert.Contains(t, manifest, "vip_arp")
				},
				func(t *testing.T, manifest string) {
					assert.Contains(t, manifest, "hostNetwork: true")
				},
				func(t *testing.T, manifest string) {
					assert.Contains(t, manifest, "node-role.kubernetes.io/control-plane")
				},
			},
		},
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: true,
			errMsg:  "config cannot be nil",
		},
		{
			name: "empty VIP",
			cfg: &VIPConfig{
				VIP:       "",
				Interface: "eth0",
			},
			wantErr: true,
			errMsg:  "VIP address is required",
		},
		{
			name: "empty interface",
			cfg: &VIPConfig{
				VIP:       "192.168.1.100",
				Interface: "",
			},
			wantErr: true,
			errMsg:  "interface is required",
		},
		{
			name: "invalid VIP in config",
			cfg: &VIPConfig{
				VIP:       "not-an-ip",
				Interface: "eth0",
			},
			wantErr: true,
			errMsg:  "invalid VIP configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GenerateKubeVIPManifest(tt.cfg)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, got)

				// Run all check functions
				for _, check := range tt.checkFuncs {
					check(t, got)
				}
			}
		})
	}
}

// TestGenerateKubeVIPRBACManifest tests RBAC manifest generation
func TestGenerateKubeVIPRBACManifest(t *testing.T) {
	manifest := GenerateKubeVIPRBACManifest()

	assert.NotEmpty(t, manifest)
	assert.Contains(t, manifest, "kind: ServiceAccount")
	assert.Contains(t, manifest, "name: kube-vip")
	assert.Contains(t, manifest, "kind: ClusterRole")
	assert.Contains(t, manifest, "name: kube-vip-role")
	assert.Contains(t, manifest, "kind: ClusterRoleBinding")
	assert.Contains(t, manifest, "name: kube-vip-binding")

	// Check for required permissions
	assert.Contains(t, manifest, "services")
	assert.Contains(t, manifest, "endpoints")
	assert.Contains(t, manifest, "nodes")
	assert.Contains(t, manifest, "configmaps")
	assert.Contains(t, manifest, "leases")
}

// TestGenerateKubeVIPCloudProviderManifest tests cloud provider manifest generation
func TestGenerateKubeVIPCloudProviderManifest(t *testing.T) {
	manifest := GenerateKubeVIPCloudProviderManifest()

	assert.NotEmpty(t, manifest)
	assert.Contains(t, manifest, "kind: ServiceAccount")
	assert.Contains(t, manifest, "name: kube-vip-cloud-provider")
	assert.Contains(t, manifest, "kind: Deployment")
	assert.Contains(t, manifest, "ghcr.io/kube-vip/kube-vip-cloud-provider")
	assert.Contains(t, manifest, "kind: ClusterRole")
	assert.Contains(t, manifest, "kind: ClusterRoleBinding")
}

// TestGenerateKubeVIPConfigMap tests ConfigMap generation
func TestGenerateKubeVIPConfigMap(t *testing.T) {
	tests := []struct {
		name       string
		cidr       string
		wantErr    bool
		errMsg     string
		checkFuncs []func(t *testing.T, manifest string)
	}{
		{
			name:    "valid CIDR",
			cidr:    "192.168.1.100/32",
			wantErr: false,
			checkFuncs: []func(t *testing.T, manifest string){
				func(t *testing.T, manifest string) {
					assert.Contains(t, manifest, "kind: ConfigMap")
					assert.Contains(t, manifest, "name: kubevip")
					assert.Contains(t, manifest, "namespace: kube-system")
				},
				func(t *testing.T, manifest string) {
					assert.Contains(t, manifest, "cidr-global: \"192.168.1.100/32\"")
				},
				func(t *testing.T, manifest string) {
					assert.Contains(t, manifest, "allow-share-global: \"true\"")
				},
			},
		},
		{
			name:    "valid CIDR range",
			cidr:    "192.168.1.0/24",
			wantErr: false,
			checkFuncs: []func(t *testing.T, manifest string){
				func(t *testing.T, manifest string) {
					assert.Contains(t, manifest, "cidr-global: \"192.168.1.0/24\"")
				},
				func(t *testing.T, manifest string) {
					assert.Contains(t, manifest, "allow-share-global: \"true\"")
				},
			},
		},
		{
			name:    "empty CIDR",
			cidr:    "",
			wantErr: true,
			errMsg:  "CIDR cannot be empty",
		},
		{
			name:    "invalid CIDR format",
			cidr:    "not-a-cidr",
			wantErr: true,
			errMsg:  "invalid CIDR format",
		},
		{
			name:    "invalid CIDR - no mask",
			cidr:    "192.168.1.100",
			wantErr: true,
			errMsg:  "invalid CIDR format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GenerateKubeVIPConfigMap(tt.cidr)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.NotEmpty(t, got)

				// Run all check functions
				for _, check := range tt.checkFuncs {
					check(t, got)
				}
			}
		})
	}
}

// TestFormatManifests tests manifest combination
func TestFormatManifests(t *testing.T) {
	tests := []struct {
		name      string
		manifests []string
		want      string
	}{
		{
			name: "multiple manifests",
			manifests: []string{
				"manifest1",
				"manifest2",
				"manifest3",
			},
			want: "manifest1\n---\nmanifest2\n---\nmanifest3",
		},
		{
			name:      "single manifest",
			manifests: []string{"manifest1"},
			want:      "manifest1",
		},
		{
			name:      "empty manifests",
			manifests: []string{},
			want:      "",
		},
		{
			name: "manifests with empty strings",
			manifests: []string{
				"manifest1",
				"",
				"manifest2",
				"   ",
			},
			want: "manifest1\n---\nmanifest2",
		},
		{
			name: "manifests with whitespace",
			manifests: []string{
				"  manifest1  ",
				"manifest2\n\n",
			},
			want: "manifest1\n---\nmanifest2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatManifests(tt.manifests...)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestIsPrivateIP tests private IP detection
func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{
			name: "private IP - 192.168.x.x",
			ip:   "192.168.1.1",
			want: true,
		},
		{
			name: "private IP - 10.x.x.x",
			ip:   "10.0.0.1",
			want: true,
		},
		{
			name: "private IP - 172.16.x.x",
			ip:   "172.16.0.1",
			want: true,
		},
		{
			name: "private IP - 172.31.x.x (upper bound)",
			ip:   "172.31.255.255",
			want: true,
		},
		{
			name: "public IP",
			ip:   "8.8.8.8",
			want: false,
		},
		{
			name: "public IP - 172.15.x.x (just before private range)",
			ip:   "172.15.0.1",
			want: false,
		},
		{
			name: "public IP - 172.32.x.x (just after private range)",
			ip:   "172.32.0.1",
			want: false,
		},
		{
			name: "loopback",
			ip:   "127.0.0.1",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := parseIP(t, tt.ip)
			got := isPrivateIP(ip)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Helper function to parse IP for tests
func parseIP(t *testing.T, ipStr string) net.IP {
	t.Helper()
	ip := net.ParseIP(ipStr)
	require.NotNil(t, ip, "failed to parse IP: %s", ipStr)
	return ip
}

// TestVIPConfigIntegration tests the full VIP configuration workflow
func TestVIPConfigIntegration(t *testing.T) {
	// Mock SSH executor that returns eth0
	mock := &mockSSHExecutor{
		execFunc: func(command string) (*ssh.ExecResult, error) {
			return &ssh.ExecResult{
				Stdout:   "eth0\n",
				ExitCode: 0,
			}, nil
		},
	}

	// Test full workflow
	vip := "192.168.1.100"

	// Step 1: Determine VIP config
	cfg, err := DetermineVIPConfig(vip, mock)
	require.NoError(t, err)
	assert.Equal(t, vip, cfg.VIP)
	assert.Equal(t, "eth0", cfg.Interface)

	// Step 2: Generate manifests
	dsManifest, err := GenerateKubeVIPManifest(cfg)
	require.NoError(t, err)
	assert.NotEmpty(t, dsManifest)

	rbacManifest := GenerateKubeVIPRBACManifest()
	assert.NotEmpty(t, rbacManifest)

	cpManifest := GenerateKubeVIPCloudProviderManifest()
	assert.NotEmpty(t, cpManifest)

	cmManifest, err := GenerateKubeVIPConfigMap("192.168.1.100/32")
	require.NoError(t, err)
	assert.NotEmpty(t, cmManifest)

	// Step 3: Combine manifests
	combined := FormatManifests(rbacManifest, dsManifest, cpManifest, cmManifest)
	assert.Contains(t, combined, "---")
	assert.Contains(t, combined, "kube-vip")
	assert.Contains(t, combined, "192.168.1.100")
	assert.Contains(t, combined, "eth0")
}
