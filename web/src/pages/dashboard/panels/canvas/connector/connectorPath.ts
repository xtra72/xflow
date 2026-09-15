// 연결선의 **화면 위 모양** — 점 목록과 `route` 에서 경로 명령 하나를 낸다
// (SPEC-CANVAS-011 M6 · M7).
//
// ## 왜 명령 목록인가
//
// 그리는 쪽(M6)과 잡는 쪽(M7)이 **같은 곡선**을 봐야 한다. 둘이 저마다 "곡선이면
// `curveSegments`, 아니면 폴리라인" 을 적으면 그 갈래가 두 벌이 되고, 한쪽만 고쳐지는 날
// "그려진 선과 잡히는 선이 다르다" 가 시작된다 — 002 가 위험 R1 로 이름 적어 둔 그 결함이며,
// 예외도 경고도 없이 **화면에서만** 드러난다. M5 가 참조 푸는 자리를 하나로 못박은 것과 같은
// 규율을 **모양**에 대해 한 번 더 적용한 것이 이 모듈이다.
//
// 명령 목록을 고른 것은 그것이 **두 소비자가 모두 읽을 수 있는 유일한 형상**이기 때문이다.
// 그리는 쪽은 명령마다 대응하는 context 호출을 내고(`M`→`moveTo` · `L`→`lineTo` ·
// `C`→`bezierCurveTo`), 잡는 쪽은 008 의 `flattenPath` 에 그대로 넘긴다. 008 이 경로를 위해
// 지은 평탄화를 011 이 **두 번째 호출자**로 쓰는 자리가 바로 여기이며, 그래서 베지어 거리
// 산술이 새로 적히지 않는다(AC-55).
//
// ## `Z` 가 없는 것이 **타입에** 적혀 있다
//
// 연결선은 두 자리를 잇는 **열린** 선이다. 닫히지 않으므로 안쪽이라는 개념이 없고, 그래서
// 잡는 쪽도 내부 판정(`isInsidePath`)을 부르지 않는다. 그 사실을 산문이 아니라 반환 타입에
// 적어 두면 닫힘 명령을 더하는 일이 문법으로 막힌다.
//
// ## 투영이 여기 **한 번** 있다
//
// 점 목록은 캔버스 단위로 들어오고(M5 의 `resolveConnector`), 나가는 명령은 스테이지 로컬
// px 다. 곡선을 **투영한 뒤에** 펴는 것이 M6 이 고른 순서이며(투영은 축마다 상수를 곱하는
// 아핀 변환이라 어느 쪽에서 펴도 같은 곡선이지만, 이쪽이면 AC-48 이 재는 제어점이 곧 그려진
// 좌표다), 두 소비자가 이 함수를 지나므로 그 순서가 두 벌이 되지 않는다.
//
// **이 모듈은 DOM 을 모른다.** 점과 숫자뿐이라 jsdom 없이 전량 단위 시험된다.
//
// @spec SPEC-CANVAS-011 REQ-04 · REQ-07-b · AC-55

import {
  projectPoint,
  type CanvasPoint,
  type CanvasProjection,
  type PxPathCommand,
} from '../canvasGeometry';
import { curveSegments } from './connectorCurve';
import type { ConnectorRoute } from './connectorTypes';

/**
 * 연결선이 낼 수 있는 경로 명령 — `Z` 를 **뺀** 008 의 어휘 그대로다.
 *
 * 008 의 `PxPathCommand` 를 좁혀서 쓰는 것이지 새 어휘를 짓는 것이 아니다. 좁힌 배열은
 * 넓은 배열이 필요한 자리(`flattenPath`)에 그대로 들어가므로 변환이 끼어들지 않는다.
 */
export type ConnectorPathCommand = Exclude<PxPathCommand, { c: 'Z' }>;

/**
 * 해석된 점 목록(`[시작, …중간점, 끝]`, **캔버스 단위**)을 스테이지 로컬 px 경로 명령으로 편다.
 *
 * - 점이 하나도 없으면 **빈 목록**이다. 그리는 쪽은 그때 아무 호출도 내지 않고, 잡는 쪽은
 *   아무것도 잡지 않는다 — 없는 선을 그릴 수도 잡을 수도 없다.
 * - `straight` · `elbow` · `free` 는 셋이 **같은 길**을 지난다. 다른 것은 점이 어디서
 *   왔는가 뿐이며(사람이 찍었는가, 손이 그은 궤적인가) 그 출처는 모양에 닿지 않는다.
 * - `curve` 만 갈라지되, 중간점이 없으면 `curveSegments` 가 빈 목록을 내므로 **그 갈래도
 *   같은 길로 떨어진다**(AC-50). 네 갈래가 같아야 한다는 REQ-04-b 가 조건문이 아니라
 *   **구조**로 지켜지는 자리다.
 */
export function connectorPath(
  points: readonly CanvasPoint[],
  route: ConnectorRoute,
  proj: CanvasProjection,
): readonly ConnectorPathCommand[] {
  const px = points.map((point) => projectPoint(point, proj));
  const start = px[0];
  // `resolveConnector` 는 늘 점을 둘 이상 내지만 그 사실은 타입에 없다. 손으로 지은 목록이
  // 들어와도 던지지 않는 쪽을 고른다 — 빈 목록에서 `M` 을 지어내면 없는 자리에 선이 선다.
  if (start === undefined) return [];

  const out: ConnectorPathCommand[] = [{ c: 'M', x: start.x, y: start.y }];
  const segments = route === 'curve' ? curveSegments(px) : [];
  if (segments.length === 0) {
    for (const point of px.slice(1)) out.push({ c: 'L', x: point.x, y: point.y });
  } else {
    for (const seg of segments) {
      out.push({
        c: 'C',
        x1: seg.c1.x,
        y1: seg.c1.y,
        x2: seg.c2.x,
        y2: seg.c2.y,
        x: seg.to.x,
        y: seg.to.y,
      });
    }
  }
  return out;
}
