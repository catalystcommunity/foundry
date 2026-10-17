package kyverno

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/retry"
	kyaml "sigs.k8s.io/yaml"
)

var policyResources = map[string]schema.GroupVersionResource{
	"ValidatingPolicy": {
		Group: "policies.kyverno.io", Version: "v1", Resource: "validatingpolicies",
	},
	"ImageValidatingPolicy": {
		Group: "policies.kyverno.io", Version: "v1", Resource: "imagevalidatingpolicies",
	},
}

// ReconcileResult reports policy changes made in one reconciliation.
type ReconcileResult struct {
	Applied int
	Deleted int
	Skipped int
}

// Reconciler converges Foundry-owned policies and leaves every other policy alone.
type Reconciler struct {
	client  dynamic.Interface
	catalog *Catalog
}

// NewReconciler constructs a policy reconciler.
func NewReconciler(client dynamic.Interface, catalog *Catalog) (*Reconciler, error) {
	if client == nil {
		return nil, fmt.Errorf("dynamic Kubernetes client is required")
	}
	if catalog == nil {
		return nil, fmt.Errorf("policy catalog is required")
	}
	if err := catalog.Validate(); err != nil {
		return nil, err
	}
	return &Reconciler{client: client, catalog: catalog}, nil
}

// Reconcile applies enabled policies and removes only disabled policies that Foundry owns.
func (r *Reconciler) Reconcile(ctx context.Context, configured []Selection) (ReconcileResult, error) {
	selections, err := r.catalog.ResolveSelections(configured)
	if err != nil {
		return ReconcileResult{}, err
	}

	var result ReconcileResult
	for _, selection := range selections {
		policy, _ := r.catalog.Policy(selection.Name)
		policy, err = selectedPolicy(policy, selection)
		if err != nil {
			return result, err
		}
		gvr := policyResources[policy.Kind]
		resource := r.client.Resource(gvr)
		existing, getErr := resource.Get(ctx, policy.Name, metav1.GetOptions{})
		if getErr != nil && !apierrors.IsNotFound(getErr) {
			return result, fmt.Errorf("get %s %q: %w", policy.Kind, policy.Name, getErr)
		}
		missing := apierrors.IsNotFound(getErr)

		if selection.State == StateDisabled {
			if apierrors.IsNotFound(getErr) || existing == nil {
				result.Skipped++
				continue
			}
			if existing.GetLabels()[ManagedByLabel] != ManagedByValue {
				result.Skipped++
				continue
			}
			uid, version := existing.GetUID(), existing.GetResourceVersion()
			if err := resource.Delete(ctx, policy.Name, metav1.DeleteOptions{
				Preconditions: &metav1.Preconditions{UID: &uid, ResourceVersion: &version},
			}); err != nil && !apierrors.IsNotFound(err) {
				return result, fmt.Errorf("delete %s %q: %w", policy.Kind, policy.Name, err)
			}
			result.Deleted++
			continue
		}

		if !missing && existing != nil && existing.GetLabels()[ManagedByLabel] != ManagedByValue {
			return result, fmt.Errorf("%s %q already exists and is not managed by %s", policy.Kind, policy.Name, ManagedByValue)
		}

		object, err := renderPolicy(policy, selection.State)
		if err != nil {
			return result, err
		}
		if missing {
			if _, err := resource.Create(ctx, object, metav1.CreateOptions{FieldManager: ManagedByValue}); err != nil {
				return result, fmt.Errorf("create %s %q: %w", policy.Kind, policy.Name, err)
			}
		} else {
			// Replace the spec rather than applying a set-valued list. SSA can
			// retain Deny when the new selection is Audit after an initial Create.
			// Recheck ownership on every conflict retry and preserve other metadata.
			err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
				current, err := resource.Get(ctx, policy.Name, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if current.GetLabels()[ManagedByLabel] != ManagedByValue {
					return fmt.Errorf("%s %q is no longer managed by %s", policy.Kind, policy.Name, ManagedByValue)
				}
				current.Object["spec"] = object.Object["spec"]
				labels := current.GetLabels()
				for key, value := range object.GetLabels() {
					labels[key] = value
				}
				current.SetLabels(labels)
				annotations := current.GetAnnotations()
				if annotations == nil {
					annotations = make(map[string]string)
				}
				for key, value := range object.GetAnnotations() {
					annotations[key] = value
				}
				current.SetAnnotations(annotations)
				_, err = resource.Update(ctx, current, metav1.UpdateOptions{FieldManager: ManagedByValue})
				return err
			})
			if err != nil {
				return result, fmt.Errorf("apply %s %q: %w", policy.Kind, policy.Name, err)
			}
		}
		result.Applied++
	}
	return result, nil
}

func renderPolicy(policy Policy, state State) (*unstructured.Unstructured, error) {
	state, err := NormalizeState(state)
	if err != nil {
		return nil, fmt.Errorf("policy %q: %w", policy.Name, err)
	}
	if state == StateDisabled {
		return nil, fmt.Errorf("policy %q cannot render the Disabled state", policy.Name)
	}

	object := &unstructured.Unstructured{}
	if err := kyaml.Unmarshal([]byte(policy.YAML), &object.Object); err != nil {
		return nil, fmt.Errorf("parse policy %q: %w", policy.Name, err)
	}
	labels := object.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[ManagedByLabel] = ManagedByValue
	object.SetLabels(labels)

	action := "Audit"
	if state == StateEnforce {
		action = "Deny"
	}
	if err := unstructured.SetNestedStringSlice(object.Object, []string{action}, "spec", "validationActions"); err != nil {
		return nil, fmt.Errorf("set policy %q validation action: %w", policy.Name, err)
	}
	return object, nil
}
