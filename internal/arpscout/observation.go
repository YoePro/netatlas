package arpscout

import "time"

const (
	EventDeviceSeen    = "device_seen"
	EventDeviceNew     = "device_new"
	EventDeviceLost    = "device_lost"
	EventIPChanged     = "ip_changed"
	EventMACChanged    = "mac_changed"
	EventDuplicateIP   = "duplicate_ip"
	EventGatewayChange = "gateway_changed"
)

type Observation struct {
	IP                    string                 `json:"ip"`
	MAC                   string                 `json:"mac,omitempty"`
	Vendor                *string                `json:"vendor"`
	MACClassification     *MACClassification     `json:"mac_classification,omitempty"`
	NetworkClassification *NetworkClassification `json:"network_classification,omitempty"`
	Interface             string                 `json:"interface,omitempty"`
	State                 string                 `json:"state,omitempty"`
	Source                string                 `json:"source"`
	Observed              time.Time              `json:"observed_at"`
}

type SensorInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Modes       []string `json:"modes"`
	EventTypes  []string `json:"event_types"`
}

func Info() SensorInfo {
	return SensorInfo{
		Name:        "arpscout",
		Description: "Local Layer 2 discovery sensor for IP-to-MAC observations.",
		Modes:       []string{"passive", "active"},
		EventTypes: []string{
			EventDeviceSeen,
			EventDeviceNew,
			EventDeviceLost,
			EventIPChanged,
			EventMACChanged,
			EventDuplicateIP,
			EventGatewayChange,
		},
	}
}
