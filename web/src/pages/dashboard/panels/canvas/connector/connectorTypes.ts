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
// **이 모듈은 DOM 도 산술도 모른다.** 자료형과 판별과 상수뿐이다 — 상한 하나가 여기
// 사는 것은 008 이 `MAX_PATH_COMMANDS` 를 `shapes/pathTypes` 에 둔 그 자리 규율 그대로다
// (어휘를 적은 파일이 그 어휘의 상한도 든다).
//
// @spec SPEC-CANVAS-011 REQ-03 · REQ-04 · REQ-06

import type {
  ElementBinding,
  ElementStyle,
  PointGeometry,
  RuleRow,
  TweenSpec,
} from '../canvasConfig';
import type { CanvasNode } from '../group/groupTypes';
// SPEC-CANVAS-011 M11 — 중간점 상한을 008 의 명령 상한에서 **파생**시킨다(아래
// `MAX_CONNECTOR_POINTS`). `shapes/pathTypes` 는 아무것도 들이지 않는 잎이라 이 값
// 하나를 들여도 고리가 생기지 않는다 — `anchorTypes` 가 격자에서 쓴 그 형상이다.
import { MAX_PATH_COMMANDS } from '../shapes/pathTypes';

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
 * 한 연결선이 들 수 있는 **중간점 수의 상한** (SPEC-CANVAS-011 REQ-06).
 *
 * **값을 베끼지 않고 008 의 명령 상한에서 파생시킨다.** `ANCHOR_LOCAL_EXTENT` 가 격자에서
 * 쓴 그 근거를 그대로 쓴다 — 둘은 같은 개념("한 도형이 들 수 있는 점의 수")이고, 값을
 * 따로 적으면 언젠가 한쪽만 바뀐다. 그래서 이 줄에 숫자 리터럴이 **없는 것**이 요구다.
 *
 * **이 한 값이 저술과 파싱 양쪽에 선다.** 받는 쪽(자유선 궤적 — `connector/freehand.ts`)과
 * 읽는 쪽(`canvasConfig.parseConnectorPoints`)이 같은 상수를 보므로 두 수가 갈릴 수 없다.
 * 갈리면 저술에서는 통과한 궤적이 저장 왕복에서 조용히 잘리고, 그 어긋남은 다음에 파일을
 * 읽는 순간에야 **화면에서만** 드러난다 — 011 이 가장 싫어하는 부류다.
 */
