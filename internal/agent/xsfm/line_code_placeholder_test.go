package xsfm

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// SPEC-XSFM-LINE-001 §9 (RD-8, Module 8) — {line_code} 토픽 placeholder 통합 테스트
//
// direct-mode 토픽 템플릿에 라인 코드 placeholder 를 도입한다. outbound(command/pub) 는 디바이스의
// 파생 라인(ResolveLine(station))을 roster 락 밖 lineHint 로 주입해 렌더하고, inbound(state/sub) 는
// {line_code} 를 추출하되 디바이스 식별에는 무시한다(식별 = station+place+index, 라인은 station 파생).
// ---------------------------------------------------------------------------

// lineCodeStateTemplate / lineCodeCommandTemplate 은 {line_code} 를 선두 세그먼트로 갖는 blob 모드
// 템플릿이다(§4.7 예시, {attribute} 없음 → JSON-blob 디코드/인코드 경로).
const (
	lineCodeStateTemplate   = "state/{line_code}/{station_code}/{place_code}/bse9000/{device_index}"
	lineCodeCommandTemplate = "cmd/{line_code}/{station_code}/{place_code}/bse9000/{device_index}"
)

// lineCodeOpts 는 {line_code} placeholder 를 쓰는 blob direct 모드 옵션을 반환한다.
func lineCodeOpts() map[string]any {
	return map[string]any{
		"broker":                   "tcp://broker:1883",
		"state_topic_template":     lineCodeStateTemplate,
		"command_topic_template":   lineCodeCommandTemplate,
		"control_response_timeout": "0s",
		"payload_mapping":          validPayloadMapping(),
	}
}

