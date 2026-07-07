package netatlas

import (
	"strings"
	"testing"
	"time"

	"netatlas/internal/arpscout"
)

func TestArpObservationParamsUseStableDeviceKeyAndUTC(t *testing.T) {
	vendor := "Raspberry Pi Foundation"
	local := time.FixedZone("Local", 2*60*60)
	observed := time.Date(2026, 7, 7, 12, 0, 0, 0, local)
	params := arpObservationParams([]arpscout.Observation{
		{
			IP:        "192.168.1.10",
			MAC:       "B8:27:EB:12:34:56",
			Vendor:    &vendor,
			Interface: "eth0",
			State:     "REACHABLE",
			Source:    "passive_neigh",
			Observed:  observed,
		},
	}, time.Time{})

	if len(params) != 1 {
		t.Fatalf("len(params) = %d, want 1", len(params))
	}
	item := params[0]
	if item["deviceKey"] != "mac:b8:27:eb:12:34:56" {
		t.Fatalf("deviceKey = %q", item["deviceKey"])
	}
	if item["mac"] != "b8:27:eb:12:34:56" {
		t.Fatalf("mac = %q", item["mac"])
	}
	if item["vendor"] != vendor {
		t.Fatalf("vendor = %q", item["vendor"])
	}
	got, ok := item["observed"].(time.Time)
	if !ok {
		t.Fatalf("observed type = %T", item["observed"])
	}
	if got.Location() != time.UTC || !got.Equal(observed) {
		t.Fatalf("observed = %s, want UTC equivalent of %s", got, observed)
	}
}

func TestArpObservationParamsFallbackToIPKey(t *testing.T) {
	params := arpObservationParams([]arpscout.Observation{{IP: "192.168.1.20"}}, time.Time{})
	if params[0]["deviceKey"] != "ip:192.168.1.20" {
		t.Fatalf("deviceKey = %q", params[0]["deviceKey"])
	}
}

func TestArpStoreCypherUsesAggregateLastSeenModel(t *testing.T) {
	for _, want := range []string{
		"MERGE (sensor:ArpSensor {id: $sensorID})",
		"MERGE (client:Client {ip: observation.ip})",
		"MERGE (device:Device {key: observation.deviceKey})",
		"MERGE (sensor)-[seen:OBSERVED_ARP]->(device)",
		"seen.lastSeen",
		"device.lastSeen",
		"client.lastSeen",
		"MERGE (mac:MacAddress {address: observation.mac})",
	} {
		if !strings.Contains(writeArpBatchCypher, want) {
			t.Fatalf("writeArpBatchCypher missing %q", want)
		}
	}
	for _, avoid := range []string{"CREATE (", "ArpObservation", "rawLine"} {
		if strings.Contains(writeArpBatchCypher, avoid) {
			t.Fatalf("writeArpBatchCypher should not create raw event data containing %q", avoid)
		}
	}
}

func TestArpStoreSchemaCoversStableIdentity(t *testing.T) {
	joined := strings.Join(arpSchemaStatements, "\n")
	for _, want := range []string{"ArpSensor", "MacAddress", "OBSERVED_ARP", "lastSeen"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("arpSchemaStatements missing %q", want)
		}
	}
}
