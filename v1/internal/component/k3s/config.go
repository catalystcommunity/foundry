package k3s

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// KubeconfigPath is the default location of the K3s kubeconfig
	KubeconfigPath = "/etc/rancher/k3s/k3s.yaml"

	// RegistriesConfigPath is the location of the registries.yaml file
	RegistriesConfigPath = "/etc/rancher/k3s/registries.yaml"

	// OpenBAO paths for K3s configuration (relative to mount point)
	KubeconfigOpenBAOPath = "k3s/kubeconfig"
)

// Registry types (RegistryConfig, RegistryMirror, RegistryAuth, etc.) are generated from CSIL in types.gen.go

// GenerateRegistriesYAML generates the registries.yaml content for K3s
// This configures K3s to use Zot as a pull-through cache for container registries,
// and merges any additional user-defined registry entries.
func GenerateRegistriesYAML(zotURL string, insecure bool, additional []AdditionalRegistry) string {
	rc := RegistryConfig{
		Mirrors: RegistryMirrorMap{
			"docker.io": RegistryMirror{Endpoint: []string{zotURL}},
			"ghcr.io":   RegistryMirror{Endpoint: []string{zotURL}},
		},
		Configs: RegistryAuthMap{},
	}

	// Add zot TLS config
	if insecure {
		insecureTrue := true
		rc.Configs[zotURL] = RegistryAuth{
			Tls: &RegistryTLSConfig{InsecureSkipVerify: &insecureTrue},
		}
	}

	// Merge additional registries
	for _, reg := range additional {
		endpoint := reg.Name
		if reg.Endpoint != nil {
			endpoint = *reg.Endpoint
		}

		// Apply http:// scheme if requested
		if reg.HTTP != nil && *reg.HTTP && !strings.HasPrefix(endpoint, "http") {
			endpoint = "http://" + endpoint
		}

		// Add mirror entry: registry name -> endpoint
		rc.Mirrors[reg.Name] = RegistryMirror{
			Endpoint: []string{endpoint},
		}

		// Build config entry for the endpoint.
		// Auth and TLS are independent — a registry can have both credentials
		// and insecure_skip_verify at the same time.
		auth := RegistryAuth{}
		needsConfig := false

		if reg.Insecure != nil && *reg.Insecure {
			insecureTrue := true
			auth.Tls = &RegistryTLSConfig{InsecureSkipVerify: &insecureTrue}
			needsConfig = true
		}

		if reg.Username != nil && reg.Password != nil {
			auth.Auth = &RegistryAuthConfig{
				Username: reg.Username,
				Password: reg.Password,
			}
			needsConfig = true
		}

		if needsConfig {
			rc.Configs[endpoint] = auth
		}
	}

	out, _ := yaml.Marshal(rc)
	return string(out)
}

// GenerateRegistriesConfig generates registries.yaml content for Zot registry
// This is a convenience wrapper around GenerateRegistriesYAML that assumes
// insecure connections (common for local development)
func GenerateRegistriesConfig(zotAddr string, additional []AdditionalRegistry) string {
	// Format the Zot address as a URL with port
	zotURL := fmt.Sprintf("http://%s:5000", zotAddr)
	return GenerateRegistriesYAML(zotURL, true, additional)
}

// GenerateK3sServerFlags generates the command-line flags for K3s server installation
func GenerateK3sServerFlags(cfg *Config) []string {
	flags := []string{}

	// Cluster initialization (for first control plane node in HA setup)
	if cfg.ClusterInit {
		flags = append(flags, "--cluster-init")
	}

	// Server URL (for joining additional control plane nodes)
	if cfg.ServerURL != "" {
		flags = append(flags, fmt.Sprintf("--server %s", cfg.ServerURL))
	}

	// Cluster token
	if cfg.ClusterToken != "" {
		flags = append(flags, fmt.Sprintf("--token %s", cfg.ClusterToken))
	}

	// Agent token (for workers to join)
	if cfg.AgentToken != "" {
		flags = append(flags, fmt.Sprintf("--agent-token %s", cfg.AgentToken))
	}

	// TLS SANs
	if len(cfg.TLSSANs) > 0 {
		for _, san := range cfg.TLSSANs {
			flags = append(flags, fmt.Sprintf("--tls-san %s", san))
		}
	}

	// Always add the VIP as a TLS SAN
	flags = append(flags, fmt.Sprintf("--tls-san %s", cfg.VIP))

	// Disable components
	if len(cfg.DisableComponents) > 0 {
		for _, component := range cfg.DisableComponents {
			flags = append(flags, fmt.Sprintf("--disable=%s", component))
		}
	}

	// Etcd args (for tuning etcd performance in virtualized environments)
	// These are passed as --etcd-arg=<arg> to K3s
	if len(cfg.EtcdArgs) > 0 {
		for _, arg := range cfg.EtcdArgs {
			flags = append(flags, fmt.Sprintf("--etcd-arg=%s", arg))
		}
	}

	return flags
}

// GenerateK3sInstallCommand generates the full K3s installation command
func GenerateK3sInstallCommand(cfg *Config) string {
	// Base installation command
	baseCmd := "curl -sfL https://get.k3s.io | sh -s - server"

	// Add flags
	flags := GenerateK3sServerFlags(cfg)
	if len(flags) > 0 {
		baseCmd += " " + strings.Join(flags, " ")
	}

	return baseCmd
}

// GenerateResolvConfContent generates the resolv.conf content with custom DNS servers
func GenerateResolvConfContent(dnsServers []string, searchDomains []string) string {
	var lines []string

	// Add search domains
	if len(searchDomains) > 0 {
		lines = append(lines, "search "+strings.Join(searchDomains, " "))
	}

	// Add nameservers
	for _, dns := range dnsServers {
		lines = append(lines, "nameserver "+dns)
	}

	return strings.Join(lines, "\n") + "\n"
}

// GenerateSystemdResolvdConfig generates systemd-resolved configuration
// This is used to configure DNS on systems using systemd-resolved
func GenerateSystemdResolvdConfig(dnsServers []string, searchDomains []string) string {
	var lines []string

	lines = append(lines, "[Resolve]")

	// Add DNS servers
	if len(dnsServers) > 0 {
		lines = append(lines, "DNS="+strings.Join(dnsServers, " "))
	}

	// Add search domains
	if len(searchDomains) > 0 {
		lines = append(lines, "Domains="+strings.Join(searchDomains, " "))
	}

	return strings.Join(lines, "\n") + "\n"
}
