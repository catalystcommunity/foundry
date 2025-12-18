package dns

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"github.com/catalystcommunity/foundry/v1/internal/component"
	"github.com/catalystcommunity/foundry/v1/internal/container"
	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/catalystcommunity/foundry/v1/internal/systemd"
)

// sshExecutorAdapter adapts ssh.Connection to container.SSHExecutor interface
type sshExecutorAdapter struct {
	conn *ssh.Connection
}

func (a *sshExecutorAdapter) Execute(cmd string) (string, error) {
	result, err := a.conn.Exec(cmd)
	if err != nil {
		return "", err
	}
	if result.ExitCode != 0 {
		return result.Stdout, fmt.Errorf("command failed with exit code %d: %s", result.ExitCode, result.Stderr)
	}
	return result.Stdout, nil
}

const (
	defaultImageRegistry = "docker.io/powerdns"
	defaultAuthImage     = "pdns-auth-master" // PowerDNS authoritative server
	defaultRecursorImage = "pdns-recursor-master" // PowerDNS recursor
	defaultImageTag      = "latest" // Use latest tag by default
)

// Install implements the component.Component interface.
func (c *Component) Install(ctx context.Context, cfg component.ComponentConfig) error {
	// Extract DNS-specific config from ComponentConfig
	dnsConfig, host, err := configFromComponentConfig(cfg)
	if err != nil {
		return fmt.Errorf("invalid DNS config: %w", err)
	}

	// Validate required fields
	if host == nil {
		return fmt.Errorf("host connection is required")
	}
	if dnsConfig.APIKey == "" {
		return fmt.Errorf("API key is required")
	}

	// Generate API key if not provided
	if dnsConfig.APIKey == "" {
		apiKey, err := generateAPIKey()
		if err != nil {
			return fmt.Errorf("failed to generate API key: %w", err)
		}
		dnsConfig.APIKey = apiKey
	}

	// Create container runtime with adapter
	adapter := &sshExecutorAdapter{conn: host}
	runtime := container.NewDockerRuntime(adapter)

	// Create data and config directories
	if err := createDirectories(host, dnsConfig); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	// Generate and write config files
	if err := writeConfigFiles(host, dnsConfig); err != nil {
		return fmt.Errorf("failed to write config files: %w", err)
	}

	// Pull container images
	// Use image tag from config, or default to "latest"
	imageTag := dnsConfig.ImageTag
	if imageTag == "" || imageTag == "49" {
		imageTag = defaultImageTag
	}
	authImage := fmt.Sprintf("%s/%s:%s", defaultImageRegistry, defaultAuthImage, imageTag)
	recursorImage := fmt.Sprintf("%s/%s:%s", defaultImageRegistry, defaultRecursorImage, imageTag)

	if err := runtime.Pull(authImage); err != nil {
		return fmt.Errorf("failed to pull auth image: %w", err)
	}
	if err := runtime.Pull(recursorImage); err != nil {
		return fmt.Errorf("failed to pull recursor image: %w", err)
	}

	// Initialize the database if using gsqlite3 backend
	if dnsConfig.Backend == "gsqlite3" {
		if err := initializeDatabase(host, dnsConfig, authImage); err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
	}

	// Create systemd services
	if err := createSystemdServices(host, dnsConfig, authImage, recursorImage); err != nil {
		return fmt.Errorf("failed to create systemd services: %w", err)
	}

	// Enable and start services
	if err := enableAndStartServices(host); err != nil {
		return fmt.Errorf("failed to start services: %w", err)
	}

	return nil
}

