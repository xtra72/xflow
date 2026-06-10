package node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// --- 인터페이스 준수 ---

var _ Node = (*SelectFieldNode)(nil)

// newSelectFieldForTest 는 테스트용 SelectFieldNode 를 생성하고 설정을 적용한다.
func newSelectFieldForTest(t *testing.T, config map[string]any) *SelectFieldNode {
	t.Helper()
	def := flow.NewNodeDef("select-field-test", "select-field")
	node, err := NewSelectFieldNode(def)
	require.NoError(t, err)
	sf := node.(*SelectFieldNode)
	require.NoError(t, sf.Configure(config))
	require.NoError(t, sf.Init(context.Background()))
	return sf
}

// newTestMessage 는 지정한 payload/metadata/type 을 가진 메시지를 생성한다.
func newTestMessage(payload map[string]any, metadata map[string]string, msgType string) message.Message {
	opts := []message.Option{
		message.WithPayload(message.NewPayload(payload)),
		message.WithType(msgType),
	}
	for k, v := range metadata {
		opts = append(opts, message.WithMetadata(k, v))
	}
	return message.New(opts...)
}

// --- 생성/초기화 테스트 ---

// TestNewSelectFieldNode_정상생성 은 SelectFieldNode 가 올바르게 생성되는지 확인한다.
func TestNewSelectFieldNode_정상생성(t *testing.T) {
	t.Parallel()
	def := flow.NewNodeDef("sf-1", "select-field")
	node, err := NewSelectFieldNode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "sf-1", node.Name())
	assert.Equal(t, "select-field", node.Type())
}

// TestSelectFieldNode_Init_상태전이 는 Init 호출 시 Running 상태로 전이하는지 확인한다.
func TestSelectFieldNode_Init_상태전이(t *testing.T) {
	t.Parallel()
	def := flow.NewNodeDef("sf-init", "select-field")
	node, _ := NewSelectFieldNode(def)
	sf := node.(*SelectFieldNode)
	require.NoError(t, sf.Init(context.Background()))
	assert.Equal(t, lifecycle.StateRunning, sf.CurrentState())
}

// TestSelectFieldNode_Shutdown_상태전이 는 Shutdown 호출 시 Stopping 상태로 전이하는지 확인한다.
func TestSelectFieldNode_Shutdown_상태전이(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{})
	require.NoError(t, sf.Shutdown(context.Background()))
	assert.Equal(t, lifecycle.StateStopping, sf.CurrentState())
}

// --- payload 화이트리스트 테스트 ($.payload.*) ---

// TestSelectFieldNode_Payload_TopLevelWhitelist 는 $.payload.<key> 경로가
// 명시된 최상위 키만 유지하고 나머지는 제거하는지 확인한다.
func TestSelectFieldNode_Payload_TopLevelWhitelist(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"fields": map[string]any{
			"$.payload.keep_a": "",
			"$.payload.keep_b": "",
		},
	})

	msg := newTestMessage(map[string]any{
		"keep_a": "A",
		"keep_b": "B",
		"drop_c": "C",
	}, nil, "")

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)

	p := out[0].Payload().ToMap()
	assert.Equal(t, "A", p["keep_a"])
	assert.Equal(t, "B", p["keep_b"])
	_, hasC := p["drop_c"]
	assert.False(t, hasC, "화이트리스트에 없는 키는 제거되어야 한다")
}

// TestSelectFieldNode_Payload_NestedFieldSelect 는 $.payload.state.mode 가
// state 하위의 mode 필드만 남기고 형제(temp)는 제거하는지 확인한다.
func TestSelectFieldNode_Payload_NestedFieldSelect(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"fields": map[string]any{
			"$.payload.state.mode": "",
		},
	})

	msg := newTestMessage(map[string]any{
		"state": map[string]any{"mode": 3, "temp": 22.5},
		"other": "drop",
	}, nil, "")

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)

	p := out[0].Payload().ToMap()
	state, ok := p["state"].(map[string]any)
	require.True(t, ok, "state 컨테이너는 보존되어야 한다")
	assert.Equal(t, 3, state["mode"], "mode 필드 보존")
	_, hasTemp := state["temp"]
	assert.False(t, hasTemp, "선택되지 않은 형제 temp 제거")
	_, hasOther := p["other"]
	assert.False(t, hasOther, "최상위 미선택 키 other 제거")
}

// TestSelectFieldNode_Payload_WholeSubtree 는 $.payload.state (컨테이너 경로) 가
// state 서브트리 전체를 보존하는지 확인한다.
func TestSelectFieldNode_Payload_WholeSubtree(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"fields": map[string]any{
			"$.payload.state": "",
		},
	})

	msg := newTestMessage(map[string]any{
		"state": map[string]any{"mode": 3, "temp": 22.5, "fan": "auto"},
		"other": "drop",
	}, nil, "")

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)

	p := out[0].Payload().ToMap()
	state, ok := p["state"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, map[string]any{"mode": 3, "temp": 22.5, "fan": "auto"}, state,
		"컨테이너 경로는 서브트리 전체를 보존해야 한다")
	_, hasOther := p["other"]
	assert.False(t, hasOther)
}

