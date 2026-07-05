// Store 에이전트 ID → 현재 이름 해석 유틸 (SPEC-WEB-006).
//
// 배경: Store API 는 이름 주소(`/store/{agent_name}/...`)이고 백엔드는 rename 시
// 시계열 데이터를 이관한다. 그러나 대시보드 config 에 에이전트 "이름"만 저장하면,
// 에이전트 이름을 바꿨을 때 저장된 옛 이름이 무효화되어 패널 데이터 연결이 끊긴다.
//
// 해결: config 에는 안정적인 agent ID 를 정본으로 저장하고, Store API 를 호출하는
// 모든 지점에서 저장된 agent_id → 현재 에이전트 이름으로 해석한 뒤 호출한다. 기존
// 이름 필드는 표시용 스냅샷 + 하위호환 폴백으로 유지한다.
//
// @spec SPEC-WEB-006

/** 이름 해석에 필요한 최소 에이전트 형상(AgentInfo 의 부분집합). */
export interface StoreAgentRef {
  id: string;
  name: string;
}

/**
 * 저장된 agent ID 를 현재 에이전트 이름으로 해석한다.
 *
 * - `agentId` 가 있고 `agents` 목록에서 매칭되면 그 에이전트의 현재 이름을 반환한다
 *   (에이전트 이름이 바뀌었더라도 항상 현재 이름).
 * - `agentId` 가 없거나(구 config) 목록에 없으면 `fallbackName`(저장된 이름 스냅샷)을
 *   그대로 반환한다.
 *
 * 빈 문자열 `fallbackName` 은 "미선택" 을 의미하므로 그대로 반환한다(호출 측이
 * 비활성 처리).
 *
 * @param agentId 저장된 안정적 agent ID(정본). undefined 가능.
 * @param fallbackName 저장된 에이전트 이름 스냅샷(하위호환 폴백).
 * @param agents 현재 에이전트 목록(id/name 필드 필요).
 * @returns Store API 호출에 사용할 현재 에이전트 이름.
 */
export function resolveStoreAgentName(
  agentId: string | undefined,
  fallbackName: string,
  agents: readonly StoreAgentRef[] | undefined,
): string {
  if (agentId && agents) {
    const match = agents.find((a) => a.id === agentId);
    if (match) return match.name;
  }
  return fallbackName;
}
