package backup

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

// newTestClient creates a VeleroClient with a fake dynamic client for testing
func newTestClient(objects ...runtime.Object) *VeleroClient {
	scheme := runtime.NewScheme()
	dynamicClient := fake.NewSimpleDynamicClient(scheme, objects...)
	return &VeleroClient{
		dynamicClient: dynamicClient,
		namespace:     VeleroNamespace,
	}
}

func TestVeleroClient_CreateBackup(t *testing.T) {
	client := newTestClient()
	ctx := context.Background()

	tests := []struct {
		name    string
		backup  string
		opts    BackupOptions
		wantErr bool
	}{
		{
			name:   "create basic backup",
			backup: "test-backup",
			opts:   BackupOptions{},
		},
		{
			name:   "create backup with namespaces",
			backup: "ns-backup",
			opts: BackupOptions{
				IncludedNamespaces: []string{"default", "app"},
			},
		},
		{
			name:   "create backup with TTL",
			backup: "ttl-backup",
			opts: BackupOptions{
				TTL: "720h",
			},
		},
		{
			name:   "create backup with excluded namespaces",
			backup: "exclude-backup",
			opts: BackupOptions{
				ExcludedNamespaces: []string{"kube-system", "velero"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.CreateBackup(ctx, tt.backup, tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateBackup() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVeleroClient_ListBackups(t *testing.T) {
	// Create some test backups
	backup1 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "velero.io/v1",
			"kind":       "Backup",
			"metadata": map[string]interface{}{
				"name":      "backup-1",
				"namespace": VeleroNamespace,
			},
			"status": map[string]interface{}{
				"phase": "Completed",
			},
		},
	}
	backup2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "velero.io/v1",
			"kind":       "Backup",
			"metadata": map[string]interface{}{
				"name":      "backup-2",
				"namespace": VeleroNamespace,
			},
			"status": map[string]interface{}{
				"phase": "InProgress",
			},
		},
	}

	client := newTestClient(backup1, backup2)
	ctx := context.Background()

	backups, err := client.ListBackups(ctx)
	if err != nil {
		t.Fatalf("ListBackups() error = %v", err)
	}

	if len(backups) != 2 {
		t.Errorf("ListBackups() got %d backups, want 2", len(backups))
	}
}

func TestVeleroClient_GetBackup(t *testing.T) {
	backup := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "velero.io/v1",
			"kind":       "Backup",
			"metadata": map[string]interface{}{
				"name":      "test-backup",
				"namespace": VeleroNamespace,
			},
			"status": map[string]interface{}{
				"phase": "Completed",
				"progress": map[string]interface{}{
					"itemsBackedUp": int64(100),
					"totalItems":    int64(100),
				},
			},
			"spec": map[string]interface{}{
				"includedNamespaces": []interface{}{"default"},
			},
		},
	}

	client := newTestClient(backup)
	ctx := context.Background()

	info, err := client.GetBackup(ctx, "test-backup")
	if err != nil {
		t.Fatalf("GetBackup() error = %v", err)
	}

	if info.Name != "test-backup" {
		t.Errorf("GetBackup() name = %v, want test-backup", info.Name)
	}
	if info.Status != "Completed" {
		t.Errorf("GetBackup() status = %v, want Completed", info.Status)
	}
	if info.ItemsBackedUp != 100 {
		t.Errorf("GetBackup() itemsBackedUp = %v, want 100", info.ItemsBackedUp)
	}
}