// TestSelectFieldNode_Payload_WholeWinsOverNested 는 $.payload.state 와
// $.payload.state.mode 가 함께 있으면 전체 보존(whole)이 우선하는지 확인한다.
func TestSelectFieldNode_Payload_WholeWinsOverNested(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"fields": map[string]any{
			"$.payload.state":      "",
			"$.payload.state.mode": "",
		},
	})

	msg := newTestMessage(map[string]any{
		"state": map[string]any{"mode": 3, "temp": 22.5},
	}, nil, "")

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)

	state := out[0].Payload().ToMap()["state"].(map[string]any)
	assert.Equal(t, map[string]any{"mode": 3, "temp": 22.5}, state,
		"전체 경로가 중첩 필드 경로를 이겨 서브트리 전체가 보존되어야 한다")
}

// TestSelectFieldNode_Payload_DeepNested 는 $.payload.a.b.c 가
// 깊은 중첩 경로를 따라 중간 컨테이너를 보존하며 leaf 만 남기는지 확인한다.
func TestSelectFieldNode_Payload_DeepNested(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"fields": map[string]any{
			"$.payload.a.b.c": "",
		},
	})

	msg := newTestMessage(map[string]any{
		"a": map[string]any{
			"b":     map[string]any{"c": "deep", "d": "drop"},
			"sib_b": "drop",
		},
		"sib_a": "drop",
	}, nil, "")

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)

	p := out[0].Payload().ToMap()
	a := p["a"].(map[string]any)
	b := a["b"].(map[string]any)
	assert.Equal(t, "deep", b["c"])
	_, hasD := b["d"]
	assert.False(t, hasD, "선택되지 않은 leaf 형제 d 제거")
	_, hasSibB := a["sib_b"]
	assert.False(t, hasSibB)
	_, hasSibA := p["sib_a"]
	assert.False(t, hasSibA)
}

// TestSelectFieldNode_Payload_OnMissing 은 payload 경로 누락 시 on_missing 정책별 동작을 검증한다.
func TestSelectFieldNode_Payload_OnMissing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		onMissing   string
		wantDropped bool
		wantPayload map[string]any
		wantMissing []string
	}{
		{
			name:        "keep_누락키건너뜀",
			onMissing:   "keep",
			wantDropped: false,
			wantPayload: map[string]any{"present": "P"},
			wantMissing: []string{"absent"},
		},
		{
			name:        "ignore_alias_누락키건너뜀",
			onMissing:   "ignore", // keep 의 별칭 (하위 호환)
			wantDropped: false,
			wantPayload: map[string]any{"present": "P"},
			wantMissing: []string{"absent"},
		},
		{
			name:        "fill_누락키채움",
			onMissing:   "fill",
			wantDropped: false,
			wantPayload: map[string]any{"present": "P", "absent": "DEFAULT"},
		},
		{
			name:        "drop_누락키있으면메시지드랍",
			onMissing:   "drop",
			wantDropped: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sf := newSelectFieldForTest(t, map[string]any{
				"on_missing": tc.onMissing,
				"fields": map[string]any{
					"$.payload.present": "",
					"$.payload.absent":  "DEFAULT",
				},
			})

			msg := newTestMessage(map[string]any{
				"present": "P",
				"extra":   "E", // 화이트리스트에 없음 → 제거
			}, nil, "")

			out, err := sf.Process(context.Background(), msg)
			require.NoError(t, err)

			if tc.wantDropped {
				assert.Empty(t, out, "drop 정책: 출력 메시지가 없어야 한다")
				return
			}

			require.Len(t, out, 1)
			p := out[0].Payload().ToMap()
			for k, v := range tc.wantPayload {
				assert.Equal(t, v, p[k], "키 %q 값", k)
			}
			for _, k := range tc.wantMissing {
				_, ok := p[k]
				assert.False(t, ok, "키 %q 는 없어야 한다", k)
			}
			_, hasExtra := p["extra"]
			assert.False(t, hasExtra, "화이트리스트에 없는 extra 키는 제거되어야 한다")
		})
	}
}

// TestSelectFieldNode_Payload_OnMissingFill_Nested 는 fill 정책이 누락된
// 중첩 leaf 에 대해 중간 컨테이너를 생성하며 채우는지 확인한다.
func TestSelectFieldNode_Payload_OnMissingFill_Nested(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing": "fill",
		"fields": map[string]any{
			"$.payload.state.mode": "DEF_MODE",
		},
	})

	// state 그룹 자체가 없음 → fill 시 컨테이너 생성
	msg := newTestMessage(map[string]any{"other": 1}, nil, "")

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)

	p := out[0].Payload().ToMap()
	state, ok := p["state"].(map[string]any)
	require.True(t, ok, "fill: 중간 컨테이너가 생성되어야 한다")
	assert.Equal(t, "DEF_MODE", state["mode"])
	_, hasOther := p["other"]
	assert.False(t, hasOther, "화이트리스트 외 키 제거")
}

