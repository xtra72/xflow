// 캔버스 편집 수학 순수 함수 (SPEC-CANVAS-002 T7).
//
// 핸들 집합·핸들 px 좌표, 이동(`moveGeometry`), 크기 조절(`resizeBox`/`resizeLine`),
// 글자 크기 조절(`resizeFontSize`), 그리고 **기하 쓰기의 단일 통로**(`patchNodeGeometry`)를
// 담는다. **DOM 무의존**이다 — `document`/`window`/포인터 이벤트/React 를 참조하지 않으며,
// 포인터 자리는 평범한 숫자로 들어온다. 이벤트를 다루는 것은 오버레이 컴포넌트의 몫이고,
// 이 모듈은 그 수학만 안다(001 이 규칙·기하·트윈을 갈라 둔 것과 같은 규율).
//
// 세 가지를 특히 조심해서 지킨다.
//
//   1) **투영은 하나다**(위험 R1). 핸들 좌표를 자체 산술로 다시 만들지 않고
//      `canvasGeometry` 의 `projectBox`/`projectLine`/`projectPoint`/`resolveTextOrigin` 을
//      그대로 통과시킨다. 두 벌이 되면 핸들이 도형에서 미끄러지고, 그 결함은 "가끔
//      어긋난다" 로만 보고되어 원인을 찾기 어렵다.
//   2) **clamp 하지 않는다**(가정 A5). 캔버스 밖 배치는 합법인 저술이다. 이 모듈은
//      좌표를 캔버스 안으로 자르지 않으며, 자를 자리도 두지 않는다. 죄는 것은 **자릿수**
//      뿐이다 — 좌표계가 정수이므로 쓰기 직전에 반올림한다(아래 §기하 쓰기 단일 통로).
//   3) **음수 크기 박스를 만들지 않는다**(위험 R7). 001 이 음수 크기를 그릴 수 있게 해 둔
//      것은 **읽기 경로의 견고성**이지 편집기가 만들어야 할 값이 아니다. 정규화하지 않으면
//      뒤집는 순간 잡은 핸들이 커서에서 떨어져 나간다. 쓰기는 언제나 `x = min`, `w = |w|` 다.
//
// 종류별 비대칭을 감추지 않는다: `text` 에는 폭·높이가 없으므로 **박스 핸들이 없고**
// 글자 크기 핸들 하나가 `style.fontSize` 를 바꾼다. 없는 폭·높이를 편집기가 발명하면 그
// 값은 config 에 저장할 곳이 없다(`PanelDragLayer` 가 이미 같은 비대칭을 같은 방식으로
// 풀어 두었다).
//
// @spec SPEC-CANVAS-002 REQ-03 / REQ-04 / REQ-06

import {
  DEFAULT_FONT_SIZE,
  MIN_ELEMENT_EXTENT,
  type BoxGeometry,
  type CanvasElement,
  type CanvasElementKind,
  type Geometry,
  type LineGeometry,
  type PointGeometry,
} from './canvasConfig';
import {
  projectBox,
  projectLine,
  projectPoint,
  resolveTextOrigin,
  type CanvasDelta,
  type CanvasPoint,
  type CanvasProjection,
  type PxBox,
  type PxPoint,
} from './canvasGeometry';
import { FONT_SIZE_MAX, FONT_SIZE_MIN } from '../charts/statLayout';

// --- 타입 ---------------------------------------------------------------

/** 박스(rect/ellipse) 핸들 — 모서리 4 + 변 4. 나침반 약어가 곧 잡는 자리다. */
export type BoxHandleId = 'nw' | 'n' | 'ne' | 'e' | 'se' | 's' | 'sw' | 'w';

/** 박스 핸들 중 **모서리** 넷. 종횡비 유지(Shift)가 뜻을 갖는 것은 이 넷뿐이다. */
export type BoxCornerHandleId = 'nw' | 'ne' | 'se' | 'sw';

/** 선 핸들 — 두 끝점. 몸통 드래그는 핸들이 아니라 이동(`moveGeometry`)이다. */
export type LineHandleId = 'p1' | 'p2';

/** 문구 핸들 — 글자 크기 하나. 박스 핸들은 없다. */
export type TextHandleId = 'font';

