package node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// modbus-remap 노드 테스트 (SPEC-MODBUS-007 AC-01~06 + 엣지)
// ---------------------------------------------------------------------------

// newRemapNode 는 Configure + Init 까지 완료한 ModbusRemapNode 를 만든다.
func newRemapNode(t *testing.T, config map[string]any) *ModbusRemapNode {
	t.Helper()
	def := flow.NewNodeDef("test-remap", "modbus-remap")
	node, err := NewModbusRemapNode(def)
	require.NoError(t, err)
	n := node.(*ModbusRemapNode)
	require.NoError(t, n.Configure(config))
	require.NoError(t, n.Init(context.Background()))
	return n
}

// makeReadMsg 는 modbus-read 출력 형태의 입력 메시지를 만든다.
func makeReadMsg(success bool, agentType string, entries []any) message.Message {
	msg := message.New()
	msg.Payload().Set("success", success)
	msg.Payload().Set("agent_type", agentType)
	msg.Payload().Set("values", entries)
	return msg
}

// outEntries 는 remap 출력 메시지에서 values[] 를 꺼낸다.
func outEntries(t *testing.T, out message.Message) []map[string]any {
	t.Helper()
	v, ok := out.Payload().Get("values")
	require.True(t, ok, "출력에 values 없음")
	entries, ok := v.([]map[string]any)
	require.True(t, ok, "values 타입 불일치")
	return entries
}

func outBool(t *testing.T, out message.Message, key string) bool {
	t.Helper()
	v, ok := out.Payload().Get(key)
	require.True(t, ok, "출력에 %s 없음", key)
	b, ok := v.(bool)
	require.True(t, ok)
	return b
}

// ---------------------------------------------------------------------------
// AC-01 — Happy path: 엔트리 전체 remap (주소 + area + device_id 재작성)
// ---------------------------------------------------------------------------

func TestModbusRemap_AC01_WholeEntryRemap(t *testing.T) {
	n := newRemapNode(t, map[string]any{
		"rules": []any{
			map[string]any{
				"source_area":    "holding_registers",
				"source_address": 100,
				"count":          5,
				"target_unit_id": 3,
				"target_area":    "input_registers",
				"target_address": 200,
			},
		},
	})

	in := makeReadMsg(true, "server", []any{
		map[string]any{"index": 0, "area": "holding_registers", "address": 100, "count": 5,
			"values": []any{10, 20, 30, 40, 50}},
	})

	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, out, 1)

	assert.True(t, outBool(t, out[0], "success"))
	entries := outEntries(t, out[0])
	require.Len(t, entries, 1)
	e := entries[0]
	assert.Equal(t, "input_registers", e["area"])
	assert.Equal(t, uint16(200), e["address"])
	assert.Equal(t, uint16(5), e["count"])
	assert.Equal(t, uint8(3), e["unit_id"])
	assert.Equal(t, []any{10, 20, 30, 40, 50}, e["values"], "값 위치 보존")
}

// ---------------------------------------------------------------------------
// AC-02 — 템플릿 적용 → 인스턴스화된 규칙이 정상 remap
// ---------------------------------------------------------------------------

func TestModbusRemap_AC02_TemplateApply(t *testing.T) {
	n := newRemapNode(t, map[string]any{
		"templates": []any{
			map[string]any{
				"area":      "holding_registers",
				"offset":    1000,
				"device_id": 7,
				"start":     100,
				"count":     3,
			},
		},
	})

	in := makeReadMsg(true, "server", []any{
		map[string]any{"index": 0, "area": "holding_registers", "address": 100, "count": 3,
			"values": []any{1, 2, 3}},
	})

	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, out, 1)

	assert.True(t, outBool(t, out[0], "success"))
	entries := outEntries(t, out[0])
	require.Len(t, entries, 1)
	e := entries[0]
	assert.Equal(t, "holding_registers", e["area"], "target_area = source area(별도 지정 없음)")
	assert.Equal(t, uint16(1100), e["address"], "target_address = start + offset")
	assert.Equal(t, uint16(3), e["count"])
	assert.Equal(t, uint8(7), e["unit_id"], "unit_id = device_id")
	assert.Equal(t, []any{1, 2, 3}, e["values"])
}

