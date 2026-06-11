package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/xtra/xflow/internal/storage"
	"github.com/xtra/xflow/pkg/flow"
)

// ---------------------------------------------------------------------------
// 서브플로우 확장(인스턴스화) — SPEC-SUBFLOW-001 그룹 D
// ---------------------------------------------------------------------------
//
// ExpandSubflows 는 flow-node 를 포함한 플로우를, 각 flow-node 가 참조 플로우의
// 노드+와이어를 네임스페이스 복제한 격리 인스턴스로 치환한 "평탄화된 단일 플로우"로
// 변환한다(결정 1 — 배포 시 서브그래프 확장). 결과 플로우에는 flow-node 타입 노드가
// 존재하지 않으며, 모든 노드 타입이 실제 노드라 엔진이 그대로 인스턴스화할 수 있다.
//
// 핵심 설계:
//   - 항상 최신(결정 2): 참조 플로우는 매 호출 시 repo.Get 으로 조회한다.
//   - 인스턴스 독립(D03): 네임스페이스 접두사에 flow-node 의 ID 를 포함하여,
//     같은 플로우를 여러 번 참조해도 충돌·상태공유가 없다.
//   - 직접 재배선(브리지 노드 없음): flow-node 핸들 ↔ 참조 플로우 경계 포트를
//     중간 노드 없이 와이어만 합성하여 연결한다(가장 가벼우며 회귀 위험 최소).
//   - 재귀: 참조 플로우 내부의 flow-node 도 먼저 확장한다(중첩 누적 접두사).
//
// 재배선 원리 — 엔드포인트 해석(endpoint resolution):
//
//	각 flow-node 인스턴스는 두 개의 경계 매핑을 노출한다.
//	  inMap[port]  = [(내부소비자노드, 포트) ...]  // 입력 포트로 들어온 메시지의 도착지
//	  outMap[port] = [(내부생산자노드, 포트) ...]  // 출력 포트로 나가는 메시지의 출발지
//	부모 와이어의 양 끝을 이 매핑으로 해석(소스 끝은 outMap, 타겟 끝은 inMap)하여
//	모든 (생산자 × 소비자) 조합을 직접 와이어로 합성한다. 이로써 일반 노드, 두 flow-node 의
//	직접 연결, 입력→출력 passthrough 가 단일 규칙으로 일관되게 처리된다.

// subflowNamespaceScheme 은 서브플로우 네임스페이스 접두사의 스킴(고정 토큰)이다.
// 확장된 서브그래프 노드/와이어 엔드포인트 ID 는 모두 이 토큰으로 시작한다.
//
// 네임스페이스 ID 정규형(WEB 팀 공유 규약):
//
//	subflow_<flowNodeID>_<originalID>
//
// 여기서
//   - "subflow_" 는 고정 접두 토큰(subflowNamespaceScheme).
//   - <flowNodeID> 는 부모 플로우 내 flow-node 노드의 ID(보통 UUID, 밑줄 없음).
//   - <originalID> 는 참조 서브플로우 정의 내 원본 노드 ID. 중첩 서브플로우인 경우
//     <originalID> 자체가 다시 "subflow_<childFlowNodeID>_<...>" 형태로 누적된다.
//
// 한 겹(immediate layer) 을 떼어내려면 ParseSubflowNodeID 를, 접두사 문자열을 만들려면
// SubflowNodeIDPrefix 를 사용한다. WEB 측은 동일한 규약을 복제하여 부모 뷰 집계(Fix 1)를
// 수행한다.
const subflowNamespaceScheme = "subflow_"

// subflowNamespacePrefix 는 확장된 서브그래프 노드/와이어 엔드포인트에 부여하는
// 네임스페이스 접두사를 만든다: "subflow_<flowNodeID>_".
// (SPEC-SUBFLOW-001 5.3 네임스페이스 규칙)
func subflowNamespacePrefix(flowNodeID string) string {
	return SubflowNodeIDPrefix(flowNodeID)
}