/** 모든 핸들 식별자. 오버레이는 이 값을 `data-*`/`aria-label` 키로 그대로 쓴다. */
export type CanvasHandleId = BoxHandleId | LineHandleId | TextHandleId;

/**
 * 핸들이 **무엇을 쓰는가**. `text` 의 글자 크기 핸들만 기하가 아니라 스타일을 쓴다 —
 * 이 비대칭을 타입으로 드러내야 오버레이가 두 경로를 헷갈리지 않는다.
 */
export type CanvasHandleRole = 'geometry' | 'fontSize';

/** 자리까지 정해진 핸들 하나. `point` 는 스테이지 로컬 CSS px 다. */
export interface CanvasHandle {
  id: CanvasHandleId;
  role: CanvasHandleRole;
  point: PxPoint;
}

/** CSS px 공간의 이동량. 글자 크기 핸들만 이 단위를 쓴다(크기는 화면 양이다). */
export interface PxDelta {
  dx: number;
  dy: number;
}

/** 핸들 자리 계산 선택 인자. */
export interface HandleLayoutOptions {
  /**
   * 렌더 층이 잰 글자 폭(CSS px). `text` 의 글자 크기 핸들 자리를 정한다.
   * 아직 한 프레임도 그리지 않아 폭이 없으면 0 으로 보고 기준점 쪽에 붙인다.
   */
  measuredWidth?: number;
}

/** `resizeBox` 선택 인자. */
export interface ResizeBoxOptions {
  /** Shift: **모서리** 핸들에서 종횡비를 유지한다. 변 핸들에는 영향이 없다. */
  preserveAspect?: boolean;
}

/** `resizeLine` 선택 인자. */
export interface ResizeLineOptions {
  /** Shift: 끝점 방향을 45° 배수(0°/45°/90°…)로 죈다. */
  constrainAngle?: boolean;
}

// --- 상수 ---------------------------------------------------------------

/** 박스 핸들 8개. 순서는 시계 방향(북서 → 서)이며 오버레이의 탭 순서와 같다. */
export const BOX_HANDLE_IDS = ['nw', 'n', 'ne', 'e', 'se', 's', 'sw', 'w'] as const;

/** 선 핸들 2개. */
export const LINE_HANDLE_IDS = ['p1', 'p2'] as const;

/** 문구 핸들 1개 — 글자 크기. */
export const TEXT_HANDLE_IDS = ['font'] as const;

/** 모서리 핸들 4개(종횡비 유지 대상). */
export const BOX_CORNER_HANDLE_IDS = ['nw', 'ne', 'se', 'sw'] as const;

/**
 * 글자 크기 허용 범위(px). 통계 패널 손잡이(`statLayout`)와 **같은 값을 그대로 쓴다** —
 * 같은 대시보드 안에서 글자 손잡이가 패널마다 다른 한계를 가지면 사용자가 패널마다 다시
 * 배워야 한다. 하한 6 은 "글자가 사라져 화면에서 다시 잡을 수 없게 되는" 것을 막고,
 * 상한 160 은 한 글자가 스테이지를 덮어 다른 요소를 가리는 것을 막는다.
 *
 * 파서(`canvasConfig`)는 유한 비음수를 모두 받으므로 이 범위는 파서보다 **좁다**. 좁은
 * 쪽이 옳다 — 파서는 남이 쓴 값을 견디는 읽기 경로이고, 여기는 값을 만드는 쓰기 경로다.
 * 범위 밖 값을 수치 입력으로 저술하는 길은 그대로 열려 있다(REQ-01).
 */
export const CANVAS_FONT_SIZE_MIN = FONT_SIZE_MIN;
export const CANVAS_FONT_SIZE_MAX = FONT_SIZE_MAX;

/** 핸들 → 정규화된 px 박스 안의 상대 위치(0=시작 변, 1=끝 변). */
const BOX_HANDLE_FACTORS: Record<BoxHandleId, readonly [number, number]> = {
  nw: [0, 0],
  n: [0.5, 0],
  ne: [1, 0],
  e: [1, 0.5],
  se: [1, 1],
  s: [0.5, 1],
  sw: [0, 1],
  w: [0, 0.5],
};

