package host

import (
	"context"
	"fmt"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/container"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/catalystcommunity/foundry/v1/internal/sudo"
	"github.com/urfave/cli/v3"
	"golang.org/x/term"
)

// ConfigureCommand configures a host with basic setup
var ConfigureCommand = &cli.Command{
	Name:      "configure",
	Usage:     "Run basic configuration on a host",
	ArgsUsage: "<hostname>",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Usage:   "Path to configuration file",
			Sources: cli.EnvVars("FOUNDRY_CONFIG"),
		},
		&cli.BoolFlag{
			Name:  "skip-update",
			Usage: "skip package updates",
		},
		&cli.BoolFlag{
			Name:  "skip-tools",
			Usage: "skip installing common tools",
		},
		&cli.BoolFlag{
			Name:  "skip-runtime",
			Usage: "skip installing container runtime (Docker/containerd)",
		},
	},
	Action: runConfigure,
}

func runConfigure(ctx context.Context, cmd *cli.Command) error {
	// Get hostname from args
	if cmd.Args().Len() == 0 {
		return fmt.Errorf("hostname is required")
	}
	hostname := cmd.Args().Get(0)

	// Get host from registry
	h, err := host.Get(hostname)
	if err != nil {
		return fmt.Errorf("host not found: %w", err)
	}

	// Load config to get cluster name
	configPath := cmd.String("config")
	if configPath == "" {
		var err error
		configPath, err = config.FindConfig("")
		if err != nil {
			return fmt.Errorf("failed to find config: %w", err)
		}
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Get SSH key from storage (prefers OpenBAO, falls back to filesystem)
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	keyStorage, err := ssh.GetKeyStorage(configDir, cfg.Cluster.Name)
	if err != nil {
		return fmt.Errorf("failed to create key storage: %w", err)
	}

	keyPair, err := keyStorage.Load(hostname)
	if err != nil {
		return fmt.Errorf("SSH key not found for host %s: %w", hostname, err)
	}

	// Create auth method from key pair
	authMethod, err := keyPair.AuthMethod()
	if err != nil {
		return fmt.Errorf("failed to create auth method: %w", err)
	}

	// Connect to host
	fmt.Printf("Connecting to %s...\n", hostname)
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
	fmt.Println("✓ Connected")

	// Create executor adapter for sudo package
	executor := &sudoCommandExecutor{conn: conn}

	// Check sudo status
	fmt.Println()
	fmt.Println("Checking sudo access...")
	sudoStatus, err := sudo.GetSudoStatus(executor)
	if err != nil {
		return fmt.Errorf("failed to check sudo access: %w", err)
	}

	switch sudoStatus {
	case sudo.SudoPasswordless:
		fmt.Println("✓ Passwordless sudo already configured")

	case sudo.SudoRequiresPassword:
		fmt.Printf("User '%s' has sudo access but requires a password\n", h.User)
		fmt.Println("Foundry requires passwordless sudo for automated operations")
		fmt.Println()
		fmt.Println("To configure passwordless sudo, we need to run commands as root.")
		fmt.Print("Enter root password: ")

		passwordBytes, err := term.ReadPassword(int(0))
		fmt.Println()
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		rootPassword := string(passwordBytes)

		fmt.Println()
		fmt.Println("Configuring passwordless sudo...")
		if err := sudo.SetupSudo(executor, h.User, rootPassword); err != nil {
			return fmt.Errorf("failed to setup sudo: %w", err)
		}
		fmt.Println("  ✓ Passwordless sudo configured for user '" + h.User + "'")

	case sudo.SudoNoAccess:
		fmt.Printf("User '%s' is not in the sudoers file\n", h.User)
		fmt.Println("Foundry requires passwordless sudo for automated operations")
		fmt.Println()
		fmt.Println("To add user to sudoers, we need to run commands as root.")
		fmt.Print("Enter root password: ")

		passwordBytes, err := term.ReadPassword(int(0))
		fmt.Println()
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		rootPassword := string(passwordBytes)

		fmt.Println()
		fmt.Println("Adding user to sudoers with passwordless access...")
		if err := sudo.SetupSudo(executor, h.User, rootPassword); err != nil {
			return fmt.Errorf("failed to setup sudo: %w", err)
		}
		fmt.Println("  ✓ User '" + h.User + "' added to sudoers")
		fmt.Println("  ✓ Passwordless sudo configured")

	case sudo.SudoNotInstalled:
		fmt.Println("sudo is not installed on this host")
		fmt.Println("Foundry requires passwordless sudo for automated operations")
		fmt.Println()
		fmt.Println("To install sudo and configure access, we need to run commands as root.")
		fmt.Print("Enter root password: ")

		passwordBytes, err := term.ReadPassword(int(0))
		fmt.Println()
		if err != nil {
			return fmt.Errorf("failed to read password: %w", err)
		}
		rootPassword := string(passwordBytes)

		fmt.Println()
		fmt.Println("Installing sudo and configuring passwordless access...")
		if err := sudo.SetupSudo(executor, h.User, rootPassword); err != nil {
			return fmt.Errorf("failed to setup sudo: %w", err)
		}
		fmt.Println("  ✓ sudo package installed")
		fmt.Println("  ✓ User '" + h.User + "' added to sudoers")
		fmt.Println("  ✓ Passwordless sudo configured")
	}

	// Build list of configuration commands
	var commands []ConfigStep

	if !cmd.Bool("skip-update") {
		commands = append(commands, ConfigStep{
			Name:        "Update package lists",
			Description: "Updating package lists...",
			Commands: []string{
				"sudo apt-get update -qq",
			},
		})
	}

	if !cmd.Bool("skip-tools") {
		commands = append(commands, ConfigStep{
			Name:        "Install common tools",
			Description: "Installing common tools (curl, git, vim, htop)...",
			Commands: []string{
				"sudo apt-get install -y -qq curl git vim htop",
			},
		})
	}

	// Always configure time sync
	commands = append(commands, ConfigStep{
		Name:        "Configure time synchronization",
		Description: "Configuring time synchronization...",
		Commands: []string{
			"sudo timedatectl set-ntp true || echo 'NTP configuration skipped (may not be supported)'",
		},
	})

	// Create dedicated system user for containers
	commands = append(commands, ConfigStep{
		Name:        "Create container system user",
		Description: "Creating foundrysys system user...",
		Commands: []string{
			// Create group with GID 374 if it doesn't exist
			"sudo groupadd -g 374 foundrysys 2>/dev/null || true",
			// Create user with UID 374 if it doesn't exist
			"sudo useradd -r -u 374 -g 374 -s /usr/sbin/nologin -M foundrysys 2>/dev/null || true",
		},
	})

	// Install container runtime if not skipped
	var needsRuntimeInstall bool
	if !cmd.Bool("skip-runtime") {
		needsRuntimeInstall = true
	}

	// Calculate total steps
	totalSteps := len(commands)
	if needsRuntimeInstall {
		totalSteps++
	}

	// Execute configuration steps
	fmt.Printf("\nRunning configuration steps on %s...\n\n", hostname)

	for i, step := range commands {
		fmt.Printf("[%d/%d] %s\n", i+1, totalSteps, step.Description)

		for _, cmdStr := range step.Commands {
			result, err := conn.Exec(cmdStr)
			if err != nil {
				fmt.Printf("  ✗ Failed: %v\n", err)
				return fmt.Errorf("configuration step '%s' failed: %w", step.Name, err)
			}

			if result.ExitCode != 0 {
				// Some commands may fail gracefully (like NTP), so we just warn
				if result.Stderr != "" {
					fmt.Printf("  ⚠ Warning: %s\n", result.Stderr)
				}
			}
		}

		fmt.Printf("  ✓ %s complete\n", step.Name)
	}

	// Install container runtime if needed
	if needsRuntimeInstall {
		fmt.Println()
		fmt.Printf("[%d/%d] Installing container runtime...\n", totalSteps, totalSteps)

		// Create adapter for container package
		executor := &sshCommandExecutor{conn: conn}

		// Detect what's currently installed
		runtimeType := container.DetectRuntimeInstallation(executor)

		switch runtimeType {
		case container.RuntimeDocker:
			fmt.Println("  ✓ Container runtime already installed (Docker)")
		case container.RuntimeNerdctl:
			fmt.Println("  ✓ Container runtime already installed (nerdctl)")
		case container.RuntimeNerdctlIncomplete:
			fmt.Println("  Detected incomplete nerdctl installation, completing setup...")
			if err := container.InstallRuntime(executor, h.User); err != nil {
				return fmt.Errorf("failed to complete container runtime installation: %w", err)
			}
			fmt.Println("  ✓ CNI plugins installed")
			fmt.Println("  ✓ Container runtime installation completed")
		case container.RuntimeNone:
			fmt.Println("  Installing containerd + nerdctl + CNI plugins...")
			if err := container.InstallRuntime(executor, h.User); err != nil {
				return fmt.Errorf("failed to install container runtime: %w", err)
			}
			fmt.Println("  ✓ Container runtime installed (containerd + nerdctl)")
			fmt.Println("  ℹ 'docker' command is now available (symlinked to nerdctl)")
		}

		// Verify installation works
		if runtimeType == container.RuntimeNone || runtimeType == container.RuntimeNerdctlIncomplete {
			fmt.Println("  Verifying container runtime...")
			if err := container.VerifyRuntimeInstalled(executor); err != nil {
				return fmt.Errorf("container runtime verification failed: %w", err)
			}
			fmt.Println("  ✓ Container runtime verified")
		}

		// Fix AppArmor for container signal handling (Ubuntu/Debian bug)
		// This must happen AFTER runc is installed
		fmt.Println("  Checking for AppArmor configuration...")
		if err := fixAppArmorForContainers(conn); err != nil {
			// Don't fail the whole configuration if AppArmor fix fails
			fmt.Printf("  ⚠ Warning: Failed to configure AppArmor: %v\n", err)
			fmt.Println("  Containers may have issues stopping gracefully")
		} else {
			fmt.Println("  ✓ AppArmor configured for containers")
		}
	}

	fmt.Printf("\n✓ Configuration complete for %s\n", hostname)
	return nil
}

// ConfigStep represents a configuration step
type ConfigStep struct {
	Name        string   // Name of the step
	Description string   // Description shown to user
	Commands    []string // Commands to execute
}

// sshCommandExecutor adapts ssh.Connection to container.CommandExecutor interface
type sshCommandExecutor struct {
	conn *ssh.Connection
}

func (e *sshCommandExecutor) Exec(cmd string) (*container.ExecResult, error) {
	result, err := e.conn.Exec(cmd)
	if err != nil {
		return nil, err
	}
	// Convert ssh.ExecResult to container.ExecResult
	return &container.ExecResult{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
	}, nil
}

// sudoCommandExecutor adapts ssh.Connection to sudo.CommandExecutor interface
type sudoCommandExecutor struct {
	conn *ssh.Connection
}

func (e *sudoCommandExecutor) Exec(cmd string) (*sudo.ExecResult, error) {
	result, err := e.conn.Exec(cmd)
	if err != nil {
		return nil, err
	}
	// Convert ssh.ExecResult to sudo.ExecResult
	return &sudo.ExecResult{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
	}, nil
}

// sshExecutor is an interface for executing commands over SSH
type sshExecutor interface {
	Exec(command string) (*ssh.ExecResult, error)
}

// fixAppArmorForContainers fixes the AppArmor profile to allow container signal handling
// This addresses a known Ubuntu/Debian bug where runc cannot signal container processes
// See: https://github.com/moby/moby/issues/47720
func fixAppArmorForContainers(conn sshExecutor) error {
	// Check if AppArmor is enabled
	result, err := conn.Exec("sudo apparmor_status --enabled 2>/dev/null")
	if err != nil || result.ExitCode != 0 {
		// AppArmor not enabled or not installed - no fix needed
		return nil
	}

	// Check if nerdctl is installed
	result, err = conn.Exec("command -v nerdctl >/dev/null 2>&1")
	if err != nil || result.ExitCode != 0 {
		// nerdctl not installed, nothing to fix
		return nil
	}

	// Check if we already created the profile file
	profilePath := "/etc/apparmor.d/nerdctl-default"
	result, err = conn.Exec(fmt.Sprintf("test -f %s && grep -q 'signal (receive) peer=runc' %s", profilePath, profilePath))
	if err == nil && result.ExitCode == 0 {
		// Profile already exists with fix
		return nil
	}

	// Create nerdctl-default profile with signal permissions
	// Based on docker-default but includes the signal fix
	profileContent := `# AppArmor profile for nerdctl containers
# Includes fix for signal handling (Ubuntu/Debian bug)
abi <abi/4.0>,
include <tunables/global>

profile nerdctl-default flags=(attach_disconnected,mediate_deleted) {
  include <abstractions/base>

  # Allow signal handling between runc processes
  signal (receive) peer=runc,
  signal (send) peer=nerdctl-default,

  network,
  capability,
  file,
  umount,

  # Allow everything else (permissive for application compatibility)
  deny @{PROC}/* w,   # deny write for now
  deny /sys/[^f]*/** wklx,
  deny /sys/f[^s]*/** wklx,
  deny /sys/fs/[^c]*/** wklx,
  deny /sys/fs/c[^g]*/** wklx,
  deny /sys/fs/cg[^r]*/** wklx,
  deny /sys/firmware/** rwklx,
  deny /sys/kernel/security/** rwklx,
}
`

	// Write the profile using heredoc
	writeCmd := fmt.Sprintf(`bash -c 'sudo tee %s > /dev/null << EOF
%s
EOF'`, profilePath, profileContent)

	result, err = conn.Exec(writeCmd)
	if err != nil || result.ExitCode != 0 {
		return fmt.Errorf("failed to create AppArmor profile (exit code %d): %v", result.ExitCode, err)
	}

	// Verify the file was created
	result, err = conn.Exec(fmt.Sprintf("test -f %s", profilePath))
	if err != nil || result.ExitCode != 0 {
		return fmt.Errorf("AppArmor profile file was not created")
	}

	// Load the profile into the kernel
	result, err = conn.Exec(fmt.Sprintf("sudo apparmor_parser -r %s", profilePath))
	if err != nil || result.ExitCode != 0 {
		return fmt.Errorf("failed to load AppArmor profile: %w", err)
	}

	return nil
}
