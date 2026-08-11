// Package node - split_test.go: split 노드 단위 테스트 (TDD)
//
// 본 파일은 SPEC-MESSAGE-SPLIT-001 의 인수 기준 AC-1 ~ AC-12 를 테이블/개별
// 케이스로 검증한다. 팬아웃 개수·순서·payload·metadata·id 유일성을 명시적으로
// assert 한다.
package node

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// 테스트 헬퍼
// ---------------------------------------------------------------------------

// mustSplitNode 는 config 로 split 노드를 생성하고 Init 한다. 실패 시 테스트를 종료한다.
func mustSplitNode(t *testing.T, cfg map[string]any) Node {
	t.Helper()
	def := flow.NodeDef{ID: "split-1", Name: "split", Type: "split", Config: cfg}
	n, err := NewSplitNode(def)
	require.NoError(t, err)
	require.NoError(t, n.Init(context.Background()))
	return n
}

// newSplitInput 는 부모 입력 메시지를 구성한다.
func newSplitInput(payload map[string]any, msgType string, ts time.Time, meta map[string]string) message.Message {
	m := message.New(
		message.WithType(msgType),
		message.WithTimestamp(ts),
		message.WithPayload(message.NewPayload(payload)),
	)
	for k, v := range meta {
		m.Metadata().Set(k, v)
	}
	return m
}

// payloadGet 는 결과 메시지 payload 의 최상위 키 값을 조회한다.
func payloadGet(t *testing.T, m message.Message, key string) any {
	t.Helper()
	v, ok := m.Payload().Get(key)
	require.True(t, ok, "payload 에 키 %q 가 있어야 한다", key)
	return v
}

// ---------------------------------------------------------------------------
// AC-1 — 케이스 A (payloads 모드, 요소=payload)
// ---------------------------------------------------------------------------

func TestSplit_AC1_PayloadsMode(t *testing.T) {
	ts := time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC)
	input := newSplitInput(map[string]any{
		"items": []any{
			map[string]any{"a": 1},
			map[string]any{"b": 2},
			map[string]any{"c": 3},
		},
	}, "T", ts, map[string]string{"pk": "pv"})

	n := mustSplitNode(t, map[string]any{"path": "items", "mode": "payloads"})
	res, err := n.Process(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, res, 3)

	// 순서 보존 + payload 내용
	assert.Equal(t, 1, payloadGet(t, res[0], "a"))
	assert.Equal(t, 2, payloadGet(t, res[1], "b"))
	assert.Equal(t, 3, payloadGet(t, res[2], "c"))

	ids := map[string]bool{}
	for _, m := range res {
		assert.Equal(t, "T", m.Type(), "부모 type 보존")
		assert.True(t, ts.Equal(m.Timestamp()), "부모 timestamp 보존")
		v, ok := m.Metadata().Get("pk")
		assert.True(t, ok)
		assert.Equal(t, "pv", v, "부모 메타데이터 공유")
		assert.NotEqual(t, input.ID(), m.ID(), "부모와 다른 새 UUID")
		assert.False(t, ids[m.ID()], "메시지 간 UUID 유일")
		ids[m.ID()] = true
	}
}

// ---------------------------------------------------------------------------
// AC-2 — 케이스 B (messages 모드, 요소 우선 메타 병합)
// ---------------------------------------------------------------------------

