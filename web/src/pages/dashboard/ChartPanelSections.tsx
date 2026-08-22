// 차트 패널 타입별 설정 섹션.
// PanelSettingsDialog 에서 사용하는 sub-컴포넌트 모음 (SPEC-CHART-001 §4.2.2 / REQ-M5-03).
//
// 5종 차트 (stat / line-chart / bar-chart / pie-chart / table) 각각의 config
// 편집 UI 를 제공한다. 상위 PanelSettingsDialog 는 panel.type 에 따라
// 분기하여 해당 Section 을 렌더링한다.

import React, { useEffect, useMemo, useRef, useState } from 'react';
import { ChevronDown, ChevronRight, GripVertical, Info, Plus, Trash2 } from 'lucide-react';

import type { PanelConfig } from '@/stores/uiStore';
import { listChartChannels, type ChartChannelSummary } from '@/services/api/charts';
import { useAgents } from '@/hooks/useAgent';
import { useStoreKeysWithTags, useStoreTagPairs } from '@/services/api/store';
import { useTranslation } from '@/lib/i18n';

import type {
  TableColumn,
  TableColumnFormat,
  BarChartMode,
  AggFunc,
  SortOrder,
  YAxisMode,
  YAxisDataType,
  YEnumLabel,
  AxisFontStyle,
  TimeWindowMode,
  YThreshold,
  ChannelRefConfig,
  StrokeStyle,
  ChartDataSourceKind,
  StoreSeriesRef,
  StoreSourceConfig,
  SeriesReduceFunc,
} from './panels/charts/chartChannelTypes';
import {
  DEFAULT_STORE_SOURCE_WINDOW,
  defaultTsdbSource,
  pickSeriesColor,
  REDUCE_PANEL_TYPES,
} from './panels/charts/chartChannelTypes';
import { SERIES_REDUCE_FUNCS } from './panels/charts/seriesReduce';
import { resolvePanelSourceBinding } from './panels/charts/panelDataSource';
import {
  filterStoreKeyObjects,
  makeTagFilterId,
} from './panels/charts/storeSourceFilter';
import {
  availableAliasTokens,
  makeAliasToken,
  resolveSeriesAlias,
} from './panels/charts/aliasTemplate';

/** REQ-M5-04: channel_name 정규식 */
const CHANNEL_NAME_REGEX = /^[a-zA-Z][a-zA-Z0-9_-]{0,63}$/;

/** Custom (수동 입력) 드롭다운 옵션 sentinel */
const CUSTOM_CHANNEL_SENTINEL = '__custom__';

/** 인라인 에러 메시지 i18n 키 (렌더 시 t() 로 변환) */
const CHANNEL_NAME_ERROR_KEY = 'dashboard.chart.channelNameError';

type OnConfig = (config: Record<string, unknown>) => void;

// --- 공용 입력 헬퍼 ---

function LabeledField(props: { label: string; children: React.ReactNode; hint?: string }): React.ReactElement {
  return (
    <div>
      <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
        {props.label}
      </label>
      {props.children}
      {props.hint && (
        <p className="mt-1 text-[10px] leading-snug text-(--color-text-muted)">{props.hint}</p>
      )}
    </div>
  );
}

function inputClass(): string {
  return 'w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500';
}

// --- 1. 공통: channel_name 편집 (등록된 채널 드롭다운 + Custom 수동 입력) ---

/**
 * 모든 차트 패널 공통 channel_name 선택.
 * REQ-M5-02: 활성 chart-emitter 채널을 드롭다운으로 제시, 수동 입력도 허용.
 * REQ-M5-04: 정규식 검증 + 인라인 에러.
 *
 * 기존 panel.config.channel_name 이 활성 목록에 없더라도 (플로우 undeploy 등)
 * 해당 값은 드롭다운에 "(현재 선택, 비활성)" 로 표시되어 선택 상태를 유지한다.
 *
 * 주입 가능한 `fetchChannels` 파라미터는 테스트 용도이며, 프로덕션에서는
 * 기본값으로 `listChartChannels` (GET /api/v1/charts/channels) 를 호출한다.
 */
export function ChartChannelSection({
  panel,
  onConfigChange,
  fetchChannels = listChartChannels,
}: {
  panel: PanelConfig;
  onConfigChange: OnConfig;
  fetchChannels?: () => Promise<ChartChannelSummary[]>;
}): React.ReactElement {
  const { t } = useTranslation();
  const currentName = (panel.config?.channel_name as string | undefined) ?? '';

  // 드롭다운 선택 상태. 초기값은 현재 저장된 채널 이름 (없으면 '')
  const [selectedOption, setSelectedOption] = useState<string>(currentName);
  const [customDraft, setCustomDraft] = useState<string>('');
  const [channels, setChannels] = useState<ChartChannelSummary[]>([]);
  const [loadState, setLoadState] = useState<'idle' | 'loading' | 'error'>('loading');
  const [loadError, setLoadError] = useState<string | null>(null);

  // 활성 채널 목록 조회 (마운트 시 1회 + 패널 변경 시)
  useEffect(() => {
    let cancelled = false;
    setLoadState('loading');
    setLoadError(null);
    fetchChannels()
      .then((result) => {
        if (cancelled) return;
        setChannels(result);
        setLoadState('idle');
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setLoadError(err instanceof Error ? err.message : String(err));
        setLoadState('error');
      });
    return () => {
      cancelled = true;
    };
  }, [fetchChannels, panel.id]);

  // 외부 currentName 이 바뀌면 드롭다운 상태도 동기화
  useEffect(() => {
    setSelectedOption(currentName);
    setCustomDraft('');
  }, [currentName, panel.id]);

  // 실제 commit 대상 채널 이름
  const effectiveName =
    selectedOption === CUSTOM_CHANNEL_SENTINEL ? customDraft : selectedOption;
  const trimmed = effectiveName.trim();
  const isEmpty = trimmed.length === 0;
  const isValid = !isEmpty && CHANNEL_NAME_REGEX.test(trimmed);
  const showError = !isEmpty && !isValid;

  // 현재 저장된 이름이 활성 목록에 있는지
  const activeNames = new Set(channels.map((c) => c.name));
  const currentIsInactive = currentName !== '' && !activeNames.has(currentName);

  // 드롭다운 변경 → 즉시 commit (활성 채널 선택 시) 또는 Custom 모드 전환
  const handleSelect = (value: string): void => {
    setSelectedOption(value);
    if (value === CUSTOM_CHANNEL_SENTINEL) {
      // Custom 모드: 현재 커스텀 값 초기화하고 사용자 입력 대기
      setCustomDraft(currentIsInactive ? currentName : '');
      return;
    }
    if (value === '') {
      if (currentName !== '') {
        onConfigChange({ channel_name: '' });
      }
      return;
    }
    // 활성 채널 선택 시 즉시 저장
    if (value !== currentName) {
      onConfigChange({ channel_name: value });
    }
  };

  // Custom 입력 commit (blur / Enter)
  const commitCustom = (): void => {
    if (isValid && trimmed !== currentName) {
      onConfigChange({ channel_name: trimmed });
    } else if (isEmpty && currentName !== '') {
      onConfigChange({ channel_name: '' });
    }
  };

  return (
    <LabeledField
      label={t('dashboard.chart.channelNameLabel')}
      hint={t('dashboard.chart.channelNameHint')}
    >
      <select
        data-testid="chart-channel-name-select"
        value={selectedOption}
        onChange={(e) => handleSelect(e.target.value)}
        disabled={loadState === 'loading'}
        className={`${inputClass()} disabled:opacity-60`}
      >
        <option value="">
          {loadState === 'loading'
            ? t('dashboard.chart.loadingChannels')
            : channels.length === 0
              ? t('dashboard.chart.noChannelsCustom')
              : t('dashboard.chart.selectChannel')}
        </option>
        {currentIsInactive && (
          <option value={currentName}>
            {t('dashboard.chart.channelInactive').replace('{name}', currentName)}
          </option>
        )}
        {channels.map((ch) => (
          <option key={ch.name} value={ch.name}>
            {t('dashboard.chart.channelOption')
              .replace('{name}', ch.name)
              .replace('{flow}', ch.flow_id || '?')
              .replace('{count}', String(ch.subscriber_count))}
          </option>
        ))}
        <option value={CUSTOM_CHANNEL_SENTINEL}>{t('dashboard.chart.customOption')}</option>
      </select>

      {loadState === 'error' && (
        <p className="mt-1 text-xs text-amber-600 dark:text-amber-400">
          {t('dashboard.chart.loadErrorCustom')}
          {loadError ? ` (${loadError})` : ''}
        </p>
      )}

      {selectedOption === CUSTOM_CHANNEL_SENTINEL && (
        <input
          type="text"
          data-testid="chart-channel-name-input"
          value={customDraft}
          onChange={(e) => setCustomDraft(e.target.value)}
          onBlur={commitCustom}
          onKeyDown={(e) => {
            if (e.key === 'Enter') (e.target as HTMLInputElement).blur();
          }}
          placeholder={t('dashboard.chart.customPlaceholder')}
          autoFocus
          className={`${inputClass()} mt-2`}
        />
      )}

      {showError && (
        <p
          data-testid="chart-channel-name-error"
          className="mt-1 text-xs text-red-500"
        >
          {t(CHANNEL_NAME_ERROR_KEY)}
        </p>
      )}
    </LabeledField>
  );
}

// --- 1.5 데이터 소스 토글 + Store 소스 선택 (SPEC-WEB-005) ---

/** 집계 옵션(UI 표기). tsdb 모달과 동일한 라벨 키를 재사용한다. */
const STORE_AGG_OPTIONS: { value: StoreSourceConfig['aggregation']; labelKey: string }[] = [
  { value: 'min', labelKey: 'tsdb.aggMin' },
  { value: 'max', labelKey: 'tsdb.aggMax' },
  { value: 'average', labelKey: 'tsdb.aggAverage' },
  { value: 'first', labelKey: 'tsdb.aggFirst' },
  { value: 'last', labelKey: 'tsdb.aggLast' },
];

/**
 * 기본 Store 소스 설정(처음 store 모드로 전환 시 사용).
 *
 * 조회 창 기본값(시간창/버킷/집계/폴링)은 `DEFAULT_STORE_SOURCE_WINDOW` 가 단일
 * 정본이다 — 게이지 레거시 이관(`buildGaugeStoreMigrationPatch`)이 spec §2.8 [E2] 1항에
 * 따라 **같은 값**을 써야 하는데, 두 곳에 복제해 두면 한쪽만 바뀔 때 이관 결과가 조용히
 * 어긋난다.
 */
function defaultStoreSource(): StoreSourceConfig {
  return {
    agent_name: '',
    namespace: 'default',
    series: [],
    ...DEFAULT_STORE_SOURCE_WINDOW,
  };
}

/**
 * 차트 패널 공통 데이터 소스 섹션.
 *
 * - 데이터 소스 토글(채널 / Store)을 제공한다.
 * - store 선택 시: Store 에이전트 선택 → 키 필터(이름/field/tag/data_type)
 *   → 키 멀티셀렉트로 store_source.series[] 를 채운다.
 * - 시간 윈도우 / 인터벌 / 집계 입력을 제공한다(tsdb 모달 컨트롤과 형상 일치).
 *
 * 두 소스는 공존하며 data_source 미지정은 'channel' 로 해석된다(하위 호환).
 *
 * @spec SPEC-WEB-005
 */
