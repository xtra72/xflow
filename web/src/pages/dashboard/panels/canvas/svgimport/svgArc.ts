// 타원 호를 3차 베지어로 (SPEC-CANVAS-007 M2).
//
// **이 모듈은 DOM 도 변환 행렬도 모른다.** 인자는 숫자 아홉 개뿐이고 그것이 불변식 K2 를
// **형상으로** 지는 방식이다: 변환 행렬을 받는 함수가 여기 없으므로, 호를 변환한 뒤에
// 축약하는 순서는 애초에 적힐 수 없다.
//
// **왜 순서가 correctness 인가.** 아핀 변환은 3차 베지어를 3차 베지어로 **정확히** 옮기지만,
// SVG 의 호는 아핀 아래에서 같은 `rx`/`ry`/`φ` 의 호가 **아니다**. 기울어진 `<g>` 안의
// 호를 나중에 변환하면 그림이 조용히 틀어진다 — 화면에서만 드러나고 어느 시험도 울리지
// 않는 부류이므로(위험 R3) 모듈 경계로 못박는다.
//
// **008 의 `k ≈ 0.5523` 을 여기 쓰지 않는다.** 그 상수는 4분원 전용이고, 회전 `φ` 와 두
// 플래그를 가진 일반 타원 호는 다른 문제다. 007 이 쓰는 것은 조각각 `δ` 에 대한 일반식
// `k = (4/3)·tan(δ/4)` 이며, `δ = 90°` 일 때 `(4/3)·tan(22.5°) = 0.552285…` 로 **008 의
// 상수와 같은 수가 나온다** — 008 의 상수는 이 식의 특수해다. 두 상수가 아니라 한 식이다.
//
// **받아들이는 오차와 그것이 화면에서 어디쯤인지**(가정 A3 · 불변식 K5):
// ```
// 최악: rx = ry = PATH_LOCAL_EXTENT/2 = 5000 로컬 단위
//   → 반경 오차 ≤ 0.00027 × 5000        = 1.35 로컬 단위
// 로컬 격자 양자화 오차                  = 0.5  로컬 단위
// 합                                     ≈ 1.85 로컬 단위
// 요소 상자 한 변 160 캔버스 단위에서:   1.85 / 10000 × 160 = 0.0296 캔버스 단위
//   → 캔버스 자체의 양자(1 단위)보다      약 34배 작다
// 스테이지 축척 0.5 에서:                 0.0148 px
//   → FLATTEN_TOLERANCE_PX(0.5) 보다      약 34배 작다
// ```
// 이 관계가 뒤집히면 그것은 결함이 아니라 **상수의 잘못**이다 — 그래서 시험은 두 수를 따로
// 세지 않고 **관계**를 단언한다(008 이 `FLATTEN_TOLERANCE_PX` 와 `HIT_TOLERANCE_PX` 에
// 대해 한 그 규율).
//
// **기각한 안 — 조각각을 45° 로 더 잘게 자른다.** 오차는 대략 `δ⁶` 로 줄어 1/64 이 되지만
// **명령 수는 두 배**가 되고 그 수는 `MAX_PATH_COMMANDS` 라는 죔쇠를 직접 먹는다. 위 산술이
// 이미 34배 여유를 보이므로 그 두 배를 살 이유가 없다.
//
// @spec SPEC-CANVAS-007 REQ-01 · AC-02 · AC-04

import type { PathCommand } from '../shapes/pathTypes';

import { ARC_SEGMENT_MAX_RAD } from './svgImportTypes';

const DEG_TO_RAD = Math.PI / 180;

/**
 * 두 벡터 사이의 **부호 있는** 각(라디안). 부호는 외적이 정한다.
 *
 * 코사인을 `[-1, 1]` 로 죄는 것이 이 함수에서 가장 잘 빠지는 자리다 — 반올림 잔차로
 * `1.0000000000000002` 가 나오면 `acos` 가 `NaN` 을 낸다.
 */
function angleBetween(ux: number, uy: number, vx: number, vy: number): number {
  const lengths = Math.hypot(ux, uy) * Math.hypot(vx, vy);
  if (lengths === 0) return 0;
  const cos = Math.min(1, Math.max(-1, (ux * vx + uy * vy) / lengths));
  const angle = Math.acos(cos);
  return ux * vy - uy * vx < 0 ? -angle : angle;
}

/**
 * 끝점→중심 매개변수화(SVG 1.1 부록 F.6.5)의 산출.
 *
 * 이 값을 **밖으로 내보내지 않는다** — 호 변수(`rx`·`ry`·`φ`)가 변환 적용 뒤에도 살아
 * 있는 자료 구조가 생기는 순간 불변식 K2 의 깨짐 신호가 켜진다. 시험만 이것을 본다.
 */
