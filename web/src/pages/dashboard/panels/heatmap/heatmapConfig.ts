// 히트맵 패널 config 타입 + 파서 (SPEC-HEATMAP-PANEL-001 T1).
//
// 히트맵 패널은 공간 온도 센서를 store 태그 필터로 바인딩하고(store_source),
// 센서별 수동 배치 좌표(sensor_positions)를 기준으로 IDW 보간 온도장을 렌더한다.
// config 는 기존 차트 패널과 동일하게 불투명 JSON 으로 영속되므로 백엔드 스키마
// 변경이 없다(REQ-04). `parseHeatmapConfig` 는 결측/손상 입력을 예외 없이 기본값으로
// 보정해 하위호환을 보존한다(A5, REQ-05).
//
// 좌표계: sensor_positions 의 {x, y} 는 정규화 좌표(0..1 권장)다. 좌표가 없는 센서는
// 보간 입력에서 제외된다(패널 측 책임, AC-E2). 여기서는 x/y 가 유한 숫자가 아닌 항목만
// 걸러낸다.
//
// @spec SPEC-HEATMAP-PANEL-001

import type { StoreSourceConfig } from '../charts/chartChannelTypes';

/** IDW 거리 감쇠 지수 기본값(REQ-05). */
export const DEFAULT_IDW_POWER = 2;
/** IDW 격자 해상도 기본값(중간 해상도, REQ-05). */
export const DEFAULT_GRID_RESOLUTION = 48;
/** 도면 위 히트맵 합성 불투명도 기본값(SPEC-002 REQ-05). */
export const DEFAULT_HEATMAP_OPACITY = 0.6;
/** 도면 배경 맞춤(fit) 기본값(SPEC-002 REQ-01/REQ-05). */
export const DEFAULT_FLOOR_PLAN_FIT: 'contain' | 'cover' = 'contain';
/** 등고선 등치 레벨 개수 기본값(SPEC-003 REQ-05). */
export const DEFAULT_CONTOUR_LEVEL_COUNT = 5;
/** 색표(colorbar) 범례 눈금 개수 기본값. */
export const DEFAULT_LEGEND_TICK_COUNT = 5;
/** 색표 범례 눈금 개수 하한(단일 라벨 방지). */
export const MIN_LEGEND_TICK_COUNT = 2;
/** 색표 범례 눈금 개수 상한(과밀 방지). */
export const MAX_LEGEND_TICK_COUNT = 10;

/** 센서 배치 좌표(정규화 0..1 권장). */
export interface SensorPosition {
  x: number;
  y: number;
}

/** 색상표 정지점: 정규화 값(0..1)에 대응하는 색(hex). */
export interface ColorStop {
  /** 0..1 정규화 위치. */
  stop: number;
  /** hex 색상('#rrggbb' 또는 '#rgb'). */
  color: string;
}

/** IDW 보간 파라미터(파싱 후 기본값이 채워진 형태). */
export interface IdwParams {
  /** 거리 감쇠 지수. 기본 2. */
  power: number;
  /** 격자 해상도(픽셀 격자 밀도). 기본 48. */
  grid_resolution: number;
}

/**
 * 도면 이미지 배경 config(SPEC-002 REQ-01/REQ-02, additive). 이미지는 1차 저장 방식으로
 * data-URL 문자열을 임베드한다(백엔드 무변경). 파싱 후 반환되면 `image`(비어 있지 않은 문자열)와
 * `fit`(기본 contain)이 항상 채워진다. 이미지가 없으면 파서가 undefined 를 반환한다(배경 없음).
 */
export interface FloorPlanConfig {
  /** 도면 이미지 data-URL. 파싱 결과에는 항상 비어 있지 않은 문자열로 존재한다. */
  image?: string;
  /** 배경 맞춤(종횡비 보존). 기본 contain. */
  fit?: 'contain' | 'cover';
  /** 로드된 이미지 원본 폭(종횡비 유지용, 선택). */
  natural_width?: number;
  /** 로드된 이미지 원본 높이(종횡비 유지용, 선택). */
  natural_height?: number;
}

