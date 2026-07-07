package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"netatlas/internal/arpscout"
)

func TestPrintHelp(t *testing.T) {
	var buf bytes.Buffer
	printHelp(&buf)
	output := buf.String()
	for _, want := range []string{"arpscout check", "arpscout changes", "arpscout daemon", "arpscout passive", "arpscout scan", "ip neigh", "-include-incomplete", "-dry-run", "-debug", "vendor", "MAC classification"} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q:\n%s", want, output)
		}
	}
}

func TestRunCheckJSON(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.ini")
	content := `[arpscout]
passive_interval = 30s
status_interval = 5m

[arpscout_transport]
enabled = false
timeout = 10s
retries = 0
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := runCheck(&buf, []string{"-config", configPath, "-format", "json"}); err != nil {
		t.Fatal(err)
	}
	var diag map[string]any
	if err := json.Unmarshal(buf.Bytes(), &diag); err != nil {
		t.Fatal(err)
	}
	if diag["ok"] != true {
		t.Fatalf("diag = %#v", diag)
	}
}

func TestRunCheckTextFailsInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.ini")
	content := `[arpscout]
passive_interval = 1s
status_interval = 1s
active_scan = true
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := runCheck(&buf, []string{"-config", configPath}); err == nil {
		t.Fatal("expected check failure")
	}
	if !strings.Contains(buf.String(), "arpscout check: failed") {
		t.Fatalf("output = %q", buf.String())
	}
}

func TestRunIdentity(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.ini")
	content := `[arpscout]
sensor_id = scout-test
display_name = Test scout
site = lab
interfaces = eth0
`
	if err := os.WriteFile(configPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := runIdentity(&buf, []string{"-config", configPath}); err != nil {
		t.Fatal(err)
	}
	var identity map[string]any
	if err := json.Unmarshal(buf.Bytes(), &identity); err != nil {
		t.Fatal(err)
	}
	if identity["sensor_id"] != "scout-test" || identity["display_name"] != "Test scout" || identity["site"] != "lab" {
		t.Fatalf("identity = %#v", identity)
	}
}

func TestPrintJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := printJSON(&buf, map[string]string{"name": "arpscout"}); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["name"] != "arpscout" {
		t.Fatalf("decoded name = %q", decoded["name"])
	}
}

func TestRunScanDryRun(t *testing.T) {
	var buf bytes.Buffer
	if err := runScan(&buf, []string{"-cidr", "192.168.1.0/30", "-dry-run"}); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["cidr"] != "192.168.1.0/30" || result["dry_run"] != true {
		t.Fatalf("scan result = %#v", result)
	}
	if _, ok := result["planned_targets"]; ok {
		t.Fatalf("planned targets should be omitted without debug: %#v", result)
	}
	plan := result["plan"].(map[string]any)
	if plan["target_count"].(float64) != 2 || plan["first_target"] != "192.168.1.1" || plan["last_target"] != "192.168.1.2" {
		t.Fatalf("scan plan = %#v", plan)
	}
}

func TestRunScanDebugIncludesPlannedTargets(t *testing.T) {
	var buf bytes.Buffer
	if err := runScan(&buf, []string{"-cidr", "192.168.1.0/30", "-dry-run", "-debug"}); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if _, ok := result["planned_targets"]; !ok {
		t.Fatalf("planned targets missing in debug output: %#v", result)
	}
}

func TestRunScanPositionalCIDRTextPlan(t *testing.T) {
	var buf bytes.Buffer
	if err := runScan(&buf, []string{"192.168.1.0/30", "-dry-run", "-format", "text"}); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "Scan plan:") || !strings.Contains(output, "192.168.1.0/30") || !strings.Contains(output, "dry_run=true") || !strings.Contains(output, "first=192.168.1.1") {
		t.Fatalf("scan output = %q", output)
	}
}

func TestRunScanRejectsDuplicateCIDRInputs(t *testing.T) {
	var buf bytes.Buffer
	if err := runScan(&buf, []string{"192.168.1.0/30", "-cidr", "192.168.2.0/30", "-dry-run"}); err == nil {
		t.Fatal("expected duplicate CIDR error")
	}
}

func TestRunScanRejectsUnknownInterface(t *testing.T) {
	var buf bytes.Buffer
	err := runScan(&buf, []string{"192.168.1.0/30", "-iface", "definitely-not-an-interface", "-dry-run"})
	if err == nil || !strings.Contains(err.Error(), "unknown interface") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunChangesText(t *testing.T) {
	dir := t.TempDir()
	previousPath := filepath.Join(dir, "previous.json")
	currentPath := filepath.Join(dir, "current.json")
	if err := os.WriteFile(previousPath, []byte(`[{"ip":"192.168.1.10","mac":"00:11:22:33:44:55","source":"test"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(currentPath, []byte(`[{"ip":"192.168.1.20","mac":"00:11:22:33:44:55","source":"test"}]`), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	err := runChanges(&buf, []string{"-previous", previousPath, "-current", currentPath, "-format", "text"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "ip_changed") {
		t.Fatalf("changes output = %q", buf.String())
	}
}

func TestPrintDaemonEventText(t *testing.T) {
	var buf bytes.Buffer
	printDaemonEventText(&buf, arpscout.DaemonEvent{
		Kind: arpscout.DaemonEventBatch,
		Status: arpscout.DaemonStatus{
			Iterations: 1,
		},
		Observations: []arpscout.Observation{{IP: "192.168.1.10", MAC: "b8:27:eb:12:34:56"}},
		Changes: []arpscout.ChangeEvent{
			{Type: arpscout.EventDeviceNew, Message: "new device seen"},
		},
	})
	output := buf.String()
	if !strings.Contains(output, "batch observations=1 changes=1") || !strings.Contains(output, "device_new") {
		t.Fatalf("daemon output = %q", output)
	}
}

func TestSplitList(t *testing.T) {
	got := splitList("eth0, wlan0,,")
	if len(got) != 2 || got[0] != "eth0" || got[1] != "wlan0" {
		t.Fatalf("splitList = %#v", got)
	}
}
