package truenas

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

// InstallConfig contains configuration for TrueNAS CSI installation
type InstallConfig struct {
	// APIURL is the TrueNAS API URL (e.g., https://truenas.example.com)
	APIURL string
	// APIKey is the TrueNAS API key
	APIKey string
	// SetupConfig contains TrueNAS setup options
	SetupConfig *SetupConfig
	// Interactive enables prompting for missing config
	Interactive bool
	// SkipSetup skips the TrueNAS setup phase (assumes already configured)
	SkipSetup bool
}

// InstallResult contains the result of TrueNAS installation prep
type InstallResult struct {
	// CSIConfig contains the configuration for democratic-csi
	CSIConfig *CSIConfig
	// SetupResult contains the TrueNAS setup result (if setup was run)
	SetupResult *SetupResult
	// APIKeyStored indicates if the API key was stored in OpenBAO
	APIKeyStored bool
}

// OpenBAOClient defines the interface for storing secrets in OpenBAO
type OpenBAOClient interface {
	WriteSecretV2(ctx context.Context, mount, path string, data map[string]interface{}) error
	ReadSecretV2(ctx context.Context, mount, path string) (map[string]interface{}, error)
}

// PrepareInstall prepares TrueNAS for CSI installation
// It validates config, runs setup if needed, and stores API key in OpenBAO
func PrepareInstall(ctx context.Context, cfg *InstallConfig, openBAOClient OpenBAOClient) (*InstallResult, error) {
	if cfg == nil {
		return nil, fmt.Errorf("install config is required")
	}

	result := &InstallResult{}

	// Step 1: Validate or prompt for required config
	if err := validateOrPromptConfig(cfg); err != nil {
		return nil, fmt.Errorf("configuration error: %w", err)
	}

	// Step 2: Create TrueNAS client and test connection
	client, err := NewClient(cfg.APIURL, cfg.APIKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create TrueNAS client: %w", err)
	}

	fmt.Println("  Testing connection to TrueNAS...")
	if err := client.Ping(); err != nil {
		return nil, fmt.Errorf("failed to connect to TrueNAS: %w", err)
	}
	fmt.Println("  ✓ Connection successful")

	// Step 3: Run TrueNAS setup if not skipped
	var setupResult *SetupResult
	if !cfg.SkipSetup {
		fmt.Println("  Setting up TrueNAS for CSI...")
		validator := NewValidator(client)

		setupCfg := cfg.SetupConfig
		if setupCfg == nil {
			setupCfg = DefaultSetupConfig()
		}

		setupResult, err = validator.Setup(setupCfg)
		if err != nil {
			return nil, fmt.Errorf("TrueNAS setup failed: %w", err)
		}
		result.SetupResult = setupResult

		// Print any warnings
		if len(setupResult.Warnings) > 0 {
			fmt.Println("  Warnings:")
			for _, w := range setupResult.Warnings {
				fmt.Printf("    ⚠ %s\n", w)
			}
		}
		fmt.Println("  ✓ TrueNAS setup complete")
	} else {
		// If skipping setup, validate requirements
		fmt.Println("  Validating TrueNAS requirements...")
		validator := NewValidator(client)
		if err := validator.ValidateRequirements(cfg.SetupConfig); err != nil {
			return nil, fmt.Errorf("TrueNAS requirements not met: %w", err)
		}
		fmt.Println("  ✓ TrueNAS requirements validated")
	}

	// Step 4: Store API key in OpenBAO if client is provided
	if openBAOClient != nil {
		fmt.Println("  Storing TrueNAS API key in OpenBAO...")
		if err := storeTrueNASAPIKey(ctx, openBAOClient, cfg.APIKey); err != nil {
			// Non-fatal - warn but continue
			fmt.Printf("  ⚠ Failed to store API key in OpenBAO: %v\n", err)
		} else {
			result.APIKeyStored = true
			fmt.Println("  ✓ API key stored in OpenBAO")
		}
	}

	// Step 5: Generate CSI config
	validator := NewValidator(client)
	if setupResult != nil {
		result.CSIConfig = validator.GetCSIConfig(setupResult, cfg.SetupConfig, cfg.APIURL)
	} else {
		// If setup was skipped, create a minimal CSI config
		result.CSIConfig = &CSIConfig{
			HTTPURL:       cfg.APIURL,
			APIKey:        cfg.APIKey,
			NFSShareHost:  extractHostFromURL(cfg.APIURL),
			PoolName:      cfg.SetupConfig.PoolName,
			DatasetParent: fmt.Sprintf("%s/%s", cfg.SetupConfig.PoolName, cfg.SetupConfig.DatasetName),
		}
	}

	return result, nil
}