export function StoreSourceSection({
  panel,
  onConfigChange,
  fetchChannels = listChartChannels,
  onModeChange,
}: {
  panel: PanelConfig;
  onConfigChange: OnConfig;
  /** 채널 모드 시리즈 편집기에 주입할 활성 채널 조회기(테스트용). */
  fetchChannels?: () => Promise<ChartChannelSummary[]>;
  /**
   * 데이터소스 바인딩 모드(채널/Store/TSDB) 변경 콜백. 모드의 단일 소스 오브 트루스는
   * `config.data_source` 이며(@spec SPEC-TSDB-002 §2.11), 이 콜백은 그 파생값을 상위
   * (PanelSettingsDataSource)에 알린다 — 상위는 Store 모드에서만 선택 테이블을 렌더한다.
   * @spec SPEC-PANEL-SETTINGS-001 (데이터소스 토글 단일화)
   */
  onModeChange?: (mode: 'channel' | 'store' | 'tsdb') => void;
}): React.ReactElement {
  const { t } = useTranslation();
  const config = panel.config ?? {};
  const storeSource =
    (config.store_source as StoreSourceConfig | undefined) ?? defaultStoreSource();
  // 라인 차트 패널은 per-line 스타일 통합 편집(채널/스토어 시리즈 양쪽)을 노출한다.
  const isLineChart = panel.type === 'line-chart';

  // SPEC-TSDB-002 §2.11 [E1]: 모드의 단일 소스 오브 트루스가 로컬 `useState` 에서
  // `config.data_source` 로 이동했다. 세 모드 모두 config 에 영속되므로 TSDB 는 더
  // 이상 UI 전용이 아니다 — SPEC-PANEL-SETTINGS-001 REQ-05 의 비목표를 본 SPEC 이 대체한다.
  //
  // 모드 판정을 `config.data_source` 원문이 아니라 계약의 `kind` 로 둔다. 그 결과 인식
  // 불가 문자열은 `'channel'` 로 접힌다(§2.17-2). 이전엔 그런 config 에서 토글이 "아무것도
  // 선택되지 않음" 으로 보이고 채널 시리즈 편집기도 사라졌는데, 정작 패널은 channel 로
  // 폴백해 렌더하고 있었다. 이제 설정 UI 와 렌더 경로가 **같은 판정**을 쓴다.
  const mode = resolvePanelSourceBinding(config).kind;
  const isStoreMode = mode === 'store';
  useEffect(() => {
    onModeChange?.(mode);
  }, [mode, onModeChange]);

  const setDataSource = (kind: ChartDataSourceKind): void => {
    if (kind === 'store' && !config.store_source) {
      // 처음 store 로 전환 시 기본 설정을 함께 채운다.
      onConfigChange({ data_source: 'store', store_source: defaultStoreSource() });
    } else if (kind === 'tsdb' && !config.tsdb_source) {
      // 처음 TSDB 로 전환 시 기본 블록을 함께 채운다(§2.11 [E1]). 이미 `tsdb_source` 가
      // 있으면 종류만 기록해 기존 선택을 덮어쓰지 않는다 — store 쪽과 같은 규칙이다.
      onConfigChange({ data_source: 'tsdb', tsdb_source: defaultTsdbSource() });
    } else {
      onConfigChange({ data_source: kind });
    }
  };

  const patchStore = (patch: Partial<StoreSourceConfig>): void => {
    onConfigChange({ store_source: { ...storeSource, ...patch } });
  };

  // 에이전트 선택 파생값 — 에이전트 셀렉트를 데이터소스 토글과 같은 행(Row 1)에 두기 위해
  // 상위로 이관했다(레이아웃 전용, 동작/데이터 불변). @spec SPEC-WEB-006
  const { data: agentsResult } = useAgents();
  const storeAgents = useMemo(
    () =>
      (agentsResult?.data ?? []).filter((a: { type: string }) => a.type === 'store'),
    [agentsResult],
  );
  const selectedAgent = useMemo(
    () =>
      storeSource.agent_id
        ? storeAgents.find((a: { id: string }) => a.id === storeSource.agent_id)
        : storeAgents.find((a: { name: string }) => a.name === storeSource.agent_name),
    [storeAgents, storeSource.agent_id, storeSource.agent_name],
  );
  const selectValue = selectedAgent?.id ?? storeSource.agent_id ?? '';

  return (
    <div className="space-y-3">
      {/* Row 1: 데이터 소스 토글 + 에이전트 선택(스토어 모드) — 한 행 배치(레이블 위). */}
      <div className="flex flex-wrap items-start gap-3">
      {/* 데이터 소스 토글 */}
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.chart.dataSourceLabel')}
        </label>
        <div className="flex items-center gap-1.5">
        <div
          className="inline-flex rounded-md border border-(--color-border-default) bg-(--color-bg-surface) p-0.5"
          role="tablist"
          aria-label={t('dashboard.chart.dataSourceLabel')}
        >
          {(['channel', 'store', 'tsdb'] as const).map((kind) => {
            const selected = mode === kind;
            return (
              <button
                key={kind}
                type="button"
                role="tab"
                aria-selected={selected}
                data-testid={`chart-data-source-${kind}`}
                onClick={() => setDataSource(kind)}
                className={`rounded px-3 py-1 text-xs font-medium transition-colors ${
                  selected
                    ? 'bg-blue-600 text-white'
                    : 'text-(--color-text-secondary) hover:bg-(--color-bg-elevated)'
                }`}
              >
                {kind === 'channel'
                  ? t('dashboard.chart.dataSourceChannel')
                  : kind === 'store'
                    ? t('dashboard.chart.dataSourceStore')
                    : t('dashboard.chart.dataSourceTsdb')}
              </button>
            );
          })}
        </div>
        {/* Store 모드일 때만 조회 설정 정보 "i" 아이콘을 데이터소스 토글 옆에 표시한다. */}
        {isStoreMode && (
          <StoreInfoPopover storeSource={storeSource} />
        )}
        </div>
      </div>
        {/* Row 1 그룹 B: 에이전트 선택(스토어 모드에서만, 레이블 위). */}
        {isStoreMode && (
          <div className="min-w-[10rem] flex-1">
            <LabeledField label={t('dashboard.chart.storeAgent')}>
              <select
                data-testid="chart-store-agent-select"
                value={selectValue}
                onChange={(e) => {
                  const id = e.target.value;
                  if (!id) {
                    // 미선택으로 초기화.
                    patchStore({ agent_id: undefined, agent_name: '', series: [] });
                    return;
                  }
                  const agent = storeAgents.find((a: { id: string }) => a.id === id);
                  // agent_id(정본)와 agent_name(현재 이름 스냅샷)을 함께 저장, 시리즈 초기화.
                  patchStore({ agent_id: id, agent_name: agent?.name ?? '', series: [] });
                }}
                className={inputClass()}
              >
                <option value="">{t('dashboard.chart.storeAgentSelect')}</option>
                {storeAgents.map((a: { id: string; name: string }) => (
                  <option key={a.id} value={a.id}>
                    {a.name}
                  </option>
                ))}
                {/* 저장된 에이전트가 목록에 없으면(비활성/삭제 등) 저장된 이름으로 선택 유지 */}
                {selectValue && !selectedAgent && (
                  <option value={selectValue}>
                    {t('dashboard.chart.storeAgentInactive').replace(
                      '{name}',
                      storeSource.agent_name || selectValue,
                    )}
                  </option>
                )}
              </select>
            </LabeledField>
          </div>
        )}
      </div>

      {/* Store 모드: 이름을 지정하지 않은 시리즈의 표시 이름 형식(패널 단위 기본값). */}
      {isStoreMode && (
        <SeriesNameFormatField
          value={storeSource.series_name_format}
          onChange={(series_name_format) => patchStore({ series_name_format })}
          sample={storeSource.series?.[0]}
        />
      )}

      {/*
        Store 모드 + 대표값 대상 패널(stat/gauge/bar-chart/pie-chart)에서만 구간 대표값
        선택기를 노출한다. line-chart/table/heatmap 은 같은 섹션을 쓰지만 선택기가 없다
        (§2.3 / UB1-10). 채널 모드에서도 노출하지 않는다(§2.10 [S2]).
      */}
      {isStoreMode && REDUCE_PANEL_TYPES.has(panel.type) && (
        <SeriesReduceField
          panelType={panel.type}
          storeSource={storeSource}
          value={config.series_reduce as SeriesReduceFunc | undefined}
          onChange={(series_reduce) => onConfigChange({ series_reduce })}
        />
      )}

      {/*
        TSDB 선택 UI(에이전트/bucket/measurement 드릴다운)는 후속 커밋이 넣는다. 현 시점엔
        선택이 config 에 기록되기만 하고 전용 UI 는 없다 — 의도된 중간 상태다.
      */}

      {/*
        채널 모드 + 라인 차트: 채널 시리즈 편집기(채널 추가/선택/순서 + per-line 스타일).
        다른 차트 타입은 채널 모드에서 단일 channel_name 을 ChartChannelSection(우측 컬럼)
        으로 편집하므로 여기서는 렌더하지 않는다.
      */}
      {mode === 'channel' && isLineChart && (
        <ChannelSeriesEditor
          panel={panel}
          onConfigChange={onConfigChange}
          fetchChannels={fetchChannels}
        />
      )}
    </div>
  );
}

/** 대표값 → i18n 라벨 키. 7종 유니온이 모두 채워졌음을 타입으로 강제한다. */
const REDUCE_LABEL_KEYS: Record<SeriesReduceFunc, string> = {
  max: 'dashboard.chart.seriesReduceMax',
  avg: 'dashboard.chart.seriesReduceAvg',
  min: 'dashboard.chart.seriesReduceMin',
  last: 'dashboard.chart.seriesReduceLast',
  sum: 'dashboard.chart.seriesReduceSum',
  count: 'dashboard.chart.seriesReduceCount',
  delta: 'dashboard.chart.seriesReduceDelta',
};

/**
 * 음수가 나올 수 있는 대표값. 파이 차트와 조합하면 해당 시리즈 조각이 생략되므로
 * 설정 시점에 경고한다(§4.4 / M3.6). `max`/`avg`/`last` 도 원본이 음수면 음수가 될 수
 * 있지만, 이 셋은 "센서 값 그대로" 이므로 사용자가 이미 부호를 알고 고른다. 반면
 * `delta`(변화량) · `min` · `sum` 은 양수 데이터에서도 음수가 나올 수 있어 놀라움이 크다.
 */
const NEGATIVE_CAPABLE_REDUCES: ReadonlySet<SeriesReduceFunc> = new Set<SeriesReduceFunc>([
  'delta',
  'min',
  'sum',
]);

/**
 * 불리언 시리즈에서 각 대표값이 무엇을 뜻하는지 (spec.md §2.2 불리언 표).
 *
 * Store 변환 시점에 `true`/`false` 는 이미 `1`/`0` 으로 정규화되므로 계산에는 분기가
 * 없지만, **읽는 사람에게는 의미가 전혀 다르다** — 온도 시리즈의 `avg` 는 평균 온도지만
 * 불리언 시리즈의 `avg` 는 duty ratio 다. 숫자만 보고는 구분할 수 없으므로 선택 시점에
 * 안내한다(M6.2).
 */
const BOOLEAN_MEANING_KEYS: Record<SeriesReduceFunc, string> = {
  max: 'dashboard.chart.seriesReduceBoolMax',
  avg: 'dashboard.chart.seriesReduceBoolAvg',
  min: 'dashboard.chart.seriesReduceBoolMin',
  last: 'dashboard.chart.seriesReduceBoolLast',
  sum: 'dashboard.chart.seriesReduceBoolSum',
  count: 'dashboard.chart.seriesReduceBoolCount',
  delta: 'dashboard.chart.seriesReduceBoolDelta',
};

/**
 * 구간 대표값 선택기 (SPEC-CHART-002 §2.3 [U3]).
 *
 * **미지정**은 "기본값 last" 가 아니라 레거시 렌더 경로를 뜻하므로 빈 값 선택지를 첫
 * 항목으로 둔다(§2.9 [S1]). 선택을 지우면 `series_reduce` 를 `undefined` 로 되돌려
 * 저장된 패널이 예전 모습으로 정확히 복귀한다.
 *
 * 사용자 선택지에 `first` 는 없다 — `delta` 의 내부 입력일 뿐이다(§2.2).
 *
 * 대표값이 선택되면 **현재 조합 설명 한 줄**(M6.1)을 함께 보여준다. `store_source.
 * aggregation`(버킷 집계)과 `series_reduce`(윈도우 대표값)는 서로 다른 축인데(§2.1 [U1])
 * 두 셀렉트가 같은 화면에 나란히 있어 사용자가 가장 혼동하기 쉬운 지점이다 — 그래서
 * 순서(집계 → 대표값)를 문장으로 못박는다.
 */