// SubflowNodeIDPrefix 는 flow-node ID 에 대한 서브플로우 네임스페이스 접두사를 반환한다:
//
//	"subflow_<flowNodeID>_"
//
// 부모 플로우에 배포된(평탄화된) 네임스페이스 노드 ID 는 모두 이 접두사로 시작한다.
// WEB 팀의 부모 뷰 집계(Fix 1)는 이 접두사 문자열을 그대로 복제하여, 특정 flow-node 의
// 직속 내부 노드(subflow_<flowNodeID>_*)를 모아 합산한다. ParseSubflowNodeID 의 역연산에
// 해당하는 생성기이며, 두 백엔드/프론트엔드가 동일한 규약을 공유하도록 노출한다.
func SubflowNodeIDPrefix(flowNodeID string) string {
	return subflowNamespaceScheme + flowNodeID + "_"
}

// ParseSubflowNodeID 는 네임스페이스 노드 ID 에서 한 겹(immediate layer)의
// "subflow_<flowNodeID>_" 접두사를 떼어낸다.
//
//	subflow_F_inner               → (flowNodeID="F", originalID="inner",          ok=true)
//	subflow_F_subflow_G_inner     → (flowNodeID="F", originalID="subflow_G_inner", ok=true)
//	inner                         → ("", "", false)  // 네임스페이스가 아님
//	subflow_inner                 → ("", "", false)  // 두 번째 구분자 없음
//	subflow__inner                → ("", "", false)  // flowNodeID 비어 있음
//	subflow_F_                    → ("", "", false)  // originalID 비어 있음
//
// 한 겹만 제거하므로, 중첩 서브플로우의 경우 originalID 가 다시 "subflow_..._..." 형태로
// 남는다. 이때 originalID 는 부모 flow-node F 가 참조하는 서브플로우 정의 안의 직속 노드
// (중첩 서브플로우라면 그 안의 flow-node) ID 에 해당한다 — 즉 한 겹 위 서브플로우의
// "원본 노드 ID" 이다. (WEB 팀 공유 규약: SubflowNodeIDPrefix 의 역연산.)
func ParseSubflowNodeID(id string) (flowNodeID, originalID string, ok bool) {
	rest, found := strings.CutPrefix(id, subflowNamespaceScheme)
	if !found {
		return "", "", false
	}
	// 접두 토큰 직후의 첫 번째 '_' 가 flowNodeID 와 originalID 의 경계이다.
	// flowNodeID 는 보통 UUID(밑줄 없음)이므로 첫 구분자 분할이 결정적이다.
	sep := strings.IndexByte(rest, '_')
	if sep < 0 {
		return "", "", false // 두 번째 구분자 없음(예: "subflow_inner").
	}
	flowNodeID = rest[:sep]
	originalID = rest[sep+1:]
	if flowNodeID == "" || originalID == "" {
		return "", "", false // 어느 한쪽이라도 비면 모호 — 네임스페이스로 간주하지 않는다.
	}
	return flowNodeID, originalID, true
}

// maxSubflowExpandDepth 는 서브플로우 확장 재귀의 최대 중첩 깊이이다.
// 순환은 DetectFlowReferenceCycle 로 이미 거부되지만, 폭주 방어를 위해 별도 상한을 둔다.
// (SPEC-SUBFLOW-001 REQ-SUBFLOW-N01)
const maxSubflowExpandDepth = 8

// maxSubflowExpandNodes 는 확장 결과 전체 노드 수의 안전 상한이다.
// 상한 초과 시 명확한 에러를 반환하여 메모리/시간 폭증을 방지한다.
// (SPEC-SUBFLOW-001 REQ-SUBFLOW-N01)
const maxSubflowExpandNodes = 5000

// DanglingWire 는 확장 과정에서 매칭되는 경계 와이어가 없어 드롭된 부모 와이어를 기록한다.
// (참조 포트가 제거되었거나, flow-node 핸들에 대응하는 내부 경계 와이어가 없는 경우.)
// (SPEC-SUBFLOW-001 REQ-SUBFLOW-C05 — dangling = 경고 + 정리)
type DanglingWire struct {
	WireID     string // 드롭된 부모 와이어의 ID
	FlowNodeID string // 관련 flow-node 노드 ID
	Handle     string // 매칭 실패한 핸들(포트) 이름
	Direction  string // "input" 또는 "output"
	Reason     string // 사람이 읽을 설명
}

