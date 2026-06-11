package backup

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/urfave/cli/v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/ssh"
)

var bslGVR = schema.GroupVersionResource{Group: "velero.io", Version: "v1", Resource: "backupstoragelocations"}
var backupRepoGVR = schema.GroupVersionResource{Group: "velero.io", Version: "v1", Resource: "backuprepositories"}

const (
	localBSLName      = "foundry-local"
	localBSLSecret    = "foundry-local-cloud"
	localBSLSecretKey = "cloud"
)

// LocalCommand streams a File System Backup (PV contents included) to the local
// machine — with no lasting in-cluster copy — by running a SeaweedFS S3 endpoint
// locally and exposing it to the cluster through a reverse SSH tunnel to one node.
var LocalCommand = &cli.Command{
	Name:      "local",
	Usage:     "Back up the cluster (incl. PersistentVolume data) to this machine",
	ArgsUsage: "[name]",
	Description: `Creates a Velero File System Backup that streams directly to this machine,
storing it under ~/.foundry/<stack>_backups — nothing lasting is left in the cluster.

How it works (all via foundry, fully torn down afterward):
  1. Runs a local SeaweedFS S3 endpoint on 127.0.0.1.
  2. Enables sshd GatewayPorts on ONE cluster node (reversible drop-in + dead-man
     auto-revert timer) so a reverse SSH tunnel can bind the node's routable IP.
  3. Opens a reverse tunnel node:port -> local S3 and points a temporary Velero
     BackupStorageLocation at it.
  4. Runs the backup with File System Backup (kopia) for volume contents.
  5. Tears everything down: deletes the temp BSL, closes the tunnel, reverts sshd,
     stops the local S3. The dead-man timer reverts sshd even if this process dies.

Requires the velero node-agent (set deploy_node_agent: true on the velero component
and run 'foundry component install velero').

Examples:
  foundry backup local
  foundry backup local my-full-backup --exclude-namespace kube-system`,
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "node", Usage: "Hostname of the cluster node to tunnel through (default: first control-plane)"},
		&cli.IntFlag{Name: "port", Usage: "Port for the local S3 endpoint and node-side tunnel bind", Value: 33099},
		&cli.StringSliceFlag{Name: "namespace", Aliases: []string{"n"}, Usage: "Only back up these namespaces (default: all). Useful for a quick validation run"},
		&cli.StringSliceFlag{Name: "exclude-namespace", Usage: "Namespaces to exclude", Value: []string{"kube-system"}},
		&cli.StringFlag{Name: "ttl", Usage: "Backup retention period", Value: "720h"},
		&cli.DurationFlag{Name: "timeout", Usage: "Max time to wait for the backup", Value: 2 * time.Hour},
		&cli.StringFlag{Name: "weed-binary", Usage: "Path to a 'weed' (SeaweedFS) binary (otherwise auto-detected/downloaded)"},
		&cli.BoolFlag{Name: "keep", Usage: "Leave the tunnel/S3/BSL up after the backup (debug; sshd still auto-reverts)"},
	},
	Action: runLocalBackup,
}

