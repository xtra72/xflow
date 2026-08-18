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
import { migrateSensorPositions } from './sensorIdentity';
import type { StageFit } from './stage';

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

/**
 * 도면 이미지 레이어(다중 이미지). 단일 `floor_plan` 의 상위 호환 형태로, 파서가 구 config 를
 * 이 배열로 이관한다(구 단일 이미지 = 스테이지를 가득 채우는 레이어 1장).
 *
 * 좌표계: x/y/w/h 는 **스테이지 정규화 박스**(0..1)다 — 첫 레이어(기준 도면)가 스테이지의
 * 종횡비를 정하고 기본 박스 {0,0,1,1} 로 스테이지를 정확히 채운다. 나머지 레이어는 그 위에
 * 임의 위치/크기로 얹힌다(부분 확대도, 구역 표시 등).
 */
export interface FloorPlanLayer {
  /**
   * 자산 id(내용 SHA-256). 신규 첨부는 이 경로를 쓴다 — 이미지 바이트는 별도 자산 API 에
   * 저장되고 config 에는 id 만 남으므로 대시보드 snapshot 이 256KB 상한에 걸리지 않는다.
   */
  asset_id?: string;
  /**
   * 레거시 인라인 data-URL. 자산 분리 이전에 저장된 패널이 여기에 이미지를 통째로 들고 있다.
   * 읽기 전용 하위호환 경로이며 신규 첨부는 사용하지 않는다 — 이 필드가 채워진 패널은
   * 대시보드 저장이 실패할 수 있다(그것이 자산 분리의 이유).
   */
  image?: string;
  /** 스테이지 정규화 좌상단 x(0..1). 기본 0. */
  x: number;
  /** 스테이지 정규화 좌상단 y(0..1). 기본 0. */
  y: number;
  /** 스테이지 정규화 폭(0 초과 1 이하). 기본 1. */
  w: number;
  /** 스테이지 정규화 높이(0 초과 1 이하). 기본 1. */
  h: number;
  /** 레이어 불투명도(0..1). 기본 1. */
  opacity: number;
  /**
   * 박스 안에서의 이미지 맞춤. 기본 contain.
   *   - contain: 비율 유지 + 박스 안에 전부 보이게(여백 생김)
   *   - cover:   비율 유지 + 박스를 덮음(넘치는 부분이 잘림)
   *   - fill:    비율 무시 + 박스에 정확히 맞춤(잘리지 않고 늘거나 줄어듦)
   * 박스 폭을 줄였을 때 "잘리지 않고 이미지가 줄어들기" 를 원하면 fill 이다 — cover 는 정의상
   * 비율을 지키므로 좁아진 축을 채우려 반대 축이 넘치고, 그 넘친 부분이 잘린다.
   */
  fit: 'contain' | 'cover' | 'fill';
  /** 원본 폭(종횡비 산출용, 선택). 첫 레이어의 값이 스테이지 종횡비를 정한다. */
  natural_width?: number;
  /** 원본 높이(종횡비 산출용, 선택). */
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
  /** 오버레이 모서리 위치. 기본 'bottom-right'. `offset` 이 있으면 그쪽이 우선한다. */
  position: 'top-left' | 'top-right' | 'bottom-left' | 'bottom-right';
  /** 크기 프리셋. 기본 'md'. */
  size: 'sm' | 'md' | 'lg';
  /** 등간 눈금 개수(2..10 clamp). 기본 5. */
  tick_count: number;
  /**
   * 드래그로 옮긴 자유 위치(범례 좌상단의 정규화 좌표 0..1, sensor_positions 와 같은 좌표계).
   * 설정되면 `position` 모서리 프리셋 대신 이 좌표로 배치한다 — 리사이즈/도면 교체에 불변.
   * 미설정(기본)이면 기존 모서리 배치와 바이트 동일하다. 설정 다이얼로그에서 모서리를 다시
   * 고르면 제거되어 프리셋으로 되돌아간다.
   */
  offset?: SensorPosition;
}

