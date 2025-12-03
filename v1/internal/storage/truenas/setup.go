package truenas

import (
	"fmt"
	"strings"
)

// SetupConfig contains configuration for TrueNAS setup
type SetupConfig struct {
	// PoolName is the name of the pool to use/create
	PoolName string
	// DatasetName is the name of the parent dataset for CSI
	DatasetName string
	// EnableNFS enables NFS service for NFS CSI
	EnableNFS bool
	// EnableISCSI enables iSCSI service for iSCSI CSI
	EnableISCSI bool
	// ISCSIPortalIP is the IP to bind iSCSI portal (0.0.0.0 for all)
	ISCSIPortalIP string
	// ISCSIPortalPort is the port for iSCSI (default 3260)
	ISCSIPortalPort int
	// VDevType is the type of vdev to create (STRIPE, MIRROR, RAIDZ1, RAIDZ2, RAIDZ3)
	VDevType string
	// MinDisksForMirror is the minimum disks required for mirroring
	MinDisksForMirror int
}

// DefaultSetupConfig returns the default setup configuration
func DefaultSetupConfig() *SetupConfig {
	return &SetupConfig{
		PoolName:          "tank",
		DatasetName:       "k8s",
		EnableNFS:         true,
		EnableISCSI:       true,
		ISCSIPortalIP:     "0.0.0.0",
		ISCSIPortalPort:   3260,
		VDevType:          "MIRROR",
		MinDisksForMirror: 2,
	}
}

// SetupResult contains the results of the setup process
type SetupResult struct {
	// SystemInfo contains TrueNAS system information
	SystemInfo *SystemInfo
	// Pool is the pool being used
	Pool *Pool
	// PoolCreated indicates if a new pool was created
	PoolCreated bool
	// Dataset is the CSI parent dataset
	Dataset *Dataset
	// DatasetCreated indicates if a new dataset was created
	DatasetCreated bool
	// NFSEnabled indicates if NFS service was enabled
	NFSEnabled bool
	// ISCSIEnabled indicates if iSCSI service was enabled
	ISCSIEnabled bool
	// ISCSIPortal is the iSCSI portal (if created/found)
	ISCSIPortal *ISCSIPortal
	// ISCSIInitiator is the iSCSI initiator group (if created/found)
	ISCSIInitiator *ISCSIInitiator
	// Warnings contains any non-fatal warnings during setup
	Warnings []string
}

// Validator provides TrueNAS setup validation
type Validator struct {
	client *Client
}

// NewValidator creates a new TrueNAS validator
func NewValidator(client *Client) *Validator {
	return &Validator{client: client}
}

// ValidateConnection tests connectivity to TrueNAS API
func (v *Validator) ValidateConnection() (*SystemInfo, error) {
	info, err := v.client.GetSystemInfo()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to TrueNAS: %w", err)
	}
	return info, nil
}

