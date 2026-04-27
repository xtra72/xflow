package xferr

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/message"
)

// TestNewDeadLetterMessage_Valid 는 유효한 인자로 DeadLetterMessage를 생성하는 것을 검증한다.
func TestNewDeadLetterMessage_Valid(t *testing.T) {
	msg := message.New()
	reason := ReasonTTLExpired

	dlm, err := NewDeadLetterMessage(msg, reason)
	require.NoError(t, err)
	require.NotNil(t, dlm)

	assert.Equal(t, msg, dlm.OriginalMessage(), "원본 메시지가 일치해야 한다")
	assert.Equal(t, reason, dlm.Reason(), "폐기 사유가 일치해야 한다")
	assert.Empty(t, dlm.SourceNodeID(), "기본 소스 노드 ID는 빈 문자열이어야 한다")
	assert.Empty(t, dlm.SourceWireID(), "기본 소스 와이어 ID는 빈 문자열이어야 한다")
	assert.NotNil(t, dlm.Context(), "기본 컨텍스트는 nil이 아닌 빈 맵이어야 한다")
	assert.Empty(t, dlm.Context(), "기본 컨텍스트는 빈 맵이어야 한다")
}

// TestNewDeadLetterMessage_NilMessage 는 nil 메시지 전달 시 에러를 반환하는지 검증한다.
func TestNewDeadLetterMessage_NilMessage(t *testing.T) {
	dlm, err := NewDeadLetterMessage(nil, ReasonTTLExpired)
	assert.Nil(t, dlm, "nil 메시지인 경우 결과가 nil이어야 한다")
	assert.ErrorIs(t, err, ErrNilMessage, "ErrNilMessage 에러를 반환해야 한다")
}

// TestNewDeadLetterMessage_InvalidReason 은 유효하지 않은 폐기 사유 전달 시 에러를 반환하는지 검증한다.
func TestNewDeadLetterMessage_InvalidReason(t *testing.T) {
	msg := message.New()

	dlm, err := NewDeadLetterMessage(msg, DropReason("invalid"))
	assert.Nil(t, dlm, "유효하지 않은 사유인 경우 결과가 nil이어야 한다")
	assert.ErrorIs(t, err, ErrInvalidDropReason, "ErrInvalidDropReason 에러를 반환해야 한다")
}

// TestNewDeadLetterMessage_WithOptions 는 옵션을 사용한 DeadLetterMessage 생성을 검증한다.
func TestNewDeadLetterMessage_WithOptions(t *testing.T) {
	msg := message.New()
	ctx := map[string]string{"key": "value", "retry_count": "3"}

	dlm, err := NewDeadLetterMessage(msg, ReasonMaxRetriesExceeded,
		WithDLSourceNodeID("node-1"),
		WithDLSourceWireID("wire-1"),
		WithDLContext(ctx),
	)
	require.NoError(t, err)
	require.NotNil(t, dlm)

	assert.Equal(t, "node-1", dlm.SourceNodeID(), "소스 노드 ID가 일치해야 한다")
	assert.Equal(t, "wire-1", dlm.SourceWireID(), "소스 와이어 ID가 일치해야 한다")
	assert.Equal(t, ctx, dlm.Context(), "컨텍스트가 일치해야 한다")
}

// TestNewDeadLetterMessage_Timestamp 는 DropTimestamp가 생성 시점에 설정되는지 검증한다.
func TestNewDeadLetterMessage_Timestamp(t *testing.T) {
	msg := message.New()

	before := time.Now()
	dlm, err := NewDeadLetterMessage(msg, ReasonChannelFull)
	after := time.Now()

	require.NoError(t, err)
	require.NotNil(t, dlm)

	ts := dlm.DropTimestamp()
	assert.False(t, ts.Before(before), "타임스탬프가 생성 전이면 안 된다")
	assert.False(t, ts.After(after), "타임스탬프가 생성 후이면 안 된다")
}

// TestNewDeadLetterMessage_ContextDefensiveCopy 는 Context 맵이 방어적으로 복사되는지 검증한다.
func TestNewDeadLetterMessage_ContextDefensiveCopy(t *testing.T) {
	msg := message.New()
	originalCtx := map[string]string{"key": "value"}

	dlm, err := NewDeadLetterMessage(msg, ReasonFilterRejected,
		WithDLContext(originalCtx),
	)
	require.NoError(t, err)
	require.NotNil(t, dlm)

	// 원본 맵 수정
	originalCtx["key"] = "modified"
	originalCtx["new_key"] = "new_value"

	// DeadLetterMessage의 Context는 영향을 받지 않아야 한다
	assert.Equal(t, "value", dlm.Context()["key"],
		"원본 맵 수정이 DeadLetterMessage의 컨텍스트에 영향을 주면 안 된다")
	assert.NotContains(t, dlm.Context(), "new_key",
		"원본 맵에 추가된 키가 DeadLetterMessage의 컨텍스트에 나타나면 안 된다")

	// 반환된 맵 수정도 내부 상태에 영향을 주면 안 된다
	returnedCtx := dlm.Context()
	returnedCtx["injected"] = "injected_value"
	assert.NotContains(t, dlm.Context(), "injected",
		"반환된 맵 수정이 내부 상태에 영향을 주면 안 된다")
}

// TestNewDeadLetterMessage_AllReasons 는 모든 유효한 폐기 사유로 생성 가능한지 검증한다.
func TestNewDeadLetterMessage_AllReasons(t *testing.T) {
	reasons := []DropReason{
		ReasonTTLExpired,
		ReasonBackpressureDrop,
		ReasonFilterRejected,
		ReasonMaxRetriesExceeded,
		ReasonNodeStopped,
		ReasonChannelFull,
	}

	for _, reason := range reasons {
		t.Run(string(reason), func(t *testing.T) {
			msg := message.New()
			dlm, err := NewDeadLetterMessage(msg, reason)
			require.NoError(t, err)
			require.NotNil(t, dlm)
			assert.Equal(t, reason, dlm.Reason())
		})
	}
}