func runLocalBackup(ctx context.Context, cmd *cli.Command) error {
	name := cmd.Args().Get(0)
	if name == "" {
		name = fmt.Sprintf("local-%s", time.Now().Format("20060102-150405"))
	}

	// On Ctrl-C/SIGTERM, cancel the context so the waits return and the deferred
	// teardown (BSL -> tunnel -> revert sshd -> stop S3) runs cleanly.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Load stack config.
	configPath, err := config.FindConfig(cmd.String("config"))
	if err != nil {
		return err
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	configDir, err := config.GetConfigDir()
	if err != nil {
		return err
	}
	kubeconfigPath := filepath.Join(configDir, "kubeconfig")

	// Kubernetes clients.
	restCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to build kube config: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return err
	}
	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return err
	}

	// Precondition: node-agent must be deployed for File System Backup.
	if _, err := clientset.AppsV1().DaemonSets(VeleroNamespace).Get(ctx, "node-agent", metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("velero node-agent is not deployed (required for volume data backup)\n\n" +
				"Enable it: set `deploy_node_agent: true` under the velero component in your stack config,\n" +
				"then run: foundry component install velero")
		}
		return fmt.Errorf("failed to check node-agent: %w", err)
	}

	// Select the node to tunnel through.
	node, err := selectTunnelNode(cfg, cmd.String("node"))
	if err != nil {
		return err
	}
	port := cmd.Int("port")
	bindAddr := fmt.Sprintf("%s:%d", node.Address, port)
	localAddr := fmt.Sprintf("127.0.0.1:%d", port)
	fmt.Printf("Tunnel node: %s (%s); local target: ~/.foundry/%s_backups\n", node.Hostname, node.Address, cfg.Cluster.Name)

	// SSH to the node using foundry's stored key.
	conn, err := connectToHost(configDir, cfg.Cluster.Name, node)
	if err != nil {
		return err
	}
	defer conn.Close()

	// 1) Local SeaweedFS S3 target.
	weedBin, err := ensureWeedBinary(cmd.String("weed-binary"), configDir)
	if err != nil {
		return err
	}
	dataDir := filepath.Join(configDir, cfg.Cluster.Name+"_backups")
	s3, err := newLocalS3(weedBin, dataDir, "velero", port)
	if err != nil {
		return err
	}
	fmt.Println("Starting local SeaweedFS S3...")
	if err := s3.Start(ctx); err != nil {
		return err
	}
	keep := cmd.Bool("keep")
	if !keep {
		defer s3.Stop()
	}

	// 2) Enable GatewayPorts on the node (auto-reverts after timeout + buffer).
	gp := newGatewayPortsManager(conn)
	revertAfter := cmd.Duration("timeout") + 30*time.Minute
	fmt.Printf("Enabling sshd GatewayPorts on %s (auto-reverts in %s if anything goes wrong)...\n", node.Hostname, revertAfter)
	gpVal, err := gp.Enable(revertAfter)
	if err != nil {
		return fmt.Errorf("failed to enable GatewayPorts: %w", err)
	}
	fmt.Printf("  node sshd: %s\n", gpVal)
	if !keep {
		defer func() {
			fmt.Println("Reverting sshd GatewayPorts...")
			if out, derr := gp.Disable(); derr != nil {
				fmt.Printf("  ⚠ revert failed (dead-man timer will still revert it): %v\n", derr)
			} else {
				fmt.Printf("  node sshd: %s\n", out)
			}
		}()
	}

	// 3) Reverse tunnel node:port -> local S3.
	// NOTE: sshd evaluates GatewayPorts per-connection at connect time, so the tunnel
	// must run over a connection opened AFTER GatewayPorts was enabled — the original
	// control connection's sshd child still has the old setting.
	tunnelConn, err := connectToHost(configDir, cfg.Cluster.Name, node)
	if err != nil {
		return fmt.Errorf("failed to open tunnel connection: %w", err)
	}
	defer tunnelConn.Close()
	fmt.Printf("Opening reverse tunnel %s -> %s...\n", bindAddr, localAddr)
	tunnel, err := startReverseTunnel(tunnelConn.Client(), bindAddr, localAddr)
	if err != nil {
		return err
	}
	if !keep {
		defer tunnel.Close()
	}
	// Confirm the tunnel bound the node's routable IP (not loopback).
	if res, derr := conn.Exec(fmt.Sprintf("ss -ltn 2>/dev/null | grep ':%d ' || true", port)); derr == nil {
		fmt.Printf("  node listener: %s\n", strings.TrimSpace(res.Stdout))
	}

	// 4) Temporary BackupStorageLocation pointing at the tunnel.
	// Clear any stale BackupRepository CRs from a prior run first: they reference a
	// kopia repo in a previous local store and would make the node-agent try to
	// CONNECT to a repo that no longer exists ("repository not initialized") instead
	// of initializing a fresh one. Defensive here in case a prior teardown was killed.
	cleanupLocalRepos(ctx, dynClient)
	if err := createLocalBSL(ctx, clientset, dynClient, node.Address, port, s3.bucket, s3.accessKey, s3.secretKey); err != nil {
		return err
	}
	if !keep {
		defer deleteLocalBSL(context.Background(), clientset, dynClient)
	}
	fmt.Println("Waiting for Velero to validate the off-cluster backup location...")
	if err := waitForBSLAvailable(ctx, dynClient, 150*time.Second); err != nil {
		return fmt.Errorf("backup location did not become Available (tunnel/S3 issue): %w\n  tunnel err: %v", err, tunnel.Err())
	}
	fmt.Println("  ✓ Backup location Available")

	// 5) Run the File System Backup.
	vc, err := NewVeleroClient()
	if err != nil {
		return err
	}
	fsTrue := true
	opts := BackupOptions{
		IncludedNamespaces:       cmd.StringSlice("namespace"),
		ExcludedNamespaces:       cmd.StringSlice("exclude-namespace"),
		TTL:                      cmd.String("ttl"),
		DefaultVolumesToFsBackup: &fsTrue,
		StorageLocation:          localBSLName,
	}
	fmt.Printf("Creating File System Backup %q (streaming PV data to this machine)...\n", name)
	if err := vc.CreateBackup(ctx, name, opts); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}
	if err := waitForBackup(ctx, vc, name, cmd.Duration("timeout")); err != nil {
		if terr := tunnel.Err(); terr != nil {
			return fmt.Errorf("%w\n  tunnel error: %v", err, terr)
		}
		return err
	}

	fmt.Printf("\n✓ Backup %q stored locally at %s\n", name, dataDir)
	if keep {
		fmt.Println("⚠ --keep set: tunnel, local S3 and temp BSL left running; sshd will auto-revert via the dead-man timer.")
	}
	return nil
}

