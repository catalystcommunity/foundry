package grafana

import (
	"testing"
)

func TestCommand(t *testing.T) {
	if Command == nil {
		t.Fatal("Command should not be nil")
	}

	if Command.Name != "grafana" {
		t.Errorf("Command.Name = %q, want grafana", Command.Name)
	}

	if len(Command.Commands) != 2 {
		t.Errorf("Command.Commands count = %d, want 2", len(Command.Commands))
	}

	// Verify subcommands
	foundPassword := false
	foundCredentials := false
	for _, cmd := range Command.Commands {
		switch cmd.Name {
		case "password":
			foundPassword = true
		case "credentials":
			foundCredentials = true
		}
	}

	if !foundPassword {
		t.Error("Should have 'password' subcommand")
	}
	if !foundCredentials {
		t.Error("Should have 'credentials' subcommand")
	}
}

func TestPasswordCommand(t *testing.T) {
	if PasswordCommand == nil {
		t.Fatal("PasswordCommand should not be nil")
	}

	if PasswordCommand.Name != "password" {
		t.Errorf("PasswordCommand.Name = %q, want password", PasswordCommand.Name)
	}

	if PasswordCommand.Action == nil {
		t.Error("PasswordCommand should have an action")
	}
}

func TestCredentialsCommand(t *testing.T) {
	if CredentialsCommand == nil {
		t.Fatal("CredentialsCommand should not be nil")
	}

	if CredentialsCommand.Name != "credentials" {
		t.Errorf("CredentialsCommand.Name = %q, want credentials", CredentialsCommand.Name)
	}

	if CredentialsCommand.Action == nil {
		t.Error("CredentialsCommand should have an action")
	}

	// Check for json flag
	foundJSON := false
	for _, flag := range CredentialsCommand.Flags {
		if flag.Names()[0] == "json" {
			foundJSON = true
			break
		}
	}
	if !foundJSON {
		t.Error("Should have --json flag")
	}
}

func TestCommandUsage(t *testing.T) {
	if Command.Usage == "" {
		t.Error("Command should have usage text")
	}

	if PasswordCommand.Usage == "" {
		t.Error("PasswordCommand should have usage text")
	}

	if CredentialsCommand.Usage == "" {
		t.Error("CredentialsCommand should have usage text")
	}
}

func TestCommandDescription(t *testing.T) {
	if Command.Description == "" {
		t.Error("Command should have description text")
	}

	if PasswordCommand.Description == "" {
		t.Error("PasswordCommand should have description text")
	}

	if CredentialsCommand.Description == "" {
		t.Error("CredentialsCommand should have description text")
	}
}
