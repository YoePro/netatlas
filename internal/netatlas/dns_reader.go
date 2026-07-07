package netatlas

import (
	"context"
	"fmt"
	"math"
	"net"
	"sort"
	"strings"
	"time"

	"netatlas/internal/config"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type DNSReader struct {
	driver   neo4j.DriverWithContext
	database string
}

type ClientMetadataPatch struct {
	IP         string
	ManualName string
	Hostname   string
	DeviceType string
	Notes      string
	ResolveDNS bool
}

func NewDNSReader(ctx context.Context, cfg *config.Config) (*DNSReader, error) {
	driver, err := neo4j.NewDriverWithContext(
		cfg.Neo4jURI,
		neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPassword, ""),
	)
	if err != nil {
		return nil, fmt.Errorf("create neo4j driver: %w", err)
	}
	if err := driver.VerifyConnectivity(ctx); err != nil {
		_ = driver.Close(ctx)
		return nil, fmt.Errorf("verify neo4j connectivity: %w", err)
	}
	return &DNSReader{driver: driver, database: cfg.Neo4jDatabase}, nil
}

func (r *DNSReader) Close(ctx context.Context) error {
	if r == nil || r.driver == nil {
		return nil
	}
	return r.driver.Close(ctx)
}

func (r *DNSReader) SensorSummaries(ctx context.Context) ([]map[string]any, error) {
	snapshot, err := r.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	var sensors []map[string]any
	if snapshot.Events != 0 || snapshot.Clients != 0 || snapshot.Domains != 0 {
		sensors = append(sensors, dnsSensorSummary(snapshot))
	}
	arpSensors, err := r.arpSensorSummaries(ctx)
	if err != nil {
		return nil, err
	}
	sensors = append(sensors, arpSensors...)
	return sensors, nil
}

func (r *DNSReader) SensorDetails(ctx context.Context, id string) (map[string]any, bool, error) {
	if id != "dns-dataset" {
		return r.arpSensorDetails(ctx, id)
	}
	snapshot, err := r.snapshot(ctx)
	if err != nil {
		return nil, false, err
	}
	topDomains, err := r.topDomains(ctx, 8)
	if err != nil {
		return nil, false, err
	}
	details := dnsSensorSummary(snapshot)
	details["cpu"] = 0
	details["memory"] = 0
	details["disk"] = 0
	details["config"] = map[string]any{
		"source":       "neo4j",
		"dns_servers":  snapshot.Servers,
		"clients":      snapshot.Clients,
		"domains":      snapshot.Domains,
		"first_seen":   formatTime(snapshot.FirstSeen),
		"last_seen":    formatTime(snapshot.LastSeen),
		"data_window":  dataWindow(snapshot.FirstSeen, snapshot.LastSeen),
		"dataset_note": "historical DNS data",
	}
	details["recentErrors"] = []map[string]any{}
	details["topDomains"] = topDomains
	details["timeline"] = r.timelineFromSnapshot(snapshot)
	return details, true, nil
}

func (r *DNSReader) Stats(ctx context.Context) (map[string]any, error) {
	snapshot, err := r.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	topTalkers, err := r.topTalkers(ctx, 5)
	if err != nil {
		return nil, err
	}
	recentDomains, err := r.recentDomains(ctx, 5)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"activeSensors": 1,
		"totalSensors":  1,
		"dnsRequests":   map[string]any{"value": snapshot.Events, "delta": 0},
		"newDevices":    map[string]any{"value": snapshot.Clients, "delta": 0},
		"newDomains":    map[string]any{"value": snapshot.Domains, "delta": 0},
		"alerts":        map[string]any{"value": snapshot.NXDomains, "delta": 0},
		"topTalkers":    topTalkers,
		"recentDomains": recentDomains,
		"timeline":      r.timelineFromSnapshot(snapshot),
	}, nil
}

