// 항목 표시 옵션 — 패널 기본값 + 항목별 덮어쓰기.
//
// 규약은 `panels/charts/graphStyle.ts` 의 resolveSeriesStyle 과 같다: 항목 값이
// `undefined` 인 것과 실제 값인 것은 다르다. 전자는 "패널을 따름" 이라 패널
// 기본값을 바꾸면 함께 바뀌고, 후자는 그 항목에 고정된다. 항목이 넷인 패널에서
// 높이를 한 번에 바꿀 수 있어야 하기 때문이다.
//
// 저장 형태:
//
//   config.style / height / legend / windowSec        ← 패널 기본값
//   config.itemOptions.<itemKey>.{style,height,...}   ← 항목별 덮어쓰기

import type { LegendPosition } from '@/pages/monitoring/MultiSeriesChart';
import {
  GRAPH_STYLES,
  hasStrokeStyle,
  isStackable,
  type GraphStyle,
} from '@/pages/dashboard/panels/charts/graphStyle';
import {
  GAUGE_TYPES,
  getThresholdColor,
  type GaugeType,
  type ThresholdEntry,
} from '@/pages/dashboard/panels/gauge/gaugeShapes';

/**
 * 항목 하나를 그리는 모양.
 *
 * 차트 계열은 `charts/graphStyle.ts` 의 `GraphStyle` 을 그대로 쓴다 — 차트 패널과
 * 같은 어휘여야 설정 화면이 두 곳에서 다른 말을 하지 않는다. 캔들은 버킷마다
 * 시·고·저·종이 필요한데 시스템 지표에는 그런 축이 없어 제외한다.
 */
export type SysMetricsStyle = 'tile' | 'gauge' | 'progress' | Exclude<GraphStyle, 'candle'>;

/** 시계열 스타일 (차트 패널의 GraphStyle 중 캔들을 뺀 것) */
const SERIES_STYLES = GRAPH_STYLES.filter(
  (g): g is Exclude<GraphStyle, 'candle'> => g !== 'candle',
);

/** 고를 수 있는 스타일 (설정 UI 표시 순서) */
export const SYSMETRICS_STYLES: readonly SysMetricsStyle[] = [
  'tile',
  'gauge',
  'progress',
  ...SERIES_STYLES,
];

/**
 * 시계열 창(window)이 필요한 스타일인가 — 지금 값 하나로는 그릴 수 없다.
 *
 * 타입 가드로 둔다. 호출부가 이 검사를 통과한 값을 차트 컴포넌트에 그대로 넘기는데,
 * boolean 을 돌려주면 타입이 좁혀지지 않아 호출부마다 단언을 쓰게 된다.
 */
export function isTimeSeriesStyle(
  style: SysMetricsStyle,
): style is Exclude<GraphStyle, 'candle'> {
  return (SERIES_STYLES as readonly string[]).includes(style);
}

/** 0~100 비율이 있어야 뜻이 통하는 스타일인가. */
export function isRatioStyle(style: SysMetricsStyle): boolean {
  return style === 'gauge' || style === 'progress';
}

/**
 * 항목의 값 성격.
 *
 *   ratio   — 0~100 비율 (CPU 사용률, 메모리 사용률, 디스크 사용률)
 *   counter — 부팅 이후 누적 카운터에서 뽑은 단위시간당 증가량 (네트워크·디스크 I/O)
 *
 * 이 구분이 고를 수 있는 스타일을 가른다. 상한이 없는 증가량에 게이지를 그리면
 * 바늘이 무엇을 가리키는지 아무도 말할 수 없다.
 */
export type SysMetricValueKind = 'ratio' | 'counter';

/**
 * 누적 카운터를 어떻게 보여줄 것인가.
 *
 *   rate  — 두 표본의 차이를 단위시간당 증가량으로 환산한다(기본).
 *   total — 부팅 이후 누적값을 그대로 보여준다.
 *
 * `rate: false` 인 값(비율·용량)에는 뜻이 없다 — 그 값들은 애초에 그 시점의 상태값이라
 * 환산할 것이 없다. 설정 UI 는 `SysMetricField.rate` 가 참인 항목에만 이 선택을 낸다.
 *
 * 기본이 `rate` 인 이유는 누적값이 언제나 커지기만 하는 수여서 추이를 읽을 수 없기
 * 때문이다. 총량이 필요한 경우(정산·쿼터 확인)에만 `total` 을 고른다.
 */
