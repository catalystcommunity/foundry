package externaldns

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
)

// Provider represents the DNS provider for External-DNS
type Provider string

const (
	// ProviderPowerDNS uses PowerDNS as the DNS provider
	ProviderPowerDNS Provider = "pdns"

	// ProviderCloudflare uses Cloudflare as the DNS provider
	ProviderCloudflare Provider = "cloudflare"

	// ProviderRoute53 uses AWS Route53 as the DNS provider
	ProviderRoute53 Provider = "aws"

	// ProviderGoogle uses Google Cloud DNS as the DNS provider
	ProviderGoogle Provider = "google"

	// ProviderAzure uses Azure DNS as the DNS provider
	ProviderAzure Provider = "azure"

	// ProviderRFC2136 uses RFC2136 dynamic DNS updates
	ProviderRFC2136 Provider = "rfc2136"

	// ProviderNone indicates no provider is configured
	ProviderNone Provider = ""
)

// Config holds External-DNS component configuration
type Config struct {
	// Version is the Helm chart version to install
	Version string `json:"version" yaml:"version"`

	// Namespace for External-DNS deployment
	Namespace string `json:"namespace" yaml:"namespace"`

	// Provider is the DNS provider to use (pdns, cloudflare, aws, google, azure, rfc2136)
	// If empty, External-DNS is installed but no provider is configured
	Provider Provider `json:"provider" yaml:"provider"`

	// DomainFilters limits which domains External-DNS will manage
	DomainFilters []string `json:"domain_filters" yaml:"domain_filters"`

	// Sources specifies which Kubernetes resources to watch (ingress, service, etc.)
	Sources []string `json:"sources" yaml:"sources"`

	// Policy specifies how DNS records are synchronized (sync, upsert-only)
	Policy string `json:"policy" yaml:"policy"`

	// TxtOwnerId is a unique identifier for this External-DNS instance
	TxtOwnerId string `json:"txt_owner_id" yaml:"txt_owner_id"`

	// PowerDNS-specific configuration
	PowerDNS *PowerDNSConfig `json:"powerdns" yaml:"powerdns"`

	// Cloudflare-specific configuration
	Cloudflare *CloudflareConfig `json:"cloudflare" yaml:"cloudflare"`

	// RFC2136-specific configuration
	RFC2136 *RFC2136Config `json:"rfc2136" yaml:"rfc2136"`

	// Values allows passing additional Helm values
	Values map[string]interface{} `json:"values" yaml:",inline"`
}

// PowerDNSConfig holds PowerDNS provider configuration
type PowerDNSConfig struct {
	// APIUrl is the PowerDNS API URL (e.g., http://powerdns.dns.svc.cluster.local:8081)
	APIUrl string `json:"api_url" yaml:"api_url"`

	// APIKey is the PowerDNS API key
	APIKey string `json:"api_key" yaml:"api_key"`

	// ServerID is the PowerDNS server ID (default: localhost)
	ServerID string `json:"server_id" yaml:"server_id"`
}

// CloudflareConfig holds Cloudflare provider configuration
type CloudflareConfig struct {
	// APIToken is the Cloudflare API token
	APIToken string `json:"api_token" yaml:"api_token"`

	// Proxied sets whether records should be proxied through Cloudflare
	Proxied bool `json:"proxied" yaml:"proxied"`
}

// RFC2136Config holds RFC2136 provider configuration
type RFC2136Config struct {
	// Host is the DNS server host
	Host string `json:"host" yaml:"host"`

	// Port is the DNS server port (default: 53)
	Port int `json:"port" yaml:"port"`

	// Zone is the DNS zone to update
	Zone string `json:"zone" yaml:"zone"`

	// TSIGKeyName is the TSIG key name for authentication
	TSIGKeyName string `json:"tsig_key_name" yaml:"tsig_key_name"`

	// TSIGSecret is the TSIG secret
	TSIGSecret string `json:"tsig_secret" yaml:"tsig_secret"`

	// TSIGSecretAlg is the TSIG algorithm (default: hmac-sha256)
	TSIGSecretAlg string `json:"tsig_secret_alg" yaml:"tsig_secret_alg"`
}

