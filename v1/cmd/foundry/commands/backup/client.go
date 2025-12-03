package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	// VeleroNamespace is the default namespace where Velero is installed
	VeleroNamespace = "velero"
)

// Velero GroupVersionResource definitions
var (
	backupGVR = schema.GroupVersionResource{
		Group:    "velero.io",
		Version:  "v1",
		Resource: "backups",
	}

	restoreGVR = schema.GroupVersionResource{
		Group:    "velero.io",
		Version:  "v1",
		Resource: "restores",
	}

	scheduleGVR = schema.GroupVersionResource{
		Group:    "velero.io",
		Version:  "v1",
		Resource: "schedules",
	}
)

// BackupInfo contains information about a Velero backup
type BackupInfo struct {
	Name              string
	Status            string
	StartTimestamp    *time.Time
	CompletionTime    *time.Time
	ExpirationTime    *time.Time
	ItemsBackedUp     int64
	TotalItems        int64
	IncludedNamespaces []string
	ExcludedNamespaces []string
	Errors            int64
	Warnings          int64
}

// RestoreInfo contains information about a Velero restore
type RestoreInfo struct {
	Name             string
	BackupName       string
	Status           string
	StartTimestamp   *time.Time
	CompletionTime   *time.Time
	ItemsRestored    int64
	Errors           int64
	Warnings         int64
}

// ScheduleInfo contains information about a Velero schedule
type ScheduleInfo struct {
	Name               string
	Schedule           string
	LastBackup         *time.Time
	Phase              string
	IncludedNamespaces []string
	ExcludedNamespaces []string
	TTL                string
}

// VeleroClient wraps the dynamic client for Velero operations
type VeleroClient struct {
	dynamicClient dynamic.Interface
	namespace     string
}

// NewVeleroClient creates a new Velero client using the kubeconfig from ~/.foundry/kubeconfig
func NewVeleroClient() (*VeleroClient, error) {
	// Get kubeconfig path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	kubeconfigPath := filepath.Join(homeDir, ".foundry", "kubeconfig")

	// Check if kubeconfig exists
	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("kubeconfig not found at %s\n\nHint: Run 'foundry cluster init' first", kubeconfigPath)
	}

	// Build config from kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build config from kubeconfig: %w", err)
	}

	// Create dynamic client
	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return &VeleroClient{
		dynamicClient: dynamicClient,
		namespace:     VeleroNamespace,
	}, nil
}

// CreateBackup creates a new Velero backup
func (c *VeleroClient) CreateBackup(ctx context.Context, name string, opts BackupOptions) error {
	// Build the backup spec
	spec := map[string]interface{}{}

	if len(opts.IncludedNamespaces) > 0 {
		spec["includedNamespaces"] = toInterfaceSlice(opts.IncludedNamespaces)
	}
	if len(opts.ExcludedNamespaces) > 0 {
		spec["excludedNamespaces"] = toInterfaceSlice(opts.ExcludedNamespaces)
	}
	if opts.TTL != "" {
		spec["ttl"] = opts.TTL
	}
	if opts.SnapshotVolumes != nil {
		spec["snapshotVolumes"] = *opts.SnapshotVolumes
	}
	if opts.StorageLocation != "" {
		spec["storageLocation"] = opts.StorageLocation
	}

	// Create the backup resource
	backup := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "velero.io/v1",
			"kind":       "Backup",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": c.namespace,
			},
			"spec": spec,
		},
	}

	_, err := c.dynamicClient.Resource(backupGVR).Namespace(c.namespace).Create(ctx, backup, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	return nil
}

// BackupOptions contains options for creating a backup
type BackupOptions struct {
	IncludedNamespaces []string
	ExcludedNamespaces []string
	TTL                string
	SnapshotVolumes    *bool
	StorageLocation    string
}

// ListBackups lists all Velero backups
func (c *VeleroClient) ListBackups(ctx context.Context) ([]BackupInfo, error) {
	list, err := c.dynamicClient.Resource(backupGVR).Namespace(c.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list backups: %w", err)
	}

	backups := make([]BackupInfo, 0, len(list.Items))
	for _, item := range list.Items {
		info := parseBackupInfo(&item)
		backups = append(backups, info)
	}

	return backups, nil
}

// GetBackup retrieves a specific backup
func (c *VeleroClient) GetBackup(ctx context.Context, name string) (*BackupInfo, error) {
	backup, err := c.dynamicClient.Resource(backupGVR).Namespace(c.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get backup: %w", err)
	}

	info := parseBackupInfo(backup)
	return &info, nil
}

