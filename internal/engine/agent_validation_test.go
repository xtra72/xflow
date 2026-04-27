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

// --- Phase 4 (SPEC-AGENT-005): Enable/Disable 검증 테스트 ---
//
// 이 테스트들은 validateAgentRefs 가 비활성화(disabled) 에이전트를 거부하는 동작을 검증한다.
// 설계 요구사항:
//   - R5.1/R5.2: DeployFlow 호출 시 disabled 에이전트를 거부해야 한다.
//   - R5.3: 에러 메시지는 에이전트 ID, 참조 노드, Enable API 안내를 포함해야 한다.
//   - R5.6: 다중 disabled 및 missing 을 early-return 대신 누적하여 한 번에 보고해야 한다.
//   - R7.1: Enabled == nil 은 기본 true 로 취급하여 하위 호환성을 유지해야 한다.

// boolPtr 는 테스트용 *bool 리터럴 헬퍼이다.
func boolPtr(b bool) *bool { return &b }

// newEngineWithAgentManager 는 Enable/Disable 테스트용 엔진과 매니저를 구성한다.
func newEngineWithAgentManager(t *testing.T) (*Engine, agent.Manager) {
	t.Helper()
	mgr := agent.NewManager()
	factory := newMockNodeFactory()
	registry := node.NewRegistry(node.WithoutBuiltins())
	_ = registry.Register("transform", factory.factory)
	e := NewEngine(
		WithNodeRegistry(registry),
		WithAgentManager(mgr),
	)
	return e, mgr
}

// TestValidateAgentRefs_AllEnabled_ReturnsNil 은 모든 참조된 에이전트가 활성화 상태일 때
// 검증이 성공해야 함을 확인한다 (characterization 보완).
func TestValidateAgentRefs_AllEnabled_ReturnsNil(t *testing.T) {
	t.Parallel()

	mgr := agent.NewManager()
	ag1, err := mgr.Create(agent.AgentConfig{
		ID:      "agent-enabled-1",
		Name:    "agent-enabled-1",
		Enabled: boolPtr(true),
	})
	require.NoError(t, err)
	ag2, err := mgr.Create(agent.AgentConfig{
		ID:      "agent-enabled-2",
		Name:    "agent-enabled-2",
		Enabled: boolPtr(true),
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
		flow.NewNodeDef("B", "transform"),
	}
	nodes[0].AgentRef = &flow.AgentRef{AgentID: ag1.ID(), AgentName: ag1.Name()}
	nodes[1].AgentRef = &flow.AgentRef{AgentID: ag2.ID(), AgentName: ag2.Name()}
	f := flow.NewFlow("test-flow", flow.WithNodes(nodes...))

	err = e.DeployFlow(context.Background(), f)
	assert.NoError(t, err)
}

// TestValidateAgentRefs_OneDisabled_ReturnsErrAgentDisabled 는 단일 disabled 에이전트가
// ErrAgentDisabled 로 감지되는지 확인한다 (R5.1).
func TestValidateAgentRefs_OneDisabled_ReturnsErrAgentDisabled(t *testing.T) {
	t.Parallel()

	e, mgr := newEngineWithAgentManager(t)
	ag, err := mgr.Create(agent.AgentConfig{
		ID:      "agent-disabled",
		Name:    "my-disabled-agent",
		Enabled: boolPtr(false),
	})
	require.NoError(t, err)

	nodes := []flow.NodeDef{flow.NewNodeDef("node-A", "transform")}
	nodes[0].AgentRef = &flow.AgentRef{AgentID: ag.ID(), AgentName: ag.Name()}
	f := flow.NewFlow("test-flow", flow.WithNodes(nodes...))

	err = e.DeployFlow(context.Background(), f)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentDisabled, "disabled 에이전트는 ErrAgentDisabled 로 감지되어야 함")
	assert.NotErrorIs(t, err, ErrAgentRefNotFound, "존재하는 에이전트이므로 missing 에러는 아니어야 함")
}

// TestValidateAgentRefs_MultipleDisabled_AllReported 는 여러 disabled 에이전트가
// 한 번의 검증에서 모두 보고되는지 확인한다 (R5.6).
func TestValidateAgentRefs_MultipleDisabled_AllReported(t *testing.T) {
	t.Parallel()

	e, mgr := newEngineWithAgentManager(t)
	_, err := mgr.Create(agent.AgentConfig{
		ID:      "disabled-001",
		Name:    "disabled-alpha",
		Enabled: boolPtr(false),
	})
	require.NoError(t, err)
	_, err = mgr.Create(agent.AgentConfig{
		ID:      "disabled-002",
		Name:    "disabled-beta",
		Enabled: boolPtr(false),
	})
	require.NoError(t, err)

	nodes := []flow.NodeDef{
		flow.NewNodeDef("node-A", "transform"),
		flow.NewNodeDef("node-B", "transform"),
	}
	nodes[0].AgentRef = &flow.AgentRef{AgentID: "disabled-001", AgentName: "disabled-alpha"}
	nodes[1].AgentRef = &flow.AgentRef{AgentID: "disabled-002", AgentName: "disabled-beta"}
	f := flow.NewFlow("test-flow", flow.WithNodes(nodes...))

	err = e.DeployFlow(context.Background(), f)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAgentDisabled)

	msg := err.Error()
	// 두 노드 모두 메시지에 포함되어야 함
	assert.Contains(t, msg, "node-A", "첫 번째 노드가 메시지에 포함되어야 함")
	assert.Contains(t, msg, "node-B", "두 번째 노드가 메시지에 포함되어야 함")
	assert.Contains(t, msg, "disabled-001", "첫 번째 agent ID 가 포함되어야 함")
	assert.Contains(t, msg, "disabled-002", "두 번째 agent ID 가 포함되어야 함")
}

