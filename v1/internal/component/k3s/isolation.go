package k3s

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const VIPInterfaceName = "foundry-vip"
const isolationAnnotation = "foundry.catalystcommunity.org/vip-isolation-state"
const isolationConfigPath = "/etc/rancher/k3s/config.yaml.d/90-foundry-network.yaml"
const isolationMarkerPath = "/var/lib/foundry/k3s-network-applied"

//go:embed vip-interface.py
var vipInterfaceScript string

// NetworkHost is an SSH target, not a persisted configuration object.
type NetworkHost struct {
	Name, Address string
	Executor      SSHExecutor
}
type isolationLink struct {
	Name        string                                  `json:"ifname"`
	Index       int                                     `json:"ifindex"`
	ParentName  string                                  `json:"link"`
	ParentIndex int                                     `json:"link_index"`
	Type        string                                  `json:"link_type"`
	Flags       []string                                `json:"flags"`
	Addresses   []struct{ Local, Family, Scope string } `json:"addr_info"`
}
type isolationPlan struct {
	Host                             NetworkHost
	Parent, Flannel, IP, Fingerprint string
	Installed                        bool
}

var interfaceNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,14}$`)

func selectIsolationInterfaces(links []isolationLink, preferredIP, defaultInterface, previousVIPInterface string) (parent, flannel, ip string, err error) {
	for _, link := range links {
		for _, address := range link.Addresses {
			if address.Family == "inet" && address.Local == preferredIP {
				flannel, ip = link.Name, address.Local
			}
		}
	}
	if flannel == "" {
		for _, link := range links {
			if link.Name == defaultInterface {
				for _, address := range link.Addresses {
					if address.Family == "inet" && address.Scope == "global" {
						if ip != "" {
							return "", "", "", fmt.Errorf("cannot choose a node address on %s; specify a host address assigned to the node", link.Name)
						}
						flannel, ip = link.Name, address.Local
					}
				}
			}
		}
	}
	if previousVIPInterface == VIPInterfaceName {
		for _, child := range links {
			if child.Name == VIPInterfaceName {
				for _, base := range links {
					if base.Index == child.ParentIndex || base.Name == child.ParentName {
						previousVIPInterface = base.Name
					}
				}
			}
		}
	}
	ethernet := func(name string) bool {
		for _, link := range links {
			if link.Name == name && link.Name != VIPInterfaceName && link.Type == "ether" {
				for _, flag := range link.Flags {
					if flag == "NOARP" {
						return false
					}
				}
				return true
			}
		}
		return false
	}
	// A CGNAT VIP still needs an Ethernet attachment. Never create a macvlan on
	// tailscale0, nor move or reconfigure the Tailscale-owned node address.
	if ethernet(previousVIPInterface) {
		parent = previousVIPInterface
	} else if ethernet(defaultInterface) {
		parent = defaultInterface
	}
	if parent == "" || flannel == "" || flannel == VIPInterfaceName || net.ParseIP(ip).To4() == nil || !net.ParseIP(ip).IsGlobalUnicast() {
		return "", "", "", fmt.Errorf("cannot isolate VIP: need a node IPv4 address and an Ethernet VIP parent")
	}
	if !interfaceNamePattern.MatchString(parent) || !interfaceNamePattern.MatchString(flannel) {
		return "", "", "", fmt.Errorf("unsupported interface name")
	}
	return parent, flannel, ip, nil
}

func planIsolation(host NetworkHost, nodeIP, oldInterface string) (isolationPlan, error) {
	p := isolationPlan{Host: host}
	var err error
	p.Installed, err = IsK3sInstalled(host.Executor)
	if err != nil {
		return p, err
	}
	data, err := restartExec(host.Executor, "ip -j -d address show")
	if err != nil {
		return p, err
	}
	var links []isolationLink
	if err = json.Unmarshal([]byte(data), &links); err != nil {
		return p, fmt.Errorf("invalid interface inventory")
	}
	route, err := restartExec(host.Executor, "ip -j -4 route show default")
	if err != nil {
		return p, err
	}
	var routes []struct {
		Dev string `json:"dev"`
	}
	if err = json.Unmarshal([]byte(route), &routes); err != nil || len(routes) == 0 {
		return p, fmt.Errorf("no IPv4 default route for %s", host.Name)
	}
	registeredIP := nodeIP
	if nodeIP == "" {
		nodeIP = host.Address
		if nodeIP != "" && net.ParseIP(nodeIP) == nil {
			// Resolve hostnames on the target, including Tailscale MagicDNS.
			script := "import socket,sys,json; print(json.dumps(sorted({a[4][0] for a in socket.getaddrinfo(sys.argv[1],None,socket.AF_INET)})))"
			resolved, err := restartExec(host.Executor, "python3 -c "+shellQuote(script)+" "+shellQuote(nodeIP))
			if err != nil {
				return p, fmt.Errorf("cannot resolve host address on %s: %w", host.Name, err)
			}
			var addresses []string
			if json.Unmarshal([]byte(resolved), &addresses) != nil {
				return p, fmt.Errorf("invalid host address resolution")
			}
			for _, link := range links {
				for _, a := range link.Addresses {
					for _, candidate := range addresses {
						if a.Family == "inet" && a.Local == candidate {
							nodeIP = candidate
						}
					}
				}
			}
		}
	}
	p.Parent, p.Flannel, p.IP, err = selectIsolationInterfaces(links, nodeIP, routes[0].Dev, oldInterface)
	if err != nil {
		return p, err
	}
	if registeredIP != "" && p.IP != registeredIP {
		return p, fmt.Errorf("registered node InternalIP is absent from its interfaces")
	}
	// Read network settings only. Never log the service command or its tokens.
	if err = checkIsolationOverrides(host.Executor, p); err != nil {
		return p, err
	}
	p.Fingerprint = fmt.Sprintf("%x", sha256.Sum256([]byte(p.Parent+"\n"+p.Flannel+"\n"+p.IP+"\n"+vipInterfaceScript)))
	return p, nil
}

func isolationConfig(p isolationPlan) string {
	return fmt.Sprintf("# Managed by Foundry.\nnode-ip: %s\nflannel-iface: %s\n", p.IP, p.Flannel)
}

type isolationOverrides struct {
	Files map[string]string
	Args  []string
	Env   map[string]string
}

// Check existing YAML and live CLI/environment overrides before any mutation.
// File bodies stay in memory; parse errors must not include their contents.
func checkIsolationOverrides(executor SSHExecutor, p isolationPlan) error {
	script := `import json,glob,os,subprocess,re,shlex