func TestSplit_AC2_MessagesMode_MetaMergeElementWins(t *testing.T) {
	input := newSplitInput(map[string]any{
		"messages": []any{
			map[string]any{"metadata": map[string]any{"x": "1"}, "payload": map[string]any{"p": 10}},
			map[string]any{"metadata": map[string]any{"y": "2"}, "payload": map[string]any{"p": 20}},
		},
	}, "T", time.Now(), map[string]string{"x": "parent", "z": "keep"})

	n := mustSplitNode(t, map[string]any{"path": "messages", "mode": "messages", "share_metadata": true})
	res, err := n.Process(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, res, 2)

	// 1번: x=1 (요소 우선), z=keep (부모 유지), y 없음
	x0, _ := res[0].Metadata().Get("x")
	z0, _ := res[0].Metadata().Get("z")
	_, hasY0 := res[0].Metadata().Get("y")
	assert.Equal(t, "1", x0)
	assert.Equal(t, "keep", z0)
	assert.False(t, hasY0)
	assert.Equal(t, 10, payloadGet(t, res[0], "p"))

	// 2번: y=2, x=parent, z=keep
	x1, _ := res[1].Metadata().Get("x")
	y1, _ := res[1].Metadata().Get("y")
	z1, _ := res[1].Metadata().Get("z")
	assert.Equal(t, "parent", x1)
	assert.Equal(t, "2", y1)
	assert.Equal(t, "keep", z1)
	assert.Equal(t, 20, payloadGet(t, res[1], "p"))
}

// ---------------------------------------------------------------------------
// AC-3 — auto 모드 자동 감지 (혼합)
// ---------------------------------------------------------------------------

func TestSplit_AC3_AutoDetect(t *testing.T) {
	input := newSplitInput(map[string]any{
		"arr": []any{
			map[string]any{"metadata": map[string]any{"m": "1"}, "payload": map[string]any{"v": 1}},
			map[string]any{"plain": true},
			42,
		},
	}, "T", time.Now(), nil)

	n := mustSplitNode(t, map[string]any{"path": "arr", "mode": "auto"})
	res, err := n.Process(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, res, 3)

	// (1) metadata/payload 키 보유 → 케이스 B
	assert.Equal(t, 1, payloadGet(t, res[0], "v"))
	m0, _ := res[0].Metadata().Get("m")
	assert.Equal(t, "1", m0)

	// (2) message 키 미보유 → 케이스 A
	assert.Equal(t, true, payloadGet(t, res[1], "plain"))

	// (3) 스칼라 → 케이스 A + 기본 scalar_key 래핑
	assert.Equal(t, 42, payloadGet(t, res[2], "value"))
}

// ---------------------------------------------------------------------------
// AC-4 — 엣지: path 누락 → passthrough
// ---------------------------------------------------------------------------

func TestSplit_AC4_MissingPath_Passthrough(t *testing.T) {
	input := newSplitInput(map[string]any{"items": []any{1, 2}}, "T", time.Now(), nil)
	n := mustSplitNode(t, map[string]any{"path": "nonexistent"})
	res, err := n.Process(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, input.ID(), res[0].ID(), "입력 메시지 그대로 반환")
}

// ---------------------------------------------------------------------------
// AC-5 — 엣지: 값이 배열 아님 → passthrough
// ---------------------------------------------------------------------------

func TestSplit_AC5_NotArray_Passthrough(t *testing.T) {
	input := newSplitInput(map[string]any{"items": "not-an-array"}, "T", time.Now(), nil)
	n := mustSplitNode(t, map[string]any{"path": "items"})
	res, err := n.Process(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, input.ID(), res[0].ID())
}

// ---------------------------------------------------------------------------
// AC-6 — 엣지: 빈 배열 → 0 메시지
// ---------------------------------------------------------------------------

func TestSplit_AC6_EmptyArray_EmitNone(t *testing.T) {
	input := newSplitInput(map[string]any{"items": []any{}}, "T", time.Now(), nil)
	n := mustSplitNode(t, map[string]any{"path": "items"})
	res, err := n.Process(context.Background(), input)
	require.NoError(t, err)
	assert.Len(t, res, 0)
}

// ---------------------------------------------------------------------------
// AC-7 — 비객체 요소 (payloads 모드 기본 동작)
// ---------------------------------------------------------------------------

func TestSplit_AC7_ScalarWrap(t *testing.T) {
	input := newSplitInput(map[string]any{"nums": []any{1, 2, 3}}, "T", time.Now(), nil)
	n := mustSplitNode(t, map[string]any{"path": "nums", "mode": "payloads"})
	res, err := n.Process(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, res, 3)
	assert.Equal(t, 1, payloadGet(t, res[0], "value"))
	assert.Equal(t, 2, payloadGet(t, res[1], "value"))
	assert.Equal(t, 3, payloadGet(t, res[2], "value"))
}

