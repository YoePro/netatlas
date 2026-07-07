package arpscout

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"

	"gopkg.in/ini.v1"
)

const Version = "1.0"

type IdentityConfig struct {
	SensorID    string
	DisplayName string
	Site        string
	Interfaces  []string
}

type Identity struct {
	SensorID     string   `json:"sensor_id"`
	DisplayName  string   `json:"display_name"`
	Version      string   `json:"version"`
	Hostname     string   `json:"hostname"`
	Site         string   `json:"site,omitempty"`
	Interfaces   []string `json:"interfaces"`
	Capabilities []string `json:"capabilities"`
}

func LoadIdentityConfig(path string) (IdentityConfig, error) {
	cfg := IdentityConfig{}
	if strings.TrimSpace(path) == "" {
		return cfg, nil
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("stat config %q: %w", path, err)
	}

	file, err := ini.Load(path)
	if err != nil {
		return cfg, fmt.Errorf("load config %q: %w", path, err)
	}
	section := file.Section("arpscout")
	cfg.SensorID = strings.TrimSpace(section.Key("sensor_id").String())
	cfg.DisplayName = strings.TrimSpace(section.Key("display_name").String())
	cfg.Site = strings.TrimSpace(section.Key("site").String())
	cfg.Interfaces = splitConfigList(section.Key("interfaces").String())
	return cfg, nil
}

func BuildIdentity(cfg IdentityConfig) (Identity, error) {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "unknown-host"
	}

	interfaces := cfg.Interfaces
	if len(interfaces) == 0 {
		interfaces = systemInterfaceNames()
	}
	sort.Strings(interfaces)

	sensorID := strings.TrimSpace(cfg.SensorID)
	if sensorID == "" {
		sensorID = generatedSensorID(hostname, interfaces)
	}

	displayName := strings.TrimSpace(cfg.DisplayName)
	if displayName == "" {
		displayName = hostname + " arpscout"
	}

	return Identity{
		SensorID:    sensorID,
		DisplayName: displayName,
		Version:     Version,
		Hostname:    hostname,
		Site:        strings.TrimSpace(cfg.Site),
		Interfaces:  interfaces,
		Capabilities: []string{
			"passive_arp",
			"active_arp",
			"change_detection",
			"daemon_mode",
			"mac_network_classification",
			"operational_safety_checks",
			"vendor_oui_lookup",
			"ip_neigh_reader",
			"json_identity",
			"json_observations",
			"observation_transport",
			"stable_arp_sensor",
		},
	}, nil
}

func generatedSensorID(hostname string, interfaces []string) string {
	hash := sha256.Sum256([]byte(hostname + "|" + strings.Join(interfaces, ",")))
	return "arpscout-" + hex.EncodeToString(hash[:])[:12]
}

func systemInterfaceNames() []string {
	items, err := net.Interfaces()
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		if item.Flags&net.FlagLoopback != 0 {
			continue
		}
		names = append(names, item.Name)
	}
	return names
}

func splitConfigList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}
