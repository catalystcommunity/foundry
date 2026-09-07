package grafana

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/component/grafana"
	"github.com/catalystcommunity/foundry/v1/internal/component/openbao"
	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/desktop"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"
)

const temporaryDisplayDuration = 5 * time.Second

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
  foundry grafana open                # Open Grafana in the default browser
  foundry grafana credentials         # Copy the admin password
  foundry grafana credentials --show  # Show credentials for five seconds`,
	Commands: []*cli.Command{
		OpenCommand,
		PasswordCommand,
		CredentialsCommand,
	},
}

// OpenCommand opens Grafana in the default browser.
var OpenCommand = &cli.Command{
	Name:   "open",
	Usage:  "Open Grafana in the default browser",
	Action: runOpen,
}

// PasswordCommand retrieves the Grafana admin password.
var PasswordCommand = &cli.Command{
	Name:  "password",
	Usage: "Copy the Grafana admin password",
	Description: `Retrieves the Grafana admin password from OpenBAO or Kubernetes.

The command copies the password to the desktop clipboard. It does not print the
password. Use --show to show the password in a temporary terminal view for five
seconds.

Examples:
  foundry grafana password
  foundry grafana password --show`,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "show",
			Usage: "Show the password for five seconds instead of copying it",
		},
	},
	Action: runPassword,
}

// CredentialsCommand retrieves the Grafana admin credentials.
var CredentialsCommand = &cli.Command{
	Name:  "credentials",
	Usage: "Get the Grafana admin credentials",
	Description: `Retrieves the Grafana admin username and password from OpenBAO or Kubernetes.

The command prints the username and copies the password to the desktop
clipboard. It does not print the password. Use --show to show both values in a
temporary terminal view for five seconds.

Examples:
  foundry grafana credentials
  foundry grafana credentials --show`,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "show",
			Usage: "Show the credentials for five seconds instead of copying the password",
		},
	},
	Action: runCredentials,
}

type credentialActions struct {
	load    func(context.Context, string) (string, string, error)
	copy    func(context.Context, string) error
	display func(context.Context, io.Writer, string, string, bool, time.Duration) error
}

type openActions struct {
	findConfig func(string) (string, error)
	loadConfig func(string) (*config.Config, error)
	openURL    func(context.Context, string) error
}

var defaultCredentialActions = credentialActions{
	load:    getGrafanaCredentials,
	copy:    desktop.CopyToClipboard,
	display: displayCredentials,
}

var defaultOpenActions = openActions{
	findConfig: config.FindConfig,
	loadConfig: config.Load,
	openURL:    desktop.OpenURL,
}

func runOpen(ctx context.Context, cmd *cli.Command) error {
	return handleOpen(ctx, cmd.Writer, cmd.String("config"), defaultOpenActions)
}

func handleOpen(ctx context.Context, output io.Writer, configName string, actions openActions) error {
	configPath, err := actions.findConfig(configName)
	if err != nil {
		return fmt.Errorf("find stack config: %w", err)
	}
	stackConfig, err := actions.loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load stack config: %w", err)
	}
	grafanaURL, err := urlForGrafana(stackConfig)
	if err != nil {
		return err
	}

	if err := actions.openURL(ctx, grafanaURL); err != nil {
		return fmt.Errorf("open Grafana: %w\nOpen %s in a browser", err, grafanaURL)
	}
	fmt.Fprintf(output, "Opened Grafana at %s\n", grafanaURL)
	return nil
}

func runPassword(ctx context.Context, cmd *cli.Command) error {
	return handleCredentials(ctx, cmd.Writer, cmd.String("config"), cmd.Bool("show"), false, defaultCredentialActions)
}

func runCredentials(ctx context.Context, cmd *cli.Command) error {
	return handleCredentials(ctx, cmd.Writer, cmd.String("config"), cmd.Bool("show"), true, defaultCredentialActions)
}

