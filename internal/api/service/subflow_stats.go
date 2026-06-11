package service

import (
	"context"
	"fmt"
	"time"

	"github.com/xtra/xflow/internal/api/handler"
	"github.com/xtra/xflow/internal/engine"
	"github.com/xtra/xflow/pkg/flow"
)

// ---------------------------------------------------------------------------
// 서브플로우 LIVE 통계 (Fix 2) — SPEC subflow live-stats
// ---------------------------------------------------------------------------
//
// 서브플로우의 내부 노드는 부모 플로우 안에서 네임스페이스 ID
// "subflow_<flowNodeID>_<originalID>" 로 실행된다(subflow_expand.go 참조). 따라서
// engine.GetFlowNodes(parent) 는 네임스페이스 ID 로 키된 NodeInstanceInfo 를 돌려주며,
// 에디터가 서브플로우를 단독으로 열면 원본 노드 ID 와 매칭되지 않아 0 으로 보인다.
//
// SubflowNodeStats 는 이를 해결한다: subflowID 를 LOCAL 참조하는 모든 배포 부모를 찾아,
// 각 부모의 네임스페이스 노드를 원본 노드 ID 로 역매핑(ParseSubflowNodeID)하여 합산한다.
// 같은 서브플로우를 여러 부모/flow-node 가 참조하면 원본 노드 단위로 모두 합산된다.

// parentNamespacedStats 는 한 부모 플로우에서 추출한, 특정 서브플로우 집계에 필요한 입력이다.
//   - flowNodeIDs: 이 부모 안에서 대상 서브플로우를 LOCAL 참조하는 flow-node 들의 ID.
//   - nodes:       이 부모의 전체 네임스페이스 노드 통계(engine.GetFlowNodes 결과).
//
// 순수 집계기(aggregateSubflowStats)와 어댑터 배선(SubflowNodeStats)을 분리하기 위한 DTO 로,
// 라이브 엔진 없이도 집계 로직을 결정적으로 단위 테스트할 수 있게 한다.
type parentNamespacedStats struct {
	flowNodeIDs []string
	nodes       []engine.NodeInstanceInfo
}

// aggregateSubflowStats 는 부모별 네임스페이스 노드 통계를 원본 노드 단위로 합산한 결과를
// 만든다(순수 함수 — 외부 의존 없음).
//
// 매핑 규칙:
//   - 각 네임스페이스 노드 ID 에 ParseSubflowNodeID 를 적용하여 (flowNodeID, originalID) 를 얻는다.
//   - flowNodeID 가 해당 부모의 대상 flow-node 집합(flowNodeIDs)에 속할 때만 채택한다.
//     (같은 부모 안의 다른 서브플로우 인스턴스 노드는 제외.)
//   - originalID 를 키로 Processed/Errors 와 포트별 Messages/Delivered/Throughput 를 합산한다.
//
// 결과의 Nodes 는 항상 비-nil 슬라이스이며(참조 부모 없음 → 빈 슬라이스), 결정적 출력을 위해
// 원본 노드 ID 오름차순으로 정렬된다.
func aggregateSubflowStats(subflowID string, parents []parentNamespacedStats) *handler.SubflowStatsInfo {
	// 원본 노드 ID → 누적 통계.
	type portAgg struct {
		direction  string
		connected  bool
		messages   int64
		delivered  int64
		throughput float64
		activeFor  time.Duration
	}
	type nodeAgg struct {
		name      string
		nodeType  string
		state     string
		processed int64
		errors    int64
		ports     map[string]*portAgg // portName → 누적
		portOrder []string            // 최초 등장 순서 보존
	}

	agg := make(map[string]*nodeAgg)
	var order []string // 원본 노드 ID 최초 등장 순서(정렬 전 안정성)

	for _, p := range parents {
		// 이 부모에서 대상 서브플로우를 참조하는 flow-node ID 집합.
		want := make(map[string]bool, len(p.flowNodeIDs))
		for _, id := range p.flowNodeIDs {
			want[id] = true
		}
		if len(want) == 0 {
			continue
		}

		for _, n := range p.nodes {
			flowNodeID, originalID, ok := ParseSubflowNodeID(n.NodeID)
			if !ok || !want[flowNodeID] {
				continue
			}

			na := agg[originalID]
			if na == nil {
				na = &nodeAgg{ports: make(map[string]*portAgg)}
				agg[originalID] = na
				order = append(order, originalID)
			}
			// 메타데이터는 첫 매칭 인스턴스 기준으로 채운다.
			if na.name == "" {
				na.name = n.Name
			}
			if na.nodeType == "" {
				na.nodeType = n.Type
			}
			if na.state == "" {
				na.state = n.State
			}
			na.processed += n.Processed
			na.errors += n.Errors

			for _, port := range n.Ports {
				pa := na.ports[port.Name]
				if pa == nil {
					pa = &portAgg{direction: port.Direction}
					na.ports[port.Name] = pa
					na.portOrder = append(na.portOrder, port.Name)
				}
				pa.connected = pa.connected || port.Connected
				pa.messages += port.Messages
				pa.delivered += port.Delivered
				pa.throughput += port.Throughput // 동시 인스턴스 처리량 합산.
				if port.ActiveFor > pa.activeFor {
					pa.activeFor = port.ActiveFor // 가장 오래된 활동 시간 채택.
				}
			}
		}
	}

	out := &handler.SubflowStatsInfo{
		FlowID: subflowID,
		Nodes:  make([]handler.SubflowNodeStat, 0, len(order)),
	}

	// 결정적 출력을 위해 원본 노드 ID 오름차순 정렬.
	sortStrings(order)

	for _, originalID := range order {
		na := agg[originalID]
		stat := handler.SubflowNodeStat{
			NodeID:    originalID,
			Name:      na.name,
			Type:      na.nodeType,
			State:     na.state,
			Processed: na.processed,
			Errors:    na.errors,
		}
		for _, portName := range na.portOrder {
			pa := na.ports[portName]
			pi := handler.PortInfo{
				Name:       portName,
				Direction:  pa.direction,
				Connected:  pa.connected,
				Messages:   pa.messages,
				Delivered:  pa.delivered,
				Throughput: fmt.Sprintf("%.3f", pa.throughput),
			}
			if pa.activeFor > 0 {
				pi.ActiveFor = pa.activeFor.Truncate(time.Second).String()
			}
			stat.Ports = append(stat.Ports, pi)
		}
		out.Nodes = append(out.Nodes, stat)
	}

	return out
}

