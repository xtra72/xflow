package xsfm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// SPEC-XSFM payload envelope — value_field / time_field 추출
// ---------------------------------------------------------------------------
//
// 실제 디바이스는 attribute-per-topic STATE 페이로드를 바 스칼라가 아니라 엔벨로프
// {"time":<ts>,"value":<axis>} 로 발행한다. value_field/time_field 로 축 값과 시각을 추출한다.

// value_field 설정 시 엔벨로프에서 value 를 추출해 축 표현으로 디코딩한다.
func TestDecodeAttributeScalar_EnvelopeValue(t *testing.T) {
	m := PayloadMapping{
		Power:      BoolField{Name: "power", OnValue: float64(1), OffValue: float64(0)},
		FanSpeed:   IntField{Name: "wind_volume", Values: map[int]any{1: 1, 2: 2, 3: 3}},
		ValueField: "value",
	}

	// power=1 → true.
	st, ok := m.decodeAttributeScalar("power", []byte(`{"time":1700000000000,"value":1}`))
	require.True(t, ok)
	assert.True(t, st.PowerSet)
	assert.True(t, st.Power)

	// power=0 → false.
	st, ok = m.decodeAttributeScalar("power", []byte(`{"time":1700000000000,"value":0}`))
	require.True(t, ok)
	assert.True(t, st.PowerSet)
	assert.False(t, st.Power)

	// wind_volume=2 → fan_speed 2. (time 없는 엔벨로프도 허용)
	st, ok = m.decodeAttributeScalar("wind_volume", []byte(`{"value":2}`))
	require.True(t, ok)
	assert.True(t, st.FanSpeedSet)
	assert.Equal(t, 2, st.FanSpeed)
}

// time_field 설정 시 엔벨로프에서 타임스탬프(epoch ms 숫자)를 추출한다.
func TestDecodeAttributeScalar_EnvelopeTime(t *testing.T) {
	m := PayloadMapping{
		Power:      BoolField{Name: "power", OnValue: float64(1), OffValue: float64(0)},
		FanSpeed:   IntField{Name: "wind_volume"},
		ValueField: "value",
		TimeField:  "time",
	}
	st, ok := m.decodeAttributeScalar("power", []byte(`{"time":1700000000000,"value":1}`))
	require.True(t, ok)
	assert.True(t, st.TimestampSet)
	assert.Equal(t, int64(1700000000000), st.TimestampMs)
	assert.True(t, st.Power)

	// time_field 미설정이면 타임스탬프 미추출(수신 시각 폴백 유도).
	m2 := m
	m2.TimeField = ""
	st, ok = m2.decodeAttributeScalar("power", []byte(`{"time":1700000000000,"value":1}`))
	require.True(t, ok)
	assert.False(t, st.TimestampSet)
}

// RFC3339 문자열 타임스탬프도 파싱한다.
func TestDecodeAttributeScalar_EnvelopeTimeRFC3339(t *testing.T) {
	m := PayloadMapping{
		Power:      BoolField{Name: "power", OnValue: float64(1), OffValue: float64(0)},
		FanSpeed:   IntField{Name: "wind_volume"},
		ValueField: "value",
		TimeField:  "time",
	}
	st, ok := m.decodeAttributeScalar("power", []byte(`{"time":"2023-11-14T22:13:20Z","value":1}`))
	require.True(t, ok)
	assert.True(t, st.TimestampSet)
	want := time.Date(2023, 11, 14, 22, 13, 20, 0, time.UTC).UnixMilli()
	assert.Equal(t, want, st.TimestampMs)
}

