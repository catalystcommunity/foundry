package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/urfave/cli/v3"
)

// Command is the top-level metrics command
var Command = &cli.Command{
	Name:      "metrics",
	Usage:     "Query Prometheus metrics",
	ArgsUsage: "[query]",
	Description: `Query Prometheus for metrics data.

This command provides quick access to Prometheus metrics. For more advanced
queries and visualization, use Grafana.

Examples:
  foundry metrics "up"                           # Check if targets are up
  foundry metrics "node_memory_Active_bytes"     # Node memory usage
  foundry metrics "rate(http_requests_total[5m])"  # Request rate
  foundry metrics list                           # List available metrics`,
	Commands: []*cli.Command{
		QueryCommand,
		ListCommand,
		TargetsCommand,
	},
	Action: runQuery, // Default action
}

// QueryCommand queries Prometheus
var QueryCommand = &cli.Command{
	Name:      "query",
	Usage:     "Execute a PromQL query",
	ArgsUsage: "<query>",
	Description: `Execute a PromQL query against Prometheus.

Examples:
  foundry metrics query "up"
  foundry metrics query "node_memory_Active_bytes"
  foundry metrics query "rate(http_requests_total[5m])"`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "namespace",
			Aliases: []string{"n"},
			Usage:   "Namespace where Prometheus is installed",
			Value:   "monitoring",
		},
		&cli.StringFlag{
			Name:  "time",
			Usage: "Evaluation time for instant query (RFC3339 or Unix timestamp)",
		},
		&cli.BoolFlag{
			Name:  "range",
			Usage: "Execute a range query instead of instant query",
		},
		&cli.StringFlag{
			Name:  "start",
			Usage: "Start time for range query (RFC3339 or Unix timestamp)",
		},
		&cli.StringFlag{
			Name:  "end",
			Usage: "End time for range query (RFC3339 or Unix timestamp)",
		},
		&cli.StringFlag{
			Name:  "step",
			Usage: "Query step for range query (e.g., 15s, 1m, 5m)",
			Value: "1m",
		},
		&cli.BoolFlag{
			Name:  "json",
			Usage: "Output raw JSON response",
		},
	},
	Action: runQuery,
}

// ListCommand lists available metrics
var ListCommand = &cli.Command{
	Name:  "list",
	Usage: "List available metrics",
	Description: `List all available metrics in Prometheus.

Examples:
  foundry metrics list
  foundry metrics list --filter node_`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "namespace",
			Aliases: []string{"n"},
			Usage:   "Namespace where Prometheus is installed",
			Value:   "monitoring",
		},
		&cli.StringFlag{
			Name:  "filter",
			Usage: "Filter metrics by prefix",
		},
	},
	Action: runList,
}

// TargetsCommand lists Prometheus scrape targets
var TargetsCommand = &cli.Command{
	Name:  "targets",
	Usage: "List Prometheus scrape targets",
	Description: `List all configured scrape targets and their status.

Examples:
  foundry metrics targets
  foundry metrics targets --unhealthy`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "namespace",
			Aliases: []string{"n"},
			Usage:   "Namespace where Prometheus is installed",
			Value:   "monitoring",
		},
		&cli.BoolFlag{
			Name:  "unhealthy",
			Usage: "Only show unhealthy targets",
		},
	},
	Action: runTargets,
}

