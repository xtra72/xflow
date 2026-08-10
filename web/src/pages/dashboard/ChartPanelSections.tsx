// 차트 패널 타입별 설정 섹션.
// PanelSettingsDialog 에서 사용하는 sub-컴포넌트 모음 (SPEC-CHART-001 §4.2.2 / REQ-M5-03).
//
// 5종 차트 (stat / line-chart / bar-chart / pie-chart / table) 각각의 config
// 편집 UI 를 제공한다. 상위 PanelSettingsDialog 는 panel.type 에 따라
// 분기하여 해당 Section 을 렌더링한다.

import React, { useEffect, useMemo, useRef, useState } from 'react';
import { ChevronDown, ChevronRight, GripVertical, Plus, Trash2 } from 'lucide-react';

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
} from './panels/charts/chartChannelTypes';
import { pickSeriesColor } from './panels/charts/chartChannelTypes';
import {
  filterStoreKeyObjects,
  makeTagFilterId,
} from './panels/charts/storeSourceFilter';
import {
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

/** 기본 Store 소스 설정(처음 store 모드로 전환 시 사용). */
function defaultStoreSource(): StoreSourceConfig {
  return {
    agent_name: '',
    namespace: 'default',
    series: [],
    time_window_ms: 60 * 60 * 1000, // 지난 1시간
    interval_ms: 60 * 1000, // 1분 버킷
    aggregation: 'average',
    refresh_interval_ms: 5000,
  };
}

/**
 * 차트 패널 공통 데이터 소스 섹션.
 *
 * - 데이터 소스 토글(채널 / Store)을 제공한다.
 * - store 선택 시: Store 에이전트 선택 → 키 필터(이름/metric_type/tag/data_type)
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
   * 데이터소스 바인딩 모드(채널/Store/TSDB) 변경 콜백. 이 컴포넌트가 모드의 단일 소스 오브
   * 트루스를 소유하며, 상위(PanelSettingsDataSource)는 Store 모드에서만 선택 테이블을 렌더
   * 하기 위해 이 값을 읽는다. @spec SPEC-PANEL-SETTINGS-001 (데이터소스 토글 단일화)
   */
  onModeChange?: (mode: 'channel' | 'store' | 'tsdb') => void;
}): React.ReactElement {
  const { t } = useTranslation();
  const config = panel.config ?? {};
  const dataSource = (config.data_source as ChartDataSourceKind | undefined) ?? 'channel';
  const storeSource =
    (config.store_source as StoreSourceConfig | undefined) ?? defaultStoreSource();
  // 라인 차트 패널은 per-line 스타일 통합 편집(채널/스토어 시리즈 양쪽)을 노출한다.
  const isLineChart = panel.type === 'line-chart';

  // TSDB 는 실동작 없는 UI 전용 모드다. config 에 기록하지 않아 기존 Store 설정을 파괴하지
  // 않는다(REQ-05/AC-05). 채널/Store 는 기존과 동일하게 config.data_source 로 영속된다.
  const [tsdbMode, setTsdbMode] = useState(false);
  const effectiveMode: 'channel' | 'store' | 'tsdb' = tsdbMode ? 'tsdb' : dataSource;
  useEffect(() => {
    onModeChange?.(effectiveMode);
  }, [effectiveMode, onModeChange]);

  const setDataSource = (kind: ChartDataSourceKind): void => {
    setTsdbMode(false);
    if (kind === 'store' && !config.store_source) {
      // 처음 store 로 전환 시 기본 설정을 함께 채운다.
      onConfigChange({ data_source: 'store', store_source: defaultStoreSource() });
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
        <div
          className="inline-flex rounded-md border border-(--color-border-default) bg-(--color-bg-surface) p-0.5"
          role="tablist"
          aria-label={t('dashboard.chart.dataSourceLabel')}
        >
          {(['channel', 'store', 'tsdb'] as const).map((kind) => {
            const selected = kind === 'tsdb' ? tsdbMode : !tsdbMode && dataSource === kind;
            return (
              <button
                key={kind}
                type="button"
                role="tab"
                aria-selected={selected}
                data-testid={`chart-data-source-${kind}`}
                onClick={() => (kind === 'tsdb' ? setTsdbMode(true) : setDataSource(kind))}
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
      </div>
        {/* Row 1 그룹 B: 에이전트 선택(스토어 모드에서만, 레이블 위). */}
        {!tsdbMode && dataSource === 'store' && (
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

      {/* TSDB: 실동작 없는 후속 SPEC 안내 placeholder (REQ-05/AC-05). Store 설정은 보존된다. */}
      {tsdbMode && (
        <div
          data-testid="chart-data-source-tsdb-placeholder"
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

      {/* Store 소스 상세 (store 선택 시) — 에이전트 셀렉트는 Row 1 로 이동. */}
      {!tsdbMode && dataSource === 'store' && (
        <StoreSourceEditor
          storeSource={storeSource}
          onPatch={patchStore}
          isLineChart={isLineChart}
        />
      )}

      {/*
        채널 모드 + 라인 차트: 채널 시리즈 편집기(채널 추가/선택/순서 + per-line 스타일).
        다른 차트 타입은 채널 모드에서 단일 channel_name 을 ChartChannelSection(우측 컬럼)
        으로 편집하므로 여기서는 렌더하지 않는다.
      */}
      {!tsdbMode && dataSource === 'channel' && isLineChart && (
        <ChannelSeriesEditor
          panel={panel}
          onConfigChange={onConfigChange}
          fetchChannels={fetchChannels}
        />
      )}
    </div>
  );
}

/** Store 소스 상세 편집기(에이전트/키 선택 + 시간/인터벌/집계). */
function StoreSourceEditor({
  storeSource,
  onPatch,
  isLineChart,
}: {
  storeSource: StoreSourceConfig;
  onPatch: (patch: Partial<StoreSourceConfig>) => void;
  isLineChart: boolean;
}): React.ReactElement {
  const { t } = useTranslation();

  // 시리즈 선택 방식. 미지정은 'keys'(기존 동작). 태그 모드 전환은 이제 목록 헤더의
  // 전용 태그 피커(PanelStoreSelectTable)가 tag_filters 존재로 함축한다(별도 keys/tag
  // 토글 제거). @spec SPEC-WEB-005 / SPEC-PANEL-SETTINGS-001 (태그 인 헤더)
  const selectionMode = storeSource.selection_mode ?? 'keys';

  return (
    <div className="space-y-3 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) p-2.5">

      {/*
        키 선택기(체크박스) + 태그 자동 선택기(목록 헤더의 전용 AND 태그 피커)는 공용
        StoreEntryTable 을 소비하는 PanelStoreSelectTable(PanelSettingsDataSource)로 일원화됐다.
        여기서는 선택 결과(series)의 alias/색상/라인 스타일만 편집한다(별도 keys/tag 토글 제거).
        @spec SPEC-PANEL-SETTINGS-001 (시리즈 선택 단일화 + 태그 인 헤더)
      */}

      {/* keys 모드: 선택된 시리즈 목록 — alias/색상/(라인 차트 시) 라인 스타일 편집 (SPEC-WEB-005) */}
      {selectionMode === 'keys' && storeSource.series.length > 0 && (
        <SelectedSeriesList
          series={storeSource.series}
          onChange={(series) => onPatch({ series })}
          isLineChart={isLineChart}
        />
      )}

      {/* 시간 윈도우 / 인터벌 / 집계 / 새로고침 — 한 줄 배치(좁으면 자동 줄바꿈). */}
      <div className="flex flex-wrap gap-2">
        <div className="min-w-[7rem] flex-1">
          <LabeledField
            label={t('dashboard.chart.storeTimeWindowSec')}
            hint={t('dashboard.chart.storeTimeWindowHint')}
          >
            <input
              type="number"
              min={1}
              data-testid="chart-store-time-window"
              value={Math.round(storeSource.time_window_ms / 1000)}
              onChange={(e) => {
                const n = parseInt(e.target.value, 10);
                if (!Number.isNaN(n) && n > 0) onPatch({ time_window_ms: n * 1000 });
              }}
              className={inputClass()}
            />
          </LabeledField>
        </div>
        <div className="min-w-[6rem] flex-1">
          <LabeledField label={t('dashboard.chart.storeIntervalSec')}>
            <input
              type="number"
              min={1}
              data-testid="chart-store-interval"
              value={Math.round(storeSource.interval_ms / 1000)}
              onChange={(e) => {
                const n = parseInt(e.target.value, 10);
                if (!Number.isNaN(n) && n > 0) onPatch({ interval_ms: n * 1000 });
              }}
              className={inputClass()}
            />
          </LabeledField>
        </div>
        <div className="min-w-[7rem] flex-1">
          <LabeledField label={t('dashboard.chart.storeAggregation')}>
            <select
              value={storeSource.aggregation}
              data-testid="chart-store-aggregation"
              onChange={(e) =>
                onPatch({ aggregation: e.target.value as StoreSourceConfig['aggregation'] })
              }
              className={inputClass()}
            >
              {STORE_AGG_OPTIONS.map((opt) => (
                <option key={opt.value} value={opt.value}>
                  {t(opt.labelKey)}
                </option>
              ))}
            </select>
          </LabeledField>
        </div>
        <div className="min-w-[6rem] flex-1">
          <LabeledField label={t('dashboard.chart.storeRefreshSec')}>
            <input
              type="number"
              min={1}
              value={Math.round((storeSource.refresh_interval_ms ?? 5000) / 1000)}
              onChange={(e) => {
                const n = parseInt(e.target.value, 10);
                if (!Number.isNaN(n) && n > 0) onPatch({ refresh_interval_ms: n * 1000 });
              }}
              className={inputClass()}
            />
          </LabeledField>
        </div>
      </div>
    </div>
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

function LineStyleControls({
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
        className="rounded border border-(--color-border-default) bg-(--color-bg-surface) px-1.5 py-1 text-xs"
      >
        <option value="solid">{t('dashboard.chart.lineSolid')}</option>
        <option value="dashed">{t('dashboard.chart.lineDashed')}</option>
        <option value="dotted">{t('dashboard.chart.lineDotted')}</option>
      </select>
      <label className="flex items-center gap-1 text-xs text-(--color-text-muted)">
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
          className="w-12 rounded border border-(--color-border-default) bg-(--color-bg-surface) px-1.5 py-1 text-xs"
        />
      </label>
      <label className="flex cursor-pointer items-center gap-1 text-xs text-(--color-text-muted)">
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
        className="w-28 rounded border border-(--color-border-default) bg-(--color-bg-surface) px-2 py-1 text-xs"
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
function SeriesAliasInput({
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
      className="w-32 shrink-0 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1 text-[11px] text-(--color-text-primary) outline-none focus:border-blue-500"
    />
  );
}

/**
 * 시리즈 alias 태그 토큰 삽입 버튼 + 실시간 미리보기 서브행 (SPEC-WEB-005).
 *
 * - 태그가 있는 시리즈에만 노출된다(채널 시리즈/태그 없는 시리즈는 plain 텍스트 유지).
 * - 각 태그 키마다 `{$.key}` 삽입 버튼을 제공하고, 클릭 시 입력 커서 위치(없으면 끝)에
 *   토큰을 삽입한다.
 * - 미리보기는 resolveSeriesAlias(alias, tags) 결과를 보여준다. alias 가 비어있으면
 *   키명으로 폴백(현재 렌더 동작과 동일).
 */
function SeriesAliasTokens({
  index,
  seriesKey,
  alias,
  tags,
  onAliasChange,
  getInput,
}: {
  index: number;
  seriesKey: string;
  alias: string | undefined;
  tags: Record<string, string>;
  onAliasChange: (alias: string | undefined) => void;
  getInput: () => HTMLInputElement | null;
}): React.ReactElement | null {
  const { t } = useTranslation();
  const tagKeys = Object.keys(tags);
  if (tagKeys.length === 0) return null;

  // 커서 위치(없으면 끝)에 토큰을 삽입한다.
  const insertToken = (tagKey: string): void => {
    const token = makeAliasToken(tagKey);
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
    alias && alias.trim() !== '' ? resolveSeriesAlias(alias, tags) : seriesKey;

  return (
    <div
      className="flex flex-wrap items-center gap-1 px-1.5 pb-1"
      data-testid={`chart-store-series-tokens-${index}`}
    >
      <span className="text-[9px] text-(--color-text-muted)">
        {t('dashboard.chart.storeAliasInsertToken')}
      </span>
      {tagKeys.map((k) => (
        <button
          key={k}
          type="button"
          onClick={() => insertToken(k)}
          data-testid={`chart-store-series-token-${index}-${k}`}
          className="rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1 py-0.5 font-mono text-[9px] text-blue-600 transition-colors hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/30"
        >
          {makeAliasToken(k)}
        </button>
      ))}
      {/* 미리보기: 라벨(i18n) + 해석값(raw). 값은 별도 노드로 두어 항상 확인 가능. */}
      <span
        className="ml-1 inline-flex min-w-0 items-center gap-0.5 text-[9px] text-(--color-text-muted)"
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
function SelectedSeriesList({
  series,
  onChange,
  isLineChart,
}: {
  series: StoreSeriesRef[];
  onChange: (series: StoreSeriesRef[]) => void;
  isLineChart: boolean;
}): React.ReactElement {
  const { t } = useTranslation();
  // 펼친 행 인덱스(라인 스타일 편집용).
  const [expanded, setExpanded] = useState<Set<number>>(() => new Set());
  // 행별 alias 입력 DOM 참조(토큰 삽입 시 커서 위치 사용).
  const aliasInputRefs = useRef<Record<number, HTMLInputElement | null>>({});

  // 인덱스 i 의 시리즈에 patch 를 적용한다(불변 갱신).
  const patchSeries = (i: number, patch: Partial<StoreSeriesRef>): void => {
    onChange(series.map((s, idx) => (idx === i ? { ...s, ...patch } : s)));
  };

  const removeSeries = (i: number): void => {
    onChange(series.filter((_, idx) => idx !== i));
  };

  const toggleExpand = (i: number): void => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(i)) next.delete(i);
      else next.add(i);
      return next;
    });
  };

  return (
    <div className="space-y-1.5" data-testid="chart-store-selected-series">
      <label className="block text-[10px] font-medium text-(--color-text-muted)">
        {t('dashboard.chart.storeSelectedSeries')}
      </label>
      {series.map((s, i) => {
        const tagStr = Object.entries(s.tags ?? {})
          .map(([k, v]) => `${k}=${v}`)
          .join(', ');
        const isExpanded = expanded.has(i);
        return (
          <div
            key={`${s.key}-${s.metric_type ?? ''}-${i}`}
            data-testid={`chart-store-series-row-${i}`}
            className="rounded border border-(--color-border-default) bg-(--color-bg-surface)"
          >
            <div className="flex items-center gap-1.5 px-1.5 py-1">
              {/* 라인 차트: 펼침 토글 */}
              {isLineChart && (
                <button
                  type="button"
                  onClick={() => toggleExpand(i)}
                  data-testid={`chart-store-series-expand-${i}`}
                  aria-label={
                    isExpanded
                      ? t('dashboard.chart.collapseAria')
                      : t('dashboard.chart.expandAria')
                  }
                  className="flex items-center text-(--color-text-muted)"
                >
                  {isExpanded ? (
                    <ChevronDown className="h-3 w-3" />
                  ) : (
                    <ChevronRight className="h-3 w-3" />
                  )}
                </button>
              )}
              {/* 키 + (metric/tags) 식별 표시 */}
              <span className="flex min-w-0 flex-1 flex-col">
                <span className="truncate font-mono text-[11px] text-(--color-text-primary)">
                  {s.key}
                </span>
                {(s.metric_type || tagStr) && (
                  <span className="truncate text-[9px] text-(--color-text-muted)">
                    {[s.metric_type, tagStr].filter(Boolean).join(' · ')}
                  </span>
                )}
              </span>
              {/*
                표시 이름(alias) 입력 + 태그 토큰 삽입/미리보기 (SPEC-WEB-005).
                빈 값은 undefined 로 저장(키명 폴백). `{$.tagKey}` 토큰 지원.
                입력 필드만 인라인에 두고, 토큰 버튼/미리보기는 아래 서브행에 렌더한다.
              */}
              <SeriesAliasInput
                index={i}
                seriesKey={s.key}
                alias={s.alias}
                onAliasChange={(alias) => patchSeries(i, { alias })}
                inputRef={(el) => {
                  aliasInputRefs.current[i] = el;
                }}
              />
              {/* 색상 선택(선택) */}
              <input
                type="color"
                value={s.color ?? pickSeriesColor(i)}
                onChange={(e) => patchSeries(i, { color: e.target.value })}
                aria-label={t('dashboard.chart.storeSeriesColorAria').replace('{key}', s.key)}
                data-testid={`chart-store-series-color-${i}`}
                className="h-5 w-5 shrink-0 cursor-pointer rounded border border-(--color-border-default) bg-transparent p-0"
              />
              {/* 제거 */}
              <button
                type="button"
                onClick={() => removeSeries(i)}
                aria-label={t('dashboard.chart.storeSeriesRemoveAria').replace('{key}', s.key)}
                data-testid={`chart-store-series-remove-${i}`}
                className="flex h-5 w-5 shrink-0 items-center justify-center rounded text-(--color-text-muted) transition-colors hover:text-red-500"
              >
                <Trash2 className="h-3 w-3" />
              </button>
            </div>

            {/*
              태그 토큰 삽입 버튼 + 미리보기 서브행 — 태그가 있는 시리즈에만 노출.
              토큰 클릭 시 입력 커서 위치(없으면 끝)에 `{$.tagKey}` 를 삽입한다.
            */}
            <SeriesAliasTokens
              index={i}
              seriesKey={s.key}
              alias={s.alias}
              tags={s.tags ?? {}}
              onAliasChange={(alias) => patchSeries(i, { alias })}
              getInput={() => aliasInputRefs.current[i] ?? null}
            />
            {/* 라인 차트: 펼침 시 통합 라인 스타일 편집 */}
            {isLineChart && isExpanded && (
              <div className="border-t border-(--color-border-default) px-1.5 py-1.5">
                <LineStyleControls
                  value={s}
                  onPatch={(patch) => patchSeries(i, patch)}
                  testIdPrefix={`chart-store-series-${i}`}
                />
              </div>
            )}
          </div>
        );
      })}
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
