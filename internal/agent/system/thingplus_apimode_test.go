package system

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/message"
)

// thingplus_apimode_test.go 는 api_mode(device | gateway) 설정 및 모드별 라우팅을 검증한다.
//
// device 모드는 기존 기본 동작(v1/devices/me/*)을 그대로 유지한다(회귀 방지).
// gateway 모드는 이전에 dormant 였던 게이트웨이 경로(v1/gateway/*)를 재활성화한다.

// === 설정 파싱: api_mode ===

// TestParseThingplusConfig_APIModeDefault 는 api_mode 미지정 시 기본값이 "device" 임을 확인한다.
func TestParseThingplusConfig_APIModeDefault(t *testing.T) {
	cfg := newThingplusTestConfig(map[string]any{
		"broker": "localhost",
	})
	tc := parseThingplusConfig(cfg)
	assert.Equal(t, "device", tc.APIMode, "api_mode 기본값은 device 여야 한다")
}

// TestParseThingplusConfig_APIModeGateway 는 "gateway" 값이 그대로 반영됨을 확인한다.
func TestParseThingplusConfig_APIModeGateway(t *testing.T) {
	cfg := newThingplusTestConfig(map[string]any{
		"broker":   "localhost",
		"api_mode": "gateway",
	})
	tc := parseThingplusConfig(cfg)
	assert.Equal(t, "gateway", tc.APIMode)
}

// TestParseThingplusConfig_APIModeExplicitDevice 는 "device" 명시가 그대로 반영됨을 확인한다.
func TestParseThingplusConfig_APIModeExplicitDevice(t *testing.T) {
	cfg := newThingplusTestConfig(map[string]any{
		"broker":   "localhost",
		"api_mode": "device",
	})
	tc := parseThingplusConfig(cfg)
	assert.Equal(t, "device", tc.APIMode)
}

// TestParseThingplusConfig_APIModeInvalid 는 알 수 없는/빈 값이 "device" 로 보정됨을 확인한다.
func TestParseThingplusConfig_APIModeInvalid(t *testing.T) {
	for _, v := range []string{"bogus", "", "GATEWAY", "Device"} {
		cfg := newThingplusTestConfig(map[string]any{
			"broker":   "localhost",
			"api_mode": v,
		})
		tc := parseThingplusConfig(cfg)
		assert.Equal(t, "device", tc.APIMode, "잘못된 api_mode(%q)는 device 로 보정되어야 한다", v)
	}
}

// TestIsGatewayMode 는 isGatewayMode() 헬퍼가 APIMode 를 올바르게 반영함을 확인한다.
func TestIsGatewayMode(t *testing.T) {
	a := newTestAgent(t)
	a.cfg.APIMode = "device"
	assert.False(t, a.isGatewayMode())
	a.cfg.APIMode = "gateway"
	assert.True(t, a.isGatewayMode())
}

// newGatewayTestAgent 는 api_mode=gateway 로 설정된 테스트 에이전트를 반환한다.
func newGatewayTestAgent(t *testing.T) *ThingplusGatewayAgent {
	t.Helper()
	a := newTestAgent(t)
	a.cfg.APIMode = "gateway"
	return a
}

// === gateway 모드 업링크: auto-connect + v1/gateway/telemetry ===

// TestGatewayMode_UplinkAutoConnectAndTelemetry 는 gateway 모드에서 업링크가
// (1) v1/gateway/connect 로 auto-connect 하고
// (2) v1/gateway/telemetry 로 {"<NAME>":[{ts,values}]} 형식으로 발행함을 검증한다.
func TestGatewayMode_UplinkAutoConnectAndTelemetry(t *testing.T) {
	a := newGatewayTestAgent(t)
	pub := newFakePublisher() // connected==true

	payload := map[string]any{
		"device":              "unit-1",
		"current_temperature": 24,
	}
	require.NoError(t, a.handleUplink(pub, payload))

	// (1) gateway 모드는 auto-connect 를 발행해야 한다.
	_, hasConnect := pub.findPublish(topicGatewayConnect)
	assert.True(t, hasConnect, "gateway 모드는 v1/gateway/connect 로 auto-connect 해야 한다")

	// (2) 텔레메트리는 v1/gateway/telemetry 로 발행되어야 한다.
	telRec, ok := pub.findPublish(topicGatewayTelemetry)
	require.True(t, ok, "gateway 모드 텔레메트리는 v1/gateway/telemetry 로 발행되어야 한다")

	// gateway 형식: {"unit-1":[{"ts":..,"values":{...}}]}
	var tel map[string]any
	require.NoError(t, json.Unmarshal(telRec.Payload, &tel))
	arr, ok := tel["unit-1"].([]any)
	require.True(t, ok, "최상위 키는 디바이스 NAME 이고 값은 배열이어야 한다")
	require.Len(t, arr, 1)
	entry := arr[0].(map[string]any)
	values := entry["values"].(map[string]any)
	assert.EqualValues(t, 24, values["current_temperature"])

	// device 모드 토픽으로는 발행되지 않아야 한다.
	_, hasDeviceTel := pub.findPublish(topicDeviceTelemetry)
	assert.False(t, hasDeviceTel, "gateway 모드에서 device 텔레메트리 토픽 발행이 없어야 한다")
}

