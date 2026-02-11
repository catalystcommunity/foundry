package tailscale

import (
	"context"
	"fmt"
	"strings"
)

const (
	// CoreDNS configuration constants
	CoreDNSNamespace    = "kube-system"
	CoreDNSConfigMap    = "coredns"
	CoreDNSConfigKey    = "Corefile"
	TailscaleDNSService = "ts-dns"
)

// CoreDNSPatcher handles CoreDNS configuration patching for Tailscale DNS.
type CoreDNSPatcher struct {
	client KubernetesClient
}

// NewCoreDNSPatcher creates a new CoreDNS patcher.
func NewCoreDNSPatcher(client KubernetesClient) (*CoreDNSPatcher, error) {
	if client == nil {
		return nil, fmt.Errorf("kubernetes client cannot be nil")
	}

	return &CoreDNSPatcher{
		client: client,
	}, nil
}

// PatchCoreDNS patches the CoreDNS ConfigMap to forward .ts.net queries to Tailscale DNS.
func (p *CoreDNSPatcher) PatchCoreDNS(ctx context.Context) error {
	// Get Tailscale DNS service IP
	dnsIP, err := p.getTailscaleDNSIP(ctx)
	if err != nil {
		return fmt.Errorf("failed to get Tailscale DNS service IP: %w", err)
	}

	// Get CoreDNS ConfigMap
	configMap, err := p.client.GetConfigMap(ctx, CoreDNSNamespace, CoreDNSConfigMap)
	if err != nil {
		return fmt.Errorf("failed to get CoreDNS ConfigMap: %w", err)
	}

	// Get Corefile content
	corefile, ok := configMap.Data[CoreDNSConfigKey]
	if !ok {
		return fmt.Errorf("Corefile key not found in ConfigMap")
	}

	// Patch Corefile
	patchedCorefile, changed, err := p.patchCorefile(corefile, dnsIP)
	if err != nil {
		return fmt.Errorf("failed to patch Corefile: %w", err)
	}

	// Only update if changes were made
	if !changed {
		return nil // Already patched, no-op
	}

	// Update ConfigMap
	configMap.Data[CoreDNSConfigKey] = patchedCorefile
	if err := p.client.UpdateConfigMap(ctx, configMap); err != nil {
		return fmt.Errorf("failed to update CoreDNS ConfigMap: %w", err)
	}

	return nil
}

// getTailscaleDNSIP retrieves the Tailscale DNS service IP.
func (p *CoreDNSPatcher) getTailscaleDNSIP(ctx context.Context) (string, error) {
	ip, err := p.client.GetServiceIP(ctx, DefaultNamespace, TailscaleDNSService)
	if err != nil {
		return "", fmt.Errorf("failed to get service IP: %w", err)
	}

	if ip == "" {
		return "", fmt.Errorf("Tailscale DNS service IP is empty")
	}

	return ip, nil
}

// patchCorefile adds the Tailscale DNS forwarding block to the Corefile.
// Returns the patched Corefile, whether changes were made, and any error.
func (p *CoreDNSPatcher) patchCorefile(corefile, dnsIP string) (string, bool, error) {
	// Check if already patched
	if strings.Contains(corefile, "ts.net:53") {
		return corefile, false, nil // Already patched
	}

	// Generate Tailscale block
	tsBlock := p.generateTailscaleBlock(dnsIP)

	// Insert block at the beginning (before other blocks)
	patchedCorefile := tsBlock + "\n" + corefile

	return patchedCorefile, true, nil
}

// generateTailscaleBlock creates the Tailscale DNS forwarding configuration block.
func (p *CoreDNSPatcher) generateTailscaleBlock(dnsIP string) string {
	return fmt.Sprintf(`ts.net:53 {
    errors
    cache 30
    forward . %s
}`, dnsIP)
}

// ParseCorefile parses a Corefile into individual server blocks.
// This is a helper function for more advanced parsing if needed in the future.
func ParseCorefile(corefile string) []string {
	var blocks []string
	var currentBlock strings.Builder
	inBlock := false
	braceDepth := 0

	lines := strings.Split(corefile, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Count braces to track block depth
		openBraces := strings.Count(line, "{")
		closeBraces := strings.Count(line, "}")
		braceDepth += openBraces - closeBraces

		// Start of a block (line with port like ".:53" or "ts.net:53")
		if !inBlock && strings.Contains(trimmed, ":") && strings.Contains(trimmed, "{") {
			inBlock = true
		}

		if inBlock {
			currentBlock.WriteString(line)
			currentBlock.WriteString("\n")

			// End of block when braces balance
			if braceDepth == 0 {
				blocks = append(blocks, strings.TrimSpace(currentBlock.String()))
				currentBlock.Reset()
				inBlock = false
			}
		} else if trimmed != "" {
			// Standalone lines outside blocks (comments, etc.)
			currentBlock.WriteString(line)
			currentBlock.WriteString("\n")
		}
	}

	// Capture any remaining content
	if currentBlock.Len() > 0 {
		blocks = append(blocks, strings.TrimSpace(currentBlock.String()))
	}

	return blocks
}
