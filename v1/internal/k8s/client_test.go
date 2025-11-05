package k8s

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

// mockSecretResolver implements SecretResolver for testing
type mockSecretResolver struct {
	secrets map[string]map[string]string
	err     error
}

func newMockSecretResolver() *mockSecretResolver {
	return &mockSecretResolver{
		secrets: make(map[string]map[string]string),
	}
}

func (m *mockSecretResolver) ResolveSecret(ctx context.Context, path, key string) (string, error) {
	if m.err != nil {
		return "", m.err
	}

	if keys, ok := m.secrets[path]; ok {
		if val, ok := keys[key]; ok {
			return val, nil
		}
	}

	return "", fmt.Errorf("secret not found: %s:%s", path, key)
}

func (m *mockSecretResolver) setSecret(path, key, value string) {
	if m.secrets[path] == nil {
		m.secrets[path] = make(map[string]string)
	}
	m.secrets[path][key] = value
}

func TestNewClientFromKubeconfig(t *testing.T) {
	tests := []struct {
		name       string
		kubeconfig []byte
		wantErr    bool
		errMsg     string
	}{
		{
			name: "valid kubeconfig",
			kubeconfig: []byte(`
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
  name: default
contexts:
- context:
    cluster: default
    user: default
  name: default
current-context: default
users:
- name: default
  user:
    token: test-token
`),
			wantErr: false,
		},
		{
			name:       "empty kubeconfig",
			kubeconfig: []byte{},
			wantErr:    true,
			errMsg:     "kubeconfig is empty",
		},
		{
			name:       "invalid yaml",
			kubeconfig: []byte("invalid: yaml: content:"),
			wantErr:    true,
			errMsg:     "failed to build config from kubeconfig",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClientFromKubeconfig(tt.kubeconfig)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				assert.NotNil(t, client.clientset)
				assert.NotNil(t, client.dynamicClient)
				assert.NotNil(t, client.config)
			}
		})
	}
}

func TestNewClientFromOpenBAO(t *testing.T) {
	validKubeconfig := `
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
  name: default
contexts:
- context:
    cluster: default
    user: default
  name: default
current-context: default
users:
- name: default
  user:
    token: test-token
`

	tests := []struct {
		name     string
		resolver SecretResolver
		path     string
		key      string
		setup    func(*mockSecretResolver)
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "valid kubeconfig from OpenBAO",
			resolver: newMockSecretResolver(),
			path:     "foundry-core/k3s/kubeconfig",
			key:      "value",
			setup: func(r *mockSecretResolver) {
				r.setSecret("foundry-core/k3s/kubeconfig", "value", validKubeconfig)
			},
			wantErr: false,
		},
		{
			name:     "nil resolver",
			resolver: nil,
			path:     "foundry-core/k3s/kubeconfig",
			key:      "value",
			wantErr:  true,
			errMsg:   "secret resolver is nil",
		},
		{
			name:     "empty path",
			resolver: newMockSecretResolver(),
			path:     "",
			key:      "value",
			wantErr:  true,
			errMsg:   "path is empty",
		},
		{
			name:     "empty key",
			resolver: newMockSecretResolver(),
			path:     "foundry-core/k3s/kubeconfig",
			key:      "",
			wantErr:  true,
			errMsg:   "key is empty",
		},
		{
			name:     "secret not found",
			resolver: newMockSecretResolver(),
			path:     "foundry-core/k3s/kubeconfig",
			key:      "value",
			setup:    func(r *mockSecretResolver) {},
			wantErr:  true,
			errMsg:   "failed to load kubeconfig from OpenBAO",
		},
		{
			name:     "empty kubeconfig",
			resolver: newMockSecretResolver(),
			path:     "foundry-core/k3s/kubeconfig",
			key:      "value",
			setup: func(r *mockSecretResolver) {
				r.setSecret("foundry-core/k3s/kubeconfig", "value", "")
			},
			wantErr: true,
			errMsg:  "kubeconfig from OpenBAO is empty",
		},
		{
			name:     "resolver error",
			resolver: newMockSecretResolver(),
			path:     "foundry-core/k3s/kubeconfig",
			key:      "value",
			setup: func(r *mockSecretResolver) {
				r.err = fmt.Errorf("connection failed")
			},
			wantErr: true,
			errMsg:  "failed to load kubeconfig from OpenBAO",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil && tt.resolver != nil {
				if mock, ok := tt.resolver.(*mockSecretResolver); ok {
					tt.setup(mock)
				}
			}

			client, err := NewClientFromOpenBAO(context.Background(), tt.resolver, tt.path, tt.key)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
			}
		})
	}
}

