// 저장소 탭 store 키 테이블의 컬럼 레지스트리 + localStorage 헬퍼 + Excel 유사 필터/
// 컬럼 설정 UI.
//
// 단일 출처(single source of truth): 컬럼의 논리적 순서/메타데이터를 STORE_COLUMNS 에
// 정의하고, 헤더(thead)와 본문(StoreEntryRow)이 모두 이 목록을 매핑하여 렌더한다.
// 숨김 컬럼은 헤더/본문에서 함께 사라지며, 히스토리 확장 행의 colSpan 은 렌더된 컬럼
// 개수와 항상 일치한다.
//
// @spec SPEC-WEB-005 (store key 테이블 구성 가능 컬럼)

import { useEffect, useMemo, useRef, useState } from 'react';
import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  Check,
  ChevronDown,
  Columns3,
  ListFilter,
  X,
} from 'lucide-react';

import type { TranslationFn } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import {
  isColumnFilterActive,
  nextSortState,
  type ColumnFilter,
  type FilterColumnId,
  type SortColumn,
  type SortState,
} from './storeEntrySort';

/** 렌더 가능한 모든 컬럼 식별자(논리 순서 정의는 STORE_COLUMNS). */
export type StoreColumnId =
  | 'key'
  | 'metric'
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
 * 키 → 메트릭 → 값 → 네임스페이스 → 태그 → 바인딩 → TTL → 히스토리 → 갱신 → 액션.
 */