// TestSelectFieldNode_Payload_EmptiedWhenNotListed 는 fields 가 비어있지 않지만
// $.payload.* 경로가 하나도 없으면 payload 가 전부 비워지는지(단일 화이트리스트) 확인한다.
func TestSelectFieldNode_Payload_EmptiedWhenNotListed(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"fields": map[string]any{
			"$.metadata.node_id": "",
		},
	})

	msg := newTestMessage(
		map[string]any{"a": 1, "b": 2},
		map[string]string{"node_id": "n-1"},
		"",
	)

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)

	p := out[0].Payload().ToMap()
	assert.Empty(t, p, "payload 경로가 없으면 payload 전체가 비워져야 한다")
	// metadata 는 화이트리스트대로 유지
	v, ok := out[0].Metadata().Get("node_id")
	assert.True(t, ok)
	assert.Equal(t, "n-1", v)
}

// --- metadata 화이트리스트 테스트 ($.metadata.*) ---

// TestSelectFieldNode_Metadata_StringWhitelist 는 $.metadata.<key> 가
// 명시된 top-level string 키만 유지하는지 확인한다.
func TestSelectFieldNode_Metadata_StringWhitelist(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"fields": map[string]any{
			"$.metadata.keep_meta": "",
			"$.payload.x":          "", // payload 도 유지 (없으면 payload 비워짐)
		},
	})

	msg := newTestMessage(
		map[string]any{"x": 1},
		map[string]string{"keep_meta": "KEEP", "drop_meta": "DROP"},
		"",
	)

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)

	md := out[0].Metadata()
	v, ok := md.Get("keep_meta")
	assert.True(t, ok)
	assert.Equal(t, "KEEP", v)
	assert.False(t, md.Has("drop_meta"), "화이트리스트에 없는 metadata 키는 제거되어야 한다")
}

// TestSelectFieldNode_Metadata_OnMissingFill 은 $.metadata 화이트리스트에서 fill 정책이
// 누락 키를 기본값으로 채우는지 확인한다.
func TestSelectFieldNode_Metadata_OnMissingFill(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing": "fill",
		"fields": map[string]any{
			"$.metadata.absent_meta": "META_DEFAULT",
		},
	})

	msg := newTestMessage(map[string]any{"x": 1}, nil, "")

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)

	v, ok := out[0].Metadata().Get("absent_meta")
	assert.True(t, ok)
	assert.Equal(t, "META_DEFAULT", v)
}

// TestSelectFieldNode_Metadata_OnMissingDrop 은 $.metadata 누락 시 drop 정책이
// 메시지를 드랍하는지 확인한다.
func TestSelectFieldNode_Metadata_OnMissingDrop(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing": "drop",
		"fields": map[string]any{
			"$.metadata.absent_meta": "",
		},
	})

	msg := newTestMessage(map[string]any{"x": 1}, map[string]string{"other": "v"}, "")

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Empty(t, out, "metadata 누락 + drop 정책: 메시지 드랍")
}

// TestSelectFieldNode_Metadata_WholeGroupSelect 는 $.metadata.device 가
// device 그룹 전체를 보존하고 나머지 그룹/string 을 제거하는지 확인한다.
func TestSelectFieldNode_Metadata_WholeGroupSelect(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"fields": map[string]any{
			"$.metadata.device": "",
		},
	})

	msg := newGroupTestMessage(
		map[string]string{"node_id": "n-1"},
		map[string]map[string]string{
			"device": {"type": "HVACR.IDU", "id": "dev-9", "name": "실내기"},
			"agent":  {"type": "serial", "id": "a-1"},
		},
	)

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)
	md := out[0].Metadata()

	device, ok := md.GetGroup("device")
	require.True(t, ok, "device 그룹은 전체 보존되어야 한다")
	assert.Equal(t, map[string]string{"type": "HVACR.IDU", "id": "dev-9", "name": "실내기"}, device)

	_, agentOk := md.GetGroup("agent")
	assert.False(t, agentOk, "화이트리스트에 없는 agent 그룹은 제거되어야 한다")
	assert.False(t, md.Has("node_id"), "화이트리스트에 없는 top-level string 키는 제거되어야 한다")
}

// TestSelectFieldNode_Metadata_GroupFieldSelect 는 $.metadata.device.id 가
// device 그룹 안의 id 필드만 남기는지 확인한다.
func TestSelectFieldNode_Metadata_GroupFieldSelect(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"fields": map[string]any{
			"$.metadata.device.id": "",
		},
	})

	msg := newGroupTestMessage(
		nil,
		map[string]map[string]string{
			"device": {"type": "HVACR.IDU", "id": "dev-9", "name": "실내기"},
			"agent":  {"type": "serial", "id": "a-1"},
		},
	)

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)
	md := out[0].Metadata()

	device, ok := md.GetGroup("device")
	require.True(t, ok, "device 그룹은 id만 남아 생존해야 한다")
	assert.Equal(t, map[string]string{"id": "dev-9"}, device)

	_, agentOk := md.GetGroup("agent")
	assert.False(t, agentOk, "화이트리스트에 없는 agent 그룹은 제거되어야 한다")
}

