// 시리즈 선택 테이블 (저장소 리스트 스타일).
//
// TSDB/Store 데이터 뷰어에서 "조회할 시리즈"를 고르는 표.
// - 저장소(StoreTab)처럼 시리즈를 평탄한 행 리스트로 출력한다.
// - 각 컬럼(필드/데이터타입/태그/등록)에 정렬 + 다중선택 필터(facet)를 둔다.
// - 선택은 컬럼 필터와 분리된 행 체크박스로 수행한다(필터로 좁히고 체크로 선택).
//
// 선택 상태(selectedIds)는 상위에서 관리하는 controlled 패턴이다 — 본 컴포넌트는
// 정렬/필터 상태만 내부에서 보유하고, 토글/일괄선택을 콜백으로 위임한다.
//
// @spec SPEC-WEB-005

import { Fragment, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { ArrowDown, ArrowUp, ArrowUpDown, Filter, Search } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { MetadataChips } from '@/components/property/MetadataChips';
import { SERIES_ID_SEPARATOR } from '@/services/api/seriesLabels';
import type {
  DataType,
  RegistrationSource,
} from '@/services/api/store';

/** 테이블의 한 행 = 하나의 시리즈(저장소 기준 분류: key + field + tags). */
export interface SeriesRow {
  /** 결정적 SeriesID (선택 키). */
  id: string;
  key: string;
  field: string;
  dataType: string;
  /** 'manual' | 'auto' | '' (TSDB 합성/미상). */
  registration: string;
  tags: Record<string, string>;
  /**
   * 시리즈 키 옆에 붙는 짧은 배지(선택).
   *
   * 대시보드 편집기에서 "현재 검색 조건에는 없지만 등록되어 있는 행" 을 같은 표에
   * 남겨 두기 위해 쓴다 — 등록은 검색 조건과 독립적으로 누적되므로, 표에서 빼면
   * 편집할 자리가 사라진다.
   */
  badge?: string;
}

/** 정렬 가능한 컬럼. */
type SortColumn = 'key' | 'field' | 'dataType' | 'tags' | 'registration';
type SortDirection = 'asc' | 'desc';

/** 다중선택 facet 필터를 갖는 컬럼. */
type FacetColumn = 'field' | 'dataType' | 'tags' | 'registration';

export interface SeriesSelectTableProps {
  rows: SeriesRow[];
  /** 선택된 SeriesID 목록(controlled). */
  selectedIds: string[];
  /** 단일 행 토글. */
  onToggle: (id: string) => void;
  /** 일괄 선택(주어진 id 들을 선택에 추가). */
  onSelectMany: (ids: string[]) => void;
  /** 일괄 해제(주어진 id 들을 선택에서 제거). */
  onClearMany: (ids: string[]) => void;
  /** 등록(registration) 컬럼 노출 여부(Store 모드에서만 의미 있음). */
  showRegistration: boolean;
  /**
   * 행 아래에 덧붙일 편집 영역(선택). `null` 을 반환하면 아무것도 그리지 않는다.
   *
   * 선택한 행에 대한 설정(이름 · 색)을 표 바깥의 두 번째 목록으로 빼지 않고 그
   * 자리에서 편집하기 위한 통로다. 표 자체는 이 내용을 모른 채 자리만 내준다.
   */
  renderRowDetail?: (row: SeriesRow) => React.ReactNode;
}

/** tags 를 정렬/표시용으로 결정적 직렬화한다(키 사전순). */
function serializeTags(tags: Record<string, string>): string {
  const keys = Object.keys(tags).sort();
  return keys.map((k) => `${k}=${tags[k]}`).join(',');
}

/** 한 행이 tags facet(선택된 key=value 쌍)을 모두 만족하는지(AND). */
function matchesTagPairs(tags: Record<string, string>, selected: Set<string>): boolean {
  for (const pair of selected) {
    const eq = pair.indexOf('=');
    const k = pair.slice(0, eq);
    const v = pair.slice(eq + 1);
    if (tags[k] !== v) return false;
  }
  return true;
}

/** 컬럼 값 추출기 — 정렬/필터에 공통 사용. */
function columnValue(row: SeriesRow, col: SortColumn): string {
  switch (col) {
    case 'key':
      return row.key;
    case 'field':
      return row.field || 'unknown';
    case 'dataType':
      return row.dataType;
    case 'registration':
      return row.registration;
    case 'tags':
      return serializeTags(row.tags);
  }
}

export function SeriesSelectTable({
  rows,
  selectedIds,
  onToggle,
  onSelectMany,
  onClearMany,
  showRegistration,
  renderRowDetail,
}: SeriesSelectTableProps) {
  const { t } = useTranslation();
  const [sort, setSort] = useState<{ column: SortColumn; direction: SortDirection }>(
    { column: 'key', direction: 'asc' },
  );
  const [keySearch, setKeySearch] = useState('');
  // facet 필터: 컬럼 → 선택된 값 집합. 비어 있으면 그 컬럼은 전체 통과.
  const [metricSel, setMetricSel] = useState<Set<string>>(new Set());
  const [dataTypeSel, setDataTypeSel] = useState<Set<string>>(new Set());
  const [registrationSel, setRegistrationSel] = useState<Set<string>>(new Set());
  const [tagSel, setTagSel] = useState<Set<string>>(new Set());
  // 현재 열려 있는 필터 팝오버 컬럼(없으면 null)과, 그때 버튼의 화면 위치.
  //
  // 팝오버를 **표 바깥(body)** 에 그리기 때문에 위치를 따로 들고 있어야 한다.
  // 표 안에 그리면 `overflow-auto` 컨테이너에 잘려 아래 항목에 닿을 수 없고,
  // `sticky` + `z-index` 인 thead 가 쌓임 맥락을 만들어 바깥 백드롭에 덮인다 —
  // 두 증상("스크롤 안 됨", "클릭하면 사라짐")이 모두 여기서 나왔다.
  const [openFilter, setOpenFilter] = useState<FacetColumn | 'key' | null>(null);
  const [anchor, setAnchor] = useState<{ left: number; top: number } | null>(null);
  const popoverRef = useRef<HTMLDivElement | null>(null);

  const closeFilter = useCallback(() => {
    setOpenFilter(null);
    setAnchor(null);
  }, []);

  /** 필터 버튼 토글 — 열 때 버튼 바로 아래를 앵커로 잡는다. */
  const toggleFilter = useCallback(
    (facet: FacetColumn | 'key', btn: HTMLElement) => {
      setOpenFilter((cur) => {
        if (cur === facet) {
          setAnchor(null);
          return null;
        }
        const r = btn.getBoundingClientRect();
        setAnchor({ left: r.left, top: r.bottom + 4 });
        return facet;
      });
    },
    [],
  );

  // 바깥 클릭으로 닫는다. 백드롭 대신 문서 리스너를 쓰는 이유는 쌓임 맥락 때문이다 —
  // 백드롭은 z-index 로 팝오버 위·아래를 다투지만, 리스너는 그 다툼 자체가 없다.
  useEffect(() => {
    if (openFilter === null) return;
    const onDown = (e: MouseEvent): void => {
      const target = e.target as Node;
      if (popoverRef.current?.contains(target)) return;
      // 필터 버튼 자신의 클릭은 토글이 처리한다 — 여기서 닫으면 열리자마자 닫힌다.
      if (target instanceof Element && target.closest('[data-series-filter-button]')) return;
      closeFilter();
    };
    // 표가 스크롤되면 앵커가 어긋난다. 따라가게 만드는 대신 닫는다 —
    // 위치만 맞추면 팝오버가 표 밖 엉뚱한 자리에 떠 있게 된다.
    const onScrollOrResize = (): void => closeFilter();
    document.addEventListener('mousedown', onDown);
    window.addEventListener('resize', onScrollOrResize);
    window.addEventListener('scroll', onScrollOrResize, true);
    return () => {
      document.removeEventListener('mousedown', onDown);
      window.removeEventListener('resize', onScrollOrResize);
      window.removeEventListener('scroll', onScrollOrResize, true);
    };
  }, [openFilter, closeFilter]);

  const selectedSet = useMemo(() => new Set(selectedIds), [selectedIds]);

  // 각 facet 컬럼의 distinct 값(전체 행 기준 — 필터 적용 후에도 옵션이 사라지지 않게).
  const facetOptions = useMemo(() => {
    const field = new Set<string>();
    const dataType = new Set<string>();
    const registration = new Set<string>();
    const tags = new Set<string>();
    for (const r of rows) {
      field.add(r.field || 'unknown');
      if (r.dataType) dataType.add(r.dataType);
      if (r.registration) registration.add(r.registration);
      for (const [k, v] of Object.entries(r.tags)) tags.add(`${k}=${v}`);
    }
    const sortArr = (s: Set<string>) => Array.from(s).sort();
    return {
      field: sortArr(field),
      dataType: sortArr(dataType),
      registration: sortArr(registration),
      tags: sortArr(tags),
    };
  }, [rows]);

  const filteredRows = useMemo(() => {
    const q = keySearch.trim().toLowerCase();
    const result = rows.filter((r) => {
      if (q && !r.key.toLowerCase().includes(q)) return false;
      if (metricSel.size > 0 && !metricSel.has(r.field || 'unknown')) return false;
      if (dataTypeSel.size > 0 && !dataTypeSel.has(r.dataType)) return false;
      if (registrationSel.size > 0 && !registrationSel.has(r.registration)) return false;
      if (tagSel.size > 0 && !matchesTagPairs(r.tags, tagSel)) return false;
      return true;
    });
    const dir = sort.direction === 'asc' ? 1 : -1;
    return result.sort((a, b) => {
      const primary = columnValue(a, sort.column).localeCompare(
        columnValue(b, sort.column),
        undefined,
        { numeric: true },
      );
      if (primary !== 0) return primary * dir;
      // 2차 정렬: key → id 로 결정적 순서 보장.
      const byKey = a.key.localeCompare(b.key);
      if (byKey !== 0) return byKey;
      return a.id.localeCompare(b.id);
    });
  }, [
    rows,
    keySearch,
    metricSel,
    dataTypeSel,
    registrationSel,
    tagSel,
    sort,
  ]);

  const filteredIds = useMemo(() => filteredRows.map((r) => r.id), [filteredRows]);

  const handleSort = useCallback((column: SortColumn) => {
    setSort((prev) =>
      prev.column === column
        ? { column, direction: prev.direction === 'asc' ? 'desc' : 'asc' }
        : { column, direction: 'asc' },
    );
  }, []);

  const facetState: Record<
    FacetColumn,
    [Set<string>, React.Dispatch<React.SetStateAction<Set<string>>>]
  > = {
    field: [metricSel, setMetricSel],
    dataType: [dataTypeSel, setDataTypeSel],
    registration: [registrationSel, setRegistrationSel],
    tags: [tagSel, setTagSel],
  };

  const toggleFacetValue = useCallback(
    (column: FacetColumn, value: string) => {
      const [, setter] = facetState[column];
      setter((prev) => {
        const next = new Set(prev);
        if (next.has(value)) next.delete(value);
        else next.add(value);
        return next;
      });
    },
    // facetState 는 매 렌더 새로 만들어지지만 setter 들은 안정적이다.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  );

  const clearFacet = useCallback((column: FacetColumn) => {
    facetState[column][1](new Set());
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // ---- 렌더 헬퍼 ----

  const sortIcon = (column: SortColumn) => {
    if (sort.column !== column) {
      return <ArrowUpDown className="h-3 w-3 text-(--color-text-muted)" aria-hidden="true" />;
    }
    return sort.direction === 'asc' ? (
      <ArrowUp className="h-3 w-3 text-blue-600" aria-hidden="true" />
    ) : (
      <ArrowDown className="h-3 w-3 text-blue-600" aria-hidden="true" />
    );
  };

  /** 정렬 + 필터 버튼을 가진 컬럼 헤더 셀. */
  const headerCell = (
    column: SortColumn,
    label: string,
    facet: FacetColumn | 'key',
  ) => {
    const facetActive =
      facet === 'key'
        ? keySearch.trim().length > 0
        : facetState[facet][0].size > 0;
    return (
      <th
        scope="col"
        className="relative whitespace-nowrap px-2 py-1.5 text-left font-medium text-(--color-text-secondary)"
        data-testid={`series-col-${column}`}
      >
        <div className="flex items-center gap-1">
          <button
            type="button"
            onClick={() => handleSort(column)}
            className="flex items-center gap-1 hover:text-(--color-text-primary)"
            data-testid={`series-sort-${column}`}
            aria-label={t('series.sortAriaLabel').replace('{label}', label)}
          >
            {label}
            {sortIcon(column)}
          </button>
          <button
            type="button"
            data-series-filter-button=""
            onClick={(e) => toggleFilter(facet, e.currentTarget)}
            className={`rounded p-0.5 hover:bg-(--color-bg-elevated) ${
              facetActive ? 'text-blue-600' : 'text-(--color-text-muted)'
            }`}
            data-testid={`series-filter-${column}`}
            aria-label={t('series.filterAriaLabel').replace('{label}', label)}
            aria-expanded={openFilter === facet}
          >
            <Filter className="h-3 w-3" aria-hidden="true" />
          </button>
        </div>
        {openFilter === facet &&
          // body 에 그린다 — 표의 `overflow-auto` 에 잘리지 않고, sticky thead 의
          // 쌓임 맥락에도 갇히지 않는다.
          createPortal(
            <div
              ref={popoverRef}
              style={{ left: anchor?.left ?? 0, top: anchor?.top ?? 0 }}
              className="fixed z-50 max-h-64 w-56 overflow-y-auto rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) p-2 text-xs shadow-lg"
              data-testid={`series-filter-popover-${column}`}
            >
              {facet === 'key' ? (
                <div className="relative">
                  <Search
                    className="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-(--color-text-muted)"
                    aria-hidden="true"
                  />
                  <input
                    type="text"
                    autoFocus
                    placeholder={t('series.keySearchPlaceholder')}
                    value={keySearch}
                    onChange={(e) => setKeySearch(e.target.value)}
                    data-testid="series-filter-key-input"
                    className="block w-full rounded border border-(--color-border-strong) bg-(--color-bg-primary) pl-7 pr-2 py-1 text-(--color-text-primary) focus:border-blue-500 focus:outline-none"
                  />
                </div>
              ) : (
                <FacetList
                  column={facet}
                  options={facetOptions[facet]}
                  selected={facetState[facet][0]}
                  onToggle={(v) => toggleFacetValue(facet, v)}
                  onClear={() => clearFacet(facet)}
                />
              )}
            </div>,
            document.body,
          )}
      </th>
    );
  };

  const colCount = 2 + 3 + (showRegistration ? 1 : 0); // checkbox + key + field + dataType + tags (+registration)

  return (
    <div className="flex flex-col gap-2">
      {/* 일괄 선택/해제 + 선택 요약 (#4). */}
      <div className="flex items-center gap-2 text-xs">
        <button
          type="button"
          onClick={() => onSelectMany(filteredIds)}
          disabled={filteredIds.length === 0}
          data-testid="tsdb-select-all"
          className="rounded border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-1 font-medium text-(--color-text-secondary) hover:bg-(--color-bg-elevated) disabled:opacity-50"
        >
          {t('series.selectAll').replace('{count}', String(filteredIds.length))}
        </button>
        <button
          type="button"
          onClick={() => onClearMany(filteredIds)}
          disabled={filteredIds.length === 0}
          data-testid="tsdb-clear-all"
          className="rounded border border-(--color-border-strong) bg-(--color-bg-surface) px-2 py-1 font-medium text-(--color-text-secondary) hover:bg-(--color-bg-elevated) disabled:opacity-50"
        >
          {t('series.clearAll')}
        </button>
      </div>

      <div className="max-h-[45vh] overflow-auto rounded-md border border-(--color-border-default) bg-(--color-bg-primary)">
        <table className="w-full border-collapse text-xs">
          <thead className="sticky top-0 z-[1] bg-(--color-bg-surface)">
            <tr className="border-b border-(--color-border-default)">
              <th scope="col" className="w-8 px-2 py-1.5" aria-label={t('series.colSelect')} />
              {headerCell('key', t('series.seriesKey'), 'key')}
              {headerCell('field', t('series.field'), 'field')}
              {headerCell('dataType', t('series.dataType'), 'dataType')}
              {headerCell('tags', t('series.tags'), 'tags')}
              {showRegistration && headerCell('registration', t('series.registration'), 'registration')}
            </tr>
          </thead>
          <tbody>
            {filteredRows.length === 0 ? (
              <tr>
                <td
                  colSpan={colCount}
                  className="p-3 text-center text-(--color-text-muted)"
                >
                  {t('series.noMatchingSeries')}
                </td>
              </tr>
            ) : (
              filteredRows.map((r) => {
                const checked = selectedSet.has(r.id);
                const tagEntries = Object.entries(r.tags);
                // SeriesID 의 NUL 구분자를 '~' 로 치환한 testid (같은 key 다중 시리즈 충돌 방지).
                const safeId = r.id.split(SERIES_ID_SEPARATOR).join('~');
                const row = (
                  <tr
                    key={r.id}
                    className="border-b border-(--color-border-default) last:border-b-0 hover:bg-(--color-bg-elevated)"
                    data-testid={`series-tr-${safeId}`}
                  >
                    <td className="px-2 py-1">
                      <input
                        type="checkbox"
                        checked={checked}
                        onChange={() => onToggle(r.id)}
                        data-testid={`series-select-${safeId}`}
                        aria-label={t('series.selectRowAriaLabel').replace('{key}', r.key)}
                        className="h-3.5 w-3.5 rounded border-gray-300 text-blue-600"
                      />
                    </td>
                    <td className="max-w-[16rem] truncate px-2 py-1 font-mono text-(--color-text-primary)">
                      {r.key}
                      {r.badge !== undefined && r.badge !== '' && (
                        <span
                          data-testid={`series-badge-${safeId}`}
                          className="ml-1 rounded bg-(--color-bg-surface) px-1 py-0.5 text-[10px] font-medium text-(--color-text-muted)"
                        >
                          {r.badge}
                        </span>
                      )}
                    </td>
                    <td className="px-2 py-1">
                      <MetadataChips
                        fieldName={r.field}
                        showAutoBadge={false}
                        className="shrink-0"
                      />
                    </td>
                    <td className="px-2 py-1">
                      <MetadataChips
                        dataType={r.dataType as DataType | undefined}
                        showAutoBadge={false}
                        className="shrink-0"
                      />
                    </td>
                    <td className="px-2 py-1">
                      {tagEntries.length > 0 ? (
                        <span className="flex flex-wrap items-center gap-1">
                          {tagEntries.map(([tk, tv]) => (
                            <span
                              key={`${r.id}-tag-${tk}`}
                              className="rounded bg-(--color-bg-surface) px-1.5 py-0.5 text-[10px] font-mono font-medium text-(--color-text-muted)"
                            >
                              {tk}={tv}
                            </span>
                          ))}
                        </span>
                      ) : (
                        <span className="text-(--color-text-muted)">—</span>
                      )}
                    </td>
                    {showRegistration && (
                      <td className="px-2 py-1">
                        <MetadataChips
                          registration={r.registration as RegistrationSource | undefined}
                          showAutoBadge
                          className="shrink-0"
                        />
                      </td>
                    )}
                  </tr>
                );
                const detail = renderRowDetail?.(r);
                if (!detail) return row;
                return (
                  <Fragment key={r.id}>
                    {row}
                    <tr
                      className="border-b border-(--color-border-default) last:border-b-0"
                      data-testid={`series-detail-${safeId}`}
                    >
                      <td />
                      <td colSpan={colCount - 1} className="px-2 pb-1.5">
                        {detail}
                      </td>
                    </tr>
                  </Fragment>
                );
              })
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}

/** facet 필터 팝오버 내부의 값 다중선택 리스트. */
function FacetList({
  column,
  options,
  selected,
  onToggle,
  onClear,
}: {
  column: FacetColumn;
  options: string[];
  selected: Set<string>;
  onToggle: (value: string) => void;
  onClear: () => void;
}) {
  const { t } = useTranslation();
  if (options.length === 0) {
    return <p className="px-1 py-2 text-(--color-text-muted)">{t('series.facetNoValues')}</p>;
  }
  return (
    <div className="flex flex-col gap-1">
      <button
        type="button"
        onClick={onClear}
        disabled={selected.size === 0}
        data-testid={`series-filter-clear-${column}`}
        className="self-start rounded px-1 py-0.5 text-[11px] text-blue-600 hover:underline disabled:text-(--color-text-muted) disabled:no-underline"
      >
        {t('series.facetAll').replace('{count}', String(options.length))}
      </button>
      {options.map((opt) => (
        <label
          key={opt}
          className="flex cursor-pointer items-center gap-2 rounded px-1 py-0.5 hover:bg-(--color-bg-elevated)"
        >
          <input
            type="checkbox"
            checked={selected.has(opt)}
            onChange={() => onToggle(opt)}
            data-testid={`series-filter-opt-${column}-${opt}`}
            className="h-3.5 w-3.5 rounded border-gray-300 text-blue-600"
          />
          <span className="truncate font-mono text-(--color-text-primary)">{opt}</span>
        </label>
      ))}
    </div>
  );
}

export default SeriesSelectTable;