func TestGetNodes(t *testing.T) {
	tests := []struct {
		name      string
		nodes     []runtime.Object
		wantCount int
		wantErr   bool
	}{
		{
			name: "single ready node",
			nodes: []runtime.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"node-role.kubernetes.io/control-plane": "",
						},
					},
					Status: corev1.NodeStatus{
						NodeInfo: corev1.NodeSystemInfo{
							KubeletVersion: "v1.28.0",
						},
						Addresses: []corev1.NodeAddress{
							{Type: corev1.NodeInternalIP, Address: "192.168.1.10"},
						},
						Conditions: []corev1.NodeCondition{
							{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
						},
					},
				},
			},
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "multiple nodes with different roles",
			nodes: []runtime.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "control-plane-1",
						Labels: map[string]string{
							"node-role.kubernetes.io/control-plane": "",
						},
					},
					Status: corev1.NodeStatus{
						NodeInfo: corev1.NodeSystemInfo{
							KubeletVersion: "v1.28.0",
						},
						Conditions: []corev1.NodeCondition{
							{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "worker-1",
						Labels: map[string]string{
							"node-role.kubernetes.io/worker": "",
						},
					},
					Status: corev1.NodeStatus{
						NodeInfo: corev1.NodeSystemInfo{
							KubeletVersion: "v1.28.0",
						},
						Conditions: []corev1.NodeCondition{
							{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
						},
					},
				},
			},
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "no nodes",
			nodes:     []runtime.Object{},
			wantCount: 0,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewSimpleClientset(tt.nodes...)
			client := &Client{clientset: fakeClient}

			nodes, err := client.GetNodes(context.Background())

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, nodes, tt.wantCount)

				// Verify node properties
				for _, node := range nodes {
					assert.NotEmpty(t, node.Name)
					assert.NotEmpty(t, node.Status)
					assert.NotEmpty(t, node.Roles)
				}
			}
		})
	}
}

func TestGetPods(t *testing.T) {
	tests := []struct {
		name      string
		pods      []runtime.Object
		namespace string
		wantCount int
		wantErr   bool
	}{
		{
			name: "pods in specific namespace",
			pods: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "default",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
						ContainerStatuses: []corev1.ContainerStatus{
							{
								Name:  "container1",
								Image: "nginx:latest",
								Ready: true,
								State: corev1.ContainerState{
									Running: &corev1.ContainerStateRunning{},
								},
							},
						},
					},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: "kube-system",
					},
					Status: corev1.PodStatus{
						Phase: corev1.PodRunning,
					},
				},
			},
			namespace: "default",
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "all pods across namespaces",
			pods: []runtime.Object{
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod1",
						Namespace: "default",
					},
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				},
				&corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "pod2",
						Namespace: "kube-system",
					},
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				},
			},
			namespace: "",
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "no pods",
			pods:      []runtime.Object{},
			namespace: "default",
			wantCount: 0,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewSimpleClientset(tt.pods...)
			client := &Client{clientset: fakeClient}

			pods, err := client.GetPods(context.Background(), tt.namespace)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, pods, tt.wantCount)

				// Verify pod properties
				for _, pod := range pods {
					assert.NotEmpty(t, pod.Name)
					assert.NotEmpty(t, pod.Namespace)
					assert.NotEmpty(t, pod.Status)
				}
			}
		})
	}
}

