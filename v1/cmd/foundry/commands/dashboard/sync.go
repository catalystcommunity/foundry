package dashboard

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

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/urfave/cli/v3"
)

//go:embed defaults/*.json
var defaultDashboards embed.FS

const (
	// dashboardLabel is the label the Grafana sidecar uses to discover dashboards.
	dashboardLabel = "grafana_dashboard"
	// managedByLabel/Value mark ConfigMaps that foundry owns, so we can prune safely.
	managedByLabel = "app.kubernetes.io/managed-by"
	managedByValue = "foundry"
	// sourceLabel records whether a dashboard came from the bundled defaults or the user.
	sourceLabel = "foundry.io/dashboard-source"

	configMapNamePrefix = "foundry-dashboard-"
	defaultsSubdir      = "defaults"
)

// SyncCommand renders dashboard JSON files into Grafana-discoverable ConfigMaps.
var SyncCommand = &cli.Command{
	Name:  "sync",
	Usage: "Install dashboards from the stack dashboards directory into Grafana",
	Description: `Seeds the bundled default dashboards into ~/.foundry/<stack>_dashboards/defaults/,
then renders every dashboard JSON file (the managed defaults plus any user dashboards
you drop directly in ~/.foundry/<stack>_dashboards/) into ConfigMaps labeled
` + "`grafana_dashboard=1`" + `. The Grafana dashboard sidecar discovers and loads them
automatically — no Grafana restart required.

User dashboards are any *.json files placed at the top level of the directory.
The defaults/ subdirectory is managed by foundry and refreshed on every sync.

Examples:
  foundry dashboard sync                 # seed defaults + install all dashboards
  foundry dashboard sync --prune         # also remove dashboards no longer present
  foundry dashboard sync --dry-run       # show what would change`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "namespace",
			Aliases: []string{"n"},
			Usage:   "Namespace where Grafana (and its dashboard sidecar) runs",
			Value:   "monitoring",
		},
		&cli.StringFlag{
			Name:  "dir",
			Usage: "Override the dashboards directory (default ~/.foundry/<stack>_dashboards)",
		},
		&cli.BoolFlag{
			Name:  "prune",
			Usage: "Delete foundry-managed dashboard ConfigMaps that are no longer present locally",
		},
		&cli.BoolFlag{
			Name:    "dry-run",
			Aliases: []string{"d"},
			Usage:   "Show what would change without modifying the cluster",
		},
		&cli.BoolFlag{
			Name:  "no-seed",
			Usage: "Do not refresh the bundled default dashboards before syncing",
		},
	},
	Action: runSync,
}

// ListCommand lists dashboards in the directory and (optionally) installed ConfigMaps.
var ListCommand = &cli.Command{
	Name:  "list",
	Usage: "List available dashboards in the stack dashboards directory",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "dir", Usage: "Override the dashboards directory"},
	},
	Action: runList,
}

// dashboardFile is a single dashboard source on disk.
type dashboardFile struct {
	FileName string // e.g. "cluster-overview.json"
	Base     string // e.g. "cluster-overview"
	Source   string // "default" or "user"
	Data     []byte
	Title    string
	Valid    bool
	Err      string
}

func runSync(ctx context.Context, cmd *cli.Command) error {
	dir, err := resolveDashboardsDir(cmd)
	if err != nil {
		return err
	}
	namespace := cmd.String("namespace")
	dryRun := cmd.Bool("dry-run")

	if !cmd.Bool("no-seed") {
		n, err := seedDefaults(dir)
		if err != nil {
			return fmt.Errorf("failed to seed default dashboards: %w", err)
		}
		fmt.Printf("Seeded %d default dashboard(s) into %s\n", n, filepath.Join(dir, defaultsSubdir))
	}

	files, err := collectDashboards(dir)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		fmt.Printf("No dashboards found in %s\n", dir)
		return nil
	}

	valid := make([]dashboardFile, 0, len(files))
	for _, f := range files {
		if !f.Valid {
			fmt.Printf("  ⚠ skipping %s (%s): %s\n", f.FileName, f.Source, f.Err)
			continue
		}
		valid = append(valid, f)
	}

	if dryRun {
		fmt.Printf("\nDry-run: would install %d dashboard(s) into namespace %q:\n", len(valid), namespace)
		for _, f := range valid {
			fmt.Printf("  %s  ->  configmap/%s  [%s]\n", f.FileName, configMapName(f.Base), f.Source)
		}
		if cmd.Bool("prune") {
			fmt.Println("  (prune enabled: stale foundry-managed dashboards would be removed)")
		}
		return nil
	}

	clientset, err := getK8sClient()
	if err != nil {
		return err
	}

	result, err := applyDashboards(ctx, clientset, namespace, valid, cmd.Bool("prune"))
	if err != nil {
		return err
	}

	fmt.Printf("\n✓ Dashboards synced to namespace %q: %d created, %d updated", namespace, result.Created, result.Updated)
	if cmd.Bool("prune") {
		fmt.Printf(", %d pruned", result.Pruned)
	}
	fmt.Println()
	fmt.Println("The Grafana sidecar will load them within ~1 minute. Open with: foundry dashboard open")
	return nil
}

