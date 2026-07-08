package system

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// === 텔레메트리 빌더 테스트 ===

func TestBuildTelemetry_WithTimestamp(t *testing.T) {
	ts := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	data, err := buildTelemetry("Device A", &ts, map[string]any{"temperature": 42})
	require.NoError(t, err)

	// {"Device A":[{"ts":<UnixMilli>,"values":{"temperature":42}}]}
	var parsed map[string][]map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))

	arr, ok := parsed["Device A"]
	require.True(t, ok, "디바이스 NAME 키가 있어야 한다")
	require.Len(t, arr, 1)

	// ts 는 int64 epoch milliseconds (UnixMilli) 여야 한다.
	tsVal, ok := arr[0]["ts"].(float64) // JSON 숫자는 float64 로 언마샬됨
	require.True(t, ok, "ts 키가 존재해야 한다")
	assert.Equal(t, ts.UnixMilli(), int64(tsVal), "ts 는 time.Time.UnixMilli() 여야 한다")

	values, ok := arr[0]["values"].(map[string]any)
	require.True(t, ok)
	assert.EqualValues(t, 42, values["temperature"])
}

func TestBuildTelemetry_TimestampOmitted(t *testing.T) {
	data, err := buildTelemetry("Device A", nil, map[string]any{"humidity": 55})
	require.NoError(t, err)

	var parsed map[string][]map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))

	arr := parsed["Device A"]
	require.Len(t, arr, 1)

	// ts 가 nil 이면 "ts" 키가 생략되어야 한다 (서버 시각 사용).
	_, hasTS := arr[0]["ts"]
	assert.False(t, hasTS, "ts 미지정 시 ts 키가 생략되어야 한다")

	values := arr[0]["values"].(map[string]any)
	assert.EqualValues(t, 55, values["humidity"])
}

func TestBuildTelemetry_EmptyName(t *testing.T) {
	_, err := buildTelemetry("", nil, map[string]any{"x": 1})
	assert.Error(t, err, "빈 NAME 은 에러여야 한다")
}

func TestBuildTelemetryBatch(t *testing.T) {
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := []telemetryEntry{
		{TS: t1.UnixMilli(), TSSet: true, Values: map[string]any{"a": 1}},
		{TSSet: false, Values: map[string]any{"b": 2}}, // ts 생략
	}
	data, err := buildTelemetryBatch("Dev", entries)
	require.NoError(t, err)

	var parsed map[string][]map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	arr := parsed["Dev"]
	require.Len(t, arr, 2, "배치 항목 2개가 배열로 조립되어야 한다")

	_, has0 := arr[0]["ts"]
	assert.True(t, has0)
	_, has1 := arr[1]["ts"]
	assert.False(t, has1, "두 번째 항목은 ts 를 생략해야 한다")
}

func TestBuildTelemetry_NilValues(t *testing.T) {
	data, err := buildTelemetry("Dev", nil, nil)
	require.NoError(t, err)
	var parsed map[string][]map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	arr := parsed["Dev"]
	require.Len(t, arr, 1)
	values, ok := arr[0]["values"].(map[string]any)
	require.True(t, ok, "nil values 는 빈 객체로 직렬화되어야 한다")
	assert.Empty(t, values)
}

// === 클라이언트 속성 빌더 테스트 ===

func TestBuildClientAttributes(t *testing.T) {
	data, err := buildClientAttributes("Device A", map[string]any{"fw": "1.0", "model": "X"})
	require.NoError(t, err)

	// {"Device A":{"fw":"1.0","model":"X"}}
	var parsed map[string]map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	attrs := parsed["Device A"]
	assert.Equal(t, "1.0", attrs["fw"])
	assert.Equal(t, "X", attrs["model"])
}

func TestBuildClientAttributes_EmptyName(t *testing.T) {
	_, err := buildClientAttributes("", map[string]any{"x": 1})
	assert.Error(t, err)
}

// === RPC 요청 파서 테스트 ===

func TestParseRPCRequest(t *testing.T) {
	raw := []byte(`{"device":"Device A","data":{"id":1,"method":"setValue","params":{"v":10}}}`)
	req, err := parseRPCRequest(raw)
	require.NoError(t, err)

	assert.Equal(t, "Device A", req.Device)
	assert.Equal(t, 1, req.Data.ID)
	assert.Equal(t, "setValue", req.Data.Method)
	assert.EqualValues(t, 10, req.Data.Params["v"])
}

