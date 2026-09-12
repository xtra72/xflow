// 단축 요소를 경로 명령으로 (SPEC-CANVAS-007 M3).
//
// **이 모듈은 DOM 을 모른다.** 받는 것은 **속성 자루**(이름 → 문자열)이고 내는 것은 명령
// 목록이다. 문서 층(M5)이 노드에서 속성을 긁어 이 자루를 만든다 — 그래야 여섯 도형의
// 산술이 jsdom 의 SVG 거동과 얽히지 않는다(불변식 K7).
//
// **모서리와 원호를 `svgArc.arcToCubics` 로 만든다.** 4분원 상수(`k ≈ 0.5523`)를 여기 다시
// 적는 안을 기각한다 — 그러면 저장소에 호 근사가 둘이 되고, 하나를 고쳐도 다른 하나가
// 남는다. 호 산술은 한 곳이다.
//
// **`<line>` 과 `<polyline>` 을 `Z` 로 닫지 않는다.** 닫으면 채움이 생기고, 008 의
// `pathSeedStyle` 은 `Z` 가 있으면 **채움 씨앗**을 심는다(실측 `canvasElementFactory.ts:225`).
// 열린 것을 닫으면 **저술한 적 없는 변**이 하나 생겨 화면에 정체 모를 덩어리가 나온다 —
// 008 이 곡선 화살표에 대해 이미 이름으로 적은 그 결함이다.
//
// **감김 방향은 SVG 사양의 등가 경로 그대로다**(`rect` · `circle` · `ellipse` 모두
// `sweep = 1`). 브라우저가 그리는 방향과 같아야 nonzero 채움에서 도넛의 구멍이 뚫린다.
//
// @spec SPEC-CANVAS-007 REQ-02 · AC-03

import type { PathCommand } from '../shapes/pathTypes';

import { arcToCubics } from './svgArc';
import type { ImportedNative } from './svgImportTypes';
import { parseLength, parseNumberList } from './svgPathData';

/** 요소 하나의 속성 자루. 문서 층이 노드에서 긁어 만든다. */
export type AttrBag = Readonly<Record<string, string | undefined>>;

/** 단축 요소 여섯(`<path>` 는 제 축약기가 따로 있다). */
export const SHORTHAND_SHAPE_TAGS = ['rect', 'circle', 'ellipse', 'line', 'polyline', 'polygon'] as const;

export type ShorthandShapeTag = (typeof SHORTHAND_SHAPE_TAGS)[number];

export function isShorthandShapeTag(tag: string): tag is ShorthandShapeTag {
  return (SHORTHAND_SHAPE_TAGS as readonly string[]).includes(tag);
}

/** 결측·비수치 길이는 0 으로 읽는다(SVG 의 기하 속성 기본값). */
function length(attrs: AttrBag, name: string): number {
  return parseLength(attrs[name]) ?? 0;
}

/**
 * 모서리 반지름 하나 — **음수·비유한은 "없음" 으로 읽는다.**
 *
 * SVG 사양은 음수 `rx` 를 오류로 정하고 속성이 지정되지 않은 것처럼 다루게 한다.
 */
function cornerRadius(attrs: AttrBag, name: string): number | undefined {
  const value = parseLength(attrs[name]);
  if (value === undefined || !Number.isFinite(value) || value < 0) return undefined;
  return value;
}

/** 90° 호 하나를 3차로 — 모서리와 4분원이 같은 산술을 지난다. */
function quarterArc(
  from: { x: number; y: number },
  rx: number,
  ry: number,
  to: { x: number; y: number },
): PathCommand[] {
  return arcToCubics(from.x, from.y, rx, ry, 0, 0, 1, to.x, to.y);
}

/** `<rect>` 가 말한 수 여섯. `width`/`height` 가 그려질 크기가 아니면 `undefined`. */
interface RectMetrics {
  readonly x: number;
  readonly y: number;
  readonly w: number;
  readonly h: number;
  readonly rx: number;
  readonly ry: number;
}

