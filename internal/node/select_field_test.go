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

// --- payload 필터 테스트 ---

// TestSelectFieldNode_PayloadFilter_화이트리스트키만유지 는 payload_filter 가 켜졌을 때
// 화이트리스트에 명시된 키만 유지되고 나머지는 제거되는지 확인한다.
func TestSelectFieldNode_PayloadFilter_화이트리스트키만유지(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"payload_filter": true,
		"payload_fields": map[string]any{
			"keep_a": "",
			"keep_b": "",
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

// TestSelectFieldNode_OnMissing 은 on_missing 정책(ignore/fill/drop)별 동작을 검증한다.
func TestSelectFieldNode_OnMissing(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		onMissing   string
		wantDropped bool           // 메시지 드랍 여부 (출력 0개)
		wantPayload map[string]any // 드랍 아닐 때 기대 payload (subset 비교)
		wantMissing []string       // 드랍 아닐 때 존재하지 않아야 할 키
	}{
		{
			name:        "ignore_누락키건너뜀",
			onMissing:   "ignore",
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
				"on_missing":     tc.onMissing,
				"payload_filter": true,
				"payload_fields": map[string]any{
					"present": "",        // 존재하는 키
					"absent":  "DEFAULT", // 누락된 키 (fill 기본값)
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

// --- metadata 필터 테스트 ---

// TestSelectFieldNode_MetadataFilter_화이트리스트키만유지 는 metadata_filter 가 켜졌을 때
// 화이트리스트 metadata 키만 유지되는지 확인한다.
func TestSelectFieldNode_MetadataFilter_화이트리스트키만유지(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"metadata_filter": true,
		"metadata_fields": map[string]any{
			"keep_meta": "",
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

// TestSelectFieldNode_MetadataFilter_OnMissingFill 은 metadata 필터에서 fill 정책이
// 누락 키를 기본값으로 채우는지 확인한다.
func TestSelectFieldNode_MetadataFilter_OnMissingFill(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing":      "fill",
		"metadata_filter": true,
		"metadata_fields": map[string]any{
			"absent_meta": "META_DEFAULT",
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

// TestSelectFieldNode_MetadataFilter_OnMissingDrop 은 metadata 누락 시 drop 정책이
// 메시지를 드랍하는지 확인한다.
func TestSelectFieldNode_MetadataFilter_OnMissingDrop(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing":      "drop",
		"metadata_filter": true,
		"metadata_fields": map[string]any{
			"absent_meta": "",
		},
	})

	msg := newTestMessage(map[string]any{"x": 1}, map[string]string{"other": "v"}, "")

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Empty(t, out, "metadata 누락 + drop 정책: 메시지 드랍")
}

// --- message-level 필터 테스트 ---

// TestSelectFieldNode_MessageFilter_Type비움_IDTimestamp보존 은 message_filter 가 켜지고
// type 이 화이트리스트에 없을 때 type 이 비워지고, id/timestamp 는 보존되는지 확인한다.
func TestSelectFieldNode_MessageFilter_Type비움_IDTimestamp보존(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"message_filter": true,
		"message_fields": []any{"id", "timestamp"}, // type 미포함 → 비워짐
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

// TestSelectFieldNode_MessageFilter_Type유지 는 type 이 화이트리스트에 있으면
// type 이 유지되는지 확인한다.
func TestSelectFieldNode_MessageFilter_Type유지(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"message_filter": true,
		"message_fields": []any{"id", "type", "timestamp"},
	})

	msg := newTestMessage(map[string]any{"x": 1}, nil, "keep.type")
	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "keep.type", out[0].Type(), "type 이 유지되어야 한다")
}

// TestSelectFieldNode_MessageFilter_StringSlice 는 message_fields 가 []string 으로
// 전달되어도 올바르게 파싱되는지 확인한다 (lenient).
func TestSelectFieldNode_MessageFilter_StringSlice(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"message_filter": true,
		"message_fields": []string{"id", "timestamp"}, // []string 형식
	})

	msg := newTestMessage(map[string]any{"x": 1}, nil, "some.type")
	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "", out[0].Type())
}

// --- pass-through (필터 off) 테스트 ---

// TestSelectFieldNode_필터off_passthrough 는 모든 필터가 꺼졌을 때(기본값)
// payload/metadata/type 이 변경되지 않고 그대로 통과하는지 확인한다.
func TestSelectFieldNode_필터off_passthrough(t *testing.T) {
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

// TestSelectFieldNode_DefaultOnMissingIsIgnore 는 on_missing 미설정 시 기본값이
// ignore 인지 확인한다 (누락 키 건너뜀, 메시지 유지).
func TestSelectFieldNode_DefaultOnMissingIsIgnore(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"payload_filter": true,
		"payload_fields": map[string]any{
			"present": "",
			"absent":  "X",
		},
	})

	msg := newTestMessage(map[string]any{"present": "P"}, nil, "")
	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1, "기본 ignore 정책: 메시지 유지")

	p := out[0].Payload().ToMap()
	assert.Equal(t, "P", p["present"])
	_, hasAbsent := p["absent"]
	assert.False(t, hasAbsent, "기본 ignore: 누락 키는 채우지 않음")
}