// --- message-level (type) 테스트 ---

// TestSelectFieldNode_Type_Cleared_IDTimestamp보존 은 $.type 이 fields 에 없으면
// type 이 비워지고, id/timestamp 는 보존되는지 확인한다.
func TestSelectFieldNode_Type_Cleared_IDTimestamp보존(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		// $.type 미포함 → type 비워짐. id/timestamp 는 항상 보존.
		"fields": map[string]any{
			"$.payload.x": "",
		},
	})

	ts := time.UnixMilli(1700000000123)
	msg := message.New(
		message.WithPayload(message.NewPayload(map[string]any{"x": 1})),
		message.WithType("device_state.change"),
		message.WithTimestamp(ts),
	)
	origID := msg.ID()

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)

	assert.Equal(t, "", out[0].Type(), "type 이 비워져야 한다")
	assert.Equal(t, origID, out[0].ID(), "id 는 보존되어야 한다")
	assert.True(t, ts.Equal(out[0].Timestamp()), "timestamp 는 보존되어야 한다")
}

// TestSelectFieldNode_Type_Kept 는 $.type 이 fields 에 있으면 type 이 유지되는지 확인한다.
func TestSelectFieldNode_Type_Kept(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"fields": map[string]any{
			"$.type":      "",
			"$.payload.x": "",
		},
	})

	msg := newTestMessage(map[string]any{"x": 1}, nil, "keep.type")
	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "keep.type", out[0].Type(), "type 이 유지되어야 한다")
}

// TestSelectFieldNode_IDTimestampPaths_Allowed 는 $.id / $.timestamp 를 fields 에
// 나열해도(no-op) 정상 동작하며 id/timestamp 가 보존되는지 확인한다.
func TestSelectFieldNode_IDTimestampPaths_Allowed(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"fields": map[string]any{
			"$.id":        "",
			"$.timestamp": "",
			"$.type":      "",
		},
	})

	ts := time.UnixMilli(1700000000456)
	msg := message.New(
		message.WithPayload(message.NewPayload(map[string]any{"a": 1})),
		message.WithType("t"),
		message.WithTimestamp(ts),
	)
	origID := msg.ID()

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, origID, out[0].ID())
	assert.True(t, ts.Equal(out[0].Timestamp()))
	assert.Equal(t, "t", out[0].Type(), "$.type 나열 시 type 유지")
	// payload 경로 없음 → payload 비워짐
	assert.Empty(t, out[0].Payload().ToMap())
}

// --- pass-through (fields 비어있음) 테스트 ---

// TestSelectFieldNode_EmptyFields_Passthrough 는 fields 가 비어있으면
// payload/metadata/type 이 변경 없이 그대로 통과하는지 확인한다.
func TestSelectFieldNode_EmptyFields_Passthrough(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{})

	ts := time.UnixMilli(1700000000999)
	msg := message.New(
		message.WithPayload(message.NewPayload(map[string]any{"a": 1, "b": 2})),
		message.WithType("orig.type"),
		message.WithTimestamp(ts),
		message.WithMetadata("m1", "v1"),
		message.WithMetadata("m2", "v2"),
	)
	origID := msg.ID()

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)

	p := out[0].Payload().ToMap()
	assert.Equal(t, 1, p["a"])
	assert.Equal(t, 2, p["b"])

	md := out[0].Metadata()
	v1, _ := md.Get("m1")
	v2, _ := md.Get("m2")
	assert.Equal(t, "v1", v1)
	assert.Equal(t, "v2", v2)

	assert.Equal(t, "orig.type", out[0].Type())
	assert.Equal(t, origID, out[0].ID())
	assert.True(t, ts.Equal(out[0].Timestamp()))
}

// TestSelectFieldNode_DefaultOnMissingIsKeep 는 on_missing 미설정 시 기본값이
// keep 인지 확인한다 (누락 키 건너뜀, 메시지 유지).
func TestSelectFieldNode_DefaultOnMissingIsKeep(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"fields": map[string]any{
			"$.payload.present": "",
			"$.payload.absent":  "X",
		},
	})

	msg := newTestMessage(map[string]any{"present": "P"}, nil, "")
	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1, "기본 keep 정책: 메시지 유지")

	p := out[0].Payload().ToMap()
	assert.Equal(t, "P", p["present"])
	_, hasAbsent := p["absent"]
	assert.False(t, hasAbsent, "기본 keep: 누락 키는 채우지 않음")
}

// TestSelectFieldNode_Configure_StringMap 는 fields 가 map[string]string 으로
// 전달되어도 올바르게 파싱되는지 확인한다 (lenient).
func TestSelectFieldNode_Configure_StringMap(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing": "fill",
		"fields": map[string]string{ // map[string]string 형식
			"$.payload.absent": "FILLED",
		},
	})

	msg := newTestMessage(map[string]any{"other": 1}, nil, "")
	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "FILLED", out[0].Payload().ToMap()["absent"])
}