// Setup performs the full TrueNAS setup process
func (v *Validator) Setup(cfg *SetupConfig) (*SetupResult, error) {
	if cfg == nil {
		cfg = DefaultSetupConfig()
	}

	result := &SetupResult{
		Warnings: make([]string, 0),
	}

	// Step 1: Validate connection
	info, err := v.ValidateConnection()
	if err != nil {
		return nil, err
	}
	result.SystemInfo = info
	fmt.Printf("  Connected to TrueNAS %s (%s)\n", info.Version, info.Hostname)

	// Step 2: Find or create pool
	pool, created, err := v.ensurePool(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure pool: %w", err)
	}
	result.Pool = pool
	result.PoolCreated = created
	if created {
		fmt.Printf("  Created pool %q\n", pool.Name)
	} else {
		fmt.Printf("  Using existing pool %q\n", pool.Name)
	}

	// Step 3: Create CSI parent dataset
	dataset, datasetCreated, err := v.ensureDataset(cfg.PoolName, cfg.DatasetName)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure dataset: %w", err)
	}
	result.Dataset = dataset
	result.DatasetCreated = datasetCreated
	if datasetCreated {
		fmt.Printf("  Created dataset %q\n", dataset.ID)
	} else {
		fmt.Printf("  Using existing dataset %q\n", dataset.ID)
	}

	// Step 4: Enable NFS if requested
	if cfg.EnableNFS {
		enabled, err := v.ensureServiceRunning("nfs")
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to enable NFS: %v", err))
		} else {
			result.NFSEnabled = true
			if enabled {
				fmt.Println("  NFS service enabled and started")
			} else {
				fmt.Println("  NFS service already running")
			}
		}
	}

	// Step 5: Enable iSCSI if requested
	if cfg.EnableISCSI {
		enabled, err := v.ensureServiceRunning("iscsitarget")
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to enable iSCSI: %v", err))
		} else {
			result.ISCSIEnabled = true
			if enabled {
				fmt.Println("  iSCSI service enabled and started")
			} else {
				fmt.Println("  iSCSI service already running")
			}

			// Create iSCSI portal if it doesn't exist
			portal, err := v.ensureISCSIPortal(cfg.ISCSIPortalIP, cfg.ISCSIPortalPort)
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to create iSCSI portal: %v", err))
			} else {
				result.ISCSIPortal = portal
				fmt.Printf("  iSCSI portal configured on %s:%d\n", cfg.ISCSIPortalIP, cfg.ISCSIPortalPort)
			}

			// Create iSCSI initiator group (allow all)
			initiator, err := v.ensureISCSIInitiator()
			if err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to create iSCSI initiator: %v", err))
			} else {
				result.ISCSIInitiator = initiator
				fmt.Println("  iSCSI initiator group configured")
			}
		}
	}

	return result, nil
}

// ensurePool finds an existing pool or creates a new one
func (v *Validator) ensurePool(cfg *SetupConfig) (*Pool, bool, error) {
	// Check for existing pools
	pools, err := v.client.ListPools()
	if err != nil {
		return nil, false, fmt.Errorf("failed to list pools: %w", err)
	}

	// Look for a pool with the configured name
	for i := range pools {
		if pools[i].Name == cfg.PoolName {
			return &pools[i], false, nil
		}
	}

	// If any pool exists, use the first one
	if len(pools) > 0 {
		return &pools[0], false, nil
	}

	// No pools exist, try to create one
	pool, err := v.createPool(cfg)
	if err != nil {
		return nil, false, err
	}

	return pool, true, nil
}

// createPool creates a new pool using unused disks
func (v *Validator) createPool(cfg *SetupConfig) (*Pool, error) {
	// Get unused disks
	disks, err := v.client.GetUnusedDisks()
	if err != nil {
		return nil, fmt.Errorf("failed to get unused disks: %w", err)
	}

	if len(disks) == 0 {
		return nil, fmt.Errorf("no unused disks available to create pool")
	}

	// Determine vdev type based on disk count
	vdevType := cfg.VDevType
	diskNames := make([]string, len(disks))
	for i, disk := range disks {
		diskNames[i] = disk.Name
	}

	// Adjust vdev type based on disk count
	switch {
	case len(disks) == 1:
		vdevType = "STRIPE"
	case len(disks) < cfg.MinDisksForMirror:
		vdevType = "STRIPE"
	case vdevType == "RAIDZ1" && len(disks) < 3:
		vdevType = "MIRROR"
	case vdevType == "RAIDZ2" && len(disks) < 4:
		if len(disks) >= 3 {
			vdevType = "RAIDZ1"
		} else {
			vdevType = "MIRROR"
		}
	case vdevType == "RAIDZ3" && len(disks) < 5:
		if len(disks) >= 4 {
			vdevType = "RAIDZ2"
		} else if len(disks) >= 3 {
			vdevType = "RAIDZ1"
		} else {
			vdevType = "MIRROR"
		}
	}

	fmt.Printf("  Creating pool %q with %d disks (%s)\n", cfg.PoolName, len(disks), vdevType)

	poolConfig := PoolCreateConfig{
		Name: cfg.PoolName,
		Topology: PoolCreateTopology{
			Data: []PoolCreateVDev{
				{
					Type:  vdevType,
					Disks: diskNames,
				},
			},
		},
	}

	return v.client.CreatePool(poolConfig)
}

