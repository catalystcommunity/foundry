package network

import (
	"fmt"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockConnection is a mock SSH connection for testing
type mockConnection struct {
	execFunc func(command string) (*ssh.ExecResult, error)
}

func (m *mockConnection) Exec(command string) (*ssh.ExecResult, error) {
	if m.execFunc != nil {
		return m.execFunc(command)
	}
	return &ssh.ExecResult{ExitCode: 0}, nil
}

func TestDetectPrimaryInterface(t *testing.T) {
	tests := []struct {
		name        string
		execFunc    func(string) (*ssh.ExecResult, error)
		want        string
		wantErr     bool
		errContains string
	}{
		{
			name: "success - eth0",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{
					Stdout:   "eth0\n",
					ExitCode: 0,
				}, nil
			},
			want:    "eth0",
			wantErr: false,
		},
		{
			name: "success - ens3",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{
					Stdout:   "ens3\n",
					ExitCode: 0,
				}, nil
			},
			want:    "ens3",
			wantErr: false,
		},
		{
			name: "success - with whitespace",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{
					Stdout:   "  enp0s3  \n",
					ExitCode: 0,
				}, nil
			},
			want:    "enp0s3",
			wantErr: false,
		},
		{
			name: "error - no default route",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{
					Stdout:   "",
					ExitCode: 0,
				}, nil
			},
			wantErr:     true,
			errContains: "no default route interface found",
		},
		{
			name: "error - command fails",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{
					Stderr:   "ip: command not found",
					ExitCode: 127,
				}, nil
			},
			wantErr:     true,
			errContains: "failed to detect primary interface",
		},
		{
			name: "error - ssh error",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				return nil, fmt.Errorf("connection timeout")
			},
			wantErr:     true,
			errContains: "failed to detect primary interface",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &mockConnection{execFunc: tt.execFunc}
			got, err := DetectPrimaryInterface(conn)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDetectMAC(t *testing.T) {
	tests := []struct {
		name        string
		iface       string
		execFunc    func(string) (*ssh.ExecResult, error)
		want        string
		wantErr     bool
		errContains string
	}{
		{
			name:  "success - valid MAC",
			iface: "eth0",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{
					Stdout:   "52:54:00:12:34:56\n",
					ExitCode: 0,
				}, nil
			},
			want:    "52:54:00:12:34:56",
			wantErr: false,
		},
		{
			name:  "success - lowercase MAC",
			iface: "ens3",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{
					Stdout:   "aa:bb:cc:dd:ee:ff\n",
					ExitCode: 0,
				}, nil
			},
			want:    "aa:bb:cc:dd:ee:ff",
			wantErr: false,
		},
		{
			name:        "error - empty interface",
			iface:       "",
			execFunc:    nil,
			wantErr:     true,
			errContains: "interface name cannot be empty",
		},
		{
			name:  "error - invalid MAC format",
			iface: "eth0",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{
					Stdout:   "invalid-mac\n",
					ExitCode: 0,
				}, nil
			},
			wantErr:     true,
			errContains: "invalid MAC address format",
		},
		{
			name:  "error - empty MAC",
			iface: "eth0",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{
					Stdout:   "",
					ExitCode: 0,
				}, nil
			},
			wantErr:     true,
			errContains: "no MAC address found",
		},
		{
			name:  "error - command fails",
			iface: "eth0",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{
					Stderr:   "No such file or directory",
					ExitCode: 1,
				}, nil
			},
			wantErr:     true,
			errContains: "failed to detect MAC address",
		},
		{
			name:  "error - ssh error",
			iface: "eth0",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				return nil, fmt.Errorf("connection lost")
			},
			wantErr:     true,
			errContains: "failed to detect MAC address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &mockConnection{execFunc: tt.execFunc}
			got, err := DetectMAC(conn, tt.iface)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDetectCurrentIP(t *testing.T) {
	tests := []struct {
		name        string
		iface       string
		execFunc    func(string) (*ssh.ExecResult, error)
		want        string
		wantErr     bool
		errContains string
	}{
		{
			name:  "success - valid IP",
			iface: "eth0",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{
					Stdout:   "192.168.1.10\n",
					ExitCode: 0,
				}, nil
			},
			want:    "192.168.1.10",
			wantErr: false,
		},
		{
			name:  "success - IP with whitespace",
			iface: "ens3",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{
					Stdout:   "  10.0.0.5  \n",
					ExitCode: 0,
				}, nil
			},
			want:    "10.0.0.5",
			wantErr: false,
		},
		{
			name:        "error - empty interface",
			iface:       "",
			execFunc:    nil,
			wantErr:     true,
			errContains: "interface name cannot be empty",
		},
		{
			name:  "error - invalid IP format",
			iface: "eth0",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{
					Stdout:   "invalid-ip\n",
					ExitCode: 0,
				}, nil
			},
			wantErr:     true,
			errContains: "invalid IP address format",
		},
		{
			name:  "error - empty IP",
			iface: "eth0",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{
					Stdout:   "",
					ExitCode: 0,
				}, nil
			},
			wantErr:     true,
			errContains: "no IP address found",
		},
		{
			name:  "error - command fails",
			iface: "eth0",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{
					Stderr:   "Device not found",
					ExitCode: 1,
				}, nil
			},
			wantErr:     true,
			errContains: "failed to detect IP address",
		},
		{
			name:  "error - ssh error",
			iface: "eth0",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				return nil, fmt.Errorf("network unreachable")
			},
			wantErr:     true,
			errContains: "failed to detect IP address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &mockConnection{execFunc: tt.execFunc}
			got, err := DetectCurrentIP(conn, tt.iface)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDetectInterface(t *testing.T) {
	tests := []struct {
		name        string
		execFunc    func(string) (*ssh.ExecResult, error)
		want        *InterfaceInfo
		wantErr     bool
		errContains string
	}{
		{
			name: "success - complete interface info",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				// Simulate different commands
				if cmd == "ip route show default | head -n1 | awk '{print $5}'" {
					return &ssh.ExecResult{Stdout: "eth0\n", ExitCode: 0}, nil
				}
				if cmd == "cat /sys/class/net/eth0/address" {
					return &ssh.ExecResult{Stdout: "52:54:00:12:34:56\n", ExitCode: 0}, nil
				}
				if cmd == "ip addr show eth0 | grep 'inet ' | head -n1 | awk '{print $2}' | cut -d'/' -f1" {
					return &ssh.ExecResult{Stdout: "192.168.1.10\n", ExitCode: 0}, nil
				}
				return &ssh.ExecResult{ExitCode: 1}, fmt.Errorf("unexpected command: %s", cmd)
			},
			want: &InterfaceInfo{
				Name:      "eth0",
				MAC:       "52:54:00:12:34:56",
				IP:        "192.168.1.10",
				IsDefault: true,
			},
			wantErr: false,
		},
		{
			name: "error - no default interface",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{Stdout: "", ExitCode: 0}, nil
			},
			wantErr:     true,
			errContains: "no default route interface found",
		},
		{
			name: "error - MAC detection fails",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				if cmd == "ip route show default | head -n1 | awk '{print $5}'" {
					return &ssh.ExecResult{Stdout: "eth0\n", ExitCode: 0}, nil
				}
				if cmd == "cat /sys/class/net/eth0/address" {
					return &ssh.ExecResult{Stderr: "not found", ExitCode: 1}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			wantErr:     true,
			errContains: "failed to detect MAC",
		},
		{
			name: "error - IP detection fails",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				if cmd == "ip route show default | head -n1 | awk '{print $5}'" {
					return &ssh.ExecResult{Stdout: "eth0\n", ExitCode: 0}, nil
				}
				if cmd == "cat /sys/class/net/eth0/address" {
					return &ssh.ExecResult{Stdout: "52:54:00:12:34:56\n", ExitCode: 0}, nil
				}
				if cmd == "ip addr show eth0 | grep 'inet ' | head -n1 | awk '{print $2}' | cut -d'/' -f1" {
					return &ssh.ExecResult{Stdout: "", ExitCode: 0}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			wantErr:     true,
			errContains: "failed to detect IP",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &mockConnection{execFunc: tt.execFunc}
			got, err := DetectInterface(conn)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestListInterfaces(t *testing.T) {
	tests := []struct {
		name        string
		execFunc    func(string) (*ssh.ExecResult, error)
		want        []*InterfaceInfo
		wantErr     bool
		errContains string
	}{
		{
			name: "success - multiple interfaces",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				// List interfaces command
				if cmd == "ip link show | grep '^[0-9]' | awk '{print $2}' | sed 's/:$//' | grep -v '^lo$'" {
					return &ssh.ExecResult{
						Stdout:   "eth0\neth1\n",
						ExitCode: 0,
					}, nil
				}
				// Default route
				if cmd == "ip route show default | head -n1 | awk '{print $5}'" {
					return &ssh.ExecResult{Stdout: "eth0\n", ExitCode: 0}, nil
				}
				// eth0 MAC
				if cmd == "cat /sys/class/net/eth0/address" {
					return &ssh.ExecResult{Stdout: "52:54:00:12:34:56\n", ExitCode: 0}, nil
				}
				// eth0 IP
				if cmd == "ip addr show eth0 | grep 'inet ' | head -n1 | awk '{print $2}' | cut -d'/' -f1" {
					return &ssh.ExecResult{Stdout: "192.168.1.10\n", ExitCode: 0}, nil
				}
				// eth1 MAC
				if cmd == "cat /sys/class/net/eth1/address" {
					return &ssh.ExecResult{Stdout: "52:54:00:12:34:57\n", ExitCode: 0}, nil
				}
				// eth1 IP
				if cmd == "ip addr show eth1 | grep 'inet ' | head -n1 | awk '{print $2}' | cut -d'/' -f1" {
					return &ssh.ExecResult{Stdout: "10.0.0.5\n", ExitCode: 0}, nil
				}
				return &ssh.ExecResult{ExitCode: 1}, fmt.Errorf("unexpected command: %s", cmd)
			},
			want: []*InterfaceInfo{
				{Name: "eth0", MAC: "52:54:00:12:34:56", IP: "192.168.1.10", IsDefault: true},
				{Name: "eth1", MAC: "52:54:00:12:34:57", IP: "10.0.0.5", IsDefault: false},
			},
			wantErr: false,
		},
		{
			name: "success - single interface",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				if cmd == "ip link show | grep '^[0-9]' | awk '{print $2}' | sed 's/:$//' | grep -v '^lo$'" {
					return &ssh.ExecResult{Stdout: "ens3\n", ExitCode: 0}, nil
				}
				if cmd == "ip route show default | head -n1 | awk '{print $5}'" {
					return &ssh.ExecResult{Stdout: "ens3\n", ExitCode: 0}, nil
				}
				if cmd == "cat /sys/class/net/ens3/address" {
					return &ssh.ExecResult{Stdout: "aa:bb:cc:dd:ee:ff\n", ExitCode: 0}, nil
				}
				if cmd == "ip addr show ens3 | grep 'inet ' | head -n1 | awk '{print $2}' | cut -d'/' -f1" {
					return &ssh.ExecResult{Stdout: "172.16.0.10\n", ExitCode: 0}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			want: []*InterfaceInfo{
				{Name: "ens3", MAC: "aa:bb:cc:dd:ee:ff", IP: "172.16.0.10", IsDefault: true},
			},
			wantErr: false,
		},
		{
			name: "success - partial info (MAC fails for one interface)",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				if cmd == "ip link show | grep '^[0-9]' | awk '{print $2}' | sed 's/:$//' | grep -v '^lo$'" {
					return &ssh.ExecResult{Stdout: "eth0\neth1\n", ExitCode: 0}, nil
				}
				if cmd == "ip route show default | head -n1 | awk '{print $5}'" {
					return &ssh.ExecResult{Stdout: "eth0\n", ExitCode: 0}, nil
				}
				// eth0 succeeds
				if cmd == "cat /sys/class/net/eth0/address" {
					return &ssh.ExecResult{Stdout: "52:54:00:12:34:56\n", ExitCode: 0}, nil
				}
				if cmd == "ip addr show eth0 | grep 'inet ' | head -n1 | awk '{print $2}' | cut -d'/' -f1" {
					return &ssh.ExecResult{Stdout: "192.168.1.10\n", ExitCode: 0}, nil
				}
				// eth1 MAC fails
				if cmd == "cat /sys/class/net/eth1/address" {
					return &ssh.ExecResult{Stderr: "not found", ExitCode: 1}, nil
				}
				// eth1 IP succeeds
				if cmd == "ip addr show eth1 | grep 'inet ' | head -n1 | awk '{print $2}' | cut -d'/' -f1" {
					return &ssh.ExecResult{Stdout: "10.0.0.5\n", ExitCode: 0}, nil
				}
				return &ssh.ExecResult{ExitCode: 0}, nil
			},
			want: []*InterfaceInfo{
				{Name: "eth0", MAC: "52:54:00:12:34:56", IP: "192.168.1.10", IsDefault: true},
				{Name: "eth1", MAC: "", IP: "10.0.0.5", IsDefault: false},
			},
			wantErr: false,
		},
		{
			name: "error - no interfaces found",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{Stdout: "", ExitCode: 0}, nil
			},
			wantErr:     true,
			errContains: "no network interfaces found",
		},
		{
			name: "error - command fails",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{Stderr: "ip: command not found", ExitCode: 127}, nil
			},
			wantErr:     true,
			errContains: "failed to list interfaces",
		},
		{
			name: "error - ssh error",
			execFunc: func(cmd string) (*ssh.ExecResult, error) {
				return nil, fmt.Errorf("connection timeout")
			},
			wantErr:     true,
			errContains: "failed to list interfaces",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &mockConnection{execFunc: tt.execFunc}
			got, err := ListInterfaces(conn)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, len(tt.want), len(got))
			for i, want := range tt.want {
				assert.Equal(t, want.Name, got[i].Name, "interface %d name mismatch", i)
				assert.Equal(t, want.MAC, got[i].MAC, "interface %d MAC mismatch", i)
				assert.Equal(t, want.IP, got[i].IP, "interface %d IP mismatch", i)
				assert.Equal(t, want.IsDefault, got[i].IsDefault, "interface %d IsDefault mismatch", i)
			}
		})
	}
}