func runList(ctx context.Context, cmd *cli.Command) error {
	dir, err := resolveDashboardsDir(cmd)
	if err != nil {
		return err
	}
	if _, err := seedDefaults(dir); err != nil {
		return fmt.Errorf("failed to seed default dashboards: %w", err)
	}
	files, err := collectDashboards(dir)
	if err != nil {
		return err
	}
	fmt.Printf("Dashboards directory: %s\n\n", dir)
	if len(files) == 0 {
		fmt.Println("(none)")
		return nil
	}
	fmt.Printf("%-32s %-8s %-7s %s\n", "FILE", "SOURCE", "VALID", "TITLE")
	for _, f := range files {
		valid := "yes"
		title := f.Title
		if !f.Valid {
			valid = "NO"
			title = f.Err
		}
		fmt.Printf("%-32s %-8s %-7s %s\n", f.FileName, f.Source, valid, title)
	}
	return nil
}

// resolveDashboardsDir determines the dashboards directory, honoring --dir, else
// derives it from the configured stack name: ~/.foundry/<stack>_dashboards.
func resolveDashboardsDir(cmd *cli.Command) (string, error) {
	if override := cmd.String("dir"); override != "" {
		return override, nil
	}
	stack := resolveStackName(cmd)
	configDir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, stack+"_dashboards"), nil
}

// resolveStackName reads the cluster name from the stack config, falling back to
// "foundry" when no config is available.
func resolveStackName(cmd *cli.Command) string {
	configPath, err := config.FindConfig(cmd.String("config"))
	if err != nil {
		return "foundry"
	}
	cfg, err := config.Load(configPath)
	if err != nil || cfg.Cluster.Name == "" {
		return "foundry"
	}
	return cfg.Cluster.Name
}

// seedDefaults writes the embedded default dashboards into <dir>/defaults/, always
// refreshing them (they are foundry-managed). Returns the number written.
func seedDefaults(dir string) (int, error) {
	entries, err := defaultDashboards.ReadDir(defaultsSubdir)
	if err != nil {
		return 0, err
	}
	target := filepath.Join(dir, defaultsSubdir)
	if err := os.MkdirAll(target, 0o755); err != nil {
		return 0, err
	}
	count := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := defaultDashboards.ReadFile(filepath.Join(defaultsSubdir, e.Name()))
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

// collectDashboards gathers managed defaults (<dir>/defaults/*.json) and user
// dashboards (<dir>/*.json at the top level), validating each. Results are sorted
// by source (default first) then file name.
func collectDashboards(dir string) ([]dashboardFile, error) {
	var files []dashboardFile

	// Managed defaults.
	defaultsPath := filepath.Join(dir, defaultsSubdir)
	if defs, err := readDashboardDir(defaultsPath, "default"); err == nil {
		files = append(files, defs...)
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	// User dashboards at the top level (non-recursive).
	if users, err := readDashboardDir(dir, "user"); err == nil {
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

func readDashboardDir(path, source string) ([]dashboardFile, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var out []dashboardFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		full := filepath.Join(path, e.Name())
		df := dashboardFile{
			FileName: e.Name(),
			Base:     strings.TrimSuffix(e.Name(), ".json"),
			Source:   source,
		}
		data, err := os.ReadFile(full)
		if err != nil {
			df.Err = err.Error()
			out = append(out, df)
			continue
		}
		df.Data = data
		title, verr := validateDashboard(data)
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

// validateDashboard ensures the bytes are a JSON object with a non-empty title,
// returning the title.
func validateDashboard(data []byte) (string, error) {
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

// SyncResult summarizes an applyDashboards run.
type SyncResult struct {
	Created int
	Updated int
	Pruned  int
}

// applyDashboards creates/updates one ConfigMap per dashboard and, when prune is
// set, removes foundry-managed dashboard ConfigMaps that are no longer present.
func applyDashboards(ctx context.Context, clientset kubernetes.Interface, namespace string, files []dashboardFile, prune bool) (*SyncResult, error) {
	result := &SyncResult{}
	cms := clientset.CoreV1().ConfigMaps(namespace)
	desired := make(map[string]bool, len(files))

	for _, f := range files {
		name := configMapName(f.Base)
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

// configMapName builds a DNS-1123-safe ConfigMap name for a dashboard base name.
func configMapName(base string) string {
	return configMapNamePrefix + sanitizeName(base)
}

func sanitizeName(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '.':
			b.WriteRune('-')
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
