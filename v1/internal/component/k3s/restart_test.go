package k3s

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/catalystcommunity/foundry/v1/internal/ssh"
	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type restartSSH struct {
	t              *testing.T
	ds             appsv1.DaemonSet
	nodes          corev1.NodeList
	commands       []string
	restarted      bool
	noDeployment   bool
	keepPod        bool
	keepVIP        bool
	badTunnel      bool
	missingTunnel  bool
	badIPJSON      bool
	restartFailure bool
	patchLostReply bool
	patchConflict  bool
	restoreFailure bool
	peerFailure    bool
	staleFlannel   bool
	installerNoop  bool
	missingStamp   bool
	brokenBefore   bool
	staleAddress   bool
}

func newRestartSSH(t *testing.T) *restartSSH {
	f := &restartSSH{t: t}
	f.ds = appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "kube-vip", Namespace: "kube-system", UID: "vip-uid", ResourceVersion: "1", Annotations: map[string]string{"keep": "value"}}, Spec: appsv1.DaemonSetSpec{UpdateStrategy: appsv1.DaemonSetUpdateStrategy{Type: appsv1.RollingUpdateDaemonSetStrategyType}}}
	for _, name := range []string{"local", "peer"} {
		ip := "192.0.2.10"
		if name == "peer" {
			ip = "192.0.2.11"
		}
		f.nodes.Items = append(f.nodes.Items, corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{MachineID: name}, Addresses: []corev1.NodeAddress{{Type: corev1.NodeInternalIP, Address: ip}}, Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}})
		f.nodes.Items[len(f.nodes.Items)-1].Annotations = map[string]string{"flannel.alpha.coreos.com/public-ip": ip}
	}
	return f
}

func (f *restartSSH) Exec(command string) (*ssh.ExecResult, error) {
	f.commands = append(f.commands, command)
	ok := func(value interface{}) (*ssh.ExecResult, error) {
		data, err := json.Marshal(value)
		require.NoError(f.t, err)
		return &ssh.ExecResult{Stdout: string(data)}, nil
	}
	fail := func() (*ssh.ExecResult, error) { return &ssh.ExecResult{ExitCode: 1, Stderr: "injected failure"}, nil }
	switch {
	case command == k3sInvocationCommand:
		if f.restarted && !f.installerNoop {
			return &ssh.ExecResult{Stdout: "new-invocation"}, nil
		}
		return &ssh.ExecResult{Stdout: "old-invocation"}, nil
	case command == flannelStampCommand:
		if f.missingStamp {
			return fail()
		}
		if f.restarted && !f.staleFlannel && !f.installerNoop {
			return &ssh.ExecResult{Stdout: "new-stamp"}, nil
		}
		return &ssh.ExecResult{Stdout: "old-stamp"}, nil
	case command == "cat /etc/machine-id":
		return &ssh.ExecResult{Stdout: "local"}, nil
	case strings.Contains(command, "get daemonset kube-vip"):
		if f.noDeployment {
			return &ssh.ExecResult{}, nil
		}
		return ok(f.ds)
	case strings.HasSuffix(command, "get nodes -o json"):
		return ok(f.nodes)
	case strings.HasSuffix(command, "get node 'local' -o json"):
		node := f.nodes.Items[0].DeepCopy()
		if f.staleAddress {
			node.Annotations["flannel.alpha.coreos.com/public-ip"] = "192.0.2.100"
		}
		return ok(node)
	case strings.Contains(command, "get pods -l"):
		list := corev1.PodList{}
		for _, node := range f.nodes.Items {
			if node.Name == "local" && f.ds.Annotations[restartStateAnnotation] != "" && !f.keepPod {
				continue
			}
			controller := true
			list.Items = append(list.Items, corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "vip-" + node.Name, OwnerReferences: []metav1.OwnerReference{{UID: f.ds.UID, Controller: &controller}}}, Spec: corev1.PodSpec{NodeName: node.Name}, Status: corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}}})
		}
		return ok(list)
	case strings.Contains(command, "patch daemonset"):
		start := strings.Index(command, " -p '")
		require.GreaterOrEqual(f.t, start, 0)
		payload := strings.TrimSuffix(command[start+5:], "'")
		payload = strings.ReplaceAll(payload, "'\"'\"'", "'")
		patch, err := jsonpatch.DecodePatch([]byte(payload))
		require.NoError(f.t, err)
		paused := f.ds.Annotations[restartStateAnnotation] != ""
		if (!paused && f.patchConflict) || (paused && f.restoreFailure) {
			return fail()
		}
		data, err := json.Marshal(f.ds)
		require.NoError(f.t, err)
		data, err = patch.Apply(data)
		if err != nil {
			return &ssh.ExecResult{ExitCode: 1, Stderr: err.Error()}, nil
		}
		f.ds = appsv1.DaemonSet{}
		require.NoError(f.t, json.Unmarshal(data, &f.ds))
		if !paused && f.patchLostReply {
			return nil, errors.New("connection lost after patch")
		}
		return &ssh.ExecResult{}, nil
	case command == "ip -j address show":
		if f.badIPJSON {
			return &ssh.ExecResult{Stdout: "broken"}, nil
		}
		if f.keepVIP {
			return &ssh.ExecResult{Stdout: `[{"addr_info":[{"local":"192.0.2.100"}]}]`}, nil
		}
		return &ssh.ExecResult{Stdout: `[{"addr_info":[{"local":"192.0.2.10"}]}]`}, nil
	case command == "ip -j -d link show type vxlan":
		if f.missingTunnel {
			return &ssh.ExecResult{Stdout: "[]"}, nil
		}
		ip := "192.0.2.10"
		if f.badTunnel || (f.brokenBefore && !f.restarted) {
			ip = "192.0.2.100"
		}
		return &ssh.ExecResult{Stdout: `[{"ifname":"flannel.1","linkinfo":{"info_data":{"local":"` + ip + `"}}}]`}, nil
	case strings.Contains(command, "get --raw=/readyz"):
		if f.peerFailure {
			return fail()
		}
		return &ssh.ExecResult{Stdout: "ok"}, nil
	case command == "test-restart", command == "sudo systemctl restart k3s":
		f.restarted = true
		if !f.noDeployment {
			require.NotEmpty(f.t, f.ds.Annotations[restartStateAnnotation])
			require.Equal(f.t, appsv1.OnDeleteDaemonSetStrategyType, f.ds.Spec.UpdateStrategy.Type)
		}
		if f.restartFailure {
			return fail()
		}
		return &ssh.ExecResult{}, nil
	case command == "sudo k3s kubectl get nodes":
		return &ssh.ExecResult{}, nil
	default:
		f.t.Fatalf("unexpected command: %s", command)
		return nil, nil
	}
}

