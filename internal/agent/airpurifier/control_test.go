package airpurifier

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// directAgentWithMock 는 direct 모드 에이전트를 생성하고 연결된 목 MQTTClient 를 주입한다.
// cmdSink 도 목 기반 brokerCommandSink 로 교체하여 발행을 관측할 수 있게 한다.
func directAgentWithMock(t *testing.T, opts map[string]any) (*AirPurifierAgent, *mockMQTTClient) {
	t.Helper()
	a, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	mock := newMockMQTTClient()
	require.NoError(t, mock.Connect())
	ap.client = mock
	ap.cmdSink = &brokerCommandSink{client: mock, topicTmpl: ap.cfg.CommandTopicTemplate, qos: ap.cfg.QoS}
	return ap, mock
}

// readEvent 는 msgCh 에서 방출 이벤트 하나를 디코딩하여 반환한다 (타임아웃 1s).
func readEvent(t *testing.T, ap *AirPurifierAgent) map[string]any {
	t.Helper()
	select {
	case b := <-ap.msgCh:
		var m map[string]any
		require.NoError(t, json.Unmarshal(b, &m))
		return m
	case <-time.After(time.Second):
		t.Fatal("expected event on msgCh")
		return nil
	}
}

// oneDeviceOpts 는 지정 device_id 하나를 config 시드로 갖는 direct 옵션을 반환한다.
//
// control_response_timeout 을 0 으로 강제하여 순수 방출(emit) 검증 테스트가 응답 대기 없이
// fire-and-forget 으로 즉시 반환하도록 한다 (B3 기본값 5s 는 에코 주입 전용 테스트에서만 사용).
func oneDeviceOpts(extra map[string]any) map[string]any {
	opts := directOpts()
	opts["control_response_timeout"] = "0s"
	dev := map[string]any{"device_id": "ap-101"}
	for k, v := range extra {
		dev[k] = v
	}
	opts["devices"] = []any{dev}
	return opts
}

// ---------------------------------------------------------------------------
// Module 2 — CRUD
// ---------------------------------------------------------------------------

// Scenario 2.1: add_device → Source="bridge", state 토픽 구독, device_registered 이벤트.
func TestAddDevice_BridgeSubscribeEvent(t *testing.T) {
	ap, mock := directAgentWithMock(t, directOpts())

	resp, err := ap.Process([]byte(`{"command":"add_device","device_id":"ap-101","name":"대합실-A","group_id":"concourse-b1"}`))
	require.NoError(t, err)
	assert.Contains(t, string(resp), `"status":"ok"`)

	dev, err := ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.Equal(t, "bridge", dev.Source)
	assert.False(t, dev.Online, "신규 디바이스는 Online=false")
	assert.Equal(t, "concourse-b1", dev.GroupID)
	assert.Equal(t, "대합실-A", dev.Name)

	assert.Contains(t, mock.subscribedTopics(), "airpurifier/ap-101/state", "state 토픽이 구독되어야 한다")

	evt := readEvent(t, ap)
	assert.Equal(t, "device_registered", evt["type"])
	assert.Equal(t, "ap-101", evt["device_id"])

	// 중복 등록 거부.
	_, err = ap.Process([]byte(`{"command":"add_device","device_id":"ap-101"}`))
	assert.ErrorIs(t, err, ErrDeviceAlreadyRegistered)
}

// Scenario 2.5: add_device 위치 계층 속성(station/place/index) 저장.
func TestAddDevice_LocationAttributes(t *testing.T) {
	ap, _ := directAgentWithMock(t, directOpts())
	_, err := ap.Process([]byte(`{"command":"add_device","device_id":"ap-101","station":"ST-101","place":"승강장","index":3}`))
	require.NoError(t, err)

	dev, err := ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.Equal(t, "ST-101", dev.Station)
	assert.Equal(t, "승강장", dev.Place)
	assert.Equal(t, 3, dev.Index)
}

// Scenario 2.2: remove_device on Source="config" → ErrConfigDeviceProtected.
func TestRemoveDevice_ConfigProtected(t *testing.T) {
	opts := directOpts()
	opts["devices"] = []any{map[string]any{"device_id": "ap-config"}}
	ap, _ := directAgentWithMock(t, opts)

	_, err := ap.Process([]byte(`{"command":"remove_device","device_id":"ap-config"}`))
	assert.ErrorIs(t, err, ErrConfigDeviceProtected)

	// 로스터에 그대로 남아 있어야 한다.
	_, err = ap.GetDevice("ap-config")
	assert.NoError(t, err)
}

