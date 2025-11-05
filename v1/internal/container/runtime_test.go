package container

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockSSHExecutor is a mock SSH connection for testing
type mockSSHExecutor struct {
	commands map[string]string // command -> output
	errors   map[string]error  // command -> error
}

func newMockSSHExecutor() *mockSSHExecutor {
	return &mockSSHExecutor{
		commands: make(map[string]string),
		errors:   make(map[string]error),
	}
}

func (m *mockSSHExecutor) Execute(cmd string) (string, error) {
	// Check for exact match first
	if output, ok := m.commands[cmd]; ok {
		if err, hasErr := m.errors[cmd]; hasErr {
			return "", err
		}
		return output, nil
	}

	// Check for prefix match (for flexible matching)
	for key, output := range m.commands {
		if strings.HasPrefix(cmd, key) {
			if err, hasErr := m.errors[key]; hasErr {
				return "", err
			}
			return output, nil
		}
	}

	return "", fmt.Errorf("unexpected command: %s", cmd)
}

func (m *mockSSHExecutor) setCommand(cmd, output string) {
	m.commands[cmd] = output
}

func (m *mockSSHExecutor) setError(cmd string, err error) {
	m.errors[cmd] = err
}

func TestDockerRuntime_Name(t *testing.T) {
	mock := newMockSSHExecutor()
	runtime := NewDockerRuntime(mock)
	assert.Equal(t, "docker", runtime.Name())
}

func TestDockerRuntime_IsAvailable(t *testing.T) {
	mock := newMockSSHExecutor()
	runtime := NewDockerRuntime(mock)

	// Docker is available
	mock.setCommand("docker --version", "Docker version 24.0.0")
	assert.True(t, runtime.IsAvailable())

	// Docker is not available
	mock2 := newMockSSHExecutor()
	mock2.setError("docker --version", fmt.Errorf("command not found"))
	runtime2 := NewDockerRuntime(mock2)
	assert.False(t, runtime2.IsAvailable())
}

func TestDockerRuntime_Pull(t *testing.T) {
	mock := newMockSSHExecutor()
	runtime := NewDockerRuntime(mock)

	mock.setCommand("docker pull nginx:latest", "latest: Pulling from library/nginx\nStatus: Downloaded newer image")

	err := runtime.Pull("nginx:latest")
	assert.NoError(t, err)
}

func TestDockerRuntime_Pull_Error(t *testing.T) {
	mock := newMockSSHExecutor()
	runtime := NewDockerRuntime(mock)

	mock.setError("docker pull invalid:image", fmt.Errorf("image not found"))

	err := runtime.Pull("invalid:image")
	assert.Error(t, err)
}

func TestDockerRuntime_Run_Simple(t *testing.T) {
	mock := newMockSSHExecutor()
	runtime := NewDockerRuntime(mock)

	containerID := "abc123def456"
	mock.setCommand("docker run", containerID)

	config := RunConfig{
		Image: "nginx:latest",
	}

	id, err := runtime.Run(config)
	require.NoError(t, err)
	assert.Equal(t, containerID, id)
}

func TestDockerRuntime_Run_Complex(t *testing.T) {
	mock := newMockSSHExecutor()
	runtime := NewDockerRuntime(mock)

	containerID := "complex123"
	mock.setCommand("docker run", containerID)

	config := RunConfig{
		Image:         "nginx:latest",
		Name:          "my-nginx",
		Detach:        true,
		Remove:        true,
		Ports:         []string{"8080:80", "8443:443"},
		Volumes:       []string{"/host/path:/container/path"},
		Env:           []string{"KEY=value", "FOO=bar"},
		Network:       "my-network",
		RestartPolicy: "unless-stopped",
		User:          "1000:1000",
		WorkDir:       "/app",
		Labels:        map[string]string{"app": "nginx", "env": "prod"},
		Privileged:    true,
		Command:       []string{"nginx", "-g", "daemon off;"},
	}

	id, err := runtime.Run(config)
	require.NoError(t, err)
	assert.Equal(t, containerID, id)
}

func TestDockerRuntime_Stop(t *testing.T) {
	mock := newMockSSHExecutor()
	runtime := NewDockerRuntime(mock)

	mock.setCommand("docker stop", "abc123")

	err := runtime.Stop("abc123", 10*time.Second)
	assert.NoError(t, err)
}

func TestDockerRuntime_Remove(t *testing.T) {
	mock := newMockSSHExecutor()
	runtime := NewDockerRuntime(mock)

	// Remove without force
	mock.setCommand("docker rm abc123", "abc123")
	err := runtime.Remove("abc123", false)
	assert.NoError(t, err)

	// Remove with force
	mock.setCommand("docker rm -f def456", "def456")
	err = runtime.Remove("def456", true)
	assert.NoError(t, err)
}

