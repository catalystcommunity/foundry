package helm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/repo"
	"helm.sh/helm/v3/pkg/release"
	"sigs.k8s.io/yaml"
)

// Client wraps Helm SDK operations
type Client struct {
	namespace  string
	kubeconfig []byte
	settings   *cli.EnvSettings
}

// NewClient creates a new Helm client
// kubeconfig is the raw kubeconfig YAML bytes
// namespace is the default namespace for operations
func NewClient(kubeconfig []byte, namespace string) (*Client, error) {
	if len(kubeconfig) == 0 {
		return nil, fmt.Errorf("kubeconfig cannot be empty")
	}
	if namespace == "" {
		namespace = "default"
	}

	// Create a temporary kubeconfig file for Helm SDK
	// Helm SDK requires a file path, not in-memory config
	tmpDir, err := os.MkdirTemp("", "foundry-helm-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	kubeconfigPath := filepath.Join(tmpDir, "kubeconfig")
	if err := os.WriteFile(kubeconfigPath, kubeconfig, 0600); err != nil {
		os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	settings := cli.New()
	settings.KubeConfig = kubeconfigPath
	settings.SetNamespace(namespace)
	// Use isolated repository config in temp directory to avoid conflicts
	settings.RepositoryConfig = filepath.Join(tmpDir, "repositories.yaml")
	settings.RepositoryCache = filepath.Join(tmpDir, "cache")

	return &Client{
		namespace:  namespace,
		kubeconfig: kubeconfig,
		settings:   settings,
	}, nil
}

// Close cleans up temporary resources
func (c *Client) Close() error {
	if c.settings != nil && c.settings.KubeConfig != "" {
		tmpDir := filepath.Dir(c.settings.KubeConfig)
		return os.RemoveAll(tmpDir)
	}
	return nil
}

// getActionConfig creates an action configuration for Helm operations
func (c *Client) getActionConfig(namespace string) (*action.Configuration, error) {
	if namespace == "" {
		namespace = c.namespace
	}

	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(c.settings.RESTClientGetter(), namespace, "secret", func(format string, v ...interface{}) {
		// Log function - currently no-op
	}); err != nil {
		return nil, fmt.Errorf("failed to initialize action config: %w", err)
	}

	return actionConfig, nil
}

// AddRepo adds a Helm repository
func (c *Client) AddRepo(ctx context.Context, opts RepoAddOptions) error {
	if opts.Name == "" {
		return fmt.Errorf("repository name cannot be empty")
	}
	if opts.URL == "" {
		return fmt.Errorf("repository URL cannot be empty")
	}

	// Ensure repository config directory and cache directory exist
	repoFile := c.settings.RepositoryConfig
	if err := os.MkdirAll(filepath.Dir(repoFile), 0755); err != nil {
		return fmt.Errorf("failed to create repository config directory: %w", err)
	}
	if err := os.MkdirAll(c.settings.RepositoryCache, 0755); err != nil {
		return fmt.Errorf("failed to create repository cache directory: %w", err)
	}

	// Load existing repositories
	repoConfig := repo.NewFile()
	if _, err := os.Stat(repoFile); err == nil {
		data, err := os.ReadFile(repoFile)
		if err != nil {
			return fmt.Errorf("failed to read repository file: %w", err)
		}
		if err := yaml.Unmarshal(data, repoConfig); err != nil {
			return fmt.Errorf("failed to parse repository file: %w", err)
		}
	}

	// Check if repository already exists
	if repoConfig.Has(opts.Name) {
		if !opts.ForceUpdate {
			return fmt.Errorf("repository %s already exists", opts.Name)
		}
		repoConfig.Update(&repo.Entry{
			Name:     opts.Name,
			URL:      opts.URL,
			Username: opts.Username,
			Password: opts.Password,
		})
	} else {
		repoConfig.Add(&repo.Entry{
			Name:     opts.Name,
			URL:      opts.URL,
			Username: opts.Username,
			Password: opts.Password,
		})
	}

	// Save repository configuration
	if err := repoConfig.WriteFile(repoFile, 0644); err != nil {
		return fmt.Errorf("failed to write repository file: %w", err)
	}

	// Download repository index
	r, err := repo.NewChartRepository(&repo.Entry{
		Name:     opts.Name,
		URL:      opts.URL,
		Username: opts.Username,
		Password: opts.Password,
	}, getter.All(c.settings))
	if err != nil {
		return fmt.Errorf("failed to create chart repository: %w", err)
	}

	// Set cache path explicitly
	r.CachePath = c.settings.RepositoryCache

	if _, err := r.DownloadIndexFile(); err != nil {
		return fmt.Errorf("failed to download repository index: %w", err)
	}

	return nil
}

// Install installs a Helm chart
func (c *Client) Install(ctx context.Context, opts InstallOptions) error {
	if opts.ReleaseName == "" {
		return fmt.Errorf("release name cannot be empty")
	}
	if opts.Chart == "" {
		return fmt.Errorf("chart cannot be empty")
	}

	namespace := opts.Namespace
	if namespace == "" {
		namespace = c.namespace
	}

	actionConfig, err := c.getActionConfig(namespace)
	if err != nil {
		return err
	}

	installAction := action.NewInstall(actionConfig)
	installAction.ReleaseName = opts.ReleaseName
	installAction.Namespace = namespace
	installAction.CreateNamespace = opts.CreateNamespace
	installAction.Wait = opts.Wait
	if opts.Timeout > 0 {
		installAction.Timeout = opts.Timeout
	}
	if opts.Version != "" {
		installAction.Version = opts.Version
	}

	// Locate the chart
	chartPath, err := installAction.ChartPathOptions.LocateChart(opts.Chart, c.settings)
	if err != nil {
		return fmt.Errorf("failed to locate chart: %w", err)
	}

	// Load the chart
	chart, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("failed to load chart: %w", err)
	}

	// Install the chart
	_, err = installAction.RunWithContext(ctx, chart, opts.Values)
	if err != nil {
		return fmt.Errorf("failed to install chart: %w", err)
	}

	return nil
}

// Upgrade upgrades a Helm release
func (c *Client) Upgrade(ctx context.Context, opts UpgradeOptions) error {
	if opts.ReleaseName == "" {
		return fmt.Errorf("release name cannot be empty")
	}
	if opts.Chart == "" {
		return fmt.Errorf("chart cannot be empty")
	}

	namespace := opts.Namespace
	if namespace == "" {
		namespace = c.namespace
	}

	actionConfig, err := c.getActionConfig(namespace)
	if err != nil {
		return err
	}

	upgradeAction := action.NewUpgrade(actionConfig)
	upgradeAction.Namespace = namespace
	upgradeAction.Wait = opts.Wait
	upgradeAction.Install = opts.Install
	if opts.Timeout > 0 {
		upgradeAction.Timeout = opts.Timeout
	}
	if opts.Version != "" {
		upgradeAction.Version = opts.Version
	}

	// Locate the chart
	chartPath, err := upgradeAction.ChartPathOptions.LocateChart(opts.Chart, c.settings)
	if err != nil {
		return fmt.Errorf("failed to locate chart: %w", err)
	}

	// Load the chart
	chart, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("failed to load chart: %w", err)
	}

	// Upgrade the release
	_, err = upgradeAction.RunWithContext(ctx, opts.ReleaseName, chart, opts.Values)
	if err != nil {
		return fmt.Errorf("failed to upgrade release: %w", err)
	}

	return nil
}

