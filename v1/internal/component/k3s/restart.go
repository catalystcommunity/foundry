package k3s

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const restartStateAnnotation = "foundry.catalystcommunity.org/k3s-restart-state"

const (
	k3sInvocationCommand = "sudo systemctl show k3s --property=InvocationID --value"
	flannelStampCommand  = "sudo stat -c '%y' /run/flannel/subnet.env"
	localRestartKubectl  = "sudo k3s kubectl --request-timeout=15s"
)

// Keep the recovery data on the DaemonSet, not only in the CLI process. If the
// process stops, another upgrade must not overwrite this unfinished operation.
type vipRestartState struct {
	Node     string                         `json:"node"`
	Affinity *corev1.Affinity               `json:"affinity"`
	Strategy appsv1.DaemonSetUpdateStrategy `json:"strategy"`
}

type vipRestartGuard struct {
	executor     SSHExecutor
	kubectl      string
	node         corev1.Node
	vip          string
	state        string
	before       appsv1.DaemonSet
	paused       appsv1.DaemonSet
	invocation   string
	flannelStamp string
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func restartExec(executor SSHExecutor, command string) (string, error) {
	result, err := executor.Exec(command)
	if err != nil {
		return "", err
	}
	if result == nil {
		return "", fmt.Errorf("remote command returned no result")
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("remote command failed (exit %d): %s", result.ExitCode, strings.TrimSpace(result.Stderr))
	}
	return strings.TrimSpace(result.Stdout), nil
}

// withVIPSafeRestart is shared by installer-driven upgrades and config restarts.
// Workers do not run kube-vip and do not use this guard.
func withVIPSafeRestart(ctx context.Context, executor SSHExecutor, cfg *Config, command string) (err error) {
	return runVIPSafeRestart(ctx, executor, cfg, command, DefaultRetryConfig())
}

// reconcileVIPNetwork also checks servers when their version and config match.
// A restart outside Foundry can leave a Ready node with the VIP as its tunnel IP.
func reconcileVIPNetwork(ctx context.Context, executor SSHExecutor, cfg *Config, retry RetryConfig) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := restartExec(executor, localRestartKubectl+" -n kube-system get daemonset kube-vip --ignore-not-found -o json")
	if err != nil || data == "" {
		return err
	}
	var ds appsv1.DaemonSet
	if err := json.Unmarshal([]byte(data), &ds); err != nil {
		return err
	}
	if ds.Annotations[restartStateAnnotation] != "" {
		return fmt.Errorf("kube-vip has an unfinished restart guard; inspect annotation %s before retrying", restartStateAnnotation)
	}
	node, _, err := localRestartNode(executor)
	if err != nil {
		return err
	}
	data, err = restartExec(executor, "ip -j -d link show type vxlan")
	if err != nil {
		return err
	}
	if err := validateFlannelNetwork(data, *node, cfg.VIP); err == nil {
		return nil
	} else {
		fmt.Printf("   Repairing Flannel on %s: %v\n", node.Name, err)
	}
	return runVIPSafeRestart(ctx, executor, cfg, "sudo systemctl restart k3s", retry)
}