// selectTunnelNode picks the node to tunnel through: an explicit --node, else the
// first control-plane host, else the first worker.
func selectTunnelNode(cfg *config.Config, want string) (*hostRef, error) {
	var cp, wk *hostRef
	for _, h := range cfg.Hosts {
		ref := &hostRef{Hostname: h.Hostname, Address: h.Address, Port: h.Port, User: h.User}
		if want != "" && h.Hostname == want {
			return ref, nil
		}
		for _, r := range h.Roles {
			if r == "cluster-control-plane" && cp == nil {
				cp = ref
			}
			if r == "cluster-worker" && wk == nil {
				wk = ref
			}
		}
	}
	if want != "" {
		return nil, fmt.Errorf("node %q not found in stack config", want)
	}
	if cp != nil {
		return cp, nil
	}
	if wk != nil {
		return wk, nil
	}
	return nil, fmt.Errorf("no cluster node found in stack config to tunnel through")
}

type hostRef struct {
	Hostname string
	Address  string
	Port     int
	User     string
}

func connectToHost(configDir, clusterName string, node *hostRef) (*ssh.Connection, error) {
	keyStorage, err := ssh.GetKeyStorage(configDir, clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to open key storage: %w", err)
	}
	keyPair, err := keyStorage.Load(node.Hostname)
	if err != nil {
		return nil, fmt.Errorf("SSH key not found for %s: %w", node.Hostname, err)
	}
	authMethod, err := keyPair.AuthMethod()
	if err != nil {
		return nil, err
	}
	port := node.Port
	if port == 0 {
		port = 22
	}
	conn, err := ssh.Connect(&ssh.ConnectionOptions{
		Host:       node.Address,
		Port:       port,
		User:       node.User,
		AuthMethod: authMethod,
		Timeout:    30,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to SSH to %s (%s): %w", node.Hostname, node.Address, err)
	}
	return conn, nil
}

// createLocalBSL creates the credentials Secret and a temporary BackupStorageLocation
// pointing at the tunnel endpoint on the node.
func createLocalBSL(ctx context.Context, clientset kubernetes.Interface, dynClient dynamic.Interface, nodeIP string, port int, bucket, accessKey, secretKey string) error {
	cloud := fmt.Sprintf("[default]\naws_access_key_id=%s\naws_secret_access_key=%s\n", accessKey, secretKey)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: localBSLSecret, Namespace: VeleroNamespace},
		StringData: map[string]string{localBSLSecretKey: cloud},
	}
	if _, err := clientset.CoreV1().Secrets(VeleroNamespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		if apierrors.IsAlreadyExists(err) {
			_, _ = clientset.CoreV1().Secrets(VeleroNamespace).Update(ctx, secret, metav1.UpdateOptions{})
		} else {
			return fmt.Errorf("failed to create credentials secret: %w", err)
		}
	}

	bsl := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "velero.io/v1",
		"kind":       "BackupStorageLocation",
		"metadata":   map[string]interface{}{"name": localBSLName, "namespace": VeleroNamespace},
		"spec": map[string]interface{}{
			"provider":   "aws",
			"accessMode": "ReadWrite",
			"objectStorage": map[string]interface{}{
				"bucket": bucket,
			},
			"credential": map[string]interface{}{
				"name": localBSLSecret,
				"key":  localBSLSecretKey,
			},
			"config": map[string]interface{}{
				"region":                "us-east-1",
				"s3ForcePathStyle":      "true",
				"s3Url":                 fmt.Sprintf("http://%s:%d", nodeIP, port),
				"insecureSkipTLSVerify": "true",
			},
		},
	}}
	if _, err := dynClient.Resource(bslGVR).Namespace(VeleroNamespace).Create(ctx, bsl, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create BackupStorageLocation: %w", err)
		}
	}
	return nil
}