/**
 * `<rect>` 의 여섯 수를 **한 곳에서** 읽는다.
 *
 * 명령 축약기와 원시형 판정이 이 함수를 **함께** 부른다. 두 곳이 각자 읽으면 `rx` 만 있을
 * 때 `ry = rx` 로 읽는 규칙이나 `w/2` 로 죄는 규칙이 한쪽에서만 고쳐질 수 있고, 그때
 * "명령은 깎인 모서리를 그리는데 요소는 사각형" 이 된다.
 */
function rectMetrics(attrs: AttrBag): RectMetrics | undefined {
  const w = length(attrs, 'width');
  const h = length(attrs, 'height');
  if (!(w > 0) || !(h > 0)) return undefined;
  const rawRx = cornerRadius(attrs, 'rx');
  const rawRy = cornerRadius(attrs, 'ry');
  return {
    x: length(attrs, 'x'),
    y: length(attrs, 'y'),
    w,
    h,
    rx: Math.min(rawRx ?? rawRy ?? 0, w / 2),
    ry: Math.min(rawRy ?? rawRx ?? 0, h / 2),
  };
}

/**
 * 모서리가 깎이지 **않았는가** — 갈래를 정하는 술어는 이것 하나다.
 *
 * 축약기는 이 술어로 곧은 네 변과 호 넷을 가르고, 원시형 판정은 **같은 술어로** 캔버스
 * 사각형이 될 수 있는지를 가른다. 캔버스의 `rect` 는 모서리 반지름을 나르지 않으므로
 * (`BoxGeometry` 는 네 수뿐이다) 깎인 모서리는 경로로 남아야 하고, 그 경계가 두 곳에서
 * 따로 정해지면 어느 날 한쪽만 움직인다.
 */
function isSharpCorner(m: RectMetrics): boolean {
  return !(m.rx > 0) || !(m.ry > 0);
}

export function rectCommands(attrs: AttrBag): PathCommand[] {
  const metrics = rectMetrics(attrs);
  if (metrics === undefined) return [];
  const { x, y, w, h, rx, ry } = metrics;

  if (isSharpCorner(metrics)) {
    return [
      { c: 'M', x, y },
      { c: 'L', x: x + w, y },
      { c: 'L', x: x + w, y: y + h },
      { c: 'L', x, y: y + h },
      { c: 'Z' },
    ];
  }

  const corners: { from: { x: number; y: number }; to: { x: number; y: number } }[] = [
    { from: { x: x + w - rx, y }, to: { x: x + w, y: y + ry } },
    { from: { x: x + w, y: y + h - ry }, to: { x: x + w - rx, y: y + h } },
    { from: { x: x + rx, y: y + h }, to: { x, y: y + h - ry } },
    { from: { x, y: y + ry }, to: { x: x + rx, y } },
  ];
  const out: PathCommand[] = [{ c: 'M', x: x + rx, y }];
  for (const corner of corners) {
    out.push({ c: 'L', x: corner.from.x, y: corner.from.y });
    for (const cubic of quarterArc(corner.from, rx, ry, corner.to)) out.push(cubic);
  }
  out.push({ c: 'Z' });
  return out;
}

/** `<ellipse cx cy rx ry>` — 4분원 넷. 반지름이 0 이하면 그릴 것이 없다. */
export function ellipseCommands(attrs: AttrBag): PathCommand[] {
  const cx = length(attrs, 'cx');
  const cy = length(attrs, 'cy');
  const rx = cornerRadius(attrs, 'rx') ?? 0;
  const ry = cornerRadius(attrs, 'ry') ?? 0;
  return ellipseAt(cx, cy, rx, ry);
}

/** `<circle cx cy r>` — 두 반지름이 같은 타원이다. */
export function circleCommands(attrs: AttrBag): PathCommand[] {
  const r = cornerRadius(attrs, 'r') ?? 0;
  return ellipseAt(length(attrs, 'cx'), length(attrs, 'cy'), r, r);
}

