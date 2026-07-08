package system

import (
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// newTestAgentWithPath 는 지정한 device_name_path 로 구성된 테스트 에이전트를 만든다.
func newTestAgentWithPath(t *testing.T, path string) *ThingplusGatewayAgent {
	t.Helper()
	a := &ThingplusGatewayAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("thingplus-gateway")),
		cfg: parseThingplusConfig(newThingplusTestConfig(map[string]any{
			"broker":           "localhost",
			"device_name_path": path,
		})),
		devices:    newDeviceStateMap(),
		mapping:    newNameIDMap(),
		recvCh:     make(chan []byte, 16),
		done:       make(chan struct{}),
		stats:      agent.NewAgentStats(),
		logger:     slog.Default(),
		createdAt:  time.Now(),
		upBuf:      newBoundedUplinkBuffer(16),
		pendingRPC: newPendingRPCMap(16),
	}
	return a
}

// marshalMessageWithDeviceGroup 는 payload 와 device 메타데이터 그룹을 가진
// message.Message 를 만들어 FULL JSON(MarshalJSON) 으로 직렬화한다.
// 이는 thingplus-uplink 노드가 agent.Process 로 넘기는 바이트 형태와 동일하다.
func marshalMessageWithDeviceGroup(t *testing.T, payload map[string]any, deviceGroup map[string]string, flat map[string]string) []byte {
	t.Helper()
	m := message.New(message.WithPayload(message.NewPayload(payload)))
	if deviceGroup != nil {
		m.Metadata().SetGroup("device", deviceGroup)
	}
	for k, v := range flat {
		m.Metadata().Set(k, v)
	}
	data, err := m.MarshalJSON()
	require.NoError(t, err, "MarshalJSON 실패")
	return data
}

// TestProcess_MetadataGroupPath_ExtractsDeviceName 는 이 버그의 재현 테스트이다.
//
// 프로덕션 에러:
//
//	thingplus-uplink: publish failed: thingplus: device name 추출 실패
//	(path="$.metadata.device.name"): path not found
//
// device_name_path 가 메시지 메타데이터 그룹 스코프("$.metadata.device.name")일 때,
// decodeInbound 가 메타데이터를 버리므로 추출이 항상 실패했다.
// 수정 후에는 device 그룹의 name 필드에서 NAME 이 정상 추출되어야 한다.
func TestProcess_MetadataGroupPath_ExtractsDeviceName(t *testing.T) {
	a := newTestAgentWithPath(t, "$.metadata.device.name")
	pub := newFakePublisher()
	a.client = nil // uplinkPublisher() 는 nil 을 반환하지만 아래에서 직접 pub 을 주입한다

	before := time.Now().UnixMilli()

	// payload {"temperature":42} + device 메타데이터 그룹 {"name":"Device A"}
	data := marshalMessageWithDeviceGroup(t,
		map[string]any{"temperature": 42},
		map[string]string{"name": "Device A"},
		nil,
	)

	// decodeInbound 를 통과한 뒤 metadata-aware 추출을 검증하기 위해
	// Process 대신 Process 내부 경로와 동일하게 검증한다.
	msgType, msg, payload, err := decodeInbound(data)
	require.NoError(t, err, "decodeInbound 는 message.Message 를 복원해야 한다")
	require.Equal(t, "", msgType)
	require.NotNil(t, msg, "decodeInbound 는 메타데이터 보존을 위해 message 를 반환해야 한다")

	require.NoError(t, a.handleUplinkFromMessage(pub, msg, payload),
		"메타데이터 그룹 스코프 device_name_path 로 업링크가 성공해야 한다")

	// 텔레메트리 ts 는 handleUplinkFromMessage 내부에서 생성되므로 호출 이후에 상한을 캡처한다.
	after := time.Now().UnixMilli()

	// Device API: gateway connect 발행이 없어야 한다.
	_, hasConnect := pub.findPublish(topicGatewayConnect)
	assert.False(t, hasConnect, "Device API 는 gateway connect 를 발행하면 안 된다")

	// 텔레메트리 발행 검증(Device API): {"ts","values":{"Device A":{"temperature":42}}}
	telRec, ok := pub.findPublish(topicDeviceTelemetry)
	require.True(t, ok, "텔레메트리가 v1/devices/me/telemetry 로 발행되어야 한다")

	var tel map[string]any
	require.NoError(t, json.Unmarshal(telRec.Payload, &tel))

	tsVal := int64(tel["ts"].(float64))
	assert.GreaterOrEqual(t, tsVal, before)
	assert.LessOrEqual(t, tsVal, after)

	values := tel["values"].(map[string]any)
	unit := values["Device A"].(map[string]any)
	assert.EqualValues(t, 42, unit["temperature"])
}

