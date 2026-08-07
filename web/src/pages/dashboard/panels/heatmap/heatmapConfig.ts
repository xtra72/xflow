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
  };
}
