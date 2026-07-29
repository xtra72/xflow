package xsfm

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// SPEC-XSFM-001 개정 — command-topic index padding + offline_timeout 기본 90s
//                            + payload-time liveness 옵션
// ---------------------------------------------------------------------------

// payloadEnvelopeOpts 는 attribute-per-topic + 엔벨로프(value/time) direct 옵션을 반환한다.
// power 는 on_value/off_value 정수(1/0), fan_speed 는 값 테이블. liveness_source 등 추가 키는
// 호출부에서 병합한다.
func payloadEnvelopeOpts() map[string]any {
	return map[string]any{
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
}

// Change 1: 명령 토픽의 {device_index} 는 3자리 0-채움(%03d)으로 렌더된다 (1 → "001", 23 → "023").
func TestCommandTopic_DeviceIndexZeroPadded(t *testing.T) {
	ap, mock := attrAgentWithMock(t, attrOpts(
		map[string]any{"station": "ST1", "place": "P1", "index": 1},
		map[string]any{"station": "ST1", "place": "P1", "index": 23},
	))

	_, err := ap.Process([]byte(`{"command":"set_power","device_id":"ST1:P1:1","params":{"power":true}}`))
	require.NoError(t, err)
	_, err = ap.Process([]byte(`{"command":"set_power","device_id":"ST1:P1:23","params":{"power":true}}`))
	require.NoError(t, err)

	assert.ElementsMatch(t,
		[]string{
			"cmd/ui-line/ST1/P1/bse9000/001/power",
			"cmd/ui-line/ST1/P1/bse9000/023/power",
		},
		mock.publishedTopics(),
		"device_index 는 3자리 0-채움으로 렌더되어야 한다",
	)
}

// Change 2: offline_timeout 미지정 시 기본값은 90s 이다.
func TestConfig_OfflineTimeoutDefault90s(t *testing.T) {
	cfg, err := parseXSFMConfig(directOpts())
	require.NoError(t, err)
	assert.Equal(t, 90*time.Second, cfg.OfflineTimeout, "offline_timeout 기본값은 90s")

	// 명시적 설정은 그대로 존중된다(기본값에 덮이지 않는다).
	opts := directOpts()
	opts["offline_timeout"] = "30s"
	cfg, err = parseXSFMConfig(opts)
	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, cfg.OfflineTimeout)
}

// Change 3: liveness_source 파싱 — 기본 "receive", 명시적 payload/receive 수용, 무효값 에러.
func TestConfig_LivenessSourceParsing(t *testing.T) {
	// 기본값 receive (하위호환).
	cfg, err := parseXSFMConfig(directOpts())
	require.NoError(t, err)
	assert.Equal(t, livenessSourceReceive, cfg.LivenessSource)

	// 명시적 payload.
	opts := directOpts()
	opts["liveness_source"] = "payload"
	cfg, err = parseXSFMConfig(opts)
	require.NoError(t, err)
	assert.Equal(t, livenessSourcePayload, cfg.LivenessSource)

	// 명시적 receive.
	opts = directOpts()
	opts["liveness_source"] = "receive"
	cfg, err = parseXSFMConfig(opts)
	require.NoError(t, err)
	assert.Equal(t, livenessSourceReceive, cfg.LivenessSource)

	// 무효값 → 에러(조용한 폴백이 아니라 조기 노출).
	opts = directOpts()
	opts["liveness_source"] = "bogus"
	_, err = parseXSFMConfig(opts)
	assert.ErrorIs(t, err, ErrInvalidLivenessSource)
}

// Change 3: liveness_source="payload" + time_field 유입 → LastSeen 이 디바이스 보고 시각(T)이 된다.
func TestLiveness_PayloadModeUsesDeviceTime(t *testing.T) {
	opts := payloadEnvelopeOpts()
	opts["liveness_source"] = "payload"
	ap, mock := attrAgentWithMock(t, opts)

	const devTime = int64(1700000000000) // ~2023-11-14, 수신 시각(now)과 크게 다른 값.
	mock.deliver("state/ui-line/ST1/P1/bse9000/3/power", []byte(`{"time":1700000000000,"value":1}`))

	uuidID, ok := secondaryLookup(ap, "ST1", "P1", 3)
	require.True(t, ok)
	dev, err := ap.GetDevice(uuidID)
	require.NoError(t, err)
	assert.Equal(t, devTime, dev.LastSeen.UnixMilli(),
		"payload 모드는 LastSeen 을 디바이스 보고 시각으로 설정해야 한다")
}

// Change 3(기본 유지): liveness_source 미설정(receive) → 같은 엔벨로프여도 LastSeen 은 수신 시각.
func TestLiveness_ReceiveModeDefaultUsesReceiveTime(t *testing.T) {
	ap, mock := attrAgentWithMock(t, payloadEnvelopeOpts()) // liveness_source 미설정 → 기본 receive.

	const devTime = int64(1700000000000)
	before := time.Now()
	mock.deliver("state/ui-line/ST1/P1/bse9000/3/power", []byte(`{"time":1700000000000,"value":1}`))

	uuidID, ok := secondaryLookup(ap, "ST1", "P1", 3)
	require.True(t, ok)
	dev, err := ap.GetDevice(uuidID)
	require.NoError(t, err)
	assert.NotEqual(t, devTime, dev.LastSeen.UnixMilli(), "기본 receive 는 LastSeen 을 디바이스 시각으로 쓰지 않는다")
	assert.False(t, dev.LastSeen.Before(before), "LastSeen 은 수신 시각(now)이어야 한다")
	assert.WithinDuration(t, time.Now(), dev.LastSeen, 2*time.Second)
}

// Change 3(행동 통합): payload 모드에서 디바이스 시각이 먼 과거이면 offline 판정이 그 시각을
// 기준으로 하므로 즉시 stale → device_offline 이 방출된다.
func TestLiveness_PayloadModeOfflineAgainstDeviceTime(t *testing.T) {
	opts := payloadEnvelopeOpts()
	opts["liveness_source"] = "payload"
	opts["offline_timeout"] = "40ms"
	ap, mock := attrAgentWithMock(t, opts)

	// 디바이스 보고 시각(2023)이 먼 과거 → LastSeen 이 과거이므로 즉시 stale.
	mock.deliver("state/ui-line/ST1/P1/bse9000/3/power", []byte(`{"time":1700000000000,"value":1}`))

	events := drainEvents(t, ap, 300*time.Millisecond)
	off := firstEventOfType(events, "device_offline")
	require.NotNil(t, off, "payload 모드에서 오래된 디바이스 시각은 즉시 오프라인으로 판정되어야 한다")
}
