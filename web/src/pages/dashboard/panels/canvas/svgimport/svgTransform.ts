// 변환을 좌표에 녹인다 (SPEC-CANVAS-007 M3).
//
// **`PathCommand` 는 변환을 나르지 않으므로 좌표에 녹이는 것 말고 다른 길이 없다.** 이것은
// 편의가 아니라 어휘의 귀결이다 — 008 이 명령 넷을 정할 때 변환 칸을 두지 않았고, 007 은
// 그 결정을 무르지 않는다(불변식 K12).
//
// **아핀이 3차 베지어를 3차 베지어로 정확히 옮긴다**(제어점을 각각 옮기면 된다)는 사실이
// 이 모듈이 존재할 수 있는 이유의 전부다. 호에는 그 성질이 없다 — 그래서 호가 먼저
// 3차가 되어야 한다(불변식 K2 · `svgArc.ts`).
//
// **기각한 안 — 3×3 동차 행렬.** SVG 의 `transform` 은 아핀뿐이라 마지막 행이 언제나
// `[0 0 1]` 이고, 그 세 칸은 곱셈마다 확인되지 않는 채로 실려 다닌다. 2×3 이면 곱셈
// 여섯 줄이 전부다.
//
// **기각한 안 — 행렬 라이브러리를 들인다.** 신규 의존성 0 이 이 SPEC 의 값 가운데 하나이고,
// 필요한 것은 곱셈 하나 · 점 적용 하나 · 행렬식 하나다.
//
// **손상된 `transform` 을 만나면 거기서 멈추고 그때까지를 돌려준다.** 사양은 속성 전체를
// 오류로 보고 요소를 그리지 않게 하지만, 오타 하나로 도형을 통째로 잃는 쪽을 기각한다 —
// 007 의 실패 규율은 "값으로 돌아오고 화면이 말한다" 이지 "조용히 사라진다" 가 아니다.
//
// @spec SPEC-CANVAS-007 REQ-02 · REQ-07 · AC-03

import type { PathCommand } from '../shapes/pathTypes';

import { parseNumberList } from './svgPathData';

/**
 * 2×3 아핀 행렬.
 * ```
 * M = [ a c e ]      x' = a·x + c·y + e
 *     [ b d f ]      y' = b·x + d·y + f
 * ```
 */
export interface Matrix2x3 {
  readonly a: number;
  readonly b: number;
  readonly c: number;
  readonly d: number;
  readonly e: number;
  readonly f: number;
}

export const IDENTITY_MATRIX: Matrix2x3 = { a: 1, b: 0, c: 0, d: 1, e: 0, f: 0 };

const DEG_TO_RAD = Math.PI / 180;

/**
 * `outer · inner` — **`inner` 가 점에 먼저 적용된다.**
 *
 * 한 속성 안의 여러 함수는 왼쪽이 바깥이고(`M₁·M₂·…·Mₙ`), 조상 `<g>` 누적도 조상이
 * 바깥이다(`M_조상 · M_자신`). 두 규칙이 같은 곱셈이라 함수가 하나면 충분하다.
 */
export function multiplyMatrix(outer: Matrix2x3, inner: Matrix2x3): Matrix2x3 {
  return {
    a: outer.a * inner.a + outer.c * inner.b,
    b: outer.b * inner.a + outer.d * inner.b,
    c: outer.a * inner.c + outer.c * inner.d,
    d: outer.b * inner.c + outer.d * inner.d,
    e: outer.a * inner.e + outer.c * inner.f + outer.e,
    f: outer.b * inner.e + outer.d * inner.f + outer.f,
  };
}

export function applyMatrix(m: Matrix2x3, x: number, y: number): { x: number; y: number } {
  return { x: m.a * x + m.c * y + m.e, y: m.b * x + m.d * y + m.f };
}

export function determinant(m: Matrix2x3): number {
  return m.a * m.d - m.b * m.c;
}

/**
 * 선 두께에 곱할 스칼라 배율 — `√|det M|`.
 *
 * SVG 는 `scale(2,1)` 안의 선을 **이방적으로** 굵게 그리지만 캔버스의 `lineWidth` 는 스칼라
 * 하나다(실측: `ctx.lineWidth = resolveStrokeWidth(style.strokeWidth)`). 넓이 보존 배율이
 * 두 축의 기하 평균이라 이 값이 가장 덜 틀리는 하나이며, **비균등일 때는 근사로 보고한다**.
 */
export function strokeScaleOf(m: Matrix2x3): number {
  return Math.sqrt(Math.abs(determinant(m)));
}

