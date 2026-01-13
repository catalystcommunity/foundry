package host

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/urfave/cli/v3"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// AddCommand adds a new host to the registry
var AddCommand = &cli.Command{
	Name:      "add",
	Usage:     "Add a new host to the registry",
	ArgsUsage: "[hostname]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "address",
			Aliases: []string{"a"},
			Usage:   "IP address or FQDN of the host",
		},
		&cli.IntFlag{
			Name:    "port",
			Aliases: []string{"p"},
			Usage:   "SSH port",
			Value:   22,
		},
		&cli.StringFlag{
			Name:    "user",
			Aliases: []string{"u"},
			Usage:   "SSH user",
		},
		&cli.StringFlag{
			Name:  "password",
			Usage: "SSH password (will prompt if not provided)",
		},
		&cli.StringSliceFlag{
			Name:    "roles",
			Aliases: []string{"r"},
			Usage:   "Roles for this host (openbao, dns, zot, cluster-control-plane, cluster-worker)",
		},
		&cli.BoolFlag{
			Name:  "non-interactive",
			Usage: "use flags instead of prompts",
		},
	},
	Action: runAdd,
}

func runAdd(ctx context.Context, cmd *cli.Command) error {
	// Initialize config-based host registry (--config flag inherited from root command)
	configPath, err := config.FindConfig(cmd.String("config"))
	if err != nil {
		return fmt.Errorf("failed to find config file: %w (run 'foundry config init' first)", err)
	}

	loader := config.NewHostConfigLoader(configPath)
	registry := host.NewConfigRegistry(configPath, loader)
	host.SetDefaultRegistry(registry)

	var (
		hostname string
		address  string
		port     int
		user     string
		password string
		roles    []string
	)

	// Get hostname from args or prompt
	if cmd.Args().Len() > 0 {
		hostname = cmd.Args().Get(0)
	} else if cmd.Bool("non-interactive") {
		return fmt.Errorf("hostname is required in non-interactive mode")
	} else {
		hostname, err = prompt("Hostname")
		if err != nil {
			return err
		}
	}

	// Check if host already exists
	exists, err := host.Exists(hostname)
	if err != nil {
		return fmt.Errorf("failed to check if host exists: %w", err)
	}
	if exists {
		return fmt.Errorf("host %s already exists in registry", hostname)
	}

	// Get address from flag or prompt
	if cmd.IsSet("address") {
		address = cmd.String("address")
	} else if cmd.Bool("non-interactive") {
		return fmt.Errorf("--address is required in non-interactive mode")
	} else {
		address, err = prompt("Address (IP or FQDN)")
		if err != nil {
			return err
		}
	}

	// Get port from flag or use default
	port = cmd.Int("port")
	if port == 0 {
		port = 22
	}

	// Get user from flag or prompt
	if cmd.IsSet("user") {
		user = cmd.String("user")
	} else if cmd.Bool("non-interactive") {
		return fmt.Errorf("--user is required in non-interactive mode")
	} else {
		user, err = prompt("SSH User")
		if err != nil {
			return err
		}
	}

	// Get password from flag or prompt
	if cmd.IsSet("password") {
		password = cmd.String("password")
	} else if cmd.Bool("non-interactive") {
		return fmt.Errorf("--password is required in non-interactive mode")
	} else {
		password, err = promptPassword("Password")
		if err != nil {
			return err
		}
	}

	// Get roles from flag or prompt
	if cmd.IsSet("roles") {
		roles = cmd.StringSlice("roles")
	} else if cmd.Bool("non-interactive") {
		// Default to all roles for single-host setup in non-interactive mode
		roles = []string{"openbao", "dns", "zot", "cluster-control-plane"}
	} else {
		roles, err = promptRoles()
		if err != nil {
			return err
		}
	}

	// Validate roles
	validRoles := map[string]bool{
		"openbao":                 true,
		"dns":                     true,
		"zot":                     true,
		"cluster-control-plane":   true,
		"cluster-worker":          true,
	}
	for _, role := range roles {
		if !validRoles[role] {
			return fmt.Errorf("invalid role: %s (valid roles: openbao, dns, zot, cluster-control-plane, cluster-worker)", role)
		}
	}

	// Test SSH connection with password
	fmt.Printf("Testing SSH connection to %s@%s:%d...\n", user, address, port)
	authMethod := gossh.Password(password)
	connOpts := &ssh.ConnectionOptions{
		Host:       address,
		Port:       port,
		User:       user,
		AuthMethod: authMethod,
		Timeout:    30,
	}

	conn, err := ssh.Connect(connOpts)
	if err != nil {
		return fmt.Errorf("failed to connect to host: %w", err)
	}
	defer conn.Close()

	fmt.Println("✓ SSH connection successful")

	// Generate SSH key pair
	fmt.Println("Generating SSH key pair...")
	keyPair, err := ssh.GenerateKeyPair()
	if err != nil {
		return fmt.Errorf("failed to generate key pair: %w", err)
	}
	fmt.Println("✓ SSH key pair generated")

	// Install public key on host
	fmt.Println("Installing public key on host...")
	if err := installPublicKey(conn, keyPair); err != nil {
		return fmt.Errorf("failed to install public key: %w", err)
	}
	fmt.Println("✓ Public key installed")

	// Store private key to filesystem
	fmt.Println("Storing SSH key...")
	keysDir, err := config.GetKeysDir()
	if err != nil {
		return fmt.Errorf("failed to get keys directory: %w", err)
	}

	keyStorage, err := ssh.NewFilesystemKeyStorage(keysDir)
	if err != nil {
		return fmt.Errorf("failed to create key storage: %w", err)
	}

	if err := keyStorage.Store(hostname, keyPair); err != nil {
		return fmt.Errorf("failed to store SSH key: %w", err)
	}
	fmt.Println("✓ SSH key stored")

	// Add host to registry
	h := &host.Host{
		Hostname:  hostname,
		Address:   address,
		Port:      port,
		User:      user,
		SSHKeySet: true,
		Roles:     roles,
		State:     "ssh-configured",
	}

	if err := host.Add(h); err != nil {
		return fmt.Errorf("failed to add host to registry: %w", err)
	}

	fmt.Printf("\n✓ Host %s added successfully\n", hostname)
	fmt.Printf("  Address: %s:%d\n", address, port)
	fmt.Printf("  User: %s\n", user)
	fmt.Printf("  SSH Key: configured\n")
	fmt.Printf("  Roles: %s\n", strings.Join(roles, ", "))
	fmt.Printf("  State: ssh-configured\n")

	return nil
}

