package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"netatlas/internal/arpscout"
	atlas "netatlas/internal/netatlas"
)

func TestServerServesPagesAndStaticAssets(t *testing.T) {
	server := httptest.NewServer(newServer(serverConfig{
		htmlDir:   "../../web/html",
		staticDir: "../../web/static",
	}))
	defer server.Close()

	tests := []string{
		"/",
		"/login",
		"/map",
		"/sensors",
		"/sensor",
		"/html/index.html",
		"/static/css/base.css",
		"/static/css/layout.css",
	}

	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			res, err := http.Get(server.URL + path)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", res.StatusCode)
			}
		})
	}
}

func TestServerAcceptsArpScoutTransport(t *testing.T) {
	store := &recordingArpStore{}
	server := httptest.NewServer(newServer(serverConfig{
		htmlDir:   "../../web/html",
		staticDir: "../../web/static",
		arpStore:  store,
	}))
	defer server.Close()

	postJSON(t, server.URL+"/api/arpscout/register", map[string]any{
		"sensor_id":    "arpscout-test",
		"display_name": "Test ARP scout",
		"version":      "0.8",
		"hostname":     "test-host",
		"site":         "lab",
		"interfaces":   []string{"eth0"},
		"capabilities": []string{"observation_transport"},
	})
	postJSON(t, server.URL+"/api/arpscout/heartbeat", map[string]any{
		"identity": map[string]any{
			"sensor_id":    "arpscout-test",
			"display_name": "Test ARP scout",
			"version":      "0.8",
			"hostname":     "test-host",
			"site":         "lab",
		},
		"status": map[string]any{
			"iterations":   1,
			"observations": 2,
			"changes":      1,
		},
	})
	postJSON(t, server.URL+"/api/arpscout/observations", map[string]any{
		"sensor_id":   "arpscout-test",
		"captured_at": "2026-07-06T22:00:00Z",
		"observations": []map[string]any{
			{"ip": "192.168.1.10", "mac": "b8:27:eb:12:34:56", "source": "passive_neigh", "observed_at": "2026-07-06T22:00:00Z"},
		},
		"changes": []map[string]any{
			{"type": "device_new", "ip": "192.168.1.10", "message": "new device seen", "observed_at": "2026-07-06T22:00:00Z"},
		},
	})

	res, err := http.Get(server.URL + "/api/sensors")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var sensors []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&sensors); err != nil {
		t.Fatal(err)
	}
	var found map[string]any
	for _, sensor := range sensors {
		if sensor["id"] == "arpscout-test" {
			found = sensor
			break
		}
	}
	if found == nil {
		t.Fatalf("arpscout sensor missing from %#v", sensors)
	}
	if found["name"] != "Test ARP scout" || found["events"].(float64) != 2 {
		t.Fatalf("sensor = %#v", found)
	}
	if _, ok := found["uptime"].(float64); !ok {
		t.Fatalf("sensor uptime should be numeric for frontend progress bars: %#v", found)
	}

	res, err = http.Get(server.URL + "/api/sensors/arpscout-test")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("detail status = %d, want 200", res.StatusCode)
	}
	var detail map[string]any
	if err := json.NewDecoder(res.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail["id"] != "arpscout-test" || detail["config"] == nil || detail["topDomains"] == nil || detail["timeline"] == nil {
		t.Fatalf("detail = %#v", detail)
	}

	res, err = http.Get(server.URL + "/api/graph")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("graph status = %d, want 200", res.StatusCode)
	}
	var graph map[string][]map[string]any
	if err := json.NewDecoder(res.Body).Decode(&graph); err != nil {
		t.Fatal(err)
	}
	if !hasGraphNode(graph["nodes"], "arp-sensor:arpscout-test") || !hasGraphNode(graph["nodes"], "arp-device:192.168.1.10") {
		t.Fatalf("graph missing arpscout nodes: %#v", graph["nodes"])
	}
	if store.registers != 1 || store.heartbeats != 1 || store.batches != 1 {
		t.Fatalf("persist calls = registers:%d heartbeats:%d batches:%d", store.registers, store.heartbeats, store.batches)
	}
	if store.lastBatch.SensorID != "arpscout-test" || len(store.lastBatch.Observations) != 1 {
		t.Fatalf("persisted batch = %#v", store.lastBatch)
	}
}

func TestArpScoutTransportRejectsBadMethod(t *testing.T) {
	server := httptest.NewServer(newServer(serverConfig{
		htmlDir:   "../../web/html",
		staticDir: "../../web/static",
	}))
	defer server.Close()

	res, err := http.Get(server.URL + "/api/arpscout/register")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", res.StatusCode)
	}
}

