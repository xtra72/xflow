// 차트 패널 타입별 설정 섹션.
// PanelSettingsDialog 에서 사용하는 sub-컴포넌트 모음 (SPEC-CHART-001 §4.2.2 / REQ-M5-03).
//
// 5종 차트 (stat / line-chart / bar-chart / pie-chart / table) 각각의 config
// 편집 UI 를 제공한다. 상위 PanelSettingsDialog 는 panel.type 에 따라
// 분기하여 해당 Section 을 렌더링한다.

import React, { useEffect, useMemo, useRef, useState } from 'react';
import { ChevronDown, ChevronRight, ChevronUp, GripVertical, Info, Plus, Trash2 } from 'lucide-react';

import type { PanelConfig } from '@/stores/uiStore';
import { listChartChannels, type ChartChannelSummary } from '@/services/api/charts';
import { useAgents } from '@/hooks/useAgent';
import { useStoreKeysWithTags, useStoreTagPairs } from '@/services/api/store';
import { useTranslation } from '@/lib/i18n';

import type {
  TableColumn,
  TableColumnFormat,
  TableRowMode,
  BarChartMode,
  AggFunc,
  SortOrder,
  YAxisMode,
  YAxisDataType,
  YEnumLabel,
  AxisFontStyle,
  TooltipConfig,
  YThreshold,
  ChannelRefConfig,
  StrokeStyle,
  ChartDataSourceKind,
  StoreSeriesRef,
  StoreSourceConfig,
  SeriesReduceFunc,
  PieLegendPosition,
} from './panels/charts/chartChannelTypes';
import { SeriesRangeField } from './SeriesRangeField';
import {
  readChartXRange,
  readSeriesRange,
  type ChartXRangeSource,
} from './panels/charts/seriesRange';
import {
  INTERVAL_PRESETS_MS,
  formatIntervalMs,
  isIntervalPreset,
} from './panels/charts/intervalPresets';
import {
  buildDefaultStoreSource,
  defaultSysmetricsSource,
  defaultTsdbSource,
  DEFAULT_PIE_LEGEND_FONT_SIZE,
  pickSeriesColor,
  REDUCE_PANEL_TYPES,
} from './panels/charts/chartChannelTypes';
import {
  DEFAULT_TILE_ROWS,
  MAX_TILE_ROWS,
  normalizeTileRows,
} from './panels/charts/multiOutputLimit';
import { SERIES_REDUCE_FUNCS } from './panels/charts/seriesReduce';
import {
  DEFAULT_DECIMAL_PLACES,
  MAX_DECIMAL_PLACES,
} from './panels/charts/decimalPlaces';
import { DEFAULT_PIE_LABEL_MIN_PERCENT } from './panels/charts/pieLabel';
import { FONT_FAMILY_OPTIONS, type ChartFontFamily } from './panels/charts/textStyle';
import { PANEL_SIZE_MAX, PANEL_SIZE_MIN } from './panels/charts/panelGeometry';
import {
  readValueScale,
  VALUE_SCALE_MAX,
  VALUE_SCALE_MIN,
} from './panels/charts/valueScale';
import {
  CUSTOM_UNIT_SENTINEL,
  isPresetUnit,
  UNIT_OPTIONS,
} from './panels/charts/unitOptions';
import {
  panelTagKeys,
  tagFieldPath,
  tagKeyOfField,
} from './panels/charts/panelTagKeys';
import {
  CAPABILITY_REASON_KEYS,
  panelSourceCapabilities,
  resolvePanelSourceBinding,
} from './panels/charts/panelDataSource';
import { isPanelSeriesSource } from './panels/charts/usePanelSeriesData';
import { FillStrategyField, TsdbSourceSection } from './TsdbSourceSection';
import { FillPreviousLimitField } from './FillPreviousLimitField';
import { SysmetricsSourceSection } from './SysmetricsSourceSection';
import {
  GRAPH_STYLES,
  hasGapDash,
  hasStrokeStyle,
  isStackable,
  readGraphStyle,
  requiresBuckets,
  type GraphStyle,
} from './panels/charts/graphStyle';
import { AliasTokenHelp } from './AliasTokenHelp';
import { SeriesNameFormatField } from './SeriesNameFormatField';
import {
  filterStoreKeyObjects,
  makeTagFilterId,
} from './panels/charts/storeSourceFilter';
import {
  availableAliasTokens,
  makeAliasToken,
  resolveSeriesAlias,
} from './panels/charts/aliasTemplate';

/**
 * 데이터 소스 종류 → 토글 버튼 라벨 i18n 키.
 *
 * `Record<ChartDataSourceKind, string>` 로 두어 **컴파일러가 전수성을 강제**하게 한다 —
 * 삼항 사슬로 두면 종류가 늘 때 마지막 가지가 조용히 새 종류를 삼킨다(TSDB 라벨이
 * sysmetrics 버튼에 붙는 식).
 */
const DATA_SOURCE_LABEL_KEYS: Record<ChartDataSourceKind, string> = {
  channel: 'dashboard.chart.dataSourceChannel',
  store: 'dashboard.chart.dataSourceStore',
  tsdb: 'dashboard.chart.dataSourceTsdb',
  sysmetrics: 'dashboard.chart.dataSourceSysmetrics',
};

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

/**
 * 값 표기 소수점 자릿수 입력 — 차트 계열 패널이 공유한다.
 *
 * 비우면 `decimal_places` 를 지워 기본값({@link DEFAULT_DECIMAL_PLACES})으로 되돌린다.
 * "지움" 과 "0 으로 지정" 은 다르다 — 후자는 정수 표기를 고정한다.
 *
 * 자리마다 입력칸을 다시 만들지 않는 이유: 종전에는 통계와 라인만 각자 만들어 두었고
 * 두 칸의 동작이 이미 달랐다(통계는 비울 수 없고, 라인은 placeholder 가 "자동").
 */
export function DecimalPlacesField({
  config,
  onConfigChange,
  testId,
}: {
  config: Record<string, unknown>;
  onConfigChange: OnConfig;
  testId: string;
}): React.ReactElement {
  const { t } = useTranslation();
  const raw = config.decimal_places;
  const value = typeof raw === 'number' && Number.isFinite(raw) ? String(raw) : '';
  return (
    <LabeledField
      label={t('dashboard.chart.decimalPlaces')}
      hint={t('dashboard.chart.decimalPlacesHint')}
    >
      <input
        type="number"
        min={0}
        max={MAX_DECIMAL_PLACES}
        value={value}
        placeholder={String(DEFAULT_DECIMAL_PLACES)}
        onChange={(e) => {
          const v = e.target.value;
          if (v === '') return onConfigChange({ decimal_places: undefined });
          const n = parseInt(v, 10);
          if (!Number.isNaN(n) && n >= 0) onConfigChange({ decimal_places: n });
        }}
        data-testid={testId}
        className={inputClass()}
      />
    </LabeledField>
  );
}

/**
 * 값 단위 선택 — 목록에서 고르거나 직접 적는다.
 *
 * 종전에는 이 UI 가 게이지 설정 안에만 있었고, 통계·라인은 자유 텍스트 한 칸이었다.
 * 같은 온도 시리즈가 패널마다 `°C` · `C` · `degC` 로 갈리는 이유가 그것이다. 목록을
 * 공유하면 대시보드 안에서 표기가 저절로 맞는다.
 *
 * **"직접 입력" 은 모드다.** 종전 게이지는 목록 옆에 좁은 텍스트 칸을 늘 띄워 두고
 * 목록의 "직접 입력" 항목은 고르면 아무 일도 하지 않았다(`return`). 무엇을 눌러야
 * 커스텀 단위를 넣을 수 있는지 화면에 드러나지 않았다. 여기서는 "직접 입력" 을 고르면
 * 입력칸이 나타나고 포커스가 간다 — 고른 것이 곧 일어난다.
 */
export function UnitField({
  value,
  onChange,
  testId,
  label,
}: {
  value: string;
  onChange: (unit: string) => void;
  testId: string;
  /** 라벨 문구 키. 미지정이면 "단위". */
  label?: string;
}): React.ReactElement {
  const { t } = useTranslation();
  return (
    <LabeledField label={t(label ?? 'dashboard.chart.unit')}>
      <UnitControl value={value} onChange={onChange} testId={testId} />
    </LabeledField>
  );
}

/**
 * 현재값 글자 **크기 배율**.
 *
 * 패널마다 기본 크기가 다르므로(통계 본값 36 · 타일 24) 절대 크기가 아니라 배율로 둔다 —
 * 배율이면 타일이 하나일 때와 여럿일 때 모두 같은 뜻으로 걸린다. 게이지와 **같은 config
 * 키**(`value_scale`)라 패널 유형을 바꿔도 "조금 크게" 가 유지된다.
 */
export function ValueScaleField({
  config,
  onConfigChange,
  testId,
}: {
  config: Record<string, unknown>;
  onConfigChange: OnConfig;
  testId: string;
}): React.ReactElement {
  const { t } = useTranslation();
  const scale = readValueScale(config.value_scale);
  return (
    <LabeledField label={t('dashboard.chart.valueScale')}>
      <div className="flex items-center gap-2">
        <input
          type="range"
          min={VALUE_SCALE_MIN}
          max={VALUE_SCALE_MAX}
          step={0.05}
          value={scale}
          onChange={(e) => onConfigChange({ value_scale: Number(e.target.value) })}
          data-testid={testId}
          className="flex-1"
        />
        <span className="w-10 shrink-0 text-right text-xs tabular-nums text-(--color-text-muted)">
          {scale.toFixed(2)}
        </span>
        <button
          type="button"
          onClick={() => onConfigChange({ value_scale: undefined })}
          data-testid={`${testId}-reset`}
          className="shrink-0 rounded-md bg-(--color-bg-elevated) px-2 py-1 text-xs text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)/80"
        >
          {t('dashboard.chart.valueScaleReset')}
        </button>
      </div>
    </LabeledField>
  );
}

