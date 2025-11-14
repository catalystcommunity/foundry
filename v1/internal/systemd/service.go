package systemd

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"
)

// SSHExecutor defines the interface for executing commands via SSH
type SSHExecutor interface {
	Execute(cmd string) (string, error)
}

// CreateService creates a systemd service unit file on the remote host
func CreateService(conn SSHExecutor, name string, unit *UnitFile) error {
	if !strings.HasSuffix(name, ".service") {
		name = name + ".service"
	}

	content, err := renderUnitFile(unit)
	if err != nil {
		return fmt.Errorf("failed to render unit file: %w", err)
	}

	// Write unit file to /etc/systemd/system/
	path := fmt.Sprintf("/etc/systemd/system/%s", name)
	cmd := fmt.Sprintf("sudo tee %s > /dev/null << 'FOUNDRY_EOF'\n%s\nFOUNDRY_EOF", path, content)

	if _, err := conn.Execute(cmd); err != nil {
		return fmt.Errorf("failed to write unit file: %w", err)
	}

	// Reload systemd daemon to pick up new unit file
	if _, err := conn.Execute("sudo systemctl daemon-reload"); err != nil {
		return fmt.Errorf("failed to reload systemd daemon: %w", err)
	}

	return nil
}

// EnableService enables a systemd service to start on boot
func EnableService(conn SSHExecutor, name string) error {
	if !strings.HasSuffix(name, ".service") {
		name = name + ".service"
	}

	cmd := fmt.Sprintf("sudo systemctl enable %s", name)
	if _, err := conn.Execute(cmd); err != nil {
		return fmt.Errorf("failed to enable service %s: %w", name, err)
	}

	return nil
}

// StartService starts a systemd service
func StartService(conn SSHExecutor, name string) error {
	if !strings.HasSuffix(name, ".service") {
		name = name + ".service"
	}

	cmd := fmt.Sprintf("sudo systemctl start %s", name)
	if _, err := conn.Execute(cmd); err != nil {
		return fmt.Errorf("failed to start service %s: %w", name, err)
	}

	return nil
}

// StopService stops a systemd service
func StopService(conn SSHExecutor, name string) error {
	if !strings.HasSuffix(name, ".service") {
		name = name + ".service"
	}

	cmd := fmt.Sprintf("sudo systemctl stop %s", name)
	if _, err := conn.Execute(cmd); err != nil {
		return fmt.Errorf("failed to stop service %s: %w", name, err)
	}

	return nil
}

// RestartService restarts a systemd service
func RestartService(conn SSHExecutor, name string) error {
	if !strings.HasSuffix(name, ".service") {
		name = name + ".service"
	}

	cmd := fmt.Sprintf("sudo systemctl restart %s", name)
	if _, err := conn.Execute(cmd); err != nil {
		return fmt.Errorf("failed to restart service %s: %w", name, err)
	}

	return nil
}

// DisableService disables a systemd service from starting on boot
func DisableService(conn SSHExecutor, name string) error {
	if !strings.HasSuffix(name, ".service") {
		name = name + ".service"
	}

	cmd := fmt.Sprintf("sudo systemctl disable %s", name)
	if _, err := conn.Execute(cmd); err != nil {
		return fmt.Errorf("failed to disable service %s: %w", name, err)
	}

	return nil
}

// GetServiceStatus queries the status of a systemd service
func GetServiceStatus(conn SSHExecutor, name string) (*ServiceStatus, error) {
	if !strings.HasSuffix(name, ".service") {
		name = name + ".service"
	}

	status := &ServiceStatus{
		Name: name,
	}

	// Get basic status
	cmd := fmt.Sprintf("systemctl show %s --no-pager", name)
	output, err := conn.Execute(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to get service status: %w", err)
	}

	// Parse systemctl show output
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := parts[0]
		value := parts[1]

		switch key {
		case "LoadState":
			status.LoadState = value
			status.Loaded = value == "loaded"
		case "ActiveState":
			status.ActiveState = value
			status.Active = value == "active"
		case "SubState":
			status.SubState = value
			status.Running = value == "running"
		case "UnitFileState":
			status.Enabled = value == "enabled"
		case "MainPID":
			if pid, err := strconv.Atoi(value); err == nil {
				status.MainPID = pid
			}
		case "ActiveEnterTimestamp":
			if t, err := parseSystemdTimestamp(value); err == nil {
				status.Since = t
			}
		case "MemoryCurrent":
			if value != "[not set]" {
				if mem, err := strconv.ParseUint(value, 10, 64); err == nil {
					status.Memory = mem
				}
			}
		case "TasksCurrent":
			if value != "[not set]" {
				if tasks, err := strconv.Atoi(value); err == nil {
					status.Tasks = tasks
				}
			}
		}
	}

	return status, nil
}

