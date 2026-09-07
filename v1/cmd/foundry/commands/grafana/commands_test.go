package grafana

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/urfave/cli/v3"
)

func TestCommand(t *testing.T) {
	if Command == nil {
		t.Fatal("Command should not be nil")
	}
	if Command.Name != "grafana" {
		t.Errorf("Command.Name = %q, want grafana", Command.Name)
	}

	wantCommands := map[string]bool{"open": false, "password": false, "credentials": false}
	for _, command := range Command.Commands {
		if _, ok := wantCommands[command.Name]; ok {
			wantCommands[command.Name] = true
		}
	}
	for name, found := range wantCommands {
		if !found {
			t.Errorf("Command does not have the %q subcommand", name)
		}
	}
}

func TestGrafanaSubcommands(t *testing.T) {
	tests := []struct {
		name        string
		command     *cli.Command
		hasShowFlag bool
	}{
		{name: "open", command: OpenCommand},
		{name: "password", command: PasswordCommand, hasShowFlag: true},
		{name: "credentials", command: CredentialsCommand, hasShowFlag: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.command.Usage == "" {
				t.Error("command does not have usage text")
			}
			if test.command.Action == nil {
				t.Error("command does not have an action")
			}
			if hasFlag(test.command, "show") != test.hasShowFlag {
				t.Errorf("show flag = %t, want %t", hasFlag(test.command, "show"), test.hasShowFlag)
			}
		})
	}
}

func TestHandleCredentialsCopiesPasswordWithoutDisplayingIt(t *testing.T) {
	const (
		username = "grafana-admin"
		password = "do-not-display-this-password"
	)
	var output bytes.Buffer
	var copied string
	var configName string

	err := handleCredentials(context.Background(), &output, "catalyst.yaml", false, true, credentialActions{
		load: func(_ context.Context, name string) (string, string, error) {
			configName = name
			return username, password, nil
		},
		copy: func(_ context.Context, value string) error {
			copied = value
			return nil
		},
		display: func(context.Context, io.Writer, string, string, bool, time.Duration) error {
			t.Fatal("display must not run")
			return nil
		},
	})
	if err != nil {
		t.Fatalf("handleCredentials() error = %v", err)
	}
	if configName != "catalyst.yaml" {
		t.Errorf("config name = %q, want catalyst.yaml", configName)
	}
	if copied != password {
		t.Errorf("copied value = %q, want the password", copied)
	}
	if strings.Contains(output.String(), password) {
		t.Fatal("command output contains the password")
	}
	if !strings.Contains(output.String(), "Username: "+username) || !strings.Contains(output.String(), "copied to the clipboard") {
		t.Errorf("command output = %q", output.String())
	}
}

func TestHandlePasswordDoesNotDisplayUsername(t *testing.T) {
	var output bytes.Buffer
	err := handleCredentials(context.Background(), &output, "", false, false, credentialActions{
		load: func(context.Context, string) (string, string, error) {
			return "grafana-admin", "test-secret-value", nil
		},
		copy: func(context.Context, string) error { return nil },
	})
	if err != nil {
		t.Fatalf("handleCredentials() error = %v", err)
	}
	if strings.Contains(output.String(), "grafana-admin") || strings.Contains(output.String(), "test-secret-value") {
		t.Errorf("password command output exposes credentials: %q", output.String())
	}
}

