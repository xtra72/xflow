// 패널 설정 데이터소스 영역 — Store/TSDB 토글 + 공용 StoreEntryTable 선택 surface.
//
// @spec SPEC-PANEL-SETTINGS-001 (T4/T6/T7)
//
// - T4: Store/TSDB 토글. TSDB 는 실동작 없이 "후속 SPEC 안내" placeholder 만 렌더하며
//   기존 Store 설정을 파괴하지 않는다(REQ-05/AC-05).
// - T6: Store 모드에서 공용 StoreEntryTable 을 소비한다. 행 선행 체크박스(selection)로
//   선택 집합을 store_source.selected_keys(additive)로 반영하고, actions 컬럼 대신
//   Alias Name 컬럼을 렌더한다(renderCellExtra). (REQ-04/07/09)
// - T7: 필터/정렬/표시숨김을 패널별 localStorage 로 영속/복원한다(AC-08). 빈 store 는
//   graceful 안내로 처리한다(AC-10).
//
// 기존 StoreSourceSection(에이전트/시리즈/시간창 편집기)은 그대로 렌더하여 회귀 0 을
// 보장한다(additive). Store 엔트리는 useStoreKeysWithTags 의 keyObjects(메타데이터)에서
// 파생한다 — 선택 surface 이므로 라이브 값 컬럼(value/updated)은 노출하지 않는다.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ListFilter } from 'lucide-react';

import { useAgents } from '@/hooks/useAgent';
import { useTranslation, type TranslationFn } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import {
  ColumnSettingsMenu,
  STORE_COLUMNS,
  type StoreColumn,
  type StoreColumnId,
} from '@/pages/agents/storeColumns';
import {
  applyColumnFilters,
  sortEntries,
  uniqueColumnValues,
  type ColumnFilter,
  type FilterColumnId,
  type FilterContext,
  type SortState,
  type StoreEntry,
} from '@/pages/agents/storeEntrySort';
import { StoreEntryTable } from '@/pages/agents/StoreEntryTable';
import { useStoreKeysWithTags, type StoreKeyObject } from '@/services/api/store';
import type { PanelConfig } from '@/stores/uiStore';

import { resolveStoreAgentName } from './panels/charts/storeAgentResolve';
import {
  pickSeriesColor,
  storeSeriesId,
  STORE_SERIES_LIMIT,
  type StoreSeriesRef,
  type StoreSourceConfig,
} from './panels/charts/chartChannelTypes';
import { makeTagFilterId, matchesTagFilters } from './panels/charts/storeSourceFilter';
import { StoreSourceSection, StoreTagSelectionEditor } from './ChartPanelSections';
import {
  loadPanelStoreTablePrefs,
  savePanelStoreTablePrefs,
  type PanelStoreTablePrefs,
} from './panels/charts/panelStoreTablePrefs';

type OnConfig = (config: Record<string, unknown>) => void;

/** 패널 설정 컨텍스트는 read-only 이므로 행 액션 핸들러는 no-op 이다(액션 컬럼 미노출). */
const NOOP = (): void => {};

/**
 * 데이터소스 영역 컨테이너. 단일 "데이터 소스" 토글([채널 | Store | TSDB])과 각 모드의
 * 편집기/placeholder 는 `StoreSourceSection` 이 소유한다(중복 헤딩/토글 제거). 여기서는
 * StoreSourceSection 이 보고하는 모드를 읽어 Store 모드에서만 공용 선택 테이블을 렌더한다.
 */
export function PanelSettingsDataSource({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: OnConfig;
}) {
  // 바인딩 모드의 단일 소스 오브 트루스는 StoreSourceSection. 초기값은 config.data_source
  // (채널/Store)에서 파생하고(TSDB 는 UI 전용이라 항상 채널/Store 로 시작), 이후 콜백으로 동기화.
  const initialMode: 'channel' | 'store' | 'tsdb' =
    (panel.config?.data_source as string | undefined) === 'store' ? 'store' : 'channel';
  const [mode, setMode] = useState<'channel' | 'store' | 'tsdb'>(initialMode);

  return (
    <div className="space-y-3" data-testid="panel-datasource">
      {/* 단일 데이터소스 토글 + 채널/Store 편집기 + TSDB placeholder(모두 StoreSourceSection 소유). */}
      <StoreSourceSection
        panel={panel}
        onConfigChange={onConfigChange}
        onModeChange={setMode}
      />
      {/* 공용 StoreEntryTable 선택 surface — Store 모드에서 항상 노출(목록이 사라지지 않도록).
          목록 헤더에 전용 AND 태그 피커가 있어, 태그 필터가 있으면 매칭 행을 read-only
          미리보기로(selection_mode:'tag'), 없으면 체크박스로 series 를 직접 고른다. */}
      {mode === 'store' && (
        <PanelStoreSelectTable panel={panel} onConfigChange={onConfigChange} />
      )}
    </div>
  );
}