export type SysMetricsCounterMode = 'rate' | 'total';

/** 고를 수 있는 표시 방식 (설정 UI 표시 순서) */
export const SYSMETRICS_COUNTER_MODES: readonly SysMetricsCounterMode[] = ['rate', 'total'];

/** 그 값 성격에 뜻이 통하는 스타일인가. */
export function supportsStyle(kind: SysMetricValueKind, style: SysMetricsStyle): boolean {
  if (kind === 'counter' && isRatioStyle(style)) return false;
  return true;
}

/** 값 성격에 맞는 스타일 목록 (설정 UI 가 고를 수 있는 것만 보여준다). */
export function stylesFor(kind: SysMetricValueKind): SysMetricsStyle[] {
  return SYSMETRICS_STYLES.filter((s) => supportsStyle(kind, s));
}

/**
 * 패널 유형이 담는 값 성격.
 *
 * 네트워크 패널은 채널이 모두 누적 카운터에서 뽑은 증가량이라 비율이 없다. 시스템
 * 패널은 CPU·메모리(비율)와 디스크 I/O·네트워크(증가량)가 섞여 있다.
 */
const PANEL_VALUE_KINDS: Record<string, SysMetricValueKind[]> = {
  'sysmetrics-system': ['ratio', 'counter'],
  'sysmetrics-network': ['counter'],
  'sysmetrics-storage': ['ratio'],
};

/**
 * 그 패널에서 고를 수 있는 스타일 목록.
 *
 * 담긴 값 성격 중 하나라도 지원하면 제시한다 — 시스템 패널의 게이지는 CPU·메모리에
 * 적용되고 증가량 항목에서는 타일로 떨어진다.
 *
 * **고를 수 없는 것을 제시하지 않는 것이 요점이다.** 예전에는 네트워크 패널에서도
 * 게이지를 고를 수 있었는데, 채널이 전부 증가량이라 그리는 쪽이 전부 타일로 되돌렸다 —
 * 사용자에게는 "골랐는데 아무 일도 안 일어남" 으로 보였다.
 */
export function stylesForPanel(panelType: string): SysMetricsStyle[] {
  const kinds = PANEL_VALUE_KINDS[panelType] ?? (['ratio', 'counter'] as SysMetricValueKind[]);
  return SYSMETRICS_STYLES.filter((style) => kinds.some((kind) => supportsStyle(kind, style)));
}

// --- 옵션 ---

/**
 * 항목 하나의 확정된 표시 옵션.
 *
 * 차트·게이지 세부 항목의 이름은 차트 패널·게이지 패널이 쓰는 config 키와 같다
 * (`smooth` / `stacked` / `gaugeType` / `min` / `max` / `unit`). 이름이 갈리면
 * 같은 설정을 두 어휘로 배워야 한다.
 */
export interface SysMetricsItemOptions {
  style: SysMetricsStyle;
  /**
   * 누적 카운터의 표시 방식. `rate: false` 인 항목에서는 쓰이지 않는다.
   *
   * 값 성격이 런타임에 갈리므로 단위 표기(`/s` 를 붙일지)와 시계열 창의 계열도 이
   * 값을 따라가야 한다 — 증가량과 누적값은 자릿수가 달라 한 축에 섞이지 않는다.
   */
  counterMode: SysMetricsCounterMode;
  /**
   * 차트·게이지 높이(px).
   *
   * **미지정이면 칸을 채운다.** 고정 px 를 기본으로 두면 패널을 크게 늘려도 항목은
   * 그대로여서 아래가 빈 채로 남는다 — 대시보드에서 패널 크기를 조절하는 이유가
   * 사라진다. 값을 넣으면 그 높이에 고정된다(여러 패널의 높이를 맞출 때 쓴다).
   */
  height?: number;
  legend: LegendPosition;
  /** 시계열 표시 구간(초). 시계열 스타일에서만 쓴다. */
  windowSec: number;
  /** 곡선 보간 (차트 패널의 `smooth`) */
  smooth: boolean;
  /** 계열 누적 (차트 패널의 `stacked`). 영역·막대에서만 뜻이 있다. */
  stacked: boolean;
  /** 게이지 모양 8종 (게이지 패널의 `gaugeType`) */
  gaugeType: GaugeType;
  /** 게이지 눈금 범위·단위 (게이지 패널과 동일) */
  min: number;
  max: number;
  unit: string;

