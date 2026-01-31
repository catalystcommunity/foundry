package gatewayapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/yaml"

	"github.com/catalystcommunity/foundry/v1/internal/k8s"
)

const (
	// GatewayAPIReleaseURL is the base URL for Gateway API releases
	GatewayAPIReleaseURL = "https://github.com/kubernetes-sigs/gateway-api/releases/download"

	// ExperimentalInstallFile is the filename for the experimental CRD installation
	// We use experimental instead of standard because Contour requires BackendTLSPolicy
	// and other experimental CRDs for full Gateway API support
	ExperimentalInstallFile = "experimental-install.yaml"
)

// crdGVR is the GroupVersionResource for CRDs
var crdGVR = schema.GroupVersionResource{
	Group:    "apiextensions.k8s.io",
	Version:  "v1",
	Resource: "customresourcedefinitions",
}

// gatewayCRDs are the Gateway API CRD names (including experimental ones needed by Contour)
var gatewayCRDs = []string{
	"gatewayclasses.gateway.networking.k8s.io",
	"gateways.gateway.networking.k8s.io",
	"httproutes.gateway.networking.k8s.io",
	"referencegrants.gateway.networking.k8s.io",
	"grpcroutes.gateway.networking.k8s.io",
	"backendtlspolicies.gateway.networking.k8s.io", // Required by Contour
}

// Install installs the Gateway API CRDs
func Install(ctx context.Context, k8sClient *k8s.Client, cfg *Config) error {
	if k8sClient == nil {
		return fmt.Errorf("k8s client cannot be nil")
	}
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// Check if already installed
	installed, version, err := CheckCRDsInstalled(ctx, k8sClient)
	if err != nil {
		return fmt.Errorf("failed to check existing CRDs: %w", err)
	}

	if installed {
		// Compare versions
		if version == cfg.Version {
			return nil // Already at desired version
		}
		// Different version - will upgrade
	}

	// Download the Gateway API CRDs manifest (experimental for Contour compatibility)
	manifestURL := fmt.Sprintf("%s/%s/%s", GatewayAPIReleaseURL, cfg.Version, ExperimentalInstallFile)
	manifest, err := downloadManifest(ctx, manifestURL)
	if err != nil {
		return fmt.Errorf("failed to download Gateway API manifest: %w", err)
	}

	// Apply the manifest
	if err := applyMultiDocManifest(ctx, k8sClient.DynamicClient(), manifest); err != nil {
		return fmt.Errorf("failed to apply Gateway API CRDs: %w", err)
	}

	// Verify installation
	if err := verifyCRDsReady(ctx, k8sClient); err != nil {
		return fmt.Errorf("CRD verification failed: %w", err)
	}

	return nil
}

// CheckCRDsInstalled checks if Gateway API CRDs are installed and returns the version
func CheckCRDsInstalled(ctx context.Context, k8sClient *k8s.Client) (bool, string, error) {
	dynamicClient := k8sClient.DynamicClient()

	// Check for the core Gateway API CRDs
	for _, crdName := range gatewayCRDs {
		_, err := dynamicClient.Resource(crdGVR).Get(ctx, crdName, metav1.GetOptions{})
		if err != nil {
			return false, "", nil // CRD not found
		}
	}

	// Get version from annotations on gatewayclasses CRD
	crd, err := dynamicClient.Resource(crdGVR).Get(ctx, "gatewayclasses.gateway.networking.k8s.io", metav1.GetOptions{})
	if err != nil {
		return true, "", nil // CRDs exist but can't determine version
	}

	// Try to extract version from annotations
	annotations := crd.GetAnnotations()
	if version, ok := annotations["gateway.networking.k8s.io/bundle-version"]; ok {
		return true, version, nil
	}

	// Try labels
	labels := crd.GetLabels()
	if version, ok := labels["gateway.networking.k8s.io/bundle-version"]; ok {
		return true, version, nil
	}

	return true, "", nil // Installed but version unknown
}

// downloadManifest downloads a YAML manifest from a URL
func downloadManifest(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	return string(body), nil
}

