package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
)

// fakeAgentManager 는 restoreAgents 의 동작을 검증하기 위한 테스트용 가짜 매니저이다.
// Create 와 Start 호출 순서와 인자를 기록한다.
type fakeAgentManager struct {
	createCalls []agent.AgentConfig
	startCalls  []string

	// createErr 는 호출 시 반환할 에러 맵이다 (ID 기준). nil 이면 nil 반환.
	createErr map[string]error
	// startErr 는 호출 시 반환할 에러 맵이다 (ID 기준). nil 이면 nil 반환.
	startErr map[string]error
}

func newFakeAgentManager() *fakeAgentManager {
	return &fakeAgentManager{
		createErr: make(map[string]error),
		startErr:  make(map[string]error),
	}
}

func (f *fakeAgentManager) Create(cfg agent.AgentConfig) (agent.Agent, error) {
	f.createCalls = append(f.createCalls, cfg)
	if err, ok := f.createErr[cfg.ID]; ok {
		return nil, err
	}
	return nil, nil
}

func (f *fakeAgentManager) Start(_ context.Context, id string) error {
	f.startCalls = append(f.startCalls, id)
	if err, ok := f.startErr[id]; ok {
		return err
	}
	return nil
}

// discardLogger 는 테스트 출력에 로그가 나오지 않도록 io.Discard 로 가는 slog.Logger 를 반환한다.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// boolPtr 는 bool 포인터 헬퍼이다.
func boolPtr(b bool) *bool { return &b }

// makeConfig 는 유효한 AgentConfig 를 생성한다.
// Validate() 를 통과시키려면 ID, Name 이 필요하다.
func makeConfig(id, name string, enabled *bool) agent.AgentConfig {
	return agent.AgentConfig{
		ID:      id,
		Name:    name,
		Enabled: enabled,
	}
}

// TestRestoreAgents_EnabledNil_StartsAgent 는 Enabled 가 nil 인 경우 (기본 활성화)
// Create 와 Start 가 모두 호출되는지 검증한다.
func TestRestoreAgents_EnabledNil_StartsAgent(t *testing.T) {
	t.Parallel()
	fm := newFakeAgentManager()
	cfgs := []agent.AgentConfig{makeConfig("a1", "agent-1", nil)}

	restoreAgents(context.Background(), fm, cfgs, discardLogger())

	require.Len(t, fm.createCalls, 1, "Create 는 1회 호출되어야 한다")
	assert.Equal(t, "a1", fm.createCalls[0].ID)
	require.Len(t, fm.startCalls, 1, "Enabled=nil 이면 Start 가 호출되어야 한다")
	assert.Equal(t, "a1", fm.startCalls[0])
}

// TestRestoreAgents_EnabledTrue_StartsAgent 는 Enabled=true 인 경우
// Create 와 Start 가 모두 호출되는지 검증한다.
func TestRestoreAgents_EnabledTrue_StartsAgent(t *testing.T) {
	t.Parallel()
	fm := newFakeAgentManager()
	cfgs := []agent.AgentConfig{makeConfig("a1", "agent-1", boolPtr(true))}

	restoreAgents(context.Background(), fm, cfgs, discardLogger())

	require.Len(t, fm.createCalls, 1)
	require.Len(t, fm.startCalls, 1, "Enabled=true 이면 Start 가 호출되어야 한다")
	assert.Equal(t, "a1", fm.startCalls[0])
}

// TestRestoreAgents_EnabledFalse_SkipsStart 는 Enabled=false 인 경우
// Create 는 호출되지만 Start 는 건너뛰는지 검증한다 (SPEC-AGENT-005 핵심 동작).
func TestRestoreAgents_EnabledFalse_SkipsStart(t *testing.T) {
	t.Parallel()
	fm := newFakeAgentManager()
	cfgs := []agent.AgentConfig{makeConfig("a1", "agent-1", boolPtr(false))}

	restoreAgents(context.Background(), fm, cfgs, discardLogger())

	require.Len(t, fm.createCalls, 1, "disabled 에이전트도 Create 는 반드시 호출되어야 한다 (R2.4)")
	assert.Equal(t, "a1", fm.createCalls[0].ID)
	assert.Empty(t, fm.startCalls, "disabled 에이전트는 Start 가 호출되지 않아야 한다 (R2.2, R2.5)")
}

