// 이름 기반 딥링크 수신 훅.
//
// 시스템 로그에서 `/agents?name=xxx` 로 넘어왔을 때, 목록을 그 이름으로 좁히고
// 일치하는 항목을 펼쳐 준다. 로그에는 ID 가 없어 이름으로 찾을 수밖에 없으므로,
// 이름이 중복되면 첫 항목을 펼치되 검색어는 남겨 사용자가 고를 수 있게 한다.

import { useEffect, useRef } from 'react';
import { useSearchParams } from 'react-router';

/**
 * `?name=` 쿼리를 읽어 목록 상태에 한 번 적용한다.
 *
 * @param items 목록 데이터 (로드 전이면 빈 배열)
 * @param getName 항목의 이름
 * @param getId 항목의 식별자
 * @param apply (검색어, 일치 항목 id | null) 를 받아 페이지 상태를 갱신한다
 *
 * 같은 `name` 값에 대해 한 번만 적용한다 — 매 렌더마다 적용하면 사용자가 검색어를
 * 지우거나 다른 항목을 펼쳐도 곧바로 되돌려져 화면이 붙잡힌다.
 */
export function useNameDeepLink<T>(
  items: T[],
  getName: (item: T) => string,
  getId: (item: T) => string,
  apply: (search: string, matchedId: string | null) => void,
): void {
  const [searchParams] = useSearchParams();
  const name = searchParams.get('name') ?? '';
  const appliedFor = useRef<string | null>(null);

  useEffect(() => {
    if (!name) return;
    if (appliedFor.current === name) return;

    // 목록이 아직 비어 있으면 일치 여부를 판단할 수 없다. 다음 렌더를 기다린다
    // (데이터 도착 후 한 번만 적용된다).
    if (items.length === 0) return;

    appliedFor.current = name;
    const hit = items.find((item) => getName(item) === name);
    apply(name, hit ? getId(hit) : null);
  }, [name, items, getName, getId, apply]);
}