function SeriesReduceField({
  panelType,
  storeSource,
  value,
  onChange,
}: {
  panelType: string;
  storeSource: StoreSourceConfig;
  value: SeriesReduceFunc | undefined;
  onChange: (next: SeriesReduceFunc | undefined) => void;
}): React.ReactElement {
  const { t } = useTranslation();
  const showPieWarning =
    panelType === 'pie-chart' && value !== undefined && NEGATIVE_CAPABLE_REDUCES.has(value);

  // 조합 설명(M6.1) — 버킷 집계 라벨 · 버킷 간격 · 대표값 라벨을 한 문장으로 합친다.
  // 간격 표기는 store 정보 팝오버(`sec()`)와 같은 규칙을 쓴다 — 같은 값이 화면마다 다른
  // 표기를 갖지 않도록.
  const aggLabelKey = STORE_AGG_OPTIONS.find((o) => o.value === storeSource.aggregation)?.labelKey;
  const comboText =
    value === undefined
      ? undefined
      : t('dashboard.chart.seriesReduceCombo')
          .replace('{agg}', aggLabelKey ? t(aggLabelKey) : String(storeSource.aggregation))
          .replace(
            '{interval}',
            `${Math.round((storeSource.interval_ms ?? 0) / 1000)}${t('dashboard.chart.storeInfoSecUnit')}`,
          )
          .replace('{reduce}', t(REDUCE_LABEL_KEYS[value]));

  // 불리언 안내(M6.2) — `keys` 모드에서 선택된 시리즈의 메타데이터로만 판정한다.
  // `tag` 모드는 폴링 시점에 키가 해석되어 설정 화면에서 data_type 을 알 수 없으므로
  // 안내하지 않는다(추측해서 틀린 안내를 하는 것보다 침묵이 낫다).
  const hasBooleanSeries = (storeSource.series ?? []).some((sr) => sr.data_type === 'boolean');
  const booleanText =
    value !== undefined && hasBooleanSeries
      ? t('dashboard.chart.seriesReduceBooleanHint').replace(
          '{meaning}',
          t(BOOLEAN_MEANING_KEYS[value]),
        )
      : undefined;

  return (
    <div className="space-y-1">
      <LabeledField
        label={t('dashboard.chart.seriesReduce')}
        hint={value === undefined ? t('dashboard.chart.seriesReduceNoneHint') : undefined}
      >
        <select
          data-testid="chart-series-reduce"
          value={value ?? ''}
          onChange={(e) => {
            const next = e.target.value;
            onChange(next === '' ? undefined : (next as SeriesReduceFunc));
          }}
          className={inputClass()}
        >
          <option value="">{t('dashboard.chart.seriesReduceNone')}</option>
          {SERIES_REDUCE_FUNCS.map((fn) => (
            <option key={fn} value={fn}>
              {t(REDUCE_LABEL_KEYS[fn])}
            </option>
          ))}
        </select>
      </LabeledField>
      {comboText !== undefined && (
        <p
          data-testid="chart-series-reduce-combo"
          className="text-[11px] leading-snug text-(--color-text-muted)"
        >
          {comboText}
        </p>
      )}
      {booleanText !== undefined && (
        <p
          data-testid="chart-series-reduce-boolean-hint"
          className="text-[11px] leading-snug text-(--color-text-muted)"
        >
          {booleanText}
        </p>
      )}
      {showPieWarning && (
        <p
          data-testid="chart-series-reduce-pie-warning"
          className="rounded-md bg-amber-50 px-2 py-1.5 text-[11px] leading-snug text-amber-700 dark:bg-amber-900/30 dark:text-amber-300"
        >
          {t('dashboard.chart.pieNegativeReduceWarning')}
        </p>
      )}
    </div>
  );
}

/**
 * 시리즈 이름 형식(패널 단위 기본값) 편집 필드.
 *
 * 이름(alias)을 직접 입력하지 않은 시리즈의 표시 이름을 이 템플릿으로 만든다. 토큰 문법은
 * 시리즈별 이름 입력과 동일하며(`{$.measurement}` / `{$.field}` / `{$.tags.NAME}`),
 * 비워 두면 내장 서술 표기(`measurement · field{k=v}`)를 쓴다.
 *
 * 미리보기는 선택된 첫 시리즈로 해석해 보여준다 — 선택 전에는 형식만 보인다.
 */
function SeriesNameFormatField({
  value,
  onChange,
  sample,
}: {
  value: string | undefined;
  onChange: (next: string | undefined) => void;
  sample: StoreSeriesRef | undefined;
}): React.ReactElement {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLInputElement | null>(null);
  const ctx = sample
    ? { measurement: sample.key, field: sample.field, tags: sample.tags ?? {} }
    : undefined;
  const tokenPaths = ctx ? availableAliasTokens(ctx) : [];

  const insertToken = (path: string): void => {
    const token = makeAliasToken(path);
    const current = value ?? '';
    const el = inputRef.current;
    let next: string;
    if (el && el.selectionStart != null && el.selectionEnd != null) {
      next = current.slice(0, el.selectionStart) + token + current.slice(el.selectionEnd);
    } else {
      next = current + token;
    }
    onChange(next.trim() === '' ? undefined : next);
  };

  const preview =
    value && value.trim() !== '' && ctx ? resolveSeriesAlias(value, ctx) : undefined;

  return (
    <div className="space-y-1" data-testid="chart-store-series-name-format">
      <LabeledField label={t('dashboard.chart.storeSeriesNameFormat')}>
        <input
          type="text"
          value={value ?? ''}
          ref={inputRef}
          placeholder={t('dashboard.chart.storeSeriesNameFormatPlaceholder')}
          onChange={(e) => onChange(e.target.value.trim() === '' ? undefined : e.target.value)}
          className={inputClass()}
          data-testid="chart-store-series-name-format-input"
        />
      </LabeledField>
      <div className="flex flex-wrap items-center gap-1.5 px-0.5">
        <span className="text-xs text-(--color-text-muted)">
          {t('dashboard.chart.storeAliasInsertToken')}
        </span>
        {tokenPaths.map((k) => (
          <button
            key={k}
            type="button"
            onClick={() => insertToken(k)}
            data-testid={`chart-store-series-name-format-token-${k}`}
            className="rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-0.5 font-mono text-xs text-blue-600 transition-colors hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/30"
          >
            {makeAliasToken(k)}
          </button>
        ))}
        {preview !== undefined && (
          <span className="ml-1 inline-flex min-w-0 items-center gap-0.5 text-xs text-(--color-text-muted)">
            <span>{t('dashboard.chart.storeAliasPreview')}</span>
            <span className="truncate font-mono text-(--color-text-primary)">{preview}</span>
          </span>
        )}
      </div>
    </div>
  );
}

/**
 * Store 조회 설정(시간 윈도우 / 인터벌 / 집계 / 갱신 주기)을 읽기전용으로 보여주는 정보
 * 말풍선. "i" 아이콘 클릭 시 팝오버로 현재 값 + 설명을 표시한다(인라인 편집 대신).
 * 값은 store_source 설정에서 읽으며 useStoreChartData 가 그대로 소비한다 — 편집 UI 만
 * 제거하고 설정 값/기본값은 그대로 보존된다. 클릭 아웃사이드로 닫는다(태그 헤더 피커와 동일).
 * @spec SPEC-PANEL-SETTINGS-001
 */
function StoreInfoPopover({
  storeSource,
}: {
  storeSource: StoreSourceConfig;
}): React.ReactElement {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLSpanElement>(null);

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

  const sec = (ms: number): string =>
    `${Math.round(ms / 1000)}${t('dashboard.chart.storeInfoSecUnit')}`;
  const aggLabelKey = STORE_AGG_OPTIONS.find(
    (o) => o.value === storeSource.aggregation,
  )?.labelKey;
  const aggLabel = aggLabelKey ? t(aggLabelKey) : storeSource.aggregation;

  const rows: { label: string; value: string }[] = [
    { label: t('dashboard.chart.storeInfoTimeWindow'), value: sec(storeSource.time_window_ms) },
    { label: t('dashboard.chart.storeInfoInterval'), value: sec(storeSource.interval_ms) },
    { label: t('dashboard.chart.storeInfoAggregation'), value: aggLabel },
    { label: t('dashboard.chart.storeInfoRefresh'), value: sec(storeSource.refresh_interval_ms ?? 5000) },
  ];

  return (
    <span className="relative inline-flex" ref={containerRef}>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-haspopup="true"
        aria-expanded={open}
        data-testid="chart-store-info-button"
        className="inline-flex items-center rounded p-0.5 text-(--color-text-muted) opacity-70 transition-colors hover:opacity-100 hover:text-(--color-text-primary)"
        title={t('dashboard.chart.storeInfoAria')}
        aria-label={t('dashboard.chart.storeInfoAria')}
      >
        <Info className="h-3.5 w-3.5" aria-hidden="true" />
      </button>
      {open && (
        <div
          data-testid="chart-store-info-popover"
          className="absolute left-0 top-full z-30 mt-1 w-60 rounded-md border border-(--color-border-default) bg-(--color-bg-primary) p-2.5 text-left shadow-lg"
        >
          <p className="mb-1.5 text-xs font-semibold text-(--color-text-primary)">
            {t('dashboard.chart.storeInfoTitle')}
          </p>
          <dl className="space-y-1">
            {rows.map((r) => (
              <div
                key={r.label}
                className="flex items-baseline justify-between gap-3 text-[11px]"
              >
                <dt className="text-(--color-text-muted)">{r.label}</dt>
                <dd className="font-mono text-(--color-text-secondary)">{r.value}</dd>
              </div>
            ))}
          </dl>
          <p className="mt-2 text-[10px] leading-snug text-(--color-text-muted)">
            {t('dashboard.chart.storeTimeWindowHint')}
          </p>
        </div>
      )}
    </span>
  );
}

/**
 * 태그 자동 선택기 (SPEC-WEB-005 tag 모드).
 *
 * 사용자가 태그 키마다 값을 하나씩 골라 AND 필터(`tag_filters`)를 구성한다. 매칭되는
 * 모든 store 키가 폴링 시점에 자동으로 시리즈가 되며(키 추가/삭제 자동 반영), 개별 키를
 * `series[]` 로 고정하지 않는다. 라이브 미리보기로 현재 매칭 키 수를 표시한다.
 *
 * `useStoreTagPairs` 로 사용 가능한 태그 키/값을 조회하고, `useStoreKeysWithTags` 의
 * 키 객체로 클라이언트 측에서 매칭 키 수를 계산한다(추가 네트워크 호출 없이).
 * 구버전 서버(태그 엔드포인트 미지원)나 태그가 하나도 없으면 안내 문구를 표시한다.
 *
 * @spec SPEC-WEB-005
 */
export function StoreTagSelectionEditor({
  agentName,
  tagFilters,
  onChange,
}: {
  agentName: string;
  tagFilters: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
}): React.ReactElement {
  const { t } = useTranslation();
  const { data: tagPairs, isLoading, isError } = useStoreTagPairs(agentName);
  const { data: keysData } = useStoreKeysWithTags(agentName);

  const pairs = useMemo(() => tagPairs ?? [], [tagPairs]);

  // 라이브 매칭 키 수: 선택된 tag_filters(AND)로 키 객체를 좁혀 distinct key 수를 센다.
  // 백엔드 GET /keys?tag=k:v 와 동일한 AND 의미(storeSourceFilter)를 재사용한다.
  const matchCount = useMemo(() => {
    const objects = keysData?.keyObjects ?? [];
    const filterSet = new Set(
      Object.entries(tagFilters).map(([k, v]) => makeTagFilterId(k, v)),
    );
    const matched = filterStoreKeyObjects(objects, { tagFilters: filterSet });
    return new Set(matched.map((o) => o.key)).size;
  }, [keysData, tagFilters]);

  const setValue = (key: string, value: string): void => {
    const next = { ...tagFilters };
    if (value === '') delete next[key];
    else next[key] = value;
    onChange(next);
  };

  if (isLoading) {
    return (
      <p className="py-2 text-center text-[11px] text-(--color-text-muted)">
        {t('dashboard.chart.storeKeysLoading')}
      </p>
    );
  }
  // 구버전 서버(4xx) 또는 태그 없음 → 안내(키 직접 선택 사용 권장).
  if (isError || pairs.length === 0) {
    return (
      <p
        className="py-2 text-center text-[11px] text-(--color-text-muted)"
        data-testid="chart-store-tag-none"
      >
        {t('dashboard.chart.storeTagNone')}
      </p>
    );
  }

  return (
    <div
      className="space-y-2 rounded border border-(--color-border-default) p-2"
      data-testid="chart-store-tag-selection"
    >
      <label className="block text-xs font-medium text-(--color-text-muted)">
        {t('dashboard.chart.storeTagPickerLabel')}
      </label>
      {/* 태그 키마다 값 셀렉트(빈 값 = 미적용). 서로 다른 키는 AND 로 결합된다. */}
      <div className="grid grid-cols-2 gap-2">
        {pairs.map((p) => (
          <LabeledField key={p.key} label={p.key}>
            <select
              value={tagFilters[p.key] ?? ''}
              data-testid={`chart-store-tag-select-${p.key}`}
              onChange={(e) => setValue(p.key, e.target.value)}
              className={inputClass()}
            >
              <option value="">{t('dashboard.chart.storeTagAnyValue')}</option>
              {p.values.map((v) => (
                <option key={v} value={v}>
                  {v}
                </option>
              ))}
            </select>
          </LabeledField>
        ))}
      </div>
      {/* 라이브 매칭 키 수 미리보기. */}
      <p
        className="text-[11px] text-(--color-text-secondary)"
        data-testid="chart-store-tag-match-count"
      >
        {t('dashboard.chart.storeTagMatchCount').replace('{count}', String(matchCount))}
      </p>
    </div>
  );
}

