// 원격 노드의 사람이 읽을 수 있는 표시명 해석 (원격 편집기 시각 구분).
//
// 호스트명을 우선 사용하고, 호스트명이 비어 있으면(노드 상세 미조회/지연/실패)
// 단축 instanceId(앞 8자 + 생략부호)로 폴백한다. 원시 UUID 전체는 본문에
// 노출하지 않으므로(툴팁/접근성에만 노출) 단축형을 표시 기본값으로 둔다.

/** 단축 instanceId 표시 시 유지할 접두 길이. */
const SHORT_ID_PREFIX_LEN = 8;

/**
 * instanceId 를 단축형으로 변환한다(8자 초과 시 앞 8자 + 생략부호).
 *
 * @param instanceId - 원본 노드 식별자.
 * @returns 8자 이하면 원본, 초과하면 `${앞8자}…`.
 */
export function shortenInstanceId(instanceId: string): string {
  return instanceId.length > SHORT_ID_PREFIX_LEN
    ? `${instanceId.slice(0, SHORT_ID_PREFIX_LEN)}…`
    : instanceId;
}

/**
 * 원격 노드 표시명을 해석한다.
 *
 * @param hostname - 노드 상세에서 해석된 호스트명(미조회/지연 시 undefined/빈 값).
 * @param instanceId - 원본 노드 식별자(폴백 소스).
 * @returns 호스트명이 있으면 호스트명, 없으면 단축 instanceId, 둘 다 없으면 빈 문자열.
 */
export function resolveRemoteNodeLabel(
  hostname: string | undefined | null,
  instanceId: string | undefined | null,
): string {
  if (hostname) return hostname;
  if (instanceId) return shortenInstanceId(instanceId);
  return '';
}
