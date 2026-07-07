package netatlas

import (
	"context"
	"fmt"
	"strings"
	"time"

	"netatlas/internal/arpscout"
	"netatlas/internal/config"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type ArpStore struct {
	driver   neo4j.DriverWithContext
	database string
}

func NewArpStore(ctx context.Context, cfg *config.Config) (*ArpStore, error) {
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
	store := &ArpStore{driver: driver, database: cfg.Neo4jDatabase}
	if err := store.EnsureSchema(ctx); err != nil {
		_ = driver.Close(ctx)
		return nil, err
	}
	return store, nil
}

func (s *ArpStore) Close(ctx context.Context) error {
	if s == nil || s.driver == nil {
		return nil
	}
	return s.driver.Close(ctx)
}

func (s *ArpStore) EnsureSchema(ctx context.Context) error {
	for _, statement := range arpSchemaStatements {
		if err := s.write(ctx, statement, nil); err != nil {
			return fmt.Errorf("ensure arpscout schema: %w", err)
		}
	}
	return nil
}

func (s *ArpStore) Register(ctx context.Context, identity arpscout.Identity) error {
	return s.write(ctx, writeArpIdentityCypher, map[string]any{"sensor": arpIdentityParams(identity, time.Now())})
}

func (s *ArpStore) Heartbeat(ctx context.Context, identity arpscout.Identity, status arpscout.DaemonStatus) error {
	params := arpIdentityParams(identity, time.Now())
	params["startedAt"] = neo4jUTC(status.StartedAt)
	params["lastRunAt"] = neo4jUTC(status.LastRunAt)
	params["iterations"] = status.Iterations
	params["observations"] = status.Observations
	params["changes"] = status.Changes
	params["lastError"] = status.LastError
	return s.write(ctx, writeArpHeartbeatCypher, map[string]any{"sensor": params})
}

func (s *ArpStore) WriteBatch(ctx context.Context, batch arpscout.ObservationBatch) error {
	if strings.TrimSpace(batch.SensorID) == "" {
		return nil
	}
	params := map[string]any{
		"sensorID":     batch.SensorID,
		"capturedAt":   neo4jUTC(batch.CapturedAt),
		"observations": arpObservationParams(batch.Observations, batch.CapturedAt),
		"changes":      arpChangeParams(batch.Changes, batch.CapturedAt),
	}
	return s.write(ctx, writeArpBatchCypher, params)
}

func (s *ArpStore) write(ctx context.Context, cypher string, params map[string]any) error {
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: s.database,
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

func arpIdentityParams(identity arpscout.Identity, seen time.Time) map[string]any {
	return map[string]any{
		"id":           identity.SensorID,
		"displayName":  identity.DisplayName,
		"version":      identity.Version,
		"hostname":     identity.Hostname,
		"site":         identity.Site,
		"interfaces":   identity.Interfaces,
		"capabilities": identity.Capabilities,
		"seenAt":       neo4jUTC(seen),
	}
}

func arpObservationParams(observations []arpscout.Observation, fallback time.Time) []map[string]any {
	items := make([]map[string]any, 0, len(observations))
	for _, observation := range observations {
		if strings.TrimSpace(observation.IP) == "" {
			continue
		}
		observed := observation.Observed
		if observed.IsZero() {
			observed = fallback
		}
		vendor := ""
		if observation.Vendor != nil {
			vendor = *observation.Vendor
		}
		deviceKey := "ip:" + observation.IP
		if strings.TrimSpace(observation.MAC) != "" {
			deviceKey = "mac:" + strings.ToLower(observation.MAC)
		}
		items = append(items, map[string]any{
			"deviceKey": deviceKey,
			"ip":        observation.IP,
			"mac":       strings.ToLower(observation.MAC),
			"vendor":    vendor,
			"interface": observation.Interface,
			"state":     observation.State,
			"source":    observation.Source,
			"observed":  neo4jUTC(observed),
		})
	}
	return items
}

func arpChangeParams(changes []arpscout.ChangeEvent, fallback time.Time) []map[string]any {
	items := make([]map[string]any, 0, len(changes))
	for _, change := range changes {
		observed := change.ObservedAt
		if observed.IsZero() {
			observed = fallback
		}
		items = append(items, map[string]any{
			"type":        change.Type,
			"ip":          change.IP,
			"mac":         strings.ToLower(change.MAC),
			"previousIP":  change.PreviousIP,
			"currentIP":   change.CurrentIP,
			"previousMAC": strings.ToLower(change.PreviousMAC),
			"currentMAC":  strings.ToLower(change.CurrentMAC),
			"message":     change.Message,
			"observed":    neo4jUTC(observed),
		})
	}
	return items
}

func neo4jUTC(value time.Time) time.Time {
	if value.IsZero() {
		return value
	}
	return value.UTC()
}

var arpSchemaStatements = []string{
	"CREATE CONSTRAINT arp_sensor_id IF NOT EXISTS FOR (n:ArpSensor) REQUIRE n.id IS UNIQUE",
	"CREATE CONSTRAINT mac_address_address IF NOT EXISTS FOR (n:MacAddress) REQUIRE n.address IS UNIQUE",
	"CREATE INDEX arp_sensor_last_seen IF NOT EXISTS FOR (n:ArpSensor) ON (n.lastSeen)",
	"CREATE INDEX mac_address_last_seen IF NOT EXISTS FOR (n:MacAddress) ON (n.lastSeen)",
	"CREATE INDEX device_primary_mac IF NOT EXISTS FOR (n:Device) ON (n.primaryMAC)",
	"CREATE INDEX arp_observed_last_seen IF NOT EXISTS FOR ()-[r:OBSERVED_ARP]-() ON (r.lastSeen)",
}

const writeArpIdentityCypher = `
MERGE (sensor:ArpSensor {id: $sensor.id})
  ON CREATE SET sensor.firstSeen = $sensor.seenAt
SET sensor.displayName = $sensor.displayName,
    sensor.version = $sensor.version,
    sensor.hostname = $sensor.hostname,
    sensor.site = $sensor.site,
    sensor.interfaces = $sensor.interfaces,
    sensor.capabilities = $sensor.capabilities,
    sensor.lastSeen = CASE WHEN sensor.lastSeen IS NULL OR $sensor.seenAt > sensor.lastSeen THEN $sensor.seenAt ELSE sensor.lastSeen END;`

const writeArpHeartbeatCypher = `
MERGE (sensor:ArpSensor {id: $sensor.id})
  ON CREATE SET sensor.firstSeen = $sensor.seenAt
SET sensor.displayName = $sensor.displayName,
    sensor.version = $sensor.version,
    sensor.hostname = $sensor.hostname,
    sensor.site = $sensor.site,
    sensor.interfaces = $sensor.interfaces,
    sensor.capabilities = $sensor.capabilities,
    sensor.startedAt = $sensor.startedAt,
    sensor.lastRunAt = $sensor.lastRunAt,
    sensor.iterations = $sensor.iterations,
    sensor.observations = $sensor.observations,
    sensor.changes = $sensor.changes,
    sensor.lastError = $sensor.lastError,
    sensor.lastHeartbeat = $sensor.seenAt,
    sensor.lastSeen = CASE WHEN sensor.lastSeen IS NULL OR $sensor.seenAt > sensor.lastSeen THEN $sensor.seenAt ELSE sensor.lastSeen END;`

const writeArpBatchCypher = `
MERGE (sensor:ArpSensor {id: $sensorID})
  ON CREATE SET sensor.firstSeen = $capturedAt
SET sensor.lastSeen = CASE WHEN sensor.lastSeen IS NULL OR $capturedAt > sensor.lastSeen THEN $capturedAt ELSE sensor.lastSeen END,
    sensor.lastBatchAt = $capturedAt
WITH sensor
UNWIND $observations AS observation
MERGE (client:Client {ip: observation.ip})
  ON CREATE SET client.firstSeen = observation.observed
SET client.lastSeen = CASE WHEN client.lastSeen IS NULL OR observation.observed > client.lastSeen THEN observation.observed ELSE client.lastSeen END
MERGE (device:Device {key: observation.deviceKey})
  ON CREATE SET
    device.firstSeen = observation.observed,
    device.identitySource = "arp"
SET device.primaryIP = CASE WHEN observation.ip <> "" THEN observation.ip ELSE device.primaryIP END,
    device.primaryMAC = CASE WHEN observation.mac <> "" THEN observation.mac ELSE device.primaryMAC END,
    device.vendor = CASE WHEN observation.vendor <> "" THEN observation.vendor ELSE device.vendor END,
    device.lastInterface = observation.interface,
    device.lastArpState = observation.state,
    device.lastArpSource = observation.source,
    device.lastSeen = CASE WHEN device.lastSeen IS NULL OR observation.observed > device.lastSeen THEN observation.observed ELSE device.lastSeen END,
    device.firstSeen = CASE WHEN device.firstSeen IS NULL OR observation.observed < device.firstSeen THEN observation.observed ELSE device.firstSeen END
MERGE (device)-[:HAS_CLIENT]->(client)
MERGE (sensor)-[seen:OBSERVED_ARP]->(device)
  ON CREATE SET seen.firstSeen = observation.observed,
                seen.count = 0,
                seen.interfaces = [],
                seen.sources = []
SET seen.count = coalesce(seen.count, 0) + 1,
    seen.lastSeen = CASE WHEN seen.lastSeen IS NULL OR observation.observed > seen.lastSeen THEN observation.observed ELSE seen.lastSeen END,
    seen.interfaces = CASE
      WHEN observation.interface = "" OR observation.interface IN coalesce(seen.interfaces, []) THEN coalesce(seen.interfaces, [])
      ELSE coalesce(seen.interfaces, []) + observation.interface
    END,
    seen.sources = CASE
      WHEN observation.source = "" OR observation.source IN coalesce(seen.sources, []) THEN coalesce(seen.sources, [])
      ELSE coalesce(seen.sources, []) + observation.source
    END
FOREACH (_ IN CASE WHEN observation.mac = "" THEN [] ELSE [1] END |
  MERGE (mac:MacAddress {address: observation.mac})
    ON CREATE SET mac.firstSeen = observation.observed
  SET mac.vendor = CASE WHEN observation.vendor <> "" THEN observation.vendor ELSE mac.vendor END,
      mac.lastSeen = CASE WHEN mac.lastSeen IS NULL OR observation.observed > mac.lastSeen THEN observation.observed ELSE mac.lastSeen END
  MERGE (device)-[:HAS_MAC]->(mac)
)
WITH sensor
UNWIND $changes AS change
SET sensor.lastChangeAt = CASE WHEN sensor.lastChangeAt IS NULL OR change.observed > sensor.lastChangeAt THEN change.observed ELSE sensor.lastChangeAt END,
    sensor.lastChangeType = change.type,
    sensor.lastChangeMessage = change.message;`
