package xsfm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func boolPtr(b bool) *bool { return &b }
func intPtr(i int) *int    { return &i }

func TestRenderTopic(t *testing.T) {
	// {device_id} 단일 필드 (하위호환): fields={"device_id": ...}.
	did := map[string]string{"device_id": "ap-101"}
	assert.Equal(t, "xsfm/ap-101/state", renderTopic("xsfm/{device_id}/state", did))
	assert.Equal(t, "cmd/ap-101", renderTopic("cmd/{device_id}", did))
	// placeholder 없는 템플릿은 그대로 반환 (검증은 config 단계).
	assert.Equal(t, "static/topic", renderTopic("static/topic", nil))

	// M14 다중 필드: 각 placeholder 세그먼트를 fields[name] 으로 치환.
	multi := map[string]string{"station_code": "ST1", "place_code": "P1", "device_index": "3", "attribute": "power"}
	assert.Equal(t,
		"state/ui-line/ST1/P1/bse9000/3/power",
		renderTopic("state/ui-line/{station_code}/{place_code}/bse9000/{device_index}/{attribute}", multi))
}

// M14: 와일드카드 구독 토픽 산출 — 모든 {...} 세그먼트를 "+" 로 치환.
func TestBuildSubscriptionTopic(t *testing.T) {
	assert.Equal(t,
		"state/ui-line/+/+/bse9000/+/+",
		buildSubscriptionTopic("state/ui-line/{station_code}/{place_code}/bse9000/{device_index}/{attribute}"))
	// {device_id} 단일 필드도 와일드카드 하나로.
	assert.Equal(t, "xsfm/+/state", buildSubscriptionTopic("xsfm/{device_id}/state"))
}

// M14: parseTopic — 리터럴 일치 + placeholder 캡처, 리터럴 불일치는 not-ours.
func TestParseTopic(t *testing.T) {
	tmpl := "state/ui-line/{station_code}/{place_code}/bse9000/{device_index}/{attribute}"

	fields, ok := parseTopic(tmpl, "state/ui-line/ST1/P1/bse9000/3/power")
	require.True(t, ok)
	assert.Equal(t, "ST1", fields["station_code"])
	assert.Equal(t, "P1", fields["place_code"])
	assert.Equal(t, "3", fields["device_index"])
	assert.Equal(t, "power", fields["attribute"])

	// 리터럴 세그먼트 불일치(bse8000) → not-ours.
	_, ok = parseTopic(tmpl, "state/ui-line/ST1/P1/bse8000/3/power")
	assert.False(t, ok)

	// 세그먼트 수 불일치 → not-ours.
	_, ok = parseTopic(tmpl, "state/ui-line/ST1/P1/bse9000/3")
	assert.False(t, ok)

	// 합성 주소(=로스터 키): {attribute} 제외, 템플릿 순서 ":" 결합.
	assert.Equal(t, "ST1:P1:3", synthesizeAddress(tmpl, fields))
}

// M14: attribute-per-topic 스칼라 디코드 — attribute 토큰이 축을 지칭.
func TestDecodeAttributeScalar(t *testing.T) {
	m := PayloadMapping{
		Power:    BoolField{Name: "power", OnValue: "on", OffValue: "off"},
		FanSpeed: IntField{Name: "fan_speed"},
		Online:   &BoolField{Name: "online"},
	}
	// power 축 스칼라 "on".
	st, ok := m.decodeAttributeScalar("power", []byte("on"))
	require.True(t, ok)
	assert.True(t, st.PowerSet)
	assert.True(t, st.Power)
	assert.False(t, st.FanSpeedSet)

	// fan_speed 축 스칼라 "2".
	st, ok = m.decodeAttributeScalar("fan_speed", []byte("2"))
	require.True(t, ok)
	assert.True(t, st.FanSpeedSet)
	assert.Equal(t, 2, st.FanSpeed)

	// 알 수 없는 attribute → not-ours.
	_, ok = m.decodeAttributeScalar("humidity", []byte("50"))
	assert.False(t, ok)
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

	directCfg, err := parseXSFMConfig(map[string]any{
		"transport_mode":         "direct",
		"broker":                 "tcp://broker:1883",
		"state_topic_template":   "xsfm/{device_id}/state",
		"command_topic_template": "xsfm/{device_id}/cmd",
		"payload_mapping":        mapping,
	})
	require.NoError(t, err)

	portCfg, err := parseXSFMConfig(map[string]any{
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
