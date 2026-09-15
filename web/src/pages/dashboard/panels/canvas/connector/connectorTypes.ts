// 연결선의 자료형과 그 판별 (SPEC-CANVAS-011 M4).
//
// ## 여섯 번째 종류를 **어디에** 더하는가
//
// `CanvasElement` 에 여섯째 `kind` 를 더하지 **않는다**. 요소는 `geometry: Geometry` 를
// 반드시 갖는데 연결선에는 그 자리에 넣을 값이 없다 — 끝점이 좌표가 아니라 **참조**이기
// 때문이다. 억지로 상자를 지어 넣으면 `patchNodeGeometry` · `handlePositions` ·
// `outlineBox` 전부가 연결선에 대한 뜻 없는 갈래를 하나씩 갖게 되고, 출시된 가드 둘이
// 곧바로 운다(`canvasElementKind.test.tsx` 의 `Record<CanvasElementKind, true>` 와
// `canvas007Regression.test.tsx` 의 소스 텍스트 판정).
//
// 그래서 004 의 선례 그대로 **최상위 노드**에 더한다:
//
//   - `CanvasPrimitiveKind` — 사용자가 고를 수 있는 **넷**(008 그대로).
//   - `CanvasElementKind`   — 요소가 가질 수 있는 **다섯**(008 그대로 — 011 도 건드리지 않는다).
//   - `CanvasNodeKind`      — 최상위 노드가 가질 수 있는 **일곱**(004 의 여섯 + 연결선).
//
// 대가도 004 와 같고 숨기지 않는다: `CanvasNode` 를 훑는 자리마다 "연결선은 건너뛴다" 가
// 한 줄씩 붙는다. 그 한 줄은 전부 아래 `isConnector` **한 함수**를 지난다.
//
// ## 왜 `CanvasElement` 가 아니라 여기인가 (모듈 자리)
//
// `canvasConfig` 가 `ConnectorElement` 를 노드 합집합의 갈래로 들어야 하는데, 이 파일은
// `canvasConfig` 의 **자료형만** 들이므로 고리가 생기지 않는다(전부 `import type` 이라
// 실행 시각에는 아무것도 남지 않는다). `anchorTypes` 가 같은 이유로 잎 모듈이 되었고,
// 이 파일도 같은 규율이다 — 앵커 자리 이름(`AnchorId`)을 `connector/anchors.ts` 에서
// 들이지 않는 것이 그 규율의 값이다(그쪽은 투영과 윤곽 상자를 지나 `canvasConfig` 에
// 도로 닿는다).
//
// **이 모듈은 DOM 도 산술도 모른다.** 자료형과 판별뿐이다.
//
// @spec SPEC-CANVAS-011 REQ-03 · REQ-04

import type {
  ElementBinding,
  ElementStyle,
  PointGeometry,
  RuleRow,
  TweenSpec,
} from '../canvasConfig';
import type { CanvasNode } from '../group/groupTypes';

/**
 * 연결선 노드의 `kind`. **이 문자열이 적히는 자리는 이 파일뿐이다**(AC-39).
 *
 * 값을 상수로 둔 것에 뜻이 있다 — 소비 측이 저마다 `node.kind === 'connector'` 를 적으면
 * 그 판정이 여럿이 되고, 그중 하나가 다른 문자열을 적는 날 "어떤 화면에서는 연결선인데
 * 어떤 화면에서는 아니다" 가 시작된다. `anchors.ts` 의 `ANCHOR_CENTER_ID` 가 자리 이름
 * 하나에 대해 쓴 그 규율과 같다.
 */
export const CONNECTOR_KIND = 'connector';

/**
 * 연결선의 한 끝 — 요소에 **붙었거나**(참조) 캔버스에 **떠 있거나**(자유).
 *
 * 붙은 끝이 좌표가 아니라 참조인 것이 011 의 전부다. 좌표로 적어 두면 참조된 도형을
 * 옮기는 순간 선이 옛 자리에 남고, 그 어긋남은 예외도 경고도 없이 **화면에서만** 드러난다.
 *
 * `a` 가 `string` 인 것은 고정 아홉(`FixedAnchorId`)과 임의 앵커 id 가 한 공간을 나눠 쓰기
 * 때문이다(`anchors.anchorPoints` 가 낸 지도의 키). 그 이름을 타입으로 들이면 이 잎 모듈이
 * `anchors.ts` 를 지나 `canvasConfig` 로 도로 닿는다 — 자리 이름의 뜻은 그 지도가 들고,
 * 여기서는 **한 자리를 가리키는 이름**이라는 사실만 남긴다.
 */
export type ConnectorEnd =
  | { el: string; a: string }
  | { x: number; y: number };

/**
 * 그리는 법 넷. **`route` 는 그리기만 가른다** — 자료형은 넷이 같다(§`route` 는 그리기만
 * 가른다).
 *
 * `elbow` 와 `free` 는 그리는 법이 같고 다른 것은 **점이 어디서 왔는가** 뿐이다: 사람이
 * 하나씩 찍었는가, 손이 그은 궤적에서 나왔는가. 그래서 둘을 한 이름으로 접지 않는다 —
 * 접으면 도구가 제 출처를 잃고, 자유선을 다시 편집하는 몸짓이 꺾은 선과 구분되지 않는다.
 */
export type ConnectorRoute = 'straight' | 'elbow' | 'curve' | 'free';

