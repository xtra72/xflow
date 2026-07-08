package engine

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// SPEC-AGENT-005: 비활성화된 에이전트(Enabled=false)는 플로우가 참조하더라도
// autoStartAgents 가 Start() 를 호출해서는 안 된다. 부팅 시 restoreAgents 와
// 동일하게 사용자의 "실행 금지" 의도를 존중해야 한다.

// fakeStartAgent 는 agent.Agent 인터페이스의 테스트용 fake 구현이다.
// Start() 호출 횟수를 기록하고, 지정한 활성화 상태(enabled)를 Info().Config 로 노출한다.
type fakeStartAgent struct {
	id         string
	name       string
	enabled    *bool // nil = 기본 활성화
	startCalls atomic.Int32
	stopCalls  atomic.Int32
}

func newFakeStartAgent(id, name string, enabled *bool) *fakeStartAgent {
	return &fakeStartAgent{id: id, name: name, enabled: enabled}
}

func (f *fakeStartAgent) Init(_ agent.AgentConfig) error      { return nil }
func (f *fakeStartAgent) Start(_ context.Context) error       { f.startCalls.Add(1); return nil }
func (f *fakeStartAgent) Stop(_ context.Context) error        { f.stopCalls.Add(1); return nil }
func (f *fakeStartAgent) Pause(_ context.Context) error       { return nil }
func (f *fakeStartAgent) Resume(_ context.Context) error      { return nil }
func (f *fakeStartAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (f *fakeStartAgent) Process(d []byte) ([]byte, error)    { return d, nil }
func (f *fakeStartAgent) Configure(_ agent.AgentConfig) error { return nil }
func (f *fakeStartAgent) ID() string                          { return f.id }
func (f *fakeStartAgent) Name() string                        { return f.name }
func (f *fakeStartAgent) Type() string                        { return "custom" }
func (f *fakeStartAgent) Info() agent.AgentInfo {
	return agent.AgentInfo{
		ID:    f.id,
		Name:  f.name,
		Type:  "custom",
		State: lifecycle.StateCreated,
		Config: agent.AgentConfig{
			ID:      f.id,
			Name:    f.name,
			Type:    "custom",
			Enabled: f.enabled,
		},
	}
}
func (f *fakeStartAgent) Stats() agent.StatsSnapshot { return agent.StatsSnapshot{} }

var _ agent.Agent = (*fakeStartAgent)(nil)

// fakeConnectedNode 는 mockNode 를 임베딩하고 connectedAgentProvider 를
// 구현하여, autoStartAgents 가 대상으로 삼는 브릿지 노드를 흉내낸다.
type fakeConnectedNode struct {
	*mockNode
	connected agent.Agent
}

func newFakeConnectedNode(id, name string, ag agent.Agent) *fakeConnectedNode {
	return &fakeConnectedNode{
		mockNode:  newMockNode(id, name, "bridge"),
		connected: ag,
	}
}

// ConnectedAgent 는 connectedAgentProvider 인터페이스 구현이다.
func (n *fakeConnectedNode) ConnectedAgent() agent.Agent { return n.connected }

var _ connectedAgentProvider = (*fakeConnectedNode)(nil)

// newRuntimeWithNodes 는 주어진 노드들로 최소한의 flowRuntime 을 구성한다.
func newRuntimeWithNodes(nodes ...*fakeConnectedNode) *flowRuntime {
	m := make(map[string]node.Node, len(nodes))
	for _, n := range nodes {
		m[n.ID()] = n
	}
	return &flowRuntime{nodes: m}
}

// TestAutoStartAgents_SkipsDisabledAgent 는 비활성화(Enabled=false)된 에이전트에
// 대해 Start() 가 호출되지 않고 반환 슬라이스에도 포함되지 않음을 검증한다.
// 수정 전(RED): Start 가 호출되고 슬라이스에 포함된다.
func TestAutoStartAgents_SkipsDisabledAgent(t *testing.T) {
	disabled := newFakeStartAgent("agent-disabled", "disabled-agent", boolPtr(false))
	rt := newRuntimeWithNodes(newFakeConnectedNode("node-1", "bridge-1", disabled))

	e := &Engine{}
	started := e.autoStartAgents(context.Background(), rt)

	assert.Equal(t, int32(0), disabled.startCalls.Load(),
		"비활성화된 에이전트에 Start() 를 호출하면 안 된다")
	assert.NotContains(t, started, agent.Agent(disabled),
		"비활성화된 에이전트는 autoStarted 슬라이스에 포함되면 안 된다")
	assert.Empty(t, started, "반환 슬라이스는 비어 있어야 한다")
}

// TestAutoStartAgents_StartsEnabledAgent 는 활성화된 에이전트(Enabled nil/true)에
// 대해 Start() 가 호출되고 반환 슬라이스에 포함됨을 검증한다 (회귀 방지).
func TestAutoStartAgents_StartsEnabledAgent(t *testing.T) {
	tests := []struct {
		name    string
		enabled *bool
	}{
		{name: "Enabled nil (기본 활성화)", enabled: nil},
		{name: "Enabled true (명시적 활성화)", enabled: boolPtr(true)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ag := newFakeStartAgent("agent-enabled", "enabled-agent", tc.enabled)
			rt := newRuntimeWithNodes(newFakeConnectedNode("node-1", "bridge-1", ag))

			e := &Engine{}
			started := e.autoStartAgents(context.Background(), rt)

			assert.Equal(t, int32(1), ag.startCalls.Load(),
				"활성화된 에이전트에는 Start() 가 호출되어야 한다")
			require.Len(t, started, 1)
			assert.Same(t, ag, started[0].(*fakeStartAgent))
		})
	}
}

// TestAutoStartAgents_MixedEnabledDisabled 는 활성화/비활성화 에이전트가 혼재된
// 경우 활성화된 에이전트만 시작됨을 검증한다.
func TestAutoStartAgents_MixedEnabledDisabled(t *testing.T) {
	enabled := newFakeStartAgent("agent-enabled", "enabled-agent", boolPtr(true))
	disabled := newFakeStartAgent("agent-disabled", "disabled-agent", boolPtr(false))
	rt := newRuntimeWithNodes(
		newFakeConnectedNode("node-1", "bridge-1", enabled),
		newFakeConnectedNode("node-2", "bridge-2", disabled),
	)

	e := &Engine{}
	started := e.autoStartAgents(context.Background(), rt)

	assert.Equal(t, int32(1), enabled.startCalls.Load(),
		"활성화된 에이전트는 시작되어야 한다")
	assert.Equal(t, int32(0), disabled.startCalls.Load(),
		"비활성화된 에이전트는 시작되면 안 된다")
	require.Len(t, started, 1, "활성화된 에이전트 하나만 반환되어야 한다")
	assert.Same(t, enabled, started[0].(*fakeStartAgent))
}
