package k8s

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
)

// Client wraps Kubernetes client-go for easier interaction
type Client struct {
	clientset     kubernetes.Interface
	dynamicClient dynamic.Interface
	config        *rest.Config
}

// SecretResolver defines the interface for resolving secrets from OpenBAO
type SecretResolver interface {
	// ResolveSecret resolves a secret reference and returns the value
	ResolveSecret(ctx context.Context, path, key string) (string, error)
}

// NewClientFromKubeconfig creates a Kubernetes client from kubeconfig bytes
func NewClientFromKubeconfig(kubeconfig []byte) (*Client, error) {
	if len(kubeconfig) == 0 {
		return nil, fmt.Errorf("kubeconfig is empty")
	}

	// Build config from kubeconfig
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to build config from kubeconfig: %w", err)
	}

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	// Create dynamic client
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return &Client{
		clientset:     clientset,
		dynamicClient: dynamicClient,
		config:        config,
	}, nil
}

// NewClientFromOpenBAO creates a Kubernetes client by loading kubeconfig from OpenBAO
// path: the OpenBAO path where the kubeconfig is stored (e.g., "foundry-core/k3s/kubeconfig")
// key: the key within the secret (e.g., "value" or "kubeconfig")
func NewClientFromOpenBAO(ctx context.Context, resolver SecretResolver, path, key string) (*Client, error) {
	if resolver == nil {
		return nil, fmt.Errorf("secret resolver is nil")
	}

	if path == "" {
		return nil, fmt.Errorf("path is empty")
	}

	if key == "" {
		return nil, fmt.Errorf("key is empty")
	}

	// Load kubeconfig from OpenBAO
	kubeconfigStr, err := resolver.ResolveSecret(ctx, path, key)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig from OpenBAO: %w", err)
	}

	if kubeconfigStr == "" {
		return nil, fmt.Errorf("kubeconfig from OpenBAO is empty")
	}

	// Create client from kubeconfig
	return NewClientFromKubeconfig([]byte(kubeconfigStr))
}

// GetNodes retrieves all nodes in the cluster
func (c *Client) GetNodes(ctx context.Context) ([]*Node, error) {
	nodeList, err := c.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	nodes := make([]*Node, 0, len(nodeList.Items))
	for i := range nodeList.Items {
		nodes = append(nodes, NodeFromCoreV1(&nodeList.Items[i]))
	}

	return nodes, nil
}

// GetPods retrieves all pods in the specified namespace
// If namespace is empty, retrieves pods from all namespaces
func (c *Client) GetPods(ctx context.Context, namespace string) ([]*Pod, error) {
	var podList *corev1.PodList
	var err error

	if namespace == "" {
		// List pods from all namespaces
		podList, err = c.clientset.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	} else {
		// List pods from specific namespace
		podList, err = c.clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	pods := make([]*Pod, 0, len(podList.Items))
	for i := range podList.Items {
		pods = append(pods, PodFromCoreV1(&podList.Items[i]))
	}

	return pods, nil
}

// GetNamespace retrieves a namespace by name
func (c *Client) GetNamespace(ctx context.Context, name string) (*Namespace, error) {
	ns, err := c.clientset.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get namespace: %w", err)
	}

	return &Namespace{
		Name:              ns.Name,
		Status:            string(ns.Status.Phase),
		CreationTimestamp: ns.CreationTimestamp.Time,
	}, nil
}

// CreateNamespace creates a new namespace
func (c *Client) CreateNamespace(ctx context.Context, name string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	_, err := c.clientset.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	return nil
}

// ApplyManifest applies a YAML manifest to the cluster
// This supports single resources or multi-document YAML (separated by ---)
func (c *Client) ApplyManifest(ctx context.Context, manifest string) error {
	if manifest == "" {
		return fmt.Errorf("manifest is empty")
	}

	// Split multi-document YAML
	documents := strings.Split(manifest, "\n---")

	for _, doc := range documents {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}

		if err := c.applySingleManifest(ctx, doc); err != nil {
			return err
		}
	}

	return nil
}