  // --- 타일 표시 ---
  /** 가로 정렬 */
  align: TileAlign;
  /** 값 글자 크기 */
  valueSize: TileTextSize;
  /** 값 색. 미지정이면 기본 글자색(또는 임계값 색). */
  valueColor?: string;
  /** 라벨 글자 크기 */
  labelSize: TileTextSize;
  /** 라벨 굵기 */
  labelWeight: TileTextWeight;
  /**
   * 값 범위별 색 (게이지 패널의 `thresholds` 와 같은 형상).
   *
   * 값이 어느 구간에 들어가면 그 색으로 그린다 — 90% 를 넘긴 디스크가 다른 값과
   * 같은 색이면 눈에 걸리지 않는다. 구간에 들지 않으면 `valueColor` 를 따른다.
   */
  thresholds: ThresholdEntry[];
}

/** 타일 가로 정렬 */
export type TileAlign = 'left' | 'center' | 'right';
export const TILE_ALIGNS: readonly TileAlign[] = ['left', 'center', 'right'];

/** 글자 크기 단계 */
export type TileTextSize = 'sm' | 'md' | 'lg' | 'xl' | '2xl';
export const TILE_TEXT_SIZES: readonly TileTextSize[] = ['sm', 'md', 'lg', 'xl', '2xl'];

/** 글자 굵기 */
export type TileTextWeight = 'normal' | 'medium' | 'bold';
export const TILE_TEXT_WEIGHTS: readonly TileTextWeight[] = ['normal', 'medium', 'bold'];

/** 아무 설정도 없을 때의 값 */
export const DEFAULT_ITEM_OPTIONS: SysMetricsItemOptions = {
  style: 'tile',
  // 누적값은 커지기만 하는 수라 추이를 읽을 수 없다 — 기본은 증가량이다.
  counterMode: 'rate',
  // 미지정 = 칸 채움 (위 height 주석 참조)
  height: undefined,
  legend: 'bottom',
  windowSec: 300,
  smooth: false,
  stacked: false,
  gaugeType: 'simple',
  // 시스템 지표의 비율은 언제나 0~100% 다. 게이지 패널은 임의 범위를 받지만
  // 여기서는 이 값이 자연스러운 기본이고, 필요하면 설정에서 바꾼다.
  min: 0,
  max: 100,
  unit: '%',
  align: 'left',
  valueSize: 'xl',
  valueColor: undefined,
  labelSize: 'sm',
  labelWeight: 'normal',
  thresholds: [],
};

/**
 * 패널 유형별 기본 스타일.
 *
 * 유형마다 어울리는 모양이 다르다 — 네트워크는 추이가 중요하니 라인, 스토리지는
 * 사용률이라 진행 막대, 시스템은 지금 값이라 타일이다.
 *
 * **이 표가 단일 출처다.** 예전에는 각 패널이 호출 인자로 자기 기본값을 넘겼는데,
 * 설정 화면의 미리보기는 그 인자를 알 수 없어 늘 타일을 그렸다 — 스토리지 패널은
 * 진행 막대로 나오는데 미리보기만 타일이었다. 표로 올려 셋이 같은 답을 보게 한다.
 */
export const PANEL_STYLE_DEFAULTS: Record<string, SysMetricsStyle> = {
  'sysmetrics-system': 'tile',
  'sysmetrics-network': 'line',
  'sysmetrics-storage': 'progress',
};

/** 패널 유형의 기본 스타일. 모르는 유형은 내장 기본값. */
export function panelStyleDefault(panelType: string): SysMetricsStyle {
  return PANEL_STYLE_DEFAULTS[panelType] ?? DEFAULT_ITEM_OPTIONS.style;
}

