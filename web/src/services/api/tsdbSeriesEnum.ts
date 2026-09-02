// TSDB 시리즈 열거(D5) 클라이언트와 그룹 후보 도출.
//
// 백엔드 REST 계약 (응답은 { data: ... } envelope — apiClient 인터셉터가 벗겨준다):
//   GET /api/v1/influxdb/{agent_name}/series
//       ?measurement=<m>&bucket=<b>&start_ms=<n>&end_ms=<n>&tags=k=v,k2=v2&limit=<n>
//     -> { series: [{ tags, fields }], field_exact, count, truncated, window }
//
// 이 모듈이 존재하는 이유는 **시리즈축 페이지네이션**이다(SPEC-TSDB-004 §2.7).
// group by 는 그룹 수에 상한을 두지 않는 대신 한 번에 가져오는 양을 페이지로
// 유계화하는데, 그러려면 "전체 그룹 목록" 을 먼저 알아야 한다. 백엔드에는 시리즈
// 개수를 자르는 네이티브 기전이 없으므로(v3 SLIMIT 미구현 · Flux 대응물 없음)
// 열거 -> 투영 -> 페이지 슬라이스 -> group_filter 질의의 2단계 절차를 쓴다.
//
// @spec SPEC-TSDB-004 §2.7.2

import { get } from './client';
import { SERIES_ID_SEPARATOR } from './seriesLabels';

/** 열거된 시리즈 1건. */
export interface TsdbEnumeratedSeries {
  /** 실재하는 태그 집합 1벌. 태그 없는 시리즈는 빈 객체다(null 아님). */
  tags: Record<string, string>;
  /** 그 태그 집합에서 관측된 field 키 목록(사전순). */
  fields: string[];
}

/** 서버가 실제로 사용한 탐색 창. */
export interface TsdbSeriesEnumWindow {
  start_ms: number;
  end_ms: number;
}

/** 열거 응답. */
export interface TsdbSeriesEnumResult {
  series: TsdbEnumeratedSeries[];
  /** fields 가 정확한 관측치인지(v2 true) 하한/근사인지(v3 false). */
  field_exact: boolean;
  count: number;
  /**
   * 상한에 걸려 잘렸는지. `true` 면 페이지를 전부 넘겨도 일부 그룹에 도달하지
   * 못한다 — UI 는 좁히는 방법을 안내해야 한다(§2.7.4).
   */
  truncated: boolean;
  window: TsdbSeriesEnumWindow;
}

/** 열거 질의 파라미터. */
export interface TsdbSeriesEnumQuery {
  measurement: string;
  bucket?: string;
  /** 사전 필터. 열거 대상을 먼저 좁힌다. */
  tags?: Record<string, string>;
  startMs?: number;
  endMs?: number;
  /** 반환 태그 집합 상한. 생략 시 서버 기본값. */
  limit?: number;
}

/** 태그 맵을 백엔드가 받는 `k=v,k2=v2` 형태로 만든다(키 오름차순 고정). */
function encodeTagFilter(tags: Record<string, string>): string {
  return Object.keys(tags)
    .sort()
    .map((k) => `${k}=${tags[k]}`)
    .join(',');
}

/**
 * 시리즈를 열거한다(D5).
 *
 * 호출자는 폴링마다 이 함수를 다시 부른다 — 캐시하지 않는 것이 확정된 정책이다
 * (§2.7.4). 새로 생긴 그룹이 즉시 보여야 하며, 캐시하면 "장비를 추가했는데
 * 차트에 안 나온다" 는 상태가 캐시 수명만큼 지속된다.
 */
