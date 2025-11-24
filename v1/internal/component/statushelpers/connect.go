package statushelpers

import (
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/catalystcommunity/foundry/v1/internal/ssh"
)

// SSHExecutorAdapter adapts ssh.Connection to container.SSHExecutor interface
type SSHExecutorAdapter struct {
	Conn *ssh.Connection
}

func (a *SSHExecutorAdapter) Execute(cmd string) (string, error) {
	result, err := a.Conn.Exec(cmd)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return result.Stdout, fmt.Errorf("command failed with exit code %d: %s", result.ExitCode, result.Stderr)
	}
	return result.Stdout, nil
}

// FindHostByIP finds a host in the registry by IP address
func FindHostByIP(ip string) (*host.Host, error) {
	hosts, err := host.List()
	if err != nil {
		return nil, fmt.Errorf("failed to list hosts: %w", err)
	}

	for _, h := range hosts {
		if h.Address == ip {
			return h, nil
		}
	}

	return nil, fmt.Errorf("no host found with address %s", ip)
}

// FindHostByHostname finds a host in the registry by hostname
func FindHostByHostname(hostname string) (*host.Host, error) {
	h, err := host.Get(hostname)
	if err != nil {
		return nil, fmt.Errorf("host %s not found in registry: %w", hostname, err)
	}
	return h, nil
}

// ConnectToHost establishes an SSH connection to a host
// configDir is the directory containing SSH keys (from config.GetConfigDir())
func ConnectToHost(h *host.Host, configDir, clusterName string) (*ssh.Connection, error) {
	keyStorage, err := ssh.GetKeyStorage(configDir, clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to create key storage: %w", err)
	}

	keyPair, err := keyStorage.Load(h.Hostname)
	if err != nil {
		return nil, fmt.Errorf("SSH key not found for host %s: %w", h.Hostname, err)
	}

	// Create auth method from key pair
	authMethod, err := keyPair.AuthMethod()
	if err != nil {
		return nil, fmt.Errorf("failed to create auth method: %w", err)
	}

	// Connect to host
	connOpts := &ssh.ConnectionOptions{
		Host:       h.Address,
		Port:       h.Port,
		User:       h.User,
		AuthMethod: authMethod,
		Timeout:    30,
	}

	conn, err := ssh.Connect(connOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", h.Hostname, err)
	}

	return conn, nil
}
