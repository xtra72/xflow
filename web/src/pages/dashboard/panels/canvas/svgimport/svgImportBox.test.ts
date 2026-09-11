// 요소 상자 시험 — 참 바운딩 박스 · 퇴화 · 상대 배치 (SPEC-CANVAS-007 결함 D3).
//
// **이 파일이 재는 것은 "손잡이가 제 잉크에 닿는가" 하나다.** 상자가 헐거우면 손잡이가
// 그림 밖에 서고, 좁으면 잉크가 상자 밖으로 새며, 도형마다 축척을 지어내면 그림이 흩어진다.
//
// **고정 입력의 규율.**
//   - `viewBox` 는 `"-13 7 317 181"` — 원점 ≠ 0 · `minX` 는 음수인데 `minY` 는 양수 ·
//     비정사각 · `10000` 을 나누어떨어뜨리지 않는다. 원점 0 이면 뺄셈이 항등이라 원점을
//     지운 결함이 통과하고, 정사각이면 x·y 축척을 맞바꾼 결함이 통과한다.
//   - 곡선은 **비대칭**이다. 대칭 곡선은 제어점 둘을 맞바꿔도 같은 모양이고, `t = 0.5` 의
//     값 하나만 보는 시험은 그 대칭 아래에서 언제나 통과한다.
//   - 도형은 **둘 이상**이다. 하나짜리 고정 입력에서는 상대 배치가 무너지는 결함이 관측되지
//     않는다 — 도형 하나는 언제나 제 자신에 대해 제자리다.
//
// **참값의 기준을 손으로 적지 않고 촘촘한 표본으로 잡는다.** 그리고 **양쪽으로** 죈다:
// 상자는 표본을 전부 담아야 하고(성기게 재면 최대를 낙관적으로 읽어 여기서 빨개진다),
// 동시에 표본보다 눈에 띄게 넓어서도 안 된다(제어점 껍질로 재면 여기서 빨개진다). 한쪽만
// 죄면 껍질도 표본도 통과한다.
//
// **확인한 뮤테이션** — "→" 뒤가 빨개지는 단언이다.
//   1. `tightCommandBounds` 를 `commandBounds`(제어점 껍질)로 갈면 → "껍질보다 좁다" 가
//      빨개진다.
//   2. `cubicExtremaTimes` 의 `a === 0` 갈래를 지우면(언제나 이차식으로 풀면) → "제어점이
//      고르게 놓인 곡선" 이 빨개진다(0/0 → `NaN`).
//   3. `cubicExtremaTimes` 의 `t > 0` 를 지우면 → "근이 구간 앞에 있는 곡선" 이 빨개진다.
//      `t < 1` 을 지우면 → "근이 구간 뒤에 있는 곡선" 이 빨개진다.
//   3b. **물지 않는 것으로 확인한 뮤테이션 셋**(그래서 그 가드들을 지웠다).
//      `t > 0 && t < 1` 을 `t >= 0 && t <= 1` 로 **넓히면** 아무것도 빨개지지 않는다 —
//      끝점은 명령 자체가 나르므로 이미 세었다. `b === 0` 가드와 `disc >= 0` 가드를 지워도
//      아무것도 빨개지지 않는다 — `±∞` 도 `NaN` 도 `inside` 가 함께 거른다. 물지 않는
//      가드는 죽은 가지이므로 남기지 않고 **지웠다**.
//   4. `axisSpan` 의 넓히기를 지우면 → "가로선의 상자가 최소 크기다" 가 빨개진다.
//   5. `axisSpan` 의 가운데 맞추기(`(lo + hi) / 2 - floor / 2`)를 `lo` 로 바꾸면 → "선이
//      상자 한가운데를 지난다" 가 빨개진다.
//   6. `placeShape` 의 `scaleX`/`scaleY` 를 맞바꾸면 → "납작한 문서" 가 빨개진다.
//      **정사각 도형만으로는 빨개지지 않는다** — 상자가 문서 종횡비를 지키므로 두 축척이
//      반올림만큼(1.2618 대 1.2597)밖에 다르지 않고, 18 단위 도형에서는 둘 다 23 으로
//      반올림된다. 축척이 크게 갈라지는 것은 납작한 문서뿐이다(실측으로 확인했다).
//   7. `placeShape` 가 도형마다 제 축척을 지어내면 → "상대 배치가 보존된다" 가 빨개진다.
//   8. `Z` 뒤 현재 점 되돌리기(`cursor = subPathStart`)를 지우면 → "Z 뒤에 이어지는 곡선"
//      이 빨개진다.
//   9. `C` 갈래의 끝점 `include(cmd.x, cmd.y)` 를 지우면 → "대칭 곡선" · "`M` 없이 시작한
//      목록" 이 빨개진다.
//  10. `placeShape` 의 도형 원점 더하기를 지우면(전부 문서 틀 원점에) → "구석의 작은 도형" ·
//      "상대 배치" · (놓기 층의) "가져온 요소들끼리 정렬" 이 빨개진다.
//  11. `− doc.viewBox.minX` 를 지우면 → "문서 전부를 차지하는 도형" 이 빨개진다.
//      **`viewBox` 원점이 0 인 고정 입력에서는 빨개지지 않는다** — 뺄셈이 항등이다.
//  12. `coordinate(…, MIN_ELEMENT_EXTENT)` 의 폴백을 0 으로 바꾸면 → "끝없이 넓은 도형"
//      이 빨개진다. `coordinate(…, doc.box.x)` 를 0 으로 바꾸면 → "잴 수 없는 원점" 이
//      빨개진다.
//
// **등가 뮤테이션(빨개지지 않는 것이 옳다)**: 상자를 `winded` 가 아니라 감김 뒤집기 **전**의
// `shape.commands` 에서 재도 아무것도 빨개지지 않는다 — `applyEvenOddWinding` 은 부분 경로의
// **방향**만 뒤집고 점을 옮기지 않으므로 두 상자가 같다. 그래도 `winded` 를 쓰는 것은 조각이
// 거기서 나오기 때문이며, 두 목록을 오가면 언젠가 갈라진다.
//
// @spec SPEC-CANVAS-007 REQ-06 · AC-05 · AC-E8

