package xsfm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// M14 — 다중 필드 토픽 주소 지정 + attribute-per-topic 모델 통합 테스트
// ---------------------------------------------------------------------------

// attrStateTemplate / attrCommandTemplate 은 사용자 구동 예시의 다중 필드 템플릿이다.
const (
	attrStateTemplate   = "state/ui-line/{station_code}/{place_code}/bse9000/{device_index}/{attribute}"
	attrCommandTemplate = "cmd/ui-line/{station_code}/{place_code}/bse9000/{device_index}/{attribute}"
)

// attrOpts 는 attribute-per-topic direct 모드 옵션을 반환한다. power 는 on/off 문자열 스칼라,
// fan_speed 는 native 정수 스칼라. payload_mapping 의 필드명이 곧 {attribute} 토큰이다.
func attrOpts(extraDevices ...map[string]any) map[string]any {
	opts := map[string]any{
		"broker":                   "tcp://broker:1883",
		"state_topic_template":     attrStateTemplate,
		"command_topic_template":   attrCommandTemplate,
		"control_response_timeout": "0s",
		"payload_mapping": map[string]any{
			"power_field":     map[string]any{"name": "power", "on_value": "on", "off_value": "off"},
			"fan_speed_field": "fan_speed",
			"online_field":    "online",
		},
	}
	if len(extraDevices) > 0 {
		devs := make([]any, 0, len(extraDevices))
		for _, d := range extraDevices {
			devs = append(devs, d)
		}
		opts["devices"] = devs
	}
	return opts
}

// attrAgentWithMock 는 attribute 모드 direct 에이전트 + 연결된 목 클라이언트를 생성하고 Start 한다.
func attrAgentWithMock(t *testing.T, opts map[string]any) (*XSFMAgent, *mockMQTTClient) {
	t.Helper()
	a, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)
	mock := newMockMQTTClient()
	require.NoError(t, mock.Connect())
	ap.client = mock
	ap.cmdSink = &brokerCommandSink{client: mock, topicTmpl: ap.cfg.CommandTopicTemplate, qos: ap.cfg.QoS}
	require.NoError(t, ap.Start(context.Background()))
	t.Cleanup(func() { _ = ap.Stop(context.Background()) })
	return ap, mock
}

// M14: 다중 필드 템플릿(no {device_id})은 수용되고, placeholder 없는 템플릿은 거부된다.
func TestConfig_MultiFieldTemplateAcceptedZeroPlaceholderRejected(t *testing.T) {
	// {device_id} 없는 다중 필드 템플릿 → 수용 (ErrInvalidTopicTemplate 없음).
	cfg, err := parseXSFMConfig(attrOpts())
	require.NoError(t, err)
	assert.True(t, cfg.stateHasAttribute)
	assert.True(t, cfg.commandHasAttribute)

	// placeholder 가 하나도 없는 템플릿 → 거부.
	opts := attrOpts()
	opts["state_topic_template"] = "state/ui-line/static"
	_, err = parseXSFMConfig(opts)
	assert.ErrorIs(t, err, ErrInvalidTopicTemplate)
}

// secondaryLookup 은 보조 인덱스(compositeKey → device_id/UUID)를 직접 조회하는 테스트 헬퍼이다.
// 유입 상태가 device_id 가 아니라 (station,place,index) 보조 인덱스로 매칭됨을 검증하는 데 쓴다.
func secondaryLookup(ap *XSFMAgent, station, place string, index int) (string, bool) {
	ap.mu.RLock()
	defer ap.mu.RUnlock()
	id, ok := ap.secondary[compositeKey(station, place, index)]
	return id, ok
}

