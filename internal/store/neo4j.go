package store

import (
	"context"
	"fmt"
	"log"

	"dnslog/internal/config"
	"dnslog/internal/model"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type EventStore interface {
	Close(ctx context.Context) error
	WriteBatch(ctx context.Context, batch []model.DNSEvent) error
}

type Neo4jStore struct {
	driver   neo4j.DriverWithContext
	database string
	dryRun   bool
	debug    bool
}

func NewNeo4jStore(ctx context.Context, cfg *config.Config) (*Neo4jStore, error) {
	if cfg.DryRun {
		return &Neo4jStore{dryRun: true, debug: cfg.Debug}, nil
	}

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

	store := &Neo4jStore{
		driver:   driver,
		database: cfg.Neo4jDatabase,
		debug:    cfg.Debug,
	}
	if err := store.EnsureSchema(ctx); err != nil {
		_ = driver.Close(ctx)
		return nil, err
	}

	return store, nil
}

func (s *Neo4jStore) Close(ctx context.Context) error {
	if s.driver == nil {
		return nil
	}
	return s.driver.Close(ctx)
}

func (s *Neo4jStore) EnsureSchema(ctx context.Context) error {
	if s.dryRun {
		if s.debug {
			log.Printf("[dry-run] would ensure neo4j schema")
		}
		return nil
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: s.database,
		AccessMode:   neo4j.AccessModeWrite,
	})
	defer session.Close(ctx)

	for _, statement := range schemaStatements {
		if _, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
			result, err := tx.Run(ctx, statement, nil)
			if err != nil {
				return nil, err
			}
			return result.Consume(ctx)
		}); err != nil {
			return fmt.Errorf("ensure neo4j schema: %w", err)
		}
	}

	return nil
}

func (s *Neo4jStore) WriteBatch(ctx context.Context, batch []model.DNSEvent) error {
	if len(batch) == 0 {
		return nil
	}
	if s.dryRun {
		if s.debug {
			log.Printf("[dry-run] would write %d dns events", len(batch))
		}
		return nil
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: s.database,
		AccessMode:   neo4j.AccessModeWrite,
	})
	defer session.Close(ctx)

	events := eventParams(batch)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		result, err := tx.Run(ctx, writeEventsCypher, map[string]any{"events": events})
		if err != nil {
			return nil, err
		}
		return result.Consume(ctx)
	})
	if err != nil {
		return fmt.Errorf("write neo4j batch: %w", err)
	}

	return nil
}

func eventParams(batch []model.DNSEvent) []map[string]any {
	events := make([]map[string]any, 0, len(batch))
	for _, event := range batch {
		events = append(events, map[string]any{
			"timestamp":      event.Timestamp,
			"serverName":     event.ServerName,
			"serverRole":     event.ServerRole,
			"clientIP":       event.ClientIP,
			"queryName":      event.QueryName,
			"queryClass":     event.QueryClass,
			"queryType":      event.QueryType,
			"responseCode":   event.ResponseCode,
			"answerIP":       event.AnswerIP,
			"protocol":       event.Protocol,
			"sourceCategory": event.SourceCategory,
			"rawLine":        event.RawLine,
			"rawHash":        event.RawHash,
		})
	}
	return events
}

var schemaStatements = []string{
	"CREATE CONSTRAINT dns_server_name IF NOT EXISTS FOR (n:DnsServer) REQUIRE n.name IS UNIQUE",
	"CREATE CONSTRAINT client_ip IF NOT EXISTS FOR (n:Client) REQUIRE n.ip IS UNIQUE",
	"CREATE CONSTRAINT domain_name IF NOT EXISTS FOR (n:Domain) REQUIRE n.name IS UNIQUE",
	"CREATE CONSTRAINT query_type_name IF NOT EXISTS FOR (n:QueryType) REQUIRE n.name IS UNIQUE",
	"CREATE CONSTRAINT ip_address_address IF NOT EXISTS FOR (n:IpAddress) REQUIRE n.address IS UNIQUE",
	"CREATE CONSTRAINT dns_event_raw_hash IF NOT EXISTS FOR (n:DnsEvent) REQUIRE n.rawHash IS UNIQUE",
	"CREATE INDEX dns_event_timestamp IF NOT EXISTS FOR (n:DnsEvent) ON (n.timestamp)",
	"CREATE INDEX domain_last_seen IF NOT EXISTS FOR (n:Domain) ON (n.lastSeen)",
	"CREATE INDEX client_last_seen IF NOT EXISTS FOR (n:Client) ON (n.lastSeen)",
	"CREATE INDEX queried_count IF NOT EXISTS FOR ()-[r:QUERIED]-() ON (r.count)",
	"CREATE INDEX queried_last_seen IF NOT EXISTS FOR ()-[r:QUERIED]-() ON (r.lastSeen)",
}