import { describe, expect, it } from 'vitest';

import { MIN_ELEMENT_EXTENT, type BoxGeometry } from '../canvasConfig';
import { PATH_LOCAL_EXTENT, type PathCommand } from '../shapes/pathTypes';

import type { ViewBox } from './svgDocument';
import { placeShape, tightCommandBounds, viewBoxBounds, type DocumentPlacement } from './svgImportBox';
import { commandBounds, type Bounds } from './svgImportSplit';
import { toLocalCommands } from './svgImportPlan';

/** 원점 ≠ 0 · 부호가 다르다 · 비정사각 · 안 나누어떨어진다. */
const VIEW_BOX: ViewBox = { minX: -13, minY: 7, width: 317, height: 181 };
/** `fitBox(VIEW_BOX, {500, 400})` 의 실측값. 축척은 x 1.2618… · y 1.2597… 로 **다르다**. */
const DOC_BOX: BoxGeometry = { x: 50, y: 86, w: 400, h: 228 };
const DOC: DocumentPlacement = { box: DOC_BOX, viewBox: VIEW_BOX };

/**
 * 곡선 위를 **촘촘히** 훑은 바운딩 박스 — 참값의 기준이다.
 *
 * 20001 점이면 극값 부근에서 곡선이 평평하므로 오차가 `1e-6` 아래다. 이 함수를 구현이
 * 아니라 **시험**에 두는 것이 요점이다: 구현이 표본을 쓰면 최대를 낙관적으로 읽고, 그
 * 낙관은 "상자가 잉크보다 조금 작다" 로 조용히 남는다.
 */