func TestVeleroClient_CreateRestore(t *testing.T) {
	client := newTestClient()
	ctx := context.Background()

	tests := []struct {
		name       string
		restore    string
		backupName string
		opts       RestoreOptions
		wantErr    bool
	}{
		{
			name:       "create basic restore",
			restore:    "test-restore",
			backupName: "test-backup",
			opts:       RestoreOptions{},
		},
		{
			name:       "create restore with namespaces",
			restore:    "ns-restore",
			backupName: "test-backup",
			opts: RestoreOptions{
				IncludedNamespaces: []string{"default"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.CreateRestore(ctx, tt.restore, tt.backupName, tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateRestore() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVeleroClient_ListRestores(t *testing.T) {
	restore := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "velero.io/v1",
			"kind":       "Restore",
			"metadata": map[string]interface{}{
				"name":      "test-restore",
				"namespace": VeleroNamespace,
			},
			"spec": map[string]interface{}{
				"backupName": "test-backup",
			},
			"status": map[string]interface{}{
				"phase": "Completed",
			},
		},
	}

	client := newTestClient(restore)
	ctx := context.Background()

	restores, err := client.ListRestores(ctx)
	if err != nil {
		t.Fatalf("ListRestores() error = %v", err)
	}

	if len(restores) != 1 {
		t.Errorf("ListRestores() got %d restores, want 1", len(restores))
	}
	if restores[0].BackupName != "test-backup" {
		t.Errorf("ListRestores() backupName = %v, want test-backup", restores[0].BackupName)
	}
}

func TestVeleroClient_CreateSchedule(t *testing.T) {
	client := newTestClient()
	ctx := context.Background()

	tests := []struct {
		name     string
		schedule string
		opts     ScheduleOptions
		wantErr  bool
	}{
		{
			name:     "create daily schedule",
			schedule: "daily-backup",
			opts: ScheduleOptions{
				Cron: "0 2 * * *",
				TTL:  "720h",
			},
		},
		{
			name:     "create weekly schedule with namespaces",
			schedule: "weekly-backup",
			opts: ScheduleOptions{
				Cron:               "0 0 * * 0",
				IncludedNamespaces: []string{"default", "app"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := client.CreateSchedule(ctx, tt.schedule, tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("CreateSchedule() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVeleroClient_ListSchedules(t *testing.T) {
	schedule := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "velero.io/v1",
			"kind":       "Schedule",
			"metadata": map[string]interface{}{
				"name":      "daily-backup",
				"namespace": VeleroNamespace,
			},
			"spec": map[string]interface{}{
				"schedule": "0 2 * * *",
				"template": map[string]interface{}{
					"ttl": "720h",
				},
			},
			"status": map[string]interface{}{
				"phase": "Enabled",
			},
		},
	}

	client := newTestClient(schedule)
	ctx := context.Background()

	schedules, err := client.ListSchedules(ctx)
	if err != nil {
		t.Fatalf("ListSchedules() error = %v", err)
	}

	if len(schedules) != 1 {
		t.Errorf("ListSchedules() got %d schedules, want 1", len(schedules))
	}
	if schedules[0].Schedule != "0 2 * * *" {
		t.Errorf("ListSchedules() schedule = %v, want '0 2 * * *'", schedules[0].Schedule)
	}
}

func TestParseBackupInfo(t *testing.T) {
	now := time.Now()
	startTime := now.Add(-1 * time.Hour)
	completionTime := now.Add(-30 * time.Minute)

	backup := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "velero.io/v1",
			"kind":       "Backup",
			"metadata": map[string]interface{}{
				"name":      "test-backup",
				"namespace": VeleroNamespace,
			},
			"status": map[string]interface{}{
				"phase":               "Completed",
				"startTimestamp":      startTime.Format(time.RFC3339),
				"completionTimestamp": completionTime.Format(time.RFC3339),
				"progress": map[string]interface{}{
					"itemsBackedUp": int64(50),
					"totalItems":    int64(100),
				},
				"errors":   int64(0),
				"warnings": int64(2),
			},
			"spec": map[string]interface{}{
				"includedNamespaces": []interface{}{"default", "app"},
				"excludedNamespaces": []interface{}{"kube-system"},
			},
		},
	}

	info := parseBackupInfo(backup)

	if info.Name != "test-backup" {
		t.Errorf("parseBackupInfo() name = %v, want test-backup", info.Name)
	}
	if info.Status != "Completed" {
		t.Errorf("parseBackupInfo() status = %v, want Completed", info.Status)
	}
	if info.ItemsBackedUp != 50 {
		t.Errorf("parseBackupInfo() itemsBackedUp = %v, want 50", info.ItemsBackedUp)
	}
	if info.TotalItems != 100 {
		t.Errorf("parseBackupInfo() totalItems = %v, want 100", info.TotalItems)
	}
	if info.Warnings != 2 {
		t.Errorf("parseBackupInfo() warnings = %v, want 2", info.Warnings)
	}
	if len(info.IncludedNamespaces) != 2 {
		t.Errorf("parseBackupInfo() includedNamespaces count = %v, want 2", len(info.IncludedNamespaces))
	}
	if len(info.ExcludedNamespaces) != 1 {
		t.Errorf("parseBackupInfo() excludedNamespaces count = %v, want 1", len(info.ExcludedNamespaces))
	}
}

func TestParseRestoreInfo(t *testing.T) {
	restore := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "velero.io/v1",
			"kind":       "Restore",
			"metadata": map[string]interface{}{
				"name":      "test-restore",
				"namespace": VeleroNamespace,
			},
			"spec": map[string]interface{}{
				"backupName": "test-backup",
			},
			"status": map[string]interface{}{
				"phase": "Completed",
				"progress": map[string]interface{}{
					"itemsRestored": int64(50),
				},
				"errors":   int64(1),
				"warnings": int64(0),
			},
		},
	}

	info := parseRestoreInfo(restore)

	if info.Name != "test-restore" {
		t.Errorf("parseRestoreInfo() name = %v, want test-restore", info.Name)
	}
	if info.BackupName != "test-backup" {
		t.Errorf("parseRestoreInfo() backupName = %v, want test-backup", info.BackupName)
	}
	if info.Status != "Completed" {
		t.Errorf("parseRestoreInfo() status = %v, want Completed", info.Status)
	}
	if info.ItemsRestored != 50 {
		t.Errorf("parseRestoreInfo() itemsRestored = %v, want 50", info.ItemsRestored)
	}
	if info.Errors != 1 {
		t.Errorf("parseRestoreInfo() errors = %v, want 1", info.Errors)
	}
}

func TestParseScheduleInfo(t *testing.T) {
	lastBackup := time.Now().Add(-24 * time.Hour)

	schedule := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "velero.io/v1",
			"kind":       "Schedule",
			"metadata": map[string]interface{}{
				"name":      "daily-backup",
				"namespace": VeleroNamespace,
			},
			"spec": map[string]interface{}{
				"schedule": "0 2 * * *",
				"template": map[string]interface{}{
					"ttl":                "720h",
					"includedNamespaces": []interface{}{"default"},
					"excludedNamespaces": []interface{}{"kube-system", "velero"},
				},
			},
			"status": map[string]interface{}{
				"phase":      "Enabled",
				"lastBackup": lastBackup.Format(time.RFC3339),
			},
		},
	}

	info := parseScheduleInfo(schedule)

	if info.Name != "daily-backup" {
		t.Errorf("parseScheduleInfo() name = %v, want daily-backup", info.Name)
	}
	if info.Schedule != "0 2 * * *" {
		t.Errorf("parseScheduleInfo() schedule = %v, want '0 2 * * *'", info.Schedule)
	}
	if info.Phase != "Enabled" {
		t.Errorf("parseScheduleInfo() phase = %v, want Enabled", info.Phase)
	}
	if info.TTL != "720h" {
		t.Errorf("parseScheduleInfo() ttl = %v, want 720h", info.TTL)
	}
	if len(info.IncludedNamespaces) != 1 {
		t.Errorf("parseScheduleInfo() includedNamespaces count = %v, want 1", len(info.IncludedNamespaces))
	}
	if len(info.ExcludedNamespaces) != 2 {
		t.Errorf("parseScheduleInfo() excludedNamespaces count = %v, want 2", len(info.ExcludedNamespaces))
	}
}

func TestVeleroClient_DeleteBackup(t *testing.T) {
	backup := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "velero.io/v1",
			"kind":       "Backup",
			"metadata": map[string]interface{}{
				"name":      "test-backup",
				"namespace": VeleroNamespace,
			},
		},
	}

	client := newTestClient(backup)
	ctx := context.Background()

	err := client.DeleteBackup(ctx, "test-backup")
	if err != nil {
		t.Fatalf("DeleteBackup() error = %v", err)
	}

	// Verify backup is deleted
	_, err = client.GetBackup(ctx, "test-backup")
	if err == nil {
		t.Error("DeleteBackup() backup still exists after deletion")
	}
}

