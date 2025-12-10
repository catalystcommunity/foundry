package cluster

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLabelArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantSet    map[string]string
		wantRemove []string
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "single set",
			args:       []string{"environment=production"},
			wantSet:    map[string]string{"environment": "production"},
			wantRemove: []string{},
			wantErr:    false,
		},
		{
			name: "multiple sets",
			args: []string{"environment=production", "zone=us-east-1a"},
			wantSet: map[string]string{
				"environment": "production",
				"zone":        "us-east-1a",
			},
			wantRemove: []string{},
			wantErr:    false,
		},
		{
			name:       "single remove",
			args:       []string{"zone-"},
			wantSet:    map[string]string{},
			wantRemove: []string{"zone"},
			wantErr:    false,
		},
		{
			name:       "multiple removes",
			args:       []string{"zone-", "environment-"},
			wantSet:    map[string]string{},
			wantRemove: []string{"zone", "environment"},
			wantErr:    false,
		},
		{
			name: "mixed set and remove",
			args: []string{"environment=production", "oldlabel-"},
			wantSet: map[string]string{
				"environment": "production",
			},
			wantRemove: []string{"oldlabel"},
			wantErr:    false,
		},
		{
			name:       "label with prefix",
			args:       []string{"app.example.com/tier=frontend"},
			wantSet:    map[string]string{"app.example.com/tier": "frontend"},
			wantRemove: []string{},
			wantErr:    false,
		},
		{
			name:       "empty value",
			args:       []string{"marker="},
			wantSet:    map[string]string{"marker": ""},
			wantRemove: []string{},
			wantErr:    false,
		},
		{
			name:    "invalid format - no equals",
			args:    []string{"invalid"},
			wantErr: true,
			errMsg:  "invalid label format",
		},
		{
			name:    "invalid key - starts with dash",
			args:    []string{"-invalid=value"},
			wantErr: true,
			errMsg:  "invalid label key",
		},
		{
			name:    "invalid value - starts with dash",
			args:    []string{"key=-value"},
			wantErr: true,
			errMsg:  "invalid label value",
		},
		{
			name:    "invalid remove - just dash",
			args:    []string{"-"},
			wantErr: true,
			errMsg:  "invalid label removal syntax",
		},
		{
			name:    "invalid remove key",
			args:    []string{"-invalid-"},
			wantErr: true,
			errMsg:  "invalid label key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set, remove, err := parseLabelArgs(tt.args)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantSet, set)
				assert.Equal(t, tt.wantRemove, remove)
			}
		})
	}
}

func TestNewNodeLabelCommand(t *testing.T) {
	cmd := NewNodeLabelCommand()

	assert.Equal(t, "label", cmd.Name)
	assert.Equal(t, "Manage node labels", cmd.Usage)

	// Check flags exist
	flagNames := make([]string, len(cmd.Flags))
	for i, f := range cmd.Flags {
		flagNames[i] = f.Names()[0]
	}
	assert.Contains(t, flagNames, "list")
	assert.Contains(t, flagNames, "save")
	assert.Contains(t, flagNames, "user-only")
}
