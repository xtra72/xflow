// 호 축약 시험 (SPEC-CANVAS-007 M2 · AC-02).
//
// **E6 이 이 파일의 고정 입력을 정한다.** 플래그가 둘 다 0 인 호 하나로는 플래그 결함
// 넷(순서 맞바꿈 · `fa` 무시 · `fs` 무시 · 서로 바꿔 씀)을 **하나도 잡지 못한다** —
// `(0,0)` 은 어느 결함 아래서도 `(0,0)` 이기 때문이다.
//
// **그리고 "넷이 쌍마다 다르다" 만으로도 잡지 못한다.** 두 플래그를 맞바꿔 읽으면 네 결과의
// **집합은 그대로이고 짝만 바뀌므로** 쌍마다 다름은 여전히 참이다. 그래서 아래 시험은
// 플래그의 **뜻**에 묶인 단언을 함께 든다.
//
//   (b) `fs` 는 **방향**이다 — `fa` 를 고정하고 `fs` 만 뒤집으면 부호 있는 넓이의 부호가
//       뒤집힌다. 맞바꿔 읽으면 `(0,0)` 과 `(0,1)` 이 `(0,0)` 과 `(1,0)` 이 되어 **둘 다
//       `fs=0`** 이 되고, 부호가 같아져 빨개진다.
//   (c) `fa` 는 **크기**다 — `fs` 를 고정하고 `fa` 를 켜면 현 중점에서 가장 먼 점까지의
//       거리가 커진다. 같은 이유로 맞바꿔 읽으면 빨개진다.
//
// **고정 입력이 "그럴듯하게 다른" 호가 되도록 골랐다.** `M 0 0 A 50 30 20 {fa} {fs} 60 20`
// 의 Λ 는 약 0.40 이라 네 조합 어느 것도 반지름 보정에 걸리지 않고, 넷 모두 온전한 호가
// 된다 — 플래그를 틀리게 읽어도 **눈에 띄게 망가진 그림이 아니라 다른 그림**이 나온다.
// 그것이 이 결함이 조용한 이유이며, 시험이 뜻으로 재야 하는 이유다.
//
// **확인한 뮤테이션(E12)** — 아래 각 시험 이름에 번호를 적었다.
//   1. `largeArc`/`sweep` 을 읽는 순서를 맞바꾸면
//      → (b) 부호 반전과 (c) 현 중점 거리가 빨개진다. **(a) 쌍마다 다름은 빨개지지 않는다** —
//        그 사실이 이 파일이 (b)(c) 를 드는 이유다.
//   2. `√Λ` 확대를 지우면 → "반지름이 모자란 호가 끝점에 닿는다" 가 빨개진다.
//   3. `max(0, …)` 를 지우면 → "반올림 잔차가 음수 제곱근을 만드는 호" 가 빨개진다.
//   4. 조각각 상한을 `2π` 로 키우면 → "오차가 0.027% 이하다" 가 빨개진다.
//   5. `acos` 인자 죔(`min/max`)을 지우면 → "cos 를 −1 밖으로 미는 호" 가 빨개진다.
//   6. `sweep` 방향 보정을 지우면 → (a) 쌍마다 다름과 (c) 가 빨개진다.
//   7. `largeArc` 부호를 항상 `+` 로 두면 → (a) 와 (c) 가 빨개진다.
//   8. 같은 끝점 건너뛰기를 지우면 → "두 끝점이 같으면 명령이 사라진다" 가 빨개진다.
//   9. `rx`/`ry` 가 0 인 갈래를 `[]` 로 바꾸면 → "직선 하나다" 가 빨개진다.
//
// **물지 않아 강화한 가드 셋(E12)** — 처음 쓴 고정 입력 셋이 전부 **대칭**이었다.
//   · 뮤테이션 2 는 "NaN 이 없다" 로만 재면 물지 않는다. 확대를 지워도 `max(0, …)` 가
//     음수를 0 으로 받아 주어 **유한하지만 끝점에 닿지 못하는** 호가 나오기 때문이다.
//     → 끝점 단언(`expectEndsAt`)을 더해 물게 했다.
//   · 뮤테이션 3·5 는 수평 대칭 입력(`A 50 50 0 0 1 100 0` · `A 10 10 0 0 1 200 0`)에서
//     분자와 코사인이 **정확히** 0 과 ±1 이 되어 잔차가 남지 않는다. → 두 축이 함께
//     움직이는 입력(`-120,-120` · `-120,-92`)으로 갈았다. 그래도 물지 않았다 —
//     `NaN` 이 퇴화 중심 폴백을 타 **직선 하나**가 되는데, 그 직선은 유한하고 끝점도
//     맞기 때문이다. → `expectIntactArc` 가 "곡선으로 남아 있다" 를 함께 재게 해 물렸다.
//     **호가 조용히 현으로 납작해지는 것**이 이 층의 가장 나쁜 실패 형상이며, 그것을
//     보는 자는 명령 종류뿐이다.
//
// **물지 않으나 남긴 갈래 하나(보고함)** — `rx`/`ry` 가 0 인 이른 반환을 **통째로 지워도**
// 어느 시험도 빨개지지 않는다: 0 반지름은 아래에서 `lambda = ∞` → `NaN` 전파 → 퇴화 중심
// 폴백을 타 **같은 `L` 하나**를 낸다. 그럼에도 남긴 것은, 지우면 사양이 정한 규칙(REQ-01
// "`rx` 또는 `ry` 가 0 이면 직선")이 **NaN 산술이 특정 반환에 닿는다는 우연**에 얹히기
// 때문이다. 값을 바꾸는 뮤테이션(9)은 문다.