// configFromComponentConfig extracts DNS-specific config from generic ComponentConfig.
func configFromComponentConfig(cfg component.ComponentConfig) (*Config, *ssh.Connection, error) {
	dnsConfig := DefaultConfig()

	// Extract Host (runtime-only, not part of persisted config)
	// Try both "host" and "ssh_conn" keys for backward compatibility
	var host *ssh.Connection
	if h, ok := cfg["host"].(*ssh.Connection); ok {
		host = h
	} else if h, ok := cfg["ssh_conn"].(*ssh.Connection); ok {
		host = h
	}

	// Extract ImageTag
	if tag, ok := cfg["image_tag"].(string); ok && tag != "" {
		dnsConfig.ImageTag = tag
	}

	// Extract APIKey
	if key, ok := cfg["api_key"].(string); ok {
		dnsConfig.APIKey = key
	}

	// Extract Forwarders
	if fwds, ok := cfg["forwarders"].([]string); ok && len(fwds) > 0 {
		dnsConfig.Forwarders = fwds
	}

	// Extract LocalZones
	if zones, ok := cfg["local_zones"].([]string); ok && len(zones) > 0 {
		dnsConfig.LocalZones = zones
	}

	// Extract Backend
	if backend, ok := cfg["backend"].(string); ok && backend != "" {
		dnsConfig.Backend = backend
	}

	// Extract DataDir
	if dataDir, ok := cfg["data_dir"].(string); ok && dataDir != "" {
		dnsConfig.DataDir = dataDir
	}

	// Extract ConfigDir
	if configDir, ok := cfg["config_dir"].(string); ok && configDir != "" {
		dnsConfig.ConfigDir = configDir
	}

	return dnsConfig, host, nil
}

