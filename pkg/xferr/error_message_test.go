package xferr

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/message"
)

// TestNewErrorMessage_Valid 는 유효한 인자로 ErrorMessage를 생성하는 것을 검증한다.
func TestNewErrorMessage_Valid(t *testing.T) {
	msg := message.New()
	testErr := errors.New("test error")
	sourceNodeID := "node-1"

	em, err := NewErrorMessage(msg, testErr, sourceNodeID)
	require.NoError(t, err)
	require.NotNil(t, em)

	assert.Equal(t, msg, em.OriginalMessage(), "원본 메시지가 일치해야 한다")
	assert.Equal(t, testErr, em.Error(), "에러가 일치해야 한다")
	assert.Equal(t, sourceNodeID, em.SourceNodeID(), "소스 노드 ID가 일치해야 한다")
	assert.Equal(t, SeverityError, em.Severity(), "기본 심각도는 error여야 한다")
	assert.Equal(t, CategoryProcessing, em.Category(), "기본 카테고리는 processing이어야 한다")
	assert.Empty(t, em.StackContext(), "기본 스택 컨텍스트는 빈 문자열이어야 한다")
}

// TestNewErrorMessage_NilMessage 는 nil 메시지 전달 시 에러를 반환하는지 검증한다.
func TestNewErrorMessage_NilMessage(t *testing.T) {
	testErr := errors.New("test error")

	em, err := NewErrorMessage(nil, testErr, "node-1")
	assert.Nil(t, em, "nil 메시지인 경우 결과가 nil이어야 한다")
	assert.ErrorIs(t, err, ErrNilMessage, "ErrNilMessage 에러를 반환해야 한다")
}

// TestNewErrorMessage_NilError 는 nil 에러 전달 시 에러를 반환하는지 검증한다.
func TestNewErrorMessage_NilError(t *testing.T) {
	msg := message.New()

	em, err := NewErrorMessage(msg, nil, "node-1")
	assert.Nil(t, em, "nil 에러인 경우 결과가 nil이어야 한다")
	assert.ErrorIs(t, err, ErrNilError, "ErrNilError 에러를 반환해야 한다")
}

// TestNewErrorMessage_WithOptions 는 옵션을 사용한 ErrorMessage 생성을 검증한다.
func TestNewErrorMessage_WithOptions(t *testing.T) {
	msg := message.New()
	testErr := errors.New("test error")

	em, err := NewErrorMessage(msg, testErr, "node-1",
		WithSeverity(SeverityCritical),
		WithCategory(CategoryTimeout),
		WithStackContext("at handler.Process()"),
	)
	require.NoError(t, err)
	require.NotNil(t, em)

	assert.Equal(t, SeverityCritical, em.Severity(), "심각도가 critical이어야 한다")
	assert.Equal(t, CategoryTimeout, em.Category(), "카테고리가 timeout이어야 한다")
	assert.Equal(t, "at handler.Process()", em.StackContext(), "스택 컨텍스트가 일치해야 한다")
}

// TestNewErrorMessage_Timestamp 는 ErrorTimestamp가 생성 시점에 설정되는지 검증한다.
func TestNewErrorMessage_Timestamp(t *testing.T) {
	msg := message.New()
	testErr := errors.New("test error")

	before := time.Now()
	em, err := NewErrorMessage(msg, testErr, "node-1")
	after := time.Now()

	require.NoError(t, err)
	require.NotNil(t, em)

	ts := em.ErrorTimestamp()
	assert.False(t, ts.Before(before), "타임스탬프가 생성 전이면 안 된다")
	assert.False(t, ts.After(after), "타임스탬프가 생성 후이면 안 된다")
}

// TestNewErrorMessage_EmptySourceNodeID 는 빈 소스 노드 ID도 허용하는지 검증한다.
func TestNewErrorMessage_EmptySourceNodeID(t *testing.T) {
	msg := message.New()
	testErr := errors.New("test error")

	em, err := NewErrorMessage(msg, testErr, "")
	require.NoError(t, err)
	require.NotNil(t, em)

	assert.Empty(t, em.SourceNodeID(), "빈 소스 노드 ID가 허용되어야 한다")
}
