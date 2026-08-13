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
// 표시 라벨 포맷은 매트릭스 컬럼명과 같은 곳(seriesLabels)에서 온다 — 같은 시리즈가 화면마다
// 다른 표기를 갖지 않도록 두 번째 포맷터를 만들지 않는다. 순수 모듈이라 순환 의존이 없다.
import { seriesRefDisplayName } from '@/services/api/seriesLabels';

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
  /**
   * Store 에이전트의 안정적 ID(정본). @spec SPEC-WEB-006
   *
   * Store API 는 이름 주소(`/store/{agent_name}/...`)이지만, 에이전트 이름은
   * 변경될 수 있어 config 에 이름만 저장하면 리네임 시 연결이 끊긴다. 따라서
   * 불변 ID 를 정본으로 저장하고, 렌더/쿼리 시 이 id 로 현재 이름을 해석해
   * API 를 호출한다. 구 config 하위호환을 위해 옵셔널이며, 부재 시 `agent_name`
   * 을 그대로 사용한다.
   */
  agent_id?: string;
  /**
   * Store 에이전트 이름.
   *
   * `agent_id` 가 있으면 이 값은 표시용 스냅샷 + 하위호환 폴백으로만 쓰인다
   * (실제 API 호출 이름은 agent_id 로 해석한 현재 이름). `agent_id` 가 없는 구
   * config 에서는 이 값이 그대로 API 호출에 사용된다. @spec SPEC-WEB-006
   */
  agent_name: string;
  /** Store 네임스페이스(미지정 시 'default'). */
  namespace?: string;
  /**
   * 시리즈 선택 방식. @spec SPEC-WEB-005
   *
   * - `'keys'`(기본): 사용자가 `series[]` 를 직접 멀티셀렉트한다(기존 동작).
   * - `'tag'`: `tag_filters` 로 매칭되는 모든 store 키를 폴링 시점마다 동적으로
   *   시리즈로 확장한다. 태그 하위 키가 추가/삭제되면 자동 반영된다. `series[]` 는 무시된다.
   *
   * 미지정/undefined 는 `'keys'` 로 해석되어 하위 호환을 보존한다(기존 패널 무영향).
   */
  selection_mode?: 'keys' | 'tag';
  /**
   * 태그 AND 필터(`selection_mode === 'tag'` 일 때만 사용). @spec SPEC-WEB-005
   *
   * 예) `{ room: '1', type: 'temperature' }` → room=1 AND type=temperature 를 가진
   * 모든 키가 시리즈가 된다. 폴링마다 재해석되므로 키 추가/삭제가 자동 반영된다.
   * 비어있으면(키 0개) tag 모드는 비활성(idle)으로 취급한다.
   */
  tag_filters?: Record<string, string>;
  /**
   * 조회할 시리즈 목록. `'keys'` 모드의 정본이다. 비어있으면 store 소스는 비활성으로
   * 취급한다. `'tag'` 모드에서는 무시되며 키가 동적으로 해석된다.
   */
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

/**
 * Y축 데이터 타입 (line-chart).
 * - `numeric`(기본): 숫자 축. y_axis_mode(auto/manual/auto_padded) + y_min/y_max 로 범위 결정.
 * - `enum`: 열거형 축. y_enum_labels 의 값→라벨 매핑으로 눈금/툴팁을 문자열로 표시한다.
 *   (기존 boolean 자동 표시 0→false / 1→true 를 사용자 정의로 일반화한 것)
 */
export type YAxisDataType = 'numeric' | 'enum';

/** 열거형 Y축의 값→라벨 매핑 항목 (line-chart). 예: { value: 0, label: '정지' } */
export interface YEnumLabel {
  /** 매핑할 숫자 값. */
  value: number;
  /** 해당 값에 표시할 문자열. */
  label: string;
}

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

/**
 * 시리즈 자동 색상 팔레트. 데이터 소스 선택 시 시리즈 인덱스별로 서로 다른 색을
 * 자동 배정하는 데 사용한다(사용자가 개별 색을 지정하면 그 값이 우선).
 * LineChartPanel 렌더와 설정 편집기(스와치 기본값)가 동일 팔레트를 공유한다.
 */
export const SERIES_PALETTE: readonly string[] = [
  '#3b82f6',
  '#10b981',
  '#f59e0b',
  '#ef4444',
  '#8b5cf6',
  '#06b6d4',
  '#ec4899',
  '#84cc16',
];

/** 시리즈 인덱스에 대응하는 팔레트 색을 반환한다(팔레트 길이로 순환). */
export function pickSeriesColor(index: number): string {
  const n = SERIES_PALETTE.length;
  const i = ((Math.trunc(index) % n) + n) % n;
  return SERIES_PALETTE[i]!;
}

/**
 * StoreSeriesRef 의 동일성 식별자(key + metric_type + 정렬된 tags). keys 모드에서 선택된
 * 시리즈를 판정/추가/제거할 때 쓴다. 반환 형식은 `"<key> <metric> <k=v,...>"` 로 고정한다.
 * @spec SPEC-PANEL-SETTINGS-001 (시리즈 선택 단일화 — 체크박스 ↔ series)
 */
export function storeSeriesId(
  key: string,
  metric: string,
  tags: Record<string, string>,
): string {
  const tagPart = Object.keys(tags)
    .sort()
    .map((k) => `${k}=${tags[k]}`)
    .join(',');
  return `${key} ${metric} ${tagPart}`;
}

