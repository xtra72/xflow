// 차트 패널 타입별 설정 섹션.
// PanelSettingsDialog 에서 사용하는 sub-컴포넌트 모음 (SPEC-CHART-001 §4.2.2 / REQ-M5-03).
//
// 5종 차트 (stat / line-chart / bar-chart / pie-chart / table) 각각의 config
// 편집 UI 를 제공한다. 상위 PanelSettingsDialog 는 panel.type 에 따라
// 분기하여 해당 Section 을 렌더링한다.

import React, { useEffect, useMemo, useState } from 'react';
import { ChevronDown, ChevronRight, GripVertical, Plus, Trash2 } from 'lucide-react';

import type { PanelConfig } from '@/stores/uiStore';
import { listChartChannels, type ChartChannelSummary } from '@/services/api/charts';
import { useTranslation } from '@/lib/i18n';

import type {
  TableColumn,
  TableColumnFormat,
  BarChartMode,
  AggFunc,
  SortOrder,
  YAxisMode,
  TimeWindowMode,
  YThreshold,
  ChannelRefConfig,
  StrokeStyle,
} from './panels/charts/chartChannelTypes';

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

  const effectiveColor = channel.color ?? '#3b82f6';

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
          {/* 줄 1: 채널 선택 + display field */}
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
            <input
              type="text"
              value={channel.display_field ?? ''}
              onChange={(e) =>
                onPatch({ display_field: e.target.value || undefined })
              }
              placeholder={t('dashboard.chart.displayFieldShort')}
              className="w-28 rounded border border-(--color-border-default) bg-(--color-bg-surface) px-2 py-1 text-xs"
              aria-label={t('dashboard.chart.displayFieldAria')}
            />
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

          {/* 줄 2: 라인 스타일 */}
          <div className="flex items-center gap-2">
            <select
              value={channel.stroke_style ?? 'solid'}
              onChange={(e) =>
                onPatch({ stroke_style: e.target.value as StrokeStyle })
              }
              className="rounded border border-(--color-border-default) bg-(--color-bg-surface) px-1.5 py-1 text-xs"
              aria-label={t('dashboard.chart.lineStyleAria')}
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
                value={channel.stroke_width ?? 2}
                onChange={(e) => {
                  const n = parseInt(e.target.value, 10);
                  if (!Number.isNaN(n)) onPatch({ stroke_width: n });
                }}
                className="w-12 rounded border border-(--color-border-default) bg-(--color-bg-surface) px-1.5 py-1 text-xs"
              />
            </label>
            <label className="flex cursor-pointer items-center gap-1 text-xs text-(--color-text-muted)">
              <input
                type="checkbox"
                checked={channel.smooth ?? false}
                onChange={(e) => onPatch({ smooth: e.target.checked })}
                className="h-3 w-3 rounded border-gray-300"
              />
              {t('dashboard.chart.curve')}
            </label>
          </div>
        </div>
      )}
    </div>
  );
}

// --- 3. line-chart 패널 설정 (SPEC §4.2.2 line-chart) ---

export function LineChartSection({
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
  const maxPoints = (config.max_points as number | undefined) ?? 100;
  const xLabel = (config.x_label as string | undefined) ?? '';
  const yMin = config.y_min as number | undefined;
  const yMax = config.y_max as number | undefined;
  const yAxisMode = (config.y_axis_mode as YAxisMode | undefined) ?? 'auto';
  const yPadPct = (config.y_axis_padding_pct as number | undefined) ?? 5;
  const yLabel = (config.y_label as string | undefined) ?? '';
  const yUnit = (config.y_unit as string | undefined) ?? '';
  const timeWindowMode =
    (config.time_window_mode as TimeWindowMode | undefined) ?? 'points';
  const recentWindowSec = (config.recent_window_sec as number | undefined) ?? 600;
  const fixedStartMs = config.fixed_start_ms as number | undefined;
  const fixedEndMs = config.fixed_end_ms as number | undefined;
  const refreshMs = (config.time_window_refresh_ms as number | undefined) ?? 1000;
  const multiSeriesField = (config.multi_series_field as string | undefined) ?? '';
  const thresholds = (config.y_thresholds as YThreshold[] | undefined) ?? [];
  const legacyChannelName = (config.channel_name as string | undefined) ?? '';

  // channel_name 만 있고 channels 가 없는 기존 패널 → 자동 마이그레이션
  const channels: ChannelRefConfig[] = useMemo(() => {
    const raw = config.channels as ChannelRefConfig[] | undefined;
    if (raw && raw.length > 0) return raw;
    if (legacyChannelName) return [{ name: legacyChannelName }];
    return [{ name: '' }];
  }, [config.channels, legacyChannelName]);

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

  function updateChannels(next: ChannelRefConfig[]): void {
    // channels 로 통합: channel_name 은 제거
    onConfigChange({ channels: next.length === 0 ? [{ name: '' }] : next, channel_name: undefined });
  }
  function addChannel(): void {
    updateChannels([...channels, { name: '' }]);
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

  // 활성 채널 fetch (다채널 모드 행 드롭다운에서 사용)
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
    <div className="space-y-3">
      {/* --- 채널 (channels) --- */}
      <div data-testid="line-chart-channels-editor">
        <div className="mb-1.5 flex items-center justify-between">
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
        <p className="mb-1 text-[10px] leading-snug text-(--color-text-muted)">
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

        {/* Y축 */}
        <div className="flex items-end gap-2">
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
          <LabeledField label={t('dashboard.chart.label')}>
            <input
              type="text"
              value={yLabel}
              onChange={(e) => onConfigChange({ y_label: e.target.value || undefined })}
              placeholder={t('dashboard.chart.yLabelPlaceholder')}
              className={inputClass()}
            />
          </LabeledField>
          <LabeledField label={t('dashboard.chart.unit')}>
            <input
              type="text"
              value={yUnit}
              onChange={(e) => onConfigChange({ y_unit: e.target.value || undefined })}
              placeholder={t('dashboard.chart.yUnitPlaceholder')}
              className="w-16 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-2 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500"
            />
          </LabeledField>
        </div>

        {yAxisMode === 'manual' && (
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

        {yAxisMode === 'auto_padded' && (
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
