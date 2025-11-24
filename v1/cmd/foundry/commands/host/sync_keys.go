package host

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/urfave/cli/v3"
	gossh "golang.org/x/crypto/ssh"
)

// SyncKeysCommand re-installs SSH keys to a host
var SyncKeysCommand = &cli.Command{
	Name:      "sync-keys",
	Usage:     "Re-install SSH keys to a host",
	ArgsUsage: "<hostname>",
	Description: `Re-install SSH keys to a host when keys are out of sync.

This command is useful when:
  - A host was reset/reimaged but you still have the SSH keys
  - The authorized_keys file was accidentally deleted
  - You need to repair SSH key authentication

The command will:
  1. Load the existing SSH key pair from local storage
  2. Prompt for the user's password
  3. Connect to the host with password authentication
  4. Re-install the public key to authorized_keys
  5. Verify key-based authentication works

Example:
  foundry host sync-keys myhost`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "password",
			Usage: "SSH password (will prompt if not provided)",
		},
	},
	Action: runSyncKeys,
}

func runSyncKeys(ctx context.Context, cmd *cli.Command) error {
	// Get hostname from args
	if cmd.Args().Len() == 0 {
		return fmt.Errorf("hostname is required\n\nUsage: foundry host sync-keys <hostname>")
	}
	hostname := cmd.Args().Get(0)

	// Get host from registry
	h, err := host.Get(hostname)
	if err != nil {
		return fmt.Errorf("host not found: %w\n\nUse 'foundry host list' to see registered hosts", err)
	}

	// Load existing SSH key pair
	fmt.Printf("Loading SSH keys for %s...\n", hostname)
	keysDir, err := config.GetKeysDir()
	if err != nil {
		return fmt.Errorf("failed to get keys directory: %w", err)
	}

	keyStorage, err := ssh.NewFilesystemKeyStorage(keysDir)
	if err != nil {
		return fmt.Errorf("failed to create key storage: %w", err)
	}

	keyPair, err := keyStorage.Load(hostname)
	if err != nil {
		return fmt.Errorf("failed to load SSH keys: %w\n\nThe keys for this host may not exist. Use 'foundry host add' to create new keys.", err)
	}
	fmt.Println("✓ SSH keys loaded")

	// Get password
	var password string
	if cmd.IsSet("password") {
		password = cmd.String("password")
	} else {
		password, err = promptPassword("SSH Password")
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
	}

	// Connect with password authentication
	fmt.Printf("Connecting to %s@%s:%d with password...\n", h.User, h.Address, h.Port)
	authMethod := gossh.Password(password)
	connOpts := &ssh.ConnectionOptions{
		Host:       h.Address,
		Port:       h.Port,
		User:       h.User,
		AuthMethod: authMethod,
		Timeout:    30,
	}

	conn, err := ssh.Connect(connOpts)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()
	fmt.Println("✓ Connected with password")

	// Install public key
	fmt.Println("Installing public key on host...")
	if err := installPublicKey(conn, keyPair); err != nil {
		return fmt.Errorf("failed to install public key: %w", err)
	}
	fmt.Println("✓ Public key installed")

	// Verify key-based authentication works
	fmt.Println("Verifying key-based authentication...")
	conn.Close() // Close password connection

	// Try connecting with key
	keyAuthMethod, err := keyPair.AuthMethod()
	if err != nil {
		return fmt.Errorf("failed to create auth method from key: %w", err)
	}

	connOpts.AuthMethod = keyAuthMethod
	conn, err = ssh.Connect(connOpts)
	if err != nil {
		return fmt.Errorf("key-based authentication failed: %w\n\nThe key was installed but authentication didn't work.", err)
	}
	conn.Close()
	fmt.Println("✓ Key-based authentication verified")

	fmt.Printf("\n✓ SSH keys synced successfully for %s\n", hostname)
	fmt.Println("\nYou can now use commands like 'foundry host configure' without a password.")

	return nil
}
