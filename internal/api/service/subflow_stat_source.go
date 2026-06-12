package service

import "github.com/xtra/xflow/internal/engine"

// ---------------------------------------------------------------------------
// 통계 출처(stat source) 표식 — SPEC-SUBFLOW-002 M4 그룹 S(REQ-SUBFLOW2-S03)
// ---------------------------------------------------------------------------
//
// 서브플로우 화면(GET /flows/{id}/nodes = ListFlowNodes)의 노드 통계는 두 출처에서 합산된다.
//
//   1) direct(직접 실행)   — engine.GetFlowNodes(flowID) 의 단독/공유 실행 인스턴스 통계.
//      참조 플로우가 자기 자신으로 실행될 때의 통계이며, mode=shared 부모가 in-process 라이브
//      브리지로 라우팅한 트래픽은 단일 공유 인스턴스에 이미 반영되어 여기에 포함된다(S01).
//   2) embedded(임베디드 병합) — mode=instance 부모 안의 네임스페이스 복제본(subflow_<F>_<orig>)
//      통계를 원본 노드 단위로 역매핑·합산한 값(기존 subflow-stats 병합, S02).
//
// shared 부모는 인라인 확장하지 않아 네임스페이스 노드를 만들지 않으므로 embedded 에 기여하지
// 않는다(localFlowNodesReferencing 의 instance-only 필터). 따라서 shared 트래픽은 direct 에만
// 한 번 계상되고 embedded 와 이중계상되지 않는다(요구사항 3 — 이중계상 방지).
//
// S03 은 운영자가 "이 숫자가 참조 플로우 직접 실행(shared)인지, 인라인 복제본 병합(instance)인지,
// 둘의 혼합인지"를 화면에서 식별하도록 요구한다. 과설계를 피하기 위해, 노드 인스턴스 응답의
// 기존 Extra 맵에 단일 문자열 표식만 추가한다(별도 DTO 필드/엔드포인트 없음). 프론트(M5)는 이
// 표식으로 모드 배지를 렌더한다.

const (
	// statSourceKey 는 노드 인스턴스 Extra 맵에서 통계 출처 표식을 담는 키이다(S03).
	statSourceKey = "stat_source"

	// statSourceDirect 는 통계가 참조 플로우의 직접/공유 실행(GetFlowNodes(flowID))에서만
	// 나온 노드를 나타낸다(shared 또는 단독 배포 — S01).
	statSourceDirect = "direct"

	// statSourceEmbedded 는 통계가 instance 부모의 임베디드 병합에서만 나온 노드를 나타낸다(S02).
	statSourceEmbedded = "embedded"

	// statSourceMixed 는 한 원본 노드의 통계가 direct 와 embedded 양쪽에서 합산된 혼합 출처를
	// 나타낸다(같은 참조 플로우를 어떤 부모는 shared, 어떤 부모는 instance 로 참조 — 요구사항 3).
	statSourceMixed = "direct+embedded"
)

// classifyStatSource 는 한 원본 노드 ID 가 direct/embedded 어느 출처에 속하는지로 출처 표식을
// 결정한다(순수 함수). 양쪽 모두면 혼합, 한쪽이면 해당 출처를 반환한다. 둘 다 아니면(논리상
// 발생하지 않음 — 병합 입력에 없던 노드) 빈 문자열을 반환하여 호출자가 표식을 생략하게 한다.
func classifyStatSource(inDirect, inEmbedded bool) string {
	switch {
	case inDirect && inEmbedded:
		return statSourceMixed
	case inDirect:
		return statSourceDirect
	case inEmbedded:
		return statSourceEmbedded
	default:
		return ""
	}
}

// tagStatSources 는 병합 결과(merged)의 각 노드 Extra 에 통계 출처 표식을 부여한다(S03).
//
// direct/embedded 는 병합 전 두 입력 슬라이스이며, 이로부터 원본 노드 ID 집합을 만들어 각 병합
// 노드의 출처를 분류한다(classifyStatSource). 입력 슬라이스 자체는 변경하지 않으며, merged 노드의
// Extra 맵만 갱신한다(없으면 생성). 결정적이며 부수효과는 Extra 표식 추가뿐이다.
//
// 호출자(ListFlowNodes)는 ownNodes(=direct), embedded 와 그 병합 결과를 넘긴다. shared 트래픽은
// direct 에만 존재하므로 표식이 자연히 direct/혼합으로 잡히고, instance 전용 노드는 embedded 로
// 잡힌다.
func tagStatSources(merged, direct, embedded []engine.NodeInstanceInfo) []engine.NodeInstanceInfo {
	directIDs := make(map[string]bool, len(direct))
	for _, n := range direct {
		directIDs[n.NodeID] = true
	}
	embeddedIDs := make(map[string]bool, len(embedded))
	for _, n := range embedded {
		embeddedIDs[n.NodeID] = true
	}

	for i := range merged {
		src := classifyStatSource(directIDs[merged[i].NodeID], embeddedIDs[merged[i].NodeID])
		if src == "" {
			continue
		}
		if merged[i].Extra == nil {
			merged[i].Extra = make(map[string]any, 1)
		}
		merged[i].Extra[statSourceKey] = src
	}
	return merged
}
