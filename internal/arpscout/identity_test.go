package arpscout

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadIdentityConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.ini")
	content := `[arpscout]
sensor_id = scout-fixed
display_name = Rack scout
site = lab
interfaces = eth0, wlan0
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadIdentityConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SensorID != "scout-fixed" || cfg.DisplayName != "Rack scout" || cfg.Site != "lab" {
		t.Fatalf("identity config = %#v", cfg)
	}
	if len(cfg.Interfaces) != 2 || cfg.Interfaces[0] != "eth0" || cfg.Interfaces[1] != "wlan0" {
		t.Fatalf("interfaces = %#v", cfg.Interfaces)
	}
}

func TestBuildIdentityGeneratesStableID(t *testing.T) {
	cfg := IdentityConfig{
		DisplayName: "Test scout",
		Site:        "lab",
		Interfaces:  []string{"eth0", "wlan0"},
	}

	first, err := BuildIdentity(cfg)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildIdentity(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if first.SensorID != second.SensorID {
		t.Fatalf("SensorID is not stable: %q != %q", first.SensorID, second.SensorID)
	}
	if !strings.HasPrefix(first.SensorID, "arpscout-") {
		t.Fatalf("SensorID = %q, want arpscout-*", first.SensorID)
	}
	if first.DisplayName != "Test scout" || first.Site != "lab" {
		t.Fatalf("identity = %#v", first)
	}
	if len(first.Capabilities) == 0 {
		t.Fatal("Capabilities is empty")
	}
}
