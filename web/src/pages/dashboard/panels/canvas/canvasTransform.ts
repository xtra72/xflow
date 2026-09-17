// 고른 것을 뒤집고 네 번에 한 바퀴 돌리는 산술 (SPEC-CANVAS-013 M1 · M2).
//
// ## 거울은 둘이고 회전은 둘이다
//
// "상/하/좌/우 뒤집기" 를 글자대로 읽으면 넷이지만 **거울은 둘뿐이다** — 위로 뒤집는 것과
// 아래로 뒤집는 것은 같은 가로축 거울이다. 방향을 넷으로 가르면 눌러도 결과가 같은 단추가
// 둘 생긴다.
//
// ## 세 로컬 격자가 같은 값이라 거울 함수가 하나다
//
// 이 저장소에는 요소 상자 안의 로컬 정수 격자가 셋 있고 셋이 같은 값을 쓴다:
//
//     PATH_LOCAL_EXTENT   = 10000             (shapes/pathTypes)
//     GROUP_LOCAL_EXTENT  = PATH_LOCAL_EXTENT (group/groupTypes)
//     ANCHOR_LOCAL_EXTENT = PATH_LOCAL_EXTENT (connector/anchorTypes)
//
// 우연이 아니라 규율이다 — 각 파일이 "값을 베끼지 않고 파생시킨다, 따로 적으면 언젠가
// 한쪽만 바뀐다" 고 적어 두었다. 그래서 경로 명령도 그룹 부품도 임의 앵커도 **같은 함수**를
// 지난다(`transformLocalPoint`).
//
// ## 반올림이 하중을 받는다 (§결정 3)
//
// 90° 회전은 **중심을 고정**한다. 정수 격자에서 `w − h` 가 홀수면 새 좌상단이 반 단위에
// 놓이는데, 그 반 단위를 어느 쪽으로 미느냐가 누적 오차를 만들거나 만들지 않는다. 상자
// 열하나를 네 번씩 돌려 실측했다:
//
//     Math.round / floor / ceil  →  11 중 6 이 제자리로 돌아오지 못함
//     Math.trunc (0 쪽으로 버림)  →  11 중 0 실패
//
// 까닭은 한 줄이다 — **`trunc` 만 홀함수**다(`trunc(−v) = −trunc(v)`). 첫 회전의 오프셋이
// `trunc((w−h)/2)` 이면 회전으로 `w`·`h` 가 맞바뀌므로 다음 오프셋은 그 값의 **정확한
// 음수**가 되어 상쇄된다. `Math.round` 는 홀함수가 아니라(−0.5 → 0, +0.5 → 1) 한쪽으로
// 쌓인다. **그래서 이 파일에서 `Math.round` 를 쓰면 안 되고, 그 금지는 시험이 지킨다.**
//
// ## 자식에게는 반올림이 아예 없다
//
// 요소마다 반올림하면 열 개를 고른 선택이 열 번 어긋난다. 산술을 **선택 상자 로컬 정수
// 좌표**에서 하므로 나눗셈이 한 번도 나오지 않고, 반올림은 새 선택 상자 원점 **한 번**뿐이며
// 모든 요소가 그 하나를 함께 쓴다.
//
// **이 모듈은 DOM 도 투영도 모른다.** 캔버스 단위 정수 산술뿐이다.
//
// @spec SPEC-CANVAS-013 REQ-01 · REQ-02 · REQ-03

import type { CanvasBox } from './canvasGeometry';

/**
 * 변환 넷. 뒤집기 둘 · 회전 둘이며, 이 문자열이 적히는 자리는 이 파일과 그 소비자뿐이다.
 *
 * `flipX` 는 **가로 방향으로 뒤집는다**(세로축 거울) — 왼쪽 것이 오른쪽으로 간다.
 * `flipY` 는 그 반대 축이다.
 */
export type TransformKind = 'flipX' | 'flipY' | 'rotateCW' | 'rotateCCW';

/** 이 변환이 상자의 가로·세로를 맞바꾸는가 — 회전 둘만 참이다. */
export function swapsExtent(kind: TransformKind): boolean {
  return kind === 'rotateCW' || kind === 'rotateCCW';
}

