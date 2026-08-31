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
import { Eraser, RefreshCw } from 'lucide-react';

import { useAgent, useAgents } from '@/hooks/useAgent';
import { useTranslation, type TranslationFn } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { ColumnSettingsMenu } from '@/pages/agents/storeColumns';
import {
  STORE_COLUMNS,
  type StoreColumn,
  type StoreColumnId,
} from '@/pages/agents/storeColumnsModel';
import {
  applyColumnFilters,
  sortEntries,
  uniqueColumnValues,
  type ColumnFilter,
  type ColumnFilterMap,
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
  normalizeStoreSeriesAlias,
  storeSeriesLabel,
  STORE_SERIES_LIMIT,
  type StoreSeriesRef,
  type StoreSourceConfig,
} from './panels/charts/chartChannelTypes';
import { makeTagFilterId } from './panels/charts/storeSourceFilter';
import {
  matchesColumnValueFilters,
  splitTagPairsToDimensions,
} from './panels/charts/storeColumnValueFilter';
import type { SensorPosition } from './panels/heatmap/heatmapConfig';
import { migrateSensorPositions } from './panels/heatmap/sensorIdentity';
import { SeriesDetailEditor, StoreSourceSection } from './ChartPanelSections';
import { resolvePanelSourceBinding } from './panels/charts/panelDataSource';
import type { ChartDataSourceKind } from './panels/charts/chartChannelTypes';
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
  // 바인딩 모드의 단일 소스 오브 트루스는 `config.data_source` 다(@spec SPEC-TSDB-002 §2.11).
  // 초기값은 판정 계약에서 파생하고(인식 불가 값은 channel 로 접힌다), 이후 StoreSourceSection
  // 의 콜백으로 동기화한다.
  const initialMode: ChartDataSourceKind = resolvePanelSourceBinding(panel.config ?? {}).kind;
  const [mode, setMode] = useState<ChartDataSourceKind>(initialMode);

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
 * v0.3.0(REQ-15): 태그를 포함한 모든 컬럼 필터(key/name/metric/tag)를 통일된 표시(display)
 * 필터로 취급한다 — 같은 컬럼 OR, 컬럼 간 AND. 태그 컬럼 필터는 표시 행만 좁히며 단독으로
 * `selection_mode` 를 전환하지 않는다(동적 바인딩은 REQ-22 토글 전용).
 */
const PANEL_FILTER_COLUMNS: readonly FilterColumnId[] = ['key', 'value', 'tags'];

/**
 * StoreKeyObject → StoreEntry(메타데이터 파생). 라이브 값(value/updated)은 없다.
 * data_type 은 series 항목 구성(StoreKeySelector 와 byte-호환)에 필요하므로 포함한다.
 */
function toStoreEntry(o: StoreKeyObject): StoreEntry {
  return {
    key: o.key,
    storage_key: o.key,
    field: o.field,
    tags: o.tags,
    data_type: o.data_type,
    registration: o.registration,
  };
}

/** 선택 시리즈(StoreSeriesRef) → 시리즈 동일성 키. 좌표 맵/체크 판정과 같은 키 공간이다. */
function seriesRefId(s: StoreSeriesRef): string {
  return storeSeriesId(s.key, s.field ?? '', s.tags ?? {});
}

/**
 * 선택돼 있으나 스토어 키 목록에 더 이상 존재하지 않는 시리즈를 표 행(StoreEntry)으로 합성한다.
 *
 * 왜 필요한가: 체크박스와 해제 경로는 **행 위에만** 존재하는데, 행은 라이브 스토어 목록에서
 * 파생된다. 스토어에서 키가 사라지면 행이 사라지고, 행이 사라지면 체크를 풀 수단이 함께
 * 사라져 잔존 선택이 config 에 영구히 남는다(히트맵에서는 판독값 없는 유령 마커로 보인다).
 * 선택 상태 자체에서 행을 합성해 그 출구를 되돌려준다.
 *
 * 라이브 값 컬럼은 원래 이 표에 없고(toStoreEntry 동일), registration 도 스토어에 없는
 * 항목이므로 채우지 않는다 — 합성 행은 오로지 "해제할 수 있는 행"으로서만 존재한다.
 */
function staleSeriesToEntry(s: StoreSeriesRef): StoreEntry {
  return {
    key: s.key,
    storage_key: s.key,
    field: s.field,
    tags: s.tags,
    data_type: s.data_type,
  };
}

