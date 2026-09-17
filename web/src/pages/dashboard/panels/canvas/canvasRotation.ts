// 회전 각도의 자료형과 산술 (SPEC-CANVAS-014 M1).
//
// ## 각도는 **정수 도**이지 라디안이 아니다 (§결정 1)
//
// 라디안은 부동소수이고 이 저장소의 좌표는 전부 정수다. 각도를 부동소수로 두면 셋을 잃는다.
//
//   1. **90° 의 배수가 정확하지 않다.** `Math.PI / 2` 를 네 번 더해도 `2π` 가 아니며,
//      013 의 "네 번 돌리면 제자리"(K1)가 표현 불가능해진다.
//   2. **저장이 커지고 왕복이 흔들린다.** `0.5235987755982988` 은 한 요소에 20 바이트이고,
//      JSON 왕복에서 마지막 자리가 흔들린다.
//   3. **화면이 말할 수 없다.** 사용자에게 보일 수가 `30°` 가 아니라 `0.5236` 이 된다.
//
// 정수 도면 셋 다 없고, 라디안이 필요한 자리(`Math.cos`)는 **그리는 순간** 한 번 환산한다.
//
// ## 0 은 키를 만들지 않는다 (§결정 7)
//
// 014 이전에 저장된 대시보드가 **바이트 동일**하게 왕복해야 한다. 파서가 `rotation: 0` 을
// 만들어 채우면 왕복 한 번에 모든 요소가 키를 하나씩 얻는다 — 001 이 "미지정 필드를 만들어
// 채우지 않는다" 로 세운 규율 그대로다.
//
// ## 도는 방향
//
// 화면 좌표는 y 가 아래로 향하므로, **양수 각도는 시계 방향**이다. 013 의
// `transformLocalPoint` 가 `rotateCW` 에서 왼쪽 위를 오른쪽 위로 보내는 것과 같은 약속이다.
//
// **이 모듈은 DOM 도 투영도 모른다.** 산술뿐이다.
//
// @spec SPEC-CANVAS-014 REQ-01 · REQ-08

import type { CanvasBox } from './canvasGeometry';

/** 한 바퀴. 정규화의 법이자 013 의 `(θ+90)×4 ≡ θ` 가 성립하는 근거다. */
export const FULL_TURN_DEGREES = 360;

/** 회전 손잡이의 Shift 눈금(도). 격자 붙임과 **다른 축**이라 그 상수를 쓰지 않는다. */
export const ROTATION_SNAP_DEGREES = 15;

/**
 * 각도를 `[0, 360)` 정수로 죈다.
 *
 * `-90 → 270` · `450 → 90` · `360 → 0`. 손상된 값(비유한)은 `undefined` 이며, 그것이
 * "각도 없음" 이다 — 0 과 같은 그림이지만 **저장에 키가 생기지 않는다**.
 */
export function normalizeDegrees(value: unknown): number | undefined {
  if (typeof value !== 'number' || !Number.isFinite(value)) return undefined;
  const whole = Math.round(value);
  const wrapped = ((whole % FULL_TURN_DEGREES) + FULL_TURN_DEGREES) % FULL_TURN_DEGREES;
  return wrapped;
}

/**
 * 저장에 실을 각도 — **0 이면 `undefined`** 다(§결정 7).
 *
 * `normalizeDegrees` 와 갈라 둔 것에 뜻이 있다. 산술은 0 을 값으로 다뤄야 하고(각도를 더한
 * 결과가 0 일 수 있다), 저장은 0 을 부재로 다뤄야 한다. 한 함수가 둘을 겸하면 산술 쪽에서
 * `undefined` 를 만나 매번 `?? 0` 을 적게 된다.
 */
export function storableDegrees(value: unknown): number | undefined {
  const deg = normalizeDegrees(value);
  return deg === undefined || deg === 0 ? undefined : deg;
}

/** 이 각도가 실제로 그림을 돌리는가. 미지정과 0 은 **같은 그림**이다. */
export function isRotated(deg: number | undefined): boolean {
  return deg !== undefined && normalizeDegrees(deg) !== 0;
}

/** 라디안. 그리는 순간에만 쓴다 — 저장에도 비교에도 부동소수를 들이지 않는다. */
export function toRadians(deg: number): number {
  return (deg * Math.PI) / 180;
}

/**
 * 점 하나를 축 둘레로 돌린다. **양수 각도는 시계 방향**이다(y 가 아래인 화면 좌표).
 *
 * 되돌리는 길은 각도의 부호를 뒤집는 것뿐이므로 함수를 둘로 두지 않는다 — 두면 그중 하나가
 * 부호를 틀리는 날이 오고, 그 결함은 **대칭 도형과 90° 배수에서 통째로 지나간다.**
 */
export function rotatePoint(
  p: { readonly x: number; readonly y: number },
  pivot: { readonly x: number; readonly y: number },
  deg: number,
): { x: number; y: number } {
  const rad = toRadians(deg);
  const cos = Math.cos(rad);
  const sin = Math.sin(rad);
  const dx = p.x - pivot.x;
  const dy = p.y - pivot.y;
  return {
    x: pivot.x + dx * cos - dy * sin,
    y: pivot.y + dx * sin + dy * cos,
  };
}

/** 상자의 가운데. 축은 014 에서 **언제나** 여기다(중심점 이동은 015 의 몫이다). */
export function boxCenter(box: CanvasBox): { x: number; y: number } {
  return { x: box.x + box.w / 2, y: box.y + box.h / 2 };
}

/**
 * 돌아간 상자를 덮는 **축-나란 상자**(§결정 2).
 *
 * 네 모서리를 돌려 최소·최대를 잡는다. 각도가 0 이면 **받은 상자를 그대로** 돌려주는 것이
 * 요점이다(K3) — 014 이전의 모든 화면이 "OBB 와 AABB 가 같은 수" 위에 서 있고, 여기서
 * 부동소수 왕복을 한 번이라도 태우면 그 등식이 마지막 자리에서 깨진다.
 */
export function rotatedAabb(box: CanvasBox, deg: number | undefined): CanvasBox {
  if (!isRotated(deg)) return { ...box };
  const pivot = boxCenter(box);
  const corners = [
    { x: box.x, y: box.y },
    { x: box.x + box.w, y: box.y },
    { x: box.x + box.w, y: box.y + box.h },
    { x: box.x, y: box.y + box.h },
  ].map((c) => rotatePoint(c, pivot, deg as number));
  const xs = corners.map((c) => c.x);
  const ys = corners.map((c) => c.y);
  const x0 = Math.min(...xs);
  const y0 = Math.min(...ys);
  return { x: x0, y: y0, w: Math.max(...xs) - x0, h: Math.max(...ys) - y0 };
}
