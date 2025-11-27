package k3s

import (
	"fmt"
	"net"
	"strings"

	"github.com/catalystcommunity/foundry/v1/internal/network"
	"github.com/catalystcommunity/foundry/v1/internal/ssh"
)

// VIPConfig holds the configuration for kube-vip
type VIPConfig struct {
	VIP       string
	Interface string
}

// SSHExecutor is an interface for executing SSH commands
// This allows for easier testing with mocks
type SSHExecutor interface {
	Exec(command string) (*ssh.ExecResult, error)
}

// ValidateVIP validates that a VIP address is in correct format
func ValidateVIP(vip string) error {
	if vip == "" {
		return fmt.Errorf("VIP address cannot be empty")
	}

	// Parse as IP address
	ip := net.ParseIP(vip)
	if ip == nil {
		return fmt.Errorf("invalid VIP address format: %s", vip)
	}

	// Check if it's IPv4 (kube-vip supports IPv6 but we'll start with IPv4)
	if ip.To4() == nil {
		return fmt.Errorf("VIP must be an IPv4 address: %s", vip)
	}

	// Check if it's a private IP (RFC1918)
	if !isPrivateIP(ip) {
		return fmt.Errorf("VIP should be a private IP address: %s", vip)
	}

	return nil
}

// isPrivateIP checks if an IP is in private ranges (RFC1918)
func isPrivateIP(ip net.IP) bool {
	private := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}

	for _, cidr := range private {
		_, subnet, _ := net.ParseCIDR(cidr)
		if subnet.Contains(ip) {
			return true
		}
	}

	return false
}

// DetectNetworkInterface detects the primary network interface on the host
// This wraps the network.DetectPrimaryInterface function for compatibility
func DetectNetworkInterface(conn network.SSHExecutor) (string, error) {
	iface, err := network.DetectPrimaryInterface(conn)
	if err != nil {
		return "", fmt.Errorf("failed to detect network interface: %w", err)
	}

	if iface == "" {
		return "", fmt.Errorf("no network interface detected")
	}

	return iface, nil
}

// DetermineVIPConfig determines the VIP configuration for the cluster
// It validates the VIP and detects the network interface
func DetermineVIPConfig(vip string, conn network.SSHExecutor) (*VIPConfig, error) {
	// Validate VIP
	if err := ValidateVIP(vip); err != nil {
		return nil, fmt.Errorf("VIP validation failed: %w", err)
	}

	// Detect network interface
	iface, err := DetectNetworkInterface(conn)
	if err != nil {
		return nil, fmt.Errorf("interface detection failed: %w", err)
	}

	return &VIPConfig{
		VIP:       vip,
		Interface: iface,
	}, nil
}

// GenerateKubeVIPManifest generates the kube-vip DaemonSet manifest YAML
// This manifest is deployed to the cluster to enable VIP functionality
func GenerateKubeVIPManifest(cfg *VIPConfig) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("VIP config cannot be nil")
	}

	if cfg.VIP == "" {
		return "", fmt.Errorf("VIP address is required")
	}

	if cfg.Interface == "" {
		return "", fmt.Errorf("interface is required")
	}

	// Validate VIP one more time
	if err := ValidateVIP(cfg.VIP); err != nil {
		return "", fmt.Errorf("invalid VIP configuration: %w", err)
	}

	// Generate the manifest
	// Based on kube-vip documentation and the user's existing scripts
	manifest := fmt.Sprintf(`apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: kube-vip
  namespace: kube-system
  labels:
    app.kubernetes.io/name: kube-vip
    app.kubernetes.io/instance: kube-vip
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: kube-vip
  template:
    metadata:
      labels:
        app.kubernetes.io/name: kube-vip
    spec:
      containers:
      - name: kube-vip
        image: ghcr.io/kube-vip/kube-vip:v0.6.4
        imagePullPolicy: IfNotPresent
        args:
        - manager
        env:
        - name: vip_interface
          value: "%s"
        - name: vip_arp
          value: "true"
        - name: port
          value: "6443"
        - name: vip_cidr
          value: "32"
        - name: cp_enable
          value: "true"
        - name: cp_namespace
          value: kube-system
        - name: svc_enable
          value: "true"
        - name: vip_address
          value: "%s"
        - name: vip_ddns
          value: "false"
        - name: vip_leaderelection
          value: "true"
        - name: vip_leaseduration
          value: "5"
        - name: vip_renewdeadline
          value: "3"
        - name: vip_retryperiod
          value: "1"
        - name: KUBECONFIG
          value: /etc/rancher/k3s/k3s.yaml
        securityContext:
          capabilities:
            add:
            - NET_ADMIN
            - NET_RAW
        volumeMounts:
        - name: kubeconfig
          mountPath: /etc/rancher/k3s/k3s.yaml
          readOnly: true
      hostNetwork: true
      nodeSelector:
        node-role.kubernetes.io/control-plane: "true"
      serviceAccountName: kube-vip
      tolerations:
      - effect: NoSchedule
        key: node-role.kubernetes.io/control-plane
        operator: Exists
      - effect: NoExecute
        key: node-role.kubernetes.io/control-plane
        operator: Exists
      volumes:
      - name: kubeconfig
        hostPath:
          path: /etc/rancher/k3s/k3s.yaml
          type: FileOrCreate
`, cfg.Interface, cfg.VIP)

	return manifest, nil
}