/** 배치 에디터 옵션(SPEC-002 REQ-05, additive). 편집 활성 여부는 런타임 상태(비영속). */
export interface EditorConfig {
  /** 그리드 스냅 간격(정규화 0..1 단위, 선택). */
  snap?: number;
  /** 마커 표시 크기(px, 선택). */
  marker_size?: number;
}

/** 등고선 선 스타일(SPEC-003 REQ-05, additive). 미지정 필드는 렌더측 기본값. */
export interface ContourLineStyle {
  /** 선 색(hex). 미지정 시 렌더측 기본. */
  color?: string;
  /** 선 두께(px, non-scaling-stroke). 미지정 시 렌더측 기본. */
  width?: number;
  /** 점선 패턴(dasharray). 미지정/빈 배열이면 실선. */
  dash?: number[];
}

/**
 * 등고선(contour lines) config(SPEC-003 REQ-03/REQ-05, additive). 파싱 후 반환되면
 * enabled/level_count/labels/line 이 항상 채워진 형태다. `levels`(명시 등치값)는 유효한
 * 유한 숫자 배열일 때만 존재하며, 설정 시 level_count 보다 우선한다(렌더측 resolveLevels).
 */
export interface ContourConfig {
  /** 등고선 표시 토글(기본 false). */
  enabled: boolean;
  /** 균등 분할 레벨 개수(기본 5). `levels` 미설정 시 사용. */
  level_count: number;
  /** 명시 등치값 배열(설정 시 level_count 보다 우선). */
  levels?: number[];
  /** 선 스타일(색/두께/dash). 미지정 필드는 렌더측 기본. */
  line: ContourLineStyle;
  /** 등치값 라벨 표시(기본 false). */
  labels: boolean;
}

/**
 * 값→색 색표(colorbar) 범례 config(additive). 파싱 후 반환되면 enabled/orientation/
 * position/size/tick_count 가 항상 채워진 형태다. 히트맵과 동일 color_table 을 사용하며
 * 별도 색 계산은 없다(HeatmapLegend 가 소비). 미설정 시 파서가 undefined 를 반환한다(범례 없음).
 */
export interface LegendConfig {
  /** 범례 표시 토글(기본 false). */
  enabled: boolean;
  /** 막대 방향. 기본 'vertical'. */
  orientation: 'vertical' | 'horizontal';
  /** 오버레이 모서리 위치. 기본 'bottom-right'. */
  position: 'top-left' | 'top-right' | 'bottom-left' | 'bottom-right';
  /** 크기 프리셋. 기본 'md'. */
  size: 'sm' | 'md' | 'lg';
  /** 등간 눈금 개수(2..10 clamp). 기본 5. */
  tick_count: number;
}

/**
 * 히트맵 패널 config.
 *
 * `store_source` 는 태그 필터로 공간 온도 센서를 동적 바인딩한다(selection_mode:'tag',
 * aggregation:'last'). `value_bounds`/`color_table` 미지정은 자동(센서값 범위 + 기본
 * gradient)으로 해석되므로 옵셔널이다. `idw` 는 파싱 시 항상 기본값이 채워진다.
 */
export interface HeatmapPanelConfig {
  store_source: StoreSourceConfig;
  /** 시리즈 표시 이름(alias/key) → 정규화 좌표. */
  sensor_positions: Record<string, SensorPosition>;
  /** 색상 매핑 clamp 범위. 미지정 시 자동(센서값 범위). */
  value_bounds?: { min: number; max: number };
  /** 정규화 값(0..1) → 색 정지점. 미지정 시 기본 gradient(렌더 측 폴백). */
  color_table?: ColorStop[];
  /** IDW 파라미터(항상 기본값 보정). */
  idw: IdwParams;
  /** 도면 이미지 배경(SPEC-002). 미첨부/무효 시 undefined(배경 없음, AC-E1). */
  floor_plan?: FloorPlanConfig;
  /** 도면 위 히트맵 합성 불투명도(0..1). 항상 기본값(0.6) 보정 — idw 와 동일 패턴. */
  heatmap_opacity: number;
  /** 배치 에디터 옵션(SPEC-002). 미지정/무효 시 undefined. */
  editor?: EditorConfig;
  /** 등고선 오버레이(SPEC-003). 미지정 시 undefined(등고선 없음, additive off). */
  contour?: ContourConfig;
  /** 값→색 색표 범례(additive). 미지정 시 undefined(범례 없음). */
  legend?: LegendConfig;
}

