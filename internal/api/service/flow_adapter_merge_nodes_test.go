package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/api/handler"
	"github.com/xtra/xflow/internal/engine"
)

// ---------------------------------------------------------------------------
// 순수 병합기 테스트 — mergeNodeInstanceStats
// ---------------------------------------------------------------------------
//
// 단독 배포된 서브플로우(idle, processed=0)와 부모 안에서 실행 중인 임베디드 인스턴스
// (active, processed>0)는 동일한 원본 노드 ID 를 가진다. mergeNodeInstanceStats 는 이
// 둘을 노드 ID 단위로 합산하여, 사용자가 단독 뷰에서도 실제 활동을 보도록 한다.

// mergePort 는 병합 입력용 포트를 손쉽게 만든다.
func mergePort(name, dir string, messages, delivered int64, throughput float64, active time.Duration, connected bool) engine.NodePortInfo {
	return engine.NodePortInfo{
		ID:         name,
		Name:       name,
		Direction:  dir,
		Connected:  connected,
		Messages:   messages,
		Delivered:  delivered,
		Throughput: throughput,
		ActiveFor:  active,
	}
}

// idle standalone(0) + active embedded(>0) 가 노드 ID 단위로 합산되어야 한다.
func TestMergeNodeInstanceStats_idle단독_active임베디드_합산(t *testing.T) {
	standalone := []engine.NodeInstanceInfo{
		{
			NodeID: "A", Name: "노드A", Type: "transform", State: "running",
			Processed: 0, Errors: 0,
			Ports: []engine.NodePortInfo{
				mergePort("out", "output", 0, 0, 0, 0, false),
			},
		},
	}
	embedded := []engine.NodeInstanceInfo{
		{
			NodeID: "A", Name: "노드A", Type: "transform", State: "running",
			Processed: 42, Errors: 3,
			Ports: []engine.NodePortInfo{
				mergePort("out", "output", 42, 40, 1.5, 5*time.Second, true),
			},
		},
	}

	got := mergeNodeInstanceStats(standalone, embedded)
	require.Len(t, got, 1)
	a := got[0]
	assert.Equal(t, "A", a.NodeID)
	assert.Equal(t, int64(42), a.Processed, "0 + 42")
	assert.Equal(t, int64(3), a.Errors, "0 + 3")
	require.Len(t, a.Ports, 1)
	assert.Equal(t, "out", a.Ports[0].Name)
	assert.Equal(t, int64(42), a.Ports[0].Messages, "port messages 합산")
	assert.Equal(t, int64(40), a.Ports[0].Delivered, "port delivered 합산")
	assert.InDelta(t, 1.5, a.Ports[0].Throughput, 1e-9, "throughput 합산")
	assert.Equal(t, 5*time.Second, a.Ports[0].ActiveFor, "activeFor max")
	assert.True(t, a.Ports[0].Connected, "connected OR")
}

// 양쪽에만 있는 노드는 모두 포함되며, 공통 노드만 합산된다(노드 ID 단위 합집합).
func TestMergeNodeInstanceStats_합집합(t *testing.T) {
	a := []engine.NodeInstanceInfo{
		{NodeID: "A", Processed: 1},
		{NodeID: "onlyA", Processed: 7},
	}
	b := []engine.NodeInstanceInfo{
		{NodeID: "A", Processed: 2},
		{NodeID: "onlyB", Processed: 9},
	}
	got := mergeNodeInstanceStats(a, b)
	byID := map[string]engine.NodeInstanceInfo{}
	for _, n := range got {
		byID[n.NodeID] = n
	}
	require.Len(t, got, 3)
	assert.Equal(t, int64(3), byID["A"].Processed, "공통 노드 합산")
	assert.Equal(t, int64(7), byID["onlyA"].Processed)
	assert.Equal(t, int64(9), byID["onlyB"].Processed)
}

// 포트는 이름 단위로 병합되고, 한쪽에만 있는 포트는 그대로 포함된다.
func TestMergeNodeInstanceStats_포트이름병합(t *testing.T) {
	a := []engine.NodeInstanceInfo{
		{
			NodeID: "A",
			Ports: []engine.NodePortInfo{
				mergePort("in", "input", 5, 5, 0, 0, true),
				mergePort("out", "output", 3, 2, 0, 0, false),
			},
		},
	}
	b := []engine.NodeInstanceInfo{
		{
			NodeID: "A",
			Ports: []engine.NodePortInfo{
				mergePort("out", "output", 10, 8, 0, 0, true),
				mergePort("err", "error", 1, 0, 0, 0, false),
			},
		},
	}
	got := mergeNodeInstanceStats(a, b)
	require.Len(t, got, 1)
	ports := map[string]engine.NodePortInfo{}
	for _, p := range got[0].Ports {
		ports[p.Name] = p
	}
	require.Len(t, got[0].Ports, 3, "in/out/err 모두 포함")
	assert.Equal(t, int64(5), ports["in"].Messages, "a 전용 포트 보존")
	assert.Equal(t, int64(13), ports["out"].Messages, "공통 포트 messages 합산(3+10)")
	assert.Equal(t, int64(10), ports["out"].Delivered, "공통 포트 delivered 합산(2+8)")
	assert.True(t, ports["out"].Connected, "connected OR")
	assert.Equal(t, int64(1), ports["err"].Messages, "b 전용 포트 보존")
}