/** 모르는 `route` 가 떨어지는 자리(AC-37). 점이 없어도 그림이 완전한 유일한 갈래다. */
export const DEFAULT_CONNECTOR_ROUTE: ConnectorRoute = 'straight';

/**
 * 이 갈래가 **중간점을 들 수 있는가** (SPEC-CANVAS-011 M10 · REQ-04 · REQ-05).
 *
 * `straight` 만 거짓이고, 그것이 이 표의 전부다. 위 §`route` 표가 "직선은 중간점이 없다" 로
 * 적은 것은 관례가 아니라 **그리기의 전제**다 — `connectorPath` 는 `curve` 가 아닌 갈래를
 * 점 목록 그대로 이어 그리므로, 직선에 점이 하나라도 들면 그 선은 폴리라인으로 그려지고
 * REQ-04 의 "두 끝을 직선으로 이어야 한다" 가 그 순간 거짓이 된다. 막는 자리를 하나 두지
 * 않으면 그 거짓은 저술에서 조용히 만들어진다.
 *
 * `free` 가 참인 것에도 뜻이 있다. 그 갈래의 점은 손이 그은 궤적에서 나지만(M11) 난 뒤에는
 * `elbow` 의 점과 **구별할 이유가 없다** — 그리는 법이 같고(위 §`route`), 그러므로 고치는
 * 법도 같아야 한다. 출처가 다르다는 이유로 편집을 막으면 자유선은 그은 순간 굳는다.
 *
 * 표로 적는 것이 요점이다. 갈래가 하나 늘면 컴파일러가 이 자리를 가리킨다 —
 * `route !== 'straight'` 로 적으면 다섯째 갈래가 **조용히** 점을 들게 되고, 그 갈래가
 * 점을 들 수 있는지 아무도 묻지 않은 채 지나간다.
 */
export const ROUTE_TAKES_POINTS: Readonly<Record<ConnectorRoute, boolean>> = {
  straight: false,
  elbow: true,
  curve: true,
  free: true,
};

/**
 * 연결선 노드 — 두 자리를 잇는 하나의 선.
 *
 * **`geometry` 가 없는 것이 이 자료형의 절반이다.** 연결선에는 늘릴 상자가 없고(REQ-07),
 * 그래서 8핸들 · 정렬 · 윤곽 상자가 이것에 걸리지 않는다. 상자를 지어 씌우면 그 상자를
 * 늘렸을 때 무엇이 일어나는지 화면이 답하지 못한다.
 *
 * ## 중간점이 **절대 좌표**인 이유
 *
 * 008 의 경로 명령은 제 상자 로컬 격자에 살고 004 의 부품도 그룹 로컬 격자에 사는데,
 * 연결선만 절대 좌표인 것은 **담을 상자가 없기 때문이다.** 두 끝은 서로 다른 두 요소에
 * 붙어 있고 그 둘을 감싸는 상자는 요소가 움직일 때마다 바뀐다 — 그런 상자를 기준으로
 * 삼으면 한쪽 도형을 옮길 때 사용자가 찍어 둔 꺾임이 제멋대로 미끄러진다.
 *
 * 대가는 하나이고 숨기지 않는다: **도형을 옮겨도 중간점은 그 자리에 남는다.** 선의 양 끝만
 * 따라가고 허리는 제자리다.
 */
export interface ConnectorElement {
  /** 최상위 배열 안에서 유일하다. 요소 · 그룹과 **같은 id 공간**을 나눠 쓴다. */
  id: string;
  kind: typeof CONNECTOR_KIND;
  /** 시작 끝. 없으면 노드 자체가 성립하지 않는다(AC-38). */
  from: ConnectorEnd;
  /** 끝나는 끝. 같은 규율이다. */
  to: ConnectorEnd;
  route: ConnectorRoute;
  /** 중간점. **캔버스 단위 절대 정수.** `straight` 에서는 비어 있다. */
  points?: PointGeometry[];
  /** 겉모습 캐스케이드 최하층 — 정적 저술. 001 과 같은 `ElementStyle` 이다. */
  style?: ElementStyle;
  /** 없으면 정적 선(규칙 평가 대상이 아니다). */
  binding?: ElementBinding;
  /** 위에서부터 첫 일치 승리. `binding` 이 있어야 평가된다. */
  rules?: RuleRow[];
  /** 패널 기본 트윈을 덮어쓴다. */
  tween?: TweenSpec;
}

/**
 * 이 노드가 연결선인가. **판별의 유일한 자리**다(AC-39).
 *
 * `isGroup` 과 같은 규율이며, 그 함수와 달리 비교하는 문자열을 상수로 둔다 — 011 은 이
 * 판별이 흩어지지 않는 것을 수락 기준으로 적었고(AC-39), 상수로 두면 그 흩어짐이 문법으로
 * 드러난다.
 */
export function isConnector(node: CanvasNode): node is ConnectorElement {
  return node.kind === CONNECTOR_KIND;
}

/**
 * **날것의** `kind` 값이 연결선을 가리키는가 — 파서가 갈라지는 그 한 줄만 부른다.
 *
 * 파싱 **전에는** 노드가 아니므로 위 `isConnector` 를 쓸 수 없다. 그렇다고 파서가 제 손으로
 * 문자열을 적으면 판별이 둘이 되므로, 같은 상수를 지나는 문 하나를 여기 둔다 — 두 함수가
 * 한 상수를 보므로 갈라짐이 표현 불가능하다.
 */
export function isConnectorKind(kind: unknown): kind is typeof CONNECTOR_KIND {
  return kind === CONNECTOR_KIND;
}