import { describe, expect, it } from 'vitest';

import type { PathCommand } from '../shapes/pathTypes';

import { arcToCubics } from './svgArc';
import { ARC_SEGMENT_MAX_RAD } from './svgImportTypes';
import { parseSvgPathData } from './svgPathData';

interface Pt {
  x: number;
  y: number;
}

/** SPEC 이 적은 오차 한계 — `0.027% × max(rx, ry)`. */
const SPEC_RELATIVE_RADIAL_LIMIT = 0.00027;

/**
 * **실측한 참 최악값** — 조각각이 정확히 90° 인 원호에서 `2.72529e-4`(= 0.0272529%).
 *
 * SPEC 이 적은 `0.027%` 는 이 수를 **내림해 적은 표기**이며, 참값은 그 한계를 1.0094 배
 * 넘는다. 감추지 않고 수로 적는다: 한계를 문자 그대로 읽으면 90° 조각의 원호가 그것을
 * 어긴다. 화면에서의 뜻은 아래 "관계로 단언한다" 시험이 잰다 — 두 자릿수 여유 안이라
 * 이 0.94% 초과는 관측되지 않는다.
 */
const MEASURED_WORST_RELATIVE_RADIAL_ERROR = 2.72529e-4;

/**
 * 3차 베지어 목록을 조각마다 `steps` 등분해 촘촘히 뽑는다.
 *
 * **제품의 평탄화기(`pathFlatten`)를 쓰지 않는다.** 그 함수는 허용 오차에 닿으면 멈추므로
 * 여기서 재려는 바로 그 오차를 스스로 가린다. 균등 표본이 더 엄한 자다.
 */
function samplePath(cmds: readonly PathCommand[], start: Pt, steps = 64): Pt[] {
  const out: Pt[] = [start];
  let current = start;
  for (const cmd of cmds) {
    if (cmd.c === 'L') {
      out.push({ x: cmd.x, y: cmd.y });
      current = { x: cmd.x, y: cmd.y };
      continue;
    }
    if (cmd.c !== 'C') continue;
    const p0 = current;
    for (let i = 1; i <= steps; i += 1) {
      const t = i / steps;
      const u = 1 - t;
      out.push({
        x: u * u * u * p0.x + 3 * u * u * t * cmd.x1 + 3 * u * t * t * cmd.x2 + t * t * t * cmd.x,
        y: u * u * u * p0.y + 3 * u * u * t * cmd.y1 + 3 * u * t * t * cmd.y2 + t * t * t * cmd.y,
      });
    }
    current = { x: cmd.x, y: cmd.y };
  }
  return out;
}

/**
 * 세 점의 외심. **제품 코드와 독립인 자다** — 호 산술이 구한 중심을 다시 쓰면 그 중심이
 * 틀렸을 때 오차가 0 으로 보인다.
 */
function circumcenter(a: Pt, b: Pt, c: Pt): Pt {
  const d = 2 * (a.x * (b.y - c.y) + b.x * (c.y - a.y) + c.x * (a.y - b.y));
  const aa = a.x * a.x + a.y * a.y;
  const bb = b.x * b.x + b.y * b.y;
  const cc = c.x * c.x + c.y * c.y;
  return {
    x: (aa * (b.y - c.y) + bb * (c.y - a.y) + cc * (a.y - b.y)) / d,
    y: (aa * (c.x - b.x) + bb * (a.x - c.x) + cc * (b.x - a.x)) / d,
  };
}

