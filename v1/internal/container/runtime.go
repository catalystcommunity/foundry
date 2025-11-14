package container

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SSHExecutor defines the interface for executing commands via SSH
// This matches the interface from internal/ssh package
type SSHExecutor interface {
	Execute(cmd string) (string, error)
}

// DockerRuntime implements the Runtime interface for Docker
type DockerRuntime struct {
	conn SSHExecutor
}

// NewDockerRuntime creates a new Docker runtime
func NewDockerRuntime(conn SSHExecutor) *DockerRuntime {
	return &DockerRuntime{conn: conn}
}

func (d *DockerRuntime) Name() string {
	return "docker"
}

func (d *DockerRuntime) IsAvailable() bool {
	_, err := d.conn.Execute("sudo docker --version")
	return err == nil
}

func (d *DockerRuntime) Pull(image string) error {
	_, err := d.conn.Execute(fmt.Sprintf("sudo docker pull %s", image))
	return err
}

func (d *DockerRuntime) Run(config RunConfig) (string, error) {
	args := []string{"sudo", "docker", "run"}

	if config.Detach {
		args = append(args, "-d")
	}

	if config.Remove {
		args = append(args, "--rm")
	}

	if config.Name != "" {
		args = append(args, "--name", config.Name)
	}

	for _, port := range config.Ports {
		args = append(args, "-p", port)
	}

	for _, volume := range config.Volumes {
		args = append(args, "-v", volume)
	}

	for _, env := range config.Env {
		args = append(args, "-e", env)
	}

	if config.Network != "" {
		args = append(args, "--network", config.Network)
	}

	if config.RestartPolicy != "" {
		args = append(args, "--restart", config.RestartPolicy)
	}

	if config.User != "" {
		args = append(args, "--user", config.User)
	}

	if config.WorkDir != "" {
		args = append(args, "--workdir", config.WorkDir)
	}

	for key, value := range config.Labels {
		args = append(args, "--label", fmt.Sprintf("%s=%s", key, value))
	}

	if config.Privileged {
		args = append(args, "--privileged")
	}

	args = append(args, config.Image)
	args = append(args, config.Command...)

	cmd := strings.Join(args, " ")
	output, err := d.conn.Execute(cmd)
	if err != nil {
		return "", err
	}

	// Docker returns the container ID on successful run
	return strings.TrimSpace(output), nil
}

func (d *DockerRuntime) Stop(containerID string, timeout time.Duration) error {
	seconds := int(timeout.Seconds())
	cmd := fmt.Sprintf("sudo docker stop --time %d %s", seconds, containerID)
	_, err := d.conn.Execute(cmd)
	return err
}

func (d *DockerRuntime) Remove(containerID string, force bool) error {
	cmd := fmt.Sprintf("sudo docker rm %s", containerID)
	if force {
		cmd = fmt.Sprintf("sudo docker rm -f %s", containerID)
	}
	_, err := d.conn.Execute(cmd)
	return err
}

func (d *DockerRuntime) Inspect(containerID string) (*ContainerInfo, error) {
	cmd := fmt.Sprintf("sudo docker inspect %s", containerID)
	output, err := d.conn.Execute(cmd)
	if err != nil {
		return nil, err
	}

	// Parse JSON output
	var inspectData []map[string]interface{}
	if err := json.Unmarshal([]byte(output), &inspectData); err != nil {
		return nil, fmt.Errorf("failed to parse docker inspect output: %w", err)
	}

	if len(inspectData) == 0 {
		return nil, fmt.Errorf("container %s not found", containerID)
	}

	data := inspectData[0]
	info := &ContainerInfo{
		ID:     containerID,
		Labels: make(map[string]string),
	}

	// Extract name
	if name, ok := data["Name"].(string); ok {
		info.Name = strings.TrimPrefix(name, "/")
	}

	// Extract image
	if config, ok := data["Config"].(map[string]interface{}); ok {
		if image, ok := config["Image"].(string); ok {
			info.Image = image
		}

		// Extract labels
		if labels, ok := config["Labels"].(map[string]interface{}); ok {
			for k, v := range labels {
				if str, ok := v.(string); ok {
					info.Labels[k] = str
				}
			}
		}
	}

	// Extract state
	if state, ok := data["State"].(map[string]interface{}); ok {
		if status, ok := state["Status"].(string); ok {
			info.State = status
		}
	}

	// Extract created time
	if created, ok := data["Created"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, created); err == nil {
			info.Created = t
		}
	}

	return info, nil
}

func (d *DockerRuntime) List(all bool) ([]ContainerInfo, error) {
	cmd := "sudo docker ps --format json"
	if all {
		cmd = "sudo docker ps -a --format json"
	}

	output, err := d.conn.Execute(cmd)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(output) == "" {
		return []ContainerInfo{}, nil
	}

	var containers []ContainerInfo
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(line), &data); err != nil {
			continue
		}

		info := ContainerInfo{
			Labels: make(map[string]string),
		}

		if id, ok := data["ID"].(string); ok {
			info.ID = id
		}
		if name, ok := data["Names"].(string); ok {
			info.Name = name
		}
		if image, ok := data["Image"].(string); ok {
			info.Image = image
		}
		if state, ok := data["State"].(string); ok {
			info.State = state
		}
		if status, ok := data["Status"].(string); ok {
			info.Status = status
		}

		containers = append(containers, info)
	}

	return containers, nil
}

// PodmanRuntime implements the Runtime interface for Podman
type PodmanRuntime struct {
	conn SSHExecutor
}

// NewPodmanRuntime creates a new Podman runtime
func NewPodmanRuntime(conn SSHExecutor) *PodmanRuntime {
	return &PodmanRuntime{conn: conn}
}

