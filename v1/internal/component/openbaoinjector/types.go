package openbaoinjector

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	corev1 "k8s.io/api/core/v1"
)

// HelmClient defines the Helm operations needed for the OpenBao injector
type HelmClient interface {
	AddRepo(ctx context.Context, opts helm.RepoAddOptions) error
	Install(ctx context.Context, opts helm.InstallOptions) error
	Upgrade(ctx context.Context, opts helm.UpgradeOptions) error
	Uninstall(ctx context.Context, opts helm.UninstallOptions) error
	List(ctx context.Context, namespace string) ([]helm.Release, error)
}

// K8sClient defines the Kubernetes operations needed for the OpenBao injector
type K8sClient interface {
	GetSecret(ctx context.Context, namespace, name string) (*corev1.Secret, error)
	CreateServiceAccount(ctx context.Context, name string, sa *corev1.ServiceAccount) error
	GetServiceAccountToken(ctx context.Context, namespace, name string) (string, error)
	ApplyClusterRoleBinding(ctx context.Context, manifest string) error
	GetClusterCACert(ctx context.Context) (string, error)
	GetKubernetesHost() string
}

// OpenBAOClient defines the OpenBAO operations needed for configuring Kubernetes auth
type OpenBAOClient interface {
	EnableAuth(ctx context.Context, authType string) error
	WriteAuthConfig(ctx context.Context, authPath string, data map[string]interface{}) error
	WriteRole(ctx context.Context, authPath, roleName string, data map[string]interface{}) error
}

// Component implements the component.Component interface for the OpenBao agent injector
type Component struct {
	helmClient    HelmClient
	k8sClient     K8sClient
	openbaoClient OpenBAOClient
}

// NewComponent creates a new OpenBao injector component instance
func NewComponent(helmClient HelmClient, k8sClient K8sClient, openbaoClient OpenBAOClient) *Component {
	return &Component{
		helmClient:    helmClient,
		k8sClient:     k8sClient,
		openbaoClient: openbaoClient,
	}
}

// Name returns the component name
func (c *Component) Name() string {
	return "openbao-injector"
}

// Dependencies returns the components this depends on
func (c *Component) Dependencies() []string {
	return []string{"openbao", "k3s"}
}

// Install installs the OpenBao agent injector via Helm
func (c *Component) Install(ctx context.Context, cfg component.ComponentConfig) error {
	config, err := ParseConfig(cfg)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	return Install(ctx, c.helmClient, c.k8sClient, c.openbaoClient, config, config.ConfigureK8sAuth)
}

// Upgrade upgrades the OpenBao agent injector
func (c *Component) Upgrade(ctx context.Context, cfg component.ComponentConfig) error {
	return fmt.Errorf("upgrade not yet implemented")
}

// Status returns the current status of the OpenBao agent injector
func (c *Component) Status(ctx context.Context) (*component.ComponentStatus, error) {
	if c.helmClient == nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   "helm client not initialized",
		}, nil
	}

	releases, err := c.helmClient.List(ctx, DefaultNamespace)
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to list releases: %v", err),
		}, nil
	}

	for _, rel := range releases {
		if rel.Name == ReleaseName {
			healthy := rel.Status == "deployed"
			msg := fmt.Sprintf("release status: %s", rel.Status)
			if healthy {
				msg = "injector webhook running"
			}
			return &component.ComponentStatus{
				Installed: true,
				Version:   rel.AppVersion,
				Healthy:   healthy,
				Message:   msg,
			}, nil
		}
	}

	return &component.ComponentStatus{
		Installed: false,
		Healthy:   false,
		Message:   "release not found",
	}, nil
}

// Uninstall removes the OpenBao agent injector
func (c *Component) Uninstall(ctx context.Context) error {
	return fmt.Errorf("uninstall not yet implemented")
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Version:           "0.26.2",
		Namespace:         DefaultNamespace,
		ExternalVaultAddr: "",
	}
}

// ParseConfig parses a ComponentConfig into an openbaoinjector Config
func ParseConfig(cfg component.ComponentConfig) (*Config, error) {
	config := DefaultConfig()

	if version, ok := cfg.GetString("version"); ok {
		config.Version = version
	}
	if namespace, ok := cfg.GetString("namespace"); ok {
		config.Namespace = namespace
	}
	if addr, ok := cfg.GetString("external_vault_addr"); ok {
		config.ExternalVaultAddr = addr
	}

	// Parse configure_k8s_auth
	if configureK8sAuth, ok := cfg.GetBool("configure_k8s_auth"); ok {
		config.ConfigureK8sAuth = configureK8sAuth
	}

	// Parse k8s_auth_roles
	if rolesVal, ok := cfg["k8s_auth_roles"]; ok {
		if roles, ok := rolesVal.([]K8sAuthRole); ok {
			config.K8sAuthRoles = roles
		} else if roleMaps, ok := rolesVal.([]interface{}); ok {
			for _, rm := range roleMaps {
				if roleMap, ok := rm.(map[string]interface{}); ok {
					role := K8sAuthRole{
						TTL: "1h", // Default TTL
					}
					if rn, ok := roleMap["role_name"].(string); ok {
						role.RoleName = rn
					}
					if san, ok := roleMap["service_account_name"].(string); ok {
						role.ServiceAccountName = san
					}
					if ns, ok := roleMap["service_account_namespace"].(string); ok {
						role.ServiceAccountNamespace = ns
					}
					if pol, ok := roleMap["policies"].(string); ok {
						role.Policies = pol
					}
					if ttl, ok := roleMap["ttl"].(string); ok {
						role.TTL = ttl
					}
					if role.RoleName != "" && role.ServiceAccountName != "" {
						config.K8sAuthRoles = append(config.K8sAuthRoles, role)
					}
				}
			}
		}
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return config, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.Version == "" {
		return fmt.Errorf("version is required")
	}
	if c.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	if c.ExternalVaultAddr == "" {
		return fmt.Errorf("external_vault_addr is required — set it to your OpenBao address (e.g. http://100.81.89.62:8200)")
	}
	return nil
}
