package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestPrintRunSummaryHidesTelemetryWhenDisabled(t *testing.T) {
	runStats := &stats{
		ignoredBind:                1,
		ignoredConfig:              2,
		ignoredFiltered:            3,
		ignoredNetwork:             4,
		ignoredNotify:              5,
		ignoredRateLimit:           6,
		ignoredResolver:            7,
		ignoredSocket:              8,
		ignoredTimeout:             9,
		ignoredXferIn:              10,
		ignoredZoneload:            11,
		notableRCode:               12,
		notableSecurityDeniedCache: 13,
		skippedGenesis:             14,
	}
	var buf bytes.Buffer
	printRunSummary(&buf, runStats, true, performanceMetrics{}, false)
	output := buf.String()

	if !strings.Contains(output, "Done. lines=0") {
		t.Fatalf("summary output missing basic Done line:\n%s", output)
	}
	if !strings.Contains(output, "notable=0") {
		t.Fatalf("summary output missing notable counter:\n%s", output)
	}
	if !strings.Contains(output, "skipped_genesis=14") {
		t.Fatalf("summary output missing skipped_genesis counter:\n%s", output)
	}
	if !strings.Contains(output, "ignored_by_category=bind_noise:1,config:2,filtered:3,network:4,notify:5,rate_limit:6,resolver:7,socket:8,timeout:9,xfer_in:10,zoneload:11") {
		t.Fatalf("summary output missing ignored_by_category:\n%s", output)
	}
	if !strings.Contains(output, "notable_by_category=rcode:12,security_denied_cache:13") {
		t.Fatalf("summary output missing notable_by_category:\n%s", output)
	}
	if strings.Contains(output, "Telemetry:") {
		t.Fatalf("summary output unexpectedly included telemetry:\n%s", output)
	}
}

func TestPrintRunSummaryIncludesTelemetryWhenEnabled(t *testing.T) {
	metrics := performanceMetrics{
		Total:            1500 * time.Millisecond,
		Lines:            500,
		Parsed:           450,
		SkippedGenesis:   25,
		Written:          400,
		Batches:          2,
		BatchSize:        250,
		LinesPerSecond:   333.33,
		ParsedPerSecond:  300,
		WrittenPerSecond: 266.67,
		PeakAllocBytes:   1536,
		CPUTime:          750 * time.Millisecond,
		CPUUtilization:   50,
		BatchEfficiency:  80,
	}

	var buf bytes.Buffer
	printRunSummary(&buf, &stats{}, true, metrics, true)
	output := buf.String()

	for _, want := range []string{
		"Done in 1.5s.",
		"Telemetry:",
		"total_time=1.5s",
		"parse_throughput=333.33 lines/sec",
		"write_throughput=266.67 events/sec",
		"skipped_by_genesis=25",
		"peak_memory=1.5 KiB",
		"cpu_time=750ms",
		"cpu_utilization=50.00%",
		"batch_efficiency=80.00%",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("performance output missing %q:\n%s", want, output)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := map[uint64]string{
		42:          "42 B",
		1536:        "1.5 KiB",
		1048576 * 2: "2.0 MiB",
	}

	for value, want := range tests {
		if got := formatBytes(value); got != want {
			t.Fatalf("formatBytes(%d) = %q, want %q", value, got, want)
		}
	}
}
