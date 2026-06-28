package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/api/handler"
	"github.com/xtra/xflow/internal/storage"
	"github.com/xtra/xflow/pkg/flow"
)

// ---------------------------------------------------------------------------
// ListFlowNodes 단일 경로 통합 테스트 (subflow live-stats fold-in)
// ---------------------------------------------------------------------------
//
// 에디터는 메인/서브 플로우를 구분하지 않고 GET /flows/{id}/nodes 만 사용한다.
// ListFlowNodes(X) 는 X 가 단독 배포면 엔진 노드를, 단독 배포가 아니지만 배포된
// 부모가 X 를 참조하면 부모의 네임스페이스 노드(subflow_F_*)에서 역매핑·합산한
// 서브플로우 임베디드 통계를, 둘 다 아니면 빈 결과를 반환해야 한다.

// buildParentReferencingSubflow 는 서브플로우 S(input X → A → output Y)와,
// 이를 LOCAL 참조하는 부모 P(src → flow-node F → sink)를 저장소에 저장한다.
func buildParentReferencingSubflow(t *testing.T, ctx context.Context, repo storage.FlowRepository, subflowID, parentID string) {
	t.Helper()

	sub := flow.NewFlowWithID(subflowID, "서브플로우",
		flow.WithNodes(passthroughNode("A", "노드A")),
		flow.WithWires(
			wire("s_in", flow.FlowInputBoundaryID, "X", "A", "in"),
			wire("s_out", "A", "out", flow.FlowOutputBoundaryID, "Y"),
		),
		flow.WithFlowInputPorts(inPort("X")),
		flow.WithFlowOutputPorts(outPort("Y")),
	)
	require.NoError(t, repo.Save(ctx, sub))

	fn := flowNode("F", "서브참조", subflowID, []flow.Port{inPort("X")}, []flow.Port{outPort("Y")})
	parent := flow.NewFlowWithID(parentID, "부모",
		flow.WithNodes(passthroughNode("src", "소스"), fn, passthroughNode("sink", "싱크")),
		flow.WithWires(
			wire("p_in", "src", "out", "F", "X"),
			wire("p_out", "F", "Y", "sink", "in"),
		),
	)
	require.NoError(t, repo.Save(ctx, parent))
}

// X 가 단독 배포는 아니지만, 배포된 부모 P 가 X 를 참조하면 ListFlowNodes(X) 는
// 네임스페이스 노드(subflow_F_A)를 원본 노드 ID(A)로 역매핑하여 노출해야 한다.
func TestListFlowNodes_서브플로우임베디드_역매핑노출(t *testing.T) {
	eng := newTestEngine()
	repo := newTestRepo(t)
	adapter := NewFlowServiceAdapter(eng, repo, nil)
	ctx := context.Background()

	const subflowID = "S"
	const parentID = "P"
	buildParentReferencingSubflow(t, ctx, repo, subflowID, parentID)

	// 부모만 배포한다(서브플로우 S 는 단독 배포되지 않는다).
	require.NoError(t, adapter.DeployFlow(ctx, parentID))

	nodes, err := adapter.ListFlowNodes(ctx, subflowID)
	require.NoError(t, err)
	require.NotNil(t, nodes)

	byID := map[string]handler.FlowNodeInfo{}
	for _, n := range nodes {
		byID[n.NodeID] = n
	}
	a, ok := byID["A"]
	require.True(t, ok, "서브플로우 원본 노드 A 가 노출되어야 한다, got=%v", nodes)
	assert.Equal(t, "노드A", a.Name)
	assert.Equal(t, "transform", a.Type)

	// 네임스페이스 ID 가 그대로 새어 나오면 안 된다.
	_, leaked := byID["subflow_F_A"]
	assert.False(t, leaked, "네임스페이스 ID 가 그대로 노출되면 안 된다")
}

// X 가 단독 배포되어 있으면 ListFlowNodes(X) 는 엔진 자신의 노드를 반환하며,
// 서브플로우 임베디드 병합을 하지 않는다(기존 동작 보존).
func TestListFlowNodes_단독배포_엔진노드보존(t *testing.T) {
	eng := newTestEngine()
	repo := newTestRepo(t)
	adapter := NewFlowServiceAdapter(eng, repo, nil)
	ctx := context.Background()

	const flowID = "standalone"
	f := flow.NewFlowWithID(flowID, "단독",
		flow.WithNodes(passthroughNode("only", "단독노드")),
	)
	require.NoError(t, repo.Save(ctx, f))
	require.NoError(t, adapter.DeployFlow(ctx, flowID))

	nodes, err := adapter.ListFlowNodes(ctx, flowID)
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, "only", nodes[0].NodeID, "엔진 자신의 노드 ID 가 그대로 반환되어야 한다")
}

// X 가 단독 배포도 아니고 어떤 배포 부모도 참조하지 않으면, ListFlowNodes(X) 는
// 빈 슬라이스를 반환하며 에러가 아니어야 한다.
func TestListFlowNodes_미배포_미참조_빈결과(t *testing.T) {
	eng := newTestEngine()
	repo := newTestRepo(t)
	adapter := NewFlowServiceAdapter(eng, repo, nil)
	ctx := context.Background()

	// 무관한 일반 플로우만 배포한다.
	_, err := adapter.CreateFlow(ctx, &dto.FlowCreateRequest{
		Name:       "plain",
		Definition: map[string]any{"id": "plain", "name": "plain", "nodes": []any{}, "wires": []any{}},
	})
	require.NoError(t, err)
	require.NoError(t, adapter.DeployFlow(ctx, "plain"))

	nodes, err := adapter.ListFlowNodes(ctx, "S")
	require.NoError(t, err, "미참조 서브플로우 조회는 에러가 아니어야 한다")
	assert.NotNil(t, nodes)
	assert.Len(t, nodes, 0)
}