/**
 * StoreSeriesRef 의 **표시 라벨**(사람이 읽는 이름). `storeSeriesId` 의 표시 짝이다.
 *
 * 결함 배경: 시리즈의 동일성은 (key, metric_type, tags) 인데 표시에는 key 만 쓰여서, 한 key 를
 * metric/tags 로 나눠 갖는 형제 시리즈들이 목록·마커에서 **같은 글자**로 보였다. 좌표/매칭은
 * 이미 동일성 키로 분리되어 있었으므로(SPEC-HEATMAP-PANEL-001 재키잉) 남은 것은 표기뿐이며,
 * 이 함수가 그 표기를 한 곳으로 모은다.
 *
 * 규칙:
 *   - 사용자가 붙인 이름(alias)이 있으면 그 이름이 항상 이긴다.
 *   - 그 외에는 매트릭스 컬럼과 동일한 서술 표기(`key · metric{k=v}`)를 쓴다. metric/tags 가
 *     없으면 자연히 `key` 하나로 줄어든다(구분자 잔여물 없음).
 *
 * `alias === key` 를 "사용자가 붙인 이름 없음"으로 보는 이유: 시리즈 생성 시 `alias: key` 가
 * 기본값으로 기록되어 왔기 때문에, alias 존재 여부만으로는 기본값과 사용자 입력을 구분할 수
 * 없다. 이미 저장된 패널도 고쳐지도록 기본값과 같은 값이면 서술 표기로 폴백한다. 비용: 사용자가
 * 굳이 key 와 똑같은 이름을 직접 입력해도 서술 표기가 나온다(그 표기는 key 로 시작하므로
 * 의도에서 크게 벗어나지 않는다).
 *
 * 매칭·동일성에는 절대 쓰지 않는다 — 이름을 바꿔도 좌표/선택이 끊기면 안 된다.
 */
export function storeSeriesLabel(
  ref: Pick<StoreSeriesRef, 'key' | 'metric_type' | 'tags' | 'alias'>,
): string {
  const alias = ref.alias?.trim() ?? '';
  if (alias !== '' && alias !== ref.key) return alias;
  return seriesRefDisplayName(ref.key, ref.metric_type, ref.tags);
}

/**
 * 선택 계열(series) 상한. 라이브 미리보기 성능 보호를 위한 합리적 상한(수십 개).
 * @spec SPEC-PANEL-SETTINGS-001 (AC-15)
 */
export const STORE_SERIES_LIMIT = 48;

/**
 * 축 텍스트(레이블/눈금) 폰트 스타일. 미지정 필드는 렌더 측 기본값으로 폴백한다.
 * 라인 차트의 X/Y 축 레이블(제목)과 값(눈금) 폰트를 축별로 독립 설정한다.
 */
export interface AxisFontStyle {
  /** 글자 크기(px). */
  size?: number;
  /** 글자 색상(hex). */
  color?: string;
  /** 굵기. */
  weight?: 'normal' | 'bold';
}

/** 축 폰트 기본값(기존 하드코딩 값과 동일). */
export const DEFAULT_AXIS_FONT: Required<AxisFontStyle> = {
  size: 10,
  color: '#9ca3af',
  weight: 'normal',
};

/**
 * AxisFontStyle 을 recharts 텍스트 props(fontSize/fill/fontWeight)로 변환한다.
 * 미지정 필드는 DEFAULT_AXIS_FONT 로 채운다.
 */
export function resolveAxisFont(
  font: AxisFontStyle | undefined,
): { fontSize: number; fill: string; fontWeight: 'normal' | 'bold' } {
  return {
    fontSize: font?.size ?? DEFAULT_AXIS_FONT.size,
    fill: font?.color ?? DEFAULT_AXIS_FONT.color,
    fontWeight: font?.weight ?? DEFAULT_AXIS_FONT.weight,
  };
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
  /** X축 레이블(제목) 폰트. */
  x_label_font?: AxisFontStyle;
  /** X축 값(눈금) 폰트. */
  x_tick_font?: AxisFontStyle;
  /** Y축 레이블(제목) 폰트. */
  y_label_font?: AxisFontStyle;
  /** Y축 값(눈금) 폰트. */
  y_tick_font?: AxisFontStyle;

  // Y축
  /**
   * Y축 데이터 타입. 'numeric'(기본) 또는 'enum'.
   * 'enum' 이면 y_enum_labels 로 값→라벨 매핑을 표시하며, 숫자 범위(y_axis_mode/min/max)는
   * 무시된다(축 도메인은 enum 값 범위로 고정).
   */
  y_axis_type?: YAxisDataType;
  /** 열거형 값→라벨 매핑 (y_axis_type === 'enum' 일 때 사용). */
  y_enum_labels?: YEnumLabel[];
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
 * 유효한 열거형 매핑만 추려 값→라벨 Map 을 만든다.
 * value 가 유한 숫자이고 label 이 비어있지 않은 항목만 포함한다.
 * 같은 value 가 중복되면 뒤 항목이 앞 항목을 덮어쓴다(마지막 정의 우선).
 */
export function buildEnumLabelMap(
  labels: YEnumLabel[] | undefined,
): Map<number, string> {
  const map = new Map<number, string>();
  if (!labels) return map;
  for (const item of labels) {
    if (typeof item.value !== 'number' || !Number.isFinite(item.value)) continue;
    const label = (item.label ?? '').trim();
    if (label === '') continue;
    map.set(item.value, label);
  }
  return map;
}

/**
 * 열거형 축에서 숫자 값을 라벨로 변환한다. 매핑에 없으면 숫자 문자열로 폴백한다.
 * 값이 숫자가 아니면 빈 문자열을 반환한다(눈금 사이 보간값 등).
 */
export function formatEnumValue(value: number, map: Map<number, string>): string {
  if (typeof value !== 'number' || !Number.isFinite(value)) return '';
  return map.get(value) ?? String(value);
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