/** 유한 숫자인지 확인한다(NaN/Infinity 방어). */
function isFiniteNumber(v: unknown): v is number {
  return typeof v === 'number' && Number.isFinite(v);
}

/** 히트맵 패널의 기본 store 소스(태그 자동 확장 + 최신값 집계). */
export function buildDefaultHeatmapStoreSource(): StoreSourceConfig {
  return {
    agent_name: '',
    namespace: 'default',
    selection_mode: 'tag',
    tag_filters: {},
    series: [],
    time_window_ms: 60 * 60 * 1000,
    interval_ms: 60 * 1000,
    aggregation: 'last',
    refresh_interval_ms: 5000,
  };
}

/** 신규 히트맵 패널의 기본 config(불투명 JSON 으로 영속). */
export function buildDefaultHeatmapConfig(): Record<string, unknown> {
  return {
    data_source: 'store',
    store_source: buildDefaultHeatmapStoreSource(),
    sensor_positions: {},
    idw: { power: DEFAULT_IDW_POWER, grid_resolution: DEFAULT_GRID_RESOLUTION },
  };
}

/** raw.sensor_positions 를 유효한 {x,y}(유한 숫자) 항목만 추려 정규화한다. */
function parseSensorPositions(raw: unknown): Record<string, SensorPosition> {
  const out: Record<string, SensorPosition> = {};
  if (!raw || typeof raw !== 'object') return out;
  for (const [key, value] of Object.entries(raw as Record<string, unknown>)) {
    if (!value || typeof value !== 'object') continue;
    const pos = value as Record<string, unknown>;
    if (isFiniteNumber(pos.x) && isFiniteNumber(pos.y)) {
      out[key] = { x: pos.x, y: pos.y };
    }
  }
  return out;
}

/** raw.value_bounds 를 파싱한다. min/max 가 유한 숫자가 아니면 undefined(자동). */
function parseValueBounds(raw: unknown): { min: number; max: number } | undefined {
  if (!raw || typeof raw !== 'object') return undefined;
  const b = raw as Record<string, unknown>;
  if (isFiniteNumber(b.min) && isFiniteNumber(b.max)) {
    return { min: b.min, max: b.max };
  }
  return undefined;
}

/** raw.color_table 을 파싱한다. 유효 정지점(0..1 stop + string color)만 남긴다. 없으면 undefined. */
function parseColorTable(raw: unknown): ColorStop[] | undefined {
  if (!Array.isArray(raw)) return undefined;
  const stops: ColorStop[] = [];
  for (const item of raw) {
    if (!item || typeof item !== 'object') continue;
    const s = item as Record<string, unknown>;
    if (isFiniteNumber(s.stop) && typeof s.color === 'string' && s.color.trim() !== '') {
      stops.push({ stop: s.stop, color: s.color });
    }
  }
  return stops.length > 0 ? stops : undefined;
}

/** raw.idw 를 파싱하되 결측 필드는 기본값으로 채운다. power<=0/해상도<=0 은 기본값 폴백. */
function parseIdw(raw: unknown): IdwParams {
  const idw = raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : {};
  const power = isFiniteNumber(idw.power) && idw.power > 0 ? idw.power : DEFAULT_IDW_POWER;
  const gridResolution =
    isFiniteNumber(idw.grid_resolution) && idw.grid_resolution > 0
      ? Math.trunc(idw.grid_resolution)
      : DEFAULT_GRID_RESOLUTION;
  return { power, grid_resolution: gridResolution };
}

