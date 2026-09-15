// 프레임 상태의 키와 2단 순회 (SPEC-CANVAS-004 M3).
//
// 001/002 는 프레임 상태를 **평평한 `el.id`** 로 담는다. 중첩이 생기면 그 가정이 깨지므로
// 부품에는 복합 키(`그룹id/부품id`)가 필요하다. 부품 id 는 그룹 안에서만 유일하면 되고,
// 복합 키가 전역 유일성을 만든다.
//
// ## 키가 걸리는 표면은 **넷**이다
//
//   1. 목표 스타일 `Record<string, ResolvedStyle>` — `CanvasPanel.buildCanvasFrame`
//   2. 문구 `Record<string, string | undefined>` — 같은 함수
//   3. 트윈 장부 `Map<string, TweenState>` — `CanvasSurface.advance`
//   4. **글자 폭 장부 `Record<string, number>`** — `drawElement.drawElements` 의 **반환값**
//
// 0.2.0 은 셋만 적었다. 넷째는 002 가 더한 반환값이고, 빠뜨리면 그룹 안 문구 부품의 폭을
// 아무도 못 찾아 **히트 영역과 선택 윤곽이 조용히 폴백으로 내려앉는다** — 예외도 빈 화면도
// 아니고 "글자를 클릭하면 가끔 안 잡힌다" 로만 보고되는 부류다.
//
// **네 표면 모두 최상위 원소에 대해서는 이전과 같은 키를 낸다**(불변식 G11). 그래서
// 001/002 의 기존 시험이 그대로 회귀 게이트로 산다.
//
// **이 모듈은 DOM 을 모른다.** 순수 함수뿐이다.
//
// ## 만드는 자리와 푸는 자리는 **같은 파일**이다 (SPEC-CANVAS-009 M2)
//
// 009 가 선택 상태를 이 복합 키로 넓히면서 키를 **되읽는** 쪽이 생겼다. 그 분해를
// 소비 측에 맡기면(`key.split('/')`) 구분자 상수가 여럿이 되고, 그중 하나가 갈라지는 날
// 조용히 어긋난다. 그래서 역함수(`parseFrameKey`)와 판별(`isPartKey`)이 구분자 상수 바로
// 옆에 선다 — `groupTypes.isGroup` 이 "판별의 유일한 자리" 로 선 것과 같은 규율이다.
//
// @spec SPEC-CANVAS-004 REQ-03 · SPEC-CANVAS-009 REQ-01

import type { CanvasElement } from '../canvasConfig';
import { isGroup, type CanvasNode, type GroupElement } from './groupTypes';
import { isConnector } from '../connector/connectorTypes';

/** 복합 키의 구분자. 요소 id 에 쓰이지 않는 글자를 고른다(`nextElementId` 는 `el-N` 을 낸다). */
const PART_SEPARATOR = '/';

/**
 * 프레임 상태를 담는 키.
 *
 * **최상위 원소에서는 `nodeId` 그대로다** — 002 와 바이트 동일해야 기존 스위트가 회귀
 * 게이트로 남는다(G11). 부품에서만 `그룹id/부품id` 가 된다.
 */
export function frameKey(nodeId: string, partId?: string): string {
  return partId === undefined ? nodeId : `${nodeId}${PART_SEPARATOR}${partId}`;
}

/** `parseFrameKey` 가 내는 것 — 키를 다시 두 조각으로 편 결과. */
export interface ParsedFrameKey {
  nodeId: string;
  /** 최상위 키면 `undefined`. 부품 키에서만 값이 있다. */
  partId?: string;
}