func (r *DNSReader) Graph(ctx context.Context) (map[string]any, error) {
	topClients, err := r.topClients(ctx, 25)
	if err != nil {
		return nil, err
	}
	topDomains, err := r.domainNodes(ctx, 35)
	if err != nil {
		return nil, err
	}
	edges, err := r.clientDomainEdges(ctx, 80)
	if err != nil {
		return nil, err
	}

	nodes := []map[string]any{{
		"id":      "dns-sensor:dns-dataset",
		"label":   "DNS dataset",
		"type":    "sensor",
		"status":  "online",
		"source":  "neo4j",
		"queries": sumCounts(edges),
	}}
	clientIDs := make(map[string]struct{})
	domainIDs := make(map[string]struct{})
	for _, client := range topClients {
		id := fmt.Sprintf("dns-client:%s", client["ip"])
		label := clientDisplayName(client)
		clientIDs[id] = struct{}{}
		nodes = append(nodes, map[string]any{
			"id":            id,
			"label":         label,
			"type":          "client",
			"ip":            client["ip"],
			"hostname":      firstString(client["hostname"], client["dnsName"]),
			"manualName":    client["manualName"],
			"dnsName":       client["dnsName"],
			"deviceType":    client["deviceType"],
			"notes":         client["notes"],
			"displaySource": clientDisplaySource(client),
			"queries":       client["queries"],
			"source":        "neo4j",
		})
	}
	for _, domain := range topDomains {
		name, _ := domain["domain"].(string)
		id := "dns-domain:" + name
		domainIDs[id] = struct{}{}
		nodes = append(nodes, map[string]any{
			"id":       id,
			"label":    name,
			"type":     "domain",
			"category": "dns",
			"queries":  domain["queries"],
			"source":   "neo4j",
		})
	}

	graphEdges := make([]map[string]any, 0, len(edges)+len(topClients))
	for _, client := range topClients {
		id := fmt.Sprintf("dns-client:%s", client["ip"])
		graphEdges = append(graphEdges, map[string]any{
			"source": id,
			"target": "dns-sensor:dns-dataset",
			"type":   "observed_by",
			"count":  client["queries"],
		})
	}
	for _, edge := range edges {
		source := fmt.Sprintf("dns-client:%s", edge["client"])
		target := fmt.Sprintf("dns-domain:%s", edge["domain"])
		if _, ok := clientIDs[source]; !ok {
			continue
		}
		if _, ok := domainIDs[target]; !ok {
			continue
		}
		graphEdges = append(graphEdges, map[string]any{
			"source": source,
			"target": target,
			"type":   "query",
			"count":  edge["count"],
		})
	}

	graph := map[string]any{"nodes": nodes, "edges": graphEdges}
	arpGraph, err := r.arpGraph(ctx)
	if err != nil {
		return nil, err
	}
	return MergeGraph(graph, arpGraph), nil
}

func (r *DNSReader) ClientDetails(ctx context.Context, ip string) (map[string]any, bool, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return nil, false, nil
	}
	row, err := r.readOne(ctx, clientDetailsCypher, map[string]any{"ip": ip})
	if err != nil {
		return nil, false, err
	}
	if row["ip"] == nil {
		return nil, false, nil
	}
	row["label"] = clientDisplayName(row)
	row["displaySource"] = clientDisplaySource(row)
	return row, true, nil
}

func (r *DNSReader) UpdateClientMetadata(ctx context.Context, patch ClientMetadataPatch) (map[string]any, error) {
	patch.IP = strings.TrimSpace(patch.IP)
	if patch.IP == "" {
		return nil, fmt.Errorf("client ip is required")
	}
	dnsName := ""
	if patch.ResolveDNS {
		dnsName = lookupDNSName(ctx, patch.IP)
	}
	params := map[string]any{
		"ip":         patch.IP,
		"manualName": strings.TrimSpace(patch.ManualName),
		"hostname":   strings.TrimSpace(patch.Hostname),
		"deviceType": strings.TrimSpace(patch.DeviceType),
		"notes":      strings.TrimSpace(patch.Notes),
		"dnsName":    dnsName,
		"resolveDNS": patch.ResolveDNS,
		"updatedAt":  neo4jUTC(time.Now()),
	}
	if err := r.write(ctx, updateClientMetadataCypher, params); err != nil {
		return nil, err
	}
	details, _, err := r.ClientDetails(ctx, patch.IP)
	return details, err
}

type dnsSnapshot struct {
	Servers   int
	Clients   int
	Domains   int
	Events    int
	NXDomains int
	FirstSeen time.Time
	LastSeen  time.Time
}