/**
 * 히트맵 패널 config.
 *
 * `store_source` 는 store 시리즈를 바인딩한다. 신규 패널 기본은 keys 모드(체크박스 선택,
 * series:[])이며, 헤더 태그 피커로 태그 자동 바인딩(selection_mode:'tag')에 진입할 수 있다.
 * `value_bounds`/`color_table` 미지정은 자동(센서값 범위 + 기본 gradient)으로 해석되므로
 * 옵셔널이다. `idw` 는 파싱 시 항상 기본값이 채워진다.
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
  /** 도면 이미지 배경(SPEC-002, 단일). 다중 이미지의 하위호환 입력으로만 남는다. */
  floor_plan?: FloorPlanConfig;
  /**
   * 도면 이미지 레이어 목록. 파서가 항상 배열로 채운다(빈 배열 = 배경 없음). 구 `floor_plan`
   * 단일 이미지는 읽는 시점에 레이어 1장으로 이관된다 — 파괴적 쓰기는 없고, 사용자가 도면을
   * 편집할 때 이관된 배열이 자연스럽게 영속된다(sensor_positions 이관 선례와 동일).
   */
  floor_plans: FloorPlanLayer[];
  /**
   * sensor_positions 좌표가 어느 공간에 저장돼 있는지. 'stage'(신규)는 기준 도면 박스 기준,
   * 미지정(레거시)은 패널 컨테이너 기준이다. 레거시 좌표는 렌더 시점에 실측 rect 로 스테이지
   * 공간으로 환산해 **보이던 위치를 그대로 보존**하고, 사용자가 좌표를 편집할 때 'stage' 로
   * 승격돼 영속된다.
   */
  sensor_space?: 'container' | 'stage';
  /**
   * 스테이지를 패널 본문에 맞추는 방식(additive, 기본 'contain').
   *
   * 'contain' 은 도면 종횡비를 지켜 레터박스 여백을 남기고, 'cover' 는 여백 대신 가장자리를
   * 자르며, 'stretch' 는 여백 대신 도면을 늘린다. 좌표는 세 모드 모두 스테이지 정규화라 마커는
   * 도면 위 같은 지점에 붙는다(stage.ts StageFit 참조).
   */
  stage_fit?: StageFit;
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

/**
 * 히트맵 패널의 기본 store 소스. 신규 패널은 아무 것도 바인딩되지 않은 keys 모드로 시작해
 * (selection_mode:'keys', series:[]) 데이터 소스 목록이 전부 미체크 상태가 된다 → 체크박스로
 * 선택한 시리즈만 표시된다. 태그 모드는 헤더 태그 피커로 언제든 진입할 수 있다(기본 아님).
 */