/**
 * 통합 per-line 스타일 컨트롤 (SPEC-WEB-005).
 *
 * stroke_style / stroke_width / smooth / display_field 를 편집한다. 채널 시리즈
 * (ChannelRefConfig)와 스토어 시리즈(StoreSeriesRef)가 동일한 스타일 필드를 가지므로
 * 하나의 컴포넌트로 양쪽 라인 차트 시리즈 스타일을 통합한다. 라인 차트 패널에서만
 * 노출된다.
 *
 * `display_field` 는 채널 시리즈에서만 렌더에 영향을 주며, 스토어 시리즈는 매트릭스가
 * 이미 단일 숫자 값을 제공하므로 표시는 되지만 렌더 결과에는 영향을 주지 않는다.
 */
interface LineStyleValue {
  stroke_style?: StrokeStyle;
  stroke_width?: number;
  smooth?: boolean;
  display_field?: string;
}

export function LineStyleControls({
  value,
  onPatch,
  testIdPrefix,
}: {
  value: LineStyleValue;
  onPatch: (patch: LineStyleValue) => void;
  testIdPrefix: string;
}): React.ReactElement {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-2" data-testid={`${testIdPrefix}-line-style`}>
      <select
        value={value.stroke_style ?? 'solid'}
        onChange={(e) => onPatch({ stroke_style: e.target.value as StrokeStyle })}
        aria-label={t('dashboard.chart.lineStyleAria')}
        data-testid={`${testIdPrefix}-stroke-style`}
        className="rounded border border-(--color-border-default) bg-(--color-bg-surface) px-2 py-1.5 text-sm"
      >
        <option value="solid">{t('dashboard.chart.lineSolid')}</option>
        <option value="dashed">{t('dashboard.chart.lineDashed')}</option>
        <option value="dotted">{t('dashboard.chart.lineDotted')}</option>
      </select>
      <label className="flex items-center gap-1 text-sm text-(--color-text-muted)">
        {t('dashboard.chart.thickness')}
        <input
          type="number"
          min={1}
          max={6}
          value={value.stroke_width ?? 2}
          onChange={(e) => {
            const n = parseInt(e.target.value, 10);
            if (!Number.isNaN(n)) onPatch({ stroke_width: n });
          }}
          data-testid={`${testIdPrefix}-stroke-width`}
          className="w-14 rounded border border-(--color-border-default) bg-(--color-bg-surface) px-2 py-1.5 text-sm"
        />
      </label>
      <label className="flex cursor-pointer items-center gap-1 text-sm text-(--color-text-muted)">
        <input
          type="checkbox"
          checked={value.smooth ?? false}
          onChange={(e) => onPatch({ smooth: e.target.checked })}
          data-testid={`${testIdPrefix}-smooth`}
          className="h-3 w-3 rounded border-gray-300"
        />
        {t('dashboard.chart.curve')}
      </label>
      <input
        type="text"
        value={value.display_field ?? ''}
        onChange={(e) => onPatch({ display_field: e.target.value || undefined })}
        placeholder={t('dashboard.chart.displayFieldShort')}
        aria-label={t('dashboard.chart.displayFieldAria')}
        data-testid={`${testIdPrefix}-display-field`}
        className="w-32 rounded border border-(--color-border-default) bg-(--color-bg-surface) px-2 py-1.5 text-sm"
      />
    </div>
  );
}

/**
 * 시리즈 alias 텍스트 입력(인라인). 토큰 삽입을 위해 DOM 참조를 상위로 전달한다.
 * 빈 값은 alias=undefined 로 저장해 키명 폴백을 유지한다(하위 호환).
 *
 * @spec SPEC-WEB-005
 */
export function SeriesAliasInput({
  index,
  seriesKey,
  alias,
  onAliasChange,
  inputRef,
}: {
  index: number;
  seriesKey: string;
  alias: string | undefined;
  onAliasChange: (alias: string | undefined) => void;
  inputRef: (el: HTMLInputElement | null) => void;
}): React.ReactElement {
  const { t } = useTranslation();
  return (
    <input
      ref={inputRef}
      type="text"
      value={alias ?? ''}
      onChange={(e) => {
        const v = e.target.value;
        onAliasChange(v.trim() === '' ? undefined : v);
      }}
      placeholder={seriesKey}
      aria-label={t('dashboard.chart.storeSeriesNameAria').replace('{key}', seriesKey)}
      data-testid={`chart-store-series-alias-${index}`}
      className="w-40 shrink-0 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-2 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-500"
    />
  );
}

/**
 * 시리즈 이름 토큰 삽입 버튼 + 실시간 미리보기 서브행 (SPEC-WEB-005).
 *
 * - 시리즈 키 / field / 각 태그마다 `{$.…}` 삽입 버튼을 제공하고, 클릭 시
 *   입력 커서 위치(없으면 끝)에 토큰을 삽입한다. 사용자는 토큰과 리터럴 문자열을
 *   섞어 표시 이름을 조립한다(예: "[{$.tags.room}] {$.measurement}/{$.metric}").
 * - 삽입 가능한 토큰이 하나도 없으면(키/필드/태그가 모두 없음) 노출하지 않는다.
 * - 미리보기는 resolveSeriesAlias 결과를 보여준다. alias 가 비어있으면 키명으로
 *   폴백(현재 렌더 동작과 동일).
 */
export function SeriesAliasTokens({
  index,
  seriesKey,
  fieldName,
  alias,
  tags,
  onAliasChange,
  getInput,
}: {
  index: number;
  seriesKey: string;
  fieldName?: string;
  alias: string | undefined;
  tags: Record<string, string>;
  onAliasChange: (alias: string | undefined) => void;
  getInput: () => HTMLInputElement | null;
}): React.ReactElement | null {
  const { t } = useTranslation();
  const aliasCtx = { measurement: seriesKey, metric: fieldName, tags };
  const tokenPaths = availableAliasTokens(aliasCtx);
  if (tokenPaths.length === 0) return null;

  // 커서 위치(없으면 끝)에 토큰을 삽입한다.
  const insertToken = (tokenPath: string): void => {
    const token = makeAliasToken(tokenPath);
    const current = alias ?? '';
    const el = getInput();
    let next: string;
    if (el && el.selectionStart != null && el.selectionEnd != null) {
      const start = el.selectionStart;
      const end = el.selectionEnd;
      next = current.slice(0, start) + token + current.slice(end);
    } else {
      next = current + token;
    }
    onAliasChange(next.trim() === '' ? undefined : next);
    // 삽입 후 커서를 토큰 끝으로 이동(가능할 때).
    if (el) {
      const caret =
        (el.selectionStart ?? current.length) + token.length;
      requestAnimationFrame(() => {
        try {
          el.focus();
          el.setSelectionRange(caret, caret);
        } catch {
          // jsdom 등에서 setSelectionRange 미지원 시 무시.
        }
      });
    }
  };

  // 미리보기: alias 비어있으면 키명 폴백(렌더 동작과 일치).
  const preview =
    alias && alias.trim() !== '' ? resolveSeriesAlias(alias, aliasCtx) : seriesKey;

  return (
    <div
      className="flex flex-wrap items-center gap-1.5 px-1.5 pb-1"
      data-testid={`chart-store-series-tokens-${index}`}
    >
      <span className="text-xs text-(--color-text-muted)">
        {t('dashboard.chart.storeAliasInsertToken')}
      </span>
      {tokenPaths.map((k) => (
        <button
          key={k}
          type="button"
          onClick={() => insertToken(k)}
          data-testid={`chart-store-series-token-${index}-${k}`}
          className="rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-0.5 font-mono text-xs text-blue-600 transition-colors hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/30"
        >
          {makeAliasToken(k)}
        </button>
      ))}
      {/* 미리보기: 라벨(i18n) + 해석값(raw). 값은 별도 노드로 두어 항상 확인 가능. */}
      <span
        className="ml-1 inline-flex min-w-0 items-center gap-0.5 text-xs text-(--color-text-muted)"
        data-testid={`chart-store-series-preview-${index}`}
      >
        <span>{t('dashboard.chart.storeAliasPreview')}</span>
        <span className="truncate font-mono text-(--color-text-primary)">{preview}</span>
      </span>
    </div>
  );
}

/**
 * 선택된 스토어 시리즈 목록 + 시리즈별 표시 이름(alias)/색상/라인 스타일 편집기.
 *
 * store_source.series[] 의 각 항목을 행으로 표시한다. 항상 alias + color 를 편집하고,
 * 라인 차트 패널(`isLineChart`)에서는 펼침 시 통합 라인 스타일(stroke_style/width/
 * smooth/display_field)도 편집한다. alias 빈 값은 undefined 로 저장해 key/컬럼명으로
 * 폴백한다. 모든 필드는 useStoreChartData 의 매핑 경로(store_source.series 직접 읽음)를
 * 통해 차트 범례/라인 스타일에 반영된다.
 *
 * @spec SPEC-WEB-005
 */
/**
 * 단일 선택 시리즈의 인라인 세부 편집기(이름/색상/(라인 차트)선 스타일/(heatmap)위치).
 *
 * v0.4.0(REQ-18/20): 별도 SelectedSeriesList 섹션을 제거하고, 선택된 각 시리즈 행의 인라인
 * 펼침 상세 안에서 이 편집기를 렌더한다. 이름은 편집 가능 텍스트 → `StoreSeriesRef.alias`
 * draft(범례/미리보기 반영). 색상/선 스타일 → 동일 `StoreSeriesRef`. positionEditor(heatmap
 * 전용, REQ-21)가 주입되면 같은 펼침 안에 좌표 입력을 함께 렌더한다. 바인딩 모드와 무관하게
 * 동작한다(keys-모드 게이트 제거, REQ-22 독립성).
 *
 * @spec SPEC-PANEL-SETTINGS-001 (REQ-18/19/20/21) / SPEC-WEB-005
 */
