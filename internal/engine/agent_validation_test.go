package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
)

// TestDeployFlow_AgentRefValidation_매니저없음_스킵 은 agentManager가 설정되지 않으면 검증을 건너뛰는지 확인한다.
func TestDeployFlow_AgentRefValidation_매니저없음_스킵(t *testing.T) {
	factory := newMockNodeFactory()
	e := newTestEngine(factory)

	// AgentRef가 설정된 노드를 포함한 플로우
	nodes := []flow.NodeDef{
		flow.NewNodeDef("A", "transform"),
	}
	nodes[0].AgentRef = &flow.AgentRef{AgentID: "ghost-agent", AgentName: "ghost"}
	f := flow.NewFlow("test-flow", flow.WithNodes(nodes...))

	// agentManager 미설정 → 검증 스킵 → 배포 성공
	err := e.DeployFlow(context.Background(), f)
	require.NoError(t, err, "agentManager가 nil이면 AgentRef 검증을 건너뛰어야 함")
}

// TestDeployFlow_AgentRefValidation_유효한참조_성공 은 유효한 AgentRef가 검증을 통과하는지 확인한다.
func TestDeployFlow_AgentRefValidation_유효한참조_성공(t *testing.T) {
	mgr := agent.NewManager()
	ag, err := mgr.Create(agent.AgentConfig{
		ID:   "agent-001",
		Name: "test-agent",
	})
	require.NoError(t, err)

	factory := newMockNodeFactory()
	registry := node.NewRegistry(node.WithoutBuiltins())
	_ = registry.Register("transform", factory.factory)
	e := NewEngine(
		WithNodeRegistry(registry),
		WithAgentManager(mgr),
	)

	nodes := []flow.NodeDef{
		flow.NewNodeDef("A", "transform"),
	}
	nodes[0].AgentRef = &flow.AgentRef{AgentID: ag.ID(), AgentName: ag.Name()}
	f := flow.NewFlow("test-flow", flow.WithNodes(nodes...))

	err = e.DeployFlow(context.Background(), f)
	assert.NoError(t, err)
}

// TestDeployFlow_AgentRefValidation_누락된참조_에러 는 누락된 AgentRef가 명확한 에러를 반환하는지 확인한다.
func TestDeployFlow_AgentRefValidation_누락된참조_에러(t *testing.T) {
	mgr := agent.NewManager()
	_, err := mgr.Create(agent.AgentConfig{
		ID:   "available-001",
		Name: "available-agent",
	})
	require.NoError(t, err)

	factory := newMockNodeFactory()
	registry := node.NewRegistry(node.WithoutBuiltins())
	_ = registry.Register("transform", factory.factory)
	e := NewEngine(
		WithNodeRegistry(registry),
		WithAgentManager(mgr),
	)

	nodes := []flow.NodeDef{
		flow.NewNodeDef("serial-receiver", "transform"),
	}
	nodes[0].AgentRef = &flow.AgentRef{
		AgentID:   "f723d2a4-f7ec-4a8a-b9b0-fc60cf77fcdb",
		AgentName: "f723d2a4-f7ec-4a8a-b9b0-fc60cf77fcdb",
	}
	f := flow.NewFlow("test-flow", flow.WithNodes(nodes...))

	err = e.DeployFlow(context.Background(), f)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentRefNotFound)

	// 에러 메시지 검증
	msg := err.Error()
	assert.Contains(t, msg, "serial-receiver", "노드 이름이 포함되어야 함")
	assert.Contains(t, msg, "f723d2a4", "참조된 agent ID가 포함되어야 함")
	assert.Contains(t, msg, "available-agent", "사용 가능한 에이전트 목록이 포함되어야 함")
}

// TestDeployFlow_AgentRefValidation_이름으로폴백_성공 은 ID가 다르더라도 이름으로 폴백되는지 확인한다.
func TestDeployFlow_AgentRefValidation_이름으로폴백_성공(t *testing.T) {
	mgr := agent.NewManager()
	_, err := mgr.Create(agent.AgentConfig{
		ID:   "real-id-123",
		Name: "my-agent",
	})
	require.NoError(t, err)

	factory := newMockNodeFactory()
	registry := node.NewRegistry(node.WithoutBuiltins())
	_ = registry.Register("transform", factory.factory)
	e := NewEngine(
		WithNodeRegistry(registry),
		WithAgentManager(mgr),
	)

	nodes := []flow.NodeDef{
		flow.NewNodeDef("A", "transform"),
	}
	// ID는 잘못되었지만 이름은 정확 → 폴백으로 찾음
	nodes[0].AgentRef = &flow.AgentRef{AgentID: "wrong-id", AgentName: "my-agent"}
	f := flow.NewFlow("test-flow", flow.WithNodes(nodes...))

	err = e.DeployFlow(context.Background(), f)
	assert.NoError(t, err)
}

