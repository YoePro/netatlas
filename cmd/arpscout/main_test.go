package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintHelp(t *testing.T) {
	var buf bytes.Buffer
	printHelp(&buf)
	output := buf.String()
	for _, want := range []string{"arpscout passive", "ip neigh", "-include-incomplete"} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q:\n%s", want, output)
		}
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

func TestSplitList(t *testing.T) {
	got := splitList("eth0, wlan0,,")
	if len(got) != 2 || got[0] != "eth0" || got[1] != "wlan0" {
		t.Fatalf("splitList = %#v", got)
	}
}
