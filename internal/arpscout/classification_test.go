package arpscout

import "testing"

func TestClassifyMACGlobalUnicast(t *testing.T) {
	got := ClassifyMAC("b8:27:eb:12:34:56")
	if got == nil {
		t.Fatal("classification is nil")
	}
	if !got.Unicast || got.Multicast || !got.GloballyUnique || got.LocallyAdministered || !got.OUIApplicable {
		t.Fatalf("classification = %#v", got)
	}
}

func TestClassifyMACLocallyAdministered(t *testing.T) {
	got := ClassifyMAC("02:42:ac:11:00:02")
	if got == nil {
		t.Fatal("classification is nil")
	}
	if !got.Unicast || !got.LocallyAdministered || got.GloballyUnique || got.OUIApplicable {
		t.Fatalf("classification = %#v", got)
	}
}

func TestClassifyMACMulticast(t *testing.T) {
	got := ClassifyMAC("01:00:5e:00:00:fb")
	if got == nil {
		t.Fatal("classification is nil")
	}
	if got.Unicast || !got.Multicast || got.OUIApplicable {
		t.Fatalf("classification = %#v", got)
	}
}

func TestClassifyNetworkDockerBridge(t *testing.T) {
	got := ClassifyNetwork("172.17.0.2", "docker0")
	if got.AddressType != "private" || got.NetworkType != "virtual_bridge" {
		t.Fatalf("network classification = %#v", got)
	}
}

func TestClassifyNetworkWireless(t *testing.T) {
	got := ClassifyNetwork("192.168.1.10", "wlan0")
	if got.AddressType != "private" || got.NetworkType != "wireless" {
		t.Fatalf("network classification = %#v", got)
	}
}