// remove_device on bridge device → 제거 + device_unregistered 이벤트. 미등록 → ErrDeviceNotFound.
func TestRemoveDevice_BridgeSuccessAndMissing(t *testing.T) {
	ap, _ := directAgentWithMock(t, directOpts())
	_, err := ap.Process([]byte(`{"command":"add_device","device_id":"ap-101"}`))
	require.NoError(t, err)
	_ = readEvent(t, ap) // device_registered 소비.

	_, err = ap.Process([]byte(`{"command":"remove_device","device_id":"ap-101"}`))
	require.NoError(t, err)
	_, err = ap.GetDevice("ap-101")
	assert.ErrorIs(t, err, ErrDeviceNotFound)

	evt := readEvent(t, ap)
	assert.Equal(t, "device_unregistered", evt["type"])
	assert.Equal(t, "ap-101", evt["device_id"])

	_, err = ap.Process([]byte(`{"command":"remove_device","device_id":"ghost"}`))
	assert.ErrorIs(t, err, ErrDeviceNotFound)
}

// Scenario 2.3: set_device group_id 변경 → GroupMembers 즉시 반영. name 등 미제공 필드는 보존.
func TestSetDevice_GroupIDChangePreservesOthers(t *testing.T) {
	ap, _ := directAgentWithMock(t, oneDeviceOpts(map[string]any{"name": "원본이름"}))

	_, err := ap.Process([]byte(`{"command":"set_device","device_id":"ap-101","group_id":"platform-1"}`))
	require.NoError(t, err)

	dev, err := ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.Equal(t, "platform-1", dev.GroupID)
	assert.Equal(t, "원본이름", dev.Name, "제공되지 않은 필드는 보존되어야 한다")
	assert.Contains(t, ap.GroupMembers("platform-1"), "ap-101")

	// 미등록 디바이스 수정 → ErrDeviceNotFound.
	_, err = ap.Process([]byte(`{"command":"set_device","device_id":"missing","name":"x"}`))
	assert.ErrorIs(t, err, ErrDeviceNotFound)
}

// list_devices 는 전체 디바이스 속성을 JSON 으로 반환한다.
func TestListDevices(t *testing.T) {
	ap, _ := directAgentWithMock(t, oneDeviceOpts(map[string]any{
		"group_id": "g1", "station": "ST-1", "place": "p", "index": 2,
	}))

	resp, err := ap.Process([]byte(`{"command":"list_devices"}`))
	require.NoError(t, err)

	var out map[string]any
	require.NoError(t, json.Unmarshal(resp, &out))
	devs, ok := out["devices"].([]any)
	require.True(t, ok)
	require.Len(t, devs, 1)
	d := devs[0].(map[string]any)
	assert.Equal(t, "ap-101", d["device_id"])
	assert.Equal(t, "g1", d["group_id"])
	assert.Equal(t, "ST-1", d["station"])
	assert.Equal(t, "config", d["source"])
	assert.Equal(t, float64(2), d["index"])
}

// ---------------------------------------------------------------------------
// Module 3 — 개별 2-축 제어
// ---------------------------------------------------------------------------

// Scenario 3.1: set_power on → command 토픽으로 인코딩된 전원 페이로드 발행, status "ok".
func TestSetPower_On(t *testing.T) {
	ap, mock := directAgentWithMock(t, oneDeviceOpts(nil))

	resp, err := ap.Process([]byte(`{"command":"set_power","device_id":"ap-101","params":{"power":true}}`))
	require.NoError(t, err)
	require.Len(t, mock.published, 1)
	assert.Equal(t, "airpurifier/ap-101/cmd", mock.published[0].topic)
	assert.JSONEq(t, `{"power":true}`, string(mock.published[0].payload))
	assert.Contains(t, string(resp), `"status":"ok"`)
}

// Scenario 3.2: set_power off → off 값 인코딩 발행.
func TestSetPower_Off(t *testing.T) {
	ap, mock := directAgentWithMock(t, oneDeviceOpts(nil))
	_, err := ap.Process([]byte(`{"command":"set_power","device_id":"ap-101","params":{"power":false}}`))
	require.NoError(t, err)
	require.Len(t, mock.published, 1)
	assert.JSONEq(t, `{"power":false}`, string(mock.published[0].payload))
}

// Scenario 3.3: set_fan_speed 2 (전원 ON) → fan_speed 인코딩 발행.
func TestSetFanSpeed_PowerOn(t *testing.T) {
	ap, mock := directAgentWithMock(t, oneDeviceOpts(nil))
	ap.FeedState("ap-101", []byte(`{"power":true}`)) // 로스터 전원 ON 관측.

	_, err := ap.Process([]byte(`{"command":"set_fan_speed","device_id":"ap-101","params":{"fan_speed":2}}`))
	require.NoError(t, err)
	require.Len(t, mock.published, 1)
	assert.JSONEq(t, `{"fan_speed":2}`, string(mock.published[0].payload))
}