interface ArcCenter {
  readonly cx: number;
  readonly cy: number;
  /** 반지름 보정(`√Λ`) 뒤의 값. 보정이 없었으면 입력과 같다. */
  readonly rx: number;
  readonly ry: number;
  readonly theta1: number;
  readonly deltaTheta: number;
}

/**
 * 끝점 표현을 중심 표현으로. 호가 없으면 `undefined`.
 *
 * **4단계의 `√Λ` 확대와 5단계의 `max(0, …)` 가 이 식에서 가장 잘 빠지는 둘이다.**
 * 앞의 것이 빠지면 반지름이 모자란 호에서 `NaN` 이 나오고, 뒤의 것이 빠지면 반올림
 * 잔차로 음수 제곱근이 나온다. **둘 다 실제 파일에서 흔하다** — 도구가 좌표를 반올림해
 * 내보내면 원래 딱 맞던 반지름이 모자라진다.
 */
function endpointToCenter(
  x0: number,
  y0: number,
  rxIn: number,
  ryIn: number,
  phi: number,
  largeArc: 0 | 1,
  sweep: 0 | 1,
  x1: number,
  y1: number,
): ArcCenter | undefined {
  const cosPhi = Math.cos(phi);
  const sinPhi = Math.sin(phi);

  // 3) 두 끝점의 중점을 원점으로 옮기고 회전을 되돌린다.
  const dx = (x0 - x1) / 2;
  const dy = (y0 - y1) / 2;
  const x1p = cosPhi * dx + sinPhi * dy;
  const y1p = -sinPhi * dx + cosPhi * dy;

  // 4) 반지름이 모자라면 `√Λ` 배로 키운다. 사양이 정한 보정이다.
  let rx = Math.abs(rxIn);
  let ry = Math.abs(ryIn);
  const lambda = (x1p * x1p) / (rx * rx) + (y1p * y1p) / (ry * ry);
  if (lambda > 1) {
    const scale = Math.sqrt(lambda);
    rx *= scale;
    ry *= scale;
  }

  // 5) 중심(회전 되돌린 공간). `max(0, …)` 가 음수 제곱근을 막는다.
  const rx2 = rx * rx;
  const ry2 = ry * ry;
  const numerator = rx2 * ry2 - rx2 * y1p * y1p - ry2 * x1p * x1p;
  const denominator = rx2 * y1p * y1p + ry2 * x1p * x1p;
  if (denominator === 0) return undefined;
  const sign = largeArc !== sweep ? 1 : -1;
  const coefficient = sign * Math.sqrt(Math.max(0, numerator / denominator));
  const cxp = (coefficient * (rx * y1p)) / ry;
  const cyp = (coefficient * -(ry * x1p)) / rx;

  // 6) 회전과 평행이동을 되돌린다.
  const cx = cosPhi * cxp - sinPhi * cyp + (x0 + x1) / 2;
  const cy = sinPhi * cxp + cosPhi * cyp + (y0 + y1) / 2;

  // 7) 시작각과 쓸어 가는 각. **두 플래그가 여기서만 뜻을 갖는다** — `largeArc` 는 위
  //    부호에서, `sweep` 은 아래 방향 보정에서.
  const ux = (x1p - cxp) / rx;
  const uy = (y1p - cyp) / ry;
  const vx = (-x1p - cxp) / rx;
  const vy = (-y1p - cyp) / ry;
  const theta1 = angleBetween(1, 0, ux, uy);
  let deltaTheta = angleBetween(ux, uy, vx, vy);
  if (sweep === 0 && deltaTheta > 0) deltaTheta -= 2 * Math.PI;
  if (sweep === 1 && deltaTheta < 0) deltaTheta += 2 * Math.PI;

  // **이 폴백이 호를 현으로 납작하게 만든다.** 호출부가 `undefined` 를 `L` 하나로 옮기므로
  // 결과는 유한하고 끝점도 맞는다 — 그래서 위 두 수치 가드(`√Λ` · `max(0, …)` · `acos` 죔)를
  // 재는 시험은 "NaN 이 없다" 가 아니라 **"곡선으로 남아 있다"** 를 재야 한다.
  if (!Number.isFinite(cx) || !Number.isFinite(cy) || !Number.isFinite(deltaTheta)) {
    return undefined;
  }
  return { cx, cy, rx, ry, theta1, deltaTheta };
}