func TestVeleroClient_DeleteSchedule(t *testing.T) {
	schedule := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "velero.io/v1",
			"kind":       "Schedule",
			"metadata": map[string]interface{}{
				"name":      "daily-backup",
				"namespace": VeleroNamespace,
			},
		},
	}

	client := newTestClient(schedule)
	ctx := context.Background()

	err := client.DeleteSchedule(ctx, "daily-backup")
	if err != nil {
		t.Fatalf("DeleteSchedule() error = %v", err)
	}

	// Verify schedule is deleted
	_, err = client.GetSchedule(ctx, "daily-backup")
	if err == nil {
		t.Error("DeleteSchedule() schedule still exists after deletion")
	}
}

func TestFormatTime(t *testing.T) {
	tests := []struct {
		name     string
		time     time.Time
		wantHas  string // substring that should be in the result
	}{
		{
			name:    "just now",
			time:    time.Now().Add(-30 * time.Second),
			wantHas: "just now",
		},
		{
			name:    "minutes ago",
			time:    time.Now().Add(-30 * time.Minute),
			wantHas: "m ago",
		},
		{
			name:    "hours ago",
			time:    time.Now().Add(-5 * time.Hour),
			wantHas: "h ago",
		},
		{
			name:    "days ago",
			time:    time.Now().Add(-3 * 24 * time.Hour),
			wantHas: "d ago",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTime(tt.time)
			if len(result) == 0 {
				t.Error("formatTime() returned empty string")
			}
			// Check that the result contains expected substring
			found := false
			if tt.wantHas != "" && len(result) >= len(tt.wantHas) {
				for i := 0; i <= len(result)-len(tt.wantHas); i++ {
					if result[i:i+len(tt.wantHas)] == tt.wantHas {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("formatTime() = %v, want substring %v", result, tt.wantHas)
				}
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		max    int
		want   string
	}{
		{
			name:  "no truncation needed",
			input: "short",
			max:   10,
			want:  "short",
		},
		{
			name:  "exact length",
			input: "exact",
			max:   5,
			want:  "exact",
		},
		{
			name:  "needs truncation",
			input: "this-is-a-very-long-backup-name",
			max:   15,
			want:  "this-is-a-ve...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncate(tt.input, tt.max)
			if result != tt.want {
				t.Errorf("truncate() = %v, want %v", result, tt.want)
			}
		})
	}
}

// Test that GVR definitions are correct
func TestGVRDefinitions(t *testing.T) {
	if backupGVR.Group != "velero.io" {
		t.Errorf("backupGVR.Group = %v, want velero.io", backupGVR.Group)
	}
	if backupGVR.Version != "v1" {
		t.Errorf("backupGVR.Version = %v, want v1", backupGVR.Version)
	}
	if backupGVR.Resource != "backups" {
		t.Errorf("backupGVR.Resource = %v, want backups", backupGVR.Resource)
	}

	if restoreGVR.Resource != "restores" {
		t.Errorf("restoreGVR.Resource = %v, want restores", restoreGVR.Resource)
	}

	if scheduleGVR.Resource != "schedules" {
		t.Errorf("scheduleGVR.Resource = %v, want schedules", scheduleGVR.Resource)
	}
}

// Test BackupOptions fields
func TestBackupOptions(t *testing.T) {
	snapshotVolumes := true
	opts := BackupOptions{
		IncludedNamespaces: []string{"default", "app"},
		ExcludedNamespaces: []string{"kube-system"},
		TTL:                "720h",
		SnapshotVolumes:    &snapshotVolumes,
		StorageLocation:    "default",
	}

	if len(opts.IncludedNamespaces) != 2 {
		t.Errorf("BackupOptions.IncludedNamespaces = %v, want 2 items", len(opts.IncludedNamespaces))
	}
	if opts.TTL != "720h" {
		t.Errorf("BackupOptions.TTL = %v, want 720h", opts.TTL)
	}
	if *opts.SnapshotVolumes != true {
		t.Errorf("BackupOptions.SnapshotVolumes = %v, want true", *opts.SnapshotVolumes)
	}
}

// Test RestoreOptions fields
func TestRestoreOptions(t *testing.T) {
	restorePVs := true
	opts := RestoreOptions{
		IncludedNamespaces: []string{"default"},
		ExcludedNamespaces: []string{"kube-system"},
		RestorePVs:         &restorePVs,
	}

	if len(opts.IncludedNamespaces) != 1 {
		t.Errorf("RestoreOptions.IncludedNamespaces = %v, want 1 item", len(opts.IncludedNamespaces))
	}
	if *opts.RestorePVs != true {
		t.Errorf("RestoreOptions.RestorePVs = %v, want true", *opts.RestorePVs)
	}
}

// Test ScheduleOptions fields
func TestScheduleOptions(t *testing.T) {
	snapshotVolumes := false
	opts := ScheduleOptions{
		Cron:               "0 2 * * *",
		IncludedNamespaces: []string{"default"},
		ExcludedNamespaces: []string{"kube-system", "velero"},
		TTL:                "720h",
		SnapshotVolumes:    &snapshotVolumes,
	}

	if opts.Cron != "0 2 * * *" {
		t.Errorf("ScheduleOptions.Cron = %v, want '0 2 * * *'", opts.Cron)
	}
	if opts.TTL != "720h" {
		t.Errorf("ScheduleOptions.TTL = %v, want 720h", opts.TTL)
	}
}

// Test BackupInfo struct
func TestBackupInfoStruct(t *testing.T) {
	now := time.Now()
	info := BackupInfo{
		Name:              "test-backup",
		Status:            "Completed",
		StartTimestamp:    &now,
		CompletionTime:    &now,
		ItemsBackedUp:     100,
		TotalItems:        100,
		IncludedNamespaces: []string{"default"},
		ExcludedNamespaces: []string{"kube-system"},
		Errors:            0,
		Warnings:          1,
	}

	if info.Name != "test-backup" {
		t.Errorf("BackupInfo.Name = %v, want test-backup", info.Name)
	}
	if info.Warnings != 1 {
		t.Errorf("BackupInfo.Warnings = %v, want 1", info.Warnings)
	}
}

// Test RestoreInfo struct
func TestRestoreInfoStruct(t *testing.T) {
	now := time.Now()
	info := RestoreInfo{
		Name:           "test-restore",
		BackupName:     "test-backup",
		Status:         "Completed",
		StartTimestamp: &now,
		CompletionTime: &now,
		ItemsRestored:  50,
		Errors:         0,
		Warnings:       0,
	}

	if info.Name != "test-restore" {
		t.Errorf("RestoreInfo.Name = %v, want test-restore", info.Name)
	}
	if info.BackupName != "test-backup" {
		t.Errorf("RestoreInfo.BackupName = %v, want test-backup", info.BackupName)
	}
}

// Test ScheduleInfo struct
func TestScheduleInfoStruct(t *testing.T) {
	now := time.Now()
	info := ScheduleInfo{
		Name:               "daily-backup",
		Schedule:           "0 2 * * *",
		LastBackup:         &now,
		Phase:              "Enabled",
		IncludedNamespaces: []string{"default"},
		ExcludedNamespaces: []string{"kube-system"},
		TTL:                "720h",
	}

	if info.Schedule != "0 2 * * *" {
		t.Errorf("ScheduleInfo.Schedule = %v, want '0 2 * * *'", info.Schedule)
	}
	if info.Phase != "Enabled" {
		t.Errorf("ScheduleInfo.Phase = %v, want Enabled", info.Phase)
	}
}

// Test parsing backup with nil timestamps
func TestParseBackupInfo_NilTimestamps(t *testing.T) {
	backup := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "velero.io/v1",
			"kind":       "Backup",
			"metadata": map[string]interface{}{
				"name":      "test-backup",
				"namespace": VeleroNamespace,
			},
			"status": map[string]interface{}{
				"phase": "New",
			},
		},
	}

	info := parseBackupInfo(backup)

	if info.StartTimestamp != nil {
		t.Error("parseBackupInfo() startTimestamp should be nil for new backup")
	}
	if info.CompletionTime != nil {
		t.Error("parseBackupInfo() completionTime should be nil for new backup")
	}
}