// endpoint 는 와이어 한쪽 끝(노드 ID + 포트 이름)이다.
type endpoint struct {
	nodeID string
	port   string
}

// flowNodeBoundary 는 한 flow-node 인스턴스의 경계 포트 → 내부 엔드포인트 매핑이다.
type flowNodeBoundary struct {
	// inMap[입력포트이름] = 입력 메시지가 도착할 내부 소비자 엔드포인트들.
	inMap map[string][]endpoint
	// outMap[출력포트이름] = 출력 메시지가 출발할 내부 생산자 엔드포인트들.
	// 엔드포인트.nodeID 가 "" 이면 "이 출력은 입력 포트로부터의 passthrough"임을 의미하며,
	// .port 에 원본 입력 포트 이름이 담긴다(해석 시 입력 매핑으로 추가 전개).
	outMap map[string][]endpoint
}

// ExpandSubflows 는 f 의 LOCAL(bare id) flow-node 를 참조 플로우의 네임스페이스
// 인스턴스로 확장한 새 플로우를 반환한다(원본 f 는 변경하지 않는다).
//
//   - ctx: 취소 컨텍스트.
//   - f:   확장 대상 플로우(부모).
//   - repo: 참조 플로우 정의 조회용 저장소(항상 최신 — 결정 2).
//
// 참조 종류별 분기(reference-kind branch — SPEC-SUBFLOW-001 v1.3 그룹 RB, REQ-SUBFLOW-RB01):
//
//   - LOCAL 참조(bare id, 예 "flow-abc"): 결정 1 의 인스턴스화(배포 시 서브그래프 임베딩)
//     를 그대로 적용한다(불변, regression-0). 참조 플로우를 네임스페이스 복제·직접
//     재배선하여 인라인 확장하며, 결과에는 flow-node 가 남지 않는다.
//   - REMOTE 참조(remote://{instance_id}/{flow_id}): 확장하지 않는다. flow-node 를
//     LIVE NODE 로 그대로 남겨, P3 매니저 엔진이 이를 브리지 엔드포인트로 실행하도록
//     한다(REQ-SUBFLOW-RB07). flow_id(remote:// 참조)와 입출력 포트가 보존되어,
//     instance_id/remote_flow_id 분해와 이름 기반 경계 포트 매핑(RB06)에 사용된다.
//
// 결과 플로우에는 LOCAL flow-node 가 남지 않으며(확장 소비), REMOTE flow-node 는 그대로
// 살아남는다. 자기 자신의 top-level 플로우 포트 경계 와이어는 남아 있으며, 단독 배포
// 전처리 StripBoundaryWires 가 별도로 처리한다.
//
// 깊이/노드 수 상한 초과 시 에러를 반환한다(REQ-SUBFLOW-N01).
// LOCAL 참조 플로우가 저장소에 없으면 에러를 반환한다(REQ-SUBFLOW-F03).
// REMOTE 참조의 형식 오류(remote:// 구분자 누락 등)는 배포를 거부한다(REQ-SUBFLOW-R01).
//
// v1.2 SUPERSEDE: 원격 참조의 "배포 시 fetch + 매니저 인라인 확장"(RemoteFlowFetcher /
// ExpandSubflowsWithFetcher)은 v1.3 에서 폐기되었다(device/secret 무동작 한계 — §1.2
// 결정 5). 원격 참조는 이제 라이브 브리지(그룹 RB)로 동작하며 매니저는 정의를 확장하지
// 않는다.
func ExpandSubflows(ctx context.Context, f flow.Flow, repo storage.FlowRepository) (flow.Flow, error) {
	return expandSubflowsRec(ctx, f, repo, 0, nil)
}

