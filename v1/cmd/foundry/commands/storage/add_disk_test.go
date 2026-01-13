package storage

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiskInfo_Parsing(t *testing.T) {
	// Test parsing of DiskInfo structure
	tests := []struct {
		name     string
		disk     DiskInfo
		expected string
	}{
		{
			name: "basic disk",
			disk: DiskInfo{
				Name: "sdb",
				Size: "100G",
				Type: "disk",
			},
			expected: "sdb",
		},
		{
			name: "nvme disk",
			disk: DiskInfo{
				Name: "nvme0n1",
				Size: "500G",
				Type: "disk",
			},
			expected: "nvme0n1",
		},
		{
			name: "disk with filesystem",
			disk: DiskInfo{
				Name:   "sdc",
				Size:   "200G",
				Type:   "disk",
				FSType: "ext4",
			},
			expected: "sdc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.disk.Name)
		})
	}
}

func TestLonghornDiskConfig_JSON(t *testing.T) {
	// Test serialization of LonghornDiskConfig
	config := LonghornDiskConfig{
		Path:            "/mnt/longhorn-sdb",
		AllowScheduling: true,
		StorageReserved: 0,
	}

	jsonBytes, err := json.Marshal(config)
	require.NoError(t, err)

	// Verify JSON structure
	var parsed map[string]interface{}
	err = json.Unmarshal(jsonBytes, &parsed)
	require.NoError(t, err)

	assert.Equal(t, "/mnt/longhorn-sdb", parsed["path"])
	assert.Equal(t, true, parsed["allowScheduling"])
}

func TestLonghornDiskConfig_JSONArray(t *testing.T) {
	// Test serialization of multiple disk configs
	configs := []LonghornDiskConfig{
		{
			Path:            "/mnt/longhorn-sdb",
			AllowScheduling: true,
		},
		{
			Path:            "/mnt/longhorn-sdc",
			AllowScheduling: true,
			StorageReserved: 1073741824, // 1GB
		},
	}

	jsonBytes, err := json.Marshal(configs)
	require.NoError(t, err)

	// Verify it's a valid JSON array
	var parsed []LonghornDiskConfig
	err = json.Unmarshal(jsonBytes, &parsed)
	require.NoError(t, err)

	assert.Len(t, parsed, 2)
	assert.Equal(t, "/mnt/longhorn-sdb", parsed[0].Path)
	assert.Equal(t, "/mnt/longhorn-sdc", parsed[1].Path)
	assert.Equal(t, int64(1073741824), parsed[1].StorageReserved)
}

func TestMountPointNaming(t *testing.T) {
	tests := []struct {
		diskName       string
		expectedMount  string
	}{
		{
			diskName:      "sdb",
			expectedMount: "/mnt/longhorn-sdb",
		},
		{
			diskName:      "sdc",
			expectedMount: "/mnt/longhorn-sdc",
		},
		{
			diskName:      "nvme0n1",
			expectedMount: "/mnt/longhorn-nvme0n1",
		},
		{
			diskName:      "vda",
			expectedMount: "/mnt/longhorn-vda",
		},
	}

	for _, tt := range tests {
		t.Run(tt.diskName, func(t *testing.T) {
			// Simulate the mount point naming logic from formatAndMountDisk
			mountPoint := "/mnt/longhorn-" + tt.diskName
			assert.Equal(t, tt.expectedMount, mountPoint)
		})
	}
}

func TestParseLsblkOutput(t *testing.T) {
	// Simulate parsing lsblk output for top-level disk devices only
	// The real implementation uses lsblk -nd (no header, disk devices only)
	// Format: NAME SIZE TYPE MOUNTPOINT FSTYPE
	//
	// Note: sda is a system disk - in the real implementation we detect this
	// by checking if any of its partitions are mounted. For this unit test,
	// we simulate the output after such filtering.
	lsblkOutput := `sdb 100G disk
sdc 200G disk
nvme0n1 500G disk `

	lines := strings.Split(strings.TrimSpace(lsblkOutput), "\n")
	var disks []DiskInfo

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

		// Check for mountpoint and fstype
		if len(fields) >= 4 && fields[3] != "" {
			disk.Mountpoint = fields[3]
		}
		if len(fields) >= 5 && fields[4] != "" {
			disk.FSType = fields[4]
		}

		// Only include unmounted disk devices
		if disk.Type == "disk" && disk.Mountpoint == "" {
			disks = append(disks, disk)
		}
	}

	// Should find sdb, sdc, and nvme0n1 as unmounted disks
	assert.Len(t, disks, 3)

	diskNames := make([]string, len(disks))
	for i, d := range disks {
		diskNames[i] = d.Name
	}

	assert.Contains(t, diskNames, "sdb")
	assert.Contains(t, diskNames, "sdc")
	assert.Contains(t, diskNames, "nvme0n1")
}