func (r *DNSReader) snapshot(ctx context.Context) (dnsSnapshot, error) {
	result, err := r.readOne(ctx, dnsSnapshotCypher, nil)
	if err != nil {
		return dnsSnapshot{}, err
	}
	return dnsSnapshot{
		Servers:   asInt(result["servers"]),
		Clients:   asInt(result["clients"]),
		Domains:   asInt(result["domains"]),
		Events:    asInt(result["events"]),
		NXDomains: asInt(result["nxDomains"]),
		FirstSeen: asTime(result["firstSeen"]),
		LastSeen:  asTime(result["lastSeen"]),
	}, nil
}

func (r *DNSReader) topTalkers(ctx context.Context, limit int) ([]map[string]any, error) {
	rows, err := r.readMany(ctx, topTalkersCypher, map[string]any{"limit": limit})
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		queries := asInt(row["queries"])
		name := clientDisplayName(row)
		items = append(items, map[string]any{
			"host":    row["client"],
			"name":    name,
			"queries": queries,
			"bytes":   fmt.Sprintf("%d queries", queries),
		})
	}
	return items, nil
}

func (r *DNSReader) recentDomains(ctx context.Context, limit int) ([]map[string]any, error) {
	rows, err := r.readMany(ctx, recentDomainsCypher, map[string]any{"limit": limit})
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{
			"domain":     row["domain"],
			"first_seen": relativeSeen(asTime(row["lastSeen"])),
			"category":   "dns",
			"clients":    asInt(row["clients"]),
		})
	}
	return items, nil
}

func (r *DNSReader) topDomains(ctx context.Context, limit int) ([]map[string]any, error) {
	rows, err := r.readMany(ctx, topDomainsCypher, map[string]any{"limit": limit})
	if err != nil {
		return nil, err
	}
	maxQueries := 1
	for _, row := range rows {
		if queries := asInt(row["queries"]); queries > maxQueries {
			maxQueries = queries
		}
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		queries := asInt(row["queries"])
		items = append(items, map[string]any{
			"domain":  row["domain"],
			"queries": queries,
			"pct":     math.Round(float64(queries)/float64(maxQueries)*1000) / 10,
		})
	}
	return items, nil
}

func (r *DNSReader) topClients(ctx context.Context, limit int) ([]map[string]any, error) {
	rows, err := r.readMany(ctx, topClientsCypher, map[string]any{"limit": limit})
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{
			"ip":      row["client"],
			"queries": asInt(row["queries"]),
		})
	}
	return items, nil
}

func (r *DNSReader) domainNodes(ctx context.Context, limit int) ([]map[string]any, error) {
	return r.topDomains(ctx, limit)
}

func (r *DNSReader) clientDomainEdges(ctx context.Context, limit int) ([]map[string]any, error) {
	rows, err := r.readMany(ctx, clientDomainEdgesCypher, map[string]any{"limit": limit})
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{
			"client": row["client"],
			"domain": row["domain"],
			"count":  asInt(row["count"]),
		})
	}
	return items, nil
}

func (r *DNSReader) arpSensorSummaries(ctx context.Context) ([]map[string]any, error) {
	rows, err := r.readMany(ctx, arpSensorSummariesCypher, nil)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		lastSeen := asTime(row["lastSeen"])
		status := sensorStatus(lastSeen)
		items = append(items, map[string]any{
			"id":       row["id"],
			"name":     firstString(row["displayName"], row["id"]),
			"type":     "arp",
			"location": firstString(row["site"], row["hostname"]),
			"version":  firstString(row["version"], "arpscout"),
			"status":   status,
			"lastSeen": relativeSeen(lastSeen),
			"sources":  []string{"ARP", "Neo4j"},
			"events":   asInt(row["events"]),
			"latency":  0,
			"uptime":   uptimeForStatus(status),
			"devices":  asInt(row["devices"]),
		})
	}
	return items, nil
}

