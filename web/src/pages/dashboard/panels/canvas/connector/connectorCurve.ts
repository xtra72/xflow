// 곡선 연결선의 3차 베지어 조각 (SPEC-CANVAS-011 M6).
//
// ## 새 명령을 만들지 않는다
//
// 요구 ⑤의 "중간점이 양옆 두 점과 이루는 삼각형" 은 그대로 **2차 베지어**다 — 시작점 A,
// 제어점 P, 끝점 B. 008 이 경로 어휘에서 2차를 기각하며 그 근거를 이미 적어 두었다:
//
// > 2차 베지어를 두지 않는 것은 3차가 그것을 **정확히** 표현하기 때문이다
// > (`c1 = p0 + 2/3(q−p0)`).
//
// 그래서 011 은 그 환산을 여기 한 번 적고, 그리는 쪽은 종전의 `bezierCurveTo` 를 그대로
// 지난다. `DrawContext2D` 에 멤버가 늘지 않는다(007 이 가드로 고정한 표면 — AC-49).
//
// ## 중간점이 둘 이상일 때 (일반화)
//
// 요구를 **글자대로** 읽으면 중간점마다 "양옆 두 점을 끝으로 하는" 2차를 그리게 되는데,
// 중간점이 둘이면 그 둘의 구간이 서로 겹쳐 선이 제자리로 되돌아온다 — m₁ 의 2차는
// `A→m₂` 를 덮고 m₂ 의 2차는 `m₁→B` 를 덮으므로 `m₁..m₂` 가 **두 번** 그려진다.
// 그러므로 글자대로의 읽기는 중간점 하나에서만 뜻이 선다.
//
// 011 이 고른 일반화는 하나다:
//
//   **중간점은 전부 제어점이고, 곡선 위의 점은 두 끝점과 이웃한 두 제어점의 중점이다.**
//
// 이것이 2차 B-스플라인을 베지어 조각으로 펴는 표준 형태이며, 고른 이유는 **줄어드는
// 방식** 때문이다:
//
//   - 중간점 하나  → 곡선 위의 점이 두 끝뿐이라 `A→B`(제어점 P) 2차 하나가 된다(AC-48).
//   - 중간점 없음  → 조각이 하나도 없어 호출자가 꺾은 선으로 떨어진다(AC-50).
//
// 즉 SPEC 이 값으로 못박아 둔 두 경우에서 이 일반화는 글자대로의 읽기와 **같은 그림**이고,
// 글자대로가 뜻을 잃는 자리에서만 갈린다. 이웃한 두 조각은 중점에서 접선을 공유하므로
// (두 제어점이 그 중점을 사이에 두고 마주 본다) 꺾임 없이 이어진다.
//
// ## 단위를 모른다
//
// 캔버스 단위로 주든 px 로 주든 같은 산술이다 — 투영은 축마다 상수를 곱하는 아핀 변환이라
// 베지어 제어점을 보존한다. 그래서 이 모듈에는 `CanvasPoint` 도 `PxPoint` 도 없다.
// **그리는 쪽은 투영한 뒤에 부른다** — 그래야 AC-48 이 재는 제어점이 곧 그려진 좌표다.
//
// **이 모듈은 DOM 도 투영도 모른다.** 순수 산술뿐이다.
//
// @spec SPEC-CANVAS-011 REQ-04 · AC-48 · AC-50

/** 평면 위의 한 점. 단위는 호출자가 정한다(캔버스 단위이거나 px 이거나). */
export interface CurvePoint {
  x: number;
  y: number;
}

/** 3차 베지어 한 조각 — `bezierCurveTo` 의 인자 셋 그대로다. 시작점은 직전 조각의 끝이다. */
export interface CubicSegment {
  c1: CurvePoint;
  c2: CurvePoint;
  to: CurvePoint;
}

/** 2차를 3차로 정확히 옮기는 그 비율. 008 이 2차 명령을 기각한 근거의 전부다. */
const TWO_THIRDS = 2 / 3;

/** 끝점 `end` 쪽 3차 제어점 — `end + ⅔(q − end)`. 두 끝에 같은 식이 걸린다. */
function control(end: CurvePoint, q: CurvePoint): CurvePoint {
  return {
    x: end.x + TWO_THIRDS * (q.x - end.x),
    y: end.y + TWO_THIRDS * (q.y - end.y),
  };
}

/** 두 점의 중점. 이웃한 두 제어점 사이에서 곡선이 지나는 자리다. */
function midway(a: CurvePoint, b: CurvePoint): CurvePoint {
  return { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 };
}

/**
 * 점 목록(`[시작, …중간점, 끝]`)을 3차 베지어 조각으로 편다.
 *
 * 중간점이 없으면 **빈 목록**이다 — 휠 자리가 없으므로 그릴 곡선도 없고, 호출자는 그때
 * 꺾은 선으로 떨어진다(AC-50 이 재는 그 성질이다). 빈 목록을 내는 것과 "직선 한 조각"을
 * 내는 것은 다르다: 후자를 내면 중간점 없는 `curve` 가 `bezierCurveTo` 를 부르게 되어
 * `straight` 와 **호출 기록이 갈린다.**
 */
export function curveSegments(points: readonly CurvePoint[]): readonly CubicSegment[] {
  // 제어점은 가운데 점들이다. 양 끝은 늘 곡선 위에 있다.
  const controls = points.slice(1, -1);
  const first = points[0];
  const last = points[points.length - 1];
  if (controls.length === 0 || first === undefined || last === undefined) return [];

  const out: CubicSegment[] = [];
  let from = first;
  controls.forEach((q, i) => {
    // 마지막 제어점의 조각만 끝점에서 멈춘다. 그 앞은 다음 제어점과의 중점까지다.
    const next = controls[i + 1];
    const to = next === undefined ? last : midway(q, next);
    out.push({ c1: control(from, q), c2: control(to, q), to });
    from = to;
  });
  return out;
}