// Scenario 3.4: set_fan_speed 4 → ErrInvalidFanSpeed, 발행 없음.
func TestSetFanSpeed_InvalidNoEmit(t *testing.T) {
	ap, mock := directAgentWithMock(t, oneDeviceOpts(nil))
	ap.FeedState("ap-101", []byte(`{"power":true}`))

	_, err := ap.Process([]byte(`{"command":"set_fan_speed","device_id":"ap-101","params":{"fan_speed":4}}`))
	assert.ErrorIs(t, err, ErrInvalidFanSpeed)
	assert.Len(t, mock.published, 0, "유효하지 않은 풍량은 발행되지 않아야 한다")
}

// Scenario 3.5: set_fan_speed when power OFF (기본 reject 정책) → ErrPowerOff, 발행 없음.
func TestSetFanSpeed_PowerOffReject(t *testing.T) {
	ap, mock := directAgentWithMock(t, oneDeviceOpts(nil)) // 기본 전원 OFF.

	_, err := ap.Process([]byte(`{"command":"set_fan_speed","device_id":"ap-101","params":{"fan_speed":3}}`))
	assert.ErrorIs(t, err, ErrPowerOff)
	assert.Len(t, mock.published, 0)
}

// power_on_first 정책: 전원 OFF 에서 fan 명령 → 전원 ON 선방출 후 fan 방출.
func TestSetFanSpeed_PowerOffPowerOnFirst(t *testing.T) {
	opts := oneDeviceOpts(nil)
	opts["fan_speed_power_off_policy"] = "power_on_first"
	ap, mock := directAgentWithMock(t, opts)

	_, err := ap.Process([]byte(`{"command":"set_fan_speed","device_id":"ap-101","params":{"fan_speed":3}}`))
	require.NoError(t, err)
	require.Len(t, mock.published, 2)
	assert.JSONEq(t, `{"power":true}`, string(mock.published[0].payload), "전원 ON 이 먼저")
	assert.JSONEq(t, `{"fan_speed":3}`, string(mock.published[1].payload), "풍량이 그다음")
}

// Scenario 3.6: 미등록 디바이스 제어 → ErrDeviceNotFound.
func TestControl_DeviceNotFound(t *testing.T) {
	ap, _ := directAgentWithMock(t, directOpts()) // 등록 디바이스 없음.
	_, err := ap.Process([]byte(`{"command":"set_power","device_id":"ap-999","params":{"power":true}}`))
	assert.ErrorIs(t, err, ErrDeviceNotFound)
}

// Scenario 3.7: set_multiple {power:true, fan_speed:3} → 전원 ON 이 fan 보다 먼저 발행.
func TestSetMultiple_PowerBeforeFan(t *testing.T) {
	ap, mock := directAgentWithMock(t, oneDeviceOpts(nil)) // 전원 OFF 시작.

	_, err := ap.Process([]byte(`{"command":"set_multiple","device_id":"ap-101","params":{"power":true,"fan_speed":3}}`))
	require.NoError(t, err)
	require.Len(t, mock.published, 2)
	assert.JSONEq(t, `{"power":true}`, string(mock.published[0].payload), "전원 ON 이 먼저")
	assert.JSONEq(t, `{"fan_speed":3}`, string(mock.published[1].payload), "풍량이 그다음")
}

// set_multiple 단일 축(power 단독)은 결합 방출 경로를 탄다.
func TestSetMultiple_PowerOnly(t *testing.T) {
	ap, mock := directAgentWithMock(t, oneDeviceOpts(nil))
	_, err := ap.Process([]byte(`{"command":"set_multiple","device_id":"ap-101","params":{"power":true}}`))
	require.NoError(t, err)
	require.Len(t, mock.published, 1)
	assert.JSONEq(t, `{"power":true}`, string(mock.published[0].payload))
}

// device_id 가 없는 제어 명령은 B4 셀렉터 라우팅 전까지 ErrInvalidCommand.
func TestControl_NoDeviceIDInvalid(t *testing.T) {
	ap, _ := directAgentWithMock(t, oneDeviceOpts(nil))
	_, err := ap.Process([]byte(`{"command":"set_power","params":{"power":true}}`))
	assert.ErrorIs(t, err, ErrInvalidCommand)
}

// ---------------------------------------------------------------------------
// port 모드 제어: cmdSink 가 제어 출력 포트로 방출한다.
// ---------------------------------------------------------------------------