// ---------------------------------------------------------------------------
// AC-03 — 미매칭 → 거부 (success=false + 규칙별 error, error 페이로드 승계)
// ---------------------------------------------------------------------------

func TestModbusRemap_AC03_NoMatchRejected(t *testing.T) {
	n := newRemapNode(t, map[string]any{
		"rules": []any{
			map[string]any{
				"source_area":    "holding_registers",
				"source_address": 100,
				"count":          10, // 엔트리 count(5)와 불일치 → 정확 매칭 실패
				"target_unit_id": 2,
				"target_address": 500,
			},
		},
	})

	in := makeReadMsg(true, "server", []any{
		map[string]any{"index": 0, "area": "holding_registers", "address": 100, "count": 5,
			"values": []any{1, 2, 3, 4, 5}},
	})

	out, err := n.Process(context.Background(), in)
	require.NoError(t, err, "규칙 미매칭은 error port(nil,err)가 아니라 success=false payload 로 승계")
	require.Len(t, out, 1)

	assert.False(t, outBool(t, out[0], "success"), "부분 매핑 금지 → success=false")
	entries := outEntries(t, out[0])
	assert.Empty(t, entries, "미매칭 규칙은 출력 엔트리를 만들지 않음")

	errsRaw, ok := out[0].Payload().Get("errors")
	require.True(t, ok, "규칙별 error 표기")
	errs := errsRaw.([]map[string]any)
	require.Len(t, errs, 1)
	assert.Equal(t, 0, errs[0]["rule_index"])
	assert.Contains(t, errs[0]["reason"], "no entry exactly matches")
}

// ---------------------------------------------------------------------------
// AC-04 — 값 보존 불변식 (values 및 raw)
// ---------------------------------------------------------------------------

func TestModbusRemap_AC04_ValuePreservation(t *testing.T) {
	n := newRemapNode(t, map[string]any{
		"rules": []any{
			map[string]any{"source_area": "holding_registers", "source_address": 0, "count": 2,
				"target_unit_id": 1, "target_address": 50},
			map[string]any{"source_area": "input_registers", "source_address": 10, "count": 4,
				"target_unit_id": 1, "target_address": 60},
		},
	})

	rawB64 := "AAECAwQF"
	in := makeReadMsg(true, "client", []any{
		map[string]any{"index": 0, "area": "holding_registers", "address": 0, "count": 2,
			"values": []any{111, 222}, "data_type": "uint16"},
		map[string]any{"index": 1, "area": "input_registers", "address": 10, "count": 4,
			"raw": rawB64},
	})

	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	entries := outEntries(t, out[0])
	require.Len(t, entries, 2)

	assert.Equal(t, []any{111, 222}, entries[0]["values"], "values 순서/내용 보존")
	assert.Equal(t, "uint16", entries[0]["data_type"], "data_type 보존")
	assert.Equal(t, rawB64, entries[1]["raw"], "raw 바이트 보존")
	assert.NotContains(t, entries[0], "raw")
	assert.NotContains(t, entries[1], "values")
}

// ---------------------------------------------------------------------------
// AC-05 — 비-modbus-read 페이로드 거부 (error port)
// ---------------------------------------------------------------------------

func TestModbusRemap_AC05_InvalidInputRejected(t *testing.T) {
	n := newRemapNode(t, map[string]any{
		"rules": []any{
			map[string]any{"source_area": "coils", "source_address": 0, "count": 1,
				"target_unit_id": 1, "target_address": 0},
		},
	})

	// values[] 없는 임의 페이로드
	msg := message.New()
	msg.Payload().Set("foo", "bar")

	out, err := n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrModbusRemapInvalidInput)
	assert.Nil(t, out, "error port → 메시지 방출 없음")
}