func TestServerUsesDNSDataSourceWhenAvailable(t *testing.T) {
	server := httptest.NewServer(newServer(serverConfig{
		htmlDir:   "../../web/html",
		staticDir: "../../web/static",
		dnsSource: fakeDNSSource{},
	}))
	defer server.Close()

	res, err := http.Get(server.URL + "/api/sensors")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var sensors []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&sensors); err != nil {
		t.Fatal(err)
	}
	if len(sensors) != 1 || sensors[0]["id"] != "dns-dataset" {
		t.Fatalf("sensors = %#v", sensors)
	}

	res, err = http.Get(server.URL + "/api/graph")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var graph map[string][]map[string]any
	if err := json.NewDecoder(res.Body).Decode(&graph); err != nil {
		t.Fatal(err)
	}
	if !hasGraphNode(graph["nodes"], "dns-sensor:dns-dataset") || !hasGraphNode(graph["nodes"], "dns-domain:example.com") {
		t.Fatalf("graph = %#v", graph)
	}

	res, err = http.Get(server.URL + "/api/stats")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var stats map[string]any
	if err := json.NewDecoder(res.Body).Decode(&stats); err != nil {
		t.Fatal(err)
	}
	dnsRequests, ok := stats["dnsRequests"].(map[string]any)
	if !ok || dnsRequests["value"].(float64) != 42 {
		t.Fatalf("stats = %#v", stats)
	}

	res, err = http.Get(server.URL + "/api/sensors/dns-dataset")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var detail map[string]any
	if err := json.NewDecoder(res.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail["id"] != "dns-dataset" {
		t.Fatalf("detail = %#v", detail)
	}
}

