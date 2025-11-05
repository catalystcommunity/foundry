package k3s

import (
	"context"
	"fmt"
	"strings"
)

// KubeconfigClient defines the interface for storing and retrieving kubeconfig
type KubeconfigClient interface {
	ReadSecretV2(ctx context.Context, mount, path string) (map[string]interface{}, error)
	WriteSecretV2(ctx context.Context, mount, path string, data map[string]interface{}) error
}

// RetrieveKubeconfig retrieves the kubeconfig from a K3s control plane node
func RetrieveKubeconfig(executor SSHExecutor) (string, error) {
	// Read the kubeconfig file from the standard K3s location
	result, err := executor.Exec(fmt.Sprintf("sudo cat %s", KubeconfigPath))
	if err != nil {
		return "", fmt.Errorf("failed to retrieve kubeconfig: %w", err)
	}

	if result.ExitCode != 0 {
		return "", fmt.Errorf("failed to retrieve kubeconfig: exit code %d, stderr: %s", result.ExitCode, result.Stderr)
	}

	kubeconfig := strings.TrimSpace(result.Stdout)
	if kubeconfig == "" {
		return "", fmt.Errorf("kubeconfig is empty")
	}

	return kubeconfig, nil
}

// ModifyKubeconfigServer modifies the server URL in a kubeconfig
// This replaces 127.0.0.1 with the actual VIP for remote access
func ModifyKubeconfigServer(kubeconfig string, vip string) string {
	// K3s defaults to using 127.0.0.1:6443 in the kubeconfig
	// We need to replace this with the VIP for remote access
	modified := strings.ReplaceAll(kubeconfig, "https://127.0.0.1:6443", fmt.Sprintf("https://%s:6443", vip))
	return modified
}

// StoreKubeconfig stores the kubeconfig in OpenBAO
func StoreKubeconfig(ctx context.Context, client KubeconfigClient, kubeconfig string) error {
	if kubeconfig == "" {
		return fmt.Errorf("kubeconfig cannot be empty")
	}

	data := map[string]interface{}{
		"kubeconfig": kubeconfig,
	}

	if err := client.WriteSecretV2(ctx, SecretMount, KubeconfigOpenBAOPath, data); err != nil {
		return fmt.Errorf("failed to store kubeconfig: %w", err)
	}

	return nil
}

// LoadKubeconfig retrieves the kubeconfig from OpenBAO
func LoadKubeconfig(ctx context.Context, client KubeconfigClient) (string, error) {
	data, err := client.ReadSecretV2(ctx, SecretMount, KubeconfigOpenBAOPath)
	if err != nil {
		return "", fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	kubeconfig, ok := data["kubeconfig"].(string)
	if !ok {
		return "", fmt.Errorf("kubeconfig is not a string")
	}

	if kubeconfig == "" {
		return "", fmt.Errorf("kubeconfig is empty")
	}

	return kubeconfig, nil
}

// RetrieveAndStoreKubeconfig is a convenience function that retrieves the kubeconfig
// from a K3s node, modifies it to use the VIP, and stores it in OpenBAO
func RetrieveAndStoreKubeconfig(ctx context.Context, executor SSHExecutor, client KubeconfigClient, vip string) error {
	// Retrieve kubeconfig from node
	kubeconfig, err := RetrieveKubeconfig(executor)
	if err != nil {
		return fmt.Errorf("failed to retrieve kubeconfig: %w", err)
	}

	// Modify server URL to use VIP
	kubeconfig = ModifyKubeconfigServer(kubeconfig, vip)

	// Store in OpenBAO
	if err := StoreKubeconfig(ctx, client, kubeconfig); err != nil {
		return fmt.Errorf("failed to store kubeconfig: %w", err)
	}

	return nil
}