// SubflowNodeStats 는 서브플로우(subflowID)의 LIVE per-original-node 통계를 반환한다(Fix 2).
//
// 배선:
//  1. engine.ListFlows() 로 현재 배포된 모든 부모 플로우를 열거한다.
//  2. 각 부모의 정의를 repo(우선) 또는 engine.GetFlow 에서 가져와, subflowID 를 LOCAL(bare id)
//     참조하는 flow-node ID 들을 찾는다. (확장된 엔진 정의에는 LOCAL flow-node 가 남지 않으므로
//     repo 정의가 1순위이다.)
//  3. 참조 flow-node 가 있는 부모만 engine.GetFlowNodes(parent) 로 네임스페이스 노드 통계를
//     모아 aggregateSubflowStats 로 합산한다.
//
// 부모가 실행 중이 아니거나 정의 조회에 실패해도 전체가 실패하지 않는다(견고성 — 해당 부모만
// 건너뛴다). 참조 부모가 없으면 Nodes 가 빈 슬라이스인 결과를 반환한다(에러 아님).
func (a *FlowServiceAdapter) SubflowNodeStats(ctx context.Context, subflowID string) (*handler.SubflowStatsInfo, error) {
	deployed := a.engine.ListFlows()

	parents := make([]parentNamespacedStats, 0, len(deployed))
	for _, s := range deployed {
		parentID := s.FlowID

		def := a.parentFlowDefinition(ctx, parentID)
		if def == nil {
			continue // 정의를 어디서도 못 얻으면 건너뛴다.
		}

		flowNodeIDs := localFlowNodesReferencing(def, subflowID)
		if len(flowNodeIDs) == 0 {
			continue // 이 부모는 대상 서브플로우를 참조하지 않는다.
		}

		nodes, err := a.engine.GetFlowNodes(parentID)
		if err != nil {
			continue // 부모가 더 이상 배포 상태가 아니면 건너뛴다.
		}

		parents = append(parents, parentNamespacedStats{
			flowNodeIDs: flowNodeIDs,
			nodes:       nodes,
		})
	}

	return aggregateSubflowStats(subflowID, parents), nil
}

// parentFlowDefinition 은 부모 플로우의 정의를 repo(우선) 또는 엔진 런타임에서 가져온다.
// 둘 다 실패하면 nil 을 반환한다(호출자가 건너뜀).
//
// repo 를 우선하는 이유: 배포 시 LOCAL flow-node 는 네임스페이스 인스턴스로 확장·소비되어
// 엔진 정의에는 남지 않는다. flow-node ID 와 참조(flow_id)는 repo 의 원본 정의에만 존재한다.
func (a *FlowServiceAdapter) parentFlowDefinition(ctx context.Context, parentID string) flow.Flow {
	if f, err := a.repo.Get(ctx, parentID); err == nil {
		return f
	}
	if f, err := a.engine.GetFlow(parentID); err == nil {
		return f
	}
	return nil
}

// localFlowNodesReferencing 은 def 안에서 subflowID 를 LOCAL(bare id)로 참조하는 flow-node
// 들의 ID 목록을 반환한다. remote:// 참조나 다른 서브플로우를 참조하는 flow-node 는 제외한다.
func localFlowNodesReferencing(def flow.Flow, subflowID string) []string {
	var ids []string
	for _, n := range def.Nodes() {
		if n.Type != flowNodeType {
			continue
		}
		ref, _ := n.Config[flowNodeFlowIDKey].(string)
		if ref == "" {
			continue
		}
		// remote:// 참조는 확장되지 않고 라이브 브리지로 동작하므로 네임스페이스 노드를
		// 만들지 않는다. LOCAL bare id 가 대상 서브플로우와 정확히 일치할 때만 채택한다.
		if _, _, isRemote, _ := parseRemoteFlowRef(ref); isRemote {
			continue
		}
		if ref == subflowID {
			ids = append(ids, n.ID)
		}
	}
	return ids
}
