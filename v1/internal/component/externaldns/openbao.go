package externaldns

import (
	"context"
	"fmt"
)

const (
	// OpenBAO path constants for External-DNS secrets
	openBAOMount      = "foundry-core"
	openBAOBasePath   = "external-dns"
)

// OpenBAOClient defines the interface for storing/retrieving secrets in OpenBAO
type OpenBAOClient interface {
	WriteSecretV2(ctx context.Context, mount, path string, data map[string]interface{}) error
	ReadSecretV2(ctx context.Context, mount, path string) (map[string]interface{}, error)
}

// PowerDNSSecrets contains PowerDNS provider credentials
type PowerDNSSecrets struct {
	APIUrl   string `json:"api_url"`
	APIKey   string `json:"api_key"`
	ServerID string `json:"server_id,omitempty"`
}

// CloudflareSecrets contains Cloudflare provider credentials
type CloudflareSecrets struct {
	APIToken string `json:"api_token"`
}

// RFC2136Secrets contains RFC2136 provider credentials
type RFC2136Secrets struct {
	Host          string `json:"host"`
	Port          int    `json:"port,omitempty"`
	Zone          string `json:"zone,omitempty"`
	TSIGKeyName   string `json:"tsig_key_name,omitempty"`
	TSIGSecret    string `json:"tsig_secret,omitempty"`
	TSIGSecretAlg string `json:"tsig_secret_alg,omitempty"`
}

// getProviderPath returns the OpenBAO path for a specific provider
func getProviderPath(provider Provider) string {
	return fmt.Sprintf("%s/%s", openBAOBasePath, string(provider))
}

// StorePowerDNSCredentials stores PowerDNS credentials in OpenBAO
func StorePowerDNSCredentials(ctx context.Context, client OpenBAOClient, secrets *PowerDNSSecrets) error {
	if client == nil {
		return fmt.Errorf("OpenBAO client is required")
	}
	if secrets == nil {
		return fmt.Errorf("secrets cannot be nil")
	}
	if secrets.APIUrl == "" {
		return fmt.Errorf("api_url is required")
	}
	if secrets.APIKey == "" {
		return fmt.Errorf("api_key is required")
	}

	// Check if credentials already exist and match
	existing, err := GetPowerDNSCredentials(ctx, client)
	if err == nil && existing != nil {
		if existing.APIUrl == secrets.APIUrl &&
			existing.APIKey == secrets.APIKey &&
			existing.ServerID == secrets.ServerID {
			return nil // Already exists and matches
		}
	}

	secretData := map[string]interface{}{
		"api_url": secrets.APIUrl,
		"api_key": secrets.APIKey,
	}
	if secrets.ServerID != "" {
		secretData["server_id"] = secrets.ServerID
	}

	return client.WriteSecretV2(ctx, openBAOMount, getProviderPath(ProviderPowerDNS), secretData)
}

// GetPowerDNSCredentials retrieves PowerDNS credentials from OpenBAO
func GetPowerDNSCredentials(ctx context.Context, client OpenBAOClient) (*PowerDNSSecrets, error) {
	if client == nil {
		return nil, fmt.Errorf("OpenBAO client is required")
	}

	data, err := client.ReadSecretV2(ctx, openBAOMount, getProviderPath(ProviderPowerDNS))
	if err != nil {
		return nil, fmt.Errorf("failed to read PowerDNS credentials from OpenBAO: %w", err)
	}

	if data == nil {
		return nil, fmt.Errorf("PowerDNS credentials not found in OpenBAO")
	}

	apiUrl, ok := data["api_url"].(string)
	if !ok || apiUrl == "" {
		return nil, fmt.Errorf("PowerDNS api_url not found or empty in OpenBAO")
	}

	apiKey, ok := data["api_key"].(string)
	if !ok || apiKey == "" {
		return nil, fmt.Errorf("PowerDNS api_key not found or empty in OpenBAO")
	}

	secrets := &PowerDNSSecrets{
		APIUrl: apiUrl,
		APIKey: apiKey,
	}

	if serverID, ok := data["server_id"].(string); ok {
		secrets.ServerID = serverID
	}

	return secrets, nil
}