// validateOrPromptConfig validates the install config and prompts for missing values
func validateOrPromptConfig(cfg *InstallConfig) error {
	reader := bufio.NewReader(os.Stdin)

	// Validate/prompt for API URL
	if cfg.APIURL == "" {
		if !cfg.Interactive {
			return fmt.Errorf("TrueNAS API URL is required (set truenas.api_url in config)")
		}

		fmt.Print("  TrueNAS API URL (e.g., https://truenas.example.com): ")
		url, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		cfg.APIURL = strings.TrimSpace(url)

		if cfg.APIURL == "" {
			return fmt.Errorf("TrueNAS API URL is required")
		}
	}

	// Validate/prompt for API Key
	if cfg.APIKey == "" {
		if !cfg.Interactive {
			return fmt.Errorf("TrueNAS API key is required (set truenas.api_key in config)")
		}

		fmt.Println("  TrueNAS API key can be generated in TrueNAS: System → API Keys → Add")
		fmt.Print("  TrueNAS API Key: ")
		key, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		cfg.APIKey = strings.TrimSpace(key)

		if cfg.APIKey == "" {
			return fmt.Errorf("TrueNAS API key is required")
		}
	}

	// Set default setup config if not provided
	if cfg.SetupConfig == nil {
		cfg.SetupConfig = DefaultSetupConfig()
	}

	return nil
}

// storeTrueNASAPIKey stores the TrueNAS API key in OpenBAO
func storeTrueNASAPIKey(ctx context.Context, client OpenBAOClient, apiKey string) error {
	// Check if key already exists
	existing, err := client.ReadSecretV2(ctx, "foundry-core", "truenas")
	if err == nil && existing != nil {
		if existingKey, ok := existing["api_key"].(string); ok && existingKey == apiKey {
			// Key already exists and matches
			return nil
		}
	}

	// Store the API key
	secretData := map[string]interface{}{
		"api_key": apiKey,
	}

	return client.WriteSecretV2(ctx, "foundry-core", "truenas", secretData)
}

// EnsureTrueNASAPIKey retrieves or stores the TrueNAS API key from/to OpenBAO
// Returns the API key (from OpenBAO if available, or stores and returns the provided key)
func EnsureTrueNASAPIKey(ctx context.Context, client OpenBAOClient, apiKey string) (string, error) {
	// Try to read existing key
	existing, err := client.ReadSecretV2(ctx, "foundry-core", "truenas")
	if err == nil && existing != nil {
		if existingKey, ok := existing["api_key"].(string); ok && existingKey != "" {
			return existingKey, nil
		}
	}

	// If no existing key and no key provided, error
	if apiKey == "" {
		return "", fmt.Errorf("no TrueNAS API key found in OpenBAO and none provided")
	}

	// Store the provided key
	if err := storeTrueNASAPIKey(ctx, client, apiKey); err != nil {
		return "", fmt.Errorf("failed to store API key: %w", err)
	}

	return apiKey, nil
}

// GetTrueNASAPIKey retrieves the TrueNAS API key from OpenBAO
func GetTrueNASAPIKey(ctx context.Context, client OpenBAOClient) (string, error) {
	data, err := client.ReadSecretV2(ctx, "foundry-core", "truenas")
	if err != nil {
		return "", fmt.Errorf("failed to read TrueNAS API key from OpenBAO: %w", err)
	}

	if data == nil {
		return "", fmt.Errorf("TrueNAS API key not found in OpenBAO")
	}

	apiKey, ok := data["api_key"].(string)
	if !ok || apiKey == "" {
		return "", fmt.Errorf("TrueNAS API key is empty or invalid")
	}

	return apiKey, nil
}