func TestVIPSafeRestart(t *testing.T) {
	for _, tc := range []struct {
		name      string
		setup     func(*restartSSH)
		wantErr   string
		restarted bool
		paused    bool
	}{
		{"success", func(*restartSSH) {}, "", true, false},
		{"no kube-vip", func(f *restartSSH) { f.noDeployment = true }, "", true, false},
		{"single server", func(f *restartSSH) { f.nodes.Items = f.nodes.Items[:1] }, "no other Ready", false, false},
		{"peer unavailable", func(f *restartSSH) { f.peerFailure = true }, "no other Ready", false, false},
		{"pod remains", func(f *restartSSH) { f.keepPod = true }, "still on the restart node", false, false},
		{"VIP remains", func(f *restartSSH) { f.keepVIP = true }, "still assigned", false, false},
		{"invalid IP output", func(f *restartSSH) { f.badIPJSON = true }, "VIP handoff failed", false, false},
		{"installer failed", func(f *restartSSH) { f.restartFailure = true }, "restart or upgrade failed", true, true},
		{"wrong tunnel", func(f *restartSSH) { f.badTunnel = true }, "post-restart Flannel", true, true},
		{"missing tunnel", func(f *restartSSH) { f.missingTunnel = true }, "post-restart Flannel", true, true},
		{"stale Flannel state", func(f *restartSSH) { f.staleFlannel = true }, "fresh startup state", true, true},
		{"stale advertised address", func(f *restartSSH) { f.staleAddress = true }, "Flannel advertises", true, true},
		{"installer skipped restart", func(f *restartSSH) { f.installerNoop = true }, "", true, false},
		{"missing Flannel state", func(f *restartSSH) { f.missingStamp = true }, "before restart", false, false},
		{"lost patch reply", func(f *restartSSH) { f.patchLostReply = true }, "connection lost", false, false},
		{"patch conflict", func(f *restartSSH) { f.patchConflict = true }, "injected failure", false, false},
		{"cleanup failed", func(f *restartSSH) { f.restoreFailure = true }, "recovery state", true, true},
		{"stale guard", func(f *restartSSH) { f.ds.Annotations[restartStateAnnotation] = "unfinished" }, "unfinished restart guard", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRestartSSH(t)
			original := f.ds.DeepCopy()
			tc.setup(f)
			err := runVIPSafeRestart(context.Background(), f, &Config{VIP: "192.0.2.100"}, "test-restart", RetryConfig{MaxRetries: 1})
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErr)
			}
			assert.Equal(t, tc.restarted, f.restarted)
			if tc.paused {
				assert.NotEmpty(t, f.ds.Annotations[restartStateAnnotation])
			} else {
				assert.Equal(t, original, &f.ds)
			}
		})
	}
}

