package manager

import (
	"fmt"
	"strings"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingExecutor struct {
	commands []string
	token    string
	stack    string
}

func (e *recordingExecutor) Execute(command string) (string, error) {
	e.commands = append(e.commands, command)
	switch {
	case command == "command -v docker":
		return "/usr/bin/docker\n", nil
	case strings.HasSuffix(command, "/stack.yaml") && e.stack != "":
		return e.stack, nil
	case strings.HasPrefix(command, "sudo cat "):
		return e.token + "\n", nil
	default:
		return "", nil
	}
}

func TestReadStack(t *testing.T) {
	executor := &recordingExecutor{stack: "cluster:\n  name: test\n"}
	data, err := ReadStack(executor, "/var/lib/foundry/")
	require.NoError(t, err)
	assert.Equal(t, executor.stack, string(data))
	assert.Equal(t, "sudo cat /var/lib/foundry/stack.yaml", executor.commands[0])

	_, err = ReadStack(executor, "../foundry")
	require.Error(t, err)
}

func TestReadToken(t *testing.T) {
	token := strings.Repeat("a", 43)
	executor := &recordingExecutor{token: token}
	actual, err := ReadToken(executor, "/var/lib/foundry")
	require.NoError(t, err)
	assert.Equal(t, token, actual)

	executor.token = "short"
	_, err = ReadToken(executor, "/var/lib/foundry")
	require.Error(t, err)
}

func TestInstallCreatesLoopbackOnlyHardenedManager(t *testing.T) {
	token := strings.Repeat("a", 43)
	executor := &recordingExecutor{token: token}
	returned, err := Install(executor, InstallInput{
		Config: &config.ManagementConfig{
			Host: "manager", Port: 9080, Image: "example.invalid/foundry", Version: "v1.2.3", DataPath: "/var/lib/foundry",
		},
		StackYAML: []byte("cluster: {}\n"),
		SSHKeys: map[string]*ssh.KeyPair{
			"node-1": {Private: []byte("private"), Public: []byte("public")},
		},
		Token: token,
	})
	require.NoError(t, err)
	assert.Equal(t, token, returned)
	all := strings.Join(executor.commands, "\n")
	assert.Contains(t, all, "-p 127.0.0.1:9080:8080")
	assert.Contains(t, all, "--read-only")
	assert.Contains(t, all, "--cap-drop ALL")
	assert.Contains(t, all, "--security-opt no-new-privileges=true")
	assert.Contains(t, all, "FOUNDRY_MANAGER=1")
	assert.Contains(t, all, "chmod 0600 /var/lib/foundry/stack.yaml")
	assert.NotContains(t, all, "docker.sock")
	assert.Contains(t, all, "sudo systemctl restart foundry-manager")
}

func TestInstallRejectsUnsafeInputs(t *testing.T) {
	valid := &config.ManagementConfig{Host: "manager", Port: 9080, Image: "foundry", Version: "latest", DataPath: "/var/lib/foundry"}
	tests := []struct {
		name   string
		change func(*config.ManagementConfig, *InstallInput)
	}{
		{"unsafe image", func(cfg *config.ManagementConfig, _ *InstallInput) { cfg.Image = "foundry;reboot" }},
		{"relative path", func(cfg *config.ManagementConfig, _ *InstallInput) { cfg.DataPath = "var/lib/foundry" }},
		{"unsafe key name", func(_ *config.ManagementConfig, input *InstallInput) {
			input.SSHKeys = map[string]*ssh.KeyPair{"../node": {Private: []byte("x")}}
		}},
		{"unknown runtime", func(_ *config.ManagementConfig, input *InstallInput) { input.Runtime = "unknown" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := *valid
			input := InstallInput{Config: &cfg, Token: strings.Repeat("a", 43)}
			test.change(&cfg, &input)
			_, err := Install(&recordingExecutor{token: input.Token}, input)
			require.Error(t, err, fmt.Sprintf("%s must be rejected", test.name))
		})
	}
}
