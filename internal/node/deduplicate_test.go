package node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

func newDeduplicateNode(t *testing.T, config map[string]any) *DeduplicateNode {
	t.Helper()
	def := flow.NewNodeDef("dedup-test", "deduplicate")
	n, err := NewDeduplicateNode(def)
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))
	if config != nil {
		require.NoError(t, n.Configure(config))
	}
	return n.(*DeduplicateNode)
}

func iduMsg(iduNum int, roomTemp float64, setTemp int) message.Message {
	return message.New(message.WithPayload(message.NewPayload(map[string]any{
		"type":                "idu",
		"idu_num":             float64(iduNum),
		"current_temperature": roomTemp,
		"target_temperature":  float64(setTemp),
		"op_mode":             float64(20),
		"fan_byte":            float64(84),
	})))
}

// 첫 메시지는 항상 통과
func TestDeduplicate_FirstMessage_Pass(t *testing.T) {
	n := newDeduplicateNode(t, map[string]any{
		"key":    "idu_num",
		"window": "30s",
	})

	results, err := n.Process(context.Background(), iduMsg(1, 20.5, 22))
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

// 동일 메시지 연속 → 두 번째 폐기
func TestDeduplicate_SameMessage_Drop(t *testing.T) {
	n := newDeduplicateNode(t, map[string]any{
		"key":    "idu_num",
		"window": "30s",
	})

	msg := iduMsg(1, 20.5, 22)
	results, _ := n.Process(context.Background(), msg)
	assert.Len(t, results, 1)

	// 동일 메시지 재전송
	results, _ = n.Process(context.Background(), iduMsg(1, 20.5, 22))
	assert.Len(t, results, 0, "동일 메시지는 폐기되어야 함")
}

// 값이 변경되면 통과
func TestDeduplicate_ValueChanged_Pass(t *testing.T) {
	n := newDeduplicateNode(t, map[string]any{
		"key":    "idu_num",
		"window": "30s",
	})

	results, _ := n.Process(context.Background(), iduMsg(1, 20.5, 22))
	assert.Len(t, results, 1)

	// room_temp 변경
	results, _ = n.Process(context.Background(), iduMsg(1, 21.0, 22))
	assert.Len(t, results, 1, "값이 변경되면 통과해야 함")
}

// 다른 키(idu_num)는 독립
func TestDeduplicate_DifferentKey_Independent(t *testing.T) {
	n := newDeduplicateNode(t, map[string]any{
		"key":    "idu_num",
		"window": "30s",
	})

	results, _ := n.Process(context.Background(), iduMsg(1, 20.5, 22))
	assert.Len(t, results, 1)

	// 다른 IDU
	results, _ = n.Process(context.Background(), iduMsg(2, 20.5, 22))
	assert.Len(t, results, 1, "다른 키는 독립적으로 통과해야 함")
}

// window 초과 시 동일 값이어도 통과
func TestDeduplicate_WindowExpired_Pass(t *testing.T) {
	n := newDeduplicateNode(t, map[string]any{
		"key":    "idu_num",
		"window": "100ms",
	})

	results, _ := n.Process(context.Background(), iduMsg(1, 20.5, 22))
	assert.Len(t, results, 1)

	time.Sleep(150 * time.Millisecond)

	results, _ = n.Process(context.Background(), iduMsg(1, 20.5, 22))
	assert.Len(t, results, 1, "window 초과 후 동일 값도 통과해야 함")
}

// compare_fields 지정 시 해당 필드만 비교
func TestDeduplicate_CompareFields(t *testing.T) {
	n := newDeduplicateNode(t, map[string]any{
		"key":            "idu_num",
		"window":         "30s",
		"compare_fields": "current_temperature,target_temperature",
	})

	results, _ := n.Process(context.Background(), iduMsg(1, 20.5, 22))
	assert.Len(t, results, 1)

	// fan_byte만 다름 → compare_fields에 없으므로 중복
	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"type":                "idu",
		"idu_num":             float64(1),
		"current_temperature": 20.5,
		"target_temperature":  float64(22),
		"op_mode":             float64(20),
		"fan_byte":            float64(20), // 변경
	})))
	results, _ = n.Process(context.Background(), msg2)
	assert.Len(t, results, 0, "compare_fields에 없는 필드 변경은 중복으로 판정")

	// set_temp 변경 → 통과
	results, _ = n.Process(context.Background(), iduMsg(1, 20.5, 25))
	assert.Len(t, results, 1, "compare_fields에 있는 필드 변경은 통과")
}

