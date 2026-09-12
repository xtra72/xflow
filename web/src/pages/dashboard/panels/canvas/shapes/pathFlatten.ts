// 경로 평탄화와 내부 판정 (SPEC-CANVAS-008 M4).
//
// 히트 판정이 **바운딩 박스가 아니어야 하는** 이유는 이 저장소가 이미 적어 두었다 —
// `canvasHitTest.ts` 의 타원 갈래가 그 문장을 갖고 있다: *"박스 모서리의 빈 곳을 눌러도
// 잡히면 겹쳐 놓은 요소의 선택이 눈에 보이는 그림과 어긋난다."* 별의 오목한 사이, 십자의
// 겨드랑이, 화살표 꼬리 양옆이 정확히 그 빈 곳이며, 카탈로그 30종 가운데 그런 오목 지역을
// 가진 것이 12종이다. 그래서 경로는 **제 윤곽으로** 잡힌다.
//
// **기각한 안 — `ctx.isPointInPath`.** 브라우저가 바로 이 판정을 해 준다. 그러나
// `canvasHitTest.ts` 는 머리말에서 "DOM 무의존" 을 계약으로 못박았고, 그 계약 덕에 히트
// 판정 전량이 jsdom 없이 단위 시험된다. 게다가 판정 때마다 경로를 **다시 발행**해야 하므로
// 발행 자리가 둘이 된다. 값이 맞지 않는다.
//
// **소비자가 둘이 되는 것은 감추지 않는다.** 렌더는 진짜 3차 베지어를 그리고 히트는 여기서
// 평탄화한 폴리라인을 훑는다. 갈라짐은 수로 묶인다 — 단일 출처는 명령 목록이고, 두 소비자는
// **같은 투영 함수**(`projectPathPoints`)를 지나며, 다른 것은 마지막 한 걸음뿐이다. 그리고
// 평탄화 오차는 집기 여유의 1/12 로 죈다(아래 `FLATTEN_TOLERANCE_PX` · 불변식 J4).
//
// **이 모듈은 DOM 을 모른다.** 점과 숫자뿐이라 jsdom 없이 전량 단위 시험된다. 변까지의
// 거리는 여기서 재지 않는다 — 그 자는 `canvasHitTest.distanceToSegment` 에 이미 있고,
// 두 번째 측정원을 만들지 않는다.
//
// @spec SPEC-CANVAS-008 REQ-07 · AC-04

import type { PxPathCommand, PxPoint } from '../canvasGeometry';

// --- 상수 ---------------------------------------------------------------

/**
 * 평탄화 허용 오차(CSS px). 곡선을 폴리라인으로 바꿀 때 참 곡선에서 벗어나는 최대 거리다.
 *
 * **`canvasHitTest.HIT_TOLERANCE_PX`(6) 의 1/12 이라는 사실이 이 상수의 전부다**(불변식 J4).
 * 렌더와 히트의 최대 어긋남이 사용자가 이미 받고 있는 집기 여유의 8% 보다 작으므로, 그
 * 어긋남은 화면에서 관측될 수 없다. **두 상수의 비가 뒤집히는 순간 그것은 결함이 아니라
 * 상수의 잘못이다** — 그래서 시험은 두 수를 따로 세지 않고 **관계**를 단언한다.
 *
 * 성능을 근거로 이 값을 키우자는 제안이 나오면 되짚어야 한다(위험 R3). 비용은 프레임당이
 * 아니라 **포인터 사건당** 1회이므로, 여기서 아낄 것이 애초에 크지 않다.
 */
export const FLATTEN_TOLERANCE_PX = 0.5;

/**
 * 3차 베지어 한 조각의 재귀 이등분 상한.
 *
 * 평탄도는 이등분마다 대략 1/4 로 줄어드는데, 좌표는 이미 스테이지 px 로 투영된 값이라
 * 화면 크기(수천 px)를 넘지 않는다 — 0.5px 까지 내려가는 데 실제로 필요한 깊이는 일곱쯤이다.
 * 상한을 두는 것은 정확도가 아니라 **한 프레임을 삼키지 않기 위한 죔쇠**이며(REQ-07 이
 * `MAX_PATH_COMMANDS` 를 둔 것과 같은 이유), 손상된 좌표가 수렴하지 않을 때 재귀가 스택을
 * 먹는 것을 막는다.
 */
const MAX_SUBDIVISION_DEPTH = 10;

// --- 타입 ---------------------------------------------------------------

/**
 * 평탄화된 부분 경로 하나 — 점 목록과 **닫혔는가**.
 *
 * 닫힘을 값으로 들고 다니는 것이 이 타입의 존재 이유다. 점 목록만 돌려주면 닫힘은
 * "첫 점과 끝 점이 우연히 같은가" 로만 읽히고, 그 우연은 열린 도형이 제자리로 돌아오는
 * 경우와 구분되지 않는다. 닫힘은 **내부 판정에 참여할 자격**이므로(아래 `isInsidePath`)
 * 우연에 기대어 읽을 값이 아니다.
 */