// TestSelectFieldNode_Configure_UnknownOnMissing_폴백 은 알 수 없는 on_missing 값이
// 기본값 keep 으로 폴백되는지 확인한다.
func TestSelectFieldNode_Configure_UnknownOnMissing_폴백(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing": "bogus",
		"fields": map[string]any{
			"$.payload.absent": "X",
		},
	})

	msg := newTestMessage(map[string]any{"present": "P"}, nil, "")
	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1, "알 수 없는 on_missing → keep 폴백 → 메시지 유지")
	_, hasAbsent := out[0].Payload().ToMap()["absent"]
	assert.False(t, hasAbsent)
}

// TestSelectFieldNode_Configure_NonStringFillValue 는 fields 의 fill 값이
// 문자열이 아닐 때(숫자/nil) 문자열로 정규화되는지 확인한다 (lenient).
func TestSelectFieldNode_Configure_NonStringFillValue(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing": "fill",
		"fields": map[string]any{
			"$.payload.num_default": 42,  // 숫자 → "42"
			"$.payload.nil_default": nil, // nil → ""
		},
	})

	msg := newTestMessage(map[string]any{"other": 1}, nil, "")
	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)

	p := out[0].Payload().ToMap()
	assert.Equal(t, "42", p["num_default"])
	assert.Equal(t, "", p["nil_default"])
}

// --- 레거시 키 마이그레이션 에러 테스트 ---

// TestSelectFieldNode_LegacyKeys_Error 는 제거된 레거시 키가 설정에 포함되면
// Configure 가 명확한 마이그레이션 에러를 반환하는지 확인한다.
func TestSelectFieldNode_LegacyKeys_Error(t *testing.T) {
	t.Parallel()

	legacyKeys := []string{
		"payload_filter",
		"payload_fields",
		"metadata_filter",
		"metadata_fields",
		"message_filter",
		"message_fields",
	}

	for _, key := range legacyKeys {
		key := key
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			def := flow.NewNodeDef("sf-legacy", "select-field")
			node, err := NewSelectFieldNode(def)
			require.NoError(t, err)
			sf := node.(*SelectFieldNode)

			err = sf.Configure(map[string]any{
				key: map[string]any{"x": ""},
			})
			require.Error(t, err, "레거시 키 %q 는 명확한 에러를 반환해야 한다", key)
			assert.Contains(t, err.Error(), key, "에러 메시지에 레거시 키 이름이 포함되어야 한다")
			assert.Contains(t, err.Error(), "fields", "에러 메시지에 마이그레이션 대상(fields)이 안내되어야 한다")
			assert.Contains(t, err.Error(), "$.", "에러 메시지에 $.-prefixed 경로 안내가 포함되어야 한다")
		})
	}
}

// --- drop_to_port 테스트 (drop 결정 시 "drop" 포트로 emit) ---

// TestSelectFieldNode_DropToPort_True_누락시Dropped포트로emit 은
// on_missing=drop + drop_to_port=true + 누락 필드가 있을 때
// 메시지가 폐기되지 않고 "drop" 포트로 (원본 그대로) emit 되는지 확인한다.
func TestSelectFieldNode_DropToPort_True_누락시Dropped포트로emit(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing":   "drop",
		"drop_to_port": true,
		"fields": map[string]any{
			"$.payload.present": "",
			"$.payload.absent":  "DEFAULT", // 누락 → drop 트리거
		},
	})

	msg := newTestMessage(
		map[string]any{"present": "P", "extra": "E"},
		map[string]string{"m1": "v1"},
		"orig.type",
	)

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1, "drop_to_port=true: 메시지가 1개 emit 되어야 한다")

	tp, ok := out[0].Metadata().Get("_target_port")
	assert.True(t, ok, "_target_port 메타데이터가 설정되어야 한다")
	assert.Equal(t, "drop", tp, "_target_port 는 \"drop\" 여야 한다")

	// payload 는 원본 그대로 (변형되지 않음) — extra 도 보존되어야 한다.
	p := out[0].Payload().ToMap()
	assert.Equal(t, "P", p["present"], "원본 present 보존")
	assert.Equal(t, "E", p["extra"], "원본 extra 보존 (화이트리스트 적용 전 원본)")
	_, hasAbsent := p["absent"]
	assert.False(t, hasAbsent, "누락 키는 원본에도 없으므로 채워지지 않아야 한다")

	v1, ok := out[0].Metadata().Get("m1")
	assert.True(t, ok)
	assert.Equal(t, "v1", v1, "원본 metadata 보존")

	assert.Equal(t, "orig.type", out[0].Type(), "원본 type 보존")
}

