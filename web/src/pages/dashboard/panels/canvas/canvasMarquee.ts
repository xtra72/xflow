// 영역 선택(마키) — 순수 기하 + 집합 산술.
//
// `canvasHitTest` 의 이웃이다. 그 모듈이 **한 점**에 무엇이 걸리는지 답하면 이 모듈은
// **한 사각형**에 무엇이 통째로 드는지 답한다. 둘을 한 파일에 두지 않은 이유는 판정의
// 축이 다르기 때문이다 — 점 판정은 "잡을 수 있는가" 라 집기 여유를 더해 **부풀리고**,
// 사각 판정은 "완전히 들었는가" 라 여유를 더하면 **경계에 걸친 것까지 끌려온다**. 같은
// 파일에 두면 `HIT_TOLERANCE_PX` 가 두 뜻으로 읽힌다.
//
// ## 왜 잉크가 아니라 **윤곽선 상자**인가
//
// 판정 대상은 `CanvasEditOverlay.outlineBox` 가 낸 상자, 곧 **선택 윤곽선이 두르는 바로
// 그 상자**다. 근거 둘.
//
//   1. **화면이 약속을 지킨다.** 사용자가 사각형으로 감싸는 것은 눈에 보이는 그 점선
//      테두리이고, 골라진 뒤에 뜨는 것도 같은 테두리다. 판정을 잉크로 두면 "테두리는
//      분명히 안에 들었는데 골라지지 않는" 경우가 생기고, 그 어긋남은 화면에서만 드러난다.
//   2. **비용이 점 판정의 예산을 넘지 않는다.** `canvasHitTest` 는 제 비용을
//      "**포인터 사건당 1회** 선형" 으로 못박아 두었다(가정 A10). 마키는 그와 달리
//      **손이 움직이는 내내** 다시 판정하므로, 경로마다 `flattenPath` 를 돌리면 그 예산이
//      한 자릿수 곱해진다. 상자 판정은 요소당 비교 넷이다.
//
// **대가는 숨기지 않는다**: 별처럼 오목한 경로는 제 잉크보다 넓은 상자를 가지므로, 잉크가
// 전부 사각형 안에 들어도 상자 모서리가 밖이면 골라지지 않는다. 방향은 **언제나 이쪽**
// 이다 — 상자가 들었으면 잉크도 들었다(잉크는 제 상자 안이다). 그래서 이 판정이 틀리는
// 쪽은 "덜 고른다" 이지 "엉뚱한 것을 고른다" 가 아니며, 덜 고른 것은 Shift 로 한 번 더
// 집으면 되지만 엉뚱하게 골라진 것은 사용자가 알아차리지 못한 채 함께 끌려간다.
//
// ## 왜 **완전히 든 것**만인가 (겹침이 아니라 포함)
//
// 겹침으로 두면 사각형이 스치기만 한 것까지 전부 딸려 온다. 캔버스가 빽빽할수록 그
// 차이가 커지고, 사용자에게는 **빼는 수단이 없다** — 큰 이웃이 걸친 자리를 피해 갈 길이
// 없기 때문이다. 포함으로 두면 "사각형 안에 온전히 보이는 것" 이라는 한 문장이 화면과
// 일치하고, 부족하면 Shift 로 더 집으면 된다(더하는 길은 열려 있고 빼는 길은 없다).
//
// ## DOM 무의존 · React 무의존
//
// `canvasHitTest` 와 같은 규율이다. 투영은 **인자로 받는다**(`boxOf`) — 이 모듈이
// `outlineBox` 를 직접 부르면 오버레이와 순환 참조가 되고, 제 투영을 새로 지으면 그것이
// 곧 두 번째 측정원이다(SPEC-CANVAS-002 위험 R1).
//
// @spec SPEC-CANVAS-002 REQ-06 · SPEC-CANVAS-009

import type { PanelSelection } from '../charts/panelEditSelection';
import type { PxBox, PxPoint } from './canvasGeometry';
import { isGroup, type CanvasNode, type OutlinedNode } from './group/groupTypes';
import { isConnector } from './connector/connectorTypes';

// --- 타입 ---------------------------------------------------------------

/**
 * 사각형 판정에 참여하는 요소 하나.
 *
 * 키가 `nodeId` 인 것이 이 모듈의 계약 그 자체다 — 그룹은 부품이 아니라 **제 상자**로
 * 판정되고 골라지는 것도 그룹이다(아래 `marqueeCandidates`).
 */
export interface MarqueeCandidate {
  nodeId: string;
  box: PxBox;
}

// --- 기하 ---------------------------------------------------------------

/**
 * 두 점이 만드는 사각형. **어느 방향으로 끌어도 같은 사각형이다.**
 *
 * 정규화를 여기 한 곳에 두는 이유: 오른쪽 아래로만 끄는 것은 몸짓의 네 방향 중 하나일
 * 뿐인데, 정규화를 빠뜨리면 나머지 셋에서 `w`·`h` 가 음수가 되어 아래 `enclosesBox` 의
 * 비교가 **전부 거짓**이 된다. 그 결함은 "위로 끌면 아무것도 안 골라진다" 로만 보고된다.
 */
export function marqueeRect(a: PxPoint, b: PxPoint): PxBox {
  return {
    x: Math.min(a.x, b.x),
    y: Math.min(a.y, b.y),
    w: Math.abs(b.x - a.x),
    h: Math.abs(b.y - a.y),
  };
}