// M14(개정): attribute-per-topic 유입 — 실제 토픽 파싱 → 보조 인덱스에 없으므로 UUID 생성 auto
// 등록 → 축 스칼라 디코드 → device_state_changed 에 device_id(UUID) + 주소/attribute 메타 + epoch-ms.
// 조회 키가 "합성 주소 = device_id" 에서 "보조 인덱스 → UUID" 로 바뀌었으므로, 디바이스는 UUID 로
// 조회하고 보조 인덱스가 (ST1,P1,3) → 그 UUID 를 가리킴을 확인한다(기능 단언은 보존).
func TestAttr_IngressAutoRegisterAndMeta(t *testing.T) {
	ap, mock := attrAgentWithMock(t, attrOpts())

	// 와일드카드 구독 확인.
	assert.ElementsMatch(t, []string{"state/ui-line/+/+/bse9000/+/+"}, mock.subscribedTopics())

	// power=on 스칼라 유입 (실제 토픽).
	mock.deliver("state/ui-line/ST1/P1/bse9000/3/power", []byte("on"))

	// 보조 인덱스로 대상 device_id(UUID)를 찾는다 — device_id 가 아니라 (station,place,index)로 매칭.
	uuidID, ok := secondaryLookup(ap, "ST1", "P1", 3)
	require.True(t, ok, "보조 인덱스가 (ST1,P1,3) 를 가리켜야 한다")
	assert.True(t, isUUID(uuidID), "auto 등록 device_id 는 생성된 UUID 여야 한다")

	dev, err := ap.GetDevice(uuidID)
	require.NoError(t, err)
	assert.True(t, dev.Power, "power 축이 갱신되어야 한다")
	assert.Equal(t, "auto", dev.Source, "최초 관측 디바이스는 Source=auto")
	assert.Equal(t, "ST1", dev.Station)
	assert.Equal(t, "P1", dev.Place)
	assert.Equal(t, 3, dev.Index)
	assert.Equal(t, "ST1:P1:003", dev.Name, "Name 은 3자리 0채움 합성")

	events := drainEvents(t, ap, 100*time.Millisecond)
	evt := firstEventOfType(events, "device_state_changed")
	require.NotNil(t, evt)
	assert.Equal(t, uuidID, evt["device_id"], "이벤트 device_id 는 UUID")
	assert.Equal(t, "ST1", evt["station_code"], "추출된 주소 필드가 메타로 실려야 한다")
	assert.Equal(t, "P1", evt["place_code"])
	assert.Equal(t, "3", evt["device_index"])
	assert.Equal(t, "power", evt["attribute"])
	assert.Equal(t, true, evt["power"])
	ts, ok := evt["timestamp"].(float64)
	require.True(t, ok)
	assert.Greater(t, ts, float64(1_000_000_000_000), "epoch ms 크기")

	// 리터럴 불일치(bse8000)는 무시되어야 한다 (우리 소유 아님).
	mock.deliver("state/ui-line/ST9/P9/bse8000/9/power", []byte("on"))
	_, ok = secondaryLookup(ap, "ST9", "P9", 9)
	assert.False(t, ok, "리터럴 불일치 토픽은 무시되어야 한다(보조 인덱스 미등록)")
}

// M14(개정): 분리된 attribute 메시지 — power 와 fan_speed 가 같은 (station,place,index) 디바이스의
// 두 축을 독립 갱신한다. 두 토픽이 동일 보조 인덱스 → 동일 UUID 로 매칭됨을 확인한다.
func TestAttr_SeparateAttributeMessagesUpdateAxes(t *testing.T) {
	ap, mock := attrAgentWithMock(t, attrOpts())

	mock.deliver("state/ui-line/ST1/P1/bse9000/3/power", []byte("on"))
	mock.deliver("state/ui-line/ST1/P1/bse9000/3/fan_speed", []byte("2"))

	uuidID, ok := secondaryLookup(ap, "ST1", "P1", 3)
	require.True(t, ok)
	dev, err := ap.GetDevice(uuidID)
	require.NoError(t, err)
	assert.True(t, dev.Power)
	assert.Equal(t, 2, dev.FanSpeed)
	// 두 메시지가 같은 디바이스를 갱신했으므로 로스터에는 단 하나의 디바이스만 존재해야 한다.
	assert.Len(t, ap.ListDevices(), 1, "두 축 메시지가 하나의 디바이스로 병합되어야 한다")
}

// M14: 명령 렌더(attribute 모드) — set_power → .../power, set_fan_speed → .../fan_speed,
// set_multiple → 두 발행(power 먼저, fan 그다음).
func TestAttr_CommandRenderPerAxis(t *testing.T) {
	// 위치 계층만으로 config 시드 (device_id 없이 합성 키 "ST1:P1:3").
	ap, mock := attrAgentWithMock(t, attrOpts(map[string]any{"station": "ST1", "place": "P1", "index": 3}))

	// set_power on.
	_, err := ap.Process([]byte(`{"command":"set_power","device_id":"ST1:P1:3","params":{"power":true}}`))
	require.NoError(t, err)
	require.Len(t, mock.published, 1)
	// 명령 토픽의 device_index 는 3자리 0-채움(%03d)으로 렌더된다(3 → "003"). SPEC-XSFM-001
	// 개정 Change 1 — 유입(STATE) 매칭은 int 정규화라 불변, 명령 egress 표기만 패딩된다.
	assert.Equal(t, "cmd/ui-line/ST1/P1/bse9000/003/power", mock.published[0].topic)
	assert.Equal(t, "on", string(mock.published[0].payload), "축 스칼라(on) 페이로드")

	// set_fan_speed 2 — 전원 ON 상태로 만든 뒤.
	mock.deliver("state/ui-line/ST1/P1/bse9000/3/power", []byte("on"))
	_, err = ap.Process([]byte(`{"command":"set_fan_speed","device_id":"ST1:P1:3","params":{"fan_speed":2}}`))
	require.NoError(t, err)
	require.Len(t, mock.published, 2)
	assert.Equal(t, "cmd/ui-line/ST1/P1/bse9000/003/fan_speed", mock.published[1].topic)
	assert.Equal(t, "2", string(mock.published[1].payload))

	// set_multiple {power:true, fan_speed:3} → 두 발행(power 먼저).
	_, err = ap.Process([]byte(`{"command":"set_multiple","device_id":"ST1:P1:3","params":{"power":true,"fan_speed":3}}`))
	require.NoError(t, err)
	require.Len(t, mock.published, 4)
	assert.Equal(t, "cmd/ui-line/ST1/P1/bse9000/003/power", mock.published[2].topic)
	assert.Equal(t, "on", string(mock.published[2].payload))
	assert.Equal(t, "cmd/ui-line/ST1/P1/bse9000/003/fan_speed", mock.published[3].topic)
	assert.Equal(t, "3", string(mock.published[3].payload))
}