export interface FlatSubpath {
  /** 스테이지 로컬 CSS px 정점들. 닫힘 변의 끝점은 중복해 담지 않는다. */
  points: readonly PxPoint[];
  /** `Z` 로 닫혔는가. 닫힌 것만 내부 판정에 참여한다. */
  closed: boolean;
}

// --- 평탄화 -------------------------------------------------------------

/**
 * 점이 현 `a`→`b` 에서 얼마나 떨어져 있는가(직선 이탈 거리).
 *
 * 현이 퇴화(두 끝점이 같음)했으면 점 거리로 떨어진다 — 그 경우 아래 평탄도 판정은
 * "제어점이 시작점에서 얼마나 멀리 나갔는가" 를 재게 되며, 그것이 옳다.
 */
function distanceFromChord(a: PxPoint, b: PxPoint, p: PxPoint): number {
  const dx = b.x - a.x;
  const dy = b.y - a.y;
  const lengthSq = dx * dx + dy * dy;
  if (lengthSq === 0) return Math.hypot(p.x - a.x, p.y - a.y);
  return Math.abs(dx * (a.y - p.y) - dy * (a.x - p.x)) / Math.sqrt(lengthSq);
}

/** 두 점의 중점(de Casteljau 이등분에 쓴다). */
function midpoint(a: PxPoint, b: PxPoint): PxPoint {
  return { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 };
}

/**
 * 3차 베지어 한 조각을 폴리라인으로 편다. **시작점은 담지 않는다** — 호출부가 이미 갖고
 * 있고, 담으면 이음매마다 점이 둘씩 겹친다.
 *
 * 평탄도 판정은 두 제어점이 현에서 벗어난 거리 가운데 **큰 쪽**이다. 하나만 보면 한
 * 제어점만 멀리 나간 S자 곡선이 평평하다고 판정된다.
 */
function flattenCubic(
  p0: PxPoint,
  p1: PxPoint,
  p2: PxPoint,
  p3: PxPoint,
  tol: number,
  depth: number,
  out: PxPoint[],
): void {
  const flatness = Math.max(distanceFromChord(p0, p3, p1), distanceFromChord(p0, p3, p2));
  if (depth >= MAX_SUBDIVISION_DEPTH || !(flatness > tol)) {
    // `!(flatness > tol)` 로 적은 것은 NaN 을 평평한 것으로 보기 위해서다. 손상된 좌표가
    // 재귀를 상한까지 몰고 가는 것보다 한 조각을 현으로 두는 편이 값이 싸다.
    out.push(p3);
    return;
  }
  // de Casteljau 이등분.
  const p01 = midpoint(p0, p1);
  const p12 = midpoint(p1, p2);
  const p23 = midpoint(p2, p3);
  const p012 = midpoint(p01, p12);
  const p123 = midpoint(p12, p23);
  const mid = midpoint(p012, p123);
  flattenCubic(p0, p01, p012, mid, tol, depth + 1, out);
  flattenCubic(mid, p123, p23, p3, tol, depth + 1, out);
}

/**
 * 투영된 명령 목록을 **부분 경로마다 하나씩**의 폴리라인으로 편다.
 *
 * - `M` 은 앞선 부분 경로를 끝내고 새 부분 경로를 연다.
 * - `L` 은 점 하나를 잇는다.
 * - `C` 는 재귀 이등분으로 잘게 나눈다(`tol` 이 그 잣대다).
 * - `Z` 는 **그 부분 경로를 닫고** 다음 명령은 닫은 자리에서 이어진다(canvas 명세와 같다).
 *
 * 첫 명령이 `M` 이 아닌 목록도 견딘다 — 파서가 그런 목록을 씨앗 경로로 떨어뜨리지만
 * (`canvasConfig`), 이 함수는 읽기 경로의 견고성을 스스로도 지킨다. 그때 원점은 `(0,0)`
 * 이 아니라 **첫 좌표 자신**이며, 그래야 없는 자리에서 선이 뻗어 나오지 않는다.
 *
 * 점이 하나뿐인 부분 경로(`M` 만 있는 경우)도 버리지 않는다. 그 경우 변이 없으므로 거리
 * 판정이 점 거리로 떨어진다 — `distanceToSegment` 가 길이 0 선분에 대해 하는 그대로다.
 */