// ---------------------------------------------------------------------------
// AC-8 — 등록/팩토리 구성 가능
// ---------------------------------------------------------------------------

func TestSplit_AC8_Registration(t *testing.T) {
	r := NewRegistry()
	assert.True(t, r.Has("split"), "split 이 등록되어 있어야 한다")

	meta, ok := r.TypeMeta("split")
	require.True(t, ok)
	assert.Equal(t, "processing", meta.Category)

	def := flow.NodeDef{ID: "n1", Name: "split", Type: "split", Config: map[string]any{"path": "items"}}
	created, err := r.Create(def)
	require.NoError(t, err)
	require.NotNil(t, created)
	_, isSplit := created.(*SplitNode)
	assert.True(t, isSplit, "Create 가 *SplitNode 를 반환해야 한다")
}

// ---------------------------------------------------------------------------
// AC-9 — JSONPath 경로 지정 (message-rooted)
//
// 변경(0.2.0): "$." 경로는 메시지 루트로 해석되므로 payload 의 items 배열은
// "$.payload.items[*]"/"$.payload.items" 로 지정한다(구 payload-rooted
// "$.items[*]" 는 버그였다 — TestSplit_AC13_* 음성 검증 참조).
// ---------------------------------------------------------------------------

func TestSplit_AC9_JSONPath(t *testing.T) {
	payload := map[string]any{"items": []any{
		map[string]any{"a": 1},
		map[string]any{"b": 2},
	}}

	for _, path := range []string{"$.payload.items[*]", "$.payload.items"} {
		input := newSplitInput(payload, "T", time.Now(), nil)
		n := mustSplitNode(t, map[string]any{"path": path, "mode": "payloads"})
		res, err := n.Process(context.Background(), input)
		require.NoError(t, err, "path=%s", path)
		require.Len(t, res, 2, "path=%s", path)
		assert.Equal(t, 1, payloadGet(t, res[0], "a"), "path=%s", path)
		assert.Equal(t, 2, payloadGet(t, res[1], "b"), "path=%s", path)
	}
}

// ---------------------------------------------------------------------------
// AC-13 — path 는 메시지 루트 (버그 회귀 방지)
//
// "$." 경로는 메시지 전체를 루트로 해석한다(payload 아님):
//   (a) "$.payload.items[*]" = msg.payload.items → 팬아웃
//   (b) "$.items[*]" (payload-상대) 는 msg.items 를 찾으므로 미해석 → passthrough
//       (payload-rooted 가 아님을 증명)
//   (c) "$.metadata.<key>" = msg.metadata.<key> (메시지 루트 일관성)
// ---------------------------------------------------------------------------

// AC-13(a): "$.payload.items[*]" 가 payload 의 items 배열을 해석하여 팬아웃한다.
func TestSplit_AC13a_MessageRooted_PayloadItems(t *testing.T) {
	input := newSplitInput(map[string]any{"items": []any{
		map[string]any{"a": 1},
		map[string]any{"b": 2},
	}}, "T", time.Now(), nil)

	n := mustSplitNode(t, map[string]any{"path": "$.payload.items[*]", "mode": "payloads"})
	res, err := n.Process(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, res, 2, "$.payload.items = msg.payload.items → 2개 팬아웃")
	assert.Equal(t, 1, payloadGet(t, res[0], "a"))
	assert.Equal(t, 2, payloadGet(t, res[1], "b"))
}