// StoreCloudflareCredentials stores Cloudflare credentials in OpenBAO
func StoreCloudflareCredentials(ctx context.Context, client OpenBAOClient, secrets *CloudflareSecrets) error {
	if client == nil {
		return fmt.Errorf("OpenBAO client is required")
	}
	if secrets == nil {
		return fmt.Errorf("secrets cannot be nil")
	}
	if secrets.APIToken == "" {
		return fmt.Errorf("api_token is required")
	}

	// Check if credentials already exist and match
	existing, err := GetCloudflareCredentials(ctx, client)
	if err == nil && existing != nil {
		if existing.APIToken == secrets.APIToken {
			return nil // Already exists and matches
		}
	}

	secretData := map[string]interface{}{
		"api_token": secrets.APIToken,
	}

	return client.WriteSecretV2(ctx, openBAOMount, getProviderPath(ProviderCloudflare), secretData)
}

// GetCloudflareCredentials retrieves Cloudflare credentials from OpenBAO
func GetCloudflareCredentials(ctx context.Context, client OpenBAOClient) (*CloudflareSecrets, error) {
	if client == nil {
		return nil, fmt.Errorf("OpenBAO client is required")
	}

	data, err := client.ReadSecretV2(ctx, openBAOMount, getProviderPath(ProviderCloudflare))
	if err != nil {
		return nil, fmt.Errorf("failed to read Cloudflare credentials from OpenBAO: %w", err)
	}

	if data == nil {
		return nil, fmt.Errorf("Cloudflare credentials not found in OpenBAO")
	}

	apiToken, ok := data["api_token"].(string)
	if !ok || apiToken == "" {
		return nil, fmt.Errorf("Cloudflare api_token not found or empty in OpenBAO")
	}

	return &CloudflareSecrets{
		APIToken: apiToken,
	}, nil
}

// StoreRFC2136Credentials stores RFC2136 credentials in OpenBAO
func StoreRFC2136Credentials(ctx context.Context, client OpenBAOClient, secrets *RFC2136Secrets) error {
	if client == nil {
		return fmt.Errorf("OpenBAO client is required")
	}
	if secrets == nil {
		return fmt.Errorf("secrets cannot be nil")
	}
	if secrets.Host == "" {
		return fmt.Errorf("host is required")
	}

	// Check if credentials already exist and match
	existing, err := GetRFC2136Credentials(ctx, client)
	if err == nil && existing != nil {
		if existing.Host == secrets.Host &&
			existing.Port == secrets.Port &&
			existing.Zone == secrets.Zone &&
			existing.TSIGKeyName == secrets.TSIGKeyName &&
			existing.TSIGSecret == secrets.TSIGSecret &&
			existing.TSIGSecretAlg == secrets.TSIGSecretAlg {
			return nil // Already exists and matches
		}
	}

	secretData := map[string]interface{}{
		"host": secrets.Host,
	}
	if secrets.Port > 0 {
		secretData["port"] = secrets.Port
	}
	if secrets.Zone != "" {
		secretData["zone"] = secrets.Zone
	}
	if secrets.TSIGKeyName != "" {
		secretData["tsig_key_name"] = secrets.TSIGKeyName
	}
	if secrets.TSIGSecret != "" {
		secretData["tsig_secret"] = secrets.TSIGSecret
	}
	if secrets.TSIGSecretAlg != "" {
		secretData["tsig_secret_alg"] = secrets.TSIGSecretAlg
	}

	return client.WriteSecretV2(ctx, openBAOMount, getProviderPath(ProviderRFC2136), secretData)
}

// GetRFC2136Credentials retrieves RFC2136 credentials from OpenBAO
func GetRFC2136Credentials(ctx context.Context, client OpenBAOClient) (*RFC2136Secrets, error) {
	if client == nil {
		return nil, fmt.Errorf("OpenBAO client is required")
	}

	data, err := client.ReadSecretV2(ctx, openBAOMount, getProviderPath(ProviderRFC2136))
	if err != nil {
		return nil, fmt.Errorf("failed to read RFC2136 credentials from OpenBAO: %w", err)
	}

	if data == nil {
		return nil, fmt.Errorf("RFC2136 credentials not found in OpenBAO")
	}

	host, ok := data["host"].(string)
	if !ok || host == "" {
		return nil, fmt.Errorf("RFC2136 host not found or empty in OpenBAO")
	}

	secrets := &RFC2136Secrets{
		Host: host,
	}

	if port, ok := data["port"].(float64); ok {
		secrets.Port = int(port)
	}
	if zone, ok := data["zone"].(string); ok {
		secrets.Zone = zone
	}
	if tsigKeyName, ok := data["tsig_key_name"].(string); ok {
		secrets.TSIGKeyName = tsigKeyName
	}
	if tsigSecret, ok := data["tsig_secret"].(string); ok {
		secrets.TSIGSecret = tsigSecret
	}
	if tsigSecretAlg, ok := data["tsig_secret_alg"].(string); ok {
		secrets.TSIGSecretAlg = tsigSecretAlg
	}

	return secrets, nil
}