/** 값을 [0,1] 로 clamp 한다(불투명도 범위 방어). */
function clamp01(v: number): number {
  return v < 0 ? 0 : v > 1 ? 1 : v;
}

/**
 * raw.floor_plan 을 파싱한다. image(비어 있지 않은 문자열)가 없으면 undefined(배경 없음, AC-E1).
 * fit 은 'cover' 만 명시적으로 인정하고 그 외에는 기본 'contain'. natural_width/height 는 양의
 * 유한 숫자만 통과시킨다(선택).
 */
function parseFloorPlan(raw: unknown): FloorPlanConfig | undefined {
  if (!raw || typeof raw !== 'object') return undefined;
  const fp = raw as Record<string, unknown>;
  const image =
    typeof fp.image === 'string' && fp.image.trim() !== '' ? fp.image : undefined;
  // 이미지가 없으면 배경 자체가 없다(fit 만 있어도 무의미).
  if (!image) return undefined;
  const fit: 'contain' | 'cover' = fp.fit === 'cover' ? 'cover' : DEFAULT_FLOOR_PLAN_FIT;
  const result: FloorPlanConfig = { image, fit };
  if (isFiniteNumber(fp.natural_width) && fp.natural_width > 0) {
    result.natural_width = fp.natural_width;
  }
  if (isFiniteNumber(fp.natural_height) && fp.natural_height > 0) {
    result.natural_height = fp.natural_height;
  }
  return result;
}

/** raw.heatmap_opacity 를 파싱한다. 유한 숫자면 0..1 clamp, 아니면 기본값(0.6). */
function parseHeatmapOpacity(raw: unknown): number {
  return isFiniteNumber(raw) ? clamp01(raw) : DEFAULT_HEATMAP_OPACITY;
}

/** raw.editor 를 파싱한다. snap/marker_size 는 양의 유한 숫자만 통과. 둘 다 없으면 undefined. */
function parseEditor(raw: unknown): EditorConfig | undefined {
  if (!raw || typeof raw !== 'object') return undefined;
  const e = raw as Record<string, unknown>;
  const out: EditorConfig = {};
  if (isFiniteNumber(e.snap) && e.snap > 0) out.snap = e.snap;
  if (isFiniteNumber(e.marker_size) && e.marker_size > 0) out.marker_size = e.marker_size;
  return out.snap !== undefined || out.marker_size !== undefined ? out : undefined;
}

/** raw.contour.levels 를 파싱한다. 유한 숫자만 남긴다. 없거나 빈 결과면 undefined. */
function parseContourLevels(raw: unknown): number[] | undefined {
  if (!Array.isArray(raw)) return undefined;
  const out = raw.filter((v): v is number => isFiniteNumber(v));
  return out.length > 0 ? out : undefined;
}

/** raw.contour.line 을 파싱한다. 유효 필드만 채운 선 스타일(항상 객체 반환, 기본 {}). */
function parseContourLine(raw: unknown): ContourLineStyle {
  if (!raw || typeof raw !== 'object') return {};
  const l = raw as Record<string, unknown>;
  const out: ContourLineStyle = {};
  if (typeof l.color === 'string' && l.color.trim() !== '') out.color = l.color;
  if (isFiniteNumber(l.width) && l.width > 0) out.width = l.width;
  if (Array.isArray(l.dash)) {
    const dash = l.dash.filter((v): v is number => isFiniteNumber(v) && v >= 0);
    if (dash.length > 0) out.dash = dash;
  }
  return out;
}

/**
 * raw.contour 를 파싱한다(SPEC-003, additive). floor_plan/editor 선례와 동일하게 raw 가
 * 객체가 아니면 undefined 를 반환한다(등고선 없음). 객체이면 기본값(enabled=false,
 * level_count=5, labels=false, line={})을 채워 반환한다. `levels`는 유효 배열일 때만 존재한다.
 */