// ---------------------------------------------------------------------------
// AC-06 — payload 오버라이드가 config 기본을 이김
// ---------------------------------------------------------------------------

func TestModbusRemap_AC06_PayloadOverride(t *testing.T) {
	// config 기본 규칙 A: holding 0/1 → unit 1 addr 0 (엔트리와 불일치하도록 설정)
	n := newRemapNode(t, map[string]any{
		"rules": []any{
			map[string]any{"source_area": "coils", "source_address": 999, "count": 1,
				"target_unit_id": 1, "target_address": 0},
		},
	})

	in := makeReadMsg(true, "server", []any{
		map[string]any{"index": 0, "area": "holding_registers", "address": 100, "count": 2,
			"values": []any{7, 8}},
	})
	// payload 규칙 B: 엔트리와 매칭 → unit 9 area input_registers addr 300
	in.Payload().Set("rules", []any{
		map[string]any{"source_area": "holding_registers", "source_address": 100, "count": 2,
			"target_unit_id": 9, "target_area": "input_registers", "target_address": 300},
	})

	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	assert.True(t, outBool(t, out[0], "success"), "payload 규칙 B로 매칭 성공")
	entries := outEntries(t, out[0])
	require.Len(t, entries, 1)
	assert.Equal(t, "input_registers", entries[0]["area"])
	assert.Equal(t, uint16(300), entries[0]["address"])
	assert.Equal(t, uint8(9), entries[0]["unit_id"])
}

// ---------------------------------------------------------------------------
// 엣지: 다중 엔트리 × 다중 규칙 (1:1 매칭, 매칭된 것만 remap)
// ---------------------------------------------------------------------------

func TestModbusRemap_Edge_MultiEntryMultiRule(t *testing.T) {
	n := newRemapNode(t, map[string]any{
		"rules": []any{
			map[string]any{"source_area": "holding_registers", "source_address": 0, "count": 2,
				"target_unit_id": 1, "target_area": "input_registers", "target_address": 100},
			map[string]any{"source_area": "coils", "source_address": 10, "count": 3,
				"target_unit_id": 2, "target_address": 20},
		},
	})

	in := makeReadMsg(true, "server", []any{
		map[string]any{"index": 0, "area": "holding_registers", "address": 0, "count": 2, "values": []any{1, 2}},
		map[string]any{"index": 1, "area": "coils", "address": 10, "count": 3, "values": []any{true, false, true}},
		map[string]any{"index": 2, "area": "input_registers", "address": 5, "count": 1, "values": []any{9}}, // 매칭 규칙 없음 → 드롭
	})

	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	assert.True(t, outBool(t, out[0], "success"))
	entries := outEntries(t, out[0])
	require.Len(t, entries, 2, "매칭된 2개 규칙만 출력")

	assert.Equal(t, "input_registers", entries[0]["area"])
	assert.Equal(t, uint16(100), entries[0]["address"])
	assert.Equal(t, uint8(1), entries[0]["unit_id"])

	assert.Equal(t, "coils", entries[1]["area"], "target_area 미지정 → source_area 유지")
	assert.Equal(t, uint16(20), entries[1]["address"])
	assert.Equal(t, uint8(2), entries[1]["unit_id"])
}

// ---------------------------------------------------------------------------
// 엣지: templates payload 오버라이드도 config 기본을 대체
// ---------------------------------------------------------------------------

func TestModbusRemap_Edge_TemplateOverride(t *testing.T) {
	n := newRemapNode(t, map[string]any{
		"templates": []any{
			map[string]any{"area": "holding_registers", "offset": 1, "device_id": 1, "start": 999, "count": 1},
		},
	})

	in := makeReadMsg(true, "server", []any{
		map[string]any{"index": 0, "area": "holding_registers", "address": 100, "count": 3, "values": []any{1, 2, 3}},
	})
	in.Payload().Set("templates", []any{
		map[string]any{"area": "holding_registers", "offset": 500, "device_id": 4, "start": 100, "count": 3},
	})

	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	assert.True(t, outBool(t, out[0], "success"))
	entries := outEntries(t, out[0])
	require.Len(t, entries, 1)
	assert.Equal(t, uint16(600), entries[0]["address"], "payload 템플릿: 100+500")
	assert.Equal(t, uint8(4), entries[0]["unit_id"])
}