// on_duplicate=reject_port 시 reject 포트로 전달
func TestDeduplicate_OnDuplicate_RejectPort(t *testing.T) {
	n := newDeduplicateNode(t, map[string]any{
		"key":          "idu_num",
		"window":       "30s",
		"on_duplicate": "reject_port",
	})

	results, _ := n.Process(context.Background(), iduMsg(1, 20.5, 22))
	assert.Len(t, results, 1)

	results, _ = n.Process(context.Background(), iduMsg(1, 20.5, 22))
	require.Len(t, results, 1)
	tp, ok := results[0].Metadata().Get("_target_port")
	assert.True(t, ok)
	assert.Equal(t, "reject", tp)
}

// key 미설정 시 전체 메시지 기준
func TestDeduplicate_NoKey_GlobalDedup(t *testing.T) {
	n := newDeduplicateNode(t, map[string]any{
		"window": "30s",
	})

	results, _ := n.Process(context.Background(), iduMsg(1, 20.5, 22))
	assert.Len(t, results, 1)

	// 동일 페이로드 (idu_num도 동일)
	results, _ = n.Process(context.Background(), iduMsg(1, 20.5, 22))
	assert.Len(t, results, 0)

	// 다른 idu_num → 전체 비교이므로 통과
	results, _ = n.Process(context.Background(), iduMsg(2, 20.5, 22))
	assert.Len(t, results, 1)
}

// 허용오차 이내 → 중복
func TestDeduplicate_Tolerance_WithinRange_Drop(t *testing.T) {
	n := newDeduplicateNode(t, map[string]any{
		"key":            "idu_num",
		"window":         "30s",
		"compare_fields": "current_temperature:0.5, target_temperature",
	})

	results, _ := n.Process(context.Background(), iduMsg(1, 20.5, 22))
	assert.Len(t, results, 1)

	// room_temp 20.5 → 20.8 (차이 0.3, 허용오차 0.5 이내) → 중복
	results, _ = n.Process(context.Background(), iduMsg(1, 20.8, 22))
	assert.Len(t, results, 0, "허용오차 이내는 중복으로 판정")
}

// 허용오차 초과 → 통과
func TestDeduplicate_Tolerance_Exceeded_Pass(t *testing.T) {
	n := newDeduplicateNode(t, map[string]any{
		"key":            "idu_num",
		"window":         "30s",
		"compare_fields": "current_temperature:0.5, target_temperature",
	})

	results, _ := n.Process(context.Background(), iduMsg(1, 20.5, 22))
	assert.Len(t, results, 1)

	// room_temp 20.5 → 21.5 (차이 1.0 > 0.5) → 통과
	results, _ = n.Process(context.Background(), iduMsg(1, 21.5, 22))
	assert.Len(t, results, 1, "허용오차 초과는 변경으로 판정")
}

// 허용오차 필드와 완전 일치 필드 혼합
func TestDeduplicate_Tolerance_MixedFields(t *testing.T) {
	n := newDeduplicateNode(t, map[string]any{
		"key":            "idu_num",
		"window":         "30s",
		"compare_fields": "current_temperature:0.5, target_temperature, op_mode",
	})

	results, _ := n.Process(context.Background(), iduMsg(1, 20.5, 22))
	assert.Len(t, results, 1)

	// room_temp 0.3 차이(허용 내) + set_temp 동일 → 중복
	results, _ = n.Process(context.Background(), iduMsg(1, 20.8, 22))
	assert.Len(t, results, 0)

	// set_temp 변경 (완전 일치 필드) → 통과
	results, _ = n.Process(context.Background(), iduMsg(1, 20.8, 25))
	assert.Len(t, results, 1, "완전 일치 필드 변경은 통과")
}

// 허용오차 경계값: 정확히 0.5 차이 → 동일 (<=)
func TestDeduplicate_Tolerance_ExactBoundary(t *testing.T) {
	n := newDeduplicateNode(t, map[string]any{
		"key":            "idu_num",
		"window":         "30s",
		"compare_fields": "current_temperature:0.5",
	})

	results, _ := n.Process(context.Background(), iduMsg(1, 20.0, 22))
	assert.Len(t, results, 1)

	// 정확히 0.5 차이 → 동일 (|20.0-20.5| = 0.5, <= 0.5)
	results, _ = n.Process(context.Background(), iduMsg(1, 20.5, 22))
	assert.Len(t, results, 0, "경계값 0.5는 동일로 판정")

	// 0.51 차이 → 변경
	results, _ = n.Process(context.Background(), iduMsg(1, 20.51, 22))
	// 이전 통과 값이 20.0이므로 |20.0-20.51| = 0.51 > 0.5 → 통과
	assert.Len(t, results, 1, "경계값 초과는 변경으로 판정")
}

// ---------------------------------------------------------------------------
// v0.18.4: compare_fields 테이블 형식 + missing_field_as_different 옵션
// ---------------------------------------------------------------------------