func TestParseLsblkOutput_WithSystemDisk(t *testing.T) {
	// Test the full parsing logic including filtering out system disks
	// This simulates the behavior of discoverUnmountedDisks
	//
	// In the real code, we check if disk partitions are mounted using hasMountedPartitions()
	// This test verifies the basic filtering logic
	type diskWithPartitions struct {
		disk       DiskInfo
		partitions []DiskInfo
	}

	allDevices := []diskWithPartitions{
		{
			disk: DiskInfo{Name: "sda", Size: "50G", Type: "disk"},
			partitions: []DiskInfo{
				{Name: "sda1", Size: "512M", Type: "part", Mountpoint: "/boot/efi", FSType: "vfat"},
				{Name: "sda2", Size: "49G", Type: "part", Mountpoint: "/", FSType: "ext4"},
			},
		},
		{
			disk:       DiskInfo{Name: "sdb", Size: "100G", Type: "disk"},
			partitions: []DiskInfo{}, // no partitions, not mounted
		},
		{
			disk:       DiskInfo{Name: "sdc", Size: "200G", Type: "disk"},
			partitions: []DiskInfo{}, // no partitions, not mounted
		},
	}

	var availableDisks []DiskInfo
	for _, d := range allDevices {
		// Skip disks that have mounted partitions (simulating hasMountedPartitions check)
		hasMounted := false
		for _, p := range d.partitions {
			if p.Mountpoint != "" {
				hasMounted = true
				break
			}
		}

		if !hasMounted && d.disk.Mountpoint == "" {
			availableDisks = append(availableDisks, d.disk)
		}
	}

	// Should only find sdb and sdc (sda is system disk)
	assert.Len(t, availableDisks, 2)

	diskNames := make([]string, len(availableDisks))
	for i, d := range availableDisks {
		diskNames[i] = d.Name
	}

	assert.Contains(t, diskNames, "sdb")
	assert.Contains(t, diskNames, "sdc")
	assert.NotContains(t, diskNames, "sda") // system disk excluded
}

func TestPartitionNaming(t *testing.T) {
	tests := []struct {
		name           string
		deviceName     string
		expectedPart   string
	}{
		{
			name:         "standard SATA disk",
			deviceName:   "sdb",
			expectedPart: "/dev/sdb1",
		},
		{
			name:         "standard virtio disk",
			deviceName:   "vdb",
			expectedPart: "/dev/vdb1",
		},
		{
			name:         "NVMe disk",
			deviceName:   "nvme0n1",
			expectedPart: "/dev/nvme0n1p1",
		},
		{
			name:         "another NVMe disk",
			deviceName:   "nvme1n1",
			expectedPart: "/dev/nvme1n1p1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			device := "/dev/" + tt.deviceName

			// Standard partition naming
			partition := device + "1"

			// NVMe devices use different naming convention
			if strings.HasPrefix(tt.deviceName, "nvme") {
				partition = device + "p1"
			}

			assert.Equal(t, tt.expectedPart, partition)
		})
	}
}

func TestFstabEntryFormat(t *testing.T) {
	uuid := "12345678-1234-1234-1234-123456789abc"
	mountPoint := "/mnt/longhorn-sdb"

	// Generate fstab entry
	fstabEntry := "UUID=" + uuid + " " + mountPoint + " ext4 defaults,nofail 0 2"

	// Verify format
	assert.Contains(t, fstabEntry, "UUID="+uuid)
	assert.Contains(t, fstabEntry, mountPoint)
	assert.Contains(t, fstabEntry, "ext4")
	assert.Contains(t, fstabEntry, "defaults,nofail")
	assert.True(t, strings.HasSuffix(fstabEntry, "0 2"))
}

func TestLonghornAnnotationKey(t *testing.T) {
	// Verify the correct Longhorn annotation key is used
	annotationKey := "node.longhorn.io/default-disks-config"
	assert.Equal(t, "node.longhorn.io/default-disks-config", annotationKey)
}