files={}
for path in ['/etc/rancher/k3s/config.yaml']+sorted(glob.glob('/etc/rancher/k3s/config.yaml.d/*.yaml')):
 if os.path.isfile(path): files[path]=open(path).read()
args=[]
env={}
def network_args(raw):
 for i,a in enumerate(raw):
  if a.split('=')[0] in ('--node-ip','--flannel-iface','--flannel-backend','--flannel-external-ip','--config','-c'):
   args.append(a if '=' in a else a+'='+('true' if a=='--flannel-external-ip' else raw[i+1] if i+1<len(raw) else ''))
# K3s can replace its process title. Check persistent ExecStart as well.
start=subprocess.run(['systemctl','show','k3s','-p','ExecStart','--value'],capture_output=True,text=True).stdout
match=re.search(r'argv\[\]=(.*?) ;',start,re.S)
if match: network_args(shlex.split(match.group(1)))
pid=subprocess.run(['systemctl','show','k3s','-p','MainPID','--value'],capture_output=True,text=True).stdout.strip()
if pid and pid!='0':
 network_args(open('/proc/'+pid+'/cmdline','rb').read().decode().split('\0'))
 for a in open('/proc/'+pid+'/environ','rb').read().decode().split('\0'):
  k,_,v=a.partition('=')
  if k in ('K3S_NODE_IP','K3S_FLANNEL_IFACE','K3S_FLANNEL_BACKEND','K3S_FLANNEL_EXTERNAL_IP','K3S_CONFIG_FILE'): env[k]=v
