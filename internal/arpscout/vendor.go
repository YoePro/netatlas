package arpscout

import "strings"

const UnknownVendor = "Unknown"

var builtinVendors = map[string]string{
	"00005e": "IANA",
	"00155d": "Microsoft",
	"001b21": "Intel",
	"001c42": "Parallels",
	"005056": "VMware",
	"080027": "Oracle VirtualBox",
	"3c5a37": "Samsung Electronics",
	"525400": "QEMU",
	"60f262": "Microsoft",
	"704d7b": "ASUSTek Computer",
	"b827eb": "Raspberry Pi Foundation",
	"d83add": "Raspberry Pi Trading",
	"dca632": "Raspberry Pi Trading",
	"e45f01": "Raspberry Pi Trading",
	"f0d5bf": "Intel",
}

func LookupVendor(mac string) string {
	prefix := ouiPrefix(mac)
	if prefix == "" {
		return ""
	}
	if vendor, ok := builtinVendors[prefix]; ok {
		return vendor
	}
	return UnknownVendor
}

func LookupVendorValue(mac string) *string {
	vendor := LookupVendor(mac)
	if vendor == "" {
		return nil
	}
	return &vendor
}

func EnrichVendor(observation *Observation) {
	if observation == nil {
		return
	}
	classification := ClassifyMAC(observation.MAC)
	observation.MACClassification = classification
	if classification == nil || !classification.OUIApplicable {
		observation.Vendor = nil
		return
	}
	observation.Vendor = LookupVendorValue(observation.MAC)
}

func ouiPrefix(mac string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(mac) {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			b.WriteRune(r)
			if b.Len() == 6 {
				return b.String()
			}
		}
	}
	return ""
}
