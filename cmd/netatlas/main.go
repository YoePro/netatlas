package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/ini.v1"
)

type serverConfig struct {
	configPath    string
	listenAddress string
	port          int
	htmlDir       string
	staticDir     string
}

func main() {
	cfg := defaultServerConfig()
	flag.StringVar(&cfg.configPath, "config", cfg.configPath, "path to INI config file")
	flag.StringVar(&cfg.listenAddress, "addr", "", "HTTP listen address override")
	flag.IntVar(&cfg.port, "port", 0, "HTTP listen port override")
	flag.StringVar(&cfg.htmlDir, "html", "", "directory with HTML pages override")
	flag.StringVar(&cfg.staticDir, "static", "", "directory with static assets override")
	flag.Parse()

	loaded, err := loadServerConfig(cfg.configPath)
	if err != nil {
		log.Fatal(err)
	}
	cfg = mergeServerConfig(loaded, cfg)

	mux := newServer(cfg)
	log.Printf("NetAtlas UI listening on http://%s", cfg.listenAddr())
	log.Printf("Serving HTML from %s and static assets from %s", cfg.htmlDir, cfg.staticDir)
	if err := http.ListenAndServe(cfg.listenAddr(), mux); err != nil {
		log.Fatal(err)
	}
}

func defaultServerConfig() serverConfig {
	return serverConfig{
		configPath:    "config.ini",
		listenAddress: "0.0.0.0",
		port:          8080,
		htmlDir:       "web/html",
		staticDir:     "web/static",
	}
}

func loadServerConfig(path string) (serverConfig, error) {
	cfg := defaultServerConfig()
	cfg.configPath = path
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
	section := file.Section("ui")
	cfg.listenAddress = section.Key("listen_address").MustString(cfg.listenAddress)
	cfg.port = section.Key("port").MustInt(cfg.port)
	cfg.htmlDir = section.Key("html_dir").MustString(cfg.htmlDir)
	cfg.staticDir = section.Key("static_dir").MustString(cfg.staticDir)
	if cfg.listenAddress == "" {
		return cfg, fmt.Errorf("ui.listen_address must not be empty")
	}
	if cfg.port <= 0 || cfg.port > 65535 {
		return cfg, fmt.Errorf("ui.port must be between 1 and 65535")
	}
	if cfg.htmlDir == "" {
		return cfg, fmt.Errorf("ui.html_dir must not be empty")
	}
	if cfg.staticDir == "" {
		return cfg, fmt.Errorf("ui.static_dir must not be empty")
	}
	cfg.htmlDir = resolveConfigPath(path, cfg.htmlDir)
	cfg.staticDir = resolveConfigPath(path, cfg.staticDir)
	return cfg, nil
}

func resolveConfigPath(configPath, value string) string {
	if filepath.IsAbs(value) {
		return value
	}
	if _, err := os.Stat(value); err == nil {
		return value
	}
	baseDir := filepath.Dir(configPath)
	if baseDir == "." || baseDir == "" {
		return value
	}
	return filepath.Join(baseDir, value)
}

func mergeServerConfig(fileCfg, flagCfg serverConfig) serverConfig {
	result := fileCfg
	result.configPath = flagCfg.configPath
	if flagCfg.listenAddress != "" {
		result.listenAddress = flagCfg.listenAddress
	}
	if flagCfg.port != 0 {
		result.port = flagCfg.port
	}
	if flagCfg.htmlDir != "" {
		result.htmlDir = flagCfg.htmlDir
	}
	if flagCfg.staticDir != "" {
		result.staticDir = flagCfg.staticDir
	}
	return result
}

func (cfg serverConfig) listenAddr() string {
	return cfg.listenAddress + ":" + strconv.Itoa(cfg.port)
}