// ---------------------------------------------------------------------------
// 엣지: 빈 규칙 셋 → error
// ---------------------------------------------------------------------------

func TestModbusRemap_Edge_EmptyRules(t *testing.T) {
	n := newRemapNode(t, map[string]any{}) // rules/templates 없음

	in := makeReadMsg(true, "server", []any{
		map[string]any{"index": 0, "area": "holding_registers", "address": 0, "count": 1, "values": []any{1}},
	})

	out, err := n.Process(context.Background(), in)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrModbusRemapEmptyRules)
	assert.Nil(t, out)
}

// ---------------------------------------------------------------------------
// 엣지: boolean area(coils) 값 표현 보존 + area 변경
// ---------------------------------------------------------------------------

func TestModbusRemap_Edge_BooleanAreaPreserved(t *testing.T) {
	n := newRemapNode(t, map[string]any{
		"rules": []any{
			map[string]any{"source_area": "coils", "source_address": 0, "count": 3,
				"target_unit_id": 5, "target_area": "discrete_inputs", "target_address": 8},
		},
	})

	in := makeReadMsg(true, "server", []any{
		map[string]any{"index": 0, "area": "coils", "address": 0, "count": 3, "values": []any{true, false, true}},
	})

	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	entries := outEntries(t, out[0])
	require.Len(t, entries, 1)
	assert.Equal(t, "discrete_inputs", entries[0]["area"], "area 변경 반영")
	assert.Equal(t, []any{true, false, true}, entries[0]["values"], "boolean 값 표현 보존")
	assert.Equal(t, uint16(8), entries[0]["address"])
}

// ---------------------------------------------------------------------------
// M1 State-driven: 입력 success=false 승계
// ---------------------------------------------------------------------------

func TestModbusRemap_InputFailureInherited(t *testing.T) {
	n := newRemapNode(t, map[string]any{
		"rules": []any{
			map[string]any{"source_area": "holding_registers", "source_address": 0, "count": 1,
				"target_unit_id": 1, "target_address": 10},
		},
	})

	in := makeReadMsg(false, "server", []any{ // 입력 success=false
		map[string]any{"index": 0, "area": "holding_registers", "address": 0, "count": 1, "values": []any{42}},
	})

	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	assert.False(t, outBool(t, out[0], "success"), "입력 실패 상태 승계(성공으로 왜곡 금지)")
	entries := outEntries(t, out[0])
	require.Len(t, entries, 1, "규칙 자체는 매칭됨")
}

// ---------------------------------------------------------------------------
// Change 1 (a) — source_unit_id 매칭: 입력 엔트리에 unit_id 가 있을 때(체인 remap)
// ---------------------------------------------------------------------------

func TestModbusRemap_SourceUnitID_MatchWhenEntryHasUnitID(t *testing.T) {
	n := newRemapNode(t, map[string]any{
		"rules": []any{
			// unit 5 만 매칭
			map[string]any{"source_unit_id": 5, "source_area": "holding_registers", "source_address": 0, "count": 2,
				"target_unit_id": 9, "target_area": "input_registers", "target_address": 100},
		},
	})

	// 상류가 또 다른 modbus-remap 인 경우 — 엔트리에 unit_id 포함
	in := makeReadMsg(true, "server", []any{
		map[string]any{"index": 0, "area": "holding_registers", "address": 0, "count": 2, "unit_id": 7, "values": []any{1, 2}}, // unit 7 → 불일치
		map[string]any{"index": 1, "area": "holding_registers", "address": 0, "count": 2, "unit_id": 5, "values": []any{3, 4}}, // unit 5 → 일치
	})

	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	assert.True(t, outBool(t, out[0], "success"), "unit 5 엔트리와 매칭")
	entries := outEntries(t, out[0])
	require.Len(t, entries, 1)
	assert.Equal(t, []any{3, 4}, entries[0]["values"], "unit 5 엔트리(값 3,4)가 선택됨")
	assert.Equal(t, uint8(9), entries[0]["unit_id"])
	assert.Equal(t, "input_registers", entries[0]["area"])
	assert.Equal(t, uint16(100), entries[0]["address"])
}