// applySingleManifest applies a single YAML document to the cluster
func (c *Client) applySingleManifest(ctx context.Context, manifest string) error {
	// Parse the manifest as unstructured object
	var obj unstructured.Unstructured
	if err := yaml.Unmarshal([]byte(manifest), &obj); err != nil {
		return fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Extract GVK (GroupVersionKind)
	gvk := obj.GroupVersionKind()
	if gvk.Kind == "" {
		return fmt.Errorf("manifest missing kind field")
	}

	// Convert to GVR (GroupVersionResource)
	gvr := schema.GroupVersionResource{
		Group:    gvk.Group,
		Version:  gvk.Version,
		Resource: pluralizeKind(gvk.Kind),
	}

	// Determine if resource is cluster-scoped or namespaced
	isClusterScoped := isClusterScopedResource(gvk.Kind)

	// Determine namespace
	namespace := obj.GetNamespace()
	if namespace == "" && !isClusterScoped {
		namespace = metav1.NamespaceDefault
	}

	// Try to create the resource first
	var err error
	if isClusterScoped {
		_, err = c.dynamicClient.Resource(gvr).Create(ctx, &obj, metav1.CreateOptions{})
		if err != nil && strings.Contains(err.Error(), "already exists") {
			// Resource exists - for idempotency, just return success
			// (we could fetch and update, but for issuers/certs that's usually not needed)
			return nil
		}
	} else {
		_, err = c.dynamicClient.Resource(gvr).Namespace(namespace).Create(ctx, &obj, metav1.CreateOptions{})
		if err != nil && strings.Contains(err.Error(), "already exists") {
			// Resource exists - for idempotency, just return success
			return nil
		}
	}

	if err != nil {
		return fmt.Errorf("failed to apply manifest (kind=%s, name=%s): %w", gvk.Kind, obj.GetName(), err)
	}

	return nil
}

// isClusterScopedResource returns true if the resource kind is cluster-scoped
func isClusterScopedResource(kind string) bool {
	clusterScoped := map[string]bool{
		"ClusterIssuer":                   true,
		"ClusterRole":                     true,
		"ClusterRoleBinding":              true,
		"Namespace":                       true,
		"Node":                            true,
		"PersistentVolume":                true,
		"StorageClass":                    true,
		"CustomResourceDefinition":        true,
		"PriorityClass":                   true,
		"IngressClass":                    true,
		"RuntimeClass":                    true,
		"VolumeSnapshotClass":             true,
		"CSIDriver":                       true,
		"CSINode":                         true,
		"ValidatingWebhookConfiguration":  true,
		"MutatingWebhookConfiguration":    true,
		"GatewayClass":                    true, // Gateway API cluster-scoped resource
	}
	return clusterScoped[kind]
}

// pluralizeKind converts a Kind to its plural resource form
// This is a simple implementation - in production, use the discovery client
func pluralizeKind(kind string) string {
	// Simple pluralization rules
	switch kind {
	case "Endpoints":
		return "endpoints"
	case "Ingress":
		return "ingresses"
	case "GatewayClass":
		return "gatewayclasses"
	default:
		// Most resources just add 's' and lowercase
		return strings.ToLower(kind) + "s"
	}
}

// Clientset returns the underlying Kubernetes clientset for advanced usage
func (c *Client) Clientset() kubernetes.Interface {
	return c.clientset
}

// DynamicClient returns the underlying dynamic client for advanced usage
func (c *Client) DynamicClient() dynamic.Interface {
	return c.dynamicClient
}

// Config returns the underlying REST config
func (c *Client) Config() *rest.Config {
	return c.config
}

// GetSecret retrieves a secret from the specified namespace
func (c *Client) GetSecret(ctx context.Context, namespace, name string) (*corev1.Secret, error) {
	return c.clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
}

// CordonNode marks a node as unschedulable
func (c *Client) CordonNode(ctx context.Context, nodeName string) error {
	// Get the node
	node, err := c.clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get node: %w", err)
	}

	// Mark as unschedulable
	node.Spec.Unschedulable = true

	// Update the node
	_, err = c.clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to cordon node: %w", err)
	}

	return nil
}

// DrainNode evicts all pods from a node (except DaemonSets and system pods)
// timeout specifies how long to wait for pods to be evicted
func (c *Client) DrainNode(ctx context.Context, nodeName string, timeout time.Duration) error {
	// Get all pods on the node
	pods, err := c.clientset.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
	})
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	// Evict pods (skip DaemonSets and static pods)
	for i := range pods.Items {
		pod := &pods.Items[i]

		// Skip DaemonSet pods
		if isDaemonSetPod(pod) {
			continue
		}

		// Skip static pods (mirror pods)
		if isStaticPod(pod) {
			continue
		}

		// Delete the pod gracefully
		deleteOptions := metav1.DeleteOptions{
			GracePeriodSeconds: new(int64), // Use default grace period
		}
		*deleteOptions.GracePeriodSeconds = 30

		err := c.clientset.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, deleteOptions)
		if err != nil {
			// Continue on error - some pods may not be deletable
			fmt.Printf("Warning: failed to delete pod %s/%s: %v\n", pod.Namespace, pod.Name, err)
		}
	}

	// Wait for pods to be deleted (with timeout)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		remainingPods, err := c.clientset.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
		})
		if err != nil {
			return fmt.Errorf("failed to check remaining pods: %w", err)
		}

		// Count non-DaemonSet, non-static pods
		count := 0
		for i := range remainingPods.Items {
			pod := &remainingPods.Items[i]
			if !isDaemonSetPod(pod) && !isStaticPod(pod) {
				count++
			}
		}

		if count == 0 {
			return nil // All pods evicted
		}

		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout waiting for pods to be evicted")
}

// DeleteNode removes a node from the cluster
func (c *Client) DeleteNode(ctx context.Context, nodeName string) error {
	err := c.clientset.CoreV1().Nodes().Delete(ctx, nodeName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete node: %w", err)
	}
	return nil
}