// Uninstall uninstalls a Helm release
func (c *Client) Uninstall(ctx context.Context, opts UninstallOptions) error {
	if opts.ReleaseName == "" {
		return fmt.Errorf("release name cannot be empty")
	}

	namespace := opts.Namespace
	if namespace == "" {
		namespace = c.namespace
	}

	actionConfig, err := c.getActionConfig(namespace)
	if err != nil {
		return err
	}

	uninstallAction := action.NewUninstall(actionConfig)
	uninstallAction.Wait = opts.Wait
	if opts.Timeout > 0 {
		uninstallAction.Timeout = opts.Timeout
	}

	_, err = uninstallAction.Run(opts.ReleaseName)
	if err != nil {
		return fmt.Errorf("failed to uninstall release: %w", err)
	}

	return nil
}

// List lists Helm releases in a namespace
func (c *Client) List(ctx context.Context, namespace string) ([]Release, error) {
	if namespace == "" {
		namespace = c.namespace
	}

	actionConfig, err := c.getActionConfig(namespace)
	if err != nil {
		return nil, err
	}

	listAction := action.NewList(actionConfig)
	listAction.All = true

	releases, err := listAction.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to list releases: %w", err)
	}

	result := make([]Release, len(releases))
	for i, rel := range releases {
		result[i] = convertRelease(rel)
	}

	return result, nil
}

// convertRelease converts a Helm release to our Release type
func convertRelease(rel *release.Release) Release {
	return Release{
		Name:      rel.Name,
		Namespace: rel.Namespace,
		Version:   rel.Version,
		Status:    rel.Info.Status.String(),
		Chart:     fmt.Sprintf("%s-%s", rel.Chart.Metadata.Name, rel.Chart.Metadata.Version),
		AppVersion: rel.Chart.Metadata.AppVersion,
		Updated:   rel.Info.LastDeployed.Time,
	}
}
