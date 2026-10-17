package kyverno

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kyaml "sigs.k8s.io/yaml"
)

const (
	ManagedByLabel = "app.kubernetes.io/managed-by"
	ManagedByValue = "foundry-cli"
)

//go:embed cel-policies/catalog.yaml cel-policies/validating/*.yaml cel-policies/image-validating/*.yaml
var policyFiles embed.FS

// State is the requested behavior for one bundled policy.
type State string

const (
	StateEnforce  State = "Enforce"
	StateAudit    State = "Audit"
	StateDisabled State = "Disabled"
)

// Selection is a policy name and its requested state.
type Selection struct {
	Name     string `json:"name" yaml:"name"`
	State    State  `json:"state" yaml:"state"`
	Manifest string `json:"manifest,omitempty" yaml:"manifest,omitempty"`
}

// Source identifies the upstream policy snapshot used by this catalog.
type Source struct {
	Repository string `json:"repository" yaml:"repository"`
	Commit     string `json:"commit" yaml:"commit"`
}

// Policy is one selectable policy and its bundled YAML manifest.
type Policy struct {
	Name                  string `json:"name" yaml:"name"`
	Title                 string `json:"title" yaml:"title"`
	Description           string `json:"description" yaml:"description"`
	DefaultState          State  `json:"default_state" yaml:"defaultState"`
	Kind                  string `json:"kind" yaml:"kind"`
	Path                  string `json:"-" yaml:"path"`
	SourcePath            string `json:"source_path,omitempty" yaml:"sourcePath,omitempty"`
	SourceURL             string `json:"source_url,omitempty" yaml:"-"`
	YAML                  string `json:"yaml" yaml:"-"`
	RequiresCustomization bool   `json:"requires_customization" yaml:"requiresCustomization,omitempty"`
}

// Catalog is the complete set of policies exposed by Foundry.
type Catalog struct {
	Version  int      `json:"version" yaml:"version"`
	Source   Source   `json:"source" yaml:"source"`
	Policies []Policy `json:"policies" yaml:"policies"`
}

// LoadCatalog reads and validates the embedded policy catalog.
func LoadCatalog() (*Catalog, error) {
	data, err := policyFiles.ReadFile("cel-policies/catalog.yaml")
	if err != nil {
		return nil, fmt.Errorf("read policy catalog: %w", err)
	}
	var catalog Catalog
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("parse policy catalog: %w", err)
	}
	if err := catalog.loadManifests(policyFiles); err != nil {
		return nil, err
	}
	if err := catalog.Validate(); err != nil {
		return nil, err
	}
	return &catalog, nil
}

func (c *Catalog) loadManifests(files fs.FS) error {
	for i := range c.Policies {
		policy := &c.Policies[i]
		manifestPath := path.Join("cel-policies", policy.Path)
		data, err := fs.ReadFile(files, manifestPath)
		if err != nil {
			return fmt.Errorf("read policy %q manifest: %w", policy.Name, err)
		}
		policy.YAML = string(data)
		if policy.SourcePath != "" {
			policy.SourceURL = strings.TrimSuffix(c.Source.Repository, "/") + "/blob/" + c.Source.Commit + "/" + policy.SourcePath
		}
	}
	return nil
}

// Validate checks catalog metadata and every bundled Kubernetes object.
func (c *Catalog) Validate() error {
	if c.Version < 1 {
		return fmt.Errorf("policy catalog version must be positive")
	}
	seen := make(map[string]struct{}, len(c.Policies))
	for _, policy := range c.Policies {
		if strings.TrimSpace(policy.Name) == "" {
			return fmt.Errorf("policy name cannot be empty")
		}
		if _, ok := seen[policy.Name]; ok {
			return fmt.Errorf("policy %q appears more than once", policy.Name)
		}
		seen[policy.Name] = struct{}{}
		if _, err := NormalizeState(policy.DefaultState); err != nil {
			return fmt.Errorf("policy %q default state: %w", policy.Name, err)
		}
		if policy.Kind != "ValidatingPolicy" && policy.Kind != "ImageValidatingPolicy" {
			return fmt.Errorf("policy %q has unsupported kind %q", policy.Name, policy.Kind)
		}
		var object unstructured.Unstructured
		if err := kyaml.Unmarshal([]byte(policy.YAML), &object.Object); err != nil {
			return fmt.Errorf("parse policy %q manifest: %w", policy.Name, err)
		}
		if object.GetAPIVersion() != "policies.kyverno.io/v1" {
			return fmt.Errorf("policy %q must use policies.kyverno.io/v1", policy.Name)
		}
		if object.GetKind() != policy.Kind {
			return fmt.Errorf("policy %q manifest kind %q does not match catalog kind %q", policy.Name, object.GetKind(), policy.Kind)
		}
		if object.GetName() != policy.Name {
			return fmt.Errorf("policy %q manifest name is %q", policy.Name, object.GetName())
		}
	}
	return nil
}

// NormalizeState returns the canonical spelling of a policy state.
func NormalizeState(state State) (State, error) {
	switch strings.ToLower(strings.TrimSpace(string(state))) {
	case "enforce", "enabled":
		return StateEnforce, nil
	case "audit":
		return StateAudit, nil
	case "disabled", "disable":
		return StateDisabled, nil
	default:
		return "", fmt.Errorf("state must be Enforce, Audit, or Disabled")
	}
}

// ResolveSelections applies user overrides to the catalog defaults.
func (c *Catalog) ResolveSelections(configured []Selection) ([]Selection, error) {
	resolved := make(map[string]Selection, len(c.Policies))
	for _, policy := range c.Policies {
		resolved[policy.Name] = Selection{Name: policy.Name, State: policy.DefaultState}
	}

	seen := make(map[string]struct{}, len(configured))
	for _, selection := range configured {
		if _, ok := resolved[selection.Name]; !ok {
			return nil, fmt.Errorf("policy %q is not in the Foundry catalog", selection.Name)
		}
		if _, ok := seen[selection.Name]; ok {
			return nil, fmt.Errorf("policy %q appears more than once", selection.Name)
		}
		seen[selection.Name] = struct{}{}
		state, err := NormalizeState(selection.State)
		if err != nil {
			return nil, fmt.Errorf("policy %q: %w", selection.Name, err)
		}
		selection.State = state
		resolved[selection.Name] = selection
	}

	selections := make([]Selection, 0, len(c.Policies))
	for _, policy := range c.Policies {
		selection := resolved[policy.Name]
		if _, err := selectedPolicy(policy, selection); err != nil {
			return nil, err
		}
		selections = append(selections, selection)
	}
	return selections, nil
}

// Policy returns one catalog entry by name.
func (c *Catalog) Policy(name string) (Policy, bool) {
	for _, policy := range c.Policies {
		if policy.Name == name {
			return policy, true
		}
	}
	return Policy{}, false
}

// Names returns sorted catalog policy names.
func (c *Catalog) Names() []string {
	names := make([]string, 0, len(c.Policies))
	for _, policy := range c.Policies {
		names = append(names, policy.Name)
	}
	sort.Strings(names)
	return names
}
