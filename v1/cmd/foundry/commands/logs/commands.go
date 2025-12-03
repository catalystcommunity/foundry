package logs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/urfave/cli/v3"
)

// Command is the top-level logs command
var Command = &cli.Command{
	Name:      "logs",
	Usage:     "View logs from pods or query Loki",
	ArgsUsage: "[pod-name]",
	Description: `View logs from Kubernetes pods or query Loki for historical logs.

This command provides quick access to pod logs. For more advanced log queries,
use Grafana's Explore feature.

Examples:
  foundry logs grafana-0                    # View logs from pod
  foundry logs grafana-0 -n monitoring      # Specify namespace
  foundry logs -l app=grafana               # View logs by label
  foundry logs grafana-0 -f                 # Follow/stream logs
  foundry logs grafana-0 --tail 100         # Last 100 lines
  foundry logs grafana-0 --previous         # Previous container logs`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "namespace",
			Aliases: []string{"n"},
			Usage:   "Namespace of the pod",
			Value:   "default",
		},
		&cli.StringFlag{
			Name:    "selector",
			Aliases: []string{"l"},
			Usage:   "Label selector to find pods (e.g., app=grafana)",
		},
		&cli.StringFlag{
			Name:    "container",
			Aliases: []string{"c"},
			Usage:   "Container name (if pod has multiple containers)",
		},
		&cli.BoolFlag{
			Name:    "follow",
			Aliases: []string{"f"},
			Usage:   "Follow log output",
		},
		&cli.IntFlag{
			Name:  "tail",
			Usage: "Number of lines to show from the end of logs",
			Value: 0,
		},
		&cli.BoolFlag{
			Name:    "previous",
			Aliases: []string{"p"},
			Usage:   "Show logs from previous container instance",
		},
		&cli.BoolFlag{
			Name:  "timestamps",
			Usage: "Include timestamps in log output",
		},
		&cli.StringFlag{
			Name:  "since",
			Usage: "Only return logs newer than duration (e.g., 1h, 30m)",
		},
		&cli.BoolFlag{
			Name:    "all-namespaces",
			Aliases: []string{"A"},
			Usage:   "Search for pods in all namespaces",
		},
	},
	Action: runLogs,
}

func runLogs(ctx context.Context, cmd *cli.Command) error {
	podName := cmd.Args().Get(0)
	selector := cmd.String("selector")

	if podName == "" && selector == "" {
		return fmt.Errorf("pod name or label selector required\n\nUsage: foundry logs <pod-name>\n       foundry logs -l <label-selector>")
	}

	client, err := getK8sClient()
	if err != nil {
		return err
	}

	namespace := cmd.String("namespace")
	if cmd.Bool("all-namespaces") {
		namespace = metav1.NamespaceAll
	}

	// Find pod(s) to get logs from
	var pods []corev1.Pod

	if selector != "" {
		// Find pods by label selector
		podList, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: selector,
		})
		if err != nil {
			return fmt.Errorf("failed to list pods: %w", err)
		}
		if len(podList.Items) == 0 {
			return fmt.Errorf("no pods found matching selector %q in namespace %q", selector, namespace)
		}
		pods = podList.Items
	} else {
		// Find specific pod
		pod, err := findPod(ctx, client, namespace, podName)
		if err != nil {
			return err
		}
		pods = []corev1.Pod{*pod}
	}

	// Build log options
	logOpts := &corev1.PodLogOptions{
		Follow:     cmd.Bool("follow"),
		Previous:   cmd.Bool("previous"),
		Timestamps: cmd.Bool("timestamps"),
	}

	if container := cmd.String("container"); container != "" {
		logOpts.Container = container
	}

	if tail := cmd.Int("tail"); tail > 0 {
		tailLines := int64(tail)
		logOpts.TailLines = &tailLines
	}

	if since := cmd.String("since"); since != "" {
		duration, err := parseDuration(since)
		if err != nil {
			return fmt.Errorf("invalid --since value: %w", err)
		}
		sinceSeconds := int64(duration.Seconds())
		logOpts.SinceSeconds = &sinceSeconds
	}

	// Stream logs from all matching pods
	for _, pod := range pods {
		if len(pods) > 1 {
			fmt.Printf("==> %s/%s <==\n", pod.Namespace, pod.Name)
		}

		req := client.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, logOpts)
		stream, err := req.Stream(ctx)
		if err != nil {
			if len(pods) > 1 {
				fmt.Printf("Error getting logs: %v\n\n", err)
				continue
			}
			return fmt.Errorf("failed to stream logs: %w", err)
		}
		defer stream.Close()

		_, err = io.Copy(os.Stdout, stream)
		if err != nil && err != io.EOF {
			if len(pods) > 1 {
				fmt.Printf("Error streaming logs: %v\n\n", err)
				continue
			}
			return fmt.Errorf("error streaming logs: %w", err)
		}

		if len(pods) > 1 {
			fmt.Println()
		}
	}

	return nil
}

// findPod finds a pod by name, with fuzzy matching support
func findPod(ctx context.Context, client *kubernetes.Clientset, namespace, name string) (*corev1.Pod, error) {
	// First try exact match
	pod, err := client.CoreV1().Pods(namespace).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return pod, nil
	}

	// If not found, try prefix match (useful for pods with generated names)
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	var matches []corev1.Pod
	for _, p := range pods.Items {
		if strings.HasPrefix(p.Name, name) {
			matches = append(matches, p)
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("pod %q not found in namespace %q", name, namespace)
	}

	if len(matches) == 1 {
		return &matches[0], nil
	}

	// Multiple matches - show options
	fmt.Printf("Multiple pods match %q:\n", name)
	for _, p := range matches {
		fmt.Printf("  %s\n", p.Name)
	}
	return nil, fmt.Errorf("please specify a more specific pod name")
}

// parseDuration parses a duration string like "1h" or "30m"
func parseDuration(s string) (duration, error) {
	// Use time.ParseDuration for simple parsing
	return parseSimpleDuration(s)
}

type duration struct {
	seconds int64
}

func (d duration) Seconds() float64 {
	return float64(d.seconds)
}

func parseSimpleDuration(s string) (duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return duration{}, fmt.Errorf("empty duration")
	}

	// Simple parser for common formats
	var value int64
	var unit string

	_, err := fmt.Sscanf(s, "%d%s", &value, &unit)
	if err != nil {
		return duration{}, fmt.Errorf("invalid duration format: %s", s)
	}

	var multiplier int64
	switch unit {
	case "s", "sec", "second", "seconds":
		multiplier = 1
	case "m", "min", "minute", "minutes":
		multiplier = 60
	case "h", "hr", "hour", "hours":
		multiplier = 3600
	case "d", "day", "days":
		multiplier = 86400
	default:
		return duration{}, fmt.Errorf("unknown duration unit: %s", unit)
	}

	return duration{seconds: value * multiplier}, nil
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