// source_unit_id 지정 + 입력 엔트리 unit_id 모두 불일치 → 미매칭 거부
func TestModbusRemap_SourceUnitID_NoMatchWhenUnitIDDiffers(t *testing.T) {
	n := newRemapNode(t, map[string]any{
		"rules": []any{
			map[string]any{"source_unit_id": 5, "source_area": "holding_registers", "source_address": 0, "count": 2,
				"target_unit_id": 9, "target_address": 100},
		},
	})
	in := makeReadMsg(true, "server", []any{
		map[string]any{"index": 0, "area": "holding_registers", "address": 0, "count": 2, "unit_id": 7, "values": []any{1, 2}},
	})
	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	assert.False(t, outBool(t, out[0], "success"), "unit_id 불일치 → 미매칭 거부")
	assert.Empty(t, outEntries(t, out[0]))
}

// ---------------------------------------------------------------------------
// Change 1 (b) — source_unit_id 무시: 입력 엔트리에 unit_id 가 없을 때(일반 modbus-read)
// ---------------------------------------------------------------------------

func TestModbusRemap_SourceUnitID_IgnoredWhenEntryLacksUnitID(t *testing.T) {
	n := newRemapNode(t, map[string]any{
		"rules": []any{
			map[string]any{"source_unit_id": 5, "source_area": "holding_registers", "source_address": 0, "count": 2,
				"target_unit_id": 9, "target_address": 100},
		},
	})

	// 일반 modbus-read 출력 — 엔트리에 unit_id 없음 → unit_id 제약 무시, area+address+count 로만 매칭
	in := makeReadMsg(true, "server", []any{
		map[string]any{"index": 0, "area": "holding_registers", "address": 0, "count": 2, "values": []any{11, 22}},
	})

	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	assert.True(t, outBool(t, out[0], "success"), "입력에 unit_id 없으면 unit_id 제외 매칭")
	entries := outEntries(t, out[0])
	require.Len(t, entries, 1)
	assert.Equal(t, []any{11, 22}, entries[0]["values"])
	assert.Equal(t, uint8(9), entries[0]["unit_id"], "To측 unit_id 는 할당됨")
}

// ---------------------------------------------------------------------------
// Change 2 (c) — 멀티 타깃 fan-out: 소스 1개 → 출력 N개
// ---------------------------------------------------------------------------

func TestModbusRemap_MultiTargetFanOut(t *testing.T) {
	n := newRemapNode(t, map[string]any{
		"rules": []any{
			map[string]any{
				"source_area": "holding_registers", "source_address": 100, "count": 3,
				"targets": []any{
					map[string]any{"target_unit_id": 1, "target_area": "input_registers", "target_address": 200},
					map[string]any{"target_unit_id": 2, "target_address": 300}, // target_area 생략 → source_area 유지
					map[string]any{"target_unit_id": 3, "target_area": "coils", "target_address": 0},
				},
			},
		},
	})

	in := makeReadMsg(true, "server", []any{
		map[string]any{"index": 0, "area": "holding_registers", "address": 100, "count": 3, "values": []any{7, 8, 9}},
	})

	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	assert.True(t, outBool(t, out[0], "success"))
	entries := outEntries(t, out[0])
	require.Len(t, entries, 3, "타깃 3개 → 출력 엔트리 3개(fan-out)")

	// 순차 index
	assert.Equal(t, 0, entries[0]["index"])
	assert.Equal(t, 1, entries[1]["index"])
	assert.Equal(t, 2, entries[2]["index"])

	// 타깃 1
	assert.Equal(t, uint8(1), entries[0]["unit_id"])
	assert.Equal(t, "input_registers", entries[0]["area"])
	assert.Equal(t, uint16(200), entries[0]["address"])
	// 타깃 2 (area 생략 → source_area)
	assert.Equal(t, uint8(2), entries[1]["unit_id"])
	assert.Equal(t, "holding_registers", entries[1]["area"])
	assert.Equal(t, uint16(300), entries[1]["address"])
	// 타깃 3
	assert.Equal(t, uint8(3), entries[2]["unit_id"])
	assert.Equal(t, "coils", entries[2]["area"])
	assert.Equal(t, uint16(0), entries[2]["address"])

	// 모든 출력이 동일 값 보존
	for _, e := range entries {
		assert.Equal(t, []any{7, 8, 9}, e["values"], "값 보존")
		assert.Equal(t, uint16(3), e["count"])
	}
}

