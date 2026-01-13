package storage

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/urfave/cli/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// AddDiskCommand adds raw disks to nodes for Longhorn storage
var AddDiskCommand = &cli.Command{
	Name:  "add-disk",
	Usage: "Format and mount raw disks for Longhorn storage",
	Description: `Discovers unmounted raw disks on a host, formats them, and configures
them for use with Longhorn distributed storage.

The command will:
1. Connect to the host via SSH
2. Discover unmounted block devices
3. Allow you to select which disks to add
4. Format each disk with ext4 filesystem
5. Mount disks to /mnt/longhorn-<device> (e.g., /mnt/longhorn-sdb)
6. Add fstab entries for persistence
7. Update the Longhorn node configuration with the new disk paths

Examples:
  foundry storage add-disk                    # Interactive mode - select host and disks
  foundry storage add-disk --host node1       # Add disks to specific host
  foundry storage add-disk --host node1 --disk sdb --disk sdc   # Non-interactive`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "host",
			Aliases: []string{"H"},
			Usage:   "Hostname of the target node (interactive selection if not specified)",
		},
		&cli.StringSliceFlag{
			Name:    "disk",
			Aliases: []string{"d"},
			Usage:   "Disk device name to add (e.g., sdb). Can be specified multiple times",
		},
		&cli.BoolFlag{
			Name:  "dry-run",
			Usage: "Show what would be done without making changes",
		},
		&cli.BoolFlag{
			Name:    "yes",
			Aliases: []string{"y"},
			Usage:   "Skip confirmation prompts",
		},
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Usage:   "Path to configuration file",
			Sources: cli.EnvVars("FOUNDRY_CONFIG"),
		},
	},
	Action: runAddDisk,
}

// DiskInfo represents information about a block device
type DiskInfo struct {
	Name       string // Device name (e.g., sdb)
	Size       string // Human-readable size (e.g., 100G)
	Type       string // Device type (disk, part, etc.)
	Mountpoint string // Current mountpoint (empty if not mounted)
	FSType     string // Filesystem type (empty if no filesystem)
}

// LonghornDiskConfig represents a disk configuration for Longhorn
type LonghornDiskConfig struct {
	Path            string `json:"path"`
	AllowScheduling bool   `json:"allowScheduling"`
	StorageReserved int64  `json:"storageReserved,omitempty"`
}