// lineCodeAgentWithMock 는 {line_code} 모드 direct 에이전트 + 연결된 목 클라이언트를 생성하고 Start 한다
// (attrAgentWithMock 미러).
func lineCodeAgentWithMock(t *testing.T, opts map[string]any) (*XSFMAgent, *mockMQTTClient) {
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

// AC-9.1: placeholder 등록 — line_code 가 기존 placeholder 와 함께 등록되어 있다 (REQ-08-01).
func TestLineCode_PlaceholderRegistered(t *testing.T) {
	assert.Equal(t, "line_code", placeholderLineCode)

	tmpl := "cmd/{line_code}/{station_code}/{place_code}/bse9000/{device_index}/{attribute}"
	names := placeholderNames(tmpl)
	assert.Contains(t, names, placeholderLineCode, "line_code 가 placeholder 집합에 존재해야 한다")
	// 기존 placeholder 전량 잔존.
	assert.Contains(t, names, placeholderStationCode)
	assert.Contains(t, names, placeholderPlaceCode)
	assert.Contains(t, names, placeholderDeviceIndex)
	assert.Contains(t, names, placeholderAttribute)

	assert.True(t, templateHasLineCode(tmpl))
	assert.False(t, templateHasLineCode("cmd/{station_code}/{place_code}/{device_index}"))

	// 파생 config 플래그도 함께 켜진다.
	cfg, err := parseXSFMConfig(lineCodeOpts())
	require.NoError(t, err)
	assert.True(t, cfg.commandHasLineCode, "명령 템플릿에 line_code 가 있으면 commandHasLineCode=true")
}

// AC-9.2: outbound 파생 렌더(라인 있음) — {line_code} ← ResolveLine(device.Station) (REQ-08-02).
func TestLineCode_OutboundDerivedRender_LinePresent(t *testing.T) {
	ap, mock := lineCodeAgentWithMock(t, lineCodeOpts())

	// st01 의 파생 라인 = line_2.
	require.NoError(t, mustProcessOK(t, ap, `{"command":"add_station","station":"st01","line":"line_2"}`))
	// 디바이스 auto-등록(inbound state, line_code 세그먼트는 wildcard).
	mock.deliver("state/line_2/st01/pump/bse9000/3", []byte(`{"power":false}`))
	id, ok := secondaryLookup(ap, "st01", "pump", 3)
	require.True(t, ok, "디바이스는 (st01,pump,3) 로 식별되어야 한다")

	_, err := ap.Process([]byte(`{"command":"set_power","device_id":"` + id + `","params":{"power":true}}`))
	require.NoError(t, err)

	require.Len(t, mock.published, 1)
	assert.Equal(t, "cmd/line_2/st01/pump/bse9000/003", mock.published[0].topic,
		"{line_code} 가 파생 라인(line_2)으로, device_index 는 3자리 0채움(003)으로 렌더")
}

// AC-9.3: 빈 라인 → 빈 세그먼트(라인 없음) — ResolveLine 미해석 시 {line_code} 는 빈 문자열 (REQ-08-04).
func TestLineCode_OutboundEmptySegment_LineAbsent(t *testing.T) {
	ap, mock := lineCodeAgentWithMock(t, lineCodeOpts())

	// st99 는 station 레지스트리에 미등록 → ResolveLine 은 ErrStationNotFound → 빈 라인.
	mock.deliver("state/anything/st99/pump/bse9000/3", []byte(`{"power":false}`))
	id, ok := secondaryLookup(ap, "st99", "pump", 3)
	require.True(t, ok)

	_, err := ap.Process([]byte(`{"command":"set_power","device_id":"` + id + `","params":{"power":true}}`))
	require.NoError(t, err)

	require.Len(t, mock.published, 1)
	assert.Equal(t, "cmd//st99/pump/bse9000/003", mock.published[0].topic,
		"빈 라인은 빈 세그먼트로 유지되고(에러 아님) 나머지 세그먼트는 정상")
}

// AC-9.4: lineHint 락 규율 — 라인 해석이 roster 락 밖에서 이루어져 레지스트리 락 중첩/재진입
// deadlock 이 없다. `go test -race` 로 데이터 경쟁을 검증한다 (REQ-08-03, REQ-07-01).
func TestLineCode_LineHintLockDiscipline_Race(t *testing.T) {
	ap, mock := lineCodeAgentWithMock(t, lineCodeOpts())
	require.NoError(t, mustProcessOK(t, ap, `{"command":"add_station","station":"st01","line":"line_2"}`))
	mock.deliver("state/line_2/st01/pump/bse9000/3", []byte(`{"power":false}`))
	id, ok := secondaryLookup(ap, "st01", "pump", 3)
	require.True(t, ok)

	setPower := `{"command":"set_power","device_id":"` + id + `","params":{"power":true}}`
	addStation := `{"command":"add_station","station":"st01","line":"line_2"}`

	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(3)
		// outbound 렌더: controlDevice 가 roster 락 해제 후 resolveLineFor(station 레지스트리 락)를 호출.
		go func() { defer wg.Done(); _, _ = ap.Process([]byte(setPower)) }()
		// 동시에 station 레지스트리를 쓰는 add_station(자체 락) — 락 중첩이면 deadlock.
		go func() { defer wg.Done(); _, _ = ap.Process([]byte(addStation)) }()
		// 로스터 조회(로스터 락).
		go func() { defer wg.Done(); _ = ap.ListDevices() }()
	}
	wg.Wait() // deadlock 이면 여기서 멈춘다(테스트 타임아웃).

	assert.Contains(t, mock.publishedTopics(), "cmd/line_2/st01/pump/bse9000/003",
		"파생 라인 렌더가 정상 산출되어야 한다")
}

// AC-9.5: inbound {line_code} 추출·식별 무시 — 토픽의 line_code 값과 무관하게 동일 디바이스로
// 식별되고, applyAddressFields 가 line_code 를 어떤 Device 필드에도 세팅하지 않는다 (REQ-08-05).
func TestLineCode_InboundExtractedIgnoredForIdentity(t *testing.T) {
	ap, mock := lineCodeAgentWithMock(t, lineCodeOpts())
	// st01 의 파생 라인은 line_9 (토픽의 line_2 와 불일치 — 이중 소스 검증용).
	require.NoError(t, mustProcessOK(t, ap, `{"command":"add_station","station":"st01","line":"line_9"}`))

	// 토픽 line_code=line_2 로 유입.
	mock.deliver("state/line_2/st01/pump/bse9000/3", []byte(`{"power":true}`))
	id, ok := secondaryLookup(ap, "st01", "pump", 3)
	require.True(t, ok, "디바이스는 station+place+index 로 식별되어야 한다")

	dev, err := ap.GetDevice(id)
	require.NoError(t, err)
	assert.Equal(t, "st01", dev.Station)
	assert.Equal(t, "pump", dev.Place)
	assert.Equal(t, 3, dev.Index)

	// line_code 값(line_2)은 어떤 Device 주소 필드에도 저장되지 않는다.
	_, hasLineKey := dev.Address[placeholderLineCode]
	assert.False(t, hasLineKey, "Address 에 line_code 키가 없어야 한다")
	for k, v := range dev.Address {
		assert.NotEqual(t, "line_2", v, "토픽의 line_code 값이 Address 필드에 저장되면 안 된다 (key=%s)", k)
	}
	assert.NotEqual(t, "line_2", dev.Station)
	assert.NotEqual(t, "line_2", dev.Place)

	// 라인은 station 파생(line_9)으로 결정된다 — 토픽의 line_2 는 무시된다.
	assert.Equal(t, "line_9", ap.resolveLineFor("st01"))

	// 다른 line_code 로 재유입해도 동일 디바이스(신규 생성 없음).
	mock.deliver("state/line_5/st01/pump/bse9000/3", []byte(`{"power":false}`))
	id2, ok := secondaryLookup(ap, "st01", "pump", 3)
	require.True(t, ok)
	assert.Equal(t, id, id2, "line_code 가 달라도 동일 디바이스로 식별")
	assert.Len(t, ap.ListDevices(), 1, "line_code 차이로 새 디바이스가 생기면 안 된다")
}