export function flattenPath(
  cmds: readonly PxPathCommand[],
  tol: number,
): readonly FlatSubpath[] {
  const subpaths: FlatSubpath[] = [];
  let points: PxPoint[] = [];
  let current: PxPoint | undefined;
  let start: PxPoint | undefined;

  /** 열린 부분 경로를 지금까지 모은 채로 끝낸다(점이 없으면 아무것도 남기지 않는다). */
  const flush = (closed: boolean): void => {
    if (points.length > 0) subpaths.push({ points, closed });
    points = [];
  };

  /**
   * `Z` 직후처럼 **현재 점은 있는데 담긴 점이 없는** 자리에서 부분 경로를 다시 연다.
   * 그래야 닫은 자리에서 이어진 선이 시작점을 잃지 않는다. `M` 없이 시작한 목록에는
   * 현재 점이 없으므로 아무것도 하지 않고, 그때의 원점은 첫 명령 자신이 된다.
   */
  const resume = (): void => {
    if (points.length === 0 && current !== undefined) points.push(current);
  };

  /** 첫 점이 정해지는 순간 부분 경로의 시작점을 기억한다(닫힘 변의 끝이 그 점이다). */
  const rememberStart = (): void => {
    if (start === undefined) start = points[0];
  };

  for (const cmd of cmds) {
    switch (cmd.c) {
      case 'M': {
        flush(false);
        current = { x: cmd.x, y: cmd.y };
        start = current;
        points = [current];
        break;
      }
      case 'L': {
        resume();
        const next = { x: cmd.x, y: cmd.y };
        points.push(next);
        current = next;
        rememberStart();
        break;
      }
      case 'C': {
        resume();
        // `M` 없이 곡선으로 시작한 목록의 원점은 **첫 제어점**이다. 끝점을 원점으로
        // 삼으면 곡선이 한 점으로 접혀 보이지 않게 된다.
        const from = current ?? { x: cmd.x1, y: cmd.y1 };
        if (points.length === 0) points.push(from);
        flattenCubic(
          from,
          { x: cmd.x1, y: cmd.y1 },
          { x: cmd.x2, y: cmd.y2 },
          { x: cmd.x, y: cmd.y },
          tol,
          0,
          points,
        );
        current = { x: cmd.x, y: cmd.y };
        rememberStart();
        break;
      }
      case 'Z': {
        flush(true);
        // 닫은 뒤의 현재 점은 그 부분 경로의 **시작점**이다(canvas 명세). 이어지는 명령이
        // 있으면 `resume` 가 거기서부터 새 부분 경로를 연다. 이어지는 것이 없으면 아무
        // 부분 경로도 열리지 않는다 — 그래서 `Z` 로 끝난 목록에 점 하나짜리 꼬리가
        // 붙지 않는다.
        current = start;
        break;
      }
    }
  }
  flush(false);
  return subpaths;
}

// --- 내부 판정 ----------------------------------------------------------

/**
 * `a`→`b` 선분을 기준으로 `p` 가 어느 쪽인가(양수 = 왼쪽). 감기 수의 부호를 정한다.
 */
function sideOf(a: PxPoint, b: PxPoint, p: PxPoint): number {
  return (b.x - a.x) * (p.y - a.y) - (p.x - a.x) * (b.y - a.y);
}

/**
 * 한 닫힌 폴리라인에 대한 감기 수(winding number).
 *
 * 위로 지나는 변은 +1, 아래로 지나는 변은 −1 을 더한다. 경계 조건을 `<=` 와 `>` 로 갈라
 * 둔 것은 정점을 정확히 지나는 수평선이 **두 번 세지지 않게** 하기 위해서다(정점 하나가
 * 두 변에 속하므로, 한쪽만 포함해야 한다).
 */
function windingNumber(points: readonly PxPoint[], p: PxPoint): number {
  let wn = 0;
  for (let i = 0; i < points.length; i++) {
    const a = points[i];
    const b = points[(i + 1) % points.length];
    if (a === undefined || b === undefined) continue;
    if (a.y <= p.y) {
      if (b.y > p.y && sideOf(a, b, p) > 0) wn++;
    } else if (b.y <= p.y && sideOf(a, b, p) < 0) {
      wn--;
    }
  }
  return wn;
}

/**
 * 점이 경로 **안쪽**인가 — nonzero winding 규칙.
 *
 * **닫힌 부분 경로만 참여한다.** 열린 부분 경로(곡선 화살표처럼 `Z` 로 끝나지 않는 도형)는
 * 안쪽이라는 개념이 없으므로 이 판정에서 빠지고, 대신 변까지의 거리 판정이 그것을 잡는다
 * (spec.md §히트 테스트: *"열린 경로도 잡힌다 — 내부 판정은 실패하지만…"*). 열린 것을
 * 암묵적으로 닫아 채우는 `fill()` 의 규칙을 여기 옮겨 오지 않는 이유는 방향이 다르기
 * 때문이다: **잡히지 않는 것은 눈에 보이고, 잘못 잡히는 것은 보이지 않는다.**
 *
 * 여러 닫힌 부분 경로의 감기 수는 **더한다** — 그것이 nonzero 규칙이며, 반대 방향으로 감은
 * 안쪽 윤곽(도넛의 구멍)이 그래야 뚫린다.
 *
 * 윤곽 **위**의 점은 정의상 어느 쪽도 아니지만 여기서는 안쪽으로 떨어진다. 히트 판정에는
 * 아무 영향이 없다 — 윤곽 위는 변까지의 거리 0 이라 어차피 잡히기 때문이다.
 */
export function isInsidePath(subpaths: readonly FlatSubpath[], p: PxPoint): boolean {
  let wn = 0;
  for (const sub of subpaths) {
    if (!sub.closed) continue;
    wn += windingNumber(sub.points, p);
  }
  return wn !== 0;
}
