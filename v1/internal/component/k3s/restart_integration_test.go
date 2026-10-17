package k3s

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Only the transport changes: the guard sends its real JSON patches to a real
// Kubernetes API, and the real DaemonSet controller performs the handoff.
type restartLocalExecutor struct{}

func (restartLocalExecutor) Exec(command string) (*ssh.ExecResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	result := &ssh.ExecResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if exit, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exit.ExitCode()
		return result, nil
	}
	return result, err
}

// TestVIPRestartDaemonSetHandoff requires a disposable two-node Kind cluster.
// It does not run K3s or bind a real VIP; host networking checks have unit tests.
func TestVIPRestartDaemonSetHandoff(t *testing.T) {
	path := os.Getenv("FOUNDRY_RESTART_TEST_KUBECONFIG")
	if testing.Short() || path == "" {
		t.Skip("set FOUNDRY_RESTART_TEST_KUBECONFIG for the disposable cluster test")
	}
	loaded, err := clientcmd.LoadFromFile(path)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(loaded.CurrentContext, "kind-foundry-restart-review-"), "use a disposable restart test cluster")
	conf, err := clientcmd.BuildConfigFromFlags("", path)
	require.NoError(t, err)
	client, err := kubernetes.NewForConfig(conf)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	nodes, err := client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(nodes.Items), 2)
	dsAPI := client.AppsV1().DaemonSets("kube-system")
	grace := int64(0)
	ds, err := dsAPI.Create(ctx, &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "kube-vip", Namespace: "kube-system", Annotations: map[string]string{"keep": "original"}},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/name": "kube-vip"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app.kubernetes.io/name": "kube-vip"}},
				Spec: corev1.PodSpec{
					Containers:                    []corev1.Container{{Name: "stand-in", Image: "busybox:latest", ImagePullPolicy: corev1.PullIfNotPresent, Command: []string{"sleep", "3600"}}},
					Tolerations:                   []corev1.Toleration{{Operator: corev1.TolerationOpExists}},
					TerminationGracePeriodSeconds: &grace,
				},
			},
		},
	}, metav1.CreateOptions{})
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 20*time.Second)
		defer stop()
		require.NoError(t, dsAPI.Delete(cleanup, ds.Name, metav1.DeleteOptions{}))
	})
	listPods := func() []corev1.Pod {
		pods, err := client.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=kube-vip"})
		require.NoError(t, err)
		return pods.Items
	}
	require.Eventually(t, func() bool {
		pods := listPods()
		return len(pods) == len(nodes.Items) && allRestartTestPodsReady(pods)
	}, time.Minute, time.Second)
	beforePods := listPods()
	target := beforePods[0].Spec.NodeName
	ds, err = dsAPI.Get(ctx, ds.Name, metav1.GetOptions{})
	require.NoError(t, err)
	guard := &vipRestartGuard{executor: restartLocalExecutor{}, kubectl: "kubectl --kubeconfig=" + shellQuote(path) + " --request-timeout=15s", node: corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: target}}, before: *ds, paused: *ds.DeepCopy()}
	guard.paused.Spec.Template.Spec.Affinity = excludeVIPNode(ds.Spec.Template.Spec.Affinity, target)
	guard.paused.Spec.UpdateStrategy = appsv1.DaemonSetUpdateStrategy{Type: appsv1.OnDeleteDaemonSetStrategyType}
	state, err := json.Marshal(vipRestartState{Node: target, Affinity: ds.Spec.Template.Spec.Affinity, Strategy: ds.Spec.UpdateStrategy})
	require.NoError(t, err)
	guard.state = string(state)
	require.NoError(t, guard.pause())
	require.Eventually(t, func() bool {
		pods := listPods()
		if len(pods) != len(nodes.Items)-1 {
			return false
		}
		for _, pod := range pods {
			if pod.Spec.NodeName == target {
				return false
			}
		}
		return allRestartTestPodsReady(pods)
	}, time.Minute, time.Second)
	for _, pod := range listPods() {
		for _, original := range beforePods {
			if original.Spec.NodeName == pod.Spec.NodeName {
				assert.Equal(t, original.UID, pod.UID, "peer must not roll during handoff")
			}
		}
	}
	require.NoError(t, guard.restore())
	require.Eventually(t, func() bool {
		pods := listPods()
		return len(pods) == len(nodes.Items) && allRestartTestPodsReady(pods)
	}, time.Minute, time.Second)
	after, err := dsAPI.Get(ctx, ds.Name, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, ds.Spec, after.Spec)
	// The controller updates its template-generation annotation during patches.
	assert.Equal(t, "original", after.Annotations["keep"])
	assert.NotContains(t, after.Annotations, restartStateAnnotation)
	for _, pod := range listPods() {
		if pod.Spec.NodeName == target {
			continue
		}
		for _, original := range beforePods {
			if original.Spec.NodeName == pod.Spec.NodeName {
				assert.Equal(t, original.UID, pod.UID, "restoration must not roll peers")
			}
		}
	}
}

func allRestartTestPodsReady(pods []corev1.Pod) bool {
	for _, pod := range pods {
		if pod.DeletionTimestamp != nil || !podReady(pod) {
			return false
		}
	}
	return true
}