// generateAPIKey generates a secure random API key.
func generateAPIKey() (string, error) {
	bytes := make([]byte, 32) // 256 bits
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// createDirectories creates the data and config directories on the remote host.
func createDirectories(conn *ssh.Connection, cfg *Config) error {
	dirs := []string{
		cfg.DataDir,
		cfg.ConfigDir,
		filepath.Join(cfg.ConfigDir, "auth"),
		filepath.Join(cfg.ConfigDir, "recursor"),
	}

	adapter := &sshExecutorAdapter{conn: conn}
	for _, dir := range dirs {
		cmd := fmt.Sprintf("sudo mkdir -p %s && sudo chmod 755 %s", dir, dir)
		if _, err := adapter.Execute(cmd); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// writeConfigFiles writes the PowerDNS config files to the remote host.
func writeConfigFiles(conn *ssh.Connection, cfg *Config) error {
	// Generate auth config
	authConfig, err := GenerateAuthConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to generate auth config: %w", err)
	}

	// Generate recursor config
	recursorConfig, err := GenerateRecursorConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to generate recursor config: %w", err)
	}

	// Write auth config
	authConfigPath := filepath.Join(cfg.ConfigDir, "auth", "pdns.conf")
	if err := writeRemoteFile(conn, authConfigPath, authConfig); err != nil {
		return fmt.Errorf("failed to write auth config: %w", err)
	}

	// Write recursor config
	recursorConfigPath := filepath.Join(cfg.ConfigDir, "recursor", "recursor.conf")
	if err := writeRemoteFile(conn, recursorConfigPath, recursorConfig); err != nil {
		return fmt.Errorf("failed to write recursor config: %w", err)
	}

	return nil
}

// writeRemoteFile writes content to a file on the remote host.
func writeRemoteFile(conn *ssh.Connection, path, content string) error {
	adapter := &sshExecutorAdapter{conn: conn}

	// Use a heredoc to write the file
	cmd := fmt.Sprintf("sudo tee %s > /dev/null <<'FOUNDRY_EOF'\n%s\nFOUNDRY_EOF", path, content)
	if _, err := adapter.Execute(cmd); err != nil {
		return fmt.Errorf("failed to write file %s: %w", path, err)
	}

	// Set permissions
	chmodCmd := fmt.Sprintf("sudo chmod 644 %s", path)
	if _, err := adapter.Execute(chmodCmd); err != nil {
		return fmt.Errorf("failed to set permissions on %s: %w", path, err)
	}

	return nil
}

// createSystemdServices creates systemd service files for PowerDNS.
func createSystemdServices(conn *ssh.Connection, cfg *Config, authImage, recursorImage string) error {
	adapter := &sshExecutorAdapter{conn: conn}

	// Create auth service
	// Use the same pattern as OpenBAO for proper container lifecycle management:
	// - ExecStartPre cleans up existing container before starting
	// - No ExecStop - systemd sends SIGTERM to docker process which forwards to container
	// - ExecStopPost cleans up the container after stopping
	// - TimeoutStopSec gives container time to gracefully shutdown
	authUnit := systemd.UnitFile{
		Description:    "PowerDNS Authoritative Server (Foundry)",
		After:          []string{"network.target"},
		Wants:          []string{"network.target"},
		Type:           "simple",
		Restart:        "always",
		ExecStartPre:   "-docker rm -f powerdns-auth",
		ExecStart:      buildAuthExecStart(authImage, cfg),
		ExecStopPost:   "-docker rm -f powerdns-auth",
		TimeoutStopSec: 30,
		WantedBy:       []string{"multi-user.target"},
	}

	if err := systemd.CreateService(adapter, "powerdns-auth", &authUnit); err != nil {
		return fmt.Errorf("failed to create auth service: %w", err)
	}

	// Create recursor service
	recursorUnit := systemd.UnitFile{
		Description:    "PowerDNS Recursor (Foundry)",
		After:          []string{"network.target"},
		Wants:          []string{"network.target"},
		Type:           "simple",
		Restart:        "always",
		ExecStartPre:   "-docker rm -f powerdns-recursor",
		ExecStart:      buildRecursorExecStart(recursorImage, cfg),
		ExecStopPost:   "-docker rm -f powerdns-recursor",
		TimeoutStopSec: 30,
		WantedBy:       []string{"multi-user.target"},
	}

	if err := systemd.CreateService(adapter, "powerdns-recursor", &recursorUnit); err != nil {
		return fmt.Errorf("failed to create recursor service: %w", err)
	}

	return nil
}

// buildAuthExecStart builds the ExecStart command for the auth service.
func buildAuthExecStart(image string, cfg *Config) string {
	// Note: No --rm flag - systemd manages the container lifecycle via ExecStartPre/ExecStopPost
	// --security-opt apparmor=unconfined: nerdctl-default profile blocks runc signal operations
	return fmt.Sprintf(
		"docker run --name powerdns-auth "+
			"--security-opt apparmor=unconfined "+
			"--network=host "+
			"-v %s/auth:/etc/powerdns "+
			"-v %s:/var/lib/powerdns "+
			"%s "+
			"--config-dir=/etc/powerdns",
		cfg.ConfigDir,
		cfg.DataDir,
		image,
	)
}

// buildRecursorExecStart builds the ExecStart command for the recursor service.
func buildRecursorExecStart(image string, cfg *Config) string {
	// Note: No --rm flag - systemd manages the container lifecycle via ExecStartPre/ExecStopPost
	// --security-opt apparmor=unconfined: nerdctl-default profile blocks runc signal operations
	return fmt.Sprintf(
		"docker run --name powerdns-recursor "+
			"--security-opt apparmor=unconfined "+
			"--user root "+
			"--network=host "+
			"-v %s/recursor:/etc/powerdns-recursor "+
			"%s "+
			"--config-dir=/etc/powerdns-recursor",
		cfg.ConfigDir,
		image,
	)
}

// initializeDatabase initializes the SQLite database for PowerDNS.
func initializeDatabase(conn *ssh.Connection, cfg *Config, authImage string) error {
	adapter := &sshExecutorAdapter{conn: conn}

	// Check if database already exists
	dbPath := filepath.Join(cfg.DataDir, "pdns.db")
	checkCmd := fmt.Sprintf("sudo test -f %s", dbPath)
	_, err := adapter.Execute(checkCmd)
	if err == nil {
		// Database already exists, skip initialization
		return nil
	}

	// SQL schema for PowerDNS SQLite backend
	schema := `CREATE TABLE domains (
  id INTEGER PRIMARY KEY,
  name VARCHAR(255) NOT NULL COLLATE NOCASE,
  master VARCHAR(128) DEFAULT NULL,
  last_check INTEGER DEFAULT NULL,
  type VARCHAR(8) NOT NULL,
  notified_serial INTEGER DEFAULT NULL,
  account VARCHAR(40) DEFAULT NULL,
  options VARCHAR(64000) DEFAULT NULL,
  catalog VARCHAR(255) DEFAULT NULL
);
CREATE UNIQUE INDEX name_index ON domains(name);
CREATE INDEX catalog_idx ON domains(catalog);
CREATE TABLE records (
  id INTEGER PRIMARY KEY,
  domain_id INTEGER DEFAULT NULL,
  name VARCHAR(255) DEFAULT NULL,
  type VARCHAR(10) DEFAULT NULL,
  content VARCHAR(64000) DEFAULT NULL,
  ttl INTEGER DEFAULT NULL,
  prio INTEGER DEFAULT NULL,
  disabled BOOLEAN DEFAULT 0,
  ordername VARCHAR(255),
  auth BOOL DEFAULT 1,
  FOREIGN KEY(domain_id) REFERENCES domains(id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX rec_name_index ON records(name);
CREATE INDEX nametype_index ON records(name,type);
CREATE INDEX domain_id ON records(domain_id);
CREATE INDEX orderindex ON records(ordername);
CREATE TABLE supermasters (
  ip VARCHAR(64) NOT NULL,
  nameserver VARCHAR(255) NOT NULL COLLATE NOCASE,
  account VARCHAR(40) NOT NULL
);
CREATE UNIQUE INDEX ip_nameserver_pk ON supermasters(ip, nameserver);
CREATE TABLE comments (
  id INTEGER PRIMARY KEY,
  domain_id INTEGER NOT NULL,
  name VARCHAR(255) NOT NULL,
  type VARCHAR(10) NOT NULL,
  modified_at INT NOT NULL,
  account VARCHAR(40) DEFAULT NULL,
  comment VARCHAR(64000) NOT NULL,
  FOREIGN KEY(domain_id) REFERENCES domains(id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX comments_domain_id_index ON comments (domain_id);
CREATE INDEX comments_nametype_index ON comments (name, type);
CREATE INDEX comments_order_idx ON comments (domain_id, modified_at);
CREATE TABLE domainmetadata (
 id INTEGER PRIMARY KEY,
 domain_id INT NOT NULL,
 kind VARCHAR(32) COLLATE NOCASE,
 content TEXT,
 FOREIGN KEY(domain_id) REFERENCES domains(id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX domainmetaidindex ON domainmetadata(domain_id);
CREATE TABLE cryptokeys (
 id INTEGER PRIMARY KEY,
 domain_id INT NOT NULL,
 flags INT NOT NULL,
 active BOOL,
 published BOOL DEFAULT 1,
 content TEXT,
 FOREIGN KEY(domain_id) REFERENCES domains(id) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX domainidindex ON cryptokeys(domain_id);
CREATE TABLE tsigkeys (
 id INTEGER PRIMARY KEY,
 name VARCHAR(255) COLLATE NOCASE,
 algorithm VARCHAR(50) COLLATE NOCASE,
 secret VARCHAR(255)
);
CREATE UNIQUE INDEX namealgoindex ON tsigkeys(name, algorithm);`

	// Write schema to temp file on remote host
	schemaPath := "/tmp/pdns_schema.sql"
	if err := writeRemoteFile(conn, schemaPath, schema); err != nil {
		return fmt.Errorf("failed to write schema file: %w", err)
	}

	// Initialize database using sqlite3 command in the PowerDNS container
	// Use --user root to ensure proper permissions
	initCmd := fmt.Sprintf(
		"sudo docker run --rm --user root -v %s:%s -v /tmp:/tmp --entrypoint /bin/sh %s -c 'sqlite3 %s < %s'",
		cfg.DataDir, cfg.DataDir,
		authImage,
		dbPath,
		schemaPath,
	)

	if _, err := adapter.Execute(initCmd); err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	// Set proper ownership and permissions for database file and directory
	// PowerDNS container runs as UID 953 (pdns user)
	// Directory: 755 (rwxr-xr-x), Database: 644 (rw-r--r--)
	permCmd := fmt.Sprintf("sudo chown -R 953:953 %s && sudo chmod 755 %s && sudo chmod 644 %s", cfg.DataDir, cfg.DataDir, dbPath)
	if _, err := adapter.Execute(permCmd); err != nil {
		return fmt.Errorf("failed to set database permissions: %w", err)
	}

	// Clean up temp schema file
	cleanupCmd := fmt.Sprintf("sudo rm -f %s", schemaPath)
	_, _ = adapter.Execute(cleanupCmd) // Ignore cleanup errors

	return nil
}

// enableAndStartServices enables and starts the PowerDNS systemd services.
func enableAndStartServices(conn *ssh.Connection) error {
	adapter := &sshExecutorAdapter{conn: conn}
	services := []string{"powerdns-auth", "powerdns-recursor"}

	for _, svc := range services {
		// Enable service
		if err := systemd.EnableService(adapter, svc); err != nil {
			return fmt.Errorf("failed to enable %s: %w", svc, err)
		}

		// Start service
		if err := systemd.StartService(adapter, svc); err != nil {
			return fmt.Errorf("failed to start %s: %w", svc, err)
		}
	}

	return nil
}
