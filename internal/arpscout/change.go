package arpscout

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type ChangeEvent struct {
	Type        string      `json:"type"`
	IP          string      `json:"ip,omitempty"`
	MAC         string      `json:"mac,omitempty"`
	PreviousIP  string      `json:"previous_ip,omitempty"`
	CurrentIP   string      `json:"current_ip,omitempty"`
	PreviousMAC string      `json:"previous_mac,omitempty"`
	CurrentMAC  string      `json:"current_mac,omitempty"`
	Message     string      `json:"message"`
	ObservedAt  time.Time   `json:"observed_at"`
	Observation Observation `json:"observation,omitempty"`
}

type ChangeOptions struct {
	GatewayIP string
	Now       time.Time
}

type ChangeDetector struct {
	previous map[string]Observation
	options  ChangeOptions
}

func NewChangeDetector(options ChangeOptions) *ChangeDetector {
	return &ChangeDetector{
		previous: make(map[string]Observation),
		options:  options,
	}
}

func (d *ChangeDetector) Apply(current []Observation) []ChangeEvent {
	events := DetectChangesFromState(d.previous, current, d.options)
	d.previous = observationState(current)
	return events
}

func DetectChanges(previous, current []Observation, options ChangeOptions) []ChangeEvent {
	return DetectChangesFromState(observationState(previous), current, options)
}

func DetectChangesFromState(previous map[string]Observation, current []Observation, options ChangeOptions) []ChangeEvent {
	now := options.Now
	if now.IsZero() {
		now = time.Now()
	}

	currentByKey := observationState(current)
	previousByIP := byIP(previous)
	previousByMAC := byMAC(previous)
	currentByIP := byIP(currentByKey)
	currentByMAC := byMAC(currentByKey)

	var events []ChangeEvent
	events = append(events, duplicateIPEvents(current, now)...)

	for key, observation := range currentByKey {
		if _, ok := previous[key]; !ok {
			events = append(events, newChangeEvent(EventDeviceNew, observation, now, "new device seen"))
		}
	}
	for key, observation := range previous {
		if _, ok := currentByKey[key]; !ok {
			events = append(events, newChangeEvent(EventDeviceLost, observation, now, "device missing from current batch"))
		}
	}

	for ip, previousObservation := range previousByIP {
		currentObservation, ok := currentByIP[ip]
		if !ok || previousObservation.MAC == "" || currentObservation.MAC == "" || previousObservation.MAC == currentObservation.MAC {
			continue
		}
		events = append(events, ChangeEvent{
			Type:        EventMACChanged,
			IP:          ip,
			PreviousMAC: previousObservation.MAC,
			CurrentMAC:  currentObservation.MAC,
			Message:     fmt.Sprintf("mac changed for %s from %s to %s", ip, previousObservation.MAC, currentObservation.MAC),
			ObservedAt:  now,
			Observation: currentObservation,
		})
	}

	for mac, previousObservation := range previousByMAC {
		currentObservation, ok := currentByMAC[mac]
		if !ok || previousObservation.IP == "" || currentObservation.IP == "" || previousObservation.IP == currentObservation.IP {
			continue
		}
		events = append(events, ChangeEvent{
			Type:        EventIPChanged,
			MAC:         mac,
			PreviousIP:  previousObservation.IP,
			CurrentIP:   currentObservation.IP,
			Message:     fmt.Sprintf("ip changed for %s from %s to %s", mac, previousObservation.IP, currentObservation.IP),
			ObservedAt:  now,
			Observation: currentObservation,
		})
	}

	gatewayIP := strings.TrimSpace(options.GatewayIP)
	if gatewayIP != "" {
		previousGateway, hadPrevious := previousByIP[gatewayIP]
		currentGateway, hasCurrent := currentByIP[gatewayIP]
		if hadPrevious && hasCurrent && previousGateway.MAC != "" && currentGateway.MAC != "" && previousGateway.MAC != currentGateway.MAC {
			events = append(events, ChangeEvent{
				Type:        EventGatewayChange,
				IP:          gatewayIP,
				PreviousMAC: previousGateway.MAC,
				CurrentMAC:  currentGateway.MAC,
				Message:     fmt.Sprintf("gateway %s mac changed from %s to %s", gatewayIP, previousGateway.MAC, currentGateway.MAC),
				ObservedAt:  now,
				Observation: currentGateway,
			})
		}
	}

	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Type == events[j].Type {
			return events[i].Message < events[j].Message
		}
		return events[i].Type < events[j].Type
	})
	return events
}

func observationState(observations []Observation) map[string]Observation {
	state := make(map[string]Observation)
	for _, observation := range observations {
		key := observationKey(observation)
		if key == "" {
			continue
		}
		state[key] = observation
	}
	return state
}

func byIP(state map[string]Observation) map[string]Observation {
	items := make(map[string]Observation)
	for _, observation := range state {
		if observation.IP != "" && observation.MAC != "" {
			items[observation.IP] = observation
		}
	}
	return items
}

func byMAC(state map[string]Observation) map[string]Observation {
	items := make(map[string]Observation)
	for _, observation := range state {
		if observation.MAC != "" && observation.IP != "" {
			items[observation.MAC] = observation
		}
	}
	return items
}

func duplicateIPEvents(observations []Observation, now time.Time) []ChangeEvent {
	seen := make(map[string]Observation)
	reported := make(map[string]struct{})
	var events []ChangeEvent
	for _, observation := range observations {
		if observation.IP == "" || observation.MAC == "" {
			continue
		}
		previous, ok := seen[observation.IP]
		if !ok {
			seen[observation.IP] = observation
			continue
		}
		if previous.MAC == observation.MAC {
			continue
		}
		key := observation.IP + "|" + previous.MAC + "|" + observation.MAC
		if _, ok := reported[key]; ok {
			continue
		}
		reported[key] = struct{}{}
		events = append(events, ChangeEvent{
			Type:        EventDuplicateIP,
			IP:          observation.IP,
			PreviousMAC: previous.MAC,
			CurrentMAC:  observation.MAC,
			Message:     fmt.Sprintf("duplicate ip %s seen with %s and %s", observation.IP, previous.MAC, observation.MAC),
			ObservedAt:  now,
			Observation: observation,
		})
	}
	return events
}

func newChangeEvent(eventType string, observation Observation, now time.Time, message string) ChangeEvent {
	return ChangeEvent{
		Type:        eventType,
		IP:          observation.IP,
		MAC:         observation.MAC,
		Message:     message,
		ObservedAt:  now,
		Observation: observation,
	}
}

func observationKey(observation Observation) string {
	if observation.MAC != "" {
		return "mac:" + observation.MAC
	}
	if observation.IP != "" {
		return "ip:" + observation.IP
	}
	return ""
}
