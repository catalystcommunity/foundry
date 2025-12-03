package storage

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/urfave/cli/v3"
)

// PVCCommand is the parent command for PVC operations
var PVCCommand = &cli.Command{
	Name:  "pvc",
	Usage: "Manage Persistent Volume Claims",
	Description: `Commands for managing Persistent Volume Claims in the cluster.

Examples:
  foundry storage pvc list              # List all PVCs
  foundry storage pvc list -n app       # List PVCs in namespace`,
	Commands: []*cli.Command{
		PVCListCommand,
		PVCDeleteCommand,
	},
}

// PVCListCommand lists PVCs
var PVCListCommand = &cli.Command{
	Name:  "list",
	Usage: "List Persistent Volume Claims",
	Description: `Lists all PVCs in the cluster or a specific namespace.

Examples:
  foundry storage pvc list              # List all PVCs
  foundry storage pvc list -n default   # List PVCs in namespace
  foundry storage pvc list --all        # List PVCs in all namespaces`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "namespace",
			Aliases: []string{"n"},
			Usage:   "Namespace to list PVCs from",
			Value:   "default",
		},
		&cli.BoolFlag{
			Name:    "all-namespaces",
			Aliases: []string{"A"},
			Usage:   "List PVCs from all namespaces",
		},
	},
	Action: runPVCList,
}

// PVCDeleteCommand deletes a PVC
var PVCDeleteCommand = &cli.Command{
	Name:      "delete",
	Usage:     "Delete a Persistent Volume Claim",
	ArgsUsage: "<name>",
	Description: `Deletes a PVC from the cluster.

Examples:
  foundry storage pvc delete my-pvc
  foundry storage pvc delete my-pvc -n app`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "namespace",
			Aliases: []string{"n"},
			Usage:   "Namespace of the PVC",
			Value:   "default",
		},
		&cli.BoolFlag{
			Name:    "force",
			Aliases: []string{"f"},
			Usage:   "Skip confirmation prompt",
		},
	},
	Action: runPVCDelete,
}

func runPVCList(ctx context.Context, cmd *cli.Command) error {
	// Get K8s client
	client, err := getK8sClient()
	if err != nil {
		return err
	}

	namespace := cmd.String("namespace")
	allNamespaces := cmd.Bool("all-namespaces")

	if allNamespaces {
		namespace = metav1.NamespaceAll
	}

	// List PVCs
	pvcs, err := client.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list PVCs: %w", err)
	}

	if len(pvcs.Items) == 0 {
		if allNamespaces {
			fmt.Println("No PVCs found in any namespace")
		} else {
			fmt.Printf("No PVCs found in namespace %q\n", namespace)
		}
		return nil
	}

	// Print header
	if allNamespaces {
		fmt.Printf("%-20s %-30s %-10s %-10s %-20s %-30s\n", "NAMESPACE", "NAME", "STATUS", "SIZE", "STORAGE CLASS", "VOLUME")
		fmt.Println(strings.Repeat("-", 120))
	} else {
		fmt.Printf("%-30s %-10s %-10s %-20s %-30s\n", "NAME", "STATUS", "SIZE", "STORAGE CLASS", "VOLUME")
		fmt.Println(strings.Repeat("-", 100))
	}

	// Print PVCs
	for _, pvc := range pvcs.Items {
		status := string(pvc.Status.Phase)
		size := ""
		if capacity, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
			size = capacity.String()
		} else if requests, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			size = requests.String()
		}

		storageClass := "<default>"
		if pvc.Spec.StorageClassName != nil && *pvc.Spec.StorageClassName != "" {
			storageClass = *pvc.Spec.StorageClassName
		}

		volume := "-"
		if pvc.Spec.VolumeName != "" {
			volume = truncatePVC(pvc.Spec.VolumeName, 30)
		}

		if allNamespaces {
			fmt.Printf("%-20s %-30s %-10s %-10s %-20s %-30s\n",
				pvc.Namespace,
				truncatePVC(pvc.Name, 30),
				status,
				size,
				truncatePVC(storageClass, 20),
				volume,
			)
		} else {
			fmt.Printf("%-30s %-10s %-10s %-20s %-30s\n",
				truncatePVC(pvc.Name, 30),
				status,
				size,
				truncatePVC(storageClass, 20),
				volume,
			)
		}
	}

	return nil
}

func runPVCDelete(ctx context.Context, cmd *cli.Command) error {
	// Get PVC name
	name := cmd.Args().Get(0)
	if name == "" {
		return fmt.Errorf("PVC name is required\n\nUsage: foundry storage pvc delete <name>")
	}

	// Get K8s client
	client, err := getK8sClient()
	if err != nil {
		return err
	}

	namespace := cmd.String("namespace")

	// Check if PVC exists
	_, err = client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("PVC %q not found in namespace %q: %w", name, namespace, err)
	}

	// Confirm unless --force
	if !cmd.Bool("force") {
		fmt.Printf("Are you sure you want to delete PVC %q in namespace %q?\n", name, namespace)
		fmt.Printf("This will delete any data stored in the volume.\n")
		fmt.Print("Type 'yes' to confirm: ")
		var response string
		fmt.Scanln(&response)
		if response != "yes" {
			fmt.Println("Aborted")
			return nil
		}
	}

	// Delete PVC
	fmt.Printf("Deleting PVC %q...\n", name)
	err = client.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete PVC: %w", err)
	}

	fmt.Printf("PVC %q deleted\n", name)
	return nil
}

// truncatePVC truncates a string for display
func truncatePVC(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