// malformed: value_field 설정인데 페이로드가 바 스칼라이거나 value 키가 없으면 축 미설정(크래시 없음).
func TestDecodeAttributeScalar_EnvelopeMalformedSafeDegrade(t *testing.T) {
	m := PayloadMapping{
		Power:      BoolField{Name: "power", OnValue: float64(1), OffValue: float64(0)},
		FanSpeed:   IntField{Name: "wind_volume"},
		ValueField: "value",
		TimeField:  "time",
	}

	// 바 스칼라(객체 아님) → 축 미설정, attribute 는 인지되어 ok=true.
	st, ok := m.decodeAttributeScalar("power", []byte("1"))
	require.True(t, ok)
	assert.False(t, st.PowerSet, "비객체 페이로드는 축을 설정하지 않는다")

	// value 키 없음 → 축 미설정.
	st, ok = m.decodeAttributeScalar("power", []byte(`{"time":1700000000000}`))
	require.True(t, ok)
	assert.False(t, st.PowerSet, "value 키 부재 시 축 미설정")

	// 완전 malformed JSON → 축 미설정, 크래시 없음.
	st, ok = m.decodeAttributeScalar("power", []byte(`{not-json`))
	require.True(t, ok)
	assert.False(t, st.PowerSet)

	// 알 수 없는 attribute 는 여전히 not-ours.
	_, ok = m.decodeAttributeScalar("humidity", []byte(`{"value":1}`))
	assert.False(t, ok)
}

// 하위호환: value_field 미설정이면 바 스칼라 경로(기존 동작)를 그대로 탄다.
func TestDecodeAttributeScalar_BackwardCompatScalar(t *testing.T) {
	m := PayloadMapping{
		Power:    BoolField{Name: "power", OnValue: "on", OffValue: "off"},
		FanSpeed: IntField{Name: "fan_speed"},
	}
	// 바 스칼라 "on".
	st, ok := m.decodeAttributeScalar("power", []byte("on"))
	require.True(t, ok)
	assert.True(t, st.PowerSet)
	assert.True(t, st.Power)
	assert.False(t, st.TimestampSet)

	// 바 스칼라 "2".
	st, ok = m.decodeAttributeScalar("fan_speed", []byte("2"))
	require.True(t, ok)
	assert.Equal(t, 2, st.FanSpeed)
}

// envelopeTimeMs 단위 검사: 숫자는 verbatim ms, RFC3339 문자열은 파싱, 그 외는 ok=false.
func TestEnvelopeTimeMs(t *testing.T) {
	ms, ok := envelopeTimeMs(float64(1700000000000))
	require.True(t, ok)
	assert.Equal(t, int64(1700000000000), ms)

	ms, ok = envelopeTimeMs("2023-11-14T22:13:20Z")
	require.True(t, ok)
	assert.Equal(t, time.Date(2023, 11, 14, 22, 13, 20, 0, time.UTC).UnixMilli(), ms)

	_, ok = envelopeTimeMs("not-a-time")
	assert.False(t, ok)

	_, ok = envelopeTimeMs(true)
	assert.False(t, ok)
}

// 명령 인코딩 대칭: value_field 설정 시 set_power 는 {"value":<wire>} 엔벨로프, 미설정 시 바 스칼라.
func TestCommandWireBytes_Envelope(t *testing.T) {
	// value_field 설정 → 엔벨로프.
	m := PayloadMapping{
		Power:      BoolField{Name: "power", OnValue: float64(1), OffValue: float64(0)},
		FanSpeed:   IntField{Name: "wind_volume", Values: map[int]any{1: 1, 2: 2, 3: 3}},
		ValueField: "value",
	}
	assert.JSONEq(t, `{"value":1}`, string(m.commandWireBytes(m.Power.encodeBool(true))))
	assert.JSONEq(t, `{"value":0}`, string(m.commandWireBytes(m.Power.encodeBool(false))))
	assert.JSONEq(t, `{"value":2}`, string(m.commandWireBytes(m.FanSpeed.encodeInt(2))))

	// value_field 미설정 → 바 스칼라(기존 동작).
	m2 := PayloadMapping{
		Power:    BoolField{Name: "power", OnValue: "on", OffValue: "off"},
		FanSpeed: IntField{Name: "fan_speed"},
	}
	assert.Equal(t, []byte("on"), m2.commandWireBytes(m2.Power.encodeBool(true)))
	assert.Equal(t, []byte("2"), m2.commandWireBytes(m2.FanSpeed.encodeInt(2)))
}