print(json.dumps({'files':files,'args':args,'env':env}))`
	data, err := restartExec(executor, "sudo python3 -c "+shellQuote(script))
	if err != nil {
		return err
	}
	var inventory isolationOverrides
	if json.Unmarshal([]byte(data), &inventory) != nil {
		return fmt.Errorf("invalid K3s network configuration inventory")
	}
	return validateIsolationOverrides(inventory, p)
}

func validateIsolationOverrides(inventory isolationOverrides, p isolationPlan) error {
	check := func(key string, value interface{}) error {
		text := fmt.Sprint(value)
		switch key {
		case "config", "-c", "config-file":
			if text != "/etc/rancher/k3s/config.yaml" {
				return fmt.Errorf("custom K3s config path is not supported by VIP conversion")
			}
		case "flannel-iface":
			if text != p.Flannel {
				return fmt.Errorf("existing flannel-iface conflicts with node interface %s", p.Flannel)
			}
		case "node-ip":
			if values, ok := value.([]interface{}); ok && len(values) == 1 {
				text = fmt.Sprint(values[0])
			}
			if text != p.IP {
				return fmt.Errorf("existing node-ip conflicts with node InternalIP %s", p.IP)
			}
		case "flannel-backend":
			if text != "vxlan" {
				return fmt.Errorf("VIP conversion currently requires the VXLAN backend")
			}
		case "flannel-external-ip":
			if text != "false" {
				return fmt.Errorf("VIP conversion does not support flannel-external-ip")
			}
		}
		return nil
	}
	for path, body := range inventory.Files {
		if path == isolationConfigPath {
			continue
		}
		var cfg map[string]interface{}
		if yaml.Unmarshal([]byte(body), &cfg) != nil {
			return fmt.Errorf("cannot parse existing K3s YAML in %s", path)
		}
		for key, value := range cfg {
			if err := check(strings.TrimSuffix(key, "+"), value); err != nil {
				return err
			}
		}
	}
	for _, arg := range inventory.Args {
		key, value, _ := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		if err := check(key, value); err != nil {
			return err
		}
	}
	for key, value := range inventory.Env {
		key = strings.ToLower(strings.ReplaceAll(strings.TrimPrefix(key, "K3S_"), "_", "-"))
		if err := check(key, value); err != nil {
			return err
		}
	}
	return nil
}

func writeIsolationFile(executor SSHExecutor, path, body string, mode int) error {
	script := `import os,sys,base64,tempfile
path,encoded,mode=sys.argv[1:]
data=base64.b64decode(encoded)
if os.path.islink(path): sys.exit('Refusing a symlink at '+path)
if os.path.exists(path):
 old=open(path,'rb').read()
 if old==data: sys.exit(0)
 if b'Managed by Foundry' not in old: sys.exit('Refusing to replace unmanaged file '+path)
os.makedirs(os.path.dirname(path),exist_ok=True)
fd,tmp=tempfile.mkstemp(dir=os.path.dirname(path))
try:
 os.fchmod(fd,int(mode,8))
 with os.fdopen(fd,'wb') as out:
  out.write(data);out.flush();os.fsync(out.fileno())
 os.replace(tmp,path)