// IsServiceRunning checks if a service is currently running
func IsServiceRunning(conn SSHExecutor, name string) (bool, error) {
	status, err := GetServiceStatus(conn, name)
	if err != nil {
		return false, err
	}
	return status.Running, nil
}

// IsServiceEnabled checks if a service is enabled to start on boot
func IsServiceEnabled(conn SSHExecutor, name string) (bool, error) {
	status, err := GetServiceStatus(conn, name)
	if err != nil {
		return false, err
	}
	return status.Enabled, nil
}

// WaitForService waits for a service to reach a specific state
func WaitForService(conn SSHExecutor, name string, targetState string, timeout time.Duration) error {
	if !strings.HasSuffix(name, ".service") {
		name = name + ".service"
	}

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			status, err := GetServiceStatus(conn, name)
			if err != nil {
				return fmt.Errorf("failed to get service status: %w", err)
			}

			switch targetState {
			case "running":
				if status.Running {
					return nil
				}
			case "active":
				if status.Active {
					return nil
				}
			case "inactive":
				if !status.Active {
					return nil
				}
			case "dead":
				if status.SubState == "dead" {
					return nil
				}
			}

			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for service %s to reach state %s (current: %s)", name, targetState, status.SubState)
			}
		}
	}
}

// renderUnitFile renders a UnitFile into systemd unit file format
func renderUnitFile(unit *UnitFile) (string, error) {
	const unitTemplate = `[Unit]
Description={{ .Description }}
{{- if .After }}
After={{ join .After " " }}
{{- end }}
{{- if .Requires }}
Requires={{ join .Requires " " }}
{{- end }}
{{- if .Wants }}
Wants={{ join .Wants " " }}
{{- end }}

[Service]
Type={{ .Type }}
{{- if .ExecStartPre }}
ExecStartPre={{ .ExecStartPre }}
{{- end }}
{{- if .ExecStart }}
ExecStart={{ .ExecStart }}
{{- end }}
{{- if .ExecStop }}
ExecStop={{ .ExecStop }}
{{- end }}
{{- if .ExecStopPost }}
ExecStopPost={{ .ExecStopPost }}
{{- end }}
{{- if .ExecReload }}
ExecReload={{ .ExecReload }}
{{- end }}
{{- if .Restart }}
Restart={{ .Restart }}
{{- end }}
{{- if gt .RestartSec 0 }}
RestartSec={{ .RestartSec }}
{{- end }}
{{- if gt .TimeoutStopSec 0 }}
TimeoutStopSec={{ .TimeoutStopSec }}
{{- end }}
{{- if .KillMode }}
KillMode={{ .KillMode }}
{{- end }}
{{- if .User }}
User={{ .User }}
{{- end }}
{{- if .Group }}
Group={{ .Group }}
{{- end }}
{{- if .WorkingDirectory }}
WorkingDirectory={{ .WorkingDirectory }}
{{- end }}
{{- if .EnvironmentFile }}
EnvironmentFile={{ .EnvironmentFile }}
{{- end }}
{{- range $key, $value := .Environment }}
Environment="{{ $key }}={{ $value }}"
{{- end }}

[Install]
{{- if .WantedBy }}
WantedBy={{ join .WantedBy " " }}
{{- end }}
{{- if .RequiredBy }}
RequiredBy={{ join .RequiredBy " " }}
{{- end }}
`

	funcMap := template.FuncMap{
		"join": strings.Join,
	}

	tmpl, err := template.New("unit").Funcs(funcMap).Parse(unitTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, unit); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// parseSystemdTimestamp parses systemd timestamp format
func parseSystemdTimestamp(ts string) (time.Time, error) {
	if ts == "" || ts == "n/a" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}

	// Systemd timestamps are in the format: "Day YYYY-MM-DD HH:MM:SS TZ"
	// Example: "Mon 2024-01-15 10:30:45 UTC"
	re := regexp.MustCompile(`\w+ (\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}) \w+`)
	matches := re.FindStringSubmatch(ts)
	if len(matches) < 2 {
		return time.Time{}, fmt.Errorf("invalid timestamp format: %s", ts)
	}

	// Parse the extracted timestamp
	t, err := time.Parse("2006-01-02 15:04:05", matches[1])
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse timestamp: %w", err)
	}

	return t, nil
}
