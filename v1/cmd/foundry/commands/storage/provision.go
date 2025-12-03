package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/urfave/cli/v3"
)

// ProvisionCommand creates a new PVC
var ProvisionCommand = &cli.Command{
	Name:      "provision",
	Usage:     "Provision a new Persistent Volume Claim",
	ArgsUsage: "<name>",
	Description: `Creates a new Persistent Volume Claim (PVC) in the cluster.

The PVC will be created using the default StorageClass unless specified.

Examples:
  foundry storage provision my-data --size 10Gi
  foundry storage provision my-data --size 10Gi --namespace app
  foundry storage provision my-data --size 10Gi --storage-class local-path
  foundry storage provision my-data --size 10Gi --access-mode ReadWriteMany`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "size",
			Aliases:  []string{"s"},
			Usage:    "Size of the PVC (e.g., 1Gi, 10Gi, 100Gi)",
			Required: true,
		},
		&cli.StringFlag{
			Name:    "namespace",
			Aliases: []string{"n"},
			Usage:   "Namespace for the PVC",
			Value:   "default",
		},
		&cli.StringFlag{
			Name:  "storage-class",
			Usage: "StorageClass to use (default: cluster default)",
		},
		&cli.StringFlag{
			Name:  "access-mode",
			Usage: "Access mode: ReadWriteOnce, ReadWriteMany, ReadOnlyMany",
			Value: "ReadWriteOnce",
		},
		&cli.BoolFlag{
			Name:  "wait",
			Usage: "Wait for PVC to be bound",
			Value: false,
		},
		&cli.DurationFlag{
			Name:  "timeout",
			Usage: "Timeout when waiting for PVC to bind (used with --wait)",
			Value: 2 * time.Minute,
		},
	},
	Action: runProvision,
}

func runProvision(ctx context.Context, cmd *cli.Command) error {
	// Get PVC name
	name := cmd.Args().Get(0)
	if name == "" {
		return fmt.Errorf("PVC name is required\n\nUsage: foundry storage provision <name> --size <size>")
	}

	// Parse size
	size := cmd.String("size")
	quantity, err := resource.ParseQuantity(size)
	if err != nil {
		return fmt.Errorf("invalid size format: %w\n\nUse Kubernetes quantity format (e.g., 1Gi, 10Gi, 100Gi)", err)
	}

	// Parse access mode
	accessModeStr := cmd.String("access-mode")
	var accessMode corev1.PersistentVolumeAccessMode
	switch accessModeStr {
	case "ReadWriteOnce", "RWO":
		accessMode = corev1.ReadWriteOnce
	case "ReadWriteMany", "RWX":
		accessMode = corev1.ReadWriteMany
	case "ReadOnlyMany", "ROX":
		accessMode = corev1.ReadOnlyMany
	default:
		return fmt.Errorf("invalid access mode: %s\n\nValid modes: ReadWriteOnce, ReadWriteMany, ReadOnlyMany", accessModeStr)
	}

	// Get K8s client
	client, err := getK8sClient()
	if err != nil {
		return err
	}

	namespace := cmd.String("namespace")
	storageClass := cmd.String("storage-class")

	// Build PVC
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{accessMode},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: quantity,
				},
			},
		},
	}

	// Set storage class if specified
	if storageClass != "" {
		pvc.Spec.StorageClassName = &storageClass
	}

	// Create PVC
	fmt.Printf("Creating PVC %q in namespace %q...\n", name, namespace)
	_, err = client.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create PVC: %w", err)
	}

	fmt.Printf("PVC %q created\n", name)

	// Wait for binding if requested
	if cmd.Bool("wait") {
		fmt.Println("Waiting for PVC to be bound...")
		timeout := cmd.Duration("timeout")
		if err := waitForPVCBound(ctx, client, namespace, name, timeout); err != nil {
			return err
		}
	} else {
		fmt.Println("\nTo check PVC status, run:")
		fmt.Printf("  foundry storage pvc list --namespace %s\n", namespace)
	}

	return nil
}

// waitForPVCBound waits for a PVC to be bound
func waitForPVCBound(ctx context.Context, client *kubernetes.Clientset, namespace, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for PVC to be bound")
			}

			pvc, err := client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("failed to get PVC status: %w", err)
			}

			switch pvc.Status.Phase {
			case corev1.ClaimBound:
				fmt.Printf("\nPVC %q bound to volume %q\n", name, pvc.Spec.VolumeName)
				return nil
			case corev1.ClaimLost:
				return fmt.Errorf("PVC is in Lost state")
			case corev1.ClaimPending:
				fmt.Printf("\r  Status: Pending")
			}
		}
	}
}

// getK8sClient creates a Kubernetes client from kubeconfig
func getK8sClient() (*kubernetes.Clientset, error) {
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

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return clientset, nil
}