/**
 * 핸들 → 움직이는 변. `h`: -1 왼쪽 변 · +1 오른쪽 변 · 0 가로 고정.
 * `v`: -1 위쪽 변 · +1 아래쪽 변 · 0 세로 고정.
 */
const BOX_HANDLE_EDGES: Record<BoxHandleId, { h: -1 | 0 | 1; v: -1 | 0 | 1 }> = {
  nw: { h: -1, v: -1 },
  n: { h: 0, v: -1 },
  ne: { h: 1, v: -1 },
  e: { h: 1, v: 0 },
  se: { h: 1, v: 1 },
  s: { h: 0, v: 1 },
  sw: { h: -1, v: 1 },
  w: { h: -1, v: 0 },
};

/** 각도 죔 간격 — 45°. 0°/45°/90° 와 그 대칭 방향이 모두 이 배수다. */
const ANGLE_STEP = Math.PI / 4;

// --- 수치 보정 도우미 ---------------------------------------------------

/** 유한 숫자면 그대로, 아니면 0(`canvasGeometry` 와 같은 규율). */
function finite(v: number): number {
  return Number.isFinite(v) ? v : 0;
}

/** 두 축이 모두 유한한 포인터인가. 비유한 포인터는 **조작을 무시**하는 근거가 된다. */
function isFinitePoint(p: CanvasPoint): boolean {
  return Number.isFinite(p.x) && Number.isFinite(p.y);
}

/** 박스 기하의 손상 필드를 0 으로 떨군다. */
function sanitizeBox(geo: BoxGeometry): BoxGeometry {
  return { x: finite(geo.x), y: finite(geo.y), w: finite(geo.w), h: finite(geo.h) };
}

/** 선 기하의 손상 필드를 0 으로 떨군다. */
function sanitizeLine(geo: LineGeometry): LineGeometry {
  return {
    x1: finite(geo.x1),
    y1: finite(geo.y1),
    x2: finite(geo.x2),
    y2: finite(geo.y2),
  };
}

/** 점 기하의 손상 필드를 0 으로 떨군다. */
function sanitizePoint(geo: PointGeometry): PointGeometry {
  return { x: finite(geo.x), y: finite(geo.y) };
}

/**
 * 박스를 양수 범위로 정규화한다(`x = min`, `w = |w|`).
 *
 * **쓰기 직전에 반드시 지나는 관문이다**(위험 R7). 읽기 경로는 음수 크기를 견디지만
 * 편집기는 만들지 않는다.
 */
function normalizeBox(box: BoxGeometry): BoxGeometry {
  const b = sanitizeBox(box);
  return {
    x: Math.min(b.x, b.x + b.w),
    y: Math.min(b.y, b.y + b.h),
    w: Math.abs(b.w),
    h: Math.abs(b.h),
  };
}

/** 투영된 px 박스를 양수 범위로 정규화한다(핸들 자리 계산용). */
function normalizePxBox(box: PxBox): PxBox {
  const x = finite(box.x);
  const y = finite(box.y);
  const w = finite(box.w);
  const h = finite(box.h);
  return { x: Math.min(x, x + w), y: Math.min(y, y + h), w: Math.abs(w), h: Math.abs(h) };
}

// --- 핸들 모델 ----------------------------------------------------------

/** 모서리 핸들인가(종횡비 유지가 뜻을 갖는가). */
export function isCornerHandle(id: CanvasHandleId): id is BoxCornerHandleId {
  return (BOX_CORNER_HANDLE_IDS as readonly string[]).includes(id);
}

/**
 * 핸들이 무엇을 쓰는가. 글자 크기 핸들만 `style.fontSize` 를 쓰고 나머지는 기하를 쓴다.
 */
export function handleRole(id: CanvasHandleId): CanvasHandleRole {
  return id === 'font' ? 'fontSize' : 'geometry';
}

/**
 * 종류별 핸들 집합.
 *
 * rect·ellipse 는 8개(모서리 4 + 변 4), line 은 끝점 2개, **text 는 글자 크기 하나뿐이며
 * 박스 핸들이 없다**. 이 비대칭은 결함이 아니라 명세다(REQ-03).
 */