func TestApplyManifest(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		wantErr  bool
		errMsg   string
	}{
		{
			name:     "empty manifest",
			manifest: "",
			wantErr:  true,
			errMsg:   "manifest is empty",
		},
		{
			name:     "invalid yaml",
			manifest: "invalid: yaml: content:",
			wantErr:  true,
			errMsg:   "failed to parse manifest",
		},
		{
			name: "missing kind",
			manifest: `
apiVersion: v1
metadata:
  name: test
`,
			wantErr: true,
			errMsg:  "failed to parse manifest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test validation logic (dynamic client testing would require more complex setup)
			client := &Client{}
			err := client.ApplyManifest(context.Background(), tt.manifest)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			}
		})
	}
}

func TestNodeFromCoreV1(t *testing.T) {
	t.Run("ready node with all fields", func(t *testing.T) {
		coreNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node",
				Labels: map[string]string{
					"node-role.kubernetes.io/control-plane": "",
				},
			},
			Status: corev1.NodeStatus{
				NodeInfo: corev1.NodeSystemInfo{
					KubeletVersion: "v1.28.0",
				},
				Addresses: []corev1.NodeAddress{
					{Type: corev1.NodeInternalIP, Address: "192.168.1.10"},
					{Type: corev1.NodeExternalIP, Address: "1.2.3.4"},
				},
				Conditions: []corev1.NodeCondition{
					{
						Type:    corev1.NodeReady,
						Status:  corev1.ConditionTrue,
						Reason:  "KubeletReady",
						Message: "kubelet is posting ready status",
					},
				},
			},
		}

		node := NodeFromCoreV1(coreNode)

		require.NotNil(t, node)
		assert.Equal(t, "test-node", node.Name)
		assert.Equal(t, "v1.28.0", node.Version)
		assert.Equal(t, "192.168.1.10", node.InternalIP)
		assert.Equal(t, "1.2.3.4", node.ExternalIP)
		assert.Equal(t, "Ready", node.Status)
		assert.Contains(t, node.Roles, "control-plane")
		assert.Len(t, node.Conditions, 1)
		assert.Equal(t, "Ready", node.Conditions[0].Type)
	})

	t.Run("not ready node", func(t *testing.T) {
		coreNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node",
			},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionFalse,
					},
				},
			},
		}

		node := NodeFromCoreV1(coreNode)
		assert.Equal(t, "NotReady", node.Status)
	})

	t.Run("node with no addresses", func(t *testing.T) {
		coreNode := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node",
			},
			Status: corev1.NodeStatus{
				Addresses: []corev1.NodeAddress{},
			},
		}

		node := NodeFromCoreV1(coreNode)
		assert.Empty(t, node.InternalIP)
		assert.Empty(t, node.ExternalIP)
	})
}

func TestExtractNodeRoles(t *testing.T) {
	tests := []struct {
		name      string
		labels    map[string]string
		wantRoles []string
	}{
		{
			name: "control-plane node",
			labels: map[string]string{
				"node-role.kubernetes.io/control-plane": "",
			},
			wantRoles: []string{"control-plane"},
		},
		{
			name: "worker node",
			labels: map[string]string{
				"node-role.kubernetes.io/worker": "",
			},
			wantRoles: []string{"worker"},
		},
		{
			name: "master node (old label)",
			labels: map[string]string{
				"node-role.kubernetes.io/master": "",
			},
			wantRoles: []string{"control-plane"},
		},
		{
			name: "both control-plane and worker",
			labels: map[string]string{
				"node-role.kubernetes.io/control-plane": "",
				"node-role.kubernetes.io/worker":        "",
			},
			wantRoles: []string{"control-plane", "worker"},
		},
		{
			name:      "no role labels - defaults to worker",
			labels:    map[string]string{},
			wantRoles: []string{"worker"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Labels: tt.labels,
				},
			}

			roles := extractNodeRoles(node)
			assert.ElementsMatch(t, tt.wantRoles, roles)
		})
	}
}

