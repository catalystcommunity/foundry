package tailscale

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// mockKubernetesClientExtended extends mockKubernetesClient with CoreDNS methods
type mockKubernetesClientExtended struct {
	mockKubernetesClient // Embed the existing mock from crds_test.go

	// Service IP lookup
	serviceIP    string
	serviceIPErr error

	// ConfigMap operations
	configMap       *ConfigMap
	getConfigMapErr error
	updateConfigMapErr error
	configMapsUpdated []*ConfigMap
}

func (m *mockKubernetesClientExtended) GetServiceIP(ctx context.Context, namespace, name string) (string, error) {
	if m.serviceIPErr != nil {
		return "", m.serviceIPErr
	}
	return m.serviceIP, nil
}

func (m *mockKubernetesClientExtended) GetConfigMap(ctx context.Context, namespace, name string) (*ConfigMap, error) {
	if m.getConfigMapErr != nil {
		return nil, m.getConfigMapErr
	}
	// Return a copy to avoid mutation issues
	if m.configMap != nil {
		return &ConfigMap{
			Name:      m.configMap.Name,
			Namespace: m.configMap.Namespace,
			Data:      copyMap(m.configMap.Data),
		}, nil
	}
	return nil, fmt.Errorf("ConfigMap not found")
}

func (m *mockKubernetesClientExtended) UpdateConfigMap(ctx context.Context, cm *ConfigMap) error {
	if m.updateConfigMapErr != nil {
		return m.updateConfigMapErr
	}
	// Store a copy of the updated ConfigMap
	m.configMapsUpdated = append(m.configMapsUpdated, &ConfigMap{
		Name:      cm.Name,
		Namespace: cm.Namespace,
		Data:      copyMap(cm.Data),
	})
	return nil
}

func copyMap(m map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range m {
		result[k] = v
	}
	return result
}

func TestNewCoreDNSPatcher(t *testing.T) {
	tests := []struct {
		name    string
		client  KubernetesClient
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil client",
			client:  nil,
			wantErr: true,
			errMsg:  "kubernetes client cannot be nil",
		},
		{
			name:    "valid client",
			client:  &mockKubernetesClientExtended{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patcher, err := NewCoreDNSPatcher(tt.client)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCoreDNSPatcher() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("NewCoreDNSPatcher() error message = %q, want %q", err.Error(), tt.errMsg)
				return
			}
			if !tt.wantErr && patcher == nil {
				t.Error("NewCoreDNSPatcher() returned nil patcher without error")
			}
		})
	}
}

func TestCoreDNSPatcher_GenerateTailscaleBlock(t *testing.T) {
	tests := []struct {
		name    string
		dnsIP   string
		wantContains []string
	}{
		{
			name:  "IPv4 address",
			dnsIP: "10.96.0.100",
			wantContains: []string{
				"ts.net:53 {",
				"errors",
				"cache 30",
				"forward . 10.96.0.100",
				"}",
			},
		},
		{
			name:  "different IPv4",
			dnsIP: "192.168.1.53",
			wantContains: []string{
				"forward . 192.168.1.53",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockKubernetesClientExtended{}
			patcher, err := NewCoreDNSPatcher(client)
			if err != nil {
				t.Fatalf("NewCoreDNSPatcher() unexpected error: %v", err)
			}

			block := patcher.generateTailscaleBlock(tt.dnsIP)

			for _, want := range tt.wantContains {
				if !strings.Contains(block, want) {
					t.Errorf("generateTailscaleBlock() missing %q in output:\n%s", want, block)
				}
			}
		})
	}
}