// GenerateKubeVIPRBACManifest generates the RBAC manifest for kube-vip
func GenerateKubeVIPRBACManifest() string {
	return `apiVersion: v1
kind: ServiceAccount
metadata:
  name: kube-vip
  namespace: kube-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kube-vip-role
rules:
- apiGroups: [""]
  resources: ["services", "endpoints", "nodes", "pods"]
  verbs: ["list", "get", "watch", "update"]
- apiGroups: [""]
  resources: ["services/status"]
  verbs: ["update", "patch"]
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["list", "get", "watch", "update", "create"]
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["list", "get", "watch", "update", "create"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kube-vip-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kube-vip-role
subjects:
- kind: ServiceAccount
  name: kube-vip
  namespace: kube-system
`
}

// GenerateKubeVIPCloudProviderManifest generates the cloud provider manifest for kube-vip
// This enables LoadBalancer service support
func GenerateKubeVIPCloudProviderManifest() string {
	return `apiVersion: v1
kind: ServiceAccount
metadata:
  name: kube-vip-cloud-provider
  namespace: kube-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: kube-vip-cloud-provider-role
rules:
- apiGroups: [""]
  resources: ["services"]
  verbs: ["get", "list", "watch", "update"]
- apiGroups: [""]
  resources: ["services/status"]
  verbs: ["update", "patch"]
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["nodes"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch", "update"]
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "create", "update", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: kube-vip-cloud-provider-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: kube-vip-cloud-provider-role
subjects:
- kind: ServiceAccount
  name: kube-vip-cloud-provider
  namespace: kube-system
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: kube-vip-cloud-provider
  namespace: kube-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: kube-vip-cloud-provider
  template:
    metadata:
      labels:
        app: kube-vip-cloud-provider
    spec:
      containers:
      - name: kube-vip-cloud-provider
        image: ghcr.io/kube-vip/kube-vip-cloud-provider:v0.0.4
        imagePullPolicy: IfNotPresent
        command:
        - /kube-vip-cloud-provider
        - --leader-elect-resource-name=kube-vip-cloud-controller
      serviceAccountName: kube-vip-cloud-provider
`
}

// GenerateKubeVIPConfigMap generates the ConfigMap for kube-vip cloud provider
// This configures the IP range for LoadBalancer services
func GenerateKubeVIPConfigMap(cidr string) (string, error) {
	if cidr == "" {
		return "", fmt.Errorf("CIDR cannot be empty")
	}

	// Validate CIDR format
	_, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("invalid CIDR format: %s: %w", cidr, err)
	}

	manifest := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: kubevip
  namespace: kube-system
data:
  cidr-global: "%s"
  allow-share-global: "true"
`, cidr)

	return manifest, nil
}

// FormatManifests combines multiple manifests into a single YAML document
func FormatManifests(manifests ...string) string {
	var filtered []string
	for _, m := range manifests {
		m = strings.TrimSpace(m)
		if m != "" {
			filtered = append(filtered, m)
		}
	}
	return strings.Join(filtered, "\n---\n")
}