func TestVIPRestartContextCancellation(t *testing.T) {
	f := newRestartSSH(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := runVIPSafeRestart(ctx, f, &Config{VIP: "192.0.2.100"}, "test-restart", RetryConfig{MaxRetries: 1})
	require.ErrorIs(t, err, context.Canceled)
	assert.False(t, f.restarted)
	assert.Empty(t, f.ds.Annotations[restartStateAnnotation])
}

func TestControlPlaneRestartEntryPoints(t *testing.T) {
	for _, joined := range []bool{false, true} {
		for _, upgrade := range []bool{false, true} {
			for _, failure := range []bool{false, true} {
				name := "first/config"
				if joined {
					name = "joined/config"
				}
				if upgrade {
					name = strings.ReplaceAll(name, "config", "upgrade")
				}
				if failure {
					name += "/command-failure"
				}
				t.Run(name, func(t *testing.T) {
					f := newRestartSSH(t)
					f.restartFailure = failure
					original := f.ds.DeepCopy()
					executor := &mockInstallSSHExecutor{execFunc: func(command string) (*ssh.ExecResult, error) {
						switch {
						case strings.Contains(command, "fs.inotify.max_user_instances"):
							return &ssh.ExecResult{}, nil
						case strings.Contains(command, "systemctl is-active k3s"):
							return &ssh.ExecResult{Stdout: "active"}, nil
						case command == "k3s --version":
							return &ssh.ExecResult{Stdout: "k3s version v1.34.2+k3s1"}, nil
						case strings.Contains(command, "curl -sfL https://get.k3s.io"), command == "sudo systemctl restart k3s":
							return f.Exec("test-restart") // Asserts that the guard is active.
						case strings.HasPrefix(command, "echo '") && (strings.Contains(command, "sudo tee") || strings.Contains(command, "kubectl apply")):
							return &ssh.ExecResult{}, nil
						case strings.HasPrefix(command, "cat /etc/rancher/k3s/registries.yaml"), command == "sudo mkdir -p /etc/rancher/k3s":
							return &ssh.ExecResult{}, nil
						default:
							return f.Exec(command)
						}
					}}
					cfg := &Config{Version: "v1.34.3+k3s1", VIP: "192.168.1.100", Interface: "eth0", ClusterInit: true, ClusterToken: "test-token"}
					if !upgrade {
						cfg.RegistryConfig = "mirrors: {}"
					}
					var err error
					if joined {
						if upgrade {
							err = UpgradeJoinedControlPlane(context.Background(), executor, "https://192.168.1.100:6443", &Tokens{ClusterToken: "test-token"}, cfg)
						} else {
							err = JoinControlPlane(context.Background(), executor, "https://192.168.1.100:6443", &Tokens{ClusterToken: "test-token"}, cfg)
						}
					} else if upgrade {
						err = UpgradeControlPlane(context.Background(), executor, cfg)
					} else {
						err = InstallControlPlane(context.Background(), executor, cfg)
					}
					if failure {
						require.ErrorContains(t, err, "injected failure")
						require.ErrorContains(t, err, "remains excluded")
						assert.NotEmpty(t, f.ds.Annotations[restartStateAnnotation])
					} else {
						require.NoError(t, err)
						assert.Equal(t, original, &f.ds)
					}
					assert.True(t, f.restarted)
				})
			}
		}
	}
}

func TestVIPRestartRestoresOnlyOwnedChanges(t *testing.T) {
	f := newRestartSSH(t)
	g, err := prepareVIPRestart(f, "192.0.2.100")
	require.NoError(t, err)
	require.NoError(t, g.pause())
	f.ds.Annotations["unrelated"] = "keep"
	require.NoError(t, g.restore())
	assert.Equal(t, "keep", f.ds.Annotations["unrelated"])
	g, err = prepareVIPRestart(f, "192.0.2.100")
	require.NoError(t, err)
	require.NoError(t, g.pause())
	f.ds.Spec.Template.Spec.Affinity.PodAffinity = &corev1.PodAffinity{}
	require.Error(t, g.restore(), "must not overwrite a concurrent affinity edit")
	assert.NotEmpty(t, f.ds.Annotations[restartStateAnnotation])
}

func TestExcludeVIPNodePreservesAffinity(t *testing.T) {
	before := &corev1.Affinity{PodAffinity: &corev1.PodAffinity{}, NodeAffinity: &corev1.NodeAffinity{RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{
		{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "pool", Operator: corev1.NodeSelectorOpIn, Values: []string{"one"}}}},
		{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "pool", Operator: corev1.NodeSelectorOpIn, Values: []string{"two"}}}},
		{},
	}}}}
	saved := before.DeepCopy()
	after := excludeVIPNode(before, "local")
	assert.Equal(t, saved, before)
	assert.Equal(t, before.PodAffinity, after.PodAffinity)
	terms := after.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	for _, term := range terms[:2] {
		require.Len(t, term.MatchFields, 1)
		assert.Equal(t, []string{"local"}, term.MatchFields[0].Values)
	}
	assert.Empty(t, terms[2].MatchFields, "an empty original term must remain unsatisfiable")
}

