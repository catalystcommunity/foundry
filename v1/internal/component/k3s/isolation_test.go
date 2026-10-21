package k3s

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestSelectIsolationInterfaces(t *testing.T) {
	const inventory = `[
 {"ifname":"ens18","ifindex":2,"link_type":"ether","flags":["UP"],"addr_info":[{"family":"inet","scope":"global","local":"10.16.0.31"},{"family":"inet","scope":"global","local":"10.16.0.30"}]},
 {"ifname":"tailscale0","ifindex":3,"link_type":"none","flags":["UP","NOARP"],"addr_info":[{"family":"inet","scope":"global","local":"100.80.0.1"}]},
 {"ifname":"lan2","ifindex":4,"link_type":"ether","flags":["UP"],"addr_info":[]},
 {"ifname":"foundry-vip","ifindex":5,"link_index":4,"link_type":"ether","flags":["UP"],"addr_info":[]}
 ]`
	for _, tt := range []struct {
		name, ip, route, old, parent, flannel string
		fail                                  bool
	}{
		{"LAN existing VIP", "10.16.0.31", "ens18", "ens18", "ens18", "ens18", false},
		{"Tailscale node with LAN VIP parent", "100.80.0.1", "ens18", "tailscale0", "ens18", "tailscale0", false},
		{"custom Ethernet parent", "10.16.0.31", "ens18", "lan2", "lan2", "ens18", false},
		{"repeat preserves isolated parent", "10.16.0.31", "ens18", "foundry-vip", "lan2", "ens18", false},
		{"ambiguous address", "", "ens18", "ens18", "", "", true},
		{"no Ethernet parent", "100.80.0.1", "tailscale0", "tailscale0", "", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var links []isolationLink
			require.NoError(t, json.Unmarshal([]byte(inventory), &links))
			parent, flannel, ip, err := selectIsolationInterfaces(links, tt.ip, tt.route, tt.old)
			if tt.fail {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.parent, parent)
			require.Equal(t, tt.flannel, flannel)
			require.Equal(t, tt.ip, ip)
		})
	}
}
func TestIsolatedVIPEnvPreservesConfiguration(t *testing.T) {
	before := []corev1.EnvVar{{Name: "vip_interface", Value: "tailscale0"}, {Name: "vip_servicesinterface", Value: "eth0"}, {Name: "vip_address", Value: "100.80.0.100"}, {Name: "vip_arp", Value: "true"}, {Name: "KUBECONFIG", Value: "/etc/rancher/k3s/k3s.yaml"}}
	after := isolatedVIPEnv(before)
	require.Equal(t, "tailscale0", before[0].Value)
	require.Equal(t, VIPInterfaceName, envValue(after, "vip_interface"))
	require.Equal(t, VIPInterfaceName, envValue(after, "vip_servicesinterface"))
	require.Equal(t, before[2:], after[2:])
	require.Equal(t, after, isolatedVIPEnv(after))
	require.Len(t, isolatedVIPEnv(nil), 2)
}

func TestValidateIsolationOverrides(t *testing.T) {
	p := isolationPlan{IP: "100.80.0.1", Flannel: "tailscale0"}
	for _, tt := range []struct {
		name      string
		inventory isolationOverrides
		failure   string
	}{
		{"matching config", isolationOverrides{Files: map[string]string{"config.yaml": "node-ip: 100.80.0.1\nflannel-iface: tailscale0\nflannel-backend: vxlan\nflannel-external-ip: false\n"}}, ""},
		{"matching single address list", isolationOverrides{Files: map[string]string{"config.yaml": "node-ip: [100.80.0.1]"}}, ""},
		{"matching args", isolationOverrides{Args: []string{"--node-ip=100.80.0.1", "--flannel-iface=tailscale0"}}, ""},
		{"matching environment", isolationOverrides{Env: map[string]string{"K3S_NODE_IP": "100.80.0.1", "K3S_FLANNEL_IFACE": "tailscale0"}}, ""},
		{"wrong interface", isolationOverrides{Args: []string{"--flannel-iface=ens18"}}, "conflicts"},
		{"wrong IP", isolationOverrides{Env: map[string]string{"K3S_NODE_IP": "10.0.0.1"}}, "conflicts"},
		{"alternate backend", isolationOverrides{Args: []string{"--flannel-backend=wireguard-native"}}, "VXLAN"},
		{"external IP mode", isolationOverrides{Args: []string{"--flannel-external-ip=true"}}, "external-ip"},
		{"custom config arg", isolationOverrides{Args: []string{"--config=/tmp/other.yaml"}}, "custom"},
		{"custom config short arg", isolationOverrides{Args: []string{"-c=/tmp/other.yaml"}}, "custom"},
		{"custom config env", isolationOverrides{Env: map[string]string{"K3S_CONFIG_FILE": "/tmp/other.yaml"}}, "custom"},
		{"malformed secret YAML", isolationOverrides{Files: map[string]string{"config.yaml": "token: [DO-NOT-PRINT"}}, "cannot parse"},
		{"managed config can be replaced", isolationOverrides{Files: map[string]string{isolationConfigPath: "node-ip: 10.0.0.1"}}, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validateIsolationOverrides(tt.inventory, p)
			if tt.failure == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.failure)
				require.NotContains(t, err.Error(), "DO-NOT-PRINT")
			}
		})
	}
}
