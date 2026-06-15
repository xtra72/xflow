// agent_id_resolver.go (SPEC-DEVICE-IDENTITY-001 — device_id 정규화)
//
// 문제: device_id / device_info 의 저장소 키는 (agentRef, unitID) 형식인데,
// agentRef 가 어떤 호출에선 에이전트 "이름"("LG HVACR2"), 어떤 호출에선
// 에이전트 "ID"(UUID, engine.go 가 노드 config 에 주입) 로 전달되어, 같은 물리
// 디바이스에 서로 다른 device_id 가 발급된다.
//
// 해결: agentRef 를 항상 에이전트의 정본 ID 로 정규화하는 패키지-레벨 resolver 를
// 추가한다. ResolveDeviceID 와 device_info 의 읽기/쓰기 진입점이 모두 이 정규화를
// 1회 거치게 하여, 호출처 25곳을 개별 수정하지 않고도 읽기/쓰기 키를 일치시킨다.
//
// 와이어링: cmd/xflowd/main.go 가 startup 시 SetAgentIDResolver 로 주입
// (에이전트 매니저/레지스트리의 GetByName 으로 이름→ID 변환).
// 미설정 (nil) 상태에서도 동작이 깨지지 않도록 ref 를 그대로 반환 (graceful).

package agent

import "sync"

// AgentIDResolver 는 에이전트 참조(이름 또는 ID)를 정본 ID 로 변환한다.
// ref 가 등록된 에이전트 "이름" 이면 그 에이전트의 ID 와 true 를 반환하고,
// 이미 ID 이거나 못 찾으면 ("", false) 를 반환한다 (호출자가 ref 폴백).
type AgentIDResolver func(ref string) (id string, ok bool)

var (
	agentIDResolverMu sync.RWMutex
	agentIDResolver   AgentIDResolver
)

// SetAgentIDResolver 는 패키지-레벨 에이전트 ID resolver 를 설정한다.
// 일반적으로 main.go 의 startup 코드에서 단 한 번 호출한다. nil 도 허용
// (정규화 비활성화 — 기존처럼 ref 를 그대로 키로 사용).
func SetAgentIDResolver(r AgentIDResolver) {
	agentIDResolverMu.Lock()
	defer agentIDResolverMu.Unlock()
	agentIDResolver = r
}

// GetAgentIDResolver 는 현재 설정된 resolver 를 반환한다 (없으면 nil).
func GetAgentIDResolver() AgentIDResolver {
	agentIDResolverMu.RLock()
	defer agentIDResolverMu.RUnlock()
	return agentIDResolver
}

// normalizeAgentRef 는 agentRef(이름 또는 ID)를 정본 에이전트 ID 로 정규화한다.
//
// 정규화 규칙:
//   - ref 가 빈 문자열이면 그대로 빈 문자열 반환.
//   - resolver 미주입(nil)이면 ref 를 그대로 반환 (graceful degradation).
//   - resolver 가 ref 를 등록된 이름으로 인식하면 해당 ID 반환.
//   - 그 외(이미 ID 이거나 미등록)는 ref 를 그대로 반환 (안전 폴백).
//
// 이 함수는 device_id / device_info 의 패키지-레벨 진입점 내부에서만 호출되어,
// 읽기/쓰기 양쪽 키가 항상 동일한 기준(에이전트 ID)으로 맞춰지도록 한다.
func normalizeAgentRef(ref string) string {
	if ref == "" {
		return ""
	}
	resolver := GetAgentIDResolver()
	if resolver == nil {
		return ref
	}
	if id, ok := resolver(ref); ok && id != "" {
		return id
	}
	return ref
}
