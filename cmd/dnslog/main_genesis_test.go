package main

import (
	"testing"
	"time"

	"dnslog/internal/config"
)

func TestNewGenesisFilter(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		mode string
		want time.Time
	}{
		{mode: "3h", want: now.Add(-3 * time.Hour)},
		{mode: "24h", want: now.Add(-24 * time.Hour)},
		{mode: "7d", want: now.Add(-7 * 24 * time.Hour)},
		{mode: "30d", want: now.Add(-30 * 24 * time.Hour)},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			filter, err := newGenesisFilter(&config.Config{Genesis: tt.mode}, now)
			if err != nil {
				t.Fatal(err)
			}
			if !filter.enabled {
				t.Fatal("filter.enabled = false, want true")
			}
			if !filter.after.Equal(tt.want) {
				t.Fatalf("after = %s, want %s", filter.after, tt.want)
			}
		})
	}
}

func TestGenesisFilterShouldSkipOnlyOlderRows(t *testing.T) {
	filter, err := newGenesisFilter(&config.Config{
		Genesis:      "custom",
		GenesisAfter: "2026-07-04T00:00:00Z",
	}, time.Now())
	if err != nil {
		t.Fatal(err)
	}

	if !filter.shouldSkip("2026-07-03T23:59:59Z 192.168.1.10 old.example. A") {
		t.Fatal("older row was not skipped")
	}
	if filter.shouldSkip("2026-07-04T00:00:00Z 192.168.1.10 edge.example. A") {
		t.Fatal("row exactly at genesis boundary was skipped")
	}
	if filter.shouldSkip("not a supported log line") {
		t.Fatal("unsupported timestamp was skipped")
	}
}