export function handlesFor(kind: CanvasElementKind): readonly CanvasHandleId[] {
  switch (kind) {
    case 'rect':
    case 'ellipse':
      return BOX_HANDLE_IDS;
    case 'line':
      return LINE_HANDLE_IDS;
    default:
      return TEXT_HANDLE_IDS;
  }
}

/**
 * 요소의 핸들들을 **스테이지 로컬 CSS px** 자리까지 풀어 돌려준다.
 *
 * 좌표는 전부 `canvasGeometry` 의 투영을 지나므로 렌더와 갈라질 수 없다(위험 R1).
 * 스테이지 크기는 표면(`CanvasSurface`)이 잰 것을 그대로 받는다 — 이 모듈은 재지 않는다.
 *
 * `text` 의 글자 크기 핸들은 글자 상자의 **오른쪽 아래 모서리**에 둔다. 상자의 세로
 * 중심이 기준점이므로(`drawElement.TEXT_BASELINE === 'middle'`) 아래 변은
 * `기준점 y + fontSize/2` 이며, 실측 폭이 아직 없으면 폭 0 으로 보아 기준점에 붙는다.
 */
export function handlePositions(
  el: CanvasElement,
  proj: CanvasProjection,
  opts: HandleLayoutOptions = {},
): CanvasHandle[] {
  switch (el.kind) {
    case 'rect':
    case 'ellipse': {
      const box = normalizePxBox(projectBox(el.geometry, proj));
      return BOX_HANDLE_IDS.map((id): CanvasHandle => {
        const [fx, fy] = BOX_HANDLE_FACTORS[id];
        return {
          id,
          role: handleRole(id),
          point: { x: box.x + box.w * fx, y: box.y + box.h * fy },
        };
      });
    }
    case 'line': {
      const line = projectLine(el.geometry, proj);
      return [
        { id: 'p1', role: handleRole('p1'), point: { x: line.x1, y: line.y1 } },
        { id: 'p2', role: handleRole('p2'), point: { x: line.x2, y: line.y2 } },
      ];
    }
    default: {
      const fontSize = clampCanvasFontSize(el.style.fontSize ?? DEFAULT_FONT_SIZE);
      const width = Math.max(0, finite(opts.measuredWidth ?? 0));
      const origin = resolveTextOrigin(
        projectPoint(el.geometry, proj),
        el.style.align ?? 'left',
        width,
      );
      return [
        {
          id: 'font',
          role: handleRole('font'),
          point: { x: origin.x + width, y: origin.y + fontSize / 2 },
        },
      ];
    }
  }
}

// --- 이동 ---------------------------------------------------------------

/**
 * 기하를 캔버스 단위 델타만큼 옮긴다.
 *
 * rect/ellipse 는 좌상단만, line 은 **두 끝점 모두**, text 는 기준점을 옮긴다.
 * **clamp 하지 않는다** — 캔버스 밖으로 나간 배치도 뜻이 있는 저술이다(가정 A5).
 * **정수화도 하지 않는다** — 그 일은 쓰기 통로(`patchNodeGeometry`) 한 곳의 몫이다.
 * 회수 경로는 목록 편집기의 수치 입력이며, 그것이 항상 남아 있어야 하는 이유 중 하나다.
 */
export function moveGeometry(geometry: BoxGeometry, delta: CanvasDelta): BoxGeometry;
export function moveGeometry(geometry: LineGeometry, delta: CanvasDelta): LineGeometry;
export function moveGeometry(geometry: PointGeometry, delta: CanvasDelta): PointGeometry;
export function moveGeometry(geometry: Geometry, delta: CanvasDelta): Geometry;
export function moveGeometry(geometry: Geometry, delta: CanvasDelta): Geometry {
  const dx = finite(delta.dx);
  const dy = finite(delta.dy);

  if ('w' in geometry) {
    const b = sanitizeBox(geometry);
    return { x: b.x + dx, y: b.y + dy, w: b.w, h: b.h };
  }
  if ('x1' in geometry) {
    const l = sanitizeLine(geometry);
    return { x1: l.x1 + dx, y1: l.y1 + dy, x2: l.x2 + dx, y2: l.y2 + dy };
  }
  const p = sanitizePoint(geometry);
  return { x: p.x + dx, y: p.y + dy };
}

