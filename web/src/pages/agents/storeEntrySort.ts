// 저장소 탭 store 키 테이블의 정렬·검색 순수 로직.
//
// AgentDetailPanel.tsx 의 StoreTab 에서 사용한다. 컴포넌트 밖으로 분리하여
// 단위 테스트가 가능하도록 한다.
//
// @spec SPEC-WEB-005 (store key 테이블 컬럼 정렬/필터)

/** Store state 엔트리. 백엔드에서 디코드된 사용자 key 와 메타데이터를 포함한다. */
export type StoreEntry = Record<string, unknown>;

/** 정렬 가능한 컬럼 식별자. */
export type SortColumn =
  | 'key'
  | 'binding'
  | 'metric'
  | 'value'
  | 'namespace'
  | 'updated';

/** 정렬 방향. */
export type SortDirection = 'asc' | 'desc';

/** 현재 정렬 상태. column 이 null 이면 정렬 미적용(원본 순서 유지). */
export interface SortState {
  column: SortColumn | null;
  direction: SortDirection;
}

/**
 * 엔트리의 metric_type 을 추출한다. 누락/비문자열은 빈 문자열을 반환한다.
 * (호출자는 표시/정렬 시 "unknown" 으로 정규화)
 */
export function entryMetricType(entry: StoreEntry): string {
  const raw = entry.metric_type;
  return typeof raw === 'string' ? raw : '';
}

/**
 * 엔트리의 tags 맵을 추출한다. 값이 문자열인 항목만 채택하며,
 * 유효 태그가 없으면 빈 객체를 반환한다.
 */
export function entryTags(entry: StoreEntry): Record<string, string> {
  const raw = entry.tags;
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return {};
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(raw as Record<string, unknown>)) {
    if (typeof v === 'string') out[k] = v;
  }
  return out;
}

/** 엔트리의 key 를 문자열로 추출한다. */
function entryKey(entry: StoreEntry): string {
  return typeof entry.key === 'string' ? entry.key : '';
}

/** 엔트리의 namespace 를 문자열로 추출한다. */
function entryNamespace(entry: StoreEntry): string {
  return typeof entry.namespace === 'string' ? entry.namespace : '';
}

/** 엔트리의 value 를 비교용 문자열로 직렬화한다. */
function entryValueString(entry: StoreEntry): string {
  const v = entry.value;
  return typeof v === 'string' ? v : JSON.stringify(v ?? null);
}

/** 엔트리의 updated_at 을 비교용 epoch(ms) 로 변환한다. 누락/무효는 0. */
function entryUpdatedAt(entry: StoreEntry): number {
  const raw = entry.updated_at;
  if (typeof raw !== 'string') return 0;
  const t = new Date(raw).getTime();
  return Number.isNaN(t) ? 0 : t;
}

/**
 * 검색어로 엔트리를 필터링한다 (부분일치, 대소문자 무시).
 *
 * 매칭 대상: key, metric_type, tags(키=값 모두). metric 은 key 컬럼에
 * 이어붙이지 않고(별도 컬럼) 검색 대상에만 포함한다.
 *
 * @param entries 원본 엔트리 배열
 * @param query 검색어 (공백은 trim, 빈 문자열이면 원본 그대로 반환)
 */
export function filterEntries(
  entries: readonly StoreEntry[],
  query: string,
): StoreEntry[] {
  const q = query.trim().toLowerCase();
  if (q === '') return [...entries];
  return entries.filter((e) => {
    if (entryKey(e).toLowerCase().includes(q)) return true;
    // metric_type: 빈 값은 "unknown" 으로 정규화하여 검색 가능하게 한다.
    const mt = (entryMetricType(e) || 'unknown').toLowerCase();
    if (mt.includes(q)) return true;
    const tags = entryTags(e);
    for (const [k, v] of Object.entries(tags)) {
      if (k.toLowerCase().includes(q)) return true;
      if (v.toLowerCase().includes(q)) return true;
      // "k=v" 결합 형태로도 매칭 (테이블 칩 표기와 일치).
      if (`${k}=${v}`.toLowerCase().includes(q)) return true;
    }
    return false;
  });
}

