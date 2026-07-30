package xsfm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 통계 계측 배선 검증 (버그 수정: 이전에는 Incr*/Add* 호출이 0건이라 모든 카운터가 0).
// 각 메시지 경로가 대응하는 카운터를 실제로 증가시키는지 확인한다.

// (a) 외부 상태 유입 → 외부 수신/총 수신/BytesRead + LoadTime(첫 메시지) 계측.
func TestStats_ExternalReceiveOnStateIngest(t *testing.T) {
	ap, mock := directAgentWithMock(t, directOpts())
	require.NoError(t, ap.Start(context.Background()))

	// 직전 스냅샷은 모두 0.
	pre := ap.Stats()
	require.Zero(t, pre.MessagesReceived)
	require.Zero(t, pre.ExternalMessagesReceived)
	require.Zero(t, pre.BytesRead)

	payload := []byte(`{"power":true,"fan_speed":2}`)
	mock.deliver("xsfm/ap-101/state", payload)

	s := ap.Stats()
	assert.Greater(t, s.MessagesReceived, int64(0), "총 수신 카운터가 증가해야 한다")
	assert.Greater(t, s.ExternalMessagesReceived, int64(0), "외부 수신 카운터가 증가해야 한다")
	assert.Equal(t, int64(len(payload)), s.BytesRead, "BytesRead 는 페이로드 길이만큼 증가해야 한다")
	assert.Greater(t, s.LoadTime, time.Duration(0), "첫 메시지에서 LoadTime 이 산출되어야 한다")
	assert.False(t, s.LastActivityAt.IsZero(), "마지막 활동 시각이 갱신되어야 한다")
}

// (a') 디코드 실패 유입 → 외부 오류 카운터 증가.
func TestStats_ExternalErrorOnDecodeFailure(t *testing.T) {
	ap, mock := directAgentWithMock(t, directOpts())
	require.NoError(t, ap.Start(context.Background()))

	mock.deliver("xsfm/ap-101/state", []byte(`not json`))

	s := ap.Stats()
	assert.Greater(t, s.MessagesErrored, int64(0), "디코드 실패 시 총 오류 카운터가 증가해야 한다")
	assert.Greater(t, s.ExternalMessagesErrored, int64(0), "디코드 실패 시 외부 오류 카운터가 증가해야 한다")
}

// (b) 명령 발신(set_power) → 외부 발신/총 발신/BytesWritten 계측.
func TestStats_ExternalSendOnCommand(t *testing.T) {
	ap, _ := directAgentWithMock(t, oneDeviceOpts(nil))

	_, err := ap.Process([]byte(`{"command":"set_power","device_id":"ap-101","params":{"power":true}}`))
	require.NoError(t, err)

	s := ap.Stats()
	assert.Greater(t, s.MessagesSent, int64(0), "총 발신 카운터가 증가해야 한다")
	assert.Greater(t, s.ExternalMessagesSent, int64(0), "외부 발신 카운터가 증가해야 한다")
	assert.Greater(t, s.BytesWritten, int64(0), "BytesWritten 이 증가해야 한다")
}

// (c) 내부 경로: Process 진입 시 내부 수신, sendEvent enqueue 시 내부 발신.
func TestStats_InternalCountersOnProcessAndEmit(t *testing.T) {
	ap, _ := directAgentWithMock(t, directOpts())

	// add_device 는 Process(내부 수신) + device_registered 이벤트 방출(sendEvent → 내부 발신).
	_, err := ap.Process([]byte(`{"command":"add_device","device_id":"ap-101"}`))
	require.NoError(t, err)

	s := ap.Stats()
	assert.Greater(t, s.InternalMessagesReceived, int64(0), "Process 진입 시 내부 수신 카운터가 증가해야 한다")
	assert.Greater(t, s.InternalMessagesSent, int64(0), "이벤트 방출 시 내부 발신 카운터가 증가해야 한다")
}

// (d) msgCh 포화 시 드롭 카운터 증가.
func TestStats_DroppedOnFullChannel(t *testing.T) {
	ap, _ := directAgentWithMock(t, directOpts())

	// 채널을 용량까지 채운 뒤 추가 방출을 유도하면 default 분기(드롭)로 빠진다.
	for i := 0; i < cap(ap.msgCh); i++ {
		ap.msgCh <- []byte("x")
	}
	ap.sendEvent("device_state_changed", map[string]any{"device_id": "ap-101"})

	s := ap.Stats()
	assert.Greater(t, s.DroppedMessages, int64(0), "채널 포화 시 드롭 카운터가 증가해야 한다")
}