// TestSelectFieldNode_DropToPort_False_기본폐기 는
// on_missing=drop + drop_to_port=false(기본값) + 누락 필드가 있을 때
// 기존 동작(메시지 폐기)이 보존되는지 확인한다.
func TestSelectFieldNode_DropToPort_False_기본폐기(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing": "drop",
		"fields": map[string]any{
			"$.payload.absent": "", // 누락 → drop 트리거
		},
		// drop_to_port 미설정 → 기본 false
	})

	msg := newTestMessage(map[string]any{"present": "P"}, nil, "")
	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Empty(t, out, "drop_to_port=false(기본): 메시지 폐기 (기존 동작 보존)")
}

// TestSelectFieldNode_DropToPort_True_누락없으면정상out 은
// on_missing=drop + drop_to_port=true 이지만 누락 필드가 없을 때
// 정상적으로 "out" 경로(_target_port 미설정 + 정상 변형)로 통과하는지 확인한다.
func TestSelectFieldNode_DropToPort_True_누락없으면정상out(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing":   "drop",
		"drop_to_port": true,
		"fields": map[string]any{
			"$.payload.keep_a": "",
			"$.payload.keep_b": "",
		},
	})

	msg := newTestMessage(map[string]any{
		"keep_a": "A",
		"keep_b": "B",
		"drop_c": "C",
	}, nil, "")

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)

	_, hasTargetPort := out[0].Metadata().Get("_target_port")
	assert.False(t, hasTargetPort, "정상 out 경로: _target_port 미설정")

	p := out[0].Payload().ToMap()
	assert.Equal(t, "A", p["keep_a"])
	assert.Equal(t, "B", p["keep_b"])
	_, hasC := p["drop_c"]
	assert.False(t, hasC, "정상 변형: 화이트리스트에 없는 키 제거")
}

// TestSelectFieldNode_DropToPort_무시됨_OnMissingKeep 는
// drop_to_port=true 라도 on_missing=keep 이면 옵션이 무시되고
// 정상 keep 동작(메시지 유지, _target_port 미설정)을 하는지 확인한다.
func TestSelectFieldNode_DropToPort_무시됨_OnMissingKeep(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing":   "keep",
		"drop_to_port": true, // keep 모드에서는 무의미
		"fields": map[string]any{
			"$.payload.present": "",
			"$.payload.absent":  "X", // 누락이지만 keep → 건너뜀
		},
	})

	msg := newTestMessage(map[string]any{"present": "P"}, nil, "")
	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1, "keep 정책: 메시지 유지")

	_, hasTargetPort := out[0].Metadata().Get("_target_port")
	assert.False(t, hasTargetPort, "keep 모드: drop_to_port 무시 → _target_port 미설정")

	p := out[0].Payload().ToMap()
	assert.Equal(t, "P", p["present"])
	_, hasAbsent := p["absent"]
	assert.False(t, hasAbsent, "keep: 누락 키 건너뜀")
}

// TestSelectFieldNode_DropToPort_StringTrue_lenient 는 drop_to_port 가
// 문자열 "true" 로 전달되어도 관대하게 파싱되는지 확인한다 (Web UI 호환).
func TestSelectFieldNode_DropToPort_StringTrue_lenient(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing":   "drop",
		"drop_to_port": "true", // 문자열 형식
		"fields": map[string]any{
			"$.payload.absent": "",
		},
	})

	msg := newTestMessage(map[string]any{"present": "P"}, nil, "")
	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1, "문자열 \"true\" → drop_to_port 활성화")
	tp, _ := out[0].Metadata().Get("_target_port")
	assert.Equal(t, "drop", tp)
}

// --- 레지스트리 등록 테스트 ---

// TestRegistry_SelectField등록 은 select-field 가 빌트인으로 등록되어 있는지 확인한다.
func TestRegistry_SelectField등록(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	assert.True(t, r.Has("select-field"), "select-field 가 등록되어 있어야 한다")

	meta, ok := r.TypeMeta("select-field")
	require.True(t, ok)
	assert.Equal(t, "select-field", meta.Type)
	assert.Equal(t, "processing", meta.Category)
	assert.Equal(t, "builtin", meta.Source)
	assert.Equal(t, "메시지에서 지정한 경로($.payload/$.metadata/$.type)만 남깁니다 (통합 화이트리스트, 누락 시 keep/drop/fill)", meta.Description)

	def := flow.NewNodeDef("sf-reg", "select-field")
	node, err := r.Create(def)
	require.NoError(t, err)
	assert.IsType(t, &SelectFieldNode{}, node)
}

// --- required_fields 테스트 (필수 필드 의미론) ---
//
// required_fields: 반드시 존재해야 하는 $.-경로 목록(key_value_map, 값은 무시).
// fields(옵션) ∪ required_fields = 유효 화이트리스트. required 누락 시 on_required_missing
// 정책(error_port 기본 / drop / error)을 적용하고 projection 으로 진행하지 않는다.