// 메타데이터(Name/Type/State)는 가진 쪽에서 채운다(빈 쪽을 채워준다).
func TestMergeNodeInstanceStats_메타보강(t *testing.T) {
	a := []engine.NodeInstanceInfo{
		{NodeID: "A", Name: "", Type: "", State: "", Processed: 0},
	}
	b := []engine.NodeInstanceInfo{
		{NodeID: "A", Name: "노드A", Type: "transform", State: "running", Processed: 5},
	}
	got := mergeNodeInstanceStats(a, b)
	require.Len(t, got, 1)
	assert.Equal(t, "노드A", got[0].Name)
	assert.Equal(t, "transform", got[0].Type)
	assert.Equal(t, "running", got[0].State)
}

// 한쪽이 비어 있으면 다른 쪽을 그대로 반환한다(항상 비-nil).
func TestMergeNodeInstanceStats_한쪽비어있음(t *testing.T) {
	only := []engine.NodeInstanceInfo{{NodeID: "A", Processed: 4}}

	gotA := mergeNodeInstanceStats(only, nil)
	require.Len(t, gotA, 1)
	assert.Equal(t, int64(4), gotA[0].Processed)

	gotB := mergeNodeInstanceStats(nil, only)
	require.Len(t, gotB, 1)
	assert.Equal(t, int64(4), gotB[0].Processed)

	gotEmpty := mergeNodeInstanceStats(nil, nil)
	assert.NotNil(t, gotEmpty)
	assert.Len(t, gotEmpty, 0)
}

// ---------------------------------------------------------------------------
// ListFlowNodes 병합 통합 테스트 (standalone-idle + embedded-active)
// ---------------------------------------------------------------------------

// 실제 버그 재현: 서브플로우 X 가 단독 배포(idle, processed=0)되어 있고, 동시에 부모 P
// 안에서 활성(processed>0)으로 실행 중이면, ListFlowNodes(X) 는 둘을 합산하여 실제
// 활동(>0)을 보여야 한다. 현재(버그) 코드는 단독 배포 노드(0)만 반환하고 임베디드를
// 건너뛰어 0 을 보여준다 → 이 테스트가 RED 가 된다.
func TestListFlowNodes_단독idle_임베디드active_합산(t *testing.T) {
	eng := newTestEngine()
	repo := newTestRepo(t)
	adapter := NewFlowServiceAdapter(eng, repo, nil)
	ctx := context.Background()

	const subflowID = "S"
	const parentID = "P"
	buildParentReferencingSubflow(t, ctx, repo, subflowID, parentID)

	// 1) 서브플로우 S 를 단독 배포한다(노드 A, idle: processed/messages=0).
	require.NoError(t, adapter.DeployFlow(ctx, subflowID))

	// 2) 부모 P 를 배포한다(서브플로우 확장 → subflow_F_A 네임스페이스 노드 생성).
	require.NoError(t, adapter.DeployFlow(ctx, parentID))

	// 3) 부모 안 임베디드 인스턴스를 결정적으로 active 로 만든다.
	//    노드 A 의 출력 포트 "out" 에 42개 메시지를 생산한 것으로 설정한다.
	//    (handler.FlowNodeInfo 는 노드 단위 Processed 를 노출하지 않으므로 포트
	//     단위 Messages 로 활동을 관측한다.)
	require.True(t, eng.SetPortMessagesForTest(parentID, "subflow_F_A", "out", 42),
		"임베디드 네임스페이스 노드의 포트 카운터를 설정할 수 있어야 한다")

	nodes, err := adapter.ListFlowNodes(ctx, subflowID)
	require.NoError(t, err)
	require.NotNil(t, nodes)

	// 단독 idle(0) + 임베디드 active(42) = 42 가 사용자에게 보여야 한다.
	a, ok := findFlowNode(nodes, "A")
	require.True(t, ok, "원본 노드 A 가 노출되어야 한다, got=%v", nodes)
	outMsgs := portMessages(a, "out")
	assert.Equal(t, int64(42), outMsgs,
		"단독 idle(out=0) + 임베디드 active(out=42) 가 합산되어야 한다")

	// 네임스페이스 ID 가 새어 나오면 안 된다.
	_, leaked := findFlowNode(nodes, "subflow_F_A")
	assert.False(t, leaked, "네임스페이스 ID 가 그대로 노출되면 안 된다")
}

// findFlowNode 는 결과에서 노드 ID 로 노드를 찾는다.
func findFlowNode(nodes []handler.FlowNodeInfo, id string) (handler.FlowNodeInfo, bool) {
	for _, n := range nodes {
		if n.NodeID == id {
			return n, true
		}
	}
	return handler.FlowNodeInfo{}, false
}

// portMessages 는 노드의 portName 포트 Messages 값을 반환한다(없으면 -1).
func portMessages(n handler.FlowNodeInfo, portName string) int64 {
	for _, p := range n.Ports {
		if p.Name == portName {
			return p.Messages
		}
	}
	return -1
}