/**
 * 스타일마다 뜻이 있는 설정 항목.
 *
 * 타일은 차트가 아니라 높이·범례·구간이 아무것도 하지 않고, 게이지는 높이만 쓴다.
 * 그런데도 입력 칸을 보여 주면 사용자는 바꿔 놓고 왜 안 변하는지 찾아 헤맨다.
 */
export interface OptionFieldSupport {
  height: boolean;
  legend: boolean;
  window: boolean;
  /** 곡선 — 차트 패널과 같은 규칙(선 모양이 뜻을 갖는 스타일에서만) */
  smooth: boolean;
  /** 누적 — 차트 패널과 같은 규칙(영역·막대에서만) */
  stacked: boolean;
  /** 게이지 모양·눈금 범위 */
  gauge: boolean;
  /** 타일 표시(정렬·글자·색). 타일·진행 막대에서만 뜻이 있다. */
  tile: boolean;
}

/**
 * 그 스타일에서 쓰이는 설정 항목.
 *
 * 곡선·누적의 판정은 차트 패널이 쓰는 `graphStyle.ts` 의 규칙을 그대로 부른다 —
 * 여기서 다시 적으면 두 화면이 다른 답을 낸다.
 */
export function optionFieldsFor(style: SysMetricsStyle): OptionFieldSupport {
  const none = {
    height: false,
    legend: false,
    window: false,
    smooth: false,
    stacked: false,
    gauge: false,
    tile: false,
  };
  if (isTimeSeriesStyle(style)) {
    return {
      ...none,
      height: true,
      legend: true,
      window: true,
      smooth: hasStrokeStyle(style),
      stacked: isStackable(style),
    };
  }
  // 게이지는 세로 공간을 차지하므로 높이가 뜻을 갖는다. 범례·구간은 계열이 없어 무의미하다.
  if (style === 'gauge') return { ...none, height: true, gauge: true };
  // 타일·진행 막대는 내용 높이를 쓰고, 글자·정렬·색이 뜻을 갖는다.
  return { ...none, tile: true };
}

/** 높이 허용 범위(px) — 너무 낮으면 축이 겹치고, 너무 높으면 한 화면을 넘긴다. */
export const MIN_ITEM_HEIGHT = 80;
export const MAX_ITEM_HEIGHT = 800;

const LEGEND_POSITIONS: readonly LegendPosition[] = ['none', 'top', 'bottom', 'right'];

/** 문자열을 스타일로 읽는다. 모르는 값은 폴백. */
export function readStyle(v: unknown, fallback: SysMetricsStyle): SysMetricsStyle {
  return (SYSMETRICS_STYLES as readonly string[]).includes(v as string)
    ? (v as SysMetricsStyle)
    : fallback;
}

/** 목록에 있는 값만 통과시킨다. 모르는 값은 폴백. */
function readEnum<T extends string>(v: unknown, choices: readonly T[], fallback: T): T {
  return (choices as readonly string[]).includes(v as string) ? (v as T) : fallback;
}

/**
 * 값 범위 목록을 읽는다.
 *
 * 형상은 게이지 패널의 `thresholds` 와 같다 — 같은 개념을 두 모양으로 저장하면
 * 나중에 한쪽만 고치게 된다. 모양이 어긋난 항목은 버린다.
 */
function readThresholds(v: unknown, fallback: ThresholdEntry[]): ThresholdEntry[] {
  if (!Array.isArray(v)) return fallback;
  const out: ThresholdEntry[] = [];
  for (const raw of v) {
    if (!raw || typeof raw !== 'object') continue;
    const e = raw as Record<string, unknown>;
    if (typeof e.color !== 'string') continue;
    if (typeof e.from !== 'number' || typeof e.to !== 'number') continue;
    out.push({
      name: typeof e.name === 'string' ? e.name : '',
      color: e.color,
      from: e.from,
      to: e.to,
    });
  }
  return out;
}

/**
 * 값에 적용할 색 — 범위에 들면 그 색, 아니면 지정 색.
 *
 * 판정은 게이지 패널과 같은 함수(`getThresholdColor`)를 쓴다.
 */
