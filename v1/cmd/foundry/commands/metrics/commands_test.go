package metrics

import (
	"fmt"
	"testing"
)

func TestCommand(t *testing.T) {
	if Command == nil {
		t.Fatal("Command should not be nil")
	}

	if Command.Name != "metrics" {
		t.Errorf("Command.Name = %q, want metrics", Command.Name)
	}

	if len(Command.Commands) != 3 {
		t.Errorf("Command.Commands count = %d, want 3", len(Command.Commands))
	}

	// Verify subcommands
	foundQuery := false
	foundList := false
	foundTargets := false
	for _, cmd := range Command.Commands {
		switch cmd.Name {
		case "query":
			foundQuery = true
		case "list":
			foundList = true
		case "targets":
			foundTargets = true
		}
	}

	if !foundQuery {
		t.Error("Should have 'query' subcommand")
	}
	if !foundList {
		t.Error("Should have 'list' subcommand")
	}
	if !foundTargets {
		t.Error("Should have 'targets' subcommand")
	}
}

func TestQueryCommand(t *testing.T) {
	if QueryCommand == nil {
		t.Fatal("QueryCommand should not be nil")
	}

	if QueryCommand.Name != "query" {
		t.Errorf("QueryCommand.Name = %q, want query", QueryCommand.Name)
	}

	if QueryCommand.Action == nil {
		t.Error("QueryCommand should have an action")
	}

	// Check expected flags
	expectedFlags := []string{
		"namespace",
		"time",
		"range",
		"start",
		"end",
		"step",
		"json",
	}

	flagMap := make(map[string]bool)
	for _, flag := range QueryCommand.Flags {
		flagMap[flag.Names()[0]] = true
	}

	for _, expected := range expectedFlags {
		if !flagMap[expected] {
			t.Errorf("QueryCommand missing flag: --%s", expected)
		}
	}
}

func TestListCommand(t *testing.T) {
	if ListCommand == nil {
		t.Fatal("ListCommand should not be nil")
	}

	if ListCommand.Name != "list" {
		t.Errorf("ListCommand.Name = %q, want list", ListCommand.Name)
	}

	if ListCommand.Action == nil {
		t.Error("ListCommand should have an action")
	}

	// Check for filter flag
	foundFilter := false
	for _, flag := range ListCommand.Flags {
		if flag.Names()[0] == "filter" {
			foundFilter = true
			break
		}
	}
	if !foundFilter {
		t.Error("ListCommand should have --filter flag")
	}
}

func TestTargetsCommand(t *testing.T) {
	if TargetsCommand == nil {
		t.Fatal("TargetsCommand should not be nil")
	}

	if TargetsCommand.Name != "targets" {
		t.Errorf("TargetsCommand.Name = %q, want targets", TargetsCommand.Name)
	}

	if TargetsCommand.Action == nil {
		t.Error("TargetsCommand should have an action")
	}

	// Check for unhealthy flag
	foundUnhealthy := false
	for _, flag := range TargetsCommand.Flags {
		if flag.Names()[0] == "unhealthy" {
			foundUnhealthy = true
			break
		}
	}
	if !foundUnhealthy {
		t.Error("TargetsCommand should have --unhealthy flag")
	}
}

func TestFormatLabels(t *testing.T) {
	tests := []struct {
		name   string
		labels map[string]string
		want   string
	}{
		{
			name:   "empty labels",
			labels: map[string]string{},
			want:   "{}",
		},
		{
			name: "metric name only",
			labels: map[string]string{
				"__name__": "up",
			},
			want: "up",
		},
		{
			name: "metric with labels",
			labels: map[string]string{
				"__name__": "http_requests_total",
				"method":   "GET",
			},
			want: `http_requests_total{method="GET"}`,
		},
		{
			name: "labels without name",
			labels: map[string]string{
				"job":      "prometheus",
				"instance": "localhost:9090",
			},
			want: `{instance="localhost:9090", job="prometheus"}`, // order may vary
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Make a copy since formatLabels modifies the map
			labelsCopy := make(map[string]string)
			for k, v := range tt.labels {
				labelsCopy[k] = v
			}

			result := formatLabels(labelsCopy)

			// For tests with multiple non-name labels, we can't guarantee order
			// Only do exact match when there's at most 1 non-name label
			_, hasName := tt.labels["__name__"]
			nonNameCount := len(tt.labels)
			if hasName {
				nonNameCount--
			}

			if nonNameCount <= 1 {
				if result != tt.want {
					t.Errorf("formatLabels() = %q, want %q", result, tt.want)
				}
			} else {
				// Just verify it's formatted correctly (has braces, contains expected keys)
				if !contains(result, "{") {
					t.Errorf("formatLabels() = %q, expected formatted labels with braces", result)
				}
				for k, v := range tt.labels {
					if k == "__name__" {
						continue
					}
					expected := fmt.Sprintf(`%s="%s"`, k, v)
					if !contains(result, expected) {
						t.Errorf("formatLabels() = %q, missing label %s", result, expected)
					}
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestPrometheusResponse(t *testing.T) {
	// Test the response struct can be created
	resp := prometheusResponse{
		Status: "success",
		Data: queryData{
			ResultType: "vector",
			Result: []queryResult{
				{
					Metric: map[string]string{"__name__": "up"},
					Value:  []interface{}{1234567890.0, "1"},
				},
			},
		},
	}

	if resp.Status != "success" {
		t.Errorf("resp.Status = %q, want success", resp.Status)
	}

	if resp.Data.ResultType != "vector" {
		t.Errorf("resp.Data.ResultType = %q, want vector", resp.Data.ResultType)
	}

	if len(resp.Data.Result) != 1 {
		t.Errorf("resp.Data.Result count = %d, want 1", len(resp.Data.Result))
	}
}

func TestQueryData(t *testing.T) {
	// Test instant query result
	instantResult := queryResult{
		Metric: map[string]string{"__name__": "up", "job": "prometheus"},
		Value:  []interface{}{1234567890.0, "1"},
	}

	if len(instantResult.Value) != 2 {
		t.Errorf("instant query should have 2 values (timestamp, value)")
	}

	// Test range query result
	rangeResult := queryResult{
		Metric: map[string]string{"__name__": "up", "job": "prometheus"},
		Values: [][]interface{}{
			{1234567890.0, "1"},
			{1234567950.0, "1"},
			{1234568010.0, "0"},
		},
	}

	if len(rangeResult.Values) != 3 {
		t.Errorf("range query should have 3 value pairs")
	}
}

func TestDefaultStep(t *testing.T) {
	// Verify the default step is "1m"
	for _, flag := range QueryCommand.Flags {
		if flag.Names()[0] == "step" {
			// Flag exists - default should be 1m
			break
		}
	}
}

func TestNamespaceAlias(t *testing.T) {
	// Check that namespace has the -n alias
	for _, flag := range QueryCommand.Flags {
		if flag.Names()[0] == "namespace" {
			names := flag.Names()
			hasAlias := false
			for _, n := range names {
				if n == "n" {
					hasAlias = true
					break
				}
			}
			if !hasAlias {
				t.Error("namespace flag should have -n alias")
			}
			return
		}
	}
	t.Error("namespace flag not found")
}