func runAddDisk(ctx context.Context, cmd *cli.Command) error {
	// Load configuration
	configPath := cmd.String("config")
	if configPath == "" {
		configPath = config.DefaultConfigPath()
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Initialize host registry
	loader := config.NewHostConfigLoader(configPath)
	registry := host.NewConfigRegistry(configPath, loader)
	host.SetDefaultRegistry(registry)

	// Get target host
	hostname := cmd.String("host")
	if hostname == "" {
		// Interactive host selection
		hostname, err = selectHost(cfg)
		if err != nil {
			return err
		}
	}

	// Verify host exists and has cluster role
	h, err := host.Get(hostname)
	if err != nil {
		return fmt.Errorf("host %s not found: %w", hostname, err)
	}

	if !h.HasRole(host.RoleClusterControlPlane) && !h.HasRole(host.RoleClusterWorker) {
		return fmt.Errorf("host %s is not a cluster node (missing cluster-control-plane or cluster-worker role)", hostname)
	}

	// Connect to host via SSH
	fmt.Printf("Connecting to %s...\n", hostname)
	conn, err := connectToHostForDisk(hostname)
	if err != nil {
		return err
	}
	defer conn.Close()
	fmt.Println("  Connected")

	// Discover available disks
	fmt.Println("Discovering available disks...")
	disks, err := discoverUnmountedDisks(conn)
	if err != nil {
		return fmt.Errorf("failed to discover disks: %w", err)
	}

	if len(disks) == 0 {
		fmt.Println("No unmounted raw disks found on this host.")
		return nil
	}

	// Get disks to add
	specifiedDisks := cmd.StringSlice("disk")
	var selectedDisks []DiskInfo

	if len(specifiedDisks) > 0 {
		// Validate specified disks exist
		diskMap := make(map[string]DiskInfo)
		for _, d := range disks {
			diskMap[d.Name] = d
		}

		for _, diskName := range specifiedDisks {
			if d, ok := diskMap[diskName]; ok {
				selectedDisks = append(selectedDisks, d)
			} else {
				return fmt.Errorf("disk %s not found or already mounted", diskName)
			}
		}
	} else {
		// Interactive disk selection
		selectedDisks, err = selectDisks(disks)
		if err != nil {
			return err
		}
	}

	if len(selectedDisks) == 0 {
		fmt.Println("No disks selected.")
		return nil
	}

	// Confirm action
	dryRun := cmd.Bool("dry-run")
	if !dryRun && !cmd.Bool("yes") {
		fmt.Println("\nThe following disks will be formatted and added to Longhorn:")
		for _, d := range selectedDisks {
			fmt.Printf("  /dev/%s (%s)\n", d.Name, d.Size)
		}
		fmt.Println("\nWARNING: All data on these disks will be DESTROYED!")
		fmt.Print("Continue? [y/N]: ")

		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	if dryRun {
		fmt.Println("\nDry-run mode - would perform the following actions:")
		for _, d := range selectedDisks {
			mountPoint := fmt.Sprintf("/mnt/longhorn-%s", d.Name)
			fmt.Printf("  1. Format /dev/%s with ext4\n", d.Name)
			fmt.Printf("  2. Create mount point %s\n", mountPoint)
			fmt.Printf("  3. Mount /dev/%s to %s\n", d.Name, mountPoint)
			fmt.Printf("  4. Add fstab entry for persistence\n")
		}
		fmt.Println("  5. Update Longhorn node configuration with new disk paths")
		return nil
	}

	// Format and mount each disk
	var mountPaths []string
	for _, d := range selectedDisks {
		mountPath, err := formatAndMountDisk(conn, d)
		if err != nil {
			return fmt.Errorf("failed to setup disk %s: %w", d.Name, err)
		}
		mountPaths = append(mountPaths, mountPath)
	}

	// Update Longhorn node configuration
	fmt.Println("Updating Longhorn node configuration...")
	if err := updateLonghornNodeDisks(ctx, hostname, mountPaths); err != nil {
		return fmt.Errorf("failed to update Longhorn configuration: %w", err)
	}

	fmt.Printf("\n%d disk(s) successfully added to %s for Longhorn storage\n", len(selectedDisks), hostname)
	return nil
}

// selectHost prompts the user to select a host from cluster nodes
func selectHost(cfg *config.Config) (string, error) {
	clusterHosts := cfg.GetClusterHosts()
	if len(clusterHosts) == 0 {
		return "", fmt.Errorf("no cluster nodes found in configuration")
	}

	fmt.Println("Available cluster nodes:")
	for i, h := range clusterHosts {
		roles := strings.Join(h.Roles, ", ")
		fmt.Printf("  %d. %s (%s) [%s]\n", i+1, h.Hostname, h.Address, roles)
	}

	fmt.Print("\nSelect a host (enter number): ")
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(input)
	index, err := strconv.Atoi(input)
	if err != nil || index < 1 || index > len(clusterHosts) {
		return "", fmt.Errorf("invalid selection: %s", input)
	}

	return clusterHosts[index-1].Hostname, nil
}

// selectDisks prompts the user to select disks to add
func selectDisks(disks []DiskInfo) ([]DiskInfo, error) {
	fmt.Println("\nAvailable unmounted disks:")
	for i, d := range disks {
		fsInfo := "no filesystem"
		if d.FSType != "" {
			fsInfo = fmt.Sprintf("has %s filesystem", d.FSType)
		}
		fmt.Printf("  %d. /dev/%s - %s (%s)\n", i+1, d.Name, d.Size, fsInfo)
	}

	fmt.Println("\nEnter disk numbers to add (comma-separated, e.g., 1,2,3), or 'all': ")
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read input: %w", err)
	}

	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return nil, nil
	}

	if input == "all" {
		return disks, nil
	}

	var selected []DiskInfo
	for _, part := range strings.Split(input, ",") {
		part = strings.TrimSpace(part)
		index, err := strconv.Atoi(part)
		if err != nil || index < 1 || index > len(disks) {
			return nil, fmt.Errorf("invalid selection: %s", part)
		}
		selected = append(selected, disks[index-1])
	}

	return selected, nil
}