/**
 * 근사 곡선의 **상대 반경 오차** — 회전과 축 배율을 되돌려 단위원 공간으로 옮긴 뒤,
 * 독립으로 구한 외심에서의 거리가 1 에서 얼마나 벗어나는가.
 *
 * 이 값 `e` 는 곧 사용자 단위 반경 오차 `e × max(rx, ry)` 의 상한이다.
 */
function maxRelativeRadialError(points: readonly Pt[], rx: number, ry: number, phiDeg: number): number {
  const phi = (phiDeg * Math.PI) / 180;
  const cos = Math.cos(phi);
  const sin = Math.sin(phi);
  const mapped = points.map((p) => ({
    x: (cos * p.x + sin * p.y) / rx,
    y: (-sin * p.x + cos * p.y) / ry,
  }));
  const first = mapped[0];
  const mid = mapped[Math.floor(mapped.length / 2)];
  const last = mapped[mapped.length - 1];
  if (first === undefined || mid === undefined || last === undefined) throw new Error('no samples');
  const centre = circumcenter(first, mid, last);
  let worst = 0;
  for (const p of mapped) {
    worst = Math.max(worst, Math.abs(Math.hypot(p.x - centre.x, p.y - centre.y) - 1));
  }
  return worst;
}

/** 닫은 다각형의 부호 있는 넓이(신발끈). 부호가 곧 감김 방향이다. */
function signedArea(points: readonly Pt[]): number {
  let sum = 0;
  for (let i = 0; i < points.length; i += 1) {
    const a = points[i];
    const b = points[(i + 1) % points.length];
    if (a === undefined || b === undefined) continue;
    sum += a.x * b.y - b.x * a.y;
  }
  return sum / 2;
}

/** 현 중점에서 폴리라인 위 가장 먼 점까지의 거리 — 호가 얼마나 부풀었는가. */
function bulge(points: readonly Pt[], p0: Pt, p1: Pt): number {
  const mx = (p0.x + p1.x) / 2;
  const my = (p0.y + p1.y) / 2;
  return points.reduce((worst, p) => Math.max(worst, Math.hypot(p.x - mx, p.y - my)), 0);
}

/** 알려진 중심에 대해 점들이 쓸어 간 각의 총합(절댓값). */
function sweptAngleAbout(centre: Pt, points: readonly Pt[]): number {
  let total = 0;
  for (let i = 1; i < points.length; i += 1) {
    const a = points[i - 1];
    const b = points[i];
    if (a === undefined || b === undefined) continue;
    const ax = a.x - centre.x;
    const ay = a.y - centre.y;
    const bx = b.x - centre.x;
    const by = b.y - centre.y;
    total += Math.abs(Math.atan2(ax * by - ay * bx, ax * bx + ay * by));
  }
  return total;
}

/**
 * 명령 목록이 **온전한 호**다 — 비유한 수가 없고, 곡선으로 남아 있다.
 *
 * "곡선으로 남아 있다" 를 함께 재는 것이 요점이다. `endpointToCenter` 는 계산이 `NaN` 으로
 * 무너지면 **퇴화 중심 폴백**을 타 직선 하나(`L`)를 낸다 — 유한하고 끝점도 맞으므로
 * 앞의 두 단언만으로는 **호가 현으로 납작해진 것을 보지 못한다**. 수치 가드를 재는 시험은
 * 반드시 이 자를 써야 한다.
 */
function expectIntactArc(cmds: readonly PathCommand[]): void {
  expect(cmds.length).toBeGreaterThan(0);
  expect(cmds.map((cmd) => cmd.c)).toEqual(cmds.map(() => 'C'));
  for (const cmd of cmds) {
    for (const value of Object.values(cmd)) {
      if (typeof value === 'number') expect(Number.isFinite(value)).toBe(true);
    }
  }
}

/**
 * 마지막 명령의 끝점이 주어진 끝점과 같다 — 매개변수화 전체를 재는 단 하나의 단언이다.
 *
 * `digits` 를 인자로 둔 것은 **반지름 보정이 큰 호에서 자릿수가 실제로 줄기 때문**이다.
 * `Λ = 288`(반지름이 17배로 늘어나는 자리)에서는 재구성 오차가 `1.3e-6` 사용자 단위까지
 * 커진다 — 좌표 120 에 대해 상대 `1e-8` 이고, 로컬 격자(0..10000) 정수 반올림 앞에서는
 * 보이지 않는다. 그 사실을 숨기지 않고 인자로 적는다.
 */