// AC-9.5(단위): applyAddressFields / compositeKeyFromFields 가 line_code 를 배제함을 직접 검증.
func TestApplyAddressFields_IgnoresLineCode(t *testing.T) {
	fields := map[string]string{
		placeholderLineCode:    "line_2",
		placeholderStationCode: "st01",
		placeholderPlaceCode:   "pump",
		placeholderDeviceIndex: "3",
	}

	d := &Device{}
	applyAddressFields(d, fields)
	assert.Equal(t, "st01", d.Station)
	assert.Equal(t, "pump", d.Place)
	assert.Equal(t, 3, d.Index)
	assert.NotEqual(t, "line_2", d.Station, "line_code 가 Station 에 매핑되면 안 된다")
	assert.NotEqual(t, "line_2", d.Place)
	assert.NotEqual(t, "line_2", d.Name)

	// 합성 식별 키도 line_code 를 포함하지 않는다(식별 = station:place:index).
	ck := compositeKeyFromFields(fields)
	assert.Equal(t, "st01:pump:3", ck)
	assert.NotContains(t, ck, "line_2")

	// Device.Address 저장용 nonAttrFields 도 line_code 를 배제한다.
	addr := nonAttrFields(fields)
	_, has := addr[placeholderLineCode]
	assert.False(t, has, "nonAttrFields 는 line_code 를 배제해야 한다")
	assert.Equal(t, "st01", addr[placeholderStationCode], "기존 주소 필드는 보존")
}

// AC-9.6: 기존 placeholder 렌더/파싱 무회귀 — line_code 없는 템플릿은 flag=false 이고 렌더/파싱이
// amendment 이전과 동일하다 (REQ-08-06, REQ-07-05).
func TestLineCode_ExistingPlaceholdersNoRegression(t *testing.T) {
	// line_code 없는 기존(attribute) 템플릿: commandHasLineCode=false → 라인 해석 미개입.
	cfg, err := parseXSFMConfig(attrOpts())
	require.NoError(t, err)
	assert.False(t, cfg.commandHasLineCode, "line_code 없는 템플릿은 commandHasLineCode=false")
	assert.False(t, templateHasLineCode(attrCommandTemplate))
	assert.False(t, templateHasLineCode(attrStateTemplate))

	// 기존 placeholder 만 쓰는 렌더는 byte-동일(무회귀).
	fields := map[string]string{
		placeholderStationCode: "ST1",
		placeholderPlaceCode:   "P1",
		placeholderDeviceIndex: "003",
		placeholderAttribute:   "power",
	}
	assert.Equal(t, "cmd/ui-line/ST1/P1/bse9000/003/power", renderTopic(attrCommandTemplate, fields))

	// 기존 placeholder 파싱도 무변경(line_code 세그먼트 없음).
	parsed, ok := parseTopic(attrStateTemplate, "state/ui-line/ST1/P1/bse9000/3/power")
	require.True(t, ok)
	assert.Equal(t, "ST1", parsed[placeholderStationCode])
	assert.Equal(t, "P1", parsed[placeholderPlaceCode])
	assert.Equal(t, "3", parsed[placeholderDeviceIndex])
	assert.Equal(t, "power", parsed[placeholderAttribute])
	_, hasLine := parsed[placeholderLineCode]
	assert.False(t, hasLine, "line_code 없는 템플릿 파싱 결과에 line_code 키가 없어야 한다")
}
