package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/xtra/xflow/internal/api/handler"
	"github.com/xtra/xflow/pkg/flow"
)

// ---------------------------------------------------------------------------
// 원격 서브플로우 참조 — SPEC-SUBFLOW-001 v1.2 그룹 R
// ---------------------------------------------------------------------------
//
// LOCAL 플로우의 flow-node 가 원격 관리 노드(SPEC-REMOTE-001 managed node)의 플로우를
// 참조하고, 배포 시점에 항상 최신으로 해석·확장한다. flow-node 의 flow_id 가
// "remote://{instance_id}/{flow_id}" 정규형이면 원격 참조이고(REQ-SUBFLOW-R01),
// bare id 이면 LOCAL 참조이다(하위 호환).
//
// 해석 경로(어댑터 사전 확장, 엔진 비침습 — §5.11):
//   1. parseRemoteFlowRef 로 (instanceID, remoteFlowID) 파싱.
//   2. RemoteFlowFetcher.FetchRemoteFlow 로 redacted 정의 fetch(항상 최신, R03).
//   3. deserializeRemoteFlow 로 flow.Flow 역직렬화(A11 — 기존 직렬화 경로 재사용).
//   4. 중첩 flow-node 포함 검사 → 포함 시 거부(R08, self-contained 경계).
//   5. 로컬 서브플로우와 동일 규칙으로 인라인 확장(네임스페이스·직접 재배선·상한).

// remoteFlowRefScheme 은 원격 참조 정규형의 스킴 접두사이다(REQ-SUBFLOW-R01).
const remoteFlowRefScheme = "remote://"

// RemoteFlowFetcher 는 배포 시점에 원격 관리 노드의 플로우 정의를 가져오는 해석기이다.
//
// 구현체는 매니저 서버가 보유한 *remote.Server 위에서
// Server.DispatchQuery(ctx, instanceID, "flow", "get", {"id": flowID}) 를 호출하여
// redacted 정의 JSON(= handler.FlowInfo 마샬)을 반환한다(SPEC-REMOTE-001 REQ-J01/J04).
//
// 인터페이스를 service 패키지에 두어 import cycle 을 피한다: 구체 구현체는
// remote.Server 와 service 를 모두 아는 와이어링 계층(cmd/xflowd)에서 주입한다.
// (service 패키지는 internal/remote 를 import 하지 않는다.)
//
// 반환:
//   - definitionJSON: flow/get 응답 바이트(handler.FlowInfo 마샬). deserializeRemoteFlow 가 파싱.
//   - err: fetch 실패(노드 오프라인/미관리 503·타임아웃 504·원격 플로우 없음/노드 오류 502).
//     실패 시 배포는 거부되어야 하며 stale/empty 정의를 무음으로 사용하지 않는다(REQ-SUBFLOW-R06).
type RemoteFlowFetcher interface {
	FetchRemoteFlow(ctx context.Context, instanceID, flowID string) (definitionJSON []byte, err error)
}

// parseRemoteFlowRef 는 flow-node 의 flow_id 를 판별·파싱한다(REQ-SUBFLOW-R01).
//
//   - bare id("flow-abc") → isRemote=false. 기존 LOCAL 동작(repo.Get)으로 해석.
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
	// flow_id 에 슬래시가 포함될 수 있으므로 SplitN(.., 2) 로 첫 슬래시만 사용한다.
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

// deserializeRemoteFlow 는 query 프록시 flow/get 응답 바이트(handler.FlowInfo 마샬)를
// flow.Flow 로 역직렬화한다(REQ-SUBFLOW-R02, A11).
//
// flow/get 응답의 Config 필드는 flowToReactFlowConfig 가 만든 React Flow 모양
// (nodes[data], edges, 최상위 inputs/outputs)이다. 이를 로컬 정의 생성 경로
// (normalizeReactFlowDefinition → flow.FlowFromJSON)에 그대로 통과시켜, 원격 정의가
// 로컬 서브플로우와 동일한 확장 알고리즘으로 처리되도록 한다.
//
// instanceID/remoteFlowID 는 에러 메시지의 식별용으로만 사용한다.
func deserializeRemoteFlow(definitionJSON []byte, instanceID, remoteFlowID string) (flow.Flow, error) {
	var info handler.FlowInfo
	if err := json.Unmarshal(definitionJSON, &info); err != nil {
		return nil, fmt.Errorf("원격 플로우 역직렬화 실패(remote://%s/%s): FlowInfo 디코드: %w", instanceID, remoteFlowID, err)
	}
	if info.Config == nil {
		return nil, fmt.Errorf("원격 플로우 정의가 비어 있습니다(remote://%s/%s): config 누락", instanceID, remoteFlowID)
	}

	// FlowInfo.Config(React Flow 모양)에 id/name/description 을 주입하여 완전한 정의를 만든다.
	def := make(map[string]any, len(info.Config)+3)
	for k, v := range info.Config {
		def[k] = v
	}
	// 원격 flow id 를 보존한다(확장 네임스페이스/추적성에는 flow-node ID 를 쓰므로 무관하나,
	// 정의 자체의 정체성을 유지한다).
	def["id"] = remoteFlowID
	if info.Name != "" {
		def["name"] = info.Name
	} else {
		def["name"] = remoteFlowID
	}
	if info.Description != "" {
		def["description"] = info.Description
	}

	// 로컬 정의 생성 경로와 동일하게 React Flow → XFlow 정규화 후 역직렬화한다.
	def = normalizeReactFlowDefinition(def)

	data, err := json.Marshal(def)
	if err != nil {
		return nil, fmt.Errorf("원격 플로우 역직렬화 실패(remote://%s/%s): marshal: %w", instanceID, remoteFlowID, err)
	}
	f, err := flow.FlowFromJSON(data)
	if err != nil {
		return nil, fmt.Errorf("원격 플로우 역직렬화 실패(remote://%s/%s): %w", instanceID, remoteFlowID, err)
	}
	return f, nil
}

// flowContainsFlowNode 는 플로우 정의가 그 자체로 flow-node(로컬/원격 불문)를 포함하는지
// 검사한다(REQ-SUBFLOW-R08 중첩 거부용). 매니저가 원격 그래프를 완전 순회할 수 없어
// 분산 순환 검출이 불가하므로, fetch 된 원격 정의는 self-contained 여야 한다.
func flowContainsFlowNode(f flow.Flow) bool {
	if f == nil {
		return false
	}
	for _, n := range f.Nodes() {
		if n.Type == flowNodeType {
			return true
		}
	}
	return false
}