func TestServerUpdatesClientMetadata(t *testing.T) {
	server := httptest.NewServer(newServer(serverConfig{
		htmlDir:   "../../web/html",
		staticDir: "../../web/static",
		dnsSource: fakeDNSSource{},
	}))
	defer server.Close()

	res, err := http.Post(server.URL+"/api/clients/192.168.1.10", "application/json", bytes.NewBufferString(`{
		"manual_name":"Kitchen tablet",
		"hostname":"kitchen-tab",
		"device_type":"tablet"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var detail map[string]any
	if err := json.NewDecoder(res.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail["label"] != "Kitchen tablet" || detail["deviceType"] != "tablet" {
		t.Fatalf("detail = %#v", detail)
	}
}

func TestServerIncludesConfiguredArpScoutSensor(t *testing.T) {
	identity := arpscoutIdentity("arpscout-configured", "Configured ARP scout")
	server := httptest.NewServer(newServer(serverConfig{
		htmlDir:   "../../web/html",
		staticDir: "../../web/static",
		arpScout:  &identity,
	}))
	defer server.Close()

	res, err := http.Get(server.URL + "/api/sensors")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var sensors []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&sensors); err != nil {
		t.Fatal(err)
	}
	var found map[string]any
	for _, sensor := range sensors {
		if sensor["id"] == "arpscout-configured" {
			found = sensor
			break
		}
	}
	if found == nil || found["status"] != "offline" {
		t.Fatalf("configured arpscout missing/offline status wrong: %#v", sensors)
	}
}

func postJSON(t *testing.T, url string, value any) {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		t.Fatalf("POST %s status = %d", url, res.StatusCode)
	}
}

func TestAuthLoginAPI(t *testing.T) {
	server := httptest.NewServer(newServer(serverConfig{
		htmlDir:   "../../web/html",
		staticDir: "../../web/static",
	}))
	defer server.Close()

	postJSON(t, server.URL+"/api/auth/login", map[string]any{
		"username": "admin",
		"password": "secret",
	})

	res, err := http.Post(server.URL+"/api/auth/login", "application/json", bytes.NewReader([]byte(`{"username":"","password":"secret"}`)))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.StatusCode)
	}
}

func hasGraphNode(nodes []map[string]any, id string) bool {
	for _, node := range nodes {
		if node["id"] == id {
			return true
		}
	}
	return false
}

type fakeDNSSource struct{}

func (fakeDNSSource) Stats(context.Context) (map[string]any, error) {
	return map[string]any{
		"activeSensors": 1,
		"totalSensors":  1,
		"dnsRequests":   map[string]any{"value": 42, "delta": 0},
		"newDevices":    map[string]any{"value": 2, "delta": 0},
		"newDomains":    map[string]any{"value": 1, "delta": 0},
		"alerts":        map[string]any{"value": 0, "delta": 0},
		"topTalkers":    []map[string]any{},
		"recentDomains": []map[string]any{},
		"timeline":      []map[string]any{},
	}, nil
}

func (fakeDNSSource) Graph(context.Context) (map[string]any, error) {
	return map[string]any{
		"nodes": []map[string]any{
			{"id": "dns-sensor:dns-dataset", "label": "DNS dataset", "type": "sensor"},
			{"id": "dns-domain:example.com", "label": "example.com", "type": "domain"},
		},
		"edges": []map[string]any{
			{"source": "dns-sensor:dns-dataset", "target": "dns-domain:example.com", "type": "query", "count": 42},
		},
	}, nil
}

func (fakeDNSSource) SensorSummaries(context.Context) ([]map[string]any, error) {
	return []map[string]any{
		{"id": "dns-dataset", "name": "DNS dataset", "status": "online", "sources": []string{"DNS"}, "events": 42, "uptime": 100, "latency": 0},
	}, nil
}

func (fakeDNSSource) SensorDetails(context.Context, string) (map[string]any, bool, error) {
	return map[string]any{"id": "dns-dataset", "config": map[string]any{}, "topDomains": []map[string]any{}, "timeline": []map[string]any{}}, true, nil
}

func (fakeDNSSource) ClientDetails(_ context.Context, ip string) (map[string]any, bool, error) {
	return map[string]any{"ip": ip, "label": ip, "displaySource": "ip"}, true, nil
}

func (fakeDNSSource) UpdateClientMetadata(_ context.Context, patch atlas.ClientMetadataPatch) (map[string]any, error) {
	return map[string]any{
		"ip":            patch.IP,
		"label":         patch.ManualName,
		"manualName":    patch.ManualName,
		"hostname":      patch.Hostname,
		"deviceType":    patch.DeviceType,
		"notes":         patch.Notes,
		"displaySource": "manual",
	}, nil
}

func arpscoutIdentity(id, name string) arpscout.Identity {
	return arpscout.Identity{
		SensorID:     id,
		DisplayName:  name,
		Version:      "1.0",
		Hostname:     "test-host",
		Site:         "lab",
		Interfaces:   []string{"eth0"},
		Capabilities: []string{"passive_arp"},
	}
}

type recordingArpStore struct {
	registers  int
	heartbeats int
	batches    int
	lastBatch  arpscout.ObservationBatch
}

func (s *recordingArpStore) Register(context.Context, arpscout.Identity) error {
	s.registers++
	return nil
}

func (s *recordingArpStore) Heartbeat(context.Context, arpscout.Identity, arpscout.DaemonStatus) error {
	s.heartbeats++
	return nil
}

func (s *recordingArpStore) WriteBatch(_ context.Context, batch arpscout.ObservationBatch) error {
	s.batches++
	s.lastBatch = batch
	return nil
}

func TestLoadServerConfigReadsUISection(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "custom", "html"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "custom", "static"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.ini")
	content := `[ui]
listen_address = 0.0.0.0
port = 9090
html_dir = custom/html
static_dir = custom/static
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadServerConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.listenAddress != "0.0.0.0" || cfg.port != 9090 {
		t.Fatalf("listen config = %s:%d", cfg.listenAddress, cfg.port)
	}
	if cfg.htmlDir != filepath.Join(dir, "custom/html") || cfg.staticDir != filepath.Join(dir, "custom/static") {
		t.Fatalf("asset dirs = %q/%q", cfg.htmlDir, cfg.staticDir)
	}
	if cfg.listenAddr() != "0.0.0.0:9090" {
		t.Fatalf("listenAddr = %q", cfg.listenAddr())
	}
}

func TestMergeServerConfigAppliesFlagOverrides(t *testing.T) {
	fileCfg := serverConfig{
		configPath:    "config.ini",
		listenAddress: "0.0.0.0",
		port:          8080,
		htmlDir:       "web/html",
		staticDir:     "web/static",
	}
	flagCfg := serverConfig{
		configPath:    "other.ini",
		listenAddress: "192.168.1.20",
		port:          9090,
	}

	cfg := mergeServerConfig(fileCfg, flagCfg)
	if cfg.configPath != "other.ini" || cfg.listenAddress != "192.168.1.20" || cfg.port != 9090 {
		t.Fatalf("merged config = %#v", cfg)
	}
	if cfg.htmlDir != "web/html" || cfg.staticDir != "web/static" {
		t.Fatalf("asset dirs should come from file config: %#v", cfg)
	}
}

func TestServerServesMockAPI(t *testing.T) {
	server := httptest.NewServer(newServer(serverConfig{
		htmlDir:   "../../web/html",
		staticDir: "../../web/static",
	}))
	defer server.Close()

	res, err := http.Get(server.URL + "/api/sensors")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}

	var sensors []map[string]any
	if err := json.NewDecoder(res.Body).Decode(&sensors); err != nil {
		t.Fatal(err)
	}
	if len(sensors) == 0 {
		t.Fatal("sensors response was empty")
	}
}
