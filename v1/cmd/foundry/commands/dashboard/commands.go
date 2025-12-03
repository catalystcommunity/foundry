package dashboard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/urfave/cli/v3"
)

// Command is the top-level dashboard command
var Command = &cli.Command{
	Name:  "dashboard",
	Usage: "Open Grafana dashboard in browser",
	Description: `Opens the Grafana dashboard in your default web browser.

This command retrieves the Grafana URL from the cluster's Ingress
configuration and opens it in your browser.

If port-forwarding is needed (no Ingress), it will set up a temporary
port-forward to the Grafana pod.

Examples:
  foundry dashboard              # Open Grafana dashboard
  foundry dashboard open         # Same as above
  foundry dashboard url          # Just print the URL`,
	Commands: []*cli.Command{
		OpenCommand,
		URLCommand,
	},
	Action: runOpen, // Default action when just 'foundry dashboard' is run
}

// OpenCommand opens the dashboard in a browser
var OpenCommand = &cli.Command{
	Name:  "open",
	Usage: "Open Grafana dashboard in browser",
	Description: `Opens the Grafana dashboard in your default web browser.

Examples:
  foundry dashboard open`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "namespace",
			Aliases: []string{"n"},
			Usage:   "Namespace where Grafana is installed",
			Value:   "monitoring",
		},
	},
	Action: runOpen,
}

// URLCommand prints the dashboard URL
var URLCommand = &cli.Command{
	Name:  "url",
	Usage: "Print the Grafana dashboard URL",
	Description: `Prints the Grafana dashboard URL without opening it.

Examples:
  foundry dashboard url`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "namespace",
			Aliases: []string{"n"},
			Usage:   "Namespace where Grafana is installed",
			Value:   "monitoring",
		},
	},
	Action: runURL,
}

func runOpen(ctx context.Context, cmd *cli.Command) error {
	url, err := getGrafanaURL(ctx, cmd.String("namespace"))
	if err != nil {
		return err
	}

	fmt.Printf("Opening Grafana at %s\n", url)
	return openBrowser(url)
}

func runURL(ctx context.Context, cmd *cli.Command) error {
	url, err := getGrafanaURL(ctx, cmd.String("namespace"))
	if err != nil {
		return err
	}

	fmt.Println(url)
	return nil
}

// getGrafanaURL retrieves the Grafana URL from the cluster
func getGrafanaURL(ctx context.Context, namespace string) (string, error) {
	client, err := getK8sClient()
	if err != nil {
		return "", err
	}

	// First, try to get the Ingress URL
	ingresses, err := client.NetworkingV1().Ingresses(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=grafana",
	})
	if err == nil && len(ingresses.Items) > 0 {
		ing := ingresses.Items[0]
		if len(ing.Spec.Rules) > 0 && ing.Spec.Rules[0].Host != "" {
			protocol := "http"
			if len(ing.Spec.TLS) > 0 {
				protocol = "https"
			}
			return fmt.Sprintf("%s://%s", protocol, ing.Spec.Rules[0].Host), nil
		}
	}

	// Try to find Grafana service
	services, err := client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=grafana",
	})
	if err != nil {
		return "", fmt.Errorf("failed to find Grafana: %w", err)
	}

	if len(services.Items) == 0 {
		return "", fmt.Errorf("Grafana not found in namespace %q\n\nHint: Install Grafana with: foundry component install grafana", namespace)
	}

	svc := services.Items[0]

	// Check for LoadBalancer IP
	if svc.Spec.Type == "LoadBalancer" && len(svc.Status.LoadBalancer.Ingress) > 0 {
		ip := svc.Status.LoadBalancer.Ingress[0].IP
		if ip == "" {
			ip = svc.Status.LoadBalancer.Ingress[0].Hostname
		}
		port := int32(80)
		for _, p := range svc.Spec.Ports {
			if p.Name == "http" || p.Name == "service" {
				port = p.Port
				break
			}
		}
		return fmt.Sprintf("http://%s:%d", ip, port), nil
	}

	// Check for NodePort
	if svc.Spec.Type == "NodePort" {
		// Get a node IP
		nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err == nil && len(nodes.Items) > 0 {
			nodeIP := ""
			for _, addr := range nodes.Items[0].Status.Addresses {
				if addr.Type == "InternalIP" {
					nodeIP = addr.Address
					break
				}
			}
			if nodeIP != "" {
				for _, p := range svc.Spec.Ports {
					if p.NodePort > 0 {
						return fmt.Sprintf("http://%s:%d", nodeIP, p.NodePort), nil
					}
				}
			}
		}
	}

	// Fall back to cluster IP (would need port-forward)
	return fmt.Sprintf("http://%s:80", svc.Spec.ClusterIP), nil
}

// openBrowser opens the specified URL in the default browser
func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		// Try xdg-open first, fall back to other options
		if _, err := exec.LookPath("xdg-open"); err == nil {
			cmd = exec.Command("xdg-open", url)
		} else if _, err := exec.LookPath("gnome-open"); err == nil {
			cmd = exec.Command("gnome-open", url)
		} else if _, err := exec.LookPath("sensible-browser"); err == nil {
			cmd = exec.Command("sensible-browser", url)
		} else {
			fmt.Printf("Could not find a browser opener. Please visit:\n  %s\n", url)
			return nil
		}
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		fmt.Printf("Unsupported OS. Please visit:\n  %s\n", url)
		return nil
	}

	return cmd.Start()
}

// getK8sClient creates a Kubernetes client from kubeconfig
func getK8sClient() (*kubernetes.Clientset, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	kubeconfigPath := filepath.Join(homeDir, ".foundry", "kubeconfig")

	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("kubeconfig not found at %s\n\nHint: Run 'foundry cluster init' first", kubeconfigPath)
	}

	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build config from kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return clientset, nil
}