function sampledBounds(from: { x: number; y: number }, cmd: Extract<PathCommand, { c: 'C' }>): Bounds {
  const at = (t: number, p0: number, p1: number, p2: number, p3: number): number => {
    const u = 1 - t;
    return u * u * u * p0 + 3 * u * u * t * p1 + 3 * u * t * t * p2 + t * t * t * p3;
  };
  const steps = 20000;
  let box: Bounds | undefined;
  for (let i = 0; i <= steps; i += 1) {
    const t = i / steps;
    const x = at(t, from.x, cmd.x1, cmd.x2, cmd.x);
    const y = at(t, from.y, cmd.y1, cmd.y2, cmd.y);
    box =
      box === undefined
        ? { minX: x, minY: y, maxX: x, maxY: y }
        : {
            minX: Math.min(box.minX, x),
            minY: Math.min(box.minY, y),
            maxX: Math.max(box.maxX, x),
            maxY: Math.max(box.maxY, y),
          };
  }
  return box!;
}

/** 비대칭 곡선 — 제어점 둘을 맞바꾸면 **모양이 달라진다**(대칭 곡선은 안 달라진다). */
const SKEWED: Extract<PathCommand, { c: 'C' }> = {
  c: 'C',
  x1: 120,
  y1: 0,
  x2: 20,
  y2: 60,
  x: 60,
  y: 90,
};

// --- 참 바운딩 박스 -------------------------------------------------------

