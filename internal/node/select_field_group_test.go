package node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/message"
)

// newGroupTestMessage 는 string 메타데이터와 nested group 메타데이터를 함께 가진
// 테스트 메시지를 생성한다.
//
//	strings: top-level string 메타데이터 (예: {"node_id": "n-1"})
//	groups:  nested group 메타데이터 (예: {"agent": {"type": "serial", "id": "a-1"}})
func newGroupTestMessage(strings map[string]string, groups map[string]map[string]string) message.Message {
	opts := []message.Option{
		message.WithPayload(message.NewPayload(map[string]any{"x": 1})),
	}
	for k, v := range strings {
		opts = append(opts, message.WithMetadata(k, v))
	}
	msg := message.New(opts...)
	for gk, fields := range groups {
		msg.Metadata().SetGroup(gk, fields)
	}
	return msg
}

// TestSelectFieldNode_Group_InGroupMultiFieldSelect 는 [$.metadata.device.id,
// $.metadata.device.name] 가 device 그룹을 {id, name} 으로 축소하는지 확인한다.
func TestSelectFieldNode_Group_InGroupMultiFieldSelect(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"fields": map[string]any{
			"$.metadata.device.id":   "",
			"$.metadata.device.name": "",
		},
	})

	msg := newGroupTestMessage(
		nil,
		map[string]map[string]string{
			"device": {"type": "HVACR.IDU", "id": "dev-9", "name": "실내기"},
		},
	)

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)

	device, ok := out[0].Metadata().GetGroup("device")
	require.True(t, ok)
	assert.Equal(t, map[string]string{"id": "dev-9", "name": "실내기"}, device)
}

// TestSelectFieldNode_Group_Mixed 는 [$.metadata.node_id, $.metadata.agent.type,
// $.metadata.device] 혼합 선택이 node_id string 보존 + agent 그룹을 {type}으로 축소 +
// device 그룹 전체 보존하는지 확인한다.
func TestSelectFieldNode_Group_Mixed(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"fields": map[string]any{
			"$.metadata.node_id":    "",
			"$.metadata.agent.type": "",
			"$.metadata.device":     "",
		},
	})

	msg := newGroupTestMessage(
		map[string]string{"node_id": "n-1", "extra": "drop-me"},
		map[string]map[string]string{
			"device": {"type": "HVACR.IDU", "id": "dev-9", "name": "실내기"},
			"agent":  {"type": "serial", "id": "a-1", "name": "에이전트"},
		},
	)

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)
	md := out[0].Metadata()

	v, ok := md.Get("node_id")
	assert.True(t, ok)
	assert.Equal(t, "n-1", v)
	assert.False(t, md.Has("extra"))

	agent, ok := md.GetGroup("agent")
	require.True(t, ok)
	assert.Equal(t, map[string]string{"type": "serial"}, agent)

	device, ok := md.GetGroup("device")
	require.True(t, ok)
	assert.Equal(t, map[string]string{"type": "HVACR.IDU", "id": "dev-9", "name": "실내기"}, device)
}

// TestSelectFieldNode_Group_WholeWinsOverField 는 [$.metadata.device,
// $.metadata.device.id] 처럼 같은 그룹에 전체 키와 필드 키가 동시에 있을 때,
// 전체 키(whole)가 우선하는지 확인한다.
func TestSelectFieldNode_Group_WholeWinsOverField(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"fields": map[string]any{
			"$.metadata.device":    "",
			"$.metadata.device.id": "",
		},
	})

	msg := newGroupTestMessage(
		nil,
		map[string]map[string]string{
			"device": {"type": "HVACR.IDU", "id": "dev-9", "name": "실내기"},
		},
	)

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)

	device, ok := out[0].Metadata().GetGroup("device")
	require.True(t, ok)
	assert.Equal(t, map[string]string{"type": "HVACR.IDU", "id": "dev-9", "name": "실내기"}, device,
		"전체 그룹 선택이 필드 선택을 이겨 그룹 전체가 보존되어야 한다")
}

// TestSelectFieldNode_Group_EmptySubsetRemovesGroup 는 선택된 그룹 필드가
// 실제 그룹에 하나도 존재하지 않으면(빈 subset) 그룹이 제거되는지 확인한다.
func TestSelectFieldNode_Group_EmptySubsetRemovesGroup(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"fields": map[string]any{
			"$.metadata.device.nonexistent": "",
		},
	})

	msg := newGroupTestMessage(
		nil,
		map[string]map[string]string{
			"device": {"type": "HVACR.IDU", "id": "dev-9"},
		},
	)

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)

	_, ok := out[0].Metadata().GetGroup("device")
	assert.False(t, ok, "선택 필드가 그룹에 없으면(빈 subset) 그룹이 제거되어야 한다")
}

// TestSelectFieldNode_Group_OnMissingDrop_FieldAbsent 는 on_missing=drop 일 때
// $.metadata.device.id 가 가리키는 필드가 그룹에 없으면 메시지를 드랍하는지 확인한다.
func TestSelectFieldNode_Group_OnMissingDrop_FieldAbsent(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing": "drop",
		"fields": map[string]any{
			"$.metadata.device.id": "",
		},
	})

	// device 그룹은 있으나 id 필드가 없음 → 누락
	msg := newGroupTestMessage(
		nil,
		map[string]map[string]string{
			"device": {"type": "HVACR.IDU"},
		},
	)

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Empty(t, out, "device.id 누락 + drop 정책: 메시지 드랍")
}