func waitForBSLAvailable(ctx context.Context, dynClient dynamic.Interface, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	lastPhase, lastMsg := "", ""
	for time.Now().Before(deadline) {
		bsl, err := dynClient.Resource(bslGVR).Namespace(VeleroNamespace).Get(ctx, localBSLName, metav1.GetOptions{})
		if err == nil {
			phase, _, _ := unstructured.NestedString(bsl.Object, "status", "phase")
			msg, _, _ := unstructured.NestedString(bsl.Object, "status", "message")
			if phase != lastPhase || msg != lastMsg {
				fmt.Printf("  BSL phase=%q %s\n", phase, msg)
				lastPhase, lastMsg = phase, msg
			}
			if phase == "Available" {
				return nil
			}
		}
		time.Sleep(3 * time.Second)
	}
	if lastMsg != "" {
		return fmt.Errorf("timed out waiting for %s to be Available (last: %s)", localBSLName, lastMsg)
	}
	return fmt.Errorf("timed out waiting for %s to be Available", localBSLName)
}

func deleteLocalBSL(ctx context.Context, clientset kubernetes.Interface, dynClient dynamic.Interface) {
	fmt.Println("Removing temporary backup location...")
	if err := dynClient.Resource(bslGVR).Namespace(VeleroNamespace).Delete(ctx, localBSLName, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		fmt.Printf("  ⚠ failed to delete BSL: %v\n", err)
	}
	if err := clientset.CoreV1().Secrets(VeleroNamespace).Delete(ctx, localBSLSecret, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		fmt.Printf("  ⚠ failed to delete credentials secret: %v\n", err)
	}
	cleanupLocalRepos(ctx, dynClient)
}

// cleanupLocalRepos deletes BackupRepository CRs bound to the transient
// foundry-local BSL, so a subsequent run against a fresh local store re-initializes
// the kopia repository instead of failing to connect to a deleted one.
func cleanupLocalRepos(ctx context.Context, dynClient dynamic.Interface) {
	list, err := dynClient.Resource(backupRepoGVR).Namespace(VeleroNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return // CRD may be absent on some clusters; nothing to clean
	}
	for _, item := range list.Items {
		bsl, _, _ := unstructured.NestedString(item.Object, "spec", "backupStorageLocation")
		if bsl != localBSLName {
			continue
		}
		if err := dynClient.Resource(backupRepoGVR).Namespace(VeleroNamespace).Delete(ctx, item.GetName(), metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
			fmt.Printf("  ⚠ failed to delete stale BackupRepository %s: %v\n", item.GetName(), err)
		}
	}
}