// DeleteBackup deletes a backup
func (c *VeleroClient) DeleteBackup(ctx context.Context, name string) error {
	err := c.dynamicClient.Resource(backupGVR).Namespace(c.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete backup: %w", err)
	}
	return nil
}

// CreateRestore creates a new Velero restore from a backup
func (c *VeleroClient) CreateRestore(ctx context.Context, name, backupName string, opts RestoreOptions) error {
	// Build the restore spec
	spec := map[string]interface{}{
		"backupName": backupName,
	}

	if len(opts.IncludedNamespaces) > 0 {
		spec["includedNamespaces"] = toInterfaceSlice(opts.IncludedNamespaces)
	}
	if len(opts.ExcludedNamespaces) > 0 {
		spec["excludedNamespaces"] = toInterfaceSlice(opts.ExcludedNamespaces)
	}
	if opts.RestorePVs != nil {
		spec["restorePVs"] = *opts.RestorePVs
	}

	// Create the restore resource
	restore := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "velero.io/v1",
			"kind":       "Restore",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": c.namespace,
			},
			"spec": spec,
		},
	}

	_, err := c.dynamicClient.Resource(restoreGVR).Namespace(c.namespace).Create(ctx, restore, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create restore: %w", err)
	}

	return nil
}

// RestoreOptions contains options for creating a restore
type RestoreOptions struct {
	IncludedNamespaces []string
	ExcludedNamespaces []string
	RestorePVs         *bool
}

// ListRestores lists all Velero restores
func (c *VeleroClient) ListRestores(ctx context.Context) ([]RestoreInfo, error) {
	list, err := c.dynamicClient.Resource(restoreGVR).Namespace(c.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list restores: %w", err)
	}

	restores := make([]RestoreInfo, 0, len(list.Items))
	for _, item := range list.Items {
		info := parseRestoreInfo(&item)
		restores = append(restores, info)
	}

	return restores, nil
}

// GetRestore retrieves a specific restore
func (c *VeleroClient) GetRestore(ctx context.Context, name string) (*RestoreInfo, error) {
	restore, err := c.dynamicClient.Resource(restoreGVR).Namespace(c.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get restore: %w", err)
	}

	info := parseRestoreInfo(restore)
	return &info, nil
}

// CreateSchedule creates a new Velero schedule
func (c *VeleroClient) CreateSchedule(ctx context.Context, name string, opts ScheduleOptions) error {
	// Build the schedule spec
	template := map[string]interface{}{}
	if len(opts.IncludedNamespaces) > 0 {
		template["includedNamespaces"] = toInterfaceSlice(opts.IncludedNamespaces)
	}
	if len(opts.ExcludedNamespaces) > 0 {
		template["excludedNamespaces"] = toInterfaceSlice(opts.ExcludedNamespaces)
	}
	if opts.TTL != "" {
		template["ttl"] = opts.TTL
	}
	if opts.SnapshotVolumes != nil {
		template["snapshotVolumes"] = *opts.SnapshotVolumes
	}

	spec := map[string]interface{}{
		"schedule": opts.Cron,
		"template": template,
	}

	// Create the schedule resource
	schedule := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "velero.io/v1",
			"kind":       "Schedule",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": c.namespace,
			},
			"spec": spec,
		},
	}

	_, err := c.dynamicClient.Resource(scheduleGVR).Namespace(c.namespace).Create(ctx, schedule, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create schedule: %w", err)
	}

	return nil
}

// ScheduleOptions contains options for creating a schedule
type ScheduleOptions struct {
	Cron               string
	IncludedNamespaces []string
	ExcludedNamespaces []string
	TTL                string
	SnapshotVolumes    *bool
}

// ListSchedules lists all Velero schedules
func (c *VeleroClient) ListSchedules(ctx context.Context) ([]ScheduleInfo, error) {
	list, err := c.dynamicClient.Resource(scheduleGVR).Namespace(c.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list schedules: %w", err)
	}

	schedules := make([]ScheduleInfo, 0, len(list.Items))
	for _, item := range list.Items {
		info := parseScheduleInfo(&item)
		schedules = append(schedules, info)
	}

	return schedules, nil
}

// GetSchedule retrieves a specific schedule
func (c *VeleroClient) GetSchedule(ctx context.Context, name string) (*ScheduleInfo, error) {
	schedule, err := c.dynamicClient.Resource(scheduleGVR).Namespace(c.namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get schedule: %w", err)
	}

	info := parseScheduleInfo(schedule)
	return &info, nil
}

