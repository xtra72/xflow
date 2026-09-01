// Pie Chart 패널 (REQ-M4-07).
// 최근 max_points 항목을 label_field 별로 그룹화한 값의 비율을 파이로 표시.
//
// SPEC-CHART-002: Store 모드 + `series_reduce` 지정 시에는 **시리즈당 조각 1개**를 그리는
// 다중 출력 경로로 갈린다. 이때 `label_field` · `agg_func` · `max_points` 는 무시된다
// (§2.9 [S1]). 분기는 아래 `derived` useMemo 진입부 한 곳뿐이다.

import { useMemo } from 'react';
import { PieChart as PieChartIcon } from 'lucide-react';
import { Cell, Pie, PieChart, ResponsiveContainer, Tooltip } from 'recharts';

import {
  DEFAULT_PIE_LEGEND_FONT_SIZE,
  pickSeriesColor,
  type ChartEntry,
  type PieLegendPosition,
  type PiePanelConfig,
} from './chartChannelTypes';
import { PieLegend, type PieLegendItem } from './PieLegend';
import {
  DEFAULT_PIE_LABEL_MIN_PERCENT,
  insideLabelPoint,
  isPieLabelVisible,
  pieSliceLabelText,
  piePercents,
  sliceLabelTextColor,
} from './pieLabel';
import { resolveFontColor, resolveFontFamily, resolveFontSize } from './textStyle';
import { clampPercentOffset, readPanelSize } from './panelGeometry';
import { clampStoredLegendOffset } from './legendOverlay';
import { ConnectionStatusIcon } from './ConnectionStatusIcon';
import { aggregateByLabel } from './chartChannelUtils';
import { reduceAllSeries } from './seriesReduce';
import { MultiOutputTruncationNotice } from './SeriesTileGrid';
import { applyMultiOutputLimit } from './multiOutputLimit';
import { useChartChannel } from './useChartChannel';
import { type StoreSeriesStyle } from './useStoreChartData';
import { resolvePanelSourceBinding } from './panelDataSource';
// 값 표기 자릿수는 차트 계열 공용 규칙을 따른다.
import { readDecimalPlaces } from './decimalPlaces';
import { formatValueWithUnit } from './unitOptions';
import { isPanelSeriesSource, usePanelSeriesData } from './usePanelSeriesData';
import { usePanelTitleVisible } from '../../panelChromeContext';
import { usePanelEditMode } from '../PanelEditToggle';
import { PieDragLayer } from '../../PieDragLayer';
import { useTranslation } from '@/lib/i18n';

interface PieChartPanelProps {
  panelId: string;
  title?: string;
  config: Record<string, unknown>;
  /**
   * 배치 편집으로 바뀐 값을 쓸 콜백. 없으면 편집 모드에 들어갈 수 없다 —
   * 끌어도 저장할 곳이 없기 때문이다.
   */
  onConfigChange?: (config: Record<string, unknown>) => void;
  /** 설정 미리보기처럼 **항상** 편집인 자리인가(토글을 감춘다). */
  forceEdit?: boolean;
}

const DEFAULT_MAX_POINTS = 20;

const PIE_COLORS = [
  '#3b82f6',
  '#10b981',
  '#f59e0b',
  '#ef4444',
  '#8b5cf6',
  '#06b6d4',
  '#ec4899',
  '#84cc16',
  '#f97316',
  '#14b8a6',
];

/** 고를 수 있는 범례 위치 — 인식 불가 값을 접기 위한 목록. */
const LEGEND_POSITIONS: Record<PieLegendPosition, true> = {
  bottom: true,
  left: true,
  right: true,
};

function isPieLegendPosition(v: unknown): v is PieLegendPosition {
  return typeof v === 'string' && v in LEGEND_POSITIONS;
}

/** 드래그 오프셋. 수가 아니면 0 — 구 config 에는 이 키가 없다. */
function readOffset(v: unknown): number {
  return typeof v === 'number' && Number.isFinite(v) ? v : 0;
}

