package kyverno

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	kyaml "sigs.k8s.io/yaml"
)

// TestPolicyAdmission uses a disposable Kind cluster with Kyverno installed.
// Set FOUNDRY_POLICY_TEST_KUBECONFIG to its kubeconfig. The context must start
// with kind-foundry-policy-review-. This test changes the bundled policies.
func TestPolicyAdmission(t *testing.T) {
	path := os.Getenv("FOUNDRY_POLICY_TEST_KUBECONFIG")
	if testing.Short() || path == "" {
		t.Skip("set FOUNDRY_POLICY_TEST_KUBECONFIG for admission tests")
	}
	loaded, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(loaded.CurrentContext, "kind-foundry-policy-review-"), "use a disposable Foundry test cluster")
	restConfig, err := clientcmd.BuildConfigFromFlags("", path)
	require.NoError(t, err)
	client, err := dynamic.NewForConfig(restConfig)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	catalog, err := LoadCatalog()
	require.NoError(t, err)
	reconciler, err := NewReconciler(client, catalog)
	require.NoError(t, err)
	policies := client.Resource(policyResources["ValidatingPolicy"])
	namespaces := client.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"})
	ns := "foundry-policy-admission"
	_, err = namespaces.Create(ctx, &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]interface{}{"name": ns},
	}}, metav1.CreateOptions{})
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanup, done := context.WithTimeout(context.Background(), time.Minute)
		defer done()
		_, err := reconciler.Reconcile(cleanup, disableAll(catalog))
		require.NoError(t, err)
		require.NoError(t, namespaces.Delete(cleanup, ns, metav1.DeleteOptions{}))
	})

	selectOnly := func(name string, state State) []Selection {
		selections := disableAll(catalog)
		for i := range selections {
			if selections[i].Name == name {
				selections[i].State = state
			}
		}
		return selections
	}
	apply := func(t *testing.T, selections []Selection) {
		t.Helper()
		_, err := reconciler.Reconcile(ctx, selections)
		require.NoError(t, err)
		resolved, err := catalog.ResolveSelections(selections)
		require.NoError(t, err)
		for _, selection := range resolved {
			if selection.State == StateDisabled {
				continue
			}
			require.Eventually(t, func() bool {
				object, err := policies.Get(ctx, selection.Name, metav1.GetOptions{})
				if err != nil {
					return false
				}
				ready, _, _ := unstructured.NestedBool(object.Object, "status", "conditionStatus", "ready")
				return ready
			}, 60*time.Second, 500*time.Millisecond, "policy %s did not become ready", selection.Name)
		}
	}
	admit := func(t *testing.T, gvr schema.GroupVersionResource, namespace string, object *unstructured.Unstructured, deniedBy string) {
		t.Helper()
		var lastErr error
		converged := assert.Eventually(t, func() bool {
			_, lastErr = client.Resource(gvr).Namespace(namespace).Create(ctx, object, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}})
			if deniedBy == "" {
				return lastErr == nil
			}
			return lastErr != nil && strings.Contains(lastErr.Error(), deniedBy)
		}, 30*time.Second, 500*time.Millisecond, "admission did not converge for %s", object.GetKind())
		if !converged {
			t.Fatalf("last admission error: %v", lastErr)
		}
		if deniedBy == "" {
			require.NoError(t, lastErr)
		} else {
			require.ErrorContains(t, lastErr, deniedBy)
			require.NotContains(t, lastErr.Error(), "no such key")
			require.NotContains(t, lastErr.Error(), "no such overload")
		}
	}
	pods := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	pod := func() *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Pod", "metadata": map[string]interface{}{"name": "admission-probe"},
			"spec": map[string]interface{}{"containers": []interface{}{map[string]interface{}{
				"name": "app", "image": "nginx:1.27", "readinessProbe": map[string]interface{}{"exec": map[string]interface{}{"command": []interface{}{"true"}}},
			}}},
		}}
	}

	t.Run("defaults and optional RBAC subjects", func(t *testing.T) {
		apply(t, nil)
		admit(t, pods, "default", pod(), "disallow-default-namespace")
		admit(t, pods, ns, pod(), "")
		binding := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding", "metadata": map[string]interface{}{"name": "empty-binding"},
			"roleRef": map[string]interface{}{"apiGroup": "rbac.authorization.k8s.io", "kind": "ClusterRole", "name": "view"},
		}}
		rbac := schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"}
		admit(t, rbac, ns, binding, "")
		binding.Object["subjects"] = []interface{}{map[string]interface{}{"kind": "Group", "apiGroup": "rbac.authorization.k8s.io", "name": "system:masters"}}
		admit(t, rbac, ns, binding, "restrict-binding-system-groups")
	})

	t.Run("probe checks and controller generation", func(t *testing.T) {
		apply(t, selectOnly("require-pod-probes", StateEnforce))
		valid := pod()
		init := map[string]interface{}{"name": "migrate", "image": "busybox:1.36"}
		require.NoError(t, unstructured.SetNestedSlice(valid.Object, []interface{}{init}, "spec", "initContainers"))
		admit(t, pods, ns, valid, "")
		invalid := pod()
		containers, _, _ := unstructured.NestedSlice(invalid.Object, "spec", "containers")
		delete(containers[0].(map[string]interface{}), "readinessProbe")
		require.NoError(t, unstructured.SetNestedSlice(invalid.Object, containers, "spec", "containers"))
		admit(t, pods, ns, invalid, "require-pod-probes")
		init["restartPolicy"] = "Always"
		sidecar := pod()
		require.NoError(t, unstructured.SetNestedSlice(sidecar.Object, []interface{}{init}, "spec", "initContainers"))
		admit(t, pods, ns, sidecar, "require-pod-probes")
		init["readinessProbe"] = map[string]interface{}{"exec": map[string]interface{}{"command": []interface{}{"true"}}}
		require.NoError(t, unstructured.SetNestedSlice(sidecar.Object, []interface{}{init}, "spec", "initContainers"))
		admit(t, pods, ns, sidecar, "")
		for _, tc := range []struct {
			name   string
			input  *unstructured.Unstructured
			denied string
		}{
			{"valid", valid, ""}, {"invalid", invalid, "require-pod-probes"},
		} {
			spec, _, _ := unstructured.NestedMap(tc.input.Object, "spec")
			spec["restartPolicy"] = "Never"
			cronjob := &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": "batch/v1", "kind": "CronJob", "metadata": map[string]interface{}{"name": "cron-" + tc.name},
				"spec": map[string]interface{}{"schedule": "0 * * * *", "jobTemplate": map[string]interface{}{"spec": map[string]interface{}{"template": map[string]interface{}{"spec": spec}}}},
			}}
			admit(t, schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}, ns, cronjob, tc.denied)
		}
	})

	t.Run("network policy selector semantics", func(t *testing.T) {
		apply(t, selectOnly("require-network-policy", StateEnforce))
		deployment := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "apps/v1", "kind": "Deployment", "metadata": map[string]interface{}{"name": "network-test"},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{"matchLabels": map[string]interface{}{"app": "web"}},
				"template": map[string]interface{}{"metadata": map[string]interface{}{"labels": map[string]interface{}{"app": "web"}}, "spec": pod().Object["spec"]},
			},
		}}
		deployments := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
		admit(t, deployments, ns, deployment, "require-network-policy")
		networks := client.Resource(schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}).Namespace(ns)
		for _, tc := range []struct {
			operator, key string
			values        []interface{}
			pass          bool
		}{
			{"In", "app", []interface{}{"other"}, false},
			{"In", "app", []interface{}{"web"}, true},
			{"NotIn", "app", []interface{}{"web"}, false},
			{"NotIn", "missing", []interface{}{"web"}, true},
			{"Exists", "app", nil, true},
			{"Exists", "missing", nil, false},
			{"DoesNotExist", "app", nil, false},
			{"DoesNotExist", "missing", nil, true},
		} {
			t.Run(tc.operator+"-"+tc.key, func(t *testing.T) {
				expression := map[string]interface{}{"key": tc.key, "operator": tc.operator}
				if tc.values != nil {
					expression["values"] = tc.values
				}
				policy := &unstructured.Unstructured{Object: map[string]interface{}{
					"apiVersion": "networking.k8s.io/v1", "kind": "NetworkPolicy", "metadata": map[string]interface{}{"name": "coverage"},
					"spec": map[string]interface{}{"podSelector": map[string]interface{}{"matchExpressions": []interface{}{expression}}, "policyTypes": []interface{}{"Ingress"}},
				}}
				_, err := networks.Create(ctx, policy, metav1.CreateOptions{})
				require.NoError(t, err)
				denied := "require-network-policy"
				if tc.pass {
					denied = ""
				}
				admit(t, deployments, ns, deployment, denied)
				require.NoError(t, networks.Delete(ctx, "coverage", metav1.DeleteOptions{}))
			})
		}
	})

	t.Run("state transitions and external ownership", func(t *testing.T) {
		apply(t, selectOnly("disallow-default-namespace", StateEnforce))
		admit(t, pods, "default", pod(), "disallow-default-namespace")
		apply(t, selectOnly("disallow-default-namespace", StateAudit))
		updated, err := policies.Get(ctx, "disallow-default-namespace", metav1.GetOptions{})
		require.NoError(t, err)
		actions, _, err := unstructured.NestedStringSlice(updated.Object, "spec", "validationActions")
		require.NoError(t, err)
		require.Equal(t, []string{"Audit"}, actions)
		admit(t, pods, "default", pod(), "")
		apply(t, disableAll(catalog))
		_, err = policies.Get(ctx, "disallow-default-namespace", metav1.GetOptions{})
		require.True(t, apierrors.IsNotFound(err))
		admit(t, pods, "default", pod(), "")
		policy, _ := catalog.Policy("disallow-default-namespace")
		external, err := renderPolicy(policy, StateAudit)
		require.NoError(t, err)
		external.SetLabels(map[string]string{ManagedByLabel: "external"})
		_, err = policies.Create(ctx, external, metav1.CreateOptions{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = policies.Delete(context.Background(), external.GetName(), metav1.DeleteOptions{}) })
		extra := external.DeepCopy()
		extra.SetName("external-extra")
		_, err = policies.Create(ctx, extra, metav1.CreateOptions{})
		require.NoError(t, err)
		t.Cleanup(func() { _ = policies.Delete(context.Background(), extra.GetName(), metav1.DeleteOptions{}) })
		apply(t, disableAll(catalog))
		_, err = reconciler.Reconcile(ctx, selectOnly("disallow-default-namespace", StateEnforce))
		require.ErrorContains(t, err, "not managed by foundry-cli")
		for _, name := range []string{external.GetName(), extra.GetName()} {
			object, err := policies.Get(ctx, name, metav1.GetOptions{})
			require.NoError(t, err)
			require.Equal(t, "external", object.GetLabels()[ManagedByLabel])
		}
	})

	t.Run("custom manifests replace the stored spec", func(t *testing.T) {
		policy, _ := catalog.Policy("require-labels")
		for _, expression := range []string{"false", "true"} {
			object, err := renderPolicy(policy, StateEnforce)
			require.NoError(t, err)
			require.NoError(t, unstructured.SetNestedSlice(object.Object, []interface{}{map[string]interface{}{
				"expression": expression, "message": "custom policy check",
			}}, "spec", "validations"))
			manifest, err := kyaml.Marshal(object.Object)
			require.NoError(t, err)
			selections := selectOnly(policy.Name, StateEnforce)
			for i := range selections {
				if selections[i].Name == policy.Name {
					selections[i].Manifest = string(manifest)
				}
			}
			apply(t, selections)
			denied := ""
			if expression == "false" {
				denied = policy.Name
			}
			admit(t, pods, ns, pod(), denied)
		}
	})

	t.Run("review regressions", func(t *testing.T) {
		testPolicyExpressionRegressions(t, ctx, client, ns, apply, selectOnly, admit)
	})
}