// TestRestoreAgents_MixedEnabledStates_StartsOnlyEnabled 는 enabled=true, enabled=false,
// enabled=nil 세 가지 상태를 혼합했을 때, Create 는 3회 모두 호출되고
// Start 는 enabled(nil 포함) 인 2건만 호출되는지 검증한다.
func TestRestoreAgents_MixedEnabledStates_StartsOnlyEnabled(t *testing.T) {
	t.Parallel()
	fm := newFakeAgentManager()
	cfgs := []agent.AgentConfig{
		makeConfig("a1", "enabled-explicit", boolPtr(true)),
		makeConfig("a2", "disabled", boolPtr(false)),
		makeConfig("a3", "enabled-default", nil),
	}

	restoreAgents(context.Background(), fm, cfgs, discardLogger())

	require.Len(t, fm.createCalls, 3, "모든 에이전트에 대해 Create 가 호출되어야 한다")
	assert.Equal(t, []string{"a1", "a2", "a3"}, []string{
		fm.createCalls[0].ID, fm.createCalls[1].ID, fm.createCalls[2].ID,
	})

	require.Len(t, fm.startCalls, 2, "enabled 에이전트 (true, nil) 만 Start 가 호출되어야 한다")
	assert.Equal(t, []string{"a1", "a3"}, fm.startCalls)
}

// TestRestoreAgents_CreateFailure_SkipsStart 는 Create 가 실패한 경우
// 해당 에이전트에 대한 Start 호출이 생략되는지 검증한다.
func TestRestoreAgents_CreateFailure_SkipsStart(t *testing.T) {
	t.Parallel()
	fm := newFakeAgentManager()
	fm.createErr["a1"] = errors.New("create failed")
	cfgs := []agent.AgentConfig{
		makeConfig("a1", "will-fail", boolPtr(true)),
		makeConfig("a2", "will-succeed", boolPtr(true)),
	}

	restoreAgents(context.Background(), fm, cfgs, discardLogger())

	require.Len(t, fm.createCalls, 2, "Create 는 모든 에이전트에 대해 시도되어야 한다")
	require.Len(t, fm.startCalls, 1, "Create 실패한 a1 은 Start 가 호출되지 않아야 한다")
	assert.Equal(t, "a2", fm.startCalls[0])
}

// TestRestoreAgents_StartFailure_ContinuesLoop 는 한 에이전트의 Start 가 실패해도
// 나머지 에이전트에 대한 복원 루프가 계속 진행되는지 검증한다.
func TestRestoreAgents_StartFailure_ContinuesLoop(t *testing.T) {
	t.Parallel()
	fm := newFakeAgentManager()
	fm.startErr["a1"] = errors.New("start failed")
	cfgs := []agent.AgentConfig{
		makeConfig("a1", "start-fails", boolPtr(true)),
		makeConfig("a2", "start-ok", boolPtr(true)),
	}

	restoreAgents(context.Background(), fm, cfgs, discardLogger())

	require.Len(t, fm.createCalls, 2)
	require.Len(t, fm.startCalls, 2, "Start 실패 후에도 다음 에이전트의 Start 가 호출되어야 한다")
	assert.Equal(t, []string{"a1", "a2"}, fm.startCalls)
}

// TestRestoreAgents_EmptyList_NoOp 는 빈 설정 리스트에 대해 아무 동작도 하지 않는지 검증한다.
func TestRestoreAgents_EmptyList_NoOp(t *testing.T) {
	t.Parallel()
	fm := newFakeAgentManager()

	restoreAgents(context.Background(), fm, nil, discardLogger())

	assert.Empty(t, fm.createCalls, "빈 리스트에서는 Create 가 호출되지 않아야 한다")
	assert.Empty(t, fm.startCalls, "빈 리스트에서는 Start 가 호출되지 않아야 한다")
}