// GetNodeLabels retrieves all labels from a node
func (c *Client) GetNodeLabels(ctx context.Context, nodeName string) (map[string]string, error) {
	node, err := c.clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get node: %w", err)
	}

	// Return a copy to prevent mutation
	labels := make(map[string]string, len(node.Labels))
	for k, v := range node.Labels {
		labels[k] = v
	}
	return labels, nil
}

// SetNodeLabels sets labels on a node
// Labels with empty values will be removed from the node
func (c *Client) SetNodeLabels(ctx context.Context, nodeName string, labels map[string]string) error {
	if len(labels) == 0 {
		return nil // Nothing to do
	}

	// Get the node
	node, err := c.clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get node: %w", err)
	}

	// Initialize labels map if nil
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}

	// Apply labels (empty value means remove)
	for key, value := range labels {
		if value == "" {
			delete(node.Labels, key)
		} else {
			node.Labels[key] = value
		}
	}

	// Update the node
	_, err = c.clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update node labels: %w", err)
	}

	return nil
}

// RemoveNodeLabel removes a specific label from a node
func (c *Client) RemoveNodeLabel(ctx context.Context, nodeName, key string) error {
	if key == "" {
		return fmt.Errorf("label key cannot be empty")
	}

	// Get the node
	node, err := c.clientset.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get node: %w", err)
	}

	// Check if label exists
	if node.Labels == nil {
		return nil // No labels to remove
	}
	if _, exists := node.Labels[key]; !exists {
		return nil // Label doesn't exist, nothing to do
	}

	// Remove the label
	delete(node.Labels, key)

	// Update the node
	_, err = c.clientset.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to remove node label: %w", err)
	}

	return nil
}

// isDaemonSetPod checks if a pod is managed by a DaemonSet
func isDaemonSetPod(pod *corev1.Pod) bool {
	for _, ownerRef := range pod.OwnerReferences {
		if ownerRef.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}

// isStaticPod checks if a pod is a static pod (mirror pod)
func isStaticPod(pod *corev1.Pod) bool {
	_, exists := pod.Annotations["kubernetes.io/config.mirror"]
	return exists
}

// DeleteJob deletes a Job by name in the specified namespace
func (c *Client) DeleteJob(ctx context.Context, namespace, name string) error {
	propagationPolicy := metav1.DeletePropagationBackground
	return c.clientset.BatchV1().Jobs(namespace).Delete(ctx, name, metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	})
}

// WaitForJobComplete waits for a Job to complete successfully
func (c *Client) WaitForJobComplete(ctx context.Context, namespace, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for job %s/%s to complete", namespace, name)
			}

			job, err := c.clientset.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				continue // Retry on error
			}

			// Check if job succeeded
			if job.Status.Succeeded > 0 {
				return nil
			}

			// Check if job failed
			if job.Status.Failed > 0 {
				return fmt.Errorf("job %s/%s failed", namespace, name)
			}
		}
	}
}

// CRDExists checks if a CustomResourceDefinition exists in the cluster
func (c *Client) CRDExists(ctx context.Context, crdName string) (bool, error) {
	crdGVR := schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}

	_, err := c.dynamicClient.Resource(crdGVR).Get(ctx, crdName, metav1.GetOptions{})
	if err != nil {
		// Check if it's a "not found" error
		if strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check CRD %s: %w", crdName, err)
	}
	return true, nil
}

// ServiceMonitorCRDExists checks if the ServiceMonitor CRD from Prometheus Operator is installed
func (c *Client) ServiceMonitorCRDExists(ctx context.Context) (bool, error) {
	return c.CRDExists(ctx, "servicemonitors.monitoring.coreos.com")
}

// PatchDeploymentArgs replaces an argument in a deployment's container args
// This is useful for modifying command-line arguments that can't be changed via Helm values
func (c *Client) PatchDeploymentArgs(ctx context.Context, namespace, name string, oldArg, newArg string) error {
	// Get the deployment
	deploy, err := c.clientset.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get deployment %s/%s: %w", namespace, name, err)
	}

	// Find and replace the argument in the first container
	if len(deploy.Spec.Template.Spec.Containers) == 0 {
		return fmt.Errorf("deployment %s/%s has no containers", namespace, name)
	}

	container := &deploy.Spec.Template.Spec.Containers[0]
	found := false
	newArgs := make([]string, 0, len(container.Args))
	for _, arg := range container.Args {
		if strings.HasPrefix(arg, oldArg) || arg == oldArg {
			// Replace with new argument (only if newArg is not empty)
			if newArg != "" {
				newArgs = append(newArgs, newArg)
			}
			found = true
		} else {
			newArgs = append(newArgs, arg)
		}
	}

	if !found {
		// Argument not found - nothing to replace
		return nil
	}

	// Update the container args
	container.Args = newArgs

	// Update the deployment
	_, err = c.clientset.AppsV1().Deployments(namespace).Update(ctx, deploy, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update deployment %s/%s: %w", namespace, name, err)
	}

	return nil
}
