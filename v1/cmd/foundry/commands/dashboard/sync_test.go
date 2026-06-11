package dashboard

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func mkCM(name, ns string, labels map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Labels: labels},
		Data:       map[string]string{"x.json": "{}"},
	}
}

func TestSanitizeName(t *testing.T) {
	cases := map[string]string{
		"cluster-overview":     "cluster-overview",
		"My Dashboard":         "my-dashboard",
		"weird__name.json.bak": "weird--name-json-bak",
		"---":                  "dashboard",
		"":                     "dashboard",
	}
	for in, want := range cases {
		assert.Equal(t, want, sanitizeName(in), "input %q", in)
	}
}

func TestValidateDashboard(t *testing.T) {
	title, err := validateDashboard([]byte(`{"title":"Hello","panels":[]}`))
	require.NoError(t, err)
	assert.Equal(t, "Hello", title)

	_, err = validateDashboard([]byte(`{"panels":[]}`))
	assert.Error(t, err, "missing title should error")

	_, err = validateDashboard([]byte(`not json`))
	assert.Error(t, err, "invalid JSON should error")
}

// TestSeedDefaultsAndCollect verifies the embedded defaults are written to
// <dir>/defaults and that collectDashboards picks them up plus a user dashboard.
func TestSeedDefaultsAndCollect(t *testing.T) {
	dir := t.TempDir()

	n, err := seedDefaults(dir)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, 3, "expected the bundled default dashboards to be seeded")

	// The embedded defaults should all be valid JSON with titles.
	files, err := collectDashboards(dir)
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, f := range files {
		assert.True(t, f.Valid, "default %s should be valid: %s", f.FileName, f.Err)
		assert.Equal(t, "default", f.Source)
	}

	// Add a user dashboard at the top level and a broken one.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mine.json"), []byte(`{"title":"Mine"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.json"), []byte(`{bad`), 0o644))

	files, err = collectDashboards(dir)
	require.NoError(t, err)

	var sawUser, sawBroken bool
	for _, f := range files {
		if f.FileName == "mine.json" {
			sawUser = true
			assert.Equal(t, "user", f.Source)
			assert.True(t, f.Valid)
			assert.Equal(t, "Mine", f.Title)
		}
		if f.FileName == "broken.json" {
			sawBroken = true
			assert.False(t, f.Valid)
		}
	}
	assert.True(t, sawUser, "user dashboard should be collected")
	assert.True(t, sawBroken, "broken dashboard should be collected (and flagged invalid)")
}

func TestApplyDashboards_CreateUpdatePrune(t *testing.T) {
	ctx := context.Background()
	clientset := fake.NewSimpleClientset()
	ns := "monitoring"

	files := []dashboardFile{
		{FileName: "cluster-overview.json", Base: "cluster-overview", Source: "default", Data: []byte(`{"title":"A"}`), Valid: true},
		{FileName: "mine.json", Base: "mine", Source: "user", Data: []byte(`{"title":"B"}`), Valid: true},
	}

	// First apply: both created.
	res, err := applyDashboards(ctx, clientset, ns, files, false)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Created)
	assert.Equal(t, 0, res.Updated)

	cm, err := clientset.CoreV1().ConfigMaps(ns).Get(ctx, "foundry-dashboard-cluster-overview", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "1", cm.Labels[dashboardLabel])
	assert.Equal(t, managedByValue, cm.Labels[managedByLabel])
	assert.Equal(t, "default", cm.Labels[sourceLabel])
	assert.Contains(t, cm.Data, "cluster-overview.json")

	// Second apply with changed data: both updated.
	files[0].Data = []byte(`{"title":"A2"}`)
	res, err = applyDashboards(ctx, clientset, ns, files, false)
	require.NoError(t, err)
	assert.Equal(t, 0, res.Created)
	assert.Equal(t, 2, res.Updated)
	cm, _ = clientset.CoreV1().ConfigMaps(ns).Get(ctx, "foundry-dashboard-cluster-overview", metav1.GetOptions{})
	assert.Equal(t, `{"title":"A2"}`, cm.Data["cluster-overview.json"])

	// Drop the user dashboard and prune: it should be deleted, default kept.
	res, err = applyDashboards(ctx, clientset, ns, files[:1], true)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Pruned)

	_, err = clientset.CoreV1().ConfigMaps(ns).Get(ctx, "foundry-dashboard-mine", metav1.GetOptions{})
	assert.Error(t, err, "pruned user dashboard should be gone")
	_, err = clientset.CoreV1().ConfigMaps(ns).Get(ctx, "foundry-dashboard-cluster-overview", metav1.GetOptions{})
	assert.NoError(t, err, "remaining dashboard should be kept")
}

// TestApplyDashboards_PruneIgnoresUnmanaged ensures prune never touches ConfigMaps
// that foundry does not manage (e.g. dashboards shipped by other components).
func TestApplyDashboards_PruneIgnoresUnmanaged(t *testing.T) {
	ctx := context.Background()
	clientset := fake.NewSimpleClientset()
	ns := "monitoring"

	// A foreign dashboard ConfigMap (sidecar label but not foundry-managed).
	_, err := clientset.CoreV1().ConfigMaps(ns).Create(ctx, mkCM("seaweedfs-grafana-dashboard", ns, map[string]string{dashboardLabel: "1"}), metav1.CreateOptions{})
	require.NoError(t, err)

	files := []dashboardFile{
		{FileName: "x.json", Base: "x", Source: "default", Data: []byte(`{"title":"X"}`), Valid: true},
	}
	res, err := applyDashboards(ctx, clientset, ns, files, true)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
	assert.Equal(t, 0, res.Pruned)

	_, err = clientset.CoreV1().ConfigMaps(ns).Get(ctx, "seaweedfs-grafana-dashboard", metav1.GetOptions{})
	assert.NoError(t, err, "unmanaged dashboard must not be pruned")
}