export function buildDefaultHeatmapStoreSource(): StoreSourceConfig {
  return {
    agent_name: '',
    namespace: 'default',
    selection_mode: 'keys',
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

/** 0 초과 1 이하로 정규화한다(레이어 폭/높이 — 0 크기 레이어 방지). */
function clampSize(v: unknown, fallback: number): number {
  if (!isFiniteNumber(v) || v <= 0) return fallback;
  return v > 1 ? 1 : v;
}

/**
 * raw 레이어 1건을 파싱한다. image(비어 있지 않은 문자열)가 없으면 null(그 레이어는 버린다).
 * 박스/불투명도는 결측·손상 시 기본값(스테이지 가득, 불투명)으로 보정한다.
 */
function parseFloorPlanLayer(raw: unknown): FloorPlanLayer | null {
  if (!raw || typeof raw !== 'object') return null;
  const l = raw as Record<string, unknown>;
  const assetId = typeof l.asset_id === 'string' && l.asset_id.trim() !== '' ? l.asset_id : undefined;
  const image = typeof l.image === 'string' && l.image.trim() !== '' ? l.image : undefined;
  // 둘 중 하나는 있어야 그릴 것이 있다. 둘 다 있으면 asset_id 가 우선하되 image 도 보존한다
  // (자산 조회 실패 시 레거시 인라인 이미지로 그릴 수 있다).
  if (!assetId && !image) return null;
  const layer: FloorPlanLayer = {
    ...(assetId !== undefined ? { asset_id: assetId } : {}),
    ...(image !== undefined ? { image } : {}),
    x: isFiniteNumber(l.x) ? clamp01(l.x) : 0,
    y: isFiniteNumber(l.y) ? clamp01(l.y) : 0,
    w: clampSize(l.w, 1),
    h: clampSize(l.h, 1),
    opacity: isFiniteNumber(l.opacity) ? clamp01(l.opacity) : 1,
    // enum 화이트리스트: 미인정 값은 기본값(contain)으로 폴백.
    fit: l.fit === 'cover' || l.fit === 'fill' ? l.fit : DEFAULT_FLOOR_PLAN_FIT,
  };
  if (isFiniteNumber(l.natural_width) && l.natural_width > 0) {
    layer.natural_width = l.natural_width;
  }
  if (isFiniteNumber(l.natural_height) && l.natural_height > 0) {
    layer.natural_height = l.natural_height;
  }
  return layer;
}

/**
 * raw.floor_plans(배열) 를 파싱하고, 없으면 구 단일 `floor_plan` 을 레이어 1장으로 이관한다.
 * 둘 다 없으면 빈 배열(배경 없음, AC-E1 유지).
 */
function parseFloorPlans(rawLayers: unknown, single: FloorPlanConfig | undefined): FloorPlanLayer[] {
  if (Array.isArray(rawLayers)) {
    const out: FloorPlanLayer[] = [];
    for (const item of rawLayers) {
      const layer = parseFloorPlanLayer(item);
      if (layer) out.push(layer);
    }
    // 빈 배열로 파싱됐어도 배열이 명시돼 있으면 그 뜻(배경 없음)을 존중한다 — 구 단일 이미지로
    // 되살아나면 "이미지를 다 지웠는데 옛 도면이 돌아온다".
    return out;
  }
  if (single?.image) {
    return [
      {
        image: single.image,
        x: 0,
        y: 0,
        w: 1,
        h: 1,
        opacity: 1,
        fit: single.fit ?? DEFAULT_FLOOR_PLAN_FIT,
        ...(single.natural_width !== undefined ? { natural_width: single.natural_width } : {}),
        ...(single.natural_height !== undefined ? { natural_height: single.natural_height } : {}),
      },
    ];
  }
  return [];
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
  const result: LegendConfig = {
    enabled: l.enabled === true,
    orientation,
    position,
    size,
    tick_count,
  };
  // 자유 위치: x/y 가 모두 유한 숫자일 때만 인정하고 0..1 로 clamp 한다(좌표 손상 시 프리셋 폴백).
  const off = l.offset;
  if (off && typeof off === 'object') {
    const o = off as Record<string, unknown>;
    if (isFiniteNumber(o.x) && isFiniteNumber(o.y)) {
      result.offset = { x: clamp01(o.x), y: clamp01(o.y) };
    }
  }
  return result;
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
  const store_source = parseStoreSource(cfg.store_source);
  const floor_plan = parseFloorPlan(cfg.floor_plan);
  return {
    store_source,
    // 좌표는 시리즈 동일성 키로 키잉된다. raw key 로 저장된 기존 패널은 읽는 시점에 이관한다
    // (파괴적 쓰기 없음 — 사용자가 좌표/선택을 편집할 때 이관된 맵이 자연스럽게 영속된다).
    sensor_positions: migrateSensorPositions(
      parseSensorPositions(cfg.sensor_positions),
      store_source.series,
    ).positions,
    value_bounds: parseValueBounds(cfg.value_bounds),
    color_table: parseColorTable(cfg.color_table),
    idw: parseIdw(cfg.idw),
    // SPEC-002 신규 필드(additive). MVP 시절 config(필드 없음)는 기본값으로 채워진다(AC-E5).
    floor_plan,
    // 다중 이미지(additive). 구 단일 floor_plan 은 레이어 1장으로 읽는 시점에 이관된다.
    floor_plans: parseFloorPlans(cfg.floor_plans, floor_plan),
    // 좌표 공간: 명시적으로 'stage' 일 때만 신규 공간. 미지정/손상은 레거시(컨테이너)로 본다.
    sensor_space: cfg.sensor_space === 'stage' ? 'stage' : undefined,
    // 스테이지 맞춤(additive). enum 화이트리스트 — 미인정 값은 기본 contain 으로 폴백해
    // 미설정 config 와 동일하게 동작한다(parseFloorPlan.fit 선례 동일, 회귀 0).
    stage_fit: cfg.stage_fit === 'cover' || cfg.stage_fit === 'stretch' ? cfg.stage_fit : undefined,
    heatmap_opacity: parseHeatmapOpacity(cfg.heatmap_opacity),
    editor: parseEditor(cfg.editor),
    // SPEC-003 신규 필드(additive). 미설정 MVP/002 config 는 undefined(등고선 없음, AC-E2/회귀 0).
    contour: parseContour(cfg.contour),
    // 색표 범례(additive). 미설정 config 는 undefined(범례 없음, 회귀 0).
    legend: parseLegend(cfg.legend),
  };
}