/**
 * 타원 호 하나를 3차 베지어 여럿으로.
 *
 * 사양이 정한 경계 셋을 **코드가 먼저 갖는다**(REQ-01):
 *
 * - **두 끝점이 같으면 그 명령을 통째로 건너뛴다** — 빈 목록을 낸다. 호가 없다.
 * - **`rx` 또는 `ry` 가 0 이면 직선**(`L`)으로 읽는다.
 * - **반지름이 모자라면 `√Λ` 배로 키운다**(위 `endpointToCenter`).
 *
 * **0 반지름 갈래가 산출로는 남는 것을 감추지 않는다.** 그 이른 반환을 통째로 지워도
 * 결과는 같다 — 0 반지름은 아래에서 `lambda = ∞` → `NaN` 전파 → 퇴화 중심 폴백을 타
 * 같은 `L` 하나를 낸다(뮤테이션이 물지 않는다). 그럼에도 남긴 것은, 지우면 사양이 정한
 * 규칙이 **`NaN` 산술이 특정 반환에 닿는다는 우연**에 얹히기 때문이다. 값을 바꾸는
 * 뮤테이션(`[]` 로 바꾸기)은 문다.
 *
 * 비유한 인자는 빈 목록이다 — 호출부가 이미 걸러 내지만(토크나이저), 이 함수는 제
 * 견고성을 스스로도 진다(REQ-07: 예외를 밖으로 던지지 않는다).
 *
 * **끝점을 `(x1, y1)` 로 스냅하지 않는다.** 스냅은 매개변수화가 틀렸을 때 그 사실을
 * 1e-15 짜리 잡음으로 덮어 준다. 대신 시험이 계산된 끝점과 주어진 끝점의 일치를 단언한다.
 * 이어지는 명령의 현재 점은 어차피 **주어진** 끝점이므로(`svgPathData` 의 축약) 오차가
 * 누적되지 않는다.
 */
export function arcToCubics(
  x0: number,
  y0: number,
  rxIn: number,
  ryIn: number,
  xAxisRotationDeg: number,
  largeArc: 0 | 1,
  sweep: 0 | 1,
  x1: number,
  y1: number,
): PathCommand[] {
  const inputs = [x0, y0, rxIn, ryIn, xAxisRotationDeg, x1, y1];
  if (inputs.some((v) => !Number.isFinite(v))) return [];
  if (x0 === x1 && y0 === y1) return [];
  if (rxIn === 0 || ryIn === 0) return [{ c: 'L', x: x1, y: y1 }];

  const phi = xAxisRotationDeg * DEG_TO_RAD;
  const center = endpointToCenter(x0, y0, rxIn, ryIn, phi, largeArc, sweep, x1, y1);
  if (center === undefined) return [{ c: 'L', x: x1, y: y1 }];

  const { cx, cy, rx, ry, theta1, deltaTheta } = center;
  const cosPhi = Math.cos(phi);
  const sinPhi = Math.sin(phi);

  // 8) 조각각을 상한 이하로 잘라 조각마다 3차 하나.
  const segments = Math.max(1, Math.ceil(Math.abs(deltaTheta) / ARC_SEGMENT_MAX_RAD));
  const delta = deltaTheta / segments;
  const k = (4 / 3) * Math.tan(delta / 4);

  /** 매개변수 `θ` 에서의 점. */
  const pointAt = (theta: number): { x: number; y: number } => {
    const cosT = Math.cos(theta);
    const sinT = Math.sin(theta);
    return {
      x: cx + rx * cosPhi * cosT - ry * sinPhi * sinT,
      y: cy + rx * sinPhi * cosT + ry * cosPhi * sinT,
    };
  };
  /** 매개변수 `θ` 에서의 접선(도함수). 제어점이 이 방향으로 `k` 만큼 나간다. */
  const tangentAt = (theta: number): { x: number; y: number } => {
    const cosT = Math.cos(theta);
    const sinT = Math.sin(theta);
    return {
      x: -rx * cosPhi * sinT - ry * sinPhi * cosT,
      y: -rx * sinPhi * sinT + ry * cosPhi * cosT,
    };
  };

  const out: PathCommand[] = [];
  for (let i = 0; i < segments; i += 1) {
    const thetaA = theta1 + i * delta;
    const thetaB = thetaA + delta;
    const pA = pointAt(thetaA);
    const pB = pointAt(thetaB);
    const tA = tangentAt(thetaA);
    const tB = tangentAt(thetaB);
    out.push({
      c: 'C',
      x1: pA.x + k * tA.x,
      y1: pA.y + k * tA.y,
      x2: pB.x - k * tB.x,
      y2: pB.y - k * tB.y,
      x: pB.x,
      y: pB.y,
    });
  }
  return out;
}
