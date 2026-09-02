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
  | 'field'
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
 * 엔트리의 field 을 추출한다. 누락/비문자열은 빈 문자열을 반환한다.
 * (호출자는 표시/정렬 시 "unknown" 으로 정규화)
 */
export function entryMetricType(entry: StoreEntry): string {
  const raw = entry.field;
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
 * 매칭 대상: key, field, tags(키=값 모두). metric 은 key 컬럼에
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
    // field: 빈 값은 "unknown" 으로 정규화하여 검색 가능하게 한다.
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
    case 'field':
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
 * - 1차 정렬이 동률이면 key, 그 다음 field 으로 결정적 정렬한다.
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
    if (column !== 'field') {
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

// ---------------------------------------------------------------------------
// Excel 유사 컬럼별 필터 (SPEC-WEB-005)
//
// 각 컬럼 헤더의 필터 버튼에서 "텍스트 부분일치 + 값 체크박스 목록" 으로 필터링한다.
// 컴포넌트 밖 순수 함수로 분리하여 단위 테스트가 가능하도록 한다.
// ---------------------------------------------------------------------------

/** 컬럼별 필터가 가능한 컬럼 식별자. */
export type FilterColumnId =
  | 'key'
  | 'field'
  | 'value'
  | 'namespace'
  | 'binding'
  | 'tags'
  | 'ttl';

/**
 * 컬럼별 셀 값 추출/필터에 필요한 컨텍스트.
 *
 * binding 컬럼은 정적/동적 라벨을 셀 값으로 사용하므로, i18n 라벨을 주입받는다.
 * (순수 함수를 유지하기 위해 라벨 문자열을 인자로 받는다 — 테스트에서는 임의 라벨 주입.)
 */
export interface FilterContext {
  /** 정적(설정 keys 에 등록된) 키 이름 집합. */
  staticKeyNames: ReadonlySet<string>;
  /** binding 컬럼의 정적/동적 표시 라벨. */
  bindingLabels: { static: string; dynamic: string };
}

/**
 * 단일 컬럼의 필터 상태.
 * - `text`: 부분일치(대소문자 무시) 검색어. 빈 문자열이면 텍스트 조건 없음.
 * - `values`: 체크된 값 집합. 비어 있으면 값 조건 없음(전체 허용).
 */
export interface ColumnFilter {
  text: string;
  values: Set<string>;
}

/** 컬럼 id → 필터 상태 맵 (부분적으로만 존재할 수 있음). */
export type ColumnFilterMap = Partial<Record<FilterColumnId, ColumnFilter>>;

/**
 * 엔트리의 tags 를 정렬된 "k=v" 문자열 배열로 변환한다.
 * (테이블 칩 표기 및 필터 값 목록과 일치시킨다.)
 */
function entryTagPairs(entry: StoreEntry): string[] {
  const tags = entryTags(entry);
  return Object.entries(tags)
    .map(([k, v]) => `${k}=${v}`)
    .sort((a, b) => a.localeCompare(b));
}

/**
 * 컬럼 하나에 대한 엔트리의 "필터/표시용 셀 값" 목록을 반환한다.
 *
 * 대부분의 컬럼은 단일 값(길이 1 배열)을 갖지만, tags 컬럼은 여러 "k=v" 값을 가진다.
 * 값 체크박스 목록과 값 매칭은 이 배열을 기준으로 계산한다.
 */
export function columnCellValues(
  entry: StoreEntry,
  column: FilterColumnId,
  ctx: FilterContext,
): string[] {
  switch (column) {
    case 'key':
      return [entryKey(entry)];
    case 'field':
      return [entryMetricType(entry) || 'unknown'];
    case 'value':
      return [entryValueString(entry)];
    case 'namespace':
      return [entryNamespace(entry)];
    case 'binding':
      return [
        ctx.staticKeyNames.has(entryKey(entry))
          ? ctx.bindingLabels.static
          : ctx.bindingLabels.dynamic,
      ];
    case 'tags':
      return entryTagPairs(entry);
    case 'ttl': {
      const raw = entry.ttl;
      return [typeof raw === 'string' && raw !== '' ? raw : '∞'];
    }
    default:
      return [];
  }
}

/**
 * 텍스트 부분일치용 결합 문자열. 다중 값(tags)은 공백으로 이어붙여 부분일치 검사한다.
 */
function columnTextValue(
  entry: StoreEntry,
  column: FilterColumnId,
  ctx: FilterContext,
): string {
  return columnCellValues(entry, column, ctx).join(' ');
}

/**
 * 현재 로드된 전체 엔트리에서 한 컬럼의 고유 값 목록을 추출한다(정렬).
 *
 * 필터 적용 후에도 옵션이 사라지지 않도록, 호출자는 반드시 필터링 전의 전체
 * 엔트리(allEntries)를 전달해야 한다.
 */
export function uniqueColumnValues(
  entries: readonly StoreEntry[],
  column: FilterColumnId,
  ctx: FilterContext,
): string[] {
  const set = new Set<string>();
  for (const e of entries) {
    for (const v of columnCellValues(e, column, ctx)) {
      set.add(v);
    }
  }
  return Array.from(set).sort((a, b) =>
    a.localeCompare(b, undefined, { numeric: true }),
  );
}

/**
 * 엔트리가 한 컬럼의 필터를 통과하는지 판정한다.
 *
 * 통과 조건: (텍스트 부분일치) AND (체크박스 미선택 OR 셀 값 중 하나가 체크 집합에 포함).
 * text 가 빈 문자열이면 텍스트 조건은 무시하고, values 가 비어 있으면 값 조건을 무시한다.
 */
export function entryPassesColumnFilter(
  entry: StoreEntry,
  column: FilterColumnId,
  filter: ColumnFilter,
  ctx: FilterContext,
): boolean {
  const text = filter.text.trim().toLowerCase();
  if (text !== '') {
    if (!columnTextValue(entry, column, ctx).toLowerCase().includes(text)) {
      return false;
    }
  }
  if (filter.values.size > 0) {
    const values = columnCellValues(entry, column, ctx);
    if (!values.some((v) => filter.values.has(v))) {
      return false;
    }
  }
  return true;
}

/** 필터 상태가 실제로 활성(텍스트 또는 값 선택)인지 여부. */
export function isColumnFilterActive(filter: ColumnFilter | undefined): boolean {
  if (!filter) return false;
  return filter.text.trim() !== '' || filter.values.size > 0;
}

/**
 * 모든 컬럼 필터를 AND 로 결합해 엔트리를 필터링한다.
 * (활성 필터가 하나도 없으면 원본의 복사본을 그대로 반환한다.)
 */
export function applyColumnFilters(
  entries: readonly StoreEntry[],
  filters: ColumnFilterMap,
  ctx: FilterContext,
): StoreEntry[] {
  const active = (Object.entries(filters) as Array<
    [FilterColumnId, ColumnFilter | undefined]
  >).filter(([, f]) => isColumnFilterActive(f));
  if (active.length === 0) return [...entries];
  return entries.filter((e) =>
    active.every(([col, f]) => entryPassesColumnFilter(e, col, f!, ctx)),
  );
}
