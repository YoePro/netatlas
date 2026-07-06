package arpscout

import (
	"strings"
	"testing"
	"time"
)

func TestParseIPNeigh(t *testing.T) {
	input := `192.168.1.1 dev eth0 lladdr aa:bb:cc:dd:ee:ff REACHABLE
192.168.1.20 dev eth0 lladdr 00:11:22:33:44:55 STALE
192.168.1.30 dev wlan0 INCOMPLETE
fe80::1 dev eth0 lladdr 66:77:88:99:aa:bb router STALE
`
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)

	observations, err := ParseIPNeigh(strings.NewReader(input), PassiveOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 3 {
		t.Fatalf("len(observations) = %d, want 3", len(observations))
	}
	first := observations[0]
	if first.IP != "192.168.1.1" || first.MAC != "aa:bb:cc:dd:ee:ff" || first.Interface != "eth0" || first.State != "REACHABLE" {
		t.Fatalf("first observation = %#v", first)
	}
	if first.Source != "passive_neigh" {
		t.Fatalf("Source = %q", first.Source)
	}
	if !first.Observed.Equal(now) {
		t.Fatalf("Observed = %s, want %s", first.Observed, now)
	}
}

func TestParseIPNeighIncludesIncompleteWhenRequested(t *testing.T) {
	input := `192.168.1.30 dev wlan0 INCOMPLETE`

	observations, err := ParseIPNeigh(strings.NewReader(input), PassiveOptions{IncludeIncomplete: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 1 {
		t.Fatalf("len(observations) = %d, want 1", len(observations))
	}
	if observations[0].State != "INCOMPLETE" {
		t.Fatalf("State = %q, want INCOMPLETE", observations[0].State)
	}
}

func TestParseIPNeighFiltersInterfaces(t *testing.T) {
	input := `192.168.1.1 dev eth0 lladdr aa:bb:cc:dd:ee:ff REACHABLE
192.168.1.2 dev wlan0 lladdr 00:11:22:33:44:55 STALE
`
	observations, err := ParseIPNeigh(strings.NewReader(input), PassiveOptions{Interfaces: []string{"wlan0"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 1 {
		t.Fatalf("len(observations) = %d, want 1", len(observations))
	}
	if observations[0].Interface != "wlan0" {
		t.Fatalf("Interface = %q, want wlan0", observations[0].Interface)
	}
}

func TestInfoDescribesSensorScope(t *testing.T) {
	info := Info()
	if info.Name != "arpscout" {
		t.Fatalf("Name = %q, want arpscout", info.Name)
	}
	if len(info.EventTypes) == 0 {
		t.Fatal("EventTypes is empty")
	}
}
