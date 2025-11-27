package zot

import (
	"encoding/json"
	"fmt"
)

// ZotConfig represents the Zot registry configuration file structure
type ZotConfig struct {
	DistSpecVersion string              `json:"distSpecVersion"`
	Storage         StorageConfiguration `json:"storage"`
	HTTP            HTTPConfiguration    `json:"http"`
	Log             LogConfiguration     `json:"log"`
	Extensions      *Extensions          `json:"extensions,omitempty"`
}

// StorageConfiguration represents storage settings
type StorageConfiguration struct {
	RootDirectory string                `json:"rootDirectory"`
	GC            bool                  `json:"gc"`
	Dedupe        bool                  `json:"dedupe"`
	SubPaths      map[string]SubPath    `json:"subPaths,omitempty"`
}

// SubPath represents a storage subpath configuration
type SubPath struct {
	RootDirectory string `json:"rootDirectory"`
	GC            bool   `json:"gc"`
	Dedupe        bool   `json:"dedupe"`
}

// HTTPConfiguration represents HTTP server settings
type HTTPConfiguration struct {
	Address string       `json:"address"`
	Port    string       `json:"port"`
	TLS     *TLSConfig   `json:"tls,omitempty"`
	Auth    *AuthMethod  `json:"auth,omitempty"`
}

// TLSConfig represents TLS settings
type TLSConfig struct {
	Cert   string `json:"cert"`
	Key    string `json:"key"`
	CACert string `json:"cacert,omitempty"`
}

// AuthMethod represents authentication settings
type AuthMethod struct {
	HTPasswd HTPasswdConfig    `json:"htpasswd,omitempty"`
	LDAP     *LDAPConfig       `json:"ldap,omitempty"`
	Bearer   *BearerConfig     `json:"bearer,omitempty"`
}

// HTPasswdConfig represents htpasswd authentication
type HTPasswdConfig struct {
	Path string `json:"path"`
}

// LDAPConfig represents LDAP authentication
type LDAPConfig struct {
	Address      string `json:"address"`
	Port         int    `json:"port"`
	BaseDN       string `json:"baseDN"`
	UserAttribute string `json:"userAttribute"`
}

// BearerConfig represents bearer token authentication (OIDC)
type BearerConfig struct {
	Realm   string `json:"realm"`
	Service string `json:"service"`
	Cert    string `json:"cert"`
}

// Extensions represents optional Zot extensions
type Extensions struct {
	Sync   *SyncExtension   `json:"sync,omitempty"`
	Search *SearchExtension `json:"search,omitempty"`
	UI     *UIExtension     `json:"ui,omitempty"`
}

// SyncExtension enables pull-through cache
type SyncExtension struct {
	Enable     bool              `json:"enable"`
	Registries []RegistryConfig  `json:"registries"`
}

// RegistryConfig represents a registry for pull-through caching
type RegistryConfig struct {
	URLs         []string          `json:"urls"`
	PollInterval string            `json:"pollInterval,omitempty"`
	TLSVerify    bool              `json:"tlsVerify"`
	CertDir      string            `json:"certDir,omitempty"`
	OnDemand     bool              `json:"onDemand"`
	Content      []ContentConfig   `json:"content,omitempty"`
}

// ContentConfig represents content filtering for sync
type ContentConfig struct {
	Prefix      string   `json:"prefix"`
	Tags        *Tags    `json:"tags,omitempty"`
	Destination string   `json:"destination,omitempty"`
}

// Tags represents tag filtering
type Tags struct {
	Regex  string `json:"regex,omitempty"`
	Semver bool   `json:"semver,omitempty"`
}

// SearchExtension enables search API
type SearchExtension struct {
	Enable bool `json:"enable"`
	CVE    *CVE `json:"cve,omitempty"`
}

// CVE represents CVE scanning configuration
type CVE struct {
	UpdateInterval string `json:"updateInterval,omitempty"`
}

// UIExtension enables web UI
type UIExtension struct {
	Enable bool `json:"enable"`
}

// LogConfiguration represents logging settings
type LogConfiguration struct {
	Level  string `json:"level"`
	Output string `json:"output,omitempty"`
	Audit  string `json:"audit,omitempty"`
}

// GenerateConfig generates a Zot configuration based on the provided Config
func GenerateConfig(cfg *Config) (string, error) {
	zotConfig := &ZotConfig{
		DistSpecVersion: "1.1.0",
		Storage: StorageConfiguration{
			RootDirectory: cfg.DataDir,
			GC:            true,
			Dedupe:        true,
		},
		HTTP: HTTPConfiguration{
			Address: "0.0.0.0",
			Port:    fmt.Sprintf("%d", cfg.Port),
		},
		Log: LogConfiguration{
			Level:  "info",
			Output: "/dev/stdout",
		},
	}

	// Add pull-through cache for registries if enabled
	if cfg.PullThroughCache {
		zotConfig.Extensions = &Extensions{
			Sync: &SyncExtension{
				Enable: true,
				Registries: []RegistryConfig{
					{
						URLs:      []string{"https://registry-1.docker.io"},
						TLSVerify: true,
						OnDemand:  true,
					},
					{
						URLs:      []string{"https://ghcr.io"},
						TLSVerify: true,
						OnDemand:  true,
					},
				},
			},
			Search: &SearchExtension{
				Enable: true,
			},
			UI: &UIExtension{
				Enable: true,
			},
		}
	}

	// Add authentication if configured
	if cfg.Auth != nil {
		switch cfg.Auth.Type {
		case "basic":
			if htpasswdPath, ok := cfg.Auth.Config["htpasswd_path"].(string); ok {
				zotConfig.HTTP.Auth = &AuthMethod{
					HTPasswd: HTPasswdConfig{
						Path: htpasswdPath,
					},
				}
			}
		case "ldap":
			// LDAP configuration for future use
			// Would parse from cfg.Auth.Config
		case "oidc":
			// OIDC/Bearer configuration for future use
			// Would parse from cfg.Auth.Config
		}
	}

	// Marshal to JSON with indentation
	jsonBytes, err := json.MarshalIndent(zotConfig, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal config to JSON: %w", err)
	}

	return string(jsonBytes), nil
}
