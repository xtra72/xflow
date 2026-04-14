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
		"type":      "idu",
		"idu_num":   float64(iduNum),
		"room_temp": roomTemp,
		"set_temp":  float64(setTemp),
		"op_mode":   float64(20),
		"fan_byte":  float64(84),
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
		"compare_fields": "room_temp,set_temp",
	})

	results, _ := n.Process(context.Background(), iduMsg(1, 20.5, 22))
	assert.Len(t, results, 1)

	// fan_byte만 다름 → compare_fields에 없으므로 중복
	msg2 := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"type":      "idu",
		"idu_num":   float64(1),
		"room_temp": 20.5,
		"set_temp":  float64(22),
		"op_mode":   float64(20),
		"fan_byte":  float64(20), // 변경
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
