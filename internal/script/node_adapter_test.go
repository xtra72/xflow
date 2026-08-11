package script

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/pkg/message"
)

// TestMapToMessage_부분갱신_type_timestamp_id_보존 은 스크립트가 payload 만
// 수정하고 msg 를 반환하는(부분 갱신) 경우 type/timestamp/id 가 원본에서
// 보존되는지 검증한다(SPEC-MESSAGE-SPLIT-001 회귀 방지).
//
// 이전 버그: mapToMessage 가 payload/metadata 만 읽어 message.New() 가 type 을
// "" 로 리셋하고 새 uuid / time.Now() 를 발급했다.
func TestMapToMessage_부분갱신_type_timestamp_id_보존(t *testing.T) {
	origTs := time.UnixMilli(1786457564195)
	original := message.New(
		message.WithID("orig-id-123"),
		message.WithType("device_state.report"),
		message.WithTimestamp(origTs),
		message.WithPayload(message.NewPayload(map[string]any{"temp": "20"})),
	)

	// MessageToLuaTable → FromLuaValue 왕복을 모사: timestamp 는 epoch ms(float64).
	// 스크립트는 payload 만 수정하고 나머지는 원본 값을 그대로 되돌려준다.
	m := map[string]any{
		"id":        original.ID(),
		"type":      original.Type(),
		"timestamp": float64(original.Timestamp().UnixMilli()),
		"payload":   map[string]any{"temp": "21"},
	}

	out := mapToMessage(m, original)

	assert.Equal(t, "device_state.report", out.Type(), "type 이 원본에서 보존되어야 함")
	assert.Equal(t, "orig-id-123", out.ID(), "id 가 원본에서 보존되어야 함")
	assert.Equal(t, origTs.UnixMilli(), out.Timestamp().UnixMilli(), "timestamp 가 원본에서 보존되어야 함")
	temp, ok := out.Payload().Get("temp")
	require.True(t, ok)
	assert.Equal(t, "21", temp, "payload 는 갱신된 값이어야 함")
}

// TestMapToMessage_반환값_type_우선 은 스크립트가 msg.type 을 명시적으로
// 설정하면 반환 map 의 값이 원본보다 우선하는지 검증한다.
func TestMapToMessage_반환값_type_우선(t *testing.T) {
	original := message.New(
		message.WithID("orig-id-456"),
		message.WithType("old_type"),
		message.WithTimestamp(time.UnixMilli(1000)),
	)

	m := map[string]any{
		"id":        original.ID(),
		"type":      "new_type",
		"timestamp": float64(original.Timestamp().UnixMilli()),
	}

	out := mapToMessage(m, original)

	assert.Equal(t, "new_type", out.Type(), "반환 map 의 type 이 원본보다 우선해야 함")
	assert.Equal(t, "orig-id-456", out.ID())
	assert.Equal(t, int64(1000), out.Timestamp().UnixMilli())
}

// TestMapToMessage_키누락_원본보존 은 반환 map 에 type/timestamp/id 키가 아예
// 없어도 원본 값이 보존되는지 검증한다(방어적 경로).
func TestMapToMessage_키누락_원본보존(t *testing.T) {
	origTs := time.UnixMilli(1786457564195)
	original := message.New(
		message.WithID("orig-id-789"),
		message.WithType("device_state.report"),
		message.WithTimestamp(origTs),
	)

	// payload 만 담긴 반환 map — top-level 키는 모두 누락.
	m := map[string]any{
		"payload": map[string]any{"x": "1"},
	}

	out := mapToMessage(m, original)

	assert.Equal(t, "device_state.report", out.Type())
	assert.Equal(t, "orig-id-789", out.ID())
	assert.Equal(t, origTs.UnixMilli(), out.Timestamp().UnixMilli())
}

// TestLuaTimestampMs 는 다양한 수치 형태에서 epoch ms 변환이 동작하는지 검증한다.
func TestLuaTimestampMs(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want int64
		ok   bool
	}{
		{"float64", float64(1786457564195), 1786457564195, true},
		{"int64", int64(1786457564195), 1786457564195, true},
		{"int", int(1000), 1000, true},
		{"int32", int32(1000), 1000, true},
		{"float32", float32(1000), 1000, true},
		{"string_불가", "1000", 0, false},
		{"nil_불가", nil, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := luaTimestampMs(c.in)
			assert.Equal(t, c.ok, ok)
			if c.ok {
				assert.Equal(t, c.want, got)
			}
		})
	}
}