func TestReconcileVIPNetwork(t *testing.T) {
	for _, tc := range []struct {
		name      string
		setup     func(*restartSSH)
		wantErr   string
		restarted bool
	}{
		{"healthy", func(*restartSSH) {}, "", false},
		{"healthy single server", func(f *restartSSH) { f.nodes.Items = f.nodes.Items[:1] }, "", false},
		{"no kube-vip", func(f *restartSSH) { f.noDeployment = true }, "", false},
		{"repair VIP tunnel", func(f *restartSSH) { f.brokenBefore = true }, "", true},
		{"repair without peer", func(f *restartSSH) { f.brokenBefore = true; f.nodes.Items = f.nodes.Items[:1] }, "no other Ready", false},
		{"restart failure", func(f *restartSSH) { f.brokenBefore = true; f.restartFailure = true }, "remains excluded", true},
		{"repair failure", func(f *restartSSH) { f.badTunnel = true }, "post-restart Flannel", true},
		{"unfinished guard", func(f *restartSSH) { f.ds.Annotations[restartStateAnnotation] = "unfinished" }, "unfinished restart guard", false},
		{"unknown machine", func(f *restartSSH) { f.nodes.Items[0].Status.NodeInfo.MachineID = "unknown" }, "does not match", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRestartSSH(t)
			tc.setup(f)
			err := reconcileVIPNetwork(context.Background(), f, &Config{VIP: "192.0.2.100"}, RetryConfig{MaxRetries: 1})
			if tc.wantErr == "" {
				require.NoError(t, err)
				assert.Empty(t, f.ds.Annotations[restartStateAnnotation])
			} else {
				require.ErrorContains(t, err, tc.wantErr)
			}
			assert.Equal(t, tc.restarted, f.restarted)
		})
	}
}

func TestValidateFlannelNetwork(t *testing.T) {
	for _, tc := range []struct{ name, tunnel, advertised, wantErr string }{
		{"node IP", "192.0.2.10", "192.0.2.10", ""},
		{"VIP", "192.0.2.100", "192.0.2.100", "invalid tunnel address"},
		{"invalid IP", "invalid", "192.0.2.10", "invalid tunnel address"},
		{"stale advertisement", "192.0.2.10", "192.0.2.100", "Flannel advertises"},
		{"missing advertisement", "192.0.2.10", "", "Flannel advertises"},
		{"other address", "192.0.2.11", "192.0.2.11", "not a node InternalIP"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := newRestartSSH(t).nodes.Items[0]
			node.Annotations["flannel.alpha.coreos.com/public-ip"] = tc.advertised
			data := `[{"ifname":"flannel.1","linkinfo":{"info_data":{"local":"` + tc.tunnel + `"}}}]`
			err := validateFlannelNetwork(data, node, "192.0.2.100")
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErr)
			}
		})
	}
}

func TestControlPlaneRepairsNetworkWithoutConfigChanges(t *testing.T) {
	for _, joined := range []bool{false, true} {
		t.Run(map[bool]string{false: "first", true: "joined"}[joined], func(t *testing.T) {
			f := newRestartSSH(t)
			f.brokenBefore = true
			executor := &mockInstallSSHExecutor{execFunc: func(command string) (*ssh.ExecResult, error) {
				switch {
				case strings.Contains(command, "fs.inotify.max_user_instances"):
					return &ssh.ExecResult{}, nil
				case strings.Contains(command, "systemctl is-active k3s"):
					return &ssh.ExecResult{Stdout: "active"}, nil
				case strings.Contains(command, "kubectl apply"):
					return &ssh.ExecResult{}, nil
				default:
					return f.Exec(command)
				}
			}}
			cfg := &Config{VIP: "192.168.1.100", Interface: "eth0", ClusterInit: true}
			var err error
			if joined {
				err = JoinControlPlane(context.Background(), executor, "https://192.168.1.100:6443", &Tokens{ClusterToken: "test-token"}, cfg)
			} else {
				err = InstallControlPlane(context.Background(), executor, cfg)
			}
			require.NoError(t, err)
			assert.True(t, f.restarted)
			assert.Empty(t, f.ds.Annotations[restartStateAnnotation])
		})
	}
}
