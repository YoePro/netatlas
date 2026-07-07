package arpscout

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/ini.v1"
)

const (
	DaemonEventBatch  = "batch"
	DaemonEventStatus = "status"
)

type DaemonConfig struct {
	Interfaces        []string
	IncludeIncomplete bool
	PassiveInterval   time.Duration
	StatusInterval    time.Duration
	GatewayIP         string
	ActiveScan        bool
	ScanCIDR          string
	ScanInterval      time.Duration
	MaxScanHosts      int
}

type DaemonOptions struct {
	Config        DaemonConfig
	Identity      Identity
	PassiveReader func(PassiveOptions) ([]Observation, error)
	Scanner       func(ScanOptions) (ScanResult, error)
	Transport     Transport
	Output        func(DaemonEvent)
	Now           func() time.Time
}

type DaemonStatus struct {
	StartedAt    time.Time `json:"started_at"`
	LastRunAt    time.Time `json:"last_run_at,omitempty"`
	Iterations   int       `json:"iterations"`
	Observations int       `json:"observations"`
	Changes      int       `json:"changes"`
	LastError    string    `json:"last_error,omitempty"`
}

type DaemonEvent struct {
	Kind         string        `json:"kind"`
	Status       DaemonStatus  `json:"status"`
	Observations []Observation `json:"observations,omitempty"`
	Changes      []ChangeEvent `json:"changes,omitempty"`
	Error        string        `json:"error,omitempty"`
}

func DefaultDaemonConfig() DaemonConfig {
	return DaemonConfig{
		PassiveInterval: 30 * time.Second,
		StatusInterval:  5 * time.Minute,
		ScanInterval:    10 * time.Minute,
		MaxScanHosts:    256,
	}
}

func LoadDaemonConfig(path string) (DaemonConfig, error) {
	cfg := DefaultDaemonConfig()
	if strings.TrimSpace(path) == "" {
		return cfg, nil
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("stat config %q: %w", path, err)
	}
	file, err := ini.Load(path)
	if err != nil {
		return cfg, fmt.Errorf("load config %q: %w", path, err)
	}
	section := file.Section("arpscout")
	cfg.Interfaces = splitConfigList(section.Key("interfaces").String())
	cfg.IncludeIncomplete = section.Key("include_incomplete").MustBool(cfg.IncludeIncomplete)
	cfg.PassiveInterval = section.Key("passive_interval").MustDuration(cfg.PassiveInterval)
	cfg.StatusInterval = section.Key("status_interval").MustDuration(cfg.StatusInterval)
	cfg.GatewayIP = strings.TrimSpace(section.Key("gateway_ip").String())
	cfg.ActiveScan = section.Key("active_scan").MustBool(cfg.ActiveScan)
	cfg.ScanCIDR = strings.TrimSpace(section.Key("scan_cidr").String())
	cfg.ScanInterval = section.Key("scan_interval").MustDuration(cfg.ScanInterval)
	cfg.MaxScanHosts = section.Key("max_scan_hosts").MustInt(cfg.MaxScanHosts)
	return cfg, nil
}

func RunDaemon(ctx context.Context, options DaemonOptions) error {
	cfg := normalizeDaemonConfig(options.Config)
	if err := ValidateDaemonConfig(cfg); err != nil {
		return err
	}
	passiveReader := options.PassiveReader
	if passiveReader == nil {
		passiveReader = ReadPassive
	}
	scanner := options.Scanner
	if scanner == nil {
		scanner = RunActiveScan
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	output := options.Output
	if output == nil {
		output = func(DaemonEvent) {}
	}

	status := DaemonStatus{StartedAt: now()}
	detector := NewChangeDetector(ChangeOptions{GatewayIP: cfg.GatewayIP})
	lastScan := time.Time{}
	if options.Transport != nil && options.Identity.SensorID != "" {
		if err := options.Transport.Register(ctx, options.Identity); err != nil {
			status.LastError = err.Error()
		}
	}

	runOnce := func() {
		runAt := now()
		status.LastRunAt = runAt
		status.Iterations++

		observations, err := passiveReader(PassiveOptions{
			Interfaces:        cfg.Interfaces,
			IncludeIncomplete: cfg.IncludeIncomplete,
			Now:               runAt,
		})
		if err != nil {
			status.LastError = err.Error()
			output(DaemonEvent{Kind: DaemonEventBatch, Status: status, Error: status.LastError})
			return
		}

		if cfg.ActiveScan && cfg.ScanCIDR != "" && shouldRunScan(runAt, lastScan, cfg.ScanInterval) {
			result, err := scanner(ScanOptions{
				CIDR:      cfg.ScanCIDR,
				Interface: firstInterface(cfg.Interfaces),
				MaxHosts:  cfg.MaxScanHosts,
				Timeout:   DefaultScanTimeout,
				Interval:  DefaultScanInterval,
				Now:       runAt,
			})
			lastScan = runAt
			if err != nil {
				status.LastError = err.Error()
			} else {
				observations = append(observations, result.Observations...)
			}
		}

		changes := detector.Apply(observations)
		status.Observations += len(observations)
		status.Changes += len(changes)
		if options.Transport != nil && options.Identity.SensorID != "" {
			err := options.Transport.UploadBatch(ctx, ObservationBatch{
				SensorID:     options.Identity.SensorID,
				CapturedAt:   runAt,
				Observations: observations,
				Changes:      changes,
			})
			if err != nil {
				status.LastError = err.Error()
			}
		}
		output(DaemonEvent{
			Kind:         DaemonEventBatch,
			Status:       status,
			Observations: observations,
			Changes:      changes,
			Error:        status.LastError,
		})
	}

	runOnce()
	passiveTicker := time.NewTicker(cfg.PassiveInterval)
	defer passiveTicker.Stop()
	statusTicker := time.NewTicker(cfg.StatusInterval)
	defer statusTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			if options.Transport != nil && options.Identity.SensorID != "" {
				_ = options.Transport.Heartbeat(context.Background(), options.Identity, status)
			}
			output(DaemonEvent{Kind: DaemonEventStatus, Status: status})
			return ctx.Err()
		case <-passiveTicker.C:
			runOnce()
		case <-statusTicker.C:
			if options.Transport != nil && options.Identity.SensorID != "" {
				if err := options.Transport.Heartbeat(ctx, options.Identity, status); err != nil {
					status.LastError = err.Error()
				}
			}
			output(DaemonEvent{Kind: DaemonEventStatus, Status: status})
		}
	}
}

func normalizeDaemonConfig(cfg DaemonConfig) DaemonConfig {
	defaults := DefaultDaemonConfig()
	if cfg.PassiveInterval <= 0 {
		cfg.PassiveInterval = defaults.PassiveInterval
	}
	if cfg.StatusInterval <= 0 {
		cfg.StatusInterval = defaults.StatusInterval
	}
	if cfg.ScanInterval <= 0 {
		cfg.ScanInterval = defaults.ScanInterval
	}
	if cfg.MaxScanHosts <= 0 {
		cfg.MaxScanHosts = defaults.MaxScanHosts
	}
	return cfg
}

func shouldRunScan(now, last time.Time, interval time.Duration) bool {
	return last.IsZero() || !now.Before(last.Add(interval))
}

func firstInterface(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
