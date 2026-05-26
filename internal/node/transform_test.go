package node

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// --- TransformNode 인터페이스 준수 ---

var _ Node = (*TransformNode)(nil)

// --- NewTransformNode 테스트 ---

// TestNewTransformNode_정상생성 은 TransformNode가 올바르게 생성되는지 확인한다.
func TestNewTransformNode_정상생성(t *testing.T) {
	def := flow.NewNodeDef("transform-1", "transform")
	node, err := NewTransformNode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "transform-1", node.Name())
	assert.Equal(t, "transform", node.Type())
}

// --- Init 테스트 ---

// TestTransformNode_Init_상태전이 는 Init 호출 시 Running 상태로 전이하는지 확인한다.
func TestTransformNode_Init_상태전이(t *testing.T) {
	def := flow.NewNodeDef("transform-init", "transform")
	node, _ := NewTransformNode(def)
	tn := node.(*TransformNode)

	err := tn.Init(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, tn.CurrentState())
}

// --- Process 테스트 ---

// TestTransformNode_Process_함수nil_패스스루 는 변환 함수가 nil이면 메시지를 통과시키는지 확인한다.
func TestTransformNode_Process_함수nil_패스스루(t *testing.T) {
	def := flow.NewNodeDef("transform-nil", "transform")
	node, _ := NewTransformNode(def)

	msg := message.New()
	results, err := node.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, msg.ID(), results[0].ID())
}

// TestTransformNode_Process_변환정상 은 변환 함수가 정상적으로 동작하는지 확인한다.
func TestTransformNode_Process_변환정상(t *testing.T) {
	def := flow.NewNodeDef("transform-ok", "transform")
	node, _ := NewTransformNode(def)
	tn := node.(*TransformNode)

	tn.transformFn = func(msg message.Message) (message.Message, error) {
		newMsg := msg.Clone()
		newMsg.Payload().Set("transformed", true)
		return newMsg, nil
	}

	msg := message.New()
	results, err := tn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	v, ok := results[0].Payload().Get("transformed")
	assert.True(t, ok)
	assert.Equal(t, true, v)
}

// TestTransformNode_Process_변환에러 는 변환 함수가 에러를 반환하면 nil과 에러를 반환하는지 확인한다.
func TestTransformNode_Process_변환에러(t *testing.T) {
	def := flow.NewNodeDef("transform-err", "transform")
	node, _ := NewTransformNode(def)
	tn := node.(*TransformNode)

	transformErr := errors.New("변환 실패")
	tn.transformFn = func(msg message.Message) (message.Message, error) {
		return nil, transformErr
	}

	msg := message.New()
	results, err := tn.Process(context.Background(), msg)
	assert.Error(t, err)
	assert.Equal(t, transformErr, err)
	assert.Nil(t, results)
}

// TestTransformNode_Process_메타데이터추가 는 변환으로 메타데이터를 추가할 수 있는지 확인한다.
func TestTransformNode_Process_메타데이터추가(t *testing.T) {
	def := flow.NewNodeDef("transform-meta", "transform")
	node, _ := NewTransformNode(def)
	tn := node.(*TransformNode)

	tn.transformFn = func(msg message.Message) (message.Message, error) {
		newMsg := msg.Clone()
		newMsg.Metadata().Set("processed_by", "transform-meta")
		return newMsg, nil
	}

	msg := message.New()
	results, err := tn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	v, ok := results[0].Metadata().Get("processed_by")
	assert.True(t, ok)
	assert.Equal(t, "transform-meta", v)
}

// --- Configure 테스트 ---

// TestTransformNode_Configure_변환함수설정 은 Configure로 변환 함수를 설정할 수 있는지 확인한다.
func TestTransformNode_Configure_변환함수설정(t *testing.T) {
	def := flow.NewNodeDef("transform-cfg", "transform")
	node, _ := NewTransformNode(def)

	fn := TransformFunc(func(msg message.Message) (message.Message, error) {
		newMsg := msg.Clone()
		newMsg.Payload().Set("via_config", true)
		return newMsg, nil
	})
	err := node.Configure(map[string]any{"transform": fn})
	require.NoError(t, err)

	msg := message.New()
	results, err := node.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)
	v, _ := results[0].Payload().Get("via_config")
	assert.Equal(t, true, v)
}

// TestTransformNode_Configure_nil에러 는 nil config 시 에러를 반환하는지 확인한다.
func TestTransformNode_Configure_nil에러(t *testing.T) {
	def := flow.NewNodeDef("transform-cfg-nil", "transform")
	node, _ := NewTransformNode(def)

	err := node.Configure(nil)
	assert.Error(t, err)
}

// --- Shutdown 테스트 ---

// TestTransformNode_Shutdown_상태전이 는 Shutdown 시 Stopping 상태로 전이하는지 확인한다.
func TestTransformNode_Shutdown_상태전이(t *testing.T) {
	def := flow.NewNodeDef("transform-shut", "transform")
	node, _ := NewTransformNode(def)
	tn := node.(*TransformNode)

	_ = tn.Init(context.Background())
	err := tn.Shutdown(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopping, tn.CurrentState())
}