// --- 크기 조절 ----------------------------------------------------------

/**
 * 8개 핸들 중 하나로 박스를 다시 잡는다. 결과는 **언제나 양수 크기로 정규화**된다.
 *
 * 잡은 핸들이 반대 변을 넘어가면 박스가 뒤집힌다. 그때 `x = min`, `w = |w|` 로 정규화해야
 * 잡은 핸들이 커서 아래에 남는다 — 정규화하지 않으면 뒤집는 순간 손잡이가 커서에서
 * 떨어져 나가고, 그 뒤의 편집이 예측 불가가 된다(위험 R7).
 *
 * `preserveAspect`(Shift)는 **모서리에서만** 뜻이 있다. 잡지 않은 반대 모서리를 앵커로
 * 두고 원래 종횡비를 유지하되, 커서에서 더 멀리 간 축을 기준으로 삼아 손이 가는 쪽을
 * 따라간다. 퇴화 박스(폭 또는 높이 0)에는 유지할 비가 없으므로 그대로 자유 조절한다.
 *
 * 비유한 포인터(측정 실패·이벤트 손상)는 **조작을 무시**한다 — 0 으로 떨어뜨리면 박스가
 * 원점으로 튀어 사용자가 한 일이 소리 없이 사라진다.
 */
export function resizeBox(
  box: BoxGeometry,
  handle: BoxHandleId,
  pointer: CanvasPoint,
  opts: ResizeBoxOptions = {},
): BoxGeometry {
  const base = normalizeBox(box);
  if (!isFinitePoint(pointer)) return base;

  const edges = BOX_HANDLE_EDGES[handle];
  let left = base.x;
  let right = base.x + base.w;
  let top = base.y;
  let bottom = base.y + base.h;

  if (opts.preserveAspect && isCornerHandle(handle) && base.w > 0 && base.h > 0) {
    // 앵커는 잡지 않은 반대 모서리다. 그 자리에서 본 부호 있는 변위로 비를 맞춘다.
    const anchorX = edges.h === -1 ? right : left;
    const anchorY = edges.v === -1 ? bottom : top;
    const dx = pointer.x - anchorX;
    const dy = pointer.y - anchorY;
    const aspect = base.w / base.h;

    let width = Math.abs(dx);
    let height = Math.abs(dy);
    if (width / aspect >= height) height = width / aspect;
    else width = height * aspect;

    left = anchorX;
    top = anchorY;
    right = anchorX + (dx < 0 ? -width : width);
    bottom = anchorY + (dy < 0 ? -height : height);
  } else {
    if (edges.h === -1) left = pointer.x;
    else if (edges.h === 1) right = pointer.x;
    if (edges.v === -1) top = pointer.y;
    else if (edges.v === 1) bottom = pointer.y;
  }

  return {
    x: Math.min(left, right),
    y: Math.min(top, bottom),
    w: Math.abs(right - left),
    h: Math.abs(bottom - top),
  };
}

/**
 * 선의 한 끝점만 옮긴다. 몸통 드래그(두 끝점 동시 이동)는 `moveGeometry` 다.
 *
 * `constrainAngle`(Shift)은 **고정된 반대 끝점에서 본 방향**을 45° 배수로 죄고, 끌린
 * 거리는 그대로 유지한다. 죔이 캔버스 단위 공간에서 일어나므로, 스테이지의 종횡비가
 * 캔버스의 종횡비와 다르면 화면상 각도는 45° 에서 조금 벗어난다 — 저장되는 값이 캔버스
 * 좌표이고 그 값이 곧 저술 결과이므로, 화면 각도를 맞추려고 스테이지 크기를 이 순수
 * 모듈에 들이지 않는다.
 *
 * 비유한 포인터는 `resizeBox` 와 같은 이유로 조작을 무시한다.
 */
