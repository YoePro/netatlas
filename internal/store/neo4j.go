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
}

func NewNeo4jStore(ctx context.Context, cfg *config.Config) (*Neo4jStore, error) {
	if cfg.DryRun {
		return &Neo4jStore{dryRun: true}, nil
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

	return &Neo4jStore{
		driver:   driver,
		database: cfg.Neo4jDatabase,
	}, nil
}

func (s *Neo4jStore) Close(ctx context.Context) error {
	if s.driver == nil {
		return nil
	}
	return s.driver.Close(ctx)
}

func (s *Neo4jStore) WriteBatch(ctx context.Context, batch []model.DNSEvent) error {
	if len(batch) == 0 {
		return nil
	}
	if s.dryRun {
		log.Printf("[dry-run] would write %d dns events", len(batch))
		return nil
	}

	session := s.driver.NewSession(ctx, neo4j.SessionConfig{
		DatabaseName: s.database,
		AccessMode:   neo4j.AccessModeWrite,
	})
	defer session.Close(ctx)

	events := make([]map[string]any, 0, len(batch))
	for _, event := range batch {
		events = append(events, map[string]any{
			"timestamp":    event.Timestamp,
			"serverName":   event.ServerName,
			"serverRole":   event.ServerRole,
			"clientIP":     event.ClientIP,
			"queryName":    event.QueryName,
			"queryType":    event.QueryType,
			"responseCode": event.ResponseCode,
			"answerIP":     event.AnswerIP,
			"protocol":     event.Protocol,
			"rawLine":      event.RawLine,
			"rawHash":      event.RawHash,
		})
	}

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

const writeEventsCypher = `
UNWIND $events AS event
MERGE (server:DnsServer {name: event.serverName})
  ON CREATE SET server.role = event.serverRole
SET server.lastSeen = event.timestamp
MERGE (client:Client {ip: event.clientIP})
SET client.lastSeen = event.timestamp
MERGE (domain:Domain {name: event.queryName})
SET domain.lastSeen = event.timestamp
MERGE (queryType:QueryType {name: event.queryType})
MERGE (dnsEvent:DnsEvent {rawHash: event.rawHash})
  ON CREATE SET
    dnsEvent.timestamp = event.timestamp,
    dnsEvent.responseCode = event.responseCode,
    dnsEvent.protocol = event.protocol,
    dnsEvent.rawLine = event.rawLine,
    dnsEvent.firstSeen = event.timestamp
SET dnsEvent.lastSeen = event.timestamp
MERGE (server)-[:OBSERVED]->(dnsEvent)
MERGE (client)-[:ASKED]->(dnsEvent)
MERGE (dnsEvent)-[:FOR_DOMAIN]->(domain)
MERGE (dnsEvent)-[:QUERY_TYPE]->(queryType)
FOREACH (_ IN CASE WHEN event.answerIP = "" THEN [] ELSE [1] END |
  MERGE (answer:IpAddress {address: event.answerIP})
  MERGE (dnsEvent)-[:ANSWERED_WITH]->(answer)
)
`
