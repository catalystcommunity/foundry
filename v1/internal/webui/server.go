package webui

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/catalystcommunity/foundry/v1/internal/config"
	"github.com/catalystcommunity/foundry/v1/internal/discovery"
	"github.com/catalystcommunity/foundry/v1/internal/host"
	"github.com/catalystcommunity/foundry/v1/internal/setup"
	"github.com/catalystcommunity/foundry/v1/internal/topology"
)

//go:embed assets/*
var assets embed.FS

var errApplyInProgress = errors.New("an apply operation is already active")

const (
	maxRequestBytes = 1 << 20
	sessionLifetime = 8 * time.Hour
	sessionIdleTime = 30 * time.Minute
)

// ApplyFunc applies the configuration at configPath.
type ApplyFunc func(ctx context.Context, configPath string) error

// InspectFunc discovers stack service links and Gateway API exposure.
type InspectFunc func(ctx context.Context, cfg *config.Config) discovery.Snapshot

// Options configures a web UI server.
type Options struct {
	ConfigPath string
	Auth       *AuthStore
	Apply      ApplyFunc
	Inspect    InspectFunc
	Mode       string
}

// Server serves the Foundry UI and API.
type Server struct {
	configPath string
	auth       *AuthStore
	apply      ApplyFunc
	inspect    InspectFunc
	mode       string
	jobsMu     sync.RWMutex
	jobs       map[string]*Job
	applyMu    sync.Mutex
	handler    http.Handler
}

