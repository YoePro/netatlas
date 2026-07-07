package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"netatlas/internal/arpscout"
	"netatlas/internal/config"
	atlas "netatlas/internal/netatlas"

	"gopkg.in/ini.v1"
)

type serverConfig struct {
	configPath    string
	listenAddress string
	port          int
	htmlDir       string
	staticDir     string
	dnsSource     dnsDataSource
	arpScout      *arpscout.Identity
	arpStore      arpScoutStore
}

type dnsDataSource interface {
	Stats(context.Context) (map[string]any, error)
	Graph(context.Context) (map[string]any, error)
	SensorSummaries(context.Context) ([]map[string]any, error)
	SensorDetails(context.Context, string) (map[string]any, bool, error)
	ClientDetails(context.Context, string) (map[string]any, bool, error)
	UpdateClientMetadata(context.Context, atlas.ClientMetadataPatch) (map[string]any, error)
}

type arpScoutStore interface {
	Register(context.Context, arpscout.Identity) error
	Heartbeat(context.Context, arpscout.Identity, arpscout.DaemonStatus) error
	WriteBatch(context.Context, arpscout.ObservationBatch) error
}

const arpStoreTimeout = 30 * time.Second

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
	if identity, ok, err := loadConfiguredArpScout(cfg.configPath); err != nil {
		log.Printf("NetAtlas configured arpscout sensor disabled: %v", err)
	} else if ok {
		cfg.arpScout = &identity
	}
	dnsReader, err := openDNSReader(cfg.configPath)
	if err != nil {
		log.Printf("NetAtlas DNS dataset sensor disabled: %v", err)
	} else if dnsReader != nil {
		cfg.dnsSource = dnsReader
		log.Printf("NetAtlas DNS dataset sensor enabled from Neo4j")
	}
	arpStore, err := openArpStore(cfg.configPath)
	if err != nil {
		log.Printf("NetAtlas arpscout Neo4j persistence disabled: %v", err)
	} else if arpStore != nil {
		cfg.arpStore = arpStore
		log.Printf("NetAtlas arpscout Neo4j persistence enabled")
	}

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
	result.dnsSource = flagCfg.dnsSource
	if flagCfg.arpScout != nil {
		result.arpScout = flagCfg.arpScout
	}
	if flagCfg.arpStore != nil {
		result.arpStore = flagCfg.arpStore
	}
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
	core := newArpScoutCore(cfg.arpStore)
	if cfg.arpScout != nil {
		core.configure(*cfg.arpScout)
	}

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

	mux.HandleFunc("/api/stats", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/stats" {
			http.NotFound(w, r)
			return
		}
		stats := mockStats()
		if cfg.dnsSource != nil {
			if dnsStats, err := withAPITimeout(r.Context(), cfg.dnsSource.Stats); err == nil {
				stats = dnsStats
			} else {
				log.Printf("read dns stats failed: %v", err)
			}
		}
		var baseSensors []map[string]any
		if cfg.dnsSource != nil {
			if dnsSensors, err := withAPITimeout(r.Context(), cfg.dnsSource.SensorSummaries); err == nil {
				baseSensors = dnsSensors
			} else {
				log.Printf("read dns sensors for stats failed: %v", err)
			}
		}
		writeJSON(w, mergeArpScoutStats(stats, filterArpScoutSummaries(baseSensors, core.sensorSummaries())))
	})
	mux.HandleFunc("/api/graph", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/graph" {
			http.NotFound(w, r)
			return
		}
		graph := mockGraph()
		if cfg.dnsSource != nil {
			if dnsGraph, err := withAPITimeout(r.Context(), cfg.dnsSource.Graph); err == nil {
				graph = dnsGraph
			} else {
				log.Printf("read dns graph failed: %v", err)
			}
		}
		writeJSON(w, atlas.MergeGraph(graph, filterArpScoutGraph(graph, core.graph())))
	})
	mux.HandleFunc("/api/sensors", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sensors" {
			http.NotFound(w, r)
			return
		}
		sensors := mockSensors()
		if cfg.dnsSource != nil {
			if dnsSensors, err := withAPITimeout(r.Context(), cfg.dnsSource.SensorSummaries); err == nil && len(dnsSensors) > 0 {
				sensors = dnsSensors
			} else if err != nil {
				log.Printf("read dns sensors failed: %v", err)
			}
		}
		writeJSON(w, appendArpScoutSensors(sensors, filterArpScoutSummaries(sensors, core.sensorSummaries())))
	})
	mux.HandleFunc("/api/sensors/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/sensors/")
		if cfg.dnsSource != nil {
			if details, ok, err := withAPITimeoutValue(r.Context(), id, cfg.dnsSource.SensorDetails); err == nil && ok {
				writeJSON(w, details)
				return
			} else if err != nil {
				log.Printf("read dns sensor detail failed: %v", err)
			}
		}
		sensors := mockSensors()
		if cfg.dnsSource != nil {
			if dnsSensors, err := withAPITimeout(r.Context(), cfg.dnsSource.SensorSummaries); err == nil && len(dnsSensors) > 0 {
				sensors = dnsSensors
			}
		}
		for _, sensor := range appendArpScoutSensors(sensors, core.sensorSummaries()) {
			if sensor["id"] == id {
				details := sensorDetails(sensor)
				if arpDetails, ok := core.sensorDetails(id); ok {
					details = arpDetails
				}
				writeJSON(w, details)
				return
			}
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/api/auth/login", authLoginHandler)
	mux.HandleFunc("/api/clients/", clientHandler(cfg.dnsSource))
	mux.HandleFunc("/api/arpscout/register", core.registerHandler)
	mux.HandleFunc("/api/arpscout/heartbeat", core.heartbeatHandler)
	mux.HandleFunc("/api/arpscout/observations", core.observationsHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	return logRequests(mux)
}

func openDNSReader(path string) (*atlas.DNSReader, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	if cfg.Neo4jPassword == "" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return atlas.NewDNSReader(ctx, cfg)
}

func openArpStore(path string) (*atlas.ArpStore, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	if cfg.Neo4jPassword == "" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), arpStoreTimeout)
	defer cancel()
	return atlas.NewArpStore(ctx, cfg)
}