/**
 * 이 행렬이 **축에 정렬된 상자를 축에 정렬된 상자로** 옮기는가.
 *
 * 캔버스의 `ElementStyle` 에는 축이 아홉 있고(`fill` · `stroke` · `strokeWidth` ·
 * `opacity` · `fontSize` · `fontWeight` · `textColor` · `align` · `visible`) **회전이
 * 없다.** `BoxGeometry` 도 네 수(`x`·`y`·`w`·`h`)뿐이다. 그래서 돌아간 `<rect>` 를 캔버스
 * 사각형으로 옮기면 그 회전은 **적을 자리가 없어 사라진다** — 문서가 말한 적 없는 그림이
 * 된다. 이 술어가 그 자리를 막는다.
 *
 * **정확한 조건:**
 * ```
 * (b === 0 && c === 0) || (a === 0 && d === 0)
 * ```
 * 단위 x 벡터는 `(a, b)` 로, 단위 y 벡터는 `(c, d)` 로 간다. 상자의 두 변이 옮겨간 뒤에도
 * 축에 나란하려면 **두 상(像)이 각각 축에 나란해야** 하고, 아핀이 가역이라면(퇴화는 이미
 * 걸러졌다) 둘이 같은 축에 겹칠 수 없으므로 갈래는 둘뿐이다:
 *   - `b = c = 0` — x 는 x 로, y 는 y 로. 배율·반사·평행이동의 합성.
 *   - `a = d = 0` — x 는 y 로, y 는 x 로. 90° 회전을 `matrix(0 1 -1 0 …)` 로 적은 문서다.
 * 기울임(`skewX`)은 `a ≠ 0` 이면서 `c ≠ 0` 이라 두 갈래 모두에서 떨어진다.
 *
 * **엄밀한 `=== 0` 이고 허용 오차를 두지 않는다.** `1e-9` 를 두면 `rotate(0.0001)` 이
 * 통과하고 그 회전이 조용히 지워진다 — 치환이 **정확하기 때문에** 사용자에게 물어볼
 * 필요가 없다는 것이 이 기능의 전제이므로, 그 전제를 오차만큼 무르면 기능의 근거가 함께
 * 무너진다. 대가는 `rotate(90)` 이 부동소수점 때문에(`cos 90° = 6.1e-17 ≠ 0`) 경로로
 * 남는다는 것이며, 경로는 언제나 옳으므로 그 대가는 **그림이 아니라 표현**에만 든다.
 */
export function preservesAxisAlignment(m: Matrix2x3): boolean {
  return (m.b === 0 && m.c === 0) || (m.a === 0 && m.d === 0);
}

/**
 * 이 행렬이 **닮음 변환**인가(회전·균등 배율·반사의 합성).
 *
 * 닮음이면 두 축의 배율이 같으므로 `√|det|` 가 **정확한 옮김**이고 보고할 것이 없다.
 * 아니면 근사이며 화면이 그 사실을 말해야 한다(`nonUniformStrokeScale`).
 *
 * 판정: 두 열 벡터의 길이가 같고 서로 직교한다.
 */
export function isSimilarity(m: Matrix2x3, epsilon = 1e-9): boolean {
  const columnA = m.a * m.a + m.b * m.b;
  const columnB = m.c * m.c + m.d * m.d;
  const dot = m.a * m.c + m.b * m.d;
  const scale = Math.max(columnA, columnB, 1);
  return Math.abs(columnA - columnB) <= epsilon * scale && Math.abs(dot) <= epsilon * scale;
}

// --- 함수별 행렬 --------------------------------------------------------