// AC-13(b) 음성 검증: payload-상대 "$.items[*]" 는 메시지 루트에서 msg.items 를
// 찾으므로 배열을 해석하지 못하고 passthrough(입력 1개) 한다. payload-rooted 였다면
// payload.items 를 찾아 2개로 팬아웃했을 것이다 — 미해석이 message-rooted 를 증명한다.
func TestSplit_AC13b_MessageRooted_PayloadRelativePathFails(t *testing.T) {
	input := newSplitInput(map[string]any{"items": []any{
		map[string]any{"a": 1},
		map[string]any{"b": 2},
	}}, "T", time.Now(), nil)

	n := mustSplitNode(t, map[string]any{"path": "$.items[*]", "mode": "payloads"})
	res, err := n.Process(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, res, 1, "$.items 는 msg.items(존재하지 않음) → passthrough")
	assert.Equal(t, input.ID(), res[0].ID(), "입력 메시지 그대로 반환")
}

// AC-13(c): "$.metadata.<key>" 는 msg.metadata.<key> 로 해석된다(메시지 루트 일관성).
// 메타데이터 값 계약은 string 뿐이라 배열을 담을 수 없으므로, payload 와 metadata 에
// 같은 키를 두고(payload=배열, metadata=문자열) 판별한다:
//   - "$.payload.shared" → payload 의 배열 해석 → 2개 팬아웃
//   - "$.metadata.shared" → metadata 의 문자열 해석(배열 아님) → passthrough
//
// 두 결과가 다르므로 "$.metadata.X" 가 payload 가 아닌 metadata 서브트리에 도달함을
// 증명한다.
func TestSplit_AC13c_MessageRooted_ReachesMetadata(t *testing.T) {
	makeInput := func() message.Message {
		m := newSplitInput(map[string]any{"shared": []any{
			map[string]any{"a": 1},
			map[string]any{"b": 2},
		}}, "T", time.Now(), nil)
		m.Metadata().Set("shared", "meta-val")
		return m
	}

	// payload 서브트리: 배열 → 2개 팬아웃
	inPayload := makeInput()
	nPayload := mustSplitNode(t, map[string]any{"path": "$.payload.shared[*]", "mode": "payloads"})
	resPayload, err := nPayload.Process(context.Background(), inPayload)
	require.NoError(t, err)
	require.Len(t, resPayload, 2, "$.payload.shared → payload 배열 해석")

	// metadata 서브트리: 문자열 "meta-val"(배열 아님) → passthrough. 이것이
	// "$.metadata.shared" 가 metadata 에 도달함(payload 배열이 아님)을 증명한다.
	inMeta := makeInput()
	nMeta := mustSplitNode(t, map[string]any{"path": "$.metadata.shared", "mode": "payloads"})
	resMeta, err := nMeta.Process(context.Background(), inMeta)
	require.NoError(t, err)
	require.Len(t, resMeta, 1, "$.metadata.shared = msg.metadata.shared(문자열) → passthrough")
	assert.Equal(t, inMeta.ID(), resMeta[0].ID(), "입력 메시지 그대로 반환")
}

// ---------------------------------------------------------------------------
// AC-10 — 타임스탬프/타입 보존 및 요소 override
// ---------------------------------------------------------------------------

func TestSplit_AC10_TypeTimestampPreserveAndOverride(t *testing.T) {
	ts := time.Date(2026, 5, 26, 11, 19, 28, 0, time.UTC)
	deviceMs := int64(1779794368319)
	input := newSplitInput(map[string]any{
		"messages": []any{
			map[string]any{"payload": map[string]any{"n": 1, "ts_ms": deviceMs}},
			map[string]any{"type": "U", "payload": map[string]any{"n": 2}},
		},
	}, "T", ts, nil)

	n := mustSplitNode(t, map[string]any{"path": "messages", "mode": "messages"})
	res, err := n.Process(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, res, 2)

	// 요소 1: 부모 type/timestamp 보존
	assert.Equal(t, "T", res[0].Type())
	assert.True(t, ts.Equal(res[0].Timestamp()))
	// payload 내 epoch-ms 값은 재포맷 없이 그대로 (int64 보존)
	assert.Equal(t, deviceMs, payloadGet(t, res[0], "ts_ms"))

	// 요소 2: type override
	assert.Equal(t, "U", res[1].Type())
}

// ---------------------------------------------------------------------------
// AC-11 — correlation id (SHOULD)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// AC-12 — share_metadata=false
// ---------------------------------------------------------------------------

