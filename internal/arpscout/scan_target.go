package arpscout

import (
	"fmt"
	"net"
	"strings"
)

type InterfaceInfo struct {
	Name  string
	Flags net.Flags
	Addrs []net.Addr
}

type ScanTarget struct {
	CIDR        string `json:"cidr"`
	Interface   string `json:"interface,omitempty"`
	AddressType string `json:"address_type"`
	NetworkType string `json:"network_type"`
	Selected    bool   `json:"selected"`
	SkipReason  string `json:"skip_reason,omitempty"`
}

type ScanTargetOptions struct {
	Interface string
	All       bool
	MaxHosts  int
}

func DiscoverScanTargets(options ScanTargetOptions) ([]ScanTarget, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("list interfaces: %w", err)
	}
	items := make([]InterfaceInfo, 0, len(interfaces))
	for _, iface := range interfaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		items = append(items, InterfaceInfo{Name: iface.Name, Flags: iface.Flags, Addrs: addrs})
	}
	return SelectScanTargets(items, options), nil
}

func SelectScanTargets(interfaces []InterfaceInfo, options ScanTargetOptions) []ScanTarget {
	maxHosts := options.MaxHosts
	if maxHosts <= 0 {
		maxHosts = 256
	}
	ifaceFilter := strings.TrimSpace(options.Interface)

	var targets []ScanTarget
	for _, iface := range interfaces {
		if ifaceFilter != "" && iface.Name != ifaceFilter {
			continue
		}
		for _, addr := range iface.Addrs {
			cidr, ok := ipv4CIDR(addr)
			if !ok {
				continue
			}
			classification := ClassifyNetwork(networkIP(cidr), iface.Name)
			target := ScanTarget{
				CIDR:        cidr,
				Interface:   iface.Name,
				AddressType: classification.AddressType,
				NetworkType: classification.NetworkType,
			}
			target.SkipReason = scanTargetSkipReason(iface, target, maxHosts)
			target.Selected = target.SkipReason == "" || options.All
			targets = append(targets, target)
		}
	}
	return targets
}

func DefaultScanTarget(targets []ScanTarget) (ScanTarget, bool) {
	for _, target := range targets {
		if target.Selected {
			return target, true
		}
	}
	return ScanTarget{}, false
}

func scanTargetSkipReason(iface InterfaceInfo, target ScanTarget, maxHosts int) string {
	if iface.Flags&net.FlagUp == 0 {
		return "interface_down"
	}
	if iface.Flags&net.FlagLoopback != 0 || target.NetworkType == "loopback" {
		return "loopback"
	}
	switch target.NetworkType {
	case "virtual_bridge":
		return "virtual_bridge"
	case "vpn":
		return "vpn"
	}
	if target.AddressType != "private" && target.AddressType != "link_local" {
		return "not_local_lan"
	}
	if _, err := PlanScanTargets(target.CIDR, maxHosts); err != nil {
		return "too_large"
	}
	return ""
}

func ipv4CIDR(addr net.Addr) (string, bool) {
	ipNet, ok := addr.(*net.IPNet)
	if !ok || ipNet.IP.To4() == nil {
		return "", false
	}
	ones, bits := ipNet.Mask.Size()
	if bits != 32 {
		return "", false
	}
	network := ipNet.IP.Mask(ipNet.Mask)
	return fmt.Sprintf("%s/%d", network.String(), ones), true
}

func networkIP(cidr string) string {
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return ""
	}
	return ip.String()
}
