// 자유선의 궤적 — 받기 · 간소화 · 상한 (SPEC-CANVAS-011 M11 · REQ-06).
//
// 연결선 넷 가운데 **점을 만드는 몸짓이 다른 것은 자유선 하나뿐이다.** 꺾은 선과 곡선은
// 사람이 선 위를 눌러 점을 하나씩 찍고(M10), 자유선은 손이 그은 궤적에서 점이 난다. 난
// 뒤에는 셋이 구별되지 않으므로(`ROUTE_POINT_HOST` 의 항등 갈래) 이 모듈이 하는 일은 **궤적을 그
// 점 목록으로 옮기는 것** 하나다.
//
// ## 왜 간소화가 필요한가
//
// 손이 그은 궤적은 포인터 사건 수만큼 점을 낸다. 한 번 긋는 동안 수백 개가 쉽게 나고,
// 그 전부를 적으면 선 하나가 대시보드 snapshot 예산(256KB)의 상당 부분을 먹는다 — 008 이
// 경로 명령에 상한을 둔 그 이유이고, 그 상한을 여기서 **같은 이름으로** 물려받는다
// (`MAX_CONNECTOR_POINTS`).
//
// ## 간소화는 **뗄 때 한 번**이다
//
// 포인터 사건마다 다시 줄이지 않는다. 근거 둘.
//
//   1. **오차가 쌓이지 않는다.** 간소화는 허용 오차 안에서 점을 버리는 일이므로, 줄인
//      목록을 다시 줄이면 오차가 겹친다(두 번이면 최대 2배, n 번이면 n 배). 한 번만 돌면
//      "버려진 점은 전부 남은 선에서 허용 오차 안" 이 **정확히** 참이고, 그 문장은 시험이
//      그대로 단언할 수 있다.
//   2. **셈이 크지 않다.** 받는 쪽에 상한이 있으므로 간소화에 드는 입력은 언제나 상한
//      이하이고, 그 한 번은 손을 떼는 순간 한 번뿐이다.
//
// ## 상한은 **받는 자리**에 선다
//
// REQ-06 이 적은 대로다 — 상한에 닿으면 더 받지 않되 **몸짓은 계속된다.** 받는 쪽에 두면
// 줄인 결과가 상한을 넘을 수 없다(간소화는 점을 늘리지 않는다). 대가는 숨기지 않는다:
// 아주 긴 궤적은 뒷부분이 기록되지 않고, 그때 선은 마지막으로 받은 점에서 **놓은 자리**로
// 곧장 간다. 몸짓 자체는 끝까지 이어지고 예외도 없다(AC-77) — 그리다 만 선을 남기는 것보다
// 거친 선을 남기는 편이 낫다는 것이 그 요구의 판단이다.
//
// ## 점–선분 거리를 **빌려 쓴다**
//
// 점을 버릴지 정하는 자는 그 점이 선분에서 얼마나 벗어났는가 하나이고, 그 산술은 이미
// `canvasHitTest.distanceToSegment` 에 있다(그리고 그것은 다시 `closestPointOnSegment`
// 하나를 지난다). 여기서 같은 여덟 줄을 다시 적으면 이 저장소에 점–선분 거리가 두 벌이
// 되고, 둘이 갈라지는 날 "그은 대로 잡히지 않는" 자리가 열린다 — 히트 모듈 머리말이
// 금지한 그 두 번째 측정원이다. 실제로 그 여덟 줄은 **틀리기 쉽다**: 무한 직선까지를 재는
// 고전 구현으로 적으면 되짚어 온 궤적에서 아래 약속이 거짓이 된다(그 시험이 그 자리를
// 지킨다).
//
// `closestPointOnSegment` 를 **직접** 부르지는 않는다 — 그쪽 허용목록(M10)은 자리를 쓰는
// 둘로 죄여 있고, 이 모듈이 필요로 하는 것은 자리가 아니라 크기다. 한 단 위의 문을 지나므로
// 그 죔쇠는 한 글자도 넓어지지 않는다.
//
// **넘기는 점은 캔버스 단위다.** 빌려 쓰는 함수의 인자 이름은 `Px…` 이지만 그 산술에
// 화면 고유의 것은 한 줄도 없다(그 함수의 §인자 이름은 역사다). 두 자료형이 구조적으로
// 같아 타입이 공간 혼동을 잡아 주지 못하므로, 이 줄이 그 밝힘이다.
//
// **이 모듈은 DOM 도 React 도 모른다.** 점과 숫자뿐이라 jsdom 없이 전량 단위 시험된다.
//
// @spec SPEC-CANVAS-011 REQ-06 · AC-74 · AC-75 · AC-76 · AC-77