// TestDeployFlow_AgentRefValidation_에이전트없음 는 매니저가 비어있을 때 에러 메시지를 확인한다.
func TestDeployFlow_AgentRefValidation_에이전트없음(t *testing.T) {
	mgr := agent.NewManager()

	factory := newMockNodeFactory()
	registry := node.NewRegistry(node.WithoutBuiltins())
	_ = registry.Register("transform", factory.factory)
	e := NewEngine(
		WithNodeRegistry(registry),
		WithAgentManager(mgr),
	)

	nodes := []flow.NodeDef{
		flow.NewNodeDef("A", "transform"),
	}
	nodes[0].AgentRef = &flow.AgentRef{AgentID: "ghost", AgentName: "ghost"}
	f := flow.NewFlow("test-flow", flow.WithNodes(nodes...))

	err := e.DeployFlow(context.Background(), f)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentRefNotFound)
	assert.Contains(t, err.Error(), "no agents are currently registered")
}

// TestDeployFlow_AgentRefValidation_여러누락 은 여러 노드가 누락된 경우 카운트가 포함되는지 확인한다.
func TestDeployFlow_AgentRefValidation_여러누락(t *testing.T) {
	mgr := agent.NewManager()

	factory := newMockNodeFactory()
	registry := node.NewRegistry(node.WithoutBuiltins())
	_ = registry.Register("transform", factory.factory)
	e := NewEngine(
		WithNodeRegistry(registry),
		WithAgentManager(mgr),
	)

	nodes := []flow.NodeDef{
		flow.NewNodeDef("A", "transform"),
		flow.NewNodeDef("B", "transform"),
		flow.NewNodeDef("C", "transform"),
	}
	nodes[0].AgentRef = &flow.AgentRef{AgentID: "ghost1", AgentName: "ghost1"}
	nodes[1].AgentRef = &flow.AgentRef{AgentID: "ghost2", AgentName: "ghost2"}
	nodes[2].AgentRef = &flow.AgentRef{AgentID: "ghost3", AgentName: "ghost3"}

	f := flow.NewFlow("test-flow", flow.WithNodes(nodes...))

	err := e.DeployFlow(context.Background(), f)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentRefNotFound)
	// 첫 번째 + 추가 N개 메시지
	assert.Contains(t, err.Error(), "and 2 more")
}

// TestAgentRefExists_ID로검색 은 agentRefExists가 ID로 에이전트를 찾는지 확인한다.
func TestAgentRefExists_ID로검색(t *testing.T) {
	mgr := agent.NewManager()
	ag, err := mgr.Create(agent.AgentConfig{
		ID:   "test-id",
		Name: "test-name",
	})
	require.NoError(t, err)

	// ID 일치
	assert.True(t, agentRefExists(mgr, flow.AgentRef{AgentID: ag.ID()}))
	// ID 불일치
	assert.False(t, agentRefExists(mgr, flow.AgentRef{AgentID: "wrong-id"}))
}

// TestAgentRefExists_이름으로검색 은 ID가 없어도 이름으로 찾는지 확인한다.
func TestAgentRefExists_이름으로검색(t *testing.T) {
	mgr := agent.NewManager()
	_, err := mgr.Create(agent.AgentConfig{
		ID:   "real-id",
		Name: "real-name",
	})
	require.NoError(t, err)

	// ID 없음, 이름만
	assert.True(t, agentRefExists(mgr, flow.AgentRef{AgentName: "real-name"}))
	// 둘 다 틀림
	assert.False(t, agentRefExists(mgr, flow.AgentRef{AgentID: "wrong", AgentName: "wrong"}))
}

// TestFormatAgentRefLabel 은 에이전트 참조 레이블 포맷팅을 확인한다.
func TestFormatAgentRefLabel(t *testing.T) {
	tests := []struct {
		name string
		ref  flow.AgentRef
		want string
	}{
		{"이름과ID다름", flow.AgentRef{AgentID: "abc", AgentName: "my-agent"}, `"my-agent" (id=abc)`},
		{"이름만", flow.AgentRef{AgentName: "my-agent"}, `"my-agent"`},
		{"ID만", flow.AgentRef{AgentID: "abc"}, `(id=abc)`},
		{"이름과ID같음", flow.AgentRef{AgentID: "same", AgentName: "same"}, `"same"`},
		{"둘다빈값", flow.AgentRef{}, `<empty>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatAgentRefLabel(tt.ref)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestValidateAgentRefs_노드에_ref없음 은 AgentRef가 nil인 노드는 검증 대상이 아님을 확인한다.
func TestValidateAgentRefs_노드에_ref없음(t *testing.T) {
	mgr := agent.NewManager()

	factory := newMockNodeFactory()
	registry := node.NewRegistry(node.WithoutBuiltins())
	_ = registry.Register("transform", factory.factory)
	e := NewEngine(
		WithNodeRegistry(registry),
		WithAgentManager(mgr),
	)

	// 모든 노드가 AgentRef 없음
	f, _ := newSimpleFlow()

	err := e.DeployFlow(context.Background(), f)
	assert.NoError(t, err, "AgentRef가 없는 노드는 검증 대상이 아니어야 함")
}