func (r *DNSReader) arpSensorDetails(ctx context.Context, id string) (map[string]any, bool, error) {
	row, err := r.readOne(ctx, arpSensorDetailCypher, map[string]any{"id": id})
	if err != nil {
		return nil, false, err
	}
	if row["id"] == nil {
		return nil, false, nil
	}
	devices, err := r.arpTopDevices(ctx, id, 8)
	if err != nil {
		return nil, false, err
	}
	lastSeen := asTime(row["lastSeen"])
	status := sensorStatus(lastSeen)
	details := map[string]any{
		"id":       row["id"],
		"name":     firstString(row["displayName"], row["id"]),
		"type":     "arp",
		"location": firstString(row["site"], row["hostname"]),
		"version":  firstString(row["version"], "arpscout"),
		"status":   status,
		"lastSeen": relativeSeen(lastSeen),
		"sources":  []string{"ARP", "Neo4j"},
		"events":   asInt(row["events"]),
		"latency":  0,
		"uptime":   uptimeForStatus(status),
		"cpu":      0,
		"memory":   0,
		"disk":     0,
		"config": map[string]any{
			"sensor_id":           row["id"],
			"hostname":            row["hostname"],
			"site":                row["site"],
			"interfaces":          row["interfaces"],
			"capabilities":        row["capabilities"],
			"last_batch_at":       formatTime(asTime(row["lastBatchAt"])),
			"last_heartbeat":      formatTime(asTime(row["lastHeartbeat"])),
			"last_change_type":    row["lastChangeType"],
			"last_change_message": row["lastChangeMessage"],
		},
		"recentErrors": []map[string]any{},
		"topDomains":   devices,
		"timeline":     arpTimeline(asInt(row["events"]), lastSeen),
	}
	return details, true, nil
}

func (r *DNSReader) arpTopDevices(ctx context.Context, sensorID string, limit int) ([]map[string]any, error) {
	rows, err := r.readMany(ctx, arpTopDevicesCypher, map[string]any{"id": sensorID, "limit": limit})
	if err != nil {
		return nil, err
	}
	maxCount := 1
	for _, row := range rows {
		if count := asInt(row["count"]); count > maxCount {
			maxCount = count
		}
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		count := asInt(row["count"])
		label := firstString(clientDisplayName(row), row["ip"], row["mac"])
		items = append(items, map[string]any{
			"domain":        label,
			"label":         label,
			"ip":            row["ip"],
			"mac":           row["mac"],
			"vendor":        row["vendor"],
			"manualName":    row["manualName"],
			"hostname":      row["hostname"],
			"dnsName":       row["dnsName"],
			"deviceType":    row["deviceType"],
			"displaySource": clientDisplaySource(row),
			"queries":       count,
			"pct":           math.Round(float64(count)/float64(maxCount)*1000) / 10,
		})
	}
	return items, nil
}

func (r *DNSReader) arpGraph(ctx context.Context) (map[string]any, error) {
	rows, err := r.readMany(ctx, arpGraphCypher, map[string]any{"limit": 80})
	if err != nil {
		return nil, err
	}
	nodes := make([]map[string]any, 0)
	edges := make([]map[string]any, 0)
	seen := make(map[string]struct{})
	sensorNodes := make(map[string]map[string]any)
	for _, row := range rows {
		sensorID := fmt.Sprint(row["sensorID"])
		deviceID := fmt.Sprint(row["deviceKey"])
		sensorNode := "arp-sensor:" + sensorID
		deviceNode := "arp-device:" + deviceID
		if _, ok := seen[sensorNode]; !ok {
			node := map[string]any{
				"id":      sensorNode,
				"label":   firstString(row["displayName"], sensorID),
				"type":    "sensor",
				"status":  sensorStatus(asTime(row["sensorLastSeen"])),
				"queries": 0,
				"source":  "neo4j",
			}
			nodes = append(nodes, node)
			sensorNodes[sensorNode] = node
			seen[sensorNode] = struct{}{}
		}
		if node := sensorNodes[sensorNode]; node != nil {
			node["queries"] = asInt(node["queries"]) + asInt(row["count"])
		}
		if _, ok := seen[deviceNode]; !ok {
			label := clientDisplayName(row)
			nodes = append(nodes, map[string]any{
				"id":            deviceNode,
				"label":         firstString(label, row["ip"], row["mac"], deviceID),
				"type":          "client",
				"ip":            row["ip"],
				"hostname":      firstString(row["hostname"], row["dnsName"], row["vendor"], row["mac"], row["ip"]),
				"manualName":    row["manualName"],
				"dnsName":       row["dnsName"],
				"deviceType":    row["deviceType"],
				"notes":         row["notes"],
				"displaySource": clientDisplaySource(row),
				"mac":           row["mac"],
				"source":        "neo4j",
			})
			seen[deviceNode] = struct{}{}
		}
		edges = append(edges, map[string]any{
			"source": sensorNode,
			"target": deviceNode,
			"type":   "arp_seen",
			"count":  row["count"],
		})
	}
	return map[string]any{"nodes": nodes, "edges": edges}, nil
}