import { coordinate, type PointGeometry } from '../canvasConfig';
import type { CanvasPoint } from '../canvasGeometry';
import { HIT_TOLERANCE_PX, distanceToSegment } from '../canvasHitTest';
import { MAX_CONNECTOR_POINTS, type ConnectorRoute } from './connectorTypes';

// --- 상수 ---------------------------------------------------------------

/**
 * 간소화 허용 오차(**캔버스 단위**). 버려지는 점이 남은 선에서 벗어날 수 있는 최대 거리다.
 *
 * **집기 여유의 절반이라는 사실이 이 상수의 전부다.** 숫자를 새로 적지 않고 그 값에서
 * 파생시키는 것은 `ANCHOR_PICK_SLOP_PX` 가 두 기존 수의 합으로만 적히는 그 규율이며,
 * 근거는 다음 한 문장이다 — **간소화가 선을 손 밑에서 빼내면 안 된다.** 사용자는 자기가
 * 그은 자리에서 그 선을 다시 잡으려 하고, 잡히는 반경은 `HIT_TOLERANCE_PX` 다. 오차가
 * 그 반경을 넘으면 "분명히 여기 그었는데 잡히지 않는" 자리가 생기고, 절반으로 두면 그
 * 자리가 표현 불가능하다.
 *
 * **단위를 갈아 쓰는 것은 숨기지 않는다.** `HIT_TOLERANCE_PX` 는 화면 양이고(그 파일의
 * 머리말이 "집기 여유는 화면 양이다" 로 못박아 두었다) 이 값은 저장되는 기하의 양이다.
 * 두 공간을 잇는 것은 축척이며, 씨앗 캔버스(500×400)가 그만한 패널에 그려질 때 둘은
 * 1:1 에 가깝다. 축척이 1 이하로 작아지면(패널이 캔버스보다 작다) 화면에서의 벗어남은
 * 이 값보다 **더 작아지므로** 위 문장은 그대로 참이고, 확대해 들어간 자리에서는 그은 선도
 * 함께 확대되어 보인다. 저장값의 오차를 화면 축척에 매달지 않는 쪽을 고른 것은 그렇게
 * 두면 **같은 궤적이 패널 크기에 따라 다르게 저장되기** 때문이다.
 */
export const FREEHAND_TOLERANCE = HIT_TOLERANCE_PX / 2;

/**
 * 이 갈래가 **궤적을 받는가** (REQ-06).
 *
 * `free` 만 참이고 그것이 이 표의 전부다. `ROUTE_POINT_HOST` 와 묻는 것이 다르다 —
 * 저쪽은 "점이 들면 **어느 갈래가 그것을 드는가**"(직선은 꺾은 선으로 승격한다)이고 이쪽은 "끄는 동안
 * 점이 **저절로 나는가**" 다. 둘을 한 표로 접으면 꺾은 선을 그을 때도 궤적이 쌓여, 사람이
 * 찍은 적 없는 꺾임이 손을 떼는 순간 생긴다.
 *
 * 표로 적는 것이 요점이다. 갈래가 하나 늘면 컴파일러가 이 자리를 가리킨다 —
 * `route === 'free'` 로 적으면 다섯째 갈래가 궤적을 받는지 아무도 묻지 않은 채 지나간다.
 */
export const ROUTE_TRACES_TRAIL: Readonly<Record<ConnectorRoute, boolean>> = {
  straight: false,
  elbow: false,
  // 직각은 **손으로 긋는 갈래가 아니다**(SPEC-CANVAS-015). 궤적에서 점을 뽑는 일은 자유선
  // 하나의 몫이고, 직각은 두 끝에서 모서리를 **지어내는** 갈래다.
  ortho: false,
  curve: false,
  free: true,
};