function expectEndsAt(cmds: readonly PathCommand[], expected: Pt, digits = 9): void {
  const last = cmds[cmds.length - 1];
  if (last === undefined || last.c === 'Z') throw new Error('expected a drawing command');
  expect(last.x).toBeCloseTo(expected.x, digits);
  expect(last.y).toBeCloseTo(expected.y, digits);
}

// --- 고정 입력 ----------------------------------------------------------

const P0: Pt = { x: 0, y: 0 };
const P1: Pt = { x: 60, y: 20 };
const RX = 50;
const RY = 30;
const ROT = 20;

const FLAG_COMBOS: readonly (readonly [0 | 1, 0 | 1])[] = [
  [0, 0],
  [0, 1],
  [1, 0],
  [1, 1],
];

function arcFor(fa: 0 | 1, fs: 0 | 1): PathCommand[] {
  return arcToCubics(P0.x, P0.y, RX, RY, ROT, fa, fs, P1.x, P1.y);
}

function polylineFor(fa: 0 | 1, fs: 0 | 1, steps?: number): Pt[] {
  return samplePath(arcFor(fa, fs), P0, steps);
}

describe('플래그 넷이 살아 있다 [E6]', () => {
  it('(a) 네 조합의 폴리라인이 쌍마다 다르다 — 여섯 쌍 전부', () => {
    const polylines = FLAG_COMBOS.map(([fa, fs]) => JSON.stringify(polylineFor(fa, fs)));
    expect(new Set(polylines).size).toBe(4);
  });

  it('(b) fs 는 방향이다 — 부호 있는 넓이의 부호가 뒤집힌다 (뮤테이션 1)', () => {
    for (const fa of [0, 1] as const) {
      const a0 = signedArea([...polylineFor(fa, 0), P1]);
      const a1 = signedArea([...polylineFor(fa, 1), P1]);
      expect(Math.sign(a0)).not.toBe(0);
      expect(Math.sign(a1)).toBe(-Math.sign(a0));
    }
  });

  it('(c) fa 는 크기다 — 현 중점에서 가장 먼 점이 더 멀다 (뮤테이션 1)', () => {
    for (const fs of [0, 1] as const) {
      expect(bulge(polylineFor(1, fs), P0, P1)).toBeGreaterThan(bulge(polylineFor(0, fs), P0, P1));
    }
  });

  it('(d) 네 조합 모두 반경 오차가 0.027% 이하다 (뮤테이션 4)', () => {
    for (const [fa, fs] of FLAG_COMBOS) {
      const error = maxRelativeRadialError(polylineFor(fa, fs), RX, RY, ROT);
      // 0 이 나오면 자가 망가진 것이다 — 근사는 반드시 유한한 오차를 낸다.
      expect(error).toBeGreaterThan(0);
      expect(error).toBeLessThanOrEqual(SPEC_RELATIVE_RADIAL_LIMIT);
    }
  });

  it('네 조합 모두 계산된 끝점이 주어진 끝점과 같다(1e-9)', () => {
    for (const [fa, fs] of FLAG_COMBOS) {
      const cubics = arcFor(fa, fs);
      const last = cubics[cubics.length - 1];
      if (last?.c !== 'C') throw new Error('expected cubic');
      expect(last.x).toBeCloseTo(P1.x, 9);
      expect(last.y).toBeCloseTo(P1.y, 9);
    }
  });
});