describe('곡선의 극값을 푼다 — 제어점 껍질이 아니다 (뮤테이션 1·2)', () => {
  it('대칭 곡선의 최대가 제어점이 아니라 `t = 1/2` 의 75 다', () => {
    // `M 0 0 C 100 0 100 100 0 100` — 껍질은 x 를 100 까지 세지만 곡선은 75 까지만 간다.
    // **이 곡선은 x 에 대해 이차항이 사라진다**(`a = 0`): 제어점이 고르게 놓였기 때문이고,
    // 그래서 일차식 갈래가 여기서 켜진다. 그 갈래를 지우면 0/0 이 되어 근이 `NaN` 이 되고,
    // 걸러진 뒤 상자가 끝점만 담아 `maxX = 0` 이 된다.
    const bounds = tightCommandBounds([
      { c: 'M', x: 0, y: 0 },
      { c: 'C', x1: 100, y1: 0, x2: 100, y2: 100, x: 0, y: 100 },
    ]);
    expect(bounds).toEqual({ minX: 0, minY: 0, maxX: 75, maxY: 100 });
  });

  it('비대칭 곡선이 **촘촘한 표본을 담되 그보다 넓지 않다** — 양쪽으로 죈다', () => {
    const from = { x: 0, y: 0 };
    const tight = tightCommandBounds([{ c: 'M', ...from }, SKEWED])!;
    const sampled = sampledBounds(from, SKEWED);
    // 담는다: 성기게 훑어 최대를 낙관적으로 읽으면 여기서 빨개진다.
    expect(tight.maxX).toBeGreaterThanOrEqual(sampled.maxX - 1e-9);
    expect(tight.maxY).toBeGreaterThanOrEqual(sampled.maxY - 1e-9);
    expect(tight.minX).toBeLessThanOrEqual(sampled.minX + 1e-9);
    expect(tight.minY).toBeLessThanOrEqual(sampled.minY + 1e-9);
    // 넓지 않다: 제어점 껍질로 재면 여기서 빨개진다.
    expect(tight.maxX).toBeLessThanOrEqual(sampled.maxX + 1e-6);
    expect(tight.maxY).toBeLessThanOrEqual(sampled.maxY + 1e-6);
  });

  it('그 곡선의 상자가 제어점 껍질보다 **실제로 좁다** — 아니면 위 시험이 무력하다', () => {
    const commands: PathCommand[] = [{ c: 'M', x: 0, y: 0 }, SKEWED];
    const tight = tightCommandBounds(commands)!;
    const hull = commandBounds(commands)!;
    // 제어점 `x1 = 120` 이 껍질을 120 까지 벌리지만 곡선은 62 를 넘지 않는다.
    expect(hull.maxX).toBe(120);
    expect(tight.maxX).toBeLessThan(hull.maxX - 50);
    // 그리고 극값이 `t = 1/2` 가 **아니다** — 중간점 하나만 보는 시험은 이 곡선을 못 잡는다.
    const mid = 0.125 * 0 + 0.375 * SKEWED.x1 + 0.375 * SKEWED.x2 + 0.125 * SKEWED.x;
    expect(Math.abs(tight.maxX - mid)).toBeGreaterThan(0.5);
  });

  it('제어점 둘을 맞바꾸면 상자가 달라진다 — 대칭 곡선은 이 결함을 숨긴다', () => {
    const straight = tightCommandBounds([{ c: 'M', x: 0, y: 0 }, SKEWED]);
    const swapped = tightCommandBounds([
      { c: 'M', x: 0, y: 0 },
      { c: 'C', x1: SKEWED.x2, y1: SKEWED.y2, x2: SKEWED.x1, y2: SKEWED.y1, x: SKEWED.x, y: SKEWED.y },
    ]);
    expect(swapped).not.toEqual(straight);
  });

  it('`Z` 뒤에 이어지는 곡선의 `P0` 는 **닫은 부분 경로의 시작점**이다 (뮤테이션 8)', () => {
    // `Z` 뒤의 곡선은 (0,0) 에서 출발한다 — 직전 점 (10,0) 이 아니다. 되돌리기를 지우면
    // `P0` 가 (10,0) 이 되어 극값이 다른 자리에서 풀리고 상자가 어긋난다.
    const bounds = tightCommandBounds([
      { c: 'M', x: 0, y: 0 },
      { c: 'L', x: 10, y: 0 },
      { c: 'Z' },
      { c: 'C', x1: -60, y1: 0, x2: -60, y2: 20, x: 0, y: 20 },
    ])!;
    const wrongP0 = tightCommandBounds([
      { c: 'M', x: 10, y: 0 },
      { c: 'C', x1: -60, y1: 0, x2: -60, y2: 20, x: 0, y: 20 },
    ])!;
    expect(bounds.minX).not.toBe(wrongP0.minX);
    // 참값: `P0 = (0,0)` 인 곡선의 x 최소는 `t = 1/2` 의 −45 다.
    expect(bounds.minX).toBeCloseTo(-45, 9);
  });

  it('`M` 없이 시작한 목록은 제어점 껍질로 떨어진다 — `P0` 가 없으면 풀 것이 없다', () => {
    // 문서 층을 지나온 목록은 언제나 `M` 으로 열리므로(실측 `reducePathSegments`) 이 가지에
    // 닿는 길은 이 함수를 직접 부르는 것뿐이다. 그래서 직접 부른다 — 부르지 않으면 이
    // 가지가 커버리지에 구멍으로 남는다.
    const lone: PathCommand[] = [{ c: 'C', x1: -5, y1: 0, x2: 5, y2: 40, x: 10, y: 0 }];
    expect(tightCommandBounds(lone)).toEqual(commandBounds(lone));
  });

  it('그릴 점이 하나도 없으면 `undefined` 다 — 예외가 아니다', () => {
    expect(tightCommandBounds([])).toBeUndefined();
    expect(tightCommandBounds([{ c: 'Z' }])).toBeUndefined();
    expect(tightCommandBounds([{ c: 'M', x: Number.NaN, y: 0 }])).toBeUndefined();
  });

  it('근이 구간 **앞**에 있으면 세지 않는다 — `t > 0` 가 없으면 상자가 왼쪽으로 샌다 (뮤테이션 3)', () => {
    // `B'(t)/3 = t² + 3t + 2` → 근은 −1 · −2 로 **둘 다 구간 앞**이다. `t = −1` 에서 곡선을
    // 평가하면 −2.5 가 나오므로, 거르지 않으면 `minX` 가 0 이 아니라 −2.5 가 된다.
    // 세로를 0 으로 눕혀 `a = b = c = 0`(0/0 → `NaN`)도 같은 비교가 거르는지 함께 잰다.
    expect(
      tightCommandBounds([
        { c: 'M', x: 0, y: 0 },
        { c: 'C', x1: 2, y1: 0, x2: 5.5, y2: 0, x: 11.5, y: 0 },
      ]),
    ).toEqual({ minX: 0, minY: 0, maxX: 11.5, maxY: 0 });
  });

  it('근이 구간 **뒤**에 있어도 세지 않는다 — `t < 1` 이 없으면 상자가 오른쪽으로 샌다', () => {
    // `B'(t)/3 = t² − 5t + 6` → 근은 2 · 3. `t = 2` 의 값은 14 이고 끝점 11.5 를 넘는다.
    expect(
      tightCommandBounds([
        { c: 'M', x: 0, y: 0 },
        { c: 'C', x1: 6, y1: 0, x2: 9.5, y2: 0, x: 11.5, y: 0 },
      ]),
    ).toEqual({ minX: 0, minY: 0, maxX: 11.5, maxY: 0 });
  });

  it('비유한 좌표가 섞여도 성한 점들의 상자가 남는다', () => {
    // 성한 점 하나가 사라지면 상자가 통째로 어긋난다. `t` 걸러내기(`t > 0 && t < 1`)가
    // `NaN` 근을 함께 막는 자리이기도 하다.
    expect(
      tightCommandBounds([
        { c: 'M', x: 4, y: 5 },
        { c: 'C', x1: Number.POSITIVE_INFINITY, y1: 0, x2: 0, y2: 0, x: 20, y: 30 },
      ]),
    ).toEqual({ minX: 4, minY: 5, maxX: 20, maxY: 30 });
  });
});

