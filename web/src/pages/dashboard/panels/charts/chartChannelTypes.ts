// 차트 패널 공통 타입 + 값 추출 유틸.
// SPEC-CHART-001 §4.1 / §4.2.2 참조.
//
// SPEC-WEB-005 (차트 Store 소스): 차트 패널이 chart-emitter 채널 대신
// Store 에이전트에서 데이터를 가져올 수 있도록 `data_source` / `store_source`
// config 를 추가했다. 두 소스는 공존하며, `data_source` 미지정(undefined)은
// 기존 채널 경로('channel')로 해석되어 하위 호환을 보존한다.
// @spec SPEC-WEB-005

// Store 키 데이터 타입(int/float/string/boolean/bytes/json). type-only import 이므로
// 컴파일 시 erase 되어 런타임 순환 의존을 만들지 않는다(store.ts 는 chartChannelTypes 를
// import 하지 않는다).
import type { DataType } from '@/services/api/store';

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

/**
 * 차트 패널의 데이터 소스 종류.
 *
 * - `channel`: 기존 chart-emitter WebSocket 채널 경로(기본값).
 * - `store`: Store 에이전트의 시리즈 매트릭스 폴링 경로.
 *
 * @spec SPEC-WEB-005
 */
export type ChartDataSourceKind = 'channel' | 'store';

/**
 * Store 소스에서 조회할 단일 시리즈 참조.
 *
 * `key` 는 Store 키 이름이고, `metric_type`/`tags` 가 지정되면 해당 key 의
 * 특정 시리즈(저장소 기준 분류)로 좁혀 조회한다. 미지정이면 그 key 의 모든
 * 시리즈를 조회한다. `alias`/`color` 는 표시 전용이다.
 *
 * @spec SPEC-WEB-005
 */
export interface StoreSeriesRef {
  /** Store 키 이름. */
  key: string;
  /** 시리즈별 선택 시 metric_type 필터(선택). */
  metric_type?: string;
  /** 시리즈별 선택 시 tag 필터(선택). */
  tags?: Record<string, string>;
  /** 키 데이터 타입(표시/필터 메타데이터). */
  data_type?: DataType;
  /** 표시 별칭(미지정 시 key). */
  alias?: string;
  /** 라인/카테고리 색상(미지정 시 자동 팔레트). */
  color?: string;
  /**
   * 라인 차트 전용 per-line 스타일 (SPEC-WEB-005, ChannelRefConfig 와 동일 형상).
   * 채널 시리즈와 스토어 시리즈의 라인 스타일을 하나의 편집기로 통합하기 위해
   * StoreSeriesRef 에도 동일 필드를 둔다. 라인 차트가 아닌 패널에서는 무시된다.
   */
  /** 라인 스타일(solid/dashed/dotted). 기본 'solid'. */
  stroke_style?: StrokeStyle;
  /** 라인 두께(px). 기본 2. */
  stroke_width?: number;
  /** 부드러운 곡선. 기본 false. */
  smooth?: boolean;
  /**
   * 값 추출 필드(dot-path). 채널 시리즈와 형상 통일을 위해 둔다.
   * 단, 스토어 소스는 매트릭스가 이미 시리즈별 단일 숫자 값을 제공하므로
   * 렌더에는 영향을 주지 않는다(메타데이터/전방 호환 목적).
   */
  display_field?: string;
}

/**
 * 차트 패널 Store 소스 설정 블록(모든 차트 config 가 공유).
 *
 * 시간 윈도우는 "지금(now) 기준 상대 윈도우" 로 해석된다:
 *   endMs = now, startMs = now - time_window_ms.
 * 매트릭스는 `interval_ms` 버킷으로 서버/클라이언트 집계(`aggregation`)되어
 * 시리즈별 타임라인으로 변환된다.
 *
 * @spec SPEC-WEB-005
 */
export interface StoreSourceConfig {
  /** Store 에이전트 이름(백엔드 라우트가 name 기반). */
  agent_name: string;
  /** Store 네임스페이스(미지정 시 'default'). */
  namespace?: string;
  /** 조회할 시리즈 목록. 비어있으면 store 소스는 비활성으로 취급한다. */
  series: StoreSeriesRef[];
  /** 상대 시간 윈도우 길이(ms). now - time_window_ms 가 시작 시각. */
  time_window_ms: number;
  /** 버킷 크기(ms). */
  interval_ms: number;
  /** 집계 함수(UI 표기 그대로). */
  aggregation: 'min' | 'max' | 'average' | 'first' | 'last';
  /** 폴링 주기(ms). 미지정 시 기본값(약 5000ms)을 사용한다. */
  refresh_interval_ms?: number;
}

/** 모든 차트 패널이 공유하는 공통 config (REQ-M4-02) */
export interface ChartPanelConfigBase {
  channel_name: string;
  display_field?: string;
  label_field?: string;
  max_points?: number;
  refresh_on_reconnect?: boolean;
  /**
   * 데이터 소스 종류. 미지정/undefined 는 'channel'(기존 채널 경로)로
   * 해석되어 하위 호환을 보존한다. @spec SPEC-WEB-005
   */
  data_source?: ChartDataSourceKind;
  /** Store 소스 설정(data_source === 'store' 일 때 사용). @spec SPEC-WEB-005 */
  store_source?: StoreSourceConfig;
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

  // X축
  /** X축 레이블 (예: "시간", "Time") */
  x_label?: string;

  // Y축
  y_min?: number;
  y_max?: number;
  y_axis_mode?: YAxisMode;
  y_axis_padding_pct?: number;
  /** Y축 레이블 (예: "온도") */
  y_label?: string;
  /** Y축 단위 (예: "°C", "kW") */
  y_unit?: string;
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
