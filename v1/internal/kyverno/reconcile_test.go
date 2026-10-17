package kyverno

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

func TestReconcilePreservesExternalPoliciesAndDeletesOwnedDisabledPolicies(t *testing.T) {
	catalog, err := LoadCatalog()
	require.NoError(t, err)

	ownedDisabled := policyObject("ValidatingPolicy", "require-network-policy", map[string]string{ManagedByLabel: ManagedByValue})
	externalExtra := policyObject("ValidatingPolicy", "external-policy", map[string]string{"owner": "operator"})
	client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), policyListKinds(), ownedDisabled, externalExtra)
	reconciler, err := NewReconciler(client, catalog)
	require.NoError(t, err)

	selections := disableAll(catalog)
	result, err := reconciler.Reconcile(context.Background(), selections)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Deleted)

	_, err = client.Resource(policyResources["ValidatingPolicy"]).Get(context.Background(), "require-network-policy", metav1.GetOptions{})
	assert.Error(t, err)
	extra, err := client.Resource(policyResources["ValidatingPolicy"]).Get(context.Background(), "external-policy", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "operator", extra.GetLabels()["owner"])
}

func TestReconcileRejectsAnUnownedNameCollision(t *testing.T) {
	catalog, err := LoadCatalog()
	require.NoError(t, err)
	external := policyObject("ValidatingPolicy", "disallow-default-namespace", nil)
	client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), policyListKinds(), external)
	reconciler, err := NewReconciler(client, catalog)
	require.NoError(t, err)

	selections := disableAll(catalog)
	for i := range selections {
		if selections[i].Name == "disallow-default-namespace" {
			selections[i].State = StateEnforce
		}
	}
	_, err = reconciler.Reconcile(context.Background(), selections)
	assert.ErrorContains(t, err, "is not managed by foundry-cli")
}

func TestReconcilePreservesDisabledPolicyWithAnExternalOwner(t *testing.T) {
	catalog, err := LoadCatalog()
	require.NoError(t, err)
	external := policyObject("ValidatingPolicy", "require-network-policy", map[string]string{"owner": "platform-team"})
	client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), policyListKinds(), external)
	reconciler, err := NewReconciler(client, catalog)
	require.NoError(t, err)

	result, err := reconciler.Reconcile(context.Background(), disableAll(catalog))
	require.NoError(t, err)
	assert.Zero(t, result.Deleted)

	object, err := client.Resource(policyResources["ValidatingPolicy"]).Get(context.Background(), "require-network-policy", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "platform-team", object.GetLabels()["owner"])
}

func TestReconcileAppliesOwnedPolicyWithRequestedAction(t *testing.T) {
	catalog, err := LoadCatalog()
	require.NoError(t, err)
	client := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), policyListKinds())
	reconciler, err := NewReconciler(client, catalog)
	require.NoError(t, err)

	selections := disableAll(catalog)
	for i := range selections {
		if selections[i].Name == "disallow-default-namespace" {
			selections[i].State = StateEnforce
		}
	}
	result, err := reconciler.Reconcile(context.Background(), selections)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Applied)

	object, err := client.Resource(policyResources["ValidatingPolicy"]).Get(context.Background(), "disallow-default-namespace", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, ManagedByValue, object.GetLabels()[ManagedByLabel])
	actions, found, err := unstructured.NestedStringSlice(object.Object, "spec", "validationActions")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, []string{"Deny"}, actions)

	// Updating an existing owned policy must replace, not append, actions.
	for i := range selections {
		if selections[i].Name == "disallow-default-namespace" {
			selections[i].State = StateAudit
		}
	}
	_, err = reconciler.Reconcile(context.Background(), selections)
	require.NoError(t, err)
	object, err = client.Resource(policyResources["ValidatingPolicy"]).Get(context.Background(), "disallow-default-namespace", metav1.GetOptions{})
	require.NoError(t, err)
	actions, _, err = unstructured.NestedStringSlice(object.Object, "spec", "validationActions")
	require.NoError(t, err)
	assert.Equal(t, []string{"Audit"}, actions)
}

func disableAll(catalog *Catalog) []Selection {
	selections := make([]Selection, 0, len(catalog.Policies))
	for _, policy := range catalog.Policies {
		selections = append(selections, Selection{Name: policy.Name, State: StateDisabled})
	}
	return selections
}

func policyObject(kind, name string, labels map[string]string) *unstructured.Unstructured {
	object := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "policies.kyverno.io/v1",
		"kind":       kind,
		"metadata":   map[string]interface{}{"name": name},
		"spec":       map[string]interface{}{"validationActions": []interface{}{"Audit"}},
	}}
	object.SetLabels(labels)
	return object
}

func policyListKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		policyResources["ValidatingPolicy"]:      "ValidatingPolicyList",
		policyResources["ImageValidatingPolicy"]: "ImageValidatingPolicyList",
	}
}
