package kyverno

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	foundrykyverno "github.com/catalystcommunity/foundry/v1/internal/kyverno"
)

const (
	DefaultChartVersion = "3.9.1"
	DefaultAppVersion   = "v1.19.1"
)

// Config controls the Kyverno Helm release and Foundry policy reconciliation.
type Config struct {
	Version   string
	Namespace string
	Values    map[string]interface{}
	Policies  []foundrykyverno.Selection
}

// DefaultConfig returns the Kyverno release supported by this Foundry build.
func DefaultConfig() *Config {
	return &Config{
		Version: DefaultChartVersion, Namespace: "kyverno", Values: map[string]interface{}{
			"config": map[string]interface{}{"resourceFiltersExclude": []interface{}{"[ReplicaSet,*,*]", "[ReplicaSet/?*,*,*]"}},
		},
	}
}

// ParseConfig converts the flexible component configuration into Kyverno settings.
func ParseConfig(input component.ComponentConfig) (*Config, error) {
	config := DefaultConfig()
	if version, ok := input.GetString("version"); ok && strings.TrimSpace(version) != "" {
		config.Version = strings.TrimSpace(version)
	}
	if namespace, ok := input.GetString("namespace"); ok && strings.TrimSpace(namespace) != "" {
		config.Namespace = strings.TrimSpace(namespace)
	}
	if values, ok := input.GetMap("values"); ok {
		config.Values = maps.Clone(values)
		settings := map[string]interface{}{}
		if raw, exists := values["config"]; exists {
			value, ok := raw.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("Kyverno values.config must be a map")
			}
			settings = maps.Clone(value)
		}
		exclusions := []interface{}{}
		switch value := settings["resourceFiltersExclude"].(type) {
		case nil:
		case []interface{}:
			exclusions = slices.Clone(value)
		case []string:
			for _, item := range value {
				exclusions = append(exclusions, item)
			}
		default:
			return nil, fmt.Errorf("Kyverno values.config.resourceFiltersExclude must be a list")
		}
		for _, item := range exclusions {
			if _, ok := item.(string); !ok {
				return nil, fmt.Errorf("Kyverno resource filter exclusions must be strings")
			}
		}
		// The chart skips ReplicaSets by default. That would let users create
		// resources in default without evaluating the namespace policy.
		for _, required := range []interface{}{"[ReplicaSet,*,*]", "[ReplicaSet/?*,*,*]"} {
			if !slices.Contains(exclusions, required) {
				exclusions = append(exclusions, required)
			}
		}
		settings["resourceFiltersExclude"] = exclusions
		config.Values["config"] = settings
	}
	if policies, ok := input["policies"].([]foundrykyverno.Selection); ok {
		config.Policies = append([]foundrykyverno.Selection(nil), policies...)
	}
	return config, nil
}

// HelmClient is the Helm surface used by this component.
type HelmClient interface {
	AddRepo(context.Context, helm.RepoAddOptions) error
	Install(context.Context, helm.InstallOptions) error
	Upgrade(context.Context, helm.UpgradeOptions) error
	Uninstall(context.Context, helm.UninstallOptions) error
	List(context.Context, string) ([]helm.Release, error)
}

// Component installs Kyverno and reconciles the Foundry policy catalog.
type Component struct {
	helmClient HelmClient
	k8sClient  *k8s.Client
}

// NewComponent creates a Kyverno component.
func NewComponent(helmClient HelmClient, k8sClient *k8s.Client) *Component {
	return &Component{helmClient: helmClient, k8sClient: k8sClient}
}

func (c *Component) Name() string { return "kyverno" }

func (c *Component) Dependencies() []string { return []string{"k3s"} }

func (c *Component) Install(ctx context.Context, input component.ComponentConfig) error {
	config, err := ParseConfig(input)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	return Install(ctx, c.helmClient, c.k8sClient, config)
}

func (c *Component) Upgrade(ctx context.Context, input component.ComponentConfig) error {
	return c.Install(ctx, input)
}

func (c *Component) Status(ctx context.Context) (*component.ComponentStatus, error) {
	if c.helmClient == nil {
		return &component.ComponentStatus{Message: "Helm client is not initialized"}, nil
	}
	releases, err := c.helmClient.List(ctx, "kyverno")
	if err != nil {
		return nil, fmt.Errorf("list Kyverno releases: %w", err)
	}
	for _, release := range releases {
		if release.Name == releaseName {
			return &component.ComponentStatus{
				Installed: true,
				Healthy:   release.Status == "deployed",
				Version:   release.AppVersion,
				Message:   release.Status,
			}, nil
		}
	}
	return &component.ComponentStatus{Message: "Kyverno is not installed"}, nil
}

func (c *Component) Uninstall(ctx context.Context) error {
	if c.helmClient == nil {
		return fmt.Errorf("Helm client is not initialized")
	}
	return c.helmClient.Uninstall(ctx, helm.UninstallOptions{
		ReleaseName: releaseName, Namespace: "kyverno", Wait: true,
	})
}