// expandSubflowsRec 는 ExpandSubflows 의 재귀 본체이다.
func expandSubflowsRec(ctx context.Context, f flow.Flow, repo storage.FlowRepository, depth int, logger *slog.Logger) (flow.Flow, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if depth > maxSubflowExpandDepth {
		return nil, fmt.Errorf("서브플로우 확장 깊이 초과(최대 %d): flowID=%q", maxSubflowExpandDepth, f.ID())
	}

	srcNodes := f.Nodes()
	srcWires := f.Wires()

	// flow-node 가 하나도 없으면 확장할 것이 없다 — 원본을 그대로 반환한다(회귀 0, F02).
	hasFlowNode := false
	for _, n := range srcNodes {
		if n.Type == flowNodeType {
			hasFlowNode = true
			break
		}
	}
	if !hasFlowNode {
		return f, nil
	}

	resultNodes := make([]flow.NodeDef, 0, len(srcNodes))
	resultWires := make([]flow.Wire, 0, len(srcWires))

	// flow-node 를 참조 종류로 분류한다(REQ-SUBFLOW-RB01 — reference-kind branch).
	//
	//   - 비-flow-node: 결과에 그대로 보존한다.
	//   - LOCAL(bare id) flow-node: localFlowNodeIDs 에 등록한다(인스턴스화 대상). 와이어
	//     재배선 시 flow-node 핸들 → 내부 엔드포인트로 전개된다.
	//   - REMOTE(remote://) flow-node: 결과에 LIVE NODE 로 그대로 보존한다(미확장 —
	//     REQ-SUBFLOW-RB07). flowNodeIDs 에 등록하지 않으므로, 부모 와이어는 살아남은
	//     flow-node 핸들을 그대로 가리킨다(재배선 없음). flow_id 의 remote:// 참조와
	//     입출력 포트가 보존되어 P3 엔진이 브리지 엔드포인트로 실행한다.
	//
	// 형식 오류 remote:// 참조(구분자 누락 등)는 배포를 거부한다(REQ-SUBFLOW-R01).
	localFlowNodeIDs := make(map[string]bool)
	for _, n := range srcNodes {
		if n.Type != flowNodeType {
			resultNodes = append(resultNodes, n)
			continue
		}
		flowID, _ := n.Config[flowNodeFlowIDKey].(string)
		if flowID == "" {
			return nil, fmt.Errorf("flow-node %q: flow_id 가 비어 있습니다", n.ID)
		}
		_, _, isRemote, parseErr := parseRemoteFlowRef(flowID)
		if parseErr != nil {
			return nil, fmt.Errorf("flow-node %q: %w", n.ID, parseErr)
		}
		if isRemote {
			// 원격 참조 = 라이브 브리지(미확장). flow-node 를 그대로 남긴다.
			resultNodes = append(resultNodes, n)
			continue
		}
		// 로컬 참조 = 인스턴스화 대상.
		localFlowNodeIDs[n.ID] = true
	}

	wireSeq := 0 // 네임스페이스/재배선 와이어 ID 유일성 보장용 시퀀스

	// 각 LOCAL flow-node 를 인스턴스화하여 내부 노드/와이어를 누적하고, 경계 매핑을 기록한다.
	boundaries := make(map[string]flowNodeBoundary, len(localFlowNodeIDs))
	for _, n := range srcNodes {
		if n.Type != flowNodeType || !localFlowNodeIDs[n.ID] {
			continue
		}
		flowID, _ := n.Config[flowNodeFlowIDKey].(string)

		// 로컬 참조(bare id) — 기존 동작(하위 호환). 참조 플로우 조회(항상 최신).
		ref, err := repo.Get(ctx, flowID)
		if err != nil {
			return nil, fmt.Errorf("flow-node %q: 참조 플로우 조회 실패(flow_id=%q): %w", n.ID, flowID, err)
		}

		// 중첩 해소: 참조 플로우 내부의 flow-node 를 먼저 평탄화한다(누적 접두사 처리).
		expandedRef, err := expandSubflowsRec(ctx, ref, repo, depth+1, logger)
		if err != nil {
			return nil, err
		}

		nsNodes, nsWires, boundary := instantiateSubflow(n.ID, expandedRef, &wireSeq)
		resultNodes = append(resultNodes, nsNodes...)
		resultWires = append(resultWires, nsWires...)
		boundaries[n.ID] = boundary
	}

	// 모든 부모 와이어를 엔드포인트 해석으로 재배선한다.
	for _, w := range srcWires {
		// 경계 센티넬을 엔드포인트로 갖는 와이어(이 플로우 자신의 top-level 플로우 포트)는
		// 센티넬 끝을 보존한 채 실제 끝만 해석한다. 실제 끝이 flow-node 핸들이면 그 인스턴스의
		// 내부 엔드포인트로 전개하여, 재귀적으로 평탄화될 때 자식 flow-node 의 경계가 부모의
		// 플로우 포트와 올바르게 이어지도록 한다. (중첩 평탄화 핵심.)
		if flow.IsBoundaryWire(w) {
			rewritten, dangling := resolveBoundaryWire(w, localFlowNodeIDs, boundaries, &wireSeq)
			if dangling != nil {
				logDangling(logger, *dangling)
				continue
			}
			resultWires = append(resultWires, rewritten...)
			continue
		}

		// REMOTE flow-node 는 localFlowNodeIDs 에 없으므로 일반 노드로 취급되어, 와이어
		// 엔드포인트가 살아남은 flow-node 핸들을 그대로 가리킨다(재배선 없음 — RB07).
		sources, srcDangling := resolveSourceEndpoints(w, localFlowNodeIDs, boundaries)
		targets, tgtDangling := resolveTargetEndpoints(w, localFlowNodeIDs, boundaries)

		// dangling 경고(C05): 한쪽 끝이라도 flow-node 핸들 매칭에 실패하면 드롭.
		if srcDangling != nil {
			logDangling(logger, *srcDangling)
			continue
		}
		if tgtDangling != nil {
			logDangling(logger, *tgtDangling)
			continue
		}

		// (생산자 × 소비자) 카르테시안 곱으로 직접 와이어를 합성한다.
		for _, s := range sources {
			for _, t := range targets {
				wireSeq++
				resultWires = append(resultWires, flow.Wire{
					ID:           fmt.Sprintf("rewire_%s_%d", w.ID, wireSeq),
					Name:         w.Name,
					Type:         flow.WireSimple,
					SourceNodeID: s.nodeID,
					SourcePort:   s.port,
					TargetNodeID: t.nodeID,
					TargetPort:   t.port,
					Mode:         w.Mode,
					BufferSize:   w.BufferSize,
					Virtual:      w.Virtual,
					TTL:          w.TTL,
				})
			}
		}
	}

	// 노드 수 상한 검사(N01).
	if len(resultNodes) > maxSubflowExpandNodes {
		return nil, fmt.Errorf("서브플로우 확장 노드 수 초과(최대 %d, got %d): flowID=%q", maxSubflowExpandNodes, len(resultNodes), f.ID())
	}

	return flow.RebuildFlow(f, resultNodes, resultWires), nil
}

