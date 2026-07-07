package arpscout

import (
	"net"
	"testing"
)

func TestSelectScanTargetsDefaultsToPhysicalLAN(t *testing.T) {
	targets := SelectScanTargets([]InterfaceInfo{
		testInterface("lo", net.FlagUp|net.FlagLoopback, "127.0.0.1/8"),
		testInterface("docker0", net.FlagUp, "172.17.0.1/24"),
		testInterface("enp3s0", net.FlagUp, "192.168.1.22/24"),
	}, ScanTargetOptions{MaxHosts: 256})

	target, ok := DefaultScanTarget(targets)
	if !ok {
		t.Fatalf("targets = %#v", targets)
	}
	if target.Interface != "enp3s0" || target.CIDR != "192.168.1.0/24" {
		t.Fatalf("target = %#v", target)
	}
}

func TestSelectScanTargetsSkipsVirtualAndVPNByDefault(t *testing.T) {
	targets := SelectScanTargets([]InterfaceInfo{
		testInterface("br-abcd", net.FlagUp, "172.18.0.1/24"),
		testInterface("wg0", net.FlagUp, "10.8.0.1/24"),
	}, ScanTargetOptions{MaxHosts: 256})

	for _, target := range targets {
		if target.Selected {
			t.Fatalf("target selected unexpectedly: %#v", target)
		}
	}
}

func TestSelectScanTargetsAllIncludesSkippedTargets(t *testing.T) {
	targets := SelectScanTargets([]InterfaceInfo{
		testInterface("docker0", net.FlagUp, "172.17.0.1/24"),
	}, ScanTargetOptions{All: true, MaxHosts: 256})

	if len(targets) != 1 || !targets[0].Selected || targets[0].SkipReason != "virtual_bridge" {
		t.Fatalf("targets = %#v", targets)
	}
}

func TestSelectScanTargetsInterfaceFilter(t *testing.T) {
	targets := SelectScanTargets([]InterfaceInfo{
		testInterface("enp3s0", net.FlagUp, "192.168.1.22/24"),
		testInterface("wlan0", net.FlagUp, "192.168.2.22/24"),
	}, ScanTargetOptions{Interface: "wlan0", MaxHosts: 256})

	if len(targets) != 1 || targets[0].Interface != "wlan0" {
		t.Fatalf("targets = %#v", targets)
	}
}

func testInterface(name string, flags net.Flags, cidr string) InterfaceInfo {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(err)
	}
	return InterfaceInfo{
		Name:  name,
		Flags: flags,
		Addrs: []net.Addr{ipNet},
	}
}
