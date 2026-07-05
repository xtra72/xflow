package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/engine"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
)

// TestAdapter_CreateFlow_자기참조_거부 는 자기참조 flow-node 를 가진 정의의 생성이
// 순환 검출로 거부되는지 확인한다(REQ-SUBFLOW-E01 저장 경로 훅).
func TestAdapter_CreateFlow_자기참조_거부(t *testing.T) {
	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)
	ctx := context.Background()

	// id 를 고정하고 그 id 를 참조하는 flow-node 를 넣어 자기참조를 만든다.
	const selfID = "self-ref-flow"
	_, err := adapter.CreateFlow(ctx, &dto.FlowCreateRequest{
		Name: "self-ref",
		Definition: map[string]any{
			"id":   selfID,
			"name": "self-ref",
			"nodes": []any{
				map[string]any{
					"id":     "fn-self",
					"type":   "flow-node",
					"config": map[string]any{"flow_id": selfID},
				},
			},
			"wires": []any{},
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "순환 참조")
}

// TestAdapter_CreateFlow_정상_허용 은 flow-node 가 없는 일반 플로우 생성이
// 순환 검출 훅에 영향을 받지 않는지 확인한다(회귀 방지).
func TestAdapter_CreateFlow_정상_허용(t *testing.T) {
	eng := newTestEngine()
	adapter := NewFlowServiceAdapter(eng, newTestRepo(t), nil)
	ctx := context.Background()

	info, err := adapter.CreateFlow(ctx, &dto.FlowCreateRequest{
		Name: "plain",
		Definition: map[string]any{
			"name":  "plain",
			"nodes": []any{},
			"wires": []any{},
		},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, info.ID)
}

// TestAdapter_UpdateFlow_순환생성_거부 는 갱신으로 A→B→A 순환을 만들면 거부되는지
// 확인한다(REQ-SUBFLOW-E02/E04 갱신 경로 훅).
func TestAdapter_UpdateFlow_순환생성_거부(t *testing.T) {
	eng := newTestEngine()
	repo := newTestRepo(t)
	adapter := NewFlowServiceAdapter(eng, repo, nil)
	ctx := context.Background()

	// A, B 생성(순환 없음). A 는 비어 있고 B 는 A 를 참조.
	const idA = "flow-A"
	const idB = "flow-B"
	_, err := adapter.CreateFlow(ctx, &dto.FlowCreateRequest{
		Name: "A",
		Definition: map[string]any{
			"id": idA, "name": "A", "nodes": []any{}, "wires": []any{},
		},
	})
	require.NoError(t, err)

	_, err = adapter.CreateFlow(ctx, &dto.FlowCreateRequest{
		Name: "B",
		Definition: map[string]any{
			"id": idB, "name": "B",
			"nodes": []any{
				map[string]any{"id": "fn-a", "type": "flow-node", "config": map[string]any{"flow_id": idA}},
			},
			"wires": []any{},
		},
	})
	require.NoError(t, err)

	// 이제 A 를 B 참조로 갱신 → A→B→A 순환.
	_, err = adapter.UpdateFlow(ctx, idA, &dto.FlowUpdateRequest{
		Definition: map[string]any{
			"id": idA, "name": "A",
			"nodes": []any{
				map[string]any{"id": "fn-b", "type": "flow-node", "config": map[string]any{"flow_id": idB}},
			},
			"wires": []any{},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "순환 참조")
}

// TestAdapter_DeployFlow_경계와이어_단독배포_무에러 는 플로우 포트 경계 와이어를 가진
// 플로우를 단독 배포할 때 경계 와이어가 제거되어 엔진 배포가 에러 없이 완료되는지
// 확인한다(REQ-SUBFLOW-F01).
func TestAdapter_DeployFlow_경계와이어_단독배포_무에러(t *testing.T) {
	eng := newTestEngine()
	repo := newTestRepo(t)
	adapter := NewFlowServiceAdapter(eng, repo, nil)
	ctx := context.Background()

	// 내부 노드 1개 + 플로우 입력/출력 경계 와이어를 직접 구성하여 저장한다.
	f := flow.NewFlow("boundary-deploy",
		flow.WithFlowInputPorts(flow.Port{ID: "p-in", Name: "in1"}),
		flow.WithFlowOutputPorts(flow.Port{ID: "p-out", Name: "out1"}),
		flow.WithNodes(flow.NodeDef{
			ID:   "filter-1",
			Type: "filter",
			Inputs: []flow.Port{
				{ID: "in", Name: "in", Direction: flow.PortInput},
			},
			Outputs: []flow.Port{
				{ID: "out", Name: "out", Direction: flow.PortOutput},
			},
			Config: map[string]any{"condition": "$.payload.value > 0"},
		}),
		flow.WithWires(
			// 플로우 입력 포트 → 내부 노드
			flow.NewWire(flow.FlowInputBoundaryID, "in1", "filter-1", "in"),
			// 내부 노드 → 플로우 출력 포트
			flow.NewWire("filter-1", "out", flow.FlowOutputBoundaryID, "out1"),
		),
	)
	require.NoError(t, repo.Save(ctx, f))

	// 단독 배포: 경계 와이어가 stripped 되어 엔진 배포가 에러 없이 완료되어야 한다.
	err := adapter.DeployFlow(ctx, f.ID())
	assert.NoError(t, err)
}

// TestAdapter_DeployFlow_경계와이어_shared옵너설정_무에러 는 로컬 opener 가 설정된 상태에서
// 경계 포트 플로우를 단독 배포·시작·정지해도(공유 경계 탭 설치 경로) 에러가 없는지 검증한다
// (SPEC-SUBFLOW-002 N01 회귀 0 — 부착 브리지 0개면 StripBoundaryWires 와 동작 동일).
func TestAdapter_DeployFlow_경계와이어_shared옵너설정_무에러(t *testing.T) {
	reg := node.NewRegistry()
	require.NoError(t, RegisterSharedBoundaryNodes(reg))
	eng := engine.NewEngine(engine.WithNodeRegistry(reg))
	repo := newTestRepo(t)
	adapter := NewFlowServiceAdapter(eng, repo, nil)
	adapter.SetLocalBridgeOpener(NewLocalBridgeOpener(eng, nil))
	ctx := context.Background()

	f := flow.NewFlow("boundary-shared",
		flow.WithFlowInputPorts(flow.Port{ID: "p-in", Name: "in1"}),
		flow.WithFlowOutputPorts(flow.Port{ID: "p-out", Name: "out1"}),
		flow.WithNodes(flow.NodeDef{
			ID:      "filter-1",
			Type:    "filter",
			Inputs:  []flow.Port{{ID: "in", Name: "in", Direction: flow.PortInput}},
			Outputs: []flow.Port{{ID: "out", Name: "out", Direction: flow.PortOutput}},
			Config:  map[string]any{"condition": "$.payload.value > 0"},
		}),
		flow.WithWires(
			flow.NewWire(flow.FlowInputBoundaryID, "in1", "filter-1", "in"),
			flow.NewWire("filter-1", "out", flow.FlowOutputBoundaryID, "out1"),
		),
	)
	require.NoError(t, repo.Save(ctx, f))

	// 배포: 공유 경계 탭으로 재배선되어 엔진 배포가 에러 없이 완료되어야 한다.
	require.NoError(t, adapter.DeployFlow(ctx, f.ID()))
	// 공유 경계 컨트롤러가 등록되어야 한다(경계 포트 보유 플로우).
	_, ok := globalSharedBoundaryTable.lookup(f.ID())
	assert.True(t, ok, "경계 포트 플로우는 공유 경계 컨트롤러를 등록해야 한다")

	// 시작·정지·undeploy 도 에러 없이 동작해야 한다(부착 브리지 0개 — 무출력).
	require.NoError(t, eng.StartFlow(ctx, f.ID()))
	require.NoError(t, eng.StopFlow(ctx, f.ID()))
	require.NoError(t, adapter.UndeployFlow(ctx, f.ID()))

	// undeploy 시 공유 경계 컨트롤러가 정리되어야 한다(누수 방지).
	_, stillThere := globalSharedBoundaryTable.lookup(f.ID())
	assert.False(t, stillThere, "undeploy 시 공유 경계 컨트롤러가 정리되어야 한다")
}
