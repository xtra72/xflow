package node

import (
	"fmt"

	"github.com/xtra/xflow/pkg/flow"
)

// flowNodeConfigFlowID 는 flow-node 가 참조하는 플로우 id 를 담는 Config 키이다.
// (SPEC-SUBFLOW-001 REQ-SUBFLOW-C01, 5.2 데이터 계약)
const flowNodeConfigFlowID = "flow_id"

// flowNodeConfigMode 는 flow-node 의 참조 실행 모드를 담는 Config 키이다(SPEC-SUBFLOW-002
// REQ-SUBFLOW2-M01). 값은 "shared"(공유 단일 인스턴스 라이브 브리지) 또는 "instance"(인라인
// 확장 복제본)이며, 미지정 시 "shared" 로 해석된다(M02 — breaking). 실제 정규화/분기는 배포
// 어댑터(internal/api/service)가 수행한다.
const flowNodeConfigMode = "mode"

// NewFlowNodePlaceholder 는 flow-node 타입의 안전망(safety-net) 팩토리이다.
//
// flow-node 는 "다른 플로우를 참조하는 서브플로우 노드"이며 실제 런타임 동작을 갖지 않는다.
// 정상 배포 경로에서는 배포 직전 서비스 레이어가 flow-node 를 참조 플로우의 노드+와이어로
// 네임스페이스 확장(인스턴스화)하여 치환하므로, 엔진이 flow-node 를 직접 인스턴스화하는
// 일은 발생하지 않는다(SPEC-SUBFLOW-001 결정 1 — 배포 시 서브그래프 확장).
//
// 따라서 본 팩토리는 다음 두 가지 목적만 갖는다.
//  1. 레지스트리 등록을 통해 flow-node 타입을 팔레트(GET /nodes)에 노출하고 계약을 예약한다.
//  2. 확장 단계가 누락된 채 flow-node 가 엔진까지 도달한 경우(설정 오류/회귀)를 즉시
//     에러로 드러내는 안전망 역할을 한다.
//
// 참조 플로우 id 는 Config["flow_id"] 로 전달된다. 확장이 선행되었다면 이 팩토리는
// 절대 호출되지 않는다.
//
// (SPEC-SUBFLOW-001 마일스톤 2 — 타입 등록/계약 예약. 확장 로직은 다음 마일스톤 소관.)
func NewFlowNodePlaceholder(def flow.NodeDef, _ ...NodeOption) (Node, error) {
	flowID, _ := def.Config[flowNodeConfigFlowID].(string)
	return nil, fmt.Errorf("flow-node must be expanded before deployment (flow_id=%q)", flowID)
}