export const STORE_COLUMNS: readonly StoreColumn[] = [
  { id: 'key', labelKey: 'colKey', sortColumn: 'key', filterColumn: 'key', hideable: true, widthClass: 'min-w-[260px]' },
  { id: 'metric', labelKey: 'colMetric', sortColumn: 'metric', filterColumn: 'metric', hideable: true },
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

// ---- 클릭 아웃사이드 훅 ----

/** 지정 ref 바깥 클릭 시 콜백을 호출하는 훅(팝오버/드롭다운 닫기용). */
function useClickOutside(
  ref: React.RefObject<HTMLElement | null>,
  active: boolean,
  onOutside: () => void,
): void {
  useEffect(() => {
    if (!active) return;
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        onOutside();
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [ref, active, onOutside]);
}

// ---- 컬럼 설정(표시/숨김) 메뉴 ----

/**
 * 컬럼 표시/숨김 토글 메뉴. 톱니(Columns3) 버튼 클릭 시 팝오버로 각 숨김 가능
 * 컬럼을 체크박스로 노출한다. actions 는 숨길 수 없어 목록에서 제외한다.
 */
export function ColumnSettingsMenu({
  columns,
  visible,
  onToggle,
  t,
}: {
  /** 현재 관련(렌더 후보) 컬럼 목록. */
  columns: readonly StoreColumn[];
  visible: ReadonlySet<StoreColumnId>;
  onToggle: (id: StoreColumnId) => void;
  t: TranslationFn;
}) {
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  useClickOutside(containerRef, open, () => setOpen(false));

  const hideable = columns.filter((c) => c.hideable);

  return (
    <div className="relative" ref={containerRef}>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-haspopup="true"
        aria-expanded={open}
        data-testid="store-columns-settings"
        className="inline-flex items-center gap-1.5 rounded-md border border-(--color-border-default) bg-(--color-bg-primary) px-2.5 py-1.5 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-secondary)"
        title={t('agents.detail.store.columnsSettings')}
      >
        <Columns3 className="h-3.5 w-3.5" aria-hidden="true" />
        {t('agents.detail.store.columns')}
      </button>
      {open && (
        <div
          role="menu"
          className="absolute right-0 z-20 mt-1 min-w-[180px] rounded-md border border-(--color-border-default) bg-(--color-bg-primary) p-1.5 shadow-lg"
        >
          <p className="px-2 py-1 text-[11px] font-medium text-(--color-text-muted)">
            {t('agents.detail.store.columnsSettings')}
          </p>
          {hideable.map((c) => {
            const checked = visible.has(c.id);
            return (
              <label
                key={c.id}
                className="flex cursor-pointer items-center gap-2 rounded px-2 py-1 text-xs text-(--color-text-secondary) hover:bg-(--color-bg-secondary)"
              >
                <input
                  type="checkbox"
                  checked={checked}
                  onChange={() => onToggle(c.id)}
                  className="h-3.5 w-3.5"
                />
                {t(`agents.detail.store.${c.labelKey}`)}
              </label>
            );
          })}
        </div>
      )}
    </div>
  );
}

// ---- Excel 유사 컬럼 필터 버튼 ----

/** 빈 필터 상태(신규 컬럼 필터 초기값). */
export function emptyColumnFilter(): ColumnFilter {
  return { text: '', values: new Set<string>() };
}

/**
 * 컬럼 헤더의 Excel 유사 필터 버튼 + 드롭다운.
 *
 * 드롭다운 구성:
 *   - 부분일치 텍스트 입력(대소문자 무시)
 *   - 전체 선택/해제
 *   - 현재 로드된 전체 엔트리의 고유 값 체크박스 목록
 *   - 필터 초기화
 *
 * 활성(텍스트 또는 값 선택)일 때 버튼에 시각적 표시를 준다.
 */
export function ColumnFilterButton({
  label,
  uniqueValues,
  filter,
  onChange,
  grouped = false,
  t,
}: {
  label: string;
  uniqueValues: readonly string[];
  filter: ColumnFilter | undefined;
  onChange: (next: ColumnFilter) => void;
  /**
   * true 이면 값들을 "키=값" 의 키(= 앞부분) 기준으로 그룹핑해 표시한다.
   * 태그 컬럼처럼 값이 "tagKey=tagValue" 형태일 때 태그 종류별 선택을 지원한다.
   */
  grouped?: boolean;
  t: TranslationFn;
}) {
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  useClickOutside(containerRef, open, () => setOpen(false));

  const current = filter ?? emptyColumnFilter();
  const active = isColumnFilterActive(filter);

  // grouped 모드: "키=값" 을 키(= 앞부분) 기준으로 묶는다. `=` 가 없으면 값 전체를 키로 쓴다.
  // 필터 매칭은 여전히 전체 "키=값" 문자열 기준이므로, 그룹핑은 표시/선택 편의만 제공한다.
  const groups = useMemo(() => {
    if (!grouped) return null;
    const map = new Map<string, string[]>();
    for (const v of uniqueValues) {
      const eq = v.indexOf('=');
      const gk = eq >= 0 ? v.slice(0, eq) : v;
      const arr = map.get(gk);
      if (arr) arr.push(v);
      else map.set(gk, [v]);
    }
    return Array.from(map.entries()).sort((a, b) => a[0].localeCompare(b[0]));
  }, [grouped, uniqueValues]);

  const setText = (text: string) => {
    onChange({ text, values: new Set(current.values) });
  };

  const toggleValue = (value: string) => {
    const next = new Set(current.values);
    if (next.has(value)) next.delete(value);
    else next.add(value);
    onChange({ text: current.text, values: next });
  };

  // 그룹(태그 종류) 단위 전체 선택/해제. 그룹이 모두 선택돼 있으면 해제, 아니면 전체 선택.
  const toggleGroup = (pairs: string[], allSelected: boolean) => {
    const next = new Set(current.values);
    for (const p of pairs) {
      if (allSelected) next.delete(p);
      else next.add(p);
    }
    onChange({ text: current.text, values: next });
  };

  const selectAll = () => {
    onChange({ text: current.text, values: new Set(uniqueValues) });
  };

  const clearValues = () => {
    onChange({ text: current.text, values: new Set<string>() });
  };

  const clearAll = () => {
    onChange(emptyColumnFilter());
  };

  return (
    <span className="relative inline-flex" ref={containerRef}>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-haspopup="true"
        aria-expanded={open}
        data-testid={`store-filter-${label}`}
        className={cn(
          'inline-flex items-center rounded p-0.5 transition-colors',
          active
            ? 'text-blue-600 dark:text-blue-400'
            : 'text-(--color-text-muted) opacity-50 hover:opacity-100 hover:text-(--color-text-primary)',
        )}
        title={t('agents.detail.store.filterColumnAriaLabel').replace('{label}', label)}
        aria-label={t('agents.detail.store.filterColumnAriaLabel').replace('{label}', label)}
      >
        <ListFilter className="h-3 w-3" aria-hidden="true" />
        {active && (
          <span
            className="ml-0.5 inline-block h-1.5 w-1.5 rounded-full bg-blue-600 dark:bg-blue-400"
            aria-hidden="true"
          />
        )}
      </button>
      {open && (
        <div
          className={cn(
            'absolute left-0 top-full z-30 mt-1 rounded-md border border-(--color-border-default) bg-(--color-bg-primary) p-2 text-left shadow-lg',
            // 태그처럼 "키=값" 이 긴 그룹 모드는 전체 값이 보이도록 폭을 넓힌다.
            grouped ? 'w-80' : 'w-56',
          )}
        >
          <div className="relative mb-2">
            <input
              type="text"
              value={current.text}
              onChange={(e) => setText(e.target.value)}
              placeholder={t('agents.detail.store.filterTextPlaceholder')}
              aria-label={t('agents.detail.store.filterTextPlaceholder')}
              className="w-full rounded border border-(--color-border-default) bg-(--color-bg-surface) py-1 pl-2 pr-6 text-xs text-(--color-text-primary) placeholder:text-(--color-text-muted) focus:outline-none focus:ring-1 focus:ring-blue-500"
            />
            {current.text !== '' && (
              <button
                type="button"
                onClick={() => setText('')}
                aria-label={t('agents.detail.store.clearSearch')}
                className="absolute right-1 top-1/2 -translate-y-1/2 rounded p-0.5 text-(--color-text-muted) hover:text-(--color-text-primary)"
              >
                <X className="h-3 w-3" aria-hidden="true" />
              </button>
            )}
          </div>
          <div className="mb-1 flex items-center justify-between text-[11px]">
            <button
              type="button"
              onClick={selectAll}
              className="font-medium text-blue-600 hover:text-blue-800 dark:text-blue-400 dark:hover:text-blue-300"
            >
              {t('agents.detail.store.filterSelectAll')}
            </button>
            <button
              type="button"
              onClick={clearValues}
              className="font-medium text-(--color-text-muted) hover:text-(--color-text-primary)"
            >
              {t('agents.detail.store.filterClearValues')}
            </button>
          </div>
          <div className={cn('overflow-y-auto', grouped ? 'max-h-64' : 'max-h-40')}>
            {uniqueValues.length === 0 ? (
              <p className="px-1 py-1 text-[11px] text-(--color-text-muted)">
                {t('agents.detail.store.filterNoValues')}
              </p>
            ) : groups ? (
              // 그룹 모드(태그): 태그 종류(키)별로 묶고, 그룹 헤더에서 종류 단위 선택을 지원한다.
              // 각 값은 "값" 부분만 표시하되(키는 그룹 헤더가 대신함) 전체 "키=값" 은 title 로 노출한다.
              groups.map(([groupKey, pairs]) => {
                const selCount = pairs.reduce(
                  (n, p) => n + (current.values.has(p) ? 1 : 0),
                  0,
                );
                const allSel = selCount === pairs.length;
                const someSel = selCount > 0 && !allSel;
                return (
                  <div key={groupKey} className="mb-1.5 last:mb-0">
                    {/* 그룹 헤더: 태그 종류(키) + 종류 단위 전체 선택/해제 (indeterminate 지원) */}
                    <label className="flex cursor-pointer items-center gap-2 rounded px-1 py-0.5 hover:bg-(--color-bg-secondary)">
                      <input
                        type="checkbox"
                        checked={allSel}
                        ref={(el) => {
                          if (el) el.indeterminate = someSel;
                        }}
                        onChange={() => toggleGroup(pairs, allSel)}
                        className="h-3 w-3 shrink-0"
                      />
                      <span
                        className="truncate font-mono text-[11px] font-semibold text-(--color-text-primary)"
                        title={groupKey}
                      >
                        {groupKey === ''
                          ? t('agents.detail.store.filterEmptyValue')
                          : groupKey}
                      </span>
                      <span className="ml-auto shrink-0 text-[10px] text-(--color-text-muted)">
                        {selCount}/{pairs.length}
                      </span>
                    </label>
                    {/* 그룹 값 목록 */}
                    <div className="ml-2 border-l border-(--color-border-default) pl-1.5">
                      {pairs.map((p) => {
                        const eq = p.indexOf('=');
                        const valPart = eq >= 0 ? p.slice(eq + 1) : p;
                        const checked = current.values.has(p);
                        return (
                          <label
                            key={p}
                            className="flex cursor-pointer items-start gap-2 rounded px-1 py-0.5 text-xs text-(--color-text-secondary) hover:bg-(--color-bg-secondary)"
                          >
                            <input
                              type="checkbox"
                              checked={checked}
                              onChange={() => toggleValue(p)}
                              className="mt-0.5 h-3 w-3 shrink-0"
                            />
                            <span className="break-all font-mono text-[11px]" title={p}>
                              {valPart === ''
                                ? t('agents.detail.store.filterEmptyValue')
                                : valPart}
                            </span>
                          </label>
                        );
                      })}
                    </div>
                  </div>
                );
              })
            ) : (
              uniqueValues.map((v) => {
                const checked = current.values.has(v);
                return (
                  <label
                    key={v}
                    className="flex cursor-pointer items-center gap-2 rounded px-1 py-0.5 text-xs text-(--color-text-secondary) hover:bg-(--color-bg-secondary)"
                  >
                    <input
                      type="checkbox"
                      checked={checked}
                      onChange={() => toggleValue(v)}
                      className="h-3 w-3"
                    />
                    <span className="truncate font-mono text-[11px]" title={v}>
                      {v === '' ? t('agents.detail.store.filterEmptyValue') : v}
                    </span>
                  </label>
                );
              })
            )}
          </div>
          {active && (
            <button
              type="button"
              onClick={clearAll}
              className="mt-2 flex w-full items-center justify-center gap-1 rounded border border-(--color-border-default) py-1 text-[11px] font-medium text-(--color-text-secondary) hover:bg-(--color-bg-secondary)"
            >
              <Check className="h-3 w-3" aria-hidden="true" />
              {t('agents.detail.store.filterClearColumn')}
            </button>
          )}
        </div>
      )}
    </span>
  );
}

// ---- 컬럼 헤더(정렬 버튼 + Excel 필터) ----

/**
 * 컬럼 헤더 셀. 정렬 가능하면 정렬 버튼을, 아니면 평문 라벨을 렌더하고,
 * 필터 가능하면 Excel 유사 필터 버튼을 함께 렌더한다.
 */
export function ColumnHeader({
  column,
  sort,
  onSort,
  filter,
  uniqueValues,
  onFilterChange,
  keyExpanded,
  onToggleKeyExpanded,
  headerSlot,
  t,
}: {
  column: StoreColumn;
  sort: SortState;
  onSort: (next: SortState) => void;
  filter: ColumnFilter | undefined;
  uniqueValues: readonly string[];
  onFilterChange: (columnId: FilterColumnId, next: ColumnFilter) => void;
  /** 키 컬럼 전체 확장 상태(키 컬럼 헤더에서만 사용). */
  keyExpanded?: boolean;
  /** 키 컬럼 전체 확장 토글(키 컬럼 헤더에서만 사용). */
  onToggleKeyExpanded?: () => void;
  /**
   * 컬럼 헤더에 주입하는 커스텀 필터 어포던스(패널 설정 전용). 예: 태그 컬럼의 전용
   * AND 태그 피커 팝오버. 미주입 시 헤더는 기존과 동일하게 렌더된다(회귀 0).
   */
  headerSlot?: React.ReactNode;
  t: TranslationFn;
}) {
  const label = t(`agents.detail.store.${column.labelKey}`);
  const align = column.align ?? 'left';
  const sortable = column.sortColumn !== undefined;
  const active = sortable && sort.column === column.sortColumn;
  const ariaSort: React.AriaAttributes['aria-sort'] = active
    ? sort.direction === 'asc'
      ? 'ascending'
      : 'descending'
    : sortable
      ? 'none'
      : undefined;

  // 키 컬럼 헤더에서 전체 키 확장/축약을 한 번에 토글(개별 행 토글 대체).
  const showKeyToggle = column.id === 'key' && onToggleKeyExpanded !== undefined;

  return (
    <th
      className={cn(
        'px-3 py-2 font-medium text-(--color-text-muted)',
        align === 'right' ? 'text-right' : 'text-left',
        column.widthClass,
      )}
      aria-sort={ariaSort}
    >
      <span
        className={cn(
          'inline-flex items-center gap-1',
          align === 'right' && 'flex-row-reverse',
        )}
      >
        {sortable && column.sortColumn ? (
          <button
            type="button"
            onClick={() => onSort(nextSortState(sort, column.sortColumn!))}
            className={cn(
              'inline-flex items-center gap-1 transition-colors hover:text-(--color-text-primary)',
              active && 'text-(--color-text-primary)',
            )}
            aria-label={t('agents.detail.store.sortAriaLabel').replace('{label}', label)}
          >
            {label}
            {active ? (
              sort.direction === 'asc' ? (
                <ArrowUp className="h-3 w-3" aria-hidden="true" />
              ) : (
                <ArrowDown className="h-3 w-3" aria-hidden="true" />
              )
            ) : (
              <ArrowUpDown className="h-3 w-3 opacity-40" aria-hidden="true" />
            )}
          </button>
        ) : (
          <span>{label}</span>
        )}
        {column.filterColumn && (
          <ColumnFilterButton
            label={label}
            uniqueValues={uniqueValues}
            filter={filter}
            onChange={(next) => onFilterChange(column.filterColumn!, next)}
            grouped={column.filterColumn === 'tags'}
            t={t}
          />
        )}
        {/* 패널 설정 전용: 컬럼 헤더 커스텀 슬롯(태그 컬럼의 전용 AND 태그 피커). */}
        {headerSlot}
        {showKeyToggle && (
          <button
            type="button"
            onClick={onToggleKeyExpanded}
            className={cn(
              'inline-flex items-center rounded px-0.5 text-(--color-text-muted) transition-colors hover:text-(--color-text-primary)',
              keyExpanded && 'text-(--color-text-primary)',
            )}
            aria-pressed={keyExpanded}
            aria-label={t(
              keyExpanded
                ? 'agents.detail.store.keyCollapseColumnAriaLabel'
                : 'agents.detail.store.keyExpandColumnAriaLabel',
            )}
            title={t(
              keyExpanded
                ? 'agents.detail.store.keyCollapseColumnAriaLabel'
                : 'agents.detail.store.keyExpandColumnAriaLabel',
            )}
          >
            <ChevronDown
              className={cn('h-3 w-3 transition-transform', keyExpanded && 'rotate-180')}
              aria-hidden="true"
            />
          </button>
        )}
      </span>
    </th>
  );
}