export function resolveValueColor(
  value: number | undefined,
  options: SysMetricsItemOptions,
): string | undefined {
  if (value === undefined || options.thresholds.length === 0) return options.valueColor;
  return getThresholdColor(value, options.thresholds, options.valueColor ?? '');
    }

/** 문자열을 범례 위치로 읽는다. 모르는 값은 폴백. */
export function readLegend(v: unknown, fallback: LegendPosition): LegendPosition {
  return (LEGEND_POSITIONS as readonly string[]).includes(v as string)
    ? (v as LegendPosition)
    : fallback;
}

/** 게이지 모양을 읽는다. 모르는 값은 폴백. */
export function readGaugeType(v: unknown, fallback: GaugeType): GaugeType {
  return (GAUGE_TYPES as readonly string[]).includes(v as string) ? (v as GaugeType) : fallback;
}

/** 불리언을 읽는다. 불리언이 아니면 폴백 (미지정과 false 를 구분하려는 곳에서 쓴다). */
function readBool(v: unknown, fallback: boolean): boolean {
  return typeof v === 'boolean' ? v : fallback;
}

/** 수를 범위 안으로 죈다. 수가 아니면 폴백. */
function readBounded(v: unknown, fallback: number, min: number, max: number): number {
  if (typeof v !== 'number' || !Number.isFinite(v)) return fallback;
  return Math.min(Math.max(Math.round(v), min), max);
}

/**
 * 높이를 읽는다. 수가 아니면 폴백을 그대로 돌려준다(undefined 포함).
 *
 * `readBounded` 와 달리 undefined 를 살려야 한다 — undefined 는 "칸 채움" 이라는
 * 뜻이지 "값 없음" 이 아니다.
 */
function readHeight(v: unknown, fallback: number | undefined): number | undefined {
  if (typeof v !== 'number' || !Number.isFinite(v)) return fallback;
  return Math.min(Math.max(Math.round(v), MIN_ITEM_HEIGHT), MAX_ITEM_HEIGHT);
}

/** 패널 기본값을 읽는다. */
export function readPanelOptions(
  config: Record<string, unknown> | undefined,
  panelType?: string,
  fallback: Partial<SysMetricsItemOptions> = {},
): SysMetricsItemOptions {
  const base = {
    ...DEFAULT_ITEM_OPTIONS,
    ...(panelType ? { style: panelStyleDefault(panelType) } : {}),
    ...fallback,
  };
  return {
    style: readStyle(config?.style, base.style),
    counterMode: readEnum(config?.counterMode, SYSMETRICS_COUNTER_MODES, base.counterMode),
    height: readHeight(config?.height, base.height),
    legend: readLegend(config?.legend, base.legend),
    windowSec: readBounded(config?.windowSec, base.windowSec, 10, 86_400),
    smooth: readBool(config?.smooth, base.smooth),
    stacked: readBool(config?.stacked, base.stacked),
    gaugeType: readGaugeType(config?.gaugeType, base.gaugeType),
    min: readBounded(config?.min, base.min, -1e12, 1e12),
    max: readBounded(config?.max, base.max, -1e12, 1e12),
    unit: typeof config?.unit === 'string' ? config.unit : base.unit,
    align: readEnum(config?.align, TILE_ALIGNS, base.align),
    valueSize: readEnum(config?.valueSize, TILE_TEXT_SIZES, base.valueSize),
    valueColor: typeof config?.valueColor === 'string' ? config.valueColor : base.valueColor,
    labelSize: readEnum(config?.labelSize, TILE_TEXT_SIZES, base.labelSize),
    labelWeight: readEnum(config?.labelWeight, TILE_TEXT_WEIGHTS, base.labelWeight),
    thresholds: readThresholds(config?.thresholds, base.thresholds),
  };
}

/** 항목별 덮어쓰기 묶음을 읽는다. */
function readOverride(
  config: Record<string, unknown> | undefined,
  itemKey: string,
): Record<string, unknown> {
  const all = config?.itemOptions;
  if (!all || typeof all !== 'object' || Array.isArray(all)) return {};
  const one = (all as Record<string, unknown>)[itemKey];
  if (!one || typeof one !== 'object' || Array.isArray(one)) return {};
  return one as Record<string, unknown>;
}

