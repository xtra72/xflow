package xferr

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// TestNewStatusEvent_Valid 는 유효한 인자로 StatusEvent를 생성하는 것을 검증한다.
func TestNewStatusEvent_Valid(t *testing.T) {
	evt, err := NewStatusEvent(ComponentNode, "node-1", lifecycle.StateCreated, lifecycle.StateRunning)
	require.NoError(t, err)
	require.NotNil(t, evt)

	assert.Equal(t, ComponentNode, evt.ComponentType(), "컴포넌트 타입이 일치해야 한다")
	assert.Equal(t, "node-1", evt.ComponentID(), "컴포넌트 ID가 일치해야 한다")
	assert.Equal(t, lifecycle.StateCreated, evt.PreviousState(), "이전 상태가 일치해야 한다")
	assert.Equal(t, lifecycle.StateRunning, evt.NewState(), "새 상태가 일치해야 한다")
	assert.NotNil(t, evt.Info(), "기본 Info는 nil이 아닌 빈 맵이어야 한다")
	assert.Empty(t, evt.Info(), "기본 Info는 빈 맵이어야 한다")
}

// TestNewStatusEvent_SameState 는 동일한 상태 전이 시 에러를 반환하는지 검증한다.
func TestNewStatusEvent_SameState(t *testing.T) {
	evt, err := NewStatusEvent(ComponentNode, "node-1", lifecycle.StateRunning, lifecycle.StateRunning)
	assert.Nil(t, evt, "동일한 상태 전이인 경우 결과가 nil이어야 한다")
	assert.ErrorIs(t, err, ErrSameStateTransition, "ErrSameStateTransition 에러를 반환해야 한다")
}

// TestNewStatusEvent_InvalidComponentType 는 유효하지 않은 컴포넌트 타입 시 에러를 반환하는지 검증한다.
func TestNewStatusEvent_InvalidComponentType(t *testing.T) {
	evt, err := NewStatusEvent(ComponentType("invalid"), "comp-1", lifecycle.StateCreated, lifecycle.StateRunning)
	assert.Nil(t, evt, "유효하지 않은 컴포넌트 타입인 경우 결과가 nil이어야 한다")
	assert.ErrorIs(t, err, ErrInvalidComponentType, "ErrInvalidComponentType 에러를 반환해야 한다")
}

// TestNewStatusEvent_WithInfo 는 WithInfo 옵션을 사용한 StatusEvent 생성을 검증한다.
func TestNewStatusEvent_WithInfo(t *testing.T) {
	info := map[string]string{"reason": "manual_stop", "operator": "admin"}

	evt, err := NewStatusEvent(ComponentFlow, "flow-1",
		lifecycle.StateRunning, lifecycle.StateStopped,
		WithInfo(info),
	)
	require.NoError(t, err)
	require.NotNil(t, evt)

	assert.Equal(t, info, evt.Info(), "Info가 일치해야 한다")
}

// TestNewStatusEvent_Timestamp 는 Timestamp가 생성 시점에 설정되는지 검증한다.
func TestNewStatusEvent_Timestamp(t *testing.T) {
	before := time.Now()
	evt, err := NewStatusEvent(ComponentAgent, "agent-1", lifecycle.StatePaused, lifecycle.StateRunning)
	after := time.Now()

	require.NoError(t, err)
	require.NotNil(t, evt)

	ts := evt.Timestamp()
	assert.False(t, ts.Before(before), "타임스탬프가 생성 전이면 안 된다")
	assert.False(t, ts.After(after), "타임스탬프가 생성 후이면 안 된다")
}

// TestNewStatusEvent_InfoDefensiveCopy 는 Info 맵이 방어적으로 복사되는지 검증한다.
func TestNewStatusEvent_InfoDefensiveCopy(t *testing.T) {
	originalInfo := map[string]string{"key": "value"}

	evt, err := NewStatusEvent(ComponentPlugin, "plugin-1",
		lifecycle.StateCreated, lifecycle.StateInitializing,
		WithInfo(originalInfo),
	)
	require.NoError(t, err)
	require.NotNil(t, evt)

	// 원본 맵 수정
	originalInfo["key"] = "modified"
	originalInfo["new_key"] = "new_value"

	// StatusEvent의 Info는 영향을 받지 않아야 한다
	assert.Equal(t, "value", evt.Info()["key"],
		"원본 맵 수정이 StatusEvent의 Info에 영향을 주면 안 된다")
	assert.NotContains(t, evt.Info(), "new_key",
		"원본 맵에 추가된 키가 StatusEvent의 Info에 나타나면 안 된다")

	// 반환된 맵 수정도 내부 상태에 영향을 주면 안 된다
	returnedInfo := evt.Info()
	returnedInfo["injected"] = "injected_value"
	assert.NotContains(t, evt.Info(), "injected",
		"반환된 맵 수정이 내부 상태에 영향을 주면 안 된다")
}

// TestNewStatusEvent_AllComponentTypes 는 모든 유효한 컴포넌트 타입으로 생성 가능한지 검증한다.
func TestNewStatusEvent_AllComponentTypes(t *testing.T) {
	types := []ComponentType{
		ComponentFlow,
		ComponentNode,
		ComponentAgent,
		ComponentScriptEngine,
		ComponentPlugin,
	}

	for _, ct := range types {
		t.Run(string(ct), func(t *testing.T) {
			evt, err := NewStatusEvent(ct, "comp-1", lifecycle.StateCreated, lifecycle.StateRunning)
			require.NoError(t, err)
			require.NotNil(t, evt)
			assert.Equal(t, ct, evt.ComponentType())
		})
	}
}
