package gateway

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/gateway"
	"github.com/catalystcommunity/foundry/v1/internal/k8s"
	"github.com/urfave/cli/v3"
)

// Command is the top-level gateway command.
var Command = &cli.Command{
	Name:  "gateway",
	Usage: "Manage the cluster ingress Gateway",
	Commands: []*cli.Command{
		ControllerCommand,
	},
}

// ControllerCommand runs the route-driven listener reconciler.
var ControllerCommand = &cli.Command{
	Name:  "controller",
	Usage: "Open Gateway listeners on the VIP from TLSRoute/TCPRoute resources",
	Description: "Watches TLSRoute and TCPRoute resources that target the Contour Gateway and " +
		"opens the matching L4 listener, Envoy service port (on the cluster VIP), and Envoy " +
		"NetworkPolicy ingress. Apps declare a route in their own chart with the listener port on " +
		"either the parentRef or the backendRefs; " +
		"this controller handles the data-plane plumbing. Listeners it creates are named with a " +
		"'gw-' prefix and pruned when their routes are removed; the built-in HTTP/HTTPS and any " +
		"operator-pinned static listeners are left untouched.",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "gateway-name", Value: "contour", Usage: "name of the Gateway to manage"},
		&cli.StringFlag{Name: "gateway-namespace", Value: "projectcontour", Usage: "namespace of the Gateway and Envoy service"},
		&cli.StringFlag{Name: "envoy-service", Value: "contour-envoy", Usage: "name of the Envoy LoadBalancer/VIP service"},
		&cli.StringFlag{Name: "network-policy", Value: "contour-envoy", Usage: "name of the Envoy NetworkPolicy (empty to skip)"},
		&cli.DurationFlag{Name: "interval", Value: 15 * time.Second, Usage: "resync interval for the watch loop"},
		&cli.BoolFlag{Name: "once", Usage: "run a single reconcile pass and exit"},
		&cli.StringFlag{Name: "kubeconfig", Usage: "path to kubeconfig (default: in-cluster when running as a pod, else ~/.foundry/kubeconfig)"},
	},
	Action: runController,
}

func runController(ctx context.Context, cmd *cli.Command) error {
	k8sClient, err := newClusterClient(cmd.String("kubeconfig"))
	if err != nil {
		return err
	}

	opts := gateway.Options{
		GatewayName:      cmd.String("gateway-name"),
		GatewayNamespace: cmd.String("gateway-namespace"),
		EnvoyService:     cmd.String("envoy-service"),
		NetworkPolicy:    cmd.String("network-policy"),
	}

	reconcileOnce := func(ctx context.Context) error {
		result, err := gateway.Reconcile(ctx, k8sClient.DynamicClient(), k8sClient.Clientset(), opts)
		if err != nil {
			return err
		}
		reportResult(result)
		return nil
	}

	if cmd.Bool("once") {
		return reconcileOnce(ctx)
	}

	// Long-running watch loop: reconcile on a fixed interval, exiting cleanly on
	// SIGINT/SIGTERM. Transient reconcile errors are logged but don't stop the
	// controller.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	interval := cmd.Duration("interval")
	fmt.Printf("Gateway controller watching routes for Gateway %s/%s (resync %s)\n",
		opts.GatewayNamespace, opts.GatewayName, interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	if err := reconcileOnce(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ reconcile failed: %v\n", err)
	}
	for {
		select {
		case <-ctx.Done():
			fmt.Println("Gateway controller stopped")
			return nil
		case <-ticker.C:
			if err := reconcileOnce(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "  ⚠ reconcile failed: %v\n", err)
			}
		}
	}
}

// newClusterClient builds a Kubernetes client, resolving its configuration in
// this order: an explicit --kubeconfig path, then the in-cluster service
// account (when running as a pod), then the Foundry-managed kubeconfig at
// ~/.foundry/kubeconfig (matching the other cluster commands).
func newClusterClient(kubeconfigOverride string) (*k8s.Client, error) {
	if kubeconfigOverride != "" {
		return clientFromFile(kubeconfigOverride)
	}

	// Running as a pod: use the mounted service account.
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		return k8s.NewClientInCluster()
	}

	configDir, err := config.GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config directory: %w", err)
	}
	return clientFromFile(filepath.Join(configDir, "kubeconfig"))
}

func clientFromFile(path string) (*k8s.Client, error) {
	kubeconfigBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read kubeconfig at %s: %w", path, err)
	}
	k8sClient, err := k8s.NewClientFromKubeconfig(kubeconfigBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}
	return k8sClient, nil
}

func reportResult(result *gateway.Result) {
	for _, conflict := range result.Conflicts {
		fmt.Printf("  ⚠ %s\n", conflict)
	}
	for _, skip := range result.Skipped {
		fmt.Printf("  ⚠ %s\n", skip)
	}
	if result.Changed() {
		fmt.Printf("  ✓ reconciled (gateway=%t service=%t networkpolicy=%t)\n",
			result.GatewayUpdated, result.ServiceUpdated, result.NetworkPolicyUpdated)
		for _, d := range result.Desired {
			fmt.Printf("    • %s listener on port %d → envoy %d\n", d.Protocol, d.Port, gateway.TargetPortFor(d.Port))
		}
	}
}