func TestHandleCredentialsShowsInTemporaryView(t *testing.T) {
	var displayCalled bool
	err := handleCredentials(context.Background(), io.Discard, "", true, true, credentialActions{
		load: func(context.Context, string) (string, string, error) {
			return "grafana-admin", "password", nil
		},
		copy: func(context.Context, string) error {
			t.Fatal("copy must not run with --show")
			return nil
		},
		display: func(_ context.Context, _ io.Writer, username, password string, includeUsername bool, duration time.Duration) error {
			displayCalled = true
			if username != "grafana-admin" || password != "password" || !includeUsername {
				t.Errorf("display arguments = %q, %q, %t", username, password, includeUsername)
			}
			if duration != 5*time.Second {
				t.Errorf("display duration = %s, want 5s", duration)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("handleCredentials() error = %v", err)
	}
	if !displayCalled {
		t.Fatal("display was not called")
	}
}

func TestHandleCredentialsReturnsSafeErrors(t *testing.T) {
	t.Run("load", func(t *testing.T) {
		err := handleCredentials(context.Background(), io.Discard, "", false, true, credentialActions{
			load: func(context.Context, string) (string, string, error) {
				return "", "", errors.New("not found")
			},
		})
		if err == nil || !strings.Contains(err.Error(), "Ensure Grafana is installed") {
			t.Fatalf("handleCredentials() error = %v", err)
		}
	})

	t.Run("clipboard", func(t *testing.T) {
		err := handleCredentials(context.Background(), io.Discard, "", false, true, credentialActions{
			load: func(context.Context, string) (string, string, error) {
				return "admin", "do-not-display", nil
			},
			copy: func(context.Context, string) error {
				return errors.New("clipboard unavailable")
			},
		})
		if err == nil || !strings.Contains(err.Error(), "Use --show") {
			t.Fatalf("handleCredentials() error = %v", err)
		}
		if strings.Contains(err.Error(), "do-not-display") {
			t.Fatal("clipboard error contains the password")
		}
	})
}

func TestHandleOpenUsesConfiguredGrafanaURL(t *testing.T) {
	var output bytes.Buffer
	var requestedConfig string
	var loadedPath string
	var openedURL string
	err := handleOpen(context.Background(), &output, "catalyst.yaml", openActions{
		findConfig: func(name string) (string, error) {
			requestedConfig = name
			return "/configs/catalyst.yaml", nil
		},
		loadConfig: func(path string) (*config.Config, error) {
			loadedPath = path
			return &config.Config{Components: config.ComponentMap{
				"grafana": {Config: map[string]any{"ingress_enabled": true, "ingress_host": "grafana.example.test"}},
			}}, nil
		},
		openURL: func(_ context.Context, value string) error {
			openedURL = value
			return nil
		},
	})
	if err != nil {
		t.Fatalf("handleOpen() error = %v", err)
	}
	if requestedConfig != "catalyst.yaml" || loadedPath != "/configs/catalyst.yaml" {
		t.Errorf("config request = %q, loaded path = %q", requestedConfig, loadedPath)
	}
	if openedURL != "https://grafana.example.test" {
		t.Errorf("opened URL = %q", openedURL)
	}
	if !strings.Contains(output.String(), openedURL) {
		t.Errorf("output = %q, want opened URL", output.String())
	}
}

func TestHandleOpenReturnsErrors(t *testing.T) {
	t.Run("find config", func(t *testing.T) {
		err := handleOpen(context.Background(), io.Discard, "", openActions{
			findConfig: func(string) (string, error) { return "", errors.New("not found") },
		})
		if err == nil || !strings.Contains(err.Error(), "find stack config") {
			t.Fatalf("handleOpen() error = %v", err)
		}
	})

	t.Run("load config", func(t *testing.T) {
		err := handleOpen(context.Background(), io.Discard, "", openActions{
			findConfig: func(string) (string, error) { return "/config.yaml", nil },
			loadConfig: func(string) (*config.Config, error) { return nil, errors.New("invalid YAML") },
		})
		if err == nil || !strings.Contains(err.Error(), "load stack config") {
			t.Fatalf("handleOpen() error = %v", err)
		}
	})

	t.Run("open browser", func(t *testing.T) {
		err := handleOpen(context.Background(), io.Discard, "", openActions{
			findConfig: func(string) (string, error) { return "/config.yaml", nil },
			loadConfig: func(string) (*config.Config, error) {
				return &config.Config{Components: config.ComponentMap{
					"grafana": {Config: map[string]any{"ingress_host": "grafana.example.test"}},
				}}, nil
			},
			openURL: func(context.Context, string) error { return errors.New("no browser") },
		})
		if err == nil || !strings.Contains(err.Error(), "Open https://grafana.example.test in a browser") {
			t.Fatalf("handleOpen() error = %v", err)
		}
	})
}

func TestURLForGrafana(t *testing.T) {
	tests := []struct {
		name    string
		config  *config.Config
		wantURL string
		wantErr string
	}{
		{
			name: "configured hostname",
			config: &config.Config{Components: config.ComponentMap{
				"grafana": {Config: map[string]any{"ingress_enabled": true, "ingress_host": "metrics.example.test"}},
			}},
			wantURL: "https://metrics.example.test",
		},
		{
			name: "primary domain fallback",
			config: &config.Config{
				Cluster: config.ClusterConfig{PrimaryDomain: "example.test"},
				Components: config.ComponentMap{
					"grafana": {Config: map[string]any{"ingress_enabled": true}},
				},
			},
			wantURL: "https://grafana.example.test",
		},
		{
			name: "disabled",
			config: &config.Config{Components: config.ComponentMap{
				"grafana": {Config: map[string]any{"ingress_enabled": false, "ingress_host": "grafana.example.test"}},
			}},
			wantErr: "disabled",
		},
		{name: "not configured", config: &config.Config{}, wantErr: "not configured"},
		{
			name: "invalid hostname",
			config: &config.Config{Components: config.ComponentMap{
				"grafana": {Config: map[string]any{"ingress_host": "https://grafana.example.test"}},
			}},
			wantErr: "invalid external hostname",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := urlForGrafana(test.config)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("urlForGrafana() error = %v, want text %q", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("urlForGrafana() error = %v", err)
			}
			if got != test.wantURL {
				t.Errorf("urlForGrafana() = %q, want %q", got, test.wantURL)
			}
		})
	}
}

func TestDisplayCredentialsRequiresTerminal(t *testing.T) {
	var output bytes.Buffer
	err := displayCredentials(context.Background(), &output, "admin", "do-not-display", true, 0)
	if err == nil || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("displayCredentials() error = %v", err)
	}
	if output.Len() != 0 {
		t.Errorf("output = %q, want empty", output.String())
	}
}

func TestDisplayCredentialsUsesAndClearsTemporaryView(t *testing.T) {
	var output bytes.Buffer
	err := displayCredentialsInTemporaryView(context.Background(), &output, "admin", "test-password", true, 0)
	if err != nil {
		t.Fatalf("displayCredentialsInTemporaryView() error = %v", err)
	}
	got := output.String()
	for _, want := range []string{"\x1b[?1049h", "Grafana username: admin", "Grafana password: test-password", "\x1b[2J", "\x1b[?1049l"} {
		if !strings.Contains(got, want) {
			t.Errorf("temporary view output does not contain %q", want)
		}
	}
}

func TestDisplayCredentialsClearsTemporaryViewWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var output bytes.Buffer
	err := displayCredentialsInTemporaryView(ctx, &output, "admin", "test-password", true, time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("displayCredentialsInTemporaryView() error = %v, want context canceled", err)
	}
	if !strings.HasSuffix(output.String(), "\x1b[2J\x1b[H\x1b[?25h\x1b[?1049l") {
		t.Error("temporary view was not cleared after cancellation")
	}
}

func hasFlag(command *cli.Command, name string) bool {
	for _, flag := range command.Flags {
		for _, flagName := range flag.Names() {
			if flagName == name {
				return true
			}
		}
	}
	return false
}
