// 태그 필터 순수 유틸 — 필터 ID 생성 / 엔트리 태그 AND 매칭.
//
// 컴포넌트 파일(TagFilterChips.tsx)에서 분리했다. 컴포넌트 파일이 컴포넌트만
// 내보내야 Fast Refresh 가 동작하기 때문이다(react-refresh/only-export-components).
//
// @spec SPEC-WEB-005 SPEC-STORE-003

/** "tagKey=tagValue" 형식의 ID 를 만든다. 값에 '=' 가 포함되어 있어도 안전하게 파싱 가능하다. */
export function makeFilterId(tagKey: string, tagValue: string): string {
  return `${tagKey}=${tagValue}`;
}

/**
 * 엔트리의 태그 맵이 선택된 필터 집합을 AND 로 모두 만족하는지 확인한다.
 *
 * - selected 가 비어 있으면 항상 true.
 * - selected 의 각 "key=value" 에 대해 entryTags[key] === value 여야 한다.
 * - 같은 태그 키에 여러 값이 동시에 선택되면(예: room=1 + room=2) 둘 다 만족하는
 *   엔트리는 없으므로 false (AND 의 정상적 결과).
 *
 * @spec SPEC-STORE-003
 */
export function matchesTagFilter(
  entryTags: Record<string, string> | null | undefined,
  selected: ReadonlySet<string>,
): boolean {
  if (selected.size === 0) return true;
  if (!entryTags) return false;
  for (const id of selected) {
    const eqIdx = id.indexOf('=');
    if (eqIdx < 0) continue; // 잘못된 형식 무시
    const tagKey = id.slice(0, eqIdx);
    const tagValue = id.slice(eqIdx + 1);
    if (entryTags[tagKey] !== tagValue) {
      return false;
    }
  }
  return true;
}