// --- 문서 틀 안의 자리 ----------------------------------------------------

describe('도형이 문서 틀 안 제 자리에 앉는다 (AC-05 · 뮤테이션 6·7)', () => {
  it('문서 전부를 차지하는 도형의 상자가 곧 문서 틀이다 — 특례 없이 성립한다', () => {
    const placed = placeShape(viewBoxBounds(VIEW_BOX), DOC);
    expect(placed.box).toEqual(DOC_BOX);
    expect(placed.frame).toEqual(VIEW_BOX);
  });

  it('구석의 작은 도형이 **작은 상자**를 얻는다 — 문서 틀이 아니다', () => {
    // 문서(317 × 181) 안의 60 × 40 사각. 공유 상자 시절에는 이 도형도 400 × 228 상자를
    // 받았고, 그래서 손잡이 여덟이 제 잉크에서 수백 단위 떨어진 자리에 섰다.
    const placed = placeShape({ minX: 37, minY: 47, maxX: 97, maxY: 87 }, DOC);
    expect(placed.box).toEqual({ x: 113, y: 136, w: 76, h: 50 });
    expect(placed.frame).toEqual({ minX: 37, minY: 47, width: 60, height: 40 });
  });

  it('원은 정사각으로 남는다 — 축척을 맞바꾸면 여기서 빨개진다 (뮤테이션 6)', () => {
    // 사각 하나로는 축을 **맞바꾼** 결함만 잡힌다. 축척이 조금 어긋난 결함은 정사각 도형이
    // 정사각으로 남는지로만 보인다(x 축척 1.2618 · y 축척 1.2597 은 아주 가깝다).
    const placed = placeShape({ minX: 51, minY: 31, maxX: 69, maxY: 49 }, DOC);
    expect(placed.box.w).toBe(placed.box.h);
  });

  it('상대 배치가 보존된다 — 모든 변이 정확한 문서 사상에서 **1 단위 안**이다 (뮤테이션 7)', () => {
    // **도형이 둘 이상이어야** 이 결함이 관측된다 — 하나는 언제나 제 자신에 대해 제자리다.
    // 문서에서 폭 40 인 사각 셋이 서로 다른 자리에 놓인다.
    const SHAPES: readonly Bounds[] = [
      { minX: 0, minY: 10, maxX: 40, maxY: 30 },
      { minX: 60, minY: 10, maxX: 100, maxY: 30 },
      { minX: 200, minY: 120, maxX: 240, maxY: 140 },
    ];
    const scaleX = DOC_BOX.w / VIEW_BOX.width;
    const scaleY = DOC_BOX.h / VIEW_BOX.height;
    const exactX = (u: number): number => DOC_BOX.x + (u - VIEW_BOX.minX) * scaleX;
    const exactY = (u: number): number => DOC_BOX.y + (u - VIEW_BOX.minY) * scaleY;

    const placed = SHAPES.map((b) => placeShape(b, DOC));
    // **네 변 전부**를 잰다. 원점만 재면 폭이 도형마다 다른 축척으로 잡힌 결함이 통과한다.
    // 상한 1 은 반올림 둘(원점 ≤ 0.5 · 길이 ≤ 0.5)이 겹친 값이다 — 500 × 400 캔버스에서
    // 0.2% 이고, 이것이 "빽빽하게 죄면서 흩어뜨리지 않는다" 가 뜻하는 전부다.
    for (const [i, box] of placed.map((p) => p.box).entries()) {
      const b = SHAPES[i]!;
      expect(Math.abs(box.x - exactX(b.minX)), `#${i} left`).toBeLessThanOrEqual(1);
      expect(Math.abs(box.y - exactY(b.minY)), `#${i} top`).toBeLessThanOrEqual(1);
      expect(Math.abs(box.x + box.w - exactX(b.maxX)), `#${i} right`).toBeLessThanOrEqual(1);
      expect(Math.abs(box.y + box.h - exactY(b.maxY)), `#${i} bottom`).toBeLessThanOrEqual(1);
    }
    // 켜져 있음: 도형마다 축척을 지어내는 결함은 **원점 사이의 거리**로만 보인다. 사이가
    // 60 사용자 단위이므로 축척을 곱한 값과 견준다 — 폭이 같다는 단언은 그 결함을 통과시킨다
    // (도형마다 제 축척을 쓰면 폭이 같은 도형은 여전히 같은 폭을 얻는다).
    expect(placed[1]!.box.x - placed[0]!.box.x).toBeCloseTo(60 * scaleX, 0);
    // 세로는 앞의 둘이 같은 줄에 있다 — 축을 섞은 결함이 여기서 드러난다.
    expect(placed[1]!.box.y).toBe(placed[0]!.box.y);
    expect(placed[2]!.box.y).not.toBe(placed[0]!.box.y);
  });

  it('상자와 정규화 틀이 **같은 수**에서 나온다 — 도형의 끝이 로컬 격자의 끝이다', () => {
    const bounds = { minX: 37, minY: 47, maxX: 97, maxY: 87 };
    const placed = placeShape(bounds, DOC);
    const local = toLocalCommands(
      [
        { c: 'M', x: bounds.minX, y: bounds.minY },
        { c: 'L', x: bounds.maxX, y: bounds.maxY },
      ],
      placed.frame,
    );
    // 상자만 좁히고 좌표를 두면 여기서 `M` 이 원점이 아니다 — 이음매의 유일한 가드다.
    expect(local[0]).toEqual({ c: 'M', x: 0, y: 0 });
    expect(local[1]).toEqual({ c: 'L', x: PATH_LOCAL_EXTENT, y: PATH_LOCAL_EXTENT });
  });
});

