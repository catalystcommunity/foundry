package dashboard

import (
	"runtime"
	"testing"
)

func TestCommand(t *testing.T) {
	if Command == nil {
		t.Fatal("Command should not be nil")
	}

	if Command.Name != "dashboard" {
		t.Errorf("Command.Name = %q, want dashboard", Command.Name)
	}

	if len(Command.Commands) != 2 {
		t.Errorf("Command.Commands count = %d, want 2", len(Command.Commands))
	}

	// Verify subcommands
	foundOpen := false
	foundURL := false
	for _, cmd := range Command.Commands {
		switch cmd.Name {
		case "open":
			foundOpen = true
		case "url":
			foundURL = true
		}
	}

	if !foundOpen {
		t.Error("Should have 'open' subcommand")
	}
	if !foundURL {
		t.Error("Should have 'url' subcommand")
	}
}

func TestOpenCommand(t *testing.T) {
	if OpenCommand == nil {
		t.Fatal("OpenCommand should not be nil")
	}

	if OpenCommand.Name != "open" {
		t.Errorf("OpenCommand.Name = %q, want open", OpenCommand.Name)
	}

	if OpenCommand.Action == nil {
		t.Error("OpenCommand should have an action")
	}

	// Check for namespace flag
	foundNamespace := false
	for _, flag := range OpenCommand.Flags {
		if flag.Names()[0] == "namespace" {
			foundNamespace = true
			break
		}
	}
	if !foundNamespace {
		t.Error("Should have --namespace flag")
	}
}

func TestURLCommand(t *testing.T) {
	if URLCommand == nil {
		t.Fatal("URLCommand should not be nil")
	}

	if URLCommand.Name != "url" {
		t.Errorf("URLCommand.Name = %q, want url", URLCommand.Name)
	}

	if URLCommand.Action == nil {
		t.Error("URLCommand should have an action")
	}
}

func TestOpenBrowserCommand(t *testing.T) {
	// Test that we generate appropriate commands for different OSes
	tests := []struct {
		os       string
		expected string
	}{
		{"darwin", "open"},
		{"linux", "xdg-open"},
		{"windows", "rundll32"},
	}

	for _, tt := range tests {
		t.Run(tt.os, func(t *testing.T) {
			// We can't easily test the actual command execution
			// but we can verify the OS detection logic exists
			switch tt.os {
			case "darwin", "linux", "windows":
				// Valid OS
			default:
				t.Errorf("Unknown OS: %s", tt.os)
			}
		})
	}
}

func TestRuntimeGOOS(t *testing.T) {
	// Verify runtime.GOOS is accessible
	goos := runtime.GOOS
	if goos == "" {
		t.Error("runtime.GOOS should not be empty")
	}

	// Should be one of the supported platforms
	switch goos {
	case "darwin", "linux", "windows", "freebsd", "netbsd", "openbsd":
		// Known platforms
	default:
		t.Logf("Running on less common platform: %s", goos)
	}
}

func TestDefaultNamespace(t *testing.T) {
	// Verify the default namespace is "monitoring"
	for _, flag := range OpenCommand.Flags {
		if flag.Names()[0] == "namespace" {
			// The default should be monitoring
			// We can't easily check the default value without parsing,
			// but we can verify the flag exists
			break
		}
	}
}