func (r *DNSReader) timelineFromSnapshot(snapshot dnsSnapshot) []map[string]any {
	if snapshot.Events == 0 {
		return []map[string]any{}
	}
	now := snapshot.LastSeen
	if now.IsZero() {
		now = time.Now()
	}
	points := make([]map[string]any, 0, 30)
	daily := snapshot.Events / 30
	if daily < 1 {
		daily = 1
	}
	for i := 29; i >= 0; i-- {
		points = append(points, map[string]any{
			"t":       now.AddDate(0, 0, -i).UnixMilli(),
			"queries": daily,
			"alerts":  snapshot.NXDomains / 30,
		})
	}
	return points
}

func (r *DNSReader) readOne(ctx context.Context, cypher string, params map[string]any) (map[string]any, error) {
	rows, err := r.readMany(ctx, cypher, params)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return map[string]any{}, nil
	}
	return rows[0], nil
}

func (r *DNSReader) readMany(ctx context.Context, cypher string, params map[string]any) ([]map[string]any, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: r.database,
		AccessMode:   neo4j.AccessModeRead,
	})
	defer session.Close(ctx)

	value, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		var rows []map[string]any
		for result.Next(ctx) {
			row := make(map[string]any, len(result.Record().Keys))
			for _, key := range result.Record().Keys {
				row[key], _ = result.Record().Get(key)
			}
			rows = append(rows, row)
		}
		if err := result.Err(); err != nil {
			return nil, err
		}
		return rows, nil
	})
	if err != nil {
		return nil, err
	}
	rows, ok := value.([]map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected neo4j result type %T", value)
	}
	return rows, nil
}

func (r *DNSReader) write(ctx context.Context, cypher string, params map[string]any) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: r.database,
		AccessMode:   neo4j.AccessModeWrite,
	})
	defer session.Close(ctx)
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx, cypher, params)
		if err != nil {
			return nil, err
		}
		return result.Consume(ctx)
	})
	return err
}

func dnsSensorSummary(snapshot dnsSnapshot) map[string]any {
	return map[string]any{
		"id":       "dns-dataset",
		"name":     "DNS dataset",
		"type":     "dns",
		"location": dataWindow(snapshot.FirstSeen, snapshot.LastSeen),
		"version":  "neo4j",
		"status":   "online",
		"lastSeen": relativeSeen(snapshot.LastSeen),
		"sources":  []string{"DNS", "Neo4j"},
		"events":   snapshot.Events,
		"latency":  0,
		"uptime":   100,
	}
}

func clientDisplayName(row map[string]any) string {
	return firstString(row["manualName"], row["hostname"], row["dnsName"], row["client"], row["ip"])
}

func clientDisplaySource(row map[string]any) string {
	switch {
	case firstString(row["manualName"]) != "":
		return "manual"
	case firstString(row["hostname"]) != "":
		return "hostname"
	case firstString(row["dnsName"]) != "":
		return "dns"
	default:
		return "ip"
	}
}