// --- 동시성 테스트 ---

// TestTransformNode_동시성안전_Process 는 Process가 동시성 안전한지 확인한다.
func TestTransformNode_동시성안전_Process(t *testing.T) {
	def := flow.NewNodeDef("transform-conc", "transform")
	node, _ := NewTransformNode(def)
	tn := node.(*TransformNode)

	tn.transformFn = func(msg message.Message) (message.Message, error) {
		return msg.Clone(), nil
	}

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			msg := message.New()
			_, _ = tn.Process(context.Background(), msg)
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}

// --- strip_nulls 테스트 ---

// TestTransformNode_Configure_StripNulls_nil값제거 는 strip_nulls 옵션이
// 결과 페이로드에서 nil 값을 가진 키를 제거하는지 확인한다.
func TestTransformNode_Configure_StripNulls_nil값제거(t *testing.T) {
	def := flow.NewNodeDef("transform-strip", "transform")
	node, _ := NewTransformNode(def)
	tn := node.(*TransformNode)

	err := tn.Configure(map[string]any{
		"strip_nulls": true,
		"expression":  "{ temp: $.payload.temperature, humidity: $.payload.humidity, missing: $.payload.nonexistent }",
	})
	require.NoError(t, err)

	_ = tn.Init(context.Background())

	// humidity와 nonexistent가 없는 메시지
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"temperature": 22.5,
	})))

	results, err := tn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	payload := results[0].Payload().ToMap()
	assert.Equal(t, 22.5, payload["temp"])
	_, hasHumidity := payload["humidity"]
	assert.False(t, hasHumidity, "nil humidity should be stripped")
	_, hasMissing := payload["missing"]
	assert.False(t, hasMissing, "nil missing should be stripped")
}

// TestTransformNode_MissingPath_OmittedByDefault 는 v0.7.11 의 새 동작을
// 검증한다 — expression 의 object literal 에서 경로가 없으면 해당 필드를
// 결과에 추가하지 않는다 (strip_nulls 옵션과 무관).
//
// 이전 동작 (v0.7.10 까지): 필드 값이 nil 이면 그대로 결과 맵에 포함되어
// 다운스트림이 null 을 받아 처리해야 했음. 사용자 요구: "변환 시 필드가
// 없을 경우 추가하지 않음".
func TestTransformNode_MissingPath_OmittedByDefault(t *testing.T) {
	def := flow.NewNodeDef("transform-missing-default", "transform")
	node, _ := NewTransformNode(def)
	tn := node.(*TransformNode)

	err := tn.Configure(map[string]any{
		"expression": "{ temp: $.payload.temperature, missing: $.payload.nonexistent }",
	})
	require.NoError(t, err)

	_ = tn.Init(context.Background())

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"temperature": 22.5,
	})))

	results, err := tn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	payload := results[0].Payload().ToMap()
	assert.Equal(t, 22.5, payload["temp"])
	_, hasMissing := payload["missing"]
	assert.False(t, hasMissing, "missing path must be omitted from result (v0.7.11)")
}

// TestTransformNode_PreservesUpstreamMessageType 는 transform 노드 (순수
// processor) 가 upstream 의 msg.Type() 을 그대로 유지하는지 확인한다.
// 통일 분류 표준 (SPEC-MESSAGE-TYPE-001): agent 노드가 emit 한 메시지의
// 1급 Type 은 downstream processor 를 지나면서 보존되어야 한다.
func TestTransformNode_PreservesUpstreamMessageType(t *testing.T) {
	def := flow.NewNodeDef("transform-preserve-mt", "transform")
	node, _ := NewTransformNode(def)
	tn := node.(*TransformNode)

	// transform 함수: payload 만 수정하고 metadata 는 손대지 않는다
	tn.transformFn = func(msg message.Message) (message.Message, error) {
		out := msg.Clone()
		out.Payload().Set("processed", true)
		return out, nil
	}

	// upstream agent 노드가 event 분류로 emit 한 메시지를 모사 (v0.12.0: msg.Type).
	msg := message.New()
	msg.SetType("event")
	msg.Metadata().Set("node_source", "poll_bulk")

	results, err := tn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "event", results[0].Type(), "transform 은 upstream msg.Type 을 변경하면 안 된다")
}

