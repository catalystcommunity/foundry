package helm

import "time"

// Release represents a Helm release
type Release struct {
	Name      string
	Namespace string
	Version   int
	Status    string
	Chart     string
	AppVersion string
	Updated   time.Time
}

// Chart represents a Helm chart
type Chart struct {
	Name       string
	Version    string
	Repository string
}

// InstallOptions contains options for installing a Helm chart
type InstallOptions struct {
	ReleaseName     string
	Namespace       string
	Chart           string
	Version         string
	Values          map[string]interface{}
	CreateNamespace bool
	Wait            bool
	Timeout         time.Duration
}

// UpgradeOptions contains options for upgrading a Helm release
type UpgradeOptions struct {
	ReleaseName string
	Namespace   string
	Chart       string
	Version     string
	Values      map[string]interface{}
	Install     bool // Install if not already installed
	Wait        bool
	Timeout     time.Duration
}

// UninstallOptions contains options for uninstalling a Helm release
type UninstallOptions struct {
	ReleaseName string
	Namespace   string
	Wait        bool
	Timeout     time.Duration
}

// RepoAddOptions contains options for adding a Helm repository
type RepoAddOptions struct {
	Name     string
	URL      string
	Username string
	Password string
	ForceUpdate bool
}