// applyMultiDocManifest applies a multi-document YAML manifest to the cluster
func applyMultiDocManifest(ctx context.Context, dynamicClient dynamic.Interface, manifest string) error {
	// Split into individual documents
	docs := strings.Split(manifest, "\n---\n")

	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" || doc == "---" {
			continue
		}

		// Parse as unstructured
		var obj unstructured.Unstructured
		if err := yaml.Unmarshal([]byte(doc), &obj); err != nil {
			// Skip invalid YAML (might be comments or empty docs)
			continue
		}

		// Skip empty objects
		if obj.GetKind() == "" {
			continue
		}

		// Apply the resource
		if err := applyResource(ctx, dynamicClient, &obj); err != nil {
			return fmt.Errorf("apply %s/%s: %w", obj.GetKind(), obj.GetName(), err)
		}
	}

	return nil
}

// applyResource applies a single resource to the cluster (create or update)
func applyResource(ctx context.Context, dynamicClient dynamic.Interface, obj *unstructured.Unstructured) error {
	gvk := obj.GroupVersionKind()

	// Build GVR from GVK
	gvr := schema.GroupVersionResource{
		Group:    gvk.Group,
		Version:  gvk.Version,
		Resource: pluralizeKind(gvk.Kind),
	}

	namespace := obj.GetNamespace()
	name := obj.GetName()

	var err error
	if namespace == "" {
		// Cluster-scoped resource
		_, err = dynamicClient.Resource(gvr).Create(ctx, obj, metav1.CreateOptions{})
		if err != nil {
			// Resource might already exist - get it to fetch resourceVersion for update
			existing, getErr := dynamicClient.Resource(gvr).Get(ctx, name, metav1.GetOptions{})
			if getErr != nil {
				return err // Return original create error
			}
			// Copy resourceVersion from existing for update
			obj.SetResourceVersion(existing.GetResourceVersion())
			_, err = dynamicClient.Resource(gvr).Update(ctx, obj, metav1.UpdateOptions{})
		}
	} else {
		// Namespace-scoped resource
		_, err = dynamicClient.Resource(gvr).Namespace(namespace).Create(ctx, obj, metav1.CreateOptions{})
		if err != nil {
			// Resource might already exist - get it to fetch resourceVersion for update
			existing, getErr := dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
			if getErr != nil {
				return err // Return original create error
			}
			// Copy resourceVersion from existing for update
			obj.SetResourceVersion(existing.GetResourceVersion())
			_, err = dynamicClient.Resource(gvr).Namespace(namespace).Update(ctx, obj, metav1.UpdateOptions{})
		}
	}

	return err
}

// pluralizeKind converts a Kind to its plural resource name
func pluralizeKind(kind string) string {
	// Common Kubernetes resource pluralization rules
	switch kind {
	case "CustomResourceDefinition":
		return "customresourcedefinitions"
	case "Endpoints":
		return "endpoints"
	case "Ingress":
		return "ingresses"
	case "GatewayClass":
		return "gatewayclasses"
	case "Gateway":
		return "gateways"
	case "HTTPRoute":
		return "httproutes"
	case "ReferenceGrant":
		return "referencegrants"
	case "TCPRoute":
		return "tcproutes"
	case "TLSRoute":
		return "tlsroutes"
	case "UDPRoute":
		return "udproutes"
	case "GRPCRoute":
		return "grpcroutes"
	default:
		// Most resources just lowercase and add 's'
		return strings.ToLower(kind) + "s"
	}
}

// verifyCRDsReady waits for Gateway API CRDs to be established
func verifyCRDsReady(ctx context.Context, k8sClient *k8s.Client) error {
	dynamicClient := k8sClient.DynamicClient()

	timeout := time.After(2 * time.Minute)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for Gateway API CRDs to be ready")
		case <-ticker.C:
			allReady := true
			for _, crdName := range gatewayCRDs {
				crd, err := dynamicClient.Resource(crdGVR).Get(ctx, crdName, metav1.GetOptions{})
				if err != nil {
					allReady = false
					break
				}

				// Check if CRD is established
				if !isCRDEstablished(crd) {
					allReady = false
					break
				}
			}

			if allReady {
				return nil
			}
		}
	}
}

// isCRDEstablished checks if a CRD is in the Established condition
func isCRDEstablished(crd *unstructured.Unstructured) bool {
	conditions, found, err := unstructured.NestedSlice(crd.Object, "status", "conditions")
	if err != nil || !found {
		return false
	}

	for _, c := range conditions {
		condition, ok := c.(map[string]interface{})
		if !ok {
			continue
		}

		condType, _, _ := unstructured.NestedString(condition, "type")
		condStatus, _, _ := unstructured.NestedString(condition, "status")

		if condType == string(apiextv1.Established) && condStatus == "True" {
			return true
		}
	}

	return false
}
