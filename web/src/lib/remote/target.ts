// 자원 타깃 추상화 (SPEC-REMOTE-001 M8, 그룹 J, REQ-J09/J13).
//
// `target` 은 목록/제어 페이지가 로컬 자원과 원격 노드 자원을 동일 코드로
// 다루기 위한 식별자다. `useEditorFlowTarget`(M7)의 local-vs-remote 분기를
// 목록/상세 페이지로 일반화한 것이다.
//
//   - 로컬:  { type: 'local' }              — 기존 로컬 API/실시간 그대로.
//   - 원격:  { type: 'remote', instanceId } — READ/QUERY 프록시 + SSE 스트림.
//
// URL 직렬화 형식(REQ-J13): `?target=remote:{instanceId}` (미지정/`local` = 로컬).
// instanceId 는 인코딩되어 운반되며 파싱 시 디코딩된다.

/** 로컬 vs 원격 노드 자원 타깃. */
export type ResourceTarget =
  | { type: 'local' }
  | { type: 'remote'; instanceId: string };

/** 로컬 타깃 싱글턴(참조 안정성 — 불필요한 재렌더 방지). */
export const LOCAL_TARGET: ResourceTarget = { type: 'local' };

/** target 이 원격인지 여부(타입 가드). */
export function isRemoteTarget(
  target: ResourceTarget,
): target is { type: 'remote'; instanceId: string } {
  return target.type === 'remote';
}

/**
 * `?target=` 쿼리 파라미터 값을 ResourceTarget 으로 파싱한다.
 *
 * - 미지정/null/`local`/빈 값 → 로컬.
 * - `remote:{instanceId}` → 원격(instanceId 디코딩). instanceId 가 비면 로컬로
 *   폴백한다(방어적).
 */
export function parseTargetParam(raw: string | null | undefined): ResourceTarget {
  if (!raw || raw === 'local') return LOCAL_TARGET;
  const prefix = 'remote:';
  if (raw.startsWith(prefix)) {
    const encoded = raw.slice(prefix.length);
    const instanceId = safeDecode(encoded);
    if (instanceId) return { type: 'remote', instanceId };
  }
  return LOCAL_TARGET;
}

/**
 * ResourceTarget 을 `?target=` 쿼리 파라미터 값으로 직렬화한다.
 * 로컬은 빈 문자열을 반환한다(파라미터 생략 신호).
 */
export function serializeTargetParam(target: ResourceTarget): string {
  if (target.type === 'local') return '';
  return `remote:${encodeURIComponent(target.instanceId)}`;
}

/** decodeURIComponent 안전 래퍼(손상된 입력은 원본 반환). */
function safeDecode(value: string): string {
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
}
