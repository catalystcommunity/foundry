package gatewaycontroller

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/catalystcommunity/foundry/v1/charts"
	"github.com/catalystcommunity/foundry/v1/internal/helm"
)

// Install deploys the gateway controller from its embedded Helm chart. The chart
// is written to a temporary directory and installed (or upgraded if the release
// already exists). No chart repository is required — the chart ships in the
// binary.
func Install(ctx context.Context, helmClient HelmClient, cfg *Config) error {
	if helmClient == nil {
		return fmt.Errorf("helm client cannot be nil")
	}
	if cfg == nil {
		cfg = DefaultConfig()
	}

	fmt.Println("  Installing gateway controller...")

	chartPath, cleanup, err := extractChart()
	if err != nil {
		return fmt.Errorf("failed to extract embedded chart: %w", err)
	}
	defer cleanup()

	values := buildHelmValues(cfg)

	// Upgrade if the release already exists, otherwise install.
	exists := false
	if releases, err := helmClient.List(ctx, cfg.Namespace); err == nil {
		for i := range releases {
			if releases[i].Name == releaseName {
				exists = true
				break
			}
		}
	}

	if exists {
		fmt.Println("  Upgrading existing gateway controller release...")
		if err := helmClient.Upgrade(ctx, helm.UpgradeOptions{
			ReleaseName:     releaseName,
			Namespace:       cfg.Namespace,
			Chart:           chartPath,
			Values:          values,
			Install:         true,
			CreateNamespace: true,
			Wait:            true,
			Timeout:         5 * time.Minute,
		}); err != nil {
			return fmt.Errorf("failed to upgrade gateway controller: %w", err)
		}
	} else {
		if err := helmClient.Install(ctx, helm.InstallOptions{
			ReleaseName:     releaseName,
			Namespace:       cfg.Namespace,
			Chart:           chartPath,
			Values:          values,
			CreateNamespace: true,
			Wait:            true,
			Timeout:         5 * time.Minute,
		}); err != nil {
			return fmt.Errorf("failed to install gateway controller: %w", err)
		}
	}

	fmt.Printf("  Gateway controller installed in namespace %q\n", cfg.Namespace)
	return nil
}

// extractChart writes the embedded chart to a temp directory and returns the
// path to the chart root plus a cleanup function.
func extractChart() (string, func(), error) {
	tmp, err := os.MkdirTemp("", "foundry-gwc-chart-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }

	err = fs.WalkDir(charts.GatewayControllerChart, charts.GatewayControllerChartDir,
		func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			target := filepath.Join(tmp, p)
			if d.IsDir() {
				return os.MkdirAll(target, 0o755)
			}
			data, readErr := charts.GatewayControllerChart.ReadFile(p)
			if readErr != nil {
				return readErr
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			return os.WriteFile(target, data, 0o644)
		})
	if err != nil {
		cleanup()
		return "", nil, err
	}
	return filepath.Join(tmp, charts.GatewayControllerChartDir), cleanup, nil
}

// buildHelmValues maps the component config onto the chart's values. The typed
// convenience fields (image, controller, replicaCount) form the base; the raw
// `values` map is then deep-merged on top as a values.yaml-style overlay, so a
// user can override any leaf (and only that leaf) without losing sibling
// defaults.
func buildHelmValues(cfg *Config) map[string]interface{} {
	base := make(map[string]interface{})

	if cfg.ReplicaCount > 0 {
		base["replicaCount"] = cfg.ReplicaCount
	}

	image := make(map[string]interface{})
	if cfg.ImageRepository != "" {
		image["repository"] = cfg.ImageRepository
	}
	if cfg.ImageTag != "" {
		image["tag"] = cfg.ImageTag
	}
	if len(image) > 0 {
		base["image"] = image
	}

	base["controller"] = map[string]interface{}{
		"gatewayName":      cfg.GatewayName,
		"gatewayNamespace": cfg.GatewayNamespace,
		"envoyService":     cfg.EnvoyService,
		"networkPolicy":    cfg.NetworkPolicy,
		"interval":         cfg.Interval,
	}

	return mergeValues(base, cfg.Values)
}

// mergeValues deep-merges overlay onto base and returns a new map. Overlay
// values win; nested maps are merged recursively rather than replaced. base is
// not mutated.
func mergeValues(base, overlay map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(base))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		if overlayMap, ok := asStringMap(v); ok {
			if baseMap, ok := asStringMap(out[k]); ok {
				out[k] = mergeValues(baseMap, overlayMap)
				continue
			}
		}
		out[k] = v
	}
	return out
}

// asStringMap normalizes a value to map[string]interface{} if it is a map,
// tolerating the map[interface{}]interface{} some YAML decoders produce.
func asStringMap(v interface{}) (map[string]interface{}, bool) {
	switch m := v.(type) {
	case map[string]interface{}:
		return m, true
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(m))
		for k, val := range m {
			if ks, ok := k.(string); ok {
				out[ks] = val
			} else {
				return nil, false
			}
		}
		return out, true
	default:
		return nil, false
	}
}
