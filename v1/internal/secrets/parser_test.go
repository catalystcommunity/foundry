package secrets

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsSecretRef(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "valid secret ref",
			input: "${secret:database/prod:password}",
			want:  true,
		},
		{
			name:  "valid secret ref with nested path",
			input: "${secret:path/to/secret:key}",
			want:  true,
		},
		{
			name:  "valid secret ref with underscores",
			input: "${secret:my_service/my_secret:my_key}",
			want:  true,
		},
		{
			name:  "plain text",
			input: "just a regular string",
			want:  false,
		},
		{
			name:  "partial prefix",
			input: "${secret:incomplete",
			want:  false,
		},
		{
			name:  "wrong prefix",
			input: "${env:VAR_NAME}",
			want:  false,
		},
		{
			name:  "empty string",
			input: "",
			want:  false,
		},
		{
			name:  "whitespace",
			input: "   ",
			want:  false,
		},
		{
			name:  "secret ref with whitespace",
			input: "  ${secret:path:key}  ",
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSecretRef(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseSecretRef(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    *SecretRef
		wantErr bool
		errMsg  string
	}{
		{
			name:  "valid simple secret",
			input: "${secret:database/prod:password}",
			want: &SecretRef{
				Path: "database/prod",
				Key:  "password",
				Raw:  "${secret:database/prod:password}",
			},
			wantErr: false,
		},
		{
			name:  "valid nested path",
			input: "${secret:path/to/deep/secret:key}",
			want: &SecretRef{
				Path: "path/to/deep/secret",
				Key:  "key",
				Raw:  "${secret:path/to/deep/secret:key}",
			},
			wantErr: false,
		},
		{
			name:  "valid with underscores and dashes",
			input: "${secret:my-service/my_secret:api_key}",
			want: &SecretRef{
				Path: "my-service/my_secret",
				Key:  "api_key",
				Raw:  "${secret:my-service/my_secret:api_key}",
			},
			wantErr: false,
		},
		{
			name:  "valid with numbers",
			input: "${secret:service123/secret456:key789}",
			want: &SecretRef{
				Path: "service123/secret456",
				Key:  "key789",
				Raw:  "${secret:service123/secret456:key789}",
			},
			wantErr: false,
		},
		{
			name:  "valid single level path",
			input: "${secret:simple:key}",
			want: &SecretRef{
				Path: "simple",
				Key:  "key",
				Raw:  "${secret:simple:key}",
			},
			wantErr: false,
		},
		{
			name:    "plain text returns nil",
			input:   "just plain text",
			want:    nil,
			wantErr: false,
		},
		{
			name:    "empty string returns nil",
			input:   "",
			want:    nil,
			wantErr: false,
		},
		{
			name:    "missing path",
			input:   "${secret::key}",
			wantErr: true,
			errMsg:  "invalid secret reference format",
		},
		{
			name:    "missing key",
			input:   "${secret:path:}",
			wantErr: true,
			errMsg:  "invalid secret reference format",
		},
		{
			name:    "missing both path and key",
			input:   "${secret::}",
			wantErr: true,
			errMsg:  "invalid secret reference format",
		},
		{
			name:    "only one colon",
			input:   "${secret:pathonly}",
			wantErr: true,
			errMsg:  "invalid secret reference format",
		},
		{
			name:    "three colons",
			input:   "${secret:path:key:extra}",
			wantErr: true,
			errMsg:  "invalid secret reference format",
		},
		{
			name:    "invalid characters in path",
			input:   "${secret:path@invalid:key}",
			wantErr: true,
			errMsg:  "invalid secret reference format",
		},
		{
			name:    "invalid characters in key",
			input:   "${secret:path:key!invalid}",
			wantErr: true,
			errMsg:  "invalid secret reference format",
		},
		{
			name:    "spaces in path",
			input:   "${secret:path with spaces:key}",
			wantErr: true,
			errMsg:  "invalid secret reference format",
		},
		{
			name:  "whitespace around valid ref",
			input: "  ${secret:path:key}  ",
			want: &SecretRef{
				Path: "path",
				Key:  "key",
				Raw:  "${secret:path:key}",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSecretRef(tt.input)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
				assert.Nil(t, got)
			} else {
				require.NoError(t, err)
				if tt.want == nil {
					assert.Nil(t, got)
				} else {
					require.NotNil(t, got)
					assert.Equal(t, tt.want.Path, got.Path)
					assert.Equal(t, tt.want.Key, got.Key)
					assert.Equal(t, tt.want.Raw, got.Raw)
				}
			}
		})
	}
}

func TestSecretRef_String(t *testing.T) {
	tests := []struct {
		name string
		ref  SecretRef
		want string
	}{
		{
			name: "with raw string",
			ref: SecretRef{
				Path: "database/prod",
				Key:  "password",
				Raw:  "${secret:database/prod:password}",
			},
			want: "${secret:database/prod:password}",
		},
		{
			name: "without raw string",
			ref: SecretRef{
				Path: "database/prod",
				Key:  "password",
			},
			want: "${secret:database/prod:password}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.ref.String()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseSecretRef_RealWorldExamples(t *testing.T) {
	// Test with examples from the design document
	examples := []struct {
		input    string
		wantPath string
		wantKey  string
	}{
		{
			input:    "${secret:database/prod:password}",
			wantPath: "database/prod",
			wantKey:  "password",
		},
		{
			input:    "${secret:external/github:api_token}",
			wantPath: "external/github",
			wantKey:  "api_token",
		},
		{
			input:    "${secret:certs/wildcard:cert}",
			wantPath: "certs/wildcard",
			wantKey:  "cert",
		},
		{
			input:    "${secret:foundry-core/truenas:api_key}",
			wantPath: "foundry-core/truenas",
			wantKey:  "api_key",
		},
	}

	for _, ex := range examples {
		t.Run(ex.input, func(t *testing.T) {
			ref, err := ParseSecretRef(ex.input)
			require.NoError(t, err)
			require.NotNil(t, ref)
			assert.Equal(t, ex.wantPath, ref.Path)
			assert.Equal(t, ex.wantKey, ref.Key)
			assert.Equal(t, ex.input, ref.Raw)
		})
	}
}
