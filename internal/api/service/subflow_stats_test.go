package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/api/handler"
	"github.com/xtra/xflow/internal/engine"
	"github.com/xtra/xflow/pkg/flow"
)

// ---------------------------------------------------------------------------
// 순수 집계기 테스트 — aggregateSubflowStats
// ---------------------------------------------------------------------------

// nsNode 는 네임스페이스 노드 통계(engine.NodeInstanceInfo)를 손쉽게 만든다.
func nsNode(id string, processed, errors int64, ports ...engine.NodePortInfo) engine.NodeInstanceInfo {
	return engine.NodeInstanceInfo{
		NodeID:    id,
		Name:      id,
		Type:      "transform",
		State:     "running",
		Processed: processed,
		Errors:    errors,
		Ports:     ports,
	}
}

func nsPort(name, dir string, messages, delivered int64) engine.NodePortInfo {
	return engine.NodePortInfo{
		ID:        name,
		Name:      name,
		Direction: dir,
		Connected: true,
		Messages:  messages,
		Delivered: delivered,
	}
}

// 실행 중인 부모가 없으면 빈(비-nil) 슬라이스여야 한다.
func TestAggregateSubflowStats_부모없음_빈결과(t *testing.T) {
	got := aggregateSubflowStats(nil)
	assert.NotNil(t, got)
	assert.Len(t, got, 0)
}

// 단일 부모: flow-node F 가 S 를 참조하고, subflow_F_* 네임스페이스 노드의 통계가
// 원본 노드 ID 로 매핑되어야 한다(접두사 미일치 노드는 무시).
func TestAggregateSubflowStats_단일부모_매핑(t *testing.T) {
	parents := []parentNamespacedStats{
		{
			flowNodeIDs: []string{"F"},
			nodes: []engine.NodeInstanceInfo{
				nsNode("subflow_F_A", 10, 1, nsPort("out", "output", 10, 9)),
				nsNode("subflow_F_B", 5, 0),
				// 다른 flow-node(G)의 노드 — 무시되어야 한다.
				nsNode("subflow_G_A", 99, 9),
				// 일반(비-네임스페이스) 노드 — 무시.
				nsNode("plain", 7, 7),
			},
		},
	}

	got := aggregateSubflowStats(parents)
	require.Len(t, got, 2)

	byID := map[string]engine.NodeInstanceInfo{}
	for _, n := range got {
		byID[n.NodeID] = n
	}

	a, ok := byID["A"]
	require.True(t, ok, "원본 노드 A 가 있어야 한다")
	assert.Equal(t, int64(10), a.Processed)
	assert.Equal(t, int64(1), a.Errors)
	require.Len(t, a.Ports, 1)
	assert.Equal(t, "out", a.Ports[0].Name)
	assert.Equal(t, int64(10), a.Ports[0].Messages)
	assert.Equal(t, int64(9), a.Ports[0].Delivered)

	b, ok := byID["B"]
	require.True(t, ok, "원본 노드 B 가 있어야 한다")
	assert.Equal(t, int64(5), b.Processed)
}

// 두 부모가 동일 서브플로우 S 를 참조하면 원본 노드 단위로 합산되어야 한다.
func TestAggregateSubflowStats_두부모_합산(t *testing.T) {
	parents := []parentNamespacedStats{
		{
			flowNodeIDs: []string{"F"},
			nodes: []engine.NodeInstanceInfo{
				nsNode("subflow_F_A", 10, 1, nsPort("out", "output", 10, 8)),
			},
		},
		{
			flowNodeIDs: []string{"H"},
			nodes: []engine.NodeInstanceInfo{
				nsNode("subflow_H_A", 5, 2, nsPort("out", "output", 5, 5)),
			},
		},
	}

	got := aggregateSubflowStats(parents)
	require.Len(t, got, 1)
	a := got[0]
	assert.Equal(t, "A", a.NodeID)
	assert.Equal(t, int64(15), a.Processed, "processed 합산")
	assert.Equal(t, int64(3), a.Errors, "errors 합산")
	require.Len(t, a.Ports, 1)
	assert.Equal(t, int64(15), a.Ports[0].Messages, "port messages 합산")
	assert.Equal(t, int64(13), a.Ports[0].Delivered, "port delivered 합산")
}