/** 인자 수가 맞지 않으면 `undefined` — 호출부가 거기서 멈춘다. */
function matrixFor(name: string, args: readonly number[]): Matrix2x3 | undefined {
  const n = args.length;
  const arg = (i: number): number => args[i] ?? 0;
  if (args.some((v) => !Number.isFinite(v))) return undefined;

  switch (name) {
    case 'translate': {
      if (n !== 1 && n !== 2) return undefined;
      return { a: 1, b: 0, c: 0, d: 1, e: arg(0), f: n === 2 ? arg(1) : 0 };
    }
    case 'scale': {
      if (n !== 1 && n !== 2) return undefined;
      const sx = arg(0);
      const sy = n === 2 ? arg(1) : sx;
      return { a: sx, b: 0, c: 0, d: sy, e: 0, f: 0 };
    }
    case 'rotate': {
      if (n !== 1 && n !== 3) return undefined;
      const angle = arg(0) * DEG_TO_RAD;
      const cos = Math.cos(angle);
      const sin = Math.sin(angle);
      const rotation: Matrix2x3 = { a: cos, b: sin, c: -sin, d: cos, e: 0, f: 0 };
      if (n === 1) return rotation;
      // `rotate(a, cx, cy)` = `T(cx,cy) · R(a) · T(−cx,−cy)`.
      const cx = arg(1);
      const cy = arg(2);
      return multiplyMatrix(
        multiplyMatrix({ a: 1, b: 0, c: 0, d: 1, e: cx, f: cy }, rotation),
        { a: 1, b: 0, c: 0, d: 1, e: -cx, f: -cy },
      );
    }
    case 'skewX': {
      if (n !== 1) return undefined;
      return { a: 1, b: 0, c: Math.tan(arg(0) * DEG_TO_RAD), d: 1, e: 0, f: 0 };
    }
    case 'skewY': {
      if (n !== 1) return undefined;
      return { a: 1, b: Math.tan(arg(0) * DEG_TO_RAD), c: 0, d: 1, e: 0, f: 0 };
    }
    case 'matrix': {
      if (n !== 6) return undefined;
      return { a: arg(0), b: arg(1), c: arg(2), d: arg(3), e: arg(4), f: arg(5) };
    }
    default:
      return undefined;
  }
}

function isIdentifierChar(ch: string | undefined): boolean {
  return ch !== undefined && ((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z'));
}

function isSeparator(ch: string | undefined): boolean {
  return ch === ' ' || ch === '\t' || ch === '\r' || ch === '\n' || ch === '\f' || ch === ',';
}

/**
 * `transform` 속성 하나를 행렬 하나로.
 *
 * **왼쪽이 바깥이다** — `transform="translate(7,11) rotate(30)"` 은 점에 회전을 먼저
 * 적용한 뒤 옮긴다. 이 순서는 `translate` 만 있는 고정 입력에서는 **관측되지 않는다**
 * (평행이동끼리는 교환법칙이 성립한다). 시험이 세 종류를 섞는 이유다(E1).
 *
 * 각도는 **도(degree)** 다. 라디안으로 읽으면 30° 짜리 회전이 1719° 가 된다.
 */
export function parseTransformList(raw: string | undefined): Matrix2x3 {
  if (raw === undefined) return IDENTITY_MATRIX;
  let result = IDENTITY_MATRIX;
  let i = 0;
  while (i < raw.length) {
    while (isSeparator(raw[i])) i += 1;
    if (i >= raw.length) break;
    const nameStart = i;
    while (isIdentifierChar(raw[i])) i += 1;
    if (i === nameStart) break; // 이름이 아닌 쓰레기 — 여기서 멈춘다.
    const name = raw.slice(nameStart, i);
    while (isSeparator(raw[i])) i += 1;
    if (raw[i] !== '(') break;
    const close = raw.indexOf(')', i);
    if (close === -1) break; // 닫히지 않은 괄호 — `transform="rotate("`.
    const matrix = matrixFor(name, parseNumberList(raw.slice(i + 1, close)));
    if (matrix === undefined) break; // 모르는 함수 또는 인자 수가 맞지 않는다.
    result = multiplyMatrix(result, matrix);
    i = close + 1;
  }
  return result;
}

// --- 좌표에 녹이기 -----------------------------------------------------

/**
 * 명령 목록 전체에 행렬을 적용한다. **`Z` 는 좌표가 없으므로 그대로 지난다.**
 *
 * 이 함수가 받는 것은 이미 `M`·`L`·`C`·`Z` 넷뿐이다 — 호 변수(`rx`·`ry`·`φ`)가 살아 있는
 * 자료 구조는 이 층에 도달하지 않는다(불변식 K2 의 형상 판정).
 */
export function transformCommands(
  commands: readonly PathCommand[],
  m: Matrix2x3,
): PathCommand[] {
  return commands.map((cmd) => {
    switch (cmd.c) {
      case 'Z':
        return { c: 'Z' };
      case 'M':
      case 'L': {
        const p = applyMatrix(m, cmd.x, cmd.y);
        return { c: cmd.c, x: p.x, y: p.y };
      }
      case 'C': {
        const c1 = applyMatrix(m, cmd.x1, cmd.y1);
        const c2 = applyMatrix(m, cmd.x2, cmd.y2);
        const p = applyMatrix(m, cmd.x, cmd.y);
        return { c: 'C', x1: c1.x, y1: c1.y, x2: c2.x, y2: c2.y, x: p.x, y: p.y };
      }
    }
  });
}