// connectToHostForDisk creates an SSH connection to a host for disk operations
func connectToHostForDisk(hostname string) (*ssh.Connection, error) {
	// Get host from registry
	h, err := host.Get(hostname)
	if err != nil {
		return nil, fmt.Errorf("host %s not found in registry: %w", hostname, err)
	}

	// Get SSH key from filesystem storage
	keysDir, err := config.GetKeysDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get keys directory: %w", err)
	}

	keyStorage, err := ssh.NewFilesystemKeyStorage(keysDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create key storage: %w", err)
	}

	keyPair, err := keyStorage.Load(hostname)
	if err != nil {
		return nil, fmt.Errorf("SSH key not found for host %s (use 'foundry host add' first): %w", hostname, err)
	}

	// Create auth method from key pair
	authMethod, err := keyPair.AuthMethod()
	if err != nil {
		return nil, fmt.Errorf("failed to create auth method: %w", err)
	}

	// Create connection options
	connOpts := &ssh.ConnectionOptions{
		Host:       h.Address,
		Port:       h.Port,
		User:       h.User,
		AuthMethod: authMethod,
		Timeout:    30,
	}

	// Connect
	conn, err := ssh.Connect(connOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	return conn, nil
}

// discoverUnmountedDisks finds all unmounted block devices on the host
func discoverUnmountedDisks(conn *ssh.Connection) ([]DiskInfo, error) {
	// Use lsblk to list all block devices with relevant info
	// -n: no header, -d: only show disk devices (not partitions), -o: output columns
	result, err := conn.Exec("lsblk -ndo NAME,SIZE,TYPE,MOUNTPOINT,FSTYPE 2>/dev/null")
	if err != nil {
		return nil, fmt.Errorf("failed to run lsblk: %w", err)
	}

	if result.ExitCode != 0 {
		return nil, fmt.Errorf("lsblk failed: %s", result.Stderr)
	}

	var disks []DiskInfo
	lines := strings.Split(strings.TrimSpace(result.Stdout), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		disk := DiskInfo{
			Name: fields[0],
			Size: fields[1],
			Type: fields[2],
		}

		// Check for mountpoint (field 4) and fstype (field 5)
		if len(fields) >= 4 && fields[3] != "" {
			disk.Mountpoint = fields[3]
		}
		if len(fields) >= 5 && fields[4] != "" {
			disk.FSType = fields[4]
		}

		// Only include unmounted disk devices (not loop, rom, etc.)
		if disk.Type == "disk" && disk.Mountpoint == "" {
			// Skip system disks (usually the first disk with partitions)
			// Check if this disk has any mounted partitions
			hasMountedPart, err := hasMountedPartitions(conn, disk.Name)
			if err != nil {
				continue
			}
			if hasMountedPart {
				continue
			}

			disks = append(disks, disk)
		}
	}

	return disks, nil
}

// hasMountedPartitions checks if a disk has any mounted partitions
func hasMountedPartitions(conn *ssh.Connection, diskName string) (bool, error) {
	// Check for partitions of this disk that are mounted
	result, err := conn.Exec(fmt.Sprintf("lsblk -nlo MOUNTPOINT /dev/%s 2>/dev/null | grep -v '^$' | head -1", diskName))
	if err != nil {
		return false, err
	}

	// If any partition has a mountpoint, the disk is in use
	return strings.TrimSpace(result.Stdout) != "", nil
}

