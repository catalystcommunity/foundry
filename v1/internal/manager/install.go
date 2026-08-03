// Package manager installs and contacts the external Foundry manager.
package manager

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/catalystcommunity/foundry/v1/internal/systemd"
	gossh "golang.org/x/crypto/ssh"
)

const serviceName = "foundry-manager"

var imagePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/:@-]*$`)
var pathPattern = regexp.MustCompile(`^/[A-Za-z0-9._/-]+$`)

// Executor runs commands on the management host.
type Executor interface {
	Execute(command string) (string, error)
}

// InstallInput contains all material needed to install the manager.
type InstallInput struct {
	Config    *config.ManagementConfig
	StackYAML []byte
	SSHKeys   map[string]*ssh.KeyPair
	Token     string
	Runtime   string
}

// Install creates or updates the systemd-managed external manager container.
// It returns the access token used by the manager.
func Install(executor Executor, input InstallInput) (string, error) {
	if input.Config == nil {
		return "", fmt.Errorf("management configuration is required")
	}
	if input.Config.Port < 1 || input.Config.Port > 65535 {
		return "", fmt.Errorf("manager port must be between 1 and 65535")
	}
	if !imagePattern.MatchString(input.Config.Image) || !imagePattern.MatchString(input.Config.Version) {
		return "", fmt.Errorf("manager image or version contains unsupported characters")
	}
	runtimeName := input.Runtime
	if runtimeName != "" && runtimeName != "docker" && runtimeName != "nerdctl" && runtimeName != "podman" {
		return "", fmt.Errorf("unsupported container runtime %q", runtimeName)
	}
	if input.Token == "" {
		var err error
		input.Token, err = newToken()
		if err != nil {
			return "", err
		}
	}

	dataPath := strings.TrimSuffix(input.Config.DataPath, "/")
	if !pathPattern.MatchString(dataPath) {
		return "", fmt.Errorf("manager data path contains unsupported characters")
	}
	runtimePath, err := findRuntime(executor, runtimeName)
	if err != nil {
		return "", err
	}
	commands := []string{
		fmt.Sprintf("sudo mkdir -p %s/keys", dataPath),
		fmt.Sprintf("sudo chown -R 65532:65532 %s", dataPath),
		writeBase64Command(dataPath+"/stack.yaml", input.StackYAML, "0600"),
		fmt.Sprintf("sudo test -s %s/admin.token || %s", dataPath, writeBase64Command(dataPath+"/admin.token", []byte(input.Token), "0600")),
	}
	for hostname, key := range input.SSHKeys {
		if key == nil || !safeName(hostname) {
			return "", fmt.Errorf("invalid SSH key entry for %q", hostname)
		}
		hostPath := dataPath + "/keys/" + hostname
		commands = append(commands,
			fmt.Sprintf("sudo mkdir -p %s && sudo chown 65532:65532 %s", hostPath, hostPath),
			writeBase64Command(hostPath+"/id_ed25519", key.Private, "0600"),
			writeBase64Command(hostPath+"/id_ed25519.pub", key.Public, "0644"),
		)
	}
	commands = append(commands, fmt.Sprintf("sudo %s pull %s:%s", runtimePath, input.Config.Image, input.Config.Version))
	for _, command := range commands {
		if _, err := executor.Execute(command); err != nil {
			return "", fmt.Errorf("prepare manager: %w", err)
		}
	}
	storedToken, err := executor.Execute("sudo cat " + dataPath + "/admin.token")
	if err != nil {
		return "", fmt.Errorf("read manager token: %w", err)
	}
	input.Token = strings.TrimSpace(storedToken)
	if len(input.Token) < 32 || strings.ContainsAny(input.Token, "\r\n\t ") {
		return "", fmt.Errorf("stored manager token is invalid")
	}

	execStart := fmt.Sprintf(
		"%s run --name %s --read-only --tmpfs /tmp:rw,noexec,nosuid,size=64m --security-opt no-new-privileges=true --cap-drop ALL -p 127.0.0.1:%d:8080 -e FOUNDRY_CONFIG_DIR=/var/lib/foundry -e FOUNDRY_MANAGER=1 -v %s:/var/lib/foundry %s:%s --config /var/lib/foundry/stack.yaml serve --listen 0.0.0.0:8080 --token-file /var/lib/foundry/admin.token",
		runtimePath, serviceName, input.Config.Port, dataPath, input.Config.Image, input.Config.Version,
	)
	unit := systemd.ContainerUnitFile(serviceName, "Foundry external management service", execStart)
	unit.ExecStartPre = fmt.Sprintf("-%s rm -f %s", runtimePath, serviceName)
	unit.ExecStopPost = fmt.Sprintf("-%s rm -f %s", runtimePath, serviceName)
	unit.TimeoutStopSec = 30
	if err := systemd.CreateService(executor, serviceName, unit); err != nil {
		return "", err
	}
	if err := systemd.EnableService(executor, serviceName); err != nil {
		return "", err
	}
	if _, err := executor.Execute("sudo systemctl restart " + serviceName); err != nil {
		return "", fmt.Errorf("restart manager: %w", err)
	}
	return input.Token, nil
}

func findRuntime(executor Executor, requested string) (string, error) {
	candidates := []string{requested}
	if requested == "" {
		candidates = []string{"docker", "nerdctl", "podman"}
	}
	for _, candidate := range candidates {
		output, err := executor.Execute("command -v " + candidate)
		path := strings.TrimSpace(output)
		if err == nil && pathPattern.MatchString(path) {
			return path, nil
		}
	}
	if requested != "" {
		return "", fmt.Errorf("container runtime %q is not available", requested)
	}
	return "", fmt.Errorf("no supported container runtime is available")
}

// Apply asks the installed manager to apply its current configuration and
// waits for the job to finish. All HTTP traffic stays inside the SSH channel.
func Apply(ctx context.Context, client *gossh.Client, port int, token string) error {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return client.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		},
	}
	httpClient := &http.Client{Transport: transport, Timeout: 30 * time.Second}
	defer transport.CloseIdleConnections()

	var job struct {
		ID      string `json:"id"`
		State   string `json:"state"`
		Message string `json:"message"`
	}
	if err := waitForManager(ctx, httpClient, token, &job); err != nil {
		return err
	}
	for {
		if job.State == "complete" {
			return nil
		}
		if job.State == "failed" {
			return fmt.Errorf("manager apply failed: %s", job.Message)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
		if err := managerRequest(ctx, httpClient, http.MethodGet, "/api/v1/jobs/"+job.ID, token, &job); err != nil {
			return err
		}
	}
}

// ReadStack returns the manager's current stack configuration.
func ReadStack(executor Executor, dataPath string) ([]byte, error) {
	path := strings.TrimSuffix(dataPath, "/")
	if !pathPattern.MatchString(path) {
		return nil, fmt.Errorf("manager data path contains unsupported characters")
	}
	output, err := executor.Execute("sudo cat " + path + "/stack.yaml")
	if err != nil {
		return nil, fmt.Errorf("read manager stack configuration: %w", err)
	}
	if strings.TrimSpace(output) == "" {
		return nil, fmt.Errorf("manager stack configuration is empty")
	}
	return []byte(output), nil
}

// ReadToken returns the manager's bootstrap access token.
func ReadToken(executor Executor, dataPath string) (string, error) {
	path := strings.TrimSuffix(dataPath, "/")
	if !pathPattern.MatchString(path) {
		return "", fmt.Errorf("manager data path contains unsupported characters")
	}
	output, err := executor.Execute("sudo cat " + path + "/admin.token")
	if err != nil {
		return "", fmt.Errorf("read manager access token: %w", err)
	}
	token := strings.TrimSpace(output)
	if len(token) < 32 || strings.ContainsAny(token, "\r\n\t ") {
		return "", fmt.Errorf("manager access token is invalid")
	}
	return token, nil
}

func waitForManager(ctx context.Context, client *http.Client, token string, job any) error {
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for {
		lastErr = managerRequest(ctx, client, http.MethodPost, "/api/v1/apply/current", token, job)
		if lastErr == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("manager did not become ready: %w", lastErr)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func managerRequest(ctx context.Context, client *http.Client, method, path, token string, target any) error {
	request, err := http.NewRequestWithContext(ctx, method, "http://foundry-manager"+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("contact manager: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("manager returned %s: %s", response.Status, strings.TrimSpace(string(body)))
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return fmt.Errorf("decode manager response: %w", err)
	}
	return nil
}

func writeBase64Command(path string, data []byte, mode string) string {
	encoded := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("echo %s | base64 -d | sudo tee %s >/dev/null && sudo chmod %s %s && sudo chown 65532:65532 %s", encoded, path, mode, path, path)
}

func safeName(value string) bool {
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '.' || char == '_' {
			continue
		}
		return false
	}
	return value != ""
}

func newToken() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate manager token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}