/** 시리즈 축이 없는 경로(채널 모드)에서 쓰는 빈 기본값 — 매 렌더 새 객체를 만들지 않는다. */
const EMPTY_SERIES_ENTRIES: ReadonlyMap<string, ChartEntry[]> = new Map();
const EMPTY_SERIES_STYLES: ReadonlyMap<string, StoreSeriesStyle> = new Map();
const EMPTY_SERIES_NAMES: readonly string[] = [];

function parseConfig(config: Record<string, unknown>): PiePanelConfig {
  return {
    channel_name: (config.channel_name as string) ?? '',
    display_field: (config.display_field as string) ?? 'value',
    label_field: (config.label_field as string) ?? 'labels.name',
    agg_func: (config.agg_func as PiePanelConfig['agg_func']) ?? 'sum',
    show_legend: (config.show_legend as boolean | undefined) ?? true,
    // 인식 불가 값은 기본 배치로 접는다 — 구 config 나 손으로 고친 값이 범례를
    // 통째로 날려버리지 않게 한다.
    legend_position: isPieLegendPosition(config.legend_position)
      ? config.legend_position
      : 'bottom',
    legend_show_percentage: (config.legend_show_percentage as boolean | undefined) ?? false,
    legend_show_value: (config.legend_show_value as boolean | undefined) ?? false,
    legend_font_size: resolveFontSize(config.legend_font_size) ?? DEFAULT_PIE_LEGEND_FONT_SIZE,
    legend_font_family: config.legend_font_family as PiePanelConfig['legend_font_family'],
    legend_font_color: resolveFontColor(config.legend_font_color),
    // 저장값에는 성긴 안전 상한만 건다 — 정확한 죄기는 요소 크기를 알아야 하는데 그
    // 값은 레이아웃 후에만 나오므로 드래그 시점에 한다.
    legend_offset_x: clampStoredLegendOffset(config.legend_offset_x),
    legend_offset_y: clampStoredLegendOffset(config.legend_offset_y),
    pie_size: readPanelSize(config.pie_size),
    pie_offset_x: clampPercentOffset(readOffset(config.pie_offset_x)),
    pie_offset_y: clampPercentOffset(readOffset(config.pie_offset_y)),
    show_percentage: (config.show_percentage as boolean | undefined) ?? true,
    show_value: (config.show_value as boolean | undefined) ?? false,
    // 미지정은 상속 — 기본값을 채우면 저장된 패널의 글자 크기가 조용히 바뀐다.
    label_font_size: resolveFontSize(config.label_font_size),
    label_font_family: config.label_font_family as PiePanelConfig['label_font_family'],
    label_font_color: resolveFontColor(config.label_font_color),
    label_position: config.label_position === 'outside' ? 'outside' : 'inside',
    label_min_percent:
      typeof config.label_min_percent === 'number' &&
      Number.isFinite(config.label_min_percent) &&
      config.label_min_percent >= 0
        ? config.label_min_percent
        : DEFAULT_PIE_LABEL_MIN_PERCENT,
    max_points: (config.max_points as number) ?? DEFAULT_MAX_POINTS,
    // SPEC-CHART-002 — 유무가 곧 렌더 경로 스위치다. 기본값을 채우지 않는다.
    series_reduce: config.series_reduce as PiePanelConfig['series_reduce'],
    multi_output_limit: config.multi_output_limit as number | undefined,
  };
}

/** 파이 조각 1개. `fill` 은 다중 출력 경로에서만 채워진다. */
interface PieSlice {
  name: string;
  value: number;
  fill?: string;
}