/**
 * `frameKey` 의 **역함수**. 만드는 자리와 푸는 자리가 같은 파일에 있어야 구분자가
 * 갈라지지 않는다(SPEC-CANVAS-009 M2).
 *
 * **첫 구분자에서 한 번만 쪼갠다**(`indexOf`). 뒤에서 쪼개거나(`lastIndexOf`) 전부
 * 쪼개면(`split`) 부품 id 에 구분자가 들어 있는 순간 `nodeId` 가 오염되고, 그 오염은
 * 예외가 아니라 **"어떤 그룹만 선택이 안 된다"** 로만 보인다. `nextElementId` 는 `el-N` 을
 * 내므로 오늘의 최상위 id 에는 구분자가 없지만, 부품 id 는 카탈로그·가져오기가 짓는 이름
 * 이라 그 보장이 없다.
 */
export function parseFrameKey(key: string): ParsedFrameKey {
  const at = key.indexOf(PART_SEPARATOR);
  if (at < 0) return { nodeId: key };
  return { nodeId: key.slice(0, at), partId: key.slice(at + PART_SEPARATOR.length) };
}

/**
 * 이 키가 부품을 가리키는가. **판별의 유일한 자리**다.
 *
 * 소비 측이 저마다 `key.includes('/')` 를 적으면 그 판정이 여럿이 되고, 그중 하나가 다른
 * 구분자를 쓰는 날 "어떤 화면에서는 부품인데 어떤 화면에서는 아니다" 가 시작된다 —
 * `isGroup` 을 판별의 유일한 자리로 둔 것과 같은 이유다(`groupTypes.ts`).
 */
export function isPartKey(key: string): boolean {
  return key.includes(PART_SEPARATOR);
}

/** 2단 순회가 내는 한 항목 — 그릴 요소 하나와 그것이 프레임 상태에서 쓰는 키. */
export interface FrameDrawable {
  /** 프레임 상태의 키(`frameKey`). */
  key: string;
  /** 실제로 그려지는 요소. 그룹 자신은 그릴 도형이 없어 여기 오지 않는다. */
  element: CanvasElement;
  /** 부품이면 그것을 담은 그룹. 최상위 원소면 `undefined`. */
  group?: GroupElement;
}

/**
 * 최상위 배열과 그룹 부품을 **그리기 순서대로** 훑는다(REQ-03 의 2단 순서).
 *
 * (1) 최상위 배열 순서, (2) 그룹 안 부품 배열 순서. 부품이 그룹 밖 요소보다 위로 올라오는
 * 일은 없다 — 그룹 자리에서 부품이 **연달아** 나오고 그 뒤에 다음 최상위 원소가 온다.
 *
 * **그리는 쪽과 장부를 드는 쪽이 같은 순회를 쓰는 것**이 이 함수의 존재 이유다. 순회가
 * 둘이 되면 키 집합이 갈라지고, 그 어긋남은 "어떤 부품만 트윈되지 않는다" 로만 보인다.
 */
export function* walkDrawables(nodes: readonly CanvasNode[]): Generator<FrameDrawable> {
  for (const node of nodes) {
    // **연결선은 아직 나오지 않는다**(SPEC-CANVAS-011 M6 가 이 줄을 걷어낸다).
    //
    // 011 M4 는 자료형과 파서까지만 세운다. 이 순회가 내는 것은 `CanvasElement` 이고
    // 연결선은 그 합집합에 들지 않으므로, 여기서 억지로 내보내려면 뜻 없는 요소 형상을
    // 지어야 한다 — 그 형상은 `drawElement` 가 그리려 드는 순간 `geometry` 를 찾다가
    // 조용히 아무것도 그리지 않는 도형이 된다. 그래서 **지금은 건너뛰고**, 순회가 낼 수
    // 있는 항목의 형상을 M6 이 함께 넓힌다(그리는 법이 있어야 낼 뜻이 생긴다).
    //
    // 순회는 그때도 **하나**다. 연결선을 위한 두 번째 순회를 만들면 키 집합이 갈라지고,
    // 그 어긋남은 "어떤 선만 트윈되지 않는다" 로만 보인다.
    if (isConnector(node)) continue;
    if (isGroup(node)) {
      for (const part of node.parts) {
        yield { key: frameKey(node.id, part.id), element: part, group: node };
      }
      continue;
    }
    yield { key: frameKey(node.id), element: node };
  }
}
