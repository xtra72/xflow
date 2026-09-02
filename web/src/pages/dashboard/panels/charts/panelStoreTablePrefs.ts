// 패널 설정 공용 Store 리스트의 필터/정렬/표시숨김 상태를 패널별 localStorage 로
// 영속/복원하기 위한 순수 헬퍼.
//
// @spec SPEC-PANEL-SETTINGS-001 (T7, AC-08)
//
// 방어적 파싱: 부재/손상 값은 기본값으로 폴백하고 예외를 던지지 않는다. ColumnFilter 의
// values(Set) 는 직렬화 시 배열로, 파싱 시 Set 으로 왕복 변환한다.

import type { StoreColumnId } from '@/pages/agents/storeColumnsModel';
import type {
  ColumnFilter,
  ColumnFilterMap,
  FilterColumnId,
  SortColumn,
  SortState,
} from '@/pages/agents/storeEntrySort';

/** 패널 설정 Store 테이블 상태(런타임 형상). */
export interface PanelStoreTablePrefs {
  sort: SortState;
  filters: ColumnFilterMap;
  /** 숨긴(hidden) 표시가능 컬럼 id 목록. */
  hidden: StoreColumnId[];
}

const PREFIX = 'panel-settings.storeTable.';

const VALID_SORT_COLUMNS: readonly SortColumn[] = [
  'key',
  'field',
  'value',
  'namespace',
  'binding',
  'updated',
];

/** 패널별 localStorage 키. */
export function panelStoreTableStorageKey(panelId: string): string {
  return `${PREFIX}${panelId}`;
}

/** 기본(비어있는) 상태. */
export function defaultPanelStoreTablePrefs(): PanelStoreTablePrefs {
  return { sort: { column: null, direction: 'asc' }, filters: {}, hidden: [] };
}

function parseSort(raw: unknown): SortState {
  if (!raw || typeof raw !== 'object') return { column: null, direction: 'asc' };
  const r = raw as Record<string, unknown>;
  const column =
    r.column === null || VALID_SORT_COLUMNS.includes(r.column as SortColumn)
      ? (r.column as SortColumn | null)
      : null;
  const direction = r.direction === 'desc' ? 'desc' : 'asc';
  return { column, direction };
}

function parseFilters(raw: unknown): ColumnFilterMap {
  if (!raw || typeof raw !== 'object') return {};
  const out: ColumnFilterMap = {};
  for (const [k, v] of Object.entries(raw as Record<string, unknown>)) {
    if (!v || typeof v !== 'object') continue;
    const rec = v as Record<string, unknown>;
    const text = typeof rec.text === 'string' ? rec.text : '';
    const values = Array.isArray(rec.values)
      ? new Set(rec.values.filter((x): x is string => typeof x === 'string'))
      : new Set<string>();
    if (text === '' && values.size === 0) continue;
    out[k as FilterColumnId] = { text, values };
  }
  return out;
}

/** 패널별 상태를 localStorage 에서 로드한다(부재/손상 시 기본값). */
export function loadPanelStoreTablePrefs(panelId: string): PanelStoreTablePrefs {
  if (typeof window === 'undefined' || !window.localStorage) {
    return defaultPanelStoreTablePrefs();
  }
  let raw: string | null = null;
  try {
    raw = window.localStorage.getItem(panelStoreTableStorageKey(panelId));
  } catch {
    return defaultPanelStoreTablePrefs();
  }
  if (raw === null) return defaultPanelStoreTablePrefs();
  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    const hidden = Array.isArray(parsed.hidden)
      ? parsed.hidden.filter((x): x is StoreColumnId => typeof x === 'string')
      : [];
    return { sort: parseSort(parsed.sort), filters: parseFilters(parsed.filters), hidden };
  } catch {
    return defaultPanelStoreTablePrefs();
  }
}

/** 패널별 상태를 localStorage 에 저장한다(Set → 배열 직렬화). 실패는 무시한다. */
export function savePanelStoreTablePrefs(
  panelId: string,
  prefs: PanelStoreTablePrefs,
): void {
  if (typeof window === 'undefined' || !window.localStorage) return;
  try {
    const filters: Record<string, { text: string; values: string[] }> = {};
    for (const [k, f] of Object.entries(prefs.filters)) {
      const cf = f as ColumnFilter | undefined;
      if (!cf) continue;
      filters[k] = { text: cf.text, values: Array.from(cf.values) };
    }
    const payload = { sort: prefs.sort, filters, hidden: prefs.hidden };
    window.localStorage.setItem(
      panelStoreTableStorageKey(panelId),
      JSON.stringify(payload),
    );
  } catch {
    // 저장 실패(용량/프라이빗 모드)는 무시 — 세션 내 상태로만 동작.
  }
}
