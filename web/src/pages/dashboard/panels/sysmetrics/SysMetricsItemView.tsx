// 항목 하나를 고른 스타일로 그린다.
//
// 여섯 스타일이 같은 입력(지금 값 + 시계열)을 받아 서로 다른 모양을 낸다. 스타일
// 분기를 패널마다 두면 세 패널이 조금씩 다르게 그리기 시작하므로 한 곳에 모은다.
//
// 게이지는 여기서 직접 그린다. 기존 GaugePanel 의 렌더러들은 그 패널의 config 파싱
// 결과(임계값·데이터소스 등)에 묶여 있어, 가져다 쓰려면 800줄짜리 패널을 먼저
// 갈라야 한다. 여기 필요한 것은 비율 하나를 도넛으로 그리는 일뿐이라 직접 그린다.

import type { ReactNode } from 'react';

import MultiSeriesChart, { type ChartSeries } from '@/pages/monitoring/MultiSeriesChart';

import { parseConfig, renderGaugeByType } from '../gauge/gaugeShapes';
import { MetricTile } from './SysMetricsPanelShell';
import { isTimeSeriesStyle, type SysMetricsItemOptions } from './sysMetricsItemOptions';

/** 비율에 따른 색 — 가득 찬 쪽이 눈에 띄어야 한다. */
export function ratioColor(percent: number): string {
  if (percent >= 90) return 'var(--color-status-error)';
  if (percent >= 75) return 'var(--color-status-warning)';
  return 'var(--color-interactive-primary)';
}

/**
 * 계열의 지금 값 — 마지막 점.
 *
 * 점이 없으면 undefined 다. 0 으로 대신하면 "아직 값이 없다"가 "0 이다"로 읽힌다.
 */
function valueOfSeries(
  s: ChartSeries | undefined,
  format: (v: number) => string,
): string | undefined {
  const last = s?.data.at(-1)?.value;
  return last === undefined ? undefined : format(last);
}

/** 0~100 으로 죈다. 값이 없으면 0. */
function clampPercent(value: number | undefined): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return 0;
  return Math.min(Math.max(value, 0), 100);
}

// --- 게이지 ---

interface GaugeProps {
  options: SysMetricsItemOptions;
  percent: number;
  label: string;
  testId?: string;
}

/**
 * 비율 하나를 게이지로 그린다.
 *
 * 모양 8종은 게이지 패널과 같은 렌더러를 쓴다(`panels/gauge/gaugeShapes`). 여기서
 * 따로 그리면 "게이지 패널의 바늘"과 "시스템 지표의 바늘"이 갈라진다.
 */
function RatioGauge({ options, percent, label, testId }: GaugeProps) {
  const value = clampPercent(percent);
  const parsed = parseConfig({
    value,
    min: options.min,
    max: options.max,
    unit: options.unit,
    gaugeType: options.gaugeType,
  });

  return (
    <div
      data-testid={testId}
      data-style="gauge"
      data-gauge-type={options.gaugeType}
      data-percent={value.toFixed(1)}
      // 높이 미지정이면 칸을 채운다(h-full). 값이 있으면 그 높이에 고정한다.
      className="flex min-w-0 flex-col rounded-lg bg-(--color-bg-surface) p-3 shadow"
      style={options.height ? { height: options.height } : { height: '100%' }}
    >
      <p className="truncate text-xs text-(--color-text-muted)">{label}</p>
      <div className="min-h-0 w-full flex-1">{renderGaugeByType(parsed, true)}</div>
    </div>
  );
}

// --- 진행 막대 ---

interface ProgressProps {
  percent: number;
  label: string;
  hint?: ReactNode;
  testId?: string;
}

