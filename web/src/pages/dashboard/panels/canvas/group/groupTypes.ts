// 그룹 노드의 자료형과 로컬 격자 (SPEC-CANVAS-004 M1).
//
// 004 가 최상위 배열에 더하는 것은 **노드 한 종류**다. 001~008 이 세운 `CanvasElement`
// 다섯 종은 한 글자도 바뀌지 않고, 그 위에 `GroupElement` 를 얹은 합집합이 새 이름
// `CanvasNode` 를 받는다.
//
// ## 종류 이름이 셋이 되는 이유 (가정 A22)
//
// `CanvasElementKind` 에 `'group'` 을 더하지 **않는다**. 더하면 출시된 가드 둘이 즉시
// 빨개진다 — `canvasElementKind.test.tsx` 의 `Record<CanvasElementKind, true>` 는 여섯째
// 키가 없어 컴파일에서 울고, `canvas007Regression.test.tsx` 는 `canvasConfig.ts` 의 소스
// 텍스트를 읽어 그 유니온의 원소가 정확히 둘임을 단언한다. 두 가드를 고치는 것은
// 회귀 방어망을 버리는 일이므로, 004 는 **셋째 이름을 더한다**:
//
//   - `CanvasPrimitiveKind` — 사용자가 고를 수 있는 **넷**(008 그대로).
//   - `CanvasElementKind`   — 요소가 가질 수 있는 **다섯**(008 그대로).
//   - `CanvasNodeKind`      — 최상위 노드가 가질 수 있는 **여섯**(004 가 더한다).
//
// SPEC-CANVAS-011 이 같은 근거로 같은 자리에 **일곱째**(연결선)를 더했다. 앞의 두 이름은
// 그때도 한 글자도 넓어지지 않았다 — 아래 `CanvasNode` · `CanvasNodeKind` 주석 참조.
//
// ## 그룹 안에 그룹은 오지 않는다 (가정 A6)
//
// `parts: CanvasElement[]` 이므로 그룹 중첩이 **타입으로** 불가능하다. 런타임 깊이 검사를
// 두지 않는 이유이며, 파서 형상(`parseNode` 가 `parseElement` 위 한 층에 선다)이 그것을
// 한 번 더 강제한다.
//
// **이 모듈은 DOM 을 모른다.** 순수 자료와 숫자뿐이다.
//
// @spec SPEC-CANVAS-004

import { PATH_LOCAL_EXTENT } from '../shapes/pathTypes';
import type { CustomAnchor } from '../connector/anchorTypes';
import type { ConnectorElement } from '../connector/connectorTypes';
import type {
  BoxGeometry,
  CanvasElement,
  CanvasElementKind,
  CanvasSize,
  ElementBinding,
  ElementStyle,
  RuleRow,
  TweenSpec,
} from '../canvasConfig';

/**
 * 부품 좌표가 사는 그룹 로컬 정수 격자. 그룹 상자 한 변이 이 값에 대응한다.
 *
 * **값을 베끼지 않고 008 의 경로 격자에서 파생시킨다**(가정 A14). 두 격자는 같은 개념
 * ("상자 하나의 로컬 정수 격자")이고, 값을 따로 적으면 언젠가 한쪽만 바뀐다. 유도해 두면
 * 갈라짐이 표현 불가능하다 — 그래서 이 줄에 숫자 리터럴이 **없는 것**이 요구다.
 *
 * 정규화 분수(0..1)를 기각한 근거는 008 이 이미 적었다(`shapes/pathTypes.ts`): 분수를
 * 들이면 `canvasConfig` 에 두 번째 수치 규율이 생기고, 부품 파싱이 001 의
 * `parseBoxGeometry`/`parseLineGeometry`/`parsePointGeometry` 를 그대로 지날 수 없게 된다.
 */
export const GROUP_LOCAL_EXTENT = PATH_LOCAL_EXTENT;

/**
 * 로컬 격자를 캔버스 크기 형상으로 적은 것. `localProjection` 의 `canvas` 축이다.
 *
 * 얼려 둔 것은 값이지 방어가 아니다 — 투영은 읽기만 한다.
 */
export const GROUP_LOCAL_SIZE: Readonly<CanvasSize> = Object.freeze({
  width: GROUP_LOCAL_EXTENT,
  height: GROUP_LOCAL_EXTENT,
});

/**
 * 출처 기록 — 이 그룹이 어느 카탈로그 심볼에서 나왔는가.
 *
 * **표시·감사용이며 렌더 경로는 읽지 않는다.** 결측이거나 모르는 값이어도 그림은
 * 완전하다(008 의 `PathElement.catalog_id` 와 같은 규율).
 */
export interface SymbolStamp {
  catalog_id: string;
  version: string;
}

/**
 * 그룹 노드 — 부품을 가진 하나의 덩어리.
 *
 * `geometry` 는 **저장된 값**이지 부품에서 매 프레임 파생되는 값이 아니다(가정 A16).
 * 파생 상자 위에서는 그룹 크기 조절이 정의되지 않는다 — 8핸들로 늘린 즉시 파생이 되받아
 * 원래 크기로 되돌아간다. 대가는 하나이고 숨기지 않는다: 부품을 상자 밖으로 밀어내면
 * 그림이 상자를 넘친다(경로 잉크가 이미 갖는 성질이다).
 *
 * 그룹 크기 조절은 **부품의 저장 좌표를 한 자리도 바꾸지 않는다**(가정 A17). 부품은
 * 투영에서 함께 늘어나며, 그 성질이 경로 명령과 정확히 같다(008 불변식 J2).
 */
