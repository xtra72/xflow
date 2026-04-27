package device

// DeviceFilter is used to filter devices in registry queries.
// All non-zero fields are AND'd together. Empty/zero fields match any device.
type DeviceFilter struct {
	Protocol  string   `json:"protocol"`
	AgentName string   `json:"agent_name"`
	Type      string   `json:"type"`
	Online    *bool    `json:"online"`
	Tags      []string `json:"tags"`
	Group     string   `json:"group"`
}

// Matches returns true if the device matches all non-zero filter criteria.
// Empty or zero-value filter fields are ignored (match any device).
func (f DeviceFilter) Matches(d Device) bool {
	if f.Protocol != "" && f.Protocol != d.Protocol() {
		return false
	}

	if f.AgentName != "" && f.AgentName != d.AgentName() {
		return false
	}

	if f.Type != "" && f.Type != string(d.Type()) {
		return false
	}

	if f.Online != nil && *f.Online != d.Online() {
		return false
	}

	if len(f.Tags) > 0 {
		deviceTags := d.Metadata().Tags
		tagSet := make(map[string]struct{}, len(deviceTags))
		for _, tag := range deviceTags {
			tagSet[tag] = struct{}{}
		}
		for _, requiredTag := range f.Tags {
			if _, found := tagSet[requiredTag]; !found {
				return false
			}
		}
	}

	if f.Group != "" && f.Group != d.Metadata().Group {
		return false
	}

	return true
}