func (p *PodmanRuntime) Name() string {
	return "podman"
}

func (p *PodmanRuntime) IsAvailable() bool {
	_, err := p.conn.Execute("sudo podman --version")
	return err == nil
}

func (p *PodmanRuntime) Pull(image string) error {
	_, err := p.conn.Execute(fmt.Sprintf("sudo podman pull %s", image))
	return err
}

func (p *PodmanRuntime) Run(config RunConfig) (string, error) {
	args := []string{"sudo", "podman", "run"}

	if config.Detach {
		args = append(args, "-d")
	}

	if config.Remove {
		args = append(args, "--rm")
	}

	if config.Name != "" {
		args = append(args, "--name", config.Name)
	}

	for _, port := range config.Ports {
		args = append(args, "-p", port)
	}

	for _, volume := range config.Volumes {
		args = append(args, "-v", volume)
	}

	for _, env := range config.Env {
		args = append(args, "-e", env)
	}

	if config.Network != "" {
		args = append(args, "--network", config.Network)
	}

	if config.RestartPolicy != "" {
		args = append(args, "--restart", config.RestartPolicy)
	}

	if config.User != "" {
		args = append(args, "--user", config.User)
	}

	if config.WorkDir != "" {
		args = append(args, "--workdir", config.WorkDir)
	}

	for key, value := range config.Labels {
		args = append(args, "--label", fmt.Sprintf("%s=%s", key, value))
	}

	if config.Privileged {
		args = append(args, "--privileged")
	}

	args = append(args, config.Image)
	args = append(args, config.Command...)

	cmd := strings.Join(args, " ")
	output, err := p.conn.Execute(cmd)
	if err != nil {
		return "", err
	}

	// Podman returns the container ID on successful run
	return strings.TrimSpace(output), nil
}

func (p *PodmanRuntime) Stop(containerID string, timeout time.Duration) error {
	seconds := int(timeout.Seconds())
	cmd := fmt.Sprintf("sudo podman stop --time %d %s", seconds, containerID)
	_, err := p.conn.Execute(cmd)
	return err
}

func (p *PodmanRuntime) Remove(containerID string, force bool) error {
	cmd := fmt.Sprintf("sudo podman rm %s", containerID)
	if force {
		cmd = fmt.Sprintf("sudo podman rm -f %s", containerID)
	}
	_, err := p.conn.Execute(cmd)
	return err
}

func (p *PodmanRuntime) Inspect(containerID string) (*ContainerInfo, error) {
	cmd := fmt.Sprintf("sudo podman inspect %s", containerID)
	output, err := p.conn.Execute(cmd)
	if err != nil {
		return nil, err
	}

	// Parse JSON output (same format as Docker)
	var inspectData []map[string]interface{}
	if err := json.Unmarshal([]byte(output), &inspectData); err != nil {
		return nil, fmt.Errorf("failed to parse podman inspect output: %w", err)
	}

	if len(inspectData) == 0 {
		return nil, fmt.Errorf("container %s not found", containerID)
	}

	data := inspectData[0]
	info := &ContainerInfo{
		ID:     containerID,
		Labels: make(map[string]string),
	}

	// Extract name
	if name, ok := data["Name"].(string); ok {
		info.Name = strings.TrimPrefix(name, "/")
	}

	// Extract image
	if config, ok := data["Config"].(map[string]interface{}); ok {
		if image, ok := config["Image"].(string); ok {
			info.Image = image
		}

		// Extract labels
		if labels, ok := config["Labels"].(map[string]interface{}); ok {
			for k, v := range labels {
				if str, ok := v.(string); ok {
					info.Labels[k] = str
				}
			}
		}
	}

	// Extract state
	if state, ok := data["State"].(map[string]interface{}); ok {
		if status, ok := state["Status"].(string); ok {
			info.State = status
		}
	}

	// Extract created time
	if created, ok := data["Created"].(string); ok {
		if t, err := time.Parse(time.RFC3339Nano, created); err == nil {
			info.Created = t
		}
	}

	return info, nil
}

func (p *PodmanRuntime) List(all bool) ([]ContainerInfo, error) {
	cmd := "sudo podman ps --format json"
	if all {
		cmd = "sudo podman ps -a --format json"
	}

	output, err := p.conn.Execute(cmd)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(output) == "" {
		return []ContainerInfo{}, nil
	}

	// Podman returns a JSON array
	var data []map[string]interface{}
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		return nil, fmt.Errorf("failed to parse podman ps output: %w", err)
	}

	var containers []ContainerInfo
	for _, item := range data {
		info := ContainerInfo{
			Labels: make(map[string]string),
		}

		if id, ok := item["Id"].(string); ok {
			info.ID = id
		}
		if names, ok := item["Names"].([]interface{}); ok && len(names) > 0 {
			if name, ok := names[0].(string); ok {
				info.Name = name
			}
		}
		if image, ok := item["Image"].(string); ok {
			info.Image = image
		}
		if state, ok := item["State"].(string); ok {
			info.State = state
		}
		if status, ok := item["Status"].(string); ok {
			info.Status = status
		}

		containers = append(containers, info)
	}

	return containers, nil
}

// DetectRuntime attempts to detect which container runtime is available
// Returns the first available runtime, or an error if none are found
func DetectRuntime(conn SSHExecutor) (Runtime, error) {
	runtimes := []Runtime{
		NewDockerRuntime(conn),
		NewPodmanRuntime(conn),
	}

	for _, runtime := range runtimes {
		if runtime.IsAvailable() {
			return runtime, nil
		}
	}

	return nil, fmt.Errorf("no container runtime found (tried: docker, podman)")
}
