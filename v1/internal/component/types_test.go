package component

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestComponentConfig_Get(t *testing.T) {
	cfg := ComponentConfig{
		"key1": "value1",
		"key2": 42,
	}

	val, ok := cfg.Get("key1")
	assert.True(t, ok)
	assert.Equal(t, "value1", val)

	val, ok = cfg.Get("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, val)
}

func TestComponentConfig_GetString(t *testing.T) {
	cfg := ComponentConfig{
		"str":   "hello",
		"int":   42,
		"bool":  true,
	}

	// Valid string
	val, ok := cfg.GetString("str")
	assert.True(t, ok)
	assert.Equal(t, "hello", val)

	// Non-existent key
	val, ok = cfg.GetString("nonexistent")
	assert.False(t, ok)
	assert.Equal(t, "", val)

	// Wrong type
	val, ok = cfg.GetString("int")
	assert.False(t, ok)
	assert.Equal(t, "", val)
}

func TestComponentConfig_GetInt(t *testing.T) {
	cfg := ComponentConfig{
		"int":     42,
		"float":   123.0,
		"string":  "hello",
	}

	// Valid int
	val, ok := cfg.GetInt("int")
	assert.True(t, ok)
	assert.Equal(t, 42, val)

	// Valid float64 (from JSON unmarshaling)
	val, ok = cfg.GetInt("float")
	assert.True(t, ok)
	assert.Equal(t, 123, val)

	// Non-existent key
	val, ok = cfg.GetInt("nonexistent")
	assert.False(t, ok)
	assert.Equal(t, 0, val)

	// Wrong type
	val, ok = cfg.GetInt("string")
	assert.False(t, ok)
	assert.Equal(t, 0, val)
}

func TestComponentConfig_GetBool(t *testing.T) {
	cfg := ComponentConfig{
		"bool":   true,
		"string": "hello",
	}

	// Valid bool
	val, ok := cfg.GetBool("bool")
	assert.True(t, ok)
	assert.True(t, val)

	// Non-existent key
	val, ok = cfg.GetBool("nonexistent")
	assert.False(t, ok)
	assert.False(t, val)

	// Wrong type
	val, ok = cfg.GetBool("string")
	assert.False(t, ok)
	assert.False(t, val)
}

func TestComponentConfig_GetMap(t *testing.T) {
	cfg := ComponentConfig{
		"map": map[string]interface{}{
			"nested": "value",
		},
		"string": "hello",
	}

	// Valid map
	val, ok := cfg.GetMap("map")
	assert.True(t, ok)
	assert.Equal(t, map[string]interface{}{"nested": "value"}, val)

	// Non-existent key
	val, ok = cfg.GetMap("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, val)

	// Wrong type
	val, ok = cfg.GetMap("string")
	assert.False(t, ok)
	assert.Nil(t, val)
}

func TestComponentConfig_GetStringSlice(t *testing.T) {
	cfg := ComponentConfig{
		"slice_interface": []interface{}{"a", "b", "c"},
		"slice_string":    []string{"x", "y", "z"},
		"string":          "hello",
		"mixed":           []interface{}{"a", 1, "c"}, // Invalid: mixed types
	}

	// Valid []interface{} (from JSON unmarshaling)
	val, ok := cfg.GetStringSlice("slice_interface")
	assert.True(t, ok)
	assert.Equal(t, []string{"a", "b", "c"}, val)

	// Valid []string
	val, ok = cfg.GetStringSlice("slice_string")
	assert.True(t, ok)
	assert.Equal(t, []string{"x", "y", "z"}, val)

	// Non-existent key
	val, ok = cfg.GetStringSlice("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, val)

	// Wrong type
	val, ok = cfg.GetStringSlice("string")
	assert.False(t, ok)
	assert.Nil(t, val)

	// Mixed types in slice
	val, ok = cfg.GetStringSlice("mixed")
	assert.False(t, ok)
	assert.Nil(t, val)
}