// logDangling 은 dangling 와이어 드롭을 경고 로깅한다(C05).
func logDangling(logger *slog.Logger, d DanglingWire) {
	logger.Warn("서브플로우 확장: dangling 와이어 드롭",
		"wireID", d.WireID,
		"flowNodeID", d.FlowNodeID,
		"handle", d.Handle,
		"direction", d.Direction,
		"reason", d.Reason,
	)
}

// resolveBoundaryWire 는 이 플로우 자신의 경계(센티넬) 와이어를 재작성한다.
// 센티넬 끝은 보존하고, 실제 끝이 flow-node 핸들이면 그 인스턴스의 내부 엔드포인트로 전개한다.
// 실제 끝이 일반 노드면 그대로(단일) 보존한다.
//
//   - 입력 경계(Source==FlowInputBoundaryID): Target 을 inMap 으로 전개하여
//     각 내부 소비자로 향하는 경계-입력 와이어들을 만든다.
//   - 출력 경계(Target==FlowOutputBoundaryID): Source 를 outMap(expandOut)으로 전개하여
//     각 내부 생산자에서 출발하는 경계-출력 와이어들을 만든다.
//
// flow-node 핸들 매칭 실패 시 dangling 을 반환한다.
func resolveBoundaryWire(w flow.Wire, flowNodeIDs map[string]bool, boundaries map[string]flowNodeBoundary, wireSeq *int) ([]flow.Wire, *DanglingWire) {
	mkWire := func(src, srcPort, tgt, tgtPort string) flow.Wire {
		*wireSeq++
		return flow.Wire{
			ID:           fmt.Sprintf("rewire_%s_%d", w.ID, *wireSeq),
			Name:         w.Name,
			Type:         flow.WireSimple,
			SourceNodeID: src,
			SourcePort:   srcPort,
			TargetNodeID: tgt,
			TargetPort:   tgtPort,
			Mode:         w.Mode,
			BufferSize:   w.BufferSize,
			Virtual:      w.Virtual,
			TTL:          w.TTL,
		}
	}

	// 입력 경계: Source 가 센티넬, Target 이 실제 끝.
	if w.SourceNodeID == flow.FlowInputBoundaryID && w.TargetNodeID != flow.FlowOutputBoundaryID {
		if !flowNodeIDs[w.TargetNodeID] {
			return []flow.Wire{w}, nil // 실제 끝이 일반 노드 — 그대로 보존.
		}
		eps := boundaries[w.TargetNodeID].inMap[w.TargetPort]
		if len(eps) == 0 {
			return nil, &DanglingWire{WireID: w.ID, FlowNodeID: w.TargetNodeID, Handle: w.TargetPort, Direction: "input",
				Reason: "참조 플로우에 매칭되는 입력 포트 경계 와이어가 없습니다"}
		}
		out := make([]flow.Wire, 0, len(eps))
		for _, ep := range eps {
			out = append(out, mkWire(flow.FlowInputBoundaryID, w.SourcePort, ep.nodeID, ep.port))
		}
		return out, nil
	}

	// 출력 경계: Target 이 센티넬, Source 가 실제 끝.
	if w.TargetNodeID == flow.FlowOutputBoundaryID && w.SourceNodeID != flow.FlowInputBoundaryID {
		if !flowNodeIDs[w.SourceNodeID] {
			return []flow.Wire{w}, nil // 실제 끝이 일반 노드 — 그대로 보존.
		}
		eps := expandOut(boundaries[w.SourceNodeID], w.SourcePort)
		if len(eps) == 0 {
			return nil, &DanglingWire{WireID: w.ID, FlowNodeID: w.SourceNodeID, Handle: w.SourcePort, Direction: "output",
				Reason: "참조 플로우에 매칭되는 출력 포트 경계 와이어가 없습니다"}
		}
		out := make([]flow.Wire, 0, len(eps))
		for _, ep := range eps {
			out = append(out, mkWire(ep.nodeID, ep.port, flow.FlowOutputBoundaryID, w.TargetPort))
		}
		return out, nil
	}

	// 양 끝이 모두 센티넬(입력 포트 → 출력 포트 직결)인 top-level passthrough 는 그대로 보존.
	return []flow.Wire{w}, nil
}