/** STORE_COLUMNS 레지스트리에서 특정 컬럼 정의를 얻는다. */
function pickColumn(id: StoreColumnId): StoreColumn {
  const col = STORE_COLUMNS.find((c) => c.id === id);
  if (!col) throw new Error(`unknown store column: ${id}`);
  return col;
}

/**
 * 패널 설정 컨텍스트에서 노출하는 표시가능(hideable) 메타데이터 컬럼(정렬/필터 대상).
 * tags 컬럼은 필터 대상에서 제외한다 — 태그 필터는 목록 헤더의 전용 AND 태그 피커가
 * 담당하므로(OR 컬럼 필터와의 의미 충돌 방지), 여기서는 key/metric 만 필터한다.
 */
const PANEL_FILTER_COLUMNS: readonly FilterColumnId[] = ['key', 'metric'];

/**
 * StoreKeyObject → StoreEntry(메타데이터 파생). 라이브 값(value/updated)은 없다.
 * data_type 은 series 항목 구성(StoreKeySelector 와 byte-호환)에 필요하므로 포함한다.
 */
function toStoreEntry(o: StoreKeyObject): StoreEntry {
  return {
    key: o.key,
    storage_key: o.key,
    metric_type: o.metric_type,
    tags: o.tags,
    data_type: o.data_type,
    registration: o.registration,
  };
}

/**
 * 공용 StoreEntryTable 을 소비하는 패널 설정 전용 선택 테이블.
 * 행 체크박스 → store_source.selected_keys, actions→Alias 치환, 필터/정렬/표시숨김 영속.
 */