// Job reports one asynchronous apply operation.
type Job struct {
	ID         string    `json:"id"`
	State      string    `json:"state"`
	Message    string    `json:"message"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
}

// WizardConfig is the editable part of a stack configuration. Component
// details remain in YAML and are preserved when the wizard applies changes.
type WizardConfig struct {
	ClusterName     string       `json:"cluster_name"`
	PrimaryDomain   string       `json:"primary_domain"`
	VIP             string       `json:"vip"`
	AllowCGNATVIP   bool         `json:"allow_cgnat_vip"`
	Gateway         string       `json:"gateway"`
	Netmask         string       `json:"netmask"`
	Hosts           []WizardHost `json:"hosts"`
	ManagementHost  string       `json:"management_host,omitempty"`
	ManagementPort  int          `json:"management_port"`
	ManagementImage string       `json:"management_image"`
}

// WizardHost is one host edited by the setup wizard.
type WizardHost struct {
	Hostname string   `json:"hostname"`
	Address  string   `json:"address"`
	Port     int      `json:"port"`
	User     string   `json:"user"`
	Roles    []string `json:"roles"`
	State    string   `json:"state,omitempty"`
}

// Plan describes the configuration changes that apply will make.
type Plan struct {
	Valid    bool           `json:"valid"`
	Summary  []string       `json:"summary"`
	Topology topology.Model `json:"topology"`
}

// New constructs an authenticated web UI server.
func New(options Options) (*Server, error) {
	if options.ConfigPath == "" {
		return nil, fmt.Errorf("config path is required")
	}
	if options.Auth == nil {
		return nil, fmt.Errorf("authentication store is required")
	}
	server := &Server{
		configPath: options.ConfigPath,
		auth:       options.Auth,
		apply:      options.Apply,
		inspect:    options.Inspect,
		mode:       options.Mode,
		jobs:       make(map[string]*Job),
	}
	if server.mode == "" {
		server.mode = "local"
	}
	server.handler = server.routes()
	return server, nil
}

// Handler returns the complete secured HTTP handler.
func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/session", s.handleSession)
	mux.Handle("GET /api/v1/config", s.requireAuth(http.HandlerFunc(s.handleConfig)))
	mux.Handle("GET /api/v1/state", s.requireAuth(http.HandlerFunc(s.handleState)))
	mux.Handle("GET /api/v1/runtime", s.requireAuth(http.HandlerFunc(s.handleRuntime)))
	mux.Handle("GET /api/v1/overview", s.requireAuth(http.HandlerFunc(s.handleOverview)))
	mux.Handle("POST /api/v1/plan", s.requireAuth(http.HandlerFunc(s.handlePlan)))
	mux.Handle("POST /api/v1/apply", s.requireAuth(http.HandlerFunc(s.handleApply)))
	mux.Handle("POST /api/v1/apply/current", s.requireAuth(http.HandlerFunc(s.handleApplyCurrent)))
	mux.Handle("GET /api/v1/jobs/{id}", s.requireAuth(http.HandlerFunc(s.handleJob)))

	content, err := fs.Sub(assets, "assets")
	if err != nil {
		panic(err)
	}
	mux.Handle("/", http.FileServer(http.FS(content)))
	return s.securityHeaders(mux)
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token := bearerToken(r); token != "" {
			if _, ok := s.auth.authenticateToken(token); ok {
				next.ServeHTTP(w, r)
				return
			}
		}

		cookie, err := r.Cookie(sessionCookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if _, ok := s.auth.authenticateSession(cookie.Value, sessionIdleTime); !ok {
			writeError(w, http.StatusUnauthorized, "session expired")
			return
		}
		if changesState(r.Method) && !sameOrigin(r) {
			writeError(w, http.StatusForbidden, "request origin is not allowed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	if !sameOrigin(r) {
		writeError(w, http.StatusForbidden, "request origin is not allowed")
		return
	}
	var request struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		return
	}
	session, ok := s.auth.exchange(request.Token, sessionLifetime)
	if !ok {
		writeError(w, http.StatusUnauthorized, "invalid or expired access token")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookieName, Value: session, Path: "/", HttpOnly: true,
		Secure: r.TLS != nil, SameSite: http.SameSiteStrictMode, MaxAge: int(sessionLifetime.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}

func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request) {
	cfg, err := s.loadConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toWizardConfig(cfg))
}

func (s *Server) handleState(w http.ResponseWriter, _ *http.Request) {
	cfg, err := s.loadConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, topology.Build(cfg))
}

func (s *Server) handleRuntime(w http.ResponseWriter, _ *http.Request) {
	cfg, err := s.loadConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"mode": s.mode, "external_manager_configured": cfg.Management != nil,
	})
}

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s.inspect == nil {
		writeJSON(w, http.StatusOK, discovery.Snapshot{})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, s.inspect(ctx, cfg))
}

func (s *Server) handlePlan(w http.ResponseWriter, r *http.Request) {
	wizard, cfg, err := s.decodeAndMerge(w, r)
	if err != nil {
		return
	}
	writeJSON(w, http.StatusOK, buildPlan(cfg, wizard))
}

func (s *Server) handleApply(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Config  WizardConfig `json:"config"`
		Confirm bool         `json:"confirm"`
	}
	if err := decodeJSON(w, r, &request); err != nil {
		return
	}
	if !request.Confirm {
		writeError(w, http.StatusBadRequest, "apply requires explicit confirmation")
		return
	}
	cfg, err := s.merge(request.Config)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	job, err := s.createApplyJob()
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err := config.Save(cfg, s.configPath); err != nil {
		s.failJob(job.ID, fmt.Errorf("save configuration: %w", err))
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("save configuration: %v", err))
		return
	}
	s.startApply(job.ID)
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) handleApplyCurrent(w http.ResponseWriter, _ *http.Request) {
	job, err := s.queueApply()
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) queueApply() (*Job, error) {
	job, err := s.createApplyJob()
	if err != nil {
		return nil, err
	}
	s.startApply(job.ID)
	return job, nil
}

func (s *Server) createApplyJob() (*Job, error) {
	jobID, err := randomToken()
	if err != nil {
		return nil, err
	}
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	for _, existing := range s.jobs {
		if existing.State == "queued" || existing.State == "running" {
			return nil, errApplyInProgress
		}
	}
	job := &Job{ID: jobID, State: "queued", Message: "Apply is queued", StartedAt: time.Now()}
	s.jobs[jobID] = job
	copy := *job
	return &copy, nil
}

func (s *Server) startApply(jobID string) { go s.runApply(jobID) }

func (s *Server) failJob(jobID string, err error) {
	s.updateJob(jobID, func(job *Job) {
		job.State = "failed"
		job.Message = err.Error()
		job.FinishedAt = time.Now()
	})
}

func (s *Server) runApply(jobID string) {
	s.applyMu.Lock()
	defer s.applyMu.Unlock()
	s.updateJob(jobID, func(job *Job) {
		job.State = "running"
		job.Message = "Applying the stack configuration"
	})

	var err error
	if s.apply != nil {
		err = s.apply(context.Background(), s.configPath)
	}
	s.updateJob(jobID, func(job *Job) {
		job.FinishedAt = time.Now()
		if err != nil {
			job.State = "failed"
			job.Message = err.Error()
			return
		}
		job.State = "complete"
		job.Message = "Apply completed"
	})
}

func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	s.jobsMu.RLock()
	job, ok := s.jobs[r.PathValue("id")]
	if ok {
		copy := *job
		job = &copy
	}
	s.jobsMu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) updateJob(id string, update func(*Job)) {
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	if job := s.jobs[id]; job != nil {
		update(job)
	}
}

func (s *Server) decodeAndMerge(w http.ResponseWriter, r *http.Request) (WizardConfig, *config.Config, error) {
	var wizard WizardConfig
	if err := decodeJSON(w, r, &wizard); err != nil {
		return WizardConfig{}, nil, err
	}
	cfg, err := s.merge(wizard)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return WizardConfig{}, nil, err
	}
	return wizard, cfg, nil
}

func (s *Server) merge(wizard WizardConfig) (*config.Config, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	applyWizardConfig(cfg, wizard)
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration is invalid: %w", err)
	}
	return cfg, nil
}

func (s *Server) loadConfig() (*config.Config, error) {
	cfg, err := config.Load(s.configPath)
	if err == nil {
		return cfg, nil
	}
	if !errors.Is(err, os.ErrNotExist) && !strings.Contains(err.Error(), "config file not found") {
		return nil, err
	}
	return defaultConfig(), nil
}

func defaultConfig() *config.Config {
	return &config.Config{
		Components: config.ComponentMap{
			"openbao": {}, "dns": {}, "zot": {}, "k3s": {},
		},
		SetupState: &setup.SetupState{},
	}
}

func toWizardConfig(cfg *config.Config) WizardConfig {
	wizard := WizardConfig{
		ClusterName: cfg.Cluster.Name, PrimaryDomain: cfg.Cluster.PrimaryDomain, VIP: cfg.Cluster.VIP,
		ManagementPort: 9080, ManagementImage: "ghcr.io/catalystcommunity/foundry",
	}
	if cfg.Cluster.AllowCGNATVIP != nil {
		wizard.AllowCGNATVIP = *cfg.Cluster.AllowCGNATVIP
	}
	if cfg.Network != nil {
		wizard.Gateway, wizard.Netmask = cfg.Network.Gateway, cfg.Network.Netmask
	}
	if cfg.Management != nil {
		wizard.ManagementHost = cfg.Management.Host
		wizard.ManagementPort = int(cfg.Management.Port)
		wizard.ManagementImage = cfg.Management.Image
	}
	for _, configuredHost := range cfg.Hosts {
		if configuredHost == nil {
			continue
		}
		wizard.Hosts = append(wizard.Hosts, WizardHost{
			Hostname: configuredHost.Hostname, Address: configuredHost.Address, Port: configuredHost.Port,
			User: configuredHost.User, Roles: append([]string(nil), configuredHost.Roles...), State: configuredHost.State,
		})
	}
	return wizard
}

func applyWizardConfig(cfg *config.Config, wizard WizardConfig) {
	cfg.Cluster.Name = strings.TrimSpace(wizard.ClusterName)
	cfg.Cluster.PrimaryDomain = strings.TrimSpace(wizard.PrimaryDomain)
	cfg.Cluster.VIP = strings.TrimSpace(wizard.VIP)
	allowCGNAT := wizard.AllowCGNATVIP
	cfg.Cluster.AllowCGNATVIP = &allowCGNAT
	if cfg.Network == nil {
		cfg.Network = &config.NetworkConfig{}
	}
	cfg.Network.Gateway = strings.TrimSpace(wizard.Gateway)
	cfg.Network.Netmask = strings.TrimSpace(wizard.Netmask)

	cfg.Hosts = make([]*host.Host, 0, len(wizard.Hosts))
	for _, wizardHost := range wizard.Hosts {
		port := wizardHost.Port
		if port == 0 {
			port = 22
		}
		user := strings.TrimSpace(wizardHost.User)
		if user == "" {
			user = "root"
		}
		roles := uniqueRoles(wizardHost.Roles)
		if wizard.ManagementHost != "" && wizardHost.Hostname == wizard.ManagementHost {
			roles = uniqueRoles(append(roles, host.RoleManagement))
		} else {
			roles = removeRole(roles, host.RoleManagement)
		}
		cfg.Hosts = append(cfg.Hosts, &host.Host{
			Hostname: strings.TrimSpace(wizardHost.Hostname), Address: strings.TrimSpace(wizardHost.Address),
			Port: port, User: user, Roles: roles, State: wizardHost.State,
		})
	}
	if wizard.ManagementHost == "" {
		cfg.Management = nil
	} else {
		port := wizard.ManagementPort
		if port == 0 {
			port = 9080
		}
		image := strings.TrimSpace(wizard.ManagementImage)
		if image == "" {
			image = "ghcr.io/catalystcommunity/foundry"
		}
		version, dataPath := "latest", "/var/lib/foundry"
		if cfg.Management != nil {
			if cfg.Management.Version != "" {
				version = cfg.Management.Version
			}
			if cfg.Management.DataPath != "" {
				dataPath = cfg.Management.DataPath
			}
		}
		cfg.Management = &config.ManagementConfig{
			Host: wizard.ManagementHost, Port: int64(port), Image: image, Version: version, DataPath: dataPath,
		}
	}
	if cfg.SetupState == nil {
		cfg.SetupState = &setup.SetupState{}
	}
}

func buildPlan(cfg *config.Config, wizard WizardConfig) Plan {
	summary := []string{
		fmt.Sprintf("Configure %d host(s)", len(cfg.Hosts)),
		fmt.Sprintf("Configure Kubernetes VIP %s", cfg.Cluster.VIP),
	}
	if wizard.ManagementHost != "" {
		summary = append([]string{fmt.Sprintf("Install the external manager on %s before the stack", wizard.ManagementHost)}, summary...)
	} else {
		summary = append(summary, "Keep the web interface local and optional")
	}
	return Plan{Valid: true, Summary: summary, Topology: topology.Build(cfg)}
}

func uniqueRoles(roles []string) []string {
	allowed := make(map[string]bool)
	for _, role := range host.ValidRoles() {
		allowed[role] = true
	}
	seen := make(map[string]bool)
	result := make([]string, 0, len(roles))
	for _, role := range roles {
		if allowed[role] && !seen[role] {
			seen[role] = true
			result = append(result, role)
		}
	}
	sort.Strings(result)
	return result
}

func removeRole(roles []string, remove string) []string {
	result := roles[:0]
	for _, role := range roles {
		if role != remove {
			result = append(result, role)
		}
	}
	return result
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON request")
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func bearerToken(r *http.Request) string {
	value := r.Header.Get("Authorization")
	if !strings.HasPrefix(value, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(value, "Bearer "))
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return origin == scheme+"://"+r.Host
}

func changesState(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}