// ---------------------------------------------------------------------------
// Change 2 (d) — 레거시 단일 타깃 규칙 하위 호환 (targets 없이 최상위 target_*)
// ---------------------------------------------------------------------------

func TestModbusRemap_LegacySingleTargetBackwardCompat(t *testing.T) {
	n := newRemapNode(t, map[string]any{
		"rules": []any{
			// targets 배열 없음 — 기존 shape
			map[string]any{
				"source_area": "holding_registers", "source_address": 100, "count": 5,
				"target_unit_id": 3, "target_area": "input_registers", "target_address": 200,
			},
		},
	})

	in := makeReadMsg(true, "server", []any{
		map[string]any{"index": 0, "area": "holding_registers", "address": 100, "count": 5, "values": []any{10, 20, 30, 40, 50}},
	})

	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	assert.True(t, outBool(t, out[0], "success"))
	entries := outEntries(t, out[0])
	require.Len(t, entries, 1, "레거시 단일 타깃 → 출력 1개")
	assert.Equal(t, "input_registers", entries[0]["area"])
	assert.Equal(t, uint16(200), entries[0]["address"])
	assert.Equal(t, uint8(3), entries[0]["unit_id"])
	assert.Equal(t, []any{10, 20, 30, 40, 50}, entries[0]["values"])
}

// ---------------------------------------------------------------------------
// Change 1+2 (e) — 템플릿에 source_unit_id 적용 파라미터
// ---------------------------------------------------------------------------

func TestModbusRemap_TemplateWithSourceUnitID(t *testing.T) {
	n := newRemapNode(t, map[string]any{
		"templates": []any{
			map[string]any{
				"source_unit_id": 8, "area": "holding_registers", "offset": 1000,
				"device_id": 2, "start": 100, "count": 3,
			},
		},
	})

	// 엔트리에 unit_id=8 포함 → source_unit_id 매칭 확인
	in := makeReadMsg(true, "server", []any{
		map[string]any{"index": 0, "area": "holding_registers", "address": 100, "count": 3, "unit_id": 8, "values": []any{1, 2, 3}},
	})

	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	assert.True(t, outBool(t, out[0], "success"), "템플릿 source_unit_id=8 + 엔트리 unit_id=8 매칭")
	entries := outEntries(t, out[0])
	require.Len(t, entries, 1, "템플릿은 항상 SINGLE-target")
	assert.Equal(t, uint16(1100), entries[0]["address"], "start+offset")
	assert.Equal(t, uint8(2), entries[0]["unit_id"], "device_id")
	assert.Equal(t, "holding_registers", entries[0]["area"])

	// 엔트리 unit_id 가 다르면(9) 미매칭
	in2 := makeReadMsg(true, "server", []any{
		map[string]any{"index": 0, "area": "holding_registers", "address": 100, "count": 3, "unit_id": 9, "values": []any{1, 2, 3}},
	})
	out2, err := n.Process(context.Background(), in2)
	require.NoError(t, err)
	assert.False(t, outBool(t, out2[0], "success"), "템플릿 source_unit_id 불일치 → 거부")
}