// TestTransformNode_StripNulls_PreservesTypeAndTimestamp 는 strip_nulls 가 활성화된
// transform 노드가 message.New 로 새 메시지를 생성하면서도 원본의 Type / Timestamp
// 를 보존하는지 검증한다 (v0.14.0 regression fix).
func TestTransformNode_StripNulls_PreservesTypeAndTimestamp(t *testing.T) {
	def := flow.NewNodeDef("transform-strip-nulls", "transform")
	node, _ := NewTransformNode(def)
	tn := node.(*TransformNode)

	err := tn.Configure(map[string]any{
		"expression":  "{ a: $.payload.a, b: $.payload.missing }",
		"strip_nulls": true,
	})
	require.NoError(t, err)
	_ = tn.Init(context.Background())

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"a": 1})))
	msg.SetType("device_state.change")
	expectedTs := time.UnixMilli(1779350220888)
	msg.SetTimestamp(expectedTs)

	results, err := tn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "device_state.change", results[0].Type(),
		"v0.14.0: strip_nulls 활성화 시에도 Type 이 보존되어야 함 (이전엔 빈 문자열)")
	assert.Equal(t, expectedTs.UnixMilli(), results[0].Timestamp().UnixMilli(),
		"v0.14.0: strip_nulls 활성화 시에도 Timestamp 가 보존되어야 함 (이전엔 time.Now())")
}

// TestTransformNode_MetadataExpression_MergeDoesNotLeakPayload 는
// metadata_expression 의 merge 모드가 payload 값을 metadata 로 leak 시키지
// 않는지 검증한다 (v0.16.0 regression fix).
//
// 이전 버그: metadata_expression 의 merge 모드가 compileExpressionV2 를 재사용
// 하면서 payload 를 base 로 사용 → payload 전체가 metadata 로 복사됨.
func TestTransformNode_MetadataExpression_MergeDoesNotLeakPayload(t *testing.T) {
	def := flow.NewNodeDef("transform-meta-merge", "transform")
	node, _ := NewTransformNode(def)
	tn := node.(*TransformNode)

	// payload 변환: current_temp → current_temperature 로 renaming.
	// metadata 변환: merge 모드로 "mqtt.topic" 추가만.
	err := tn.Configure(map[string]any{
		"expression": []any{
			map[string]any{
				"select": "{ current_temperature: $.payload.current_temperature }",
			},
		},
		"metadata_expression": []any{
			map[string]any{
				"merge": "{ mqtt_topic: $.metadata.device_id }",
			},
		},
	})
	require.NoError(t, err)
	_ = tn.Init(context.Background())

	// payload 에 state 필드들이 있는 LGCNP-like 메시지.
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"current_temperature": 20,
		"mode":                1,
		"fan_speed":           3,
		"power":               true,
	})))
	msg.Metadata().Set("device_id", "idu-3")
	msg.Metadata().Set("device_type", "HVACR.IDU")
	msg.SetType("device_state.change")

	results, err := tn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)
	out := results[0]

	// payload: 변환된 결과만 (current_temperature) — 원본 state 필드는 select 모드라 제거됨.
	pl := out.Payload().ToMap()
	assert.Equal(t, 20, pl["current_temperature"])

	// metadata: 기존 metadata + mqtt_topic. payload 의 state 필드들이 leak 되면 안 됨.
	devID, _ := out.Metadata().Get("device_id")
	assert.Equal(t, "idu-3", devID, "기존 metadata 보존")
	deviceType, _ := out.Metadata().Get("device_type")
	assert.Equal(t, "HVACR.IDU", deviceType, "기존 metadata 보존")
	mqttTopic, ok := out.Metadata().Get("mqtt_topic")
	assert.True(t, ok, "metadata_expression merge 결과가 metadata 에 추가되어야 함")
	assert.Equal(t, "idu-3", mqttTopic)

	// v0.16.0 회귀 검증: payload state 필드가 metadata 로 leak 되지 않음.
	for _, leakKey := range []string{"current_temperature", "mode", "fan_speed", "power"} {
		_, has := out.Metadata().Get(leakKey)
		assert.False(t, has, "v0.16.0: payload key %q 가 metadata 로 leak 되면 안 됨", leakKey)
	}
}

// TestTransformNode_MetadataExpression_SelectReplacesMetadata 는
// metadata_expression 의 select 모드가 metadata 를 완전히 새로 구성 (REPLACE) 하는지
// 검증한다 (v0.16.0).
func TestTransformNode_MetadataExpression_SelectReplacesMetadata(t *testing.T) {
	def := flow.NewNodeDef("transform-meta-select", "transform")
	node, _ := NewTransformNode(def)
	tn := node.(*TransformNode)

	err := tn.Configure(map[string]any{
		"expression": "{ x: $.payload.a }",
		"metadata_expression": []any{
			map[string]any{
				"select": "{ topic: $.metadata.device_id }",
			},
		},
	})
	require.NoError(t, err)
	_ = tn.Init(context.Background())

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"a": 1})))
	msg.Metadata().Set("device_id", "idu-3")
	msg.Metadata().Set("device_type", "HVACR.IDU") // select 모드라 사라져야 함

	results, err := tn.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)
	out := results[0]

	topic, ok := out.Metadata().Get("topic")
	assert.True(t, ok)
	assert.Equal(t, "idu-3", topic)
	// select 모드는 기존 metadata 를 대체 — device_type 사라짐.
	_, hasDeviceType := out.Metadata().Get("device_type")
	assert.False(t, hasDeviceType, "v0.16.0: select 모드는 기존 metadata 를 REPLACE")
}