func runVIPSafeRestart(ctx context.Context, executor SSHExecutor, cfg *Config, command string, retry RetryConfig) (err error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	guard, err := prepareVIPRestart(executor, cfg.VIP)
	if err != nil {
		return fmt.Errorf("prepare VIP-safe restart: %w", err)
	}
	restartAttempted, networkValidated := false, false
	if guard != nil {
		// Install cleanup before changing the DaemonSet: an SSH error can occur
		// after the API server has accepted the patch.
		defer func() {
			if restartAttempted && !networkValidated {
				// Do not let the VIP return while a failed or slow K3s start can
				// still select its interface. Keep the peer serving the VIP.
				err = errors.Join(err, fmt.Errorf("kube-vip remains excluded from %s; verify K3s and Flannel before restoring settings from annotation %s", guard.node.Name, restartStateAnnotation))
				return
			}
			err = errors.Join(err, guard.restore())
		}()
		if err = guard.pause(); err != nil {
			return err
		}
		fmt.Printf("   Waiting for kube-vip to leave %s...\n", guard.node.Name)
		if err = retryRestartCheck(ctx, retry, guard.handedOff); err != nil {
			return fmt.Errorf("VIP handoff failed; K3s was not restarted: %w", err)
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if guard != nil {
		if guard.invocation, err = restartExec(executor, k3sInvocationCommand); err != nil || guard.invocation == "" {
			return fmt.Errorf("cannot read the K3s service invocation before restart: %v", err)
		}
		if guard.flannelStamp, err = restartExec(executor, flannelStampCommand); err != nil || guard.flannelStamp == "" {
			return fmt.Errorf("cannot read Flannel startup state before restart: %v", err)
		}
	}
	restartAttempted = true
	if _, err = restartExec(executor, command); err != nil {
		return fmt.Errorf("K3s restart or upgrade failed: %w", err)
	}
	if err = waitForK3sReady(executor, retry); err != nil {
		return err
	}
	if guard != nil {
		if err = retryRestartCheck(ctx, retry, guard.networkReady); err != nil {
			return fmt.Errorf("post-restart Flannel validation failed on %s: %w", guard.node.Name, err)
		}
		networkValidated = true
	}
	return nil
}

func retryRestartCheck(ctx context.Context, retry RetryConfig, check func() error) error {
	var last error
	for i := 0; i < retry.MaxRetries; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if last = check(); last == nil {
			return nil
		}
		if i+1 < retry.MaxRetries {
			timer := time.NewTimer(retry.RetryDelay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return fmt.Errorf("check did not pass after %d attempts: %w", retry.MaxRetries, last)
}

func prepareVIPRestart(executor SSHExecutor, vip string) (*vipRestartGuard, error) {
	local := localRestartKubectl
	data, err := restartExec(executor, local+" -n kube-system get daemonset kube-vip --ignore-not-found -o json")
	if err != nil {
		return nil, err
	}
	if data == "" {
		return nil, nil // No kube-vip deployment on this cluster.
	}
	var ds appsv1.DaemonSet
	if err := json.Unmarshal([]byte(data), &ds); err != nil {
		return nil, err
	}
	if ds.Annotations[restartStateAnnotation] != "" {
		return nil, fmt.Errorf("kube-vip has an unfinished restart guard; inspect annotation %s before retrying", restartStateAnnotation)
	}
	if net.ParseIP(vip) == nil {
		return nil, fmt.Errorf("invalid cluster VIP %q", vip)
	}
	node, nodes, err := localRestartNode(executor)
	if err != nil {
		return nil, err
	}
	guard := &vipRestartGuard{executor: executor, kubectl: local, node: *node, vip: vip, before: ds}
	pods, err := guard.pods()
	if err != nil {
		return nil, err
	}
	// Use a healthy peer API for the guard so cleanup does not depend on the
	// API server being restarted, or on VIP ownership during the handoff.
	for _, pod := range pods {
		if pod.Spec.NodeName == node.Name || pod.DeletionTimestamp != nil || !podReady(pod) {
			continue
		}
		for _, peer := range nodes.Items {
			if peer.Name != pod.Spec.NodeName || !nodeReady(peer) {
				continue
			}
			for _, address := range peer.Status.Addresses {
				if address.Type != corev1.NodeInternalIP || net.ParseIP(address.Address) == nil || address.Address == vip {
					continue
				}
				candidate := local + " --server=" + shellQuote("https://"+net.JoinHostPort(address.Address, "6443"))
				if _, err := restartExec(executor, candidate+" get --raw=/readyz"); err == nil {
					guard.kubectl = candidate
					guard.paused = *ds.DeepCopy()
					guard.paused.Spec.Template.Spec.Affinity = excludeVIPNode(ds.Spec.Template.Spec.Affinity, node.Name)
					guard.paused.Spec.UpdateStrategy = appsv1.DaemonSetUpdateStrategy{Type: appsv1.OnDeleteDaemonSetStrategyType}
					state, err := json.Marshal(vipRestartState{Node: node.Name, Affinity: ds.Spec.Template.Spec.Affinity, Strategy: ds.Spec.UpdateStrategy})
					if err != nil {
						return nil, err
					}
					guard.state = string(state)
					return guard, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("no other Ready kube-vip node with a reachable API; refusing automatic restart")
}

func localRestartNode(executor SSHExecutor) (*corev1.Node, *corev1.NodeList, error) {
	machineID, err := restartExec(executor, "cat /etc/machine-id")
	if err != nil || machineID == "" {
		return nil, nil, fmt.Errorf("cannot identify the local node machine ID: %v", err)
	}
	data, err := restartExec(executor, localRestartKubectl+" get nodes -o json")
	if err != nil {
		return nil, nil, err
	}
	var nodes corev1.NodeList
	if err := json.Unmarshal([]byte(data), &nodes); err != nil {
		return nil, nil, err
	}
	var node *corev1.Node
	for i := range nodes.Items {
		if nodes.Items[i].Status.NodeInfo.MachineID == machineID {
			if node != nil {
				return nil, nil, fmt.Errorf("multiple nodes have local machine ID")
			}
			node = &nodes.Items[i]
		}
	}
	if node == nil {
		return nil, nil, fmt.Errorf("local machine ID does not match a Kubernetes node")
	}
	return node, &nodes, nil
}

func nodeReady(node corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func podReady(pod corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func excludeVIPNode(original *corev1.Affinity, node string) *corev1.Affinity {
	affinity := original.DeepCopy()
	if affinity == nil {
		affinity = &corev1.Affinity{}
	}
	if affinity.NodeAffinity == nil {
		affinity.NodeAffinity = &corev1.NodeAffinity{}
	}
	selector := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	newSelector := selector == nil
	if selector == nil {
		selector = &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{}}}
		affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = selector
	}
	// Terms are ORed. Add the exclusion to every term, not as a new term.
	for i := range selector.NodeSelectorTerms {
		if !newSelector && len(selector.NodeSelectorTerms[i].MatchFields) == 0 && len(selector.NodeSelectorTerms[i].MatchExpressions) == 0 {
			continue // An existing empty term matches no nodes; do not broaden it.
		}
		selector.NodeSelectorTerms[i].MatchFields = append(selector.NodeSelectorTerms[i].MatchFields,
			corev1.NodeSelectorRequirement{Key: "metadata.name", Operator: corev1.NodeSelectorOpNotIn, Values: []string{node}})
	}
	return affinity
}

type restartPatch struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func (g *vipRestartGuard) patch(ops []restartPatch) error {
	data, err := json.Marshal(ops)
	if err != nil {
		return err
	}
	_, err = restartExec(g.executor, g.kubectl+" -n kube-system patch daemonset kube-vip --type=json -p "+shellQuote(string(data)))
	return err
}

func (g *vipRestartGuard) pause() error {
	annotations := make(map[string]string, len(g.before.Annotations)+1)
	for key, value := range g.before.Annotations {
		annotations[key] = value
	}
	annotations[restartStateAnnotation] = g.state
	return g.patch([]restartPatch{
		{"test", "/metadata/resourceVersion", g.before.ResourceVersion},
		{"add", "/metadata/annotations", annotations},
		{"add", "/spec/updateStrategy", g.paused.Spec.UpdateStrategy},
		{"add", "/spec/template/spec/affinity", g.paused.Spec.Template.Spec.Affinity},
	})
}

func (g *vipRestartGuard) restore() error {
	data, err := restartExec(g.executor, g.kubectl+" -n kube-system get daemonset kube-vip -o json")
	if err != nil {
		return fmt.Errorf("restore kube-vip eligibility for %s: %w (recovery state is in %s)", g.node.Name, err, restartStateAnnotation)
	}
	var current appsv1.DaemonSet
	if err := json.Unmarshal([]byte(data), &current); err != nil {
		return err
	}
	if current.Annotations[restartStateAnnotation] == "" {
		return nil // The prepare patch was not accepted.
	}
	ops := []restartPatch{
		{"test", "/metadata/uid", g.before.UID},
		{"test", "/metadata/annotations/foundry.catalystcommunity.org~1k3s-restart-state", g.state},
		{"test", "/spec/updateStrategy", g.paused.Spec.UpdateStrategy},
		{"test", "/spec/template/spec/affinity", g.paused.Spec.Template.Spec.Affinity},
		{"add", "/spec/updateStrategy", g.before.Spec.UpdateStrategy},
		{"remove", "/metadata/annotations/foundry.catalystcommunity.org~1k3s-restart-state", nil},
	}
	if g.before.Spec.Template.Spec.Affinity == nil {
		ops = append(ops, restartPatch{"remove", "/spec/template/spec/affinity", nil})
	} else {
		ops = append(ops, restartPatch{"add", "/spec/template/spec/affinity", g.before.Spec.Template.Spec.Affinity})
	}
	if err := g.patch(ops); err != nil {
		return fmt.Errorf("restore kube-vip eligibility for %s: %w (recovery state is in %s)", g.node.Name, err, restartStateAnnotation)
	}
	return nil
}

func (g *vipRestartGuard) pods() ([]corev1.Pod, error) {
	data, err := restartExec(g.executor, g.kubectl+" -n kube-system get pods -l app.kubernetes.io/name=kube-vip -o json")
	if err != nil {
		return nil, err
	}
	var list corev1.PodList
	if err := json.Unmarshal([]byte(data), &list); err != nil {
		return nil, err
	}
	var pods []corev1.Pod
	for _, pod := range list.Items {
		for _, owner := range pod.OwnerReferences {
			if owner.UID == g.before.UID && owner.Controller != nil && *owner.Controller {
				pods = append(pods, pod)
			}
		}
	}
	return pods, nil
}

func (g *vipRestartGuard) handedOff() error {
	pods, err := g.pods()
	if err != nil {
		return err
	}
	peerReady := false
	for _, pod := range pods {
		if pod.Spec.NodeName == g.node.Name {
			return fmt.Errorf("kube-vip Pod %s is still on the restart node", pod.Name)
		}
		peerReady = peerReady || (pod.DeletionTimestamp == nil && podReady(pod))
	}
	if !peerReady {
		return fmt.Errorf("no Ready peer kube-vip Pod remains")
	}
	data, err := restartExec(g.executor, "ip -j address show")
	if err != nil {
		return err
	}
	var interfaces []struct {
		Addresses []struct {
			Local string `json:"local"`
		} `json:"addr_info"`
	}
	if err := json.Unmarshal([]byte(data), &interfaces); err != nil {
		return err
	}
	for _, iface := range interfaces {
		for _, addr := range iface.Addresses {
			if net.ParseIP(addr.Local).Equal(net.ParseIP(g.vip)) {
				return fmt.Errorf("VIP %s is still assigned to the restart node", g.vip)
			}
		}
	}
	_, err = restartExec(g.executor, "sudo k3s kubectl --request-timeout=15s --server="+shellQuote("https://"+net.JoinHostPort(g.vip, "6443"))+" get --raw=/readyz")
	return err
}

func (g *vipRestartGuard) networkReady() error {
	invocation, err := restartExec(g.executor, k3sInvocationCommand)
	if err != nil {
		return err
	}
	if invocation == "" {
		return fmt.Errorf("K3s service has no active invocation")
	}
	// The API can respond while Flannel is still starting. A tunnel from the
	// previous process is not proof that the new process selected the right IP.
	// K3s writes subnet.env after it selects the interface and registers VXLAN.
	// The installer can skip a restart when the version and settings match.
	if invocation != g.invocation {
		stamp, err := restartExec(g.executor, flannelStampCommand)
		if err != nil {
			return err
		}
		if stamp == "" || stamp == g.flannelStamp {
			return fmt.Errorf("Flannel has not written fresh startup state")
		}
	}
	data, err := restartExec(g.executor, "ip -j -d link show type vxlan")
	if err != nil {
		return err
	}
	nodeData, err := restartExec(g.executor, g.kubectl+" get node "+shellQuote(g.node.Name)+" -o json")
	if err != nil {
		return err
	}
	var node corev1.Node
	if err := json.Unmarshal([]byte(nodeData), &node); err != nil {
		return err
	}
	return validateFlannelNetwork(data, node, g.vip)
}

func validateFlannelNetwork(data string, node corev1.Node, vip string) error {
	var links []struct {
		Name string `json:"ifname"`
		Info struct {
			Data struct {
				Local string `json:"local"`
			} `json:"info_data"`
		} `json:"linkinfo"`
	}
	if err := json.Unmarshal([]byte(data), &links); err != nil {
		return err
	}
	for _, link := range links {
		if link.Name != "flannel.1" {
			continue
		}
		ip := net.ParseIP(link.Info.Data.Local)
		if ip == nil || ip.Equal(net.ParseIP(vip)) {
			return fmt.Errorf("Flannel has invalid tunnel address %q", link.Info.Data.Local)
		}
		advertised := node.Annotations["flannel.alpha.coreos.com/public-ip"]
		if !ip.Equal(net.ParseIP(advertised)) {
			return fmt.Errorf("Flannel advertises %q but its tunnel uses %s", advertised, ip)
		}
		for _, address := range node.Status.Addresses {
			if address.Type == corev1.NodeInternalIP && ip.Equal(net.ParseIP(address.Address)) {
				return nil
			}
		}
		return fmt.Errorf("Flannel tunnel address %s is not a node InternalIP", ip)
	}
	return fmt.Errorf("Flannel VXLAN interface is not ready")
}
