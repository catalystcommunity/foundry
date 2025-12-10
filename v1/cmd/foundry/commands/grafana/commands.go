package grafana

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/catalystcommunity/foundry/v1/internal/component/grafana"
	"github.com/catalystcommunity/foundry/v1/internal/component/openbao"
	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	"github.com/urfave/cli/v3"
)

// Kubernetes secret locations for Grafana credentials
var grafanaSecretLocations = []struct {
	namespace  string
	secretName string
	userKey    string
	passKey    string
}{
	{"monitoring", "grafana", "admin-user", "admin-password"},
	{"grafana", "grafana", "admin-user", "admin-password"},
}

// Command is the top-level grafana command
var Command = &cli.Command{
	Name:  "grafana",
	Usage: "Manage Grafana credentials and access",
	Description: `Commands for managing Grafana access and credentials.

Examples:
  foundry grafana password     # Get Grafana admin password
  foundry grafana credentials  # Get both username and password`,
	Commands: []*cli.Command{
		PasswordCommand,
		CredentialsCommand,
	},
}

// PasswordCommand retrieves the Grafana admin password
var PasswordCommand = &cli.Command{
	Name:  "password",
	Usage: "Get the Grafana admin password",
	Description: `Retrieves the Grafana admin password from OpenBAO.

The password is stored securely in OpenBAO and is used to login
to the Grafana web console.

Examples:
  foundry grafana password`,
	Action: runPassword,
}

// CredentialsCommand retrieves the full Grafana credentials
var CredentialsCommand = &cli.Command{
	Name:  "credentials",
	Usage: "Get Grafana admin username and password",
	Description: `Retrieves both the Grafana admin username and password from OpenBAO.

Examples:
  foundry grafana credentials`,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "json",
			Aliases: []string{"j"},
			Usage:   "Output in JSON format",
		},
	},
	Action: runCredentials,
}

func runPassword(ctx context.Context, cmd *cli.Command) error {
	_, password, err := getGrafanaCredentials(ctx)
	if err != nil {
		return fmt.Errorf("failed to get Grafana password: %w\n\nHint: Ensure Grafana is installed with 'foundry component install grafana'", err)
	}

	fmt.Println(password)
	return nil
}

func runCredentials(ctx context.Context, cmd *cli.Command) error {
	username, password, err := getGrafanaCredentials(ctx)
	if err != nil {
		return fmt.Errorf("failed to get Grafana credentials: %w\n\nHint: Ensure Grafana is installed with 'foundry component install grafana'", err)
	}

	if cmd.Bool("json") {
		output := map[string]string{
			"username": username,
			"password": password,
		}
		jsonBytes, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(jsonBytes))
	} else {
		fmt.Printf("Username: %s\n", username)
		fmt.Printf("Password: %s\n", password)
	}

	return nil
}

// getGrafanaCredentials retrieves Grafana credentials from OpenBAO or Kubernetes secrets
func getGrafanaCredentials(ctx context.Context) (username, password string, err error) {
	// First, try OpenBAO
	openBAOClient, err := getOpenBAOClient()
	if err == nil {
		username, password, err = grafana.GetGrafanaCredentials(ctx, openBAOClient)
		if err == nil {
			return username, password, nil
		}
	}

	// Fall back to Kubernetes secrets
	k8sClient, err := getK8sClient()
	if err != nil {
		return "", "", fmt.Errorf("failed to create k8s client: %w", err)
	}

	for _, loc := range grafanaSecretLocations {
		secret, err := k8sClient.GetSecret(ctx, loc.namespace, loc.secretName)
		if err != nil {
			continue
		}

		userBytes, hasUser := secret.Data[loc.userKey]
		passBytes, hasPass := secret.Data[loc.passKey]

		if hasPass {
			password = string(passBytes)
			if hasUser {
				username = string(userBytes)
			} else {
				username = "admin"
			}
			return username, password, nil
		}
	}

	return "", "", fmt.Errorf("Grafana credentials not found in OpenBAO or Kubernetes secrets")
}

// getOpenBAOClient creates an OpenBAO client using the stored credentials
func getOpenBAOClient() (*openbao.Client, error) {
	// Load stack configuration
	configPath := config.DefaultConfigPath()
	stackConfig, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load stack config: %w\n\nHint: Run 'foundry config init' to create a configuration", err)
	}

	// Get OpenBAO address
	addr, err := stackConfig.GetPrimaryOpenBAOAddress()
	if err != nil {
		return nil, fmt.Errorf("OpenBAO host not configured: %w", err)
	}
	openBAOAddr := fmt.Sprintf("http://%s:8200", addr)

	// Get config directory for OpenBAO keys
	configDir, err := config.GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}

	// Get OpenBAO token from keys file
	keysPath := filepath.Join(configDir, "openbao-keys", stackConfig.Cluster.Name, "keys.json")
	keysData, err := os.ReadFile(keysPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read OpenBAO keys from %s: %w\n\nHint: Ensure OpenBAO is initialized", keysPath, err)
	}

	var keys struct {
		RootToken string `json:"root_token"`
	}
	if err := json.Unmarshal(keysData, &keys); err != nil {
		return nil, fmt.Errorf("failed to parse OpenBAO keys: %w", err)
	}

	return openbao.NewClient(openBAOAddr, keys.RootToken), nil
}

// getK8sClient creates a Kubernetes client from kubeconfig
func getK8sClient() (*k8s.Client, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}

	kubeconfigPath := filepath.Join(configDir, "kubeconfig")
	kubeconfigBytes, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("kubeconfig not found at %s: %w\n\nHint: Run 'foundry cluster init' first", kubeconfigPath, err)
	}

	return k8s.NewClientFromKubeconfig(kubeconfigBytes)
}
