package arpscout

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/ini.v1"
)

type TransportConfig struct {
	Enabled    bool
	CoreURL    string
	Token      string
	SpoolPath  string
	DryRunPath string
	Timeout    time.Duration
	Retries    int
}

type ObservationBatch struct {
	SensorID     string        `json:"sensor_id"`
	CapturedAt   time.Time     `json:"captured_at"`
	Observations []Observation `json:"observations"`
	Changes      []ChangeEvent `json:"changes,omitempty"`
}

type Transport interface {
	Register(ctx context.Context, identity Identity) error
	Heartbeat(ctx context.Context, identity Identity, status DaemonStatus) error
	UploadBatch(ctx context.Context, batch ObservationBatch) error
}

type CoreClient struct {
	baseURL string
	token   string
	client  *http.Client
}

type Uploader struct {
	Transport  Transport
	SpoolPath  string
	DryRunPath string
	Retries    int
}

func DefaultTransportConfig() TransportConfig {
	return TransportConfig{
		SpoolPath: "state/arpscout-spool.jsonl",
		Timeout:   10 * time.Second,
		Retries:   2,
	}
}

func LoadTransportConfig(path string) (TransportConfig, error) {
	cfg := DefaultTransportConfig()
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
	section := file.Section("arpscout_transport")
	cfg.Enabled = section.Key("enabled").MustBool(cfg.Enabled)
	cfg.CoreURL = strings.TrimRight(strings.TrimSpace(section.Key("core_url").String()), "/")
	cfg.Token = strings.TrimSpace(section.Key("token").String())
	cfg.SpoolPath = section.Key("spool_path").MustString(cfg.SpoolPath)
	cfg.DryRunPath = strings.TrimSpace(section.Key("dry_run_path").String())
	cfg.Timeout = section.Key("timeout").MustDuration(cfg.Timeout)
	cfg.Retries = section.Key("retries").MustInt(cfg.Retries)
	return cfg, nil
}

func NewCoreClient(cfg TransportConfig) (*CoreClient, error) {
	if err := ValidateTransportConfig(cfg); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.CoreURL) == "" {
		return nil, fmt.Errorf("arpscout transport core_url is required when transport is enabled")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTransportConfig().Timeout
	}
	return &CoreClient{
		baseURL: strings.TrimRight(cfg.CoreURL, "/"),
		token:   cfg.Token,
		client:  &http.Client{Timeout: timeout},
	}, nil
}

func (c *CoreClient) Register(ctx context.Context, identity Identity) error {
	return c.post(ctx, "/api/arpscout/register", identity)
}

func (c *CoreClient) Heartbeat(ctx context.Context, identity Identity, status DaemonStatus) error {
	payload := map[string]any{
		"identity": identity,
		"status":   status,
	}
	return c.post(ctx, "/api/arpscout/heartbeat", payload)
}

func (c *CoreClient) UploadBatch(ctx context.Context, batch ObservationBatch) error {
	return c.post(ctx, "/api/arpscout/observations", batch)
}

func (c *CoreClient) post(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("core %s returned %s", path, resp.Status)
	}
	return nil
}

func (u *Uploader) Register(ctx context.Context, identity Identity) error {
	if u == nil || u.Transport == nil {
		return nil
	}
	return u.retry(ctx, func() error { return u.Transport.Register(ctx, identity) })
}

func (u *Uploader) Heartbeat(ctx context.Context, identity Identity, status DaemonStatus) error {
	if u == nil || u.Transport == nil {
		return nil
	}
	return u.retry(ctx, func() error { return u.Transport.Heartbeat(ctx, identity, status) })
}

func (u *Uploader) UploadBatch(ctx context.Context, batch ObservationBatch) error {
	if u == nil {
		return nil
	}
	if u.DryRunPath != "" {
		return appendJSONLine(u.DryRunPath, batch)
	}
	if u.Transport == nil {
		return nil
	}
	if err := u.retry(ctx, func() error { return u.Transport.UploadBatch(ctx, batch) }); err != nil {
		if u.SpoolPath != "" {
			if spoolErr := appendJSONLine(u.SpoolPath, batch); spoolErr != nil {
				return fmt.Errorf("%w; spool failed: %v", err, spoolErr)
			}
			return nil
		}
		return err
	}
	return nil
}

func (u *Uploader) retry(ctx context.Context, fn func() error) error {
	retries := u.Retries
	if retries < 0 {
		retries = 0
	}
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if err := fn(); err != nil {
			lastErr = err
			if attempt == retries {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
			continue
		}
		return nil
	}
	return lastErr
}

func appendJSONLine(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	return encoder.Encode(value)
}