describe('실측 — 오차가 SPEC 의 주장과 어디쯤인가 (불변식 K5)', () => {
  it('실측 최악 2.72529e-4 가 SPEC 의 2.7e-4 를 1.0094배 넘는다 — 수로 적는다', () => {
    // **실측값**(표본 256/조각, 독립 외심으로 잰 상대 반경 오차):
    //   조각각 90° 원호(90°·180°·270° 전부) : 2.72529e-4  ← 참 최악. 조각각이 상한에 닿는다
    //   타원 고정 입력 fa=0 (조각 1, ~120°)  : 1.20066e-4
    //   타원 고정 입력 fa=1 (조각 4, ~64°)   : 6.21389e-5
    //
    // **표본 밀도가 자의 일부다.** 64/조각으로는 봉우리를 놓쳐 2.71994e-4 로 낮게 잡힌다 —
    // 재려는 것이 최댓값이므로 성기게 재면 언제나 낙관적으로 나온다.
    //
    // 조각각이 상한에 **닿을 때만** 최악이 나온다는 사실이 요점이다 — 타원 고정 입력만으로
    // 재면 참 최악을 절반도 보지 못하고, 그 아래에서는 SPEC 의 한계가 지켜지는 것처럼 보인다.
    let worst = 0;
    for (const [fa, fs] of FLAG_COMBOS) {
      worst = Math.max(worst, maxRelativeRadialError(polylineFor(fa, fs, 256), RX, RY, ROT));
    }
    for (const [x1, y1, fa] of [
      [0, 100, 0],
      [-100, 0, 0],
      [0, 100, 1],
    ] as const) {
      const cubics = arcToCubics(100, 0, 100, 100, 0, fa, 1, x1, y1);
      worst = Math.max(worst, maxRelativeRadialError(samplePath(cubics, { x: 100, y: 0 }, 256), 100, 100, 0));
    }
    expect(worst).toBeCloseTo(MEASURED_WORST_RELATIVE_RADIAL_ERROR, 8);
    expect(worst).toBeGreaterThan(SPEC_RELATIVE_RADIAL_LIMIT);
    expect(worst / SPEC_RELATIVE_RADIAL_LIMIT).toBeLessThan(1.01);
  });

  it('오차가 평탄화 허용 오차보다 두 자릿수 작다 — 관계로 단언한다', () => {
    // 최악 rx = PATH_LOCAL_EXTENT/2 = 5000 로컬 단위, 상자 한 변 160 캔버스 단위,
    // 스테이지 축척 0.5(단위 축척이 아니다 — 시험 규율).
    const localRadiusError = MEASURED_WORST_RELATIVE_RADIAL_ERROR * 5000;
    const quantisationError = 0.5;
    const pxError = ((localRadiusError + quantisationError) / 10000) * 160 * 0.5;
    const flattenTolerancePx = 0.5;
    expect(pxError * 30).toBeLessThan(flattenTolerancePx);
  });
});

describe('조각각 상한 (뮤테이션 4)', () => {
  it('90° 원호는 조각 하나다', () => {
    // 중심 (0,0) · 반지름 100 · (100,0) → (0,100), 작은 호.
    const cubics = arcToCubics(100, 0, 100, 100, 0, 0, 1, 0, 100);
    expect(cubics).toHaveLength(1);
    const swept = sweptAngleAbout({ x: 0, y: 0 }, samplePath(cubics, { x: 100, y: 0 }));
    expect(swept).toBeCloseTo(Math.PI / 2, 3);
  });

  it('270° 큰 호는 조각 셋이고 조각각이 상한 이하다', () => {
    // **큰 호는 반대쪽 중심을 쓴다.** 같은 두 끝점과 반지름에 대해 중심 후보는 (0,0) 과
    // (100,100) 둘이고, `fa=1` 이 뒤의 것을 고른다 — 그 사실이 이 시험의 중심 인자다.
    const cubics = arcToCubics(100, 0, 100, 100, 0, 1, 1, 0, 100);
    const swept = sweptAngleAbout({ x: 100, y: 100 }, samplePath(cubics, { x: 100, y: 0 }));
    expect(swept).toBeCloseTo((3 * Math.PI) / 2, 2);
    expect(cubics).toHaveLength(Math.ceil(swept / ARC_SEGMENT_MAX_RAD));
    expect(swept / cubics.length).toBeLessThanOrEqual(ARC_SEGMENT_MAX_RAD + 1e-9);
  });

  it('180° 반원은 조각 둘이다', () => {
    const cubics = arcToCubics(100, 0, 100, 100, 0, 0, 1, -100, 0);
    expect(cubics).toHaveLength(2);
  });
});