// TestDeduplicate_CompareFieldsArray 는 array (table) 형식의 compare_fields 가
// legacy string 형식과 동등하게 동작하는지 검증한다.
func TestDeduplicate_CompareFieldsArray(t *testing.T) {
	n := newDeduplicateNode(t, map[string]any{
		"key":    "idu_num",
		"window": "30s",
		"compare_fields": []any{
			map[string]any{"name": "current_temperature", "tolerance": 0.5},
			map[string]any{"name": "target_temperature"},
		},
	})

	// 첫 메시지 통과.
	results, _ := n.Process(context.Background(), iduMsg(1, 20.0, 22))
	assert.Len(t, results, 1)

	// 0.3 차이 (허용 내) + set 동일 → 중복.
	results, _ = n.Process(context.Background(), iduMsg(1, 20.3, 22))
	assert.Len(t, results, 0, "tolerance 0.5 내 동일")

	// set 변경 → 통과.
	results, _ = n.Process(context.Background(), iduMsg(1, 20.3, 25))
	assert.Len(t, results, 1, "target_temperature 변경은 통과")
}

// TestDeduplicate_MissingFieldAsDifferent_True 는 missing_field_as_different=true
// 일 때 비교 필드 중 하나라도 부재면 통과로 판정되는지 검증한다.
func TestDeduplicate_MissingFieldAsDifferent_True(t *testing.T) {
	n := newDeduplicateNode(t, map[string]any{
		"key":                        "idu_num",
		"window":                     "30s",
		"compare_fields":             "current_temperature, target_temperature",
		"missing_field_as_different": true,
	})

	// 첫 메시지 (모든 필드 보유) 통과.
	results, _ := n.Process(context.Background(), iduMsg(1, 20.0, 22))
	assert.Len(t, results, 1)

	// 동일한 두 번째 메시지 → 중복.
	results, _ = n.Process(context.Background(), iduMsg(1, 20.0, 22))
	assert.Len(t, results, 0)

	// target_temperature 가 부재한 메시지 → missing_field_as_different=true 이므로 통과.
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"idu_num":             float64(1),
		"current_temperature": 20.0,
		// target_temperature 누락
	})))
	results, _ = n.Process(context.Background(), msg)
	assert.Len(t, results, 1, "missing_field_as_different=true: 부재 필드는 다름으로 판정 → 통과")
}

// ---------------------------------------------------------------------------
// SPEC-AGENT-METADATA-GROUPING: 그룹핑 key 의 $.-path 지원
// ---------------------------------------------------------------------------

// iduMsgWithDevice 는 device 메타데이터 그룹(id 포함)을 가진 idu 메시지를 만든다.
func iduMsgWithDevice(deviceID string, roomTemp float64, setTemp int) message.Message {
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"type":                "idu",
		"current_temperature": roomTemp,
		"target_temperature":  float64(setTemp),
		"op_mode":             float64(20),
		"fan_byte":            float64(84),
	})))
	msg.Metadata().SetGroup("device", map[string]string{"id": deviceID, "type": "HVACR.IDU"})
	return msg
}

// TestDeduplicate_Key_MetadataGroupPath 는 key 가 "$.metadata.device.id" 일 때
// device 그룹의 id 로 그룹핑되어, 서로 다른 device.id 는 독립적으로 통과하고
// 동일 device.id 의 동일 비교값은 중복으로 판정되는지 검증한다.
func TestDeduplicate_Key_MetadataGroupPath(t *testing.T) {
	n := newDeduplicateNode(t, map[string]any{
		"key":    "$.metadata.device.id",
		"window": "30s",
	})

	// device-1 첫 메시지 → 통과.
	results, _ := n.Process(context.Background(), iduMsgWithDevice("device-1", 20.5, 22))
	assert.Len(t, results, 1)

	// 다른 device-2 (동일 비교값) → 다른 그룹이므로 통과.
	results, _ = n.Process(context.Background(), iduMsgWithDevice("device-2", 20.5, 22))
	assert.Len(t, results, 1, "다른 device.id 는 독립 그룹이므로 통과해야 함")

	// device-1 동일 비교값 재전송 → 동일 그룹 중복으로 판정.
	results, _ = n.Process(context.Background(), iduMsgWithDevice("device-1", 20.5, 22))
	assert.Len(t, results, 0, "동일 device.id 의 동일 값은 중복으로 판정")
}