// StoreProviderCredentials stores credentials for the configured provider
func StoreProviderCredentials(ctx context.Context, client OpenBAOClient, cfg *Config) error {
	if client == nil {
		return fmt.Errorf("OpenBAO client is required")
	}
	if cfg == nil {
		return fmt.Errorf("config is required")
	}

	switch cfg.Provider {
	case ProviderPowerDNS:
		if cfg.PowerDNS == nil {
			return fmt.Errorf("PowerDNS config is required for PowerDNS provider")
		}
		return StorePowerDNSCredentials(ctx, client, &PowerDNSSecrets{
			APIUrl:   cfg.PowerDNS.APIUrl,
			APIKey:   cfg.PowerDNS.APIKey,
			ServerID: cfg.PowerDNS.ServerID,
		})
	case ProviderCloudflare:
		if cfg.Cloudflare == nil {
			return fmt.Errorf("Cloudflare config is required for Cloudflare provider")
		}
		return StoreCloudflareCredentials(ctx, client, &CloudflareSecrets{
			APIToken: cfg.Cloudflare.APIToken,
		})
	case ProviderRFC2136:
		if cfg.RFC2136 == nil {
			return fmt.Errorf("RFC2136 config is required for RFC2136 provider")
		}
		return StoreRFC2136Credentials(ctx, client, &RFC2136Secrets{
			Host:          cfg.RFC2136.Host,
			Port:          cfg.RFC2136.Port,
			Zone:          cfg.RFC2136.Zone,
			TSIGKeyName:   cfg.RFC2136.TSIGKeyName,
			TSIGSecret:    cfg.RFC2136.TSIGSecret,
			TSIGSecretAlg: cfg.RFC2136.TSIGSecretAlg,
		})
	case ProviderNone:
		return nil // No credentials to store
	default:
		// For other providers (AWS, Google, Azure), we don't manage credentials
		// Users configure these via other means (IAM roles, workload identity, etc.)
		return nil
	}
}

// LoadProviderCredentials loads credentials from OpenBAO into the config
// This allows retrieving previously stored credentials
func LoadProviderCredentials(ctx context.Context, client OpenBAOClient, cfg *Config) error {
	if client == nil {
		return fmt.Errorf("OpenBAO client is required")
	}
	if cfg == nil {
		return fmt.Errorf("config is required")
	}

	switch cfg.Provider {
	case ProviderPowerDNS:
		secrets, err := GetPowerDNSCredentials(ctx, client)
		if err != nil {
			return err
		}
		if cfg.PowerDNS == nil {
			cfg.PowerDNS = &PowerDNSConfig{}
		}
		cfg.PowerDNS.APIUrl = secrets.APIUrl
		cfg.PowerDNS.APIKey = secrets.APIKey
		if secrets.ServerID != "" {
			cfg.PowerDNS.ServerID = secrets.ServerID
		}
		return nil

	case ProviderCloudflare:
		secrets, err := GetCloudflareCredentials(ctx, client)
		if err != nil {
			return err
		}
		if cfg.Cloudflare == nil {
			cfg.Cloudflare = &CloudflareConfig{}
		}
		cfg.Cloudflare.APIToken = secrets.APIToken
		return nil

	case ProviderRFC2136:
		secrets, err := GetRFC2136Credentials(ctx, client)
		if err != nil {
			return err
		}
		if cfg.RFC2136 == nil {
			cfg.RFC2136 = &RFC2136Config{}
		}
		cfg.RFC2136.Host = secrets.Host
		if secrets.Port > 0 {
			cfg.RFC2136.Port = secrets.Port
		}
		cfg.RFC2136.Zone = secrets.Zone
		cfg.RFC2136.TSIGKeyName = secrets.TSIGKeyName
		cfg.RFC2136.TSIGSecret = secrets.TSIGSecret
		cfg.RFC2136.TSIGSecretAlg = secrets.TSIGSecretAlg
		return nil

	case ProviderNone:
		return nil // No credentials to load

	default:
		// For other providers, no credentials to load from OpenBAO
		return nil
	}
}