func handleCredentials(ctx context.Context, output io.Writer, configName string, show, includeUsername bool, actions credentialActions) error {
	username, password, err := actions.load(ctx, configName)
	if err != nil {
		return fmt.Errorf("get Grafana credentials: %w\n\nHint: Ensure Grafana is installed with 'foundry component install grafana'", err)
	}

	if show {
		if err := actions.display(ctx, output, username, password, includeUsername, temporaryDisplayDuration); err != nil {
			return fmt.Errorf("show Grafana credentials: %w", err)
		}
		return nil
	}

	if err := actions.copy(ctx, password); err != nil {
		return fmt.Errorf("copy Grafana password: %w\nUse --show from an interactive terminal instead", err)
	}
	if includeUsername {
		fmt.Fprintf(output, "Username: %s\n", username)
	}
	fmt.Fprintln(output, "Grafana admin password copied to the clipboard.")

	return nil
}

func urlForGrafana(stackConfig *config.Config) (string, error) {
	componentConfig, ok := stackConfig.Components["grafana"]
	if !ok {
		return "", fmt.Errorf("Grafana is not configured\n\nHint: Install it with 'foundry component install grafana'")
	}
	if enabled, ok := componentConfig.Config["ingress_enabled"].(bool); ok && !enabled {
		return "", fmt.Errorf("external Grafana access is disabled in the stack config")
	}

	host, _ := componentConfig.Config["ingress_host"].(string)
	host = strings.TrimSpace(host)
	if host == "" && stackConfig.Cluster.PrimaryDomain != "" {
		host = "grafana." + strings.TrimSpace(stackConfig.Cluster.PrimaryDomain)
	}
	if host == "" {
		return "", fmt.Errorf("Grafana does not have an external hostname in the stack config")
	}
	if strings.ContainsAny(host, "/?#") {
		return "", fmt.Errorf("Grafana has an invalid external hostname %q", host)
	}

	result := &url.URL{Scheme: "https", Host: host}
	if result.Hostname() == "" {
		return "", fmt.Errorf("Grafana has an invalid external hostname %q", host)
	}
	return result.String(), nil
}

func displayCredentials(ctx context.Context, output io.Writer, username, password string, includeUsername bool, duration time.Duration) error {
	file, ok := output.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return fmt.Errorf("--show requires an interactive terminal")
	}
	return displayCredentialsInTemporaryView(ctx, output, username, password, includeUsername, duration)
}

func displayCredentialsInTemporaryView(ctx context.Context, output io.Writer, username, password string, includeUsername bool, duration time.Duration) error {
	const (
		enterTemporaryView = "\x1b[?1049h\x1b[?25l\x1b[2J\x1b[H"
		leaveTemporaryView = "\x1b[2J\x1b[H\x1b[?25h\x1b[?1049l"
	)

	if _, err := fmt.Fprint(output, enterTemporaryView); err != nil {
		return fmt.Errorf("open temporary terminal view: %w", err)
	}
	defer func() {
		_, _ = fmt.Fprint(output, leaveTemporaryView)
	}()
	signalContext, stopSignals := signal.NotifyContext(ctx, os.Interrupt)
	defer stopSignals()

	if includeUsername {
		if _, err := fmt.Fprintf(output, "Grafana username: %s\n", username); err != nil {
			return fmt.Errorf("write Grafana username: %w", err)
		}
	}
	if _, err := fmt.Fprintf(output, "Grafana password: %s\n\n", password); err != nil {
		return fmt.Errorf("write Grafana password: %w", err)
	}
	if _, err := fmt.Fprintf(output, "This view closes in %d seconds.\n", int(duration.Seconds())); err != nil {
		return fmt.Errorf("write display timeout: %w", err)
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-signalContext.Done():
		return signalContext.Err()
	case <-timer.C:
		return nil
	}
}

// getGrafanaCredentials retrieves Grafana credentials from OpenBAO or Kubernetes secrets
func getGrafanaCredentials(ctx context.Context, configName string) (username, password string, err error) {
	// First, try OpenBAO
	openBAOClient, err := getOpenBAOClient(configName)
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
func getOpenBAOClient(configName string) (*openbao.Client, error) {
	// Load stack configuration
	configPath, err := config.FindConfig(configName)
	if err != nil {
		return nil, fmt.Errorf("failed to find stack config: %w", err)
	}
	stackConfig, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load stack config: %w\n\nHint: Run 'foundry config init' to create a configuration", err)
	}

	// Get OpenBAO address
	openBAOAddr, err := stackConfig.GetPrimaryOpenBAOURL()
	if err != nil {
		return nil, fmt.Errorf("OpenBAO host not configured: %w", err)
	}

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