// TestDeduplicate_Key_NestedPayloadPath 는 key 가 "$.payload.state.mode" 일 때
// 중첩 payload 값으로 그룹핑되는지 검증한다.
func TestDeduplicate_Key_NestedPayloadPath(t *testing.T) {
	n := newDeduplicateNode(t, map[string]any{
		"key":            "$.payload.state.mode",
		"window":         "30s",
		"compare_fields": "current_temperature",
	})

	mk := func(mode int, temp float64) message.Message {
		return message.New(message.WithPayload(message.NewPayload(map[string]any{
			"state":               map[string]any{"mode": float64(mode)},
			"current_temperature": temp,
		})))
	}

	// mode=1 첫 메시지 → 통과.
	results, _ := n.Process(context.Background(), mk(1, 20.5))
	assert.Len(t, results, 1)

	// mode=2 (동일 비교값) → 다른 그룹이므로 통과.
	results, _ = n.Process(context.Background(), mk(2, 20.5))
	assert.Len(t, results, 1, "다른 nested payload 값은 독립 그룹")

	// mode=1 동일 비교값 재전송 → 중복.
	results, _ = n.Process(context.Background(), mk(1, 20.5))
	assert.Len(t, results, 0, "동일 nested payload 그룹의 동일 값은 중복")
}

// TestDeduplicate_Key_LegacyBareName 는 레거시 bare name key("idu_num") 가
// 여전히 top-level payload 필드로 그룹핑되는지 검증한다 (하위 호환).
func TestDeduplicate_Key_LegacyBareName(t *testing.T) {
	n := newDeduplicateNode(t, map[string]any{
		"key":    "idu_num",
		"window": "30s",
	})

	// idu 1 첫 메시지 → 통과.
	results, _ := n.Process(context.Background(), iduMsg(1, 20.5, 22))
	assert.Len(t, results, 1)

	// idu 2 (동일 비교값) → 다른 그룹이므로 통과.
	results, _ = n.Process(context.Background(), iduMsg(2, 20.5, 22))
	assert.Len(t, results, 1, "레거시 bare name: 다른 top-level 값은 독립 그룹")

	// idu 1 동일 비교값 재전송 → 중복.
	results, _ = n.Process(context.Background(), iduMsg(1, 20.5, 22))
	assert.Len(t, results, 0, "레거시 bare name: 동일 그룹 동일 값은 중복")
}

// TestDeduplicate_Key_UnresolvablePath_FallbackAll 은 $.-path 가 해석 불가능할 때
// 모든 메시지가 "_all" 그룹으로 묶여 (비교값 동일 시) 중복 처리되는지 검증한다.
func TestDeduplicate_Key_UnresolvablePath_FallbackAll(t *testing.T) {
	n := newDeduplicateNode(t, map[string]any{
		"key":            "$.metadata.device.id", // device 그룹 없는 메시지 → 해석 불가
		"window":         "30s",
		"compare_fields": "current_temperature",
	})

	mk := func(temp float64) message.Message {
		// device 그룹 없음 → "$.metadata.device.id" 해석 실패 → _all 폴백.
		return message.New(message.WithPayload(message.NewPayload(map[string]any{
			"current_temperature": temp,
		})))
	}

	// 첫 메시지 → 통과.
	results, _ := n.Process(context.Background(), mk(20.5))
	assert.Len(t, results, 1)

	// 동일 비교값 → 같은 _all 그룹이므로 중복.
	results, _ = n.Process(context.Background(), mk(20.5))
	assert.Len(t, results, 0, "해석 불가 경로는 _all 그룹으로 폴백되어 동일 값은 중복")

	// 다른 비교값 → 변경으로 통과.
	results, _ = n.Process(context.Background(), mk(25.0))
	assert.Len(t, results, 1, "_all 그룹 내 비교값 변경은 통과")
}

// TestDeduplicate_MissingFieldAsDifferent_False 는 기본 동작 (false) 에서
// 부재 필드가 nil 로 비교되어 두 번째 누락 메시지는 중복으로 판정되는지 검증한다.
func TestDeduplicate_MissingFieldAsDifferent_False(t *testing.T) {
	n := newDeduplicateNode(t, map[string]any{
		"key":            "idu_num",
		"window":         "30s",
		"compare_fields": "current_temperature, target_temperature",
		// missing_field_as_different 미지정 (기본 false)
	})

	// 첫 메시지 (target_temperature 누락) 통과.
	msg1 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"idu_num":             float64(1),
		"current_temperature": 20.0,
	})))
	results, _ := n.Process(context.Background(), msg1)
	assert.Len(t, results, 1)

	// 동일하게 누락된 두 번째 메시지 → 둘 다 nil 이므로 중복.
	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"idu_num":             float64(1),
		"current_temperature": 20.0,
	})))
	results, _ = n.Process(context.Background(), msg2)
	assert.Len(t, results, 0, "기본 동작: nil==nil 이므로 동일")
}
