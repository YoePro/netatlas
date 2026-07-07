package arpscout

import "testing"

func TestLookupVendorKnownPrefix(t *testing.T) {
	got := LookupVendor("b8:27:eb:12:34:56")
	if got != "Raspberry Pi Foundation" {
		t.Fatalf("vendor = %q", got)
	}
}

func TestLookupVendorNormalizesFormats(t *testing.T) {
	for _, mac := range []string{"B8-27-EB-12-34-56", "b827.eb12.3456", "b827eb123456"} {
		if got := LookupVendor(mac); got != "Raspberry Pi Foundation" {
			t.Fatalf("LookupVendor(%q) = %q", mac, got)
		}
	}
}

func TestLookupVendorUnknownPrefix(t *testing.T) {
	got := LookupVendor("aa:bb:cc:dd:ee:ff")
	if got != UnknownVendor {
		t.Fatalf("vendor = %q, want %q", got, UnknownVendor)
	}
}

func TestLookupVendorEmptyMAC(t *testing.T) {
	if got := LookupVendor(""); got != "" {
		t.Fatalf("vendor = %q, want empty", got)
	}
}

func TestEnrichVendorSkipsLocallyAdministeredMAC(t *testing.T) {
	observation := Observation{MAC: "02:42:ac:11:00:02"}
	EnrichVendor(&observation)
	if observation.Vendor != nil {
		t.Fatalf("Vendor = %q, want nil", *observation.Vendor)
	}
	if observation.MACClassification == nil || observation.MACClassification.OUIApplicable {
		t.Fatalf("MACClassification = %#v", observation.MACClassification)
	}
}

func TestEnrichVendorSkipsMulticastMAC(t *testing.T) {
	observation := Observation{MAC: "01:00:5e:00:00:fb"}
	EnrichVendor(&observation)
	if observation.Vendor != nil {
		t.Fatalf("Vendor = %q, want nil", *observation.Vendor)
	}
	if observation.MACClassification == nil || observation.MACClassification.OUIApplicable {
		t.Fatalf("MACClassification = %#v", observation.MACClassification)
	}
}
