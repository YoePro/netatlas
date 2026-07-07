package arpscout

import (
	"testing"
	"time"
)

func TestDetectChangesReportsNewAndLostDevices(t *testing.T) {
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	events := DetectChanges(
		[]Observation{{IP: "192.168.1.10", MAC: "00:11:22:33:44:55"}},
		[]Observation{{IP: "192.168.1.20", MAC: "66:77:88:99:aa:bb"}},
		ChangeOptions{Now: now},
	)

	assertHasChange(t, events, EventDeviceNew)
	assertHasChange(t, events, EventDeviceLost)
}

func TestDetectChangesReportsMACChangedForIP(t *testing.T) {
	events := DetectChanges(
		[]Observation{{IP: "192.168.1.10", MAC: "00:11:22:33:44:55"}},
		[]Observation{{IP: "192.168.1.10", MAC: "66:77:88:99:aa:bb"}},
		ChangeOptions{},
	)

	event := findChange(events, EventMACChanged)
	if event == nil {
		t.Fatalf("events = %#v", events)
	}
	if event.IP != "192.168.1.10" || event.PreviousMAC != "00:11:22:33:44:55" || event.CurrentMAC != "66:77:88:99:aa:bb" {
		t.Fatalf("event = %#v", event)
	}
}

func TestDetectChangesReportsIPChangedForMAC(t *testing.T) {
	events := DetectChanges(
		[]Observation{{IP: "192.168.1.10", MAC: "00:11:22:33:44:55"}},
		[]Observation{{IP: "192.168.1.20", MAC: "00:11:22:33:44:55"}},
		ChangeOptions{},
	)

	event := findChange(events, EventIPChanged)
	if event == nil {
		t.Fatalf("events = %#v", events)
	}
	if event.MAC != "00:11:22:33:44:55" || event.PreviousIP != "192.168.1.10" || event.CurrentIP != "192.168.1.20" {
		t.Fatalf("event = %#v", event)
	}
}

func TestDetectChangesReportsDuplicateIP(t *testing.T) {
	events := DetectChanges(nil, []Observation{
		{IP: "192.168.1.10", MAC: "00:11:22:33:44:55"},
		{IP: "192.168.1.10", MAC: "66:77:88:99:aa:bb"},
	}, ChangeOptions{})

	event := findChange(events, EventDuplicateIP)
	if event == nil {
		t.Fatalf("events = %#v", events)
	}
	if event.IP != "192.168.1.10" {
		t.Fatalf("event = %#v", event)
	}
}

func TestDetectChangesReportsGatewayChanged(t *testing.T) {
	events := DetectChanges(
		[]Observation{{IP: "192.168.1.1", MAC: "00:11:22:33:44:55"}},
		[]Observation{{IP: "192.168.1.1", MAC: "66:77:88:99:aa:bb"}},
		ChangeOptions{GatewayIP: "192.168.1.1"},
	)

	assertHasChange(t, events, EventGatewayChange)
}

func TestChangeDetectorKeepsInMemoryState(t *testing.T) {
	detector := NewChangeDetector(ChangeOptions{})
	first := detector.Apply([]Observation{{IP: "192.168.1.10", MAC: "00:11:22:33:44:55"}})
	if len(first) != 1 || first[0].Type != EventDeviceNew {
		t.Fatalf("first = %#v", first)
	}
	second := detector.Apply([]Observation{{IP: "192.168.1.10", MAC: "00:11:22:33:44:55"}})
	if len(second) != 0 {
		t.Fatalf("second = %#v", second)
	}
}

func assertHasChange(t *testing.T, events []ChangeEvent, eventType string) {
	t.Helper()
	if findChange(events, eventType) == nil {
		t.Fatalf("missing %s in %#v", eventType, events)
	}
}

func findChange(events []ChangeEvent, eventType string) *ChangeEvent {
	for i := range events {
		if events[i].Type == eventType {
			return &events[i]
		}
	}
	return nil
}