export function SeriesDetailEditor({
  series,
  index,
  isLineChart,
  onPatch,
  positionEditor,
}: {
  series: StoreSeriesRef;
  index: number;
  isLineChart: boolean;
  onPatch: (patch: Partial<StoreSeriesRef>) => void;
  /** heatmap 전용(REQ-21): 같은 펼침 안에 렌더할 센서 좌표(x/y) 편집 노드. */
  positionEditor?: React.ReactNode;
}): React.ReactElement {
  const { t } = useTranslation();
  const aliasInputRef = useRef<HTMLInputElement | null>(null);
  // v0.5.0(REQ-18/19 폐지/AC-20/21): 펼침 상세는 시리즈 편집 필드만 — 이름 위 키·종류·태그 설명
  // 서브라인 없음(테이블 컬럼 + alias 컬럼 키 에코와 중복). 레이아웃: 이름 옆 색상 입력 제거(색상은
  // 아래 전용 행), (heatmap)좌표는 이름 뒤 2열(이름 | 좌표). 본문 폰트는 text-sm 이상.
  return (
    <div className="space-y-2 text-sm" data-testid={`series-detail-${index}`}>
      {/* 이름(name/alias, 편집 가능 텍스트) | (heatmap)좌표 — 2열 레이아웃(이름 뒤 좌표). */}
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2">
        <div className="flex items-center gap-2">
          <label className="text-sm font-medium text-(--color-text-muted)">
            {t('dashboard.settings.seriesDetailsName')}
          </label>
          <SeriesAliasInput
            index={index}
            seriesKey={series.key}
            alias={series.alias}
            onAliasChange={(alias) => onPatch({ alias })}
            inputRef={(el) => {
              aliasInputRef.current = el;
            }}
          />
        </div>
        {positionEditor}
      </div>
      {/* 이름 템플릿 토큰(키 / field / 태그). */}
      <SeriesAliasTokens
        index={index}
        seriesKey={series.key}
        fieldName={series.field}
        alias={series.alias}
        tags={series.tags ?? {}}
        onAliasChange={(alias) => onPatch({ alias })}
        getInput={() => aliasInputRef.current}
      />
      {/* 색상(이름 옆에서 이동한 전용 행 — 모든 패널 타입에서 편집 가능, `color` 불변). */}
      <div className="flex items-center gap-2">
        <label className="text-sm font-medium text-(--color-text-muted)">
          {t('dashboard.settings.seriesDetailsColor')}
        </label>
        <input
          type="color"
          value={series.color ?? pickSeriesColor(index)}
          onChange={(e) => onPatch({ color: e.target.value })}
          aria-label={t('dashboard.chart.storeSeriesColorAria').replace('{key}', series.key)}
          data-testid={`chart-store-series-color-${index}`}
          className="h-7 w-9 shrink-0 cursor-pointer rounded border border-(--color-border-default) bg-transparent p-0"
        />
      </div>
      {/* 라인 차트: 통합 라인 스타일 편집. */}
      {isLineChart && (
        <LineStyleControls
          value={series}
          onPatch={(patch) => onPatch(patch)}
          testIdPrefix={`chart-store-series-${index}`}
        />
      )}
    </div>
  );
}

// --- 2. stat 패널 설정 (SPEC §4.2.2 stat) ---