// --- 궤적 받기 ------------------------------------------------------------

/**
 * 궤적에 표본 하나를 **받는다** — 받을 것이 없으면 **받은 배열을 그대로** 돌려준다.
 *
 * 받지 않는 경우 둘.
 *
 *   1. **상한에 닿았다**(REQ-06 · AC-76 · AC-77). 더 받지 않을 뿐 몸짓은 계속되고, 부르는
 *      쪽은 이 함수가 던지지 않으므로 아무것도 알 필요가 없다.
 *   2. **직전 표본과 같은 자리다.** 손이 멈춰 있어도 포인터 사건은 계속 오므로, 죄고 나면
 *      같은 정수 자리가 수십 번 들어온다. 버리는 점이 남은 점과 **정확히 같으므로** 이
 *      걸러냄은 오차를 한 톨도 만들지 않는다 — 간소화의 오차 보장이 이 함수 때문에
 *      흐려지지 않는다.
 *
 * 좌표를 여기서 **정수로 죈다.** 죄는 자는 파서의 그 함수(`coordinate`)이며, 이유는
 * `freeConnectorEnd` 가 끝점에 대해 적어 둔 그것과 같다(저술과 읽기가 다른 규칙으로 죄면
 * 저장 왕복에 값이 달라진다). 여기서 먼저 죄는 것은 위 2번이 **정수 자리의 같음**을 묻기
 * 때문이다 — 죄기 전에는 손 떨림 한 톨이 같은 자리를 다른 자리로 만든다.
 *
 * 값이 바뀌지 않았을 때 **같은 참조**를 돌려주는 것에도 뜻이 있다. 부르는 쪽이 그것을
 * 그대로 다시 들면 배열이 자라지 않고, 새 배열을 흘리는 자리도 생기지 않는다(AC-E4 의 규율).
 */
export function takeFreehandSample(
  trail: readonly PointGeometry[],
  at: CanvasPoint,
): readonly PointGeometry[] {
  if (trail.length >= MAX_CONNECTOR_POINTS) return trail;
  const point: PointGeometry = { x: coordinate(at.x, 0), y: coordinate(at.y, 0) };
  const last = trail[trail.length - 1];
  if (last !== undefined && last.x === point.x && last.y === point.y) return trail;
  return [...trail, point];
}

/**
 * 궤적을 **저장할 중간점**으로 옮긴다 — 손을 떼는 순간 한 번 불린다.
 *
 * 허용 오차를 여기서 묶는다. 부르는 쪽(오버레이)이 오차를 고르게 두면 "어떤 화면에서 그은
 * 선이 다른 화면의 선보다 거칠다" 가 표현 가능해지고, 그 차이는 config 에 남는다.
 *
 * ## 끝점과 겹치는 앞뒤 표본은 **중간점이 아니다**
 *
 * 손을 떼는 자리는 거의 언제나 마지막으로 움직인 자리다 — `pointerup` 은 대개 마지막
 * `pointermove` 와 같은 좌표로 온다. 그 표본을 그대로 중간점으로 적으면 **끝점 손잡이 밑에
 * 중간점 손잡이가 겹쳐** 서고(REQ-07 이 끝점과 중간점마다 손잡이를 세운다), 그 자리를 끄는
 * 손은 둘 중 어느 것을 잡았는지 알 수 없다. 길이 0 인 구간이 하나 붙는 것은 그리기에는
 * 보이지 않지만 편집에는 보이는 부류다.
 *
 * 걷어내는 문턱은 위 허용 오차 그대로다 — 새 눈금을 두지 않는다. 걷어낸 표본은 끝점에서
 * 오차 안에 있고 그 끝점은 선에 그대로 남으므로, "버려진 점은 전부 남은 선에서 오차 안"
 * 이라는 약속이 이 걷어냄 뒤에도 참이다.
 */