// TestSelectFieldNode_Required_Present_통과및유지 는 필수 필드가 존재하면 메시지가
// 통과하고, 화이트리스트가 required + optional(fields) 필드를 모두 유지하는지 확인한다.
func TestSelectFieldNode_Required_Present_통과및유지(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"required_fields": map[string]any{
			"$.payload.temperature": "",
		},
		"fields": map[string]any{
			"$.payload.humidity": "", // optional
		},
	})

	msg := newTestMessage(map[string]any{
		"temperature": 21.5,
		"humidity":    40,
		"drop_me":     "x",
	}, nil, "")

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1, "required 충족 시 정상 통과")

	p := out[0].Payload().ToMap()
	assert.Equal(t, 21.5, p["temperature"], "required 필드는 유지되어야 한다")
	assert.Equal(t, 40, p["humidity"], "optional(fields) 필드도 유지되어야 한다")
	_, hasDrop := p["drop_me"]
	assert.False(t, hasDrop, "화이트리스트 외 필드는 제거되어야 한다 (union 화이트리스트)")
	_, hasTargetPort := out[0].Metadata().Get("_target_port")
	assert.False(t, hasTargetPort, "정상 통과 시 _target_port 미설정")
}

// TestSelectFieldNode_Required_Missing_ErrorPort 는 required 필드 누락 시 기본
// on_required_missing=error_port 가 원본(미변형) 메시지를 "error" 포트로 라우팅하는지
// 확인한다 (_target_port="error" + MetaKeyError 에 누락 필드명 포함).
func TestSelectFieldNode_Required_Missing_ErrorPort(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		// on_required_missing 미설정 → 기본 error_port
		"required_fields": map[string]any{
			"$.payload.temperature": "",
		},
	})

	msg := newTestMessage(map[string]any{
		"humidity": 40, // temperature 없음
	}, nil, "orig.type")

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err, "error_port 모드는 Go 에러를 반환하지 않는다")
	require.Len(t, out, 1, "error_port: 메시지가 1개 emit 되어야 한다")

	tp, ok := out[0].Metadata().Get("_target_port")
	require.True(t, ok, "_target_port 메타데이터가 설정되어야 한다")
	assert.Equal(t, "error", tp, "_target_port 는 \"error\" 여야 한다")

	errMeta, ok := out[0].Metadata().Get(message.MetaKeyError)
	require.True(t, ok, "에러 메타데이터가 설정되어야 한다")
	assert.Contains(t, errMeta, "$.payload.temperature", "누락된 필수 필드명이 포함되어야 한다")

	// 원본(미변형) 보존: projection 이 적용되지 않아 원본 payload/type 가 유지된다.
	p := out[0].Payload().ToMap()
	assert.Equal(t, 40, p["humidity"], "원본 메시지는 변형되지 않아야 한다")
	assert.Equal(t, "orig.type", out[0].Type(), "원본 type 보존")
}

// TestSelectFieldNode_Required_Missing_Drop 은 on_required_missing=drop 시
// 메시지가 폐기(출력 없음)되는지 확인한다.
func TestSelectFieldNode_Required_Missing_Drop(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_required_missing": "drop",
		"required_fields": map[string]any{
			"$.payload.temperature": "",
		},
	})

	msg := newTestMessage(map[string]any{"humidity": 40}, nil, "")
	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Empty(t, out, "drop: 메시지가 폐기되어야 한다 (출력 없음)")
}

// TestSelectFieldNode_Required_Missing_Error 는 on_required_missing=error 시
// Process 가 누락 필드명을 포함한 Go 에러를 반환하는지 확인한다.
func TestSelectFieldNode_Required_Missing_Error(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_required_missing": "error",
		"required_fields": map[string]any{
			"$.payload.temperature": "",
		},
	})

	msg := newTestMessage(map[string]any{"humidity": 40}, nil, "")
	out, err := sf.Process(context.Background(), msg)
	require.Error(t, err, "error 모드는 Go 에러를 반환해야 한다")
	assert.Contains(t, err.Error(), "$.payload.temperature", "에러에 누락 필드명이 포함되어야 한다")
	assert.Nil(t, out, "error 모드는 출력 메시지를 반환하지 않는다")
}

// TestSelectFieldNode_Required_MetadataGroup_Present 는 metadata group 필수 경로
// ($.metadata.device.id)가 존재하면 통과하고 해당 그룹 필드가 유지되는지 확인한다.
func TestSelectFieldNode_Required_MetadataGroup_Present(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"required_fields": map[string]any{
			"$.metadata.device.id": "",
		},
	})

	msg := newGroupTestMessage(
		nil,
		map[string]map[string]string{
			"device": {"id": "dev-9", "name": "실내기"},
		},
	)

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)
	device, ok := out[0].Metadata().GetGroup("device")
	require.True(t, ok, "required group 필드는 유지되어야 한다")
	assert.Equal(t, map[string]string{"id": "dev-9"}, device, "required 경로만 유지 (union 화이트리스트)")
}

