package secrets

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFoundryVarsResolver(t *testing.T) {
	fixturesDir, err := filepath.Abs("../../test/fixtures")
	require.NoError(t, err)

	t.Run("load valid foundryvars file", func(t *testing.T) {
		path := filepath.Join(fixturesDir, "test-foundryvars")
		resolver, err := NewFoundryVarsResolver(path)
		require.NoError(t, err)
		require.NotNil(t, resolver)
		assert.NotEmpty(t, resolver.vars)

		// Check that some known values were loaded
		assert.Contains(t, resolver.vars, "myapp-prod/database/main:password")
		assert.Equal(t, "prod-secret-123", resolver.vars["myapp-prod/database/main:password"])
	})

	t.Run("non-existent file returns empty resolver", func(t *testing.T) {
		path := filepath.Join(fixturesDir, "does-not-exist")
		resolver, err := NewFoundryVarsResolver(path)
		require.NoError(t, err)
		require.NotNil(t, resolver)
		assert.Empty(t, resolver.vars)
	})

	t.Run("invalid format returns error", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "invalid-foundryvars-*")
		require.NoError(t, err)
		tmpPath := tmpFile.Name()
		defer os.Remove(tmpPath)

		// Write invalid content
		_, err = tmpFile.WriteString("invalid line without equals sign\n")
		require.NoError(t, err)
		tmpFile.Close()

		_, err = NewFoundryVarsResolver(tmpPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid format")
	})

	t.Run("empty key returns error", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "empty-key-foundryvars-*")
		require.NoError(t, err)
		tmpPath := tmpFile.Name()
		defer os.Remove(tmpPath)

		// Write line with empty key
		_, err = tmpFile.WriteString("=value\n")
		require.NoError(t, err)
		tmpFile.Close()

		_, err = NewFoundryVarsResolver(tmpPath)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty key")
	})

	t.Run("comments and empty lines ignored", func(t *testing.T) {
		tmpFile, err := os.CreateTemp("", "comments-foundryvars-*")
		require.NoError(t, err)
		tmpPath := tmpFile.Name()
		defer os.Remove(tmpPath)

		content := `# Comment line
key1:value1=test1

# Another comment
key2:value2=test2
`
		_, err = tmpFile.WriteString(content)
		require.NoError(t, err)
		tmpFile.Close()

		resolver, err := NewFoundryVarsResolver(tmpPath)
		require.NoError(t, err)
		assert.Len(t, resolver.vars, 2)
		assert.Equal(t, "test1", resolver.vars["key1:value1"])
		assert.Equal(t, "test2", resolver.vars["key2:value2"])
	})
}

func TestFoundryVarsResolver_Resolve(t *testing.T) {
	fixturesDir, err := filepath.Abs("../../test/fixtures")
	require.NoError(t, err)

	path := filepath.Join(fixturesDir, "test-foundryvars")
	resolver, err := NewFoundryVarsResolver(path)
	require.NoError(t, err)

	tests := []struct {
		name    string
		ctx     *ResolutionContext
		ref     SecretRef
		want    string
		wantErr bool
		errMsg  string
	}{
		{
			name: "resolve myapp-prod password",
			ctx:  NewResolutionContext("myapp-prod"),
			ref:  SecretRef{Path: "database/main", Key: "password"},
			want: "prod-secret-123",
		},
		{
			name: "resolve myapp-stable password",
			ctx:  NewResolutionContext("myapp-stable"),
			ref:  SecretRef{Path: "database/main", Key: "password"},
			want: "stable-secret-456",
		},
		{
			name: "resolve foundry-core openbao token",
			ctx:  NewResolutionContext("foundry-core"),
			ref:  SecretRef{Path: "openbao", Key: "token"},
			want: "root-token",
		},
		{
			name:    "secret not found",
			ctx:     NewResolutionContext("myapp-prod"),
			ref:     SecretRef{Path: "nonexistent", Key: "key"},
			wantErr: true,
			errMsg:  "secret not found in foundryvars",
		},
		{
			name:    "nil context",
			ctx:     nil,
			ref:     SecretRef{Path: "database/main", Key: "password"},
			wantErr: true,
			errMsg:  "resolution context is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolver.Resolve(tt.ctx, tt.ref)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestFoundryVarsResolver_MultipleInstances(t *testing.T) {
	fixturesDir, err := filepath.Abs("../../test/fixtures")
	require.NoError(t, err)

	path := filepath.Join(fixturesDir, "test-foundryvars")
	resolver, err := NewFoundryVarsResolver(path)
	require.NoError(t, err)

	// Same secret path/key but different instances should resolve to different values
	ref := SecretRef{Path: "database/main", Key: "password"}

	prodValue, err := resolver.Resolve(NewResolutionContext("myapp-prod"), ref)
	require.NoError(t, err)

	stableValue, err := resolver.Resolve(NewResolutionContext("myapp-stable"), ref)
	require.NoError(t, err)

	assert.NotEqual(t, prodValue, stableValue)
	assert.Equal(t, "prod-secret-123", prodValue)
	assert.Equal(t, "stable-secret-456", stableValue)
}