/**
 * 항목 하나의 확정 옵션.
 *
 * 항목이 지정하지 않은 값은 패널 기본값을 따른다. 값 성격에 맞지 않는 스타일이
 * 저장되어 있으면(예: 네트워크 항목에 게이지) 타일로 떨어뜨린다 — 저장은 남겨 둔 채
 * 그리기만 안전한 쪽으로 간다. 설정 UI 가 애초에 고를 수 없게 하지만, 패널 유형을
 * 바꾸거나 손으로 편집한 config 가 들어올 수 있다.
 */
export function resolveItemOptions(
  config: Record<string, unknown> | undefined,
  itemKey: string,
  kind: SysMetricValueKind,
  panelType?: string,
): SysMetricsItemOptions {
  const panel = readPanelOptions(config, panelType);
  const override = readOverride(config, itemKey);

  const style = readStyle(override.style, panel.style);
  return {
    style: supportsStyle(kind, style) ? style : 'tile',
    counterMode: readEnum(override.counterMode, SYSMETRICS_COUNTER_MODES, panel.counterMode),
    height: readHeight(override.height, panel.height),
    legend: readLegend(override.legend, panel.legend),
    windowSec: readBounded(override.windowSec, panel.windowSec, 10, 86_400),
    smooth: readBool(override.smooth, panel.smooth),
    stacked: readBool(override.stacked, panel.stacked),
    gaugeType: readGaugeType(override.gaugeType, panel.gaugeType),
    min: readBounded(override.min, panel.min, -1e12, 1e12),
    max: readBounded(override.max, panel.max, -1e12, 1e12),
    unit: typeof override.unit === 'string' ? override.unit : panel.unit,
    align: readEnum(override.align, TILE_ALIGNS, panel.align),
    valueSize: readEnum(override.valueSize, TILE_TEXT_SIZES, panel.valueSize),
    valueColor: typeof override.valueColor === 'string' ? override.valueColor : panel.valueColor,
    labelSize: readEnum(override.labelSize, TILE_TEXT_SIZES, panel.labelSize),
    labelWeight: readEnum(override.labelWeight, TILE_TEXT_WEIGHTS, panel.labelWeight),
    thresholds: readThresholds(override.thresholds, panel.thresholds),
  };
}

/**
 * 항목별 덮어쓰기를 갱신한 `itemOptions` 맵을 만든다 (설정 UI 용).
 *
 * 값이 `undefined` 면 그 키를 지워 "패널을 따름"으로 되돌린다 — 덮어쓰기를 해제할
 * 방법이 없으면 한 번 고른 값에 갇힌다. 덮어쓰기가 모두 사라진 항목은 항목 자체를
 * 지워 config 가 빈 껍데기로 불어나지 않게 한다.
 */
export function withItemOverride(
  config: Record<string, unknown> | undefined,
  itemKey: string,
  patch: Partial<Record<keyof SysMetricsItemOptions, unknown>>,
): Record<string, unknown> {
  const all = { ...(readAllOverrides(config)) };
  const current = { ...(all[itemKey] ?? {}) };

  for (const [key, value] of Object.entries(patch)) {
    if (value === undefined || value === '') delete current[key];
    else current[key] = value;
  }

  if (Object.keys(current).length === 0) delete all[itemKey];
  else all[itemKey] = current;

  return all;
}

/** 저장된 항목별 덮어쓰기 전체를 읽는다. */
export function readAllOverrides(
  config: Record<string, unknown> | undefined,
): Record<string, Record<string, unknown>> {
  const all = config?.itemOptions;
  if (!all || typeof all !== 'object' || Array.isArray(all)) return {};
  const out: Record<string, Record<string, unknown>> = {};
  for (const [key, value] of Object.entries(all as Record<string, unknown>)) {
    if (value && typeof value === 'object' && !Array.isArray(value)) {
      out[key] = value as Record<string, unknown>;
    }
  }
  return out;
}

/** 그 항목이 패널 기본값을 덮어쓰고 있는가 (설정 UI 표시용). */
export function hasOverride(
  config: Record<string, unknown> | undefined,
  itemKey: string,
): boolean {
  return Object.keys(readAllOverrides(config)[itemKey] ?? {}).length > 0;
}