export function StatChartSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: OnConfig;
}): React.ReactElement {
  const { t } = useTranslation();
  const config = panel.config ?? {};
  const displayField = (config.display_field as string | undefined) ?? 'value';
  const unit = (config.unit as string | undefined) ?? '';
  const decimalPlaces = (config.decimal_places as number | undefined) ?? 2;
  const rules =
    (config.threshold_color_rules as Array<{ min: number; color: string }> | undefined) ?? [];

  const addRule = (): void => {
    onConfigChange({
      threshold_color_rules: [...rules, { min: 0, color: '#3b82f6' }],
    });
  };
  const updateRule = (i: number, patch: Partial<{ min: number; color: string }>): void => {
    onConfigChange({
      threshold_color_rules: rules.map((r, idx) => (idx === i ? { ...r, ...patch } : r)),
    });
  };
  const removeRule = (i: number): void => {
    onConfigChange({
      threshold_color_rules: rules.filter((_, idx) => idx !== i),
    });
  };

  return (
    <div className="space-y-3">
      <LabeledField
        label={t('dashboard.chart.displayField')}
        hint={t('dashboard.chart.displayFieldHint')}
      >
        <input
          type="text"
          value={displayField}
          onChange={(e) => onConfigChange({ display_field: e.target.value })}
          className={inputClass()}
        />
      </LabeledField>
      <LabeledField label={t('dashboard.chart.unit')}>
        <input
          type="text"
          value={unit}
          onChange={(e) => onConfigChange({ unit: e.target.value })}
          placeholder={t('dashboard.chart.unitPlaceholder')}
          className={inputClass()}
        />
      </LabeledField>
      <LabeledField label={t('dashboard.chart.decimalPlaces')}>
        <input
          type="number"
          min={0}
          max={10}
          value={decimalPlaces}
          onChange={(e) => {
            const n = parseInt(e.target.value, 10);
            if (!Number.isNaN(n)) onConfigChange({ decimal_places: n });
          }}
          className={inputClass()}
        />
      </LabeledField>
      <div>
        <div className="mb-1.5 flex items-center justify-between">
          <label className="text-xs font-medium text-(--color-text-muted)">
            {t('dashboard.chart.thresholdColorRules')}
          </label>
          <button
            type="button"
            onClick={addRule}
            className="flex items-center gap-1 rounded-md px-2 py-0.5 text-xs font-medium text-blue-600 transition-colors hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20"
          >
            <Plus className="h-3 w-3" /> {t('dashboard.chart.add')}
          </button>
        </div>
        <div className="space-y-1.5">
          {rules.map((r, i) => (
            <div key={i} className="flex items-center gap-1.5">
              <input
                type="number"
                value={r.min}
                onChange={(e) => {
                  const n = parseFloat(e.target.value);
                  if (!Number.isNaN(n)) updateRule(i, { min: n });
                }}
                placeholder="min"
                className="w-20 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
              />
              <label className="relative flex h-6 w-6 shrink-0 cursor-pointer items-center justify-center rounded-md">
                <span
                  className="h-4 w-4 rounded-sm border border-gray-200 dark:border-gray-600"
                  style={{ backgroundColor: r.color }}
                />
                <input
                  type="color"
                  value={r.color}
                  onChange={(e) => updateRule(i, { color: e.target.value })}
                  className="absolute inset-0 cursor-pointer opacity-0"
                />
              </label>
              <span className="flex-1 text-[11px] text-(--color-text-muted)">
                {t('dashboard.chart.ruleHint')
                  .replace('{min}', String(r.min))
                  .replace('{color}', r.color)}
              </span>
              <button
                type="button"
                onClick={() => removeRule(i)}
                className="rounded p-0.5 text-(--color-text-muted) transition-colors hover:text-red-500"
                aria-label={t('dashboard.chart.deleteRuleAria')}
              >
                <Trash2 className="h-3 w-3" />
              </button>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

// --- 다채널 행 컴포넌트 (LineChartSection 내부에서 사용) ---

function ChannelRow({
  idx,
  channel,
  activeChannels,
  activeNameSet,
  channelsLoadState,
  canDelete,
  onPatch,
  onRemove,
  onDragStart,
  onDragOver,
  onDrop,
}: {
  idx: number;
  channel: ChannelRefConfig;
  activeChannels: ChartChannelSummary[];
  activeNameSet: Set<string>;
  channelsLoadState: 'idle' | 'loading' | 'error';
  canDelete: boolean;
  onPatch: (patch: Partial<ChannelRefConfig>) => void;
  onRemove: () => void;
  onDragStart: (e: React.DragEvent) => void;
  onDragOver: (e: React.DragEvent) => void;
  onDrop: (e: React.DragEvent) => void;
}): React.ReactElement {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);

  const currentName = channel.name ?? '';
  const isInactive = currentName !== '' && !activeNameSet.has(currentName);

  const [selectedOption, setSelectedOption] = useState<string>(currentName);
  const [customDraft, setCustomDraft] = useState<string>('');
  useEffect(() => {
    setSelectedOption(currentName);
    setCustomDraft('');
  }, [currentName]);
  const handleSelect = (value: string): void => {
    setSelectedOption(value);
    if (value === CUSTOM_CHANNEL_SENTINEL) {
      setCustomDraft(isInactive ? currentName : '');
      return;
    }
    if (value !== currentName) {
      const patch: Partial<ChannelRefConfig> = { name: value };
      if (!channel.alias && value) patch.alias = value;
      onPatch(patch);
    }
  };
  const commitCustom = (): void => {
    const trimmed = customDraft.trim();
    if (trimmed && CHANNEL_NAME_REGEX.test(trimmed) && trimmed !== currentName) {
      const patch: Partial<ChannelRefConfig> = { name: trimmed };
      if (!channel.alias && trimmed) patch.alias = trimmed;
      onPatch(patch);
    }
  };

  const effectiveColor = channel.color ?? pickSeriesColor(idx);

  return (
    <div
      data-testid={`line-chart-channel-row-${idx}`}
      onDragOver={onDragOver}
      onDrop={onDrop}
      className="rounded-md border border-(--color-border-default) bg-(--color-bg-elevated)"
    >
      {/* 접힌 상태: 이름 + 색상 dot + 삭제 */}
      <div className="flex items-center gap-1.5 px-2 py-1.5">
        <span
          draggable
          onDragStart={onDragStart}
          data-testid={`line-chart-channel-drag-${idx}`}
          title={t('dashboard.chart.dragOrderTitle')}
          className="flex h-5 w-4 cursor-grab items-center justify-center text-(--color-text-muted) active:cursor-grabbing"
        >
          <GripVertical className="h-3 w-3" />
        </span>
        <button
          type="button"
          onClick={() => setExpanded((v) => !v)}
          className="flex items-center gap-1 text-(--color-text-muted)"
          aria-label={expanded ? t('dashboard.chart.collapseAria') : t('dashboard.chart.expandAria')}
        >
          {expanded
            ? <ChevronDown className="h-3 w-3" />
            : <ChevronRight className="h-3 w-3" />}
        </button>
        <span
          className="h-3 w-3 shrink-0 cursor-pointer rounded-full ring-1 ring-(--color-border-default)"
          style={{ backgroundColor: effectiveColor }}
          title={t('dashboard.chart.colorChangeTitle')}
          onClick={() => {
            const input = document.getElementById(`ch-color-${idx}`);
            input?.click();
          }}
        />
        <input
          id={`ch-color-${idx}`}
          type="color"
          value={effectiveColor}
          onChange={(e) => onPatch({ color: e.target.value })}
          className="invisible absolute h-0 w-0"
          tabIndex={-1}
        />
        <input
          type="text"
          value={channel.alias ?? ''}
          onChange={(e) => onPatch({ alias: e.target.value || undefined })}
          placeholder={currentName || t('dashboard.chart.unspecified')}
          className="min-w-0 flex-1 truncate border-0 bg-transparent px-0 text-xs font-medium text-(--color-text-primary) outline-none placeholder:text-(--color-text-muted) focus:ring-0"
          aria-label={t('dashboard.chart.displayNameAria')}
        />
        {canDelete && (
          <button
            type="button"
            onClick={onRemove}
            aria-label={t('dashboard.chart.deleteChannelAria')}
            className="flex h-5 w-5 items-center justify-center rounded text-(--color-text-muted) hover:bg-red-50 hover:text-red-600"
          >
            <Trash2 className="h-3 w-3" />
          </button>
        )}
      </div>

      {/* 펼친 상태 */}
      {expanded && (
        <div className="space-y-2 border-t border-(--color-border-default) px-2 pt-2 pb-2">
          {/* 줄 1: 채널 선택 */}
          <div className="flex items-center gap-1.5">
            <select
              data-testid={`line-chart-channel-row-select-${idx}`}
              value={selectedOption}
              onChange={(e) => handleSelect(e.target.value)}
              disabled={channelsLoadState === 'loading'}
              className="flex-1 rounded border border-(--color-border-default) bg-(--color-bg-surface) px-2 py-1 text-xs disabled:opacity-60"
              aria-label={t('dashboard.chart.selectChannelAria')}
            >
              <option value="">
                {channelsLoadState === 'loading'
                  ? t('dashboard.chart.loading')
                  : activeChannels.length === 0
                    ? t('dashboard.chart.noActiveChannels')
                    : t('dashboard.chart.selectChannelAria')}
              </option>
              {isInactive && selectedOption !== CUSTOM_CHANNEL_SENTINEL && (
                <option value={currentName}>{t('dashboard.chart.channelInactiveShort').replace('{name}', currentName)}</option>
              )}
              {activeChannels.map((ch) => (
                <option key={ch.name} value={ch.name}>{ch.name}</option>
              ))}
              <option value={CUSTOM_CHANNEL_SENTINEL}>{t('dashboard.chart.customShort')}</option>
            </select>
          </div>
          {selectedOption === CUSTOM_CHANNEL_SENTINEL && (
            <input
              type="text"
              data-testid={`line-chart-channel-row-custom-${idx}`}
              value={customDraft}
              onChange={(e) => setCustomDraft(e.target.value)}
              onBlur={commitCustom}
              onKeyDown={(e) => {
                if (e.key === 'Enter') (e.target as HTMLInputElement).blur();
              }}
              placeholder={t('dashboard.chart.customDeployPlaceholder')}
              autoFocus
              className="w-full rounded border border-(--color-border-default) bg-(--color-bg-surface) px-2 py-1 text-xs"
            />
          )}

          {/* 줄 2: 통합 라인 스타일(stroke/width/smooth/display_field) */}
          <LineStyleControls
            value={channel}
            onPatch={(patch) => onPatch(patch)}
            testIdPrefix={`line-chart-channel-row-${idx}`}
          />
        </div>
      )}
    </div>
  );
}

/**
 * 채널 모드 시리즈 편집기 (SPEC-WEB-005).
 *
 * 라인 차트 패널의 채널(channels[]) 추가/선택/순서변경 + per-line 스타일을 데이터
 * 소스 영역에서 편집한다. 기존 LineChartSection 의 채널 편집 블록을 이곳으로 이전했다.
 * channel_name 만 있는 기존 패널은 channels[] 로 자동 마이그레이션된다(하위 호환).
 *
 * @spec SPEC-WEB-005
 */
export function ChannelSeriesEditor({
  panel,
  onConfigChange,
  fetchChannels = listChartChannels,
}: {
  panel: PanelConfig;
  onConfigChange: OnConfig;
  fetchChannels?: () => Promise<ChartChannelSummary[]>;
}): React.ReactElement {
  const { t } = useTranslation();
  const config = panel.config ?? {};
  const legacyChannelName = (config.channel_name as string | undefined) ?? '';

  // channel_name 만 있고 channels 가 없는 기존 패널 → 자동 마이그레이션.
  const channels: ChannelRefConfig[] = useMemo(() => {
    const raw = config.channels as ChannelRefConfig[] | undefined;
    if (raw && raw.length > 0) return raw;
    if (legacyChannelName) return [{ name: legacyChannelName }];
    return [{ name: '' }];
  }, [config.channels, legacyChannelName]);

  function updateChannels(next: ChannelRefConfig[]): void {
    // channels 로 통합: channel_name 은 제거.
    onConfigChange({ channels: next.length === 0 ? [{ name: '' }] : next, channel_name: undefined });
  }
  function addChannel(): void {
    // 시리즈 인덱스별 팔레트 색을 자동 배정(사용자 변경 가능).
    updateChannels([...channels, { name: '', color: pickSeriesColor(channels.length) }]);
  }
  function removeChannel(idx: number): void {
    if (channels.length <= 1) return;
    updateChannels(channels.filter((_, i) => i !== idx));
  }
  function patchChannel(idx: number, patch: Partial<ChannelRefConfig>): void {
    updateChannels(channels.map((c, i) => (i === idx ? { ...c, ...patch } : c)));
  }
  function moveChannel(from: number, to: number): void {
    if (from === to || from < 0 || to < 0) return;
    if (from >= channels.length || to >= channels.length) return;
    const next = channels.slice();
    const [item] = next.splice(from, 1);
    next.splice(to, 0, item!);
    updateChannels(next);
  }

  // 활성 채널 fetch (행 드롭다운에서 사용).
  const [activeChannels, setActiveChannels] = useState<ChartChannelSummary[]>([]);
  const [channelsLoadState, setChannelsLoadState] = useState<
    'idle' | 'loading' | 'error'
  >('loading');
  useEffect(() => {
    let cancelled = false;
    setChannelsLoadState('loading');
    fetchChannels()
      .then((result) => {
        if (cancelled) return;
        setActiveChannels(result);
        setChannelsLoadState('idle');
      })
      .catch(() => {
        if (cancelled) return;
        setChannelsLoadState('error');
      });
    return () => {
      cancelled = true;
    };
  }, [fetchChannels, panel.id]);
  const activeNameSet = new Set(activeChannels.map((c) => c.name));

  return (
    <div
      className="space-y-2 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) p-2.5"
      data-testid="line-chart-channels-editor"
    >
      <div className="flex items-center justify-between">
        <label className="text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.chart.channels')}
        </label>
        <button
          type="button"
          onClick={addChannel}
          data-testid="line-chart-add-channel"
          className="flex items-center gap-1 rounded px-2 py-0.5 text-xs text-blue-600 hover:bg-blue-50"
        >
          <Plus className="h-3 w-3" /> {t('dashboard.chart.add')}
        </button>
      </div>
      <p className="text-[10px] leading-snug text-(--color-text-muted)">
        {t('dashboard.chart.channelsHint')}
      </p>
      <div className="space-y-2">
        {channels.map((c, idx) => (
          <ChannelRow
            key={idx}
            idx={idx}
            channel={c}
            activeChannels={activeChannels}
            activeNameSet={activeNameSet}
            channelsLoadState={channelsLoadState}
            canDelete={channels.length > 1}
            onPatch={(patch) => patchChannel(idx, patch)}
            onRemove={() => removeChannel(idx)}
            onDragStart={(e) => {
              e.dataTransfer.setData('text/x-channel-idx', String(idx));
              e.dataTransfer.effectAllowed = 'move';
            }}
            onDragOver={(e) => {
              e.preventDefault();
              e.dataTransfer.dropEffect = 'move';
            }}
            onDrop={(e) => {
              e.preventDefault();
              const raw = e.dataTransfer.getData('text/x-channel-idx');
              const from = parseInt(raw, 10);
              if (Number.isNaN(from)) return;
              moveChannel(from, idx);
            }}
          />
        ))}
      </div>
    </div>
  );
}

// --- 3. line-chart 패널 설정 — 전역 스타일만 (채널/시리즈 편집은 데이터 소스 영역) ---

/**
 * 축 폰트(레이블/눈금) 한 줄 편집기. 크기(px)·색상·굵기(보통/굵게)를 조절한다.
 * 미지정 필드는 렌더 기본값(size 10, #9ca3af, normal)으로 폴백하므로, 입력 placeholder
 * 로 기본값을 안내한다.
 */
function AxisFontRow({
  label,
  font,
  onChange,
}: {
  label: string;
  font: AxisFontStyle | undefined;
  onChange: (patch: Partial<AxisFontStyle>) => void;
}): React.ReactElement {
  const { t } = useTranslation();
  const isBold = (font?.weight ?? 'normal') === 'bold';
  return (
    <div className="flex items-center gap-1.5">
      <span className="w-20 shrink-0 truncate text-[11px] text-(--color-text-secondary)">
        {label}
      </span>
      <input
        type="number"
        min={6}
        max={40}
        value={font?.size ?? ''}
        onChange={(e) => {
          const v = e.target.value;
          onChange({ size: v === '' ? undefined : parseInt(v, 10) || undefined });
        }}
        placeholder="10"
        aria-label={`${label} ${t('dashboard.chart.fontSize')}`}
        className="w-14 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1 text-center text-[11px] text-(--color-text-primary) outline-none focus:border-blue-500"
      />
      <input
        type="color"
        value={font?.color ?? '#9ca3af'}
        onChange={(e) => onChange({ color: e.target.value })}
        aria-label={`${label} ${t('dashboard.chart.fontColor')}`}
        className="h-6 w-6 shrink-0 cursor-pointer rounded border border-(--color-border-default) bg-transparent p-0"
      />
      <button
        type="button"
        onClick={() => onChange({ weight: isBold ? 'normal' : 'bold' })}
        aria-pressed={isBold}
        aria-label={`${label} ${t('dashboard.chart.fontBold')}`}
        title={t('dashboard.chart.fontBold')}
        className={`h-6 w-6 shrink-0 rounded border text-[11px] font-bold transition-colors ${
          isBold
            ? 'border-blue-500 bg-blue-500/10 text-blue-500'
            : 'border-(--color-border-default) text-(--color-text-muted) hover:bg-(--color-bg-hover)'
        }`}
      >
        B
      </button>
    </div>
  );
}

export function LineChartSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: OnConfig;
}): React.ReactElement {
  const { t } = useTranslation();
  const config = panel.config ?? {};
  const maxPoints = (config.max_points as number | undefined) ?? 100;
  const xLabel = (config.x_label as string | undefined) ?? '';
  const yMin = config.y_min as number | undefined;
  const yMax = config.y_max as number | undefined;
  const yAxisMode = (config.y_axis_mode as YAxisMode | undefined) ?? 'auto';
  const yPadPct = (config.y_axis_padding_pct as number | undefined) ?? 5;
  const yLabel = (config.y_label as string | undefined) ?? '';
  const yUnit = (config.y_unit as string | undefined) ?? '';
  const yAxisType = (config.y_axis_type as YAxisDataType | undefined) ?? 'numeric';
  const enumLabels = (config.y_enum_labels as YEnumLabel[] | undefined) ?? [];

  // 축 폰트(레이블/눈금) — 축별 독립. patch 병합 후 빈 객체는 undefined 로 정리한다.
  type FontField = 'x_label_font' | 'x_tick_font' | 'y_label_font' | 'y_tick_font';
  function patchFont(field: FontField, patch: Partial<AxisFontStyle>): void {
    const cur = (config[field] as AxisFontStyle | undefined) ?? {};
    const next: AxisFontStyle = { ...cur, ...patch };
    // 값이 모두 비면(undefined) 필드를 제거해 config 를 깔끔히 유지한다.
    const cleaned: AxisFontStyle = {};
    if (next.size !== undefined) cleaned.size = next.size;
    if (next.color !== undefined) cleaned.color = next.color;
    if (next.weight !== undefined) cleaned.weight = next.weight;
    onConfigChange({
      [field]: Object.keys(cleaned).length > 0 ? cleaned : undefined,
    });
  }

  function updateEnumLabels(next: YEnumLabel[]): void {
    onConfigChange({ y_enum_labels: next.length === 0 ? undefined : next });
  }
  function addEnumLabel(): void {
    // 다음 정수 값을 기본값으로 제안(마지막 값 + 1, 없으면 0).
    const nextValue =
      enumLabels.length > 0 ? (enumLabels[enumLabels.length - 1]!.value ?? -1) + 1 : 0;
    updateEnumLabels([...enumLabels, { value: nextValue, label: '' }]);
  }
  function removeEnumLabel(idx: number): void {
    updateEnumLabels(enumLabels.filter((_, i) => i !== idx));
  }
  function patchEnumLabel(idx: number, patch: Partial<YEnumLabel>): void {
    updateEnumLabels(enumLabels.map((e, i) => (i === idx ? { ...e, ...patch } : e)));
  }
  const timeWindowMode =
    (config.time_window_mode as TimeWindowMode | undefined) ?? 'points';
  const recentWindowSec = (config.recent_window_sec as number | undefined) ?? 600;
  const fixedStartMs = config.fixed_start_ms as number | undefined;
  const fixedEndMs = config.fixed_end_ms as number | undefined;
  const refreshMs = (config.time_window_refresh_ms as number | undefined) ?? 1000;
  const multiSeriesField = (config.multi_series_field as string | undefined) ?? '';
  const thresholds = (config.y_thresholds as YThreshold[] | undefined) ?? [];

  function updateThresholds(next: YThreshold[]): void {
    onConfigChange({ y_thresholds: next.length === 0 ? undefined : next });
  }
  function addThreshold(): void {
    updateThresholds([...thresholds, { value: 0, color: '#f59e0b' }]);
  }
  function removeThreshold(idx: number): void {
    updateThresholds(thresholds.filter((_, i) => i !== idx));
  }
  function patchThreshold(idx: number, patch: Partial<YThreshold>): void {
    updateThresholds(thresholds.map((t, i) => (i === idx ? { ...t, ...patch } : t)));
  }

  return (
    <div className="space-y-3">
      {/*
        채널/시리즈 편집은 데이터 소스 영역(ChannelSeriesEditor / SelectedSeriesList)으로
        이전되었다. 본 섹션은 전역 스타일(X/Y축·임계선·범례·다중시리즈)만 다룬다.
      */}

      {/* ═══ 차트 스타일 ═══ */}
      <div className="border-t border-(--color-border-default) pt-3">
        <label className="mb-2 block text-xs font-semibold text-(--color-text-primary)">{t('dashboard.chart.chartStyle')}</label>

        {/* X축 */}
        <div className="flex items-end gap-2">
          <LabeledField label={t('dashboard.chart.xAxis')}>
            <select
              value={timeWindowMode}
              onChange={(e) =>
                onConfigChange({ time_window_mode: e.target.value as TimeWindowMode })
              }
              className={inputClass()}
            >
              <option value="points">{t('dashboard.chart.xWindowPoints')}</option>
              <option value="recent">{t('dashboard.chart.xWindowRecent')}</option>
              <option value="fixed">{t('dashboard.chart.xWindowFixed')}</option>
            </select>
          </LabeledField>
          <LabeledField label={t('dashboard.chart.label')}>
            <input
              type="text"
              value={xLabel}
              onChange={(e) => onConfigChange({ x_label: e.target.value || undefined })}
              placeholder={t('dashboard.chart.xLabelPlaceholder')}
              className={inputClass()}
            />
          </LabeledField>
        </div>

        {timeWindowMode === 'points' && (
          <LabeledField label={t('dashboard.chart.maxPoints')}>
            <input
              type="number"
              min={1}
              max={10000}
              value={maxPoints}
              onChange={(e) => {
                const n = parseInt(e.target.value, 10);
                if (!Number.isNaN(n)) onConfigChange({ max_points: n });
              }}
              className={inputClass()}
            />
          </LabeledField>
        )}

        {timeWindowMode === 'recent' && (
          <div className="flex gap-2">
            <LabeledField label={t('dashboard.chart.windowSizeSec')}>
              <input
                type="number"
                min={1}
                max={86400}
                value={recentWindowSec}
                onChange={(e) => {
                  const n = parseInt(e.target.value, 10);
                  if (!Number.isNaN(n)) onConfigChange({ recent_window_sec: n });
                }}
                className={inputClass()}
              />
            </LabeledField>
            <LabeledField label={t('dashboard.chart.refreshMs')}>
              <input
                type="number"
                min={200}
                max={60000}
                step={100}
                value={refreshMs}
                onChange={(e) => {
                  const n = parseInt(e.target.value, 10);
                  if (!Number.isNaN(n)) onConfigChange({ time_window_refresh_ms: n });
                }}
                className={inputClass()}
              />
            </LabeledField>
          </div>
        )}

        {timeWindowMode === 'fixed' && (
          <div className="flex gap-2">
            <LabeledField label={t('dashboard.chart.startMs')}>
              <input
                type="number"
                value={fixedStartMs ?? ''}
                onChange={(e) => {
                  const v = e.target.value;
                  onConfigChange({
                    fixed_start_ms: v === '' ? undefined : parseInt(v, 10) || undefined,
                  });
                }}
                className={inputClass()}
              />
            </LabeledField>
            <LabeledField label={t('dashboard.chart.endMs')}>
              <input
                type="number"
                value={fixedEndMs ?? ''}
                onChange={(e) => {
                  const v = e.target.value;
                  onConfigChange({
                    fixed_end_ms: v === '' ? undefined : parseInt(v, 10) || undefined,
                  });
                }}
                className={inputClass()}
              />
            </LabeledField>
          </div>
        )}

        {/* Y축 데이터 타입 (숫자형 / 열거형) */}
        <div className="flex items-end gap-2">
          <LabeledField label={t('dashboard.chart.yAxisType')}>
            <select
              value={yAxisType}
              onChange={(e) =>
                onConfigChange({ y_axis_type: e.target.value as YAxisDataType })
              }
              className={inputClass()}
            >
              <option value="numeric">{t('dashboard.chart.yTypeNumeric')}</option>
              <option value="enum">{t('dashboard.chart.yTypeEnum')}</option>
            </select>
          </LabeledField>
          <LabeledField label={t('dashboard.chart.label')}>
            <input
              type="text"
              value={yLabel}
              onChange={(e) => onConfigChange({ y_label: e.target.value || undefined })}
              placeholder={t('dashboard.chart.yLabelPlaceholder')}
              className={inputClass()}
            />
          </LabeledField>
          {yAxisType === 'numeric' && (
            <LabeledField label={t('dashboard.chart.unit')}>
              <input
                type="text"
                value={yUnit}
                onChange={(e) => onConfigChange({ y_unit: e.target.value || undefined })}
                placeholder={t('dashboard.chart.yUnitPlaceholder')}
                className="w-16 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-2 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
              />
            </LabeledField>
          )}
        </div>

        {/* 숫자형: Y축 범위(자동/수동/자동+여백) */}
        {yAxisType === 'numeric' && (
          <LabeledField label={t('dashboard.chart.yAxis')}>
            <select
              value={yAxisMode}
              onChange={(e) => onConfigChange({ y_axis_mode: e.target.value as YAxisMode })}
              className={inputClass()}
            >
              <option value="auto">{t('dashboard.chart.yAuto')}</option>
              <option value="manual">{t('dashboard.chart.yManual')}</option>
              <option value="auto_padded">{t('dashboard.chart.yAutoPadded')}</option>
            </select>
          </LabeledField>
        )}

        {yAxisType === 'numeric' && yAxisMode === 'manual' && (
          <div className="flex gap-2">
            <LabeledField label={t('dashboard.chart.min')}>
              <input
                type="number"
                value={yMin ?? ''}
                onChange={(e) => {
                  const v = e.target.value;
                  onConfigChange({ y_min: v === '' ? undefined : parseFloat(v) || undefined });
                }}
                className={inputClass()}
              />
            </LabeledField>
            <LabeledField label={t('dashboard.chart.max')}>
              <input
                type="number"
                value={yMax ?? ''}
                onChange={(e) => {
                  const v = e.target.value;
                  onConfigChange({ y_max: v === '' ? undefined : parseFloat(v) || undefined });
                }}
                className={inputClass()}
              />
            </LabeledField>
          </div>
        )}

        {yAxisType === 'numeric' && yAxisMode === 'auto_padded' && (
          <LabeledField label={t('dashboard.chart.paddingPct')}>
            <input
              type="number"
              min={0}
              max={50}
              step={0.5}
              value={yPadPct}
              onChange={(e) => {
                const n = parseFloat(e.target.value);
                if (!Number.isNaN(n)) onConfigChange({ y_axis_padding_pct: n });
              }}
              className={inputClass()}
            />
          </LabeledField>
        )}

        {/* 열거형: 값→라벨 매핑 편집기 */}
        {yAxisType === 'enum' && (
          <div className="space-y-1.5 rounded-md border border-(--color-border-default) p-2">
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium text-(--color-text-secondary)">
                {t('dashboard.chart.enumLabels')}
              </span>
              <button
                type="button"
                onClick={addEnumLabel}
                className="flex items-center gap-1 rounded px-1.5 py-0.5 text-xs text-blue-500 hover:bg-blue-500/10"
              >
                <Plus className="h-3 w-3" />
                {t('dashboard.chart.enumAdd')}
              </button>
            </div>
            {enumLabels.length === 0 ? (
              <p className="py-1 text-[11px] text-(--color-text-muted)">
                {t('dashboard.chart.enumEmpty')}
              </p>
            ) : (
              enumLabels.map((row, i) => (
                <div key={i} className="flex items-center gap-1.5">
                  <input
                    type="number"
                    value={Number.isFinite(row.value) ? row.value : ''}
                    onChange={(e) => {
                      const v = e.target.value;
                      patchEnumLabel(i, {
                        value: v === '' ? Number.NaN : parseFloat(v),
                      });
                    }}
                    placeholder={t('dashboard.chart.enumValuePlaceholder')}
                    className="w-16 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1 text-center text-[11px] text-(--color-text-primary) outline-none focus:border-blue-500"
                  />
                  <span className="text-[11px] text-(--color-text-muted)">→</span>
                  <input
                    type="text"
                    value={row.label}
                    onChange={(e) => patchEnumLabel(i, { label: e.target.value })}
                    placeholder={t('dashboard.chart.enumLabelPlaceholder')}
                    className="min-w-0 flex-1 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1 text-[11px] text-(--color-text-primary) outline-none focus:border-blue-500"
                  />
                  <button
                    type="button"
                    onClick={() => removeEnumLabel(i)}
                    aria-label={t('dashboard.chart.enumRemove')}
                    className="shrink-0 rounded p-1 text-(--color-text-muted) hover:bg-red-500/10 hover:text-red-500"
                  >
                    <Trash2 className="h-3 w-3" />
                  </button>
                </div>
              ))
            )}
          </div>
        )}

        {/* 축 폰트 (레이블/값, 축별 독립) */}
        <div className="space-y-1.5 rounded-md border border-(--color-border-default) p-2">
          <div className="flex items-center gap-2 text-[11px] text-(--color-text-muted)">
            <span className="w-20 shrink-0">{t('dashboard.chart.axisFont')}</span>
            <span className="w-14 text-center">{t('dashboard.chart.fontSize')}</span>
            <span className="w-6 text-center">{t('dashboard.chart.fontColorShort')}</span>
            <span className="w-6 text-center">{t('dashboard.chart.fontBoldShort')}</span>
          </div>
          <AxisFontRow
            label={t('dashboard.chart.xAxisLabelFont')}
            font={config.x_label_font as AxisFontStyle | undefined}
            onChange={(p) => patchFont('x_label_font', p)}
          />
          <AxisFontRow
            label={t('dashboard.chart.xAxisTickFont')}
            font={config.x_tick_font as AxisFontStyle | undefined}
            onChange={(p) => patchFont('x_tick_font', p)}
          />
          <AxisFontRow
            label={t('dashboard.chart.yAxisLabelFont')}
            font={config.y_label_font as AxisFontStyle | undefined}
            onChange={(p) => patchFont('y_label_font', p)}
          />
          <AxisFontRow
            label={t('dashboard.chart.yAxisTickFont')}
            font={config.y_tick_font as AxisFontStyle | undefined}
            onChange={(p) => patchFont('y_tick_font', p)}
          />
        </div>

        {/* 범례 */}
        <LabeledField label={t('dashboard.chart.legendPosition')}>
          <select
            value={(config.legend as Record<string, unknown> | undefined)?.position as string ?? 'bottom'}
            onChange={(e) =>
              onConfigChange({
                legend: {
                  ...((config.legend as Record<string, unknown>) ?? {}),
                  position: e.target.value,
                },
              })
            }
            className={inputClass()}
          >
            <option value="bottom">{t('dashboard.chart.legendBottom')}</option>
            <option value="left">{t('dashboard.chart.legendLeft')}</option>
            <option value="right">{t('dashboard.chart.legendRight')}</option>
          </select>
        </LabeledField>
        <div className="flex flex-wrap gap-3 text-xs text-(--color-text-muted)">
          {(['show_name', 'show_line', 'show_last_value'] as const).map((field) => {
            const labelKeys = {
              show_name: 'dashboard.chart.legendShowName',
              show_line: 'dashboard.chart.legendShowLine',
              show_last_value: 'dashboard.chart.legendShowLastValue',
            } as const;
            const defaults = { show_name: true, show_line: true, show_last_value: false };
            return (
              <label key={field} className="flex cursor-pointer items-center gap-1">
                <input
                  type="checkbox"
                  checked={(config.legend as Record<string, unknown> | undefined)?.[field] as boolean ?? defaults[field]}
                  onChange={(e) =>
                    onConfigChange({
                      legend: {
                        ...((config.legend as Record<string, unknown>) ?? {}),
                        [field]: e.target.checked,
                      },
                    })
                  }
                  className="h-3 w-3 rounded border-gray-300"
                />
                {t(labelKeys[field])}
              </label>
            );
          })}
        </div>

        {/* 다중 시리즈 */}
        <LabeledField label={t('dashboard.chart.multiSeriesField')} hint={t('dashboard.chart.multiSeriesHint')}>
          <input
            type="text"
            value={multiSeriesField}
            onChange={(e) => onConfigChange({ multi_series_field: e.target.value || undefined })}
            placeholder={t('dashboard.chart.multiSeriesPlaceholder')}
            className={inputClass()}
          />
        </LabeledField>
      </div>

      {/* ═══ 경계 설정 ═══ */}
      <div data-testid="line-chart-thresholds-editor" className="border-t border-(--color-border-default) pt-3">
        <div className="mb-1.5 flex items-center justify-between">
          <label className="text-xs font-semibold text-(--color-text-primary)">{t('dashboard.chart.boundarySettings')}</label>
          <button
            type="button"
            onClick={addThreshold}
            data-testid="line-chart-add-threshold"
            className="flex items-center gap-1 rounded px-2 py-0.5 text-xs text-blue-600 hover:bg-blue-50"
          >
            <Plus className="h-3 w-3" /> {t('dashboard.chart.add')}
          </button>
        </div>
        {thresholds.length === 0 ? (
          <p className="text-[10px] leading-snug text-(--color-text-muted)">
            {t('dashboard.chart.boundaryEmpty')}
          </p>
        ) : (
          <div className="space-y-2">
            {thresholds.map((th, idx) => (
              <div
                key={idx}
                data-testid={`line-chart-threshold-row-${idx}`}
                className="flex items-center gap-1.5 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) p-1.5"
              >
                <input
                  type="number"
                  value={th.value}
                  onChange={(e) => {
                    const n = parseFloat(e.target.value);
                    if (!Number.isNaN(n)) patchThreshold(idx, { value: n });
                  }}
                  className="w-20 rounded border border-(--color-border-default) bg-(--color-bg-surface) px-2 py-1 text-xs"
                  placeholder={t('dashboard.chart.boundaryValuePlaceholder')}
                  aria-label={t('dashboard.chart.boundaryValueAria')}
                />
                <span
                  className="h-5 w-5 shrink-0 cursor-pointer rounded ring-1 ring-(--color-border-default)"
                  style={{ backgroundColor: th.color }}
                  title={t('dashboard.chart.colorChangeTitle')}
                  onClick={() => {
                    document.getElementById(`th-color-${idx}`)?.click();
                  }}
                />
                <input
                  id={`th-color-${idx}`}
                  type="color"
                  value={th.color}
                  onChange={(e) => patchThreshold(idx, { color: e.target.value })}
                  className="invisible absolute h-0 w-0"
                  tabIndex={-1}
                />
                <select
                  value={th.fill_direction ?? ''}
                  onChange={(e) => {
                    const v = e.target.value;
                    patchThreshold(idx, {
                      fill_direction: v === '' ? undefined : (v as 'below' | 'above'),
                      fill_to: undefined,
                    });
                  }}
                  className="flex-1 rounded border border-(--color-border-default) bg-(--color-bg-surface) px-1.5 py-1 text-xs"
                  aria-label={t('dashboard.chart.fillAria')}
                >
                  <option value="">{t('dashboard.chart.fillNone')}</option>
                  <option value="below">{t('dashboard.chart.fillBelow')}</option>
                  <option value="above">{t('dashboard.chart.fillAbove')}</option>
                </select>
                <button
                  type="button"
                  onClick={() => removeThreshold(idx)}
                  aria-label={t('dashboard.chart.deleteBoundaryAria')}
                  className="flex h-5 w-5 items-center justify-center rounded text-(--color-text-muted) hover:bg-red-50 hover:text-red-600"
                >
                  <Trash2 className="h-3 w-3" />
                </button>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

// --- 4. bar-chart 패널 설정 ---

export function BarChartSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: OnConfig;
}): React.ReactElement {
  const { t } = useTranslation();
  const config = panel.config ?? {};
  const displayField = (config.display_field as string | undefined) ?? 'value';
  const labelField = (config.label_field as string | undefined) ?? 'labels.name';
  const mode = (config.mode as BarChartMode | undefined) ?? 'category';
  const binSec = (config.bin_sec as number | undefined) ?? 60;
  const aggFunc = (config.agg_func as AggFunc | undefined) ?? 'avg';
  const maxPoints = (config.max_points as number | undefined) ?? 20;

  return (
    <div className="space-y-3">
      <LabeledField label={t('dashboard.chart.displayField')}>
        <input
          type="text"
          value={displayField}
          onChange={(e) => onConfigChange({ display_field: e.target.value })}
          className={inputClass()}
        />
      </LabeledField>
      <LabeledField label={t('dashboard.chart.modeField')}>
        <select
          value={mode}
          onChange={(e) => onConfigChange({ mode: e.target.value as BarChartMode })}
          className={inputClass()}
        >
          <option value="category">{t('dashboard.chart.modeCategory')}</option>
          <option value="time_bin">{t('dashboard.chart.modeTimeBin')}</option>
        </select>
      </LabeledField>
      {mode === 'category' && (
        <LabeledField
          label={t('dashboard.chart.labelField')}
          hint={t('dashboard.chart.labelFieldHintBar')}
        >
          <input
            type="text"
            value={labelField}
            onChange={(e) => onConfigChange({ label_field: e.target.value })}
            className={inputClass()}
          />
        </LabeledField>
      )}
      {mode === 'time_bin' && (
        <LabeledField label={t('dashboard.chart.binSec')}>
          <input
            type="number"
            min={1}
            value={binSec}
            onChange={(e) => {
              const n = parseInt(e.target.value, 10);
              if (!Number.isNaN(n)) onConfigChange({ bin_sec: n });
            }}
            className={inputClass()}
          />
        </LabeledField>
      )}
      <LabeledField label={t('dashboard.chart.aggFunc')}>
        <select
          value={aggFunc}
          onChange={(e) => onConfigChange({ agg_func: e.target.value as AggFunc })}
          className={inputClass()}
        >
          <option value="count">count</option>
          <option value="sum">sum</option>
          <option value="avg">avg</option>
        </select>
      </LabeledField>
      <LabeledField label={t('dashboard.chart.maxPoints')}>
        <input
          type="number"
          min={1}
          value={maxPoints}
          onChange={(e) => {
            const n = parseInt(e.target.value, 10);
            if (!Number.isNaN(n)) onConfigChange({ max_points: n });
          }}
          className={inputClass()}
        />
      </LabeledField>
    </div>
  );
}

// --- 5. pie-chart 패널 설정 ---

export function PieChartSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: OnConfig;
}): React.ReactElement {
  const { t } = useTranslation();
  const config = panel.config ?? {};
  const displayField = (config.display_field as string | undefined) ?? 'value';
  const labelField = (config.label_field as string | undefined) ?? 'labels.name';
  const aggFunc = (config.agg_func as AggFunc | undefined) ?? 'sum';
  const showLegend = (config.show_legend as boolean | undefined) ?? true;
  const showPercentage = (config.show_percentage as boolean | undefined) ?? true;
  const maxPoints = (config.max_points as number | undefined) ?? 20;

  return (
    <div className="space-y-3">
      <LabeledField label={t('dashboard.chart.displayField')}>
        <input
          type="text"
          value={displayField}
          onChange={(e) => onConfigChange({ display_field: e.target.value })}
          className={inputClass()}
        />
      </LabeledField>
      <LabeledField
        label={t('dashboard.chart.labelField')}
        hint={t('dashboard.chart.labelFieldHintPie')}
      >
        <input
          type="text"
          value={labelField}
          onChange={(e) => onConfigChange({ label_field: e.target.value })}
          className={inputClass()}
        />
      </LabeledField>
      <LabeledField label={t('dashboard.chart.aggFunc')}>
        <select
          value={aggFunc}
          onChange={(e) => onConfigChange({ agg_func: e.target.value as AggFunc })}
          className={inputClass()}
        >
          <option value="count">count</option>
          <option value="sum">sum</option>
          <option value="avg">avg</option>
        </select>
      </LabeledField>
      <label className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1 transition-colors hover:bg-(--color-bg-elevated)">
        <input
          type="checkbox"
          checked={showLegend}
          onChange={(e) => onConfigChange({ show_legend: e.target.checked })}
          className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
        />
        <span className="text-sm text-(--color-text-primary)">{t('dashboard.chart.showLegend')}</span>
      </label>
      <label className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1 transition-colors hover:bg-(--color-bg-elevated)">
        <input
          type="checkbox"
          checked={showPercentage}
          onChange={(e) => onConfigChange({ show_percentage: e.target.checked })}
          className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
        />
        <span className="text-sm text-(--color-text-primary)">
          {t('dashboard.chart.showPercentage')}
        </span>
      </label>
      <LabeledField label={t('dashboard.chart.maxPoints')}>
        <input
          type="number"
          min={1}
          value={maxPoints}
          onChange={(e) => {
            const n = parseInt(e.target.value, 10);
            if (!Number.isNaN(n)) onConfigChange({ max_points: n });
          }}
          className={inputClass()}
        />
      </LabeledField>
    </div>
  );
}

// --- 6. table 패널 설정 ---

export function TableChartSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: OnConfig;
}): React.ReactElement {
  const { t } = useTranslation();
  const config = panel.config ?? {};
  const columns =
    (config.columns as TableColumn[] | undefined) ?? [
      { field: 'timestamp', header: t('dashboard.chart.colTime'), format: 'datetime' as TableColumnFormat },
      { field: 'value', header: t('dashboard.chart.colValue'), format: 'number' as TableColumnFormat },
    ];
  const rowsPerPage = (config.rows_per_page as number | undefined) ?? 20;
  const maxPoints = (config.max_points as number | undefined) ?? 200;
  const defaultSort =
    (config.default_sort as { field: string; order: SortOrder } | undefined) ?? undefined;

  const updateColumn = (i: number, patch: Partial<TableColumn>): void => {
    onConfigChange({
      columns: columns.map((c, idx) => (idx === i ? { ...c, ...patch } : c)),
    });
  };
  const addColumn = (): void => {
    onConfigChange({ columns: [...columns, { field: 'value', header: t('dashboard.chart.newColumn') }] });
  };
  const removeColumn = (i: number): void => {
    if (columns.length <= 1) return;
    onConfigChange({ columns: columns.filter((_, idx) => idx !== i) });
  };

  return (
    <div className="space-y-3">
      <div>
        <div className="mb-1.5 flex items-center justify-between">
          <label className="text-xs font-medium text-(--color-text-muted)">{t('dashboard.chart.columns')}</label>
          <button
            type="button"
            onClick={addColumn}
            className="flex items-center gap-1 rounded-md px-2 py-0.5 text-xs font-medium text-blue-600 transition-colors hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20"
          >
            <Plus className="h-3 w-3" /> {t('dashboard.chart.add')}
          </button>
        </div>
        <div className="space-y-1.5">
          {columns.map((c, i) => (
            <div key={i} className="flex items-center gap-1">
              <input
                type="text"
                value={c.field}
                onChange={(e) => updateColumn(i, { field: e.target.value })}
                placeholder={t('dashboard.chart.displayFieldPlaceholder')}
                className="flex-1 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
              />
              <input
                type="text"
                value={c.header}
                onChange={(e) => updateColumn(i, { header: e.target.value })}
                placeholder={t('dashboard.chart.headerPlaceholder')}
                className="flex-1 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
              />
              <select
                value={c.format ?? 'string'}
                onChange={(e) =>
                  updateColumn(i, { format: e.target.value as TableColumnFormat })
                }
                className="shrink-0 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1 py-1 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
              >
                <option value="string">string</option>
                <option value="number">number</option>
                <option value="datetime">datetime</option>
              </select>
              <button
                type="button"
                onClick={() => removeColumn(i)}
                disabled={columns.length <= 1}
                className="rounded p-0.5 text-(--color-text-muted) transition-colors hover:text-red-500 disabled:opacity-40"
                aria-label={t('dashboard.chart.deleteColumnAria')}
              >
                <Trash2 className="h-3 w-3" />
              </button>
            </div>
          ))}
        </div>
      </div>
      <LabeledField label={t('dashboard.chart.rowsPerPage')}>
        <input
          type="number"
          min={1}
          value={rowsPerPage}
          onChange={(e) => {
            const n = parseInt(e.target.value, 10);
            if (!Number.isNaN(n)) onConfigChange({ rows_per_page: n });
          }}
          className={inputClass()}
        />
      </LabeledField>
      <LabeledField label={t('dashboard.chart.maxPoints')}>
        <input
          type="number"
          min={1}
          value={maxPoints}
          onChange={(e) => {
            const n = parseInt(e.target.value, 10);
            if (!Number.isNaN(n)) onConfigChange({ max_points: n });
          }}
          className={inputClass()}
        />
      </LabeledField>
      <div>
        <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
          {t('dashboard.chart.defaultSort')}
        </label>
        <div className="flex gap-1.5">
          <input
            type="text"
            value={defaultSort?.field ?? ''}
            onChange={(e) => {
              const field = e.target.value;
              if (!field) {
                onConfigChange({ default_sort: undefined });
              } else {
                onConfigChange({
                  default_sort: { field, order: defaultSort?.order ?? 'asc' },
                });
              }
            }}
            placeholder={t('dashboard.chart.sortFieldPlaceholder')}
            className="flex-1 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-2 py-1 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
          />
          <select
            value={defaultSort?.order ?? 'asc'}
            onChange={(e) => {
              if (!defaultSort?.field) return;
              onConfigChange({
                default_sort: {
                  field: defaultSort.field,
                  order: e.target.value as SortOrder,
                },
              });
            }}
            disabled={!defaultSort?.field}
            className="shrink-0 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1 py-1 text-xs text-(--color-text-primary) outline-none focus:border-blue-500 disabled:opacity-40"
          >
            <option value="asc">asc</option>
            <option value="desc">desc</option>
          </select>
        </div>
      </div>
    </div>
  );
}
