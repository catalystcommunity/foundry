package dashboards

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

func TestValidate(t *testing.T) {
	title, err := validate([]byte(`{"title":"Hello","panels":[]}`))
	require.NoError(t, err)
	assert.Equal(t, "Hello", title)

	_, err = validate([]byte(`{"panels":[]}`))
	assert.Error(t, err, "missing title should error")

	_, err = validate([]byte(`not json`))
	assert.Error(t, err, "invalid JSON should error")
}

func TestDir(t *testing.T) {
	assert.Equal(t, "/c/catalyst_dashboards", Dir("/c", "catalyst"))
	assert.Equal(t, "/c/foundry_dashboards", Dir("/c", ""))
}

// TestSeedDefaultsAndCollect verifies the embedded defaults are written to
// <dir>/defaults and that Collect picks them up plus a user dashboard.
func TestSeedDefaultsAndCollect(t *testing.T) {
	dir := t.TempDir()

	n, err := SeedDefaults(dir)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, 3, "expected the bundled default dashboards to be seeded")

	files, err := Collect(dir)
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, f := range files {
		assert.True(t, f.Valid, "default %s should be valid: %s", f.FileName, f.Err)
		assert.Equal(t, "default", f.Source)
	}

	require.NoError(t, os.WriteFile(filepath.Join(dir, "mine.json"), []byte(`{"title":"Mine"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.json"), []byte(`{bad`), 0o644))

	files, err = Collect(dir)
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
	assert.True(t, sawUser)
	assert.True(t, sawBroken)
	assert.Len(t, Valid(files), len(files)-1, "Valid() drops the broken one")
}

func TestApply_CreateUpdatePrune(t *testing.T) {
	ctx := context.Background()
	clientset := fake.NewSimpleClientset()
	ns := "monitoring"

	files := []File{
		{FileName: "cluster-overview.json", Base: "cluster-overview", Source: "default", Data: []byte(`{"title":"A"}`), Valid: true},
		{FileName: "mine.json", Base: "mine", Source: "user", Data: []byte(`{"title":"B"}`), Valid: true},
	}

	res, err := Apply(ctx, clientset, ns, files, false)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Created)
	assert.Equal(t, 0, res.Updated)

	cm, err := clientset.CoreV1().ConfigMaps(ns).Get(ctx, "foundry-dashboard-cluster-overview", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "1", cm.Labels[dashboardLabel])
	assert.Equal(t, managedByValue, cm.Labels[managedByLabel])
	assert.Equal(t, "default", cm.Labels[sourceLabel])

	files[0].Data = []byte(`{"title":"A2"}`)
	res, err = Apply(ctx, clientset, ns, files, false)
	require.NoError(t, err)
	assert.Equal(t, 0, res.Created)
	assert.Equal(t, 2, res.Updated)

	res, err = Apply(ctx, clientset, ns, files[:1], true)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Pruned)
	_, err = clientset.CoreV1().ConfigMaps(ns).Get(ctx, "foundry-dashboard-mine", metav1.GetOptions{})
	assert.Error(t, err, "pruned user dashboard should be gone")
	_, err = clientset.CoreV1().ConfigMaps(ns).Get(ctx, "foundry-dashboard-cluster-overview", metav1.GetOptions{})
	assert.NoError(t, err)
}

// TestInstallForStack covers the shared entry point used by both the dashboard
// command and the automatic sync during Grafana install: it seeds defaults into
// <configDir>/<stack>_dashboards and applies them (plus user dashboards) to a ns.
func TestInstallForStack(t *testing.T) {
	ctx := context.Background()
	configDir := t.TempDir()
	clientset := fake.NewSimpleClientset()

	// A user dashboard at the top level of the resolved stack dir.
	stackDir := Dir(configDir, "catalyst")
	require.NoError(t, os.MkdirAll(stackDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(stackDir, "mine.json"), []byte(`{"title":"Mine"}`), 0o644))

	seeded, res, err := InstallForStack(ctx, clientset, configDir, "catalyst", "monitoring")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, seeded, 3, "bundled defaults seeded")
	// 3 defaults + 1 user dashboard all created.
	assert.Equal(t, seeded+1, res.Created)

	// The user dashboard and a default both became ConfigMaps.
	_, err = clientset.CoreV1().ConfigMaps("monitoring").Get(ctx, "foundry-dashboard-mine", metav1.GetOptions{})
	assert.NoError(t, err)
	_, err = clientset.CoreV1().ConfigMaps("monitoring").Get(ctx, "foundry-dashboard-cluster-overview", metav1.GetOptions{})
	assert.NoError(t, err)
}

func TestApply_PruneIgnoresUnmanaged(t *testing.T) {
	ctx := context.Background()
	clientset := fake.NewSimpleClientset()
	ns := "monitoring"

	_, err := clientset.CoreV1().ConfigMaps(ns).Create(ctx, mkCM("seaweedfs-grafana-dashboard", ns, map[string]string{dashboardLabel: "1"}), metav1.CreateOptions{})
	require.NoError(t, err)

	files := []File{{FileName: "x.json", Base: "x", Source: "default", Data: []byte(`{"title":"X"}`), Valid: true}}
	res, err := Apply(ctx, clientset, ns, files, true)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Created)
	assert.Equal(t, 0, res.Pruned)

	_, err = clientset.CoreV1().ConfigMaps(ns).Get(ctx, "seaweedfs-grafana-dashboard", metav1.GetOptions{})
	assert.NoError(t, err, "unmanaged dashboard must not be pruned")
}