/**
 * 동적 바인딩(REQ-22) ON 시, 태그-컬럼 표시 필터의 선택 값("k=v" 집합)에서 바인딩 기준
 * `tag_filters`(Record<string,string>)를 파생한다. 기존 스키마(키당 단일값)를 유지하므로
 * 같은 키의 값이 여러 개면 마지막 값이 우선한다(additive only, useStoreChartData 소비 호환).
 */
function deriveTagFilters(values: ReadonlySet<string> | undefined): Record<string, string> {
  const out: Record<string, string> = {};
  if (!values) return out;
  for (const pair of values) {
    const eq = pair.indexOf('=');
    if (eq < 0) continue;
    out[pair.slice(0, eq)] = pair.slice(eq + 1);
  }
  return out;
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

  // 히트맵 패널: keys 모드 체크박스 선택이 센서 위치(sensor_positions)를 함께 구동한다.
  // 선택 시 중앙(0.5,0.5) 기본 좌표를 부여하고, 해제 시 좌표 항목을 제거한다(additive —
  // 다른 패널 타입은 위치 부수효과 없음). @spec SPEC-PANEL-SETTINGS-001 (heatmap 시리즈 위치)
  const isHeatmap = panel.type === 'heatmap';
  const rawSensorPositions = useMemo(
    () => (config.sensor_positions as Record<string, SensorPosition> | undefined) ?? {},
    [config.sensor_positions],
  );

  const { data: agentsResult } = useAgents();
  const agentName = resolveStoreAgentName(
    storeSource?.agent_id,
    storeSource?.agent_name ?? '',
    agentsResult?.data,
  );
  const hasAgent = agentName !== '';

  // 현재 값 컬럼의 소스. 스토어 키 목록(useStoreKeysWithTags)은 메타데이터만 주므로
  // 라이브 값은 에이전트 상태 스냅샷(state.entries)에서 가져온다 — 에이전트 상세
  // 스토어 탭과 같은 출처다. 별도 폴링 루프를 두지 않고 시리즈 목록과 동일한 React
  // Query 갱신 트리거(마운트/포커스/무효화)를 공유하므로, 시리즈가 갱신될 때 값도 함께
  // 갱신된다.
  const agentId = useMemo(
    () => (agentsResult?.data ?? []).find((a) => a.name === agentName)?.id ?? '',
    [agentsResult?.data, agentName],
  );
  const { data: agentFull } = useAgent(agentId, 'full');

  const {
    data: keysData,
    refetch: refetchKeys,
    isFetching: keysFetching,
    isSuccess: keysLoaded,
  } = useStoreKeysWithTags(agentName);
  const keyObjects = useMemo(() => keysData?.keyObjects ?? [], [keysData?.keyObjects]);

  // 에이전트 상태의 라이브 엔트리를 시리즈 동일성 키로 색인한다(현재 값 조인용).
  const liveValueById = useMemo(() => {
    const state = agentFull?.state as { entries?: StoreEntry[] } | undefined;
    const map = new Map<string, unknown>();
    for (const e of state?.entries ?? []) {
      if (typeof e.key !== 'string') continue;
      map.set(
        storeSeriesId(
          e.key,
          (e.field as string) ?? '',
          (e.tags as Record<string, string> | undefined) ?? {},
        ),
        e.value,
      );
    }
    return map;
  }, [agentFull?.state]);

  // keyObjects(메타데이터) → StoreEntry 파생 + 라이브 값 병합.
  // 값이 아직 없는 시리즈는 value 미설정으로 남는다(표는 빈 셀로 안전하게 렌더한다).
  const allEntries: StoreEntry[] = useMemo(
    () =>
      keyObjects.map((o) => {
        const entry = toStoreEntry(o);
        const value = liveValueById.get(
          storeSeriesId(o.key, o.field ?? '', o.tags ?? {}),
        );
        return value === undefined ? entry : { ...entry, value };
      }),
    [keyObjects, liveValueById],
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

  // 관련(표시가능) 컬럼: key, value, (tags 있을 때) tags.
  // metric 컬럼은 제외한다 — 시리즈를 구분하는 표기는 이름(alias) 셀이 key+metric+tags 를
  // 합쳐 이미 보여주므로 전용 컬럼은 같은 정보를 두 번 차지한다.
  // v0.3.0(REQ-15): tags 컬럼은 통일된 표시 필터의 일부로 표준 다중값 필터(ColumnFilterButton
  // grouped)를 그대로 사용한다(전용 AND 팝오버 제거). 태그 값(k=v) 다중선택은 OR, 컬럼 간 AND.
  const relevantCols = useMemo(() => {
    const base = [pickColumn('key'), pickColumn('value')];
    if (showTags) base.push(pickColumn('tags'));
    return base;
  }, [showTags]);

  // 렌더 컬럼 순서: 키(key) · 이름(name/alias) · 현재 값(value) · 태그(tags). (REQ-17/AC-19)
  // alias(actions 대체) 컬럼을 key 바로 뒤로 배치한다. key 가 숨겨진 경우에도 나머지 순서
  // (name · value · tag)는 유지된다.
  const columns: StoreColumn[] = useMemo(() => {
    const visible = relevantCols.filter((c) => !hidden.has(c.id));
    const alias: StoreColumn = { id: 'alias', labelKey: 'colAlias', hideable: false };
    const keyCols = visible.filter((c) => c.id === 'key');
    const rest = visible.filter((c) => c.id !== 'key');
    return [...keyCols, alias, ...rest];
  }, [relevantCols, hidden]);

  // --- 동적 바인딩(REQ-22) + 통일된 표시 필터(REQ-15) ---
  // v0.3.0: 표시(display)와 바인딩(binding)을 분리한다. selection_mode 는 오직 명시적
  // "동적 바인딩" 토글로만 제어되며, 태그 컬럼 필터는 표시 행만 좁힌다.
  //   - OFF(신규 기본, selection_mode !== 'tag'): keys/명시 선택. 컬럼 필터는 표시 전용.
  //   - ON(selection_mode === 'tag'): poll 시 tag_filters 로 매칭 키 동적 해석. 태그 기준은
  //     태그-컬럼 표시 필터 값에서 파생한다.
  const dynamicBinding = storeSource?.selection_mode === 'tag';
  const tagFilters = useMemo(
    () => storeSource?.tag_filters ?? {},
    [storeSource?.tag_filters],
  );

  // 표시 필터 상태(prefs.filters). 하위호환: 동적 바인딩 ON 인 기존 패널이 로드될 때
  // 태그 표시 필터가 아직 없으면 저장된 tag_filters(바인딩 기준)를 태그 표시 필터로
  // 시드하여, 선택 테이블이 바인딩 대상 행만 보여주고 헤더 피커에 현재 기준이 반영되도록 한다.
  const effectiveFilters = useMemo(() => {
    const persistedTags = prefs.filters['tags'];
    const hasPersistedTags =
      persistedTags !== undefined &&
      (persistedTags.text.trim() !== '' || persistedTags.values.size > 0);
    if (dynamicBinding && !hasPersistedTags && Object.keys(tagFilters).length > 0) {
      const values = new Set(
        Object.entries(tagFilters).map(([k, v]) => makeTagFilterId(k, v)),
      );
      return { ...prefs.filters, tags: { text: '', values } };
    }
    return prefs.filters;
  }, [prefs.filters, dynamicBinding, tagFilters]);

  // 필터 → 정렬 파이프라인. 통일된 표시 필터(같은 차원 OR + 차원 간 AND)로 표시 행만 좁힌다.
  // v0.5.0(REQ-15/AC-17b): 태그는 **태그 키(종류)별 차원**으로 결합한다 — "태그" 를 하나의
  // 컬럼으로 뭉쳐 모든 k=v 를 OR 하지 않는다. 일반 컬럼(key/metric)은 기존 파이프라인(텍스트+값,
  // 같은 컬럼 OR / 컬럼 간 AND)을 유지하고, 태그 컬럼 필터는 태그 키별 차원 매처로 별도 결합한다.
  // 바인딩 모드와 무관하게 동일 파이프라인을 쓴다(표시/바인딩 분리).
  const entries = useMemo(() => {
    // 1) 비-tags 컬럼 필터(key/metric): 기존 파이프라인(텍스트 부분일치 + 값 OR, 컬럼 간 AND).
    const nonTagFilters: ColumnFilterMap = {};
    for (const key of Object.keys(effectiveFilters) as FilterColumnId[]) {
      if (key !== 'tags') nonTagFilters[key] = effectiveFilters[key];
    }
    let base = applyColumnFilters(allEntries, nonTagFilters, filterCtx);
    // 2) 태그 필터: 태그 키별 차원 AND/OR + (있으면) "k=v" 텍스트 부분일치.
    const tagsFilter = effectiveFilters['tags'];
    if (tagsFilter) {
      const text = tagsFilter.text.trim().toLowerCase();
      const dims = splitTagPairsToDimensions(tagsFilter.values);
      const hasDims = Object.keys(dims).length > 0;
      if (text !== '' || hasDims) {
        base = base.filter((e) => {
          const tags = (e.tags as Record<string, string> | undefined) ?? {};
          if (text !== '') {
            const joined = Object.entries(tags)
              .map(([k, v]) => `${k}=${v}`)
              .join(' ')
              .toLowerCase();
            if (!joined.includes(text)) return false;
          }
          return matchesColumnValueFilters((dim) => tags[dim], dims);
        });
      }
    }
    return sortEntries(base, prefs.sort, { staticKeyNames: new Set<string>() });
  }, [allEntries, effectiveFilters, prefs.sort, filterCtx]);

  const uniqueValuesByColumn = useMemo(() => {
    const map = new Map<FilterColumnId, string[]>();
    for (const col of PANEL_FILTER_COLUMNS) {
      map.set(col, uniqueColumnValues(allEntries, col, filterCtx));
    }
    return map;
  }, [allEntries, filterCtx]);

  // 선택의 단일 소스 오브 트루스는 store_source.series(keys 모드). 체크박스는 이 series 를
  // StoreKeySelector 와 byte-호환 형태로 추가/제거한다(렌더 경로 불변). @spec SPEC-PANEL-SETTINGS-001
  const series = useMemo<StoreSeriesRef[]>(
    () => normalizeStoreSeriesAlias(storeSource?.series ?? []),
    [storeSource?.series],
  );
  // 동일성 키 → 선택된 시리즈. 체크 판정(seriesIds)과 이름 셀(사용자 alias 우선)이 같은 색인을
  // 공유한다. 같은 동일성 키가 둘 이상 있으면 앞 항목이 선택 판정의 기준이므로 앞 항목을 남긴다.
  const seriesById = useMemo(() => {
    const map = new Map<string, StoreSeriesRef>();
    for (const s of series) {
      const id = storeSeriesId(s.key, s.field ?? '', s.tags ?? {});
      if (!map.has(id)) map.set(id, s);
    }
    return map;
  }, [series]);
  const seriesIds = useMemo(() => new Set(seriesById.keys()), [seriesById]);

  // 센서 좌표는 store key 가 아니라 **시리즈 동일성 키**로 키잉된다. 한 key 가 metric/tags 별
  // 다중 시리즈로 나뉘므로 key 키잉은 형제끼리 좌표를 공유하게 만들고, 하나를 해제하면 아직
  // 체크된 형제의 좌표까지 지워져 히트맵이 비어버렸다. 기존 패널(raw key 키잉)은 읽는 시점에
  // 이관하고, 이후 편집(선택/좌표)이 이관된 맵을 그대로 저장해 자연스럽게 영속된다.
  const sensorPositions = useMemo(
    () => (isHeatmap ? migrateSensorPositions(rawSensorPositions, series).positions : rawSensorPositions),
    [isHeatmap, rawSensorPositions, series],
  );
  const entryToSeriesId = (entry: StoreEntry): string =>
    storeSeriesId(
      entry.key as string,
      (entry.field as string) ?? '',
      (entry.tags as Record<string, string>) ?? {},
    );

  // --- 유령 선택(스토어에서 사라진 선택 시리즈) 회수 ---
  // 판정은 키 목록 조회가 **성공한 뒤에만** 한다. 로딩/실패 중에는 allEntries 가 비어 모든
  // 선택이 stale 로 보이므로, 그 상태에서 배지를 띄우거나 일괄 정리를 열어주면 멀쩡한 시리즈를
  // 지우게 된다(fetching 중에도 react-query 는 직전 data 를 유지하므로 isSuccess 로 충분하다).
  const liveSeriesIds = useMemo(
    () => new Set(allEntries.map((e) => entryToSeriesId(e))),
    [allEntries],
  );
  const staleSeries = useMemo(
    () =>
      hasAgent && keysLoaded ? series.filter((s) => !liveSeriesIds.has(seriesRefId(s))) : [],
    [hasAgent, keysLoaded, series, liveSeriesIds],
  );
  const staleIds = useMemo(() => new Set(staleSeries.map(seriesRefId)), [staleSeries]);
  // 합성 행은 컬럼 필터/정렬 파이프라인을 통과시키지 않고 목록 맨 위에 고정한다 — 필터에 걸려
  // 다시 사라지면 "해제할 행이 없다"는 원래 문제로 되돌아간다.
  const rows = useMemo(
    () => (staleSeries.length > 0 ? [...staleSeries.map(staleSeriesToEntry), ...entries] : entries),
    [staleSeries, entries],
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
      // 동적 바인딩 ON + 태그 컬럼 필터 변경 → 바인딩 기준 tag_filters 를 태그 표시 필터에서
      // 재파생한다(표시 필터가 바인딩 기준의 원천). 표시/바인딩 분리이므로 OFF 이면 미갱신. REQ-22
      if (columnId === 'tags' && storeSource?.selection_mode === 'tag') {
        onConfigChange({
          store_source: { ...(storeSource ?? {}), tag_filters: deriveTagFilters(next.values) },
        });
      }
    },
    [persist, prefs, storeSource, onConfigChange],
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

  // 선택(체크)된 행의 인라인 펼침 상세(REQ-18/19/20/21) — 기본 접음(빈 집합), 펼친 행만 담는다.
  // seriesId 기준으로 관리하며, 그룹 트리가 아니라 행별 상태이고 정렬/필터 상태와 독립이다.
  // 미선택 행은 펼침 대상에서 제외된다(isExpandable = 선택 여부).
  const [expandedSeriesRows, setExpandedSeriesRows] = useState<Set<string>>(() => new Set());
  const toggleSeriesRow = useCallback((entry: StoreEntry) => {
    const id = entryToSeriesId(entry);
    setExpandedSeriesRows((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);
  // 선택 상한 초과 안내(AC-15). 상한 도달 상태에서 추가 시도 시 표시한다.
  const [overLimitNotice, setOverLimitNotice] = useState(false);

  const isLineChart = panel.type === 'graph-chart';

  // 인라인 상세 편집 → 해당 시리즈(store_source.series[idx])에 patch 를 draft 반영한다.
  // 이름(alias)/색상/선 스타일 → StoreSeriesRef. keys 모드는 명시 series[] 에 직접 영속되고,
  // tag 모드(동적 바인딩 ON)에서는 현재 선택된 series[] 항목 기준으로 best-effort 반영된다.
  const patchSeries = useCallback(
    (idx: number, patch: Partial<StoreSeriesRef>) => {
      const next = series.map((s, i) => (i === idx ? { ...s, ...patch } : s));
      onConfigChange({ store_source: { ...(storeSource ?? {}), series: next } });
    },
    [series, storeSource, onConfigChange],
  );

  // 히트맵 전용(REQ-21): 선택 행 인라인 상세 안에서 센서 좌표(x/y, 0..1)를 편집한다.
  // 한 축만 입력해도 잃지 않도록 부분 병합하고, 둘 다 비면 좌표 항목을 제거한다.
  // 키는 시리즈 동일성 키다 — 같은 store key 를 공유하는 형제 시리즈가 서로 독립된 좌표를 갖는다.
  const setSensorPosition = useCallback(
    (sensorId: string, axis: 'x' | 'y', value: number | undefined) => {
      const cur = {
        ...((sensorPositions[sensorId] as { x?: number; y?: number } | undefined) ?? {}),
      };
      if (value === undefined) delete cur[axis];
      else cur[axis] = value;
      const nextPositions: Record<string, { x?: number; y?: number }> = { ...sensorPositions };
      if (cur.x === undefined && cur.y === undefined) delete nextPositions[sensorId];
      else nextPositions[sensorId] = cur;
      onConfigChange({ sensor_positions: nextPositions });
    },
    [sensorPositions, onConfigChange],
  );

  const handleToggleSelection = useCallback(
    (entry: StoreEntry) => {
      const id = entryToSeriesId(entry);
      const key = entry.key as string;
      if (seriesIds.has(id)) {
        // 제거: 동일 seriesId 항목을 series 에서 뺀다.
        setOverLimitNotice(false);
        const next = series.filter(
          (s) => storeSeriesId(s.key, s.field ?? '', s.tags ?? {}) !== id,
        );
        const nextStore: Record<string, unknown> = { ...(storeSource ?? {}), series: next };
        const patch: Record<string, unknown> = { store_source: nextStore };
        // 히트맵: 선택 해제 시 **그 시리즈의** 좌표 항목만 제거한다(동일성 키 기준). key 기준이면
        // 같은 key 를 공유하는 아직 체크된 형제의 좌표까지 지워져 히트맵이 비어버린다(이 결함의
        // 직접 증상). selection_mode/tag_filters 는 건드리지 않는다 — 바인딩 모드는 동적 바인딩
        // 토글(REQ-22)이 단독 제어한다(표시/바인딩 분리).
        if (isHeatmap && sensorPositions[id] !== undefined) {
          const nextPositions = { ...sensorPositions };
          delete nextPositions[id];
          patch.sensor_positions = nextPositions;
        }
        onConfigChange(patch);
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
        key,
        field: (entry.field as string) || undefined,
        tags:
          entry.tags && Object.keys(entry.tags as object).length > 0
            ? (entry.tags as Record<string, string>)
            : undefined,
        data_type: entry.data_type as StoreSeriesRef['data_type'],
        // alias 는 비워 둔다. 기본값으로 key 를 넣으면 사용자가 직접 붙인 이름과
        // 구분할 수 없어, measurement 와 같은 이름을 입력했을 때 무시된다.
        color: pickSeriesColor(series.length),
      };
      const nextStore: Record<string, unknown> = {
        ...(storeSource ?? {}),
        series: [...series, nextEntry],
      };
      const patch: Record<string, unknown> = { store_source: nextStore };
      // 히트맵: 좌표가 없으면 중앙(0.5,0.5) 기본 좌표를 부여한다(동일성 키 기준 — 같은 key 의
      // 형제 시리즈끼리 좌표를 공유하지 않는다). selection_mode/tag_filters 는 건드리지 않는다
      // (바인딩 모드는 동적 바인딩 토글이 단독 제어 — 표시/바인딩 분리). REQ-22
      if (isHeatmap && sensorPositions[id] === undefined) {
        patch.sensor_positions = { ...sensorPositions, [id]: { x: 0.5, y: 0.5 } };
      }
      onConfigChange(patch);
    },
    [series, seriesIds, storeSource, onConfigChange, isHeatmap, sensorPositions],
  );

  // 유령 선택 일괄 정리: 스토어에 없는 선택 시리즈를 series[] 에서 모두 빼고, 히트맵이면 그
  // 좌표 항목까지 함께 지운다(개별 해제 경로 handleToggleSelection 과 동일한 부수효과 규약).
  // staleSeries 가 비면 버튼 자체를 렌더하지 않으므로 여기서는 방어만 한다.
  const handleCleanupStale = useCallback(() => {
    if (staleIds.size === 0) return;
    setOverLimitNotice(false);
    const next = series.filter((s) => !staleIds.has(seriesRefId(s)));
    const patch: Record<string, unknown> = {
      store_source: { ...(storeSource ?? {}), series: next },
    };
    if (isHeatmap) {
      const nextPositions = { ...sensorPositions };
      let removed = false;
      for (const id of staleIds) {
        if (nextPositions[id] !== undefined) {
          delete nextPositions[id];
          removed = true;
        }
      }
      if (removed) patch.sensor_positions = nextPositions;
    }
    onConfigChange(patch);
  }, [staleIds, series, storeSource, isHeatmap, sensorPositions, onConfigChange]);

  // 동적 바인딩 토글(REQ-22). ON: selection_mode:'tag' + 현재 태그 표시 필터에서 tag_filters
  // 파생. OFF: selection_mode:'keys'(명시 체크박스 선택). tag_filters/series 는 보존한다
  // (additive only — OFF 에서 tag_filters 는 useStoreChartData 가 무시). 표시/바인딩 분리.
  const handleDynamicBindingToggle = useCallback(
    (on: boolean) => {
      if (on) {
        const tagsFilter = prefs.filters['tags'];
        onConfigChange({
          store_source: {
            ...(storeSource ?? {}),
            selection_mode: 'tag',
            tag_filters: deriveTagFilters(tagsFilter?.values),
          },
        });
      } else {
        onConfigChange({
          store_source: { ...(storeSource ?? {}), selection_mode: 'keys' },
        });
      }
    },
    [prefs.filters, storeSource, onConfigChange],
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
        <div className="flex items-center gap-2">
          {/* 동적 바인딩 토글(REQ-22) — 표시 필터와 분리된 명시적 바인딩 모드 제어.
              OFF(기본)=keys/명시 선택, ON=tag/poll 동적 해석(태그 표시 필터에서 기준 파생). */}
          <label
            className="flex cursor-pointer items-center gap-1 text-[11px] text-(--color-text-muted)"
            title={t('dashboard.settings.dynamicBindingHint')}
          >
            <input
              type="checkbox"
              data-testid="chart-dynamic-binding-toggle"
              checked={dynamicBinding}
              onChange={(e) => handleDynamicBindingToggle(e.target.checked)}
              className="h-3.5 w-3.5"
            />
            {t('dashboard.settings.dynamicBindingLabel')}
          </label>
          <div className="flex items-center gap-1">
          {/* 유령 선택 일괄 정리 — 스토어에서 사라진 선택 시리즈가 있을 때만 노출한다. */}
          {staleSeries.length > 0 && (
            <button
              type="button"
              onClick={handleCleanupStale}
              data-testid="panel-store-select-cleanup-stale"
              aria-label={t('dashboard.settings.dataSourceStoreSelectCleanupStale')}
              title={t('dashboard.settings.dataSourceStoreSelectCleanupStale').replace(
                '{count}',
                String(staleSeries.length),
              )}
              className="inline-flex items-center gap-1 rounded px-1 py-0.5 text-[11px] text-amber-700 transition-colors hover:bg-(--color-bg-hover) dark:text-amber-300"
            >
              <Eraser className="h-3.5 w-3.5" />
              {String(staleSeries.length)}
            </button>
          )}
          {/* store 키 목록 새로고침 — 새로 추가된 키가 나타나도록 react-query 재조회. */}
          <button
            type="button"
            onClick={() => void refetchKeys()}
            disabled={!hasAgent || keysFetching}
            data-testid="panel-store-select-refresh"
            aria-label={t('dashboard.settings.dataSourceStoreSelectRefresh')}
            title={t('dashboard.settings.dataSourceStoreSelectRefresh')}
            className="inline-flex items-center rounded p-0.5 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-hover) hover:text-(--color-text-default) disabled:opacity-40"
          >
            <RefreshCw className={cn('h-3.5 w-3.5', keysFetching && 'animate-spin')} />
          </button>
          <ColumnSettingsMenu
            columns={relevantCols}
            visible={visibleColumnSet}
            onToggle={handleToggleColumn}
            t={t}
          />
          </div>
        </div>
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
      ) : rows.length === 0 ? (
        <p className="rounded-md border border-(--color-border-default) bg-(--color-bg-primary) p-4 text-center text-xs text-(--color-text-muted)">
          {t('dashboard.settings.dataSourceStoreSelectEmpty')}
        </p>
      ) : (
        <StoreEntryTable
          entries={rows}
          columns={columns}

          sort={prefs.sort}
          onSort={handleSort}
          columnFilters={effectiveFilters}
          onColumnFilterChange={handleColumnFilterChange}
          uniqueValuesByColumn={uniqueValuesByColumn}
          rowExpansion={{
            // 선택(체크)된 행만 펼침 가능. 바인딩 모드와 무관하게 접근 가능(REQ-22 독립성).
            isExpandable: (e) => seriesIds.has(entryToSeriesId(e as StoreEntry)),
            isExpanded: (e) => expandedSeriesRows.has(entryToSeriesId(e as StoreEntry)),
            onToggle: (e) => toggleSeriesRow(e as StoreEntry),
            renderDetail: (e) => {
              const id = entryToSeriesId(e as StoreEntry);
              const idx = series.findIndex(
                (s) => storeSeriesId(s.key, s.field ?? '', s.tags ?? {}) === id,
              );
              const s = series[idx];
              if (!s) return null;
              return (
                <SeriesDetailEditor
                  series={s}
                  index={idx}
                  isLineChart={isLineChart}
                  onPatch={(patch) => patchSeries(idx, patch)}
                  positionEditor={
                    isHeatmap ? (
                      <SensorPositionInputs
                        sensorId={id}
                        pos={sensorPositions[id] as { x?: number; y?: number } | undefined}
                        onChange={setSensorPosition}
                        t={t}
                      />
                    ) : undefined
                  }
                />
              );
            },
          }}
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
            // 체크 상태는 명시적 선택(series)만 반영한다 — 검색/필터된 행은 기본 미체크.
            // 바인딩 모드와 무관하게 체크박스는 명시적 keys 선택(series)을 편집한다. 동적 바인딩
            // ON 이면 이 series 는 useStoreChartData 에서 무시되나 편집 상태로 보존된다.
            isSelected: (e) => seriesIds.has(entryToSeriesId(e)),
            onToggle: handleToggleSelection,
          }}
          renderCellExtra={(col, entry) => {
            if (col !== 'alias') return undefined;
            // 이름 셀은 시리즈를 **실제로 구분하는** 표기를 보인다. key 만 찍으면 한 key 를
            // metric/tags 로 나눠 갖는 형제 행들이 같은 글자로 보여 어느 행이 어느 센서인지
            // 알 수 없다(보고된 결함). 선택된 행은 사용자가 붙인 이름이 있으면 그 이름을
            // 쓰고(마커와 동일 규칙), 미선택 행은 ref 가 없으므로 엔트리 메타데이터로 만든다.
            const e = entry as StoreEntry;
            const label = storeSeriesLabel(
              seriesById.get(entryToSeriesId(e)) ?? {
                key: e.key as string,
                field: (e.field as string) || undefined,
                tags: e.tags as Record<string, string> | undefined,
              },
              storeSource?.series_name_format,
            );
            // 행이 좁으므로 잘라 쓰되 전체 값은 title 로 남긴다(레이아웃 파괴 방지).
            // 합성된 유령 행은 배지로 구분한다 — 배지가 없으면 스토어에 살아있는 행과 구별되지
            // 않아, 사용자가 "왜 값이 안 오지" 를 계속 데이터 문제로 오해하게 된다.
            const isStale = staleIds.has(entryToSeriesId(e));
            return (
              <span className="flex max-w-[300px] items-center gap-1">
                <span
                  className="block max-w-[220px] truncate font-mono text-xs text-(--color-text-secondary)"
                  title={label}
                >
                  {label}
                </span>
                {isStale && (
                  <span
                    data-testid="panel-store-select-stale-badge"
                    className="shrink-0 rounded bg-amber-100 px-1 py-px text-[10px] leading-tight text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
                  >
                    {t('dashboard.settings.dataSourceStoreSelectStale')}
                  </span>
                )}
              </span>
            );
          }}
        />
      )}
    </div>
  );
}

