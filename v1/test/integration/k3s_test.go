package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/component/k3s"
	"github.com/catalystcommunity/foundry/v1/internal/component/openbao"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	"github.com/catalystcommunity/foundry/v1/internal/secrets"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/kind/pkg/cluster"
	kindconfig "sigs.k8s.io/kind/pkg/apis/config/v1alpha4"
)

// TestK3sIntegration tests K3s cluster operations using Kind
// Kind simulates a Kubernetes cluster for testing our K8s client operations
func TestK3sIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Create unique cluster name
	clusterName := fmt.Sprintf("foundry-test-%d", time.Now().Unix())

	// Create Kind provider
	provider := cluster.NewProvider()

	// Start OpenBAO container for kubeconfig storage tests
	openbaoContainer, apiURL, rootToken, err := startOpenBAOContainer(ctx, t)
	require.NoError(t, err, "Failed to start OpenBAO container")
	defer func() {
		if err := openbaoContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate OpenBAO container: %v", err)
		}
	}()

	// Create Kind cluster configuration
	kindCfg := &kindconfig.Cluster{
		TypeMeta: kindconfig.TypeMeta{
			APIVersion: "kind.x-k8s.io/v1alpha4",
			Kind:       "Cluster",
		},
		Nodes: []kindconfig.Node{
			{
				Role: kindconfig.ControlPlaneRole,
			},
		},
	}

	// Create the cluster
	t.Logf("Creating Kind cluster: %s", clusterName)
	err = provider.Create(
		clusterName,
		cluster.CreateWithV1Alpha4Config(kindCfg),
		cluster.CreateWithWaitForReady(time.Minute*5),
	)
	require.NoError(t, err, "Failed to create Kind cluster")

	// Ensure cleanup
	defer func() {
		t.Logf("Deleting Kind cluster: %s", clusterName)
		if err := provider.Delete(clusterName, ""); err != nil {
			t.Logf("Failed to delete cluster: %v", err)
		}
	}()

	// Get kubeconfig from Kind
	// Kind stores kubeconfig in ~/.kube/config by default or in KUBECONFIG env
	kubeconfigPath := filepath.Join(os.Getenv("HOME"), ".kube", "kind-config-"+clusterName)

	// Export kubeconfig for this specific cluster
	err = provider.ExportKubeConfig(clusterName, kubeconfigPath, false)
	require.NoError(t, err, "Failed to export kubeconfig")
	t.Logf("Kubeconfig path: %s", kubeconfigPath)

	// Read kubeconfig file
	kubeconfigData, err := os.ReadFile(kubeconfigPath)
	require.NoError(t, err, "Failed to read kubeconfig")
	require.NotEmpty(t, kubeconfigData, "Kubeconfig should not be empty")

	t.Run("Kubeconfig_Retrieval", func(t *testing.T) {
		// Verify kubeconfig is valid YAML
		require.Contains(t, string(kubeconfigData), "apiVersion:", "Kubeconfig should be valid YAML")
		require.Contains(t, string(kubeconfigData), "clusters:", "Kubeconfig should contain clusters")
		require.Contains(t, string(kubeconfigData), "users:", "Kubeconfig should contain users")
		require.Contains(t, string(kubeconfigData), "contexts:", "Kubeconfig should contain contexts")
	})

	t.Run("Kubeconfig_Storage_In_OpenBAO", func(t *testing.T) {
		// Create OpenBAO client
		client := openbao.NewClient(apiURL, rootToken)

		// Test storing kubeconfig in OpenBAO
		kubeconfigPath := "foundry-core/k3s/kubeconfig"
		kubeconfigSecret := map[string]interface{}{
			"kubeconfig": string(kubeconfigData),
		}

		err := client.WriteSecretV2(ctx, "secret", kubeconfigPath, kubeconfigSecret)
		require.NoError(t, err, "Should store kubeconfig in OpenBAO")

		// Retrieve kubeconfig from OpenBAO
		retrieved, err := client.ReadSecretV2(ctx, "secret", kubeconfigPath)
		require.NoError(t, err, "Should retrieve kubeconfig from OpenBAO")
		require.NotNil(t, retrieved, "Retrieved kubeconfig should not be nil")

		retrievedConfig, ok := retrieved["kubeconfig"].(string)
		require.True(t, ok, "Kubeconfig should be a string")
		assert.Equal(t, string(kubeconfigData), retrievedConfig, "Retrieved kubeconfig should match")
	})

	t.Run("K8s_Client_From_Kubeconfig", func(t *testing.T) {
		// Create K8s client from kubeconfig
		k8sClient, err := k8s.NewClientFromKubeconfig(kubeconfigData)
		require.NoError(t, err, "Should create K8s client from kubeconfig")
		require.NotNil(t, k8sClient, "K8s client should not be nil")

		// List nodes
		nodes, err := k8sClient.GetNodes(ctx)
		require.NoError(t, err, "Should list nodes")
		assert.NotEmpty(t, nodes, "Cluster should have at least one node")
		assert.Equal(t, 1, len(nodes), "Should have exactly one node (control-plane)")

		// Verify node properties
		node := nodes[0]
		assert.NotEmpty(t, node.Name, "Node should have a name")
		assert.Contains(t, node.Name, "control-plane", "Node name should contain 'control-plane'")
		assert.True(t, node.Ready, "Node should be ready")
	})

	t.Run("K8s_Client_From_OpenBAO", func(t *testing.T) {
		// Create resolver with OpenBAO
		resolver, err := secrets.NewOpenBAOResolverWithMount(apiURL, rootToken, "secret")
		require.NoError(t, err, "Should create OpenBAO resolver")

		// Store kubeconfig in OpenBAO first
		client := openbao.NewClient(apiURL, rootToken)
		kubeconfigPath := "foundry-core/k3s/kubeconfig"
		kubeconfigSecret := map[string]interface{}{
			"value": string(kubeconfigData),
		}
		err = client.WriteSecretV2(ctx, "secret", kubeconfigPath, kubeconfigSecret)
		require.NoError(t, err, "Should store kubeconfig in OpenBAO")

		// Create K8s client from OpenBAO
		k8sClient, err := k8s.NewClientFromOpenBAO(ctx, resolver, kubeconfigPath, "value")
		require.NoError(t, err, "Should create K8s client from OpenBAO")
		require.NotNil(t, k8sClient, "K8s client should not be nil")

		// Verify client works
		nodes, err := k8sClient.GetNodes(ctx)
		require.NoError(t, err, "Should list nodes")
		assert.Equal(t, 1, len(nodes), "Should have one node")
	})

	t.Run("Cluster_Health_Checks", func(t *testing.T) {
		// Create K8s client
		k8sClient, err := k8s.NewClientFromKubeconfig(kubeconfigData)
		require.NoError(t, err, "Should create K8s client")

		// Check cluster health via nodes
		nodes, err := k8sClient.GetNodes(ctx)
		require.NoError(t, err, "Should get nodes")
		require.NotEmpty(t, nodes, "Should have nodes")

		allReady := true
		for _, node := range nodes {
			if !node.Ready {
				allReady = false
				break
			}
		}
		assert.True(t, allReady, "All nodes should be ready")

		// Check system pods are running
		pods, err := k8sClient.GetPods(ctx, "kube-system")
		require.NoError(t, err, "Should get kube-system pods")
		assert.NotEmpty(t, pods, "kube-system should have pods")

		// Count running pods
		runningCount := 0
		for _, pod := range pods {
			if pod.Status == "Running" {
				runningCount++
			}
		}
		assert.Greater(t, runningCount, 0, "Should have running pods in kube-system")
	})

	t.Run("Node_Operations", func(t *testing.T) {
		// Create K8s client
		k8sClient, err := k8s.NewClientFromKubeconfig(kubeconfigData)
		require.NoError(t, err, "Should create K8s client")

		// List nodes
		nodes, err := k8sClient.GetNodes(ctx)
		require.NoError(t, err, "Should list nodes")
		require.Equal(t, 1, len(nodes), "Should have one node")

		node := nodes[0]

		// Verify node details
		assert.NotEmpty(t, node.Name, "Node should have name")
		assert.NotEmpty(t, node.Status, "Node should have status")
		assert.True(t, node.Ready, "Node should be ready")
		assert.Contains(t, node.Roles, "control-plane", "Node should have control-plane role")

		// Check node has allocatable resources
		assert.NotNil(t, node.AllocatableCPU, "Node should have allocatable CPU")
		assert.NotNil(t, node.AllocatableMemory, "Node should have allocatable memory")

		t.Logf("Node: %s, Status: %s, Ready: %v, Roles: %v",
			node.Name, node.Status, node.Ready, node.Roles)
	})

	t.Run("Kubeconfig_Helper_Functions", func(t *testing.T) {
		// Test the kubeconfig validation helper from k3s package
		// This tests that our kubeconfig handling logic works
		tempDir := t.TempDir()
		kubeconfigFile := filepath.Join(tempDir, "kubeconfig.yaml")

		err := os.WriteFile(kubeconfigFile, kubeconfigData, 0600)
		require.NoError(t, err, "Should write kubeconfig to temp file")

		// Read it back
		readData, err := os.ReadFile(kubeconfigFile)
		require.NoError(t, err, "Should read kubeconfig")
		assert.Equal(t, kubeconfigData, readData, "Kubeconfig should match")

		// Verify it can be used to create a client
		k8sClient, err := k8s.NewClientFromKubeconfig(readData)
		require.NoError(t, err, "Should create client from file-based kubeconfig")
		require.NotNil(t, k8sClient, "Client should not be nil")
	})

	t.Run("Token_Storage_Integration", func(t *testing.T) {
		// Test that we can store and retrieve K3s tokens from OpenBAO
		// This validates the integration between k3s package and OpenBAO
		client := openbao.NewClient(apiURL, rootToken)

		// Create test tokens
		testTokens := &k3s.Tokens{
			ClusterToken: "test-cluster-token-abc123",
			AgentToken:   "test-agent-token-def456",
		}

		// Store tokens using the same path structure as k3s package
		clusterTokenData := map[string]interface{}{
			"value": testTokens.ClusterToken,
		}
		err := client.WriteSecretV2(ctx, "secret", "foundry-core/k3s/cluster-token", clusterTokenData)
		require.NoError(t, err, "Should store cluster token")

		agentTokenData := map[string]interface{}{
			"value": testTokens.AgentToken,
		}
		err = client.WriteSecretV2(ctx, "secret", "foundry-core/k3s/agent-token", agentTokenData)
		require.NoError(t, err, "Should store agent token")

		// Retrieve tokens
		clusterTokenSecret, err := client.ReadSecretV2(ctx, "secret", "foundry-core/k3s/cluster-token")
		require.NoError(t, err, "Should retrieve cluster token")
		assert.Equal(t, testTokens.ClusterToken, clusterTokenSecret["value"], "Cluster token should match")

		agentTokenSecret, err := client.ReadSecretV2(ctx, "secret", "foundry-core/k3s/agent-token")
		require.NoError(t, err, "Should retrieve agent token")
		assert.Equal(t, testTokens.AgentToken, agentTokenSecret["value"], "Agent token should match")
	})
}