function parseContour(raw: unknown): ContourConfig | undefined {
  if (!raw || typeof raw !== 'object') return undefined;
  const c = raw as Record<string, unknown>;
  const levelCount =
    isFiniteNumber(c.level_count) && c.level_count > 0
      ? Math.trunc(c.level_count)
      : DEFAULT_CONTOUR_LEVEL_COUNT;
  const result: ContourConfig = {
    enabled: c.enabled === true,
    level_count: levelCount,
    line: parseContourLine(c.line),
    labels: c.labels === true,
  };
  const levels = parseContourLevels(c.levels);
  if (levels) result.levels = levels;
  return result;
}

/**
 * raw.legend 를 파싱한다(additive). floor_plan/contour 선례와 동일하게 raw 가 객체가 아니면
 * undefined 를 반환한다(범례 없음). 객체이면 enum 화이트리스트(orientation/position/size)와
 * tick_count 2..10 clamp 로 기본값을 보정한다(enabled=false, vertical, bottom-right, md, 5).
 */
function parseLegend(raw: unknown): LegendConfig | undefined {
  if (!raw || typeof raw !== 'object') return undefined;
  const l = raw as Record<string, unknown>;
  // enum 화이트리스트: 미인정 값은 기본값으로 폴백(parseFloorPlan.fit 선례 동일).
  const orientation: LegendConfig['orientation'] =
    l.orientation === 'horizontal' ? 'horizontal' : 'vertical';
  const position: LegendConfig['position'] =
    l.position === 'top-left' || l.position === 'top-right' || l.position === 'bottom-left'
      ? l.position
      : 'bottom-right';
  const size: LegendConfig['size'] = l.size === 'sm' || l.size === 'lg' ? l.size : 'md';
  // 눈금 개수: 유한 숫자면 정수화 후 2..10 clamp, 아니면 기본값(5).
  const tickRaw = isFiniteNumber(l.tick_count)
    ? Math.trunc(l.tick_count)
    : DEFAULT_LEGEND_TICK_COUNT;
  const tick_count =
    tickRaw < MIN_LEGEND_TICK_COUNT
      ? MIN_LEGEND_TICK_COUNT
      : tickRaw > MAX_LEGEND_TICK_COUNT
        ? MAX_LEGEND_TICK_COUNT
        : tickRaw;
  return { enabled: l.enabled === true, orientation, position, size, tick_count };
}

/** raw.store_source 를 파싱한다. 객체가 아니면 기본 store 소스로 폴백(하위호환). */
function parseStoreSource(raw: unknown): StoreSourceConfig {
  if (!raw || typeof raw !== 'object') return buildDefaultHeatmapStoreSource();
  // store_source 는 기존 StoreSourceConfig 형상을 그대로 신뢰한다(useStoreChartData 가
  // 무효 필드를 idle 로 방어함). 여기서는 참조를 그대로 반환해 사용자 설정을 보존한다.
  return raw as StoreSourceConfig;
}

/**
 * 히트맵 패널 config 를 파싱한다. 결측/손상 입력(null/문자열/숫자 등)은 예외 없이
 * 기본값으로 보정한다(하위호환, REQ-04/REQ-05).
 */
export function parseHeatmapConfig(raw: unknown): HeatmapPanelConfig {
  const cfg = raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : {};
  return {
    store_source: parseStoreSource(cfg.store_source),
    sensor_positions: parseSensorPositions(cfg.sensor_positions),
    value_bounds: parseValueBounds(cfg.value_bounds),
    color_table: parseColorTable(cfg.color_table),
    idw: parseIdw(cfg.idw),
    // SPEC-002 신규 필드(additive). MVP 시절 config(필드 없음)는 기본값으로 채워진다(AC-E5).
    floor_plan: parseFloorPlan(cfg.floor_plan),
    heatmap_opacity: parseHeatmapOpacity(cfg.heatmap_opacity),
    editor: parseEditor(cfg.editor),
    // SPEC-003 신규 필드(additive). 미설정 MVP/002 config 는 undefined(등고선 없음, AC-E2/회귀 0).
    contour: parseContour(cfg.contour),
    // 색표 범례(additive). 미설정 config 는 undefined(범례 없음, 회귀 0).
    legend: parseLegend(cfg.legend),
  };
}