export function resizeLine(
  line: LineGeometry,
  endpoint: LineHandleId,
  pointer: CanvasPoint,
  opts: ResizeLineOptions = {},
): LineGeometry {
  const base = sanitizeLine(line);
  if (!isFinitePoint(pointer)) return base;

  const anchorX = endpoint === 'p1' ? base.x2 : base.x1;
  const anchorY = endpoint === 'p1' ? base.y2 : base.y1;

  let px = pointer.x;
  let py = pointer.y;
  if (opts.constrainAngle) {
    const dx = px - anchorX;
    const dy = py - anchorY;
    const length = Math.hypot(dx, dy);
    const angle = Math.round(Math.atan2(dy, dx) / ANGLE_STEP) * ANGLE_STEP;
    px = anchorX + length * Math.cos(angle);
    py = anchorY + length * Math.sin(angle);
  }

  return endpoint === 'p1'
    ? { x1: px, y1: py, x2: base.x2, y2: base.y2 }
    : { x1: base.x1, y1: base.y1, x2: px, y2: py };
}

// --- 글자 크기(기하가 아니다) -------------------------------------------

/**
 * 글자 크기를 허용 범위로 죈다. 비유한 값은 기본 크기로 떨어뜨린다 —
 * NaN 을 그대로 저장하면 파서가 폴백을 먹여 다음에 열 때 값이 조용히 달라진다.
 */
export function clampCanvasFontSize(v: number): number {
  if (!Number.isFinite(v)) return DEFAULT_FONT_SIZE;
  return Math.min(Math.max(v, CANVAS_FONT_SIZE_MIN), CANVAS_FONT_SIZE_MAX);
}

/**
 * 글자 크기 핸들의 결과. **기하가 아니라 `style.fontSize` 를 쓴다**(REQ-03).
 *
 * 델타는 CSS px 다 — 크기는 화면 양이므로 정규화 공간에서 재면 패널이 클수록 손이
 * 더 크게 움직여야 같은 크기 변화를 얻는다. 대각선으로 끌 때 포인터보다 빨리 커지지
 * 않도록 두 축의 평균을 쓴다(`PanelDragLayer` 가 같은 이유로 같은 식을 쓴다).
 *
 * 시작 크기가 0 이거나 비유한이면 기본 크기에서 시작한다 — 0 은 화면에서 글자가 사라진
 * 상태라 손잡이의 시작점이 될 수 없다.
 */
export function resizeFontSize(baseFontSize: number, delta: PxDelta): number {
  const base = Number.isFinite(baseFontSize) && baseFontSize > 0 ? baseFontSize : DEFAULT_FONT_SIZE;
  return clampCanvasFontSize(base + (finite(delta.dx) + finite(delta.dy)) / 2);
}

// --- 기하 쓰기 단일 통로 (REQ-06) ---------------------------------------
//
// ## 정수화와 퇴화 방지가 이 절에 있는 이유
//
// 좌표계가 정수이므로 어디선가 반올림해야 하고, **어디서 하느냐가 곧 설계**다. 조작
// 함수마다 반올림하면 이동은 이동대로 크기 조절은 크기대로 자기 반올림을 갖게 되어,
// 같은 손짓이 경로에 따라 한 단위씩 다른 곳에 떨어진다. 그래서 조작 함수들은 실수를
// 그대로 돌려주고 **쓰기 직전 한 곳**에서만 정수가 된다 — 004 가 그룹 분기를 더할 자리를
// 하나로 두려던 그 이유와 같은 이유다.
//
// 같은 자리에서 **퇴화도 막는다**(`MIN_ELEMENT_EXTENT`). 읽는 쪽(파서)은 퇴화 기하를
// 씨앗으로 되살리지만, 그 되살림이 드래그 중에 일어나면 손잡이를 반대 변까지 끌었을 때
// 요소가 화면 반대편으로 **순간이동**한다. 쓰는 쪽이 애초에 만들지 않으면 그 되살림은
// 옛 config 를 읽을 때에만 쓰이며, 그것이 두 규율의 옳은 분담이다.

/** 쓰기용 박스 — 정수로 반올림하고 최소 크기를 보장한다. 음수 크기는 만들지 않는다. */
function writableBox(geo: BoxGeometry): BoxGeometry {
  const b = sanitizeBox(geo);
  const w = Math.round(Math.abs(b.w));
  const h = Math.round(Math.abs(b.h));
  return {
    x: Math.round(b.x),
    y: Math.round(b.y),
    w: Math.max(MIN_ELEMENT_EXTENT, w),
    h: Math.max(MIN_ELEMENT_EXTENT, h),
  };
}