func TestParseRPCRequest_Invalid(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"비-JSON", "not-json"},
		{"device 누락", `{"data":{"id":1,"method":"m"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseRPCRequest([]byte(tt.raw))
			assert.Error(t, err)
		})
	}
}

// === RPC 응답 빌더 테스트 ===

func TestBuildRPCResponse(t *testing.T) {
	data, err := buildRPCResponse("Device A", 1, map[string]any{"success": true})
	require.NoError(t, err)

	// {"device":"Device A","id":1,"data":{"success":true}}
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.Equal(t, "Device A", parsed["device"])
	assert.EqualValues(t, 1, parsed["id"])
	inner := parsed["data"].(map[string]any)
	assert.Equal(t, true, inner["success"])
}

func TestBuildRPCResponse_EmptyName(t *testing.T) {
	_, err := buildRPCResponse("", 1, nil)
	assert.Error(t, err)
}

func TestBuildRPCResponse_NilData(t *testing.T) {
	data, err := buildRPCResponse("Dev", 5, nil)
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	inner, ok := parsed["data"].(map[string]any)
	require.True(t, ok, "nil data 는 빈 객체로 직렬화되어야 한다")
	assert.Empty(t, inner)
}

// === 공유 속성 파서 테스트 ===

func TestParseSharedAttributes(t *testing.T) {
	raw := []byte(`{"device":"Device A","data":{"fw":"1.0"}}`)
	msg, err := parseSharedAttributes(raw)
	require.NoError(t, err)

	assert.Equal(t, "Device A", msg.Device)
	assert.Equal(t, "1.0", msg.Data["fw"])
}

func TestParseSharedAttributes_Invalid(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"비-JSON", "{bad"},
		{"device 누락", `{"data":{"fw":"1.0"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSharedAttributes([]byte(tt.raw))
			assert.Error(t, err)
		})
	}
}

func TestParseSharedAttributes_NilDataDefaultsEmpty(t *testing.T) {
	raw := []byte(`{"device":"Dev"}`)
	msg, err := parseSharedAttributes(raw)
	require.NoError(t, err)
	assert.NotNil(t, msg.Data, "data 누락 시 빈 맵으로 기본화되어야 한다")
	assert.Empty(t, msg.Data)
}

// === (Optional) 속성 요청/응답 상관 테스트 (A8 tolerant) ===

func TestBuildAttributesRequest(t *testing.T) {
	topic, data, err := buildAttributesRequest(7, "Device A", []string{"model"}, []string{"targetTemp"})
	require.NoError(t, err)
	assert.Equal(t, topicGatewayAttributesRequest, topic, "요청 빌더는 발행 토픽을 함께 반환해야 한다")

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	assert.EqualValues(t, 7, parsed["id"])
	assert.Equal(t, "Device A", parsed["device"])

	client := parsed["client"].([]any)
	assert.Equal(t, "model", client[0])
	shared := parsed["shared"].([]any)
	assert.Equal(t, "targetTemp", shared[0])
}

func TestBuildAttributesRequest_EmptyName(t *testing.T) {
	_, _, err := buildAttributesRequest(1, "", nil, nil)
	assert.Error(t, err)
}

func TestParseAttributesResponse_Tolerant(t *testing.T) {
	// A8: 정확한 인코딩은 라이브 검증 전이므로 id/device 만 신뢰하고 value 는 원시 보관.
	raw := []byte(`{"id":7,"device":"Device A","value":{"targetTemp":21}}`)
	topic, resp, err := parseAttributesResponse(raw)
	require.NoError(t, err)
	assert.Equal(t, topicGatewayAttributesResponse, topic, "응답 파서는 상관 구독 토픽을 함께 반환해야 한다")
	assert.Equal(t, 7, resp.ID)
	assert.Equal(t, "Device A", resp.Device)
	assert.NotEmpty(t, resp.Value, "value 는 raw 로 보관되어야 한다")
}

func TestParseAttributesResponse_Invalid(t *testing.T) {
	_, _, err := parseAttributesResponse([]byte("not-json"))
	assert.Error(t, err)
}