/** 비율 하나를 가로 막대로 그린다. 게이지보다 세로 공간을 덜 먹는다. */
function RatioProgress({ percent, label, hint, testId }: ProgressProps) {
  const value = clampPercent(percent);

  return (
    <div
      data-testid={testId}
      data-style="progress"
      data-percent={value.toFixed(1)}
      className="flex min-w-0 flex-col justify-center rounded-lg bg-(--color-bg-surface) p-3 shadow"
    >
      <div className="flex items-baseline justify-between gap-2">
        <span className="truncate text-xs text-(--color-text-muted)">{label}</span>
        <span className="shrink-0 text-sm font-semibold text-(--color-text-primary)">
          {value.toFixed(1)}%
        </span>
      </div>
      <div className="mt-1 h-2 w-full overflow-hidden rounded bg-(--color-bg-sunken)">
        <div
          className="h-full rounded"
          style={{ width: `${value}%`, backgroundColor: ratioColor(value) }}
        />
      </div>
      {hint && <div className="mt-1 text-xs text-(--color-text-muted)">{hint}</div>}
    </div>
  );
}

// --- 통합 뷰 ---

interface SysMetricsItemViewProps {
  options: SysMetricsItemOptions;
  label: string;
  /** 타일 본문 값 (스타일이 tile 일 때) */
  value?: string;
  /** 타일·진행막대 보조 설명 */
  hint?: ReactNode;
  /** 비율 값 (게이지·진행막대에서 쓴다) */
  percent?: number;
  /** 시계열 계열 (라인·영역·막대에서 쓴다) */
  series?: ChartSeries[];
  /** 시계열 X축 구간 */
  timeDomain?: [number, number];
  /** 시계열 값 표기 */
  format?: (value: number) => string;
  /** 수집이 꺼진 항목 — 값 0 과 구별해 표시한다. */
  disabled?: boolean;
  labelColor?: string;
  valueColor?: string;
  testId?: string;
}

/**
 * 항목 하나를 고른 스타일로 그린다.
 *
 * 수집이 꺼진 항목은 스타일과 무관하게 타일로 떨어뜨린다 — 빈 차트나 0% 게이지는
 * "관측하지 않는다"를 "값이 0 이다"로 잘못 읽히게 한다.
 */
export function SysMetricsItemView({
  options,
  label,
  value,
  hint,
  percent,
  series,
  timeDomain,
  format,
  disabled,
  labelColor,
  valueColor,
  testId,
}: SysMetricsItemViewProps) {
  if (disabled) {
    return (
      <MetricTile
        options={options}
        testId={testId}
        label={label}
        disabled
        labelColor={labelColor}
        valueColor={valueColor}
      />
    );
  }

  if (options.style === 'gauge' && percent !== undefined) {
    return <RatioGauge options={options} percent={percent} label={label} testId={testId} />;
  }

  if (options.style === 'progress' && percent !== undefined) {
    return <RatioProgress percent={percent} label={label} hint={hint} testId={testId} />;
  }

  if (isTimeSeriesStyle(options.style) && series && timeDomain && format) {
    return (
      // 높이 미지정이면 칸을 채운다 — 차트가 부모의 확정 높이를 받아야 하므로
      // 래퍼도 h-full 로 둔다.
      <div data-testid={testId} data-style={options.style} className="flex min-w-0 flex-col">
        <MultiSeriesChart
          title={label}
          series={series}
          format={format}
          timeDomain={timeDomain}
          height={options.height}
          fillParent={!options.height}
          style={options.style}
          legend={options.legend}
          smooth={options.smooth}
          stacked={options.stacked}
        />
      </div>
    );
  }

  // 타일. 대상이 여럿이면 대상마다 한 줄씩 보여준다 — 첫 대상만 그리면 사용자가
  // 고른 나머지가 조용히 사라진다.
  if (series && series.length > 1 && format) {
    return (
      <MetricTile
        options={options}
        testId={testId}
        label={label}
        labelColor={labelColor}
        valueColor={valueColor}
        rows={series.map((s) => ({
          name: s.name,
          color: s.color,
          value: valueOfSeries(s, format),
        }))}
      />
    );
  }

  // 폴백: 타일. 고른 스타일이 그릴 재료를 못 받았을 때도 값은 보여야 한다.
  return (
    <MetricTile
      options={options}
      testId={testId}
      label={label}
      value={value ?? (series && format ? valueOfSeries(series[0], format) : undefined)}
      hint={typeof hint === 'string' ? hint : undefined}
      labelColor={labelColor}
      valueColor={valueColor}
    />
  );
}