func runQuery(ctx context.Context, cmd *cli.Command) error {
	query := cmd.Args().Get(0)
	if query == "" {
		return fmt.Errorf("query is required\n\nUsage: foundry metrics <query>\n\nExamples:\n  foundry metrics \"up\"\n  foundry metrics \"node_memory_Active_bytes\"")
	}

	promURL, err := getPrometheusURL(ctx, cmd.String("namespace"))
	if err != nil {
		return err
	}

	// Build query URL
	endpoint := "/api/v1/query"
	params := url.Values{}
	params.Set("query", query)

	if cmd.Bool("range") {
		endpoint = "/api/v1/query_range"
		start := cmd.String("start")
		end := cmd.String("end")
		step := cmd.String("step")

		if start == "" {
			// Default to 1 hour ago
			start = time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
		}
		if end == "" {
			end = time.Now().Format(time.RFC3339)
		}

		params.Set("start", start)
		params.Set("end", end)
		params.Set("step", step)
	} else if evalTime := cmd.String("time"); evalTime != "" {
		params.Set("time", evalTime)
	}

	queryURL := fmt.Sprintf("%s%s?%s", promURL, endpoint, params.Encode())

	// Execute query
	resp, err := http.Get(queryURL)
	if err != nil {
		return fmt.Errorf("failed to query Prometheus: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if cmd.Bool("json") {
		fmt.Println(string(body))
		return nil
	}

	// Parse and format response
	var result prometheusResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Status != "success" {
		return fmt.Errorf("query failed: %s", result.Error)
	}

	return formatQueryResult(result.Data)
}

func runList(ctx context.Context, cmd *cli.Command) error {
	promURL, err := getPrometheusURL(ctx, cmd.String("namespace"))
	if err != nil {
		return err
	}

	// Get list of metrics
	resp, err := http.Get(fmt.Sprintf("%s/api/v1/label/__name__/values", promURL))
	if err != nil {
		return fmt.Errorf("failed to query Prometheus: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Status string   `json:"status"`
		Data   []string `json:"data"`
		Error  string   `json:"error,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Status != "success" {
		return fmt.Errorf("query failed: %s", result.Error)
	}

	filter := cmd.String("filter")
	count := 0
	for _, metric := range result.Data {
		if filter == "" || strings.HasPrefix(metric, filter) {
			fmt.Println(metric)
			count++
		}
	}

	fmt.Printf("\n%d metrics found\n", count)
	return nil
}

func runTargets(ctx context.Context, cmd *cli.Command) error {
	promURL, err := getPrometheusURL(ctx, cmd.String("namespace"))
	if err != nil {
		return err
	}

	// Get targets
	resp, err := http.Get(fmt.Sprintf("%s/api/v1/targets", promURL))
	if err != nil {
		return fmt.Errorf("failed to query Prometheus: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Status string `json:"status"`
		Data   struct {
			ActiveTargets []struct {
				Labels       map[string]string `json:"labels"`
				ScrapeURL    string            `json:"scrapeUrl"`
				Health       string            `json:"health"`
				LastError    string            `json:"lastError"`
				LastScrape   string            `json:"lastScrape"`
				ScrapePool   string            `json:"scrapePool"`
			} `json:"activeTargets"`
		} `json:"data"`
		Error string `json:"error,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Status != "success" {
		return fmt.Errorf("query failed: %s", result.Error)
	}

	unhealthyOnly := cmd.Bool("unhealthy")

	fmt.Printf("%-50s %-10s %-40s\n", "TARGET", "HEALTH", "JOB")
	fmt.Println(strings.Repeat("-", 100))

	healthy := 0
	unhealthy := 0

	for _, target := range result.Data.ActiveTargets {
		if unhealthyOnly && target.Health == "up" {
			healthy++
			continue
		}

		if target.Health == "up" {
			healthy++
		} else {
			unhealthy++
		}

		health := target.Health
		if health == "up" {
			health = "UP"
		} else {
			health = "DOWN"
		}

		job := target.Labels["job"]
		instance := target.Labels["instance"]
		if len(instance) > 50 {
			instance = instance[:47] + "..."
		}

		fmt.Printf("%-50s %-10s %-40s\n", instance, health, job)

		if target.LastError != "" && !unhealthyOnly {
			fmt.Printf("  Error: %s\n", target.LastError)
		}
	}

	fmt.Printf("\nTotal: %d healthy, %d unhealthy\n", healthy, unhealthy)
	return nil
}

// getPrometheusURL retrieves the Prometheus URL from the cluster
func getPrometheusURL(ctx context.Context, namespace string) (string, error) {
	client, err := getK8sClient()
	if err != nil {
		return "", err
	}

	// Try to find Prometheus service
	services, err := client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list services: %w", err)
	}

	// Look for Prometheus service (various naming conventions)
	prometheusNames := []string{
		"prometheus-server",
		"prometheus",
		"prometheus-kube-prometheus-prometheus",
		"kube-prometheus-stack-prometheus",
	}

	for _, svc := range services.Items {
		for _, name := range prometheusNames {
			if svc.Name == name || strings.Contains(svc.Name, "prometheus") {
				// Found Prometheus
				port := int32(9090)
				for _, p := range svc.Spec.Ports {
					if p.Name == "http" || p.Name == "web" || p.Port == 9090 {
						port = p.Port
						break
					}
				}
				return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", svc.Name, svc.Namespace, port), nil
			}
		}
	}

	return "", fmt.Errorf("Prometheus not found in namespace %q\n\nHint: Install Prometheus with: foundry component install prometheus", namespace)
}

// prometheusResponse represents a Prometheus API response
type prometheusResponse struct {
	Status string         `json:"status"`
	Data   queryData      `json:"data"`
	Error  string         `json:"error,omitempty"`
}

type queryData struct {
	ResultType string        `json:"resultType"`
	Result     []queryResult `json:"result"`
}

type queryResult struct {
	Metric map[string]string `json:"metric"`
	Value  []interface{}     `json:"value,omitempty"`  // for instant queries
	Values [][]interface{}   `json:"values,omitempty"` // for range queries
}

func formatQueryResult(data queryData) error {
	if len(data.Result) == 0 {
		fmt.Println("No results found")
		return nil
	}

	for _, result := range data.Result {
		// Format metric labels
		labels := formatLabels(result.Metric)

		if len(result.Value) == 2 {
			// Instant query result
			fmt.Printf("%s => %v\n", labels, result.Value[1])
		} else if len(result.Values) > 0 {
			// Range query result
			fmt.Printf("%s:\n", labels)
			for _, v := range result.Values {
				if len(v) == 2 {
					timestamp := v[0].(float64)
					t := time.Unix(int64(timestamp), 0)
					fmt.Printf("  %s => %v\n", t.Format("15:04:05"), v[1])
				}
			}
		}
	}

	return nil
}

func formatLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return "{}"
	}

	// Get metric name first
	name := labels["__name__"]
	delete(labels, "__name__")

	if len(labels) == 0 {
		return name
	}

	parts := make([]string, 0, len(labels))
	for k, v := range labels {
		parts = append(parts, fmt.Sprintf("%s=%q", k, v))
	}

	if name != "" {
		return fmt.Sprintf("%s{%s}", name, strings.Join(parts, ", "))
	}
	return fmt.Sprintf("{%s}", strings.Join(parts, ", "))
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
