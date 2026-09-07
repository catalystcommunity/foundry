package desktop

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestOpenURLStartsPlatformBrowser(t *testing.T) {
	tests := []struct {
		name      string
		goos      string
		available map[string]bool
		wantPath  string
		wantArgs  []string
	}{
		{name: "macOS", goos: "darwin", available: map[string]bool{"open": true}, wantPath: "/bin/open", wantArgs: []string{"https://grafana.example.test"}},
		{name: "Linux xdg", goos: "linux", available: map[string]bool{"xdg-open": true}, wantPath: "/bin/xdg-open", wantArgs: []string{"https://grafana.example.test"}},
		{name: "Linux fallback", goos: "linux", available: map[string]bool{"gio": true}, wantPath: "/bin/gio", wantArgs: []string{"open", "https://grafana.example.test"}},
		{name: "Windows", goos: "windows", available: map[string]bool{"rundll32.exe": true}, wantPath: "/bin/rundll32.exe", wantArgs: []string{"url.dll,FileProtocolHandler", "https://grafana.example.test"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var started commandSpec
			err := openURL(
				context.Background(),
				"https://grafana.example.test",
				test.goos,
				fakeLookup(test.available),
				func(_ context.Context, spec commandSpec) error {
					started = spec
					return nil
				},
			)
			if err != nil {
				t.Fatalf("openURL() error = %v", err)
			}
			if started.path != test.wantPath {
				t.Errorf("path = %q, want %q", started.path, test.wantPath)
			}
			if !reflect.DeepEqual(started.args, test.wantArgs) {
				t.Errorf("args = %#v, want %#v", started.args, test.wantArgs)
			}
		})
	}
}

func TestOpenURLReturnsErrors(t *testing.T) {
	tests := []struct {
		name string
		goos string
		want string
	}{
		{name: "missing browser", goos: "linux", want: "no browser opener"},
		{name: "unsupported platform", goos: "plan9", want: "not supported"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := openURL(context.Background(), "https://grafana.example.test", test.goos, fakeLookup(nil), func(context.Context, commandSpec) error {
				t.Fatal("browser command must not start")
				return nil
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("openURL() error = %v, want text %q", err, test.want)
			}
		})
	}
}

func TestCopyToClipboardUsesStdinWithoutChangingTheSecret(t *testing.T) {
	const secret = "secret with spaces\nand a newline"
	var ran []commandSpec
	var inputs []string

	err := copyToClipboard(
		context.Background(),
		secret,
		"linux",
		fakeLookup(map[string]bool{"wl-copy": true, "xclip": true}),
		func(_ context.Context, spec commandSpec, input string) error {
			ran = append(ran, spec)
			inputs = append(inputs, input)
			if strings.HasSuffix(spec.path, "wl-copy") {
				return errors.New("Wayland is not available")
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("copyToClipboard() error = %v", err)
	}
	if len(ran) != 2 {
		t.Fatalf("ran %d commands, want 2", len(ran))
	}
	if ran[1].path != "/bin/xclip" || !reflect.DeepEqual(ran[1].args, []string{"-selection", "clipboard"}) {
		t.Errorf("fallback command = %#v", ran[1])
	}
	for _, input := range inputs {
		if input != secret {
			t.Errorf("clipboard input = %q, want the unchanged secret", input)
		}
	}
}

func TestClipboardCommandsForEachPlatform(t *testing.T) {
	tests := []struct {
		goos      string
		available map[string]bool
		wantPath  string
		wantArgs  []string
	}{
		{goos: "darwin", available: map[string]bool{"pbcopy": true}, wantPath: "/bin/pbcopy"},
		{goos: "windows", available: map[string]bool{"clip.exe": true}, wantPath: "/bin/clip.exe"},
		{
			goos:      "windows",
			available: map[string]bool{"powershell.exe": true},
			wantPath:  "/bin/powershell.exe",
			wantArgs:  []string{"-NoProfile", "-NonInteractive", "-Command", "[Console]::In.ReadToEnd() | Set-Clipboard"},
		},
	}

	for _, test := range tests {
		t.Run(test.goos+test.wantPath, func(t *testing.T) {
			commands, err := clipboardCommands(test.goos, fakeLookup(test.available))
			if err != nil {
				t.Fatalf("clipboardCommands() error = %v", err)
			}
			if len(commands) != 1 {
				t.Fatalf("command count = %d, want 1", len(commands))
			}
			if commands[0].path != test.wantPath || !reflect.DeepEqual(commands[0].args, test.wantArgs) {
				t.Errorf("command = %#v, want path %q and args %#v", commands[0], test.wantPath, test.wantArgs)
			}
		})
	}
}

func TestCopyToClipboardReturnsErrors(t *testing.T) {
	tests := []struct {
		name string
		goos string
		want string
	}{
		{name: "missing Linux command", goos: "linux", want: "install wl-clipboard, xclip, or xsel"},
		{name: "unsupported platform", goos: "plan9", want: "not supported"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := copyToClipboard(context.Background(), "do-not-print", test.goos, fakeLookup(nil), func(context.Context, commandSpec, string) error {
				t.Fatal("clipboard command must not run")
				return nil
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("copyToClipboard() error = %v, want text %q", err, test.want)
			}
		})
	}
}

func fakeLookup(available map[string]bool) executableLookup {
	return func(name string) (string, error) {
		if available[name] {
			return "/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
}
