package service

import (
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// 원격 서브플로우 참조 — 데이터 모델 (SPEC-SUBFLOW-001 R01, v1.3 보존)
// ---------------------------------------------------------------------------
//
// LOCAL 플로우의 flow-node 가 원격 관리 노드(SPEC-REMOTE-001 managed node)의 플로우를
// 참조할 수 있다. flow-node 의 flow_id 가 "remote://{instance_id}/{flow_id}" 정규형이면
// 원격 참조이고(REQ-SUBFLOW-R01), bare id 이면 LOCAL 참조이다(하위 호환).
//
// v1.2 → v1.3 SUPERSEDE: v1.2 의 "배포 시 fetch + 매니저 인라인 확장"(RemoteFlowFetcher,
// deserializeRemoteFlow, flowContainsFlowNode)은 device/secret 무동작 한계로 폐기되었다
// (§1.2 결정 5). 원격 참조는 이제 라이브 브리지(그룹 RB)로 동작한다 — 매니저는 정의를
// fetch·확장·실행하지 않고, 원격 노드가 참조 플로우를 자기 디바이스/에이전트로 실행하며,
// 로컬 flow-node 는 기존 remote WS 세션 위의 브리지 엔드포인트가 된다.
//
// 따라서 본 파일은 v1.3 에서 데이터 모델 파서(parseRemoteFlowRef)만 보존한다. 이 파서는
// (1) ExpandSubflows 의 참조 종류 분기(REMOTE = 미확장·라이브 노드 유지 — REQ-SUBFLOW-RB01)
// 와 (2) P3 브리지 open(remote:// → instance_id/remote_flow_id 분해 — REQ-SUBFLOW-RB05)
// 양쪽에서 공유 규약으로 재사용된다.

// remoteFlowRefScheme 은 원격 참조 정규형의 스킴 접두사이다(REQ-SUBFLOW-R01).
const remoteFlowRefScheme = "remote://"

// parseRemoteFlowRef 는 flow-node 의 flow_id 를 판별·파싱한다(REQ-SUBFLOW-R01).
//
//   - bare id("flow-abc") → isRemote=false. LOCAL 참조(repo.Get 으로 인라인 확장).
//   - 정규형("remote://{instance_id}/{flow_id}") → isRemote=true, instanceID/remoteFlowID 추출.
//
// 정규형이지만 instance_id 또는 flow_id 가 비어 있거나 슬래시 구분자가 없으면
// isRemote=true 와 함께 에러를 반환한다(모호성 없는 결정적 판별).
func parseRemoteFlowRef(flowID string) (instanceID, remoteFlowID string, isRemote bool, err error) {
	if !strings.HasPrefix(flowID, remoteFlowRefScheme) {
		// bare id — LOCAL 참조(빈 문자열 포함; 빈 flow_id 는 호출자가 별도 검증).
		return "", "", false, nil
	}

	rest := strings.TrimPrefix(flowID, remoteFlowRefScheme)
	// 첫 번째 슬래시로 instance_id 와 flow_id 를 분리한다.
	// flow_id 에 슬래시가 포함될 수 있으므로 첫 슬래시만 사용한다.
	slash := strings.Index(rest, "/")
	if slash < 0 {
		return "", "", true, fmt.Errorf("원격 참조 형식 오류(구분자 '/' 누락): %q (형식: remote://{instance_id}/{flow_id})", flowID)
	}
	instanceID = rest[:slash]
	remoteFlowID = rest[slash+1:]
	if instanceID == "" {
		return "", "", true, fmt.Errorf("원격 참조 형식 오류(instance_id 비어 있음): %q", flowID)
	}
	if remoteFlowID == "" {
		return "", "", true, fmt.Errorf("원격 참조 형식 오류(flow_id 비어 있음): %q", flowID)
	}
	return instanceID, remoteFlowID, true, nil
}