// formatAndMountDisk formats a disk with ext4 and mounts it for Longhorn
func formatAndMountDisk(conn *ssh.Connection, disk DiskInfo) (string, error) {
	device := fmt.Sprintf("/dev/%s", disk.Name)
	mountPoint := fmt.Sprintf("/mnt/longhorn-%s", disk.Name)

	fmt.Printf("Setting up %s (%s)...\n", device, disk.Size)

	// Step 1: Create partition table and partition using fdisk
	// fdisk is more universally available than parted
	fmt.Printf("  Creating partition table...\n")

	// Use fdisk with a heredoc to create GPT partition table with single partition
	// Commands: g (create GPT), n (new partition), accept defaults, w (write)
	fdiskCommands := `g
n
1


w
`
	result, err := conn.Exec(fmt.Sprintf("echo '%s' | sudo fdisk %s", fdiskCommands, device))
	if err != nil {
		return "", fmt.Errorf("failed to create partition: %w", err)
	}
	// fdisk may return non-zero even on success due to re-reading partition table
	// Check if partition was actually created
	if result.ExitCode != 0 && !strings.Contains(result.Stdout, "The partition table has been altered") {
		return "", fmt.Errorf("fdisk failed: %s", result.Stderr)
	}

	// The partition will be deviceName + "1" (e.g., sdb1) or deviceName + "p1" for nvme
	partition := fmt.Sprintf("%s1", device)
	if strings.HasPrefix(disk.Name, "nvme") {
		partition = fmt.Sprintf("%sp1", device)
	}

	// Wait for partition to appear
	result, err = conn.Exec(fmt.Sprintf("sudo udevadm settle && sleep 1 && ls %s", partition))
	if err != nil || result.ExitCode != 0 {
		return "", fmt.Errorf("partition %s did not appear after fdisk", partition)
	}

	// Step 2: Format with ext4
	fmt.Printf("  Formatting with ext4...\n")
	result, err = conn.Exec(fmt.Sprintf("sudo mkfs.ext4 -F %s", partition))
	if err != nil {
		return "", fmt.Errorf("failed to format disk: %w", err)
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("mkfs.ext4 failed: %s", result.Stderr)
	}

	// Step 3: Create mount point
	fmt.Printf("  Creating mount point %s...\n", mountPoint)
	result, err = conn.Exec(fmt.Sprintf("sudo mkdir -p %s", mountPoint))
	if err != nil || result.ExitCode != 0 {
		return "", fmt.Errorf("failed to create mount point: %w", err)
	}

	// Step 4: Get UUID for fstab entry
	result, err = conn.Exec(fmt.Sprintf("sudo blkid -s UUID -o value %s", partition))
	if err != nil {
		return "", fmt.Errorf("failed to get disk UUID: %w", err)
	}
	uuid := strings.TrimSpace(result.Stdout)

	// Step 5: Add fstab entry
	fmt.Printf("  Adding fstab entry...\n")
	fstabEntry := fmt.Sprintf("UUID=%s %s ext4 defaults,nofail 0 2", uuid, mountPoint)

	// Check if entry already exists
	result, _ = conn.Exec(fmt.Sprintf("grep -q '%s' /etc/fstab", mountPoint))
	if result.ExitCode != 0 {
		// Entry doesn't exist, add it
		result, err = conn.Exec(fmt.Sprintf("echo '%s' | sudo tee -a /etc/fstab", fstabEntry))
		if err != nil || result.ExitCode != 0 {
			return "", fmt.Errorf("failed to add fstab entry: %w", err)
		}
	}

	// Step 6: Mount the disk
	fmt.Printf("  Mounting disk...\n")
	result, err = conn.Exec(fmt.Sprintf("sudo mount %s", mountPoint))
	if err != nil {
		return "", fmt.Errorf("failed to mount disk: %w", err)
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("mount failed: %s", result.Stderr)
	}

	// Step 7: Set proper permissions for Longhorn
	result, err = conn.Exec(fmt.Sprintf("sudo chmod 777 %s", mountPoint))
	if err != nil || result.ExitCode != 0 {
		return "", fmt.Errorf("failed to set permissions: %w", err)
	}

	fmt.Printf("  Disk %s mounted at %s\n", device, mountPoint)
	return mountPoint, nil
}

