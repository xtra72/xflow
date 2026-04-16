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

export interface LineChartPanelConfig extends ChartPanelConfigBase {
  y_min?: number;
  y_max?: number;
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