// installPublicKey installs the public key on the remote host
func installPublicKey(conn *ssh.Connection, keyPair *ssh.KeyPair) error {
	// Ensure .ssh directory exists
	createDirCmd := "mkdir -p ~/.ssh && chmod 700 ~/.ssh"
	result, err := conn.Exec(createDirCmd)
	if err != nil {
		return fmt.Errorf("failed to create .ssh directory: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to create .ssh directory: %s", result.Stderr)
	}

	// Append public key to authorized_keys
	publicKey := strings.TrimSpace(keyPair.PublicKeyString())
	appendKeyCmd := fmt.Sprintf("echo '%s' >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys", publicKey)
	result, err = conn.Exec(appendKeyCmd)
	if err != nil {
		return fmt.Errorf("failed to append public key: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("failed to append public key: %s", result.Stderr)
	}

	return nil
}

// prompt prompts the user for input
func prompt(message string) (string, error) {
	fmt.Printf("%s: ", message)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(input), nil
}

// promptPassword prompts the user for a password (without echoing)
func promptPassword(message string) (string, error) {
	fmt.Printf("%s: ", message)

	// Read password without echoing to screen
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		return "", fmt.Errorf("failed to read password: %w", err)
	}

	fmt.Println() // Print newline after password input
	return string(password), nil
}

// promptRoles prompts the user to select roles for the host
func promptRoles() ([]string, error) {
	fmt.Println("\nSelect roles for this host (comma-separated):")
	fmt.Println("  1. openbao              - OpenBAO secrets management")
	fmt.Println("  2. dns                  - PowerDNS server")
	fmt.Println("  3. zot                  - Zot container registry")
	fmt.Println("  4. cluster-control-plane - Kubernetes control plane node")
	fmt.Println("  5. cluster-worker       - Kubernetes worker node")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  Single-host: openbao,dns,zot,cluster-control-plane")
	fmt.Println("  HA control-plane: openbao,dns,zot,cluster-control-plane")
	fmt.Println("  Worker only: cluster-worker")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Roles [openbao,dns,zot,cluster-control-plane]: ")
	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	input = strings.TrimSpace(input)

	// Default to single-host setup if empty
	if input == "" {
		return []string{"openbao", "dns", "zot", "cluster-control-plane"}, nil
	}

	// Split by comma and trim spaces
	roleStrs := strings.Split(input, ",")
	roles := make([]string, 0, len(roleStrs))
	for _, role := range roleStrs {
		trimmed := strings.TrimSpace(role)
		if trimmed != "" {
			roles = append(roles, trimmed)
		}
	}

	return roles, nil
}

// ResetRegistry resets the global registry (for testing)
func ResetRegistry() {
	host.SetDefaultRegistry(host.NewMemoryRegistry())
}