/**
 * 정렬 키 추출용 컨텍스트.
 * binding 컬럼은 정적 여부를 사용하므로 정적 키 이름 집합을 받는다.
 */
export interface SortContext {
  /** 정적(설정 keys 에 등록된) 키 이름 집합. */
  staticKeyNames: ReadonlySet<string>;
}

/** 정적/동적을 정렬용 정수로 매핑. 정적(0) < 동적(1) (asc 시 정적 먼저). */
function bindingRank(entry: StoreEntry, ctx: SortContext): number {
  return ctx.staticKeyNames.has(entryKey(entry)) ? 0 : 1;
}

/**
 * 두 엔트리를 지정 컬럼 기준으로 비교한다. (방향 미적용 — 항상 오름차순 기준)
 * 반환값 <0: a 가 앞, >0: b 가 앞, 0: 동률.
 */
function compareByColumn(
  a: StoreEntry,
  b: StoreEntry,
  column: SortColumn,
  ctx: SortContext,
): number {
  switch (column) {
    case 'key':
      return entryKey(a).localeCompare(entryKey(b));
    case 'binding': {
      const r = bindingRank(a, ctx) - bindingRank(b, ctx);
      return r;
    }
    case 'metric':
      return (entryMetricType(a) || 'unknown').localeCompare(
        entryMetricType(b) || 'unknown',
      );
    case 'value':
      return entryValueString(a).localeCompare(entryValueString(b), undefined, {
        numeric: true,
      });
    case 'namespace':
      return entryNamespace(a).localeCompare(entryNamespace(b));
    case 'updated':
      return entryUpdatedAt(a) - entryUpdatedAt(b);
    default:
      return 0;
  }
}

/**
 * 엔트리를 정렬한다. 안정성과 결정성을 위해 보조 정렬(key → metric)을 적용한다.
 *
 * - column 이 null 이면 원본 순서를 그대로 반환(복사본).
 * - 1차 정렬이 동률이면 key, 그 다음 metric_type 으로 결정적 정렬한다.
 *   (같은 key 의 다중 시리즈가 metric 으로 결정적으로 정렬되도록)
 * - direction 은 1차 정렬에만 적용하고, 보조 정렬은 항상 오름차순으로 두어
 *   결과가 안정적이도록 한다.
 *
 * @param entries 원본 엔트리 배열
 * @param sort 정렬 상태
 * @param ctx binding 정렬용 컨텍스트
 */
export function sortEntries(
  entries: readonly StoreEntry[],
  sort: SortState,
  ctx: SortContext,
): StoreEntry[] {
  const out = [...entries];
  if (sort.column === null) return out;
  const dir = sort.direction === 'desc' ? -1 : 1;
  const column = sort.column;
  out.sort((a, b) => {
    const primary = compareByColumn(a, b, column, ctx) * dir;
    if (primary !== 0) return primary;
    // 보조 정렬: key → metric (항상 오름차순, 결정성 확보).
    if (column !== 'key') {
      const byKey = entryKey(a).localeCompare(entryKey(b));
      if (byKey !== 0) return byKey;
    }
    if (column !== 'metric') {
      return (entryMetricType(a) || 'unknown').localeCompare(
        entryMetricType(b) || 'unknown',
      );
    }
    return 0;
  });
  return out;
}

/**
 * 헤더 클릭 시 다음 정렬 상태를 계산한다.
 * - 다른 컬럼 클릭: 해당 컬럼 오름차순.
 * - 같은 컬럼 클릭: asc → desc → asc 토글.
 */
export function nextSortState(
  current: SortState,
  column: SortColumn,
): SortState {
  if (current.column !== column) {
    return { column, direction: 'asc' };
  }
  return {
    column,
    direction: current.direction === 'asc' ? 'desc' : 'asc',
  };
}
