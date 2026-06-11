// Package dashboards manages Grafana dashboards as ConfigMaps that the Grafana
// dashboard sidecar discovers. It bundles default dashboards (embedded), seeds them
// into the per-stack dashboards directory, and applies any dashboards found there.
// It is used both by the `foundry dashboard` command and automatically during
// `foundry component install grafana`.
package dashboards

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

//go:embed defaults/*.json
var defaultDashboards embed.FS

const (
	// dashboardLabel is the label the Grafana sidecar uses to discover dashboards.
	dashboardLabel = "grafana_dashboard"
	// managedByLabel/Value mark ConfigMaps foundry owns, so prune is safe.
	managedByLabel = "app.kubernetes.io/managed-by"
	managedByValue = "foundry"
	// sourceLabel records whether a dashboard came from the bundled defaults or user.
	sourceLabel = "foundry.io/dashboard-source"

	configMapNamePrefix = "foundry-dashboard-"

	// DefaultsSubdir is the managed subdirectory under the stack dashboards dir.
	DefaultsSubdir = "defaults"
)

// File is a single dashboard source on disk.
type File struct {
	FileName string // e.g. "cluster-overview.json"
	Base     string // e.g. "cluster-overview"
	Source   string // "default" or "user"
	Data     []byte
	Title    string
	Valid    bool
	Err      string
}

// SyncResult summarizes an Apply run.
type SyncResult struct {
	Created int
	Updated int
	Pruned  int
}

// Dir returns the per-stack dashboards directory: <configDir>/<stack>_dashboards.
func Dir(configDir, stackName string) string {
	if stackName == "" {
		stackName = "foundry"
	}
	return filepath.Join(configDir, stackName+"_dashboards")
}

// SeedDefaults writes the embedded default dashboards into <dir>/defaults/, always
// refreshing them (they are foundry-managed). Returns the number written.
func SeedDefaults(dir string) (int, error) {
	entries, err := defaultDashboards.ReadDir(DefaultsSubdir)
	if err != nil {
		return 0, err
	}
	target := filepath.Join(dir, DefaultsSubdir)
	if err := os.MkdirAll(target, 0o755); err != nil {
		return 0, err
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := defaultDashboards.ReadFile(filepath.Join(DefaultsSubdir, e.Name()))
		if err != nil {
			return count, err
		}
		if err := os.WriteFile(filepath.Join(target, e.Name()), data, 0o644); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// Collect gathers managed defaults (<dir>/defaults/*.json) and user dashboards
// (<dir>/*.json at the top level), validating each. Sorted by source (default
// first) then file name. A missing directory is not an error (returns what exists).
func Collect(dir string) ([]File, error) {
	var files []File

	defaultsPath := filepath.Join(dir, DefaultsSubdir)
	if defs, err := readDir(defaultsPath, "default"); err == nil {
		files = append(files, defs...)
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	if users, err := readDir(dir, "user"); err == nil {
		files = append(files, users...)
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].Source != files[j].Source {
			return files[i].Source < files[j].Source // "default" < "user"
		}
		return files[i].FileName < files[j].FileName
	})
	return files, nil
}

// Valid returns only the entries that parsed successfully.
func Valid(files []File) []File {
	out := make([]File, 0, len(files))
	for _, f := range files {
		if f.Valid {
			out = append(out, f)
		}
	}
	return out
}

func readDir(path, source string) ([]File, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var out []File
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		df := File{
			FileName: e.Name(),
			Base:     strings.TrimSuffix(e.Name(), ".json"),
			Source:   source,
		}
		data, err := os.ReadFile(filepath.Join(path, e.Name()))
		if err != nil {
			df.Err = err.Error()
			out = append(out, df)
			continue
		}
		df.Data = data
		title, verr := validate(data)
		if verr != nil {
			df.Err = verr.Error()
		} else {
			df.Valid = true
			df.Title = title
		}
		out = append(out, df)
	}
	return out, nil
}

// validate ensures the bytes are a JSON object with a non-empty title.
func validate(data []byte) (string, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}
	title, _ := m["title"].(string)
	if strings.TrimSpace(title) == "" {
		return "", fmt.Errorf("dashboard has no \"title\" field")
	}
	return title, nil
}

// Apply creates/updates one ConfigMap per dashboard and, when prune is set, removes
// foundry-managed dashboard ConfigMaps that are no longer present. Progress is
// printed to stdout.
func Apply(ctx context.Context, clientset kubernetes.Interface, namespace string, files []File, prune bool) (*SyncResult, error) {
	result := &SyncResult{}
	cms := clientset.CoreV1().ConfigMaps(namespace)
	desired := make(map[string]bool, len(files))

	for _, f := range files {
		name := ConfigMapName(f.Base)
		desired[name] = true
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					dashboardLabel: "1",
					managedByLabel: managedByValue,
					sourceLabel:    f.Source,
				},
			},
			Data: map[string]string{f.FileName: string(f.Data)},
		}

		existing, err := cms.Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			if _, err := cms.Create(ctx, cm, metav1.CreateOptions{}); err != nil {
				return result, fmt.Errorf("failed to create %s: %w", name, err)
			}
			result.Created++
			fmt.Printf("  + %s  (%s)\n", name, f.Source)
			continue
		}
		if err != nil {
			return result, fmt.Errorf("failed to read %s: %w", name, err)
		}
		cm.ResourceVersion = existing.ResourceVersion
		if _, err := cms.Update(ctx, cm, metav1.UpdateOptions{}); err != nil {
			return result, fmt.Errorf("failed to update %s: %w", name, err)
		}
		result.Updated++
		fmt.Printf("  ~ %s  (%s)\n", name, f.Source)
	}

	if prune {
		list, err := cms.List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=%s,%s", managedByLabel, managedByValue, dashboardLabel),
		})
		if err != nil {
			return result, fmt.Errorf("failed to list managed dashboards for prune: %w", err)
		}
		for _, item := range list.Items {
			if desired[item.Name] {
				continue
			}
			if err := cms.Delete(ctx, item.Name, metav1.DeleteOptions{}); err != nil {
				return result, fmt.Errorf("failed to prune %s: %w", item.Name, err)
			}
			result.Pruned++
			fmt.Printf("  - %s  (pruned)\n", item.Name)
		}
	}

	return result, nil
}

// InstallForStack seeds the bundled defaults into the stack's dashboards directory
// (<configDir>/<stackName>_dashboards) and applies all valid dashboards — the
// managed defaults plus any user dashboards — to namespace as ConfigMaps. It does
// not prune. Returns the number of defaults seeded and the apply result. This is the
// shared entry point used by both `foundry dashboard sync` callers and the automatic
// sync during Grafana install/upgrade.
func InstallForStack(ctx context.Context, clientset kubernetes.Interface, configDir, stackName, namespace string) (int, *SyncResult, error) {
	dir := Dir(configDir, stackName)
	seeded, err := SeedDefaults(dir)
	if err != nil {
		return 0, nil, fmt.Errorf("seed default dashboards: %w", err)
	}
	files, err := Collect(dir)
	if err != nil {
		return seeded, nil, err
	}
	valid := Valid(files)
	if len(valid) == 0 {
		return seeded, &SyncResult{}, nil
	}
	res, err := Apply(ctx, clientset, namespace, valid, false)
	return seeded, res, err
}

// ConfigMapName builds a DNS-1123-safe ConfigMap name for a dashboard base name.
func ConfigMapName(base string) string {
	return configMapNamePrefix + sanitizeName(base)
}

func sanitizeName(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "dashboard"
	}
	if len(out) > 200 {
		out = out[:200]
	}
	return out
}