export interface GroupElement {
  /** 최상위 배열 안에서 유일하다. */
  id: string;
  kind: 'group';
  /** **정수 캔버스 단위.** 그룹이 놓인 자리이자 크기 조절의 대상이다. */
  geometry: BoxGeometry;
  /**
   * 부품. 기하는 **그룹 로컬 정수 격자**(공칭 0..`GROUP_LOCAL_EXTENT`)이며 clamp 하지
   * 않는다 — 경로 로컬 좌표와 같은 규율이다.
   *
   * 타입이 `CanvasElement` 라 그룹은 여기 올 수 없다(A6 을 타입으로 강제).
   */
  parts: CanvasElement[];
  /** 부품이 물려받는 기본 바인딩. 없으면 상속 없음. */
  binding?: ElementBinding;
  /** 겉모습 캐스케이드 4층(최하) — 정적 저술. 001 과 같은 `ElementStyle` 이다. */
  style?: ElementStyle;
  /** 겉모습 캐스케이드 2층 — 동적 상태 판정. `binding` 이 있어야 평가된다. */
  rules?: RuleRow[];
  /** 부품이 물려받는 기본 트윈. 패널 기본보다 우선한다. */
  tween?: TweenSpec;
  /** 출처 기록. 렌더는 읽지 않는다. */
  symbol?: SymbolStamp;
  /**
   * 임의 앵커(SPEC-CANVAS-011 A10 · A11). 좌표는 **그룹 상자의 로컬 정수 격자**다.
   *
   * 011 A11 은 상자형을 `rect · ellipse · path · group` 넷으로 적었는데, 그 SPEC 본문은
   * "`CanvasElementBase` 에 더한다" 고만 적혀 있다. 그룹은 `CanvasElement` 가 아니므로
   * 그 한 줄로는 넷째가 빠진다 — 그래서 여기에도 **같은 이름의 같은 필드**를 둔다.
   * 그룹 크기 조절이 부품 저장 좌표를 바꾸지 않는 성질(A17)과 정확히 같은 성질이며,
   * 그래서 여기에도 크기 변경에 딸린 코드가 한 줄도 없다.
   */
  anchors?: CustomAnchor[];
}

/**
 * **윤곽 상자를 갖는** 노드 — 001~008 의 요소 다섯 종에 그룹 하나를 더한 것이다.
 *
 * 011 이 이 이름을 지었다. 그전까지 이것이 곧 `CanvasNode` 였는데, 연결선이 들어오면서
 * 둘이 갈렸다: 연결선에는 `geometry` 가 없어 상자를 낼 수 없다(`connectorTypes` 머리말).
 * 그래서 상자를 요구하는 통로(`outlineBox` · `handlePositions` · `anchorPoints`)는 넓어진
 * `CanvasNode` 가 아니라 **이 이름**을 받는다 — 그 자리에 연결선을 넘기는 것이 타입으로
 * 불가능해지고, "연결선에는 뜻 없는 상자" 라는 갈래를 지어 넣을 이유가 사라진다.
 */
export type OutlinedNode = CanvasElement | GroupElement;

/**
 * 최상위 배열의 원소. 상자를 가진 여섯에 **연결선**을 더한 것이다(SPEC-CANVAS-011 M4).
 *
 * 004 가 요소 다섯 위에 그룹을 얹었듯, 011 은 그 위에 연결선을 얹는다. `CanvasElement` 도
 * `CanvasElementKind` 도 한 글자도 넓어지지 않는다 — 그것이 출시된 가드 둘을 무수정으로
 * 살리는 값이다(`connectorTypes` 머리말).
 */
export type CanvasNode = OutlinedNode | ConnectorElement;

/** 상자를 가진 노드의 종류 **여섯**. 손잡이와 윤곽이 정의되는 범위가 이것이다. */
export type OutlinedNodeKind = CanvasElementKind | 'group';

/**
 * 최상위 노드가 가질 수 있는 **일곱**. 앞의 두 이름을 넓히지 않고 **더한 셋째**다(A22).
 *
 * `CanvasElementKind` 와 달리 이 이름은 `canvasConfig.ts` 바깥에 산다 — 그 파일의 소스
 * 텍스트를 읽는 출시된 가드가 그 유니온의 원소를 세고 있기 때문이다.
 *
 * 연결선의 이름을 리터럴로 적지 않고 **그 자료형에서 파생**시킨다. 그 문자열이 적히는
 * 자리는 `connectorTypes` 하나뿐이어야 한다(AC-39).
 */
export type CanvasNodeKind = OutlinedNodeKind | ConnectorElement['kind'];

/**
 * 이 노드가 그룹인가. **판별의 유일한 자리**다.
 *
 * 소비 측이 저마다 `node.kind === 'group'` 을 적으면 그 판정이 여럿이 되고, 그중 하나가
 * 다른 문자열을 적는 날 "어떤 화면에서는 그룹인데 어떤 화면에서는 아니다" 가 시작된다.
 */
export function isGroup(node: CanvasNode): node is GroupElement {
  return node.kind === 'group';
}