function PanelStoreSelectTable({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: OnConfig;
}) {
  const { t } = useTranslation();
  const config = panel.config ?? {};
  const storeSource = config.store_source as StoreSourceConfig | undefined;

  const { data: agentsResult } = useAgents();
  const agentName = resolveStoreAgentName(
    storeSource?.agent_id,
    storeSource?.agent_name ?? '',
    agentsResult?.data,
  );
  const hasAgent = agentName !== '';

  const { data: keysData } = useStoreKeysWithTags(agentName);
  const keyObjects = useMemo(() => keysData?.keyObjects ?? [], [keysData?.keyObjects]);

  // keyObjects(메타데이터) → StoreEntry 파생. 라이브 값(value/namespace/updated)은 없다.
  const allEntries: StoreEntry[] = useMemo(
    () => keyObjects.map(toStoreEntry),
    [keyObjects],
  );

  // --- 필터/정렬/표시숨김 상태(패널별 localStorage 영속) ---
  const [prefs, setPrefs] = useState<PanelStoreTablePrefs>(() =>
    loadPanelStoreTablePrefs(panel.id),
  );
  useEffect(() => {
    setPrefs(loadPanelStoreTablePrefs(panel.id));
  }, [panel.id]);
  const persist = useCallback(
    (next: PanelStoreTablePrefs) => {
      setPrefs(next);
      savePanelStoreTablePrefs(panel.id, next);
    },
    [panel.id],
  );

  const hidden = useMemo(() => new Set(prefs.hidden), [prefs.hidden]);

  const filterCtx: FilterContext = useMemo(
    () => ({
      staticKeyNames: new Set<string>(),
      bindingLabels: {
        static: t('agents.detail.store.static'),
        dynamic: t('agents.detail.store.dynamic'),
      },
    }),
    [t],
  );

  const showTags = useMemo(
    () => allEntries.some((e) => e.tags && Object.keys(e.tags as object).length > 0),
    [allEntries],
  );

  // 관련(표시가능) 컬럼: key, metric, (tags 있을 때) tags.
  // tags 컬럼은 표시/숨김만 — 필터(filterColumn)는 제거해 목록 헤더의 전용 AND 태그
  // 피커가 유일한 태그 컨트롤이 되도록 한다(OR 컬럼 필터와의 의미 충돌 방지).
  const relevantCols = useMemo(() => {
    const base = [pickColumn('key'), pickColumn('metric')];
    if (showTags) base.push({ ...pickColumn('tags'), filterColumn: undefined });
    return base;
  }, [showTags]);

  // 렌더 컬럼: 표시가능 컬럼 중 숨김 제외 + Alias(actions 대체) 컬럼.
  const columns: StoreColumn[] = useMemo(() => {
    const visible = relevantCols.filter((c) => !hidden.has(c.id));
    const alias: StoreColumn = { id: 'alias', labelKey: 'colAlias', hideable: false };
    return [...visible, alias];
  }, [relevantCols, hidden]);

  // --- 태그 자동 바인딩(tag 모드) ---
  // 목록 헤더의 전용 AND 태그 피커가 tag_filters(Record<string,string>)를 구성한다.
  // tag_filters 가 하나라도 있으면 tag 모드로 함축되어(별도 토글 없음), 매칭 행을
  // read-only 미리보기로 보여준다. @spec SPEC-PANEL-SETTINGS-001 (태그 인 헤더)
  const tagFilters = useMemo(
    () => storeSource?.tag_filters ?? {},
    [storeSource?.tag_filters],
  );
  const tagFilterSet = useMemo(
    () => new Set(Object.entries(tagFilters).map(([k, v]) => makeTagFilterId(k, v))),
    [tagFilters],
  );
  const isTagMode = tagFilterSet.size > 0;

  // 필터 → 정렬 파이프라인.
  // tag 모드: AND 매처(storeSourceFilter.matchesTagFilters)로 매칭 행만 — 공용 OR 컬럼
  // 필터는 우회한다(미리보기 = 실제 바인딩 집합 일치). keys 모드: 기존 컬럼 필터 파이프라인.
  const entries = useMemo(() => {
    const base = isTagMode
      ? keyObjects.filter((o) => matchesTagFilters(o, tagFilterSet)).map(toStoreEntry)
      : applyColumnFilters(allEntries, prefs.filters, filterCtx);
    return sortEntries(base, prefs.sort, { staticKeyNames: new Set<string>() });
  }, [isTagMode, keyObjects, tagFilterSet, allEntries, prefs.filters, prefs.sort, filterCtx]);

  const uniqueValuesByColumn = useMemo(() => {
    const map = new Map<FilterColumnId, string[]>();
    for (const col of PANEL_FILTER_COLUMNS) {
      map.set(col, uniqueColumnValues(allEntries, col, filterCtx));
    }
    return map;
  }, [allEntries, filterCtx]);

  // 선택의 단일 소스 오브 트루스는 store_source.series(keys 모드). 체크박스는 이 series 를
  // StoreKeySelector 와 byte-호환 형태로 추가/제거한다(렌더 경로 불변). @spec SPEC-PANEL-SETTINGS-001
  const series = useMemo<StoreSeriesRef[]>(() => storeSource?.series ?? [], [storeSource?.series]);
  const seriesIds = useMemo(
    () => new Set(series.map((s) => storeSeriesId(s.key, s.metric_type ?? '', s.tags ?? {}))),
    [series],
  );
  const entryToSeriesId = (entry: StoreEntry): string =>
    storeSeriesId(
      entry.key as string,
      (entry.metric_type as string) ?? '',
      (entry.tags as Record<string, string>) ?? {},
    );

  const handleSort = useCallback(
    (next: SortState) => persist({ ...prefs, sort: next }),
    [persist, prefs],
  );

  const handleColumnFilterChange = useCallback(
    (columnId: FilterColumnId, next: ColumnFilter) => {
      const filters = { ...prefs.filters };
      if (next.text.trim() !== '' || next.values.size > 0) {
        filters[columnId] = next;
      } else {
        delete filters[columnId];
      }
      persist({ ...prefs, filters });
    },
    [persist, prefs],
  );

  const handleToggleColumn = useCallback(
    (id: StoreColumnId) => {
      const nextHidden = new Set(prefs.hidden);
      if (nextHidden.has(id)) nextHidden.delete(id);
      else nextHidden.add(id);
      persist({ ...prefs, hidden: Array.from(nextHidden) });
    },
    [persist, prefs],
  );

  const [keyExpanded, setKeyExpanded] = useState(false);
  // 선택 상한 초과 안내(AC-15). 상한 도달 상태에서 추가 시도 시 표시한다.
  const [overLimitNotice, setOverLimitNotice] = useState(false);

  const handleToggleSelection = useCallback(
    (entry: StoreEntry) => {
      const id = entryToSeriesId(entry);
      if (seriesIds.has(id)) {
        // 제거: 동일 seriesId 항목을 series 에서 뺀다.
        setOverLimitNotice(false);
        const next = series.filter(
          (s) => storeSeriesId(s.key, s.metric_type ?? '', s.tags ?? {}) !== id,
        );
        onConfigChange({ store_source: { ...(storeSource ?? {}), series: next } });
        return;
      }
      // 추가: 상한 초과 시 억제 + 안내(미리보기 성능 보호, AC-15 — series 개수 기준).
      if (series.length >= STORE_SERIES_LIMIT) {
        setOverLimitNotice(true);
        return;
      }
      setOverLimitNotice(false);
      // StoreKeySelector 와 동일한 series 항목 형태(byte-호환)로 추가한다.
      const nextEntry: StoreSeriesRef = {
        key: entry.key as string,
        metric_type: (entry.metric_type as string) || undefined,
        tags:
          entry.tags && Object.keys(entry.tags as object).length > 0
            ? (entry.tags as Record<string, string>)
            : undefined,
        data_type: entry.data_type as StoreSeriesRef['data_type'],
        alias: entry.key as string,
        color: pickSeriesColor(series.length),
      };
      onConfigChange({ store_source: { ...(storeSource ?? {}), series: [...series, nextEntry] } });
    },
    [series, seriesIds, storeSource, onConfigChange],
  );

  // 목록 헤더의 전용 태그 피커 변경 → tag_filters + selection_mode 함축 갱신.
  // 태그 값 형태는 구 StoreTagSelectionEditor 와 byte-호환(Record<string,string>, AND).
  // 태그가 하나라도 있으면 tag 모드, 모두 지우면 keys 모드로 복귀(series 는 보존).
  const handleTagFiltersChange = useCallback(
    (nextTagFilters: Record<string, string>) => {
      const hasTags = Object.keys(nextTagFilters).length > 0;
      onConfigChange({
        store_source: {
          ...(storeSource ?? {}),
          tag_filters: hasTags ? nextTagFilters : undefined,
          selection_mode: hasTags ? 'tag' : 'keys',
        },
      });
    },
    [storeSource, onConfigChange],
  );

  // ColumnSettingsMenu 는 visible 집합을 기대한다(hidden 의 여집합).
  const visibleColumnSet = useMemo(() => {
    const s = new Set<StoreColumnId>();
    for (const c of relevantCols) if (!hidden.has(c.id)) s.add(c.id);
    return s;
  }, [relevantCols, hidden]);

  return (
    <div className="space-y-2" data-testid="panel-store-select">
      <div className="flex items-center justify-between">
        <label className="text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.settings.dataSourceStoreSelectTitle')}
        </label>
        <ColumnSettingsMenu
          columns={relevantCols}
          visible={visibleColumnSet}
          onToggle={handleToggleColumn}
          t={t}
        />
      </div>

      {overLimitNotice && (
        <p
          data-testid="panel-store-select-overlimit"
          className="rounded-md border border-amber-300 bg-amber-50 px-3 py-1.5 text-[11px] text-amber-700 dark:border-amber-900/40 dark:bg-amber-900/20 dark:text-amber-300"
        >
          {t('dashboard.settings.dataSourceStoreSelectOverLimit').replace(
            '{limit}',
            String(STORE_SERIES_LIMIT),
          )}
        </p>
      )}

      {!hasAgent ? (
        <p className="rounded-md border border-(--color-border-default) bg-(--color-bg-primary) p-4 text-center text-xs text-(--color-text-muted)">
          {t('dashboard.settings.dataSourceStoreSelectNoAgent')}
        </p>
      ) : entries.length === 0 ? (
        <p className="rounded-md border border-(--color-border-default) bg-(--color-bg-primary) p-4 text-center text-xs text-(--color-text-muted)">
          {t('dashboard.settings.dataSourceStoreSelectEmpty')}
        </p>
      ) : (
        <StoreEntryTable
          entries={entries}
          columns={columns}
          sort={prefs.sort}
          onSort={handleSort}
          columnFilters={prefs.filters}
          onColumnFilterChange={handleColumnFilterChange}
          uniqueValuesByColumn={uniqueValuesByColumn}
          keyColumnExpanded={keyExpanded}
          onToggleKeyExpanded={() => setKeyExpanded((v) => !v)}
          maxHistorySize={0}
          staticKeyNames={new Set<string>()}
          rowActions={{
            agentId: '',
            onPromote: NOOP,
            onRename: NOOP,
            onReset: NOOP,
            onEditMeta: NOOP,
            readOnly: true,
          }}
          selection={{
            // tag 모드: 모든 행이 바인딩 집합(read-only) → 체크 표시 + 토글 무동작.
            // keys 모드: series 기준 선택 + 체크박스로 명시적 series 편집.
            isSelected: isTagMode ? () => true : (e) => seriesIds.has(entryToSeriesId(e)),
            onToggle: isTagMode ? NOOP : handleToggleSelection,
          }}
          renderCellExtra={(col, entry) =>
            col === 'alias' ? (
              <span
                className="font-mono text-xs text-(--color-text-secondary)"
                title={entry.key as string}
              >
                {entry.key as string}
              </span>
            ) : undefined
          }
          // 태그 컬럼 헤더에 전용 AND 태그 피커 팝오버를 주입한다(태그 인 헤더).
          columnHeaderSlots={{
            tags: (
              <TagsColumnHeaderFilter
                agentName={agentName}
                tagFilters={tagFilters}
                onChange={handleTagFiltersChange}
                t={t}
              />
            ),
          }}
        />
      )}
    </div>
  );
}