/**
 * 안쪽 상자가 바깥 상자에 **완전히** 드는가. 변이 닿는 것은 든 것으로 본다.
 *
 * **비유한 좌표는 거짓으로 떨어진다** — `NaN` 과의 비교는 모두 거짓이기 때문이다. 그래서
 * 좌표가 손상되면 이 판정은 "아무것도 고르지 않는다" 로 퇴화하지, "전부 고른다" 로
 * 퇴화하지 않는다. 방어의 방향이 안전한 쪽이므로 닿지 않는 가드를 따로 세우지 않는다
 * (세우면 검증되지 않은 채 남아 읽는 사람에게 거짓말한다 — 이 저장소의 규율이다).
 *
 * 두 상자 모두 **양수 범위로 정규화되어 있다고 본다**. 바깥은 위 `marqueeRect` 가,
 * 안쪽은 `outlineBox` 의 `normalizeBox` 가 각자 보장한다.
 */
export function enclosesBox(outer: PxBox, inner: PxBox): boolean {
  return (
    inner.x >= outer.x &&
    inner.y >= outer.y &&
    inner.x + inner.w <= outer.x + outer.w &&
    inner.y + inner.h <= outer.y + outer.h
  );
}

// --- 후보 고르기 --------------------------------------------------------

/**
 * 판정에 참여할 요소들. **`hitTest` 가 건너뛰는 것을 그대로 건너뛴다.**
 *
 * 규칙을 베끼지 않고 **같은 문장**으로 적는다: 최상위 원소는 `style.visible === false` 면
 * 빠지고(보이지 않는 것을 잡을 수 없다), **그룹은 제 `style.visible` 을 보지 않는다** —
 * `canvasHitTest.hitsGroup` 이 같은 결정을 이미 적어 두었고(M3 의 `drawElements` 도 보지
 * 않는다), 여기서만 보면 "그려지는데 골라지지 않는" 어긋남이 이 경로에만 생긴다.
 *
 * 그룹이 **부품이 아니라 제 상자로** 판정되는 것이 점 판정과 갈리는 유일한 자리이며,
 * 그것은 이 모듈이 잉크가 아니라 윤곽선 상자를 본다는 결정의 당연한 따름이다 — 그룹의
 * 윤곽선은 `outlineBox` 가 이미 **저장된 제 상자**로 정의해 두었다(가정 A16). 부품을
 * 따로 고를 수는 없다: 선택 키는 언제나 `nodeId` 하나다(REQ-06).
 *
 * @param boxOf 요소 → 스테이지 로컬 px 윤곽선 상자. 투영을 든 쪽이 넘긴다.
 */
export function marqueeCandidates(
  elements: readonly CanvasNode[],
  boxOf: (el: OutlinedNode) => PxBox,
): MarqueeCandidate[] {
  const out: MarqueeCandidate[] = [];
  for (const node of elements) {
    // **연결선은 아직 사각형에 들지 않는다**(SPEC-CANVAS-011 M9).
    //
    // 이 모듈의 판정은 `outlineBox` 가 낸 상자인데 연결선에는 그 상자가 없다 — 그래서
    // `boxOf` 의 인자도 `OutlinedNode` 다(타입이 "여기 올 수 없다" 고 말한다). 두 끝을
    // 감싸는 상자를 지어 재면 잉크가 사각형 밖으로 한참 나간 선까지 딸려 오고, 그것은
    // 이 모듈이 "덜 고르는 쪽으로만 틀린다" 고 못박아 둔 방향을 **뒤집는다**.
    if (isConnector(node)) continue;
    if (!isGroup(node) && node.style.visible === false) continue;
    out.push({ nodeId: node.id, box: boxOf(node) });
  }
  return out;
}

// --- 집합 산술 ----------------------------------------------------------

/**
 * 사각형이 낸 다음 선택 — **바탕과의 합집합**이다.
 *
 * `base` 는 몸짓을 **시작할 때**의 선택이고 매 이동마다 그 값에서 다시 센다. 직전 결과에
 * 얹으면 사각형을 줄여도 한 번 들어온 것이 빠지지 않아, 사용자가 되돌릴 수 없는 선택이
 * 쌓인다(늘릴 때만 맞고 줄일 때 틀리는 부류의 결함이다).
 *
 * **`nextSelection` 을 요소마다 부르지 않는다.** 그 함수의 `additive` 는 **토글**이라
 * (있으면 뺀다) 이미 골라 둔 것을 사각형이 덮는 순간 **선택이 풀린다** — 감싼 것이
 * 풀리는 마키는 사용자가 이해할 수 없는 그림이다. 같은 modifier 가 한 요소에서는
 * "더하거나 뺀다" 이고 집합에서는 "합친다" 인 것에 모순은 없다: 토글은 원소 하나에만
 * 정의되는 연산이고, 사각형이 말하는 것은 언제나 **집합**이다.
 *
 * 더한 것이 하나도 없으면 **`base` 를 그대로** 돌려준다 — 새 `Set` 을 돌려주면 값이 같은데도
 * React 가 매 이동마다 다시 그린다(`nextSelection` 이 같은 규율을 쓴다).
 */
export function marqueeSelection(
  candidates: readonly MarqueeCandidate[],
  rect: PxBox,
  base: PanelSelection<string>,
): PanelSelection<string> {
  const next = new Set(base);
  let added = false;
  for (const candidate of candidates) {
    if (!enclosesBox(rect, candidate.box)) continue;
    if (next.has(candidate.nodeId)) continue;
    next.add(candidate.nodeId);
    added = true;
  }
  return added ? next : base;
}
