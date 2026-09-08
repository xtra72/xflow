// 캔버스 패널 config 타입 + 관용 파서 (SPEC-CANVAS-001 T1).
//
// 캔버스 패널은 사용자가 저술한 도형 목록(elements)을 정규화(0..1) 스테이지 좌표로
// 들고 있다가 Canvas 2D 로 그린다. 각 요소는 시리즈에 바인딩되어, 조건 규칙 표
// (first-match-wins)의 결과로 스타일·문구가 바뀐다(REQ-02/REQ-03/REQ-04).
//
// config 는 기존 패널과 동일하게 Go 쪽에서 불투명 JSON 으로 영속되므로 백엔드 변경이
// 없다. 그래서 스키마가 002/003 을 거치며 자라도 구버전 패널이 깨지지 않으려면 읽기
// 경로에 관용 파서가 있어야 한다 — `parseCanvasConfig` 는 결측/손상/구버전 입력을
// **예외 없이** 기본값으로 보정한다(§위험 R3, `parseHeatmapConfig` 패턴).
//
// 데이터 소스 축에 대하여: `usePanelSeriesData` 는 파싱된 view 가 아니라 **원본 config
// 레코드**를 그대로 받는다(HeatmapPanel 선례). 다중 소스 배열(`sources[]`)·`data_sources`
// 같은 필드는 그 계약이 소유하므로 여기서는 보존하지 않는다 — 이 파서가 돌려주는 것은
// 캔버스가 그리기 위해 읽는 필드의 view 다.
//
// @spec SPEC-CANVAS-001

import {
  buildDefaultStoreSource,
  type ChartDataSourceKind,
  type ChartPanelConfigBase,
  type StoreSourceConfig,
  type TsdbSourceConfig,
} from '../charts/chartChannelTypes';

// --- 기본값 상수 ---------------------------------------------------------

/** `{value}` 치환 시 기본 소수 자리(명세 §문구 템플릿). */
export const DEFAULT_DECIMALS = 1;
/** 선 두께 기본값(px). 손상된 strokeWidth 가 들어왔을 때의 폴백이기도 하다. */
export const DEFAULT_STROKE_WIDTH = 1;
/** 글자 크기 기본값(px). 손상된 fontSize 가 들어왔을 때의 폴백이기도 하다. */
export const DEFAULT_FONT_SIZE = 14;
/** 불투명도 기본값. 손상된 opacity 가 들어왔을 때의 폴백이다. */
export const DEFAULT_OPACITY = 1;
/** 신규 패널의 기본 트윈 지속 시간(ms). */
export const DEFAULT_TWEEN_DURATION_MS = 300;
/** 신규 패널의 기본 이징. */
export const DEFAULT_TWEEN_EASING: TweenEasing = 'ease-out';

/** rect/ellipse 기하가 손상됐을 때 대신 쓰는 좌상단+크기(정규화 0..1). */
export const DEFAULT_BOX_GEOMETRY: Readonly<BoxGeometry> = { x: 0.1, y: 0.1, w: 0.2, h: 0.2 };
/** line 기하가 손상됐을 때 대신 쓰는 두 끝점(정규화 0..1). */
export const DEFAULT_LINE_GEOMETRY: Readonly<LineGeometry> = { x1: 0.1, y1: 0.5, x2: 0.9, y2: 0.5 };
/** text 기하가 손상됐을 때 대신 쓰는 정렬 기준점(정규화 0..1). */
export const DEFAULT_POINT_GEOMETRY: Readonly<PointGeometry> = { x: 0.5, y: 0.5 };

// --- 타입 ---------------------------------------------------------------

/** 지원 도형 원시형 4종(REQ-02). */
export type CanvasElementKind = 'rect' | 'ellipse' | 'line' | 'text';

/** 비교 연산자 집합(명세 §비교 연산자 집합). `nodata` 는 결측 판정이다. */
export type RuleOp = 'gt' | 'gte' | 'lt' | 'lte' | 'eq' | 'ne' | 'between' | 'nodata';

/** 트윈 이징 4종(명세 §트윈 명세). */
export type TweenEasing = 'linear' | 'ease-in' | 'ease-out' | 'ease-in-out';

/** 글자 굵기. 보간이 성립하지 않아 트윈 없이 즉시 전환되는 속성이다. */
export type ElementFontWeight = 'normal' | 'bold';