export default function PieChartPanel({
  panelId: _panelId,
  title,
  config,
  onConfigChange,
  forceEdit = false,
}: PieChartPanelProps) {
  const { t } = useTranslation();
  const showTitle = usePanelTitleVisible();
  // 대시보드 패널에서도 파이·범례를 끌어 배치한다(히트맵과 같은 규칙).
  const edit = usePanelEditMode({
    canEdit: typeof onConfigChange === 'function',
    forced: forceEdit,
    testId: 'pie-chart-edit-toggle',
    below: showTitle,
  });
  const cfg = parseConfig(config);
  const decimals = readDecimalPlaces(config);
  // 조각 라벨은 전체 대비 비중(%)이라 단위와 축이 다르다 — 단위는 툴팁의 실제 값에만
  // 붙인다. 두 자리에 다 붙이면 `35%` 라는 비중 옆에 `12.3kW` 가 같은 뜻처럼 보인다.
  const unit = config.unit as string | undefined;
  // SPEC-WEB-005: data_source 에 따라 Store 소스 또는 채널 소스를 사용한다(공존).
  // SPEC-TSDB-002 §2.3 [U3]: 소스 판정은 `panelDataSource` 계약이 소유한다. 패널은
  // `data_source` 를 직접 비교하지 않는다 — 소스 종류가 늘어도 이 지점이 종류만큼
  // 곱해지지 않게 하기 위함이다(UB1-1).
  // `isStore` 는 "채널이 아닌 시리즈 소스가 활성인가" 를 뜻한다. M3 시점에는 store 만
  // 그 조건을 만족하며, tsdb 는 M6 에서 같은 이름을 통해 합류한다.
  const isStore = isPanelSeriesSource(resolvePanelSourceBinding(config));

  const channelRes = useChartChannel(
    isStore ? undefined : cfg.channel_name || undefined,
    { maxPoints: Math.max(cfg.max_points ?? DEFAULT_MAX_POINTS, 100) },
  );
  const storeRes = usePanelSeriesData(config);

  const { entries, status, closedReason, errorReason } = isStore
    ? storeRes
    : channelRes;

  // 시리즈 축은 Store 경로에만 존재한다.
  const seriesEntries = storeRes.seriesEntries ?? EMPTY_SERIES_ENTRIES;
  const seriesNames = storeRes.seriesNames ?? EMPTY_SERIES_NAMES;
  const seriesStyles = storeRes.seriesStyles ?? EMPTY_SERIES_STYLES;

  // SPEC-CHART-002 §2.9 [S1] — 데이터 파생의 유일한 분기점.
  const isReduceMode = isStore && cfg.series_reduce !== undefined;

  const derived = useMemo<{
    slices: PieSlice[];
    truncated: number;
    negativeOmitted: number;
    reduce: boolean;
  }>(() => {
    if (isReduceMode && cfg.series_reduce !== undefined) {
      // 다중 출력: 조각 = 시리즈, 채움색 = 시리즈 색 ?? 팔레트(§2.6).
      // 팔레트 인덱스는 **필터 전** 시리즈 순서를 쓴다 — 생략된 조각 때문에 나머지 색이
      // 밀려나면 폴링마다 색이 바뀐다.
      const slices: PieSlice[] = [];
      let negativeOmitted = 0;
      reduceAllSeries(seriesEntries, seriesNames, seriesStyles, cfg.series_reduce).forEach(
        (r, i) => {
          // 값 없음: 조각 생략(0 과 구분되지 않는 조각을 그리지 않는다).
          if (r.value === undefined) return;
          // 음수: 파이는 음수를 표현할 수 없다. 절댓값(부호 소실)도 0 clamp(사유 은폐)도
          // 아닌 **생략 + 사유 안내**를 택한다(§4.4 / OQ6).
          if (r.value < 0) {
            negativeOmitted += 1;
            return;
          }
          slices.push({ name: r.name, value: r.value, fill: r.color ?? pickSeriesColor(i) });
        },
      );
      const { visible, truncated } = applyMultiOutputLimit(slices, cfg.multi_output_limit);
      return { slices: visible, truncated, negativeOmitted, reduce: true };
    }

    // --- 레거시 경로(변경 금지 — 특성화 CH-08~CH-10 이 지킨다) ---
    const recent =
      entries.length > (cfg.max_points ?? DEFAULT_MAX_POINTS)
        ? entries.slice(-(cfg.max_points ?? DEFAULT_MAX_POINTS))
        : entries;
    const out = aggregateByLabel(
      recent,
      cfg.label_field ?? 'labels.name',
      cfg.display_field ?? 'value',
      cfg.agg_func ?? 'sum',
    );
    return {
      // 조각 라벨이 시리즈 표시 이름이면(Store 계열 소스의 기본) 그 시리즈에 고른 색을
      // 쓴다. 색을 고르지 않은 조각은 `fill` 이 undefined 로 남아 종전 팔레트 순환을
      // 그대로 따른다(CH-09).
      slices: out.map((r) => ({
        name: r.label,
        value: r.value,
        fill: seriesStyles.get(r.label)?.color,
      })),
      truncated: 0,
      negativeOmitted: 0,
      reduce: false,
    };
  }, [
    isReduceMode,
    cfg.series_reduce,
    cfg.multi_output_limit,
    seriesEntries,
    seriesNames,
    seriesStyles,
    entries,
    cfg.label_field,
    cfg.display_field,
    cfg.agg_func,
    cfg.max_points,
  ]);

  const chartData = derived.slices;

  // 비율은 조각 라벨과 범례가 **같은 값**을 써야 한다 — 각자 계산하면 recharts 의
  // 내부 반올림과 어긋나 조각에는 34%, 범례에는 33% 가 적힌다.
  const percents = useMemo(() => piePercents(chartData.map((s) => s.value)), [chartData]);

  const legendItems = useMemo<PieLegendItem[]>(
    () =>
      chartData.map((slice, i) => ({
        name: slice.name,
        value: slice.value,
        percent: percents[i] ?? 0,
        color: slice.fill ?? PIE_COLORS[i % PIE_COLORS.length]!,
      })),
    [chartData, percents],
  );

  const minPercent = cfg.label_min_percent ?? DEFAULT_PIE_LABEL_MIN_PERCENT;
  const labelOutside = cfg.label_position === 'outside';
  // 지정 크기가 이긴다. 미지정이면 라벨 위치가 정한다 — 바깥 라벨은 지시선 + 글자
  // 폭만큼 파이 밖을 쓰므로, 그만큼 줄이지 않으면 패널 경계에서 잘린다
  // (`96% 418.65GB` 가 `6% GB` 로 보였다).
  const pieSize =
    cfg.pie_size ?? (labelOutside && (cfg.show_percentage || cfg.show_value) ? 62 : 80);

  /**
   * 조각 라벨. 위치에 따라 자리와 기본 글자색이 갈린다.
   *
   * - **안쪽**: 조각이 곧 경계라 패널 밖으로 잘리지 않지만, 좁은 조각에는 글자가
   *   들어가지 않는다. 기본 글자색은 조각 색과 대비되는 색이다.
   * - **바깥**: 좁은 조각도 적을 수 있지만 지시선 + 글자 폭만큼 영역을 더 써서,
   *   파이를 그만큼 줄이지 않으면 패널 경계에서 잘린다. 기본 글자색은 테마 글자색이다.
   *
   * 어느 쪽이든 비율·값은 **한 줄**로 잇는다. 두 줄로 쌓으면 라벨의 세로 높이가 두
   * 배가 되어 나란한 조각의 라벨과 훨씬 쉽게 겹친다.
   *
   * 문자열이 아니라 `<text>` 를 직접 돌려주는 것은 자리·글꼴·크기·색을 모두 지정해야
   * 하기 때문이다. 미지정 항목은 속성을 붙이지 않아 상속된다.
   */
  const renderSliceLabel = useMemo(() => {
    if (!cfg.show_percentage && !cfg.show_value) return undefined;
    return function SliceLabel(props: {
      x?: number;
      y?: number;
      cx?: number;
      cy?: number;
      innerRadius?: number;
      outerRadius?: number;
      midAngle?: number;
      percent?: number;
      value?: number;
      index?: number;
      textAnchor?: 'start' | 'middle' | 'end' | 'inherit';
    }): React.ReactElement | null {
      // 작은 조각은 적지 않는다 — 안쪽에서는 글자가 조각을 넘치고, 바깥에서는 나란한
      // 라벨끼리 겹친다. 값은 범례로 볼 수 있다.
      if (!isPieLabelVisible(props.percent ?? 0, minPercent)) return null;
      const text = pieSliceLabelText(props.percent ?? 0, props.value ?? 0, {
        showPercentage: cfg.show_percentage === true,
        showValue: cfg.show_value === true,
        decimals,
        unit,
      });
      if (text === '') return null;

      // 바깥은 recharts 가 계산해 준 자리를 그대로 쓴다 — 지시선 끝과 글자가 어긋나면
      // 선이 엉뚱한 곳을 가리킨다.
      const at = labelOutside
        ? { x: props.x ?? 0, y: props.y ?? 0 }
        : insideLabelPoint({
            cx: props.cx ?? 0,
            cy: props.cy ?? 0,
            innerRadius: props.innerRadius ?? 0,
            outerRadius: props.outerRadius ?? 0,
            midAngle: props.midAngle ?? 0,
          });
      // 지정색이 이긴다. 미지정이면 배경이 무엇이냐로 갈린다 — 안쪽은 조각 색 위,
      // 바깥은 패널 배경 위다. 조각 색은 범례와 같은 배열에서 읽는다(색 근거를 하나로).
      const fill =
        cfg.label_font_color ??
        (labelOutside ? 'currentColor' : sliceLabelTextColor(legendItems[props.index ?? -1]?.color));
      return (
        <text
          x={at.x}
          y={at.y}
          textAnchor={labelOutside ? (props.textAnchor ?? 'middle') : 'middle'}
          dominantBaseline="central"
          fontFamily={resolveFontFamily(cfg.label_font_family)}
          fontSize={cfg.label_font_size}
          fill={fill}
          data-pie-slice-label=""
        >
          {text}
        </text>
      );
    };
  }, [
    cfg.show_percentage,
    cfg.show_value,
    cfg.label_font_size,
    cfg.label_font_family,
    cfg.label_font_color,
    labelOutside,
    minPercent,
    decimals,
    unit,
    legendItems,
  ]);

  /**
   * 라벨 지시선(바깥 배치 전용). 라벨을 접은 조각은 선도 함께 접는다 — 가리키는
   * 글자 없이 선만 남으면 조각에서 허공으로 뻗은 선이 된다.
   *
   * 접을 때 `null` 대신 빈 `<g>` 를 돌려주는 것은 이 prop 의 타입이 요소를 요구하기
   * 때문이다. 안쪽 배치에서는 `false` 로 두어 선 자체를 만들지 않는다.
   */
  const renderLabelLine = useMemo(() => {
    if (!labelOutside || renderSliceLabel === undefined) return false;
    return function SliceLabelLine(props: {
      points?: Array<{ x: number; y: number }>;
      percent?: number;
      stroke?: string;
    }): React.ReactElement<SVGElement> {
      if (!isPieLabelVisible(props.percent ?? 0, minPercent) || !props.points?.length) {
        return <g /> as React.ReactElement<SVGElement>;
      }
      return (
        <polyline
          className="recharts-pie-label-line"
          points={props.points.map((p) => `${p.x},${p.y}`).join(' ')}
          stroke={props.stroke}
          fill="none"
        />
      ) as React.ReactElement<SVGElement>;
    };
  }, [labelOutside, renderSliceLabel, minPercent]);

  return (
    <div className="relative flex min-h-0 flex-1 flex-col rounded-2xl bg-(--color-bg-surface) p-4 ring-1 ring-(--color-border-default)">
      <div className="absolute right-3 top-3 z-10">
        <ConnectionStatusIcon status={status} />
      </div>
      {edit.toggle}

      {showTitle && (
        <div className="mb-1 flex shrink-0 items-center gap-2 pr-6">
          <PieChartIcon className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
          <span className="truncate text-sm font-semibold text-(--color-text-primary)">
            {title || cfg.channel_name || '채널 미지정'}
          </span>
        </div>
      )}

      {/*
        파이는 **패널 본문 전체**를 쓰고 범례가 그 위에 겹쳐 뜬다. 종전처럼 flex 로
        자리를 나눠 가지면 범례 위치(하단/좌/우)에 따라 남는 폭이 달라져 파이가 따라
        움직였다 — 파이 자리를 옮기려고 범례를 건드리게 되는 결합이다. 겹치는 자리에서는
        범례가 위에 보인다.
      */}
      <PieDragLayer
        enabled={edit.active}
        legend={{
          offsetX: cfg.legend_offset_x ?? 0,
          offsetY: cfg.legend_offset_y ?? 0,
          onChange: ({ x, y }) => onConfigChange?.({ legend_offset_x: x, legend_offset_y: y }),
        }}
        chart={{
          offsetX: cfg.pie_offset_x ?? 0,
          offsetY: cfg.pie_offset_y ?? 0,
          onChange: ({ x, y }) => onConfigChange?.({ pie_offset_x: x, pie_offset_y: y }),
        }}
      >
      <div className="relative min-h-0 min-w-0 flex-1">
      <div
        className="absolute inset-0"
        data-testid="pie-chart-container"
        data-pie-chart-area=""
      >
        <ResponsiveContainer width="100%" height="100%">
          <PieChart>
            <Pie
              data={chartData}
              dataKey="value"
              nameKey="name"
              // 중심은 기본 한가운데에서 오프셋만큼 민다(백분율이라 패널 크기와 무관).
              cx={`${50 + (cfg.pie_offset_x ?? 0)}%`}
              cy={`${50 + (cfg.pie_offset_y ?? 0)}%`}
              outerRadius={`${pieSize}%`}
              label={renderSliceLabel}
              labelLine={renderLabelLine}
              isAnimationActive={false}
            >
              {/*
                조각 색은 **그 조각에 색이 있으면** 그 색으로, 없으면 종전 PIE_COLORS
                순환 배정으로 그린다(CH-09). 다중 출력 경로는 언제나 색이 실리므로
                종전과 같고, 카테고리 경로는 시리즈에 고른 색만 반영된다.
              */}
              {chartData.map((slice, i) => (
                <Cell key={i} fill={slice.fill ?? PIE_COLORS[i % PIE_COLORS.length]} />
              ))}
            </Pie>
            {/*
              조각 라벨(`show_percentage`)은 **전체 대비 비중**이라 이 설정과 다른 축이다.
              비중까지 기본 2자리로 바꾸면 `45%` 가 `45.00%` 가 되어 읽기만 나빠진다.
              자릿수는 값 툴팁에만 적용한다 — 종전에는 포맷터가 없어 원값이 그대로 나왔다.
            */}
            <Tooltip
              contentStyle={{ fontSize: '0.75rem' }}
              formatter={(v: unknown) =>
                typeof v === 'number'
                  ? formatValueWithUnit(v, decimals, unit)
                  : (v as React.ReactNode)
              }
            />
          </PieChart>
        </ResponsiveContainer>
      </div>

      {cfg.show_legend && (
        <PieLegend
          items={legendItems}
          position={cfg.legend_position ?? 'bottom'}
          showPercentage={cfg.legend_show_percentage === true}
          showValue={cfg.legend_show_value === true}
          fontSize={cfg.legend_font_size ?? DEFAULT_PIE_LEGEND_FONT_SIZE}
          fontFamily={cfg.legend_font_family}
          fontColor={cfg.legend_font_color}
          decimals={decimals}
          unit={unit}
          offsetX={cfg.legend_offset_x ?? 0}
          offsetY={cfg.legend_offset_y ?? 0}
        />
      )}
      </div>
      </PieDragLayer>

      {/* 음수 대표값으로 생략된 조각의 사유 안내(§4.4 — 조각이 사라진 이유가 화면에 남는다). */}
      {derived.negativeOmitted > 0 && (
        <p
          data-testid="pie-negative-omitted"
          className="mt-2 shrink-0 text-center text-xs text-(--color-text-muted)"
        >
          {t('dashboard.chart.pieNegativeOmitted').replace(
            '{count}',
            String(derived.negativeOmitted),
          )}
        </p>
      )}

      <MultiOutputTruncationNotice truncated={derived.truncated} />

      {(status === 'closed' || status === 'error') && (
        <div
          className="absolute inset-0 flex items-center justify-center rounded-2xl bg-black/50 p-4 text-center text-sm text-white"
          data-testid="pie-chart-overlay"
        >
          {status === 'closed'
            ? `Channel closed: ${closedReason ?? 'unknown'}`
            : `Error: ${errorReason ?? 'unknown'}`}
        </div>
      )}
    </div>
  );
}
