package k3s

import (
	"fmt"
	"strconv"
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
	selector := ""
	if cfg.Version != "" && cfg.Version != "latest" {
		selector = fmt.Sprintf("INSTALL_K3S_VERSION=%s ", cfg.Version)
	}
	return generateK3sServerCommand(cfg, selector)
}

// GenerateK3sUpgradeCommand selects a supported server upgrade target.
func GenerateK3sUpgradeCommand(cfg *Config, currentVersion string) (string, error) {
	selector, err := K3sUpgradeSelector(currentVersion, cfg.Version)
	if err != nil {
		return "", err
	}
	return generateK3sServerCommand(cfg, selector), nil
}

// GenerateK3sUpgradeCommands returns each safe server upgrade step. K3s nodes
// must pass through each Kubernetes minor version in order.
func GenerateK3sUpgradeCommands(cfg *Config, currentVersion string) ([]string, error) {
	selectors, err := K3sUpgradeSelectors(currentVersion, cfg.Version)
	if err != nil {
		return nil, err
	}
	commands := make([]string, 0, len(selectors))
	for _, selector := range selectors {
		commands = append(commands, generateK3sServerCommand(cfg, selector))
	}
	return commands, nil
}

func generateK3sServerCommand(cfg *Config, selector string) string {
	baseCmd := "curl -sfL https://get.k3s.io | " + selector + "sh -s - server"

	// Add flags
	flags := GenerateK3sServerFlags(cfg)
	if len(flags) > 0 {
		baseCmd += " " + strings.Join(flags, " ")
	}

	return baseCmd
}

// K3sUpgradeSelector returns the installer selector for a safe upgrade.
// An omitted target updates only within the current Kubernetes minor release.
func K3sUpgradeSelector(currentVersion, targetVersion string) (string, error) {
	currentMajor, currentMinor, err := k3sMajorMinor(currentVersion)
	if err != nil {
		return "", fmt.Errorf("invalid current K3s version: %w", err)
	}
	if targetVersion == "" || targetVersion == "latest" {
		return fmt.Sprintf("INSTALL_K3S_CHANNEL=v%d.%d ", currentMajor, currentMinor), nil
	}

	targetMajor, targetMinor, err := k3sMajorMinor(targetVersion)
	if err != nil {
		return "", fmt.Errorf("invalid target K3s version: %w", err)
	}
	if targetMajor != currentMajor || targetMinor < currentMinor {
		return "", fmt.Errorf("K3s downgrade from %s to %s is not supported", currentVersion, targetVersion)
	}
	if targetMinor > currentMinor+1 {
		return "", fmt.Errorf("K3s upgrade from %s to %s skips an intermediate minor version", currentVersion, targetVersion)
	}
	return fmt.Sprintf("INSTALL_K3S_VERSION=%s ", targetVersion), nil
}

// K3sUpgradeSelectors returns an ordered upgrade plan. An exact target can be
// more than one minor version ahead. The plan uses a channel for each
// intermediate minor and the exact version for the final step.
func K3sUpgradeSelectors(currentVersion, targetVersion string) ([]string, error) {
	current, err := parseK3sVersion(currentVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid current K3s version: %w", err)
	}
	if targetVersion == "" || targetVersion == "latest" {
		return []string{fmt.Sprintf("INSTALL_K3S_CHANNEL=v%d.%d ", current.major, current.minor)}, nil
	}

	target, err := parseK3sVersion(targetVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid target K3s version: %w", err)
	}
	if target.less(current) {
		return nil, fmt.Errorf("K3s downgrade from %s to %s is not supported", currentVersion, targetVersion)
	}
	if target.equal(current) {
		return nil, nil
	}
	if target.major != current.major {
		return nil, fmt.Errorf("K3s major-version upgrade from %s to %s is not supported", currentVersion, targetVersion)
	}

	selectors := make([]string, 0, target.minor-current.minor)
	for minor := current.minor + 1; minor < target.minor; minor++ {
		selectors = append(selectors, fmt.Sprintf("INSTALL_K3S_CHANNEL=v%d.%d ", current.major, minor))
	}
	selectors = append(selectors, fmt.Sprintf("INSTALL_K3S_VERSION=%s ", targetVersion))
	return selectors, nil
}

type parsedK3sVersion struct {
	major int
	minor int
	patch int
}

func parseK3sVersion(version string) (parsedK3sVersion, error) {
	matches := versionPattern.FindStringSubmatch(version)
	if matches == nil {
		return parsedK3sVersion{}, fmt.Errorf("version %q is invalid", version)
	}
	parts := make([]int, 3)
	for i := range parts {
		value, err := strconv.Atoi(matches[i+1])
		if err != nil {
			return parsedK3sVersion{}, fmt.Errorf("version %q is invalid", version)
		}
		parts[i] = value
	}
	return parsedK3sVersion{major: parts[0], minor: parts[1], patch: parts[2]}, nil
}

func (v parsedK3sVersion) less(other parsedK3sVersion) bool {
	if v.major != other.major {
		return v.major < other.major
	}
	if v.minor != other.minor {
		return v.minor < other.minor
	}
	return v.patch < other.patch
}

func (v parsedK3sVersion) equal(other parsedK3sVersion) bool {
	return v.major == other.major && v.minor == other.minor && v.patch == other.patch
}

// HighestK3sVersion returns the newest version in an installed node set.
func HighestK3sVersion(versions []string) (string, error) {
	if len(versions) == 0 {
		return "", fmt.Errorf("no installed K3s versions were provided")
	}
	highestText := versions[0]
	highest, err := parseK3sVersion(highestText)
	if err != nil {
		return "", err
	}
	for _, version := range versions[1:] {
		parsed, err := parseK3sVersion(version)
		if err != nil {
			return "", err
		}
		if highest.less(parsed) {
			highest = parsed
			highestText = version
		}
	}
	return highestText, nil
}

func k3sMajorMinor(version string) (int, int, error) {
	parsed, err := parseK3sVersion(version)
	if err != nil {
		return 0, 0, err
	}
	return parsed.major, parsed.minor, nil
}

// K3sVersionsShareMinor reports whether two K3s versions use the same
// Kubernetes major and minor version.
func K3sVersionsShareMinor(first, second string) (bool, error) {
	firstMajor, firstMinor, err := k3sMajorMinor(first)
	if err != nil {
		return false, err
	}
	secondMajor, secondMinor, err := k3sMajorMinor(second)
	if err != nil {
		return false, err
	}
	return firstMajor == secondMajor && firstMinor == secondMinor, nil
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