// LonghornNodeDiskSpec represents the disk spec in the Longhorn Node CRD
type LonghornNodeDiskSpec struct {
	AllowScheduling   bool     `json:"allowScheduling"`
	DiskDriver        string   `json:"diskDriver,omitempty"`
	DiskType          string   `json:"diskType"`
	EvictionRequested bool     `json:"evictionRequested"`
	Path              string   `json:"path"`
	StorageReserved   int64    `json:"storageReserved"`
	Tags              []string `json:"tags"`
}

// updateLonghornNodeDisks updates the Longhorn Node CRD to include new disk paths
func updateLonghornNodeDisks(ctx context.Context, nodeName string, newPaths []string) error {
	// Get kubeconfig
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	kubeconfigPath := filepath.Join(configDir, "kubeconfig")
	kubeconfig, err := os.ReadFile(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to read kubeconfig: %w", err)
	}

	// Create K8s client
	k8sClient, err := k8s.NewClientFromKubeconfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create k8s client: %w", err)
	}

	// Get the Longhorn Node CRD
	longhornNodeGVR := schema.GroupVersionResource{
		Group:    "longhorn.io",
		Version:  "v1beta2",
		Resource: "nodes",
	}

	longhornNode, err := k8sClient.DynamicClient().Resource(longhornNodeGVR).Namespace("longhorn-system").Get(ctx, nodeName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get Longhorn node %s: %w", nodeName, err)
	}

	// Get existing disks from spec
	spec, ok := longhornNode.Object["spec"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("failed to parse Longhorn node spec")
	}

	disks, ok := spec["disks"].(map[string]interface{})
	if !ok {
		disks = make(map[string]interface{})
	}

	// Create a set of existing paths to avoid duplicates
	existingPaths := make(map[string]bool)
	for _, diskData := range disks {
		if disk, ok := diskData.(map[string]interface{}); ok {
			if path, ok := disk["path"].(string); ok {
				existingPaths[path] = true
			}
		}
	}

	// Add new disk entries
	addedCount := 0
	for _, path := range newPaths {
		if existingPaths[path] {
			fmt.Printf("  Disk path %s already configured, skipping\n", path)
			continue
		}

		// Generate a unique disk name based on the path
		// e.g., /mnt/longhorn-sdb -> disk-longhorn-sdb
		diskName := "disk-" + strings.TrimPrefix(path, "/mnt/")

		disks[diskName] = map[string]interface{}{
			"allowScheduling":   true,
			"diskDriver":        "",
			"diskType":          "filesystem",
			"evictionRequested": false,
			"path":              path,
			"storageReserved":   0,
			"tags":              []string{},
		}
		existingPaths[path] = true
		addedCount++
	}

	if addedCount == 0 {
		fmt.Printf("  No new disks to add to Longhorn node %s\n", nodeName)
		return nil
	}

	// Update the spec with new disks
	spec["disks"] = disks

	// Patch the Longhorn Node CRD
	patchData := map[string]interface{}{
		"spec": map[string]interface{}{
			"disks": disks,
		},
	}

	patchBytes, err := json.Marshal(patchData)
	if err != nil {
		return fmt.Errorf("failed to create patch: %w", err)
	}

	_, err = k8sClient.DynamicClient().Resource(longhornNodeGVR).Namespace("longhorn-system").Patch(
		ctx,
		nodeName,
		types.MergePatchType,
		patchBytes,
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to patch Longhorn node: %w", err)
	}

	fmt.Printf("  Added %d disk(s) to Longhorn node %s\n", addedCount, nodeName)
	return nil
}