export async function enumerateTsdbSeries(
  agentName: string,
  query: TsdbSeriesEnumQuery,
  signal?: AbortSignal,
): Promise<TsdbSeriesEnumResult> {
  if (!query.measurement) {
    throw new Error('enumerateTsdbSeries: measurement 가 필요합니다');
  }
  const params = new URLSearchParams();
  params.set('measurement', query.measurement);
  if (query.bucket) params.set('bucket', query.bucket);
  if (query.tags && Object.keys(query.tags).length > 0) {
    params.set('tags', encodeTagFilter(query.tags));
  }
  if (query.startMs !== undefined) params.set('start_ms', String(query.startMs));
  if (query.endMs !== undefined) params.set('end_ms', String(query.endMs));
  if (query.limit !== undefined) params.set('limit', String(query.limit));

  const resp = await get<TsdbSeriesEnumResult>(
    `/influxdb/${encodeURIComponent(agentName)}/series?${params.toString()}`,
    signal ? { signal } : undefined,
  );
  return {
    series: resp?.series ?? [],
    field_exact: resp?.field_exact ?? false,
    count: resp?.count ?? 0,
    truncated: resp?.truncated ?? false,
    window: resp?.window ?? { start_ms: 0, end_ms: 0 },
  };
}

/**
 * 그룹 조합의 결정적 서명. 정렬·중복 제거의 기준이다.
 *
 * 구분자는 NUL(`SERIES_ID_SEPARATOR`)이다 — 태그 값에 등장할 수 없으므로
 * `a|b` 와 `a` + `|b` 가 같은 서명이 되는 충돌을 막는다. 서버의
 * `seriesGroupSignature` 와 같은 규약이다.
 */
export function groupComboSignature(
  combo: Record<string, string>,
  groupKeys: readonly string[],
): string {
  return groupKeys.map((k) => combo[k] ?? '').join(SERIES_ID_SEPARATOR);
}

/**
 * 열거 결과를 group by 키로 투영해 **그룹 후보 목록**을 만든다(§2.7.2 2단계).
 *
 * 열거는 전 태그 조합을 주지만 그룹 축은 지정 키뿐이므로 투영 후 중복을 제거한다.
 * 예: 시리즈가 {host:a,rack:r1}·{host:a,rack:r2} 둘이어도 `group_by:['host']`
 * 에서는 후보가 {host:a} 하나다.
 *
 * 결과는 서명 사전순으로 정렬한다 — 순서가 안정적이어야 페이지 경계가 폴링 간
 * 흔들리지 않고, 자동 팔레트 색이 라인 사이를 옮겨 다니지 않는다.
 *
 * 열거 시리즈에 그룹 키가 없으면 빈 문자열로 둔다. 조용히 버리면 태그가 결손된
 * 시리즈가 차트에서 사라지고, 사용자는 그 사실을 알 방법이 없다.
 */
export function deriveGroupCombos(
  series: readonly TsdbEnumeratedSeries[],
  groupKeys: readonly string[],
): Array<Record<string, string>> {
  if (groupKeys.length === 0) return [];
  const keys = [...groupKeys].sort();
  const seen = new Map<string, Record<string, string>>();
  for (const s of series) {
    const combo: Record<string, string> = {};
    for (const k of keys) combo[k] = s.tags[k] ?? '';
    const sig = groupComboSignature(combo, keys);
    if (!seen.has(sig)) seen.set(sig, combo);
  }
  return [...seen.entries()]
    .sort((a, b) => (a[0] < b[0] ? -1 : a[0] > b[0] ? 1 : 0))
    .map(([, combo]) => combo);
}

/**
 * 그룹 후보 목록에서 한 페이지를 잘라낸다(§2.7.2 3단계).
 *
 * `pageSize` 가 0 이하이면 전량을 돌려준다 — 페이지네이션 비활성이다.
 */
export function sliceGroupPage(
  combos: ReadonlyArray<Record<string, string>>,
  page: number,
  pageSize: number,
): Array<Record<string, string>> {
  if (pageSize <= 0) return [...combos];
  const start = Math.max(0, page) * pageSize;
  return combos.slice(start, start + pageSize);
}

/** 전체 그룹 수에서 페이지 수를 구한다. `pageSize` 가 0 이하이면 1 이다. */
export function groupPageCount(total: number, pageSize: number): number {
  if (pageSize <= 0) return 1;
  return Math.max(1, Math.ceil(total / pageSize));
}