/**
 * 태그 컬럼 헤더의 전용 AND 태그 피커(팝오버). 다른 컬럼 헤더 필터(ColumnFilterButton)와
 * 동일한 어포던스(ListFilter 버튼 + 활성 점 + 팝오버)로 표시하되, 열면 one-value-per-key /
 * cross-key-AND 의 StoreTagSelectionEditor 를 띄워 tag_filters(Record<string,string>)를
 * 구성한다(byte-호환). 컬럼별 OR 필터와의 의미 충돌을 피하려고 tags 컬럼은 이 전용
 * 컨트롤만 갖는다(기본 ColumnFilterButton 미노출). @spec SPEC-PANEL-SETTINGS-001 (태그 인 헤더)
 */
function TagsColumnHeaderFilter({
  agentName,
  tagFilters,
  onChange,
  t,
}: {
  agentName: string;
  tagFilters: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
  t: TranslationFn;
}): React.ReactElement {
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLSpanElement>(null);

  // 팝오버 바깥 클릭 시 닫는다(다른 컬럼 필터 팝오버와 동일 동작).
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent): void => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [open]);

  const active = Object.keys(tagFilters).length > 0;

  return (
    <span className="relative inline-flex" ref={containerRef}>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-haspopup="true"
        aria-expanded={open}
        data-testid="panel-store-tags-header-filter"
        className={cn(
          'inline-flex items-center rounded p-0.5 transition-colors',
          active
            ? 'text-blue-600 dark:text-blue-400'
            : 'text-(--color-text-muted) opacity-50 hover:opacity-100 hover:text-(--color-text-primary)',
        )}
        title={t('dashboard.chart.storeTagPickerLabel')}
        aria-label={t('dashboard.chart.storeTagPickerLabel')}
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
        <div className="absolute left-0 top-full z-30 mt-1 w-72 rounded-md border border-(--color-border-default) bg-(--color-bg-primary) p-2 text-left shadow-lg">
          <StoreTagSelectionEditor
            agentName={agentName}
            tagFilters={tagFilters}
            onChange={onChange}
          />
        </div>
      )}
    </span>
  );
}
