package kyverno

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	kyaml "sigs.k8s.io/yaml"
)

func testDefaultNamespaceAdmission(t *testing.T, ctx context.Context, client dynamic.Interface, restConfig *rest.Config, ns string,
	apply func(*testing.T, []Selection), selectOnly func(string, State) []Selection,
	admit func(*testing.T, schema.GroupVersionResource, string, *unstructured.Unstructured, string)) {
	const policy = "disallow-default-namespace"
	object := func(yaml string) *unstructured.Unstructured {
		t.Helper()
		var fields map[string]interface{}
		require.NoError(t, kyaml.Unmarshal([]byte(yaml), &fields))
		return &unstructured.Unstructured{Object: fields}
	}
	cm := schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	apply(t, selectOnly(policy, StateDisabled))
	// Seed a legacy resource to verify UPDATE and cleanup after enforcement.
	legacy, err := client.Resource(cm).Namespace("default").Create(ctx, object("apiVersion: v1\nkind: ConfigMap\nmetadata: {generateName: foundry-legacy-}\ndata: {test: old}"), metav1.CreateOptions{})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Resource(cm).Namespace("default").Delete(context.Background(), legacy.GetName(), metav1.DeleteOptions{}))
	})
	crds := schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}
	crd := object(`apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata: {name: namespacechecks.foundry.test}
spec:
 group: foundry.test
 scope: Namespaced
 names: {kind: NamespaceCheck, plural: namespacechecks}
 versions:
 - name: v1
   served: true
   storage: true
   schema:
    openAPIV3Schema: {type: object}
`)
	_, err = client.Resource(crds).Create(ctx, crd, metav1.CreateOptions{})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, client.Resource(crds).Delete(context.Background(), crd.GetName(), metav1.DeleteOptions{}))
	})
	require.Eventually(t, func() bool {
		got, err := client.Resource(crds).Get(ctx, crd.GetName(), metav1.GetOptions{})
		if err != nil {
			return false
		}
		conditions, _, _ := unstructured.NestedSlice(got.Object, "status", "conditions")
		for _, condition := range conditions {
			c := condition.(map[string]interface{})
			if c["type"] == "Established" && c["status"] == "True" {
				return true
			}
		}
		return false
	}, 30*time.Second, 250*time.Millisecond)
	apply(t, selectOnly(policy, StateEnforce))
	for _, tc := range []struct{ group, version, resource, yaml string }{
		{"", "v1", "configmaps", "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: namespace-check}\ndata: {test: value}"},
		{"", "v1", "secrets", "apiVersion: v1\nkind: Secret\nmetadata: {name: namespace-check}\ntype: Opaque"},
		{"", "v1", "services", "apiVersion: v1\nkind: Service\nmetadata: {name: namespace-check}\nspec: {ports: [{port: 80}]}"},
		{"", "v1", "serviceaccounts", "apiVersion: v1\nkind: ServiceAccount\nmetadata: {name: namespace-check}"},
		{"apps", "v1", "deployments", "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: namespace-check}\nspec: {replicas: 0, selector: {matchLabels: {app: check}}, template: {metadata: {labels: {app: check}}, spec: {containers: [{name: app, image: busybox:1.36}]}}}"},
		{"apps", "v1", "replicasets", "apiVersion: apps/v1\nkind: ReplicaSet\nmetadata: {name: namespace-check}\nspec: {replicas: 0, selector: {matchLabels: {app: check}}, template: {metadata: {labels: {app: check}}, spec: {containers: [{name: app, image: busybox:1.36}]}}}"},
		{"rbac.authorization.k8s.io", "v1", "roles", "apiVersion: rbac.authorization.k8s.io/v1\nkind: Role\nmetadata: {name: namespace-check}\nrules: []"},
		{"networking.k8s.io", "v1", "networkpolicies", "apiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\nmetadata: {name: namespace-check}\nspec: {podSelector: {}, policyTypes: [Ingress]}"},
		{"foundry.test", "v1", "namespacechecks", "apiVersion: foundry.test/v1\nkind: NamespaceCheck\nmetadata: {name: namespace-check}"},
	} {
		t.Run(tc.resource, func(t *testing.T) {
			gvr := schema.GroupVersionResource{Group: tc.group, Version: tc.version, Resource: tc.resource}
			admit(t, gvr, "default", object(tc.yaml), policy)
			admit(t, gvr, ns, object(tc.yaml), "")
		})
	}
	t.Run("generated names", func(t *testing.T) {
		admit(t, cm, "default", object("apiVersion: v1\nkind: ConfigMap\nmetadata: {generateName: namespace-check-}"), policy)
	})
	t.Run("legacy update denied", func(t *testing.T) {
		require.NoError(t, unstructured.SetNestedField(legacy.Object, "changed", "data", "test"))
		_, err := client.Resource(cm).Namespace("default").Update(ctx, legacy, metav1.UpdateOptions{DryRun: []string{metav1.DryRunAll}})
		require.ErrorContains(t, err, policy)
	})
	// Impersonation isolates policy checks from RBAC. The privileged group does
	// not bypass admission, and is used only in this disposable test cluster.
	actorClient := func(actor string) dynamic.Interface {
		cfg := rest.CopyConfig(restConfig)
		cfg.Impersonate = rest.ImpersonationConfig{UserName: actor, Groups: []string{"system:masters"}}
		c, err := dynamic.NewForConfig(cfg)
		require.NoError(t, err)
		return c
	}
	for _, tc := range []struct{ group, resource, name, actor string }{
		{"", "services", "kubernetes", "system:apiserver"},
		{"", "endpoints", "kubernetes", "system:apiserver"},
		{"discovery.k8s.io", "endpointslices", "kubernetes", "system:apiserver"},
		{"", "serviceaccounts", "default", "system:serviceaccount:kube-system:service-account-controller"},
		{"", "configmaps", "kube-root-ca.crt", "system:serviceaccount:kube-system:root-ca-cert-publisher"},
	} {
		t.Run("builtin "+tc.resource, func(t *testing.T) {
			gvr := schema.GroupVersionResource{Group: tc.group, Version: "v1", Resource: tc.resource}
			current, err := client.Resource(gvr).Namespace("default").Get(ctx, tc.name, metav1.GetOptions{})
			require.NoError(t, err)
			_, err = actorClient(tc.actor).Resource(gvr).Namespace("default").Update(ctx, current, metav1.UpdateOptions{DryRun: []string{metav1.DryRunAll}})
			require.NoError(t, err)
			_, err = client.Resource(gvr).Namespace("default").Update(ctx, current, metav1.UpdateOptions{DryRun: []string{metav1.DryRunAll}})
			require.ErrorContains(t, err, policy, "a user must not bypass the policy with a built-in name")
		})
	}
	for _, tc := range []struct{ resource, name string }{{"serviceaccounts", "default"}, {"configmaps", "kube-root-ca.crt"}} {
		t.Run("controller recreates "+tc.name, func(t *testing.T) {
			resource := client.Resource(schema.GroupVersionResource{Version: "v1", Resource: tc.resource}).Namespace("default")
			require.NoError(t, resource.Delete(ctx, tc.name, metav1.DeleteOptions{}))
			require.Eventually(t, func() bool { _, err := resource.Get(ctx, tc.name, metav1.GetOptions{}); return err == nil }, 45*time.Second, 500*time.Millisecond)
		})
	}
	t.Run("controller cannot create applications", func(t *testing.T) {
		_, err := actorClient("system:kube-controller-manager").Resource(cm).Namespace("default").Create(ctx, object("apiVersion: v1\nkind: ConfigMap\nmetadata: {name: not-a-builtin}"), metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}})
		require.ErrorContains(t, err, policy)
	})
	t.Run("events still work", func(t *testing.T) {
		admit(t, schema.GroupVersionResource{Version: "v1", Resource: "events"}, "default", object("apiVersion: v1\nkind: Event\nmetadata: {name: namespace-check}\ninvolvedObject: {apiVersion: v1, kind: ConfigMap, name: namespace-check, namespace: default}\nreason: NamespaceCheck\nmessage: test\ntype: Normal"), "")
	})
	t.Run("cluster resources unaffected", func(t *testing.T) {
		admit(t, schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}, "", object("apiVersion: v1\nkind: Namespace\nmetadata: {name: foundry-namespace-dryrun}"), "")
	})
	// A DELETE must remain possible so old resources can be removed.
	err = client.Resource(cm).Namespace("default").Delete(ctx, legacy.GetName(), metav1.DeleteOptions{DryRun: []string{metav1.DryRunAll}})
	require.NoError(t, err)
	// Verify that disabled policy state restores CREATE behavior.
	apply(t, selectOnly(policy, StateDisabled))
	admit(t, cm, "default", object("apiVersion: v1\nkind: ConfigMap\nmetadata: {name: disabled-check}"), "")
}