/** 텍스트 정렬 기준. `measureText` 로 잰 폭을 이 기준으로 배치한다(REQ-02). */
export type ElementAlign = 'left' | 'center' | 'right';

/** 바인딩 집계. 001 은 최신값만 쓴다 — 다른 집계는 후속 SPEC. */
export type ElementBindingAgg = 'last';

/** rect | ellipse 기하 — 좌상단 + 크기. 모두 정규화(0..1) 스테이지 좌표. */
export interface BoxGeometry {
  x: number;
  y: number;
  w: number;
  h: number;
}

/** line 기하 — 두 끝점. 모두 정규화(0..1) 스테이지 좌표. */
export interface LineGeometry {
  x1: number;
  y1: number;
  x2: number;
  y2: number;
}

/** text 기하 — 정렬 기준점. 정규화(0..1) 스테이지 좌표. */
export interface PointGeometry {
  x: number;
  y: number;
}

/**
 * 도형별 기하 합집합. 어느 갈래인지는 `CanvasElement.kind` 가 판별한다 —
 * 요소를 `kind` 로 좁히면 `geometry` 도 함께 좁혀지므로 소비 측(canvasGeometry.ts /
 * drawElement.ts)이 `'w' in geo` 같은 형상 검사를 하지 않아도 된다.
 */
export type Geometry = BoxGeometry | LineGeometry | PointGeometry;

/**
 * 요소의 기본 스타일. 모든 필드가 옵셔널이며, 미지정은 "렌더측 기본" 을 뜻한다
 * (히트맵 `ContourLineStyle` 선례). 파서는 미지정 필드를 만들어 채우지 않는다 —
 * 채우면 "지정 안 함" 과 "값이 우연히 기본값과 같음" 이 구분되지 않는다.
 */
export interface ElementStyle {
  fill?: string;
  stroke?: string;
  /** 선 두께(px). */
  strokeWidth?: number;
  /** 0..1. */
  opacity?: number;
  /** 글자 크기(px). */
  fontSize?: number;
  fontWeight?: ElementFontWeight;
  textColor?: string;
  align?: ElementAlign;
  /** 기본 true(미지정 = 보임). */
  visible?: boolean;
}

/** 요소가 참조하는 시리즈. `series` 는 로케일 비의존 동일성 키다(REQ-03). */
export interface ElementBinding {
  series: string;
  agg: ElementBindingAgg;
}

/** 규칙 행의 임계값. `between` 만 2원소 튜플이다. */
export type RuleValue = number | [number, number];

/**
 * 일치한 행이 기본 스타일 위에 덮어쓰는 패치. 명세가 든 패치 가능 속성은
 * fill · stroke · strokeWidth · opacity · textColor · fontWeight · visible · text 이며,
 * 타입은 그보다 넓은 `Partial<ElementStyle>` 이다 — 좁히면 003 이 기하·정렬을 패치
 * 대상에 넣을 때 타입을 다시 바꿔야 한다. 설정 UI 가 노출 범위를 좁히는 축이다.
 */
export type StylePatch = Partial<ElementStyle> & { text?: string };

/** 규칙 표의 한 행. 표의 위에서부터 처음 일치한 행 하나만 이긴다(REQ-04). */
export interface RuleRow {
  op: RuleOp;
  /** `nodata` 행에서는 평가에 쓰이지 않는다(파서가 0 으로 정규화한다). */
  value: RuleValue;
  patch: StylePatch;
}

/** 상태 전이 트윈 명세. `duration_ms: 0` 이면 즉시 전환이며 루프를 깨우지 않는다. */
export interface TweenSpec {
  duration_ms: number;
  easing: TweenEasing;
}

/** 도형 종류와 무관한 요소 공통 필드. */
export interface CanvasElementBase {
  /** 안정 식별자(규칙/설정 UI 가 참조). 배열 안에서 유일하다. */
  id: string;
  /** 기본 스타일. */
  style: ElementStyle;
  /** 기본 문구 템플릿. kind:'text' 는 필수이나 파서는 결측을 탈락 사유로 보지 않는다. */
  text?: string;
  /** `{value}` 치환 시 소수 자리. 미지정이면 `DEFAULT_DECIMALS`. */
  decimals?: number;
  /** `{unit}` 치환 값. */
  unit?: string;
  /** 없으면 정적 도형(규칙 평가 대상이 아니다). */
  binding?: ElementBinding;
  /** 위에서부터 첫 일치 승리. 유효 행이 하나도 없으면 미지정과 같다. */
  rules?: RuleRow[];
  /** 패널 기본 트윈을 덮어쓴다. */
  tween?: TweenSpec;
}