func TestDockerRuntime_Inspect(t *testing.T) {
	mock := newMockSSHExecutor()
	runtime := NewDockerRuntime(mock)

	inspectOutput := `[{
		"Id": "abc123",
		"Name": "/my-container",
		"Created": "2023-01-01T12:00:00.000000000Z",
		"Config": {
			"Image": "nginx:latest",
			"Labels": {
				"app": "nginx",
				"env": "prod"
			}
		},
		"State": {
			"Status": "running"
		}
	}]`

	mock.setCommand("docker inspect abc123", inspectOutput)

	info, err := runtime.Inspect("abc123")
	require.NoError(t, err)
	assert.Equal(t, "abc123", info.ID)
	assert.Equal(t, "my-container", info.Name)
	assert.Equal(t, "nginx:latest", info.Image)
	assert.Equal(t, "running", info.State)
	assert.Equal(t, "nginx", info.Labels["app"])
	assert.Equal(t, "prod", info.Labels["env"])
	assert.False(t, info.Created.IsZero())
}

func TestDockerRuntime_Inspect_NotFound(t *testing.T) {
	mock := newMockSSHExecutor()
	runtime := NewDockerRuntime(mock)

	mock.setCommand("docker inspect nonexistent", "[]")

	_, err := runtime.Inspect("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDockerRuntime_List(t *testing.T) {
	mock := newMockSSHExecutor()
	runtime := NewDockerRuntime(mock)

	listOutput := `{"ID":"abc123","Names":"container1","Image":"nginx:latest","State":"running","Status":"Up 5 minutes"}
{"ID":"def456","Names":"container2","Image":"redis:alpine","State":"exited","Status":"Exited (0) 2 hours ago"}`

	mock.setCommand("docker ps --format json", listOutput)

	containers, err := runtime.List(false)
	require.NoError(t, err)
	assert.Len(t, containers, 2)
	assert.Equal(t, "abc123", containers[0].ID)
	assert.Equal(t, "container1", containers[0].Name)
	assert.Equal(t, "nginx:latest", containers[0].Image)
	assert.Equal(t, "running", containers[0].State)
}

func TestDockerRuntime_List_Empty(t *testing.T) {
	mock := newMockSSHExecutor()
	runtime := NewDockerRuntime(mock)

	mock.setCommand("docker ps --format json", "")

	containers, err := runtime.List(false)
	require.NoError(t, err)
	assert.Empty(t, containers)
}

func TestDockerRuntime_List_All(t *testing.T) {
	mock := newMockSSHExecutor()
	runtime := NewDockerRuntime(mock)

	listOutput := `{"ID":"abc123","Names":"container1","Image":"nginx:latest","State":"running","Status":"Up 5 minutes"}`

	mock.setCommand("docker ps -a --format json", listOutput)

	containers, err := runtime.List(true)
	require.NoError(t, err)
	assert.Len(t, containers, 1)
}

func TestPodmanRuntime_Name(t *testing.T) {
	mock := newMockSSHExecutor()
	runtime := NewPodmanRuntime(mock)
	assert.Equal(t, "podman", runtime.Name())
}

func TestPodmanRuntime_IsAvailable(t *testing.T) {
	mock := newMockSSHExecutor()
	runtime := NewPodmanRuntime(mock)

	// Podman is available
	mock.setCommand("podman --version", "podman version 4.5.0")
	assert.True(t, runtime.IsAvailable())

	// Podman is not available
	mock2 := newMockSSHExecutor()
	mock2.setError("podman --version", fmt.Errorf("command not found"))
	runtime2 := NewPodmanRuntime(mock2)
	assert.False(t, runtime2.IsAvailable())
}

func TestPodmanRuntime_Pull(t *testing.T) {
	mock := newMockSSHExecutor()
	runtime := NewPodmanRuntime(mock)

	mock.setCommand("podman pull nginx:latest", "Trying to pull nginx:latest...\nGetting image source signatures")

	err := runtime.Pull("nginx:latest")
	assert.NoError(t, err)
}

func TestPodmanRuntime_Run_Simple(t *testing.T) {
	mock := newMockSSHExecutor()
	runtime := NewPodmanRuntime(mock)

	containerID := "xyz789abc123"
	mock.setCommand("podman run", containerID)

	config := RunConfig{
		Image: "nginx:latest",
	}

	id, err := runtime.Run(config)
	require.NoError(t, err)
	assert.Equal(t, containerID, id)
}

func TestPodmanRuntime_Run_Complex(t *testing.T) {
	mock := newMockSSHExecutor()
	runtime := NewPodmanRuntime(mock)

	containerID := "podman-complex"
	mock.setCommand("podman run", containerID)

	config := RunConfig{
		Image:         "nginx:latest",
		Name:          "my-nginx",
		Detach:        true,
		Remove:        true,
		Ports:         []string{"8080:80"},
		Volumes:       []string{"/data:/app"},
		Env:           []string{"ENV=prod"},
		Network:       "bridge",
		RestartPolicy: "always",
		User:          "nginx",
		WorkDir:       "/usr/share/nginx/html",
		Labels:        map[string]string{"version": "1.0"},
		Privileged:    false,
		Command:       []string{"nginx", "-g", "daemon off;"},
	}

	id, err := runtime.Run(config)
	require.NoError(t, err)
	assert.Equal(t, containerID, id)
}

func TestPodmanRuntime_Stop(t *testing.T) {
	mock := newMockSSHExecutor()
	runtime := NewPodmanRuntime(mock)

	mock.setCommand("podman stop", "xyz789")

	err := runtime.Stop("xyz789", 15*time.Second)
	assert.NoError(t, err)
}

func TestPodmanRuntime_Remove(t *testing.T) {
	mock := newMockSSHExecutor()
	runtime := NewPodmanRuntime(mock)

	// Remove without force
	mock.setCommand("podman rm xyz789", "xyz789")
	err := runtime.Remove("xyz789", false)
	assert.NoError(t, err)

	// Remove with force
	mock.setCommand("podman rm -f abc123", "abc123")
	err = runtime.Remove("abc123", true)
	assert.NoError(t, err)
}

func TestPodmanRuntime_Inspect(t *testing.T) {
	mock := newMockSSHExecutor()
	runtime := NewPodmanRuntime(mock)

	inspectOutput := `[{
		"Id": "xyz789",
		"Name": "my-podman-container",
		"Created": "2023-06-01T10:30:00.000000000Z",
		"Config": {
			"Image": "redis:alpine",
			"Labels": {
				"service": "cache"
			}
		},
		"State": {
			"Status": "running"
		}
	}]`

	mock.setCommand("podman inspect xyz789", inspectOutput)

	info, err := runtime.Inspect("xyz789")
	require.NoError(t, err)
	assert.Equal(t, "xyz789", info.ID)
	assert.Equal(t, "my-podman-container", info.Name)
	assert.Equal(t, "redis:alpine", info.Image)
	assert.Equal(t, "running", info.State)
	assert.Equal(t, "cache", info.Labels["service"])
}

func TestPodmanRuntime_List(t *testing.T) {
	mock := newMockSSHExecutor()
	runtime := NewPodmanRuntime(mock)

	containers := []map[string]interface{}{
		{
			"Id":     "xyz789",
			"Names":  []interface{}{"pod-container1"},
			"Image":  "nginx:latest",
			"State":  "running",
			"Status": "Up 10 minutes",
		},
		{
			"Id":     "abc123",
			"Names":  []interface{}{"pod-container2"},
			"Image":  "redis:alpine",
			"State":  "exited",
			"Status": "Exited (0) 1 hour ago",
		},
	}

	listOutput, _ := json.Marshal(containers)
	mock.setCommand("podman ps --format json", string(listOutput))

	result, err := runtime.List(false)
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, "xyz789", result[0].ID)
	assert.Equal(t, "pod-container1", result[0].Name)
	assert.Equal(t, "nginx:latest", result[0].Image)
	assert.Equal(t, "running", result[0].State)
}

