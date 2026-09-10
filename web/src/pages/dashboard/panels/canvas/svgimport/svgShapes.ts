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

/**
 * `<rect x y width height rx ry>`.
 *
 * 실제 파일이 자주 어기는 자리 셋을 코드가 먼저 갖는다.
 * - `rx` 만 있으면 `ry = rx`, `ry` 만 있으면 `rx = ry`, 둘 다 없으면 모서리 없음.
 * - `rx > w/2` 는 `w/2` 로, `ry > h/2` 는 `h/2` 로 **죈다**. 죄지 않으면 모서리 호가
 *   상자 밖으로 나가 도형이 뒤집힌다.
 * - 음수·비유한은 없는 것으로 읽는다.
 */
export function rectCommands(attrs: AttrBag): PathCommand[] {
  const x = length(attrs, 'x');
  const y = length(attrs, 'y');
  const w = length(attrs, 'width');
  const h = length(attrs, 'height');
  if (!(w > 0) || !(h > 0)) return [];

  const rawRx = cornerRadius(attrs, 'rx');
  const rawRy = cornerRadius(attrs, 'ry');
  const rx = Math.min(rawRx ?? rawRy ?? 0, w / 2);
  const ry = Math.min(rawRy ?? rawRx ?? 0, h / 2);

  if (!(rx > 0) || !(ry > 0)) {
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