// TestProcess_MetadataGroupPath_EndToEnd 는 Process() 전체 경로(decodeInbound 포함)를
// 통해 메타데이터 그룹 스코프 device_name_path 가 "path not found" 없이 처리됨을 검증한다.
//
// a.client 는 mqtt.Client 전체 인터페이스라 fakePublisher 를 주입할 수 없으므로,
// 미연결(client==nil) 상태로 Process 를 호출한다. 버그가 있으면 발행 이전 단계인
// device name 추출에서 "path not found" 에러가 반환된다. 수정 후에는 미연결이므로
// 무손실 버퍼링되어 에러 없이 통과한다.
func TestProcess_MetadataGroupPath_EndToEnd(t *testing.T) {
	a := newTestAgentWithPath(t, "$.metadata.device.name")
	// a.client == nil → uplinkPublisher() 는 nil → 미연결 버퍼링 경로.

	data := marshalMessageWithDeviceGroup(t,
		map[string]any{"temperature": 42},
		map[string]string{"name": "Device A"},
		nil,
	)

	_, err := a.Process(data)
	require.NoError(t, err,
		"Process 는 메타데이터 그룹 스코프 device_name_path 에서 'path not found' 없이 통과해야 한다")

	// 미연결이므로 텔레메트리는 업링크 버퍼에 무손실 적재되어야 한다.
	assert.Positive(t, a.upBuf.len(), "미연결 시 텔레메트리가 버퍼링되어야 한다")
	// device NAME 이 정상 해석되어 매핑에 등록되었는지 확인한다.
	_, ok := a.devices.get("Device A")
	assert.True(t, ok, "메타데이터에서 추출한 NAME 으로 디바이스가 등록되어야 한다")
}

// TestProcess_MetadataFlatPath_ExtractsDeviceName 는 flat 메타데이터 키 스코프
// ("$.metadata.device_id") 추출을 검증한다.
func TestProcess_MetadataFlatPath_ExtractsDeviceName(t *testing.T) {
	a := newTestAgentWithPath(t, "$.metadata.device_id")
	pub := newFakePublisher()

	data := marshalMessageWithDeviceGroup(t,
		map[string]any{"temperature": 7},
		nil,
		map[string]string{"device_id": "Device Flat"},
	)

	_, msg, payload, err := decodeInbound(data)
	require.NoError(t, err)
	require.NotNil(t, msg)

	require.NoError(t, a.handleUplinkFromMessage(pub, msg, payload))

	// Device API: NAME 은 텔레메트리 values 의 키로 확인한다(connect 발행 없음).
	telRec, ok := pub.findPublish(topicDeviceTelemetry)
	require.True(t, ok)
	var tel map[string]any
	require.NoError(t, json.Unmarshal(telRec.Payload, &tel))
	_, ok = tel["values"].(map[string]any)["Device Flat"]
	assert.True(t, ok, "메타데이터 flat 키에서 NAME 이 추출되어 values 키가 되어야 한다")
}

// TestProcess_PayloadScopedExplicitPath 는 명시적 payload 스코프
// ("$.payload.dev") 추출을 검증한다.
func TestProcess_PayloadScopedExplicitPath(t *testing.T) {
	a := newTestAgentWithPath(t, "$.payload.dev")
	pub := newFakePublisher()

	data := marshalMessageWithDeviceGroup(t,
		map[string]any{"dev": "Device Payload", "temperature": 5},
		map[string]string{"name": "IGNORED"}, // 메타데이터는 무시되어야 한다
		nil,
	)

	_, msg, payload, err := decodeInbound(data)
	require.NoError(t, err)
	require.NotNil(t, msg)

	require.NoError(t, a.handleUplinkFromMessage(pub, msg, payload))

	// Device API: payload 스코프 경로에서 추출한 NAME 이 텔레메트리 values 키가 되어야 한다.
	telRec, ok := pub.findPublish(topicDeviceTelemetry)
	require.True(t, ok)
	var tel map[string]any
	require.NoError(t, json.Unmarshal(telRec.Payload, &tel))
	_, ok = tel["values"].(map[string]any)["Device Payload"]
	assert.True(t, ok, "payload 스코프 경로는 payload 에서 NAME 을 추출해야 한다")
}

