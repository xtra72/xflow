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

export interface LineChartPanelConfig extends ChartPanelConfigBase {
  // Y축 (기존)
  y_min?: number;
  y_max?: number;

  // Y축 (신규)
  /** 기본 'auto'. 'manual' 이면 y_min/y_max 사용, 'auto_padded' 이면 데이터 범위 ± padding */
  y_axis_mode?: YAxisMode;
  /** auto_padded 모드의 양쪽 여백 비율 (%). 기본 5, 범위 0..50 */
  y_axis_padding_pct?: number;

  // X축 시간 윈도우 (신규)
  /** 기본 'points' (기존 호환, max_points 개수 기준) */
  time_window_mode?: TimeWindowMode;
  /** 'recent' 모드의 윈도우 크기(초). 기본 600 */
  recent_window_sec?: number;
  /** 'fixed' 모드의 시작 timestamp (epoch ms) */
  fixed_start_ms?: number;
  /** 'fixed' 모드의 끝 timestamp (epoch ms). 미지정 시 현재 시각 */
  fixed_end_ms?: number;
  /** 'recent' 모드의 도메인 갱신 주기(ms). 기본 1000, 범위 200..60000 */
  time_window_refresh_ms?: number;

  // 기타 (기존)
  smooth?: boolean;
  multi_series_field?: string;
}

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