func lookupDNSName(ctx context.Context, ip string) string {
	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	names, err := net.DefaultResolver.LookupAddr(lookupCtx, ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	return strings.TrimSuffix(strings.TrimSpace(names[0]), ".")
}

func asInt(value any) int {
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

func asTime(value any) time.Time {
	switch v := value.(type) {
	case time.Time:
		return v
	case neo4j.Time:
		return time.Time(v)
	case neo4j.Date:
		return time.Time(v)
	case neo4j.LocalDateTime:
		return time.Time(v).UTC()
	default:
		return time.Time{}
	}
}

func dataWindow(first, last time.Time) string {
	if first.IsZero() && last.IsZero() {
		return "historical DNS data"
	}
	if first.IsZero() {
		return "through " + formatDate(last)
	}
	if last.IsZero() {
		return "from " + formatDate(first)
	}
	return formatDate(first) + " - " + formatDate(last)
}

func formatDate(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format("2006-01-02")
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Format(time.RFC3339)
}

func relativeSeen(value time.Time) string {
	if value.IsZero() {
		return "unknown"
	}
	days := int(time.Since(value).Hours() / 24)
	if days <= 0 {
		return "today"
	}
	if days == 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
}

func sensorStatus(lastSeen time.Time) string {
	if lastSeen.IsZero() {
		return "offline"
	}
	if time.Since(lastSeen) > 10*time.Minute {
		return "warning"
	}
	return "online"
}

func uptimeForStatus(status string) float64 {
	switch status {
	case "online":
		return 100
	case "warning":
		return 97
	default:
		return 0
	}
}

func firstString(values ...any) string {
	for _, value := range values {
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}

func arpTimeline(events int, lastSeen time.Time) []map[string]any {
	if lastSeen.IsZero() {
		lastSeen = time.Now()
	}
	points := make([]map[string]any, 0, 30)
	daily := events / 30
	if daily < 1 && events > 0 {
		daily = 1
	}
	for i := 29; i >= 0; i-- {
		points = append(points, map[string]any{
			"t":       lastSeen.AddDate(0, 0, -i).UnixMilli(),
			"queries": daily,
			"alerts":  0,
		})
	}
	return points
}

func sumCounts(edges []map[string]any) int {
	total := 0
	for _, edge := range edges {
		total += asInt(edge["count"])
	}
	return total
}

func MergeGraph(base, extra map[string]any) map[string]any {
	return map[string]any{
		"nodes": mergeGraphItems(base["nodes"], extra["nodes"]),
		"edges": mergeGraphItems(base["edges"], extra["edges"]),
	}
}

func mergeGraphItems(left, right any) []map[string]any {
	seen := make(map[string]struct{})
	items := make([]map[string]any, 0)
	for _, source := range []any{left, right} {
		values, ok := source.([]map[string]any)
		if !ok {
			continue
		}
		for _, item := range values {
			key := fmt.Sprint(item["id"])
			if key == "" || key == "<nil>" {
				key = fmt.Sprintf("%s|%s|%s", item["source"], item["target"], item["type"])
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			items = append(items, item)
		}
	}
	return items
}

func SortByCount(items []map[string]any, key string) {
	sort.SliceStable(items, func(i, j int) bool {
		return asInt(items[i][key]) > asInt(items[j][key])
	})
}

const dnsSnapshotCypher = `
MATCH (server:DnsServer)
WITH count(server) AS servers
MATCH (client:Client)
WITH servers, count(client) AS clients
MATCH (domain:Domain)
WITH servers, clients, count(domain) AS domains, min(domain.firstSeen) AS firstDomainSeen, max(domain.lastSeen) AS lastDomainSeen
MATCH ()-[queried:QUERIED]->()
RETURN servers,
       clients,
       domains,
       coalesce(sum(queried.count), 0) AS events,
       coalesce(sum(coalesce(queried.nxCount, 0)), 0) AS nxDomains,
       firstDomainSeen AS firstSeen,
       lastDomainSeen AS lastSeen;`

const topTalkersCypher = `
MATCH (client:Client)-[queried:QUERIED]->(:Domain)
RETURN client.ip AS client,
       client.manualName AS manualName,
       client.hostname AS hostname,
       client.dnsName AS dnsName,
       coalesce(sum(queried.count), 0) AS queries
ORDER BY queries DESC, client ASC
LIMIT $limit;`

const recentDomainsCypher = `
MATCH (:Client)-[queried:QUERIED]->(domain:Domain)
RETURN domain.name AS domain,
       count(queried) AS clients,
       max(queried.lastSeen) AS lastSeen
ORDER BY lastSeen DESC, domain ASC
LIMIT $limit;`

const topDomainsCypher = `
MATCH (:Client)-[queried:QUERIED]->(domain:Domain)
RETURN domain.name AS domain,
       coalesce(sum(queried.count), 0) AS queries
ORDER BY queries DESC, domain ASC
LIMIT $limit;`

const topClientsCypher = `
MATCH (client:Client)-[queried:QUERIED]->(:Domain)
RETURN client.ip AS client,
       client.manualName AS manualName,
       client.hostname AS hostname,
       client.dnsName AS dnsName,
       client.deviceType AS deviceType,
       client.notes AS notes,
       coalesce(sum(queried.count), 0) AS queries
ORDER BY queries DESC, client ASC
LIMIT $limit;`

const clientDomainEdgesCypher = `
MATCH (client:Client)-[queried:QUERIED]->(domain:Domain)
RETURN client.ip AS client,
       domain.name AS domain,
       coalesce(queried.count, 0) AS count
ORDER BY count DESC, client ASC, domain ASC
LIMIT $limit;`

const arpSensorSummariesCypher = `
MATCH (sensor:ArpSensor)
OPTIONAL MATCH (sensor)-[seen:OBSERVED_ARP]->(:Device)
RETURN sensor.id AS id,
       sensor.displayName AS displayName,
       sensor.hostname AS hostname,
       sensor.site AS site,
       sensor.version AS version,
       sensor.lastSeen AS lastSeen,
       coalesce(sum(seen.count), 0) AS events,
       count(seen) AS devices
ORDER BY displayName ASC, id ASC;`

const arpSensorDetailCypher = `
MATCH (sensor:ArpSensor {id: $id})
OPTIONAL MATCH (sensor)-[seen:OBSERVED_ARP]->(:Device)
RETURN sensor.id AS id,
       sensor.displayName AS displayName,
       sensor.hostname AS hostname,
       sensor.site AS site,
       sensor.version AS version,
       sensor.interfaces AS interfaces,
       sensor.capabilities AS capabilities,
       sensor.lastSeen AS lastSeen,
       sensor.lastBatchAt AS lastBatchAt,
       sensor.lastHeartbeat AS lastHeartbeat,
       sensor.lastChangeType AS lastChangeType,
       sensor.lastChangeMessage AS lastChangeMessage,
       coalesce(sum(seen.count), 0) AS events;`

const arpTopDevicesCypher = `
MATCH (:ArpSensor {id: $id})-[seen:OBSERVED_ARP]->(device:Device)
OPTIONAL MATCH (device)-[:HAS_CLIENT]->(client:Client)
RETURN device.key AS deviceKey,
       device.primaryIP AS ip,
       device.primaryMAC AS mac,
       device.vendor AS vendor,
       client.manualName AS manualName,
       client.hostname AS hostname,
       client.dnsName AS dnsName,
       client.deviceType AS deviceType,
       client.notes AS notes,
       coalesce(seen.count, 0) AS count,
       seen.lastSeen AS lastSeen
ORDER BY count DESC, lastSeen DESC
LIMIT $limit;`

const arpGraphCypher = `
MATCH (sensor:ArpSensor)-[seen:OBSERVED_ARP]->(device:Device)
OPTIONAL MATCH (device)-[:HAS_CLIENT]->(client:Client)
RETURN sensor.id AS sensorID,
       sensor.displayName AS displayName,
       sensor.lastSeen AS sensorLastSeen,
       device.key AS deviceKey,
       device.primaryIP AS ip,
       device.primaryMAC AS mac,
       device.vendor AS vendor,
       client.manualName AS manualName,
       client.hostname AS hostname,
       client.dnsName AS dnsName,
       client.deviceType AS deviceType,
       client.notes AS notes,
       coalesce(seen.count, 0) AS count,
       seen.lastSeen AS lastSeen
ORDER BY count DESC, lastSeen DESC
LIMIT $limit;`

const clientDetailsCypher = `
MATCH (client:Client {ip: $ip})
OPTIONAL MATCH (client)-[queried:QUERIED]->(:Domain)
RETURN client.ip AS ip,
       client.manualName AS manualName,
       client.hostname AS hostname,
       client.dnsName AS dnsName,
       client.deviceType AS deviceType,
       client.notes AS notes,
       client.updatedAt AS updatedAt,
       client.firstSeen AS firstSeen,
       client.lastSeen AS lastSeen,
       coalesce(sum(queried.count), 0) AS queries;`

const updateClientMetadataCypher = `
MERGE (client:Client {ip: $ip})
  ON CREATE SET client.firstSeen = $updatedAt
SET client.manualName = CASE WHEN $manualName = "" THEN NULL ELSE $manualName END,
    client.hostname = CASE WHEN $hostname = "" THEN NULL ELSE $hostname END,
    client.deviceType = CASE WHEN $deviceType = "" THEN NULL ELSE $deviceType END,
    client.notes = CASE WHEN $notes = "" THEN NULL ELSE $notes END,
    client.dnsName = CASE
      WHEN $resolveDNS AND $dnsName <> "" THEN $dnsName
      WHEN $resolveDNS AND $dnsName = "" THEN client.dnsName
      ELSE client.dnsName
    END,
    client.updatedAt = $updatedAt,
    client.lastSeen = CASE WHEN client.lastSeen IS NULL THEN $updatedAt ELSE client.lastSeen END;`