func newServer(cfg serverConfig) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", pageHandler(cfg.htmlDir, "index.html"))
	mux.HandleFunc("/login", pageHandler(cfg.htmlDir, "login.html"))
	mux.HandleFunc("/map", pageHandler(cfg.htmlDir, "map.html"))
	mux.HandleFunc("/sensors", pageHandler(cfg.htmlDir, "sensors.html"))
	mux.HandleFunc("/sensor", pageHandler(cfg.htmlDir, "sensor.html"))

	mux.HandleFunc("/html/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/html/")
		if name == "" || strings.Contains(name, "..") {
			http.NotFound(w, r)
			return
		}
		serveHTML(w, r, filepath.Join(cfg.htmlDir, name))
	})

	mux.HandleFunc("/static/css/layout.css", func(w http.ResponseWriter, r *http.Request) {
		serveStaticFile(w, r, filepath.Join(cfg.staticDir, "css", "layouts.css"))
	})
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(cfg.staticDir))))

	mux.HandleFunc("/api/stats", jsonHandler(mockStats()))
	mux.HandleFunc("/api/graph", jsonHandler(mockGraph()))
	mux.HandleFunc("/api/sensors", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sensors" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, mockSensors())
	})
	mux.HandleFunc("/api/sensors/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/sensors/")
		for _, sensor := range mockSensors() {
			if sensor["id"] == id {
				details := sensorDetails(sensor)
				writeJSON(w, details)
				return
			}
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	return logRequests(mux)
}

func pageHandler(htmlDir, name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && strings.Trim(r.URL.Path, "/") != strings.TrimSuffix(name, ".html") {
			http.NotFound(w, r)
			return
		}
		serveHTML(w, r, filepath.Join(htmlDir, name))
	}
}

func serveHTML(w http.ResponseWriter, r *http.Request, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, fmt.Sprintf("HTML page not found: %s", path), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func serveStaticFile(w http.ResponseWriter, r *http.Request, path string) {
	if _, err := os.Stat(path); err != nil {
		http.Error(w, fmt.Sprintf("static file not found: %s", path), http.StatusNotFound)
		return
	}
	http.ServeFile(w, r, path)
}

func jsonHandler(value any) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, value)
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if r.URL.Path != "/healthz" {
			log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
		}
	})
}

func mockStats() map[string]any {
	return map[string]any{
		"activeSensors": 4,
		"totalSensors":  5,
		"dnsRequests":   map[string]any{"value": 241040, "delta": 8.3},
		"newDevices":    map[string]any{"value": 12, "delta": 2},
		"newDomains":    map[string]any{"value": 47, "delta": -3},
		"alerts":        map[string]any{"value": 3, "delta": 1},
		"topTalkers": []map[string]any{
			{"host": "192.168.1.42", "name": "laptop-alice", "queries": 1820, "bytes": "2.4 MB"},
			{"host": "192.168.1.71", "name": "phone-carol", "queries": 3201, "bytes": "4.2 MB"},
			{"host": "192.168.3.20", "name": "workstation-dave", "queries": 1204, "bytes": "1.8 MB"},
			{"host": "192.168.1.55", "name": "desktop-bob", "queries": 942, "bytes": "1.1 MB"},
			{"host": "192.168.1.88", "name": "iot-cam01", "queries": 88, "bytes": "0.1 MB"},
		},
		"recentDomains": []map[string]any{
			{"domain": "api.example.org", "first_seen": "2 min ago", "category": "internal", "clients": 3},
			{"domain": "cdn-eu.fastly.net", "first_seen": "8 min ago", "category": "cdn", "clients": 7},
			{"domain": "telemetry.ms.com", "first_seen": "14 min ago", "category": "telemetry", "clients": 12},
			{"domain": "metrics.corp.local", "first_seen": "22 min ago", "category": "internal", "clients": 1},
		},
		"timeline": timeline(),
	}
}