/** 사각형 요소. */
export interface RectElement extends CanvasElementBase {
  kind: 'rect';
  geometry: BoxGeometry;
}

/** 타원 요소. */
export interface EllipseElement extends CanvasElementBase {
  kind: 'ellipse';
  geometry: BoxGeometry;
}

/** 선 요소. */
export interface LineElement extends CanvasElementBase {
  kind: 'line';
  geometry: LineGeometry;
}

/** 문구 요소. */
export interface TextElement extends CanvasElementBase {
  kind: 'text';
  geometry: PointGeometry;
}

/** 캔버스 요소 — `kind` 로 판별하는 합집합. */
export type CanvasElement = RectElement | EllipseElement | LineElement | TextElement;

/**
 * 캔버스 패널 config. 데이터 소스 축은 기존 차트 패널과 동일한
 * `ChartPanelConfigBase` 를 그대로 확장한다(신규 데이터 경로 없음, REQ-03).
 */
export interface CanvasPanelConfig extends ChartPanelConfigBase {
  /** 캔버스 배경색. 미지정이면 패널 표면색. */
  background?: string;
  /** 패널 기본 트윈. 요소가 덮어쓸 수 있다. */
  tween?: TweenSpec;
  /** 배열 순서 = 그리기 순서(뒤가 위). 001 의 유일한 z-order 수단. */
  elements: CanvasElement[];
}

// --- 원시 값 보정 도우미 -------------------------------------------------

/** 유한 숫자인지 확인한다(NaN/Infinity 방어). */
function isFiniteNumber(v: unknown): v is number {
  return typeof v === 'number' && Number.isFinite(v);
}

/** 객체가 아니면 빈 레코드로 본다(필드 접근을 예외 없이 만드는 죔쇠). */
function asRecord(raw: unknown): Record<string, unknown> {
  return raw && typeof raw === 'object' ? (raw as Record<string, unknown>) : {};
}

/** 값을 [0,1] 로 clamp 한다. */
function clamp01(v: number): number {
  return v < 0 ? 0 : v > 1 ? 1 : v;
}

/** 유한 숫자면 그대로, 아니면 폴백. 기하 좌표 보정에 쓴다. */
function coordinate(v: unknown, fallback: number): number {
  return isFiniteNumber(v) ? v : fallback;
}

/** 값이 아예 없는가(부재). null 도 부재로 본다 — JSON 왕복에서 흔한 형태다. */
function isAbsent(v: unknown): boolean {
  return v === undefined || v === null;
}

/** 비어 있지 않은 문자열만 통과시킨다. 빈 문자열은 "지정 안 함" 과 같다. */
function optionalString(v: unknown): string | undefined {
  return typeof v === 'string' && v.trim() !== '' ? v : undefined;
}

/**
 * 부재는 미지정(undefined)으로, **있는데 손상된 값**(비유한·음수·타입 불일치)은
 * 폴백으로 보정한다. 손상 값을 조용히 지우면 사용자가 화면에서 원인을 볼 수 없고,
 * 부재를 기본값으로 채우면 "지정 안 함" 이 사라진다 — 두 경우를 갈라 두는 이유다.
 */
function optionalNonNegative(v: unknown, fallback: number): number | undefined {
  if (isAbsent(v)) return undefined;
  return isFiniteNumber(v) && v >= 0 ? v : fallback;
}

/** 부재는 미지정, 유한 숫자는 0..1 clamp, 손상 값은 기본 불투명도로 보정한다. */
function optionalOpacity(v: unknown): number | undefined {
  if (isAbsent(v)) return undefined;
  return isFiniteNumber(v) ? clamp01(v) : DEFAULT_OPACITY;
}

// --- 부분 파서 -----------------------------------------------------------

