package k3s

import (
	"context"
	"fmt"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockKubeconfigSSHExecutor is a separate mock for kubeconfig tests
// We can't reuse the one in vip_test.go
type mockKubeconfigSSHExecutor struct {
	execFunc func(command string) (*ssh.ExecResult, error)
}

func (m *mockKubeconfigSSHExecutor) Exec(command string) (*ssh.ExecResult, error) {
	if m.execFunc != nil {
		return m.execFunc(command)
	}
	return &ssh.ExecResult{ExitCode: 0}, nil
}

// Mock kubeconfig client for testing
type mockKubeconfigClient struct {
	readFunc  func(ctx context.Context, mount, path string) (map[string]interface{}, error)
	writeFunc func(ctx context.Context, mount, path string, data map[string]interface{}) error
}

func (m *mockKubeconfigClient) ReadSecretV2(ctx context.Context, mount, path string) (map[string]interface{}, error) {
	if m.readFunc != nil {
		return m.readFunc(ctx, mount, path)
	}
	return nil, nil
}

func (m *mockKubeconfigClient) WriteSecretV2(ctx context.Context, mount, path string, data map[string]interface{}) error {
	if m.writeFunc != nil {
		return m.writeFunc(ctx, mount, path, data)
	}
	return nil
}

func TestRetrieveKubeconfig(t *testing.T) {
	tests := []struct {
		name    string
		exec    func(command string) (*ssh.ExecResult, error)
		want    string
		wantErr bool
	}{
		{
			name: "successful retrieval",
			exec: func(command string) (*ssh.ExecResult, error) {
				assert.Contains(t, command, "sudo cat /etc/rancher/k3s/k3s.yaml")
				return &ssh.ExecResult{
					Stdout:   "apiVersion: v1\nkind: Config\nclusters:\n- cluster:\n    server: https://127.0.0.1:6443\n",
					ExitCode: 0,
				}, nil
			},
			want:    "apiVersion: v1\nkind: Config\nclusters:\n- cluster:\n    server: https://127.0.0.1:6443",
			wantErr: false,
		},
		{
			name: "command execution fails",
			exec: func(command string) (*ssh.ExecResult, error) {
				return nil, fmt.Errorf("connection error")
			},
			wantErr: true,
		},
		{
			name: "non-zero exit code",
			exec: func(command string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{
					ExitCode: 1,
					Stderr:   "file not found",
				}, nil
			},
			wantErr: true,
		},
		{
			name: "empty kubeconfig",
			exec: func(command string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{
					Stdout:   "   \n  \n",
					ExitCode: 0,
				}, nil
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &mockKubeconfigSSHExecutor{execFunc: tt.exec}
			got, err := RetrieveKubeconfig(executor)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestModifyKubeconfigServer(t *testing.T) {
	tests := []struct {
		name       string
		kubeconfig string
		vip        string
		want       string
	}{
		{
			name: "replace localhost with VIP",
			kubeconfig: `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
  name: default`,
			vip: "192.168.1.100",
			want: `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://192.168.1.100:6443
  name: default`,
		},
		{
			name: "multiple server references",
			kubeconfig: `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
  name: cluster1
- cluster:
    server: https://127.0.0.1:6443
  name: cluster2`,
			vip: "192.168.1.100",
			want: `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://192.168.1.100:6443
  name: cluster1
- cluster:
    server: https://192.168.1.100:6443
  name: cluster2`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ModifyKubeconfigServer(tt.kubeconfig, tt.vip)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestStoreKubeconfig(t *testing.T) {
	tests := []struct {
		name       string
		kubeconfig string
		writeFunc  func(ctx context.Context, mount, path string, data map[string]interface{}) error
		wantErr    bool
	}{
		{
			name:       "successful storage",
			kubeconfig: "apiVersion: v1\nkind: Config",
			writeFunc: func(ctx context.Context, mount, path string, data map[string]interface{}) error {
				assert.Equal(t, SecretMount, mount)
				assert.Equal(t, KubeconfigOpenBAOPath, path)
				assert.Contains(t, data, "kubeconfig")
				assert.Equal(t, "apiVersion: v1\nkind: Config", data["kubeconfig"])
				return nil
			},
			wantErr: false,
		},
		{
			name:       "empty kubeconfig",
			kubeconfig: "",
			wantErr:    true,
		},
		{
			name:       "write error",
			kubeconfig: "apiVersion: v1\nkind: Config",
			writeFunc: func(ctx context.Context, mount, path string, data map[string]interface{}) error {
				return fmt.Errorf("storage error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockKubeconfigClient{writeFunc: tt.writeFunc}
			err := StoreKubeconfig(context.Background(), client, tt.kubeconfig)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLoadKubeconfig(t *testing.T) {
	tests := []struct {
		name     string
		readFunc func(ctx context.Context, mount, path string) (map[string]interface{}, error)
		want     string
		wantErr  bool
	}{
		{
			name: "successful load",
			readFunc: func(ctx context.Context, mount, path string) (map[string]interface{}, error) {
				assert.Equal(t, SecretMount, mount)
				assert.Equal(t, KubeconfigOpenBAOPath, path)
				return map[string]interface{}{
					"kubeconfig": "apiVersion: v1\nkind: Config",
				}, nil
			},
			want:    "apiVersion: v1\nkind: Config",
			wantErr: false,
		},
		{
			name: "read error",
			readFunc: func(ctx context.Context, mount, path string) (map[string]interface{}, error) {
				return nil, fmt.Errorf("storage error")
			},
			wantErr: true,
		},
		{
			name: "kubeconfig not a string",
			readFunc: func(ctx context.Context, mount, path string) (map[string]interface{}, error) {
				return map[string]interface{}{
					"kubeconfig": 123,
				}, nil
			},
			wantErr: true,
		},
		{
			name: "empty kubeconfig",
			readFunc: func(ctx context.Context, mount, path string) (map[string]interface{}, error) {
				return map[string]interface{}{
					"kubeconfig": "",
				}, nil
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockKubeconfigClient{readFunc: tt.readFunc}
			got, err := LoadKubeconfig(context.Background(), client)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRetrieveAndStoreKubeconfig(t *testing.T) {
	tests := []struct {
		name      string
		vip       string
		exec      func(command string) (*ssh.ExecResult, error)
		writeFunc func(ctx context.Context, mount, path string, data map[string]interface{}) error
		wantErr   bool
	}{
		{
			name: "successful retrieval and storage",
			vip:  "192.168.1.100",
			exec: func(command string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{
					Stdout: `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
  name: default`,
					ExitCode: 0,
				}, nil
			},
			writeFunc: func(ctx context.Context, mount, path string, data map[string]interface{}) error {
				kubeconfig, ok := data["kubeconfig"].(string)
				require.True(t, ok)
				assert.Contains(t, kubeconfig, "https://192.168.1.100:6443")
				assert.NotContains(t, kubeconfig, "https://127.0.0.1:6443")
				return nil
			},
			wantErr: false,
		},
		{
			name: "retrieval error",
			vip:  "192.168.1.100",
			exec: func(command string) (*ssh.ExecResult, error) {
				return nil, fmt.Errorf("connection error")
			},
			wantErr: true,
		},
		{
			name: "storage error",
			vip:  "192.168.1.100",
			exec: func(command string) (*ssh.ExecResult, error) {
				return &ssh.ExecResult{
					Stdout:   "apiVersion: v1\nkind: Config",
					ExitCode: 0,
				}, nil
			},
			writeFunc: func(ctx context.Context, mount, path string, data map[string]interface{}) error {
				return fmt.Errorf("storage error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &mockKubeconfigSSHExecutor{execFunc: tt.exec}
			client := &mockKubeconfigClient{writeFunc: tt.writeFunc}

			err := RetrieveAndStoreKubeconfig(context.Background(), executor, client, tt.vip)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