// TestSelectFieldNode_Configure_StringMap 은 payload_fields 가 map[string]string 으로
// 전달되어도 올바르게 파싱되는지 확인한다 (lenient).
func TestSelectFieldNode_Configure_StringMap(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing":     "fill",
		"payload_filter": true,
		"payload_fields": map[string]string{ // map[string]string 형식
			"absent": "FILLED",
		},
	})

	msg := newTestMessage(map[string]any{"other": 1}, nil, "")
	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "FILLED", out[0].Payload().ToMap()["absent"])
}

// TestSelectFieldNode_Configure_UnknownOnMissing_폴백 은 알 수 없는 on_missing 값이
// 기본값 ignore 로 폴백되는지 확인한다.
func TestSelectFieldNode_Configure_UnknownOnMissing_폴백(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing":     "bogus",
		"payload_filter": true,
		"payload_fields": map[string]any{"absent": "X"},
	})

	msg := newTestMessage(map[string]any{"present": "P"}, nil, "")
	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1, "알 수 없는 on_missing → ignore 폴백 → 메시지 유지")
	_, hasAbsent := out[0].Payload().ToMap()["absent"]
	assert.False(t, hasAbsent)
}

// TestSelectFieldNode_DropOnlyWhenFilterEnabled 는 필터 그룹이 비활성화되어 있으면
// 누락 키가 있어도 drop 정책이 메시지를 드랍하지 않는지 확인한다.
func TestSelectFieldNode_DropOnlyWhenFilterEnabled(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing":     "drop",
		"payload_filter": false, // 비활성화
		"payload_fields": map[string]any{"absent": ""},
	})

	msg := newTestMessage(map[string]any{"present": "P"}, nil, "")
	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1, "필터 비활성화 시 drop 적용 안 됨 → 메시지 유지 (pass-through)")
	assert.Equal(t, "P", out[0].Payload().ToMap()["present"])
}

// TestSelectFieldNode_Configure_NonStringFillValue 는 payload_fields 의 fill 값이
// 문자열이 아닐 때(숫자/nil) 문자열로 정규화되는지 확인한다 (parseStringMap lenient).
func TestSelectFieldNode_Configure_NonStringFillValue(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing":     "fill",
		"payload_filter": true,
		"payload_fields": map[string]any{
			"num_default": 42,  // 숫자 → "42"
			"nil_default": nil, // nil → ""
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

// TestSelectFieldNode_MetadataFilter_NonStringExistingValue 는 metadata 가 항상
// 문자열로 저장되므로, metadata 화이트리스트 유지 시 값이 그대로 보존되는지 확인한다.
func TestSelectFieldNode_MetadataFilter_NonStringExistingValue(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"metadata_filter": true,
		"metadata_fields": map[string]string{"keep": ""},
	})

	msg := newTestMessage(map[string]any{"x": 1}, map[string]string{"keep": "123", "drop": "y"}, "")
	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)

	v, ok := out[0].Metadata().Get("keep")
	assert.True(t, ok)
	assert.Equal(t, "123", v)
	assert.False(t, out[0].Metadata().Has("drop"))
}

// TestSelectFieldNode_Configure_빈설정_무필터 은 빈 *_fields/무토글 설정에서
// 안전하게 동작(pass-through)하는지 확인한다.
func TestSelectFieldNode_Configure_빈설정_무필터(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"payload_filter":  false,
		"metadata_filter": false,
		"message_filter":  false,
	})

	msg := newTestMessage(map[string]any{"a": 1}, map[string]string{"m": "v"}, "t")
	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, 1, out[0].Payload().ToMap()["a"])
	assert.Equal(t, "t", out[0].Type())
}

// --- drop_to_port 테스트 (drop 결정 시 "dropped" 포트로 emit) ---