// DeleteSchedule deletes a schedule
func (c *VeleroClient) DeleteSchedule(ctx context.Context, name string) error {
	err := c.dynamicClient.Resource(scheduleGVR).Namespace(c.namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete schedule: %w", err)
	}
	return nil
}

// Helper functions to parse Velero resources

func parseBackupInfo(u *unstructured.Unstructured) BackupInfo {
	info := BackupInfo{
		Name: u.GetName(),
	}

	status, _, _ := unstructured.NestedString(u.Object, "status", "phase")
	info.Status = status

	if startTime, ok, _ := unstructured.NestedString(u.Object, "status", "startTimestamp"); ok {
		if t, err := time.Parse(time.RFC3339, startTime); err == nil {
			info.StartTimestamp = &t
		}
	}

	if completionTime, ok, _ := unstructured.NestedString(u.Object, "status", "completionTimestamp"); ok {
		if t, err := time.Parse(time.RFC3339, completionTime); err == nil {
			info.CompletionTime = &t
		}
	}

	if expirationTime, ok, _ := unstructured.NestedString(u.Object, "status", "expiration"); ok {
		if t, err := time.Parse(time.RFC3339, expirationTime); err == nil {
			info.ExpirationTime = &t
		}
	}

	info.ItemsBackedUp, _, _ = unstructured.NestedInt64(u.Object, "status", "progress", "itemsBackedUp")
	info.TotalItems, _, _ = unstructured.NestedInt64(u.Object, "status", "progress", "totalItems")
	info.Errors, _, _ = unstructured.NestedInt64(u.Object, "status", "errors")
	info.Warnings, _, _ = unstructured.NestedInt64(u.Object, "status", "warnings")

	includedNS, _, _ := unstructured.NestedStringSlice(u.Object, "spec", "includedNamespaces")
	info.IncludedNamespaces = includedNS

	excludedNS, _, _ := unstructured.NestedStringSlice(u.Object, "spec", "excludedNamespaces")
	info.ExcludedNamespaces = excludedNS

	return info
}

func parseRestoreInfo(u *unstructured.Unstructured) RestoreInfo {
	info := RestoreInfo{
		Name: u.GetName(),
	}

	info.BackupName, _, _ = unstructured.NestedString(u.Object, "spec", "backupName")
	info.Status, _, _ = unstructured.NestedString(u.Object, "status", "phase")

	if startTime, ok, _ := unstructured.NestedString(u.Object, "status", "startTimestamp"); ok {
		if t, err := time.Parse(time.RFC3339, startTime); err == nil {
			info.StartTimestamp = &t
		}
	}

	if completionTime, ok, _ := unstructured.NestedString(u.Object, "status", "completionTimestamp"); ok {
		if t, err := time.Parse(time.RFC3339, completionTime); err == nil {
			info.CompletionTime = &t
		}
	}

	info.ItemsRestored, _, _ = unstructured.NestedInt64(u.Object, "status", "progress", "itemsRestored")
	info.Errors, _, _ = unstructured.NestedInt64(u.Object, "status", "errors")
	info.Warnings, _, _ = unstructured.NestedInt64(u.Object, "status", "warnings")

	return info
}

func parseScheduleInfo(u *unstructured.Unstructured) ScheduleInfo {
	info := ScheduleInfo{
		Name: u.GetName(),
	}

	info.Schedule, _, _ = unstructured.NestedString(u.Object, "spec", "schedule")
	info.Phase, _, _ = unstructured.NestedString(u.Object, "status", "phase")

	if lastBackup, ok, _ := unstructured.NestedString(u.Object, "status", "lastBackup"); ok {
		if t, err := time.Parse(time.RFC3339, lastBackup); err == nil {
			info.LastBackup = &t
		}
	}

	includedNS, _, _ := unstructured.NestedStringSlice(u.Object, "spec", "template", "includedNamespaces")
	info.IncludedNamespaces = includedNS

	excludedNS, _, _ := unstructured.NestedStringSlice(u.Object, "spec", "template", "excludedNamespaces")
	info.ExcludedNamespaces = excludedNS

	info.TTL, _, _ = unstructured.NestedString(u.Object, "spec", "template", "ttl")

	return info
}

// toInterfaceSlice converts a string slice to an interface slice
// This is required for proper deep copy support in unstructured objects
func toInterfaceSlice(strs []string) []interface{} {
	result := make([]interface{}, len(strs))
	for i, s := range strs {
		result[i] = s
	}
	return result
}
