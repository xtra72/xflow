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

// aggregateSubflowStats 는 부모별 네임스페이스 노드 통계를 원본 노드 단위로 합산하여
// de-namespaced [] engine.NodeInstanceInfo 로 반환한다(순수 함수 — 외부 의존 없음).
//
// 단일 진실원(single source of truth): 이 함수가 서브플로우 임베디드 통계의 정규형이며,
// engine.NodeInstanceInfo 를 직접 돌려주므로 ListFlowNodes 의 단독-배포 경로와 동일한
// DTO(engineNodeToFlowNodeInfo) 변환을 그대로 재사용한다. /subflow-stats 응답
// (*handler.SubflowStatsInfo)이 필요하면 subflowStatsFromNodes 로 얇게 감싼다.
//
// 매핑 규칙:
//   - 각 네임스페이스 노드 ID 에 ParseSubflowNodeID 를 적용하여 (flowNodeID, originalID) 를 얻는다.
//   - flowNodeID 가 해당 부모의 대상 flow-node 집합(flowNodeIDs)에 속할 때만 채택한다.
//     (같은 부모 안의 다른 서브플로우 인스턴스 노드는 제외.)
//   - originalID 를 NodeID 로 하여 Processed/Errors 와 포트별 Messages/Delivered/Throughput 를 합산한다.
//
// 결과 슬라이스는 항상 비-nil 이며(참조 부모 없음 → 빈 슬라이스), 결정적 출력을 위해
// NodeID(원본 노드 ID) 오름차순으로 정렬된다.
func aggregateSubflowStats(parents []parentNamespacedStats) []engine.NodeInstanceInfo {
	// 원본 노드 ID → 누적 통계.
	type portAgg struct {
		id         string
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
					pa = &portAgg{id: port.ID, direction: port.Direction}
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

	// 결정적 출력을 위해 원본 노드 ID 오름차순 정렬.
	sortStrings(order)

	out := make([]engine.NodeInstanceInfo, 0, len(order))
	for _, originalID := range order {
		na := agg[originalID]
		ni := engine.NodeInstanceInfo{
			NodeID:    originalID,
			Name:      na.name,
			Type:      na.nodeType,
			State:     na.state,
			Processed: na.processed,
			Errors:    na.errors,
		}
		for _, portName := range na.portOrder {
			pa := na.ports[portName]
			ni.Ports = append(ni.Ports, engine.NodePortInfo{
				ID:         pa.id,
				Name:       portName,
				Direction:  pa.direction,
				Connected:  pa.connected,
				Messages:   pa.messages,
				Delivered:  pa.delivered,
				Throughput: pa.throughput,
				ActiveFor:  pa.activeFor,
			})
		}
		out = append(out, ni)
	}

	return out
}

// subflowStatsFromNodes 는 de-namespaced 노드 통계를 /subflow-stats 응답 DTO 로 감싼다.
// aggregateSubflowStats 의 정규형(engine.NodeInstanceInfo)을 handler.SubflowStatsInfo 로
// 변환하여, /subflow-stats 엔드포인트가 기존 계약을 그대로 유지하게 한다.
func subflowStatsFromNodes(subflowID string, nodes []engine.NodeInstanceInfo) *handler.SubflowStatsInfo {
	out := &handler.SubflowStatsInfo{
		FlowID: subflowID,
		Nodes:  make([]handler.SubflowNodeStat, 0, len(nodes)),
	}
	for _, n := range nodes {
		stat := handler.SubflowNodeStat{
			NodeID:    n.NodeID,
			Name:      n.Name,
			Type:      n.Type,
			State:     n.State,
			Processed: n.Processed,
			Errors:    n.Errors,
		}
		for _, p := range n.Ports {
			pi := handler.PortInfo{
				ID:         p.ID,
				Name:       p.Name,
				Direction:  p.Direction,
				Connected:  p.Connected,
				Messages:   p.Messages,
				Delivered:  p.Delivered,
				Throughput: fmt.Sprintf("%.3f", p.Throughput),
			}
			if p.ActiveFor > 0 {
				pi.ActiveFor = p.ActiveFor.Truncate(time.Second).String()
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
	nodes := a.subflowEmbeddedNodes(ctx, subflowID)
	return subflowStatsFromNodes(subflowID, nodes), nil
}

// subflowEmbeddedNodes 는 subflowID 를 LOCAL 참조하는 모든 배포 부모에서, 서브플로우의
// LIVE per-original-node 통계를 de-namespaced []engine.NodeInstanceInfo 로 집계한다(공유 헬퍼).
//
// 이것이 서브플로우 임베디드 통계의 단일 진실원이다:
//   - /subflow-stats(SubflowNodeStats)는 이를 subflowStatsFromNodes 로 감싸 응답한다.
//   - /flows/{id}/nodes(ListFlowNodes)는 단독 배포가 없을 때 이 결과를 그대로 노출하여
//     에디터가 메인/서브 구분 없이 동일한 쿼리로 라이브 통계를 본다.
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
// 건너뛴다). 참조 부모가 없으면 빈(비-nil) 슬라이스를 반환한다.
func (a *FlowServiceAdapter) subflowEmbeddedNodes(ctx context.Context, subflowID string) []engine.NodeInstanceInfo {
	deployed := a.engine.ListFlows()

	matchedParents := 0
	totalNamespaced := 0
	parents := make([]parentNamespacedStats, 0, len(deployed))
	for _, s := range deployed {
		parentID := s.FlowID

		def := a.parentFlowDefinition(ctx, parentID)
		if def == nil {
			a.logger.Debug("subflow-stats: 부모 정의 조회 실패", "parent_id", parentID)
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
		matchedParents++
		totalNamespaced += len(nodes)
		a.logger.Debug("subflow-stats: 참조 부모 발견",
			"subflow_id", subflowID, "parent_id", parentID,
			"flow_node_ids", flowNodeIDs, "parent_node_count", len(nodes))

		parents = append(parents, parentNamespacedStats{
			flowNodeIDs: flowNodeIDs,
			nodes:       nodes,
		})
	}

	out := aggregateSubflowStats(parents)
	a.logger.Debug("subflow-stats: 결과",
		"subflow_id", subflowID, "deployed", len(deployed),
		"matched_parents", matchedParents, "parent_nodes_total", totalNamespaced,
		"result_nodes", len(out))
	return out
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
//
// TODO(SPEC-SUBFLOW-002 M4 — 통계 정합): 현재는 mode=instance(인라인 확장)만 임베디드 병합
// 대상으로 의미가 있다. mode=shared(또는 미지정→shared) flow-node 는 인라인 확장하지 않아
// 네임스페이스 노드를 만들지 않으므로 이 함수가 자연히 아무것도 반환하지 않는다(병합 비활성 —
// S01 과 정합). M4 에서 shared 통계를 "참조 플로우 자체 통계 직접"으로 연결하고(S01), 화면에
// 모드 배지를 노출(S03)하도록 mode 인지 처리를 추가한다. 현재 백엔드 핵심(M1~M3) 범위에서는
// 기존 instance 병합 경로를 보존만 하고 shared 는 병합에서 제외되도록 둔다(회귀 0).
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
		// mode=shared(또는 미지정) flow-node 는 인라인 확장하지 않으므로 임베디드 병합 대상이
		// 아니다(네임스페이스 노드 부재). mode=instance 만 병합 대상으로 채택한다(M4 전까지의
		// 보수적 처리 — shared 통계는 별도 직접 경로로 M4 에서 연결).
		if normalizeFlowNodeMode(n.Config) != flowModeInstance {
			continue
		}
		if ref == subflowID {
			ids = append(ids, n.ID)
		}
	}
	return ids
}