/**
 * 쓰기용 선 — 정수로 반올림하고 **길이 0 을 만들지 않는다**.
 *
 * 두 끝점이 같아지면 두 번째 점을 한 단위 밀어 둔다. 씨앗 선으로 되돌리지 않는 것에 뜻이
 * 있다 — 지금 손에 쥐고 끄는 중인 선이 갑자기 캔버스를 가로지르면 그것이야말로 순간이동이며,
 * 한 단위 짜리 선은 사용자가 손을 조금만 더 움직이면 곧바로 자란다.
 */
function writableLine(geo: LineGeometry): LineGeometry {
  const l = sanitizeLine(geo);
  const x1 = Math.round(l.x1);
  const y1 = Math.round(l.y1);
  const x2 = Math.round(l.x2);
  const y2 = Math.round(l.y2);
  const degenerate = x1 === x2 && y1 === y2;
  return { x1, y1, x2: degenerate ? x2 + MIN_ELEMENT_EXTENT : x2, y2 };
}

/** 쓰기용 기준점 — 정수로 반올림한다. 넓이가 없으므로 퇴화도 없다. */
function writablePoint(geo: PointGeometry): PointGeometry {
  const p = sanitizePoint(geo);
  return { x: Math.round(p.x), y: Math.round(p.y) };
}

/**
 * **모든 기하 쓰기가 지나는 한 함수**(REQ-06). 새 배열을 돌려주며 입력을 건드리지 않는다.
 *
 * 004 가 `group` 노드를 얹을 때 그룹 분기를 더할 지점이 **한 곳**이 되게 하려고 존재한다.
 * 그래서 이 모듈의 다른 함수는 전부 **벌거벗은 기하 값**(또는 숫자)만 돌려주고, 요소
 * 배열을 받아 요소 배열을 돌려주는 export 는 이 함수 하나뿐이다 — 다른 경로로 기하를
 * 요소에 써 넣는 것이 형상 자체로 불가능하다.
 *
 * 식별은 언제나 `nodeId` 다. 배열 위치(index)를 비동기 경계 너머로 들고 다니지 않는다 —
 * 드래그 중 요소가 재정렬되면 index 는 다른 것을 가리킨다(REQ-06).
 *
 * 요소 종류와 기하 형상이 어긋나면(예: rect 에 선 기하) 그 요소를 **그대로 둔다**.
 * 형상이 맞으면 손상 필드를 떨구고(`finite`) **정수로 반올림하며 퇴화를 막는다**(위 §정수화).
 */
export function patchNodeGeometry(
  elements: readonly CanvasElement[],
  nodeId: string,
  geometry: Geometry,
): CanvasElement[] {
  return elements.map((el): CanvasElement => {
    if (el.id !== nodeId) return el;

    switch (el.kind) {
      case 'rect':
      case 'ellipse': {
        if (!('w' in geometry)) return el;
        return { ...el, geometry: writableBox(geometry) };
      }
      case 'line': {
        if (!('x1' in geometry)) return el;
        return { ...el, geometry: writableLine(geometry) };
      }
      case 'text': {
        if ('w' in geometry) return el;
        if ('x1' in geometry) return el;
        return { ...el, geometry: writablePoint(geometry) };
      }
      // 경로는 rect 와 **같은 상자 기하**를 갖지만 이 자리는 아직 그것을 쓰지 않는다 —
      // 상자 쓰기를 붙이는 것은 **M5 의 몫이다.** 그때까지 경로는 끌리지도 크기가 바뀌지도
      // 않는다. `default:` 를 갈래 둘로 편 것이 이 변경의 절반이다: 종전의 `default:` 는
      // 새 종류를 **문구 요소로 읽어** 조용히 삼켰고, 갈래를 이름으로 적어 두면 여섯 번째
      // 종류가 들어올 때 컴파일러가 이 자리를 다시 가리킨다.
      case 'path':
        return el;
    }
  });
}
