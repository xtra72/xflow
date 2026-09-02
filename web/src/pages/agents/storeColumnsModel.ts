// 저장소 탭 store 키 테이블의 컬럼 레지스트리 + localStorage 헬퍼 + 필터 초기값.
//
// UI 컴포넌트 파일(storeColumns.tsx)에서 분리했다. 컴포넌트 파일이 컴포넌트만
// 내보내야 Fast Refresh 가 동작하기 때문이다(react-refresh/only-export-components).
//
// 단일 출처(single source of truth): 컬럼의 논리적 순서/메타데이터를 STORE_COLUMNS 에
// 정의하고, 헤더(thead)와 본문(StoreEntryRow)이 모두 이 목록을 매핑하여 렌더한다.
//
// @spec SPEC-WEB-005 (store key 테이블 구성 가능 컬럼)

import type {
  ColumnFilter,
  FilterColumnId,
  SortColumn,
} from './storeEntrySort';

/** 렌더 가능한 모든 컬럼 식별자(논리 순서 정의는 STORE_COLUMNS). */
export type StoreColumnId =
  | 'key'
  | 'field'
  | 'value'
  | 'namespace'
  | 'tags'
  | 'binding'
  | 'ttl'
  | 'history'
  | 'updated'
  | 'actions'
  // @spec SPEC-PANEL-SETTINGS-001 (T6): 패널 설정 컨텍스트 전용 컬럼. 기본 STORE_COLUMNS
  // 레지스트리에는 포함하지 않으므로(에이전트 상세 무영향), 패널 설정이 컬럼 세트를 직접
  // 구성하고 셀은 renderCellExtra 로 렌더한다.
  | 'alias';

/** 컬럼 정의. */
export interface StoreColumn {
  /** 안정적 컬럼 식별자. */
  id: StoreColumnId;
  /** i18n 키 (agents.detail.store.* 하위). */
  labelKey: string;
  /** 정렬 가능하면 대응하는 SortColumn, 아니면 미지정. */
  sortColumn?: SortColumn;
  /** Excel 유사 필터 가능하면 대응하는 FilterColumnId, 아니면 미지정. */
  filterColumn?: FilterColumnId;
  /** 셀 정렬(액션은 우측). 기본 left. */
  align?: 'left' | 'right';
  /** 사용자가 숨길 수 있는지 여부(actions=false). */
  hideable: boolean;
  /**
   * 조건부 컬럼 종류:
   *   - 'tags': 어떤 엔트리든 태그가 있을 때만 관련(showTagsColumn).
   *   - 'history': maxHistorySize > 0 일 때만 관련.
   * 미지정이면 항상 관련.
   */
  conditional?: 'tags' | 'history';
  /** 헤더 th 와 본문 td 에 공통 적용할 폭 Tailwind 클래스(예: 'w-16', 'min-w-[260px]'). */
  widthClass?: string;
}

/**
 * 논리적 기본 컬럼 순서(단일 출처).
 * 키 → 필드 → 값 → 네임스페이스 → 태그 → 바인딩 → TTL → 히스토리 → 갱신 → 액션.
 */
export const STORE_COLUMNS: readonly StoreColumn[] = [
  { id: 'key', labelKey: 'colKey', sortColumn: 'key', filterColumn: 'key', hideable: true, widthClass: 'min-w-[260px]' },
  { id: 'field', labelKey: 'colField', sortColumn: 'field', filterColumn: 'field', hideable: true },
  { id: 'value', labelKey: 'colValue', sortColumn: 'value', filterColumn: 'value', hideable: true },
  { id: 'namespace', labelKey: 'colNamespace', sortColumn: 'namespace', filterColumn: 'namespace', hideable: true },
  { id: 'tags', labelKey: 'colTags', filterColumn: 'tags', hideable: true, conditional: 'tags' },
  { id: 'binding', labelKey: 'colBinding', sortColumn: 'binding', filterColumn: 'binding', hideable: true, widthClass: 'w-32' },
  { id: 'ttl', labelKey: 'colTtl', filterColumn: 'ttl', hideable: true, widthClass: 'w-16' },
  { id: 'history', labelKey: 'colHistory', hideable: true, conditional: 'history', widthClass: 'w-32' },
  { id: 'updated', labelKey: 'colUpdated', sortColumn: 'updated', hideable: true },
  { id: 'actions', labelKey: 'colActions', align: 'right', hideable: false },
];