describe('사양이 정한 경계 셋 (REQ-01)', () => {
  it('두 끝점이 같으면 명령이 통째로 사라진다', () => {
    expect(arcToCubics(5, 5, 50, 30, 20, 1, 1, 5, 5)).toEqual([]);
  });

  it('rx 또는 ry 가 0 이면 직선 하나다', () => {
    expect(arcToCubics(0, 0, 0, 30, 0, 1, 1, 60, 20)).toEqual([{ c: 'L', x: 60, y: 20 }]);
    expect(arcToCubics(0, 0, 50, 0, 0, 1, 1, 60, 20)).toEqual([{ c: 'L', x: 60, y: 20 }]);
  });

  it('반지름이 모자란 호가 끝점에 닿는다 — √Λ 확대 (뮤테이션 2)', () => {
    // 현 길이 200 인데 반지름 10 — 도구가 좌표를 반올림해 내보내면 실제로 흔하다.
    // **"NaN 이 없다" 만으로는 이 가드가 물지 않는다**: 확대를 지워도 `max(0, …)` 가
    // 음수 제곱근을 0 으로 받아 주므로 유한한 — 그러나 **끝점에 닿지 못하는** 작은 호가
    // 나온다(반지름 10 짜리 호가 (110,0) 에서 끝난다). 끝점을 재는 것이 이 가드의 자다.
    const cubics = arcToCubics(0, 0, 10, 10, 0, 0, 1, 200, 0);
    expectIntactArc(cubics);
    expectEndsAt(cubics, { x: 200, y: 0 });
    // 확대된 반지름은 현의 절반이므로 반원이 된다.
    const swept = sweptAngleAbout({ x: 100, y: 0 }, samplePath(cubics, { x: 0, y: 0 }));
    expect(swept).toBeCloseTo(Math.PI, 2);
  });

  it('반올림 잔차가 음수 제곱근을 만드는 호에 NaN 이 없다 — max(0, …) (뮤테이션 3)', () => {
    // **대칭 고정 입력이 이 가드를 재운다.** 처음 쓴 `A 50 50 0 0 1 100 0`(수평 · Λ = 1)은
    // 분자가 **정확히 0** 이라 음수가 될 여지가 없었다. 아래는 확대 뒤 분자가
    // `−2.87e−16` 이 되는 자리 — 두 축이 함께 움직여야 잔차가 남는다.
    const cubics = arcToCubics(0, 0, 5, 5, 0, 0, 1, -120, -120);
    expectIntactArc(cubics);
    // Λ = 288 — 반지름이 17배로 늘어나는 자리라 끝점 재구성 오차가 1.3e-6 까지 커진다.
    expectEndsAt(cubics, { x: -120, y: -120 }, 4);
  });

  it('반올림 잔차가 cos 를 −1 밖으로 미는 호에 NaN 이 없다 — acos 죔 (뮤테이션 5)', () => {
    // 같은 이유로 대칭 입력은 이 가드도 재운다. 아래는 `cos = −1.0000000000000002` 가
    // 나오는 자리이며, 죔이 없으면 `acos` 가 NaN 을 낸다.
    const cubics = arcToCubics(0, 0, 5, 5, 0, 0, 1, -120, -92);
    expectIntactArc(cubics);
    expectEndsAt(cubics, { x: -120, y: -92 }, 4);
  });

  it('음수 반지름은 절댓값으로 읽는다', () => {
    const negative = arcToCubics(0, 0, -50, -30, 20, 1, 1, 60, 20);
    expect(negative).toEqual(arcFor(1, 1));
  });

  it('비유한 인자는 빈 목록이다 — 예외를 던지지 않는다 (REQ-07)', () => {
    expect(() => arcToCubics(0, 0, Number.NaN, 30, 0, 0, 1, 60, 20)).not.toThrow();
    expect(arcToCubics(0, 0, Number.NaN, 30, 0, 0, 1, 60, 20)).toEqual([]);
    expect(arcToCubics(0, 0, 50, 30, 0, 0, 1, Number.POSITIVE_INFINITY, 20)).toEqual([]);
  });
});

describe('붙은 플래그가 실제 축약을 지난다 (위험 R2)', () => {
  it('a50 30 20 0160 20 이 a 50 30 20 0 1 60 20 과 같은 곡선을 낸다', () => {
    // 토크나이저 단위가 아니라 **끝까지 흐른 결과**로 잰다 — 두 층 사이 이음매가 이
    // 저장소가 이미 물린 자리다.
    const packed = parseSvgPathData('M 0 0 a50 30 20 0160 20');
    const spaced = parseSvgPathData('M 0 0 a 50 30 20 0 1 60 20');
    expect(packed).toEqual(spaced);
    expect(packed.length).toBeGreaterThan(1);
  });

  it('붙은 플래그를 한 수로 읽으면 그림이 달라진다 — 그 사실을 시험이 든다', () => {
    // `0160` 을 한 수로 삼킨 파서가 낼 결과(`fa=160` 을 flag 로 못 읽어 명령을 버림)와
    // 다름을 보인다.
    const packed = parseSvgPathData('M 0 0 a50 30 20 0160 20');
    const swallowed = parseSvgPathData('M 0 0 a50 30 20 160 20');
    expect(packed).not.toEqual(swallowed);
  });
});