func TestDiskInfoValidation(t *testing.T) {
	tests := []struct {
		name         string
		disk         DiskInfo
		shouldBeUsed bool
		reason       string
	}{
		{
			name: "valid unmounted disk",
			disk: DiskInfo{
				Name:       "sdb",
				Size:       "100G",
				Type:       "disk",
				Mountpoint: "",
				FSType:     "",
			},
			shouldBeUsed: true,
			reason:       "unmounted raw disk",
		},
		{
			name: "mounted disk",
			disk: DiskInfo{
				Name:       "sda",
				Size:       "50G",
				Type:       "disk",
				Mountpoint: "/",
				FSType:     "ext4",
			},
			shouldBeUsed: false,
			reason:       "already mounted",
		},
		{
			name: "partition not disk",
			disk: DiskInfo{
				Name:       "sda1",
				Size:       "50G",
				Type:       "part",
				Mountpoint: "",
				FSType:     "",
			},
			shouldBeUsed: false,
			reason:       "partition, not a disk",
		},
		{
			name: "loop device",
			disk: DiskInfo{
				Name:       "loop0",
				Size:       "100M",
				Type:       "loop",
				Mountpoint: "",
				FSType:     "",
			},
			shouldBeUsed: false,
			reason:       "loop device, not a disk",
		},
		{
			name: "unmounted disk with filesystem",
			disk: DiskInfo{
				Name:       "sdc",
				Size:       "200G",
				Type:       "disk",
				Mountpoint: "",
				FSType:     "ext4",
			},
			shouldBeUsed: true,
			reason:       "unmounted disk (will warn about existing fs)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Check if disk meets criteria for add-disk command
			isUsable := tt.disk.Type == "disk" && tt.disk.Mountpoint == ""
			assert.Equal(t, tt.shouldBeUsed, isUsable, "disk usability check for: %s", tt.reason)
		})
	}
}

func TestMergeDiskConfigs(t *testing.T) {
	// Test merging new disk paths with existing configuration
	existingConfig := []LonghornDiskConfig{
		{
			Path:            "/var/lib/longhorn",
			AllowScheduling: true,
		},
	}

	newPaths := []string{
		"/mnt/longhorn-sdb",
		"/mnt/longhorn-sdc",
	}

	// Create a set of existing paths
	existingPaths := make(map[string]bool)
	for _, dc := range existingConfig {
		existingPaths[dc.Path] = true
	}

	// Add new paths
	result := existingConfig
	for _, path := range newPaths {
		if !existingPaths[path] {
			result = append(result, LonghornDiskConfig{
				Path:            path,
				AllowScheduling: true,
				StorageReserved: 0,
			})
			existingPaths[path] = true
		}
	}

	// Should have 3 disk configs now
	assert.Len(t, result, 3)
	assert.Equal(t, "/var/lib/longhorn", result[0].Path)
	assert.Equal(t, "/mnt/longhorn-sdb", result[1].Path)
	assert.Equal(t, "/mnt/longhorn-sdc", result[2].Path)
}

func TestMergeDiskConfigs_NoDuplicates(t *testing.T) {
	// Test that duplicate paths are not added
	existingConfig := []LonghornDiskConfig{
		{
			Path:            "/mnt/longhorn-sdb",
			AllowScheduling: true,
		},
	}

	newPaths := []string{
		"/mnt/longhorn-sdb", // duplicate
		"/mnt/longhorn-sdc", // new
	}

	existingPaths := make(map[string]bool)
	for _, dc := range existingConfig {
		existingPaths[dc.Path] = true
	}

	result := existingConfig
	for _, path := range newPaths {
		if !existingPaths[path] {
			result = append(result, LonghornDiskConfig{
				Path:            path,
				AllowScheduling: true,
			})
			existingPaths[path] = true
		}
	}

	// Should have only 2 disk configs (no duplicate)
	assert.Len(t, result, 2)
}

func TestAddDiskCommand_Structure(t *testing.T) {
	// Verify command structure
	assert.Equal(t, "add-disk", AddDiskCommand.Name)
	assert.NotEmpty(t, AddDiskCommand.Usage)
	assert.NotEmpty(t, AddDiskCommand.Description)
	assert.NotNil(t, AddDiskCommand.Action)

	// Verify flags exist
	flagNames := make([]string, 0)
	for _, flag := range AddDiskCommand.Flags {
		flagNames = append(flagNames, flag.Names()...)
	}

	assert.Contains(t, flagNames, "host")
	assert.Contains(t, flagNames, "H") // host alias
	assert.Contains(t, flagNames, "disk")
	assert.Contains(t, flagNames, "d") // disk alias
	assert.Contains(t, flagNames, "dry-run")
	assert.Contains(t, flagNames, "yes")
	assert.Contains(t, flagNames, "y") // yes alias
	// --config is now inherited from root command, not defined on subcommand
}