// HelmClient defines the Helm operations needed for External-DNS component
type HelmClient interface {
	AddRepo(ctx context.Context, opts helm.RepoAddOptions) error
	Install(ctx context.Context, opts helm.InstallOptions) error
	Upgrade(ctx context.Context, opts helm.UpgradeOptions) error
	Uninstall(ctx context.Context, opts helm.UninstallOptions) error
	List(ctx context.Context, namespace string) ([]helm.Release, error)
}

// K8sClient defines the Kubernetes operations needed for External-DNS component
type K8sClient interface {
	GetPods(ctx context.Context, namespace string) ([]*k8s.Pod, error)
}

// Component implements the component.Component interface for External-DNS
type Component struct {
	helmClient HelmClient
	k8sClient  K8sClient
}

// NewComponent creates a new External-DNS component instance
func NewComponent(helmClient HelmClient, k8sClient K8sClient) *Component {
	return &Component{
		helmClient: helmClient,
		k8sClient:  k8sClient,
	}
}

// Name returns the component name
func (c *Component) Name() string {
	return "external-dns"
}

// Install installs External-DNS
func (c *Component) Install(ctx context.Context, cfg component.ComponentConfig) error {
	config, err := ParseConfig(cfg)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	return Install(ctx, c.helmClient, c.k8sClient, config)
}

// Upgrade upgrades External-DNS
func (c *Component) Upgrade(ctx context.Context, cfg component.ComponentConfig) error {
	return fmt.Errorf("upgrade not yet implemented")
}

// Status returns the current status of External-DNS
func (c *Component) Status(ctx context.Context) (*component.ComponentStatus, error) {
	if c.helmClient == nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   "helm client not initialized",
		}, nil
	}

	// Check for external-dns release
	releases, err := c.helmClient.List(ctx, "external-dns")
	if err != nil {
		return &component.ComponentStatus{
			Installed: false,
			Healthy:   false,
			Message:   fmt.Sprintf("failed to list releases: %v", err),
		}, nil
	}

	// Look for external-dns release
	for _, rel := range releases {
		if rel.Name == "external-dns" {
			healthy := rel.Status == "deployed"
			return &component.ComponentStatus{
				Installed: true,
				Version:   rel.AppVersion,
				Healthy:   healthy,
				Message:   fmt.Sprintf("release status: %s", rel.Status),
			}, nil
		}
	}

	return &component.ComponentStatus{
		Installed: false,
		Healthy:   false,
		Message:   "external-dns release not found",
	}, nil
}

// Uninstall removes External-DNS
func (c *Component) Uninstall(ctx context.Context) error {
	return fmt.Errorf("uninstall not yet implemented")
}

// Dependencies returns the list of components that External-DNS depends on
func (c *Component) Dependencies() []string {
	return []string{} // No strict dependencies, but typically used with ingress
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Version:       "1.15.0", // external-dns Helm chart version
		Namespace:     "external-dns",
		Provider:      ProviderNone, // No provider configured by default
		DomainFilters: []string{},
		Sources:       []string{"ingress", "service"},
		Policy:        "upsert-only", // Safe default - won't delete records
		TxtOwnerId:    "foundry",
		Values:        make(map[string]interface{}),
	}
}

