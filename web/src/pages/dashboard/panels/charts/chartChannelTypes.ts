// 차트 패널 공통 타입 + 값 추출 유틸.
// SPEC-CHART-001 §4.1 / §4.2.2 참조.

/** 단일 차트 항목 (WS 로 전송되는 entry) */
export interface ChartEntry {
  /** epoch milliseconds (int64) */
  timestamp: number;
  /** 값 — 숫자/문자열/객체 모두 허용 */
  value: unknown;
  /** 선택적 라벨 (bar/pie 의 카테고리) */
  labels?: Record<string, string>;
  /** 디버깅/추적용 메타데이터 */
  meta?: Record<string, unknown>;
}

/** 연결 상태 (REQ-M4-09) */
export type ChartConnectionStatus =
  | 'idle'
  | 'connecting'
  | 'connected'
  | 'disconnected'
  | 'closed'
  | 'error';

/** 모든 차트 패널이 공유하는 공통 config (REQ-M4-02) */
export interface ChartPanelConfigBase {
  channel_name: string;
  display_field?: string;
  label_field?: string;
  max_points?: number;
  refresh_on_reconnect?: boolean;
}

// --- 차트 타입별 config (SPEC-CHART-001 §4.2.2) ---

export interface StatPanelConfig extends ChartPanelConfigBase {
  unit?: string;
  decimal_places?: number;
  threshold_color_rules?: Array<{ min: number; color: string }>;
}

/** Y축 도메인 결정 방식 (line-chart) */
export type YAxisMode = 'auto' | 'manual' | 'auto_padded';

/** X축 시간 윈도우 결정 방식 (line-chart) */
export type TimeWindowMode = 'points' | 'recent' | 'fixed';

/** Y축 임계선 심각도 (line-chart) — 하위 호환용 유지 */
export type ThresholdSeverity = 'info' | 'warning' | 'critical';

/** 경계 라인 정의.
 *  ReferenceLine 으로 그려지고, fill_to 지정 시 ReferenceArea 로 사이를 색칠. */
/** 경계 채우기 방향 */
export type FillDirection = 'below' | 'above';

export interface YThreshold {
  /** Y축 값 */
  value: number;
  /** 솔리드 색상 (필수). 라인과 fill 에 모두 사용 */
  color: string;
  /** 채우기 방향. 'below' = 값 이하, 'above' = 값 이상. 미지정 시 채우기 없음 */
  fill_direction?: FillDirection;
  /** @deprecated fill_to 대신 fill_direction 사용 */
  fill_to?: number;
  /** 하위 호환: severity */
  severity?: ThresholdSeverity;
  /** 하위 호환: label */
  label?: string;
}

/** 라인 스타일 — 채널별로 적용 */
export type StrokeStyle = 'solid' | 'dashed' | 'dotted';

/** 채널 ref. channels 배열의 각 항목. */
export interface ChannelRefConfig {
  /** chart-emitter 채널명 (필수) */
  name: string;
  /** 라인 표시 별칭. 미지정 시 name 사용 */
  alias?: string;
  /** 채널별 표시 필드. 미지정 시 'value' */
  display_field?: string;
  /** 라인 색상. 미지정 시 자동 팔레트 */
  color?: string;
  /** 라인 스타일. 기본 'solid' */
  stroke_style?: StrokeStyle;
  /** 라인 두께 (px). 기본 2 */
  stroke_width?: number;
  /** 부드러운 곡선. 기본 false */
  smooth?: boolean;
}

/** 범례 설정 */
export type LegendPosition = 'left' | 'right' | 'bottom';

export interface LegendConfig {
  /** 범례 위치. 기본 'bottom' */
  position?: LegendPosition;
  /** 시리즈 이름 표시. 기본 true */
  show_name?: boolean;
  /** 라인 미리보기 표시. 기본 true */
  show_line?: boolean;
  /** 마지막 값 표시. 기본 false */
  show_last_value?: boolean;
}

export interface LineChartPanelConfig extends ChartPanelConfigBase {
  /** 채널 목록 — 기본 입력 */
  channels?: ChannelRefConfig[];

  // Y축
  y_min?: number;
  y_max?: number;
  y_axis_mode?: YAxisMode;
  y_axis_padding_pct?: number;
  /** 경계 라인 (threshold) */
  y_thresholds?: YThreshold[];

  // X축 시간 윈도우
  time_window_mode?: TimeWindowMode;
  recent_window_sec?: number;
  fixed_start_ms?: number;
  fixed_end_ms?: number;
  time_window_refresh_ms?: number;

  /** 범례 */
  legend?: LegendConfig;

  /** @deprecated 채널별로 이동됨 — 하위 호환 fallback */
  smooth?: boolean;
  multi_series_field?: string;
}

/** severity 별 기본 색상 — 하위 호환 및 기본 threshold 색상 */
export const THRESHOLD_DEFAULT_COLORS: Record<ThresholdSeverity, string> = {
  info: '#3b82f6',
  warning: '#f59e0b',
  critical: '#ef4444',
};

/** stroke_style → SVG strokeDasharray 매핑 */
export const STROKE_DASHARRAY: Record<StrokeStyle, string> = {
  solid: '',
  dashed: '8 4',
  dotted: '2 3',
};

export type BarChartMode = 'category' | 'time_bin';
export type AggFunc = 'count' | 'sum' | 'avg';

export interface BarChartPanelConfig extends ChartPanelConfigBase {
  mode?: BarChartMode;
  bin_sec?: number;
  agg_func?: AggFunc;
}

export interface PiePanelConfig extends ChartPanelConfigBase {
  agg_func?: AggFunc;
  show_legend?: boolean;
  show_percentage?: boolean;
}

export type TableColumnFormat = 'datetime' | 'number' | 'string';

export interface TableColumn {
  field: string;
  header: string;
  format?: TableColumnFormat;
}

export type SortOrder = 'asc' | 'desc';

export interface TablePanelConfig extends ChartPanelConfigBase {
  columns: TableColumn[];
  rows_per_page?: number;
  default_sort?: { field: string; order: SortOrder };
}

/**
 * dot 경로로 ChartEntry 에서 값을 추출한다.
 * 예: getByPath(entry, "labels.room") -> entry.labels.room
 *     getByPath(entry, "value.inner") -> (entry.value as any).inner
 *
 * 중간 경로가 null/undefined 이거나 객체가 아니면 undefined 반환.
 */
export function getByPath(entry: ChartEntry, path: string): unknown {
  if (!path) return undefined;
  const parts = path.split('.');
  let cur: unknown = entry;
  for (const part of parts) {
    if (cur == null || typeof cur !== 'object') return undefined;
    cur = (cur as Record<string, unknown>)[part];
  }
  return cur;
}