/** 현재 데이터 조건(태그 존재/히스토리 크기)에 따른 컬럼 관련성 판정. */
export interface ColumnRelevance {
  showTagsColumn: boolean;
  hasHistory: boolean;
}

/** 조건부 컬럼(tags/history)을 관련성에 따라 걸러 렌더 가능한 컬럼만 남긴다. */
export function relevantColumns(rel: ColumnRelevance): StoreColumn[] {
  return STORE_COLUMNS.filter((c) => {
    if (c.conditional === 'tags') return rel.showTagsColumn;
    if (c.conditional === 'history') return rel.hasHistory;
    return true;
  });
}

/**
 * 관련 컬럼 중 실제로 렌더할(보이는) 컬럼을 계산한다.
 * actions 는 항상 표시하며, 그 외는 visibleColumns 집합에 포함될 때만 표시한다.
 */
export function renderedColumns(
  rel: ColumnRelevance,
  visible: ReadonlySet<StoreColumnId>,
): StoreColumn[] {
  return relevantColumns(rel).filter(
    (c) => c.id === 'actions' || visible.has(c.id),
  );
}

// ---- localStorage 헬퍼 ----

/** per-agent 컬럼 가시성 저장 키 접두. 최종 키: `xflow.store.columns.<agentId>`. */
export const STORE_COLUMNS_STORAGE_PREFIX = 'xflow.store.columns.';

/** per-agent localStorage 키를 구성한다. */
export function storeColumnsStorageKey(agentId: string): string {
  return `${STORE_COLUMNS_STORAGE_PREFIX}${agentId}`;
}

/** 숨길 수 있는(hideable) 모든 컬럼 id 집합 = 기본 전체 표시. */
export function defaultVisibleColumns(): Set<StoreColumnId> {
  return new Set(STORE_COLUMNS.filter((c) => c.hideable).map((c) => c.id));
}

/** 유효한 컬럼 id 인지 검사(저장된 무효 값 방어). */
function isValidColumnId(v: unknown): v is StoreColumnId {
  return (
    typeof v === 'string' && STORE_COLUMNS.some((c) => c.id === v)
  );
}

/**
 * per-agent 가시 컬럼 집합을 localStorage 에서 로드한다.
 * 저장값이 없거나 파싱 실패면 기본(전체 표시)을 반환한다.
 * 저장된 무효/미지정 id 는 무시하며, actions 는 항상 렌더되므로 집합에서 제외한다.
 */
export function loadVisibleColumns(agentId: string): Set<StoreColumnId> {
  if (typeof window === 'undefined' || !window.localStorage) {
    return defaultVisibleColumns();
  }
  let raw: string | null = null;
  try {
    raw = window.localStorage.getItem(storeColumnsStorageKey(agentId));
  } catch {
    return defaultVisibleColumns();
  }
  if (raw === null) return defaultVisibleColumns();
  try {
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return defaultVisibleColumns();
    const out = new Set<StoreColumnId>();
    for (const v of parsed) {
      if (isValidColumnId(v) && v !== 'actions') out.add(v);
    }
    return out;
  } catch {
    return defaultVisibleColumns();
  }
}

/** per-agent 가시 컬럼 집합을 localStorage 에 저장한다(직렬화 가능한 배열로). */
export function saveVisibleColumns(
  agentId: string,
  visible: ReadonlySet<StoreColumnId>,
): void {
  if (typeof window === 'undefined' || !window.localStorage) return;
  try {
    const arr = STORE_COLUMNS.filter(
      (c) => c.hideable && visible.has(c.id),
    ).map((c) => c.id);
    window.localStorage.setItem(
      storeColumnsStorageKey(agentId),
      JSON.stringify(arr),
    );
  } catch {
    // 저장 실패(용량 초과/프라이빗 모드 등)는 무시한다 — 세션 내 상태로만 동작.
  }
}

/** 빈 필터 상태(신규 컬럼 필터 초기값). */
export function emptyColumnFilter(): ColumnFilter {
  return { text: '', values: new Set<string>() };
}
