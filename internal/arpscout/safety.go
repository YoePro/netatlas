package arpscout

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	MinScanTimeout      = time.Second
	MaxScanTimeout      = 10 * time.Second
	MinScanInterval     = 10 * time.Millisecond
	MaxScanTargets      = 1024
	MinPassiveInterval  = 5 * time.Second
	MinStatusInterval   = 10 * time.Second
	MinDaemonScanPeriod = time.Minute
)

type PermissionDiagnostics struct {
	EffectiveUserID  int    `json:"effective_user_id"`
	RunningAsRoot    bool   `json:"running_as_root"`
	ArpingPath       string `json:"arping_path,omitempty"`
	ArpingAvailable  bool   `json:"arping_available"`
	ActiveScanReady  bool   `json:"active_scan_ready"`
	ActiveScanAdvice string `json:"active_scan_advice,omitempty"`
}

type ConfigDiagnostics struct {
	OK          bool                  `json:"ok"`
	Errors      []string              `json:"errors,omitempty"`
	Permissions PermissionDiagnostics `json:"permissions"`
	Interfaces  []InterfaceDiagnostic `json:"interfaces,omitempty"`
	Daemon      map[string]any        `json:"daemon,omitempty"`
	Transport   map[string]any        `json:"transport,omitempty"`
}

type InterfaceDiagnostic struct {
	Name        string `json:"name"`
	Up          bool   `json:"up"`
	Loopback    bool   `json:"loopback"`
	NetworkType string `json:"network_type"`
}

func ValidateScanOptions(options ScanOptions) error {
	if strings.TrimSpace(options.CIDR) == "" {
		return fmt.Errorf("scan cidr is required; use arpscout scan -dry-run to auto-discover a safe target")
	}
	maxHosts := options.MaxHosts
	if maxHosts <= 0 {
		maxHosts = DefaultMaxScanHosts
	}
	if maxHosts > MaxScanTargets {
		return fmt.Errorf("max scan hosts %d exceeds safety limit %d", maxHosts, MaxScanTargets)
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = DefaultScanTimeout
	}
	if timeout < MinScanTimeout || timeout > MaxScanTimeout {
		return fmt.Errorf("scan timeout must be between %s and %s", MinScanTimeout, MaxScanTimeout)
	}
	if options.Interval > 0 && options.Interval < MinScanInterval {
		return fmt.Errorf("scan interval must be at least %s", MinScanInterval)
	}
	return nil
}

func ValidateDaemonConfig(cfg DaemonConfig) error {
	var errs []string
	cfg = normalizeDaemonConfig(cfg)
	if cfg.PassiveInterval < MinPassiveInterval {
		errs = append(errs, fmt.Sprintf("passive_interval must be at least %s", MinPassiveInterval))
	}
	if cfg.StatusInterval < MinStatusInterval {
		errs = append(errs, fmt.Sprintf("status_interval must be at least %s", MinStatusInterval))
	}
	if cfg.ActiveScan {
		if strings.TrimSpace(cfg.ScanCIDR) == "" {
			errs = append(errs, "scan_cidr is required when active_scan=true")
		}
		if cfg.ScanInterval < MinDaemonScanPeriod {
			errs = append(errs, fmt.Sprintf("scan_interval must be at least %s", MinDaemonScanPeriod))
		}
		if cfg.MaxScanHosts > MaxScanTargets {
			errs = append(errs, fmt.Sprintf("max_scan_hosts %d exceeds safety limit %d", cfg.MaxScanHosts, MaxScanTargets))
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func ValidateTransportConfig(cfg TransportConfig) error {
	if !cfg.Enabled && cfg.DryRunPath == "" {
		return nil
	}
	if cfg.Enabled && strings.TrimSpace(cfg.CoreURL) == "" {
		return fmt.Errorf("arpscout_transport.core_url is required when transport is enabled")
	}
	if cfg.Timeout > 0 && cfg.Timeout < time.Second {
		return fmt.Errorf("arpscout_transport.timeout must be at least 1s")
	}
	if cfg.Retries < 0 {
		return fmt.Errorf("arpscout_transport.retries must not be negative")
	}
	if cfg.Enabled && cfg.SpoolPath == "" {
		return fmt.Errorf("arpscout_transport.spool_path must not be empty when transport is enabled")
	}
	return nil
}

func PermissionCheck() PermissionDiagnostics {
	diag := PermissionDiagnostics{
		EffectiveUserID: os.Geteuid(),
		RunningAsRoot:   os.Geteuid() == 0,
	}
	if path, err := exec.LookPath("arping"); err == nil {
		diag.ArpingPath = path
		diag.ArpingAvailable = true
	}
	diag.ActiveScanReady = diag.ArpingAvailable
	if !diag.ArpingAvailable {
		diag.ActiveScanAdvice = "install arping before active scans"
	}
	return diag
}

func ActiveScanReadinessError() error {
	diag := PermissionCheck()
	if diag.ActiveScanReady {
		return nil
	}
	if diag.ActiveScanAdvice != "" {
		return fmt.Errorf("active scan is not ready: %s", diag.ActiveScanAdvice)
	}
	return fmt.Errorf("active scan is not ready")
}

func InterfaceDiagnostics() []InterfaceDiagnostic {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	items := make([]InterfaceDiagnostic, 0, len(interfaces))
	for _, iface := range interfaces {
		items = append(items, InterfaceDiagnostic{
			Name:        iface.Name,
			Up:          iface.Flags&net.FlagUp != 0,
			Loopback:    iface.Flags&net.FlagLoopback != 0,
			NetworkType: classifyInterface(iface.Name),
		})
	}
	return items
}

func ValidateInterfaceNames(names []string) error {
	if len(names) == 0 {
		return nil
	}
	known := make(map[string]struct{})
	interfaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("list interfaces: %w", err)
	}
	for _, iface := range interfaces {
		known[iface.Name] = struct{}{}
	}
	var missing []string
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := known[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("unknown interface(s): %s", strings.Join(missing, ", "))
	}
	return nil
}

func BuildConfigDiagnostics(daemon DaemonConfig, transport TransportConfig) ConfigDiagnostics {
	var errs []string
	if err := ValidateDaemonConfig(daemon); err != nil {
		errs = append(errs, err.Error())
	}
	if err := ValidateTransportConfig(transport); err != nil {
		errs = append(errs, err.Error())
	}
	if err := ValidateInterfaceNames(daemon.Interfaces); err != nil {
		errs = append(errs, err.Error())
	}
	permissions := PermissionCheck()
	if daemon.ActiveScan && !permissions.ActiveScanReady {
		errs = append(errs, "active_scan=true but active scan permissions/tools are not ready: "+permissions.ActiveScanAdvice)
	}
	daemon = normalizeDaemonConfig(daemon)
	return ConfigDiagnostics{
		OK:          len(errs) == 0,
		Errors:      errs,
		Permissions: permissions,
		Interfaces:  InterfaceDiagnostics(),
		Daemon: map[string]any{
			"passive_interval": daemon.PassiveInterval.String(),
			"status_interval":  daemon.StatusInterval.String(),
			"active_scan":      daemon.ActiveScan,
			"scan_interval":    daemon.ScanInterval.String(),
			"max_scan_hosts":   daemon.MaxScanHosts,
		},
		Transport: map[string]any{
			"enabled":      transport.Enabled,
			"core_url_set": strings.TrimSpace(transport.CoreURL) != "",
			"spool_path":   transport.SpoolPath,
			"dry_run_path": transport.DryRunPath,
			"timeout":      transport.Timeout.String(),
			"retries":      transport.Retries,
		},
	}
}