// TestSelectFieldNode_DropToPort_True_누락시Dropped포트로emit 은
// on_missing=drop + drop_to_port=true + 누락 필드가 있을 때
// 메시지가 폐기되지 않고 "dropped" 포트로 (원본 그대로) emit 되는지 확인한다.
func TestSelectFieldNode_DropToPort_True_누락시Dropped포트로emit(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing":     "drop",
		"drop_to_port":   true,
		"payload_filter": true,
		"payload_fields": map[string]any{
			"present": "",
			"absent":  "DEFAULT", // 누락 → drop 트리거
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

	// "dropped" 포트로 라우팅 표시되어야 한다.
	tp, ok := out[0].Metadata().Get("_target_port")
	assert.True(t, ok, "_target_port 메타데이터가 설정되어야 한다")
	assert.Equal(t, "dropped", tp, "_target_port 는 \"dropped\" 여야 한다")

	// payload 는 원본 그대로 (변형되지 않음) — extra 도 보존되어야 한다.
	p := out[0].Payload().ToMap()
	assert.Equal(t, "P", p["present"], "원본 present 보존")
	assert.Equal(t, "E", p["extra"], "원본 extra 보존 (화이트리스트 적용 전 원본)")
	_, hasAbsent := p["absent"]
	assert.False(t, hasAbsent, "누락 키는 원본에도 없으므로 채워지지 않아야 한다")

	// metadata 는 _target_port 외에는 원본 그대로여야 한다.
	v1, ok := out[0].Metadata().Get("m1")
	assert.True(t, ok)
	assert.Equal(t, "v1", v1, "원본 metadata 보존")

	// type 도 원본 그대로여야 한다.
	assert.Equal(t, "orig.type", out[0].Type(), "원본 type 보존")
}

// TestSelectFieldNode_DropToPort_False_기본폐기 는
// on_missing=drop + drop_to_port=false(기본값) + 누락 필드가 있을 때
// 기존 동작(메시지 폐기)이 보존되는지 확인한다.
func TestSelectFieldNode_DropToPort_False_기본폐기(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing":     "drop",
		"payload_filter": true,
		"payload_fields": map[string]any{
			"absent": "", // 누락 → drop 트리거
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
		"on_missing":     "drop",
		"drop_to_port":   true,
		"payload_filter": true,
		"payload_fields": map[string]any{
			"keep_a": "",
			"keep_b": "",
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

	// 정상 out 경로: _target_port 가 설정되어선 안 된다.
	_, hasTargetPort := out[0].Metadata().Get("_target_port")
	assert.False(t, hasTargetPort, "정상 out 경로: _target_port 미설정")

	// payload 는 정상 변형 (화이트리스트 적용) 되어야 한다.
	p := out[0].Payload().ToMap()
	assert.Equal(t, "A", p["keep_a"])
	assert.Equal(t, "B", p["keep_b"])
	_, hasC := p["drop_c"]
	assert.False(t, hasC, "정상 변형: 화이트리스트에 없는 키 제거")
}

// TestSelectFieldNode_DropToPort_무시됨_OnMissingIgnore 는
// drop_to_port=true 라도 on_missing=ignore 이면 옵션이 무시되고
// 정상 ignore 동작(메시지 유지, _target_port 미설정)을 하는지 확인한다.
func TestSelectFieldNode_DropToPort_무시됨_OnMissingIgnore(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing":     "ignore",
		"drop_to_port":   true, // ignore 모드에서는 무의미
		"payload_filter": true,
		"payload_fields": map[string]any{
			"present": "",
			"absent":  "X", // 누락이지만 ignore → 건너뜀
		},
	})

	msg := newTestMessage(map[string]any{"present": "P"}, nil, "")
	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1, "ignore 정책: 메시지 유지")

	// drop_to_port 가 적용되지 않아야 한다.
	_, hasTargetPort := out[0].Metadata().Get("_target_port")
	assert.False(t, hasTargetPort, "ignore 모드: drop_to_port 무시 → _target_port 미설정")

	p := out[0].Payload().ToMap()
	assert.Equal(t, "P", p["present"])
	_, hasAbsent := p["absent"]
	assert.False(t, hasAbsent, "ignore: 누락 키 건너뜀")
}

// TestSelectFieldNode_DropToPort_StringTrue_lenient 는 drop_to_port 가
// 문자열 "true" 로 전달되어도 관대하게 파싱되는지 확인한다 (Web UI 호환).
func TestSelectFieldNode_DropToPort_StringTrue_lenient(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing":     "drop",
		"drop_to_port":   "true", // 문자열 형식
		"payload_filter": true,
		"payload_fields": map[string]any{"absent": ""},
	})

	msg := newTestMessage(map[string]any{"present": "P"}, nil, "")
	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1, "문자열 \"true\" → drop_to_port 활성화")
	tp, _ := out[0].Metadata().Get("_target_port")
	assert.Equal(t, "dropped", tp)
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
	assert.Equal(t, "메시지에서 지정한 필드만 남깁니다 (payload/metadata/message 그룹별 화이트리스트, 누락 시 무시/드랍/채움)", meta.Description)

	// 팩토리로 생성 가능한지 확인.
	def := flow.NewNodeDef("sf-reg", "select-field")
	node, err := r.Create(def)
	require.NoError(t, err)
	assert.IsType(t, &SelectFieldNode{}, node)
}