func TestPodmanRuntime_List_Empty(t *testing.T) {
	mock := newMockSSHExecutor()
	runtime := NewPodmanRuntime(mock)

	mock.setCommand("podman ps --format json", "")

	containers, err := runtime.List(false)
	require.NoError(t, err)
	assert.Empty(t, containers)
}

func TestDetectRuntime_Docker(t *testing.T) {
	mock := newMockSSHExecutor()
	mock.setCommand("docker --version", "Docker version 24.0.0")
	mock.setError("podman --version", fmt.Errorf("command not found"))

	runtime, err := DetectRuntime(mock)
	require.NoError(t, err)
	assert.Equal(t, "docker", runtime.Name())
}

func TestDetectRuntime_Podman(t *testing.T) {
	mock := newMockSSHExecutor()
	mock.setError("docker --version", fmt.Errorf("command not found"))
	mock.setCommand("podman --version", "podman version 4.5.0")

	runtime, err := DetectRuntime(mock)
	require.NoError(t, err)
	assert.Equal(t, "podman", runtime.Name())
}

func TestDetectRuntime_PreferDocker(t *testing.T) {
	mock := newMockSSHExecutor()
	mock.setCommand("docker --version", "Docker version 24.0.0")
	mock.setCommand("podman --version", "podman version 4.5.0")

	runtime, err := DetectRuntime(mock)
	require.NoError(t, err)
	// Should prefer Docker when both are available
	assert.Equal(t, "docker", runtime.Name())
}

func TestDetectRuntime_NoneAvailable(t *testing.T) {
	mock := newMockSSHExecutor()
	mock.setError("docker --version", fmt.Errorf("command not found"))
	mock.setError("podman --version", fmt.Errorf("command not found"))

	_, err := DetectRuntime(mock)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no container runtime found")
}