// ensureDataset finds or creates the CSI parent dataset
func (v *Validator) ensureDataset(poolName, datasetName string) (*Dataset, bool, error) {
	fullName := fmt.Sprintf("%s/%s", poolName, datasetName)

	// Try to get existing dataset
	dataset, err := v.client.GetDataset(fullName)
	if err == nil {
		return dataset, false, nil
	}

	// Check if it's a "not found" error
	if apiErr, ok := err.(*APIError); ok {
		if !strings.Contains(strings.ToLower(apiErr.Error()), "not found") &&
			!strings.Contains(strings.ToLower(apiErr.Error()), "does not exist") {
			return nil, false, err
		}
	}

	// Create dataset
	config := DatasetConfig{
		Name:     fullName,
		Type:     "FILESYSTEM",
		Comments: "Foundry CSI parent dataset",
	}

	dataset, err = v.client.CreateDataset(config)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create dataset: %w", err)
	}

	return dataset, true, nil
}

// ensureServiceRunning ensures a service is enabled and running
// Returns true if the service was started (wasn't running before)
func (v *Validator) ensureServiceRunning(serviceName string) (bool, error) {
	svc, err := v.client.GetService(serviceName)
	if err != nil {
		return false, err
	}

	wasNotRunning := svc.State != "RUNNING"

	if err := v.client.EnsureServiceRunning(serviceName); err != nil {
		return false, err
	}

	return wasNotRunning, nil
}

// ensureISCSIPortal finds or creates an iSCSI portal
func (v *Validator) ensureISCSIPortal(ip string, port int) (*ISCSIPortal, error) {
	// List existing portals
	portals, err := v.client.ListISCSIPortals()
	if err != nil {
		return nil, err
	}

	// Check if a portal with this IP/port exists
	for i := range portals {
		for _, listen := range portals[i].Listen {
			if listen.IP == ip && listen.Port == port {
				return &portals[i], nil
			}
			// Also check for 0.0.0.0 which covers all IPs
			if listen.IP == "0.0.0.0" && listen.Port == port {
				return &portals[i], nil
			}
		}
	}

	// Create new portal
	config := ISCSIPortalConfig{
		Listen: []ISCSIPortalListen{
			{IP: ip, Port: port},
		},
		Comment: "Foundry iSCSI portal",
	}

	return v.client.CreateISCSIPortal(config)
}

// ensureISCSIInitiator finds or creates an iSCSI initiator group (allow all)
func (v *Validator) ensureISCSIInitiator() (*ISCSIInitiator, error) {
	// List existing initiators
	initiators, err := v.client.ListISCSIInitiators()
	if err != nil {
		return nil, err
	}

	// Look for an "allow all" initiator (empty initiators list)
	for i := range initiators {
		if len(initiators[i].Initiators) == 0 {
			return &initiators[i], nil
		}
	}

	// Create new initiator group that allows all
	config := ISCSIInitiatorConfig{
		Initiators: []string{}, // Empty means allow all
		Comment:    "Foundry CSI - allow all initiators",
	}

	return v.client.CreateISCSIInitiator(config)
}

