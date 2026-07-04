package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestPrintPerformanceBaselineIncludesBaselineMeasures(t *testing.T) {
	metrics := performanceMetrics{
		Total:            1500 * time.Millisecond,
		Lines:            500,
		Parsed:           450,
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
	printPerformanceBaseline(&buf, metrics)
	output := buf.String()

	for _, want := range []string{
		"Performance baseline:",
		"total_time=1.5s",
		"parse_throughput=333.33 lines/sec",
		"write_throughput=266.67 events/sec",
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