/**
 * 기하 손상 정책: **요소를 버리지 않고 기본 기하로 대체한다**(필드 단위 폴백).
 *
 * 좌표 한 칸이 손상됐다고 요소를 버리면 그 요소의 스타일·규칙·바인딩이 저장 왕복
 * 한 번에 사라진다 — 사용자가 되돌릴 수 없는 손실이다. 기본 기하로 대체하면 요소가
 * 화면에 남아 사용자가 보고 고칠 수 있다. 요소를 버리는 것은 **정체성이 없을 때**
 * (id 결측·미지 kind)로 한정한다. 좌표는 0..1 로 clamp 하지 않는다 — 스테이지 밖으로
 * 일부 걸치는 배치도 뜻이 있는 저술이며, 잘라내면 사용자 의도가 조용히 바뀐다.
 */
function parseBoxGeometry(raw: unknown): BoxGeometry {
  const g = asRecord(raw);
  return {
    x: coordinate(g.x, DEFAULT_BOX_GEOMETRY.x),
    y: coordinate(g.y, DEFAULT_BOX_GEOMETRY.y),
    w: coordinate(g.w, DEFAULT_BOX_GEOMETRY.w),
    h: coordinate(g.h, DEFAULT_BOX_GEOMETRY.h),
  };
}

/** line 기하. 손상 필드는 기본 끝점으로 대체한다(위 정책 동일). */
function parseLineGeometry(raw: unknown): LineGeometry {
  const g = asRecord(raw);
  return {
    x1: coordinate(g.x1, DEFAULT_LINE_GEOMETRY.x1),
    y1: coordinate(g.y1, DEFAULT_LINE_GEOMETRY.y1),
    x2: coordinate(g.x2, DEFAULT_LINE_GEOMETRY.x2),
    y2: coordinate(g.y2, DEFAULT_LINE_GEOMETRY.y2),
  };
}

/** text 기하. 손상 필드는 기본 기준점으로 대체한다(위 정책 동일). */
function parsePointGeometry(raw: unknown): PointGeometry {
  const g = asRecord(raw);
  return {
    x: coordinate(g.x, DEFAULT_POINT_GEOMETRY.x),
    y: coordinate(g.y, DEFAULT_POINT_GEOMETRY.y),
  };
}

/**
 * 스타일을 파싱한다. enum 은 화이트리스트로 좁히고(미인정 값은 미지정으로 떨어진다),
 * 수치는 위 도우미의 "부재 = 미지정 / 손상 = 폴백" 규율을 따른다.
 */
function parseStyle(raw: unknown): ElementStyle {
  const out: ElementStyle = {};
  if (!raw || typeof raw !== 'object') return out;
  const s = raw as Record<string, unknown>;

  const fill = optionalString(s.fill);
  if (fill !== undefined) out.fill = fill;
  const stroke = optionalString(s.stroke);
  if (stroke !== undefined) out.stroke = stroke;
  const textColor = optionalString(s.textColor);
  if (textColor !== undefined) out.textColor = textColor;

  const strokeWidth = optionalNonNegative(s.strokeWidth, DEFAULT_STROKE_WIDTH);
  if (strokeWidth !== undefined) out.strokeWidth = strokeWidth;
  const fontSize = optionalNonNegative(s.fontSize, DEFAULT_FONT_SIZE);
  if (fontSize !== undefined) out.fontSize = fontSize;
  const opacity = optionalOpacity(s.opacity);
  if (opacity !== undefined) out.opacity = opacity;

  if (s.fontWeight === 'bold' || s.fontWeight === 'normal') out.fontWeight = s.fontWeight;
  if (s.align === 'left' || s.align === 'center' || s.align === 'right') out.align = s.align;
  if (typeof s.visible === 'boolean') out.visible = s.visible;

  return out;
}

/**
 * 규칙 패치를 파싱한다. 스타일 축은 `parseStyle` 과 같은 규율이고, 문구는 빈 문자열도
 * 뜻이 있어(라벨 지우기) 길이 검사 없이 문자열이면 통과시킨다.
 */
function parsePatch(raw: unknown): StylePatch {
  const patch: StylePatch = parseStyle(raw);
  if (raw && typeof raw === 'object') {
    const t = (raw as Record<string, unknown>).text;
    if (typeof t === 'string') patch.text = t;
  }
  return patch;
}