// resolveSourceEndpoints 는 와이어의 소스 끝을 실제 생산자 엔드포인트들로 해석한다.
//   - 일반 노드면 그대로 [(SourceNodeID, SourcePort)].
//   - flow-node 면 그 인스턴스의 outMap[SourcePort] 로 전개(passthrough 는 재귀 전개).
//
// 매칭 실패(핸들에 대응하는 내부 출력 경계 없음) 시 dangling 을 반환한다.
func resolveSourceEndpoints(w flow.Wire, flowNodeIDs map[string]bool, boundaries map[string]flowNodeBoundary) ([]endpoint, *DanglingWire) {
	if !flowNodeIDs[w.SourceNodeID] {
		return []endpoint{{nodeID: w.SourceNodeID, port: w.SourcePort}}, nil
	}
	b := boundaries[w.SourceNodeID]
	eps := expandOut(b, w.SourcePort)
	if len(eps) == 0 {
		return nil, &DanglingWire{
			WireID:     w.ID,
			FlowNodeID: w.SourceNodeID,
			Handle:     w.SourcePort,
			Direction:  "output",
			Reason:     "참조 플로우에 매칭되는 출력 포트 경계 와이어가 없습니다",
		}
	}
	return eps, nil
}

// resolveTargetEndpoints 는 와이어의 타겟 끝을 실제 소비자 엔드포인트들로 해석한다.
//   - 일반 노드면 그대로 [(TargetNodeID, TargetPort)].
//   - flow-node 면 그 인스턴스의 inMap[TargetPort] 로 전개.
func resolveTargetEndpoints(w flow.Wire, flowNodeIDs map[string]bool, boundaries map[string]flowNodeBoundary) ([]endpoint, *DanglingWire) {
	if !flowNodeIDs[w.TargetNodeID] {
		return []endpoint{{nodeID: w.TargetNodeID, port: w.TargetPort}}, nil
	}
	b := boundaries[w.TargetNodeID]
	eps := b.inMap[w.TargetPort]
	if len(eps) == 0 {
		return nil, &DanglingWire{
			WireID:     w.ID,
			FlowNodeID: w.TargetNodeID,
			Handle:     w.TargetPort,
			Direction:  "input",
			Reason:     "참조 플로우에 매칭되는 입력 포트 경계 와이어가 없습니다",
		}
	}
	return eps, nil
}