// port 모드 set_power → ControlPort 로 인코딩된 페이로드 방출 (direct 와 동일 로직 경로).
func TestControl_PortModeEmit(t *testing.T) {
	opts := map[string]any{
		"transport_mode":           "port",
		"payload_mapping":          validPayloadMapping(),
		"control_response_timeout": "0s", // 순수 방출 검증: 응답 대기 없이 fire-and-forget.
		"devices":                  []any{map[string]any{"device_id": "ap-101"}},
	}
	a, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	_, err = ap.Process([]byte(`{"command":"set_power","device_id":"ap-101","params":{"power":true}}`))
	require.NoError(t, err)

	select {
	case msg := <-ap.ControlPort():
		assert.Equal(t, "ap-101", msg.DeviceID)
		assert.JSONEq(t, `{"power":true}`, string(msg.Payload))
	case <-time.After(time.Second):
		t.Fatal("expected control message on port")
	}
}

// port 모드 add_device 는 구독을 건너뛰고(client==nil) device_registered 를 방출한다.
func TestAddDevice_PortModeNoSubscribe(t *testing.T) {
	opts := map[string]any{
		"transport_mode":  "port",
		"payload_mapping": validPayloadMapping(),
	}
	a, err := NewAirPurifierAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	_, err = ap.Process([]byte(`{"command":"add_device","device_id":"ap-101"}`))
	require.NoError(t, err)
	dev, err := ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.Equal(t, "bridge", dev.Source)

	evt := readEvent(t, ap)
	assert.Equal(t, "device_registered", evt["type"])
}

// 잘못된/누락된 제어 params 는 ErrInvalidCommand 로 거부되며 발행이 없어야 한다.
func TestControl_InvalidParams(t *testing.T) {
	ap, mock := directAgentWithMock(t, oneDeviceOpts(nil))

	// set_power: power 파라미터 누락.
	_, err := ap.Process([]byte(`{"command":"set_power","device_id":"ap-101","params":{}}`))
	assert.ErrorIs(t, err, ErrInvalidCommand)
	// set_power: power 타입 불일치(문자열).
	_, err = ap.Process([]byte(`{"command":"set_power","device_id":"ap-101","params":{"power":"yes"}}`))
	assert.ErrorIs(t, err, ErrInvalidCommand)

	// set_fan_speed: fan_speed 누락 / 타입 불일치.
	ap.FeedState("ap-101", []byte(`{"power":true}`))
	_, err = ap.Process([]byte(`{"command":"set_fan_speed","device_id":"ap-101","params":{}}`))
	assert.ErrorIs(t, err, ErrInvalidCommand)
	_, err = ap.Process([]byte(`{"command":"set_fan_speed","device_id":"ap-101","params":{"fan_speed":"fast"}}`))
	assert.ErrorIs(t, err, ErrInvalidCommand)

	// set_multiple: power/fan_speed 둘 다 없음.
	_, err = ap.Process([]byte(`{"command":"set_multiple","device_id":"ap-101","params":{}}`))
	assert.ErrorIs(t, err, ErrInvalidCommand)
	// set_multiple: 유효하지 않은 fan_speed.
	_, err = ap.Process([]byte(`{"command":"set_multiple","device_id":"ap-101","params":{"fan_speed":9}}`))
	assert.ErrorIs(t, err, ErrInvalidFanSpeed)

	assert.Len(t, mock.published, 0, "거부된 명령은 발행되지 않아야 한다")
}

// set_multiple fan 단독(전원 미포함)은 결합 방출 경로로 fan 페이로드를 방출한다.
func TestSetMultiple_FanOnly(t *testing.T) {
	ap, mock := directAgentWithMock(t, oneDeviceOpts(nil))
	ap.FeedState("ap-101", []byte(`{"power":true}`))

	_, err := ap.Process([]byte(`{"command":"set_multiple","device_id":"ap-101","params":{"fan_speed":2}}`))
	require.NoError(t, err)
	require.Len(t, mock.published, 1)
	assert.JSONEq(t, `{"fan_speed":2}`, string(mock.published[0].payload))
}

// fan_speed_power_off_policy 기본값은 "reject".
func TestConfig_FanSpeedPolicyDefault(t *testing.T) {
	cfg, err := parseAirPurifierConfig(directOpts())
	require.NoError(t, err)
	assert.Equal(t, fanSpeedPolicyReject, cfg.FanSpeedPowerOffPolicy)

	opts := directOpts()
	opts["fan_speed_power_off_policy"] = "power_on_first"
	cfg, err = parseAirPurifierConfig(opts)
	require.NoError(t, err)
	assert.Equal(t, fanSpeedPolicyPowerOnFirst, cfg.FanSpeedPowerOffPolicy)
}