// TestValidateAgentRefs_MissingAndDisabled_BothReported 는 missing + disabled 가 동시에 존재할 때
// errors.Is 로 양쪽 모두 감지 가능함을 확인한다 (R5.6, errors.Join 동작).
func TestValidateAgentRefs_MissingAndDisabled_BothReported(t *testing.T) {
	t.Parallel()

	e, mgr := newEngineWithAgentManager(t)
	_, err := mgr.Create(agent.AgentConfig{
		ID:      "disabled-x",
		Name:    "disabled-x",
		Enabled: boolPtr(false),
	})
	require.NoError(t, err)

	nodes := []flow.NodeDef{
		flow.NewNodeDef("missing-node", "transform"),
		flow.NewNodeDef("disabled-node", "transform"),
	}
	nodes[0].AgentRef = &flow.AgentRef{AgentID: "ghost-agent", AgentName: "ghost-agent"}
	nodes[1].AgentRef = &flow.AgentRef{AgentID: "disabled-x", AgentName: "disabled-x"}
	f := flow.NewFlow("test-flow", flow.WithNodes(nodes...))

	err = e.DeployFlow(context.Background(), f)
	require.Error(t, err)
	// errors.Join 으로 묶였으므로 errors.Is 는 양쪽 모두에 대해 true 여야 함
	assert.ErrorIs(t, err, ErrAgentRefNotFound, "missing 에이전트가 감지되어야 함")
	assert.ErrorIs(t, err, ErrAgentDisabled, "disabled 에이전트가 감지되어야 함")

	msg := err.Error()
	assert.Contains(t, msg, "missing-node", "missing 노드 이름이 메시지에 포함되어야 함")
	assert.Contains(t, msg, "disabled-node", "disabled 노드 이름이 메시지에 포함되어야 함")
	assert.Contains(t, msg, "ghost-agent", "missing agent 식별자가 메시지에 포함되어야 함")
	assert.Contains(t, msg, "disabled-x", "disabled agent 식별자가 메시지에 포함되어야 함")
}

// TestValidateAgentRefs_DisabledErrorMessage_IncludesAgentID 는 에러 메시지에 agent ID,
// 노드 정보, Enable API 안내가 모두 포함되는지 확인한다 (R5.3).
func TestValidateAgentRefs_DisabledErrorMessage_IncludesAgentID(t *testing.T) {
	t.Parallel()

	e, mgr := newEngineWithAgentManager(t)
	_, err := mgr.Create(agent.AgentConfig{
		ID:      "agt-42",
		Name:    "worker",
		Enabled: boolPtr(false),
	})
	require.NoError(t, err)

	nodes := []flow.NodeDef{flow.NewNodeDef("consumer", "transform")}
	nodes[0].AgentRef = &flow.AgentRef{AgentID: "agt-42", AgentName: "worker"}
	f := flow.NewFlow("test-flow", flow.WithNodes(nodes...))

	err = e.DeployFlow(context.Background(), f)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrAgentDisabled)

	msg := err.Error()
	assert.Contains(t, msg, "agt-42", "agent ID 가 포함되어야 함")
	assert.Contains(t, msg, "worker", "agent name 이 포함되어야 함")
	assert.Contains(t, msg, "consumer", "노드 이름이 포함되어야 함")
	assert.Contains(t, msg, "POST /agents/agt-42/enable", "Enable API 안내가 포함되어야 함")
	assert.Contains(t, msg, "transform", "노드 타입이 포함되어야 함")
}

// TestValidateAgentRefs_NilEnabled_TreatedAsEnabled 는 Enabled 필드가 nil 일 때
// 기본 true 로 취급되어 검증을 통과해야 함을 확인한다 (R7.1 하위 호환성).
func TestValidateAgentRefs_NilEnabled_TreatedAsEnabled(t *testing.T) {
	t.Parallel()

	e, mgr := newEngineWithAgentManager(t)
	// Enabled 필드를 명시하지 않음 → nil → IsEnabled() == true
	ag, err := mgr.Create(agent.AgentConfig{
		ID:   "legacy-agent",
		Name: "legacy",
	})
	require.NoError(t, err)
	// 방어적으로 확인: Enabled 는 실제로 nil 이어야 함
	require.Nil(t, ag.Info().Config.Enabled, "새로 생성한 에이전트의 Enabled 는 nil 이어야 함")

	nodes := []flow.NodeDef{flow.NewNodeDef("A", "transform")}
	nodes[0].AgentRef = &flow.AgentRef{AgentID: ag.ID(), AgentName: ag.Name()}
	f := flow.NewFlow("test-flow", flow.WithNodes(nodes...))

	err = e.DeployFlow(context.Background(), f)
	assert.NoError(t, err, "Enabled == nil 은 기본 활성화로 취급되어야 함")
}

// TestValidateAgentRefs_ExplicitlyEnabled_ReturnsNil 은 Enabled=&true 인 경우도
// 정상적으로 검증을 통과해야 함을 확인한다.
func TestValidateAgentRefs_ExplicitlyEnabled_ReturnsNil(t *testing.T) {
	t.Parallel()

	e, mgr := newEngineWithAgentManager(t)
	ag, err := mgr.Create(agent.AgentConfig{
		ID:      "explicit-on",
		Name:    "explicit-on",
		Enabled: boolPtr(true),
	})
	require.NoError(t, err)

	nodes := []flow.NodeDef{flow.NewNodeDef("A", "transform")}
	nodes[0].AgentRef = &flow.AgentRef{AgentID: ag.ID(), AgentName: ag.Name()}
	f := flow.NewFlow("test-flow", flow.WithNodes(nodes...))

	err = e.DeployFlow(context.Background(), f)
	assert.NoError(t, err)
}