// TestGatewayMode_ClientAttributes 는 gateway 모드 클라이언트 속성이
// v1/gateway/attributes 로 {"<NAME>":{...}} 형식으로 발행됨을 검증한다.
func TestGatewayMode_ClientAttributes(t *testing.T) {
	a := newGatewayTestAgent(t)
	pub := newFakePublisher()

	payload := map[string]any{
		"device":     "unit-1",
		"attributes": map[string]any{"fw": "1.0"},
	}
	require.NoError(t, a.handleUplink(pub, payload))

	attrRec, ok := pub.findPublish(topicGatewayAttributes)
	require.True(t, ok, "gateway 모드 클라이언트 속성은 v1/gateway/attributes 로 발행되어야 한다")

	// {"unit-1":{"fw":"1.0"}} — NAME 래핑.
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(attrRec.Payload, &parsed))
	unit := parsed["unit-1"].(map[string]any)
	assert.Equal(t, "1.0", unit["fw"])
}

// TestGatewayMode_SubscribeDownlink 는 gateway 모드에서 subscribeDownlink 가
// v1/gateway/rpc 와 v1/gateway/attributes 를 구독함을 검증한다.
func TestGatewayMode_SubscribeDownlink(t *testing.T) {
	a := newGatewayTestAgent(t)
	sc := newSubscribeCapture()

	a.subscribeDownlink(sc)

	assert.True(t, sc.subscribed(topicGatewayRPC), "gateway 모드는 v1/gateway/rpc 를 구독해야 한다")
	assert.True(t, sc.subscribed(topicGatewayAttributes), "gateway 모드는 v1/gateway/attributes 를 구독해야 한다")
	// device 모드 토픽은 구독하지 않아야 한다.
	assert.False(t, sc.subscribed(topicDeviceRPCRequestSub), "gateway 모드에서 device RPC 구독이 없어야 한다")
}

// TestGatewayMode_RPCResponsePublishesGatewayRPC 는 gateway 모드에서 플로우 RPC 응답이
// v1/gateway/rpc 로 {"device","id","data"} 형식으로 발행됨을 검증한다(Process 경로).
func TestGatewayMode_RPCResponsePublishesGatewayRPC(t *testing.T) {
	a := newGatewayTestAgent(t)
	pub := newFakePublisher()
	a.mu.Lock()
	a.rpcResponsePublisher = pub // 테스트 발행자 주입
	a.mu.Unlock()

	// 디바이스를 connected 상태로 만든다(게이팅 통과).
	require.NoError(t, a.connectDevice(pub, "Device A"))

	replyMsg := message.New(
		message.WithType("thingplus.rpc.response"),
		message.WithPayload(message.NewPayload(map[string]any{
			"device": "Device A",
			"id":     5,
			"data":   map[string]any{"ok": true},
		})),
	)
	replyBytes, err := replyMsg.MarshalJSON()
	require.NoError(t, err)

	_, procErr := a.Process(replyBytes)
	require.NoError(t, procErr, "gateway 모드 RPC 응답은 성공적으로 발행되어야 한다")

	rpcRec, ok := pub.findPublish(topicGatewayRPC)
	require.True(t, ok, "gateway 모드 RPC 응답은 v1/gateway/rpc 로 발행되어야 한다")
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rpcRec.Payload, &resp))
	assert.Equal(t, "Device A", resp["device"])
	assert.EqualValues(t, 5, resp["id"])
	assert.Equal(t, true, resp["data"].(map[string]any)["ok"])

	// device RPC 응답 토픽으로는 발행되지 않아야 한다.
	_, hasDeviceRPC := pub.findPublish(topicDeviceRPCResponsePrefix + "5")
	assert.False(t, hasDeviceRPC, "gateway 모드에서 device RPC 응답 토픽 발행이 없어야 한다")
}

// TestGatewayMode_RPCDownlinkEmitsRequest 는 gateway 모드 RPC 다운링크(v1/gateway/rpc,
// payload 에 id)가 thingplus.rpc.request 로 방출되고 id 가 payload 에서 추출됨을 검증한다.
func TestGatewayMode_RPCDownlinkEmitsRequest(t *testing.T) {
	a := newGatewayTestAgent(t)

	rpcBytes := []byte(`{"device":"Device A","data":{"id":7,"method":"setValue","params":{"v":10}}}`)
	a.routeDownlink(topicGatewayRPC, rpcBytes)

	emitted := drainRecv(t, a)
	msg, err := message.FromJSON(emitted)
	require.NoError(t, err)
	assert.Equal(t, "thingplus.rpc.request", msg.Type())

	pm := msg.Payload().ToMap()
	assert.EqualValues(t, 7, pm["id"], "gateway RPC 는 payload 에서 id 를 추출해야 한다")
	assert.Equal(t, "Device A", pm["device"])
	assert.Equal(t, "setValue", pm["method"])
}