// TestProcess_DefaultBarePath_BackwardCompatible 는 기본값 "$.device" (payload 스코프)
// 가 계속 동작함을 검증한다 (하위 호환).
func TestProcess_DefaultBarePath_BackwardCompatible(t *testing.T) {
	a := newTestAgentWithPath(t, "$.device")
	pub := newFakePublisher()

	// message.Message 로 온 경우에도 bare 경로는 payload 를 본다.
	data := marshalMessageWithDeviceGroup(t,
		map[string]any{"device": "Device Bare", "temperature": 3},
		nil,
		nil,
	)

	_, msg, payload, err := decodeInbound(data)
	require.NoError(t, err)
	require.NotNil(t, msg)

	require.NoError(t, a.handleUplinkFromMessage(pub, msg, payload))

	// Device API: bare 경로(기본 $.device)에서 추출한 NAME 이 텔레메트리 values 키가 되어야 한다.
	telRec, ok := pub.findPublish(topicDeviceTelemetry)
	require.True(t, ok)
	var tel map[string]any
	require.NoError(t, json.Unmarshal(telRec.Payload, &tel))
	_, ok = tel["values"].(map[string]any)["Device Bare"]
	assert.True(t, ok, "bare 경로에서 payload NAME 이 추출되어 values 키가 되어야 한다")
}

// TestProcess_MetadataPath_RawJSONFallback 는 raw JSON(메타데이터 없음)에 대해
// 메타데이터 스코프 경로가 명확한 에러를 반환함을 검증한다.
func TestProcess_MetadataPath_RawJSONFallback(t *testing.T) {
	a := newTestAgentWithPath(t, "$.metadata.device.name")
	pub := newFakePublisher()

	// message.Message 가 아닌 raw JSON payload
	raw := []byte(`{"temperature":1}`)

	msgType, msg, payload, err := decodeInbound(raw)
	require.NoError(t, err)
	require.Equal(t, "", msgType)
	require.Nil(t, msg, "raw JSON 은 message 를 반환하지 않아야 한다")

	err = a.handleUplinkFromMessage(pub, msg, payload)
	require.Error(t, err, "메타데이터 없는 raw JSON 에 메타데이터 스코프 경로는 실패해야 한다")
}

// TestResolveDeviceNameFromMessage 는 resolveDeviceNameFromMessage 의 모든 경로 형태와
// 에러 분기를 직접 검증한다 (스코프별 해석 + 하위 호환 + 방어 경로).
func TestResolveDeviceNameFromMessage(t *testing.T) {
	// device 그룹 + flat 키 + payload 를 가진 메시지 픽스처.
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"device": "PayloadDevice",
		"dev":    "PayloadDev",
	})))
	msg.Metadata().SetGroup("device", map[string]string{"name": "GroupName", "empty": ""})
	msg.Metadata().Set("device_id", "FlatID")
	msg.Metadata().Set("blank", "")
	payload := msg.Payload().ToMap()

	tests := []struct {
		name    string
		msg     message.Message
		path    string
		want    string
		wantErr bool
	}{
		// 메타데이터 그룹 스코프
		{name: "metadata group name", msg: msg, path: "$.metadata.device.name", want: "GroupName"},
		{name: "metadata group missing key", msg: msg, path: "$.metadata.device.missing", wantErr: true},
		{name: "metadata group empty value", msg: msg, path: "$.metadata.device.empty", wantErr: true},
		{name: "metadata missing group", msg: msg, path: "$.metadata.nogroup.name", wantErr: true},
		// 메타데이터 flat 스코프
		{name: "metadata flat key", msg: msg, path: "$.metadata.device_id", want: "FlatID"},
		{name: "metadata flat missing", msg: msg, path: "$.metadata.nope", wantErr: true},
		{name: "metadata flat empty value", msg: msg, path: "$.metadata.blank", wantErr: true},
		// payload 명시 스코프
		{name: "payload explicit dev", msg: msg, path: "$.payload.dev", want: "PayloadDev"},
		{name: "payload explicit missing", msg: msg, path: "$.payload.nope", wantErr: true},
		{name: "payload explicit empty rest", msg: msg, path: "$.payload.", wantErr: true},
		// bare payload 스코프 (기본/하위 호환)
		{name: "bare default device", msg: msg, path: "$.device", want: "PayloadDevice"},
		{name: "bare missing", msg: msg, path: "$.nope", wantErr: true},
		// nil 메시지 (raw JSON)
		{name: "nil msg metadata path errors", msg: nil, path: "$.metadata.device.name", wantErr: true},
		{name: "nil msg payload path works", msg: nil, path: "$.device", want: "PayloadDevice"},
		{name: "nil msg bare missing errors", msg: nil, path: "$.nope", wantErr: true},
		// 잘못된 경로
		{name: "metadata empty rest", msg: msg, path: "$.metadata.", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveDeviceNameFromMessage(tt.msg, payload, tt.path)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