func TestCoreDNSPatcher_PatchCorefile(t *testing.T) {
	tests := []struct {
		name         string
		corefile     string
		dnsIP        string
		wantChanged  bool
		wantContains []string
		wantErr      bool
	}{
		{
			name: "clean corefile",
			corefile: `.:53 {
    errors
    health
    ready
    kubernetes cluster.local in-addr.arpa ip6.arpa {
       pods insecure
       fallthrough in-addr.arpa ip6.arpa
    }
    forward . /etc/resolv.conf
    cache 30
    loop
    reload
    loadbalance
}`,
			dnsIP:       "10.96.0.100",
			wantChanged: true,
			wantContains: []string{
				"ts.net:53 {",
				"forward . 10.96.0.100",
				".:53 {",
			},
		},
		{
			name: "already patched",
			corefile: `ts.net:53 {
    errors
    cache 30
    forward . 10.96.0.100
}

.:53 {
    errors
    health
}`,
			dnsIP:       "10.96.0.100",
			wantChanged: false,
		},
		{
			name:         "empty corefile",
			corefile:     "",
			dnsIP:        "10.96.0.100",
			wantChanged:  true,
			wantContains: []string{"ts.net:53 {"},
		},
		{
			name: "corefile with comments",
			corefile: `# CoreDNS configuration
.:53 {
    errors
    health
}`,
			dnsIP:       "10.96.0.100",
			wantChanged: true,
			wantContains: []string{
				"ts.net:53 {",
				"# CoreDNS configuration",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockKubernetesClientExtended{}
			patcher, err := NewCoreDNSPatcher(client)
			if err != nil {
				t.Fatalf("NewCoreDNSPatcher() unexpected error: %v", err)
			}

			patched, changed, err := patcher.patchCorefile(tt.corefile, tt.dnsIP)
			if (err != nil) != tt.wantErr {
				t.Errorf("patchCorefile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if changed != tt.wantChanged {
				t.Errorf("patchCorefile() changed = %v, want %v", changed, tt.wantChanged)
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(patched, want) {
					t.Errorf("patchCorefile() missing %q in output:\n%s", want, patched)
				}
			}

			// Verify original content preserved (if changed)
			if tt.wantChanged && tt.corefile != "" {
				if !strings.Contains(patched, tt.corefile) {
					t.Error("patchCorefile() did not preserve original Corefile content")
				}
			}
		})
	}
}

func TestCoreDNSPatcher_GetTailscaleDNSIP(t *testing.T) {
	tests := []struct {
		name      string
		serviceIP string
		err       error
		wantIP    string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "successful lookup",
			serviceIP: "10.96.0.100",
			wantIP:    "10.96.0.100",
			wantErr:   false,
		},
		{
			name:      "service not found",
			serviceIP: "",
			err:       fmt.Errorf("service not found"),
			wantErr:   true,
			errMsg:    "failed to get service IP: service not found",
		},
		{
			name:      "empty IP",
			serviceIP: "",
			wantErr:   true,
			errMsg:    "Tailscale DNS service IP is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockKubernetesClientExtended{
				serviceIP:    tt.serviceIP,
				serviceIPErr: tt.err,
			}
			patcher, err := NewCoreDNSPatcher(client)
			if err != nil {
				t.Fatalf("NewCoreDNSPatcher() unexpected error: %v", err)
			}

			ip, err := patcher.getTailscaleDNSIP(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("getTailscaleDNSIP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errMsg != "" && err.Error() != tt.errMsg {
				t.Errorf("getTailscaleDNSIP() error message = %q, want %q", err.Error(), tt.errMsg)
				return
			}
			if !tt.wantErr && ip != tt.wantIP {
				t.Errorf("getTailscaleDNSIP() = %q, want %q", ip, tt.wantIP)
			}
		})
	}
}

func TestCoreDNSPatcher_PatchCoreDNS(t *testing.T) {
	tests := []struct {
		name               string
		serviceIP          string
		serviceIPErr       error
		configMap          *ConfigMap
		getConfigMapErr    error
		updateConfigMapErr error
		wantErr            bool
		wantUpdateCalled   bool
		errContains        string
	}{
		{
			name:      "successful patch",
			serviceIP: "10.96.0.100",
			configMap: &ConfigMap{
				Name:      "coredns",
				Namespace: "kube-system",
				Data: map[string]string{
					"Corefile": `.:53 {
    errors
    health
}`,
				},
			},
			wantErr:          false,
			wantUpdateCalled: true,
		},
		{
			name:      "already patched - no update",
			serviceIP: "10.96.0.100",
			configMap: &ConfigMap{
				Name:      "coredns",
				Namespace: "kube-system",
				Data: map[string]string{
					"Corefile": `ts.net:53 {
    errors
    cache 30
    forward . 10.96.0.100
}

.:53 {
    errors
}`,
				},
			},
			wantErr:          false,
			wantUpdateCalled: false, // Should not update if already patched
		},
		{
			name:         "service IP lookup error",
			serviceIP:    "",
			serviceIPErr: fmt.Errorf("service not found"),
			wantErr:      true,
			errContains:  "failed to get Tailscale DNS service IP",
		},
		{
			name:            "configmap get error",
			serviceIP:       "10.96.0.100",
			getConfigMapErr: fmt.Errorf("ConfigMap not found"),
			wantErr:         true,
			errContains:     "failed to get CoreDNS ConfigMap",
		},
		{
			name:      "missing Corefile key",
			serviceIP: "10.96.0.100",
			configMap: &ConfigMap{
				Name:      "coredns",
				Namespace: "kube-system",
				Data:      map[string]string{}, // No Corefile key
			},
			wantErr:     true,
			errContains: "Corefile key not found",
		},
		{
			name:      "update error",
			serviceIP: "10.96.0.100",
			configMap: &ConfigMap{
				Name:      "coredns",
				Namespace: "kube-system",
				Data: map[string]string{
					"Corefile": `.:53 {
    errors
}`,
				},
			},
			updateConfigMapErr: fmt.Errorf("update failed"),
			wantErr:            true,
			errContains:        "failed to update CoreDNS ConfigMap",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockKubernetesClientExtended{
				serviceIP:          tt.serviceIP,
				serviceIPErr:       tt.serviceIPErr,
				configMap:          tt.configMap,
				getConfigMapErr:    tt.getConfigMapErr,
				updateConfigMapErr: tt.updateConfigMapErr,
				configMapsUpdated:  []*ConfigMap{},
			}
			patcher, err := NewCoreDNSPatcher(client)
			if err != nil {
				t.Fatalf("NewCoreDNSPatcher() unexpected error: %v", err)
			}

			err = patcher.PatchCoreDNS(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("PatchCoreDNS() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
				t.Errorf("PatchCoreDNS() error = %q, want to contain %q", err.Error(), tt.errContains)
				return
			}

			// Check if update was called as expected
			updateCalled := len(client.configMapsUpdated) > 0
			if updateCalled != tt.wantUpdateCalled {
				t.Errorf("PatchCoreDNS() update called = %v, want %v", updateCalled, tt.wantUpdateCalled)
			}

			// Verify patched content if update was called
			if tt.wantUpdateCalled && len(client.configMapsUpdated) > 0 {
				updatedCM := client.configMapsUpdated[0]
				corefile := updatedCM.Data["Corefile"]
				if !strings.Contains(corefile, "ts.net:53") {
					t.Error("PatchCoreDNS() updated ConfigMap missing ts.net:53 block")
				}
				if !strings.Contains(corefile, tt.serviceIP) {
					t.Errorf("PatchCoreDNS() updated ConfigMap missing service IP %s", tt.serviceIP)
				}
			}
		})
	}
}

