package config

import (
	"fmt"
	"reflect"

	"github.com/catalystcommunity/foundry/v1/internal/secrets"
)

// ValidateSecretRefs validates that all secret references in the config have valid syntax
// This does NOT resolve the secrets, just validates the reference format
func ValidateSecretRefs(cfg *Config) error {
	return walkConfigValues(cfg, func(value string) error {
		if !secrets.IsSecretRef(value) {
			return nil // Not a secret ref, skip
		}

		// Try to parse it
		_, err := secrets.ParseSecretRef(value)
		if err != nil {
			return fmt.Errorf("invalid secret reference %q: %w", value, err)
		}

		return nil
	})
}

// ResolveSecrets resolves all secret references in the config
// Requires a ResolutionContext (instance must be provided)
// Replaces secret references with actual values in place
func ResolveSecrets(cfg *Config, ctx *secrets.ResolutionContext, resolver secrets.Resolver) error {
	if ctx == nil {
		return fmt.Errorf("resolution context is required")
	}

	if resolver == nil {
		return fmt.Errorf("resolver is required")
	}

	return walkConfigValuesWithReplace(cfg, func(value string) (string, error) {
		if !secrets.IsSecretRef(value) {
			return value, nil // Not a secret ref, return as-is
		}

		// Parse the reference
		ref, err := secrets.ParseSecretRef(value)
		if err != nil {
			return "", fmt.Errorf("invalid secret reference %q: %w", value, err)
		}

		// Resolve it
		resolved, err := resolver.Resolve(ctx, *ref)
		if err != nil {
			return "", fmt.Errorf("failed to resolve secret %s: %w", ref.String(), err)
		}

		return resolved, nil
	})
}

// walkConfigValues walks all string values in the config struct
func walkConfigValues(cfg *Config, fn func(string) error) error {
	return walkValue(reflect.ValueOf(cfg), fn)
}

// walkConfigValuesWithReplace walks all string values and replaces them
func walkConfigValuesWithReplace(cfg *Config, fn func(string) (string, error)) error {
	return walkValueWithReplace(reflect.ValueOf(cfg), fn)
}

// walkValue recursively walks a reflect.Value and calls fn for each string
func walkValue(v reflect.Value, fn func(string) error) error {
	if !v.IsValid() {
		return nil
	}

	// Dereference pointers
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		return walkValue(v.Elem(), fn)
	}

	switch v.Kind() {
	case reflect.String:
		return fn(v.String())

	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			if err := walkValue(field, fn); err != nil {
				return err
			}
		}

	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if err := walkValue(v.Index(i), fn); err != nil {
				return err
			}
		}

	case reflect.Map:
		iter := v.MapRange()
		for iter.Next() {
			if err := walkValue(iter.Value(), fn); err != nil {
				return err
			}
		}

	case reflect.Interface:
		if !v.IsNil() {
			return walkValue(v.Elem(), fn)
		}
	}

	return nil
}

// walkValueWithReplace recursively walks and replaces string values
func walkValueWithReplace(v reflect.Value, fn func(string) (string, error)) error {
	if !v.IsValid() {
		return nil
	}

	// Dereference pointers
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		return walkValueWithReplace(v.Elem(), fn)
	}

	switch v.Kind() {
	case reflect.String:
		if v.CanSet() {
			newValue, err := fn(v.String())
			if err != nil {
				return err
			}
			v.SetString(newValue)
		}

	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			if err := walkValueWithReplace(field, fn); err != nil {
				return err
			}
		}

	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if err := walkValueWithReplace(v.Index(i), fn); err != nil {
				return err
			}
		}

	case reflect.Map:
		// For maps with string values, we need to get/set since map values aren't addressable
		if v.Type().Elem().Kind() == reflect.String {
			iter := v.MapRange()
			for iter.Next() {
				key := iter.Key()
				oldValue := iter.Value().String()
				newValue, err := fn(oldValue)
				if err != nil {
					return err
				}
				if newValue != oldValue {
					v.SetMapIndex(key, reflect.ValueOf(newValue))
				}
			}
		} else {
			iter := v.MapRange()
			for iter.Next() {
				if err := walkValueWithReplace(iter.Value(), fn); err != nil {
					return err
				}
			}
		}

	case reflect.Interface:
		if !v.IsNil() {
			return walkValueWithReplace(v.Elem(), fn)
		}
	}

	return nil
}