func mockSensors() []map[string]any {
	return []map[string]any{
		{"id": "s1", "name": "dns-resolver-01", "location": "DC-A Rack 3", "version": "2.4.1", "status": "online", "lastSeen": "just now", "sources": []string{"DNS"}, "events": 142830, "latency": 1.2, "uptime": 99.8},
		{"id": "s2", "name": "dns-resolver-02", "location": "DC-B Rack 7", "version": "2.4.1", "status": "online", "lastSeen": "2 min ago", "sources": []string{"DNS"}, "events": 98210, "latency": 1.8, "uptime": 99.1},
		{"id": "s3", "name": "dhcp-monitor-01", "location": "DC-A Rack 1", "version": "1.1.0", "status": "warning", "lastSeen": "4 min ago", "sources": []string{"DHCP"}, "events": 4210, "latency": 12.4, "uptime": 97.3},
		{"id": "s4", "name": "fw-edge-01", "location": "Edge Router", "version": "1.0.0", "status": "online", "lastSeen": "1 min ago", "sources": []string{"Firewall"}, "events": 892000, "latency": 0.9, "uptime": 100},
		{"id": "s5", "name": "vpn-gateway-01", "location": "DMZ", "version": "0.9.2", "status": "offline", "lastSeen": "1 hr ago", "sources": []string{"VPN"}, "events": 0, "latency": nil, "uptime": 0},
	}
}

func sensorDetails(sensor map[string]any) map[string]any {
	details := map[string]any{}
	for key, value := range sensor {
		details[key] = value
	}
	details["cpu"] = 34
	details["memory"] = 61
	details["disk"] = 42
	details["config"] = map[string]any{
		"interface":    "eth0",
		"port":         53,
		"capture_mode": "passive",
		"buffer_size":  "512MB",
		"retention":    "30d",
	}
	details["recentErrors"] = []map[string]any{
		{"time": "10:42:03", "level": "warn", "msg": "Buffer utilization above 75%"},
		{"time": "09:18:51", "level": "info", "msg": "Configuration reloaded"},
		{"time": "08:03:22", "level": "warn", "msg": "Upstream timeout from 8.8.8.8"},
	}
	details["topDomains"] = []map[string]any{
		{"domain": "github.com", "queries": 4210, "pct": 12.1},
		{"domain": "microsoft.com", "queries": 3802, "pct": 10.9},
		{"domain": "cloudfront.net", "queries": 2901, "pct": 8.3},
	}
	details["timeline"] = timeline()
	return details
}

func mockGraph() map[string]any {
	return map[string]any{
		"nodes": []map[string]any{
			{"id": "n1", "label": "dns-resolver-01", "type": "sensor", "ip": "10.0.0.10", "status": "online", "queries": 142830},
			{"id": "n2", "label": "dns-resolver-02", "type": "sensor", "ip": "10.0.0.11", "status": "online", "queries": 98210},
			{"id": "n3", "label": "192.168.1.42", "type": "client", "ip": "192.168.1.42", "hostname": "laptop-alice"},
			{"id": "n4", "label": "192.168.1.71", "type": "client", "ip": "192.168.1.71", "hostname": "phone-carol"},
			{"id": "n5", "label": "github.com", "type": "domain", "tld": "com", "category": "development"},
			{"id": "n6", "label": "microsoft.com", "type": "domain", "tld": "com", "category": "productivity"},
			{"id": "n7", "label": "Internet", "type": "internet"},
		},
		"edges": []map[string]any{
			{"source": "n3", "target": "n1", "type": "query", "count": 1820},
			{"source": "n4", "target": "n2", "type": "query", "count": 3201},
			{"source": "n1", "target": "n5", "type": "resolve", "count": 420},
			{"source": "n2", "target": "n6", "type": "resolve", "count": 380},
			{"source": "n5", "target": "n7", "type": "upstream", "count": 420},
			{"source": "n6", "target": "n7", "type": "upstream", "count": 380},
		},
	}
}

func timeline() []map[string]any {
	points := make([]map[string]any, 0, 60)
	now := time.Now()
	for i := 59; i >= 0; i-- {
		points = append(points, map[string]any{
			"t":       now.Add(-time.Duration(i) * time.Minute).UnixMilli(),
			"queries": 2400 + (i%13)*137,
			"alerts":  i % 19,
		})
	}
	return points
}