// expandOut 은 flow-node 출력 포트의 생산자 엔드포인트를 전개한다.
// passthrough 항목(nodeID=="")은 해당 입력 포트의 소비자로 재귀 전개하지 않고,
// 입력 매핑은 부모 와이어의 소스가 직접 연결되므로 여기서는 빈 결과로 처리해야 한다.
// 단, 부모 소스가 flow-node 가 아닌 일반 노드인 경우는 호출자(resolveSource)에서
// 이미 일반 처리되므로, passthrough 전개는 "출력 포트가 입력 포트로 직결"된 경우의
// 내부 생산자 부재를 의미한다. 이때는 입력 포트의 내부 소비자(inMap)로 대체 전개한다.
func expandOut(b flowNodeBoundary, port string) []endpoint {
	raw := b.outMap[port]
	if len(raw) == 0 {
		return nil
	}
	var out []endpoint
	for _, ep := range raw {
		if ep.nodeID != "" {
			out = append(out, ep)
			continue
		}
		// passthrough: 출력 포트가 입력 포트(ep.port)로 직결됨.
		// 이 경우 "생산자"는 그 입력 포트의 내부 소비자들이다(입력→출력 직통).
		out = append(out, b.inMap[ep.port]...)
	}
	return out
}

// instantiateSubflow 는 단일 flow-node(flowNodeID)에 대해 참조 플로우 ref 의 노드+와이어를
// 네임스페이스 복제하고, 경계 포트 매핑을 산출한다.
//
// 반환:
//   - nsNodes: 네임스페이스 접두사가 부여된 ref 의 노드 복제본.
//   - nsWires: 네임스페이스 접두사가 부여된 ref 의 내부(비-경계) 와이어 복제본.
//   - boundary: 입력/출력 포트 → 내부 엔드포인트 매핑(부모 와이어 재배선에 사용).
//
// wireSeq 는 와이어 ID 유일성 보장용 카운터(포인터로 누적).
func instantiateSubflow(flowNodeID string, ref flow.Flow, wireSeq *int) (nsNodes []flow.NodeDef, nsWires []flow.Wire, boundary flowNodeBoundary) {
	prefix := subflowNamespacePrefix(flowNodeID)
	boundary = flowNodeBoundary{
		inMap:  make(map[string][]endpoint),
		outMap: make(map[string][]endpoint),
	}

	// 1) ref 의 노드를 네임스페이스 복제한다(경계 센티넬은 실제 노드가 아니라 노드 목록에 없음).
	for _, rn := range ref.Nodes() {
		nsNodes = append(nsNodes, cloneNodeWithPrefix(rn, prefix))
	}

	// 2) ref 의 와이어를 분류한다.
	for _, w := range ref.Wires() {
		switch {
		case w.SourceNodeID == flow.FlowInputBoundaryID && w.TargetNodeID == flow.FlowOutputBoundaryID:
			// 입력 포트 → 출력 포트 직결(passthrough).
			//   입력 포트(SourcePort) 의 소비자는 곧 출력 포트(TargetPort) 의 생산자로 직결된다.
			//   outMap 에 passthrough 마커(nodeID="")를 두어 해석 시 입력 매핑으로 전개한다.
			boundary.outMap[w.TargetPort] = append(boundary.outMap[w.TargetPort], endpoint{nodeID: "", port: w.SourcePort})
		case w.SourceNodeID == flow.FlowInputBoundaryID:
			// 입력 경계: 입력 포트(SourcePort) → 내부 소비자(prefix+Target, TargetPort).
			boundary.inMap[w.SourcePort] = append(boundary.inMap[w.SourcePort], endpoint{
				nodeID: prefix + w.TargetNodeID,
				port:   w.TargetPort,
			})
		case w.TargetNodeID == flow.FlowOutputBoundaryID:
			// 출력 경계: 내부 생산자(prefix+Source, SourcePort) → 출력 포트(TargetPort).
			boundary.outMap[w.TargetPort] = append(boundary.outMap[w.TargetPort], endpoint{
				nodeID: prefix + w.SourceNodeID,
				port:   w.SourcePort,
			})
		default:
			// 내부 와이어: 네임스페이스 복제.
			nsWires = append(nsWires, cloneWireWithPrefix(w, prefix, wireSeq))
		}
	}

	return nsNodes, nsWires, boundary
}