// ValidateRequirements checks if TrueNAS is properly configured for CSI
func (v *Validator) ValidateRequirements(cfg *SetupConfig) error {
	if cfg == nil {
		cfg = DefaultSetupConfig()
	}

	// Check connection
	_, err := v.ValidateConnection()
	if err != nil {
		return err
	}

	// Check for pools
	pools, err := v.client.ListPools()
	if err != nil {
		return fmt.Errorf("failed to list pools: %w", err)
	}
	if len(pools) == 0 {
		return fmt.Errorf("no storage pools found - run setup first")
	}

	// Check for CSI dataset
	fullName := fmt.Sprintf("%s/%s", cfg.PoolName, cfg.DatasetName)
	_, err = v.client.GetDataset(fullName)
	if err != nil {
		return fmt.Errorf("CSI dataset %q not found - run setup first", fullName)
	}

	// Check NFS service if needed
	if cfg.EnableNFS {
		svc, err := v.client.GetService("nfs")
		if err != nil {
			return fmt.Errorf("NFS service not found: %w", err)
		}
		if svc.State != "RUNNING" {
			return fmt.Errorf("NFS service is not running")
		}
	}

	// Check iSCSI service if needed
	if cfg.EnableISCSI {
		svc, err := v.client.GetService("iscsitarget")
		if err != nil {
			return fmt.Errorf("iSCSI service not found: %w", err)
		}
		if svc.State != "RUNNING" {
			return fmt.Errorf("iSCSI service is not running")
		}

		// Check for portal
		portals, err := v.client.ListISCSIPortals()
		if err != nil {
			return fmt.Errorf("failed to list iSCSI portals: %w", err)
		}
		if len(portals) == 0 {
			return fmt.Errorf("no iSCSI portals configured")
		}

		// Check for initiator
		initiators, err := v.client.ListISCSIInitiators()
		if err != nil {
			return fmt.Errorf("failed to list iSCSI initiators: %w", err)
		}
		if len(initiators) == 0 {
			return fmt.Errorf("no iSCSI initiator groups configured")
		}
	}

	return nil
}

// GetCSIConfig returns the configuration needed for democratic-csi
type CSIConfig struct {
	// HTTPURL is the TrueNAS API URL
	HTTPURL string
	// APIKey is the TrueNAS API key
	APIKey string
	// PoolName is the ZFS pool name
	PoolName string
	// DatasetParent is the parent dataset for CSI volumes
	DatasetParent string
	// NFSShareHost is the hostname/IP for NFS shares
	NFSShareHost string
	// ISCSIPortal is the iSCSI portal address
	ISCSIPortal string
	// ISCSITargetPortalGroup is the portal group ID
	ISCSITargetPortalGroup int
	// ISCSIInitiatorGroup is the initiator group ID
	ISCSIInitiatorGroup int
}

// GetCSIConfig generates CSI configuration from setup result
func (v *Validator) GetCSIConfig(result *SetupResult, cfg *SetupConfig, apiURL string) *CSIConfig {
	csiConfig := &CSIConfig{
		HTTPURL:       apiURL,
		APIKey:        v.client.apiKey,
		PoolName:      result.Pool.Name,
		DatasetParent: result.Dataset.ID,
	}

	// NFS host is typically the TrueNAS IP
	// Extract from API URL
	csiConfig.NFSShareHost = extractHostFromURL(apiURL)

	// iSCSI configuration
	if result.ISCSIPortal != nil {
		if len(result.ISCSIPortal.Listen) > 0 {
			listen := result.ISCSIPortal.Listen[0]
			if listen.IP == "0.0.0.0" {
				csiConfig.ISCSIPortal = fmt.Sprintf("%s:%d", csiConfig.NFSShareHost, listen.Port)
			} else {
				csiConfig.ISCSIPortal = fmt.Sprintf("%s:%d", listen.IP, listen.Port)
			}
		}
		csiConfig.ISCSITargetPortalGroup = result.ISCSIPortal.Tag
	}

	if result.ISCSIInitiator != nil {
		csiConfig.ISCSIInitiatorGroup = result.ISCSIInitiator.Tag
	}

	return csiConfig
}

// extractHostFromURL extracts the host from a URL
func extractHostFromURL(url string) string {
	// Remove protocol
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")

	// Remove port if present
	if idx := strings.Index(url, ":"); idx != -1 {
		url = url[:idx]
	}

	// Remove path if present
	if idx := strings.Index(url, "/"); idx != -1 {
		url = url[:idx]
	}

	return url
}
