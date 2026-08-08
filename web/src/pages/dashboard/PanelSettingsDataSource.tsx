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

import { useCallback, useEffect, useMemo, useState } from 'react';

import { useAgents } from '@/hooks/useAgent';
import { useTranslation } from '@/lib/i18n';
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
import { useStoreKeysWithTags } from '@/services/api/store';
import type { PanelConfig } from '@/stores/uiStore';

import { resolveStoreAgentName } from './panels/charts/storeAgentResolve';
import type { StoreSourceConfig } from './panels/charts/chartChannelTypes';
import { StoreSourceSection } from './ChartPanelSections';
import {
  loadPanelStoreTablePrefs,
  savePanelStoreTablePrefs,
  type PanelStoreTablePrefs,
} from './panels/charts/panelStoreTablePrefs';
import {
  isSelectionAtLimit,
  readSelectedKeys,
  SELECTED_KEYS_LIMIT,
  toggleSelectedKey,
} from './panels/charts/storeSelectedKeys';

type OnConfig = (config: Record<string, unknown>) => void;

/** 패널 설정 컨텍스트는 read-only 이므로 행 액션 핸들러는 no-op 이다(액션 컬럼 미노출). */
const NOOP = (): void => {};

/** 데이터소스 영역 컨테이너: Store/TSDB 토글 + 영역별 렌더. */
export function PanelSettingsDataSource({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: OnConfig;
}) {
  const { t } = useTranslation();
  // 데이터소스 백엔드 토글(로컬 UI 상태). TSDB 는 본 SPEC 에서 placeholder 만.
  const [mode, setMode] = useState<'store' | 'tsdb'>('store');

  return (
    <div className="space-y-3" data-testid="panel-datasource">
      <div
        role="tablist"
        aria-label={t('dashboard.settings.dataSource')}
        className="inline-flex rounded-md border border-(--color-border-default) bg-(--color-bg-surface) p-0.5"
      >
        {(['store', 'tsdb'] as const).map((kind) => {
          const selected = mode === kind;
          return (
            <button
              key={kind}
              type="button"
              role="tab"
              aria-selected={selected}
              data-testid={`panel-datasource-${kind}`}
              onClick={() => setMode(kind)}
              className={cn(
                'rounded px-3 py-1 text-xs font-medium transition-colors',
                selected
                  ? 'bg-blue-600 text-white'
                  : 'text-(--color-text-secondary) hover:bg-(--color-bg-elevated)',
              )}
            >
              {kind === 'store'
                ? t('dashboard.settings.dataSourceStoreTab')
                : t('dashboard.settings.dataSourceTsdbTab')}
            </button>
          );
        })}
      </div>

      {mode === 'store' ? (
        <>
          {/* 기존 에이전트/시리즈/시간창 편집기 — 로직 보존(회귀 0). */}
          <StoreSourceSection panel={panel} onConfigChange={onConfigChange} />
          {/* 공용 StoreEntryTable 선택 surface(체크박스 + Alias). */}
          <PanelStoreSelectTable panel={panel} onConfigChange={onConfigChange} />
        </>
      ) : (
        <div
          data-testid="panel-datasource-tsdb-placeholder"
          className="rounded-md border border-dashed border-(--color-border-default) bg-(--color-bg-elevated) p-4 text-center"
        >
          <p className="text-sm font-medium text-(--color-text-secondary)">
            {t('dashboard.settings.dataSourceTsdbTitle')}
          </p>
          <p className="mt-1 text-xs text-(--color-text-muted)">
            {t('dashboard.settings.dataSourceTsdbBody')}
          </p>
        </div>
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

/** 패널 설정 컨텍스트에서 노출하는 표시가능(hideable) 메타데이터 컬럼(정렬/필터 대상). */
const PANEL_FILTER_COLUMNS: readonly FilterColumnId[] = ['key', 'metric', 'tags'];

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
    () =>
      keyObjects.map((o) => ({
        key: o.key,
        storage_key: o.key,
        metric_type: o.metric_type,
        tags: o.tags,
        registration: o.registration,
      })),
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
  const relevantCols = useMemo(() => {
    const base = [pickColumn('key'), pickColumn('metric')];
    if (showTags) base.push(pickColumn('tags'));
    return base;
  }, [showTags]);

  // 렌더 컬럼: 표시가능 컬럼 중 숨김 제외 + Alias(actions 대체) 컬럼.
  const columns: StoreColumn[] = useMemo(() => {
    const visible = relevantCols.filter((c) => !hidden.has(c.id));
    const alias: StoreColumn = { id: 'alias', labelKey: 'colAlias', hideable: false };
    return [...visible, alias];
  }, [relevantCols, hidden]);

  // 필터 → 정렬 파이프라인(공용 순수 함수 재사용).
  const entries = useMemo(() => {
    const filtered = applyColumnFilters(allEntries, prefs.filters, filterCtx);
    return sortEntries(filtered, prefs.sort, { staticKeyNames: new Set<string>() });
  }, [allEntries, prefs.filters, prefs.sort, filterCtx]);

  const uniqueValuesByColumn = useMemo(() => {
    const map = new Map<FilterColumnId, string[]>();
    for (const col of PANEL_FILTER_COLUMNS) {
      map.set(col, uniqueColumnValues(allEntries, col, filterCtx));
    }
    return map;
  }, [allEntries, filterCtx]);

  const selectedKeys = readSelectedKeys(storeSource);

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
      const key = entry.key as string;
      const willAdd = !selectedKeys.includes(key);
      if (willAdd && isSelectionAtLimit(selectedKeys)) {
        // 상한 초과 반영을 억제하고 안내만 표시(미리보기 성능 보호).
        setOverLimitNotice(true);
        return;
      }
      setOverLimitNotice(false);
      const next = toggleSelectedKey(selectedKeys, key);
      onConfigChange({ store_source: { ...(storeSource ?? {}), selected_keys: next } });
    },
    [selectedKeys, storeSource, onConfigChange],
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
            String(SELECTED_KEYS_LIMIT),
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
            isSelected: (e) => selectedKeys.includes(e.key as string),
            onToggle: handleToggleSelection,
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
        />
      )}
    </div>
  );
}