func TestPodFromCoreV1(t *testing.T) {
	t.Run("pod with multiple containers", func(t *testing.T) {
		corePod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				NodeName: "test-node",
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				PodIP: "10.0.0.5",
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:         "nginx",
						Image:        "nginx:1.21",
						Ready:        true,
						RestartCount: 2,
						State: corev1.ContainerState{
							Running: &corev1.ContainerStateRunning{},
						},
					},
					{
						Name:         "sidecar",
						Image:        "sidecar:latest",
						Ready:        false,
						RestartCount: 0,
						State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{
								Reason: "ImagePullBackOff",
							},
						},
					},
				},
			},
		}

		pod := PodFromCoreV1(corePod)

		require.NotNil(t, pod)
		assert.Equal(t, "test-pod", pod.Name)
		assert.Equal(t, "default", pod.Namespace)
		assert.Equal(t, corev1.PodRunning, pod.Phase)
		assert.Equal(t, "Running", pod.Status)
		assert.Equal(t, "test-node", pod.NodeName)
		assert.Equal(t, "10.0.0.5", pod.PodIP)
		assert.Len(t, pod.Containers, 2)

		// Check first container
		assert.Equal(t, "nginx", pod.Containers[0].Name)
		assert.Equal(t, "nginx:1.21", pod.Containers[0].Image)
		assert.True(t, pod.Containers[0].Ready)
		assert.Equal(t, "Running", pod.Containers[0].State)
		assert.Equal(t, int32(2), pod.Containers[0].Restarts)

		// Check second container
		assert.Equal(t, "sidecar", pod.Containers[1].Name)
		assert.False(t, pod.Containers[1].Ready)
		assert.Equal(t, "Waiting", pod.Containers[1].State)
	})

	t.Run("pod with terminated container", func(t *testing.T) {
		corePod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "terminated-pod",
				Namespace: "default",
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodFailed,
				ContainerStatuses: []corev1.ContainerStatus{
					{
						Name:  "failed-container",
						Image: "app:1.0",
						State: corev1.ContainerState{
							Terminated: &corev1.ContainerStateTerminated{
								ExitCode: 1,
							},
						},
					},
				},
			},
		}

		pod := PodFromCoreV1(corePod)
		assert.Equal(t, "Terminated", pod.Containers[0].State)
	})

	t.Run("pod with no containers", func(t *testing.T) {
		corePod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "empty-pod",
				Namespace: "default",
			},
			Status: corev1.PodStatus{
				Phase:             corev1.PodPending,
				ContainerStatuses: []corev1.ContainerStatus{},
			},
		}

		pod := PodFromCoreV1(corePod)
		assert.Len(t, pod.Containers, 0)
		assert.Equal(t, "Pending", pod.Status)
	})
}

func TestClientAccessors(t *testing.T) {
	fakeClientset := fake.NewSimpleClientset()
	client := &Client{
		clientset: fakeClientset,
		config:    &rest.Config{},
	}

	assert.NotNil(t, client.Clientset())
	assert.NotNil(t, client.Config())
	// DynamicClient is nil in this test since we didn't set it
	assert.Nil(t, client.DynamicClient())
}

func TestPluralizeKind(t *testing.T) {
	tests := []struct {
		kind string
		want string
	}{
		{"Pod", "Pods"},
		{"Service", "Services"},
		{"Deployment", "Deployments"},
		{"Endpoints", "endpoints"},
		{"Ingress", "ingresses"},
		{"ConfigMap", "ConfigMaps"},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			got := pluralizeKind(tt.kind)
			assert.Equal(t, tt.want, got)
		})
	}
}