/**
 * heatmap 전용(REQ-21): 선택 행 인라인 상세 안에서 편집하는 센서 좌표(x/y, 0..1) 입력.
 * 프리뷰 마커 드래그와 동일한 sensor_positions 를 편집한다(별도 heatmap 영역에서 이동).
 * 키는 시리즈 동일성 키(`storeSeriesId`)다 — store key 를 공유하는 형제 시리즈가 각자의
 * 좌표 입력을 갖도록(예전에는 같은 값을 가리켜 함께 움직였다).
 * @spec SPEC-PANEL-SETTINGS-001 (REQ-21)
 */
function SensorPositionInputs({
  sensorId,
  pos,
  onChange,
  t,
}: {
  sensorId: string;
  pos: { x?: number; y?: number } | undefined;
  onChange: (sensorId: string, axis: 'x' | 'y', value: number | undefined) => void;
  t: TranslationFn;
}): React.ReactElement {
  const inputClass =
    'w-16 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-2 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-500';
  return (
    <div className="flex items-center gap-2" data-testid={`series-position-${sensorId}`}>
      <label className="text-sm font-medium text-(--color-text-muted)">
        {t('dashboard.settings.seriesDetailsPositionX')}
      </label>
      <input
        type="number"
        min={0}
        max={1}
        step={0.05}
        value={pos?.x !== undefined ? String(pos.x) : ''}
        data-testid={`heatmap-pos-x-${sensorId}`}
        placeholder="x"
        onChange={(e) =>
          onChange(sensorId, 'x', e.target.value === '' ? undefined : Number(e.target.value))
        }
        className={inputClass}
      />
      <label className="text-sm font-medium text-(--color-text-muted)">
        {t('dashboard.settings.seriesDetailsPositionY')}
      </label>
      <input
        type="number"
        min={0}
        max={1}
        step={0.05}
        value={pos?.y !== undefined ? String(pos.y) : ''}
        data-testid={`heatmap-pos-y-${sensorId}`}
        placeholder="y"
        onChange={(e) =>
          onChange(sensorId, 'y', e.target.value === '' ? undefined : Number(e.target.value))
        }
        className={inputClass}
      />
    </div>
  );
}
