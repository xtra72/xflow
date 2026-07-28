package airpurifier

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPtr(b bool) *bool { return &b }
func intPtr(i int) *int    { return &i }

func TestRenderTopic(t *testing.T) {
	assert.Equal(t, "airpurifier/ap-101/state", renderTopic("airpurifier/{device_id}/state", "ap-101"))
	assert.Equal(t, "cmd/ap-101", renderTopic("cmd/{device_id}", "ap-101"))
	// placeholder 없는 템플릿은 그대로 반환 (검증은 config 단계).
	assert.Equal(t, "static/topic", renderTopic("static/topic", "ap-101"))
}

func TestEncodeCommandPayload_NativeBool(t *testing.T) {
	m := PayloadMapping{
		Power:    BoolField{Name: "power"},
		FanSpeed: IntField{Name: "fan_speed"},
	}
	b, err := encodeCommandPayload(m, commandPayload{Power: boolPtr(true)})
	require.NoError(t, err)
	assert.JSONEq(t, `{"power":true}`, string(b))

	b, err = encodeCommandPayload(m, commandPayload{FanSpeed: intPtr(2)})
	require.NoError(t, err)
	assert.JSONEq(t, `{"fan_speed":2}`, string(b))
}

func TestEncodeCommandPayload_ValueMapped(t *testing.T) {
	m := PayloadMapping{
		Power:    BoolField{Name: "pw", OnValue: "ON", OffValue: "OFF"},
		FanSpeed: IntField{Name: "fan", Values: map[int]any{1: "low", 2: "mid", 3: "high"}},
	}
	b, err := encodeCommandPayload(m, commandPayload{Power: boolPtr(true)})
	require.NoError(t, err)
	assert.JSONEq(t, `{"pw":"ON"}`, string(b))

	b, err = encodeCommandPayload(m, commandPayload{Power: boolPtr(false)})
	require.NoError(t, err)
	assert.JSONEq(t, `{"pw":"OFF"}`, string(b))

	b, err = encodeCommandPayload(m, commandPayload{FanSpeed: intPtr(3)})
	require.NoError(t, err)
	assert.JSONEq(t, `{"fan":"high"}`, string(b))
}

func TestDecodeStatePayload_NativeAndMapped(t *testing.T) {
	// native bool + native int
	m := PayloadMapping{
		Power:    BoolField{Name: "power"},
		FanSpeed: IntField{Name: "fan_speed"},
	}
	st, err := decodeStatePayload(m, []byte(`{"power":true,"fan_speed":2}`))
	require.NoError(t, err)
	assert.True(t, st.Power)
	assert.True(t, st.PowerSet)
	assert.Equal(t, 2, st.FanSpeed)
	assert.True(t, st.FanSpeedSet)
	assert.False(t, st.OnlineSet)

	// value-mapped string representations
	m2 := PayloadMapping{
		Power:    BoolField{Name: "pw", OnValue: "ON", OffValue: "OFF"},
		FanSpeed: IntField{Name: "fan", Values: map[int]any{1: "low", 2: "mid", 3: "high"}},
		Online:   &BoolField{Name: "online"},
	}
	st, err = decodeStatePayload(m2, []byte(`{"pw":"ON","fan":"mid","online":true}`))
	require.NoError(t, err)
	assert.True(t, st.Power)
	assert.Equal(t, 2, st.FanSpeed)
	assert.True(t, st.Online)
	assert.True(t, st.OnlineSet)
}

func TestDecodeStatePayload_PartialObserved(t *testing.T) {
	m := PayloadMapping{
		Power:    BoolField{Name: "power"},
		FanSpeed: IntField{Name: "fan_speed"},
	}
	// power 만 관측 — fan_speed 는 미관측(Set=false).
	st, err := decodeStatePayload(m, []byte(`{"power":false}`))
	require.NoError(t, err)
	assert.True(t, st.PowerSet)
	assert.False(t, st.Power)
	assert.False(t, st.FanSpeedSet)
}

// Scenario 1B.8: 동일한 payload_mapping 으로 구성된 direct·port 모드가 산출한 제어 명령
// 페이로드가 바이트 단위로 동일해야 한다 (I/O 경계만 다르고 공유 로직 레이어는 동일).
func TestByteIdenticalEncoding_DirectVsPort(t *testing.T) {
	mapping := map[string]any{
		"power_field":     map[string]any{"name": "pw", "on_value": "ON", "off_value": "OFF"},
		"fan_speed_field": map[string]any{"name": "fan", "values": map[string]any{"1": "low", "2": "mid", "3": "high"}},
	}

	directCfg, err := parseAirPurifierConfig(map[string]any{
		"transport_mode":         "direct",
		"broker":                 "tcp://broker:1883",
		"state_topic_template":   "airpurifier/{device_id}/state",
		"command_topic_template": "airpurifier/{device_id}/cmd",
		"payload_mapping":        mapping,
	})
	require.NoError(t, err)

	portCfg, err := parseAirPurifierConfig(map[string]any{
		"transport_mode":  "port",
		"payload_mapping": mapping,
	})
	require.NoError(t, err)

	// set_power(true) 인코딩이 두 모드에서 바이트 동일.
	directPayload, err := encodeCommandPayload(directCfg.PayloadMapping, commandPayload{Power: boolPtr(true)})
	require.NoError(t, err)
	portPayload, err := encodeCommandPayload(portCfg.PayloadMapping, commandPayload{Power: boolPtr(true)})
	require.NoError(t, err)
	assert.Equal(t, directPayload, portPayload)

	// set_fan_speed(2) 도 동일.
	dFan, err := encodeCommandPayload(directCfg.PayloadMapping, commandPayload{FanSpeed: intPtr(2)})
	require.NoError(t, err)
	pFan, err := encodeCommandPayload(portCfg.PayloadMapping, commandPayload{FanSpeed: intPtr(2)})
	require.NoError(t, err)
	assert.Equal(t, dFan, pFan)
}

func TestPayloadMapping_IsValid(t *testing.T) {
	assert.True(t, PayloadMapping{Power: BoolField{Name: "p"}, FanSpeed: IntField{Name: "f"}}.IsValid())
	assert.False(t, PayloadMapping{Power: BoolField{Name: "p"}}.IsValid())
	assert.False(t, PayloadMapping{FanSpeed: IntField{Name: "f"}}.IsValid())
}