func TestCoreDNSPatcher_PatchCoreDNS_Idempotency(t *testing.T) {
	// Test that patching twice results in the same Corefile
	serviceIP := "10.96.0.100"
	originalCorefile := `.:53 {
    errors
    health
    kubernetes cluster.local
    forward . /etc/resolv.conf
}`

	client := &mockKubernetesClientExtended{
		serviceIP: serviceIP,
		configMap: &ConfigMap{
			Name:      "coredns",
			Namespace: "kube-system",
			Data: map[string]string{
				"Corefile": originalCorefile,
			},
		},
		configMapsUpdated: []*ConfigMap{},
	}

	patcher, err := NewCoreDNSPatcher(client)
	if err != nil {
		t.Fatalf("NewCoreDNSPatcher() unexpected error: %v", err)
	}

	// First patch
	if err := patcher.PatchCoreDNS(context.Background()); err != nil {
		t.Fatalf("First PatchCoreDNS() unexpected error: %v", err)
	}

	if len(client.configMapsUpdated) != 1 {
		t.Fatalf("Expected 1 update after first patch, got %d", len(client.configMapsUpdated))
	}

	firstPatchedCorefile := client.configMapsUpdated[0].Data["Corefile"]

	// Update mock with patched Corefile for second patch
	client.configMap.Data["Corefile"] = firstPatchedCorefile
	client.configMapsUpdated = []*ConfigMap{} // Reset

	// Second patch (should be no-op)
	if err := patcher.PatchCoreDNS(context.Background()); err != nil {
		t.Fatalf("Second PatchCoreDNS() unexpected error: %v", err)
	}

	if len(client.configMapsUpdated) != 0 {
		t.Errorf("Expected 0 updates after second patch (idempotent), got %d", len(client.configMapsUpdated))
	}

	// Verify ts.net block appears only once
	count := strings.Count(firstPatchedCorefile, "ts.net:53")
	if count != 1 {
		t.Errorf("Expected ts.net:53 to appear once, found %d times", count)
	}
}

func TestParseCorefile(t *testing.T) {
	tests := []struct {
		name       string
		corefile   string
		wantBlocks int
		wantContains []string
	}{
		{
			name: "single block",
			corefile: `.:53 {
    errors
    health
}`,
			wantBlocks: 1,
			wantContains: []string{".:53"},
		},
		{
			name: "multiple blocks",
			corefile: `ts.net:53 {
    forward . 10.96.0.100
}

.:53 {
    errors
    health
}`,
			wantBlocks: 2,
			wantContains: []string{"ts.net:53", ".:53"},
		},
		{
			name: "complex block with nested braces",
			corefile: `.:53 {
    errors
    kubernetes cluster.local {
       pods insecure
    }
    forward . /etc/resolv.conf
}`,
			wantBlocks: 1,
			wantContains: []string{"kubernetes cluster.local"},
		},
		{
			name:       "empty corefile",
			corefile:   "",
			wantBlocks: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := ParseCorefile(tt.corefile)

			if len(blocks) != tt.wantBlocks {
				t.Errorf("ParseCorefile() returned %d blocks, want %d", len(blocks), tt.wantBlocks)
			}

			combinedBlocks := strings.Join(blocks, " ")
			for _, want := range tt.wantContains {
				if !strings.Contains(combinedBlocks, want) {
					t.Errorf("ParseCorefile() blocks missing %q", want)
				}
			}
		})
	}
}