// cloneNodeWithPrefix 는 참조 플로우 노드를 네임스페이스 접두사를 부여하여 깊은 복제한다.
// Type/Name/Config/Enabled/Inputs/Outputs/Errors/AgentRef/Metadata 를 모두 보존하고
// ID 만 접두사를 부여한다. Config/Metadata/포트 슬라이스는 별칭 방지를 위해 복사한다.
// (SPEC-SUBFLOW-001 5.3 — 노드 ID 네임스페이스, REQ-SUBFLOW-D06 추적성)
func cloneNodeWithPrefix(n flow.NodeDef, prefix string) flow.NodeDef {
	cp := flow.NodeDef{
		ID:      prefix + n.ID,
		Name:    n.Name,
		Type:    n.Type,
		Enabled: n.Enabled,
	}

	if n.Config != nil {
		cfg := make(map[string]any, len(n.Config))
		for k, v := range n.Config {
			cfg[k] = v
		}
		cp.Config = cfg
	}
	cp.Inputs = clonePorts(n.Inputs)
	cp.Outputs = clonePorts(n.Outputs)
	cp.Errors = clonePorts(n.Errors)

	if n.AgentRef != nil {
		ref := *n.AgentRef
		cp.AgentRef = &ref
	}
	if n.Metadata != nil {
		md := make(map[string]string, len(n.Metadata))
		for k, v := range n.Metadata {
			md[k] = v
		}
		cp.Metadata = md
	}
	return cp
}

// clonePorts 는 포트 슬라이스를 방어적으로 복사한다.
func clonePorts(ports []flow.Port) []flow.Port {
	if ports == nil {
		return nil
	}
	out := make([]flow.Port, len(ports))
	copy(out, ports)
	return out
}

// cloneWireWithPrefix 는 참조 플로우 내부 와이어를 네임스페이스 접두사를 부여하여 복제한다.
// 엔드포인트(SourceNodeID/TargetNodeID)에 접두사를 부여하고, 와이어 ID 도 접두사+시퀀스로
// 재생성하여 유일성을 보장한다. Mode/BufferSize/Virtual/TTL/Name/Type 은 보존한다.
// (SPEC-SUBFLOW-001 5.3 — 와이어 엔드포인트/ID 네임스페이스, Mode/Buffer/Virtual 보존)
func cloneWireWithPrefix(w flow.Wire, prefix string, wireSeq *int) flow.Wire {
	*wireSeq++
	return flow.Wire{
		ID:           fmt.Sprintf("%s%s_%d", prefix, w.ID, *wireSeq),
		Name:         w.Name,
		Type:         w.Type,
		SourceNodeID: prefix + w.SourceNodeID,
		SourcePort:   w.SourcePort,
		TargetNodeID: prefix + w.TargetNodeID,
		TargetPort:   w.TargetPort,
		Mode:         w.Mode,
		BufferSize:   w.BufferSize,
		Virtual:      w.Virtual,
		TTL:          w.TTL,
	}
}