export function freehandPoints(
  trail: readonly PointGeometry[],
  from: CanvasPoint,
  to: CanvasPoint,
): PointGeometry[] {
  const points = simplifyTrail(trail, FREEHAND_TOLERANCE);
  const meets = (point: PointGeometry | undefined, end: CanvasPoint): boolean =>
    point !== undefined && Math.hypot(point.x - end.x, point.y - end.y) <= FREEHAND_TOLERANCE;

  let head = 0;
  while (head < points.length && meets(points[head], from)) head += 1;
  let tail = points.length;
  while (tail > head && meets(points[tail - 1], to)) tail -= 1;
  return points.slice(head, tail);
}

// --- 간소화 ---------------------------------------------------------------

/**
 * 점이 선분에서 벗어난 거리 — **히트 층의 그 자를 그대로 빌린다**(머리말 §점–선분 거리).
 *
 * **무한 직선이 아니라 선분까지의 거리다.** 고전적인 Ramer–Douglas–Peucker 는 직선까지를
 * 재지만, 궤적은 되짚어 올 수 있다 — 손이 오른쪽으로 갔다가 되돌아오면 중간 점의 **직선**
 * 거리는 작은데 **선분** 거리는 크다. 직선으로 재면 그런 점이 버려져 그은 적 없는 자리에
 * 선이 남고, 아래 함수가 약속하는 "버려진 점은 전부 허용 오차 안" 이 거짓이 된다. 빌려
 * 쓰는 자가 이미 선분까지를 재므로 그 함정은 이 모듈에서 표현 불가능하다.
 *
 * 넘기는 셋은 전부 **캔버스 단위**다(같은 절).
 */
function strayFromSegment(point: PointGeometry, a: PointGeometry, b: PointGeometry): number {
  return distanceToSegment({ x1: a.x, y1: a.y, x2: b.x, y2: b.y }, point);
}

/**
 * 허용 오차 안에서 점을 줄인다 — Ramer–Douglas–Peucker (REQ-06 · AC-75).
 *
 * **약속은 개수가 아니라 모양이다**: 버려진 점은 전부 남은 폴리라인에서 `tolerance` 이내에
 * 있다. 개수만 약속하면 "언제나 끝점 둘만 남긴다" 도 그 약속을 지키므로, 재는 쪽은 이
 * 기하 성질을 단언한다.
 *
 * 양 끝은 **언제나 남는다.** 궤적의 처음과 끝은 사용자가 누르고 뗀 자리이며, 그 둘을
 * 줄이면 선이 시작하거나 끝나는 자리가 달라진다.
 *
 * **재귀가 아니라 명시적인 스택이다.** 008 이 평탄화에서 적어 둔 그 경계다 — 재귀로 도는
 * 자리는 입력이 이상할 때 스택으로 드러나고, 그 드러남은 시험 시간 초과라는 가장 읽기
 * 어려운 형태로 온다. 상한이 있어 깊이가 크지는 않으나, 깊이에 기대지 않는 편이 싸다.
 */
export function simplifyTrail(
  points: readonly PointGeometry[],
  tolerance: number,
): PointGeometry[] {
  const copy = (point: PointGeometry): PointGeometry => ({ x: point.x, y: point.y });
  if (points.length <= 2) return points.map(copy);

  const keep = points.map(() => false);
  keep[0] = true;
  keep[points.length - 1] = true;

  const spans: { a: number; b: number }[] = [{ a: 0, b: points.length - 1 }];
  for (let span = spans.pop(); span !== undefined; span = spans.pop()) {
    const { a, b } = span;
    const head = points[a];
    const tail = points[b];
    if (head === undefined || tail === undefined || b <= a + 1) continue;

    // 가장 멀리 벗어난 점 하나를 찾는다. 허용 오차를 **넘는** 점이 없으면 이 구간은
    // 선분 하나로 충분하다 — `> tolerance` 이므로 정확히 오차만큼 벗어난 점은 버려지고,
    // 그래서 위 약속의 부등호가 `이내`(≤)로 남는다.
    let far = -1;
    let stray = tolerance;
    for (let i = a + 1; i < b; i += 1) {
      const point = points[i];
      if (point === undefined) continue;
      const distance = strayFromSegment(point, head, tail);
      if (distance > stray) {
        stray = distance;
        far = i;
      }
    }
    if (far < 0) continue;

    keep[far] = true;
    spans.push({ a, b: far }, { a: far, b });
  }

  return points.filter((_point, i) => keep[i] === true).map(copy);
}