// 한 부모가 같은 서브플로우를 두 번 참조(F, G 두 flow-node)하면 둘 다 집계된다.
func TestAggregateSubflowStats_한부모_다중참조_합산(t *testing.T) {
	parents := []parentNamespacedStats{
		{
			flowNodeIDs: []string{"F", "G"},
			nodes: []engine.NodeInstanceInfo{
				nsNode("subflow_F_A", 3, 0),
				nsNode("subflow_G_A", 4, 0),
			},
		},
	}
	got := aggregateSubflowStats(parents)
	require.Len(t, got, 1)
	assert.Equal(t, int64(7), got[0].Processed)
}

// ---------------------------------------------------------------------------
// 어댑터 통합 테스트 — SubflowNodeStats (parent-discovery 배선)
// ---------------------------------------------------------------------------

// 배포된 부모가 없으면 빈 결과(200)를 반환한다.
func TestSubflowNodeStats_부모없음_빈결과(t *testing.T) {
	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)

	got, err := adapter.SubflowNodeStats(context.Background(), "S")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "S", got.FlowID)
	assert.NotNil(t, got.Nodes)
	assert.Len(t, got.Nodes, 0)
}

// 서브플로우 S 를 참조하는 부모를 배포하면, 단독 뷰에서 S 의 원본 노드 통계가
// 네임스페이스 노드로부터 집계되어 노출된다(카운트는 0 이어도 노드 존재가 보장됨).
func TestSubflowNodeStats_부모배포_원본노드노출(t *testing.T) {
	eng := newTestEngine()
	repo := newTestRepo(t)
	adapter := NewFlowServiceAdapter(eng, repo, nil)
	ctx := context.Background()

	const subflowID = "S"
	const parentID = "P"

	// 서브플로우 S: input X → A(transform) → output Y
	sub := flow.NewFlowWithID(subflowID, "서브플로우S",
		flow.WithNodes(passthroughNode("A", "노드A")),
		flow.WithWires(
			wire("s_in", flow.FlowInputBoundaryID, "X", "A", "in"),
			wire("s_out", "A", "out", flow.FlowOutputBoundaryID, "Y"),
		),
		flow.WithFlowInputPorts(inPort("X")),
		flow.WithFlowOutputPorts(outPort("Y")),
	)
	require.NoError(t, repo.Save(ctx, sub))

	// 부모 P: src → flow-node(F→S) → sink
	fn := flowNode("F", "서브참조", subflowID, []flow.Port{inPort("X")}, []flow.Port{outPort("Y")})
	parent := flow.NewFlowWithID(parentID, "부모P",
		flow.WithNodes(passthroughNode("src", "소스"), fn, passthroughNode("sink", "싱크")),
		flow.WithWires(
			wire("p_in", "src", "out", "F", "X"),
			wire("p_out", "F", "Y", "sink", "in"),
		),
	)
	require.NoError(t, repo.Save(ctx, parent))

	// 부모 배포(서브플로우 확장 → subflow_F_A 네임스페이스 노드 생성).
	require.NoError(t, adapter.DeployFlow(ctx, parentID))

	got, err := adapter.SubflowNodeStats(ctx, subflowID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, subflowID, got.FlowID)

	// 원본 노드 A 가 노출되어야 한다(네임스페이스 subflow_F_A → 원본 A).
	byID := map[string]handler.SubflowNodeStat{}
	for _, n := range got.Nodes {
		byID[n.NodeID] = n
	}
	_, ok := byID["A"]
	assert.True(t, ok, "서브플로우 원본 노드 A 가 단독 뷰 통계에 있어야 한다, got=%v", got.Nodes)
}

// 다른 서브플로우를 참조하는 부모만 배포되어 있으면 빈 결과여야 한다(매칭 무관).
func TestSubflowNodeStats_무관부모_빈결과(t *testing.T) {
	eng := newTestEngine()
	repo := newTestRepo(t)
	adapter := NewFlowServiceAdapter(eng, repo, nil)
	ctx := context.Background()

	// 부모는 flow-node 없는 일반 플로우.
	_, err := adapter.CreateFlow(ctx, &dto.FlowCreateRequest{
		Name:       "plain",
		Definition: map[string]any{"id": "plain", "name": "plain", "nodes": []any{}, "wires": []any{}},
	})
	require.NoError(t, err)
	require.NoError(t, adapter.DeployFlow(ctx, "plain"))

	got, err := adapter.SubflowNodeStats(ctx, "S")
	require.NoError(t, err)
	assert.Len(t, got.Nodes, 0)
}
