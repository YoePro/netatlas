package arpscout

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPlanScanTargets(t *testing.T) {
	got, err := PlanScanTargets("192.168.1.0/30", 8)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"192.168.1.1", "192.168.1.2"}
	if len(got) != len(want) {
		t.Fatalf("targets = %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("targets = %#v", got)
		}
	}
}

func TestPlanScanTargetsRejectsTooLargeCIDR(t *testing.T) {
	if _, err := PlanScanTargets("192.168.0.0/24", 16); err == nil {
		t.Fatal("expected safety limit error")
	}
}

func TestPlanScanTargetsRejectsIPv6(t *testing.T) {
	if _, err := PlanScanTargets("2001:db8::/120", 256); err == nil {
		t.Fatal("expected IPv4-only error")
	}
}

func TestParseArpingOutput(t *testing.T) {
	observed := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	observation, ok := ParseArpingOutput("192.168.1.10", []byte("Unicast reply from 192.168.1.10 [B8:27:EB:DD:EE:FF]  1.123ms"), observed)
	if !ok {
		t.Fatal("expected parsed arping output")
	}
	if observation.IP != "192.168.1.10" || observation.MAC != "b8:27:eb:dd:ee:ff" || observation.Source != SourceActiveScan || observation.Vendor == nil || *observation.Vendor != "Raspberry Pi Foundation" {
		t.Fatalf("observation = %#v", observation)
	}
	if observation.MACClassification == nil || !observation.MACClassification.OUIApplicable {
		t.Fatalf("MACClassification = %#v", observation.MACClassification)
	}
}

func TestRunActiveScanDryRun(t *testing.T) {
	result, err := RunActiveScan(ScanOptions{
		CIDR:     "192.168.1.0/30",
		DryRun:   true,
		MaxHosts: 8,
		Now:      time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun || result.Statistics.ScannedCount != 0 || result.Plan.TargetCount != 2 || result.Plan.FirstTarget != "192.168.1.1" || result.Plan.LastTarget != "192.168.1.2" {
		t.Fatalf("result = %#v", result)
	}
	if len(result.PlannedTargets) != 0 {
		t.Fatalf("planned targets should be omitted by default: %#v", result.PlannedTargets)
	}
}

func TestRunActiveScanCanIncludeDebugPlannedTargets(t *testing.T) {
	result, err := RunActiveScan(ScanOptions{
		CIDR:                  "192.168.1.0/30",
		DryRun:                true,
		MaxHosts:              8,
		IncludePlannedTargets: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.PlannedTargets) != 2 {
		t.Fatalf("planned targets = %#v", result.PlannedTargets)
	}
}

func TestRunActiveScanWithRunner(t *testing.T) {
	result, err := RunActiveScan(ScanOptions{
		CIDR:     "192.168.1.0/30",
		MaxHosts: 8,
		Timeout:  time.Second,
		Runner: func(ctx context.Context, target string, options ScanOptions) ([]byte, error) {
			if target == "192.168.1.1" {
				return []byte("Unicast reply from 192.168.1.1 [00:11:22:33:44:55]"), nil
			}
			return nil, errors.New("no reply")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Statistics.ScannedCount != 2 || result.Statistics.ReplyCount != 1 || len(result.Observations) != 1 || len(result.Discoveries) != 1 {
		t.Fatalf("result = %#v", result)
	}
	if result.Observations[0].MAC != "00:11:22:33:44:55" {
		t.Fatalf("observations = %#v", result.Observations)
	}
	if len(result.Statistics.NonResponsiveRanges) != 1 || result.Statistics.NonResponsiveRanges[0].First != "192.168.1.2" {
		t.Fatalf("non responsive ranges = %#v", result.Statistics.NonResponsiveRanges)
	}
}

func TestRunActiveScanClassifiesInterface(t *testing.T) {
	result, err := RunActiveScan(ScanOptions{
		CIDR:      "192.168.1.0/32",
		Interface: "docker0",
		MaxHosts:  8,
		Timeout:   time.Second,
		Runner: func(ctx context.Context, target string, options ScanOptions) ([]byte, error) {
			return []byte("Unicast reply from 192.168.1.1 [B8:27:EB:12:34:56]"), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Observations) != 1 {
		t.Fatalf("observations = %#v", result.Observations)
	}
	classification := result.Observations[0].NetworkClassification
	if classification == nil || classification.NetworkType != "virtual_bridge" {
		t.Fatalf("NetworkClassification = %#v", classification)
	}
}

func TestCompressAddressRanges(t *testing.T) {
	got := CompressAddressRanges([]string{"192.168.1.1", "192.168.1.2", "192.168.1.4"})
	if len(got) != 2 {
		t.Fatalf("ranges = %#v", got)
	}
	if got[0].First != "192.168.1.1" || got[0].Last != "192.168.1.2" || got[0].Count != 2 {
		t.Fatalf("first range = %#v", got[0])
	}
	if got[1].First != "192.168.1.4" || got[1].Last != "" || got[1].Count != 1 {
		t.Fatalf("second range = %#v", got[1])
	}
}

func TestArpingArgsRoundsTimeoutUpToWholeSecond(t *testing.T) {
	args := arpingArgs("192.168.1.10", ScanOptions{
		Interface: "eth0",
		Timeout:   1500 * time.Millisecond,
	})
	want := []string{"-c", "1", "-w", "2", "-I", "eth0", "192.168.1.10"}
	if len(args) != len(want) {
		t.Fatalf("args = %#v", args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %#v", args)
		}
	}
}