/**
 * 라벨 없는 단위 컨트롤. 표의 열 편집기처럼 이미 좁은 행 안에 들어가는 자리에서 쓴다.
 *
 * `compact` 는 글자·여백만 줄인다 — 동작(직접 입력 모드 전환)은 두 크기가 같다.
 */
export function UnitControl({
  value,
  onChange,
  testId,
  compact = false,
}: {
  value: string;
  onChange: (unit: string) => void;
  testId: string;
  compact?: boolean;
}): React.ReactElement {
  const { t } = useTranslation();
  // 목록에 없는 값으로 열렸다면 이미 커스텀이다 — 사용자가 적어 둔 값을 목록으로
  // 되돌려 놓으면 안 된다.
  const [custom, setCustom] = useState(() => !isPresetUnit(value));
  const [draft, setDraft] = useState(value);
  const inputRef = useRef<HTMLInputElement>(null);

  // 외부에서 config 가 바뀌면(다른 패널을 열거나 되돌리기) 초안을 맞춘다.
  useEffect(() => {
    setDraft(value);
    if (isPresetUnit(value)) setCustom(false);
  }, [value]);

  const smallBox =
    'rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1 py-1 text-xs text-(--color-text-primary) outline-none focus:border-blue-500';

  return (
    <div className={compact ? 'flex items-center gap-1' : 'flex gap-2'}>
      <select
        value={custom ? CUSTOM_UNIT_SENTINEL : value}
        onChange={(e) => {
          const v = e.target.value;
          if (v === CUSTOM_UNIT_SENTINEL) {
            setCustom(true);
            // 입력칸이 나타난 뒤에 포커스를 준다.
            requestAnimationFrame(() => inputRef.current?.focus());
            return;
          }
          setCustom(false);
          setDraft(v);
          if (v !== value) onChange(v);
        }}
        aria-label={t('dashboard.chart.unit')}
        data-testid={testId}
        className={
          compact
            ? `w-24 shrink-0 ${smallBox}`
            : custom
              ? 'w-28 shrink-0 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-2 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-500'
              : inputClass()
        }
      >
        {UNIT_OPTIONS.map((group) => (
          <optgroup key={group.labelKey} label={t(group.labelKey)}>
            {group.units.map((u) => (
              <option key={u.value} value={u.value}>
                {u.labelKey ? t(u.labelKey) : u.value}
              </option>
            ))}
          </optgroup>
        ))}
        <option value={CUSTOM_UNIT_SENTINEL}>{t('dashboard.settings.gaugeSection.custom')}</option>
      </select>
      {custom && (
        <input
          ref={inputRef}
          type="text"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onBlur={() => {
            if (draft !== value) onChange(draft);
          }}
          onKeyDown={(e) => {
            if (e.key === 'Enter') (e.target as HTMLInputElement).blur();
          }}
          data-testid={`${testId}-custom`}
          placeholder={t('dashboard.settings.gaugeSection.customInput')}
          className={compact ? `w-16 ${smallBox}` : inputClass()}
        />
      )}
    </div>
  );
}


function inputClass(): string {
  return 'w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500';
}

/**
 * 다중 출력 타일 배열의 **행 수** 설정.
 *
 * 시리즈가 2개 이상일 때만 의미가 있으므로 통계·게이지 설정에 함께 둔다. 배열을 쓰지 않는
 * 바·파이는 이 컨트롤을 노출하지 않는다 — 두 패널은 시리즈를 한 차트 안의 막대·조각으로
 * 그리므로 "행" 이라는 개념이 없다.
 *
 * 행 수는 상한이 아니라 **목표**다(`tileColumnCount`). 패널이 좁아 타일 최소 폭을 확보하지
 * 못하면 열이 줄고 행이 목표보다 늘어난다 — 힌트 문구가 그 사실을 알린다.
 */