// config 파싱: payload_mapping.value_field / time_field 가 PayloadMapping 으로 파싱된다.
func TestParsePayloadMapping_ValueTimeFields(t *testing.T) {
	mapping, err := parsePayloadMapping(map[string]any{
		"payload_mapping": map[string]any{
			"power_field":     map[string]any{"name": "power", "on_value": float64(1), "off_value": float64(0)},
			"fan_speed_field": map[string]any{"name": "wind_volume", "values": map[string]any{"1": 1, "2": 2, "3": 3}},
			"value_field":     "value",
			"time_field":      "time",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "value", mapping.ValueField)
	assert.Equal(t, "time", mapping.TimeField)

	// 미지정 시 빈 값(하위호환).
	mapping, err = parsePayloadMapping(map[string]any{
		"payload_mapping": map[string]any{
			"power_field":     "power",
			"fan_speed_field": "fan_speed",
		},
	})
	require.NoError(t, err)
	assert.Empty(t, mapping.ValueField)
	assert.Empty(t, mapping.TimeField)
}

// 통합: time_field 설정 시 방출된 device_state_changed timestamp 가 페이로드 시각이고,
// LastSeen 은 수신 시각을 유지(offline 판정 불변)한다.
func TestAttr_TimeFieldEmitTimestamp(t *testing.T) {
	opts := map[string]any{
		"broker":                   "tcp://broker:1883",
		"state_topic_template":     attrStateTemplate,
		"command_topic_template":   attrCommandTemplate,
		"control_response_timeout": "0s",
		"payload_mapping": map[string]any{
			"power_field":     map[string]any{"name": "power", "on_value": float64(1), "off_value": float64(0)},
			"fan_speed_field": map[string]any{"name": "wind_volume", "values": map[string]any{"1": 1, "2": 2, "3": 3}},
			"value_field":     "value",
			"time_field":      "time",
		},
	}
	ap, mock := attrAgentWithMock(t, opts)

	const devTime = int64(1700000000000) // ~2023-11-14, 수신 시각(now)과 크게 다른 값.
	before := time.Now()
	mock.deliver("state/ui-line/ST1/P1/bse9000/3/power", []byte(`{"time":1700000000000,"value":1}`))

	events := drainEvents(t, ap, 100*time.Millisecond)
	evt := firstEventOfType(events, "device_state_changed")
	require.NotNil(t, evt)
	assert.Equal(t, true, evt["power"])
	ts, ok := evt["timestamp"].(float64)
	require.True(t, ok)
	assert.Equal(t, devTime, int64(ts), "방출 timestamp 는 페이로드 time(디바이스 보고 시각)이어야 한다")

	// LastSeen 은 수신 시각을 유지한다 — 디바이스 시각(2023)이 아니라 now 근처.
	uuidID, ok := secondaryLookup(ap, "ST1", "P1", 3)
	require.True(t, ok)
	dev, err := ap.GetDevice(uuidID)
	require.NoError(t, err)
	assert.False(t, dev.LastSeen.Before(before), "LastSeen 은 수신 시각(now)이어야 한다")
	assert.WithinDuration(t, time.Now(), dev.LastSeen, 2*time.Second)
	assert.NotEqual(t, devTime, dev.LastSeen.UnixMilli(), "LastSeen 은 디바이스 시각이 아니다")
}

// 통합(하위호환): value_field 미설정이면 엔벨로프가 아닌 바 스칼라 유입이 기존대로 동작하고,
// timestamp 는 수신 시각(epoch ms 크기)이다.
func TestAttr_NoTimeFieldUsesReceiveTime(t *testing.T) {
	ap, mock := attrAgentWithMock(t, attrOpts())
	before := time.Now().UnixMilli()
	mock.deliver("state/ui-line/ST1/P1/bse9000/3/power", []byte("on"))

	events := drainEvents(t, ap, 100*time.Millisecond)
	evt := firstEventOfType(events, "device_state_changed")
	require.NotNil(t, evt)
	ts, ok := evt["timestamp"].(float64)
	require.True(t, ok)
	assert.GreaterOrEqual(t, int64(ts), before, "timestamp 는 수신 시각이어야 한다")
}
