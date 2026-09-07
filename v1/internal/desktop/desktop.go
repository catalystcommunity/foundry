// Package desktop opens URLs and writes text to the desktop clipboard.
package desktop

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
)

type commandSpec struct {
	path string
	args []string
}

type executableLookup func(string) (string, error)
type commandStarter func(context.Context, commandSpec) error
type commandRunner func(context.Context, commandSpec, string) error

// OpenURL opens a URL in the default browser.
func OpenURL(ctx context.Context, url string) error {
	return openURL(ctx, url, runtime.GOOS, exec.LookPath, startCommand)
}

func openURL(ctx context.Context, url, goos string, lookup executableLookup, start commandStarter) error {
	spec, err := browserCommand(goos, url, lookup)
	if err != nil {
		return err
	}
	if err := start(ctx, spec); err != nil {
		return fmt.Errorf("start %s: %w", spec.path, err)
	}
	return nil
}

func browserCommand(goos, url string, lookup executableLookup) (commandSpec, error) {
	var candidates []commandSpec
	switch goos {
	case "darwin":
		candidates = []commandSpec{{path: "open", args: []string{url}}}
	case "linux":
		candidates = []commandSpec{
			{path: "xdg-open", args: []string{url}},
			{path: "gio", args: []string{"open", url}},
			{path: "sensible-browser", args: []string{url}},
		}
	case "windows":
		candidates = []commandSpec{{path: "rundll32.exe", args: []string{"url.dll,FileProtocolHandler", url}}}
	default:
		return commandSpec{}, fmt.Errorf("opening a browser is not supported on %s", goos)
	}

	for _, candidate := range candidates {
		path, err := lookup(candidate.path)
		if err == nil {
			candidate.path = path
			return candidate, nil
		}
	}
	return commandSpec{}, fmt.Errorf("no browser opener is installed")
}

func startCommand(ctx context.Context, spec commandSpec) error {
	command := exec.CommandContext(ctx, spec.path, spec.args...)
	if err := command.Start(); err != nil {
		return err
	}
	go func() {
		_ = command.Wait()
	}()
	return nil
}

// CopyToClipboard writes text to the desktop clipboard through standard input.
func CopyToClipboard(ctx context.Context, value string) error {
	return copyToClipboard(ctx, value, runtime.GOOS, exec.LookPath, runCommand)
}

func copyToClipboard(ctx context.Context, value, goos string, lookup executableLookup, run commandRunner) error {
	candidates, err := clipboardCommands(goos, lookup)
	if err != nil {
		return err
	}

	for _, candidate := range candidates {
		if err := run(ctx, candidate, value); err == nil {
			return nil
		}
	}
	return fmt.Errorf("the installed clipboard commands could not write to the clipboard")
}

func clipboardCommands(goos string, lookup executableLookup) ([]commandSpec, error) {
	var candidates []commandSpec
	switch goos {
	case "darwin":
		candidates = []commandSpec{{path: "pbcopy"}}
	case "linux":
		candidates = []commandSpec{
			{path: "wl-copy"},
			{path: "xclip", args: []string{"-selection", "clipboard"}},
			{path: "xsel", args: []string{"--clipboard", "--input"}},
		}
	case "windows":
		candidates = []commandSpec{
			{path: "clip.exe"},
			{path: "powershell.exe", args: []string{"-NoProfile", "-NonInteractive", "-Command", "[Console]::In.ReadToEnd() | Set-Clipboard"}},
		}
	default:
		return nil, fmt.Errorf("clipboard access is not supported on %s", goos)
	}

	available := make([]commandSpec, 0, len(candidates))
	for _, candidate := range candidates {
		path, err := lookup(candidate.path)
		if err != nil {
			continue
		}
		candidate.path = path
		available = append(available, candidate)
	}
	if len(available) == 0 {
		return nil, fmt.Errorf("no clipboard command is installed%s", clipboardInstallHint(goos))
	}
	return available, nil
}

func clipboardInstallHint(goos string) string {
	if goos == "linux" {
		return "; install wl-clipboard, xclip, or xsel"
	}
	return ""
}

func runCommand(ctx context.Context, spec commandSpec, input string) error {
	command := exec.CommandContext(ctx, spec.path, spec.args...)
	command.Stdin = strings.NewReader(input)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	return command.Run()
}
