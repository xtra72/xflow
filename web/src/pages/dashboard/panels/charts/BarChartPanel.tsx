// Bar Chart 패널 (REQ-M4-06).
// 두 모드:
//  - category: label_field 별 최신 값을 막대로 표시
//  - time_bin: bin_sec 간격으로 시간 bin 별 집계 (count/sum/avg)
//
// SPEC-CHART-002: Store 모드 + `series_reduce` 지정 시에는 **시리즈당 막대 1개**를 그리는
// 다중 출력 경로로 갈린다. 이때 `mode` · `label_field` · `agg_func` 는 무시된다(§2.9 [S1]).
// 분기는 아래 `derived` useMemo 진입부 한 곳뿐이며 레거시 계산은 else 가지에 그대로 남는다.

import { useMemo } from 'react';
import { BarChart3 } from 'lucide-react';
import {
  Bar,
  BarChart,
  CartesianGrid,
  Cell,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts';

import {
  DEFAULT_PIE_LEGEND_FONT_SIZE,
  getByPath,
  isPieLegendPosition,
  pickSeriesColor,
  type BarChartPanelConfig,
  type ChartEntry,
} from './chartChannelTypes';
import { PieLegend, type PieLegendItem } from './PieLegend';
import { clampStoredLegendOffset } from './legendOverlay';
import { resolveFontColor, resolveFontSize } from './textStyle';
import {
  PANEL_SIZE_MAX,
  PANEL_SIZE_MIN,
  clampPercentOffset,
  panelBoxTransform,
  readPanelOffset,
  readBarSize,
  readPanelSize,
} from './panelGeometry';
import { ConnectionStatusIcon } from './ConnectionStatusIcon';
import {
  aggregateByTimeBin,
  formatTimeShort,
  toNumber,
} from './chartChannelUtils';
import { reduceAllSeries } from './seriesReduce';
import { MultiOutputTruncationNotice } from './SeriesTileGrid';
import { applyMultiOutputLimit } from './multiOutputLimit';
import { type StoreSeriesStyle } from './useStoreChartData';
import { isPanelSeriesActive } from './panelDataSource';
// 값 표기 자릿수는 차트 계열 공용 규칙을 따른다.
import {
  hasExplicitDecimalPlaces,
  readDecimalPlaces,
} from './decimalPlaces';
import { formatTickValue, formatValueWithUnit } from './unitOptions';
import { usePanelSeriesData } from './usePanelSeriesData';
import { usePanelTitleStyle, usePanelTitleVisible } from '../../panelChromeContext';
import { useTranslation } from '@/lib/i18n';

import { usePanelEditMode } from '../PanelEditToggle';
import {
  StatDragLayer as PanelDragLayer,
  StatResizeHandle as PanelResizeHandle,
  PANEL_EDIT_OUTLINE_CLASS,
  PANEL_SELECTED_OUTLINE_CLASS,
} from '../../PanelDragLayer';
import { PanelEditGrid } from '../../PanelEditGrid';
import { PanelAlignToolbar } from '../../PanelAlignToolbar';
import { usePanelElementEdit } from '../../usePanelElementEdit';

interface BarChartPanelProps {
  panelId: string;
  title?: string;
  config: Record<string, unknown>;
  /** config 를 쓸 콜백. 없으면 배치 편집이 꺼진다. */
  onConfigChange?: (patch: Record<string, unknown>) => void;
  /** 설정 미리보기처럼 **항상** 편집인 자리인가. 그때는 토글을 감춘다. */
  forceEdit?: boolean;
}

/** 끌 수 있는 요소. 바는 그림과 범례 둘이다. */
type BarElementKind = 'plot' | 'legend';
const BAR_ELEMENT_KINDS: readonly BarElementKind[] = ['plot', 'legend'];

const DEFAULT_MAX_POINTS = 20;

/** 시리즈 축이 없는 경로(채널 모드)에서 쓰는 빈 기본값 — 매 렌더 새 객체를 만들지 않는다. */
const EMPTY_SERIES_ENTRIES: ReadonlyMap<string, ChartEntry[]> = new Map();
const EMPTY_SERIES_STYLES: ReadonlyMap<string, StoreSeriesStyle> = new Map();
const EMPTY_SERIES_NAMES: readonly string[] = [];

function parseConfig(config: Record<string, unknown>): BarChartPanelConfig {
  return {
    channel_name: (config.channel_name as string) ?? '',
    display_field: (config.display_field as string) ?? 'value',
    label_field: (config.label_field as string) ?? 'labels.name',
    mode: (config.mode as BarChartPanelConfig['mode']) ?? 'category',
    bin_sec: (config.bin_sec as number) ?? 60,
    agg_func: (config.agg_func as BarChartPanelConfig['agg_func']) ?? 'avg',
    max_points: (config.max_points as number) ?? DEFAULT_MAX_POINTS,
    // SPEC-CHART-002 — 유무가 곧 렌더 경로 스위치다. 기본값을 채우지 않는다.
    series_reduce: config.series_reduce as BarChartPanelConfig['series_reduce'],
    multi_output_limit: config.multi_output_limit as number | undefined,

    // --- SPEC-CHART-005 ---
    // 미지정은 **끔**이다(파이와 다른 기본값 — 바에는 원래 범례가 없었다).
    show_legend: (config.show_legend as boolean | undefined) ?? false,
    // 인식 불가 값은 기본 배치로 접는다 — 손으로 고친 값이 범례를 날려버리지 않게 한다.
    legend_position: isPieLegendPosition(config.legend_position)
      ? config.legend_position
      : 'bottom',
    legend_show_value: (config.legend_show_value as boolean | undefined) ?? true,
    legend_font_size: resolveFontSize(config.legend_font_size) ?? DEFAULT_PIE_LEGEND_FONT_SIZE,
    legend_font_family: config.legend_font_family as BarChartPanelConfig['legend_font_family'],
    legend_font_color: resolveFontColor(config.legend_font_color),
    // 저장값에는 성긴 안전 상한만 건다 — 정확한 죄기는 요소 크기를 알아야 하는데 그
    // 값은 레이아웃 후에만 나오므로 드래그 시점에 한다(파이와 같은 규칙).
    legend_offset_x: clampStoredLegendOffset(config.legend_offset_x),
    legend_offset_y: clampStoredLegendOffset(config.legend_offset_y),
    plot_size: readPanelSize(config.plot_size),
    plot_size_y: readPanelSize(config.plot_size_y),
    bar_size: readBarSize(config.bar_size),
    plot_offset_x: clampPercentOffset(readPanelOffset(config.plot_offset_x)),
    plot_offset_y: clampPercentOffset(readPanelOffset(config.plot_offset_y)),
  };
}

/** 차트에 그려지는 막대 1개. `fill` 은 다중 출력 경로에서만 채워진다. */
interface BarRow {
  label: string;
  value: number;
  fill?: string;
}

/** category 모드: label_field 값별 최신 값 추출 */
function buildCategoryData(
  entries: readonly ChartEntry[],
  labelField: string,
  displayField: string,
): Array<{ label: string; value: number }> {
  // 마지막으로 본 label -> 값 기록 (map 순서 = 삽입 순서)
  const latest = new Map<string, number>();
  for (const e of entries) {
    const lRaw = getByPath(e, labelField);
    const label = lRaw == null ? 'unknown' : String(lRaw);
    const v = toNumber(getByPath(e, displayField));
    latest.set(label, v);
  }
  return Array.from(latest.entries()).map(([label, value]) => ({ label, value }));
}

export default function BarChartPanel({
  panelId: _panelId,
  title,
  config,
  onConfigChange,
  forceEdit = false,
}: BarChartPanelProps) {
  const { t } = useTranslation();
  const showTitle = usePanelTitleVisible();
  const titleStyle = usePanelTitleStyle();
  const cfg = parseConfig(config);
  const decimals = readDecimalPlaces(config);
  // 단위는 축 눈금과 툴팁에 함께 붙는다 — 한쪽에만 붙이면 같은 수가 화면 안에서
  // 서로 다른 것을 가리키는 것처럼 읽힌다.
  const unit = config.unit as string | undefined;
  // SPEC-WEB-005: data_source 에 따라 Store 소스 또는 채널 소스를 사용한다(공존).
  // SPEC-TSDB-002 §2.3 [U3]: 소스 판정은 `panelDataSource` 계약이 소유한다. 패널은
  // `data_source` 를 직접 비교하지 않는다 — 소스 종류가 늘어도 이 지점이 종류만큼
  // 곱해지지 않게 하기 위함이다(UB1-1).
  // `isStore` 는 "채널이 아닌 시리즈 소스가 활성인가" 를 뜻한다. M3 시점에는 store 만
  // 그 조건을 만족하며, tsdb 는 M6 에서 같은 이름을 통해 합류한다.
  // 활성 판정은 소스 **목록 전체**를 본다 — 단일 축 해석기로 판정하면 목록 쪽 인스턴스에만
  // 시리즈가 있는 패널이 비활성으로 보인다.
  const isStore = isPanelSeriesActive(config);

  const storeRes = usePanelSeriesData(config);

  // 채널이 패널 소스에서 빠진 뒤로 데이터는 시리즈 소스 하나로 들어온다.
  const { entries, status, closedReason, errorReason } = storeRes;

  // 시리즈 축은 Store 경로에만 존재한다.
  const seriesEntries = storeRes.seriesEntries ?? EMPTY_SERIES_ENTRIES;
  const seriesNames = storeRes.seriesNames ?? EMPTY_SERIES_NAMES;
  const seriesStyles = storeRes.seriesStyles ?? EMPTY_SERIES_STYLES;

  // SPEC-CHART-002 §2.9 [S1] — 데이터 파생의 유일한 분기점.
  const isReduceMode = isStore && cfg.series_reduce !== undefined;

  const derived = useMemo<{ rows: BarRow[]; truncated: number; reduce: boolean }>(() => {
    if (isReduceMode && cfg.series_reduce !== undefined) {
      // 다중 출력: 카테고리 = 시리즈 표시 이름, 채움색 = 시리즈 색 ?? 팔레트(§2.6).
      // 팔레트 인덱스는 **필터 전** 시리즈 순서를 쓴다 — 값 없는 시리즈가 생략되었다고
      // 나머지 막대의 색이 밀려나면 폴링마다 색이 바뀐다.
      const bars: BarRow[] = [];
      reduceAllSeries(seriesEntries, seriesNames, seriesStyles, cfg.series_reduce).forEach(
        (r, i) => {
          // 대표값이 없는 시리즈는 막대를 생략한다 — 값 없는 막대는 0 과 구분되지 않는다.
          if (r.value === undefined) return;
          bars.push({ label: r.name, value: r.value, fill: r.color ?? pickSeriesColor(i) });
        },
      );
      const { visible, truncated } = applyMultiOutputLimit(bars, cfg.multi_output_limit);
      return { rows: visible, truncated, reduce: true };
    }

    // --- 레거시 경로(변경 금지 — 특성화 CH-05~CH-07 이 지킨다) ---
    const displayField = cfg.display_field ?? 'value';
    if (cfg.mode === 'time_bin') {
      const out = aggregateByTimeBin(
        entries,
        displayField,
        cfg.bin_sec ?? 60,
        cfg.agg_func ?? 'avg',
      );
      const max = cfg.max_points ?? DEFAULT_MAX_POINTS;
      const trimmed = out.length > max ? out.slice(-max) : out;
      return {
        rows: trimmed.map((b) => ({ label: formatTimeShort(b.binStart), value: b.value })),
        truncated: 0,
        reduce: false,
      };
    }
    const cat = buildCategoryData(entries, cfg.label_field ?? 'labels.name', displayField);
    const max = cfg.max_points ?? DEFAULT_MAX_POINTS;
    const rows = cat.length > max ? cat.slice(-max) : cat;
    return {
      // 카테고리 라벨이 시리즈 표시 이름이면(Store 계열 소스의 기본) 그 시리즈에 고른
      // 색을 막대에 입힌다. 시리즈마다 색을 골라 두어도 막대가 전부 같은 색이면
      // 범례와 그림이 어긋나 어느 막대가 어느 시리즈인지 색으로 읽을 수 없다.
      //
      // 값 자체를 바꾸지는 않는다 — 색이 없는 라벨(시간 bin, 채널 소스의 임의
      // 카테고리)은 `fill` 이 undefined 로 남아 종전 하드코딩 색으로 그려진다.
      rows: rows.map((r) => ({ ...r, fill: seriesStyles.get(r.label)?.color })),
      truncated: 0,
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
    cfg.mode,
    cfg.display_field,
    cfg.label_field,
    cfg.bin_sec,
    cfg.agg_func,
    cfg.max_points,
  ]);

  const chartData = derived.rows;
  // 범례 항목 — 파이와 같은 표현을 쓰므로 같은 형태로 맞춘다(SPEC-CHART-005 §5 D3).
  // `percent` 는 합계 대비 비중이다. 바에서는 뜻이 옅어 비중 표시를 내지 않으므로
  // (`showPercentage=false`) 실제로 쓰이지 않지만, 형태를 맞춰 두면 두 패널이 같은
  // 컴포넌트를 그대로 공유한다.
  const legendItems = useMemo<PieLegendItem[]>(() => {
    if (cfg.show_legend !== true) return [];
    const total = chartData.reduce((acc, r) => acc + (Number.isFinite(r.value) ? r.value : 0), 0);
    return chartData.map((row, i) => ({
      name: row.label,
      value: row.value,
      percent: total > 0 ? row.value / total : 0,
      color: row.fill ?? pickSeriesColor(i),
    }));
  }, [cfg.show_legend, chartData]);
  // 행에 색이 하나라도 있으면 행별 Cell 로 그린다. 색이 없는 행은 종전 색으로 남는다.
  const hasRowFill = chartData.some((r) => r.fill !== undefined);

  // --- SPEC-CHART-005: 배치 편집 ---
  const edit = usePanelEditMode({
    canEdit: onConfigChange !== undefined,
    forced: forceEdit,
    testId: 'bar-chart-edit-toggle',
    below: showTitle,
  });
  const plotOffset = { x: cfg.plot_offset_x ?? 0, y: cfg.plot_offset_y ?? 0 };
  const legendOffset = { x: cfg.legend_offset_x ?? 0, y: cfg.legend_offset_y ?? 0 };
  const {
    selection,
    setSelection,
    snap,
    setSnap,
    boundsRef,
    align,
    reset,
  } = usePanelElementEdit<BarElementKind>({
    kinds: BAR_ELEMENT_KINDS,
    enabled: edit.active,
    offsets: { plot: plotOffset, legend: legendOffset },
    writeOffsets: (patches) => {
      const next: Record<string, unknown> = {};
      for (const p of patches) {
        if (p.kind === 'plot') {
          next.plot_offset_x = p.x;
          next.plot_offset_y = p.y;
        } else {
          next.legend_offset_x = p.x;
          next.legend_offset_y = p.y;
        }
      }
      onConfigChange?.(next);
    },
  });
  /**
   * 요소 상자에 붙는 편집 속성. 편집이 꺼져 있으면 DOM 이 종전과 같다.
   *
   * **위치 클래스를 붙이지 않는다** — 파이에서 `relative` 를 함께 실었다가 `absolute`
   * 상자를 무너뜨렸다. 위치는 각 상자가 정한다.
   */
  const editProps = (kind: BarElementKind) =>
    edit.active
      ? { 'data-panel-drag': kind, tabIndex: 0 }
      : {};
  /** 선택 여부에 따른 윤곽 클래스. 편집이 꺼져 있으면 빈 문자열. */
  const outlineOf = (kind: BarElementKind): string =>
    edit.active
      ? selection.has(kind)
        ? PANEL_SELECTED_OUTLINE_CLASS
        : PANEL_EDIT_OUTLINE_CLASS
      : '';

  return (
    <div className="relative flex min-h-0 flex-1 flex-col rounded-2xl bg-(--color-bg-surface) p-4 ring-1 ring-(--color-border-default)">
      <div className="absolute right-3 top-3 z-10">
        <ConnectionStatusIcon status={status} />
      </div>

      {/* 배치 편집 토글 — 대시보드 편집모드에서만, 미리보기에서는 감춘다. */}
      {edit.toggle}

      {showTitle && (
        <div className="mb-1 flex shrink-0 items-center gap-2 pr-6">
          <BarChart3 className="h-4 w-4 shrink-0 text-(--color-text-muted)" />
          <span className="truncate text-sm font-semibold text-(--color-text-primary)" style={titleStyle}>
            {title || cfg.channel_name || '채널 미지정'}
          </span>
        </div>
      )}
      {/*
        부제: 채널 경로에서만 채널 이름을 앞에 붙인다. 시리즈 소스(store/tsdb) 패널은
        `channel_name` 이 비어 있어 "채널 미지정" 으로 떨어지는데, 그 패널은 애초에 채널을
        쓰지 않으므로 설정이 빠진 것처럼 읽히는 잘못된 안내가 된다.
      */}
      <div className="mb-2 truncate pr-6 text-xs font-medium text-(--color-text-muted)">
        {!isStore && `${cfg.channel_name || '채널 미지정'} · `}
        {derived.reduce ? cfg.series_reduce : cfg.mode}
      </div>

      {/*
        그림 영역. 범례가 이 안에 **겹쳐** 뜨므로 기준 상자가 필요하다 — 흐름에 끼워
        넣으면 범례가 자리를 나눠 가져 막대 폭이 범례 위치에 따라 달라진다(파이와 같은
        이유). 플롯 상자는 옮긴 뒤 키운다(`panelBoxTransform`).
      */}
      <PanelDragLayer<BarElementKind>
        enabled={edit.active}
        snap={snap}
        selection={selection}
        onSelectionChange={setSelection}
        targets={{
          plot: {
            offsetX: plotOffset.x,
            offsetY: plotOffset.y,
            // 바는 사각형이라 손잡이가 축을 나눈다 — 가로는 **그림 영역 폭**, 세로는
            // 그림 영역 높이다(둘 다 패널 대비 %).
            //
            // 손잡이는 도형의 상자를 잡는 조작이므로 막대 굵기에 걸지 않는다. 굵기는
            // 도형의 폭이 아니라 그 안의 막대 하나하나에 대한 값이라 뜻이 다르고,
            // 설정의 슬라이더가 따로 맡는다.
            fontSize: cfg.plot_size ?? PANEL_SIZE_MAX,
            sizeRange: { min: PANEL_SIZE_MIN, max: PANEL_SIZE_MAX },
            onMove: ({ x, y }) => onConfigChange?.({ plot_offset_x: x, plot_offset_y: y }),
            onResize: (v) => onConfigChange?.({ plot_size: readPanelSize(v) ?? PANEL_SIZE_MAX }),
            sizeY: cfg.plot_size_y ?? cfg.plot_size ?? PANEL_SIZE_MAX,
            sizeYRange: { min: PANEL_SIZE_MIN, max: PANEL_SIZE_MAX },
            onResizeY: (v) =>
              onConfigChange?.({ plot_size_y: readPanelSize(v) ?? PANEL_SIZE_MAX }),
          },
          legend: {
            offsetX: legendOffset.x,
            offsetY: legendOffset.y,
            fontSize: cfg.legend_font_size ?? DEFAULT_PIE_LEGEND_FONT_SIZE,
            onMove: ({ x, y }) => onConfigChange?.({ legend_offset_x: x, legend_offset_y: y }),
            onResize: (legend_font_size) => onConfigChange?.({ legend_font_size }),
          },
        }}
      >
      <div ref={boundsRef} className="relative min-h-0 flex-1" data-panel-bounds="">
      {/* 배치 그리드와 중심 표식 — 요소 뒤에 깔리고 포인터를 받지 않는다. */}
      <PanelEditGrid enabled={edit.active} />
      <div
        data-testid="bar-chart-container"
        {...editProps('plot')}
        // 손잡이가 모서리에 붙으려면 기준이 필요하다 — 이 상자는 흐름에 있으므로
        // 여기서 `relative` 를 준다(파이 그림 상자는 이미 absolute 라 필요 없다).
        className={`relative h-full w-full ${outlineOf('plot')}`}
        style={{
          transform: panelBoxTransform(
            cfg.plot_size ?? PANEL_SIZE_MAX,
            cfg.plot_offset_x ?? 0,
            cfg.plot_offset_y ?? 0,
            cfg.plot_size_y,
          ),
        }}
      >
        <ResponsiveContainer width="100%" height="100%">
          <BarChart data={chartData} margin={{ top: 8, right: 16, left: 0, bottom: 0 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="#e5e7eb" />
            <XAxis dataKey="label" tick={{ fontSize: 10 }} stroke="#9ca3af" />
            {/* 축 눈금은 자릿수를 **명시했을 때만** 따른다 — 기본값을 걸면 아무 설정도
                안 한 패널의 눈금이 `0.00 · 25.00` 이 된다(`decimalPlaces.ts` 머리말). */}
            <YAxis
              tick={{ fontSize: 10 }}
              stroke="#9ca3af"
              width={50}
              // 눈금은 눈금자다 — 단위를 붙이고 자릿수는 그 눈금의 크기가 정한다
              // (`formatTickValue`). 사용자가 자릿수를 명시했으면 그 값이 이긴다.
              tickFormatter={(v: number) =>
                formatTickValue(
                  v,
                  unit,
                  hasExplicitDecimalPlaces(config) ? decimals : undefined,
                )
              }
            />
            {/* 종전에는 포맷터가 없어 원값(21.533333333333335)이 툴팁에 그대로 나왔다. */}
            <Tooltip
              contentStyle={{ fontSize: '0.75rem' }}
              formatter={(v: unknown) =>
                typeof v === 'number'
                  ? formatValueWithUnit(v, decimals, unit)
                  : (v as React.ReactNode)
              }
            />
            {/*
              막대 색은 **행에 색이 있으면** 행별 Cell 로, 없으면 종전 하드코딩 색으로
              그린다(CH-07). 판정을 `derive.reduce` 가 아니라 행의 `fill` 유무로 두는
              이유: 다중 출력이 아니어도 카테고리가 시리즈일 때는 시리즈 색이 실린다.
              두 축(값 색 / 시리즈 색)이 섞이지 않도록 색을 쓸 때는 Bar 레벨 fill 을
              아예 비운다.
            */}
            <Bar
              dataKey="value"
              fill={hasRowFill ? undefined : '#3b82f6'}
              isAnimationActive={false}
              // 미지정이면 recharts 가 칸 폭에 맞춰 자동으로 정한다(종전 동작).
              barSize={cfg.bar_size}
            >
              {hasRowFill
                ? chartData.map((row, i) => <Cell key={i} fill={row.fill ?? '#3b82f6'} />)
                : null}
            </Bar>
          </BarChart>
        </ResponsiveContainer>
        {edit.active && selection.has('plot') && (
          <PanelResizeHandle kind="plot" enabled label={t('dashboard.chart.plotSize')} />
        )}
      </div>
      {cfg.show_legend === true && legendItems.length > 0 && (
        <PieLegend
          items={legendItems}
          position={cfg.legend_position ?? 'bottom'}
          // 비중은 내지 않는다 — 막대는 합계 대비 비중을 읽는 그림이 아니다.
          showPercentage={false}
          showValue={cfg.legend_show_value !== false}
          fontSize={cfg.legend_font_size ?? DEFAULT_PIE_LEGEND_FONT_SIZE}
          fontFamily={cfg.legend_font_family}
          fontColor={cfg.legend_font_color}
          decimals={decimals}
          unit={unit}
          offsetX={cfg.legend_offset_x ?? 0}
          offsetY={cfg.legend_offset_y ?? 0}
          // 표식은 범례 **자신**에 붙는다(파이와 같은 이유 — 감싸는 상자는 크기가 0이다).
          edit={
            edit.active
              ? {
                  kind: 'legend',
                  selected: selection.has('legend'),
                  outlineClass: outlineOf('legend'),
                  overlay: selection.has('legend') ? (
                    <PanelResizeHandle
                      kind="legend"
                      enabled
                      label={t('dashboard.chart.legendElement')}
                    />
                  ) : null,
                }
              : undefined
          }
        />
      )}
      </div>
      </PanelDragLayer>

      {/* 정렬 툴바 — 편집 중에만. */}
      <PanelAlignToolbar
        enabled={edit.active}
        snap={snap}
        onSnapChange={setSnap}
        onAlign={align}
        onReset={reset}
      />

      <MultiOutputTruncationNotice truncated={derived.truncated} />

      {(status === 'closed' || status === 'error') && (
        <div
          className="absolute inset-0 flex items-center justify-center rounded-2xl bg-black/50 p-4 text-center text-sm text-white"
          data-testid="bar-chart-overlay"
        >
          {status === 'closed'
            ? `Channel closed: ${closedReason ?? 'unknown'}`
            : `Error: ${errorReason ?? 'unknown'}`}
        </div>
      )}
    </div>
  );
}