// M14: 응답 대기 에코 상관(attribute 모드) — set_power → pending, .../power 상태 에코가 해소.
// set_fan_speed pending 과 독립적이다.
func TestAttr_ResponseWaitEchoCorrelation(t *testing.T) {
	opts := attrOpts(map[string]any{"station": "ST1", "place": "P1", "index": 3})
	opts["control_response_timeout"] = "2s"
	a, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)
	mock := newMockMQTTClient()
	require.NoError(t, mock.Connect())
	ap.client = mock
	ap.cmdSink = &brokerCommandSink{client: mock, topicTmpl: ap.cfg.CommandTopicTemplate, qos: ap.cfg.QoS}
	require.NoError(t, ap.Start(context.Background()))
	defer func() { _ = ap.Stop(context.Background()) }()

	// set_power 에코를 발행 관측 후 주입 → 해소.
	go func() {
		waitForPublish(mock, 1)
		ap.FeedStateFromTopic("state/ui-line/ST1/P1/bse9000/3/power", []byte("on"))
	}()
	resp, err := ap.Process([]byte(`{"command":"set_power","device_id":"ST1:P1:3","params":{"power":true}}`))
	require.NoError(t, err)
	assert.Contains(t, string(resp), `"status":"ok"`)
	assert.Equal(t, 0, ap.pendings.len(), "에코 해소 후 pending 제거")
}

// M14: 셀렉터(station) 는 attribute 모드에서도 동작 — 같은 station_code 두 디바이스로 축별 fan-out.
func TestAttr_StationSelectorFanOut(t *testing.T) {
	ap, mock := attrAgentWithMock(t, attrOpts(
		map[string]any{"station": "ST1", "place": "P1", "index": 1},
		map[string]any{"station": "ST1", "place": "P2", "index": 2},
	))

	resp, err := ap.Process([]byte(`{"command":"set_power","station":"ST1","params":{"power":true}}`))
	require.NoError(t, err)
	assert.Contains(t, string(resp), `"status":"ok"`)

	// 두 멤버 각자의 축별 토픽으로 발행되어야 한다.
	assert.ElementsMatch(t,
		[]string{
			"cmd/ui-line/ST1/P1/bse9000/001/power",
			"cmd/ui-line/ST1/P2/bse9000/002/power",
		},
		mock.publishedTopics(),
	)
}

// M14: 하위호환 — {device_id} + JSON blob 흐름은 그대로 JSON 모드로 디코드/제어된다.
func TestAttr_BackwardCompatBlobMode(t *testing.T) {
	opts := directOpts() // xsfm/{device_id}/state, JSON blob.
	opts["control_response_timeout"] = "0s"
	opts["devices"] = []any{map[string]any{"device_id": "ap-101"}}
	a, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)
	assert.False(t, ap.cfg.stateHasAttribute, "{attribute} 없는 템플릿은 blob 모드")

	mock := newMockMQTTClient()
	require.NoError(t, mock.Connect())
	ap.client = mock
	ap.cmdSink = &brokerCommandSink{client: mock, topicTmpl: ap.cfg.CommandTopicTemplate, qos: ap.cfg.QoS}
	require.NoError(t, ap.Start(context.Background()))
	defer func() { _ = ap.Stop(context.Background()) }()

	// JSON blob 유입 → device_id 키 갱신.
	mock.deliver("xsfm/ap-101/state", []byte(`{"power":true,"fan_speed":2}`))
	dev, err := ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.True(t, dev.Power)
	assert.Equal(t, 2, dev.FanSpeed)

	// JSON blob 명령 → 단일 발행.
	_, err = ap.Process([]byte(`{"command":"set_power","device_id":"ap-101","params":{"power":true}}`))
	require.NoError(t, err)
	require.Len(t, mock.published, 1)
	assert.Equal(t, "xsfm/ap-101/cmd", mock.published[0].topic)
	assert.JSONEq(t, `{"power":true}`, string(mock.published[0].payload))
}