func loadConfiguredArpScout(path string) (arpscout.Identity, bool, error) {
	cfg, err := arpscout.LoadIdentityConfig(path)
	if err != nil {
		return arpscout.Identity{}, false, err
	}
	if strings.TrimSpace(cfg.SensorID) == "" && strings.TrimSpace(cfg.DisplayName) == "" && strings.TrimSpace(cfg.Site) == "" && len(cfg.Interfaces) == 0 {
		return arpscout.Identity{}, false, nil
	}
	identity, err := arpscout.BuildIdentity(cfg)
	if err != nil {
		return arpscout.Identity{}, false, err
	}
	return identity, true, nil
}

func withAPITimeout[T any](parent context.Context, fn func(context.Context) (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(parent, arpStoreTimeout)
	defer cancel()
	return fn(ctx)
}

func withAPITimeoutValue[T any, V any](parent context.Context, value V, fn func(context.Context, V) (T, bool, error)) (T, bool, error) {
	ctx, cancel := context.WithTimeout(parent, arpStoreTimeout)
	defer cancel()
	return fn(ctx, value)
}

func withAPITimeoutUpdate[T any, V any](parent context.Context, value V, fn func(context.Context, V) (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(parent, arpStoreTimeout)
	defer cancel()
	return fn(ctx, value)
}

type arpScoutCore struct {
	mu          sync.Mutex
	sensorState map[string]*arpScoutSensor
	store       arpScoutStore
}

type arpScoutSensor struct {
	Identity     arpscout.Identity
	Status       arpscout.DaemonStatus
	LastSeen     time.Time
	Observations int
	Changes      int
	Batches      int
	Recent       []arpscout.Observation
	RecentChange []arpscout.ChangeEvent
}

func newArpScoutCore(store arpScoutStore) *arpScoutCore {
	return &arpScoutCore{sensorState: make(map[string]*arpScoutSensor), store: store}
}

func (c *arpScoutCore) configure(identity arpscout.Identity) {
	if strings.TrimSpace(identity.SensorID) == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	sensor := c.ensureSensor(identity.SensorID)
	if sensor.Identity.DisplayName == "" {
		sensor.Identity = identity
	}
}

func (c *arpScoutCore) registerHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var identity arpscout.Identity
	if err := decodeJSON(r, &identity); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(identity.SensorID) == "" {
		http.Error(w, "sensor_id is required", http.StatusBadRequest)
		return
	}
	c.mu.Lock()
	sensor := c.ensureSensor(identity.SensorID)
	sensor.Identity = identity
	sensor.LastSeen = time.Now()
	c.mu.Unlock()
	c.persistRegister(r.Context(), identity)
	writeJSON(w, map[string]any{"ok": true, "sensor_id": identity.SensorID})
}

func (c *arpScoutCore) heartbeatHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var payload struct {
		Identity arpscout.Identity     `json:"identity"`
		Status   arpscout.DaemonStatus `json:"status"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(payload.Identity.SensorID) == "" {
		http.Error(w, "identity.sensor_id is required", http.StatusBadRequest)
		return
	}
	c.mu.Lock()
	sensor := c.ensureSensor(payload.Identity.SensorID)
	sensor.Identity = payload.Identity
	sensor.Status = payload.Status
	sensor.LastSeen = time.Now()
	c.mu.Unlock()
	c.persistHeartbeat(r.Context(), payload.Identity, payload.Status)
	writeJSON(w, map[string]any{"ok": true, "sensor_id": payload.Identity.SensorID})
}

func (c *arpScoutCore) observationsHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var batch arpscout.ObservationBatch
	if err := decodeJSON(r, &batch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(batch.SensorID) == "" {
		http.Error(w, "sensor_id is required", http.StatusBadRequest)
		return
	}
	c.mu.Lock()
	sensor := c.ensureSensor(batch.SensorID)
	sensor.LastSeen = time.Now()
	sensor.Observations += len(batch.Observations)
	sensor.Changes += len(batch.Changes)
	sensor.Batches++
	sensor.Recent = appendCappedObservations(sensor.Recent, batch.Observations, 50)
	sensor.RecentChange = appendCappedChanges(sensor.RecentChange, batch.Changes, 50)
	c.mu.Unlock()
	c.persistBatch(r.Context(), batch)
	writeJSON(w, map[string]any{
		"ok":           true,
		"sensor_id":    batch.SensorID,
		"observations": len(batch.Observations),
		"changes":      len(batch.Changes),
	})
}

func (c *arpScoutCore) persistRegister(parent context.Context, identity arpscout.Identity) {
	if c.store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, arpStoreTimeout)
	defer cancel()
	if err := c.store.Register(ctx, identity); err != nil {
		log.Printf("persist arpscout register failed: %v", err)
	}
}

func (c *arpScoutCore) persistHeartbeat(parent context.Context, identity arpscout.Identity, status arpscout.DaemonStatus) {
	if c.store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, arpStoreTimeout)
	defer cancel()
	if err := c.store.Heartbeat(ctx, identity, status); err != nil {
		log.Printf("persist arpscout heartbeat failed: %v", err)
	}
}

func (c *arpScoutCore) persistBatch(parent context.Context, batch arpscout.ObservationBatch) {
	if c.store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, arpStoreTimeout)
	defer cancel()
	if err := c.store.WriteBatch(ctx, batch); err != nil {
		log.Printf("persist arpscout batch failed: %v", err)
	}
}

func (c *arpScoutCore) ensureSensor(sensorID string) *arpScoutSensor {
	sensor := c.sensorState[sensorID]
	if sensor == nil {
		sensor = &arpScoutSensor{Identity: arpscout.Identity{SensorID: sensorID}}
		c.sensorState[sensorID] = sensor
	}
	return sensor
}

func (c *arpScoutCore) sensorSummaries() []map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	items := make([]map[string]any, 0, len(c.sensorState))
	for _, sensor := range c.sensorState {
		items = append(items, arpScoutSensorSummary(sensor))
	}
	return items
}

func (c *arpScoutCore) sensorDetails(sensorID string) (map[string]any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	sensor := c.sensorState[sensorID]
	if sensor == nil {
		return nil, false
	}
	details := arpScoutSensorSummary(sensor)
	details["cpu"] = 0
	details["memory"] = 0
	details["disk"] = 0
	details["config"] = map[string]any{
		"sensor_id":    sensor.Identity.SensorID,
		"hostname":     sensor.Identity.Hostname,
		"site":         sensor.Identity.Site,
		"interfaces":   sensor.Identity.Interfaces,
		"capabilities": sensor.Identity.Capabilities,
		"batches":      sensor.Batches,
	}
	details["recentErrors"] = arpScoutChangeActivities(sensor.RecentChange)
	details["topDomains"] = arpScoutTopDevices(sensor.Recent)
	details["timeline"] = timeline()
	return details, true
}

func (c *arpScoutCore) graph() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	nodes := make([]map[string]any, 0, len(c.sensorState))
	edges := make([]map[string]any, 0)
	seenDevices := make(map[string]struct{})
	for _, sensor := range c.sensorState {
		summary := arpScoutSensorSummary(sensor)
		sensorNodeID := "arp-sensor:" + sensor.Identity.SensorID
		nodes = append(nodes, map[string]any{
			"id":      sensorNodeID,
			"label":   summary["name"],
			"type":    "sensor",
			"status":  summary["status"],
			"queries": summary["events"],
			"source":  "arpscout",
		})
		for _, observation := range sensor.Recent {
			if strings.TrimSpace(observation.IP) == "" {
				continue
			}
			deviceNodeID := "arp-device:" + observation.IP
			if _, ok := seenDevices[deviceNodeID]; !ok {
				nodes = append(nodes, map[string]any{
					"id":       deviceNodeID,
					"label":    observation.IP,
					"type":     "client",
					"ip":       observation.IP,
					"hostname": arpDeviceLabel(observation),
					"mac":      observation.MAC,
					"source":   "arpscout",
				})
				seenDevices[deviceNodeID] = struct{}{}
			}
			edges = append(edges, map[string]any{
				"source": sensorNodeID,
				"target": deviceNodeID,
				"type":   "arp_seen",
				"count":  1,
			})
		}
	}
	return map[string]any{"nodes": nodes, "edges": edges}
}

func arpScoutSensorSummary(sensor *arpScoutSensor) map[string]any {
	name := sensor.Identity.DisplayName
	if name == "" {
		name = sensor.Identity.SensorID
	}
	location := sensor.Identity.Site
	if location == "" {
		location = sensor.Identity.Hostname
	}
	status := "online"
	if sensor.LastSeen.IsZero() {
		status = "offline"
	} else if time.Since(sensor.LastSeen) > 10*time.Minute {
		status = "warning"
	}
	uptime := 100.0
	if status == "offline" {
		uptime = 0
	} else if status != "online" {
		uptime = 97.0
	}
	return map[string]any{
		"id":       sensor.Identity.SensorID,
		"name":     name,
		"type":     "arp",
		"location": location,
		"version":  sensor.Identity.Version,
		"status":   status,
		"lastSeen": relativeTime(sensor.LastSeen),
		"sources":  []string{"ARP"},
		"events":   sensor.Observations + sensor.Changes,
		"latency":  0,
		"uptime":   uptime,
		"batches":  sensor.Batches,
	}
}

func appendCappedObservations(current, next []arpscout.Observation, limit int) []arpscout.Observation {
	if limit <= 0 {
		return nil
	}
	items := append(append([]arpscout.Observation{}, current...), next...)
	if len(items) > limit {
		items = items[len(items)-limit:]
	}
	return items
}

func appendCappedChanges(current, next []arpscout.ChangeEvent, limit int) []arpscout.ChangeEvent {
	if limit <= 0 {
		return nil
	}
	items := append(append([]arpscout.ChangeEvent{}, current...), next...)
	if len(items) > limit {
		items = items[len(items)-limit:]
	}
	return items
}

func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	age := time.Since(t)
	switch {
	case age < time.Minute:
		return "just now"
	case age < time.Hour:
		return fmt.Sprintf("%d min ago", int(age.Minutes()))
	case age < 24*time.Hour:
		return fmt.Sprintf("%d hr ago", int(age.Hours()))
	default:
		return fmt.Sprintf("%d days ago", int(age.Hours()/24))
	}
}

func arpScoutChangeActivities(changes []arpscout.ChangeEvent) []map[string]any {
	if len(changes) == 0 {
		return []map[string]any{}
	}
	start := 0
	if len(changes) > 10 {
		start = len(changes) - 10
	}
	items := make([]map[string]any, 0, len(changes)-start)
	for _, change := range changes[start:] {
		level := "info"
		if change.Type == arpscout.EventDuplicateIP || change.Type == arpscout.EventGatewayChange {
			level = "warn"
		}
		observed := change.ObservedAt
		if observed.IsZero() {
			observed = time.Now()
		}
		items = append(items, map[string]any{
			"time":  observed.Format("15:04:05"),
			"level": level,
			"msg":   change.Message,
		})
	}
	return items
}

func arpScoutTopDevices(observations []arpscout.Observation) []map[string]any {
	counts := make(map[string]int)
	latest := make(map[string]arpscout.Observation)
	for _, observation := range observations {
		if strings.TrimSpace(observation.IP) == "" {
			continue
		}
		counts[observation.IP]++
		latest[observation.IP] = observation
	}
	type pair struct {
		ip    string
		count int
	}
	pairs := make([]pair, 0, len(counts))
	for ip, count := range counts {
		pairs = append(pairs, pair{ip: ip, count: count})
	}
	sort.SliceStable(pairs, func(i, j int) bool {
		if pairs[i].count == pairs[j].count {
			return pairs[i].ip < pairs[j].ip
		}
		return pairs[i].count > pairs[j].count
	})
	if len(pairs) > 5 {
		pairs = pairs[:5]
	}
	total := len(observations)
	if total == 0 {
		total = 1
	}
	items := make([]map[string]any, 0, len(pairs))
	for _, item := range pairs {
		observation := latest[item.ip]
		vendor := ""
		if observation.Vendor != nil {
			vendor = *observation.Vendor
		}
		items = append(items, map[string]any{
			"domain":  item.ip,
			"label":   item.ip,
			"ip":      item.ip,
			"mac":     observation.MAC,
			"vendor":  vendor,
			"queries": item.count,
			"pct":     float64(item.count) * 100 / float64(total),
		})
	}
	return items
}

func arpDeviceLabel(observation arpscout.Observation) string {
	if observation.Vendor != nil && strings.TrimSpace(*observation.Vendor) != "" {
		return *observation.Vendor
	}
	if observation.MAC != "" {
		return observation.MAC
	}
	return observation.IP
}

func mergeArpScoutGraph(base, arpGraph map[string]any) map[string]any {
	return map[string]any{
		"nodes": appendGraphItems(base["nodes"], arpGraph["nodes"]),
		"edges": appendGraphItems(base["edges"], arpGraph["edges"]),
	}
}

func appendGraphItems(base, extra any) []map[string]any {
	var result []map[string]any
	if items, ok := base.([]map[string]any); ok {
		result = append(result, items...)
	}
	if items, ok := extra.([]map[string]any); ok {
		result = append(result, items...)
	}
	return result
}

func mergeArpScoutStats(base map[string]any, sensors []map[string]any) map[string]any {
	result := make(map[string]any, len(base))
	for key, value := range base {
		result[key] = value
	}
	totalSensors := intValue(result["totalSensors"]) + len(sensors)
	activeSensors := intValue(result["activeSensors"])
	newDevices := intValueFromStat(result["newDevices"])
	for _, sensor := range sensors {
		if sensor["status"] == "online" {
			activeSensors++
		}
		newDevices += intValue(sensor["events"])
	}
	result["activeSensors"] = activeSensors
	result["totalSensors"] = totalSensors
	if stat, ok := result["newDevices"].(map[string]any); ok {
		clone := make(map[string]any, len(stat))
		for key, value := range stat {
			clone[key] = value
		}
		clone["value"] = newDevices
		result["newDevices"] = clone
	}
	return result
}

func intValue(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func intValueFromStat(value any) int {
	if stat, ok := value.(map[string]any); ok {
		return intValue(stat["value"])
	}
	return 0
}

func authLoginHandler(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(payload.Username) == "" {
		http.Error(w, "username is required", http.StatusUnauthorized)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "user": payload.Username})
}

func clientHandler(source dnsDataSource) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if source == nil {
			http.Error(w, "client metadata requires Neo4j", http.StatusServiceUnavailable)
			return
		}
		ip := strings.TrimPrefix(r.URL.Path, "/api/clients/")
		if decoded, err := url.PathUnescape(ip); err == nil {
			ip = decoded
		}
		ip = strings.TrimSpace(ip)
		if ip == "" || strings.Contains(ip, "/") {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			details, ok, err := withAPITimeoutValue(r.Context(), ip, source.ClientDetails)
			if err != nil {
				log.Printf("read client metadata failed: %v", err)
				http.Error(w, "read client metadata failed", http.StatusInternalServerError)
				return
			}
			if !ok {
				http.NotFound(w, r)
				return
			}
			writeJSON(w, details)
		case http.MethodPatch, http.MethodPost:
			var payload struct {
				ManualName string `json:"manual_name"`
				Hostname   string `json:"hostname"`
				DeviceType string `json:"device_type"`
				Notes      string `json:"notes"`
				ResolveDNS bool   `json:"resolve_dns"`
			}
			if err := decodeJSON(r, &payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			patch := atlas.ClientMetadataPatch{
				IP:         ip,
				ManualName: payload.ManualName,
				Hostname:   payload.Hostname,
				DeviceType: payload.DeviceType,
				Notes:      payload.Notes,
				ResolveDNS: payload.ResolveDNS,
			}
			details, err := withAPITimeoutUpdate(r.Context(), patch, source.UpdateClientMetadata)
			if err != nil {
				log.Printf("update client metadata failed: %v", err)
				http.Error(w, "update client metadata failed", http.StatusInternalServerError)
				return
			}
			writeJSON(w, details)
		default:
			w.Header().Set("Allow", "GET, PATCH, POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	return false
}

func decodeJSON(r *http.Request, value any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}

func appendArpScoutSensors(base []map[string]any, sensors []map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(base)+len(sensors))
	seen := make(map[any]struct{})
	for _, sensor := range append(base, sensors...) {
		id := sensor["id"]
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, sensor)
	}
	return result
}

func filterArpScoutSummaries(base []map[string]any, sensors []map[string]any) []map[string]any {
	persisted := make(map[any]struct{})
	for _, sensor := range base {
		if sensor["type"] == "arp" {
			persisted[sensor["id"]] = struct{}{}
		}
	}
	if len(persisted) == 0 {
		return sensors
	}
	filtered := make([]map[string]any, 0, len(sensors))
	for _, sensor := range sensors {
		if _, ok := persisted[sensor["id"]]; ok {
			continue
		}
		filtered = append(filtered, sensor)
	}
	return filtered
}

func filterArpScoutGraph(base, graph map[string]any) map[string]any {
	persistedSensors := persistentArpSensors(base)
	if len(persistedSensors) == 0 {
		return graph
	}
	nodes, _ := graph["nodes"].([]map[string]any)
	edges, _ := graph["edges"].([]map[string]any)
	removedDeviceNodes := make(map[string]struct{})
	filteredEdges := make([]map[string]any, 0, len(edges))
	for _, edge := range edges {
		source := fmt.Sprint(edge["source"])
		target := fmt.Sprint(edge["target"])
		if _, ok := persistedSensors[source]; ok {
			removedDeviceNodes[target] = struct{}{}
			continue
		}
		filteredEdges = append(filteredEdges, edge)
	}
	keptIDs := make(map[string]struct{})
	for _, edge := range filteredEdges {
		keptIDs[fmt.Sprint(edge["source"])] = struct{}{}
		keptIDs[fmt.Sprint(edge["target"])] = struct{}{}
	}
	filteredNodes := make([]map[string]any, 0, len(nodes))
	for _, node := range nodes {
		id := fmt.Sprint(node["id"])
		if _, ok := persistedSensors[id]; ok {
			continue
		}
		if _, removed := removedDeviceNodes[id]; removed {
			if _, kept := keptIDs[id]; !kept {
				continue
			}
		}
		filteredNodes = append(filteredNodes, node)
	}
	return map[string]any{"nodes": filteredNodes, "edges": filteredEdges}
}

func persistentArpSensors(graph map[string]any) map[string]struct{} {
	persisted := make(map[string]struct{})
	nodes, _ := graph["nodes"].([]map[string]any)
	for _, node := range nodes {
		id := fmt.Sprint(node["id"])
		if strings.HasPrefix(id, "arp-sensor:") {
			persisted[id] = struct{}{}
		}
	}
	return persisted
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