func TestSplit_AC12_ShareMetadataFalse(t *testing.T) {
	input := newSplitInput(map[string]any{"items": []any{
		map[string]any{"a": 1},
	}}, "T", time.Now(), map[string]string{"x": "1"})

	n := mustSplitNode(t, map[string]any{"path": "items", "mode": "payloads", "share_metadata": false})
	res, err := n.Process(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, res, 1)

	_, hasX := res[0].Metadata().Get("x")
	assert.False(t, hasX, "부모 메타를 복사하지 않아야 한다")
}

// ---------------------------------------------------------------------------
// 팩토리 config 검증 (REQ-03, REQ-05)
// ---------------------------------------------------------------------------

func TestSplit_ConfigErrors(t *testing.T) {
	cases := []struct {
		name string
		cfg  map[string]any
	}{
		{"nil config", nil},
		{"missing path", map[string]any{"mode": "auto"}},
		{"empty path", map[string]any{"path": ""}},
		{"invalid mode", map[string]any{"path": "items", "mode": "bogus"}},
		{"invalid on_missing", map[string]any{"path": "items", "on_missing": "bogus"}},
		{"invalid on_empty", map[string]any{"path": "items", "on_empty": "bogus"}},
		{"non-string path", map[string]any{"path": 42}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def := flow.NodeDef{ID: "n1", Name: "split", Type: "split", Config: tc.cfg}
			_, err := NewSplitNode(def)
			assert.Error(t, err)
		})
	}
}

// ---------------------------------------------------------------------------
// on_missing=error / on_empty=passthrough 분기
// ---------------------------------------------------------------------------

func TestSplit_OnMissingError(t *testing.T) {
	input := newSplitInput(map[string]any{"items": "not-an-array"}, "T", time.Now(), nil)
	n := mustSplitNode(t, map[string]any{"path": "items", "on_missing": "error"})
	res, err := n.Process(context.Background(), input)
	assert.Error(t, err)
	assert.Nil(t, res)
}

func TestSplit_OnEmptyPassthrough(t *testing.T) {
	input := newSplitInput(map[string]any{"items": []any{}}, "T", time.Now(), nil)
	n := mustSplitNode(t, map[string]any{"path": "items", "on_empty": "passthrough"})
	res, err := n.Process(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, input.ID(), res[0].ID())
}

// ---------------------------------------------------------------------------
// 통합 — output.json (payload.items 9-요소 디바이스 배열) 실데이터
// ---------------------------------------------------------------------------

func TestSplit_Integration_OutputJSON(t *testing.T) {
	raw, err := os.ReadFile("../../output.json")
	if err != nil {
		t.Skipf("output.json 없음, 통합 테스트 건너뜀: %v", err)
	}
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	payloadRaw, ok := doc["payload"].(map[string]any)
	require.True(t, ok, "payload 객체 필요")
	items, ok := payloadRaw["items"].([]any)
	require.True(t, ok, "payload.items 배열 필요")
	require.NotEmpty(t, items)

	input := newSplitInput(map[string]any{"items": items}, "inventory.event", time.Now(), nil)
	n := mustSplitNode(t, map[string]any{"path": "items", "mode": "payloads"})
	res, err := n.Process(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, res, len(items), "디바이스 하나당 메시지 1개")

	for _, m := range res {
		assert.Equal(t, "inventory.event", m.Type())
		_, hasID := m.Payload().Get("id")
		assert.True(t, hasID, "각 디바이스 payload 에 id 키 존재")
	}
}

// ---------------------------------------------------------------------------
// 보강 — Shutdown lifecycle
// ---------------------------------------------------------------------------

func TestSplit_Shutdown(t *testing.T) {
	n := mustSplitNode(t, map[string]any{"path": "items"})
	require.NoError(t, n.Shutdown(context.Background()))
	// 이미 stopped 상태에서 재호출은 no-op
	require.NoError(t, n.Shutdown(context.Background()))
}