// ParseConfig parses a ComponentConfig into an External-DNS Config
func ParseConfig(cfg component.ComponentConfig) (*Config, error) {
	config := DefaultConfig()

	if version, ok := cfg.GetString("version"); ok {
		config.Version = version
	}

	if namespace, ok := cfg.GetString("namespace"); ok {
		config.Namespace = namespace
	}

	if provider, ok := cfg.GetString("provider"); ok {
		config.Provider = Provider(provider)
	}

	if domainFilters, ok := cfg.GetStringSlice("domain_filters"); ok {
		config.DomainFilters = domainFilters
	}

	if sources, ok := cfg.GetStringSlice("sources"); ok {
		config.Sources = sources
	}

	if policy, ok := cfg.GetString("policy"); ok {
		config.Policy = policy
	}

	if txtOwnerId, ok := cfg.GetString("txt_owner_id"); ok {
		config.TxtOwnerId = txtOwnerId
	}

	// Parse PowerDNS config
	if pdnsMap, ok := cfg.GetMap("powerdns"); ok {
		config.PowerDNS = &PowerDNSConfig{}
		if apiUrl, ok := pdnsMap["api_url"].(string); ok {
			config.PowerDNS.APIUrl = apiUrl
		}
		if apiKey, ok := pdnsMap["api_key"].(string); ok {
			config.PowerDNS.APIKey = apiKey
		}
		if serverID, ok := pdnsMap["server_id"].(string); ok {
			config.PowerDNS.ServerID = serverID
		}
		// If PowerDNS config is provided, set provider to pdns
		if config.Provider == ProviderNone && config.PowerDNS.APIUrl != "" {
			config.Provider = ProviderPowerDNS
		}
	}

	// Parse Cloudflare config
	if cfMap, ok := cfg.GetMap("cloudflare"); ok {
		config.Cloudflare = &CloudflareConfig{}
		if apiToken, ok := cfMap["api_token"].(string); ok {
			config.Cloudflare.APIToken = apiToken
		}
		if proxied, ok := cfMap["proxied"].(bool); ok {
			config.Cloudflare.Proxied = proxied
		}
		// If Cloudflare config is provided, set provider to cloudflare
		if config.Provider == ProviderNone && config.Cloudflare.APIToken != "" {
			config.Provider = ProviderCloudflare
		}
	}

	// Parse RFC2136 config
	if rfc2136Map, ok := cfg.GetMap("rfc2136"); ok {
		config.RFC2136 = &RFC2136Config{
			Port:          53,
			TSIGSecretAlg: "hmac-sha256",
		}
		if host, ok := rfc2136Map["host"].(string); ok {
			config.RFC2136.Host = host
		}
		if port, ok := rfc2136Map["port"].(float64); ok {
			config.RFC2136.Port = int(port)
		}
		if zone, ok := rfc2136Map["zone"].(string); ok {
			config.RFC2136.Zone = zone
		}
		if tsigKeyName, ok := rfc2136Map["tsig_key_name"].(string); ok {
			config.RFC2136.TSIGKeyName = tsigKeyName
		}
		if tsigSecret, ok := rfc2136Map["tsig_secret"].(string); ok {
			config.RFC2136.TSIGSecret = tsigSecret
		}
		if tsigSecretAlg, ok := rfc2136Map["tsig_secret_alg"].(string); ok {
			config.RFC2136.TSIGSecretAlg = tsigSecretAlg
		}
		// If RFC2136 config is provided, set provider to rfc2136
		if config.Provider == ProviderNone && config.RFC2136.Host != "" {
			config.Provider = ProviderRFC2136
		}
	}

	if values, ok := cfg.GetMap("values"); ok {
		config.Values = values
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// Validate validates the External-DNS configuration
func (c *Config) Validate() error {
	// Validate provider-specific configuration
	switch c.Provider {
	case ProviderPowerDNS:
		if c.PowerDNS == nil || c.PowerDNS.APIUrl == "" {
			return fmt.Errorf("powerdns.api_url is required when using PowerDNS provider")
		}
		if c.PowerDNS.APIKey == "" {
			return fmt.Errorf("powerdns.api_key is required when using PowerDNS provider")
		}
	case ProviderCloudflare:
		if c.Cloudflare == nil || c.Cloudflare.APIToken == "" {
			return fmt.Errorf("cloudflare.api_token is required when using Cloudflare provider")
		}
	case ProviderRFC2136:
		if c.RFC2136 == nil || c.RFC2136.Host == "" {
			return fmt.Errorf("rfc2136.host is required when using RFC2136 provider")
		}
	case ProviderNone:
		// No provider configured - this is valid, user can configure later
	case ProviderRoute53, ProviderGoogle, ProviderAzure:
		// These providers require additional setup not covered here
		// Allow them to pass through for custom configuration via Values
	default:
		return fmt.Errorf("unsupported provider: %s", c.Provider)
	}

	// Validate policy
	switch c.Policy {
	case "sync", "upsert-only":
		// Valid policies
	default:
		return fmt.Errorf("invalid policy: %s (must be 'sync' or 'upsert-only')", c.Policy)
	}

	return nil
}

// IsProviderConfigured returns true if a DNS provider is configured
func (c *Config) IsProviderConfigured() bool {
	return c.Provider != ProviderNone
}