const writeEventsCypher = `
UNWIND $events AS event
MERGE (server:DnsServer {name: event.serverName})
  ON CREATE SET
    server.role = event.serverRole,
    server.firstSeen = event.timestamp
SET server.role = event.serverRole,
    server.firstSeen = CASE WHEN server.firstSeen IS NULL OR event.timestamp < server.firstSeen THEN event.timestamp ELSE server.firstSeen END,
    server.lastSeen = CASE WHEN server.lastSeen IS NULL OR event.timestamp > server.lastSeen THEN event.timestamp ELSE server.lastSeen END
MERGE (client:Client {ip: event.clientIP})
  ON CREATE SET client.firstSeen = event.timestamp
SET client.firstSeen = CASE WHEN client.firstSeen IS NULL OR event.timestamp < client.firstSeen THEN event.timestamp ELSE client.firstSeen END,
    client.lastSeen = CASE WHEN client.lastSeen IS NULL OR event.timestamp > client.lastSeen THEN event.timestamp ELSE client.lastSeen END
MERGE (domain:Domain {name: event.queryName})
  ON CREATE SET domain.firstSeen = event.timestamp
SET domain.firstSeen = CASE WHEN domain.firstSeen IS NULL OR event.timestamp < domain.firstSeen THEN event.timestamp ELSE domain.firstSeen END,
    domain.lastSeen = CASE WHEN domain.lastSeen IS NULL OR event.timestamp > domain.lastSeen THEN event.timestamp ELSE domain.lastSeen END
MERGE (queryType:QueryType {name: event.queryType})
  ON CREATE SET queryType.firstSeen = event.timestamp
SET queryType.firstSeen = CASE WHEN queryType.firstSeen IS NULL OR event.timestamp < queryType.firstSeen THEN event.timestamp ELSE queryType.firstSeen END,
    queryType.lastSeen = CASE WHEN queryType.lastSeen IS NULL OR event.timestamp > queryType.lastSeen THEN event.timestamp ELSE queryType.lastSeen END
MERGE (dnsEvent:DnsEvent {rawHash: event.rawHash})
  ON CREATE SET
    dnsEvent.timestamp = event.timestamp,
    dnsEvent.serverName = event.serverName,
    dnsEvent.serverRole = event.serverRole,
    dnsEvent.clientIP = event.clientIP,
    dnsEvent.queryName = event.queryName,
    dnsEvent.queryClass = event.queryClass,
    dnsEvent.queryType = event.queryType,
    dnsEvent.responseCode = event.responseCode,
    dnsEvent.protocol = event.protocol,
    dnsEvent.sourceCategory = event.sourceCategory,
    dnsEvent.rawLine = event.rawLine,
    dnsEvent.firstSeen = event.timestamp,
    dnsEvent.aggregateApplied = false
SET dnsEvent.firstSeen = CASE WHEN dnsEvent.firstSeen IS NULL OR event.timestamp < dnsEvent.firstSeen THEN event.timestamp ELSE dnsEvent.firstSeen END,
    dnsEvent.lastSeen = CASE WHEN dnsEvent.lastSeen IS NULL OR event.timestamp > dnsEvent.lastSeen THEN event.timestamp ELSE dnsEvent.lastSeen END
MERGE (server)-[:OBSERVED]->(dnsEvent)
MERGE (client)-[:ASKED]->(dnsEvent)
MERGE (dnsEvent)-[:FOR_DOMAIN]->(domain)
MERGE (dnsEvent)-[:QUERY_TYPE]->(queryType)
FOREACH (_ IN CASE WHEN event.answerIP = "" THEN [] ELSE [1] END |
  MERGE (answer:IpAddress {address: event.answerIP})
  ON CREATE SET answer.firstSeen = event.timestamp
  SET answer.firstSeen = CASE WHEN answer.firstSeen IS NULL OR event.timestamp < answer.firstSeen THEN event.timestamp ELSE answer.firstSeen END,
      answer.lastSeen = CASE WHEN answer.lastSeen IS NULL OR event.timestamp > answer.lastSeen THEN event.timestamp ELSE answer.lastSeen END
  MERGE (dnsEvent)-[:ANSWERED_WITH]->(answer)
)
WITH event, client, domain, dnsEvent, coalesce(dnsEvent.aggregateApplied, false) = false AS shouldAggregate
MERGE (client)-[queried:QUERIED]->(domain)
  ON CREATE SET
    queried.count = 0,
    queried.nxCount = 0,
    queried.queryTypes = [],
    queried.serverSeenOn = [],
    queried.firstSeen = event.timestamp
SET queried.serverSeenOn = CASE
      WHEN event.serverName = "" OR event.serverName IN coalesce(queried.serverSeenOn, []) THEN coalesce(queried.serverSeenOn, [])
      ELSE coalesce(queried.serverSeenOn, []) + event.serverName
    END
FOREACH (_ IN CASE WHEN shouldAggregate THEN [1] ELSE [] END |
  SET queried.count = coalesce(queried.count, 0) + 1,
      queried.nxCount = coalesce(queried.nxCount, 0) + CASE WHEN toUpper(coalesce(event.responseCode, "")) = "NXDOMAIN" THEN 1 ELSE 0 END,
      queried.firstSeen = CASE WHEN queried.firstSeen IS NULL OR event.timestamp < queried.firstSeen THEN event.timestamp ELSE queried.firstSeen END,
      queried.lastSeen = CASE WHEN queried.lastSeen IS NULL OR event.timestamp > queried.lastSeen THEN event.timestamp ELSE queried.lastSeen END,
      queried.lastResponseCode = event.responseCode,
      queried.queryTypes = CASE
        WHEN event.queryType = "" OR event.queryType IN coalesce(queried.queryTypes, []) THEN coalesce(queried.queryTypes, [])
        ELSE coalesce(queried.queryTypes, []) + event.queryType
      END,
      dnsEvent.aggregateApplied = true
)
`