/** 규칙 행 1건. 연산자·임계값이 성립하지 않으면 그 행을 버린다(null). */
function parseRuleRow(raw: unknown): RuleRow | null {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null;
  const r = raw as Record<string, unknown>;
  const op = r.op;

  // 결측 판정 행. value 는 평가에 쓰이지 않으므로 형상 유지를 위해 0 으로 정규화한다.
  if (op === 'nodata') {
    return { op, value: 0, patch: parsePatch(r.patch) };
  }
  // 구간 행. 2원소 유한 숫자 튜플이 아니면 판정이 성립하지 않으므로 버린다.
  if (op === 'between') {
    const v = r.value;
    if (!Array.isArray(v) || v.length !== 2) return null;
    if (!isFiniteNumber(v[0]) || !isFiniteNumber(v[1])) return null;
    // 순서는 저술 그대로 둔다 — 정렬은 평가기(canvasRules)의 몫이다.
    return { op, value: [v[0], v[1]], patch: parsePatch(r.patch) };
  }
  if (op === 'gt' || op === 'gte' || op === 'lt' || op === 'lte' || op === 'eq' || op === 'ne') {
    if (!isFiniteNumber(r.value)) return null;
    return { op, value: r.value, patch: parsePatch(r.patch) };
  }
  // 미지 연산자는 화면에서 조용히 다른 판정으로 바뀌는 것보다 버리는 편이 안전하다.
  return null;
}

/**
 * 규칙 표. 배열이 아니면 미지정이며, 유효 행이 하나도 남지 않아도 미지정이다 —
 * 빈 표와 표 없음은 렌더에서 완전히 같은 상태라 죽은 필드를 남길 이유가 없다.
 */
function parseRules(raw: unknown): RuleRow[] | undefined {
  if (!Array.isArray(raw)) return undefined;
  const rows: RuleRow[] = [];
  for (const item of raw) {
    const row = parseRuleRow(item);
    if (row) rows.push(row);
  }
  return rows.length > 0 ? rows : undefined;
}

/** 바인딩. 시리즈 키가 없으면 정적 도형이다. 001 의 집계는 'last' 뿐이다. */
function parseBinding(raw: unknown): ElementBinding | undefined {
  if (!raw || typeof raw !== 'object') return undefined;
  const b = raw as Record<string, unknown>;
  const series = optionalString(b.series);
  if (series === undefined) return undefined;
  return { series, agg: 'last' };
}

/**
 * 트윈. 객체가 아니면 미지정(즉시 전환)이다. 지속 시간은 유한 비음수로 죄고,
 * 손상 값은 0(즉시 전환)으로 떨어뜨린다 — 임의의 기본 지속 시간을 넣으면 사용자가
 * 설정하지 않은 애니메이션이 생겨 루프가 깨어난다(REQ-05 유휴 정지).
 */
function parseTween(raw: unknown): TweenSpec | undefined {
  if (!raw || typeof raw !== 'object') return undefined;
  const t = raw as Record<string, unknown>;
  const duration = isFiniteNumber(t.duration_ms) && t.duration_ms > 0 ? t.duration_ms : 0;
  const easing: TweenEasing =
    t.easing === 'ease-in' || t.easing === 'ease-out' || t.easing === 'ease-in-out'
      ? t.easing
      : 'linear';
  return { duration_ms: duration, easing };
}

/** 요소 1건. 정체성(id · kind)이 성립하지 않으면 버린다(null). */
function parseElement(raw: unknown): CanvasElement | null {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null;
  const e = raw as Record<string, unknown>;

  const id = optionalString(e.id);
  if (id === undefined) return null;

  const kind = e.kind;
  if (kind !== 'rect' && kind !== 'ellipse' && kind !== 'line' && kind !== 'text') return null;

  const decimals =
    isFiniteNumber(e.decimals) && e.decimals >= 0 ? Math.trunc(e.decimals) : undefined;
  const unit = optionalString(e.unit);
  const binding = parseBinding(e.binding);
  const rules = parseRules(e.rules);
  const tween = parseTween(e.tween);

  const base: CanvasElementBase = {
    id,
    style: parseStyle(e.style),
    // 문구는 빈 문자열도 뜻이 있다(라벨 없음) — 길이 검사 없이 문자열이면 보존한다.
    ...(typeof e.text === 'string' ? { text: e.text } : {}),
    ...(decimals !== undefined ? { decimals } : {}),
    ...(unit !== undefined ? { unit } : {}),
    ...(binding !== undefined ? { binding } : {}),
    ...(rules !== undefined ? { rules } : {}),
    ...(tween !== undefined ? { tween } : {}),
  };

  switch (kind) {
    case 'rect':
      return { ...base, kind, geometry: parseBoxGeometry(e.geometry) };
    case 'ellipse':
      return { ...base, kind, geometry: parseBoxGeometry(e.geometry) };
    case 'line':
      return { ...base, kind, geometry: parseLineGeometry(e.geometry) };
    default:
      return { ...base, kind, geometry: parsePointGeometry(e.geometry) };
  }
}

