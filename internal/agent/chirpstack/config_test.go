package chirpstack

import (
	"testing"

	"github.com/xtra/xflow/internal/agent"
)

// TestParseChirpStackConfig_Defaults 는 Transport.Options 가 없을 때 기본값이
// 적용되는지 검증한다. 특히 기본 구독 토픽은 ChirpStack application 이벤트를
// 포괄하는 "application/#" 이어야 한다 (REQ-M2-01).
func TestParseChirpStackConfig_Defaults(t *testing.T) {
	cc := parseChirpStackConfig(agent.AgentConfig{})

	if cc.Broker != "tcp://localhost:1883" {
		t.Errorf("Broker = %q, want tcp://localhost:1883", cc.Broker)
	}
	if len(cc.Topics) != 1 || cc.Topics[0] != "application/#" {
		t.Errorf("Topics = %v, want [application/#]", cc.Topics)
	}
	if cc.QoS != 1 {
		t.Errorf("QoS = %d, want 1", cc.QoS)
	}
	if !cc.AutoReconnect {
		t.Error("AutoReconnect = false, want true")
	}
	if cc.BufferSize <= 0 {
		t.Errorf("BufferSize = %d, want > 0", cc.BufferSize)
	}
	if cc.ClientID == "" {
		t.Error("ClientID should be auto-generated when unset")
	}
}

// TestParseChirpStackConfig_Overrides 는 Transport.Options 의 값이 기본값을
// 덮어쓰는지 검증한다.
func TestParseChirpStackConfig_Overrides(t *testing.T) {
	cc := parseChirpStackConfig(agent.AgentConfig{
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"broker":    "tcp://mqtt.example:1883",
				"client_id": "cs-client",
				"username":  "u",
				"password":  "p",
				"topics":    []any{"application/1/#", "application/2/#"},
				"qos":       2,
			},
		},
	})

	if cc.Broker != "tcp://mqtt.example:1883" {
		t.Errorf("Broker = %q", cc.Broker)
	}
	if cc.ClientID != "cs-client" {
		t.Errorf("ClientID = %q", cc.ClientID)
	}
	if cc.Username != "u" || cc.Password != "p" {
		t.Errorf("auth = %q/%q", cc.Username, cc.Password)
	}
	if len(cc.Topics) != 2 {
		t.Errorf("Topics = %v, want 2 entries", cc.Topics)
	}
	if cc.QoS != 2 {
		t.Errorf("QoS = %d, want 2", cc.QoS)
	}
}
