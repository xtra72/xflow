package engine

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/pkg/message"
)

// TestErrorMessageSummary_CarriesMetadataAndPayload 는 에러 로그 요약이 어떤
// 메시지가 터졌는지 판별할 수 있는 정보(metadata + payload)를 싣는지 검증한다.
// 에러 문구만으로는 형태가 다른 메시지가 섞인 스트림에서 원인을 특정할 수 없다.
func TestErrorMessageSummary_CarriesMetadataAndPayload(t *testing.T) {
	msg := message.New(
		message.WithPayload(message.NewPayload(map[string]any{"value": false})),
		message.WithTimestamp(time.UnixMilli(1787706926119)),
	)
	msg.SetType("device_state.report")
	msg.Metadata().Set("measurement", "power")
	msg.Metadata().SetGroup("device", map[string]string{
		"id": "60756b6f", "name": "집중회의실", "type": "HVACR.IDU",
	})

	var got struct {
		ID        string         `json:"id"`
		Type      string         `json:"type"`
		Timestamp int64          `json:"timestamp"`
		Metadata  map[string]any `json:"metadata"`
		Payload   map[string]any `json:"payload"`
	}
	require.NoError(t, json.Unmarshal([]byte(errorMessageSummary(msg)), &got))

	assert.Equal(t, msg.ID(), got.ID)
	assert.Equal(t, "device_state.report", got.Type)
	assert.Equal(t, int64(1787706926119), got.Timestamp, "디버그 노드와 같은 epoch ms")
	assert.Equal(t, "power", got.Metadata["measurement"])
	assert.Equal(t, map[string]any{"value": false}, got.Payload)
	// 그룹 metadata 도 중첩 객체로 보존된다 (어느 디바이스인지 식별 가능해야 한다).
	device, ok := got.Metadata["device"].(map[string]any)
	require.True(t, ok, "device 그룹이 중첩 객체로 보존되어야 한다")
	assert.Equal(t, "집중회의실", device["name"])
}

// TestErrorMessageSummary_TruncatesLargePayload 는 대용량 payload 가 로그를
// 잠식하지 않도록 상한에서 잘리는지 검증한다.
func TestErrorMessageSummary_TruncatesLargePayload(t *testing.T) {
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"blob": strings.Repeat("x", errorMessageSummaryLimit*2),
	})))

	out := errorMessageSummary(msg)
	assert.True(t, strings.HasSuffix(out, "...(truncated)"), "잘림 표시가 있어야 한다")
	assert.LessOrEqual(t, len(out), errorMessageSummaryLimit+len("...(truncated)"))
}

// TestErrorMessageSummary_TruncationKeepsValidUTF8 는 멀티바이트 문자 중간에서
// 잘려 깨진 UTF-8 이 로그에 남지 않는지 검증한다 (한글 디바이스 이름 등).
func TestErrorMessageSummary_TruncationKeepsValidUTF8(t *testing.T) {
	// 한글로만 채워 상한 경계가 문자 중간에 걸리도록 만든다.
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"name": strings.Repeat("집", errorMessageSummaryLimit),
	})))

	out := errorMessageSummary(msg)
	assert.True(t, utf8.ValidString(out), "잘린 결과도 유효한 UTF-8 이어야 한다")
	assert.True(t, strings.HasSuffix(out, "...(truncated)"))
}

// TestErrorMessageSummary_EmptyMessage 는 빈 메시지도 패닉 없이 요약되는지 검증한다.
func TestErrorMessageSummary_EmptyMessage(t *testing.T) {
	out := errorMessageSummary(message.New())
	assert.True(t, json.Valid([]byte(out)), "유효한 JSON 이어야 한다")
}
