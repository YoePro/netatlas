package arpscout

import (
	"strings"
	"testing"
	"time"
)

func TestValidateScanOptionsRejectsAggressiveSettings(t *testing.T) {
	err := ValidateScanOptions(ScanOptions{
		CIDR:     "192.168.1.0/24",
		MaxHosts: MaxScanTargets + 1,
		Timeout:  time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "safety limit") {
		t.Fatalf("err = %v", err)
	}

	err = ValidateScanOptions(ScanOptions{
		CIDR:     "192.168.1.0/24",
		MaxHosts: 256,
		Timeout:  10 * time.Millisecond,
	})
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateDaemonConfigRejectsUnsafeActiveScan(t *testing.T) {
	err := ValidateDaemonConfig(DaemonConfig{
		PassiveInterval: time.Second,
		StatusInterval:  time.Second,
		ActiveScan:      true,
		ScanInterval:    time.Second,
		MaxScanHosts:    MaxScanTargets + 1,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	for _, want := range []string{"passive_interval", "status_interval", "scan_cidr", "scan_interval", "max_scan_hosts"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %v, missing %s", err, want)
		}
	}
}

func TestValidateTransportConfig(t *testing.T) {
	if err := ValidateTransportConfig(TransportConfig{Enabled: true}); err == nil {
		t.Fatal("expected missing core_url error")
	}
	if err := ValidateTransportConfig(TransportConfig{Enabled: true, CoreURL: "http://core", SpoolPath: "state/spool.jsonl", Timeout: time.Second, Retries: 0}); err != nil {
		t.Fatal(err)
	}
}

func TestBuildConfigDiagnosticsReportsErrors(t *testing.T) {
	diag := BuildConfigDiagnostics(
		DaemonConfig{ActiveScan: true, PassiveInterval: time.Second},
		TransportConfig{Enabled: true},
	)
	if diag.OK {
		t.Fatalf("diag = %#v", diag)
	}
	if len(diag.Errors) == 0 {
		t.Fatalf("diag = %#v", diag)
	}
}

func TestActiveScanReadinessErrorMatchesPermissionCheck(t *testing.T) {
	err := ActiveScanReadinessError()
	diag := PermissionCheck()
	if diag.ActiveScanReady && err != nil {
		t.Fatalf("err = %v", err)
	}
	if !diag.ActiveScanReady && err == nil {
		t.Fatal("expected readiness error")
	}
}
