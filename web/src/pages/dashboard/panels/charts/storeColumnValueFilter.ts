// 시리즈 선택 테이블의 표시 필터 순수 매처.
//
// @spec SPEC-PANEL-SETTINGS-001 (REQ-15) — 같은 차원(dimension) 내 다중값 OR + 차원 간 AND.
//
// v0.5.0: 필터 상태를 차원(dimension) 단위의 `Record<dimId, Set<value>>` 로 일반화한다.
// 일반 컬럼(key/name/metric)은 각자 1개 차원이고, **태그는 태그 키(종류)마다 별도 차원**
// (`tag:<키>`)이다. 각 차원에 대해 행의 값이 그 차원 값 집합 중 하나에 포함되면 통과(같은
// 차원 OR)하고, 필터가 걸린 모든 차원을 AND 로 결합한다. 미설정 차원(빈 집합)은 결합에서
// 제외한다(항상 통과). 예: `device_id ∈ {A,B}` AND `type ∈ {report}`.
//
// 기존 태그 필터(`tag_filters: Record<string,string>`, 키당 단일값·크로스키 AND)는
// 값 하나를 원소 1개의 집합으로 승격해 해석한다(하위호환) — `tagFiltersToColumnValues`.
//
// 순수 함수로 유지하여 컴포넌트 밖에서 단위 테스트가 가능하도록 한다.

/** 차원 id(컬럼 id 또는 `tag:<키>`) → 허용 값 집합. 빈 집합/미설정 차원은 매칭에서 제외한다. */
export type ColumnValueFilters = Record<string, ReadonlySet<string>>;

/**
 * 한 행이 컬럼 값 필터를 통과하는지 판정한다.
 *
 * - 같은 컬럼 내 다중값: 행의 셀 값이 그 값 집합에 포함되면 통과(OR).
 * - 서로 다른 컬럼 간: 필터가 걸린 모든 컬럼을 통과해야 한다(AND).
 * - 필터가 없는(빈 집합) 컬럼: 결합에서 제외한다(항상 통과).
 *
 * @param getCellValue 컬럼 id → 그 행의 셀 값(없으면 undefined).
 * @param filters      컬럼 id → 허용 값 집합.
 */
export function matchesColumnValueFilters(
  getCellValue: (columnId: string) => string | undefined,
  filters: ColumnValueFilters,
): boolean {
  for (const col of Object.keys(filters)) {
    const values = filters[col];
    if (!values || values.size === 0) continue; // 미설정 컬럼 → 항상 통과.
    const cell = getCellValue(col);
    if (cell === undefined || !values.has(cell)) return false; // OR(집합 포함) / 불충족 시 탈락.
  }
  return true;
}

/**
 * 필터가 하나라도 활성인지(값 집합이 비어있지 않은 컬럼이 존재하는지) 여부.
 * 모두 비어 있으면 전체 통과이므로 필터를 적용하지 않아도 된다.
 */
export function hasActiveColumnValueFilters(filters: ColumnValueFilters): boolean {
  for (const col of Object.keys(filters)) {
    const values = filters[col];
    if (values && values.size > 0) return true;
  }
  return false;
}

/**
 * 기존 `tag_filters`(키당 단일값·크로스키 AND)를 일반화된 `ColumnValueFilters` 로 승격한다.
 * 각 태그 키의 단일값을 원소 1개의 값 집합으로 해석한다(하위호환) — 결과는 기존 AND 매칭과
 * 동일하다(1원소 집합이므로 OR 은 항등).
 */
export function tagFiltersToColumnValues(
  tagFilters: Record<string, string>,
): ColumnValueFilters {
  const out: Record<string, Set<string>> = {};
  for (const [k, v] of Object.entries(tagFilters)) {
    out[k] = new Set([v]);
  }
  return out;
}

/**
 * 태그 컬럼 필터의 "k=v" 값 집합을 **태그 키별 차원**으로 분해한다(v0.5.0, REQ-15/AC-17b).
 *
 * 같은 태그 키의 여러 값은 그 키 차원의 값 집합(OR)이 되고, 서로 다른 태그 키는 각자
 * 차원(AND)이 된다. 예: `{device_id=A, device_id=B, type=report}` →
 * `{ device_id: {A, B}, type: {report} }`. `=` 가 없는 값은 무시한다.
 */
export function splitTagPairsToDimensions(
  pairs: ReadonlySet<string>,
): Record<string, Set<string>> {
  const out: Record<string, Set<string>> = {};
  for (const pair of pairs) {
    const eq = pair.indexOf('=');
    if (eq < 0) continue;
    const key = pair.slice(0, eq);
    const value = pair.slice(eq + 1);
    (out[key] ??= new Set<string>()).add(value);
  }
  return out;
}