// TestSelectFieldNode_Group_OnMissingDrop_GroupAbsent 는 on_missing=drop 일 때
// 그룹 자체가 없으면 $.metadata.device.id 가 누락으로 간주되어 드랍하는지 확인한다.
func TestSelectFieldNode_Group_OnMissingDrop_GroupAbsent(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing": "drop",
		"fields": map[string]any{
			"$.metadata.device.id": "",
		},
	})

	// device 그룹 자체가 없음
	msg := newGroupTestMessage(map[string]string{"node_id": "n-1"}, nil)

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	assert.Empty(t, out, "device 그룹 부재 + drop 정책: 메시지 드랍")
}

// TestSelectFieldNode_Group_OnMissingFill_FieldInGroup 은 on_missing=fill 일 때
// 그룹이 존재하지만 선택 필드가 없으면 기본값으로 채우는지 확인한다.
func TestSelectFieldNode_Group_OnMissingFill_FieldInGroup(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing": "fill",
		"fields": map[string]any{
			"$.metadata.device.id": "DEFAULT_ID",
		},
	})

	// device 그룹 존재, id 필드 없음 → fill
	msg := newGroupTestMessage(
		nil,
		map[string]map[string]string{
			"device": {"type": "HVACR.IDU"},
		},
	)

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)

	device, ok := out[0].Metadata().GetGroup("device")
	require.True(t, ok)
	assert.Equal(t, map[string]string{"id": "DEFAULT_ID"}, device,
		"누락된 그룹 필드는 fill 기본값으로 채워져야 한다")
}

// TestSelectFieldNode_Group_OnMissingFill_GroupAbsent 는 on_missing=fill 일 때
// 그룹 자체가 없으면 그룹을 만들어 필드를 채우는지 확인한다.
func TestSelectFieldNode_Group_OnMissingFill_GroupAbsent(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"on_missing": "fill",
		"fields": map[string]any{
			"$.metadata.device.id": "DEFAULT_ID",
		},
	})

	msg := newGroupTestMessage(map[string]string{"node_id": "n-1"}, nil)

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)

	device, ok := out[0].Metadata().GetGroup("device")
	require.True(t, ok, "그룹 부재 + fill: 그룹이 생성되어야 한다")
	assert.Equal(t, map[string]string{"id": "DEFAULT_ID"}, device)
}

// TestSelectFieldNode_Group_TooDeepIgnored 는 한 단계 초과 경로($.metadata.a.b.c)가
// 무시(에러 관대)되는지 확인한다 — 해당 엔트리는 어떤 것도 선택하지 않는다.
func TestSelectFieldNode_Group_TooDeepIgnored(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"fields": map[string]any{
			"$.metadata.device.id.deep": "", // group 2단계 초과 → 무시
			"$.metadata.device.id":      "", // 유효
		},
	})

	msg := newGroupTestMessage(
		nil,
		map[string]map[string]string{
			"device": {"type": "HVACR.IDU", "id": "dev-9"},
		},
	)

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)

	device, ok := out[0].Metadata().GetGroup("device")
	require.True(t, ok)
	assert.Equal(t, map[string]string{"id": "dev-9"}, device,
		"$.metadata.a.b.c 는 무시되고 유효한 $.metadata.device.id 만 적용되어야 한다")
}

// TestSelectFieldNode_Group_BackwardCompat_StringOnly 는 그룹이 없는
// top-level string 전용 설정이 동작하는지 확인한다.
func TestSelectFieldNode_Group_BackwardCompat_StringOnly(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"fields": map[string]any{
			"$.metadata.node_id": "",
		},
	})

	msg := newGroupTestMessage(
		map[string]string{"node_id": "n-1", "other": "drop"},
		nil,
	)

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)
	md := out[0].Metadata()

	v, ok := md.Get("node_id")
	assert.True(t, ok)
	assert.Equal(t, "n-1", v)
	assert.False(t, md.Has("other"))
}

// TestSelectFieldNode_Group_RemovedWhenNotListed 는 화이트리스트에 없는 그룹이
// 제거되는지 확인한다 (단일 화이트리스트: 미나열 그룹 제거).
func TestSelectFieldNode_Group_RemovedWhenNotListed(t *testing.T) {
	t.Parallel()
	sf := newSelectFieldForTest(t, map[string]any{
		"fields": map[string]any{
			"$.metadata.node_id": "",
		},
	})

	msg := newGroupTestMessage(
		map[string]string{"node_id": "n-1"},
		map[string]map[string]string{
			"agent": {"type": "serial"},
		},
	)

	out, err := sf.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, out, 1)
	md := out[0].Metadata()

	v, ok := md.Get("node_id")
	assert.True(t, ok)
	assert.Equal(t, "n-1", v)
	_, agentOk := md.GetGroup("agent")
	assert.False(t, agentOk, "화이트리스트에 없는 그룹은 제거되어야 한다")
}