/**
 * 로컬 격자(0..`extent`) 안의 점 하나를 변환한다.
 *
 * 넷 다 **나눗셈이 없다** — 정확하고 되돌릴 수 있다는 뜻이며, 그것이 K1(네 번이면 제자리)과
 * K2(같은 거울 두 번이면 제자리)의 근거다.
 *
 * 화면 좌표는 y 가 아래로 향하므로, `rotateCW` 에서 왼쪽 위 `(0, 0)` 은 **오른쪽 위**
 * `(extent, 0)` 으로 간다.
 */
export function transformLocalPoint(
  kind: TransformKind,
  p: { readonly x: number; readonly y: number },
  extent: number,
): { x: number; y: number } {
  switch (kind) {
    case 'flipX':
      return { x: extent - p.x, y: p.y };
    case 'flipY':
      return { x: p.x, y: extent - p.y };
    case 'rotateCW':
      return { x: extent - p.y, y: p.x };
    case 'rotateCCW':
      return { x: p.y, y: extent - p.x };
  }
}

/**
 * 변환 뒤의 선택 상자.
 *
 * 뒤집기는 상자를 바꾸지 않는다(거울은 자기 자신을 자기 자리에 비춘다). 회전은 가로·세로를
 * 맞바꾸고 **중심을 지킨다** — 새 원점의 오프셋이 이 파일의 유일한 반올림이다(§반올림).
 */
export function transformSelectionBox(kind: TransformKind, sel: CanvasBox): CanvasBox {
  if (!swapsExtent(kind)) return { ...sel };
  return {
    // `Math.trunc` 다. `Math.round` 로 바꾸면 네 번 돌린 그림이 제자리로 오지 않는다.
    x: sel.x + Math.trunc((sel.w - sel.h) / 2),
    y: sel.y + Math.trunc((sel.h - sel.w) / 2),
    w: sel.h,
    h: sel.w,
  };
}

/**
 * 상자 하나를 **선택 상자 안에서** 변환한다.
 *
 * 선택 상자 로컬 좌표로 옮겨 정수 산술만 쓰고, 마지막에 새 선택 상자의 원점을 더한다.
 * 점(선 끝점 · 문구 기준점)은 `w = h = 0` 인 상자로 이 함수를 지난다 — 갈래를 따로 두면
 * 두 산술이 갈라질 자리가 생긴다.
 */
export function transformBoxWithin(
  kind: TransformKind,
  box: CanvasBox,
  sel: CanvasBox,
): CanvasBox {
  const lx = box.x - sel.x;
  const ly = box.y - sel.y;
  const next = transformSelectionBox(kind, sel);

  switch (kind) {
    case 'flipX':
      return { x: next.x + (sel.w - lx - box.w), y: next.y + ly, w: box.w, h: box.h };
    case 'flipY':
      return { x: next.x + lx, y: next.y + (sel.h - ly - box.h), w: box.w, h: box.h };
    case 'rotateCW':
      return { x: next.x + (sel.h - ly - box.h), y: next.y + lx, w: box.h, h: box.w };
    case 'rotateCCW':
      return { x: next.x + ly, y: next.y + (sel.w - lx - box.w), w: box.h, h: box.w };
  }
}

/** 점 하나를 선택 상자 안에서 변환한다 — 넓이 0 인 상자로 위 함수를 지난다. */
export function transformPointWithin(
  kind: TransformKind,
  p: { readonly x: number; readonly y: number },
  sel: CanvasBox,
): { x: number; y: number } {
  const out = transformBoxWithin(kind, { x: p.x, y: p.y, w: 0, h: 0 }, sel);
  return { x: out.x, y: out.y };
}

/**
 * 상자 여럿을 덮는 가장 작은 상자. 비었으면 `undefined` 다 — 0 크기 상자를 지어내면
 * 그것이 곧 "아무것도 안 골랐는데 뒤집을 축이 있다" 가 된다.
 */
export function unionBox(boxes: readonly CanvasBox[]): CanvasBox | undefined {
  const first = boxes[0];
  if (first === undefined) return undefined;
  let x0 = first.x;
  let y0 = first.y;
  let x1 = first.x + first.w;
  let y1 = first.y + first.h;
  for (const b of boxes.slice(1)) {
    if (b.x < x0) x0 = b.x;
    if (b.y < y0) y0 = b.y;
    if (b.x + b.w > x1) x1 = b.x + b.w;
    if (b.y + b.h > y1) y1 = b.y + b.h;
  }
  return { x: x0, y: y0, w: x1 - x0, h: y1 - y0 };
}
