package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"netatlas/internal/config"
	"netatlas/internal/fingerprint"
	"netatlas/internal/model"
)

func TestNewNeo4jStoreCarriesDebugFlagInDryRun(t *testing.T) {
	store, err := NewNeo4jStore(context.Background(), &config.Config{
		DryRun: true,
		Debug:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !store.dryRun {
		t.Fatal("dryRun = false, want true")
	}
	if !store.debug {
		t.Fatal("debug = false, want true")
	}
}

func TestEventParamsIncludesGraphProperties(t *testing.T) {
	timestamp := time.Date(2026, 7, 4, 22, 13, 1, 0, time.UTC)
	params := eventParams([]model.DNSEvent{
		{
			Timestamp:      timestamp,
			ServerName:     "dns-primary",
			ServerRole:     "primary",
			ClientIP:       "192.168.1.10",
			QueryName:      "example.com",
			QueryClass:     "IN",
			QueryType:      "A",
			ResponseCode:   "NOERROR",
			AnswerIP:       "93.184.216.34",
			Protocol:       "udp",
			SourceCategory: "queries",
			RawLine:        "raw",
			RawHash:        "hash",
		},
	})

	if len(params) != 1 {
		t.Fatalf("len(params) = %d, want 1", len(params))
	}

	event := params[0]
	assertParam(t, event, "timestamp", timestamp)
	assertParam(t, event, "serverName", "dns-primary")
	assertParam(t, event, "serverRole", "primary")
	assertParam(t, event, "clientIP", "192.168.1.10")
	assertParam(t, event, "queryName", "example.com")
	assertParam(t, event, "queryClass", "IN")
	assertParam(t, event, "queryType", "A")
	assertParam(t, event, "responseCode", "NOERROR")
	assertParam(t, event, "answerIP", "93.184.216.34")
	assertParam(t, event, "protocol", "udp")
	assertParam(t, event, "sourceCategory", "queries")
	assertParam(t, event, "rawLine", "raw")
	assertParam(t, event, "rawHash", "hash")
}

func TestSchemaStatementsCoverGraphIdentity(t *testing.T) {
	want := []string{
		"DnsServer",
		"Device",
		"Client",
		"Domain",
		"QueryType",
		"IpAddress",
		"DnsEvent",
		"Fingerprint",
		"OperatingSystem",
		"DeviceType",
		"Software",
		"InfrastructureRole",
		"Vendor",
		"QUERIED",
	}

	joined := ""
	for _, statement := range schemaStatements {
		joined += statement + "\n"
	}
	for _, label := range want {
		if !strings.Contains(joined, label) {
			t.Fatalf("schemaStatements missing %s", label)
		}
	}
}

func TestEnrichmentParamsIncludesEvidenceProperties(t *testing.T) {
	timestamp := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	params := enrichmentParams([]fingerprint.Evidence{
		{
			DeviceKey:     "ip:192.168.1.10",
			Category:      fingerprint.CategoryOperatingSystem,
			Target:        "Windows",
			Score:         35,
			Confidence:    "low",
			Timestamp:     timestamp,
			EvidenceHash:  "hash",
			FingerprintID: "os-windows-update",
			MatchedDomain: "windowsupdate.com",
		},
	})
	if len(params) != 1 {
		t.Fatalf("len(params) = %d, want 1", len(params))
	}
	item := params[0]
	assertParam(t, item, "deviceKey", "ip:192.168.1.10")
	assertParam(t, item, "category", fingerprint.CategoryOperatingSystem)
	assertParam(t, item, "target", "Windows")
	assertParam(t, item, "score", 35)
	assertParam(t, item, "confidence", "low")
	assertParam(t, item, "timestamp", timestamp)
	assertParam(t, item, "evidenceHash", "hash")
	assertParam(t, item, "fingerprintID", "os-windows-update")
	assertParam(t, item, "matchedDomain", "windowsupdate.com")
}

func TestWriteEnrichmentsCypherCoversCategoriesAndEvidence(t *testing.T) {
	for _, want := range []string{
		"LIKELY_RUNNING",
		"LIKELY_IS",
		"LIKELY_HAS",
		"LIKELY_INFRASTRUCTURE",
		"LIKELY_VENDOR",
		"evidenceCount",
		"evidenceHashes",
		"confidence",
		"score",
		"lastMatchedDomain",
	} {
		if !strings.Contains(writeEnrichmentsCypher, want) {
			t.Fatalf("writeEnrichmentsCypher missing %q", want)
		}
	}
}

func TestWriteEventsCypherMaintainsAggregateRelationship(t *testing.T) {
	for _, want := range []string{
		"MERGE (client)-[queried:QUERIED]->(domain)",
		"AS shouldAggregate",
		"CASE WHEN shouldAggregate THEN [1] ELSE [] END",
		"queried.count",
		"queried.nxCount",
		"queried.queryTypes",
		"queried.serverSeenOn",
		"queried.lastResponseCode",
		"dnsEvent.aggregateApplied",
	} {
		if !strings.Contains(writeEventsCypher, want) {
			t.Fatalf("writeEventsCypher missing %q", want)
		}
	}
}

func TestWriteEventsCypherCreatesDeviceIdentity(t *testing.T) {
	for _, want := range []string{
		"MERGE (device:Device {key: \"ip:\" + event.clientIP})",
		"device.primaryIP",
		"device.identitySource",
		"MERGE (device)-[:HAS_CLIENT]->(client)",
	} {
		if !strings.Contains(writeEventsCypher, want) {
			t.Fatalf("writeEventsCypher missing device identity fragment %q", want)
		}
	}
}

func TestWriteEventsCypherKeepsSeenTimestampsMonotonic(t *testing.T) {
	for _, want := range []string{
		"event.timestamp < server.firstSeen",
		"event.timestamp > server.lastSeen",
		"event.timestamp < device.firstSeen",
		"event.timestamp > device.lastSeen",
		"event.timestamp < client.firstSeen",
		"event.timestamp > client.lastSeen",
		"event.timestamp < domain.firstSeen",
		"event.timestamp > domain.lastSeen",
		"event.timestamp < queryType.firstSeen",
		"event.timestamp > queryType.lastSeen",
		"event.timestamp < dnsEvent.firstSeen",
		"event.timestamp > dnsEvent.lastSeen",
	} {
		if !strings.Contains(writeEventsCypher, want) {
			t.Fatalf("writeEventsCypher missing monotonic timestamp guard %q", want)
		}
	}
}

func assertParam(t *testing.T, event map[string]any, key string, want any) {
	t.Helper()
	if got := event[key]; got != want {
		t.Fatalf("%s = %#v, want %#v", key, got, want)
	}
}
