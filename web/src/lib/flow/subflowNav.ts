// 서브플로우 네비게이션 백 스택(들어가기 / 돌아가기) 순수 헬퍼.
//
// "들어가기"로 진입할 때마다 직전(부모) 플로우 id 를 스택에 쌓고,
// "돌아가기"로 가장 최근 부모 플로우로 되돌아간다. 중첩(A→B→C)을 지원하며,
// 스택은 react-router 의 location.state.subflowBack(string[]) 으로만 운반된다
// (스토어/영속화 없음 — 새로고침 시 손실되어 돌아가기가 숨겨지는 것은 허용).

/** location.state.subflowBack 에 보관하는 백 스택 키 */
export const SUBFLOW_BACK_STATE_KEY = 'subflowBack';

/**
 * 알 수 없는 location.state 에서 백 스택을 안전하게 읽어온다.
 *
 * - state 가 객체가 아니거나 subflowBack 이 배열이 아니면 빈 배열을 반환한다.
 * - 배열 요소 중 비어 있지 않은 문자열만 남겨 방어적으로 정규화한다.
 */
export function readBackStack(state: unknown): string[] {
  if (!state || typeof state !== 'object') return [];
  const raw = (state as Record<string, unknown>)[SUBFLOW_BACK_STATE_KEY];
  if (!Array.isArray(raw)) return [];
  return raw.filter((id): id is string => typeof id === 'string' && id !== '');
}

/**
 * "들어가기" 시 새 백 스택을 만든다.
 * 현재(부모) 플로우 id 를 기존 스택 끝에 추가한다.
 *
 * @param stack 현재 백 스택
 * @param currentFlowId 들어가기 직전(부모) 플로우 id
 */
export function pushBackStack(stack: string[], currentFlowId: string): string[] {
  if (!currentFlowId) return [...stack];
  return [...stack, currentFlowId];
}

/**
 * "돌아가기" 시 직전 플로우 id 와 그 이후의 스택을 계산한다.
 *
 * @returns prev 직전(돌아갈) 플로우 id, 없으면 null
 * @returns rest prev 를 제거한 나머지 스택(돌아갈 플로우의 새 백 스택)
 */
export function popBackStack(stack: string[]): {
  prev: string | null;
  rest: string[];
} {
  const rest = [...stack];
  const prev = rest.pop() ?? null;
  return { prev, rest };
}