function ellipseAt(cx: number, cy: number, rx: number, ry: number): PathCommand[] {
  if (!(rx > 0) || !(ry > 0)) return [];
  // SVG 1.1 §9.4 의 등가 경로 — 시작점은 오른쪽 끝이고 `sweep = 1` 로 한 바퀴 돈다.
  const stops = [
    { x: cx, y: cy + ry },
    { x: cx - rx, y: cy },
    { x: cx, y: cy - ry },
    { x: cx + rx, y: cy },
  ];
  const out: PathCommand[] = [{ c: 'M', x: cx + rx, y: cy }];
  let from = { x: cx + rx, y: cy };
  for (const to of stops) {
    for (const cubic of quarterArc(from, rx, ry, to)) out.push(cubic);
    from = to;
  }
  out.push({ c: 'Z' });
  return out;
}

/**
 * `<line x1 y1 x2 y2>` — **열린다.** `Z` 를 붙이지 않는다(위 머리말).
 *
 * 두 끝점이 같아도 목록을 낸다 — 퇴화 상자의 처리는 상자를 정하는 층(M6)의 몫이고,
 * 여기서 도형을 지우면 그 층이 볼 것이 없어진다.
 */
export function lineCommands(attrs: AttrBag): PathCommand[] {
  return [
    { c: 'M', x: length(attrs, 'x1'), y: length(attrs, 'y1') },
    { c: 'L', x: length(attrs, 'x2'), y: length(attrs, 'y2') },
  ];
}

/**
 * `points` 속성 → 점 목록. **홀수로 남은 마지막 수는 버린다** — 짝을 이루지 못한 좌표는
 * 점이 아니다(사양: 오류 지점까지 렌더).
 */
function parsePoints(raw: string | undefined): { x: number; y: number }[] {
  if (raw === undefined) return [];
  const numbers = parseNumberList(raw);
  const points: { x: number; y: number }[] = [];
  for (let i = 0; i + 1 < numbers.length; i += 2) {
    const x = numbers[i];
    const y = numbers[i + 1];
    // 앞의 두 조건은 **루프 상한과 같은 가드를 한 번 더 적은 것**이다 —
    // `noUncheckedIndexedAccess` 가 인덱스 접근을 `| undefined` 로 주므로 타입에는
    // 필요하지만, 런타임에는 도달하지 않는다(둘 중 하나만 지워도 거동이 같다). 실제로
    // 무는 것은 뒤의 유한 검사이며, `1e999` 이 든 `points` 가 그것을 잰다.
    if (x === undefined || y === undefined || !Number.isFinite(x) || !Number.isFinite(y)) continue;
    points.push({ x, y });
  }
  return points;
}

function polyCommands(attrs: AttrBag, close: boolean): PathCommand[] {
  const points = parsePoints(attrs['points']);
  const first = points[0];
  // 점 하나짜리는 그려질 것이 없다 — 변이 없고 채울 넓이도 없다.
  if (first === undefined || points.length < 2) return [];
  const out: PathCommand[] = [{ c: 'M', x: first.x, y: first.y }];
  for (let i = 1; i < points.length; i += 1) {
    const p = points[i];
    if (p === undefined) continue;
    out.push({ c: 'L', x: p.x, y: p.y });
  }
  if (close) out.push({ c: 'Z' });
  return out;
}

/** `<polyline points>` — **열린다.** */
export function polylineCommands(attrs: AttrBag): PathCommand[] {
  return polyCommands(attrs, false);
}

/** `<polygon points>` — 닫힌다. */
export function polygonCommands(attrs: AttrBag): PathCommand[] {
  return polyCommands(attrs, true);
}