/**
 * 요소 목록. 배열이 아니면 빈 목록이고, 정체성이 없는 항목은 버린다.
 * id 중복은 **먼저 온 것이 이긴다** — 규칙·설정 UI 가 id 로 요소를 지목하므로
 * 중복이 남으면 어느 쪽을 가리키는지 정할 수 없다.
 */
function parseElements(raw: unknown): CanvasElement[] {
  if (!Array.isArray(raw)) return [];
  const out: CanvasElement[] = [];
  const seen = new Set<string>();
  for (const item of raw) {
    const el = parseElement(item);
    if (!el) continue;
    if (seen.has(el.id)) continue;
    seen.add(el.id);
    out.push(el);
  }
  return out;
}

/** 데이터 소스 종류. 미인정 값은 미지정으로 떨어뜨린다(소스 판정 계약이 idle 로 방어). */
function parseDataSource(raw: unknown): ChartDataSourceKind | undefined {
  return raw === 'store' || raw === 'tsdb' || raw === 'sysmetrics' ? raw : undefined;
}

// --- 공개 API ------------------------------------------------------------

/**
 * 신규 캔버스 패널의 기본 config.
 *
 * 요소 0개로 시작한다 — 빈 캔버스는 오류가 아니라 안내 상태다(REQ-05). 데이터 소스는
 * 다른 패널과 같은 기본 store 소스(비활성: 에이전트·시리즈 미선택)이며, `channel_name`
 * 키는 비워 둔 채 남긴다(기존 패널 기본값 규약 동일).
 */
export function buildDefaultCanvasConfig(): CanvasPanelConfig {
  return {
    channel_name: '',
    data_source: 'store',
    store_source: buildDefaultStoreSource(),
    tween: { duration_ms: DEFAULT_TWEEN_DURATION_MS, easing: DEFAULT_TWEEN_EASING },
    elements: [],
  };
}

/**
 * 캔버스 패널 config 를 파싱한다.
 *
 * 어떤 입력(null·숫자·문자열·배열·손상 객체)에도 **예외를 던지지 않는다**. 알 수 없는
 * 필드는 보존하지 않고 무시하며, 결측 필드는 기본값으로 채운다(REQ-01/REQ-05, §위험 R3).
 */
export function parseCanvasConfig(raw: unknown): CanvasPanelConfig {
  // 배열도 객체지만 config 형상이 아니다 — 필드 접근이 전부 undefined 가 되어 기본값이 된다.
  const cfg = raw && typeof raw === 'object' && !Array.isArray(raw)
    ? (raw as Record<string, unknown>)
    : {};

  const background = optionalString(cfg.background);
  const dataSource = parseDataSource(cfg.data_source);
  const tween = parseTween(cfg.tween);

  return {
    // 채널 키는 문자열이 아니면 빈 문자열로 둔다 — 키가 사라지면 설정 화면이 편집할
    // 필드를 잃는다(uiStore.createDefaultPanel 의 `channel_name: ''` 주석과 같은 이유).
    channel_name: typeof cfg.channel_name === 'string' ? cfg.channel_name : '',
    ...(dataSource !== undefined ? { data_source: dataSource } : {}),
    // 소스 블록은 형상을 그대로 신뢰한다(히트맵 parseStoreSource 선례) — 무효 필드는
    // 조회 훅이 idle 로 방어하며, 여기서 손대면 사용자 설정이 조용히 사라진다.
    ...(cfg.store_source && typeof cfg.store_source === 'object'
      ? { store_source: cfg.store_source as StoreSourceConfig }
      : {}),
    ...(cfg.tsdb_source && typeof cfg.tsdb_source === 'object'
      ? { tsdb_source: cfg.tsdb_source as TsdbSourceConfig }
      : {}),
    ...(background !== undefined ? { background } : {}),
    ...(tween !== undefined ? { tween } : {}),
    elements: parseElements(cfg.elements),
  };
}
