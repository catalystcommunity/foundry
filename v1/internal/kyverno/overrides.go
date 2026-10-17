package kyverno

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kyaml "sigs.k8s.io/yaml"
)

// selectedPolicy validates an override before any cluster changes are made.
// State and ownership remain under Foundry control, including for overrides.
func selectedPolicy(policy Policy, selection Selection) (Policy, error) {
	if strings.TrimSpace(selection.Manifest) == "" {
		if policy.RequiresCustomization && selection.State != StateDisabled {
			return policy, fmt.Errorf("policy %q requires a custom manifest before it can be enabled", policy.Name)
		}
		return policy, nil
	}
	decoder := yaml.NewDecoder(strings.NewReader(selection.Manifest))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return policy, fmt.Errorf("policy %q manifest: %w", policy.Name, err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return policy, fmt.Errorf("policy %q manifest must contain exactly one YAML document", policy.Name)
	}
	object := &unstructured.Unstructured{}
	if err := kyaml.UnmarshalStrict([]byte(selection.Manifest), &object.Object); err != nil {
		return policy, fmt.Errorf("policy %q manifest: %w", policy.Name, err)
	}
	if object.GetAPIVersion() != "policies.kyverno.io/v1" || object.GetKind() != policy.Kind || object.GetName() != policy.Name || object.GetNamespace() != "" {
		return policy, fmt.Errorf("policy %q manifest must keep its catalog apiVersion, kind, name, and cluster scope", policy.Name)
	}
	spec, found, err := unstructured.NestedMap(object.Object, "spec")
	if err != nil || !found || len(spec) == 0 {
		return policy, fmt.Errorf("policy %q manifest requires a non-empty spec", policy.Name)
	}
	if policy.RequiresCustomization && selection.State != StateDisabled {
		if err := validateCustomization(policy, spec); err != nil {
			return policy, err
		}
	}
	// Ignore API-server fields copied from a live object.
	clean := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": object.GetAPIVersion(), "kind": object.GetKind(),
		"metadata": map[string]interface{}{"name": object.GetName()}, "spec": spec,
	}}
	clean.SetLabels(object.GetLabels())
	clean.SetAnnotations(object.GetAnnotations())
	data, err := kyaml.Marshal(clean.Object)
	if err != nil {
		return policy, err
	}
	policy.YAML = string(data)
	return policy, nil
}

func validateCustomization(policy Policy, spec map[string]interface{}) error {
	data, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	for _, example := range []string{"eu.foo.io/", "bar.io/", "ghcr.io/kyverno/test-verify-image"} {
		if strings.Contains(string(data), example) {
			return fmt.Errorf("policy %q must replace the example setting %q", policy.Name, example)
		}
	}
	if policy.Name == "verify-image" {
		var bundled map[string]interface{}
		if err := kyaml.Unmarshal([]byte(policy.YAML), &bundled); err != nil {
			return err
		}
		attestors, _, _ := unstructured.NestedSlice(bundled, "spec", "attestors")
		refs, refsOK := spec["matchImageReferences"].([]interface{})
		custom, customOK := spec["attestors"].([]interface{})
		if !refsOK || len(refs) == 0 || !customOK || len(custom) == 0 || reflect.DeepEqual(custom, attestors) {
			return fmt.Errorf("policy %q requires custom image references and trust settings", policy.Name)
		}
	}
	return nil
}