// ---------------------------------------------------------------------------
// 보강 — messages 모드 timestamp override (epoch ms / RFC3339)
// ---------------------------------------------------------------------------

func TestSplit_MessagesTimestampOverride(t *testing.T) {
	parentTS := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	epochMs := int64(1779794368319)
	input := newSplitInput(map[string]any{
		"messages": []any{
			map[string]any{"payload": map[string]any{"n": 1}, "timestamp": epochMs},
			map[string]any{"payload": map[string]any{"n": 2}, "timestamp": "2026-01-02T03:04:05Z"},
		},
	}, "T", parentTS, nil)

	n := mustSplitNode(t, map[string]any{"path": "messages", "mode": "messages"})
	res, err := n.Process(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, res, 2)

	assert.Equal(t, epochMs, res[0].Timestamp().UnixMilli(), "epoch ms override")
	rfc, _ := time.Parse(time.RFC3339, "2026-01-02T03:04:05Z")
	assert.True(t, rfc.Equal(res[1].Timestamp()), "RFC3339 override")
}

// ---------------------------------------------------------------------------
// 보강 — messages 모드 비문자열 요소 메타 값 문자열화 (REQ-12)
// ---------------------------------------------------------------------------

func TestSplit_MessagesNonStringMetaStringified(t *testing.T) {
	input := newSplitInput(map[string]any{
		"messages": []any{
			map[string]any{"metadata": map[string]any{"num": 42, "flag": true}, "payload": map[string]any{"n": 1}},
		},
	}, "T", time.Now(), nil)

	n := mustSplitNode(t, map[string]any{"path": "messages", "mode": "messages"})
	res, err := n.Process(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, res, 1)

	num, _ := res[0].Metadata().Get("num")
	flag, _ := res[0].Metadata().Get("flag")
	assert.Equal(t, "42", num)
	assert.Equal(t, "true", flag)
}

// ---------------------------------------------------------------------------
// 보강 — payloads 모드에서 부모 group 메타데이터 공유 (REQ-11)
// ---------------------------------------------------------------------------

func TestSplit_SharedGroupMetadata(t *testing.T) {
	input := newSplitInput(map[string]any{"items": []any{map[string]any{"a": 1}}}, "T", time.Now(), nil)
	input.Metadata().SetGroup("agent", map[string]string{"name": "a1", "type": "x"})

	n := mustSplitNode(t, map[string]any{"path": "items", "mode": "payloads", "share_metadata": true})
	res, err := n.Process(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, res, 1)

	grp, ok := res[0].Metadata().GetGroup("agent")
	require.True(t, ok, "group 메타 공유")
	assert.Equal(t, "a1", grp["name"])
	assert.Equal(t, "x", grp["type"])
}

// ---------------------------------------------------------------------------
// 보강 — 커스텀 scalar_key
// ---------------------------------------------------------------------------

func TestSplit_CustomScalarKey(t *testing.T) {
	input := newSplitInput(map[string]any{"nums": []any{7, 8}}, "T", time.Now(), nil)
	n := mustSplitNode(t, map[string]any{"path": "nums", "mode": "payloads", "scalar_key": "val"})
	res, err := n.Process(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, res, 2)
	assert.Equal(t, 7, payloadGet(t, res[0], "val"))
	assert.Equal(t, 8, payloadGet(t, res[1], "val"))
}

// ---------------------------------------------------------------------------
// 보강 — 비 []any 슬라이스 타입(reflect 경로) 팬아웃 (REQ-06)
// ---------------------------------------------------------------------------

func TestSplit_ReflectSliceType(t *testing.T) {
	input := newSplitInput(map[string]any{
		"items": []map[string]any{{"a": 1}, {"b": 2}},
	}, "T", time.Now(), nil)
	n := mustSplitNode(t, map[string]any{"path": "items", "mode": "payloads"})
	res, err := n.Process(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, res, 2)
	assert.Equal(t, 1, payloadGet(t, res[0], "a"))
	assert.Equal(t, 2, payloadGet(t, res[1], "b"))
}
