// 차트 패널 타입별 설정 섹션.
// PanelSettingsDialog 에서 사용하는 sub-컴포넌트 모음 (SPEC-CHART-001 §4.2.2 / REQ-M5-03).
//
// 5종 차트 (stat / line-chart / bar-chart / pie-chart / table) 각각의 config
// 편집 UI 를 제공한다. 상위 PanelSettingsDialog 는 panel.type 에 따라
// 분기하여 해당 Section 을 렌더링한다.

import React, { useEffect, useState } from 'react';
import { Plus, Trash2 } from 'lucide-react';

import type { PanelConfig } from '@/stores/uiStore';
import { listChartChannels, type ChartChannelSummary } from '@/services/api/charts';

import type {
  TableColumn,
  TableColumnFormat,
  BarChartMode,
  AggFunc,
  SortOrder,
} from './panels/charts/chartChannelTypes';

/** REQ-M5-04: channel_name 정규식 */
const CHANNEL_NAME_REGEX = /^[a-zA-Z][a-zA-Z0-9_-]{0,63}$/;

/** Custom (수동 입력) 드롭다운 옵션 sentinel */
const CUSTOM_CHANNEL_SENTINEL = '__custom__';

/** 인라인 에러 메시지 (한국어 UI) */
const CHANNEL_NAME_ERROR_MESSAGE =
  '유효한 채널 이름이 아닙니다. 영문자로 시작하고 영숫자/하이픈/밑줄만 허용됩니다 (최대 64자).';

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
      label="채널 이름 (channel_name)"
      hint="활성 chart-emitter 채널을 선택하거나, Custom 을 눌러 배포 예정인 채널 이름을 직접 입력하세요."
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
            ? '활성 채널 목록 불러오는 중...'
            : channels.length === 0
              ? '활성 채널 없음 (Custom 으로 수동 입력)'
              : '채널을 선택하세요'}
        </option>
        {currentIsInactive && (
          <option value={currentName}>
            {currentName} — (현재 선택, 비활성)
          </option>
        )}
        {channels.map((ch) => (
          <option key={ch.name} value={ch.name}>
            {ch.name} — flow {ch.flow_id || '?'} ({ch.subscriber_count} subs)
          </option>
        ))}
        <option value={CUSTOM_CHANNEL_SENTINEL}>Custom... (직접 입력)</option>
      </select>

      {loadState === 'error' && (
        <p className="mt-1 text-xs text-amber-600 dark:text-amber-400">
          채널 목록 조회 실패. Custom 으로 수동 입력을 사용하세요.
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
          placeholder="예: room1_temp"
          autoFocus
          className={`${inputClass()} mt-2`}
        />
      )}

      {showError && (
        <p
          data-testid="chart-channel-name-error"
          className="mt-1 text-xs text-red-500"
        >
          {CHANNEL_NAME_ERROR_MESSAGE}
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
        label="표시 필드 (display_field)"
        hint="payload 에서 값으로 사용할 필드 경로. 예: value, labels.temperature"
      >
        <input
          type="text"
          value={displayField}
          onChange={(e) => onConfigChange({ display_field: e.target.value })}
          className={inputClass()}
        />
      </LabeledField>
      <LabeledField label="단위 (unit)">
        <input
          type="text"
          value={unit}
          onChange={(e) => onConfigChange({ unit: e.target.value })}
          placeholder="예: °C, %, kWh"
          className={inputClass()}
        />
      </LabeledField>
      <LabeledField label="소수점 자릿수 (decimal_places)">
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
            임계값 색상 규칙
          </label>
          <button
            type="button"
            onClick={addRule}
            className="flex items-center gap-1 rounded-md px-2 py-0.5 text-xs font-medium text-blue-600 transition-colors hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20"
          >
            <Plus className="h-3 w-3" /> 추가
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
                값 ≥ {r.min} → {r.color}
              </span>
              <button
                type="button"
                onClick={() => removeRule(i)}
                className="rounded p-0.5 text-(--color-text-muted) transition-colors hover:text-red-500"
                aria-label="규칙 삭제"
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

// --- 3. line-chart 패널 설정 (SPEC §4.2.2 line-chart) ---

export function LineChartSection({
  panel,
  onConfigChange,
}: {
  panel: PanelConfig;
  onConfigChange: OnConfig;
}): React.ReactElement {
  const config = panel.config ?? {};
  const displayField = (config.display_field as string | undefined) ?? 'value';
  const maxPoints = (config.max_points as number | undefined) ?? 100;
  const yMin = (config.y_min as number | undefined);
  const yMax = (config.y_max as number | undefined);
  const smooth = (config.smooth as boolean | undefined) ?? false;
  const multiSeriesField = (config.multi_series_field as string | undefined) ?? '';

  return (
    <div className="space-y-3">
      <LabeledField label="표시 필드 (display_field)">
        <input
          type="text"
          value={displayField}
          onChange={(e) => onConfigChange({ display_field: e.target.value })}
          className={inputClass()}
        />
      </LabeledField>
      <LabeledField label="최대 포인트 (max_points)">
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
      <div className="flex gap-2">
        <LabeledField label="Y 최소">
          <input
            type="number"
            value={yMin ?? ''}
            onChange={(e) => {
              const v = e.target.value;
              if (v === '') {
                onConfigChange({ y_min: undefined });
              } else {
                const n = parseFloat(v);
                if (!Number.isNaN(n)) onConfigChange({ y_min: n });
              }
            }}
            className={inputClass()}
          />
        </LabeledField>
        <LabeledField label="Y 최대">
          <input
            type="number"
            value={yMax ?? ''}
            onChange={(e) => {
              const v = e.target.value;
              if (v === '') {
                onConfigChange({ y_max: undefined });
              } else {
                const n = parseFloat(v);
                if (!Number.isNaN(n)) onConfigChange({ y_max: n });
              }
            }}
            className={inputClass()}
          />
        </LabeledField>
      </div>
      <label className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1 transition-colors hover:bg-(--color-bg-elevated)">
        <input
          type="checkbox"
          checked={smooth}
          onChange={(e) => onConfigChange({ smooth: e.target.checked })}
          className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
        />
        <span className="text-sm text-(--color-text-primary)">부드러운 곡선 (smooth)</span>
      </label>
      <LabeledField
        label="다중 시리즈 필드 (multi_series_field)"
        hint="지정 시 해당 라벨 값별로 라인을 분리. 예: labels.room"
      >
        <input
          type="text"
          value={multiSeriesField}
          onChange={(e) => onConfigChange({ multi_series_field: e.target.value || undefined })}
          placeholder="비워두면 단일 시리즈"
          className={inputClass()}
        />
      </LabeledField>
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
  const config = panel.config ?? {};
  const displayField = (config.display_field as string | undefined) ?? 'value';
  const labelField = (config.label_field as string | undefined) ?? 'labels.name';
  const mode = (config.mode as BarChartMode | undefined) ?? 'category';
  const binSec = (config.bin_sec as number | undefined) ?? 60;
  const aggFunc = (config.agg_func as AggFunc | undefined) ?? 'avg';
  const maxPoints = (config.max_points as number | undefined) ?? 20;

  return (
    <div className="space-y-3">
      <LabeledField label="표시 필드 (display_field)">
        <input
          type="text"
          value={displayField}
          onChange={(e) => onConfigChange({ display_field: e.target.value })}
          className={inputClass()}
        />
      </LabeledField>
      <LabeledField label="모드 (mode)">
        <select
          value={mode}
          onChange={(e) => onConfigChange({ mode: e.target.value as BarChartMode })}
          className={inputClass()}
        >
          <option value="category">카테고리 (category)</option>
          <option value="time_bin">시간 bin (time_bin)</option>
        </select>
      </LabeledField>
      {mode === 'category' && (
        <LabeledField
          label="라벨 필드 (label_field)"
          hint="카테고리로 사용할 필드 경로. 예: labels.room"
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
        <LabeledField label="bin 간격 (초)">
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
      <LabeledField label="집계 함수 (agg_func)">
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
      <LabeledField label="최대 포인트 (max_points)">
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
  const config = panel.config ?? {};
  const displayField = (config.display_field as string | undefined) ?? 'value';
  const labelField = (config.label_field as string | undefined) ?? 'labels.name';
  const aggFunc = (config.agg_func as AggFunc | undefined) ?? 'sum';
  const showLegend = (config.show_legend as boolean | undefined) ?? true;
  const showPercentage = (config.show_percentage as boolean | undefined) ?? true;
  const maxPoints = (config.max_points as number | undefined) ?? 20;

  return (
    <div className="space-y-3">
      <LabeledField label="표시 필드 (display_field)">
        <input
          type="text"
          value={displayField}
          onChange={(e) => onConfigChange({ display_field: e.target.value })}
          className={inputClass()}
        />
      </LabeledField>
      <LabeledField
        label="라벨 필드 (label_field)"
        hint="파이 슬라이스 그룹화 기준. 예: labels.category"
      >
        <input
          type="text"
          value={labelField}
          onChange={(e) => onConfigChange({ label_field: e.target.value })}
          className={inputClass()}
        />
      </LabeledField>
      <LabeledField label="집계 함수 (agg_func)">
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
        <span className="text-sm text-(--color-text-primary)">범례 표시 (show_legend)</span>
      </label>
      <label className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1 transition-colors hover:bg-(--color-bg-elevated)">
        <input
          type="checkbox"
          checked={showPercentage}
          onChange={(e) => onConfigChange({ show_percentage: e.target.checked })}
          className="h-4 w-4 rounded border-gray-300 text-blue-600 focus:ring-blue-500"
        />
        <span className="text-sm text-(--color-text-primary)">
          비율(%) 표시 (show_percentage)
        </span>
      </label>
      <LabeledField label="최대 포인트 (max_points)">
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
  const config = panel.config ?? {};
  const columns =
    (config.columns as TableColumn[] | undefined) ?? [
      { field: 'timestamp', header: '시간', format: 'datetime' as TableColumnFormat },
      { field: 'value', header: '값', format: 'number' as TableColumnFormat },
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
    onConfigChange({ columns: [...columns, { field: 'value', header: '새 열' }] });
  };
  const removeColumn = (i: number): void => {
    if (columns.length <= 1) return;
    onConfigChange({ columns: columns.filter((_, idx) => idx !== i) });
  };

  return (
    <div className="space-y-3">
      <div>
        <div className="mb-1.5 flex items-center justify-between">
          <label className="text-xs font-medium text-(--color-text-muted)">열 (columns)</label>
          <button
            type="button"
            onClick={addColumn}
            className="flex items-center gap-1 rounded-md px-2 py-0.5 text-xs font-medium text-blue-600 transition-colors hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/20"
          >
            <Plus className="h-3 w-3" /> 추가
          </button>
        </div>
        <div className="space-y-1.5">
          {columns.map((c, i) => (
            <div key={i} className="flex items-center gap-1">
              <input
                type="text"
                value={c.field}
                onChange={(e) => updateColumn(i, { field: e.target.value })}
                placeholder="field"
                className="flex-1 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1 text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
              />
              <input
                type="text"
                value={c.header}
                onChange={(e) => updateColumn(i, { header: e.target.value })}
                placeholder="header"
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
                aria-label="열 삭제"
              >
                <Trash2 className="h-3 w-3" />
              </button>
            </div>
          ))}
        </div>
      </div>
      <LabeledField label="페이지 당 행 수 (rows_per_page)">
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
      <LabeledField label="최대 포인트 (max_points)">
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
          기본 정렬 (default_sort)
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
            placeholder="정렬 필드"
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