export const MAX_CONNECTOR_POINTS = MAX_PATH_COMMANDS;

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
 * 이 갈래의 **중간점이 사는 갈래** (SPEC-CANVAS-011 M10 · REQ-04 · REQ-05).
 *
 * ## 무엇을 묻는 표인가
 *
 * "이 갈래가 점을 들 수 있는가" 가 아니라 **"이 갈래에 점이 들면 그 선은 무엇이 되는가"**
 * 를 묻는다. 셋은 제 이름이 답이고(항등), `straight` 만 `elbow` 를 가리킨다.
 *
 * 전신은 `ROUTE_TAKES_POINTS` 라는 불리언 표였고, 그 표가 답한 것은 거절이었다 — 직선
 * 위의 더블클릭은 **아무 말 없이** 아무 일도 하지 않았다. 직선은 도구 넷 가운데 **첫째**라
 * 사람이 가장 먼저 긋는 선이고, 그래서 011 이 가장 먼저 배달한 것이 "꺾이지 않는데 왜
 * 안 되는지도 말하지 않는 선" 이었다(사용자 신고 2026-09-16).
 *
 * 고침은 거절을 **말하게** 하는 것이 아니라 없애는 것이다. 거절을 말하면 화면은 "그 일을
 * 하려면 도구를 다시 골라 오라" 고 시키는데, 사용자는 방금 그 일을 하겠다고 말했다.
 * 승격은 그 말을 그대로 듣는다 — `route` 는 사용자가 저술한 값이고 그것을 바꾸는 것도
 * 정당한 편집이며, 눌린 자리에서 선이 꺾이는 것이 곧 그 몸짓이 요청한 결과다.
 *
 * ## REQ-04 는 여전히 참이다
 *
 * "직선은 두 끝을 곧게 잇는다" 는 한 글자도 물러서지 않는다. 바뀌는 것은 **그 선이 더는
 * 직선이 아니라는 사실**이다. 그러므로 아래 §불변식이 지켜야 하는 것은 종전과 같다:
 * `route === 'straight'` 인 연결선은 `points` 를 **결코** 들지 않는다. `connectorPath` 는
 * `curve` 가 아닌 갈래를 점 목록 그대로 이어 그리므로, 직선이 점을 하나라도 들면 그 선은
 * 폴리라인으로 그려지고 REQ-04 가 그 순간 거짓이 된다.
 *
 * ## 그래서 표가 **하나**다
 *
 * 불리언 표를 남겨 두고 승격 표를 따로 세우면 둘이 같은 말("`takes` 가 거짓인 갈래만
 * 승격한다")을 두 벌로 적게 되고, 한쪽만 고쳐지는 날 "점은 드는데 갈래가 그대로" 또는
 * 그 반대가 표현 가능해진다. 항등 여부가 곧 "점을 들 수 있는가" 이므로 물음 하나에 표
 * 하나로 충분하다.
 *
 * `free` 가 제 이름을 가리키는 것에도 뜻이 있다. 그 갈래의 점은 손이 그은 궤적에서 나지만
 * (M11) 난 뒤에는 `elbow` 의 점과 **구별할 이유가 없다** — 그리는 법이 같고(위 §`route`),
 * 그러므로 고치는 법도 같아야 한다. 출처가 다르다는 이유로 편집을 막으면 자유선은 그은
 * 순간 굳는다.
 *
 * 표로 적는 것이 요점이다. 갈래가 하나 늘면 컴파일러가 이 자리를 가리킨다 — 새 갈래는
 * "내 점은 어디 사는가" 에 답해야 하고, `route === 'straight' ? 'elbow' : route` 로 적으면
 * 그 물음이 다섯째 갈래에게는 **아무도 묻지 않은 채** 지나간다.
 */
export const ROUTE_POINT_HOST: Readonly<Record<ConnectorRoute, ConnectorRoute>> = {
  straight: 'elbow',
  elbow: 'elbow',
  curve: 'curve',
  free: 'free',
};

/**
 * 점 `count` 개를 든 연결선이 **가져야 하는** `route` — 점을 쓰는 **모든 문**이 지난다.
 *
 * 문은 셋이고 그 셋이 전부다: 만드는 쪽(`appendConnector`) · 고치는 쪽(`insertPointAt`) ·
 * 읽어 들이는 쪽(`parseConnector`). 셋이 이 한 함수를 지나므로 위 §불변식("직선은 점을
 * 들지 않는다")이 관례가 아니라 **사실**이 된다 — 한 문만 열려 있어도 그것은 지켜지기를
 * 바라는 문장일 뿐이고, 손으로 적은 config 하나가 그 문으로 들어와 직선을 폴리라인으로
 * 그린다.
 *
 * **점이 없으면 아무것도 바꾸지 않는다.** 승격은 점이 드는 그 순간의 일이고, 빈 연결선의
 * `route` 를 건드리면 사용자가 고른 도구가 이유 없이 갈린다.
 *
 * 되돌리는 짝(강등)은 **없다.** `removePointAt` 이 마지막 점을 빼도 `route` 는 그 자리에
 * 남는다. 근거 둘:
 *
 *   1. **무엇으로 되돌릴지 알 수 없다.** 점 없는 `elbow` 가 승격해서 된 것인지 사용자가
 *      꺾은 선 도구로 그은 것인지 구별하려면 출처를 적어 두어야 하고, 그 기록은 저장
 *      형상을 넓히면서 아무 화면에도 닿지 않는다.
 *   2. **화면이 달라지지 않는다.** 점 없는 네 갈래는 같은 그림이다(REQ-04-b · AC-50 —
 *      `curveSegments` 가 빈 목록을 내므로 `connectorPath` 가 같은 길로 떨어진다).
 *
 * 대가는 숨기지 않는다: **승격은 값에서 되돌아오지 않는다.** 꺾었다 편 선은 목록 행에서
 * "꺾은 선" 으로 읽히고 저장 파일에도 그렇게 남는다. 그 자리에서 화면이 거짓말을 하지는
 * 않는다 — 그 선은 실제로 **다시 꺾을 수 있는** 선이고, 직선이었다면 그러지 못했다.
 */
export function routeHosting(route: ConnectorRoute, count: number): ConnectorRoute {
  return count > 0 ? ROUTE_POINT_HOST[route] : route;
}

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
  /**
   * 중간점. **캔버스 단위 절대 정수.**
   *
   * 이 키가 서는 순간 `route` 는 그 점을 들 수 있는 갈래다(위 `routeHosting`) — 그래서
   * `straight` 와 이 키는 **함께 설 수 없다**(REQ-04).
   */
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