// --- 퇴화 ----------------------------------------------------------------

describe('퇴화한 축을 사용자 단위에서 넓힌다 (REQ-06 · 가정 A15 · 뮤테이션 4·5)', () => {
  it('가로선의 상자가 두 변 모두 최소 크기 이상이다', () => {
    const placed = placeShape({ minX: 0, minY: 50, maxX: 100, maxY: 50 }, DOC);
    expect(placed.box.h).toBe(MIN_ELEMENT_EXTENT);
    expect(placed.box.w).toBeGreaterThan(MIN_ELEMENT_EXTENT);
  });

  it('그리고 선이 상자 **한가운데**를 지난다 — 위 모서리가 아니다 (뮤테이션 5)', () => {
    const placed = placeShape({ minX: 0, minY: 50, maxX: 100, maxY: 50 }, DOC);
    const local = toLocalCommands([{ c: 'M', x: 50, y: 50 }], placed.frame);
    // 상자 크기만 재는 단언은 "선이 위 모서리에 붙었다" 를 통과시킨다. 그 결함은 저장
    // 왕복도 견디므로 **화면으로만** 보인다.
    expect(local[0]).toEqual({ c: 'M', x: 5000, y: PATH_LOCAL_EXTENT / 2 });
  });

  it('세로선도 같다 — 축 하나만 재면 다른 축의 결함이 새어 나간다', () => {
    const placed = placeShape({ minX: 30, minY: 0, maxX: 30, maxY: 100 }, DOC);
    expect(placed.box.w).toBe(MIN_ELEMENT_EXTENT);
    expect(placed.box.h).toBeGreaterThan(MIN_ELEMENT_EXTENT);
    expect(toLocalCommands([{ c: 'M', x: 30, y: 50 }], placed.frame)[0]).toEqual({
      c: 'M',
      x: PATH_LOCAL_EXTENT / 2,
      y: 5000,
    });
  });

  it('점 하나(두 축 모두 퇴화)도 상자를 얻는다', () => {
    const placed = placeShape({ minX: 30, minY: 50, maxX: 30, maxY: 50 }, DOC);
    expect(placed.box.w).toBe(MIN_ELEMENT_EXTENT);
    expect(placed.box.h).toBe(MIN_ELEMENT_EXTENT);
  });

  it('넓히기는 **모자랄 때만** 켜진다 — 멀쩡한 도형의 상자를 부풀리지 않는다 (뮤테이션 4)', () => {
    const placed = placeShape({ minX: 37, minY: 47, maxX: 97, maxY: 87 }, DOC);
    // 언제나 넓히는 뮤테이션에서는 프레임 원점이 바운딩 박스의 것과 달라진다.
    expect(placed.frame.minX).toBe(37);
    expect(placed.frame.minY).toBe(47);
  });

  it('끝없이 넓은 도형의 **폭**이 최소 크기로 떨어진다 — 예외가 아니다 (뮤테이션 12)', () => {
    // 길이가 `∞` 면 곱셈도 `∞` 라 `coordinate` 의 폴백이 켜진다. 그 폴백이 0 이면 상자가
    // 퇴화하고, 퇴화 상자는 저장 왕복에서 기하를 통째로 씨앗으로 갈아 끼운다.
    const placed = placeShape({ minX: 0, minY: 0, maxX: Number.POSITIVE_INFINITY, maxY: 10 }, DOC);
    expect(placed.box.w).toBe(MIN_ELEMENT_EXTENT);
    expect(placed.box.h).toBeGreaterThan(MIN_ELEMENT_EXTENT);
  });

  it('잴 수 없는 **원점**은 문서 틀의 원점으로 떨어진다 — 0 이 아니다', () => {
    // 원점이 `NaN` 이면 그 도형이 어디 있었는지 알 길이 없다. 0 으로 떨어뜨리면 캔버스
    // 왼쪽 위 구석으로 튀고, 문서 틀로 떨어뜨리면 **그림이 있던 자리**에 남는다.
    const placed = placeShape({ minX: Number.NaN, minY: 5, maxX: 40, maxY: 45 }, DOC);
    expect(placed.box.x).toBe(DOC_BOX.x);
    for (const v of [placed.box.x, placed.box.y, placed.box.w, placed.box.h]) {
      expect(Number.isFinite(v)).toBe(true);
    }
    expect(placed.box.w).toBeGreaterThanOrEqual(MIN_ELEMENT_EXTENT);
    expect(placed.box.h).toBeGreaterThanOrEqual(MIN_ELEMENT_EXTENT);
  });

  it('납작한 문서에서는 두 축척이 크게 갈라진다 — 축을 맞바꾸면 여기서 빨개진다 (뮤테이션 10)', () => {
    // 문서 종횡비를 지키는 상자에서 두 축척은 반올림만큼만 다르므로(1.2618 대 1.2597),
    // 정사각 도형으로는 축 맞바꿈이 **보이지 않는다** — 실측으로 확인했다. 납작한 문서
    // (`0 0 1000 1` → 상자 400 × 1)에서는 0.4 대 1.0 으로 갈라져 비로소 보인다.
    const flat: DocumentPlacement = {
      box: { x: 50, y: 200, w: 400, h: 1 },
      viewBox: { minX: 0, minY: 0, width: 1000, height: 1 },
    };
    // 가로선 — 세로축이 퇴화하므로 **세로축의 최소 길이**가 어느 축척에서 나왔는지가 답을
    // 가른다. 맞바꾸면 높이가 1 이 아니라 3 이 되고 원점도 함께 어긋난다.
    expect(placeShape({ minX: 0, minY: 0.5, maxX: 1000, maxY: 0.5 }, flat).box).toEqual({
      x: 50,
      y: 200,
      w: 400,
      h: 1,
    });
  });
});