// Test parsing restore with nil timestamps
func TestParseRestoreInfo_NilTimestamps(t *testing.T) {
	restore := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "velero.io/v1",
			"kind":       "Restore",
			"metadata": map[string]interface{}{
				"name":      "test-restore",
				"namespace": VeleroNamespace,
			},
			"spec": map[string]interface{}{
				"backupName": "test-backup",
			},
			"status": map[string]interface{}{
				"phase": "New",
			},
		},
	}

	info := parseRestoreInfo(restore)

	if info.StartTimestamp != nil {
		t.Error("parseRestoreInfo() startTimestamp should be nil for new restore")
	}
}

// Test parsing schedule with no last backup
func TestParseScheduleInfo_NoLastBackup(t *testing.T) {
	schedule := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "velero.io/v1",
			"kind":       "Schedule",
			"metadata": map[string]interface{}{
				"name":      "new-schedule",
				"namespace": VeleroNamespace,
			},
			"spec": map[string]interface{}{
				"schedule": "0 2 * * *",
			},
			"status": map[string]interface{}{
				"phase": "Enabled",
			},
		},
	}

	info := parseScheduleInfo(schedule)

	if info.LastBackup != nil {
		t.Error("parseScheduleInfo() lastBackup should be nil for new schedule")
	}
}

// Test constants
func TestConstants(t *testing.T) {
	if VeleroNamespace != "velero" {
		t.Errorf("VeleroNamespace = %v, want velero", VeleroNamespace)
	}
}

// Compile-time interface check
var _ metav1.Object = &unstructured.Unstructured{}
