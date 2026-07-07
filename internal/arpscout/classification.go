package arpscout

import (
	"net"
	"net/netip"
	"strings"
)

type MACClassification struct {
	Normalized          string `json:"normalized"`
	Unicast             bool   `json:"unicast"`
	Multicast           bool   `json:"multicast"`
	GloballyUnique      bool   `json:"globally_unique"`
	LocallyAdministered bool   `json:"locally_administered"`
	OUIApplicable       bool   `json:"oui_applicable"`
}

type NetworkClassification struct {
	Interface   string `json:"interface,omitempty"`
	AddressType string `json:"address_type"`
	NetworkType string `json:"network_type"`
}

func EnrichObservation(observation *Observation) {
	if observation == nil {
		return
	}
	EnrichVendor(observation)
	classification := ClassifyNetwork(observation.IP, observation.Interface)
	observation.NetworkClassification = &classification
}

func ClassifyMAC(mac string) *MACClassification {
	normalized := normalizeMAC(mac)
	if normalized == "" {
		return nil
	}
	hw, err := net.ParseMAC(normalized)
	if err != nil || len(hw) == 0 {
		return nil
	}
	first := hw[0]
	multicast := first&1 == 1
	local := first&2 == 2
	return &MACClassification{
		Normalized:          normalized,
		Unicast:             !multicast,
		Multicast:           multicast,
		GloballyUnique:      !local,
		LocallyAdministered: local,
		OUIApplicable:       !multicast && !local,
	}
}

func ClassifyNetwork(ip, iface string) NetworkClassification {
	return NetworkClassification{
		Interface:   strings.TrimSpace(iface),
		AddressType: classifyAddress(ip),
		NetworkType: classifyInterface(iface),
	}
}

func classifyAddress(value string) string {
	addr, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil {
		return "unknown"
	}
	switch {
	case addr.IsLoopback():
		return "loopback"
	case addr.IsMulticast():
		return "multicast"
	case addr.IsLinkLocalUnicast():
		return "link_local"
	case addr.IsPrivate():
		return "private"
	case addr.IsGlobalUnicast():
		return "global"
	default:
		return "unknown"
	}
}

func classifyInterface(value string) string {
	iface := strings.ToLower(strings.TrimSpace(value))
	switch {
	case iface == "":
		return "unknown"
	case iface == "docker0" || strings.HasPrefix(iface, "br-") || strings.HasPrefix(iface, "veth") || strings.HasPrefix(iface, "cni") || strings.HasPrefix(iface, "virbr"):
		return "virtual_bridge"
	case strings.HasPrefix(iface, "tun") || strings.HasPrefix(iface, "tap") || strings.HasPrefix(iface, "wg") || strings.HasPrefix(iface, "tailscale") || strings.HasPrefix(iface, "zt"):
		return "vpn"
	case strings.HasPrefix(iface, "wl") || strings.HasPrefix(iface, "wlan"):
		return "wireless"
	case strings.HasPrefix(iface, "en") || strings.HasPrefix(iface, "eth"):
		return "ethernet"
	case strings.HasPrefix(iface, "lo"):
		return "loopback"
	default:
		return "unknown"
	}
}

func normalizeMAC(mac string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(mac) {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			b.WriteRune(r)
		}
	}
	if b.Len() != 12 {
		return ""
	}
	value := b.String()
	return value[0:2] + ":" + value[2:4] + ":" + value[4:6] + ":" + value[6:8] + ":" + value[8:10] + ":" + value[10:12]
}
