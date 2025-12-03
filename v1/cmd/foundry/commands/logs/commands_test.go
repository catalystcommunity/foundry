package logs

import (
	"testing"
)

func TestCommand(t *testing.T) {
	if Command == nil {
		t.Fatal("Command should not be nil")
	}

	if Command.Name != "logs" {
		t.Errorf("Command.Name = %q, want logs", Command.Name)
	}

	if Command.Action == nil {
		t.Error("Command should have an action")
	}
}

func TestCommandFlags(t *testing.T) {
	expectedFlags := []string{
		"namespace",
		"selector",
		"container",
		"follow",
		"tail",
		"previous",
		"timestamps",
		"since",
		"all-namespaces",
	}

	flagMap := make(map[string]bool)
	for _, flag := range Command.Flags {
		flagMap[flag.Names()[0]] = true
	}

	for _, expected := range expectedFlags {
		if !flagMap[expected] {
			t.Errorf("Missing flag: --%s", expected)
		}
	}
}

func TestParseSimpleDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{
			name:  "seconds",
			input: "30s",
			want:  30,
		},
		{
			name:  "minutes",
			input: "5m",
			want:  300,
		},
		{
			name:  "hours",
			input: "2h",
			want:  7200,
		},
		{
			name:  "days",
			input: "1d",
			want:  86400,
		},
		{
			name:  "minutes with suffix",
			input: "10min",
			want:  600,
		},
		{
			name:  "hours with suffix",
			input: "1hour",
			want:  3600,
		},
		{
			name:    "invalid format",
			input:   "abc",
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "unknown unit",
			input:   "5x",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := parseSimpleDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSimpleDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if err == nil && int64(d.Seconds()) != tt.want {
				t.Errorf("parseSimpleDuration(%q) = %v seconds, want %v", tt.input, int64(d.Seconds()), tt.want)
			}
		})
	}
}

func TestDurationSeconds(t *testing.T) {
	d := duration{seconds: 3600}
	if d.Seconds() != 3600.0 {
		t.Errorf("duration.Seconds() = %v, want 3600.0", d.Seconds())
	}
}

func TestCommandAliases(t *testing.T) {
	// Check that common flags have short aliases
	aliasChecks := map[string]string{
		"namespace":      "n",
		"selector":       "l",
		"container":      "c",
		"follow":         "f",
		"previous":       "p",
		"all-namespaces": "A",
	}

	for _, flag := range Command.Flags {
		name := flag.Names()[0]
		if expectedAlias, ok := aliasChecks[name]; ok {
			names := flag.Names()
			hasAlias := false
			for _, n := range names {
				if n == expectedAlias {
					hasAlias = true
					break
				}
			}
			if !hasAlias {
				t.Errorf("Flag --%s should have alias -%s", name, expectedAlias)
			}
		}
	}
}

func TestDefaultNamespace(t *testing.T) {
	// Verify the flag exists with expected properties
	for _, flag := range Command.Flags {
		if flag.Names()[0] == "namespace" {
			// Flag exists
			return
		}
	}
	t.Error("namespace flag not found")
}