export function TileRowsField({
  value,
  onChange,
}: {
  value: number | undefined;
  onChange: (rows: number | undefined) => void;
}): React.ReactElement {
  const { t } = useTranslation();
  return (
    <LabeledField
      label={t('dashboard.chart.tileRows')}
      hint={t('dashboard.chart.tileRowsHint')}
    >
      <input
        type="number"
        data-testid="chart-tile-rows"
        min={DEFAULT_TILE_ROWS}
        max={MAX_TILE_ROWS}
        value={normalizeTileRows(value)}
        onChange={(e) => {
          const n = parseInt(e.target.value, 10);
          // 빈 입력·비수치는 기본값으로 되돌린다. 키를 지워 두면 config 가 깔끔하고,
          // 읽는 쪽(`normalizeTileRows`)이 같은 기본값을 쓴다.
          if (Number.isNaN(n)) {
            onChange(undefined);
            return;
          }
          const clamped = normalizeTileRows(n);
          onChange(clamped === DEFAULT_TILE_ROWS ? undefined : clamped);
        }}
        className={inputClass()}
      />
    </LabeledField>
  );
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

/**
 * 결측 구간 점선 표기의 기본 임계(연속 결측 개수). @spec SPEC-TSDB-004 §2.19
 *
 * 1 이 아니라 2 인 이유: 표본 하나가 빠지는 것은 흔한 잡음이라 매번 점선이 되면
 * 신호가 되지 못한다. 2 부터가 "구간" 으로 읽힌다.
 */
const GAP_DASH_DEFAULT = 2;

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
 * 형상 전체가 `buildDefaultStoreSource()` 단일 정본이다 — 게이지 레거시 이관
 * (`buildGaugeStoreMigrationPatch`)이 spec §2.8 [E2] 1항에 따라 **같은 값**을 써야 하고,
 * 신규 통계/게이지/바/파이 패널의 기본 config(`uiStore.createDefaultPanel`)도 같은 값으로
 * 시작해야 한다. 복제해 두면 한쪽만 바뀔 때 조용히 어긋난다.
 *
 * 이 이름을 남겨 두는 이유: spec 과 `gaugeLegacyBinding.ts` 주석이 기본값을
 * "`defaultStoreSource()`" 로 지목하고 있어, 참조 대상을 없애면 그 문장들이 가리키는
 * 곳이 사라진다.
 */
function defaultStoreSource(): StoreSourceConfig {
  return buildDefaultStoreSource();
}

/**
 * 차트 패널 공통 데이터 소스 섹션.
 *
 * ## 설정 순서 (세 소스 공통 · 정본)
 *
 * Store · TSDB · 시스템 지표가 **같은 순서**로 늘어선다. 소스를 갈아탄 사용자가 같은
 * 설정을 같은 자리에서 찾게 하려는 것이며, 순서가 갈라지면 소스 수만큼 화면을 다시
 * 익혀야 한다. 새 소스를 붙일 때도 이 순서를 따른다.
 *
 *   1. 에이전트 선택
 *   2. 소스 고유 축      TSDB: bucket · 드릴다운 · 그룹 기준 / 시스템 지표: 이력 없음 고지
 *   3. 조회 창           범위 → 인터벌 → 집계 → 빈 구간 처리 (없는 축은 건너뛴다)
 *   4. 구간 대표값       해당 패널(stat/gauge/bar/pie)에서만
 *   5. 시리즈 이름 형식
 *   6. 시리즈 표         Store 는 `PanelSettingsDataSource` 가 이 섹션 **뒤에** 렌더한다
 *   7. 선택 요약
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
  onModeChange?: (mode: ChartDataSourceKind) => void;
}): React.ReactElement {
  const { t } = useTranslation();
  const config = panel.config ?? {};
  const storeSource =
    (config.store_source as StoreSourceConfig | undefined) ?? defaultStoreSource();
  // 라인 차트 패널은 per-line 스타일 통합 편집(채널/스토어 시리즈 양쪽)을 노출한다.
  const isLineChart = panel.type === 'graph-chart';

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
    } else if (kind === 'sysmetrics' && !config.sysmetrics_source) {
      // store/tsdb 와 같은 규칙 — 처음 전환할 때만 기본 블록을 함께 채운다.
      onConfigChange({
        data_source: 'sysmetrics',
        sysmetrics_source: defaultSysmetricsSource(),
      });
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
          {(['channel', 'store', 'tsdb', 'sysmetrics'] as const).map((kind) => {
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
                {t(DATA_SOURCE_LABEL_KEYS[kind])}
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
      </div>

      {/* Row 2: 에이전트 선택 — TSDB 쪽과 같은 레이아웃으로 **한 줄 아래**에 둔다.
          토글과 같은 행에 두면 소스를 바꿀 때 셀렉트가 나타났다 사라지며 행 높이가
          출렁인다. 두 소스가 같은 자리에 같은 모양으로 있는 편이 읽기 쉽다. */}
      {isStoreMode && (
        <div className="flex flex-wrap items-start gap-3">
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
        </div>
      )}

      {/* Store 모드: 가져올 데이터 범위 — 기간(상대·절대) 또는 갯수. TSDB 와 같은 편집기다. */}
      {isStoreMode && (
        <SeriesRangeField
          range={readSeriesRange(storeSource.range, storeSource.time_window_ms)}
          onChange={(range) => patchStore({ range })}
          testIdPrefix="chart-store"
        />
      )}

      {/* Store 모드: 인터벌(버킷) 간격. TSDB 와 같은 눈금·같은 조작이다 — 소스를 바꿔도
          같은 값을 같은 방식으로 고른다. */}
      {isStoreMode && (
        <StoreIntervalField
          intervalMs={storeSource.interval_ms}
          timeWindowMs={storeSource.time_window_ms}
          onChange={(interval_ms) => patchStore({ interval_ms })}
        />
      )}

      {/* Store 모드: 인터벌 집계 — 버킷 하나를 대표하는 값을 무엇으로 삼을지.
          종전에는 이 값이 config 에만 있고 조작 통로가 없어 `average` 로 고정이었다.
          TSDB 모드의 같은 컨트롤과 같은 어휘·같은 자리를 쓴다. */}
      {isStoreMode && (
        <LabeledField label={t('dashboard.chart.tsdbAggregation')}>
          <select
            data-testid="chart-store-aggregation"
            value={storeSource.aggregation}
            onChange={(e) =>
              patchStore({ aggregation: e.target.value as StoreSourceConfig['aggregation'] })
            }
            className={inputClass()}
          >
            {STORE_AGG_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>
                {t(o.labelKey)}
              </option>
            ))}
          </select>
        </LabeledField>
      )}

      {/*
        Store 모드: 빈 버킷 처리 전략. 서버(`/store/{agent}/query`)가 집계 결과의 빈
        버킷을 채운다 — 직전값 사용 기간 제한 판단은 TSDB 와 같은 정본(fillpolicy)을
        쓰므로 소스를 갈아타도 같은 설정이 같은 그림을 낸다. `avg` 만 비활성이며,
        선택지를 지우지 않고 남기는 이유는 §2.13 [S1] 이다.
      */}
      {isStoreMode && (
        <FillStrategyField
          value={storeSource.fill ?? ''}
          onChange={(fill) => patchStore({ fill: fill === '' ? undefined : fill })}
          supported={panelSourceCapabilities('store').fillStrategies}
          avgSupported={panelSourceCapabilities('store').fillAvg}
          reasonKey={CAPABILITY_REASON_KEYS.fillAvg}
          testId="chart-store-fill"
        />
      )}
      {/* 사용 기간 제한은 `직전값 사용` 에서만 뜻이 있다. */}
      {isStoreMode && storeSource.fill === 'previous' && (
        <FillPreviousLimitField
          value={{
            maxMs: storeSource.fill_previous_max_ms,
            overflow: storeSource.fill_previous_overflow,
            overflowValue: storeSource.fill_previous_overflow_value,
          }}
          onChange={(next) =>
            patchStore({
              fill_previous_max_ms: next.maxMs,
              fill_previous_overflow: next.overflow,
              fill_previous_overflow_value: next.overflowValue,
            })
          }
          testIdPrefix="chart-store-fill-prev"
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


      {/* Store 모드: 이름을 지정하지 않은 시리즈의 표시 이름 형식(패널 단위 기본값). */}
      {isStoreMode && (
        <SeriesNameFormatField
          value={storeSource.series_name_format}
          onChange={(series_name_format) => patchStore({ series_name_format })}
          sample={storeSource.series?.[0]}
        />
      )}

      {/* TSDB 모드: 에이전트 · bucket · measurement→field→tag 드릴다운 선택 UI(§2.11 ~ §2.15). */}
      {mode === 'tsdb' && (
        <TsdbSourceSection panel={panel} onConfigChange={onConfigChange} />
      )}

      {/* sysmetrics 모드: 에이전트 · 값 · 대상 선택 UI(이력 없는 라이브 누적 소스). */}
      {mode === 'sysmetrics' && (
        <SysmetricsSourceSection panel={panel} onConfigChange={onConfigChange} />
      )}

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
 * Store 인터벌(버킷) 간격 편집기.
 *
 * sysmetrics 소스도 같은 컴포넌트를 쓴다 — 두 소스가 인터벌을 다른 눈금·다른 조작으로
 * 고르면 소스를 갈아탄 사용자가 같은 설정을 다시 배운다.
 *
 * 지금까지 Store 소스의 인터벌은 정보 팝오버에 **읽기 전용**으로만 있었다 — 기본값
 * 1분 버킷을 바꿀 방법이 화면에 없었다. TSDB 쪽과 같은 프리셋·같은 표기·같은 "직접
 * 입력" 경로를 쓴다(`intervalPresets`).
 *
 * 0 이하는 저장하지 않는다 — `useStoreChartData` 의 pollKey 가 비어 폴링이 멈춘다.
 */
export function StoreIntervalField({
  intervalMs,
  timeWindowMs,
  onChange,
  testIdPrefix = 'chart-store',
}: {
  intervalMs: number;
  timeWindowMs: number;
  onChange: (intervalMs: number) => void;
  /**
   * testid 접두사. 소스별로 갈라야 같은 화면에 두 소스 절이 있어도 질의가 겹치지 않는다.
   * `SeriesRangeField` 와 같은 규약이다.
   */
  testIdPrefix?: string;
}): React.ReactElement {
  const { t } = useTranslation();
  const preset = isIntervalPreset(intervalMs);
  // 인터벌이 시간 윈도우보다 크면 버킷이 하나뿐이라 그래프가 점 하나로 보인다.
  // 막지는 않는다(의도적으로 그렇게 쓸 수 있다) — 왜 그렇게 보이는지만 알려준다.
  const tooCoarse = intervalMs > 0 && timeWindowMs > 0 && intervalMs > timeWindowMs;

  return (
    <div className="space-y-1">
      <LabeledField label={t('dashboard.chart.storeInterval')}>
        <select
          data-testid={`${testIdPrefix}-interval`}
          value={preset ? String(intervalMs) : 'custom'}
          onChange={(e) => {
            const v = e.target.value;
            // "직접 입력" 선택만으로는 값을 바꾸지 않는다 — 아래 입력칸이 열릴 뿐이다.
            if (v === 'custom') return;
            onChange(Number(v));
          }}
          className={inputClass()}
        >
          {INTERVAL_PRESETS_MS.map((ms) => (
            <option key={ms} value={ms}>
              {formatIntervalMs(ms)}
            </option>
          ))}
          <option value="custom">{t('tsdb.intervalCustom')}</option>
        </select>
      </LabeledField>
      {!preset && (
        <label className="flex items-center gap-1 text-[11px] text-(--color-text-muted)">
          <input
            type="number"
            min={1}
            data-testid={`${testIdPrefix}-interval-custom`}
            value={Math.round(intervalMs / 1000)}
            aria-label={t('dashboard.chart.storeIntervalCustomAria')}
            onChange={(e) => {
              const sec = Number(e.target.value);
              if (!Number.isFinite(sec) || sec <= 0) return;
              onChange(Math.round(sec) * 1000);
            }}
            className="w-20 rounded border border-(--color-border-default) bg-(--color-bg-surface) px-1 py-0.5 text-[11px]"
          />
          <span>{t('dashboard.chart.storeInfoSecUnit')}</span>
        </label>
      )}
      {tooCoarse && (
        <p
          data-testid={`${testIdPrefix}-interval-warning`}
          className="text-[11px] leading-snug text-amber-600 dark:text-amber-400"
        >
          {t('dashboard.chart.storeIntervalTooCoarse')}
        </p>
      )}
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
  /** 이 시리즈의 모양. 비우면 패널 기본값을 따른다(`'line'` 고정과 다르다). */
  graph_style?: GraphStyle;
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
      {/* 이 시리즈의 모양. 비우면 패널 기본값을 따른다 — 패널을 바꾸면 같이 바뀐다. */}
      <select
        value={value.graph_style ?? ''}
        onChange={(e) =>
          onPatch({ graph_style: (e.target.value || undefined) as GraphStyle | undefined })
        }
        aria-label={t('dashboard.chart.graphStyleSeries')}
        data-testid={`${testIdPrefix}-graph-style`}
        className="rounded border border-(--color-border-default) bg-(--color-bg-surface) px-2 py-1.5 text-sm"
      >
        <option value="">{t('dashboard.chart.graphStyleFollowPanel')}</option>
        {GRAPH_STYLES.map((g) => (
          <option key={g} value={g}>
            {t(`dashboard.chart.graphStyle_${g}`)}
          </option>
        ))}
      </select>
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
 * 시리즈 이름 토큰 도움말 — 공용 `AliasTokenHelp` 에 삽입 동작을 붙인 얇은 껍데기.
 *
 * 팝오버의 생김새·여닫힘은 `AliasTokenHelp` 소관이고, 여기서는 "누르면 이름 입력의
 * 커서 위치에 넣고 커서를 토큰 끝으로 옮긴다"만 정한다. 이름 형식 입력(SeriesNameFormatField)
 * 은 커서 복원이 필요 없어 같은 팝오버에 다른 삽입 동작을 붙인다.
 */
export function SeriesAliasTokenHelp({
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
  const tokenPaths = availableAliasTokens({ measurement: seriesKey, field: fieldName, tags });

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
      const caret = (el.selectionStart ?? current.length) + token.length;
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

  return (
    <AliasTokenHelp
      tokenPaths={tokenPaths}
      onInsert={insertToken}
      testIdPrefix={`chart-store-series-${index}`}
    />
  );
}

/**
 * 시리즈 표시 이름 실시간 미리보기 서브행 (SPEC-WEB-005).
 *
 * 토큰 목록과 달리 상시 노출한다 — 토큰을 넣은 결과가 무엇인지는 편집 중 계속
 * 봐야 하고, 클릭 뒤에 숨기면 "형식이 먹었는지" 확인할 길이 없어진다.
 * alias 가 비어있으면 키명으로 폴백한다(현재 렌더 동작과 동일).
 */
export function SeriesAliasPreview({
  index,
  seriesKey,
  fieldName,
  alias,
  tags,
}: {
  index: number;
  seriesKey: string;
  fieldName?: string;
  alias: string | undefined;
  tags: Record<string, string>;
}): React.ReactElement {
  const { t } = useTranslation();
  const preview =
    alias && alias.trim() !== ''
      ? resolveSeriesAlias(alias, { measurement: seriesKey, field: fieldName, tags })
      : seriesKey;

  return (
    <span
      className="inline-flex min-w-0 max-w-full items-center gap-0.5 text-xs text-(--color-text-muted)"
      data-testid={`chart-store-series-preview-${index}`}
    >
      <span>{t('dashboard.chart.storeAliasPreview')}</span>
      <span className="truncate font-mono text-(--color-text-primary)">{preview}</span>
    </span>
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
  // 서브라인 없음(테이블 컬럼 + alias 컬럼 키 에코와 중복). 본문 폰트는 text-sm 이상.
  //
  // 레이아웃: 이름·미리보기·색상·(heatmap)좌표·(라인)선 스타일을 **한 줄**에 늘어놓고,
  // 폭이 모자라면 그룹 단위로 다음 줄로 접는다(flex-wrap). 항목마다 줄을 하나씩 쓰면
  // 시리즈 한 개가 네 줄을 차지해, 시리즈가 몇 개만 늘어도 목록을 스크롤해야 했다.
  //
  // 각 그룹(라벨+컨트롤)은 내부에서 wrap 하지 않는다 — 라벨만 윗줄에 남고 컨트롤이
  // 아랫줄로 떨어지면 무엇의 라벨인지 읽히지 않는다. 줄바꿈은 항상 그룹 경계에서만
  // 일어난다. 선 스타일만은 예외로 내부 wrap 을 허용한다(컨트롤 4개가 한 덩어리라
  // 좁은 폭에서 통째로 밀면 오히려 빈 줄이 생긴다).
  return (
    <div
      className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm"
      data-testid={`series-detail-${index}`}
    >
      {/* 이름(name/alias, 편집 가능 텍스트) + 토큰 도움말(물음표). */}
      <div className="flex items-center gap-2">
        <label className="shrink-0 text-sm font-medium text-(--color-text-muted)">
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
        {/* 이름 템플릿 토큰(키 / field / 태그) — 물음표 뒤 도움말. */}
        <SeriesAliasTokenHelp
          index={index}
          seriesKey={series.key}
          fieldName={series.field}
          alias={series.alias}
          tags={series.tags ?? {}}
          onAliasChange={(alias) => onPatch({ alias })}
          getInput={() => aliasInputRef.current}
        />
      </div>
      {/* 표시 이름 미리보기(상시 노출). */}
      <SeriesAliasPreview
        index={index}
        seriesKey={series.key}
        fieldName={series.field}
        alias={series.alias}
        tags={series.tags ?? {}}
      />
      {/* 색상 — 모든 패널 타입에서 편집 가능(`color` 불변). */}
      <div className="flex items-center gap-2">
        <label className="shrink-0 text-sm font-medium text-(--color-text-muted)">
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
      {positionEditor}
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
      <UnitField
        value={unit}
        onChange={(v) => onConfigChange({ unit: v })}
        testId="stat-unit"
      />
      <DecimalPlacesField
        config={config}
        onConfigChange={onConfigChange}
        testId="stat-decimal-places"
      />
      <ValueScaleField
        config={config}
        onConfigChange={onConfigChange}
        testId="stat-value-scale"
      />
      <TileRowsField
        value={config.tile_rows as number | undefined}
        onChange={(tile_rows) => onConfigChange({ tile_rows })}
      />
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
/**
 * 설정 한 덩어리 — 제목 줄 + 본문.
 *
 * 종전에는 "차트 스타일" 한 덩어리 안에 X축·Y축·라인·범례가 모두 들어 있어,
 * 무엇이 어느 축의 설정인지 줄 순서로만 구분됐다. 목업대로 축별·주제별로 쪼갠다.
 *
 * `design` 은 제목 오른쪽에 붙는 배지다 — 색·크기처럼 "보이는 방식"만 모아 두어
 * 본문에는 값 설정만 남긴다.
 */
function SettingsSection({
  title,
  design,
  children,
}: {
  title: string;
  design?: React.ReactNode;
  children: React.ReactNode;
}): React.ReactElement {
  return (
    <div className="space-y-2 border-t border-(--color-border-default) pt-3">
      <div className="flex items-center gap-1.5">
        <span className="text-xs font-semibold text-(--color-text-primary)">{title}</span>
        {design}
      </div>
      {children}
    </div>
  );
}

/**
 * 축 디자인 배지 — 레이블/값 글꼴(크기·색·굵기)과 Y축의 자동 여백을 접어 둔다.
 *
 * 글꼴 네 줄(X레이블·X값·Y레이블·Y값)이 본문에 펼쳐져 있으면, 정작 자주 고치는
 * 레이블·범위보다 자리를 많이 차지한다. 축마다 배지 하나로 접고, 그 축의 것만 담는다.
 *
 * `onPadPct` 가 오면 자동 여백 칸을 함께 낸다(Y축 전용). 여백은 최소·최대가 비어
 * 있을 때 축을 데이터 범위보다 얼마나 넓게 잡을지를 정한다 — 값이 없으면 쓰지 않는다.
 */
function AxisDesignPopover({
  testId,
  labelFont,
  tickFont,
  onLabelFont,
  onTickFont,
  padPct,
  onPadPct,
}: {
  testId: string;
  labelFont: AxisFontStyle | undefined;
  tickFont: AxisFontStyle | undefined;
  onLabelFont: (patch: Partial<AxisFontStyle>) => void;
  onTickFont: (patch: Partial<AxisFontStyle>) => void;
  padPct?: number;
  onPadPct?: (next: number | undefined) => void;
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

  return (
    <span className="relative inline-flex" ref={containerRef}>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-haspopup="true"
        aria-expanded={open}
        data-testid={`${testId}-button`}
        className="rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-0.5 text-[11px] text-(--color-text-muted) transition-colors hover:text-(--color-text-primary)"
      >
        {t('dashboard.chart.designBadge')}
      </button>
      {open && (
        <div
          data-testid={`${testId}-popover`}
          className="absolute left-0 top-full z-30 mt-1 w-72 space-y-1.5 rounded-md border border-(--color-border-default) bg-(--color-bg-primary) p-2.5 text-left shadow-lg"
        >
          <div className="flex items-center gap-2 text-[11px] text-(--color-text-muted)">
            <span className="w-20 shrink-0">{t('dashboard.chart.axisFont')}</span>
            <span className="w-14 text-center">{t('dashboard.chart.fontSize')}</span>
            <span className="w-6 text-center">{t('dashboard.chart.fontColorShort')}</span>
            <span className="w-6 text-center">{t('dashboard.chart.fontBoldShort')}</span>
          </div>
          <AxisFontRow
            label={t('dashboard.chart.designLabelRow')}
            font={labelFont}
            onChange={onLabelFont}
          />
          <AxisFontRow
            label={t('dashboard.chart.designValueRow')}
            font={tickFont}
            onChange={onTickFont}
          />
          {onPadPct && (
            <label className="flex items-center gap-2 pt-1 text-[11px] text-(--color-text-muted)">
              <span className="w-20 shrink-0">{t('dashboard.chart.yAutoPadded')}</span>
              <input
                type="number"
                min={0}
                max={50}
                value={padPct ?? ''}
                placeholder={t('dashboard.chart.notUsedWhenEmpty')}
                onChange={(e) => {
                  const v = e.target.value;
                  if (v === '') return onPadPct(undefined);
                  const n = parseInt(v, 10);
                  if (!Number.isNaN(n) && n >= 0) onPadPct(n);
                }}
                data-testid={`${testId}-pad-pct`}
                className="w-14 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1 text-center text-[11px] text-(--color-text-primary) outline-none focus:border-blue-500"
              />
              <span>%</span>
            </label>
          )}
        </div>
      )}
    </span>
  );
}

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
  const gapDashThreshold = (config.gap_dash_threshold as number | undefined) ?? 0;
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
  // X축 범위 — 데이터 소스와 같은 어휘(SeriesRange). 구 time_window_mode 계열은
  // readChartXRange 안에서 폴백으로 해석되므로 여기서는 새 어휘만 다룬다.
  const xRange = readChartXRange(config as ChartXRangeSource);
  const refreshMs = (config.time_window_refresh_ms as number | undefined) ?? 1000;
  const tooltipCfg = (config.tooltip as TooltipConfig | undefined) ?? {};
  const panelSmooth = (config.smooth as boolean | undefined) ?? false;
  const panelGraphStyle = readGraphStyle(config.graph_style);
  const panelStacked = config.stacked === true;
  // 스타일의 옵션 줄을 낼지 — 셋 다 뜻이 없으면(캔들) 빈 줄만 남아 간격이 어긋난다.
  const styleOptionsVisible =
    isStackable(panelGraphStyle) || hasStrokeStyle(panelGraphStyle) || hasGapDash(panelGraphStyle);
  // 소스가 채널이 아니면 X축 범위는 데이터 소스 설정이 정한다 — 조회 범위가 곧
  // 표시 범위다. 같은 값을 두 곳에서 편집하게 두면 서로 어긋난다.
  const xRangeOwnedBySource = isPanelSeriesSource(resolvePanelSourceBinding(config));

  /**
   * Y축 도메인 방식은 저장 필드로 남아 있지만(렌더러가 읽는다), 화면에는 노출하지
   * 않는다 — 목업의 규약은 "최소·최대가 비면 자동"이다. 사용자가 최소/최대나 자동
   * 여백을 건드릴 때만 이 함수로 방식을 다시 계산해 함께 저장한다. 건드리지 않은
   * 패널의 저장값은 그대로 두므로 기존 대시보드의 축이 변하지 않는다.
   */
  function deriveYAxisMode(
    min: number | undefined,
    max: number | undefined,
    padPct: number,
  ): YAxisMode {
    if (min !== undefined || max !== undefined) return 'manual';
    return padPct > 0 ? 'auto_padded' : 'auto';
  }
  // 저장된 방식이 auto_padded 일 때만 여백이 살아 있다.
  const padActive = yAxisMode === 'auto_padded';
  const effectivePadPct = padActive ? yPadPct : 0;

  function setYBound(patch: { y_min?: number; y_max?: number }): void {
    const nextMin = 'y_min' in patch ? patch.y_min : yMin;
    const nextMax = 'y_max' in patch ? patch.y_max : yMax;
    onConfigChange({
      ...patch,
      y_axis_mode: deriveYAxisMode(nextMin, nextMax, effectivePadPct),
    });
  }
  function setYPadPct(next: number | undefined): void {
    onConfigChange({
      y_axis_padding_pct: next,
      y_axis_mode: deriveYAxisMode(yMin, yMax, next ?? 0),
    });
  }

  function patchLegend(patch: Record<string, unknown>): void {
    onConfigChange({
      legend: { ...((config.legend as Record<string, unknown>) ?? {}), ...patch },
    });
  }
  function patchTooltip(patch: Partial<TooltipConfig>): void {
    const next = { ...tooltipCfg, ...patch };
    // 기본값(켬 + 전체)으로 되돌아오면 필드를 지운다 — config 를 깔끔히 유지한다.
    const cleaned: TooltipConfig = {};
    if (next.enabled === false) cleaned.enabled = false;
    if (next.single === true) cleaned.single = true;
    onConfigChange({ tooltip: Object.keys(cleaned).length > 0 ? cleaned : undefined });
  }
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

      {/* ═══ X 축 ═══ */}
      <SettingsSection
        title={t('dashboard.chart.xAxis')}
        design={
          <AxisDesignPopover
            testId="line-chart-x-design"
            labelFont={config.x_label_font as AxisFontStyle | undefined}
            tickFont={config.x_tick_font as AxisFontStyle | undefined}
            onLabelFont={(p) => patchFont('x_label_font', p)}
            onTickFont={(p) => patchFont('x_tick_font', p)}
          />
        }
      >
        <LabeledField label={t('dashboard.chart.label')}>
          <input
            type="text"
            value={xLabel}
            onChange={(e) => onConfigChange({ x_label: e.target.value || undefined })}
            placeholder={t('dashboard.chart.xLabelPlaceholder')}
            data-testid="line-chart-x-label"
            className={inputClass()}
          />
        </LabeledField>

        {/* 범위 — 채널 모드에서만 편집한다. 시리즈 소스는 조회 범위가 곧 표시 범위라
            데이터 소스 설정이 소유한다(같은 값을 두 곳에서 고치면 어긋난다). */}
        {xRangeOwnedBySource ? (
          <p
            data-testid="line-chart-x-range-owned-note"
            className="text-[11px] text-(--color-text-muted)"
          >
            {t('dashboard.chart.xRangeFromSource')}
          </p>
        ) : (
          <>
            <SeriesRangeField
              range={xRange}
              onChange={(next) => onConfigChange({ x_range: next })}
              testIdPrefix="line-chart-x"
            />
            {xRange.mode === 'relative' && (
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
            )}
          </>
        )}
      </SettingsSection>

      {/* ═══ Y 축 ═══ */}
      <SettingsSection
        title={t('dashboard.chart.yAxis')}
        design={
          <AxisDesignPopover
            testId="line-chart-y-design"
            labelFont={config.y_label_font as AxisFontStyle | undefined}
            tickFont={config.y_tick_font as AxisFontStyle | undefined}
            onLabelFont={(p) => patchFont('y_label_font', p)}
            onTickFont={(p) => patchFont('y_tick_font', p)}
            padPct={padActive ? yPadPct : undefined}
            onPadPct={setYPadPct}
          />
        }
      >
        <LabeledField label={t('dashboard.chart.label')}>
          <input
            type="text"
            value={yLabel}
            onChange={(e) => onConfigChange({ y_label: e.target.value || undefined })}
            placeholder={t('dashboard.chart.yLabelPlaceholder')}
            data-testid="line-chart-y-label"
            className={inputClass()}
          />
        </LabeledField>

        {/* 형식 | 소수점 이하(숫자형에서만) */}
        <div className="flex flex-wrap items-end gap-2">
          <LabeledField label={t('dashboard.chart.yAxisType')}>
            <select
              value={yAxisType}
              onChange={(e) => onConfigChange({ y_axis_type: e.target.value as YAxisDataType })}
              data-testid="line-chart-y-type"
              className={inputClass()}
            >
              <option value="numeric">{t('dashboard.chart.yTypeNumeric')}</option>
              <option value="enum">{t('dashboard.chart.yTypeEnum')}</option>
            </select>
          </LabeledField>
          {yAxisType === 'numeric' && (
            <DecimalPlacesField
              config={config}
              onConfigChange={onConfigChange}
              testId="line-chart-decimal-places"
            />
          )}
        </div>

        {/* 최소값 | 최대값 — 비우면 자동. 방식(y_axis_mode)은 여기서 파생 저장한다. */}
        {yAxisType === 'numeric' && (
          <div className="flex flex-wrap items-end gap-2">
            <LabeledField label={t('dashboard.chart.yMin')}>
              <input
                type="number"
                value={yMin ?? ''}
                placeholder={t('dashboard.chart.autoWhenEmpty')}
                onChange={(e) => {
                  const v = e.target.value;
                  setYBound({ y_min: v === '' ? undefined : parseFloat(v) });
                }}
                data-testid="line-chart-y-min"
                className={inputClass()}
              />
            </LabeledField>
            <LabeledField label={t('dashboard.chart.yMax')}>
              <input
                type="number"
                value={yMax ?? ''}
                placeholder={t('dashboard.chart.autoWhenEmpty')}
                onChange={(e) => {
                  const v = e.target.value;
                  setYBound({ y_max: v === '' ? undefined : parseFloat(v) });
                }}
                data-testid="line-chart-y-max"
                className={inputClass()}
              />
            </LabeledField>
          </div>
        )}

        {/* 열거형 값→라벨 매핑 */}
        {yAxisType === 'enum' && (
          <div className="space-y-1.5">
            <div className="flex items-center justify-between">
              <span className="text-xs font-medium text-(--color-text-secondary)">
                {t('dashboard.chart.enumLabels')}
              </span>
              <button
                type="button"
                onClick={addEnumLabel}
                className="flex items-center gap-1 rounded-md px-2 py-0.5 text-xs font-medium text-blue-600 transition-colors hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20"
              >
                <Plus className="h-3 w-3" />
                {t('dashboard.chart.enumAdd')}
              </button>
            </div>
            {enumLabels.length === 0 ? (
              <p className="text-[11px] text-(--color-text-muted)">
                {t('dashboard.chart.enumEmpty')}
              </p>
            ) : (
              enumLabels.map((row, i) => (
                <div key={i} className="flex items-center gap-1.5">
                  <input
                    type="number"
                    value={Number.isNaN(row.value) ? '' : row.value}
                    onChange={(e) => {
                      const v = e.target.value;
                      patchEnumLabel(i, { value: v === '' ? Number.NaN : parseFloat(v) });
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

        <UnitField
          value={yUnit}
          onChange={(v) => onConfigChange({ y_unit: v || undefined })}
          testId="line-chart-y-unit"
        />
      </SettingsSection>

      {/* ═══ 그래프 스타일 ═══ */}
      <SettingsSection title={t('dashboard.chart.lineStyleSection')}>
        {/* 패널 기본 모양. 시리즈 세부 설정에서 개별로 덮어쓸 수 있다. */}
        <div className="flex flex-wrap items-end gap-2">
          <LabeledField label={t('dashboard.chart.graphStyle')}>
            <select
              value={panelGraphStyle}
              onChange={(e) => onConfigChange({ graph_style: e.target.value as GraphStyle })}
              data-testid="line-chart-graph-style"
              className={inputClass()}
            >
              {GRAPH_STYLES.map((g) => (
                <option
                  key={g}
                  value={g}
                  // 캔들은 버킷마다 시·고·저·종이 필요해 집계 소스에서만 그릴 수 있다.
                  // 숨기지 않고 비활성으로 두어 "왜 없지" 대신 "왜 못 쓰지" 를 답한다.
                  disabled={requiresBuckets(g) && !xRangeOwnedBySource}
                >
                  {t(`dashboard.chart.graphStyle_${g}`)}
                </option>
              ))}
            </select>
          </LabeledField>
          {requiresBuckets(panelGraphStyle) && !xRangeOwnedBySource && (
            <p
              data-testid="line-chart-candle-unsupported"
              className="w-full text-[11px] text-amber-500"
            >
              {t('dashboard.chart.candleNeedsBuckets')}
            </p>
          )}
        </div>

        {/* 스타일의 옵션들 — 셋 다 "그 스타일에서만 뜻이 있는" 같은 성격이라 한 줄에 모은다.
            스택킹만 스타일 옆에 두고 나머지를 아래에 두면, 스타일을 바꿀 때 체크박스가
            두 자리에서 따로 나타나고 사라져 무엇이 무엇에 딸린 설정인지 읽히지 않는다.

            판정은 `graphStyle.ts` 의 함수를 그대로 부른다 — 여기서 다시 적으면 렌더러와
            설정 화면이 다른 답을 내어 "설정은 켰는데 그림은 안 바뀐다" 가 된다. */}
        {styleOptionsVisible && (
          <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
            {/* 스택킹은 영역·바에서만 뜻이 있다 — 라인은 쌓아도 겹친 선이고,
                캔들은 네 값이 한 덩어리라 쌓을 수 없다. */}
            {isStackable(panelGraphStyle) && (
              <label className="flex cursor-pointer items-center gap-1.5 text-xs text-(--color-text-muted)">
                <input
                  type="checkbox"
                  checked={panelStacked}
                  onChange={(e) => onConfigChange({ stacked: e.target.checked || undefined })}
                  data-testid="line-chart-stacked"
                />
                <span>{t('dashboard.chart.stacked')}</span>
              </label>
            )}

            {/* 곡선은 선이 있어야 뜻이 있다 — 바·캔들에는 구부릴 선이 없다. */}
            {hasStrokeStyle(panelGraphStyle) && (
              <label className="flex cursor-pointer items-center gap-1.5 text-xs text-(--color-text-muted)">
                <input
                  type="checkbox"
                  checked={panelSmooth}
                  onChange={(e) => onConfigChange({ smooth: e.target.checked || undefined })}
                  data-testid="line-chart-smooth"
                />
                <span>{t('dashboard.chart.curve')}</span>
              </label>
            )}

            {/* 결측 구간 점선 표기 (SPEC-TSDB-004 §2.19).
                켜면 값이 없는 구간에서 실선을 끊고 그 구간만 점선으로 잇는다 —
                이은 것과 잰 것을 눈으로 가른다. 끄면 종전대로 조용히 이어 그린다.
                바·캔들은 값이 없으면 막대가 서지 않아 결측이 이미 눈에 보인다. */}
            {hasGapDash(panelGraphStyle) && (
              <>
                <label className="flex cursor-pointer items-center gap-1.5 text-xs text-(--color-text-muted)">
                  <input
                    type="checkbox"
                    data-testid="line-chart-gap-dash"
                    checked={gapDashThreshold > 0}
                    onChange={(e) =>
                      // 끌 때 0 을 남기지 않는다 — "켜져 있는데 임계 0" 처럼 읽힌다.
                      onConfigChange({
                        gap_dash_threshold: e.target.checked ? GAP_DASH_DEFAULT : undefined,
                      })
                    }
                  />
                  <span>{t('dashboard.chart.gapDash')}</span>
                </label>
                {/* 결측 개수는 켰을 때만 낸다 — 꺼진 상태의 임계값은 읽을 뜻이 없다. */}
                {gapDashThreshold > 0 && (
                  <input
                    type="number"
                    min={1}
                    max={10000}
                    data-testid="line-chart-gap-dash-threshold"
                    aria-label={t('dashboard.chart.gapDashThreshold')}
                    value={gapDashThreshold}
                    onChange={(e) => {
                      const n = parseInt(e.target.value, 10);
                      // 1 미만은 "끄기" 와 같은 뜻인데 토글은 켜져 있다 — 모순된
                      // 상태를 만들지 않으려면 끄기는 토글로만 한다.
                      if (!Number.isNaN(n) && n >= 1) onConfigChange({ gap_dash_threshold: n });
                    }}
                    className="w-20 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-2 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-500"
                  />
                )}
              </>
            )}
          </div>
        )}
        {/* 숫자의 뜻을 화면에 남긴다. 이 문구가 없으면 "2" 가 무엇의 2인지 알 수
            없어, 한 칸짜리 결측이 안 걸리는 이유를 짐작할 길이 없다. */}
        {hasGapDash(panelGraphStyle) && gapDashThreshold > 0 && (
          <p
            data-testid="line-chart-gap-dash-hint"
            className="text-[11px] leading-snug text-(--color-text-muted)"
          >
            {t('dashboard.chart.gapDashHint')}
          </p>
        )}
      </SettingsSection>

      {/* ═══ 범례 ═══ */}
      <SettingsSection title={t('dashboard.chart.legendSection')}>
        <div className="space-y-1">
          <span className="block text-xs font-medium text-(--color-text-muted)">
            {t('dashboard.chart.legendComposition')}
          </span>
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
                    checked={
                      ((config.legend as Record<string, unknown> | undefined)?.[
                        field
                      ] as boolean) ?? defaults[field]
                    }
                    onChange={(e) => patchLegend({ [field]: e.target.checked })}
                    data-testid={`line-chart-legend-${field}`}
                    className="h-3 w-3 rounded border-gray-300"
                  />
                  {t(labelKeys[field])}
                </label>
              );
            })}
          </div>
        </div>
        <LabeledField label={t('dashboard.chart.legendPosition')}>
          <select
            value={
              ((config.legend as Record<string, unknown> | undefined)?.position as string) ??
              'bottom'
            }
            onChange={(e) => patchLegend({ position: e.target.value })}
            data-testid="line-chart-legend-position"
            className={inputClass()}
          >
            <option value="bottom">{t('dashboard.chart.legendBottom')}</option>
            <option value="left">{t('dashboard.chart.legendLeft')}</option>
            <option value="right">{t('dashboard.chart.legendRight')}</option>
          </select>
        </LabeledField>
      </SettingsSection>

      {/* ═══ 툴팁 ═══ */}
      <SettingsSection title={t('dashboard.chart.tooltipSection')}>
        <div className="flex flex-wrap gap-3 text-xs text-(--color-text-muted)">
          <label className="flex cursor-pointer items-center gap-1">
            <input
              type="checkbox"
              checked={tooltipCfg.enabled !== false}
              onChange={(e) => patchTooltip({ enabled: e.target.checked })}
              data-testid="line-chart-tooltip-enabled"
              className="h-3 w-3 rounded border-gray-300"
            />
            {t('dashboard.chart.tooltipEnabled')}
          </label>
          {/* 단일 값은 툴팁이 켜져 있을 때만 뜻이 있다. */}
          {tooltipCfg.enabled !== false && (
            <label className="flex cursor-pointer items-center gap-1">
              <input
                type="checkbox"
                checked={tooltipCfg.single === true}
                onChange={(e) => patchTooltip({ single: e.target.checked })}
                data-testid="line-chart-tooltip-single"
                className="h-3 w-3 rounded border-gray-300"
              />
              {t('dashboard.chart.tooltipSingle')}
            </label>
          )}
        </div>
      </SettingsSection>

      {/* ═══ 다중 시리즈 ═══ */}
      <SettingsSection title={t('dashboard.chart.multiSeriesSection')}>
        <LabeledField
          label={t('dashboard.chart.multiSeriesField')}
          hint={t('dashboard.chart.multiSeriesHint')}
        >
          <input
            type="text"
            value={multiSeriesField}
            onChange={(e) => onConfigChange({ multi_series_field: e.target.value || undefined })}
            placeholder={t('dashboard.chart.multiSeriesPlaceholder')}
            className={inputClass()}
          />
        </LabeledField>
      </SettingsSection>

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
  const unit = (config.unit as string | undefined) ?? '';

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
      <UnitField
        value={unit}
        onChange={(v) => onConfigChange({ unit: v || undefined })}
        testId="bar-chart-unit"
      />
      <DecimalPlacesField
        config={config}
        onConfigChange={onConfigChange}
        testId="bar-chart-decimal-places"
      />
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

/**
 * 글자 스타일 한 줄 — 글꼴 · 크기 · 색.
 *
 * 셋을 한 줄에 묶는 이유는 축 폰트 행(`AxisFontRow`)과 같다. 따로 놓으면 같은 대상의
 * 설정이 세 칸 떨어져 어느 것이 무엇에 걸리는지 화면에서 읽히지 않는다.
 *
 * 세 값 모두 **비우면 상속**이다. 색만 비우는 수단이 따로 필요한 이유는 색 입력에
 * "없음" 상태가 없기 때문이다 — 지정한 뒤에만 나타나는 초기화 버튼이 그 출구다.
 */
function TextStyleFields({
  label,
  family,
  size,
  color,
  sizePlaceholder,
  testIdPrefix,
  onChange,
}: {
  label: string;
  family: ChartFontFamily | undefined;
  size: number | undefined;
  color: string | undefined;
  sizePlaceholder: string;
  testIdPrefix: string;
  onChange: (patch: {
    family?: ChartFontFamily | undefined;
    size?: number | undefined;
    color?: string | undefined;
  }) => void;
}): React.ReactElement {
  const { t } = useTranslation();
  return (
    <LabeledField label={label}>
      <div className="flex items-center gap-1.5">
        <select
          value={family ?? ''}
          data-testid={`${testIdPrefix}-family`}
          aria-label={`${label} ${t('dashboard.chart.fontFamily')}`}
          onChange={(e) =>
            onChange({ family: (e.target.value || undefined) as ChartFontFamily | undefined })
          }
          className={`${inputClass()} flex-1`}
        >
          <option value="">{t('dashboard.chart.inherit')}</option>
          {FONT_FAMILY_OPTIONS.map((o) => (
            <option key={o.value} value={o.value}>
              {t(o.labelKey)}
            </option>
          ))}
        </select>
        <input
          type="number"
          min={6}
          max={40}
          value={size ?? ''}
          placeholder={sizePlaceholder}
          data-testid={`${testIdPrefix}-size`}
          aria-label={`${label} ${t('dashboard.chart.fontSize')}`}
          onChange={(e) => {
            const v = e.target.value;
            onChange({ size: v === '' ? undefined : parseInt(v, 10) || undefined });
          }}
          className="w-16 shrink-0 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1 text-center text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
        />
        <input
          type="color"
          value={color ?? '#9ca3af'}
          data-testid={`${testIdPrefix}-color`}
          aria-label={`${label} ${t('dashboard.chart.fontColor')}`}
          onChange={(e) => onChange({ color: e.target.value })}
          className="h-7 w-7 shrink-0 cursor-pointer rounded border border-(--color-border-default) bg-transparent p-0"
        />
        {color !== undefined && (
          <button
            type="button"
            data-testid={`${testIdPrefix}-color-reset`}
            aria-label={`${label} ${t('dashboard.chart.fontColorReset')}`}
            title={t('dashboard.chart.fontColorReset')}
            onClick={() => onChange({ color: undefined })}
            className="h-7 w-7 shrink-0 rounded border border-(--color-border-default) text-xs text-(--color-text-muted) hover:bg-(--color-bg-elevated)"
          >
            x
          </button>
        )}
      </div>
    </LabeledField>
  );
}

export function PieChartSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: OnConfig;
}): React.ReactElement {
  const { t } = useTranslation();
  const config = panel.config ?? {};
  const showLegend = (config.show_legend as boolean | undefined) ?? true;
  const legendPosition = (config.legend_position as PieLegendPosition | undefined) ?? 'bottom';
  const showPercentage = (config.show_percentage as boolean | undefined) ?? true;
  const showValue = (config.show_value as boolean | undefined) ?? false;
  const labelPosition = config.label_position === 'outside' ? 'outside' : 'inside';
  // 슬라이더는 "미지정" 을 표현할 수 없으므로 자동값을 그대로 보여 준다 — 빈 칸이
  // 0으로 읽히거나, 자동일 때 슬라이더가 왼쪽 끝에 붙어 있는 것을 막는다.
  const autoPieSize = labelPosition === 'outside' && (showPercentage || showValue) ? 62 : 80;
  const pieSize = (config.pie_size as number | undefined) ?? autoPieSize;
  const legendShowPercentage = (config.legend_show_percentage as boolean | undefined) ?? false;
  const legendShowValue = (config.legend_show_value as boolean | undefined) ?? false;
  const unit = (config.unit as string | undefined) ?? '';

  /** 체크박스 한 줄 — 이 섹션 안에서만 쓰는 모양이라 지역 헬퍼로 둔다. */
  const checkbox = (
    key: string,
    labelKey: string,
    checked: boolean,
    testId: string,
  ): React.ReactElement => (
    <label className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1 transition-colors hover:bg-(--color-bg-elevated)">
      <input
        type="checkbox"
        checked={checked}
        data-testid={testId}
        onChange={(e) => onConfigChange({ [key]: e.target.checked })}
        className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
      />
      <span className="text-sm text-(--color-text-primary)">{t(labelKey)}</span>
    </label>
  );

  return (
    <div className="space-y-3">
      <UnitField
        value={unit}
        onChange={(v) => onConfigChange({ unit: v || undefined })}
        testId="pie-chart-unit"
      />
      <DecimalPlacesField
        config={config}
        onConfigChange={onConfigChange}
        testId="pie-chart-decimal-places"
      />

      {/* ═══ 파이 ═══ */}
      <SettingsSection title={t('dashboard.chart.pieSection')}>
        <LabeledField label={t('dashboard.chart.pieSize')} hint={t('dashboard.chart.pieSizeHint')}>
          <div className="flex items-center gap-2">
            <input
              type="range"
              min={PANEL_SIZE_MIN}
              max={PANEL_SIZE_MAX}
              step={1}
              value={pieSize}
              data-testid="pie-chart-size"
              onChange={(e) => onConfigChange({ pie_size: Number(e.target.value) })}
              className="flex-1"
            />
            <span className="w-12 shrink-0 text-right text-xs tabular-nums text-(--color-text-muted)">
              {pieSize}%
            </span>
            {/* 자동으로 되돌리는 유일한 출구다 — 슬라이더에는 "미지정" 자리가 없다. */}
            {config.pie_size !== undefined && (
              <button
                type="button"
                data-testid="pie-chart-size-reset"
                onClick={() => onConfigChange({ pie_size: undefined })}
                className="shrink-0 rounded border border-(--color-border-default) px-2 py-1 text-xs text-(--color-text-secondary) hover:bg-(--color-bg-elevated)"
              >
                {t('dashboard.chart.auto')}
              </button>
            )}
          </div>
        </LabeledField>
        <p className="text-[10px] leading-snug text-(--color-text-muted)">
          {t('dashboard.chart.pieDragHint')}
        </p>
        {(config.pie_offset_x || config.pie_offset_y) ? (
          <button
            type="button"
            data-testid="pie-chart-reset-offset"
            onClick={() => onConfigChange({ pie_offset_x: undefined, pie_offset_y: undefined })}
            className="rounded border border-(--color-border-default) px-2 py-1 text-xs text-(--color-text-secondary) hover:bg-(--color-bg-elevated)"
          >
            {t('dashboard.chart.pieResetOffset')}
          </button>
        ) : null}
      </SettingsSection>

      {/* ═══ 조각 라벨 ═══ */}
      <SettingsSection title={t('dashboard.chart.sliceLabelSection')}>
        {checkbox('show_percentage', 'dashboard.chart.showPercentage', showPercentage, 'pie-chart-show-percentage')}
        {checkbox('show_value', 'dashboard.chart.showValue', showValue, 'pie-chart-show-value')}
        {/* 크기는 적을 것이 있을 때만 뜻이 있다 — 둘 다 끈 상태에서 크기를 고르게
            두면 설정이 적용되지 않는 이유를 화면에서 알 수 없다. */}
        {(showPercentage || showValue) && (
          <>
            <LabeledField
              label={t('dashboard.chart.labelPosition')}
              hint={t('dashboard.chart.labelPositionHint')}
            >
              <select
                value={labelPosition}
                data-testid="pie-chart-label-position"
                onChange={(e) => onConfigChange({ label_position: e.target.value })}
                className={inputClass()}
              >
                <option value="inside">{t('dashboard.chart.labelInside')}</option>
                <option value="outside">{t('dashboard.chart.labelOutside')}</option>
              </select>
            </LabeledField>
            <TextStyleFields
              label={t('dashboard.chart.labelTextStyle')}
              family={config.label_font_family as ChartFontFamily | undefined}
              size={config.label_font_size as number | undefined}
              color={config.label_font_color as string | undefined}
              sizePlaceholder={t('dashboard.chart.inherit')}
              testIdPrefix="pie-chart-label-font"
              onChange={(patch) =>
                onConfigChange({
                  ...('family' in patch ? { label_font_family: patch.family } : null),
                  ...('size' in patch ? { label_font_size: patch.size } : null),
                  ...('color' in patch ? { label_font_color: patch.color } : null),
                })
              }
            />
            <LabeledField
              label={t('dashboard.chart.labelMinPercent')}
              hint={t('dashboard.chart.labelMinPercentHint')}
            >
              <input
                type="number"
                min={0}
                max={100}
                value={(config.label_min_percent as number | undefined) ?? ''}
                placeholder={String(DEFAULT_PIE_LABEL_MIN_PERCENT)}
                data-testid="pie-chart-label-min-percent"
                onChange={(e) => {
                  const v = e.target.value;
                  if (v === '') return onConfigChange({ label_min_percent: undefined });
                  const n = parseInt(v, 10);
                  if (!Number.isNaN(n) && n >= 0) onConfigChange({ label_min_percent: n });
                }}
                className={inputClass()}
              />
            </LabeledField>
          </>
        )}
      </SettingsSection>

      {/* ═══ 범례 ═══ */}
      <SettingsSection title={t('dashboard.chart.legendSection')}>
        {checkbox('show_legend', 'dashboard.chart.showLegend', showLegend, 'pie-chart-show-legend')}
        {/* 아래 항목은 모두 범례를 켰을 때만 뜻이 있다. */}
        {showLegend && (
          <>
            <LabeledField label={t('dashboard.chart.legendPosition')} hint={t('dashboard.chart.legendDragHint')}>
              <select
                value={legendPosition}
                onChange={(e) =>
                  onConfigChange({ legend_position: e.target.value as PieLegendPosition })
                }
                data-testid="pie-chart-legend-position"
                className={inputClass()}
              >
                <option value="bottom">{t('dashboard.chart.legendBottom')}</option>
                <option value="left">{t('dashboard.chart.legendLeft')}</option>
                <option value="right">{t('dashboard.chart.legendRight')}</option>
              </select>
            </LabeledField>
            {checkbox('legend_show_percentage', 'dashboard.chart.legendShowPercentage', legendShowPercentage, 'pie-chart-legend-show-percentage')}
            {checkbox('legend_show_value', 'dashboard.chart.legendShowValue', legendShowValue, 'pie-chart-legend-show-value')}
            <TextStyleFields
              label={t('dashboard.chart.legendTextStyle')}
              family={config.legend_font_family as ChartFontFamily | undefined}
              size={config.legend_font_size as number | undefined}
              color={config.legend_font_color as string | undefined}
              sizePlaceholder={String(DEFAULT_PIE_LEGEND_FONT_SIZE)}
              testIdPrefix="pie-chart-legend-font"
              onChange={(patch) =>
                onConfigChange({
                  ...('family' in patch ? { legend_font_family: patch.family } : null),
                  ...('size' in patch ? { legend_font_size: patch.size } : null),
                  ...('color' in patch ? { legend_font_color: patch.color } : null),
                })
              }
            />
            {/* 끌어 옮긴 자리를 되돌리는 유일한 출구다 — 오프셋이 남으면 위치를 바꿔도
                범례가 엉뚱한 자리에 붙어 있고, 드래그는 미리보기에서만 되기 때문이다. */}
            {(config.legend_offset_x || config.legend_offset_y) ? (
              <button
                type="button"
                data-testid="pie-chart-legend-reset-offset"
                onClick={() =>
                  onConfigChange({ legend_offset_x: undefined, legend_offset_y: undefined })
                }
                className="rounded border border-(--color-border-default) px-2 py-1 text-xs text-(--color-text-secondary) hover:bg-(--color-bg-elevated)"
              >
                {t('dashboard.chart.legendResetOffset')}
              </button>
            ) : null}
          </>
        )}
      </SettingsSection>
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
  const rowMode = (config.row_mode as TableRowMode | undefined) ?? 'entry';
  const pivotTimeHeader = (config.pivot_time_header as string | undefined) ?? '';
  const pivotUnit = (config.pivot_unit as string | undefined) ?? '';

  const updateColumn = (i: number, patch: Partial<TableColumn>): void => {
    onConfigChange({
      columns: columns.map((c, idx) => (idx === i ? { ...c, ...patch } : c)),
    });
  };
  const addColumn = (): void => {
    onConfigChange({ columns: [...columns, { field: 'value', header: t('dashboard.chart.newColumn') }] });
  };
  /**
   * 태그를 열로 추가한다. 헤더는 태그 키를 그대로 쓴다 — 사용자가 고른 이름이 곧
   * 열 이름인 편이 예측 가능하고, 바꾸고 싶으면 헤더 칸에서 고치면 된다.
   */
  const addTagColumn = (tagKey: string): void => {
    onConfigChange({
      columns: [...columns, { field: tagFieldPath(tagKey), header: tagKey, format: 'string' as TableColumnFormat }],
    });
  };
  // 이미 열로 쓰고 있는 태그는 후보에서 뺀다 — 같은 태그를 두 번 넣을 이유가 없고,
  // 목록에 남아 있으면 "눌렀는데 아무 일도 안 일어난다" 로 읽힌다.
  const usedTagKeys = new Set(
    columns.map((c) => tagKeyOfField(c.field)).filter((k): k is string => k !== undefined),
  );
  const tagCandidates = panelTagKeys(config).filter((k) => !usedTagKeys.has(k));
  const removeColumn = (i: number): void => {
    if (columns.length <= 1) return;
    onConfigChange({ columns: columns.filter((_, idx) => idx !== i) });
  };
  /**
   * 열을 한 칸 위/아래로 옮긴다. 배열 순서가 곧 표의 열 순서이므로 인접 교환이면 충분하다
   * (드래그는 목록이 길어질 때 이득이 나는데, 열은 보통 2~5개다).
   */
  const moveColumn = (i: number, delta: -1 | 1): void => {
    const j = i + delta;
    if (j < 0 || j >= columns.length) return;
    const next = columns.slice();
    const a = next[i]!;
    next[i] = next[j]!;
    next[j] = a;
    onConfigChange({ columns: next });
  };

  return (
    <div className="space-y-3">
      {/* 행 기준 — 표의 형상을 정하는 설정이라 열 편집기보다 위에 둔다. */}
      <LabeledField
        label={t('dashboard.chart.rowMode')}
        hint={
          rowMode === 'timestamp'
            ? t('dashboard.chart.rowModeTimestampHint')
            : t('dashboard.chart.rowModeEntryHint')
        }
      >
        <select
          value={rowMode}
          onChange={(e) => {
            const v = e.target.value as TableRowMode;
            // 기본값은 저장하지 않는다 — 기존 패널 config 와 같은 모양을 유지한다.
            onConfigChange({ row_mode: v === 'entry' ? undefined : v });
          }}
          data-testid="table-row-mode"
          className={inputClass()}
        >
          <option value="entry">{t('dashboard.chart.rowModeEntry')}</option>
          <option value="timestamp">{t('dashboard.chart.rowModeTimestamp')}</option>
        </select>
      </LabeledField>
      {/* 자릿수는 `format: 'number'` 열에만 적용된다 — 시각·문자열 열은 영향이 없다. */}
      <DecimalPlacesField
        config={config}
        onConfigChange={onConfigChange}
        testId="table-decimal-places"
      />
      {/* 시각 기준 행에서는 열이 데이터에서 파생되므로 열 목록 편집기를 내리고,
          사용자가 정할 수 있는 것(시각 열 이름 · 값 단위)만 남긴다. 파생 열을 목록으로
          띄우면 필드 칸에 `$.series.<이름>` 이 노출되고, 고쳐도 다음 렌더에 되돌아간다. */}
      {rowMode === 'timestamp' ? (
        <div className="space-y-3" data-testid="table-pivot-settings">
          <LabeledField label={t('dashboard.chart.pivotTimeHeader')}>
            <input
              type="text"
              value={pivotTimeHeader}
              onChange={(e) => onConfigChange({ pivot_time_header: e.target.value || undefined })}
              placeholder={t('dashboard.chart.colTime')}
              data-testid="table-pivot-time-header"
              className={inputClass()}
            />
          </LabeledField>
          <UnitField
            value={pivotUnit}
            onChange={(v) => onConfigChange({ pivot_unit: v || undefined })}
            testId="table-pivot-unit"
          />
          <p className="text-[11px] leading-snug text-(--color-text-muted)">
            {t('dashboard.chart.pivotColumnsHint')}
          </p>
        </div>
      ) : (
      <div>
        <div className="mb-1.5 flex items-center justify-between gap-2">
          <label className="text-xs font-medium text-(--color-text-muted)">{t('dashboard.chart.columns')}</label>
          <div className="flex items-center gap-1.5">
            {/* 태그 열 추가 — 데이터 소스 설정에 적힌 태그를 골라 바로 열로 만든다.
                종전에는 `$.tags.<키>` 를 손으로 적는 것 말고는 방법이 없었다. 후보가
                하나도 없으면(태그를 걸지 않은 소스) 셀렉트를 내린다 — 고를 것이 없는
                빈 목록은 안내가 아니라 방해다. 필드 직접 입력은 그대로 남아 있고,
                아래 안내 문구가 그 통로를 설명한다. */}
            {tagCandidates.length > 0 && (
              <select
                value=""
                onChange={(e) => {
                  const v = e.target.value;
                  if (v !== '') addTagColumn(v);
                }}
                data-testid="table-add-tag-column"
                aria-label={t('dashboard.chart.addTagColumn')}
                className="rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-0.5 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
              >
                <option value="">{t('dashboard.chart.addTagColumn')}</option>
                {tagCandidates.map((k) => (
                  <option key={k} value={k}>
                    {k}
                  </option>
                ))}
              </select>
            )}
            <button
              type="button"
              onClick={addColumn}
              className="flex items-center gap-1 rounded-md px-2 py-0.5 text-xs font-medium text-blue-600 transition-colors hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20"
            >
              <Plus className="h-3 w-3" /> {t('dashboard.chart.add')}
            </button>
          </div>
        </div>
        {/* 태그 컬럼 표기 안내 — 필드에 무엇을 쓸 수 있는지 화면에서 알 수 있어야 한다. */}
        <p className="mb-1.5 text-[11px] leading-snug text-(--color-text-muted)">
          {t('dashboard.chart.tagColumnHint')}
        </p>
        <div className="space-y-2">
          {columns.map((c, i) => (
            <div
              key={i}
              className="space-y-1 rounded border border-(--color-border-default) p-1.5"
              data-testid={`table-column-row-${i}`}
            >
              {/* 1행: 순서 · 필드 · 헤더 · 삭제 */}
              <div className="flex items-center gap-1">
                <div className="flex shrink-0 flex-col">
                  <button
                    type="button"
                    onClick={() => moveColumn(i, -1)}
                    disabled={i === 0}
                    className="rounded px-0.5 text-(--color-text-muted) transition-colors hover:text-(--color-text-primary) disabled:opacity-30"
                    aria-label={t('dashboard.chart.moveColumnUpAria')}
                  >
                    <ChevronUp className="h-3 w-3" />
                  </button>
                  <button
                    type="button"
                    onClick={() => moveColumn(i, 1)}
                    disabled={i === columns.length - 1}
                    className="rounded px-0.5 text-(--color-text-muted) transition-colors hover:text-(--color-text-primary) disabled:opacity-30"
                    aria-label={t('dashboard.chart.moveColumnDownAria')}
                  >
                    <ChevronDown className="h-3 w-3" />
                  </button>
                </div>
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

              {/* 2행: 형식 · 폭 비율 · 정렬/필터 허용 */}
              <div className="flex flex-wrap items-center gap-x-2 gap-y-1 pl-5">
                <select
                  value={c.format ?? 'string'}
                  onChange={(e) =>
                    updateColumn(i, { format: e.target.value as TableColumnFormat })
                  }
                  aria-label={t('dashboard.chart.columnFormatAria')}
                  className="shrink-0 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1 py-1 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
                >
                  <option value="string">string</option>
                  <option value="number">number</option>
                  <option value="datetime">datetime</option>
                </select>
                {/* 단위는 수치 열에만 뜻이 있다 — 시각·문자열 열에서는 컨트롤 자체를
                    내린다(있는데 아무 효과가 없는 칸이 가장 헷갈린다). */}
                {c.format === 'number' && (
                  <UnitControl
                    value={c.unit ?? ''}
                    onChange={(v) => updateColumn(i, { unit: v || undefined })}
                    testId={`table-column-unit-${i}`}
                    compact
                  />
                )}
                <label className="flex items-center gap-1 text-xs text-(--color-text-muted)">
                  {t('dashboard.chart.columnWidth')}
                  <input
                    type="number"
                    min={1}
                    step={1}
                    value={c.width ?? ''}
                    placeholder={t('dashboard.chart.columnWidthAuto')}
                    onChange={(e) => {
                      const raw = e.target.value.trim();
                      if (raw === '') {
                        updateColumn(i, { width: undefined });
                        return;
                      }
                      const n = Number(raw);
                      // 0 이하/비수치는 무시한다 — 0 비율은 열을 사라지게 만든다.
                      if (Number.isFinite(n) && n > 0) updateColumn(i, { width: n });
                    }}
                    className="w-14 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1 py-1 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
                  />
                </label>
                <label className="flex items-center gap-1 text-xs text-(--color-text-muted)">
                  <input
                    type="checkbox"
                    className="h-3 w-3"
                    checked={c.sortable !== false}
                    onChange={(e) => updateColumn(i, { sortable: e.target.checked })}
                  />
                  {t('dashboard.chart.columnSortable')}
                </label>
                <label className="flex items-center gap-1 text-xs text-(--color-text-muted)">
                  <input
                    type="checkbox"
                    className="h-3 w-3"
                    checked={c.filterable === true}
                    onChange={(e) => updateColumn(i, { filterable: e.target.checked })}
                  />
                  {t('dashboard.chart.columnFilterable')}
                </label>
              </div>
            </div>
          ))}
        </div>
      </div>
      )}
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
