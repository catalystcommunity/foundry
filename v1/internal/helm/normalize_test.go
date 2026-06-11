package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNormalizeValues_ConcreteSliceBecomesGeneric verifies that a Go-native
// []map[string]interface{} is converted to the generic []interface{} that Helm's
// template engine expects. Charts that branch on `typeIs "[]interface {}"`
// (e.g. Velero's BackupStorageLocation) silently fail to render without this.
func TestNormalizeValues_ConcreteSliceBecomesGeneric(t *testing.T) {
	in := map[string]interface{}{
		"configuration": map[string]interface{}{
			"backupStorageLocation": []map[string]interface{}{
				{"name": "default", "provider": "aws", "bucket": "velero"},
			},
		},
	}

	out, err := normalizeValues(in)
	require.NoError(t, err)

	cfg, ok := out["configuration"].(map[string]interface{})
	require.True(t, ok, "configuration should be a map[string]interface{}")

	bsl := cfg["backupStorageLocation"]
	_, isGeneric := bsl.([]interface{})
	assert.True(t, isGeneric, "backupStorageLocation should normalize to []interface{}, got %T", bsl)

	list := bsl.([]interface{})
	require.Len(t, list, 1)
	item, ok := list[0].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "default", item["name"])
	assert.Equal(t, "velero", item["bucket"])
}

func TestNormalizeValues_EmptyAndNil(t *testing.T) {
	out, err := normalizeValues(nil)
	require.NoError(t, err)
	assert.Nil(t, out)

	out, err = normalizeValues(map[string]interface{}{})
	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestNormalizeValues_PreservesNestedScalars(t *testing.T) {
	in := map[string]interface{}{
		"enabled":  true,
		"replicas": 3,
		"nested": map[string]interface{}{
			"list": []string{"a", "b"},
		},
	}
	out, err := normalizeValues(in)
	require.NoError(t, err)
	assert.Equal(t, true, out["enabled"])

	nested := out["nested"].(map[string]interface{})
	_, isGeneric := nested["list"].([]interface{})
	assert.True(t, isGeneric, "[]string should normalize to []interface{}, got %T", nested["list"])
}