finally:
 if os.path.exists(tmp):os.unlink(tmp)`
	_, err := restartExec(executor, "sudo python3 -c "+shellQuote(script)+" "+shellQuote(path)+" "+shellQuote(base64.StdEncoding.EncodeToString([]byte(body)))+fmt.Sprintf(" %o", mode))
	return err
}
func prepareIsolation(p isolationPlan) error {
	e := p.Host.Executor
	// Validate/create the interface before installing any startup configuration.
	if _, err := restartExec(e, "sudo python3 -c "+shellQuote(vipInterfaceScript)+" "+shellQuote(p.Parent)); err != nil {
		return err
	}
	if err := writeIsolationFile(e, "/usr/local/libexec/foundry-vip-interface", vipInterfaceScript, 0755); err != nil {
		return err
	}
	if err := writeIsolationFile(e, isolationConfigPath, isolationConfig(p), 0644); err != nil {
		return err
	}
	dropin := "# Managed by Foundry.\n[Unit]\nWants=network-online.target\nAfter=network-online.target\n[Service]\nExecStartPre=/usr/local/libexec/foundry-vip-interface " + p.Parent + "\n"
	if err := writeIsolationFile(e, "/etc/systemd/system/k3s.service.d/20-foundry-vip-interface.conf", dropin, 0644); err != nil {
		return err
	}
	_, err := restartExec(e, "sudo systemctl daemon-reload")
	return err
}

// PrepareVIPHost prepares a new server before the kube-vip DaemonSet can land
// on it. Existing clusters use ReconcileVIPIsolation to migrate all servers.
func PrepareVIPHost(host NetworkHost) error {
	p, err := planIsolation(host, "", "")
	if err != nil {
		return err
	}
	return prepareIsolation(p)
}

func isolatedVIPEnv(env []corev1.EnvVar) []corev1.EnvVar {
	out := append([]corev1.EnvVar(nil), env...)
	for _, name := range []string{"vip_interface", "vip_servicesinterface"} {
		found := false
		for i := range out {
			if out[i].Name == name {
				out[i] = corev1.EnvVar{Name: name, Value: VIPInterfaceName}
				found = true
			}
		}
		if !found {
			out = append(out, corev1.EnvVar{Name: name, Value: VIPInterfaceName})
		}
	}
	return out
}
func envValue(env []corev1.EnvVar, name string) string {
	for _, e := range env {
		if e.Name == name {
			return e.Value
		}
	}
	return ""
}

// ReconcileVIPIsolation converts an existing cluster before other upgrade work.
// All hosts are prepared first. Then each server uses the existing restart
// guard, and its replacement kube-vip Pod uses the isolated interface.
func ReconcileVIPIsolation(ctx context.Context, hosts []NetworkHost, vip string, dryRun bool) error {
	if len(hosts) == 0 {
		return fmt.Errorf("no control-plane hosts configured")
	}
	anyInstalled := false
	var api SSHExecutor
	var ds appsv1.DaemonSet
	for _, h := range hosts {
		installed, err := IsK3sInstalled(h.Executor)
		if err != nil {
			return err
		}
		if installed {
			anyInstalled = true
			data, err := restartExec(h.Executor, localRestartKubectl+" -n kube-system get daemonset kube-vip --ignore-not-found -o json")
			if err != nil {
				continue
			}
			api = h.Executor
			if data != "" {
				if err := json.Unmarshal([]byte(data), &ds); err != nil {
					return err
				}
			}
			break
		}
	}
	if anyInstalled && api == nil {
		return fmt.Errorf("no installed control plane has a reachable API; network conversion was not started")
	}
	nodeIPs := map[string]string{}
	nodeNames := map[string]string{}
	var nodes corev1.NodeList
	if api != nil {
		data, err := restartExec(api, localRestartKubectl+" get nodes -o json")
		if err != nil {
			return err
		}
		if err = json.Unmarshal([]byte(data), &nodes); err != nil {
			return err
		}
		for _, node := range nodes.Items {
			for _, a := range node.Status.Addresses {
				if a.Type == corev1.NodeInternalIP && net.ParseIP(a.Address).To4() != nil {
					nodeIPs[node.Status.NodeInfo.MachineID] = a.Address
					nodeNames[node.Status.NodeInfo.MachineID] = node.Name
				}
			}
		}
	}
	oldInterface := ""
	containerIndex := -1
	if ds.Name != "" {
		if ds.Annotations[restartStateAnnotation] != "" {
			return fmt.Errorf("unfinished K3s restart guard; recover it before network conversion")
		}
		for i, c := range ds.Spec.Template.Spec.Containers {
			if c.Name == "kube-vip" {
				containerIndex = i
				oldInterface = envValue(c.Env, "vip_interface")
			}
		}
		if containerIndex < 0 {
			return fmt.Errorf("kube-vip container not found")
		}
		if envValue(ds.Spec.Template.Spec.Containers[containerIndex].Env, "vip_address") != vip {
			return fmt.Errorf("configured VIP differs from kube-vip; reconcile the VIP address before interface conversion")
		}
	}
	plans := make([]isolationPlan, 0, len(hosts))
	covered := map[string]bool{}
	for _, h := range hosts {
		id, err := restartExec(h.Executor, "cat /etc/machine-id")
		if err != nil {
			return err
		}
		p, err := planIsolation(h, nodeIPs[id], oldInterface)
		if err != nil {
			return fmt.Errorf("%s network preflight: %w", h.Name, err)
		}
		if p.IP == vip {
			return fmt.Errorf("VIP must differ from node address on %s", h.Name)
		}
		if nodeNames[id] != "" {
			if !p.Installed {
				return fmt.Errorf("K3s is not active on registered control plane %s; restore service health before conversion", nodeNames[id])
			}
			p.Host.Name = nodeNames[id]
			covered[nodeNames[id]] = true
		}
		plans = append(plans, p)
		fmt.Printf("  %s: node %s on %s; VIP interface %s on %s\n", h.Name, p.IP, p.Flannel, VIPInterfaceName, p.Parent)
	}
	for _, node := range nodes.Items {
		if _, cp := node.Labels["node-role.kubernetes.io/control-plane"]; cp && !covered[node.Name] {
			return fmt.Errorf("control plane %s is absent from the host inventory", node.Name)
		}
	}
	strategy := ds.Spec.UpdateStrategy
	if saved := ds.Annotations[isolationAnnotation]; saved != "" {
		if json.Unmarshal([]byte(saved), &strategy) != nil {
			return fmt.Errorf("invalid saved VIP migration strategy")
		}
	}
	if dryRun {
		return nil
	}
	for _, p := range plans {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := prepareIsolation(p); err != nil {
			return fmt.Errorf("prepare %s: %w", p.Host.Name, err)
		}
	}
	if ds.Name == "" {
		return nil
	} // New cluster: every server is ready before installation.
	saved, _ := json.Marshal(strategy)
	desiredEnv := isolatedVIPEnv(ds.Spec.Template.Spec.Containers[containerIndex].Env)
	patches := []map[string]interface{}{
		{"op": "test", "path": "/metadata/resourceVersion", "value": ds.ResourceVersion},
		{"op": "add", "path": "/spec/updateStrategy", "value": appsv1.DaemonSetUpdateStrategy{Type: appsv1.OnDeleteDaemonSetStrategyType}},
		{"op": "replace", "path": fmt.Sprintf("/spec/template/spec/containers/%d/env", containerIndex), "value": desiredEnv},
	}
	if ds.Annotations == nil {
		patches = append(patches, map[string]interface{}{"op": "add", "path": "/metadata/annotations", "value": map[string]string{isolationAnnotation: string(saved)}})
	} else {
		patches = append(patches, map[string]interface{}{"op": "add", "path": "/metadata/annotations/foundry.catalystcommunity.org~1vip-isolation-state", "value": string(saved)})
	}
	patch := func(ops []map[string]interface{}) error {
		data, _ := json.Marshal(ops)
		_, err := restartExec(api, localRestartKubectl+" -n kube-system patch daemonset kube-vip --type=json -p "+shellQuote(string(data)))
		return err
	}
	if err := patch(patches); err != nil {
		return err
	}
	// Followers first; the active holder stays available while peers convert.
	data, err := restartExec(api, localRestartKubectl+" -n kube-system get lease plndr-cp-lock -o json")
	if err != nil {
		return err
	}
	var lease struct {
		Spec struct {
			Holder string `json:"holderIdentity"`
		} `json:"spec"`
	}
	if err = json.Unmarshal([]byte(data), &lease); err != nil {
		return err
	}
	sort.SliceStable(plans, func(i, j int) bool {
		return plans[i].Host.Name != lease.Spec.Holder && plans[j].Host.Name == lease.Spec.Holder
	})
	installedCount := 0
	for _, p := range plans {
		if p.Installed {
			installedCount++
		}
	}
	for _, p := range plans {
		if !p.Installed {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		marker, err := restartExec(p.Host.Executor, "sudo sh -c "+shellQuote("if test -f "+isolationMarkerPath+"; then cat "+isolationMarkerPath+"; fi"))
		if err != nil {
			return err
		}
		podsData, err := restartExec(api, localRestartKubectl+" -n kube-system get pods -l app.kubernetes.io/name=kube-vip -o json")
		if err != nil {
			return err
		}
		var pods corev1.PodList
		if err = json.Unmarshal([]byte(podsData), &pods); err != nil {
			return err
		}
		current := false
		var podName string
		for _, pod := range pods.Items {
			if pod.Spec.NodeName == p.Host.Name {
				podName = pod.Name
				for _, c := range pod.Spec.Containers {
					if c.Name == "kube-vip" && envValue(c.Env, "vip_interface") == VIPInterfaceName && envValue(c.Env, "vip_servicesinterface") == VIPInterfaceName && podReady(pod) {
						current = true
					}
				}
			}
		}
		if strings.Contains(marker, p.Fingerprint) && current {
			if err := validateIsolatedHost(p, vip); err == nil {
				fmt.Printf("  %s: VIP isolation already applied\n", p.Host.Name)
				continue
			}
		}
		fmt.Printf("  Converting VIP networking on %s...\n", p.Host.Name)
		if installedCount == 1 {
			if podName != "" {
				if _, err = restartExec(api, localRestartKubectl+" -n kube-system delete pod "+shellQuote(podName)+" --wait=true --timeout=60s"); err != nil {
					return err
				}
			}
			// The API stays up during the Pod handoff. Wait until the VIP is isolated.
			err = retryRestartCheck(ctx, DefaultRetryConfig(), func() error { return validateVIPPlacement(p, vip, true) })
			if err == nil {
				_, err = restartExec(p.Host.Executor, "sudo systemctl restart k3s")
			}
			if err == nil {
				err = waitForK3sReady(p.Host.Executor, DefaultRetryConfig())
			}
		} else {
			err = withVIPSafeRestart(ctx, p.Host.Executor, &Config{VIP: vip}, "sudo systemctl restart k3s")
		}
		if err != nil {
			return fmt.Errorf("network conversion stopped on %s; saved DaemonSet state remains: %w", p.Host.Name, err)
		}
		if err = retryRestartCheck(ctx, DefaultRetryConfig(), func() error { return validateIsolatedHost(p, vip) }); err != nil {
			return err
		}
		if err = retryRestartCheck(ctx, DefaultRetryConfig(), func() error { return readyIsolatedVIPPod(api, p.Host.Name) }); err != nil {
			return err
		}
		if err = writeIsolationFile(p.Host.Executor, isolationMarkerPath, "# Managed by Foundry.\n"+p.Fingerprint+"\n", 0644); err != nil {
			return err
		}
	}
	// Wait for all replacement Pods before restoring the original rollout policy.
	if err = retryRestartCheck(ctx, DefaultRetryConfig(), func() error {
		data, err := restartExec(api, localRestartKubectl+" -n kube-system get pods -l app.kubernetes.io/name=kube-vip -o json")
		if err != nil {
			return err
		}
		var pods corev1.PodList
		if err = json.Unmarshal([]byte(data), &pods); err != nil {
			return err
		}
		if len(pods.Items) != installedCount {
			return fmt.Errorf("waiting for all kube-vip Pods")
		}
		for _, pod := range pods.Items {
			if !podReady(pod) || pod.DeletionTimestamp != nil {
				return fmt.Errorf("waiting for Ready kube-vip Pods")
			}
			found := false
			for _, c := range pod.Spec.Containers {
				if c.Name == "kube-vip" && envValue(c.Env, "vip_interface") == VIPInterfaceName && envValue(c.Env, "vip_servicesinterface") == VIPInterfaceName {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("waiting for converted kube-vip Pods")
			}
		}
		return nil
	}); err != nil {
		return err
	}
	// Ready kube-vip Pods do not prove that the floating API address works.
	if err = retryRestartCheck(ctx, DefaultRetryConfig(), func() error {
		response, err := restartExec(api, localRestartKubectl+" --server="+shellQuote("https://"+vip+":6443")+" get --raw=/readyz")
		if err != nil {
			return err
		}
		if response != "ok" {
			return fmt.Errorf("API is not ready through the VIP")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("VIP connectivity check failed; saved migration state remains: %w", err)
	}
	return patch([]map[string]interface{}{
		{"op": "test", "path": "/metadata/annotations/foundry.catalystcommunity.org~1vip-isolation-state", "value": string(saved)},
		{"op": "test", "path": "/spec/updateStrategy", "value": appsv1.DaemonSetUpdateStrategy{Type: appsv1.OnDeleteDaemonSetStrategyType}},
		{"op": "test", "path": fmt.Sprintf("/spec/template/spec/containers/%d/env", containerIndex), "value": desiredEnv},
		{"op": "add", "path": "/spec/updateStrategy", "value": strategy},
		{"op": "remove", "path": "/metadata/annotations/foundry.catalystcommunity.org~1vip-isolation-state"},
	})
}
func validateVIPPlacement(p isolationPlan, vip string, requireVIP bool) error {
	data, err := restartExec(p.Host.Executor, "ip -j -d address show")
	if err != nil {
		return err
	}
	var links []isolationLink
	if err = json.Unmarshal([]byte(data), &links); err != nil {
		return err
	}
	found := false
	for _, link := range links {
		for _, a := range link.Addresses {
			if a.Local == vip {
				if link.Name != VIPInterfaceName {
					return fmt.Errorf("VIP remains on %s", link.Name)
				}
				found = true
			}
			if link.Name == p.Flannel && a.Family == "inet" && a.Scope == "global" && a.Local != p.IP {
				return fmt.Errorf("Flannel interface has another IPv4 address: %s", a.Local)
			}
		}
	}
	if requireVIP && !found {
		return fmt.Errorf("waiting for VIP on %s", VIPInterfaceName)
	}
	return nil
}
func validateIsolatedHost(p isolationPlan, vip string) error {
	if err := validateVIPPlacement(p, vip, false); err != nil {
		return err
	}
	node, _, err := localRestartNode(p.Host.Executor)
	if err != nil {
		return err
	}
	if !nodeReady(*node) {
		return fmt.Errorf("node is not Ready")
	}
	data, err := restartExec(p.Host.Executor, "ip -j -d link show type vxlan")
	if err != nil {
		return err
	}
	return validateFlannelNetwork(data, *node, vip)
}

func readyIsolatedVIPPod(api SSHExecutor, node string) error {
	data, err := restartExec(api, localRestartKubectl+" -n kube-system get pods -l app.kubernetes.io/name=kube-vip -o json")
	if err != nil {
		return err
	}
	var pods corev1.PodList
	if err = json.Unmarshal([]byte(data), &pods); err != nil {
		return err
	}
	for _, pod := range pods.Items {
		if pod.Spec.NodeName != node || pod.DeletionTimestamp != nil || !podReady(pod) {
			continue
		}
		for _, c := range pod.Spec.Containers {
			if c.Name == "kube-vip" && envValue(c.Env, "vip_interface") == VIPInterfaceName && envValue(c.Env, "vip_servicesinterface") == VIPInterfaceName {
				return nil
			}
		}
	}
	return fmt.Errorf("waiting for converted kube-vip Pod on %s", node)
}