/**
 * 태그가 말한 **원시 도형** — 캔버스가 이미 가진 종류로 그대로 옮길 수 있는 것들.
 *
 * 좌표는 사용자 단위이고 **변환은 아직 녹아 있지 않다**(축약기가 내는 명령과 같은 자리다).
 * 변환을 녹이는 것도, 그 변환이 이 도형을 원시형으로 둘 수 있게 하는지 재는 것도 문서
 * 층의 몫이다 — 이 모듈은 행렬을 모른다.
 *
 * **`undefined` 는 "경로로 남는다" 를 뜻한다.** 세 갈래가 그렇게 떨어진다:
 *   - `<polygon>` · `<polyline>` · `<path>` — 캔버스에 그 종류가 없다.
 *   - 모서리가 깎인 `<rect>` — `BoxGeometry` 가 모서리 반지름을 나르지 않는다.
 *   - 그려질 것이 없는 도형(`width <= 0` · `r <= 0`) — 축약기도 빈 목록을 내고, 그때
 *     문서 층이 아무 요소도 세우지 않는다. 여기서만 원시형을 내면 **축약기가 버린 도형이
 *     요소로 되살아난다.**
 */
export function shorthandNativeShape(tag: string, attrs: AttrBag): ImportedNative | undefined {
  switch (tag) {
    case 'rect': {
      const m = rectMetrics(attrs);
      if (m === undefined || !isSharpCorner(m)) return undefined;
      return { kind: 'rect', minX: m.x, minY: m.y, maxX: m.x + m.w, maxY: m.y + m.h };
    }
    case 'circle': {
      const r = cornerRadius(attrs, 'r') ?? 0;
      return ellipseNative(length(attrs, 'cx'), length(attrs, 'cy'), r, r);
    }
    case 'ellipse': {
      // **반지름을 `ellipseCommands` 와 똑같이 읽는다** — 그 함수는 `rx` 만 있는 `<ellipse>`
      // 에 `ry = rx` 를 채우지 않고 0 으로 읽어 빈 목록을 낸다. 여기서 채우면 축약기가
      // 그리지 않기로 한 타원이 요소로 선다.
      return ellipseNative(
        length(attrs, 'cx'),
        length(attrs, 'cy'),
        cornerRadius(attrs, 'rx') ?? 0,
        cornerRadius(attrs, 'ry') ?? 0,
      );
    }
    case 'line':
      // **퇴화한 선도 여기서는 낸다.** 두 끝점이 같은 선을 걸러야 하는 것은 참이지만
      // (`isDegenerateLine` 이 저장 왕복에서 기하를 통째로 갈아 끼운다), 그 판정은
      // **캔버스 정수로 반올림한 뒤**라야 성립한다 — 사용자 단위로는 다른 두 점이 같은
      // 칸에 떨어질 수 있다. 그래서 그 게이트는 계획 층에 있고 여기에는 없다.
      return {
        kind: 'line',
        x1: length(attrs, 'x1'),
        y1: length(attrs, 'y1'),
        x2: length(attrs, 'x2'),
        y2: length(attrs, 'y2'),
      };
    default:
      return undefined;
  }
}

function ellipseNative(cx: number, cy: number, rx: number, ry: number): ImportedNative | undefined {
  if (!(rx > 0) || !(ry > 0)) return undefined;
  return { kind: 'ellipse', minX: cx - rx, minY: cy - ry, maxX: cx + rx, maxY: cy + ry };
}

/**
 * 태그 이름 하나로 갈라 부른다. 단축 요소가 아니면 `undefined` — "그릴 것이 없다"(빈
 * 배열)와 "내가 다룰 종류가 아니다" 를 값으로 가른다.
 */
export function shorthandShapeCommands(tag: string, attrs: AttrBag): PathCommand[] | undefined {
  switch (tag) {
    case 'rect':
      return rectCommands(attrs);
    case 'circle':
      return circleCommands(attrs);
    case 'ellipse':
      return ellipseCommands(attrs);
    case 'line':
      return lineCommands(attrs);
    case 'polyline':
      return polylineCommands(attrs);
    case 'polygon':
      return polygonCommands(attrs);
    default:
      return undefined;
  }
}