// TestSelectFieldNode_Required_MetadataGroup_Missing 은 metadata group 필수 경로의
// 필드가 없으면 error_port 로 라우팅되는지 확인한다.
func TestSelectFieldNode_Required_MetadataGroup_Missing(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"required_fields": map[string]any{
			"$.metadata.device.id": "",
		},
	})

	msg := newGroupTestMessage(
		nil,
		map[string]map[string]string{
			"device": {"name": "실내기"}, // id 없음
		},
	)

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)
	tp, _ := out[0].Metadata().Get("_target_port")
	assert.Equal(t, "error", tp, "group 필드 누락 → error_port")
	errMeta, _ := out[0].Metadata().Get(message.MetaKeyError)
	assert.Contains(t, errMeta, "$.metadata.device.id")
}

// TestSelectFieldNode_Optional_Absent_NoError 는 optional(fields) 필드가 없어도
// 에러 없이 그냥 건너뛰는지(누락 keep), 존재하면 유지되는지 확인한다.
func TestSelectFieldNode_Optional_Absent_NoError(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"required_fields": map[string]any{
			"$.payload.temperature": "",
		},
		"fields": map[string]any{
			"$.payload.optional_a": "", // 부재
			"$.payload.optional_b": "", // 존재
		},
	})

	msg := newTestMessage(map[string]any{
		"temperature": 20,
		"optional_b":  "B",
	}, nil, "")

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err, "optional 부재는 에러가 아니다")
	require.Len(t, out, 1)
	p := out[0].Payload().ToMap()
	assert.Equal(t, 20, p["temperature"], "required 유지")
	assert.Equal(t, "B", p["optional_b"], "존재하는 optional 유지")
	_, hasA := p["optional_a"]
	assert.False(t, hasA, "부재 optional 은 건너뜀(keep)")
}

// TestSelectFieldNode_Required_And_Fields_둘다비면passthrough 는 fields 와
// required_fields 가 모두 비어있으면 pass-through 인지 확인한다 (기존 동작 보존).
func TestSelectFieldNode_Required_And_Fields_둘다비면passthrough(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		// fields, required_fields 모두 없음
	})

	msg := newTestMessage(map[string]any{"a": 1, "b": 2}, map[string]string{"m": "v"}, "t")
	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)
	p := out[0].Payload().ToMap()
	assert.Equal(t, 1, p["a"])
	assert.Equal(t, 2, p["b"], "둘 다 비면 pass-through (모든 필드 유지)")
	assert.Equal(t, "t", out[0].Type(), "type 보존")
}

// TestSelectFieldNode_Required_TypePresence 는 $.type 필수 경로가 type 이 비어있으면
// 누락으로 간주되고, 존재하면 통과하는지 확인한다.
func TestSelectFieldNode_Required_TypePresence(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"required_fields": map[string]any{
			"$.type": "",
		},
	})

	// type 존재 → 통과 + type 유지
	out, err := sf.Process(context.Background(), newTestMessage(map[string]any{"x": 1}, nil, "evt"))
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "evt", out[0].Type(), "required $.type 는 유지되어야 한다")

	// type 비어있음 → 누락 → error_port
	out2, err2 := sf.Process(context.Background(), newTestMessage(map[string]any{"x": 1}, nil, ""))
	require.NoError(t, err2)
	require.Len(t, out2, 1)
	tp, _ := out2[0].Metadata().Get("_target_port")
	assert.Equal(t, "error", tp, "빈 type → required 누락 → error_port")
}

// TestSelectFieldNode_Required_RunsBeforeProjection 는 required 누락이
// on_missing=drop 보다 먼저 평가되어 on_required_missing 정책이 우선하는지 확인한다.
func TestSelectFieldNode_Required_RunsBeforeProjection(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing":          "drop", // optional 누락 시 drop (폐기)
		"on_required_missing": "error",
		"required_fields": map[string]any{
			"$.payload.temperature": "",
		},
		"fields": map[string]any{
			"$.payload.optional_a": "",
		},
	})

	// temperature(required) 누락 → on_required_missing=error 가 우선해야 한다
	msg := newTestMessage(map[string]any{"other": 1}, nil, "")
	out, err := sf.Process(context.Background(), msg)
	require.Error(t, err, "required 누락은 on_missing=drop 보다 먼저 평가되어 error 를 반환")
	assert.Contains(t, err.Error(), "$.payload.temperature")
	assert.Nil(t, out)
}

// TestSelectFieldNode_BackwardCompat_FieldsOnly 는 required_fields 없이 fields 만
// 설정된 기존 설정이 정확히 기존과 동일하게 동작하는지 확인한다 (회귀 방지).
func TestSelectFieldNode_BackwardCompat_FieldsOnly(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"fields": map[string]any{
			"$.payload.keep": "",
		},
	})

	msg := newTestMessage(map[string]any{"keep": "K", "drop": "D"}, nil, "t")
	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)
	p := out[0].Payload().ToMap()
	assert.Equal(t, "K", p["keep"])
	_, hasDrop := p["drop"]
	assert.False(t, hasDrop)
	assert.Equal(t, "", out[0].Type(), "$.type 미선택 → type 비움 (기존 동작)")
	_, hasTargetPort := out[0].Metadata().Get("_target_port")
	assert.False(t, hasTargetPort)
}
