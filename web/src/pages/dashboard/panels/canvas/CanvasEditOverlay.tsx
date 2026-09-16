// 캔버스 편집 DOM 오버레이 — 선택 외곽선 + 드래그 이동 + 크기 조절 핸들
// (SPEC-CANVAS-002 T5 · T8).
//
// 002 는 캔버스에 **한 픽셀도 새로 칠하지 않는다.** 선택·호버·핸들을 rAF 루프 안에 칠하면
// 편집기 상태가 001 이 유휴로 만들려고 지은 그 루프 안으로 들어와, "트윈이 없으면 프레임을
// 예약하지 않는다" 는 금지 조항에 예외가 생긴다(REQ-05 · 위험 R3). 그래서 이 층은 DOM 이며
// `CanvasSurface` 의 props 를 건드리지 않는다 — 선택을 바꿔도, 호버해도, **핸들에 초점을
// 줘도** 프레임은 0 건이다. 루프를 깨우는 유일한 경로는 종전과 같이 **`elements` 변경**이고,
// 드래그가 루프를 깨우는 것은 그것이 기하를 실제로 바꾸기 때문이지 편집기라서가 아니다
// (AC-E4).
//
// **크기를 스스로 재지 않고 투영을 다시 만들지 않는다**(위험 R1 · AC-E2). 스테이지 크기는
// 표면의 `ResizeObserver` 가 잰 값을 `CanvasOverlayContext` 로 받고, 좌표는 `canvasGeometry`
// 의 같은 함수를 쓴다. 핸들 자리도 예외가 아니다 — 이 파일은 핸들 px 를 자체 산술로 만들지
// 않고 `canvasEditGeometry.handlePositions` 를 그대로 부른다. 측정원이 둘이 되거나 투영이
// 두 벌이 되는 순간 외곽선과 핸들이 도형에서 미끄러지고, 그 결함은 "가끔 어긋난다" 로만
// 보고되어 원인을 찾기 어렵다.
//
// **포인터만은 제 공간을 스스로 맞춰 온다**(위험 R1 이 실제로 터진 자리 — AC-E9). 설정
// 미리보기는 패널을 대시보드에서의 실제 픽셀 크기로 렌더한 뒤 **통째로 축소**한다
// (`PanelSettingsDialog` §미리보기 — `computePreviewStage` + `previewZoom`). 그래서
// `getBoundingClientRect()` 는 변환 **뒤**의 화면 px 를 주고, 표면의 `ResizeObserver` 가 잰
// `stage` 는 변환 **앞**의 CSS px 다 — `ResizeObserver` 는 CSS 변환을 보지 않기 때문이다.
// 두 값을 그대로 섞으면 모든 포인터 좌표가 **정확히 축척만큼** 어긋나, 고른 도형이 엉뚱해지고
// 끌린 거리가 달라진다. 그 어긋남은 사용자에게 "도형의 영역과 그려진 자리가 다르다" 로만
// 보인다. 그래서 이 파일은 세 번째 측정원을 만드는 대신 **이미 손에 든 두 값의 비**
// (`rect.width / stage.width`)로 포인터를 `stage` 의 공간으로 되돌린다. 좌표 공간을 넘는
// 자리는 `stagePoint` **한 곳뿐**이다 — 둘이 되면 갈라진다.
//
// **핸들은 칠한 그림이 아니라 진짜 DOM 요소다**(REQ-01 · 위험 R10). 이 층을 DOM 으로 둔
// 근거 셋 중 하나가 바로 그것이다 — 핸들이 초점을 받는 `<button>` 이면 `aria-label`·탭
// 이동·호버가 전부 따라오고, 칠한 핸들이라면 그 전부를 손으로 다시 만들어야 한다. 종류별
// 핸들 집합의 비대칭은 `handlesFor` 가 이미 알고 있으므로 이 층은 그것을 되풀이하지 않고
// **`handle.id` 로 갈래를 탄다**: 글자 크기 핸들만 기하가 아니라 `style.fontSize` 를 쓰며,
// 그래서 **기하 쓰기 단일 통로(`patchNodeGeometry`)를 지나지 않는 유일한 쓰기**다. 기하를
// 쓰는 나머지 전부(이동·박스 8핸들·선 끝점)는 여전히 그 한 함수를 지난다(REQ-06).
//
// **핸들은 한 요소만 골랐을 때만 뜬다.** §크기 조절 표는 종류마다 다른 핸들 집합을 정의할
// 뿐 여러 종류가 섞인 무리의 핸들을 정의하지 않으며, 무리에 상자 하나를 씌워 조절하는 것은
// 001 에 저장할 곳이 없는 새 개념이다. 다중 선택의 조작은 **무리 이동**(여기)과 **정렬**
// (T13)이고, 그 규칙은 목록 편집기 쪽 AC-06("둘 이상 선택되면 아무 행도 자동으로 펼치지
// 않는다")과 같은 방향이다 — 다중 선택에서는 표시만 남는다.
//
// **영역 선택은 빈 자리의 주 버튼 누름으로 시작한다**(SPEC-CANVAS-010). 009 는 이 몸짓을
// modifier 로만 열어 두었고 그 근거는 "문턱(몇 px 움직이면 마키로 승격)으로 가르면
// `previewPan` 이 누름에서 이미 팬을 시작했고 그것을 무를 신호가 없다" 였다 — 그 근거는
// **문턱**에 대해서는 지금도 참이지만, 문턱을 쓰지 않고 **누르는 순간 곧장** 가져가면 무를
// 것이 없다. 그 층은 `defaultPrevented` 로 임자를 가리므로 누름을 소비하면 팬은 시작조차
// 하지 않는다.
//
// **011 이 그 안에서 조작키의 배정을 뒤집었다.** 맨손으로 끌면 **작업 영역이 옮겨지고**
// (팬), Ctrl·Cmd 를 누른 채 끌면 사각형이 서서 그 안에 **온전히 든** 것들이 선택을
// **갈아 끼우며**, Shift 를 누른 채 끌면 같은 것들이 지금 선택에 **더해진다**. 빈 자리를
// 끌지 않고 한 번 누르면 종전 그대로 선택이 풀린다(움직이지 않은 팬이 곧 클릭이다).
// 근거는 잦기 하나다 — 화면을 옮기는 일은 늘 하고 감싸 고르는 일은 가끔 하므로, 맨손이
// 잦은 쪽을 맡는다.
//
// 판정은 잉크가 아니라 **`outlineBox` 가 낸 상자**이며 그 근거와 대가는 `canvasMarquee`
// 머리말이 갖는다. 여기서 지킬 것은 하나다 — 판정하는 상자와 골라진 뒤 화면에 뜨는
// 상자가 **같은 함수**에서 나온다(위험 R1).
//
// **빈 지점의 주 버튼만 가로챈다**(REQ-05 · AC-E3 · 위험 R2 · SPEC-CANVAS-010). 설정
// 미리보기에는 이미 휠 확대와 패널 크기 조절이 걸려 있고, 대시보드 패널은 편집모드에서
// 몸통을 잡아 옮긴다 — `PanelDragLayer` 헤더가 같은 이유로 같은 결정을 했다. 010 이 빈
// 자리의 **주 버튼** 누름을 사각형에 내주면서 AC-E3 이 지키던 문장은 좁아졌다: 종전
// "빈 지점 누름은 소비하지 않는다" 가 지금은 **"주 버튼이 아닌 누름은 소비하지 않는다"**
// 이다. 011 뒤에도 그 문장은 같다 — 소비하는 임자만 사각형에서 팬으로 바뀌었다. 좁아진
// 만큼 위 세 조작 중 캔버스 안에서 잃는 것은 **바깥 층의 주 버튼 끌기 팬 하나**인데,
// 011 이 같은 몸짓을 이 층의 팬으로 돌려주었으므로 사용자가 잃는 것은 이제 없다. 휠
// 확대·크기 조절·오른쪽 버튼은 한 글자도 달라지지 않는다(누름을 쓰지 않거나 버튼이
// 다르다). 핸들은 스스로 이벤트를 소비하므로(자기 `pointer-events-auto` 위에서
// `stopPropagation`) 핸들을 잡은 포인터는 몸통 히트 테스트에 닿지 않는다.
//
// 포인터를 이 층의 루트에서 받는 이유: 캔버스는 **칠해진 픽셀**이라 잡을 DOM 노드가 없고,
// 핸들·팔레트(T9)는 어차피 이 층 안의 진짜 DOM 요소여서 같은 루트 아래 사는 편이
// 좌표 기준이 하나로 유지된다. spec.md §표면의 오버레이 슬롯은 처음에 `<canvas>` 가
// 포인터를 받는 형상을 그렸고 T3 이 그대로 통과 슬롯 넷을 달아 두었으나, 이 층이 서면서
// 그 넷은 **호출부가 하나도 없는 API** 가 되어 걷어냈다(문서도 함께 고쳤다 — spec.md
// HISTORY 0.2.0). AC-E3 이 약해지지는 않는다 — 그 인수 기준이 요구하는 것은 "어느 노드가
// 받는가" 가 아니라 **"소비하지 않는다"** 이며, 그 책임은 처음부터 이 층의 것이다.
//
// 좌표 변환은 이 파일에 **딱 하나** 있다: 포인터의 화면 좌표 → 스테이지 로컬 px → 캔버스
// 단위(축마다 스테이지 길이로 나눈 뒤 캔버스 크기를 곱한다 — `unprojectPoint`). 이것은 §히트 테스트가 금지한 "도형별 역산" 이
// 아니다 — 히트 **판정**은 여전히 순방향 투영 뒤 px 공간에서 이뤄지고(`canvasHitTest`),
// 여기서 나누는 것은 판정이 아니라 **이동량과 포인터 자리**다. 판정과 좌표는 축이 다르므로
// 둘이 모순되지 않는다. px 로 둘 수 없는 이유는 저장 좌표가 **캔버스 단위**이기 때문이다 —
// 스테이지 px 는 패널 크기에 딸린 값이라, 그대로 저장하면 창 크기가 바뀌는 순간 같은 숫자가
// 다른 자리를 뜻하게 된다.
//
// **도형 팔레트는 새 컨트롤이 아니다**(T9 · 가정 A7). 목록 편집기 하단에 있던 추가 버튼과
// **같은 생성 경로**(`canvasElementFactory.appendElement`)를 부르는 한 벌이다. 처음에는 둘
// 다 남겼지만, 팔레트가 도크로 옮겨 이름과 누를 면적을 갖춘 뒤로는 같은 함수를 부르는
// 입구가 둘일 이유가 없어 목록 쪽 줄을 걷었다 — 이제 도형을 만드는 자리는 여기 하나다.
// 팔레트가 그 함수보다 더 하는 일은 **만든 것을 고르는 것** 하나뿐이고, 그래서 다음 몸짓이
// 배치 드래그가 된다. 이것이 끌어다 놓기 대신 **눌러서 놓기**를 고른 이유다 — 끌어다
// 놓기는 드래그 고스트·드롭 좌표·취소 경로가 필요한 두 번째 드래그 기구인데, 그것이 주는
// 이득을 눌러서 놓기가 이미 준다.
//
// **다만 그 한 벌이 그려지는 자리는 스테이지가 아니다.** 처음에는 스테이지 왼쪽 위에 뜨는
// 아이콘 띠였는데, 그 자리는 그림을 가리고 스테이지 폭에 갇혀 이름을 달 수 없었다(사용
// 시험: "도형 팔레트 크기가 너무 작음"). 도구를 쓰는 곳은 패널 설정뿐이므로 미리보기
// **옆**의 제 영역으로 옮겼고(`CanvasEditDock`), 이 층은 그 자리를 컨텍스트로 받아
// 포털로 그린다. 만드는 쪽은 그대로 여기다 — 격자·정렬·순서가 스테이지 크기·선택·요소를
// 모두 봐야 하기 때문이다.
//
// **드래그가 유일한 수단이 되지 않게 한다**(T15 · REQ-01 · REQ-05 · 위험 R10). 이 층은
// 포인터 조작의 키보드 등가물을 셋으로 갚는다.
//   1) **루트가 초점을 받는다.** `tabIndex={0}` 이라 탭만으로 캔버스 편집 표면에 닿고,
//      닿은 뒤 방향키로 **고른 것을 미세 이동**(1 CSS px), Shift+방향키로 **한 격자 칸**
//      (화면에 그려진 그 칸 — 팔레트에서 고른 간격)을 옮긴다. 눌러서 골랐을 때 루트에 초점을 옮기는 것은
//      필수다 — 히트가 있을 때 `preventDefault` 하므로 브라우저의 기본 초점 이동이 함께
//      막히고, 그러면 방금 고른 것을 방향키로 옮길 수 없다.
//   2) **핸들·팔레트가 진짜 `<button>` 이다.** 초점·`aria-label`·Enter/Space 활성화가
//      공짜로 따라온다. 팔레트로 놓은 요소는 곧바로 **선택**되므로(§도형 팔레트) 포인터를
//      전혀 쓰지 않고 "놓고 → 방향키로 옮기기" 가 성립한다.
//   3) **수치 입력은 목록 편집기에 그대로 남는다**(REQ-05 금지 조항). 캔버스 선택이
//      그 칸을 지우거나 잠그지 않는다 — 스테이지 밖으로 전부 나간 요소의 회수 경로가
//      그 칸이기도 하다(AC-E5).
//
// 방향키 이동은 **격자 붙임 토글을 보지 않는다.** `snapDelta` 는 "지금 자리에서 가장 가까운
// 격자선" 으로 죄는 계산이라 1 단위 미세 이동을 0 으로 만들어 버린다 — 격자를 켜는 순간
// 방향키가 죽는 것은 접근성 후퇴다. 격자 칸 단위 이동은 Shift 가 이미 제공하고, 그 칸은
// **화면에 그려진 바로 그 칸**이므로 격자 어휘는 여전히 하나다.
//
// 어휘가 하나로 유지되는 방식이 0.8.0 에서 달라졌다. 종전에는 `gridStep` 한 변수에서
// `squareGridSteps` 가 축 쌍을 파생했고, 그리는 쪽·붙는 쪽·Shift 세 자리가 모두 그 파생을
// 지나야 했다. 이제 붙임과 Shift 는 **그 정수를 그대로** 쓴다(캔버스 단위끼리라 환산할
// 것이 없다). 0.9.0 은 남은 하나(그리는 쪽)의 환산마저 걷어냈다 — 표면이 그리는 영역을
// 그 정수의 배수로 맞춰 두고 칸을 px 로 건네므로, 이 층은 받은 값을 그대로 그린다. 셋이
// 갈라질 수 있는 자리 자체가 사라졌다 — 환산이 없는 곳은 어긋날 수도 없다.
//
// **어디가 패널에 나오는지 보인다**(SPEC-CANVAS-006 M6). 006 이 캔버스 비트맵을 패널
// 출력 영역보다 넓은 **작업 영역**으로 넓혔으므로, 이 층은 그 안에서 "여기까지가 패널에
// 나온다" 를 말하는 표시 셋을 든다 — 작업 영역 전체에 펴진 **격자**, 출력 영역 밖을 덮는
// **흐림**, 출력 영역의 **경계**. 셋 다 DOM 이고 셋 다 `pointer-events-none` 이며 캔버스에
// 한 픽셀도 칠하지 않는다(REQ-02 · 유휴 정지가 걸린 그 이유 그대로다).
//
// 셋 가운데 격자만 제 상자가 다르다. 오버레이 루트는 여전히 출력 영역 상자이므로 흐림과
// 경계는 `inset-0` 하나로 그 사각형이 되지만, 격자는 **원점만큼 되돌아 나간** 상자 안에
// 산다. 그 원점과 작업 영역 크기는 **표면이 지어 컨텍스트로 내려준 값**이며 여기서 다시
// 파생하지 않는다(위험 R1 · 불변식 I10).
//
// **그림이 나간 만큼 손도 나간다**(SPEC-CANVAS-006 M7 · REQ-08). 위 표시 셋이 서면서
// 저술 여백의 요소가 **칠해지게** 되었으나 그것만으로는 만질 수 없었다 — 포인터 처리자를
// 단 노드는 이 루트 하나뿐이고 루트는 `canvas-stage` 안의 `absolute inset-0` 이라 **닿는
// 면이 정확히 출력 영역**이었다. 여백을 누르면 그 사건은 `canvas-workspace` 나 `<canvas>`
// 에 떨어지는데 둘 다 처리자를 달지 않으므로 `handlePointerDown` 에 영영 닿지 않았다.
//
// 그래서 **아무것도 그리지 않는 층** 하나(`canvas-workspace-hit`)를 루트의 첫 자식으로
// 둔다. 격자 상자와 **같은 음수 인셋 · 같은 크기**라 닿는 면이 작업 영역만큼 넓어지되,
// 처리자는 **달지 않는다** — 누름이 루트로 버블링하고 React 는 처리자가 달린 노드를
// `currentTarget` 으로 주므로 **재는 노드가 여전히 루트**다. 그래서 `pointerFrameOf(rect,
// stage)` 의 두 값이 여전히 같은 노드를 가리키고 포인터 산술이 한 줄도 바뀌지 않는다
// (불변식 I18 · I4 · 가정 A16). 재는 면까지 함께 넓혔다면 `rect.width ÷ stage.width` 가
// 축척과 원점을 **함께** 틀리게 했을 것이다.
//
// **이 층은 표시 층이 아니다.** 위험 R8 의 가드가 열거하는 세 이름(격자 · 흐림 · 경계)에
// 이것을 더하지 않는다 — 더하면 그 가드가 곧 이 기능을 금지한다. 가드가 지키는 문장은
// "**그리는** 층은 포인터를 먹지 않는다" 이고, 이 층은 그리지 않는다.
//
// @spec SPEC-CANVAS-002 REQ-01 / REQ-02 / REQ-03 / REQ-04 / REQ-05 / REQ-06 ·
//       SPEC-CANVAS-006 REQ-02 / REQ-04 / REQ-08

import { useCallback, useEffect, useId, useRef, useState } from 'react';
import { createPortal } from 'react-dom';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import { PanelEditGrid } from '../../PanelEditGrid';
import { EMPTY_SELECTION, nextSelection } from '../charts/panelEditSelection';
import { CanvasEditDockBody } from './CanvasEditDock';
import { CanvasWorkspaceZoomField } from './CanvasWorkspaceZoomField';
import { marqueeCandidates, marqueeRect, marqueeSelection } from './canvasMarquee';
import { CanvasGroupTools } from './group/CanvasGroupTools';
import { CanvasAnchorTools } from './connector/CanvasAnchorTools';
import { CanvasConnectorTools } from './connector/CanvasConnectorTools';
import type { AnchorRefusal } from './connector/anchorTypes';
import {
  addAnchorAt,
  anchorGestureAt,
  anchorHitAt,
  anchorPoints,
  nextAnchorId,
  removeAnchor,
  type AnchorRef,
} from './connector/anchors';
import {
  DEFAULT_CANVAS_TOOL,
  TOOL_ANCHOR_GESTURE,
  TOOL_CONNECTOR_ROUTE,
  TOOL_POINT_GESTURE,
  TOOL_SHOWS_ANCHORS,
  toggleTool,
  type CanvasTool,
} from './connector/canvasTools';
import { frameKey, isPartKey, parseFrameKey } from './group/frameKey';
import {
  detachPart,
  findPart,
  groupNodes,
  partInCanvasUnits,
  patchPartGeometry,
  replacePart,
  rulesLostByDetach,
  rulesLostByUngroup,
  ungroupNode,
  type GroupRefusal,
} from './group/groupOps';
import { isGroup, type CanvasNode, type OutlinedNode } from './group/groupTypes';
import {
  isConnector,
  type ConnectorElement,
  type ConnectorRoute,
} from './connector/connectorTypes';
import {
  connectorPointGestureAt,
  insertPointAt,
  removePointAt,
} from './connector/connectorEdit';
import { resolveConnector } from './connector/resolveConnector';
// SPEC-CANVAS-011 M11 — 자유선의 궤적. 받는 일도 줄이는 일도 그 모듈이 하고, 이 층은
// 포인터가 온 자리를 캔버스 단위로 넘길 뿐이다(허용 오차도 상한도 여기에 적히지 않는다).
import { freehandPoints, ROUTE_TRACES_TRAIL, takeFreehandSample } from './connector/freehand';
import {
  type BoxGeometry,
  type CanvasElement,
  type CanvasPrimitiveKind,
  type Geometry,
  type LineGeometry,
  type PointGeometry,
} from './canvasConfig';
import {
  appendConnector,
  appendElement,
  freeConnectorEnd,
  appendImportedElements,
  appendPathElement,
  moveConnectorPoint,
  nextElementId,
  repointConnector,
  seedOffset,
  type ConnectorSide,
} from './canvasElementFactory';
import {
  CanvasScratchpadDropContext,
  pointInRect,
  type ScratchpadDropPoint,
} from './scratchpad/canvasScratchpadDrop';
import { useScratchpadStore } from './scratchpad/scratchpadStore';
import {
  cloneElements,
  elementsBounds,
  type ScratchpadEntry,
} from './scratchpad/scratchpadTypes';
import type { ShapeCatalogEntry } from './shapes/shapeCatalog';
import {
  handlePositions,
  moveGeometry,
  patchNodeGeometry,
  resizeBox,
  resizeFontSize,
  resizeLine,
  type BoxHandleId,
  type CanvasHandleId,
  type LineHandleId,
} from './canvasEditGeometry';
import {
  CANVAS_GRID_STEP_UNITS,
  alignDeltas,
  bringToFront,
  removeNodesWithConnectors,
  sendToBack,
  snapDelta,
  type AlignAxis,
  type AlignMode,
} from './canvasEditArrange';
import { useCanvasEditSelection, type CanvasSelection } from './canvasEditContext';
import { useCanvasEditDockHost } from './canvasEditDockHost';
import type { ImportedShapeSpec, ImportedTextSpec } from './svgimport/svgImportPlan';
import { useCanvasStageGrid } from './canvasStageGrid';
import {
  DEFAULT_WORKSPACE_ZOOM,
  NO_WORKSPACE_PAN,
  clampWorkspacePan,
} from './canvasWorkspace';
import {
  projectPoint,
  stageLattice,
  unprojectBox,
  unprojectPoint,
  type CanvasBox,
  type CanvasDelta,
  type CanvasProjection,
  type PxPoint,
  type StageCell,
  type StageSize,
} from './canvasGeometry';
import { hitTest } from './canvasHitTest';
import { outlineBox, resolveFontSize, resolveMeasuredWidth } from './canvasOutline';

// --- 타입 ---------------------------------------------------------------

export interface CanvasEditOverlayProps {
  /**
   * 편집이 켜져 있는가(`usePanelEditMode` 의 `active`). 꺼져 있으면 **아무것도 렌더하지
   * 않고**(표시 전용) 선택도 비운다 — 대시보드에 놓인 패널은 편집이 꺼져 있으면 표시만
   * 남고 끌리지 않는다(REQ-01 · AC-07, `PanelDragLayer` 와 같은 규칙).
   */
  enabled: boolean;
  /**
   * 배열 순서 = 그리기 순서(뒤가 위). 히트 순회의 z-order 이기도 하다.
   *
   * SPEC-CANVAS-004 M6 — 원소 타입이 `CanvasNode` 로 넓어졌다. 그전까지 `CanvasPanel` 은
   * 그룹을 **걸러 낸** 배열을 이 층에 내려보냈고, 그래서 이 층의 쓰기(드래그 · 순서 ·
   * 붙여넣기)가 손으로 저술한 그룹을 조용히 떨어뜨렸다. 그 좁히기가 여기서 사라진다.
   */
  elements: readonly CanvasNode[];
  /**
   * 표면이 넘겨준 투영 한 벌(잰 스테이지 px + config 의 캔버스 단위 크기).
   * 이 층은 크기를 **스스로 재지 않고** 투영을 **다시 만들지 않는다**.
   */
  projection: CanvasProjection;
  /** 직전 프레임이 잰 글자 폭(요소 id → px). 없으면 기준점 둘레로 폴백한다(AC-E7). */
  textWidths: Readonly<Record<string, number>>;
  /**
   * 드래그가 만든 새 요소 배열. **끄는 동안 계속** 호출되어 그림이 손을 따라온다
   * (`PanelDragLayer` 의 "미리보기가 즉시 따라와야 어디에 놓일지 보인다" 와 같은 계약).
   */
  onElementsChange: (next: CanvasNode[]) => void;
}

/** 드래그에 참여하는 요소 하나 — **잡는 순간의** 기하를 든다. */
interface DragBase {
  nodeId: string;
  geometry: Geometry;
}

/**
 * 화면 좌표를 스테이지 로컬 CSS px 로 옮길 때 쓰는 **한 벌의 기준**.
 *
 * 원점(`left`·`top`)만으로는 모자란다. 미리보기가 패널을 통째로 축소하면 화면에서 잰 상자는
 * 변환 **뒤**의 크기이고 표면이 잰 `stage` 는 변환 **앞**의 크기여서, 원점만 빼면 남는 값이
 * 여전히 화면 px 공간에 있다(파일 머리말 §포인터). 축척을 함께 들어야 포인터가 `stage` 와
 * 투영이 쓰는 그 공간으로 돌아온다.
 */
interface PointerFrame {
  /** 컨테이너의 화면 원점. */
  left: number;
  top: number;
  /** 화면 px ÷ 스테이지 px. 변환이 없으면 1 이다. */
  scaleX: number;
  scaleY: number;
}

/**
 * 진행 중인 **영역 선택**(마키) — 누른 자리부터 지금 자리까지의 사각형.
 *
 * **`dragRef` 가 아니라 React 상태다.** 드래그 상태를 ref 에 둔 것은 그것이 화면에 아무것도
 * 그리지 않고 기하 쓰기만 하기 때문이고(쓰기는 rAF 로 모인다), 마키는 반대로 **제 사각형을
 * 그려야** 하므로 렌더에 참여해야 한다. 둘을 한 자리에 합치면 이동 드래그가 매 포인터
 * 이벤트마다 재렌더를 부르게 되어 AC-E4 의 규율이 깨진다.
 *
 * `base` 는 몸짓을 **시작할 때**의 선택이다. 매 이동마다 이 값에서 다시 합집합을 세므로
 * 사각형을 줄이면 빠져나간 것이 실제로 풀린다(`canvasMarquee.marqueeSelection` §합집합).
 */
interface MarqueeState {
  pointerId: number;
  /** 잡는 순간의 좌표 기준. 드래그와 같은 규율으로 다시 재지 않는다. */
  frame: PointerFrame;
  /** 누른 자리(스테이지 로컬 CSS px). */
  origin: PxPoint;
  /** 지금 자리(스테이지 로컬 CSS px). */
  point: PxPoint;
  /** 몸짓을 시작할 때의 선택 — 합집합의 좌변이다. */
  base: CanvasSelection;
}

/**
 * 진행 중인 **긋기**(SPEC-CANVAS-011 M8 · REQ-03) — 누른 앵커에서 지금 손까지.
 *
 * ## `dragRef` 가 아니라 React 상태다 — 마키와 **같은 근거**로
 *
 * 드래그 상태를 ref 에 둔 것은 그것이 화면에 아무것도 그리지 않고 기하 쓰기만 하기
 * 때문이고(쓰기는 rAF 로 모인다), 긋기는 반대로 **제 미리보기를 그려야** 하므로 렌더에
 * 참여해야 한다. 그리고 긋는 동안 기하 쓰기는 **한 글자도 없다** — 배열이 달라지는 것은
 * 손을 뗄 때 한 번뿐이므로, ref + rAF 통로를 빌리면 흘릴 것이 없는데 루프가 돈다
 * (`MarqueeState` 가 같은 문장으로 같은 결정을 했다).
 *
 * ## 시작은 **언제나 앵커**다 (REQ-03)
 *
 * `from` 이 자유 끝점을 들 수 없는 것이 그 사실이다. 몸짓은 "한 앵커에서 눌러" 로 시작하며,
 * 앵커가 아닌 곳의 누름은 이 상태를 만들지 않고 종전의 뜻(고르기 · 마키)으로 지나간다.
 * 놓는 쪽만 자유롭다 — 그쪽은 REQ-03 이 명시적으로 허락한 갈래다.
 */
interface ConnectorDraw {
  /** 이 몸짓을 시작한 포인터. 다른 포인터의 이동은 무시한다(드래그와 같은 규율). */
  pointerId: number;
  /** 잡는 순간의 좌표 기준. 드래그·마키와 같은 규율으로 다시 재지 않는다. */
  frame: PointerFrame;
  /** 누른 앵커의 참조 — 이것이 곧 만들 선의 `from` 이다. */
  from: AnchorRef;
  /**
   * 그 앵커가 **화면에 찍힌 자리**(스테이지 로컬 px). 미리보기의 고정된 한 끝이다.
   *
   * 포인터가 내려앉은 자리가 아니라 **점의 자리**다. 둘은 오차(`ANCHOR_PICK_SLOP_PX`)만큼
   * 어긋날 수 있고, 포인터 쪽을 쓰면 미리보기가 점에서 시작하지 않는다 — 만들어질 선은
   * 점에서 시작하므로 그때 미리보기는 결과를 두고 거짓말을 한다.
   */
  origin: PxPoint;
  /** 지금 손이 있는 자리(스테이지 로컬 px). 미리보기의 움직이는 끝이다. */
  point: PxPoint;
  /**
   * 이 몸짓이 만들 선의 그리는 법. **누를 때의 도구**에서 온다(`frame` 과 같은 규율).
   *
   * 뗄 때 `tool` 을 다시 읽지 않는다. 오늘 도구는 단추로만 바뀌고 단추는 끄는 동안 눌릴
   * 수 없으나, 잡는 순간의 값을 드는 것이 이 파일의 관용구이고 그 관용구는 "왜 여기만
   * 다른가" 를 묻게 하지 않는다.
   */
  route: ConnectorRoute;
}

/** 어느 드래그든 공통으로 드는 것. */
interface DragCommon {
  /** 이 드래그를 시작한 포인터. 다른 포인터의 이동은 무시한다(멀티터치 방어). */
  pointerId: number;
  /** 잡은 자리(스테이지 로컬 CSS px). 이동량의 원점이다. */
  origin: PxPoint;
  /**
   * 잡는 순간의 좌표 기준(화면 원점 + 축척). 드래그 중에 다시 재지 않는다 — 재면 그 사이의
   * 스크롤·레이아웃·확대 변화가 이동량에 섞여 들어간다.
   */
  frame: PointerFrame;
}

/**
 * 몸통 드래그 — 선택된 **모든** 요소가 같은 델타로 움직인다.
 *
 * 함께 움직일 요소들을 **배열 위치(index)가 아니라 `nodeId` 로 든다**(REQ-06) —
 * 드래그 중 요소가 재정렬되면 index 는 다른 것을 가리킨다.
 */
interface MoveDrag extends DragCommon {
  mode: 'move';
  bases: DragBase[];
  /**
   * 격자 붙임의 **기준 상자**(잡는 순간의 캔버스 단위, 양수 범위).
   *
   * 잡은 요소 하나가 기준이며 무리는 같은 델타로 따라온다 — 무리의 각 요소를 저마다
   * 격자에 붙이면 끌려가는 동안 무리가 서로 흩어진다. 격자 붙임이 꺼져 있으면 쓰이지
   * 않으며, **잡는 순간의 값**이라 드래그 도중에 다시 재지 않는다(`frame` 과 같은 규율).
   */
  anchor: CanvasBox;
  /**
   * 잡는 순간의 **드롭 존 클라이언트 상자**. 도크가 없으면(대시보드) `null` 이고 그때
   * 놓임 판정은 아예 돌지 않는다(SPEC-CANVAS-008 REQ-03).
   *
   * **여기서 한 번만 잰다** — `frame` · `anchor` 와 같은 규율이다. 매 `pointermove` 마다
   * `getBoundingClientRect()` 를 부르면 이동 한 번마다 강제 리플로가 하나 붙는다. 도크는
   * 드래그 도중에 움직이지 않으므로 다시 잴 이유도 없다.
   */
  dropRect: DOMRect | null;
}

/** 박스 8핸들 드래그. 잡는 순간의 기하를 들고 매 프레임 **절대 포인터 자리**로 다시 잡는다. */
interface BoxResizeDrag extends DragCommon {
  mode: 'box';
  nodeId: string;
  handle: BoxHandleId;
  geometry: BoxGeometry;
}

/** 선 끝점 드래그. 고정된 반대 끝점은 잡는 순간의 값을 그대로 쓴다. */
interface LineResizeDrag extends DragCommon {
  mode: 'line';
  nodeId: string;
  endpoint: LineHandleId;
  geometry: LineGeometry;
}

/** 글자 크기 드래그. **기하가 아니라 `style.fontSize` 를 쓴다**(REQ-03). */
interface FontResizeDrag extends DragCommon {
  mode: 'font';
  nodeId: string;
  fontSize: number;
}

/**
 * 연결선 손잡이 하나 — **끝점 둘**이거나 **중간점 하나**다 (SPEC-CANVAS-011 M9 · REQ-07).
 *
 * `CanvasHandleId` 를 넓히지 않는 것이 이 자료형이 있는 이유다. 저쪽은
 * `HANDLE_ARIA_KEYS` · `HANDLE_CURSOR` 두 `Record` 가 **빠짐없이** 덮는 닫힌 이름 집합
 * 인데, 중간점은 개수가 저술마다 다르므로 그 표에 넣을 고정된 이름이 없다. 억지로 넣으면
 * 두 표가 뜻 없는 항목을 하나씩 갖고, 그때 "이 손잡이는 무엇을 하는가" 에 답이 없다.
 *
 * 그래서 M9 는 **별도 렌더 갈래**를 세운다(plan §M9 2). 그 갈래가 지나는 자리는
 * `handlePositions` 도 `handleDragState` 도 아니며, 둘은 한 글자도 바뀌지 않는다.
 */
type ConnectorHandle = { kind: 'end'; side: ConnectorSide } | { kind: 'mid'; index: number };

/**
 * 연결선 손잡이 드래그 (SPEC-CANVAS-011 M9).
 *
 * **잡는 순간의 기하를 들지 않는다** — 다른 셋과 갈리는 자리다. 연결선에는 상자가 없고
 * 끝점은 좌표가 아니라 참조이므로 "잡을 때의 값으로 매 프레임 다시 잡는다" 가 성립하지
 * 않는다. 대신 매 프레임 **포인터 자리 그대로**를 쓴다: 중간점은 그 자리로 옮기고,
 * 끝점은 그 자리가 어느 앵커 위인지 물어 참조를 갈아 끼운다.
 *
 * 그래서 프레임을 건너뛰어도 결과가 같다는 성질은 유지된다 — 누적하지 않기 때문이다.
 */
interface ConnectorPointDrag extends DragCommon {
  mode: 'connectorPoint';
  nodeId: string;
  handle: ConnectorHandle;
}

/** 진행 중인 드래그. `null` 이면 유휴. */
type DragState =
  | MoveDrag
  | BoxResizeDrag
  | LineResizeDrag
  | FontResizeDrag
  | ConnectorPointDrag;

/**
 * 아직 반영하지 않은 마지막 포인터 상태.
 *
 * Shift 를 **누른 순간이 아니라 움직인 순간**의 값으로 든다 — 끌던 도중에 Shift 를 누르거나
 * 떼면 그 자리에서 종횡비·각도 죔이 켜지고 꺼져야 한다. 잡을 때의 값을 얼려 두면 사용자가
 * 손가락을 올린 뒤에도 아무 일이 없어 "Shift 가 안 먹는다" 가 된다.
 */
interface PendingPointer {
  point: PxPoint;
  shift: boolean;
}

// --- 상수 ---------------------------------------------------------------

/**
 * 외곽선의 최소 표시 크기(px). 폭·높이 0 인 퇴화 도형과 아직 폭을 재지 못한 문구도
 * 화면에 보여야 한다 — 보이지 않으면 골라 놓고도 무엇을 골랐는지 알 수 없다(AC-E6 의
 * "화면에서 되살릴 수 없게 되지 않는다" 와 같은 취지).
 */
const MIN_OUTLINE_PX = 2;

/**
 * 표면이 없을 때의 작업 영역 원점(SPEC-CANVAS-006 M6).
 *
 * 컨텍스트가 `null` 이면 상자를 지은 쪽이 없다는 뜻이므로 원점도 없다 — `(0,0)` 은
 * 곧 "작업 영역과 출력 영역이 겹친다" 이고, 그것이 006 이전의 그림이다.
 */
const NO_WORKSPACE_ORIGIN = { x: 0, y: 0 } as const;

/**
 * 출력 영역 **밖**을 덮는 흐림의 색(SPEC-CANVAS-006 REQ-02).
 *
 * `PanelEditGrid` 가 "라이트/다크 어느 쪽에서도 보이도록" 고른 그 색상(slate 계열)을
 * **그대로** 쓰고 불투명도만 달리한다 — 색을 새로 고르면 한 화면에 편집 보조 표시가 두
 * 색으로 존재하게 된다. 안팎의 **단차**가 읽힐 만큼이되 그 위에서 저술할 수 있을 만큼
 * 옅다(격자선 0.6 보다 옅다).
 *
 * **파랑이 아닌 것이 제약이다.** 이 화면에서 파랑은 이미 두 가지 뜻을 갖는다 — 선택
 * 윤곽선(`border-blue-500/80`)과 격자의 중심 표식. 출력 영역에 파랑을 쓰면 아무것도
 * 고르지 않았는데 무언가 골라진 것처럼 읽힌다.
 */
const REGION_SCRIM_COLOR = 'rgba(148, 163, 184, 0.22)';

/**
 * 출력 영역 **경계**의 색. 같은 색상에 불투명도만 올린다 — 격자선(0.6)보다 진하고
 * **실선**이라 격자와 헷갈리지 않는다.
 */
const REGION_BOUNDS_COLOR = 'rgba(148, 163, 184, 0.95)';

/**
 * 흐림을 만드는 그림자의 퍼짐(px). 상자 **하나**에 바깥으로 퍼지는 그림자를 주면
 * "구멍 뚫린 막" 이 요소 하나로 나오고, 잘라 내는 일은 표면 컨테이너의 `overflow-hidden`
 * 이 이미 한다. 사각형 넷을 좌표로 계산해 두르는 길은 **같은 상자를 네 번 다시 파생하는
 * 일**이라 위험 R1 의 축소판이다.
 */
const REGION_SCRIM_SPREAD_PX = 9999;

/**
 * 떠 있는 배율 줄이 **작업 영역 모서리에서 떨어지는 px**(SPEC-CANVAS-006 REQ-10 · 결정 2).
 *
 * 기본 배율에서 저술 여백은 사방 100px 을 넘으므로(고정 입력 원점 (244, 111)) 이 값이
 * 여백보다 작은 한 줄은 **출력 영역을 한 픽셀도 덮지 않는다.** `z = 1.00` 에서는 여백이
 * 0 이라 작업 영역의 왼쪽 아래가 곧 출력 영역의 왼쪽 아래이고, 그때 줄은 경계의 한
 * 모서리를 덮는다 — 그 배율은 사용자가 **여백을 0 으로 하겠다고 고른** 자리이므로 덮을
 * 여백이 없다는 사실 자체가 그 선택의 결과다(위험 R25).
 */
const ZOOM_BAR_INSET_PX = 8;

/**
 * 떠 있는 배율 줄의 겉모습. **불투명하다** — 반투명 숫자는 읽을 수 없고, 아래에 요소가
 * 있을 때 사라지는 컨트롤은 "필요할 때 없는" 컨트롤이다(결정 2 가 기각한 두 안).
 */
const ZOOM_BAR_CLASS =
  'absolute z-30 flex items-center gap-1 rounded-md border border-(--color-border-default) ' +
  'bg-(--color-bg-surface) px-1.5 py-1 shadow-sm';

/** 줄 안의 입력 칸 — 떠 있는 컨트롤의 좁은 눈금이다(도크의 칸보다 짧다). */
const ZOOM_BAR_INPUT_CLASS =
  'w-14 rounded border border-(--color-border-default) bg-transparent px-1.5 py-0.5 text-xs ' +
  'tabular-nums text-(--color-text-secondary) focus:outline-none focus:ring-2 focus:ring-blue-300';

/**
 * 핸들의 `aria-label` i18n 키. **키 이름 안에 점을 넣지 않는다**(프로젝트 규약 — 이름에
 * 점이 든 키는 어떤 조회 경로로도 닿지 않는다).
 *
 * 한 요소만 골랐을 때만 핸들이 뜨므로 라벨에 요소를 밝히지 않아도 가리키는 대상이
 * 모호해지지 않는다. 라벨이 곧 "무엇을 어느 쪽으로 바꾸는가" 다.
 */
const HANDLE_ARIA_KEYS: Record<CanvasHandleId, string> = {
  nw: 'dashboard.canvas.edit.handleNw',
  n: 'dashboard.canvas.edit.handleN',
  ne: 'dashboard.canvas.edit.handleNe',
  e: 'dashboard.canvas.edit.handleE',
  se: 'dashboard.canvas.edit.handleSe',
  s: 'dashboard.canvas.edit.handleS',
  sw: 'dashboard.canvas.edit.handleSw',
  w: 'dashboard.canvas.edit.handleW',
  p1: 'dashboard.canvas.edit.handleP1',
  p2: 'dashboard.canvas.edit.handleP2',
  font: 'dashboard.canvas.edit.handleFont',
};

/** 핸들별 커서. 잡기 전에 무엇이 움직일지 손 모양으로 먼저 알린다. */
const HANDLE_CURSOR: Record<CanvasHandleId, string> = {
  nw: 'cursor-nwse-resize',
  n: 'cursor-ns-resize',
  ne: 'cursor-nesw-resize',
  e: 'cursor-ew-resize',
  se: 'cursor-nwse-resize',
  s: 'cursor-ns-resize',
  sw: 'cursor-nesw-resize',
  w: 'cursor-ew-resize',
  p1: 'cursor-move',
  p2: 'cursor-move',
  font: 'cursor-nwse-resize',
};

/**
 * 손잡이의 **몸통** — 크기 · 흰 테두리 · 칠 · 초점 테두리. **모양은 빠져 있다.**
 *
 * 크기·흰 테두리는 `FloorPlanTransformOverlay` 의 모서리 핸들 어휘를 그대로 따르고(같은
 * 대시보드 안에서 손잡이가 두 모양이 되지 않게 한다), **색만** 캔버스 선택 외곽선과 같은
 * 파랑으로 둔다 — 한 선택 안에서 외곽선과 손잡이가 다른 색이면 둘이 다른 것을 가리키는
 * 것처럼 보인다.
 *
 * 자리는 `left`/`top` 에 **투영된 핸들 점 그대로**를 두고 변환으로 중심을 맞춘다. 반 칸을
 * 미리 빼서 넣으면 그 산술이 곧 두 번째 투영이 되어 AC-E2 가 지키려는 성질이 깨진다.
 */
const HANDLE_BODY_CLASS =
  'pointer-events-auto absolute h-2.5 w-2.5 -translate-x-1/2 -translate-y-1/2 ' +
  'border border-white bg-blue-500 shadow focus:outline-none focus:ring-2 focus:ring-blue-300';

/**
 * 잡아서 **옮기거나 늘리는** 손잡이 — 네모다. 8핸들과 연결선 **끝점**이 이 옷을 입는다.
 *
 * 모서리(`rounded-*`)만 몸통에서 떼어 둔 것에 뜻이 있다. 아래 중간점 손잡이가 같은 크기 ·
 * 같은 칠 · 같은 초점 테두리를 쓰면서 **모양만** 갈리므로, 둘이 다른 일을 한다는 사실이
 * 화면에 서되 "같은 무리" 라는 사실은 남는다. 몸통을 베껴 적었다면 크기를 한 번 바꾸는 날
 * 두 손잡이가 서로 다른 크기로 갈라졌을 것이다.
 */
const HANDLE_CLASS = `${HANDLE_BODY_CLASS} rounded-sm`;

/**
 * 연결선 손잡이의 `aria-label` i18n 키 (SPEC-CANVAS-011 M9).
 *
 * **표가 따로인 것이 요점이다.** `HANDLE_ARIA_KEYS` 는 `Record<CanvasHandleId, string>`
 * 이라 이름 하나가 늘면 컴파일러가 그 표를 가리키는데, 중간점은 개수가 저술마다 달라
 * 그 닫힌 집합에 넣을 고정 이름이 없다. 그래서 여기 셋만 둔다 — 끝점 둘과, **몇 번째인가**
 * 를 치환자로 받는 중간점 하나.
 *
 * 중간점 텍스트가 번호를 말하는 것에 뜻이 있다. 한 선에 점이 셋이면 손잡이도 셋이고, 셋이
 * 같은 이름을 읽으면 보조기기를 쓰는 사람에게는 **구별 불가능한 단추 셋**이 된다.
 *
 * ## 이름이 **하는 일 둘을 모두** 말한다 (사용자 신고 2026-09-16)
 *
 * 중간점을 빼는 길은 M10 부터 있었다(그 손잡이 위의 더블클릭). 사용자는 그 길이 **없다고**
 * 신고했다 — 화면이 그 사실을 어디에서도 말하지 않았기 때문이다. 손잡이는 끌리기도 하므로
 * 모양만으로는 "끌어라" 밖에 말하지 못하고, 둘 중 하나만 말하는 이름은 다른 하나를 숨긴다.
 * 그래서 세 이름 모두가 **끄는 일**을 먼저 말하고, 중간점만 **두 번 누르는 일**을 이어 말한다.
 *
 * 순서에 뜻이 있다: 끌기는 손이 먼저 닿는 조작이고 빼기는 그 다음이다. 빼기를 앞세우면
 * 이름이 "이것은 지우는 단추" 로 읽혀, 끌어 옮기려던 손이 망설인다.
 *
 * 없애는 **두 번째 컨트롤**을 세우지 않는다(REQ-05-b). 그 컨트롤은 어디에 서야 하는지 ·
 * 점이 몰렸을 때 무엇이 되는지 · 끝점 손잡이 밑에 깔린 점에서 무엇이 되는지를 다시 묻게
 * 하고, 만드는 몸짓과 없애는 몸짓이 같다는 011 의 성질을 깬다. 이름은 그 셋을 하나도
 * 새로 묻지 않는다.
 */
const CONNECTOR_HANDLE_ARIA_KEYS: Readonly<Record<ConnectorSide | 'mid', string>> = {
  from: 'dashboard.canvas.edit.connectorHandleFrom',
  to: 'dashboard.canvas.edit.connectorHandleTo',
  mid: 'dashboard.canvas.edit.connectorHandleMid',
};

/**
 * 연결선 손잡이의 **모양** — 끝점은 네모, 중간점은 동그라미 (사용자 신고 2026-09-16).
 *
 * M9 는 셋에 같은 옷을 입혔고, 그래서 화면에는 "파란 네모 셋" 만 있었다. 그 셋은 하는 일이
 * 같지 않다: 끝점은 **다른 앵커로 갈아 끼우는** 자리이고(REQ-07-a), 중간점은 옮기거나
 * **뺄 수 있는** 자리다(REQ-05-b). 구별이 없으면 "두 번 누르면 없어진다" 를 배운 사람도
 * 그 규칙이 **어느 손잡이의 것**인지 알 수 없다 — 배울 수 없는 규칙은 없는 규칙이다.
 *
 * 동그라미를 고른 근거는 두 가지다. 하나는 이 표면이 이미 쓰는 어휘 — 파란 네모는 "잡아서
 * 크기·끝을 바꾸는 것" 이다(8핸들 · `ANCHOR_DOT_CLASS` 주석이 같은 말을 한다). 다른 하나는
 * 중간점이 실제로 **선 위에 꿴 구슬**이라는 사실이고, 구슬은 빼낼 수 있는 것으로 읽힌다.
 *
 * 앵커 점(초록 동그라미)과 섞이지 않는다 — 칠이 다르고, 둘이 함께 뜨는 동안에는 손잡이가
 * 포인터를 내려놓는다(아래 렌더 §앵커 도구가 켜진 동안).
 *
 * **표로 두는 것이 요점이다.** `CONNECTOR_HANDLE_ARIA_KEYS` 와 **같은 키 셋**이므로 손잡이
 * 종류가 하나 늘면 컴파일러가 두 표를 함께 가리킨다 — "이 손잡이는 무슨 모양이고 무슨
 * 이름인가" 가 한 번에 물어진다.
 */
const CONNECTOR_HANDLE_CLASS: Readonly<Record<ConnectorSide | 'mid', string>> = {
  from: HANDLE_CLASS,
  to: HANDLE_CLASS,
  mid: `${HANDLE_BODY_CLASS} rounded-full`,
};

/**
 * 연결선 손잡이의 커서. **셋이 아니라 하나다** — 끝점도 중간점도 **끌었을 때** 하는 일이
 * "이 점을 저기로 옮긴다" 하나이므로, 방향을 뜻하는 8핸들의 커서 어휘가 여기서는 거짓말이
 * 된다. 선 끝점 핸들(`p1`·`p2`)이 같은 이유로 같은 커서를 쓴다.
 *
 * 중간점이 모양·이름에서는 갈리면서 커서에서는 갈리지 않는 것에 뜻이 있다. 커서가 답하는
 * 물음은 **"끌면 무엇이 되는가"** 하나이고 그 답은 둘이 같다. 갈래를 하나 더 만들면 그
 * 커서는 "이것은 끄는 것이 아니다" 를 뜻하게 되는데 그것은 거짓이다 — 중간점은 끌린다.
 * 끌기 **말고도** 할 수 있는 일은 커서가 아니라 이름이 나른다(위 표).
 */
const CONNECTOR_HANDLE_CURSOR = 'cursor-move';

/**
 * 펜 커서의 그림 — 24×24 SVG 하나 (사용자 신고 2026-09-16).
 *
 * ## 왜 Tailwind 클래스가 아닌가
 *
 * 위 두 커서는 `cursor-*` 유틸리티지만 그 어휘에 **펜이 없다.** CSS 표준 커서 33 가지에도
 * 없다 — 가장 가까운 것은 `crosshair`(정밀 지정)이고 그것으로 끝냈다면 사용자가 요청한
 * "펜 모양" 은 없는 채로 남았을 것이다. 그리는 도구를 쥔 손에 十자를 주는 것은 틀린 말은
 * 아니지만 **아무 말도 아니다**: 이 표면에서 정밀 지정이 뜻하는 일이 셋이라(고르기 · 앵커
 * 세우기 · 선 긋기) 十자는 그 셋을 가르지 못한다.
 *
 * ## 그림이 지키는 것 셋
 *
 *   1. **끝이 맞는 자리에 온다.** 핫스팟은 몸통 가운데가 아니라 **펜촉**(아래 `2 21`)이다.
 *      펜은 촉으로 그으므로, 가운데를 잡으면 그림과 실제로 찍히는 자리가 어긋난다 — 이
 *      파일이 손잡이 자리에 대해 지키는 그 성질(AC-E2)과 같은 종류다.
 *   2. **어느 바탕에서도 보인다.** 칠은 선택 파랑 · 테두리는 흰색이다. 밝은 바탕에서는
 *      파란 몸통이, 어두운 바탕에서는 흰 테두리가 읽힌다. 한쪽만 두면 그 반대 바탕에서
 *      커서가 사라지고, 캔버스 바탕은 저술하는 사람이 정한다.
 *   3. **그림이 없어도 뜻이 남는다.** 값 끝의 `crosshair` 는 장식이 아니라 **낱말 대체**다.
 *      SVG 커서를 그리지 못하는 브라우저(사파리가 오래 그랬다)나 이미지를 못 읽은 경우
 *      CSS 는 그 낱말로 떨어지므로, 최악의 경우에도 "여기서 정밀하게 무언가를 시작한다"
 *      까지는 남는다. 낱말을 빼면 그 경우 커서가 기본 화살표로 돌아가 아무 말도 하지 않는다.
 *
 * `%3C`·`%23` 로 미리 죈 것은 `<`·`#` 이 데이터 URI 안에서 그대로 서지 못하기 때문이다.
 */
const PEN_CURSOR_SVG =
  "%3Csvg xmlns='http://www.w3.org/2000/svg' width='24' height='24' viewBox='0 0 24 24'%3E" +
  "%3Cpath d='M2.5 21.5L4 17L17 4L20 7L7 20Z' fill='%233b82f6' stroke='%23ffffff' " +
  "stroke-width='1.6' stroke-linejoin='round'/%3E" +
  "%3Cpath d='M4 17L7 20' stroke='%23ffffff' stroke-width='1.2' stroke-linecap='round'/%3E" +
  '%3C/svg%3E';

/**
 * 선을 그을 수 있는 자리의 커서 — **CSS 값 그대로**다(클래스가 아니다).
 *
 * 인라인 style 로 두는 것에 뜻이 있다. Tailwind 임의값으로 적으려면 이 문자열의 공백 ·
 * 따옴표 · 쉼표를 전부 그 문법으로 도로 죄어야 하고, 그러면 **커서를 읽으려는 사람이
 * 두 겹의 인코딩을 풀어야** 한다. 값이 켜졌다 꺼졌다 하는 것도 여기서는 상태이므로
 * (`penCursor`) 클래스 이름이 늘 뜻을 나르지도 않는다.
 *
 * 핫스팟 `2 21` 은 위 그림의 펜촉(2.5, 21.5)을 정수로 내린 값이다 — CSS 핫스팟은 정수
 * px 이고, 촉에서 반 픽셀 안쪽이 촉 밖보다 낫다.
 */
const PEN_CURSOR = `url("data:image/svg+xml,${PEN_CURSOR_SVG}") 2 21, crosshair`;

/**
 * 앵커 점의 지름(스테이지 px). `h-2 w-2` 가 그리는 그 크기를 **수로도** 적는다.
 *
 * 한 벌뿐인 값이 아니라 **두 자리에서 읽히는 값**이라 상수가 필요하다: 그리는 크기와
 * 집는 오차가 같은 수에서 나와야, 눈에 보이는 점보다 좁게 집히거나 한참 멀리서 집히는
 * 일이 생기지 않는다. 클래스 문자열과 이 수가 갈라지면 그 어긋남은 화면에서만 보인다.
 */
const ANCHOR_DOT_PX = 8;

/**
 * 앵커 점. **단추가 아니다**(AC-28 · REQ-02'-c).
 *
 * 임의 앵커를 제 요소를 눌러 빼게 만들면 더블클릭 판정이 **둘**이 된다 — 하나는 도형
 * 위의 것, 하나는 점 위의 것. 그 둘은 문턱도 대상 키도 따로 들게 되고, 갈리는 날
 * "도형에서는 되는데 점에서는 가끔 안 된다" 가 시작된다. 그래서 점은 **표식**이고
 * (`pointer-events-none`), 빼는 일은 여전히 오버레이의 그 한 판정(`isSecondPress`)이
 * 결정하며 **자리로만** 갈린다(`anchorGestureAt`).
 *
 * 8핸들과 칠을 달리한다 — 파랑 네모는 이 화면에서 이미 "잡아서 크기를 바꾸는 것" 을
 * 뜻한다. 앵커는 잡히지 않으므로 같은 옷을 입으면 안 된다.
 */
const ANCHOR_DOT_CLASS =
  'pointer-events-none absolute h-2 w-2 -translate-x-1/2 -translate-y-1/2 rounded-full ' +
  'border border-white bg-emerald-500 shadow-sm';

/**
 * 긋는 중인 선의 미리보기 옷 (SPEC-CANVAS-011 M8).
 *
 * 마키 사각형과 **같은 어휘**다 — 파랑 파선. 둘 다 "손이 눌려 있는 동안에만 있는 몸짓의
 * 그림자" 이므로 같은 옷을 입는 것이 옳고, 확정된 연결선(씨앗 색의 실선)과는 그래서 한눈에
 * 갈린다.
 *
 * `border-t-2` 로 긋고 높이를 0 으로 두는 것에 뜻이 있다: 이 층은 `<div>` 하나이므로
 * 위험 R8 의 **형상 가드**(칠하면서 `aria-hidden` 인 자식은 포인터를 먹지 않는다)가 이
 * 요소를 실제로 검사한다. `<svg>` 였다면 그 가드의 `instanceof HTMLElement` 에 걸리지
 * 않아 조용히 면제되었을 것이고, 면제를 얻으려고 원소 종류를 고르는 것은 가드를 우회하는
 * 일이다.
 */
const CONNECTOR_PREVIEW_CLASS =
  'pointer-events-none absolute h-0 border-t-2 border-dashed border-blue-500';

/**
 * 두 점을 잇는 **눕힌 상자**의 style — 길이와 각도뿐이다.
 *
 * 이 산술은 다른 어느 것과도 맞아떨어질 필요가 없다. 저장되지도, 히트에 쓰이지도, 확정된
 * 연결선의 자리를 정하지도 않는다 — 손이 눌려 있는 동안 화면에만 있는 값이다. 그래서
 * `connectorPath` 를 지나지 않는 것이 두 번째 기하가 되지 않는다: 그 함수는 **캔버스
 * 단위 점 목록**을 명령으로 옮기는 자이고, 여기서 필요한 것은 px 두 점 사이의 각뿐이다.
 *
 * 중간점을 그리지 않는 것도 같은 이유다. 긋는 동안에는 중간점이 없고(만드는 몸짓은
 * M10 · M11 의 것이다), 없는 동안 네 갈래는 **같은 그림**이다(REQ-04-b · AC-50) — 그래서
 * 직선 하나가 도구 넷 모두의 정직한 미리보기다.
 */
function previewLineStyle(from: PxPoint, to: PxPoint): React.CSSProperties {
  const dx = to.x - from.x;
  const dy = to.y - from.y;
  return {
    left: from.x,
    top: from.y,
    width: Math.hypot(dx, dy),
    transform: `rotate(${Math.atan2(dy, dx)}rad)`,
    transformOrigin: '0 50%',
  };
}

/**
 * 방향키 한 번의 **미세 이동**(캔버스 단위).
 *
 * 좌표계가 정수가 되면서 이 값이 1 CSS px 에서 **1 캔버스 단위**로 바뀌었다. 정수
 * 좌표계에서 1 단위는 저술할 수 있는 **가장 작은 차이**이므로, 이 조작의 뜻("손으로는
 * 낼 수 없는 정밀도")이 그대로 유지된다 — 오히려 더 정확해진다: 종전에는 1px 를 분수로
 * 환산해 더했으므로 패널 크기에 따라 저장되는 값이 달라졌지만, 이제는 어떤 패널에서
 * 눌러도 정확히 1 단위가 더해진다.
 *
 * 성큼 옮기고 싶은 사람에게는 Shift 가 한 격자 칸을 준다.
 */
const NUDGE_UNITS = 1;

/**
 * 주 버튼(`PointerEvent.button`). **빈 지점의 사각형은 이 버튼만 시작한다**(SPEC-CANVAS-010).
 *
 * 가릴 이유: 010 이전에는 빈 지점 누름이 아무것도 소비하지 않았으므로 버튼을 가릴 일이
 * 없었다. 지금은 누르는 순간 `preventDefault` 하므로, 가리지 않으면 오른쪽 버튼 누름이
 * 캔버스 위에서 상황 메뉴를 죽인다. `previewPan` 도 같은 이유로 버튼 2 를 받지 않는다.
 *
 * 가운데 버튼은 여기까지 오지 않는다 — `previewPan.onPointerDownCapture` 가 **캡처
 * 단계**에서 끊으므로 이 층은 그 누름을 보지 못한다(previewPan.ts §몸짓의 소유권).
 */
const PRIMARY_BUTTON = 0;

/**
 * 방향키 → 축 방향. 여기 없는 키는 **우리 것이 아니므로 소비하지 않는다**(Tab·Esc 같은
 * 브라우저 조작이 그대로 살아 있어야 한다).
 *
 * `y` 가 아래로 갈 때 양수인 것은 화면 좌표계 그대로다 — 여기서만 뒤집으면 이 파일 안에
 * 좌표계가 둘이 된다.
 */
const ARROW_STEPS: Record<string, { x: -1 | 0 | 1; y: -1 | 0 | 1 }> = {
  ArrowLeft: { x: -1, y: 0 },
  ArrowRight: { x: 1, y: 0 },
  ArrowUp: { x: 0, y: -1 },
  ArrowDown: { x: 0, y: 1 },
};

/**
 * **지우는 키**(SPEC-CANVAS-010). 둘을 함께 받는 이유는 손이 둘 다 쓰기 때문이다 —
 * 본체 자판은 Delete 로 손이 가고 노트북·맥 자판은 Backspace 로 간다. 한쪽만 받으면
 * 다른 자판을 쓰는 사람에게는 이 기능이 **없는 것과 같다**(사용자가 둘을 함께 적은
 * 이유이기도 하다).
 */
const DELETE_KEYS: ReadonlySet<string> = new Set(['Delete', 'Backspace']);

/**
 * **팬의 구분자** — `KeyboardEvent.key` 의 스페이스 값이다(이름이 아니라 글자 하나다).
 *
 * 이 표면에서 Space 는 그때까지 **아무 뜻도 없었다**: `handleKeyDown` 은 지우는 키와
 * 방향키만 보고 나머지를 흘려보냈다. 다만 **비어 있는 것은 루트에서일 뿐**이다 — 손잡이 ·
 * 팔레트 · 도구는 진짜 `<button>` 이라 Space 가 곧 "누름" 이고, 배율 칸은 글자 입력이다.
 * 그래서 팬은 **루트가 직접 초점을 든 동안에만** Space 를 가져간다(아래 `handleKeyDown`).
 */
const SPACE_KEY = ' ';

/**
 * Space+방향키 한 번의 팬 이동량(화면 px).
 *
 * `previewPan.PREVIEW_PAN_ARROW_PX` 와 **같은 수이고 같은 근거**다(편집 영역은 대체로
 * 500~800px 이라 한 번에 눈에 보이면서 지나치지 않는 크기가 그쯤이다). 그 상수를 빌려
 * 오지 않고 여기 따로 적는 것은 둘이 **다른 것을 옮기기** 때문이다 — 그쪽은 미리보기
 * 전체를, 이쪽은 작업 영역 안의 출력 영역을. 한쪽을 손보는 날 다른 쪽까지 따라 움직이면
 * 그것은 재사용이 아니라 우연한 결합이다.
 */
const PAN_ARROW_PX = 24;

/**
 * 스크린 리더에 알리는 단축키 목록. 값은 W3C 가 정한 키 이름이라 **번역하지 않는다**
 * (번역하면 보조기기가 알아듣지 못한다). 사람이 읽는 설명은 `keyboardHint` ·
 * `deleteHint` 가 따로 낸다.
 */
const EDIT_KEY_SHORTCUTS =
  'Delete Backspace ' +
  'ArrowUp ArrowDown ArrowLeft ArrowRight ' +
  'Shift+ArrowUp Shift+ArrowDown Shift+ArrowLeft Shift+ArrowRight ' +
  // 팬(사용자 신고 2026-09-16). **`Space+ArrowUp` 으로 적지 않는다** — 이 속성의 문법이
  // 아는 조합 키는 Alt·Control·Shift·Meta 넷뿐이라 Space 는 조합 키 자리에 설 수 없다.
  // 그래서 짚는 키 하나만 알리고, 그것으로 무엇을 하는지는 아래 `panHint` 가 말한다.
  // (W3C 가 정한 이름이라 글자 그대로 `Space` 이며 값 `' '` 가 아니다.)
  'Space';

// --- 순수 도우미 ---------------------------------------------------------

/**
 * 잡은 핸들이 **무엇을 쓸지** 잡는 순간에 한 번만 정한다.
 *
 * 종류와 핸들 id 를 프레임마다 다시 맞춰 보지 않는 이유: 다시 맞추는 자리가 곧 어긋날 수
 * 있는 두 번째 지점이 되고, 드래그 중에 요소가 갈리면(같은 id 로 다른 종류가 들어오면)
 * 잡을 때의 기하가 아니라 새 기하를 쓰게 되어 손이 뛴다. 여기서 든 기하는 **잡는 순간의
 * 값**이며, 그 값으로만 매 프레임 다시 잡는다.
 *
 * 두 `null` 은 **일어나지 않는 조합**이다(사각형에 선 끝점 핸들 따위). 핸들은 언제나
 * `handlesFor(el.kind)` 가 낸 집합에서만 렌더되므로 이 자리에 닿을 길이 없지만, 닿았다면
 * 어긋난 기하를 쓰느니 아무것도 하지 않는 편이 낫다. 지우지 않고 남겨 둔다.
 *
 * **이 함수가 상자 조작의 실제 관문이다.** 종류 목록에서 경로를 빠뜨리면 `handlesFor` 가
 * 여덟 손잡이를 내주고 `handlePositions` 가 자리까지 잡아 주는데도 **드래그가 시작되지
 * 않는다** — 손잡이가 보이는데 잡히지 않는 도형이 되고, 그 증상은 화면에서만 드러난다.
 * 앞선 두 함수를 고치고 이 한 줄을 빠뜨리면 크기 조절이 그대로 죽어 있다.
 *
 * **연결선은 이 함수에 오지 않는다**(SPEC-CANVAS-011 M9). 맞춰 볼 이름도(손잡이 id 가
 * 가변이라 `CanvasHandleId` 에 없다) 뜰 기하도(`geometry` 자체가 없다) 없으므로, 갈래를
 * 더하면 두 `null` 이 셋이 되고 그 셋이 서로 다른 뜻을 갖는다. 대신 제 시작 함수를 따로
 * 둔다(`startConnectorHandleDrag`).
 */
function handleDragState(
  el: CanvasNode,
  handle: CanvasHandleId,
  common: DragCommon,
): DragState | null {
  // 글자 크기 핸들은 기하를 보지 않으므로 종류와 맞춰 볼 것이 없다. 그룹에는 `style` 이
  // 선택 필드이므로(그릴 도형이 없다) 없을 수 있고, 그때는 기본 글자 크기로 떨어진다 —
  // 애초에 그룹에는 글자 크기 핸들이 서지 않으므로 닿지 않는 자리다.
  if (handle === 'font') {
    return { ...common, mode: 'font', nodeId: el.id, fontSize: resolveFontSize(el.style?.fontSize) };
  }
  if (handle === 'p1' || handle === 'p2') {
    if (el.kind !== 'line') return null;
    return { ...common, mode: 'line', nodeId: el.id, endpoint: handle, geometry: el.geometry };
  }
  // **그룹이 이 목록에 들어가는 것이 M6 가 이 함수에 한 전부다.** 빠뜨리면 `handlesFor` 가
  // 여덟을 내주고 `handlePositions` 가 자리까지 잡아 주는데도 드래그가 시작되지 않는다 —
  // 손잡이가 보이는데 잡히지 않는 그룹이 되고, 그 증상은 화면에서만 드러난다.
  if (el.kind !== 'rect' && el.kind !== 'ellipse' && el.kind !== 'path' && el.kind !== 'group') {
    return null;
  }
  return { ...common, mode: 'box', nodeId: el.id, handle, geometry: el.geometry };
}

/**
 * 글자 크기 쓰기. **기하가 아니므로 `patchNodeGeometry` 를 지나지 않는다.**
 *
 * `style.fontSize` 는 001 이 스타일로 정의한 값이고 기하 통로에 태울 수 없다 — 태우면
 * 004 가 그 통로에 그룹 좌표 분기를 더할 때 기하가 아닌 것까지 함께 걸린다(REQ-03 ·
 * REQ-06). 대신 이 함수는 기하 통로와 **같은 규율**을 지킨다: 새 배열을 돌려주고, 식별은
 * `nodeId` 이며, 배열 위치를 들고 다니지 않는다.
 *
 * 종류를 먼저 보는 것에 뜻이 있다 — 글자 크기 핸들은 문구 요소에만 뜨므로, 다른 종류에
 * 이 쓰기가 닿는다면 그것은 관용할 입력이 아니라 잘못된 호출이다.
 */
function patchNodeFontSize(
  elements: readonly CanvasNode[],
  nodeId: string,
  fontSize: number,
): CanvasNode[] {
  return elements.map((el): CanvasNode =>
    el.kind === 'text' && el.id === nodeId ? { ...el, style: { ...el.style, fontSize } } : el,
  );
}

// --- 복합 키 분배 (SPEC-CANVAS-009 M3 · M4) --------------------------------
//
// 009 부터 선택 상태에는 최상위 id 와 **부품 복합 키**가 함께 담긴다. 아래 셋은 그 키를
// 받아 갈래를 고르는 유일한 자리다 — 소비 측이 저마다 갈래를 적으면 "끌 때는 부품이
// 움직이는데 방향키로는 안 움직인다" 같은 부분 마비가 생긴다.
//
// **분해는 `parseFrameKey` 하나가 한다**(구분자 리터럴이 이 파일에 없다 — AC-04).

// --- 그룹 진입 (SPEC-CANVAS-009 0.3.0) -------------------------------------
//
// 0.2.0 은 "부품 위를 누르면 그 부품을 고른다" 로 못박았고, 그 결과 **그룹을 클릭으로
// 고를 길이 사라졌다.** 004 의 히트 테스트는 그룹을 제 상자가 아니라 **부품 잉크**로만
// 잡으므로(빈 상자를 잡으면 밸브 심볼의 빈 공간에서 뒤 요소가 영영 안 잡힌다), 부품이
// 선택을 가져가는 순간 그룹은 아무 데서도 잡히지 않는다.
//
// 그래서 그림 도구의 관용구를 들인다 — **단일 클릭은 그룹, 더블클릭은 그 안.**
//
// ## "안에 있는가" 를 상태로 들지 않는다
//
// Figma 처럼 한 번 들어가면 그 그룹 안에서는 단일 클릭도 부품을 고른다. 그 "안에 있음" 을
// 별도 상태로 들면 선택과 어긋난 중간 상태가 생기고("부품이 골라져 있는데 밖에 있다")
// 그것을 되돌릴 경로가 화면에 없다. 대신 **선택에서 파생시킨다**: 지금 골라진 것이 그
// 그룹의 부품이면 안에 있는 것이다. `canvasAutoExpandedId` 가 자동 펼침을 선택에서
// 파생시킨 것과 같은 규율이며, 빈 곳·다른 요소를 누르면 선택이 갈리므로 **빠져나오는
// 일도 공짜로 따라온다.**

/**
 * 두 번째 누름이 첫 번째와 **같은 몸짓**으로 읽히는 시간(ms).
 *
 * macOS·Windows 의 기본 더블클릭 속도가 대략 이 값이다. 더 좁히면 손이 느린 사용자가
 * 그룹 안으로 못 들어가고, 그 실패는 "더블클릭이 가끔 안 먹는다" 로만 보고된다.
 */
const DOUBLE_PRESS_MS = 500;
/** 그 사이 손이 이만큼 넘게 움직였으면 다른 자리를 누른 것이다(스테이지 px). */
const DOUBLE_PRESS_SLOP_PX = 5;

/**
 * 앵커를 집는 오차(스테이지 px). **두 기존 수에서 파생시킨다** — 새 눈금이 아니다.
 *
 *   보이는 반지름 `ANCHOR_DOT_PX / 2` = 4
 * + 더블클릭이 **이미 허락한** 손 떨림 `DOUBLE_PRESS_SLOP_PX` = 5
 *
 * 앞의 항이 없으면 점 위를 정확히 눌러도 빠지지 않는다 — 보이는 것보다 좁게 집히는
 * 컨트롤은 고장으로 읽힌다. 뒤의 항이 없으면 더 나쁘다: `isSecondPress` 는 두 누름이
 * 5px 까지 어긋나도 더블클릭으로 읽으므로, 오차가 4 뿐이면 **판정은 더블클릭인데 자리는
 * 빗나가** 빼려던 손이 바로 옆에 앵커를 하나 더 만든다. 한 몸짓 안에서 두 규칙이 서로
 * 다른 여유를 갖는 그 상태가 "가끔 지워지고 가끔 늘어난다" 의 형상이다.
 *
 * 더 키우지 않는 까닭은 반대쪽이다 — 오차가 크면 이미 있는 앵커 **곁에** 새 앵커를 놓을
 * 길이 사라진다. 9px 은 점 지름의 한 배 남짓이라, 눈으로 "떨어진 자리" 로 보이는 곳은
 * 여전히 더하기가 된다.
 *
 * **이 값은 더블클릭 판정이 아니다.** 판정은 위 `isSecondPress` 하나가 하고(AC-28), 이
 * 수는 그 판정이 이미 "두 번째 누름" 이라고 말한 뒤에 **어느 자리인가**만 가른다.
 */
const ANCHOR_PICK_SLOP_PX = ANCHOR_DOT_PX / 2 + DOUBLE_PRESS_SLOP_PX;

/** 직전 누름 — 더블클릭 판정에만 쓴다. */
interface LastPress {
  at: number;
  point: PxPoint;
  /** 그때 눌린 히트의 키. 같은 것을 두 번 눌러야 진입이다. */
  key: string;
}

/**
 * 이번 누름이 **같은 자리를 두 번째로** 누른 것인가.
 *
 * 시각·좌표·대상 셋을 모두 본다. `PointerEvent.detail` 에 기대지 않는 것에 뜻이 있다 —
 * 그 값은 입력 장치와 브라우저에 따라 채워지지 않으며, 채워지지 않으면 진입이 **조용히**
 * 죽는다(손가락으로는 되는데 펜으로는 안 되는 부류의 결함이다).
 */
function isSecondPress(prev: LastPress | null, now: number, point: PxPoint, key: string): boolean {
  if (prev === null || prev.key !== key) return false;
  if (now - prev.at > DOUBLE_PRESS_MS) return false;
  return (
    Math.abs(point.x - prev.point.x) <= DOUBLE_PRESS_SLOP_PX &&
    Math.abs(point.y - prev.point.y) <= DOUBLE_PRESS_SLOP_PX
  );
}

/** 지금 선택이 이 그룹 **안**을 가리키는가 — 즉 그 그룹의 부품이 골라져 있는가. */
function isInsideGroup(selection: CanvasSelection, groupId: string): boolean {
  for (const key of selection) {
    const { nodeId, partId } = parseFrameKey(key);
    if (partId !== undefined && nodeId === groupId) return true;
  }
  return false;
}

/**
 * 이번 누름이 선택에 넣을 키.
 *
 * **순수 함수로 떼어 둔 것이 요점이다.** 이 갈래는 화면에서만 드러나는 종류의 규칙이라
 * (눌러 봐야 안다) 시험이 직접 겨눌 수 있어야 한다.
 *
 * - 최상위 요소 → 제 id 그대로(004 와 바이트 동일).
 * - 부품 위, 밖에서 첫 누름 → **그룹**을 고른다.
 * - 부품 위, 두 번째 누름 → **그 부품**으로 진입한다.
 * - 부품 위, 이미 그 그룹 안 → 단일 누름도 부품을 고른다(머무름).
 */
function pressTargetKey(
  hit: { nodeId: string; partId?: string },
  selection: CanvasSelection,
  secondPress: boolean,
): string {
  if (hit.partId === undefined) return hit.nodeId;
  const enter = secondPress || isInsideGroup(selection, hit.nodeId);
  return enter ? frameKey(hit.nodeId, hit.partId) : hit.nodeId;
}

/**
 * 키가 가리키는 노드. 부품이면 **캔버스 단위 의사 노드**이고, 최상위면 그 노드 자신이다.
 *
 * 그래서 윤곽 상자 · 8핸들 · 드래그 상태가 부품에 대해서도 **한 글자도 바뀌지 않고**
 * 걸린다(009 가 004 의 A18 을 뒤집는 방식이 이것이다). 없는 키(지워진 요소 · 없는 부품)는
 * `undefined` 이고 예외가 아니다(REQ-07 · AC-40).
 *
 * ## 연결선은 **아직** 여기서 나오지 않는다 (SPEC-CANVAS-011 M4)
 *
 * 이 해석기를 지난 값은 곧바로 `outlineBox` · `handlePositions` · `moveGeometry` 에
 * 들어간다 — 전부 상자를 요구하는 통로이고, 연결선에는 그 상자가 없다. 그래서 반환이
 * `OutlinedNode` 이고, 연결선 키는 **해석되지 않은 것과 같이** 조용히 빠진다: 지워진
 * 요소의 키를 그렇게 다루는 그 자리와 같은 규율이며, 소비 측 다섯 곳에 건너뛰기를 한
 * 줄씩 심지 않아도 된다.
 *
 * **009 의 선택 모델은 한 글자도 바뀌지 않는다.** 연결선은 최상위 노드이므로 선택 키는
 * 평평한 `nodeId` 이고, 그 키가 선택 집합에 드는 것도 지금 그대로다 — 다만 그 키를 **상자
 * 로** 푸는 길이 없을 뿐이다. 연결선의 손잡이(끝점·중간점)는 id 가 가변이라 8핸들 표를
 * 지날 수 없으므로 M9 가 **별도 렌더 갈래**를 세우며, 그때 이 함수 옆에 제 해석기가 선다.
 */
function nodeForKey(elements: readonly CanvasNode[], key: string): OutlinedNode | undefined {
  const { nodeId, partId } = parseFrameKey(key);
  if (partId === undefined) {
    const node = elements.find((el) => el.id === nodeId);
    return node !== undefined && !isConnector(node) ? node : undefined;
  }
  return partInCanvasUnits(elements, nodeId, partId);
}

/**
 * 키가 가리키는 **연결선** — 위 `nodeForKey` 가 조용히 떨어뜨리는 그 갈래를 받는다
 * (SPEC-CANVAS-011 M9).
 *
 * 둘로 나눈 것이 요점이다. 저쪽의 반환이 `OutlinedNode` 인 것은 그 값을 받는 통로
 * (`outlineBox` · `handlePositions` · `moveGeometry`)가 전부 **상자**를 요구하기 때문이고,
 * 연결선에는 그 상자가 없다. 한 함수로 접어 합집합을 돌려주면 그 다섯 통로가 저마다
 * "연결선은 건너뛴다" 를 한 줄씩 갖게 된다 — 004 가 그룹에서 치른 그 대가를 되풀이하는
 * 일이다.
 *
 * 부품 키는 여기서도 부재다. 연결선은 최상위 노드이고(A4) 부품이 될 수 없으므로,
 * 복합 키가 연결선을 가리키는 상태 자체가 없다.
 */
function connectorForKey(
  elements: readonly CanvasNode[],
  key: string,
): ConnectorElement | undefined {
  const { nodeId, partId } = parseFrameKey(key);
  if (partId !== undefined) return undefined;
  const node = elements.find((el) => el.id === nodeId);
  return node !== undefined && isConnector(node) ? node : undefined;
}

/**
 * 해석된 점 목록의 `index` 번째가 **어느 손잡이인가** (SPEC-CANVAS-011 M9).
 *
 * `resolveConnector` 가 내는 목록이 `[시작, …중간점, 끝]` 이므로 양 끝만 끝점이고 나머지는
 * 중간점이다. 그 사실을 **여기 한 자리에서만** 읽는다 — 렌더와 드래그가 저마다 "0 은
 * 시작이고 마지막은 끝" 을 적으면, 중간점이 늘거나 줄 때 한쪽만 고쳐진 채 손잡이가 엉뚱한
 * 점을 쓰게 된다.
 *
 * 중간점의 `index` 는 **저장 배열의 자리**다(해석된 목록의 자리가 아니다). 쓰는 쪽
 * (`moveConnectorPoint`)이 그 배열을 그대로 색인하므로, 여기서 1 을 빼 두지 않으면
 * 끄는 손이 늘 이웃을 옮긴다.
 */
function connectorHandleAt(index: number, total: number): ConnectorHandle {
  if (index === 0) return { kind: 'end', side: 'from' };
  if (index === total - 1) return { kind: 'end', side: 'to' };
  return { kind: 'mid', index: index - 1 };
}

/**
 * 손잡이의 이름 — `data-testid` 와 React `key` 가 함께 쓴다.
 *
 * 8핸들의 이름 공간(`canvas-handle-*`)을 **쓰지 않는다.** 섞으면 `canvas-handle-nw` 가
 * "상자 왼쪽 위" 인지 "연결선의 어떤 자리" 인지 이름만으로는 답하지 못하고, 8핸들이 서지
 * 않았음을 재는 자리(AC-63)가 그 이름으로는 가려낼 것이 없어진다.
 *
 * 중간점이 **번호를 달고 나오는 것**에 뜻이 있다 — 점이 둘 이상이면 이름이 같은 단추가
 * 여럿 서고, 그때 "그 점 하나만 움직였다"(AC-65)를 겨눌 수단이 사라진다.
 */
function connectorHandleName(handle: ConnectorHandle): string {
  return handle.kind === 'end' ? handle.side : `mid-${handle.index}`;
}

/** 손잡이가 스크린 리더에 읽히는 문구. 중간점만 번호를 채워 넣는다. */
function connectorHandleLabel(t: (key: string) => string, handle: ConnectorHandle): string {
  if (handle.kind === 'end') return t(CONNECTOR_HANDLE_ARIA_KEYS[handle.side]);
  // `replaceAll` 인 것은 문구가 번호를 두 번 말할 수도 있기 때문이다(svgimport 의 상한
  // 안내가 이 저장소에서 이미 물린 자리다 — 위험 R14).
  return t(CONNECTOR_HANDLE_ARIA_KEYS.mid).replaceAll('{index}', String(handle.index + 1));
}

/** 고른 것들을 노드로 푼다. 가리킬 것이 없는 키는 조용히 빠진다. */
function selectedNodes(
  elements: readonly CanvasNode[],
  selection: CanvasSelection,
): OutlinedNode[] {
  const out: OutlinedNode[] = [];
  for (const key of selection) {
    const node = nodeForKey(elements, key);
    if (node !== undefined) out.push(node);
  }
  return out;
}

/**
 * 기하 쓰기의 **분배기**. 최상위 키는 004 의 통로로, 부품 키는 `groupOps` 의 통로로 간다.
 *
 * `canvasEditGeometry` 는 `parts` 를 여전히 모른다(AC-17) — 부품 쓰기는 저 모듈을 지나지
 * 않고 `groupOps` 안에서 끝난다. 역투영을 부르는 자리도 늘지 않는다(AC-16).
 */
function patchGeometryByKey(
  nodes: readonly CanvasNode[],
  key: string,
  geometry: Geometry,
): CanvasNode[] {
  const { nodeId, partId } = parseFrameKey(key);
  if (partId === undefined) return patchNodeGeometry(nodes, nodeId, geometry);
  return [...patchPartGeometry(nodes, nodeId, partId, geometry)];
}

/**
 * 글자 크기 쓰기의 분배기. **기하가 아니므로 위 통로를 지나지 않는다**(004 의 그 구분 그대로).
 *
 * 부품 갈래를 두지 않으면 문구 부품의 글자 크기 손잡이가 **보이는데 잡히지 않는** 손잡이가
 * 된다 — 이 파일이 `handleDragState` 머리말에서 이름 적어 둔 바로 그 결함이다.
 */
function patchFontSizeByKey(
  nodes: readonly CanvasNode[],
  key: string,
  fontSize: number,
): CanvasNode[] {
  const { nodeId, partId } = parseFrameKey(key);
  if (partId === undefined) return patchNodeFontSize(nodes, nodeId, fontSize);
  const found = findPart(nodes, nodeId, partId);
  if (found === undefined || found.part.kind !== 'text') return [...nodes];
  return [
    ...replacePart(nodes, nodeId, partId, {
      ...found.part,
      style: { ...found.part.style, fontSize },
    }),
  ];
}

/**
 * 한 축의 축척. 화면 길이 ÷ 스테이지 길이다.
 *
 * 둘 중 하나라도 0·음수·비유한이면 **1 을 돌려준다**. 그 몫은 NaN 이거나 Infinity 인데,
 * 그런 값이 포인터를 타고 흐르면 히트가 조용히 전부 빗나가고 이동량이 요소를 화면 밖으로
 * 날린다(§품질 게이트 Secured: 외부 입력은 NaN/Infinity 방어). 1 로 떨어지면 최악이라도
 * 변환 이전 — 즉 이 결함을 고치기 전 — 의 동작이며, 그 자리는 축척이 1 인 대시보드다.
 */
function axisScale(screenLength: number, stageLength: number): number {
  if (!Number.isFinite(screenLength) || !Number.isFinite(stageLength)) return 1;
  if (!(screenLength > 0) || !(stageLength > 0)) return 1;
  return screenLength / stageLength;
}

/**
 * 컨테이너의 화면 상자와 표면이 잰 스테이지 크기에서 좌표 기준을 만든다.
 *
 * **세 번째 측정원을 만들지 않는다**(AC-E2). 여기서 새로 재는 것은 없다 — 이미 손에 든
 * 두 값(포인터를 받으려면 어차피 불러야 하는 `getBoundingClientRect()`, 표면이 넘겨준
 * `stage`)의 비를 취할 뿐이다.
 */
function pointerFrameOf(rect: DOMRect, stage: StageSize): PointerFrame {
  return {
    left: rect.left,
    top: rect.top,
    scaleX: axisScale(rect.width, stage.width),
    scaleY: axisScale(rect.height, stage.height),
  };
}

/**
 * 포인터 화면 좌표 → 스테이지 로컬 CSS px.
 *
 * **이 파일에서 좌표 공간을 넘는 유일한 자리다.** 누름·이동·뗌·취소·핸들 드래그가 전부 이
 * 한 함수를 지난다 — 변환이 두 벌이 되면 한쪽만 고쳐진 채 갈라지고, 그 갈라짐은 "핸들로
 * 잡으면 조금 밀린다" 처럼 부분적인 증상으로만 보고된다.
 */
function stagePoint(clientX: number, clientY: number, frame: PointerFrame): PxPoint {
  return {
    x: (clientX - frame.left) / frame.scaleX,
    y: (clientY - frame.top) / frame.scaleY,
  };
}

// --- 컴포넌트 -----------------------------------------------------------

export default function CanvasEditOverlay({
  enabled,
  elements,
  projection,
  textWidths,
  onElementsChange,
}: CanvasEditOverlayProps) {
  /** 스테이지 px 크기 — 포인터 좌표를 화면에서 스테이지로 옮길 때만 쓴다. */
  const stage = projection.stage;
  const { t } = useTranslation();
  // provider 가 없으면 로컬 선택이다 — 대시보드에 놓인 패널에는 목록 편집기가 없다.
  const { selection, setSelection } = useCanvasEditSelection();
  /**
   * 편집 도구를 그릴 자리(패널 밖). `null` 이면 도크가 없다는 뜻이고, 그때 도구는
   * **아무 데도 그려지지 않는다** — 대시보드에 놓인 패널이 그 경우다.
   */
  const dockHost = useCanvasEditDockHost();

  /**
   * 드롭 존 노드. **자손이 올려 준다** — 도크는 이 층이 포털로 그리는 자식이므로, 자리를
   * 내려보내는 `dockHost` 와 방향이 반대다(`canvasScratchpadDrop` 머리말). `setDropZone`
   * 은 `useState` 가 주는 고정 참조라 콜백 ref 로 그대로 내려보낼 수 있다.
   */
  const [dropZone, setDropZone] = useState<HTMLElement | null>(null);

  /**
   * 마지막 묶기 거절 사유(SPEC-CANVAS-004 REQ-07 · AC-12).
   *
   * **거절은 조용하지 않다.** 그룹이 섞인 선택을 묶으려 하면 단추는 눌리되 아무 일도
   * 일어나지 않으므로, 그 사실을 화면이 말하지 않으면 사용자는 고장으로 읽는다. 사유를
   * 여기 드는 것은 컨트롤이 두 표면에 각각 서기 때문이다 — 상태를 컨트롤 안에 두면
   * 표면을 갈아 끼울 때 함께 사라진다.
   *
   * 선택이 바뀌면 지운다(아래 effect) — 사유는 **그 선택에 대한 말**이라, 다른 것을 고른
   * 뒤에도 남아 있으면 지금 고른 것을 두고 하는 말로 읽힌다.
   */
  const [groupRefusal, setGroupRefusal] = useState<GroupRefusal | null>(null);

  /**
   * 지금 손에 쥔 도구 (SPEC-CANVAS-011 REQ-02 · M3'b).
   *
   * **이 층의 첫 도구 상태다.** 011 이전의 `useState` 일곱은 전부 그리는 것과 진행 중인
   * 몸짓을 들었고(드롭 존 · 거절 · 드롭 강조 · 붙임 · 간격 · 배율 · 마키), 포인터 경로는
   * 언제나 "고르기" 한 뜻으로만 돌았다.
   *
   * 불리언이 아니라 **갈래 하나를 드는 값**인 까닭은 `connector/canvasTools.ts` 머리말에
   * 적혀 있다 — M8 이 넷을 더해 여섯이 되고, 불리언이면 그때 "둘이 함께 켜져 있다" 가
   * 형상으로 가능해진다.
   *
   * 선택·붙임과 같이 **저장하지 않는 런타임 상태**다(가정 A4). config 스키마를 넓히지
   * 않으며, 패널을 다시 열면 고르기로 돌아온다.
   */
  const [tool, setTool] = useState<CanvasTool>(DEFAULT_CANVAS_TOOL);

  /**
   * 마지막 앵커 거절 사유 (REQ-02'-d · AC-24).
   *
   * `groupRefusal` 과 **같은 자리·같은 규율**이다: 상태를 컨트롤 안에 두면 표면을 갈아
   * 끼울 때 함께 사라지고, 사유를 말하지 않으면 선·문구 위의 더블클릭은 "도구가 가끔
   * 안 먹는다" 로 읽힌다.
   */
  const [anchorRefusal, setAnchorRefusal] = useState<AnchorRefusal | null>(null);

  /** 끌던 손이 드롭 존 위에 있는가. **강조는 드롭 존 자신이 입는다**(REQ-07 · I23). */
  const [dropActive, setDropActive] = useState(false);
  /**
   * 위 값의 거울. 매 `pointermove` 마다 `setState` 를 부르지 않으려는 것이다 — 한 드래그에서
   * 실제로 바뀌는 횟수는 많아야 두어 번이고, 나머지는 전부 헛된 렌더 요청이 된다.
   */
  const dropActiveRef = useRef(false);

  /**
   * 서랍에 넣는 **한 함수**(불변식 J11). 액션만 고르므로 항목이 늘어도 이 층은 다시 그려
   * 지지 않는다 — zustand 의 액션 참조는 고정이다.
   */
  const saveEntry = useScratchpadStore((s) => s.saveEntry);

  /**
   * 키보드 설명문의 id. 한 화면에 캔버스 패널이 둘 이상 뜰 수 있으므로 고정 문자열을
   * 쓸 수 없다 — id 가 겹치면 `aria-describedby` 가 남의 설명을 가리킨다.
   */
  const hintId = useId();

  /**
   * 격자 붙임. **하나의 토글이 격자 표시와 붙임을 함께 켠다**(T12) — 붙는데 격자가 보이지
   * 않으면 무엇에 붙었는지 알 수 없고, 격자만 보이고 붙지 않으면 그 선들이 아무것도
   * 설명하지 않는다. 그래서 켜기 전에는 **아무것도 죄지 않는다**(REQ-04 의 "Where" —
   * 붙임은 켜야 하는 것이지 기본값이 아니다).
   *
   * 선택과 같이 **저장하지 않는 런타임 상태**다(가정 A4). 이 값을 바꿔도 캔버스 props 는
   * 그대로이므로 프레임을 예약하지 않는다(AC-E4).
   */
  const [snapToGrid, setSnapToGrid] = useState(false);

  /**
   * 격자 간격(정수 캔버스 단위). **이 값 하나가 그려지는 격자 · 드래그 붙임 · Shift+방향키
   * 한 칸을 함께 정한다.** 셋 다 이 정수를 **그대로** 쓴다 — 붙임과 Shift 는 캔버스 단위
   * 끼리라 환산할 것이 없고, 그리는 쪽은 표면이 이미 이 정수로 맞춰 둔 칸(px)을 받는다.
   * 환산이 없는 곳은 어긋날 수도 없다.
   *
   * **주인은 표면이다**(`CanvasSurface` §스테이지 격자). 그리는 영역이 이 간격의 정수배로
   * 맞춰지므로 값을 상자 짓는 쪽이 들어야 한다. 표면 밖(단위 시험)에서는 provider 가
   * 없으므로 아래 지역 상태로 떨어진다 — `useCanvasEditSelection` 과 같은 규율이다.
   *
   * 고를 수 있는 값은 `canvasEditArrange` 가 소유한다(격자 어휘의 주인은 여전히 그 모듈
   * 하나다). 격자 토글과 같이 **저장하지 않는 표시 상태**다(가정 A4) — config 스키마를
   * 넓히지 않는다.
   */
  const stageGrid = useCanvasStageGrid();
  const [localGridStep, setLocalGridStep] = useState<number>(CANVAS_GRID_STEP_UNITS);
  const gridStep = stageGrid?.step ?? localGridStep;
  const setGridStep = stageGrid?.setStep ?? setLocalGridStep;

  /**
   * **보기 배율**(분수) — 편집기가 작업 영역을 보여 주는 축척(SPEC-CANVAS-006 M9 · REQ-09).
   *
   * 주인도 폴백도 위 `gridStep` 과 **같은 자리·같은 규율**이다: 상태는 표면이 들고
   * (상자를 짓는 쪽이 배율을 들어야 오버레이가 같은 상자를 다시 파생하지 않는다 —
   * 불변식 I10), 표면 밖(오버레이만 세운 단위 시험)에서는 지역 상태로 떨어진다.
   *
   * 이 층은 배율을 **읽어 도크로 넘기기만 한다** — 그리는 층도 포인터 경로도 배율을 보지
   * 않는다. 그리고 배율에는 **몸짓이 하나도 붙지 않는다**: Ctrl/⌘+휠은 이 화면에서 이미
   * 미리보기 확대의 것이고, 방향키는 이 층 안에서 이미 둘로 갈려 있다(선택 있음 → 요소
   * 이동 / 없음 → 화면 이동). 셋째 주인이 낄 자리가 없다.
   */
  const [localZoom, setLocalZoom] = useState<number>(DEFAULT_WORKSPACE_ZOOM);
  const workspaceZoom = stageGrid?.zoom ?? localZoom;
  const setWorkspaceZoom = stageGrid?.setZoom ?? setLocalZoom;

  /**
   * **보기 팬** — 주인도 폴백도 바로 위 배율과 같은 자리·같은 규율이다
   * (사용자 신고 2026-09-16 · `canvasWorkspace` §보기 팬).
   *
   * 배율과 갈리는 것이 하나 있다: **팬에는 몸짓이 붙는다.** 배율의 주석이 "셋째 주인이 낄
   * 자리가 없다" 고 적은 그 셋(Ctrl/⌘+휠 · 방향키 두 갈래)은 여전히 임자가 있지만, 팬이
   * 쓰는 것은 그 셋이 아니라 **끄는 손** 둘이다:
   *
   *   - **빈 자리의 맨손 끌기**(SPEC-CANVAS-011) — 가장 잦은 몸짓이라 가장 싼 자리에 둔다.
   *   - **Space 를 짚은 채 끄는 손**(006) — 잉크 위에서도 듣는 그 몸짓이며, 이 표면에서
   *     Space 는 어떤 뜻도 갖고 있지 않았고(아래 `handleKeyDown` 은 지우는 키와 방향키만
   *     본다) 도해 도구가 모두 쓰는 관용이다.
   */
  const [localPan, setLocalPan] = useState<StageCell>(NO_WORKSPACE_PAN);
  const workspacePan = stageGrid?.pan ?? localPan;
  const setWorkspacePan = stageGrid?.setPan ?? setLocalPan;

  /**
   * **Space 를 짚고 있는가** — **도형 위에서도** 팬을 켜는 구분자다.
   *
   * 010 에서는 이 값이 팬의 유일한 문이었다(그때 빈 자리 끌기는 영역 선택의 것이었다).
   * 011 이 빈 자리 맨손 끌기를 팬에 주면서 이 값의 몫이 좁아졌다 — 남은 몫은 **잉크
   * 위**다. 도형으로 빽빽한 캔버스에는 빈 자리가 없으므로 그 몫이 사라지지는 않는다.
   *
   * 끄는 중(`panRef`)과 따로 두는 것에 뜻이 있다. 짚기만 한 상태에도 화면이 답해야 하고
   * (손잡이 · 단추의 커서까지 손 모양으로 덮인다), 그 답이 "다음 누름은 어디서든 팬이다"
   * 를 미리 말해 준다 — 눌러 봐야 아는 몸짓은 배울 수 없다.
   */
  const [spaceHeld, setSpaceHeld] = useState(false);

  /**
   * 진행 중인 팬 한 벌. `null` 이면 끌고 있지 않다.
   *
   * 드래그(`dragRef`)·마키(`marquee`)와 나란한 셋째 몸짓이지만 **state 가 아니라 ref** 인
   * 것은 프레임마다 갱신되는 값이 아니기 때문이다 — 움직일 때마다 바뀌는 것은 팬 자체이고,
   * 그 팬은 표면이 든다. 여기 남는 것은 "어느 포인터가, 어디서, 어떤 값에서 시작했는가"
   * 셋뿐이라 렌더에 아무 영향이 없다.
   */
  const panRef = useRef<{
    pointerId: number;
    startX: number;
    startY: number;
    base: StageCell;
    /**
     * 움직이지 않고 끝나면 **선택을 비우는** 누름인가 (SPEC-CANVAS-011 — 빈 자리 몸짓 뒤집기).
     *
     * 빈 자리 맨손 누름에서만 참이다. 그 누름이 팬을 가져가면서 "빈 자리를 눌러 선택을
     * 푼다" 는 010 이전부터의 뜻이 갈 곳을 잃었는데, 누르는 순간 비우면 **끄는 동안에도**
     * 비워져 팬이 선택을 잡아먹는다. 그래서 뜻을 뗌으로 옮기고, 그 뜻이 살아 있는
     * 누름인지를 잡는 순간 한 번만 적어 둔다 — Space 를 짚은 팬과 도형 위의 팬은 거짓이다.
     */
    clearsSelectionOnClick: boolean;
  } | null>(null);
  /** 끌고 있는가 — 커서 모양이 이 값으로 갈린다(`previewPan.panning` 과 같은 몫). */
  const [panning, setPanning] = useState(false);

  /**
   * 격자를 **그리기 위한** 한 칸의 CSS px.
   *
   * 표면이 그리는 영역을 이 칸의 정수배로 맞춰 두었으므로(위 `stageGrid`), 여기서 받는
   * 값은 **정수**이고 선은 소수 자리에서 시작하지 않는다 — 사용자가 세 번 돌려보낸
   * "격자가 일정하지 않음" 의 답이 그 한 사실이다. 표면이 없으면 같은 순수 함수로 직접
   * 물어본다: 계산이 두 벌이 되지 않게 **함수는 하나**이고, 그 함수는 이미 맞춰진 영역을
   * 다시 넣어도 같은 답을 낸다(멱등 — `stageLattice`).
   *
   * **이 값을 보는 곳은 `PanelEditGrid` 하나뿐이다.** 붙임(`snapDelta`)과 Shift+방향키는
   * px 를 거치지 않고 `gridStep` 정수를 그대로 쓴다.
   */
  const gridCell =
    stageGrid?.cell ?? stageLattice(projection.stage, projection.canvas, gridStep).cell;

  /**
   * **작업 영역의 원점과 크기** — 표면이 지어 내려준 값 그대로다(SPEC-CANVAS-006 M6).
   *
   * 오버레이 루트는 여전히 **패널 출력 영역** 상자다. 격자만이 그보다 넓은 **작업 영역**
   * 전체에 펴지므로, 이 층은 원점만큼 되돌아 나간 상자를 하나 두고 그 안에 격자를 넣는다.
   *
   * **다시 파생하지 않는다**(위험 R1 · 불변식 I10). 두 값은 상자를 지은 쪽이 이미 손에
   * 들고 있던 것이고, 여기서 `outer` 와 축소 비율로 되짚으면 그것이 곧 두 번째 측정원이다 —
   * 오버레이가 제 상자로 재는 값과 표면이 지은 상자가 갈라지는 그 함정이다.
   *
   * 표면 밖(오버레이만 세운 단위 시험)에서는 컨텍스트가 `null` 이므로 `{0,0}` 과
   * 출력 영역으로 떨어진다 — 그것이 곧 "상자가 하나뿐이던 시절" 의 값이며 006 이전과 같다.
   */
  const workspaceOrigin = stageGrid?.origin ?? NO_WORKSPACE_ORIGIN;
  const workspaceSize = stageGrid?.box ?? projection.stage;

  /**
   * 오버레이 루트. 핸들에서 시작한 드래그도 **루트의 상자**로 좌표를 옮기고 **루트에서**
   * 포인터를 잡는다 — 기준이 둘이 되면 몸통 드래그와 핸들 드래그가 서로 다른 원점을 믿게
   * 되고, 그 어긋남은 "핸들로 잡으면 조금 밀린다" 로만 보인다.
   */
  /**
   * 진행 중인 영역 선택. `null` 이면 그리지 않는다 — 이 층에 사각형이 **있는 시간은 손이
   * 눌려 있는 동안뿐**이며, 그것이 아래 §마키 사각형이 표시 층이 아닌 근거다(불변식 I23).
   */
  const [marquee, setMarquee] = useState<MarqueeState | null>(null);

  /**
   * 진행 중인 긋기(SPEC-CANVAS-011 M8). `null` 이면 미리보기를 그리지 않는다.
   *
   * 마키와 **같은 자리에 같은 이유로** 산다 — 손이 눌려 있는 동안에만 있고 제 그림을 그려야
   * 하므로 렌더에 참여한다. 그래서 위 사각형이 표시 층이 아닌 근거(§마키 사각형 · 불변식
   * I23)가 이 선에도 한 글자도 다르지 않게 붙는다: 다스릴 지속 상태가 없으므로 다스릴
   * 컨트롤도 없고, 컨트롤을 지어 붙인다면 그것은 누를 시간이 존재하지 않는 단추가 된다.
   */
  const [connectorDraw, setConnectorDraw] = useState<ConnectorDraw | null>(null);

  /**
   * 지금 손이 **선을 시작할 수 있는 자리**에 있는가 — 펜 커서의 절반이다
   * (사용자 신고 2026-09-16).
   *
   * 앵커 점은 표식이라 포인터를 먹지 않으므로(`ANCHOR_DOT_CLASS`) 커서가 그 점에서 나올
   * 수 없다. 나오게 하려면 점을 단추로 바꿔야 하고, 그러면 AC-28 이 막는 "더블클릭 판정이
   * 둘" 이 DOM 층에서 되살아난다. 그래서 **루트가 제 커서를 갈아 끼우고**, 갈아 끼울지는
   * 포인터 자리로 판정한다 — 점은 표식인 채로 남는다.
   *
   * 불리언 하나뿐이고 자리를 들지 않는 것에 뜻이 있다: 자리를 들면 그 값이 곧 두 번째
   * 포인터 장부가 되어 `dragRef`·`lastPressRef` 와 어긋날 수 있다. 여기서 필요한 것은
   * "펜이냐 아니냐" 뿐이다.
   */
  const [penHover, setPenHover] = useState(false);
  const penHoverRef = useRef(false);

  const rootRef = useRef<HTMLDivElement>(null);

  /**
   * 직전 누름 — **더블클릭 판정에만** 쓴다(SPEC-CANVAS-009 0.3.0 §그룹 진입).
   *
   * `useRef` 인 것에 뜻이 있다. 이 값은 화면에 아무것도 그리지 않으므로 상태로 들면
   * 누를 때마다 렌더가 한 번씩 더 돌고, 001 이 지은 유휴 정지가 그만큼 흔들린다.
   */
  const lastPressRef = useRef<LastPress | null>(null);

  const dragRef = useRef<DragState | null>(null);
  /**
   * 자유선이 긋는 동안 쌓는 **궤적**(캔버스 단위 정수 — SPEC-CANVAS-011 M11 · REQ-06).
   *
   * **`ConnectorDraw` 안이 아니라 `useRef` 다.** 그 상태는 미리보기를 그리므로 렌더에
   * 참여해야 하지만, 궤적은 화면에 아무것도 그리지 않는다(미리보기는 누른 앵커에서 지금
   * 손까지의 곧은 줄 하나다). 상태에 실으면 포인터 사건마다 렌더가 한 번 더 도는데,
   * 그 렌더가 바꾸는 픽셀은 한 점도 없다 — `lastPressRef` 가 같은 문장으로 같은 결정을
   * 했다.
   *
   * 몸짓이 시작·끝·취소될 때마다 비운다. 남겨 두면 다음에 그은 자유선이 **앞 몸짓의 손짓**
   * 을 물려받고, 그 선은 사용자가 그은 적 없는 자리를 지난다.
   */
  const trailRef = useRef<readonly PointGeometry[]>([]);
  /** 아직 반영하지 않은 마지막 포인터 상태(스테이지 로컬 px + 보조키). */
  const pendingRef = useRef<PendingPointer | null>(null);
  /** 예약된 합류 프레임. `null` 이면 예약 없음. */
  const frameRef = useRef<number | null>(null);

  /**
   * 지금 앵커를 보일 노드들 — **최상위 전부**다(REQ-02-b · 앵커를 보이는 도구가 켜진
   * 동안에만).
   *
   * 고른 것에만 세우지 않는 까닭은 M8 이다: 잇는 일은 요소 **둘** 사이에서 일어나므로,
   * 출발 앵커를 고르는 순간 도착 앵커가 사라지는 화면이 된다. M3' 가 미리 그렇게 정해
   * 두었고, M8 은 그 결정을 되돌리지 않는다.
   *
   * 도구가 꺼져 있으면 **빈 배열**이라 아래 `map` 이 DOM 에 아무것도 남기지 않으며,
   * 잇는 몸짓도 집을 점이 없어 시작되지 않는다(`anchorHitAt` 이 부재를 낸다). 편집이 꺼진
   * 표면에는 이 층 자체가 서지 않으므로 표시 전용 패널에도 앵커가 없다(위
   * `if (!enabled) return null`) — AC-60 은 그 한 줄이 이미 참으로 만든다.
   *
   * **그리는 자리와 집는 자리가 이 한 목록을 함께 본다.** 둘이 저마다 걸러 내면 "보이는
   * 점인데 잡히지 않는다"(또는 그 반대)가 표현 가능해진다 — 위험 R1 의 그 형상이다.
   * 그래서 이 값이 포인터 경로 **앞**에 선다.
   */
  // 연결선은 앵커를 내지 않는다(SPEC-CANVAS-011 M4 · A6) — 낼 윤곽 상자가 없다. 걸러 두면
  // 선에 선을 붙이는 길이 애초에 열리지 않는다(`connector/anchors.ts` §연결선도 앵커를
  // 내지 않는다). `anchorHitAt` 의 인자가 `OutlinedNode[]` 이므로 이 걸러냄이 빠지면
  // **컴파일되지 않는다** — A6 을 붙드는 것은 검사가 아니라 타입이다.
  const anchorHosts: readonly OutlinedNode[] = TOOL_SHOWS_ANCHORS[tool]
    ? elements.filter((el) => !isConnector(el))
    : [];

  /**
   * 프레임 콜백이 늦게 실행될 때 **최신** props 를 보게 한다. 드래그 중에는 매 프레임
   * `elements` 가 새로 오므로, 예약 시점의 클로저를 그대로 쓰면 한 프레임 뒤처진 배열에
   * 기하를 써 넣게 된다.
   *
   * **M9 가 둘을 더한다**(`textWidths` · `anchorHosts`). 연결선 끝점 손잡이는 끄는 동안
   * 매 프레임 "지금 손이 어느 앵커 위인가" 를 물어야 하고(REQ-07-a), 그 물음은 앵커를
   * **그리는 그 목록**을 봐야 한다 — 여기서 제 손으로 다시 거르면 보이는 점과 붙는 점이
   * 갈린다(위험 R1). 그래서 위 `anchorHosts` 가 이 줄보다 **앞에** 선다.
   */
  const latestRef = useRef({
    elements,
    projection,
    onElementsChange,
    snapToGrid,
    gridStep,
    textWidths,
    anchorHosts,
  });
  useEffect(() => {
    latestRef.current = {
      elements,
      projection,
      onElementsChange,
      snapToGrid,
      gridStep,
      textWidths,
      anchorHosts,
    };
  });

  /**
   * 한 프레임분의 조작을 반영한다.
   *
   * 이동은 **잡는 순간의 기하**(`bases`)에 매번 누적이 아니라 **절대량**으로 더하고, 크기
   * 조절은 **절대 포인터 자리**로 잡는 순간의 상자를 다시 잡는다 — 둘 다 프레임을 건너뛰거나
   * 이벤트가 합류해도 결과가 같아지는 형태다(누적하면 합류한 만큼 어긋난다).
   *
   * 기하 쓰기는 언제나 `patchNodeGeometry` 한 함수를 지난다(REQ-06). 그 통로를 지나지 않는
   * 쓰기는 **글자 크기 하나뿐**이며, 그것은 기하가 아니라 스타일이기 때문이다(REQ-03).
   *
   * **clamp 하지 않는다**(가정 A5 · AC-E5). 캔버스 밖으로 나간 배치도 뜻이 있는
   * 저술이며, 전부 나간 요소의 회수 경로는 목록 편집기의 수치 입력이다. 글자 크기만은
   * 예외로 죄는데(`resizeFontSize`), 그것은 좌표가 아니라 화면에서 글자를 잃지 않기 위한
   * 범위이며 수치 입력으로 범위 밖을 저술하는 길은 그대로 열려 있다.
   */
  const flush = useCallback(() => {
    frameRef.current = null;
    const drag = dragRef.current;
    const pending = pendingRef.current;
    if (drag === null || pending === null) return;

    const {
      elements: els,
      projection: proj,
      onElementsChange: emit,
      snapToGrid: snap,
      gridStep: step,
      textWidths: widths,
      anchorHosts: hosts,
    } = latestRef.current;
    // 0 으로 나눈 이동량은 요소를 화면 밖으로 날린다.
    if (!(proj.stage.width > 0) || !(proj.stage.height > 0)) return;

    const { point } = pending;
    const px = { dx: point.x - drag.origin.x, dy: point.y - drag.origin.y };
    // 포인터 자리와 이동량을 **캔버스 단위**로 되돌린다. 두 점을 각각 되돌려 빼므로
    // 역투영을 부르는 함수가 하나로 유지된다(`canvasGeometry` §역투영).
    const pointer = unprojectPoint(point, proj);
    const originUnits = unprojectPoint(drag.origin, proj);

    switch (drag.mode) {
      case 'move': {
        const raw = { dx: pointer.x - originUnits.x, dy: pointer.y - originUnits.y };
        // 켜져 있을 때만 죈다. 붙임은 백분율 공간을 지나지 않으므로 `clampPercentOffset`
        // 의 ±40 함정이 닿을 자리가 아예 없다(위험 R6 · AC-E5).
        // 간격은 **화면에 그려진 그 칸**이다 — 그리는 쪽도 같은 `gridStep` 에서 나온
        // 칸을 그대로 받아 그리므로 둘이 갈라질 수 없다.
        const delta = snap ? snapDelta(raw, drag.anchor, step) : raw;
        let next: CanvasNode[] = [...els];
        for (const base of drag.bases) {
          next = patchGeometryByKey(next, base.nodeId, moveGeometry(base.geometry, delta));
        }
        emit(next);
        return;
      }
      case 'box': {
        const box = resizeBox(drag.geometry, drag.handle, pointer, {
          preserveAspect: pending.shift,
        });
        emit(patchGeometryByKey(els, drag.nodeId, box));
        return;
      }
      case 'line': {
        const line = resizeLine(drag.geometry, drag.endpoint, pointer, {
          constrainAngle: pending.shift,
        });
        emit(patchGeometryByKey(els, drag.nodeId, line));
        return;
      }
      case 'connectorPoint': {
        // **기하 통로를 지나지 않는 둘째 쓰기다.** 글자 크기(아래)가 첫째였고 그것은
        // 기하가 아니라 스타일이기 때문인데, 여기는 이유가 다르다 — 연결선에 `geometry`
        // 자체가 없다(A4). `patchGeometryByKey` 는 `Geometry` 를 받으므로 실을 값이 없고,
        // 억지로 상자를 지어 넣으면 011 이 §여섯 번째 종류에서 기각한 그 형상이 된다.
        if (drag.handle.kind === 'mid') {
          // 중간점은 **그 자리 하나만** 쓴다(AC-65). 이웃은 쓰는 함수가 같은 객체 그대로
          // 지나 보낸다.
          emit(moveConnectorPoint(els, drag.nodeId, drag.handle.index, pointer));
          return;
        }
        // 끝점은 **매 프레임 다시 묻는다** — 지금 손이 어느 앵커 위인가(REQ-07-a).
        // 집는 자는 잇는 몸짓이 쓰는 그 함수 하나이고(`anchorHitAt`), 보는 목록도 점을
        // 찍은 그 목록이다(`anchorHosts`) — 보이는 점과 붙는 점이 갈릴 수 없다(위험 R1).
        //
        // 오차 밖이면 **자유 끝점**이다. 그래서 붙이는 길과 떼는 길이 한 몸짓이며,
        // 빈 곳에서 놓은 그은 선과 같은 규칙을 탄다(AC-58).
        const landed = anchorHitAt(hosts, point, proj, widths, ANCHOR_PICK_SLOP_PX);
        emit(repointConnector(els, drag.nodeId, drag.handle.side, landed?.ref, pointer));
        return;
      }
      default: {
        // 유일하게 기하 통로를 지나지 않는 쓰기다. 델타가 px 인 것은 글자 크기가 **화면
        // 양**이기 때문이다 — 캔버스 단위로 재면 캔버스가 클수록 손이 더 가야 같은 크기가 된다.
        emit(patchFontSizeByKey(els, drag.nodeId, resizeFontSize(drag.fontSize, px)));
      }
    }
  }, []);

  /**
   * 드래그를 끝내고 **마지막 유효 위치를 그대로 확정한다** — 되돌리면 사용자가 한 일이
   * 소리 없이 사라진다(REQ-03 · AC-03). `pointercancel`(브라우저·OS 가 끊은 경우)도 같다.
   */
  const finishDrag = useCallback(
    (commit: PendingPointer | null) => {
      if (dragRef.current === null) return;
      if (commit !== null) pendingRef.current = commit;
      if (frameRef.current !== null) {
        cancelAnimationFrame(frameRef.current);
        frameRef.current = null;
      }
      // 프레임 대기 중에 손을 떼면 그 이동이 사라지므로 여기서 한 번 더 흘린다.
      flush();
      dragRef.current = null;
      pendingRef.current = null;
    },
    [flush],
  );

  // 편집이 꺼지면 선택을 비우고 진행 중이던 드래그를 버린다(REQ-02).
  // `EMPTY_SELECTION` 은 상수 참조라 이미 비어 있으면 React 가 재렌더를 건너뛴다.
  useEffect(() => {
    if (enabled) return;
    dragRef.current = null;
    pendingRef.current = null;
    // 진행 중이던 사각형도 함께 거둔다. 남기면 편집이 꺼진 표면 위에 **조작할 수 없는
    // 그림**이 그대로 떠 있고, 선택은 이미 비워졌으므로 그 사각형은 아무것도 뜻하지 않는다.
    setMarquee(null);
    setSelection(EMPTY_SELECTION);
  }, [enabled, setSelection]);

  // 선택이 바뀌면 묶기 거절 안내를 거둔다(SPEC-CANVAS-004 REQ-07).
  //
  // 안내는 **그 선택에 대한 말**이다("고른 것에 그룹이 섞여 있다"). 선택을 고친 뒤에도
  // 남아 있으면 지금 고른 것을 두고 하는 말로 읽히고, 사용자는 고쳤는데도 같은 거절을
  // 보게 된다. `null` 은 상수라 이미 비어 있으면 React 가 재렌더를 건너뛴다.
  useEffect(() => setGroupRefusal(null), [selection]);

  // 도구가 바뀌면 앵커 거절 안내를 거둔다(SPEC-CANVAS-011 REQ-02'-d).
  //
  // 위 묶기 거절과 **같은 규율**이고 매달리는 것만 다르다: 묶기 안내는 그 **선택**에 대한
  // 말이고, 앵커 안내는 그 **도구**에 대한 말이다("이 도구로는 선·문구에 놓을 수 없다").
  // 도구를 끈 뒤에도 남아 있으면 지금 하는 일을 두고 하는 말로 읽힌다.
  useEffect(() => setAnchorRefusal(null), [tool]);

  // 언마운트 정리 — 예약된 합류 프레임을 남기지 않는다.
  useEffect(
    () => () => {
      if (frameRef.current !== null) cancelAnimationFrame(frameRef.current);
      frameRef.current = null;
      dragRef.current = null;
      pendingRef.current = null;
      trailRef.current = [];
    },
    [],
  );

  /**
   * 잇는 몸짓을 **끝낸다** — 놓은 자리가 앵커면 참조로, 아니면 자유 끝점으로 (REQ-03).
   *
   * ## 만드는 일은 여기서 하지 않는다
   *
   * `appendConnector` 한 함수가 id 를 발급하고 씨앗 스타일을 심는다
   * (`canvasElementFactory` — §연결선). 이 층이 제 손으로 객체를 지으면 그 조립이 곧
   * 갈라질 수 있는 두 번째 지점이 되고, 무엇보다 **씨앗 스타일을 빠뜨리기 쉽다**: 스타일이
   * 없는 연결선은 배열에 있는데 화면에는 없다(`paintStroke` 는 색과 양수 두께가 둘 다
   * 있을 때만 칠한다). 001 이 그 결함을 한 번 배달했고, 그때 고친 자리가 그 모듈이다.
   *
   * ## 같은 앵커에서 놓으면 **아무것도 만들지 않는다**
   *
   * 길이 0 인 선은 그려도 보이지 않고(두 끝이 같은 점이다) 잉크가 없어 잡기도 어렵다 —
   * 배열에만 있고 화면에는 없는 노드는 011 이 REQ-08 에서 "가장 나쁜 실패" 로 이름 적은
   * 그 형상이다. 그렇다고 안내를 띄우지도 않는다: 누른 자리에서 그대로 뗀 것은 **몸짓을
   * 그만둔 것**이지 거절당한 것이 아니며, 그 뜻은 마키가 움직임 없이 끝날 때와 같다.
   *
   * 같은 **요소**의 다른 앵커는 막지 않는다 — 한 상자의 `nw` 와 `se` 를 이으면 대각선
   * 하나가 그어지고, 그것은 사용자가 실제로 볼 수 있는 선이다.
   *
   * ## 놓은 것이 곧 **골라진다** (AC-59)
   *
   * 팔레트 · 카탈로그 · 가져오기 · 서랍이 이미 지킨 규율이다 — 방금 놓은 것에 다음 몸짓이
   * 곧바로 걸려야 한다. 연결선의 선택 키는 최상위 노드이므로 **평평하다**(009 의 선택
   * 모델은 한 글자도 바뀌지 않는다 — AC-66).
   */
  const finishConnectorDraw = (draw: ConnectorDraw, at: PxPoint): void => {
    setConnectorDraw(null);
    // 궤적은 **이 몸짓의 것**이므로 여기서 거둔다 — 아래 어느 갈래로 빠지든(같은 앵커에서
    // 놓아 아무것도 만들지 않는 갈래까지) 다음 몸짓에 넘어가지 않는다.
    const trail = trailRef.current;
    trailRef.current = [];
    const landed = anchorHitAt(anchorHosts, at, projection, textWidths, ANCHOR_PICK_SLOP_PX);
    if (landed?.ref.el === draw.from.el && landed?.ref.a === draw.from.a) return;
    // 참조든 자유 끝점이든 **캔버스 단위**다 — `anchorHitAt` 이 가리키는 자리도,
    // `unprojectPoint` 가 내는 점도 그 공간에 있으므로 한 자료형 안에 두 공간이 섞이지
    // 않는다(`connectorTypes` §중간점이 절대 좌표인 이유와 같은 규율).
    //
    // 정수로 죄는 일은 **만드는 모듈**이 한다(`freeConnectorEnd`). 이 층이 제 손으로
    // 반올림하면 파서의 규칙과 두 벌이 되고, 갈리는 날 저장 왕복에 끝점이 옮겨 앉는다.
    // 놓은 자리를 **캔버스 단위로 한 번만** 낸다. 앵커 위면 집는 함수가 이미 잰 그 자리이고
    // (`AnchorHit.at`), 아니면 포인터를 되돌린 자리다. 끝을 짓는 쪽과 궤적을 다듬는 쪽이
    // 저마다 셈하면 "선이 끝나는 자리" 가 두 벌이 된다.
    const landedAt = landed?.at ?? unprojectPoint(at, projection);
    const to = landed?.ref ?? freeConnectorEnd(landedAt);
    // 자유선만 궤적을 싣는다(M11 — `ROUTE_TRACES_TRAIL`). 나머지 셋은 궤적을 쌓은 적이
    // 없으므로 빈 목록이 지나가고, 그때 `points` 키는 아예 서지 않는다(`appendConnector`).
    //
    // **줄이는 일은 여기서 하지 않는다.** 허용 오차도 상한도 그 모듈의 것이며, 이 층이
    // 제 손으로 오차를 고르면 "어느 화면에서 그었느냐" 가 config 에 남는다. 두 끝을
    // 넘기는 것은 끝점에 겹치는 표본을 걷어내기 위함이다(그 함수의 §끝점과 겹치는 앞뒤 표본).
    const { next, created } = appendConnector(
      elements,
      draw.from,
      to,
      draw.route,
      freehandPoints(trail, unprojectPoint(draw.origin, projection), landedAt),
    );
    onElementsChange(next);
    setSelection(new Set([created.id]));
  };

  /**
   * 앵커 도구가 켜진 채로 온 **두 번째 누름** 하나를 처리한다 (REQ-02' · REQ-02'-c · AC-24).
   *
   * **판정을 여기서 다시 짓지 않는다.** 더하기인지 빼기인지 거절인지는 `anchorGestureAt`
   * 하나가 정하고(`connector/anchors.ts`), 이 층은 그 답을 배열에 옮길 뿐이다 —
   * `applyGroup` 이 `groupNodes` 에 대해 지키는 그 규율이다. 둘이 되면 "단추는 눌렸는데
   * 아무 일도 없다" 와 "안내는 떴는데 실제로는 놓였다" 가 함께 가능해진다.
   *
   * 앵커 자리도 **여기서 다시 셈하지 않는다** — 화면에 점을 찍은 `anchorPoints` 와 같은
   * 함수를 부른다. 두 벌이 되면 보이는 점과 집히는 점이 갈리고, 그 어긋남은 화면에서만
   * 드러난다(위험 R1).
   *
   * 선택을 건드리지 않는 것에도 뜻이 있다. 첫 누름이 이미 그 요소(부품이면 그 그룹)를
   * 골라 두었고, 앵커를 놓는 일은 고른 것을 바꾸는 일이 아니다.
   */
  const applyAnchorGesture = (nodeId: string, at: PxPoint): void => {
    const node = elements.find((el) => el.id === nodeId);
    if (node === undefined) return;
    // 연결선 위에서는 아무 일도 없다 — 앵커를 낼 상자가 없으므로 위 `anchorHosts` 에도
    // 점이 찍히지 않았다(M4). 거절 안내를 띄우지 않는 것에 뜻이 있다: `notBoxed` 는
    // "상자형이 아니어서 못 놓는다" 는 **도형**에 대한 사유이고, 선 위의 누름은 애초에
    // 놓을 대상을 고른 적이 없다.
    if (isConnector(node)) return;

    const gesture = anchorGestureAt(
      node,
      anchorPoints(node, projection, textWidths),
      at,
      projection,
      ANCHOR_PICK_SLOP_PX,
    );
    if (gesture.kind === 'refuse') {
      setAnchorRefusal(gesture.reason);
      return;
    }
    setAnchorRefusal(null);

    const next =
      gesture.kind === 'remove'
        ? removeAnchor(node, gesture.id)
        : addAnchorAt(node, gesture.at, nextAnchorId(node));
    // 순수 함수 둘은 할 일이 없으면 **받은 노드를 그대로** 돌려준다. 그때 배열을 새로
    // 흘리면 값이 한 자리도 달라지지 않은 채 패널이 다시 그려진다(AC-E4 의 규율).
    if (next === node) return;
    onElementsChange(elements.map((el) => (el === node ? next : el)));
  };

  /**
   * 이번 누름이 **같은 자리를 두 번째로** 누른 것인가 — 그리고 그 사실을 장부에 적는다.
   *
   * **판정을 부르는 자리가 여기 하나다**(AC-28 · AC-72). 누름은 두 문으로 들어온다:
   * 루트의 몸통 누름(아래 `handlePointerDown`)과 연결선 손잡이의 누름
   * (`startConnectorHandleDrag` — 손잡이는 진짜 단추라 이벤트를 제가 먹는다). 둘이 저마다
   * `isSecondPress` 를 부르면 판정은 여전히 한 함수이지만 **장부를 적는 자리가 둘**이 되고,
   * 그때 한쪽만 고쳐지는 날 "선 위에서는 되는데 점 위에서는 가끔 안 된다" 가 시작된다 —
   * AC-28 이 막는 형상이 그것이다. 그래서 묻는 일과 적는 일을 한 함수에 묶는다.
   *
   * 키는 **겨눈 것**이다(M9 가 세운 그 규율). 두 문이 같은 연결선을 겨누면 같은 키를 적으므로,
   * 첫 누름이 잉크에 닿고 둘째 누름이 손잡이에 닿아도 짝이 유지된다 — 손잡이는 중간점 위에
   * 서 있고 연타 오차(5px)는 손잡이보다 좁으므로 실제로 일어나는 일이다.
   */
  const pressedTwice = (at: PxPoint, key: string, now: number): boolean => {
    const second = isSecondPress(lastPressRef.current, now, at, key);
    lastPressRef.current = { at: now, point: at, key };
    return second;
  };

  /**
   * 선 위의 **두 번째 누름** 하나를 처리한다 — 점을 끼워 넣거나 뺀다 (REQ-05 · M10).
   *
   * **판정을 여기서 다시 짓지 않는다.** 더하기인지 빼기인지, 더한다면 목록의 어느 자리인지는
   * `connectorPointGestureAt` 하나가 정하고(`connector/connectorEdit.ts`), 이 층은 그 답을
   * 배열에 옮길 뿐이다 — `applyAnchorGesture` 가 앵커에 대해 지키는 그 규율이다.
   *
   * 선의 자리도 **여기서 다시 풀지 않는다** — 그리는 쪽·잡는 쪽·손잡이가 지나는 그
   * `resolveConnector` 를 부른다(AC-45). 끊긴 연결이면 목록이 부재이고, 그 부재는 그대로
   * 넘어가 아무 뜻도 없는 누름이 된다(REQ-08 — 예외가 아니다).
   *
   * 선택을 건드리지 않는 것에도 뜻이 있다. 첫 누름이 이미 그 연결선을 골라 두었고(REQ-05 의
   * "선택된 연결선" 이 그렇게 성립한다), 점을 찍는 일은 고른 것을 바꾸는 일이 아니다.
   */
  /**
   * 고친 연결선을 배열에 옮긴다 — **두 문이 같은 한 줄을 지난다**(잉크 갈래와 손잡이 갈래).
   *
   * 순수 함수들은 할 일이 없으면 **받은 것을 그대로** 돌려준다. 그때 배열을 새로 흘리면 값이
   * 한 자리도 달라지지 않은 채 패널이 다시 그려지므로(AC-E4 의 규율), 그 판정도 여기 한
   * 줄에 둔다 — 두 문에 나눠 적으면 한쪽만 빠지는 날 그 헛된 렌더가 조용히 돌아온다.
   */
  const commitConnector = (connector: ConnectorElement, next: ConnectorElement): void => {
    if (next === connector) return;
    onElementsChange(elements.map((el) => (el === connector ? next : el)));
  };

  const applyConnectorPointGesture = (connector: ConnectorElement, at: PxPoint): void => {
    const gesture = connectorPointGestureAt(
      resolveConnector(connector, elements, projection, textWidths),
      connector.route,
      at,
      projection,
      ANCHOR_PICK_SLOP_PX,
    );
    if (gesture === undefined) return;
    commitConnector(
      connector,
      gesture.kind === 'remove'
        ? removePointAt(connector, gesture.index)
        : insertPointAt(connector, gesture.index, gesture.at),
    );
  };

  /**
   * **팬 한 벌** — 시작 · 이동 · 끝. 셋을 한자리에 모아 두는 것은 팬이 이 층에서 가장 늦게
   * 들어온 몸짓이라, 흩어 두면 다음 사람이 세 갈래를 각각 찾아 읽어야 하기 때문이다.
   *
   * 옮기는 값은 **화면 px 그대로**다. 작업 영역은 잰 상자 그 자체라 축척이 1 이므로
   * (`canvasWorkspace` 불변식 I22), 손이 100px 가면 그림도 100px 간다 — 환산할 것이 없고,
   * 환산을 넣으면 그것이 곧 두 번째 투영이다.
   */
  const beginPan = (
    event: React.PointerEvent<HTMLDivElement>,
    clearsSelectionOnClick: boolean,
  ): void => {
    panRef.current = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      // **잡는 순간의 값에서만 잰다**(`handleDragState` §머리말과 같은 규율). 매 이동마다
      // 직전 팬에 더하면 죔에 걸린 동안의 손짓이 사라져, 되돌아올 때 손과 그림이 갈린다.
      base: workspacePan,
      clearsSelectionOnClick,
    };
    setPanning(true);
    // 포인터가 상자를 벗어나도 이벤트가 계속 오게 한다. jsdom 에는 없는 API 다.
    event.currentTarget.setPointerCapture?.(event.pointerId);
  };

  /**
   * 끌린 만큼 팬을 옮긴다 — **죄고 나서 넘긴다**(`clampWorkspacePan`).
   *
   * 죄지 않고 날값을 쌓으면 범위를 넘어간 만큼이 보이지 않는 빚으로 남아, 손을 되돌려도
   * 그 빚을 다 갚기 전까지 그림이 꿈쩍하지 않는다(`previewPan` 이 같은 이유로 이동에서
   * 죈다). 상한을 재는 상자는 **작업 영역**이며 그것은 표면이 지어 내려준 값이다.
   */
  const movePan = (clientX: number, clientY: number): void => {
    const pan = panRef.current;
    if (pan === null) return;
    setWorkspacePan(
      clampWorkspacePan(
        { x: pan.base.x + (clientX - pan.startX), y: pan.base.y + (clientY - pan.startY) },
        workspaceSize,
      ),
    );
  };

  /**
   * 이 뗌이 **끌기가 아니라 클릭**이었는가 (SPEC-CANVAS-011).
   *
   * 재는 것은 **누른 자리와 뗀 자리**이지 지나온 길이 아니다 — 브라우저가 클릭을 그렇게
   * 정의하고(누름과 뗌이 같은 자리), 길을 기억하면 몸짓 내내 들고 다닐 상태가 하나 는다.
   *
   * 여유는 이 파일이 이미 "손 떨림" 으로 인정해 둔 그 값이다(`DOUBLE_PRESS_SLOP_PX`).
   * 새 수를 세우면 "클릭으로 치는 떨림" 이 이 파일 안에서 두 벌이 된다.
   */
  const panWasClick = (
    pan: { startX: number; startY: number },
    clientX: number,
    clientY: number,
  ): boolean =>
    Math.abs(clientX - pan.startX) <= DOUBLE_PRESS_SLOP_PX &&
    Math.abs(clientY - pan.startY) <= DOUBLE_PRESS_SLOP_PX;

  /**
   * 팬을 끝낸다. **팬 값은 건드리지 않는다** — 마지막 이동이 이미 확정했고, 되돌리면
   * 사용자가 옮겨 놓은 시야가 뗌과 함께 사라진다(마키 · 이동 드래그가 같은 문장을 쓴다).
   */
  const finishPan = (host: HTMLDivElement, pointerId: number): void => {
    releaseCapture(host, pointerId);
    panRef.current = null;
    setPanning(false);
  };

  const handlePointerDown = (event: React.PointerEvent<HTMLDivElement>): void => {
    if (!enabled) return;
    const host = event.currentTarget;

    // **Space 를 짚은 누름은 팬이다 — 어느 갈래보다 먼저다**(사용자 신고 2026-09-16).
    //
    // 맨 앞인 것이 이 몸짓의 뜻 전부다. Space 가 구분자인 이유는 아래 세 갈래가 이미
    // 맨손 누름을 **남김없이** 나눠 가졌기 때문이다: 앵커 위면 잇기, 잉크 위면 고르기·이동,
    // 빈 자리면 팬(SPEC-CANVAS-011 — 010 의 영역 선택이 있던 자리다). 뒤에 서면 팬은
    // "아무도 안 가져간 자리" 에서만 도는데 그런 자리가 없으므로, 뒤에 선 팬은 **없는 팬**이다.
    //
    // **빈 자리가 팬이 된 뒤에도 이 갈래는 남는다**(SPEC-CANVAS-011). 아래 갈래는 빈
    // 자리에서만 팬이고, 이 갈래는 **도형 위에서도** 팬이다 — 그것이 도해 도구가 Space 에
    // 맡겨 온 뜻이고, 캔버스를 도형으로 채운 사람에게는 유일하게 남는 길이다.
    //
    // 그래서 짚은 동안에는 도형도 손잡이도 잡히지 않는다 — 그것이 잃는 것이 아니라,
    // 도형 위에서도 화면을 옮길 수 있다는 뜻이다(도해 도구의 그 관용 그대로).
    //
    // 주 버튼만 받는다. 오른쪽은 상황 메뉴의 것이고 가운데 버튼은 여기까지 오지도 않는다
    // (`previewPan` 이 캡처 단계에서 끊는다) — 아래 빈 자리 갈래가 세운 그 규칙과 같다.
    if (spaceHeld && event.button === PRIMARY_BUTTON) {
      event.preventDefault();
      event.stopPropagation();
      // 히트 갈래와 같은 이유다 — 위 `preventDefault` 가 브라우저의 기본 초점 이동을
      // 막으므로, 이 한 줄이 없으면 Space 를 뗀 뒤 키가 이 층에 닿지 않는다(T15).
      host.focus();
      // **짚은 팬은 선택을 비우지 않는다** — 움직이지 않고 끝나도 그렇다. Space 를 짚는
      // 일 자체가 "지금부터 화면을 옮긴다" 는 선언이라, 그 몸짓에는 "빈 자리를 눌러
      // 선택을 푼다" 는 뜻이 애초에 없었다(010 도 이 갈래에서는 비우지 않았다).
      beginPan(event, false);
      return;
    }

    const frame = pointerFrameOf(host.getBoundingClientRect(), stage);
    const point = stagePoint(event.clientX, event.clientY, frame);

    // **한 낱말이 세 물음에 답하던 자리다**(SPEC-CANVAS-011 — 빈 자리 몸짓 뒤집기).
    //
    // 010 까지 Shift·Ctrl·Cmd 는 이 함수 어디서나 한 낱말이었다("더한다"). 그래서 한 줄
    // (`additive`)이 세 갈래로 그대로 흘러갔고, 그것이 옳았다 — 뜻이 하나였으므로.
    //
    // 011 에서 **빈 자리에서만** 뜻이 갈린다: Ctrl/Cmd 는 사각형을 세우되 **갈아 끼우고**,
    // Shift 는 종전대로 **더하며**, 맨손은 팬이다. 잉크 위에서는 셋이 여전히 한 낱말이다
    // ("이것을 선택에 더하거나 뺀다"). 한 이름이 두 뜻을 겸하는 순간 읽는 사람은 그
    // 이름이 **어느 물음에 답하는지**를 호출 자리마다 되짚어야 하고, 그 되짚음은 언젠가
    // 반드시 한 번 틀린다 — 그때 갈리는 것은 화면이지 이 줄이 아니다.
    //
    // 그래서 조작키를 **읽는 일**만 여기 한 번 두고(아래 둘), **답은 자리마다 따로 짓는다**.
    // 세 이름은 각각 제가 무엇을 정하는지를 이름으로 말한다.
    const shiftHeld = event.shiftKey;
    const ctrlOrMetaHeld = event.ctrlKey || event.metaKey;

    /** 빈 자리 주 버튼 누름이 **무엇을 시작하는가**. 세 값이 맨손·Ctrl/Cmd·Shift 와 일대일이다. */
    const emptyPressStarts: 'pan' | 'replacingMarquee' | 'addingMarquee' = shiftHeld
      ? 'addingMarquee'
      : ctrlOrMetaHeld
        ? 'replacingMarquee'
        : 'pan';

    /**
     * 빈 자리 **보조 버튼** 누름이 지금 선택을 비우는가.
     *
     * 조작키를 짚었으면 지킨다 — 010 과 한 글자도 다르지 않다. 그 누름은 사각형을 세우지
     * 않으므로(주 버튼이 아니다) **갈아 끼울 것이 없고**, 따라서 Ctrl 의 새 뜻이 여기에
     * 닿지 않는다. 짚은 손이 말하는 것은 그대로 "내 선택을 건드리지 마라" 다.
     */
    const secondaryPressClearsSelection = !shiftHeld && !ctrlOrMetaHeld;

    /** 잉크 위 누름이 겨눈 것을 선택에 **더하거나 빼는가**(004 부터의 그 뜻 그대로다). */
    const pressTogglesSelection = shiftHeld || ctrlOrMetaHeld;

    // **연결선 도구가 켜져 있고 누른 자리가 앵커면 잇기가 시작된다**(REQ-03 · AC-57).
    //
    // 히트 테스트 **앞**에 선다. 앵커는 상자의 모서리·변 가운데·중심이라 잉크 위일 수도
    // 아닐 수도 있고(속을 채우지 않은 사각형의 모서리가 그렇다), 히트에 기대면 같은 점이
    // 도형에 따라 집히기도 하고 안 집히기도 한다. 집는 자는 `anchorHitAt` 하나이며 그것은
    // 화면에 점을 찍은 **그 지도**를 본다(위험 R1).
    //
    // ## 앵커가 아닌 누름은 **종전의 뜻 그대로** 지나간다
    //
    // 도구가 켜져 있어도 도형 몸통을 누르면 고르기·이동이고, 빈 자리를 맨손으로 누르면
    // 팬 · 조작키를 짚고 누르면 마키다(SPEC-CANVAS-011 — 011 전에는 빈 자리가 늘 마키였다).
    // M3'b 가 앵커 도구에 대해 정한 그 형상이다 — 도구는 **한 몸짓의 뜻**만 갈아 끼우고
    // 나머지는 건드리지 않는다. 막아 두면 잇는 동안에는 도형을 옮길 수도 골라 볼 수도
    // 없어, 사용자가 잇기 전후로 도구를 계속 껐다 켜게 된다.
    //
    // 주 버튼이 아닌 누름은 여기 오지 않는다 — 아래 빈 자리 갈래가 그것을 종전대로
    // 흘려보내듯, 잇기도 상황 메뉴를 가로채지 않는다.
    const drawRoute = TOOL_CONNECTOR_ROUTE[tool];
    if (drawRoute !== null && event.button === PRIMARY_BUTTON) {
      const from = anchorHitAt(anchorHosts, point, projection, textWidths, ANCHOR_PICK_SLOP_PX);
      if (from !== undefined) {
        event.preventDefault();
        event.stopPropagation();
        // 히트 갈래와 같은 이유다 — `preventDefault` 가 브라우저의 기본 초점 이동을
        // 막으므로, 이 한 줄이 없으면 그은 뒤 방향키가 이 층에 닿지 않는다(T15).
        host.focus();
        // **연타 사슬을 끊는다.** 끊지 않으면 앵커 위에서 연달아 그은 두 몸짓의 둘째 누름이
        // 첫째와 짝을 지어 더블클릭으로 읽힌다 — 앵커 갈래가 같은 한 줄을 같은 이유로 쓴다.
        lastPressRef.current = null;
        // **궤적을 비우고 시작한다**(M11). 취소·언마운트가 이미 비우지만, 시작하는 쪽이
        // 제 전제를 스스로 세우지 않으면 "어떤 경로로 들어왔느냐" 에 따라 첫 점이 달라진다.
        trailRef.current = [];
        setConnectorDraw({
          pointerId: event.pointerId,
          frame,
          from: from.ref,
          // 미리보기의 고정된 끝은 **점의 자리**이지 포인터가 내려앉은 자리가 아니다
          // (`ConnectorDraw.origin` 주석). 그 자리는 집는 함수가 **이미 잰 값**을 그대로
          // 투영한 것이라 상자를 다시 재지 않는다(AC-33 · `AnchorHit`).
          origin: projectPoint(from.at, projection),
          point,
          route: drawRoute,
        });
        host.setPointerCapture?.(event.pointerId);
        return;
      }
    }

    const hit = hitTest(elements, point, projection, textWidths);
    if (hit === undefined) {
      // **주 버튼이 아니면 종전 그대로다.** 선택만 비우고 이벤트를 소비하지 않는다 —
      // 오른쪽 버튼은 상황 메뉴의 것이고, 가운데 버튼은 애초에 여기까지 오지 않는다
      // (`previewPan` 이 캡처 단계에서 끊는다). 여기서 소비하면 캔버스 위에서만
      // 상황 메뉴가 사라지는 화면이 된다.
      if (event.button !== PRIMARY_BUTTON) {
        if (secondaryPressClearsSelection && selection.size > 0) setSelection(EMPTY_SELECTION);
        return;
      }

      // **빈 지점의 주 버튼 누름은 우리 것이다**(SPEC-CANVAS-010 — 009 의 modifier 게이트를
      // 걷어낸 자리다).
      //
      // 009 는 맨손 누름을 흘려보냈고 그 근거는 "문턱으로 가르면 `previewPan` 이 누름에서
      // 이미 팬을 시작했고 그것을 무를 신호가 없다" 였다. 그 근거는 **문턱**에 대해서는
      // 지금도 참이다. 다만 문턱을 쓰지 않고 **누르는 순간 곧장 가져가면** 무를 것이 없다 —
      // 그 층은 `if (event.defaultPrevented) return;` 한 줄로 임자를 가리므로, 여기서
      // 누름을 소비하면 팬은 **시작조차 하지 않는다**. 새 약속이 아니라 그 파일이 이미
      // 적어 둔 규칙("아무도 가져가지 않은 몸짓만 받는다")을 그대로 읽은 것이다.
      //
      // 그 대가로 **캔버스 안에서 주 버튼 끌기로 화면을 옮기는 길은 닫힌다** — 고 010 이
      // 적었고, **011 이 그 대가를 되돌린다**(아래 §맨손은 팬이다). 캔버스 **밖**
      // (게이지·차트·히트맵 미리보기)은 아무도 몸짓을 가져가지 않으므로 주 버튼 끌기가
      // 종전 그대로 `previewPan` 의 팬이다.
      event.preventDefault();
      event.stopPropagation();
      // 히트 때와 같은 이유다 — 바로 위 `preventDefault` 가 브라우저의 기본 초점 이동을
      // 막으므로, 이 한 줄이 없으면 방금 감싸 고른 것을 방향키로 옮길 수 없다(T15).
      host.focus();

      // 빈 자리를 누르면 **연타 사슬이 끊긴다**(SPEC-CANVAS-009 0.3.0). 끊지 않으면 빈
      // 곳을 거쳐 같은 부품을 다시 누른 것이 더블클릭으로 읽혀, 사용자가 한 번도 겹쳐
      // 누르지 않았는데 그룹 안으로 들어간다. **세 뜻이 함께 쓰는 한 줄**이다 — 끊는
      // 까닭이 "빈 자리를 지나왔다" 이지 "무엇이 시작되었다" 가 아니기 때문이다.
      lastPressRef.current = null;

      // **맨손 끌기는 팬이다**(SPEC-CANVAS-011 — 010 의 맨손 사각형이 있던 자리다).
      //
      // 두 몸짓의 잦기가 자리를 정한다. 화면을 옮기는 일은 **늘** 하고 사각형으로 감싸는
      // 일은 **가끔** 한다 — 그러니 맨손이 잦은 쪽을 맡고, 가끔 쓰는 쪽이 조작키를 짚는다.
      // 010 은 그 반대였고, 그래서 사용자는 화면을 옮길 때마다 Space 를 찾아 짚어야 했다.
      //
      // **선택을 비우지 않고, 사각형도 세우지 않는다.** 하나라도 하면 팬이 제 일이 아닌
      // 것을 하게 된다: 비우면 화면을 옮길 때마다 고른 것이 사라지고, 사각형을 세우면 손이
      // 지나간 자리의 것들이 딸려 온다. "빈 자리를 눌러 선택을 푼다" 는 뜻은 사라지지 않고
      // **뗌으로 옮겨 간다** — 움직이지 않은 팬이 곧 클릭이며, 그 판정은 뗌이 한다
      // (`clearsSelectionOnClick` · `panWasClick`).
      if (emptyPressStarts === 'pan') {
        beginPan(event, true);
        return;
      }

      // **Ctrl/Cmd 는 갈아 끼우고 Shift 는 더한다**(SPEC-CANVAS-011). 010 에서 맨손이
      // 맡던 "갈아 끼우기" 가 Ctrl/Cmd 로 옮겨 온 것이고, Shift 의 뜻은 009 부터 한 글자도
      // 바뀌지 않았다. 둘이 같은 뜻이면 둘 중 하나는 아무것도 가르지 않는 장식이 된다.
      //
      // **누르는 순간에는 비우지 않는다**(010 과 갈리는 자리다). 010 이 누름에서 비운 것은
      // 그 맨손 누름이 곧 "선택을 푸는" 몸짓이었기 때문인데, 그 뜻은 위 팬 갈래로 옮겨
      // 갔다. 조작키를 짚은 누름은 "사각형을 세우겠다" 는 선언일 뿐이므로, 움직이지 않고
      // 끝난 Ctrl 누름이 선택을 쓸어버릴 까닭이 없다 — Shift 누름이 010 에서도 그랬다.
      // 갈아 끼우기는 **이동에서** 일어난다: 좌변이 빈 선택이므로 사각형이 덮은 것만 남는다.
      const base = emptyPressStarts === 'addingMarquee' ? selection : EMPTY_SELECTION;
      setMarquee({ pointerId: event.pointerId, frame, origin: point, point, base });
      host.setPointerCapture?.(event.pointerId);
      return;
    }

    // 여기부터는 우리 조작이다 — 상위가 함께 반응하지 않도록 이 이벤트만 소비한다.
    event.preventDefault();
    event.stopPropagation();

    // **초점을 손으로 옮긴다.** 바로 위 `preventDefault` 가 브라우저의 기본 초점 이동까지
    // 함께 막으므로, 이 한 줄이 없으면 방금 고른 것을 방향키로 옮길 수 없다 — 포인터로
    // 고른 사람과 키보드로 옮기려는 사람이 같은 사람이다(T15 · REQ-01).
    host.focus();

    // **단일 클릭은 그룹, 더블클릭은 그 안**(SPEC-CANVAS-009 0.3.0 · REQ-01).
    //
    // 히트 테스트는 004 부터 `{ nodeId, partId }` 를 돌려주고 있었고, 무엇을 고를지는
    // `pressTargetKey` 한 함수가 정한다. 최상위 요소에서는 `frameKey` 가 `nodeId` 를
    // **그대로** 돌려주므로 그 경로는 004 와 바이트 동일하다(불변식 G11 · AC-06).
    //
    // 판정에 쓸 직전 누름은 **대상 키까지** 함께 본다. 그래서 서로 다른 두 부품을 빠르게
    // 연달아 누르는 것은 더블클릭이 아니다 — 그때 사용자가 한 일은 "이것, 그리고 저것"
    // 이지 "이 안으로" 가 아니다.
    // **앵커 도구가 켜진 동안 앵커가 잉크를 이긴다**(SPEC-CANVAS-011 M9).
    //
    // M8 이 붙들어 둔 사실이 이 자리의 결함이었다: 연결선의 끝은 **정확히 그 앵커 자리**
    // 이므로 그 점을 누르면 히트가 잉크라고 답하고(REQ-07-b · 배열 뒤가 곧 위다), 대상이
    // 연결선이면 앵커 갈래가 아무 일도 하지 않는다 — 선이 걸린 앵커는 앵커 도구로 뺄 수
    // 없었다. 도구가 있는데 닿지 못하는 자리가 있는 것이 그 자체로 결함이다.
    //
    // 고치는 근거는 M3'b 가 이미 세웠다: 몸짓의 뜻을 가르는 것은 **대상이 아니라 도구**다
    // ("앵커 도구가 켜진 동안 더블클릭은 언제나 앵커다 — 그룹 부품 위에서도"). 앵커에 무엇이
    // 붙어 있다는 이유로 그 앵커에 닿지 못하는 것은 그 규칙의 구멍이다.
    //
    // **도구에 매인 우선순위다.** 그리기 순서는 한 글자도 바뀌지 않고(배열 순서가 여전히
    // 유일한 z-order), 도구가 꺼져 있거나 고르기 도구일 때의 평범한 고르기에서는 잉크가
    // 종전대로 이긴다 — 연결선을 눌러 고르는 길이 막히면 M9 의 손잡이 자체가 설 수 없다.
    const hitNode = elements.find((el) => el.id === hit.nodeId);
    const overAnchor =
      TOOL_ANCHOR_GESTURE[tool] && hitNode !== undefined && isConnector(hitNode)
        ? anchorHitAt(anchorHosts, point, projection, textWidths, ANCHOR_PICK_SLOP_PX)
        : undefined;

    /** 이 누름이 **겨누는** 것. 앵커가 이겼으면 그 앵커를 든 요소다. */
    const gestureNodeId = overAnchor?.ref.el ?? hit.nodeId;

    // **연타 짝짓기에 쓰는 키는 겨눈 것의 키다** — 히트의 키가 아니다.
    //
    // 두 누름이 같은 몸짓으로 읽히려면 첫 누름이 적은 키를 둘째 누름이 **다시 적어야**
    // 한다. 잉크를 기준으로 적으면 그 조건이 잉크의 경계에서 깨진다: 같은 앵커를 겨눈 두
    // 누름이라도 하나는 선 위(연결선 키)이고 하나는 그 곁(요소 키)일 수 있고 — 집는 오차
    // 9px 은 연타 오차 5px 보다 넓으므로 실제로 일어난다 — 그때 키가 갈려 둘째 누름이
    // 짝을 잃는다. 사용자에게는 "더블클릭이 가끔 안 먹는다" 로만 보이는 그 부류다.
    //
    // 겨눈 것을 적으면 두 누름 모두 그 요소를 적으므로 짝이 유지된다. 반대로 정말 다른
    // 것을 겨눈 두 누름(선 위의 아무 곳 · 그 곁의 앵커)은 키가 갈려 짝지어지지 않으며,
    // 그것이 옳다 — 009 가 "서로 다른 두 부품을 빠르게 연달아 누르는 것은 더블클릭이
    // 아니다" 로 세운 그 규칙과 같은 문장이다.
    //
    // 도구가 꺼져 있으면 `overAnchor` 가 언제나 부재이므로 이 줄은 `frameKey(hit…)` 로
    // 떨어지고, 009 의 그룹 진입은 한 글자도 바뀌지 않는다(AC-26 · AC-27).
    const hitKey =
      overAnchor === undefined ? frameKey(hit.nodeId, hit.partId) : frameKey(gestureNodeId);
    const second = pressedTwice(point, hitKey, event.timeStamp);

    // **앵커 도구가 켜진 동안 두 번째 누름은 언제나 앵커다**(REQ-02' · AC-17 · AC-23).
    //
    // 판정은 바로 위 `second` 하나다 — 이 갈래는 그 값을 **읽을 뿐** 시각도 거리도 다시
    // 재지 않는다(AC-28). 갈리는 것은 그 누름의 **뜻**이지 판정이 아니며, 그 뜻을 가르는
    // 것은 대상이 아니라 **도구**다(`TOOL_ANCHOR_GESTURE`). 대상으로 가르면(도형이면
    // 앵커 · 부품이면 진입) 같은 손짓이 무엇을 할지 눌러 봐야 알게 되고, 그것이 REQ-05-c
    // 가 "두 몸짓은 섞이지 않는다" 로 막으려는 바로 그 상태다.
    //
    // 그래서 대상은 `hit.nodeId` 다: 부품 위였어도 앵커는 **그 그룹**에 선다(부품은 앵커를
    // 내지 않는다 — A3). 도구가 꺼져 있으면 이 줄은 지나가고 009 의 그룹 진입이 한 글자도
    // 바뀌지 않은 채 아래에서 돈다(AC-26 · AC-27).
    if (second && TOOL_ANCHOR_GESTURE[tool]) {
      // **몸짓을 먹었으면 연타 사슬을 끊는다**(빈 자리 누름이 쓰는 그 한 줄이다).
      //
      // 끊지 않으면 셋째 누름이 둘째와 다시 짝을 지어 방금 더한 앵커를 빼고, 넷째가 그것을
      // 도로 더한다 — 손가락 네 번에 결과가 홀짝으로 갈린다. 009 의 그룹 진입이 이 사슬을
      // 끊지 않고도 멀쩡했던 것은 진입이 **멱등**이기 때문이고(같은 부품으로 두 번 들어가면
      // 같은 자리다), 더하기·빼기를 오가는 몸짓에는 그 성질이 없다.
      lastPressRef.current = null;
      // **겨눈 것**에 건다 — 위 `gestureNodeId` 다. 잉크가 이겼으면 종전과 같은
      // `hit.nodeId` 이고, 앵커가 이겼으면 그 앵커를 든 요소다.
      applyAnchorGesture(gestureNodeId, point);
      return;
    }

    // **선 위의 두 번째 누름은 중간점이다**(REQ-05 · AC-67 · AC-71 · M10).
    //
    // 판정은 위 `second` 하나다 — 이 갈래도 그 값을 **읽을 뿐** 시각도 거리도 다시 재지
    // 않는다(AC-72). 앞 갈래와 나란히 서는 것에 뜻이 있다: 한 몸짓(두 번째 누름)의 뜻을
    // 가르는 답이 **표 둘**에 적혀 있고, 그 표는 도구마다 빠짐없이 답을 갖는다
    // (`TOOL_ANCHOR_GESTURE` · `TOOL_POINT_GESTURE`). 조건을 즉석에서 적으면 일곱째 도구가
    // 조용히 한쪽으로 떨어진다.
    //
    // **그룹 부품 위의 더블클릭은 여전히 그룹 진입이다**(REQ-05-c · AC-73). 여기서 갈리는
    // 잣대는 **대상**이며, 부품 위의 누름은 그 그룹을 겨누므로 `hitNode` 가 연결선이 아니다 —
    // 아래 `pressTargetKey` 가 009 그대로 진입을 처리한다. 두 몸짓은 대상이 다르므로 섞이지
    // 않고, 그래서 어느 쪽도 상대의 갈래를 알 필요가 없다.
    if (second && TOOL_POINT_GESTURE[tool] && hitNode !== undefined && isConnector(hitNode)) {
      // **몸짓을 먹었으면 연타 사슬을 끊는다**(앵커 갈래가 쓰는 그 한 줄이다). 끊지 않으면
      // 셋째 누름이 둘째와 다시 짝을 지어 방금 찍은 점을 빼고, 넷째가 그것을 도로 찍는다 —
      // 더하기·빼기를 오가는 몸짓에는 그룹 진입이 가진 멱등성이 없다.
      lastPressRef.current = null;
      applyConnectorPointGesture(hitNode, point);
      return;
    }

    const key = pressTargetKey(hit, selection, second);
    const enteredPart = isPartKey(key);

    // **부품 선택은 언제나 하나이고, 그 그룹과 함께 서지 않는다**(REQ-01-a · REQ-01-b).
    // modifier 를 부품 경로에서 흘려보내는 것에 뜻이 있다: 부품을 무리에 더할 수 있게 하면
    // "서로 다른 그룹의 부품 둘" 이라는 뜻이 정의되지 않은 상태가 만들어지고(A1), 8핸들이
    // 두 상자에 서는 화면이 그 뒤를 따른다. 그룹을 고르는 경로는 최상위 원소와 같으므로
    // modifier 가 종전 그대로 산다.
    const picked = enteredPart
      ? new Set([key])
      : nextSelection(selection, key, pressTogglesSelection);
    if (picked !== selection) setSelection(picked);
    if (pressTogglesSelection && !enteredPart) return;

    if (!(stage.width > 0) || !(stage.height > 0)) return;

    // 무리 이동: 같은 캔버스 단위 델타를 선택된 **모든** 요소에 더한다. 상한이 없으므로
    // `clampGroupDelta` 는 쓰지 않는다 — 아무 일도 하지 않는 호출은 읽는 사람에게
    // 상한이 있다고 거짓말한다(spec.md §드래그 기구).
    //
    // 부품은 `selectedNodes` 가 **캔버스 단위 의사 노드**로 풀어 주므로, 잡는 순간의 기하도
    // 이동 산술도 최상위 요소와 한 글자도 다르지 않다. 갈리는 자리는 쓰기 하나뿐이다
    // (`patchGeometryByKey`).
    const bases: DragBase[] = selectedNodes(elements, picked).map((el) => ({
      nodeId: el.id,
      geometry: el.geometry,
    }));
    if (bases.length === 0) return;

    // 히트는 `elements` 를 훑어 나온 키이므로 그 노드는 반드시 있다(`canvasHitTest` 는
    // 배열 밖의 id 를 만들지 않는다). 없을 수 없는 경우에 가드를 두면 그 가드는 검증되지
    // 않은 채 남아 읽는 사람에게 "없을 수도 있다" 고 거짓말한다.
    const anchorEl = nodeForKey(elements, key)!;

    dragRef.current = {
      mode: 'move',
      pointerId: event.pointerId,
      origin: point,
      frame,
      bases,
      // 선택 외곽선이 두르는 **그 상자**를 캔버스 단위로 되돌린 값이다 — 화면에 보이는
      // 테두리와 격자에 붙는 중심이 다른 상자에서 나오면 "보이는 것과 다른 곳에 붙는다"
      // 가 된다. 잡는 순간 한 번만 되돌리고 드래그 중에는 다시 재지 않는다(`frame` 규율).
      anchor: unprojectBox(outlineBox(anchorEl, projection, textWidths), projection),
      // 드래그 도중에 다시 재지 않는다(위 `dropRect` 주석). 도크가 없으면 `null` 이다.
      dropRect: dropZone?.getBoundingClientRect() ?? null,
    };
    // 포인터가 스테이지를 벗어나도 이벤트가 계속 오게 한다. jsdom 에는 없는 API 라
    // 존재를 확인하고 부른다(§품질 게이트).
    host.setPointerCapture?.(event.pointerId);
  };

  /**
   * 핸들에서 시작하는 드래그.
   *
   * **이벤트를 여기서 끊는다.** 끊지 않으면 루트의 몸통 히트 테스트가 이어서 돌아 핸들 뒤에
   * 있는 도형을 새로 고르고 이동 드래그를 시작한다 — 잡은 것은 손잡이인데 옮겨지는 결과가
   * 된다. 선택은 건드리지 않는다: 핸들은 **이미 골라진 것**에만 뜨므로 다시 고를 것이 없다.
   *
   * 포인터를 잡는 대상도, 좌표의 원점도 **루트**다(위 `rootRef` 주석). 핸들 자신을 원점으로
   * 삼으면 잡은 자리가 핸들 안 어디냐에 따라 원점이 달라진다.
   */
  const startHandleDrag = (
    el: CanvasNode,
    handle: CanvasHandleId,
    event: React.PointerEvent<HTMLButtonElement>,
  ): void => {
    event.preventDefault();
    event.stopPropagation();

    // 핸들이 DOM 에 있다는 것이 곧 루트가 마운트되어 있다는 뜻이다(핸들은 루트의 자식이다).
    const host = rootRef.current!;
    const frame = pointerFrameOf(host.getBoundingClientRect(), stage);
    const next = handleDragState(el, handle, {
      pointerId: event.pointerId,
      origin: stagePoint(event.clientX, event.clientY, frame),
      frame,
    });
    if (next === null) return;

    dragRef.current = next;
    pendingRef.current = null;
    host.setPointerCapture?.(event.pointerId);
  };

  /**
   * 연결선 손잡이에서 시작하는 드래그 (SPEC-CANVAS-011 M9 · REQ-07).
   *
   * 위 `startHandleDrag` 와 **같은 세 줄**을 지킨다 — 이벤트를 여기서 끊고(끊지 않으면
   * 루트의 몸통 히트 테스트가 이어서 돌아 손잡이 뒤의 것을 새로 고르거나 빈 자리의
   * 사각형을 세운다), 좌표의 원점은 루트이며, 선택은 건드리지 않는다(손잡이는 이미
   * 골라진 연결선에만 뜬다).
   *
   * 갈리는 것은 `handleDragState` 를 지나지 않는다는 점 하나다. 그 함수는 **종류와 핸들
   * 이름을 맞춰 보고 잡는 순간의 기하를 뜬다** — 연결선에는 맞춰 볼 이름도(고정 표에 없다)
   * 뜰 기하도(`geometry` 가 없다) 없으므로, 그 함수에 연결선 갈래를 더하면 두 `null` 이
   * 세 개가 되고 그 셋은 서로 다른 뜻을 갖는다.
   */
  const startConnectorHandleDrag = (
    connector: ConnectorElement,
    handle: ConnectorHandle,
    event: React.PointerEvent<HTMLButtonElement>,
  ): void => {
    event.preventDefault();
    event.stopPropagation();

    const host = rootRef.current!;
    const frame = pointerFrameOf(host.getBoundingClientRect(), stage);
    const origin = stagePoint(event.clientX, event.clientY, frame);

    // **중간점 손잡이의 두 번째 누름은 그 점을 뺀다**(REQ-05-b · AC-71).
    //
    // 이 갈래가 여기 있는 까닭은 손잡이가 **진짜 단추**이기 때문이다(REQ-01). 단추는 제
    // 누름을 먹으므로 루트의 그 갈래가 이 자리를 영영 보지 못하고, 그러면 화면에 서 있는
    // 점을 뺄 길이 없다 — 도구가 있는데 닿지 못하는 자리가 있는 것이 그 자체로 결함이다
    // (M9 가 "선이 걸린 앵커를 뺄 수 없다" 를 같은 문장으로 고쳤다).
    //
    // 그렇다고 여기서 **판정을 새로 짓지 않는다.** 묻는 일도 적는 일도 위 `pressedTwice`
    // 하나를 지나며, 키는 그 연결선이다 — 첫 누름이 잉크에 닿고 둘째가 손잡이에 닿아도
    // (연타 오차 5px 은 손잡이보다 좁다) 두 누름이 같은 키를 적으므로 짝이 유지된다.
    //
    // 대상으로 가르지 않고 **표**를 보는 것도 루트 갈래와 같은 규율이다. 오늘 앵커 도구는
    // 손잡이에서 포인터를 걷어 두므로(아래 렌더 §앵커 도구가 켜진 동안) 이 자리에 닿지
    // 않지만, 닿는 날 두 갈래가 서로 다른 답을 내면 "빼기는 되는데 만들기는 안 된다" 가 된다.
    //
    // 묻는 일은 **조건보다 먼저**다. 뒤에 두면 끝점 손잡이의 누름이 장부에 적히지 않아,
    // 그 누름을 사이에 둔 두 누름이 서로 짝을 짓는다 — 이 표면이 본 누름은 전부 장부를
    // 지나야 그 뒤섞임이 없다(루트 갈래가 같은 차례로 적는다).
    const second = pressedTwice(origin, frameKey(connector.id), event.timeStamp);
    if (second && TOOL_POINT_GESTURE[tool] && handle.kind === 'mid') {
      // 루트 갈래와 같은 이유로 사슬을 끊는다 — 빼기와 찍기를 오가는 몸짓은 멱등이 아니다.
      lastPressRef.current = null;
      commitConnector(connector, removePointAt(connector, handle.index));
      // 뺀 점을 끌 수는 없다. 드래그를 시작하지 않고 돌아간다.
      return;
    }

    dragRef.current = {
      mode: 'connectorPoint',
      pointerId: event.pointerId,
      origin,
      frame,
      nodeId: connector.id,
      handle,
    };
    pendingRef.current = null;
    host.setPointerCapture?.(event.pointerId);
  };

  /**
   * 드롭 존 강조를 **바뀔 때만** 갈아 끼운다. 같은 값을 다시 넣으면 React 가 렌더를
   * 건너뛰기는 하나, 그 전에 갱신을 예약하는 비용은 매 이동마다 든다(AC-E4 의 규율).
   */
  const setDropHighlight = (next: boolean): void => {
    if (dropActiveRef.current === next) return;
    dropActiveRef.current = next;
    setDropActive(next);
  };

  /**
   * 펜 커서 판정을 **바뀔 때만** 갈아 끼운다 — 위 `setDropHighlight` 와 같은 규율이다
   * (AC-E4). 이동마다 같은 값을 넣으면 렌더는 건너뛰어도 갱신을 예약하는 비용은 매번 든다.
   */
  const setPenHovering = (next: boolean): void => {
    if (penHoverRef.current === next) return;
    penHoverRef.current = next;
    setPenHover(next);
  };

  /**
   * 이번 이동이 커서를 바꾸는가 (사용자 신고 2026-09-16).
   *
   * ## 판정은 **누름이 읽는 그 표**를 읽는다
   *
   * 펜이 뜨는 조건은 `TOOL_CONNECTOR_ROUTE[tool] !== null` 이고, 그것은 누름이 잇기를
   * 시작할지 정하는 **바로 그 줄**이다(`handlePointerDown`). 두 자가 갈리면 화면이 지키지
   * 못할 약속을 하게 된다 — 펜을 보여 놓고 눌렀더니 고르기가 되는 자리다(위험 R1 의 규율을
   * 커서에 적용한 것이다).
   *
   * 그래서 **앵커 도구에서는 펜이 뜨지 않는다.** 그 도구도 앵커를 보여 주지만
   * (`TOOL_SHOWS_ANCHORS`) 그 점을 눌러 나는 것은 선이 아니라 앵커이고(REQ-02'), 그 표는
   * `null` 을 낸다. 보이는 점마다 펜을 씌웠다면 앵커를 세우려는 손에게 화면이 선을 약속했을
   * 것이다. 두 물음이 다르다는 사실은 `canvasTools.ts` 가 표를 둘로 나눠 이미 적어 두었다.
   *
   * ## 잡는 자는 **앵커를 집는 그 함수**다
   *
   * `anchorHitAt` 은 누름이 지나는 그 함수이고 오차도 같은 `ANCHOR_PICK_SLOP_PX` 다. 여기서
   * 자리를 새로 셈하면 "펜이 뜨는 띠" 와 "실제로 집히는 띠" 가 두 벌이 된다.
   *
   * ## 긋는 동안에는 **묻지 않는다** — 커서는 몸짓이 끝날 때까지 펜이다
   *
   * 앵커에서 눌러 빈 자리로 끌고 가는 것이 이 몸짓의 전부이므로, 자리를 다시 물으면 앵커를
   * 벗어나는 순간 커서가 기본으로 돌아간다. 긋는 중이라는 사실이 곧 "여기는 선을 긋는
   * 자리" 이며, 그 사실은 `connectorDraw` 가 이미 들고 있다.
   */
  const updatePenHover = (event: React.PointerEvent<HTMLDivElement>): void => {
    if (connectorDraw !== null) return;
    if (TOOL_CONNECTOR_ROUTE[tool] === null) return;
    const frame = pointerFrameOf(event.currentTarget.getBoundingClientRect(), stage);
    const point = stagePoint(event.clientX, event.clientY, frame);
    setPenHovering(
      anchorHitAt(anchorHosts, point, projection, textWidths, ANCHOR_PICK_SLOP_PX) !== undefined,
    );
  };

  /**
   * 사각형이 지금 감싸는 것들 — **바탕과의 합집합**이다.
   *
   * 투영은 `outlineBox` 를 지난다: 판정하는 상자와 골라진 뒤 **화면에 뜨는 그 상자**가
   * 같은 함수에서 나오므로 둘이 갈라질 수 없다(위험 R1 의 규율 그대로다).
   */
  const marqueeNext = (state: MarqueeState, point: PxPoint): CanvasSelection =>
    marqueeSelection(
      marqueeCandidates(elements, (el) => outlineBox(el, projection, textWidths)),
      marqueeRect(state.origin, point),
      state.base,
    );

  const handlePointerMove = (event: React.PointerEvent<HTMLDivElement>): void => {
    // **커서 판정이 아래 세 갈래 **앞**에 선다** — 어느 갈래가 이 이동을 먹든 커서는
    // 갱신되어야 한다(위 `updatePenHover`). 갈래 안에 넣으면 그 갈래가 늘 때마다 같은
    // 한 줄을 베껴 넣어야 하고, 한 곳에서 빠지는 날 "어떤 몸짓 뒤에는 커서가 옛 모양에
    // 남는" 자리가 생긴다.
    updatePenHover(event);

    // **팬이 가장 먼저다.** 팬을 시작하는 누름은 둘(짚은 채의 누름 · 빈 자리의 맨손
    // 누름)이고 그 둘은 아래 셋을 시작하는 누름과 갈리므로 배타적이며, 순서가 뜻을
    // 바꾸지는 않는다 — 다만 먼저 끊어 두면 아래 세 경로가 팬을 모른 채로 남는다
    // (마키 · 긋기가 같은 이유로 앞에 섰다).
    //
    // **프레임을 예약하지 않는다** — 고 말할 수 없는 유일한 갈래다. 팬은 상자의 자리를
    // 바꾸므로 표면이 받는 `geometry` 가 실제로 달라지고, 그래서 프레임이 한 장 돈다.
    // 그 한 장은 새 깨우기 경로가 아니라 **종전의 props 변경 경로**이며, 배율 변경 ·
    // 리사이즈와 같은 부류다(REQ-05 · AC-E4).
    const panDrag = panRef.current;
    if (panDrag !== null && event.pointerId === panDrag.pointerId) {
      event.preventDefault();
      movePan(event.clientX, event.clientY);
      return;
    }

    // **긋기가 그다음이다**(SPEC-CANVAS-011 M8). 마키·드래그와 배타적이므로 순서가 뜻을
    // 바꾸지는 않으나(셋은 서로 다른 누름에서만 시작된다), 먼저 끊어 두면 아래 두 경로가
    // 잇기를 모른 채로 남는다 — 마키가 같은 이유로 드래그 앞에 섰다.
    //
    // **프레임을 예약하지 않는다**(AC-E4). 긋는 동안 기하는 한 글자도 쓰이지 않으므로
    // `CanvasSurface` 가 받는 props 가 그대로이고, 001 이 지은 유휴 정지가 유지된다.
    if (connectorDraw !== null && event.pointerId === connectorDraw.pointerId) {
      event.preventDefault();
      const point = stagePoint(event.clientX, event.clientY, connectorDraw.frame);
      // **자유선만 궤적을 받는다**(M11 · REQ-06). 표를 보고 갈리므로 갈래가 하나 늘면
      // 컴파일러가 그 표를 가리킨다. 상한에 닿으면 그 함수가 받은 배열을 그대로 돌려주고,
      // 몸짓은 이 줄을 지나 끝까지 이어진다(AC-77).
      if (ROUTE_TRACES_TRAIL[connectorDraw.route]) {
        trailRef.current = takeFreehandSample(
          trailRef.current,
          unprojectPoint(point, projection),
        );
      }
      setConnectorDraw({ ...connectorDraw, point });
      return;
    }
    // **영역 선택이 그다음이다.** 둘은 배타적이므로(마키는 히트가 없을 때만, 이동 드래그는
    // 있을 때만 시작된다) 순서가 뜻을 바꾸지는 않으나, 먼저 끊어 두면 아래 이동 경로가
    // 마키를 모른 채로 남는다 — 그 경로는 REQ-08 이 "한 줄도 바뀌지 않는다" 로 이름
    // 적어 둔 자리다.
    if (marquee !== null && event.pointerId === marquee.pointerId) {
      event.preventDefault();
      const point = stagePoint(event.clientX, event.clientY, marquee.frame);
      setMarquee({ ...marquee, point });
      // **프레임을 예약하지 않는다**(REQ-05 · AC-E4). 마키는 기하를 한 글자도 쓰지 않으므로
      // `CanvasSurface` 가 받는 props 가 그대로이고, 따라서 001 이 지은 유휴 정지가 유지된다.
      // 이동 드래그가 rAF 로 모이는 것은 그쪽이 **config 를 쓰기** 때문이지 포인터라서가
      // 아니다 — 여기서 그 통로(`pendingRef`·`frameRef`)를 빌리면 쓸 것이 없는데도 루프가 돈다.
      const picked = marqueeNext(marquee, point);
      if (picked !== selection) setSelection(picked);
      return;
    }
    const drag = dragRef.current;
    if (drag === null || event.pointerId !== drag.pointerId) return;
    event.preventDefault();
    pendingRef.current = {
      point: stagePoint(event.clientX, event.clientY, drag.frame),
      shift: event.shiftKey,
    };
    // 손이 서랍 위에 있는가 — **클라이언트 좌표 containment** 다(불변식 J7). 잡는 순간에
    // 재 둔 상자를 쓰므로 이동마다 다시 재지 않는다. 몸통 이동이 아닐 때는 강조하지
    // 않는다: 손잡이로 크기를 조절하는 중에 서랍이 켜지면 화면이 거짓말을 한다.
    if (drag.mode === 'move') {
      setDropHighlight(pointInRect(drag.dropRect, event.clientX, event.clientY));
    }
    // 한 프레임 사이에 이벤트가 열 번 와도 쓰기는 한 번이다(AC-E4).
    if (frameRef.current === null) frameRef.current = requestAnimationFrame(flush);
  };

  /** 잡고 있던 포인터를 놓아준다. 잡은 적이 없으면 브라우저가 예외를 던지므로 확인한다. */
  const releaseCapture = (host: HTMLDivElement, pointerId: number): void => {
    if (host.hasPointerCapture?.(pointerId)) host.releasePointerCapture(pointerId);
  };

  /**
   * 고른 것들의 **사본**을 서랍에 넣는다 — **저장 경로는 이 함수 하나뿐**이다(J11 · AC-06).
   *
   * 끌어 넣기와 단추가 같은 이 함수를 부르고, 고를 요소를 가리는 규칙(`selection`)도 하나다.
   * 두 벌이 되면 "끌어 넣은 것과 단추로 넣은 것이 다르다" 가 생기며, 그 차이는 서랍을 열어
   * 보기 전까지 드러나지 않는다.
   *
   * 배열을 인자로 받는 것은 끌어 넣기 때문이다 — 그쪽은 **되돌린 뒤의** 배열에서 떠야
   * 서랍에 든 것과 캔버스에 남은 것이 같아진다. 선택은 지워진 요소의 id 를 들고 있을 수
   * 있으므로(목록 편집기에서 지우면 그렇다) 순회는 배열 쪽을 돈다.
   */
  const saveSelectionToScratchpad = (source: readonly CanvasNode[]): void => {
    // **그룹은 서랍에 들어가지 않는다**(SPEC-CANVAS-004 M6). 서랍의 저장 형상은
    // `CanvasElement[]` 이고 004 는 그것을 한 바이트도 바꾸지 않기로 했다(가정 A20) —
    // 그룹을 담으려면 저장 형상이 넓어져야 하고, 그것은 이 SPEC 이 명시적으로 금지한
    // 변경이다. 담는 길은 M14 의 "그룹으로 놓기" 반대편에 따로 서며, 그때까지는 빠진다.
    // **감추지 않되 새 문구도 만들지 않는다**: 그룹만 골라 두고 저장을 누르면 남는 요소가
    // 0 개이므로 서랍이 이미 가진 그 안내(`saveEntry` 의 `empty`)가 그대로 뜬다. 같은
    // 사실을 두 문구로 말하면 어느 쪽이 참인지 화면이 답하지 못한다.
    saveEntry(source.filter((el): el is CanvasElement => !isGroup(el) && selection.has(el.id)));
  };

  /**
   * 서랍 위에서 손을 뗐다 — **복사이지 이동이 아니다**(REQ-03 · AC-05).
   *
   * 끌던 요소의 기하를 **드래그 시작값**(`drag.bases`)으로 되돌린다. 되돌리지 않으면
   * 서랍에 넣었을 뿐인데 캔버스의 도형이 도크 쪽으로 밀려나 있다. 되돌림 쓰기도
   * `patchNodeGeometry` 를 지난다 — 기하 쓰기 통로는 여전히 하나다(REQ-03 · 불변식 J2).
   *
   * 프레임 대기 중인 이동은 **흘리지 않고 버린다**. 어차피 되돌릴 값이므로 한 번 쓰고
   * 되돌리면 config 쓰기가 헛되이 두 번이 된다(AC-E4).
   */
  const dropToScratchpad = (drag: MoveDrag): void => {
    if (frameRef.current !== null) {
      cancelAnimationFrame(frameRef.current);
      frameRef.current = null;
    }
    dragRef.current = null;
    pendingRef.current = null;
    setDropHighlight(false);

    const { elements: els, onElementsChange: emit } = latestRef.current;
    let next: CanvasNode[] = [...els];
    for (const base of drag.bases) {
      next = patchNodeGeometry(next, base.nodeId, base.geometry);
    }
    saveSelectionToScratchpad(next);
    emit(next);
  };

  /**
   * 영역 선택을 끝낸다. **선택은 다시 세지 않는다** — 마지막 이동이 이미 확정해 두었고,
   * 뗌 좌표로 한 번 더 세면 그 사이 좌표가 다른 경우에만 결과가 갈리는 두 번째 규칙이 된다.
   * 움직임이 한 번도 없었으면(감싼 면적이 0) 합집합이 바탕 그대로이므로 선택은 불변이다.
   */
  const finishMarquee = (host: HTMLDivElement, pointerId: number): void => {
    releaseCapture(host, pointerId);
    setMarquee(null);
  };

  const handlePointerUp = (event: React.PointerEvent<HTMLDivElement>): void => {
    // **Space 를 뗐어도 팬은 손을 뗄 때까지 이어진다**(가정: 몸짓의 뜻은 잡는 순간 한 번만
    // 정해진다 — `handleDragState` §머리말의 그 규율). 중간에 끊으면 그림이 손 밑에서
    // 멈춰 서고, 사용자는 제가 무엇을 잘못 눌렀는지 알 수 없다. 도해 도구가 모두 그렇다.
    const panDrag = panRef.current;
    if (panDrag !== null && event.pointerId === panDrag.pointerId) {
      event.preventDefault();
      finishPan(event.currentTarget, event.pointerId);
      // **움직이지 않은 팬은 클릭이고, 빈 자리의 클릭은 선택을 푼다**(SPEC-CANVAS-011).
      //
      // 009 이전부터 참이던 그 뜻이 서는 유일한 자리다 — 누름에서 비우면 끄는 동안에도
      // 비워지므로(위 `beginPan` 갈래 주석), 뜻은 여기까지 미뤄져야 한다. 어느 누름이 그
      // 뜻을 들고 왔는지는 잡는 순간 한 번 적혔다: 빈 자리 맨손 누름만 참이다.
      if (
        panDrag.clearsSelectionOnClick &&
        panWasClick(panDrag, event.clientX, event.clientY) &&
        selection.size > 0
      ) {
        setSelection(EMPTY_SELECTION);
      }
      return;
    }
    if (connectorDraw !== null && event.pointerId === connectorDraw.pointerId) {
      event.preventDefault();
      releaseCapture(event.currentTarget, event.pointerId);
      finishConnectorDraw(
        connectorDraw,
        stagePoint(event.clientX, event.clientY, connectorDraw.frame),
      );
      return;
    }
    if (marquee !== null && event.pointerId === marquee.pointerId) {
      event.preventDefault();
      finishMarquee(event.currentTarget, event.pointerId);
      return;
    }
    const drag = dragRef.current;
    if (drag === null || event.pointerId !== drag.pointerId) return;
    event.preventDefault();
    releaseCapture(event.currentTarget, event.pointerId);
    // **놓임 판정은 여기다.** 오버레이가 누름에서 루트에 포인터를 잡으므로, 손이 도크 위에
    // 있어도 이 사건은 여기로 온다 — 드롭 존에 처리자를 달면 한 번도 불리지 않는다(실측).
    // 판정은 클라이언트 좌표 containment 이며 `projection` 도 `stage` 도 보지 않는다
    // (불변식 J7 · AC-E7). 도크가 없으면 `dropRect` 가 `null` 이라 이 갈래는 아예 돌지 않는다.
    if (drag.mode === 'move' && pointInRect(drag.dropRect, event.clientX, event.clientY)) {
      dropToScratchpad(drag);
      return;
    }
    setDropHighlight(false);
    finishDrag({
      point: stagePoint(event.clientX, event.clientY, drag.frame),
      shift: event.shiftKey,
    });
  };

  const handlePointerCancel = (event: React.PointerEvent<HTMLDivElement>): void => {
    // 끊긴 팬은 **마지막 자리를 그대로 둔다** — 이동 드래그의 취소가 마지막 유효 위치를
    // 확정하는 것과 같은 규칙이다. 되돌리면 브라우저·OS 가 끊었을 뿐인데 사용자가 옮겨
    // 놓은 시야가 사라진다.
    const panDrag = panRef.current;
    if (panDrag !== null && event.pointerId === panDrag.pointerId) {
      // **끊긴 팬은 클릭이 아니다**(뗌 갈래와 갈리는 자리다 — SPEC-CANVAS-011). 브라우저·
      // OS 가 가져간 몸짓은 "여기를 눌렀다 뗐다" 는 뜻이 아니므로 선택을 풀지 않는다 —
      // 끊긴 긋기가 아무것도 만들지 않는 그 규칙과 같은 문장이다.
      finishPan(event.currentTarget, event.pointerId);
      return;
    }
    // **끊긴 긋기는 아무것도 만들지 않는다.** 이동 드래그의 취소가 마지막 유효 위치를
    // 확정하는 것과 갈리는 자리이며, 근거는 그쪽의 그 근거와 같다: 확정하면 사용자가 한
    // 일이 남아야 하는데, 잇기는 **놓는 자리가 정해져야** 비로소 한 일이 된다. 브라우저·
    // OS 가 끊은 몸짓은 "여기에 놓았다" 는 뜻이 아니다(서랍이 같은 문장을 쓴다).
    if (connectorDraw !== null && event.pointerId === connectorDraw.pointerId) {
      releaseCapture(event.currentTarget, event.pointerId);
      setConnectorDraw(null);
      // 끊긴 몸짓의 궤적도 함께 버린다 — 남기면 다음 자유선이 이 손짓을 물려받는다.
      trailRef.current = [];
      return;
    }
    // 이동 드래그의 취소가 **마지막 유효 위치를 확정하는** 것과 같은 규칙이다 — 되돌리면
    // 사용자가 한 일이 소리 없이 사라진다. 사각형만 거둔다.
    if (marquee !== null && event.pointerId === marquee.pointerId) {
      finishMarquee(event.currentTarget, event.pointerId);
      return;
    }
    const drag = dragRef.current;
    if (drag === null || event.pointerId !== drag.pointerId) return;
    releaseCapture(event.currentTarget, event.pointerId);
    setDropHighlight(false);
    // 취소는 좌표를 주지 않는다 — **마지막 유효 위치**를 그대로 확정한다. 서랍에도 넣지
    // 않는다: 브라우저·OS 가 끊은 몸짓은 "여기에 놓았다" 는 뜻이 아니다.
    finishDrag(null);
  };

  /**
   * 고른 것들을 캔버스 단위 델타만큼 옮긴다 — **드래그의 키보드 등가물**이다(T15 · REQ-04).
   *
   * 쓰기는 드래그와 **같은 통로**를 지난다: `moveGeometry` 로 기하를 옮기고
   * `patchNodeGeometry` 로 배열에 써 넣는다(REQ-06). 두 번째 이동 규칙을 만들면 "끌었을
   * 때와 방향키로 옮겼을 때가 다르다" 가 생기고, 그것은 화면에서만 드러난다.
   *
   * **clamp 하지 않는다**(가정 A5 · AC-E5) — 방향키로 스테이지 밖까지 나갈 수 있어야
   * 수치 입력과 등가다.
   *
   * 선택에는 이미 지워진 요소의 id 가 남아 있을 수 있으므로(목록 편집기에서 지우면 그렇게
   * 된다) 순회는 **배열** 쪽을 돈다. 옮길 것이 하나도 없으면 쓰지 않는다 — 헛된 config
   * 쓰기는 곧 헛된 렌더 프레임이다(AC-E4).
   */
  const nudgeSelection = (delta: CanvasDelta): boolean => {
    // **드래그와 같은 해석기를 지난다**(`selectedNodes`). 부품이 골라져 있으면 여기서도
    // 캔버스 단위 의사 노드가 나오므로, 끌었을 때와 방향키로 옮겼을 때가 갈라질 수 없다.
    const targets = selectedNodes(elements, selection);
    if (targets.length === 0) return false;
    let next: CanvasNode[] = [...elements];
    for (const el of targets) {
      next = patchGeometryByKey(next, el.id, moveGeometry(el.geometry, delta));
    }
    onElementsChange(next);
    return true;
  };

  /**
   * 고른 것들을 지운다 — **규칙은 `canvasEditArrange.removeNodesWithConnectors` 한 곳에
   * 있다** (SPEC-CANVAS-010 · 011 M12). 목록 편집기의 휴지통이 같은 함수를 지나므로
   * "목록에서 지웠는가 캔버스에서 지웠는가" 에 따라 결과가 갈릴 수 없다 — 지워지는 요소를
   * 가리키던 연결선이 함께 가는 일(REQ-08)도 그래서 두 입구에서 같다.
   *
   * **한 번만 방출한다.** 고른 것마다 한 번씩 부르면 중간 배열이 프레임마다 화면에 서고
   * config 쓰기가 N 번이 된다(AC-E4 의 규율). 그 함수가 집합 하나를 받는 것이 그래서다.
   *
   * **선택을 비운다.** 지운 뒤에도 그 id 들이 남아 있으면 서랍 저장·묶기 단추가 없는
   * 것을 가리킨 채 켜져 있고, 사용자에게는 눌러도 아무 일이 없는 단추로 보인다.
   *
   * **확인을 묻지 않는다.** 이 저장소의 판정은 "잃을 것이 없으면 묻지 않는다" 이고
   * (`CanvasGroupTools` — 그룹 해제는 규칙 행이 버려질 때만 묻는다), 지우기가 없애는
   * 것은 윤곽선이 둘린 **바로 그것들**이라 보이지 않게 잃는 것이 없다. 무엇보다 목록
   * 편집기의 휴지통이 이미 묻지 않고 지우므로, 키에만 확인을 달면 "설정에서는 물어보는데
   * 목록에서는 그냥 지워진다" 가 된다. 되돌리기가 없다는 사실(SPEC-CANVAS-008 위험)은
   * 확인 대화가 아니라 **오는 길을 좁히는 것**으로 다룬다 — 글자를 치는 칸에서 누른 키는
   * 이 층에 닿지 않는다(`CanvasEditDock` 이 제 자리에서 끊는다).
   *
   * 돌려주는 값은 **지웠는가** 다. 지우지 않았으면 이벤트를 소비하지 않는다.
   */
  const deleteSelection = (): boolean => {
    const next = removeNodesWithConnectors(elements, selection);
    if (next === elements) return false;
    onElementsChange([...next]);
    setSelection(EMPTY_SELECTION);
    return true;
  };

  /**
   * 방향키 미세 이동 · Shift+방향키 한 격자 칸(T15 · AC-08) · Delete·Backspace 지우기.
   *
   * **우리가 실제로 옮겼을 때에만 이벤트를 소비한다.** 고른 것이 없거나 스테이지를 아직
   * 재지 못했으면 그대로 흘려보낸다 — 아무 일도 하지 않으면서 브라우저의 스크롤·초점
   * 이동을 막으면 그것이 곧 접근성 결함이다(AC-E3 이 포인터에 대해 세운 규칙과 같은 뜻).
   *
   * Ctrl/Cmd/Alt 가 붙은 방향키는 브라우저·OS 의 다른 조작이므로 손대지 않는다. Shift 만
   * 우리 것이며, 그 뜻은 드래그에서와 달리 "종횡비" 가 아니라 **한 격자 칸**이다 —
   * 드래그에는 이미 격자 붙임 토글이 있고 방향키에는 없기 때문이다.
   *
   * 끌고 있는 동안에는 **손이 이긴다.** 같은 요소를 포인터가 절대 자리로 다시 잡는 중에
   * 방향키가 상대 이동을 얹으면 둘이 서로를 덮어써 튄다.
   */
  const handleKeyDown = (event: React.KeyboardEvent<HTMLDivElement>): void => {
    // `enabled` 를 다시 보지 않는다 — 꺼져 있으면 이 층이 DOM 에 아예 없어 키가 닿을 길이
    // 없다. 검증되지 않는 가드는 읽는 사람에게 "닿을 수도 있다" 고 거짓말한다.
    if (event.ctrlKey || event.metaKey || event.altKey) return;
    // **두 갈래가 함께 쓰는 문턱이다.** 끌고 있는 동안에는 손이 이기고(아래 머리말),
    // 고른 것이 없으면 옮길 것도 지울 것도 없다. 갈래마다 따로 두면 한쪽만 고칠 수 있다.
    if (dragRef.current !== null) return;

    // **팬 갈래가 선택 문턱 앞에 선다** — 팬은 고른 것이 없을 때가 오히려 예사이므로,
    // 아래 문턱 뒤에 두면 가장 흔한 경우에 동작하지 않는다(사용자 신고 2026-09-16).
    //
    // **표적을 보지 않는다**(AC-10 (BA) 의 형상 가드). 컨트롤의 키를 끊는 자리는 이
    // 함수가 아니라 **컨트롤 쪽**이며, 그 자리는 이미 서 있다: 도크도 떠 있는 배율 줄도
    // 제 뿌리에서 `stopPropagation` 한다("도크에서 누른 키는 도크의 것"). 여기에 표적
    // 가드를 세우면 그 문장이 두 벌이 되고, 칸이 늘 때마다 한쪽이 조용히 낡는다.
    //
    // 루트 **안쪽**의 초점 가능한 컨트롤은 손잡이 둘(8핸들 · 연결선 손잡이)뿐이고 둘 다
    // `onPointerDown` 말고는 아무 처리자가 없다 — 그 자리의 Space 는 오늘 **아무 일도
    // 하지 않으므로** 여기서 가져가도 빼앗는 것이 없다. 언젠가 손잡이가 Space 로 하는
    // 일을 갖게 된다면 끊을 자리는 여기가 아니라 **그 손잡이**다(도크가 세운 그 규칙).
    if (event.key === SPACE_KEY) {
      setSpaceHeld(true);
      // **여기서만 소비한다.** 짚는 일이 실제로 무언가를 켰을 때이며, 그러지 않으면
      // 브라우저가 페이지를 한 칸 스크롤해 편집하던 자리가 화면 밖으로 밀린다.
      event.preventDefault();
      return;
    }
    if (spaceHeld) {
      // **Space+방향키는 끌기의 키보드 등가물이다.** 없으면 팬은 포인터 전용 기능이 되고,
      // 이 층이 T15 · REQ-01 에서 "키보드로 닿는다" 로 적어 둔 규율에 구멍이 하나 남는다.
      // 부호는 **끄는 것과 같다** — 오른쪽 키는 그림을 오른쪽으로 민다.
      const step = ARROW_STEPS[event.key];
      if (step === undefined) return;
      const next = clampWorkspacePan(
        {
          x: workspacePan.x + step.x * PAN_ARROW_PX,
          y: workspacePan.y + step.y * PAN_ARROW_PX,
        },
        workspaceSize,
      );
      // **실제로 옮겼을 때에만 소비한다** — 이미 끝에 닿았으면 그대로 흘려보낸다(이 함수가
      // 방향키와 지우기에 대해 세운 그 규율).
      if (next.x === workspacePan.x && next.y === workspacePan.y) return;
      setWorkspacePan(next);
      event.preventDefault();
      return;
    }

    if (selection.size === 0) return;

    // **지우기가 먼저다.** Shift 를 배제하지 않는다 — 방향키의 Shift 는 "한 격자 칸" 이라는
    // 제 뜻이 있지만 지우기에는 더 셀 것이 없고, 배제하면 Shift 를 짚은 채 누른 Delete 가
    // 조용히 아무 일도 하지 않는다.
    if (DELETE_KEYS.has(event.key)) {
      if (!deleteSelection()) return;
      // 여기까지 왔다는 것은 실제로 지웠다는 뜻이다 — 그때에만 Backspace 의 뒤로 가기를 막는다.
      event.preventDefault();
      return;
    }

    const step = ARROW_STEPS[event.key];
    if (step === undefined) return;

    // 두 갈래 다 **캔버스 단위 정수**라 나눌 것도 환산할 것도 없다. 그래서 스테이지를
    // 아직 재지 못한 순간에도 뜻이 성립하며, 무엇보다 "한 격자 칸" 이 화면에 그려진 그
    // 칸과 **정의상** 같다 — 그리는 쪽도 같은 `gridStep` 에서 나온 칸을 그린다.
    const amount = event.shiftKey ? gridStep : NUDGE_UNITS;
    const delta: CanvasDelta = { dx: step.x * amount, dy: step.y * amount };

    if (!nudgeSelection(delta)) return;
    // 여기까지 왔다는 것은 실제로 옮겼다는 뜻이다 — 그때에만 스크롤을 막는다.
    event.preventDefault();
  };

  /**
   * Space 를 뗀다 — 커서가 손 모양에서 돌아오고, 다음 누름이 다시 고르기가 된다.
   *
   * **끌고 있는 팬은 끊지 않는다**(`handlePointerUp` 머리말). 여기서 하는 일은 "다음
   * 누름의 뜻" 을 되돌리는 것 하나뿐이다.
   */
  const handleKeyUp = (event: React.KeyboardEvent<HTMLDivElement>): void => {
    if (event.key !== SPACE_KEY) return;
    setSpaceHeld(false);
  };

  /**
   * 초점이 떠나면 짚은 상태를 **강제로 푼다**.
   *
   * 없으면 Space 를 짚은 채 다른 곳을 눌러 초점을 옮긴 순간 뗌이 이 층에 오지 않아,
   * 손 커서가 눌러도 풀리지 않는 채로 남는다(그 상태에서는 도형이 잡히지 않으므로
   * "캔버스가 고장 났다" 로 보인다). 짚음은 초점을 가진 동안에만 뜻이 있다.
   */
  const handleBlur = (): void => {
    setSpaceHeld(false);
  };

  /**
   * 팔레트가 도형을 놓는다 — **요소를 만드는 유일한 입구**이며, 목록 편집기가 하던 그
   * 경로(`appendElement`)를 그대로 지난다(가정 A7).
   *
   * 이 함수가 그 순수 함수보다 더 하는 일은 마지막 한 줄, **새 요소를 고르는 것**뿐이다.
   * 그래야 방금 놓은 것에 외곽선과 손잡이가 붙고 사용자의 다음 몸짓이 곧 배치 드래그가
   * 된다(spec.md §도형 팔레트). 고른 결과는 목록 편집기의 그 행을 펼치기도 한다 —
   * 캔버스 선택이 목록을 조종하는 그 배선(T10)을 그대로 타므로 **자동 펼침 규칙도 하나**다.
   *
   * 자리는 정하지 않는다. 씨앗 기하와 계단 오프셋이 `appendElement` 안에 있으므로 연속으로
   * 놓아도 같은 자리에 겹쳐 쌓이지 않고, 그 규칙은 어느 버튼을 눌렀든 같다.
   */
  const placeFromPalette = (kind: CanvasPrimitiveKind): void => {
    const { next, created } = appendElement(elements, kind);
    onElementsChange(next);
    setSelection(new Set([created.id]));
  };

  /**
   * 카탈로그가 도형을 놓는다. 위 함수와 **같은 세 줄**이며 다른 것은 부르는 입구 하나뿐이다
   * (`appendPathElement` — 경로는 명령 목록을 요구하므로 인자가 다르다).
   *
   * 그 셋이 같아야 하는 이유: 사용자에게 원시형 사각형과 카탈로그 별은 **같은 종류의 일**
   * 이다. 놓은 것이 맨 위에 오고, 계단을 타고, 놓자마자 골라져 다음 몸짓이 배치 드래그가
   * 되는 것이 어느 쪽에서 놓았든 같아야 한다.
   */
  const placeFromCatalog = (entry: ShapeCatalogEntry): void => {
    const { next, created } = appendPathElement(elements, entry.id, entry.path);
    onElementsChange(next);
    setSelection(new Set([created.id]));
  };

  /**
   * 가져온 SVG 도형들을 놓는다 — **위 두 함수와 같은 세 줄**이며 다른 것은 부르는 입구
   * 하나뿐이다(`appendImportedElements`).
   *
   * **새 id 전부를 선택으로 세운다**(REQ-06). 그래야 놓자마자 한 덩어리로 끌린다 — 오버레이의
   * 무리 이동이 선택된 **모든** 요소에서 `bases` 를 짓기 때문이고, 정렬 · 순서 · 방향키 미세
   * 이동도 같은 선택 위에서 그대로 돈다. 004 의 `group` 이 서기 전까지 가져온 그림을 한
   * 덩어리로 다루는 길은 이것과 서랍 저장 둘뿐이며, 그것이 완전한 대체가 아님을 감추지
   * 않는다(위험 R11).
   *
   * 상자는 **도형마다** 다르다(결함 D3 정정) — 그래야 손잡이 여덟이 제 잉크에 닿고 가져온
   * 요소들끼리 정렬이 산다. 계단 오프셋은 그래도 **무리 전체에 한 번만** 더해진다:
   * `appendImportedElements` 가 **같은** 오프셋을 모든 상자에 더하므로 문서에서의 상대 배치가
   * 그대로 보존된다. 요소마다 다른 오프셋을 주면 그때 그림이 25 단위씩 흩어진다.
   */
  const placeFromImport = (
    shapes: readonly ImportedShapeSpec[],
    texts: readonly ImportedTextSpec[],
  ): void => {
    const { next, created, createdTexts } = appendImportedElements(elements, shapes, texts);
    onElementsChange(next);
    // **문구도 선택에 든다.** 빠뜨리면 놓자마자 한 덩어리로 끌리는 것이 도형뿐이어서,
    // 사용자가 그림을 옮기면 이름표만 제자리에 남는다.
    setSelection(new Set([...created, ...createdTexts].map((el) => el.id)));
  };

  /**
   * 서랍의 항목을 캔버스에 놓는다 — **형제 요소들을 옮겨 찍는 평평한 붙여넣기**다
   * (REQ-04 · spec.md §004 의 `group` 에 기대지 않는다 (c)).
   *
   * `at` 이 있으면 그 자리를 묶음의 **좌상단**으로 삼고, 없으면(단추) 저장된 자리에서
   * 계단(`seedOffset`)만큼 민다. 계단은 이미 있는 요소 수를 보므로, 같은 항목을 잇달아
   * 눌러도 정확히 겹치지 않는다 — 팔레트가 새 도형에 대해 하는 그 일과 같은 규칙이다.
   *
   * **좌표는 `stagePoint` 를 지난다**(불변식 J7). 두 번째 호출 자리이지 두 번째 **함수**가
   * 아니며, 드래그가 쓰는 그 프레임(`pointerFrameOf(루트 상자, stage)`)을 그대로 만든다.
   * 붙임도 마찬가지로 **`snapDelta` 하나**를 쓴다 — 기준 상자는 저장된 묶음의 바깥 상자이고,
   * 그래서 무리를 끌 때와 같은 규칙(중심을 격자에 맞춘다)이 걸린다.
   *
   * **id 는 새로 발급한다.** 저장된 id 를 그대로 쓰면 배열에 중복이 생기고, 001 의 파서는
   * 중복 id 를 **먼저 온 것으로 접으므로** 놓은 것이 조용히 사라진다(REQ-04).
   *
   * **놓은 id 전부를 선택으로 세운다.** 이 SPEC 이 평평한 붙여넣기를 고르며 치른 값
   * (요소 여덟이면 목록에 여덟 줄)의 유일한 완화이자 충분한 완화다 — 오버레이의 무리
   * 이동이 선택된 **모든** 요소에서 `bases` 를 짓기 때문에, 놓자마자 한 덩어리로 끌리고
   * 정렬 · 순서 · 방향키 미세 이동도 같은 선택 위에서 그대로 돈다.
   *
   * 돌려주는 값은 **놓았는가**다. 캔버스 밖에서 뗀 몸짓은 놓기가 아니며, 그 사실을 아는
   * 것은 제 상자를 든 이 층뿐이다.
   */
  const placeFromScratchpad = (
    entry: ScratchpadEntry,
    at: ScratchpadDropPoint | null,
  ): boolean => {
    // 몸체가 빈 항목은 **여기까지 오지 못한다.** 저장은 빈 선택을 거절하고(`saveEntry`),
    // 파서는 살아남은 요소가 없는 항목을 버린다(`parseScratchpadEntry`). 두 입구가 전부
    // 막혀 있으므로 여기에 가드를 두면 검증되지 않은 채 남아 읽는 사람에게 "빈 항목이 올
    // 수도 있다" 고 거짓말한다(이 파일이 `anchorEl` 에 대해 세운 그 규율).
    let delta: CanvasDelta;
    if (at === null) {
      const off = seedOffset(elements.length);
      delta = { dx: off, dy: off };
    } else {
      const host = rootRef.current;
      if (host === null) return false;
      const rect = host.getBoundingClientRect();
      // 캔버스 **밖**에서 뗀 것은 놓기가 아니다. 판정은 드롭 존의 그것과 같은 규칙이며
      // (클라이언트 좌표 containment) 같은 함수를 쓴다.
      if (!pointInRect(rect, at.clientX, at.clientY)) return false;
      if (!(stage.width > 0) || !(stage.height > 0)) return false;
      const frame = pointerFrameOf(rect, stage);
      const point = unprojectPoint(stagePoint(at.clientX, at.clientY, frame), projection);
      const raw = { dx: point.x - entry.origin.x, dy: point.y - entry.origin.y };
      delta = snapToGrid ? snapDelta(raw, elementsBounds(entry.elements), gridStep) : raw;
    }

    // **사본을 놓는다.** 서랍은 영속되는 자료이므로, 놓인 요소가 항목의 `style`·`path`·
    // `rules` 를 그대로 나눠 가지면 캔버스 쪽에서 그중 하나를 제자리에서 고치는 순간 서랍
    // 속 원본까지 함께 바뀐다(`canvasElementFactory.withElementText` 가 같은 이유로 style 을
    // 새 객체로 갈아 끼운다). 기하는 아래 쓰기 통로가 어차피 새 값을 넣지만, 나머지는 이
    // 한 줄이 유일한 경계다.
    const body = cloneElements(entry.elements);

    let next: CanvasNode[] = [...elements];
    const created: string[] = [];
    for (const el of body) {
      const id = nextElementId(next);
      // 배열 끝에 순서대로 붙는다 — 배열 순서가 001 의 유일한 z-order 이므로 저장할 때의
      // 앞뒤가 그대로 살아난다. 기하는 **한 통로**(`patchNodeGeometry`)로 들어간다.
      next = [...next, { ...el, id }];
      next = patchNodeGeometry(next, id, moveGeometry(el.geometry, delta));
      created.push(id);
    }

    onElementsChange(next);
    setSelection(new Set(created));
    return true;
  };

  /**
   * 고른 것들을 서로 맞춘다(T13 · REQ-04).
   *
   * 변환 사슬(**투영 px 상자 → 백분율 오프셋 → ÷100 → 캔버스 단위 델타**)은 순수 모듈
   * `canvasEditArrange.alignDeltas` 가 소유한다 — 여기서는 상자를 모아 넘기고 돌려받은
   * 델타를 기하에 더할 뿐이다. 넘기는 상자가 선택 외곽선이 두르는 **그 상자**
   * (`outlineBox`)라, 화면에 보이는 테두리와 맞춰지는 변이 어긋날 수 없다.
   *
   * 쓰기는 여전히 `patchNodeGeometry` 한 함수를 지난다(REQ-06).
   *
   * **2개 미만이면 아무 일도 없다.** 그 판정은 `computeAlignPatches` 가 이미 소유하므로
   * (맞출 상대가 없으면 빈 배열이다) 여기서 다시 세지 않는다 — 두 곳에서 세면 갈라진다.
   */
  const applyAlign = (axis: AlignAxis, mode: AlignMode): void => {
    // 연결선은 맞출 상자가 없으므로 빠진다(M4). 남겨 두면 `outlineBox` 가 받을 수 없는
    // 노드가 되고, 억지로 상자를 지어 맞추면 "선을 왼쪽에 맞췄는데 아무 데도 안 붙는다"
    // 가 된다 — 연결선의 자리는 제 좌표가 아니라 **두 끝이 가리키는 것**이 정한다.
    const picked = elements
      .filter((el) => !isConnector(el))
      .filter((el) => selection.has(el.id));
    const deltas = alignDeltas(
      picked.map((el) => ({ nodeId: el.id, box: outlineBox(el, projection, textWidths) })),
      projection,
      axis,
      mode,
    );
    if (deltas.length === 0) return;

    const geometryById = new Map(picked.map((el) => [el.id, el.geometry] as const));
    let next: CanvasNode[] = [...elements];
    for (const { nodeId, delta } of deltas) {
      // 순수 모듈은 넘긴 노드 id 를 그대로 달아 돌려주므로 여기서 빌 수 없다. 그래도
      // 가드를 두는 것은 타입이 `Map.get` 의 `undefined` 를 요구하기 때문이며, 그 자리에
      // 단언(`!`)을 쓰면 나중에 그 모듈이 목록을 걸러 내게 되었을 때 조용히 깨진다.
      const geometry = geometryById.get(nodeId);
      if (geometry === undefined) continue;
      next = patchNodeGeometry(next, nodeId, moveGeometry(geometry, delta));
    }
    onElementsChange(next);
  };

  /**
   * 앞/뒤로 보내기(T14 · REQ-04) — **목록 편집기의 순서 이동과 같은 연산**을 부른다.
   *
   * 001 의 z-order 수단은 배열 순서 하나뿐이므로 두 번째 수단을 만들지 않는다. 규칙은
   * `canvasEditArrange.moveElementTo`(제거 후 삽입) 하나이고, 앞/뒤 보내기는 그것에
   * 목표 위치만 끝값(마지막 / 0)으로 준 것이다.
   *
   * 아무것도 움직이지 않으면 그 모듈이 **받은 배열을 그대로** 돌려주므로, 그때는 쓰지
   * 않는다 — 헛된 config 쓰기는 곧 헛된 렌더 프레임이다(AC-E4).
   */
  const applyZOrder = (toFront: boolean): void => {
    const next = toFront ? bringToFront(elements, selection) : sendToBack(elements, selection);
    if (next === elements) return;
    onElementsChange([...next]);
  };

  /**
   * 풀 수 있는 그룹 — **정확히 하나를 골랐고 그것이 그룹일 때만** 있다.
   *
   * 핸들이 서는 규칙(`selection.size === 1`)과 같은 자를 쓴다. 둘 이상을 골라 놓고 "무엇을
   * 푸는가" 는 답이 없는 질문이며, 임의로 첫째를 고르면 사용자가 고르지 않은 것이 풀린다.
   *
   * **한 함수가 판정을 소유한다** — 단추의 활성 여부 · 안내가 뜰지 · 실제로 무엇을 푸는지가
   * 전부 이 하나에서 나온다. 셋을 따로 지으면 "단추는 켜졌는데 눌러도 아무 일이 없다" 가
   * 표현 가능해진다.
   */
  const selectedGroup = (): CanvasNode | undefined => {
    if (selection.size !== 1) return undefined;
    const picked = elements.find((el) => selection.has(el.id));
    return picked !== undefined && isGroup(picked) ? picked : undefined;
  };

  /**
   * 고른 것들을 묶는다 — **판정도 산술도 하지 않는다**(SPEC-CANVAS-004 REQ-07).
   *
   * 합집합 상자 · 퇴화 넓히기 · 로컬 변환 · 배열 자리 · 거절 판정은 전부 `groupNodes` 가
   * 소유한다(M5). 이 함수가 그 순수 함수보다 더 하는 일은 둘뿐이다: **거절 사유를 화면에
   * 올리는 것**과 **새 그룹 하나를 선택으로 세우는 것**. 뒤엣것은 팔레트 · 카탈로그 ·
   * 가져오기 · 서랍이 이미 지킨 규율이다(놓은 것은 바로 끌 수 있어야 한다).
   *
   * **판정을 여기서 다시 짓지 않는 것이 요점이다.** 거절 조건을 이 층이 한 벌 더 들면
   * "단추는 눌렸는데 아무 일도 없다" 와 "안내는 떴는데 실제로는 묶였다" 가 둘 다 가능해진다.
   */
  const applyGroup = (): void => {
    const outcome = groupNodes(elements, selection);
    if (outcome.refusal !== undefined) {
      setGroupRefusal(outcome.refusal);
      return;
    }
    setGroupRefusal(null);
    onElementsChange([...outcome.nodes]);
    // 거절이 아니면 `groupId` 는 반드시 있다(`GroupOutcome` 의 계약). 그래도 단언(`!`)을
    // 쓰지 않는 것은, 나중에 그 계약이 바뀌면 여기가 조용히 `undefined` 를 선택에 넣기
    // 때문이다 — 그때는 선택이 비는 편이 낫다.
    if (outcome.groupId !== undefined) setSelection(new Set([outcome.groupId]));
  };

  /**
   * 고른 그룹 하나를 푼다 — 확인은 **컨트롤이 이미 받았다**(REQ-07 · 가정 A19).
   *
   * 버려지는 규칙 행의 수는 `rulesLostByUngroup` 하나가 판정하고(M5), 컨트롤은 그 수가
   * 0 보다 클 때만 확인을 묻는다. **같은 판정을 두 곳에서 짓지 않는 것**이 그 형상의
   * 이유다 — 둘이 되면 "안내는 떴는데 아무것도 안 버렸다" 와 그 반대가 함께 가능해진다.
   */
  const applyUngroup = (): void => {
    const target = selectedGroup();
    if (target === undefined) return;
    const outcome = ungroupNode(elements, target.id);
    if (outcome.refusal !== undefined) return;
    setGroupRefusal(null);
    onElementsChange([...outcome.nodes]);
    // **풀린 부품 전부**가 선택으로 남는다 — 그래야 방금 푼 것이 한 덩어리로 계속 끌린다
    // (가져오기가 같은 이유로 같은 일을 한다).
    setSelection(new Set(outcome.liftedIds));
  };

  /**
   * 분리할 부품 — **정확히 하나를 골랐고 그것이 부품일 때만** 있다(SPEC-CANVAS-009 M6).
   *
   * `selectedGroup` 과 **같은 형상**이다: 한 함수가 단추의 활성 여부 · 안내가 뜰지 · 실제로
   * 무엇을 빼는지를 모두 소유한다. 셋을 따로 지으면 "단추는 켜졌는데 눌러도 아무 일이
   * 없다" 가 표현 가능해진다.
   *
   * 배열에 실재하는지까지 여기서 가린다 — 없는 부품 키가 선택에 남아 있어도(지워진 그룹)
   * 단추가 켜지지 않는다(REQ-07).
   */
  const selectedPart = (): { groupId: string; partId: string } | undefined => {
    if (selection.size !== 1) return undefined;
    const key = [...selection][0]!;
    if (!isPartKey(key)) return undefined;
    const { nodeId, partId } = parseFrameKey(key);
    if (partId === undefined || findPart(elements, nodeId, partId) === undefined) return undefined;
    return { groupId: nodeId, partId };
  };

  /**
   * 고른 부품 하나를 그룹 밖으로 뺀다 — 확인은 **컨트롤이 이미 받았다**(REQ-05-c).
   *
   * `applyUngroup` 과 같은 형상이고, 실제로 같은 자리에서 갈린다: 남을 부품이 1 개 이하면
   * `detachPart` 자신이 `ungroupNode` 를 부른다(M6). 이 층은 그 갈래를 **알지 못한다** —
   * 알면 조건이 두 자리가 되고, 둘이 어긋나는 날 "빼면 그룹이 남는다고 했는데 사라졌다"
   * 가 된다.
   */
  const applyDetach = (): void => {
    const target = selectedPart();
    if (target === undefined) return;
    const outcome = detachPart(elements, target.groupId, target.partId);
    if (outcome.refusal !== undefined) return;
    setGroupRefusal(null);
    onElementsChange([...outcome.nodes]);
    // 올라온 것이 선택으로 남는다 — 풀기가 같은 이유로 같은 일을 한다(방금 뺀 것을 곧바로
    // 끌 수 있어야 한다).
    setSelection(new Set(outcome.liftedIds));
  };

  // 편집이 꺼져 있으면 DOM 에 아무것도 남기지 않는다(표시 전용).
  if (!enabled) return null;

  /**
   * 핸들을 달 요소. **하나만 골랐을 때만** 있다(파일 머리말의 다중 선택 규칙).
   * `find` 가 비는 것은 실제로 일어난다 — 골라 둔 요소를 목록 편집기에서 지우면 선택에는
   * 그 id 가 남고 배열에는 없다.
   */
  /** 하나만 골랐을 때의 그 키. 손잡이는 **한 요소만 골랐을 때만** 뜬다(§크기 조절). */
  const soleKey = selection.size === 1 ? [...selection][0]! : undefined;
  const handleHost = soleKey === undefined ? undefined : nodeForKey(elements, soleKey);

  /**
   * 손잡이를 세울 **연결선** (SPEC-CANVAS-011 M9 · REQ-07).
   *
   * 위 `handleHost` 와 **배타적**이다 — `nodeForKey` 는 연결선을 조용히 떨어뜨리고
   * `connectorForKey` 는 그것만 받으므로, 한 키가 둘 다를 채우는 상태가 없다. 그래서
   * "연결선에 8핸들이 선다"(AC-63)가 검사가 아니라 **형상**으로 불가능하다.
   */
  const connectorHost = soleKey === undefined ? undefined : connectorForKey(elements, soleKey);

  /**
   * 그 연결선의 점들 — **그린 그 함수**가 낸 목록이다(AC-45 ①의 셋째 소비자).
   *
   * 여기서 참조를 다시 풀지 않는 것이 요점이다. 그리는 쪽·잡는 쪽과 다른 자리에서 풀면
   * "보이는 선과 잡는 손잡이가 다른 자리에 있다" 가 표현 가능해진다 — 002 가 위험 R1 로
   * 이름 적어 둔 그 결함이며, 011 은 그것을 막으려고 해석을 한 함수로 못박았다.
   *
   * 끊긴 연결이면 `undefined` 이고, 그때 손잡이는 **하나도 서지 않는다**(REQ-08).
   * 그려지지 않는 선에 손잡이가 서면 사용자는 보이지 않는 것을 끌게 된다.
   */
  const connectorPoints =
    connectorHost === undefined
      ? undefined
      : resolveConnector(connectorHost, elements, projection, textWidths);

  /** 정렬은 **맞출 상대가 있어야** 뜻이 있다 — 하나만 골라 놓고 맞출 곳은 없다. */
  const canAlign = selection.size >= 2;
  /**
   * 순서 이동은 하나만 골라도 뜻이 있다 — 다만 **최상위 원소**여야 한다.
   *
   * z-order 는 최상위 배열의 자리이고 부품에는 그 자리가 없다(부품 순서 바꾸기는 009 의
   * 범위 밖이다). 세는 자를 `selection.size` 로 두면 부품만 고른 상태에서 단추가 켜지고,
   * 눌러도 `bringToFront` 가 아무것도 찾지 못해 **눌러도 아무 일이 없는 단추**가 된다.
   */
  const canOrder = elements.some((el) => selection.has(el.id));

  /**
   * 묶을 수 있는가 — **둘 이상**이다(REQ-07 · AC-12). `canAlign` 과 같은 자다.
   *
   * 부품 하나짜리 그룹은 정체성도 캐스케이드도 주지 않으면서 목록에 층만 더하므로,
   * 그 상태에서는 단추를 **끈다** — 눌러도 아무 일이 없는 단추는 사용자에게 고장으로 보인다.
   *
   * **그룹이 섞인 선택에서는 끄지 않는다.** 그 경우는 "아직 고를 것이 모자라다" 가 아니라
   * "이 조합은 묶을 수 없다" 이고, 고쳐야 할 것이 다르다 — 꺼진 단추는 이유를 말하지 못한다.
   */
  const canGroup = selection.size >= 2;

  /** 풀 수 있는 그룹(없으면 단추가 꺼진다). */
  const ungroupTarget = selectedGroup();

  /**
   * 풀면 **버려질** 규칙 행의 수. 판정은 M5 의 순수 함수 하나가 소유하며 이 층은 그 수를
   * 나를 뿐이다 — 컨트롤이 `> 0` 일 때만 확인을 묻는다(REQ-07 · 가정 A19).
   */
  const ungroupRulesAtRisk = rulesLostByUngroup(ungroupTarget);

  /** 뺄 수 있는 부품(없으면 단추가 꺼진다 — SPEC-CANVAS-009 M6). */
  const detachTarget = selectedPart();

  /**
   * 분리가 **버릴** 규칙 행의 수. 위 `ungroupRulesAtRisk` 와 **같은 규율**이다 — 판정은
   * `groupOps` 의 순수 함수 하나가 소유하고 이 층은 그 수를 나를 뿐이다(REQ-05-c).
   */
  const detachRulesAtRisk =
    detachTarget === undefined
      ? 0
      : rulesLostByDetach(elements, detachTarget.groupId, detachTarget.partId);

  /** 두 표면이 **같은 컴포넌트**를 그린다 — 그것이 I23 을 형상으로 만드는 유일한 길이다. */
  const groupTools = (
    <CanvasGroupTools
      canGroup={canGroup}
      canUngroup={ungroupTarget !== undefined}
      rulesAtRisk={ungroupRulesAtRisk}
      canDetach={detachTarget !== undefined}
      detachRulesAtRisk={detachRulesAtRisk}
      refusal={groupRefusal}
      onGroup={applyGroup}
      onUngroup={applyUngroup}
      onDetach={applyDetach}
    />
  );

  /**
   * 앵커 도구도 **한 번 짓고 두 자리에서 그린다** — `groupTools` 와 같은 값이다(AC-61).
   *
   * 그룹 도구가 두 표면에 서는 근거(가정 A21 · 불변식 I23)가 여기에는 한 걸음 더 곧게
   * 적용된다: 앵커는 **조건 없이 그려지는 표시 층**이다(도구가 켜진 동안 최상위 전부에
   * 점이 선다). 컨트롤을 도크에만 두면 대시보드에 놓인 패널에서는 그 층을 켤 수도 끌
   * 수도 없고, 그것이 006 이 배달한 그 결함의 형상이다.
   */
  const anchorTools = (
    <CanvasAnchorTools
      active={tool === 'anchor'}
      onToggle={() => setTool((prev) => toggleTool(prev, 'anchor'))}
      refusal={anchorRefusal}
    />
  );

  /**
   * 연결선 도구 넷도 **한 번 짓고 두 자리에서 그린다** — `groupTools` · `anchorTools` 와
   * 같은 값이다(SPEC-CANVAS-011 AC-61 · 불변식 I24).
   *
   * 근거는 앵커 도구의 그것보다 한 걸음 더 곧다: 연결선 도구를 켜면 **앵커 점이 함께**
   * 뜬다(REQ-02-b — `TOOL_SHOWS_ANCHORS`). 즉 이 단추들은 조건 없이 그려지는 표시 층을
   * 켜고 끄는 손잡이이기도 하며, 도크에만 두면 대시보드에 놓인 패널에서는 그 층을 켤 수도
   * 끌 수도 없다 — 006 이 배달한 그 결함의 형상이다(불변식 I23).
   *
   * 넷을 **하나의 컴포넌트**가 그리는 것에도 뜻이 있다. 도구 넷이 서로 배타임을 화면이
   * 말해야 하고(많아야 하나가 `aria-pressed`), 네 자리에 각각 짜 넣으면 그 배타성을 네
   * 벌이 따로 지키게 된다.
   */
  const connectorTools = (
    <CanvasConnectorTools
      tool={tool}
      onToggle={(pressed) => setTool((prev) => toggleTool(prev, pressed))}
    />
  );

  return (
    <div
      ref={rootRef}
      data-testid="canvas-edit-overlay"
      role="group"
      aria-label={t('dashboard.canvas.edit.surfaceAria')}
      // 이 층이 **탭으로 닿는 자리**다(T15 · REQ-01). 칠한 픽셀에는 초점을 줄 수 없으므로,
      // 캔버스 편집을 키보드로 시작하는 유일한 문이 여기다. 설명은 아래 sr-only 문단이
      // 맡고, 보조기기가 읽을 단축키 이름은 `aria-keyshortcuts` 가 그대로 알린다.
      tabIndex={0}
      aria-describedby={hintId}
      aria-keyshortcuts={EDIT_KEY_SHORTCUTS}
      // 캔버스 위 전면 층. 터치 스크롤이 드래그를 가로채지 않게 `touch-none` 을 둔다.
      //
      // **손 커서는 자식까지 덮는다**(사용자 신고 2026-09-16). 루트의 커서는 인라인
      // `style` 이 지지만 손잡이 · 단추는 제 `cursor-*` 를 제 노드에 달고 있어 그쪽이
      // 이긴다 — Space 를 짚은 동안 손잡이 위에서 크기 조절 커서가 뜨면 그것은 **거짓말**
      // 이다(그 순간 손잡이는 잡히지 않는다). 후손 변형으로 덮는 이 관용구는 이 저장소의
      // 세 드래그 층(`PanelDragLayer` · `ChartDragLayer` · `GaugeDragLayer`)이 이미 쓴다.
      className={cn(
        'absolute inset-0 z-20 touch-none',
        panning ? '[&_*]:cursor-grabbing' : spaceHeld ? '[&_*]:cursor-grab' : undefined,
      )}
      // **커서의 우선순위가 여기 한 줄에 선다**(위에서 아래로 읽는다).
      //
      //   1) 끌고 있는 팬 — 쥔 손. 그 몸짓이 끝나기 전에는 다른 어떤 것도 시작될 수 없다.
      //   2) 짚고 있는 Space — 편 손. **펜을 이긴다**: 짚은 동안 누름은 팬이므로, 펜을
      //      그대로 두면 커서가 "여기서 선이 시작된다" 고 약속하고 그 약속은 지켜지지 않는다.
      //   3) 펜(사용자 신고 2026-09-16) — 앵커 점은 표식이라 포인터를 먹지 않으므로 커서가
      //      그 점에서 나올 수 없고(§`penHover`), 루트가 곧 그 점 아래 깔린 면이다. 조건
      //      둘은 사용자가 말한 그 둘이다 — **선이 시작될 수 있는 자리 위**와 **긋는 동안**.
      //   4) 그 밖에는 **편 손**(SPEC-CANVAS-011) — 맨손 끌기가 이제 팬이기 때문이다.
      //
      // **손 커서 규칙을 둘로 두지 않는다**(011). 011 전에는 손이 뜨는 조건이 "Space 를
      // 짚었다" 하나였고, 011 은 거기에 "빈 자리 위" 를 더하려는 것이 아니라 **기본값을
      // 손으로 바꾼다**: 맨손 누름이 팬이라는 사실이 이제 자리를 가리지 않고 참이기
      // 때문이다(잉크 위의 맨손 끌기도 "잡아서 옮긴다" 이므로 손은 거기서도 거짓말하지
      // 않는다). 그래서 2) 는 **손을 켜는 규칙이 아니라 펜을 이기는 규칙**으로만 남는다 —
      // 조건 둘을 나란히 두면 같은 커서를 두 자리에서 켜게 되고, 한쪽만 고쳐지는 날
      // "Space 를 짚으면 손인데 그냥은 아닌" 화면이 돌아온다.
      //
      // 손 커서를 **자식까지 덮는 일**(아래 `className`)은 여전히 2)·1) 의 몫이다. 그 둘은
      // 손잡이마저 잡히지 않는 상태이므로 손잡이의 커서가 거짓말이 되지만, 기본값으로서의
      // 편 손은 손잡이를 가로챌 까닭이 없다 — 그 자리에서 손잡이는 실제로 잡힌다.
      //
      // 도구 표를 **여기서 한 번 더** 읽는 것에 뜻이 있다. 판정은 이동에서만 도는데 도구는
      // 단추로 바뀌므로, 이 줄이 없으면 도구를 끈 뒤에도 손을 움직이기 전까지 펜이 남는다.
      // 값이 아니라 렌더가 그 사실을 들게 하면 끄는 쪽이 즉시 참이 된다.
      style={{
        cursor: panning
          ? 'grabbing'
          : spaceHeld
            ? 'grab'
            : connectorDraw !== null || (TOOL_CONNECTOR_ROUTE[tool] !== null && penHover)
              ? PEN_CURSOR
              : 'grab',
      }}
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerUp}
      onPointerCancel={handlePointerCancel}
      onKeyDown={handleKeyDown}
      onKeyUp={handleKeyUp}
      onBlur={handleBlur}
    >
      {/* **닿는 면**(REQ-08 · 불변식 I18). 그리지 않고 닿기만 하는 층이며, 그래서
          `pointer-events-none` 이 **없는** 유일한 자식이다.

          **루트의 첫 자식이어야 한다.** 뒤에 오는 형제(격자 · 흐림 · 경계 · 선택 윤곽선 ·
          손잡이 단추)가 위에 얹혀야 손잡이를 잡을 수 있다 — 이 층이 손잡이를 덮으면 크기
          조절이 통째로 죽는다.

          **처리자를 달지 않는다.** 달면 `handlePointerDown` 의 `event.currentTarget` 이 이
          상자가 되어 `pointerFrameOf(rect, stage)` 가 **틀린 축척과 틀린 원점**을 함께 얻는다
          (가정 A16). 버블링만 태우면 재는 노드가 루트로 남는다.

          **`touch-none` 을 제 몫으로 가져야 한다.** `touch-action` 은 상속되지 않는 속성이라
          루트의 값이 여기로 내려오지 않는다 — 빠뜨리면 터치에서만 여백의 드래그가 스크롤에
          먹힌다.

          **이름을 갖지 않는다.** 장식조차 아닌 빈 상자이므로 `aria-hidden` 이다. 탭 정지점은
          여전히 루트 하나뿐이다(T15 · REQ-01).

          잘리는 자리는 표면 컨테이너의 `overflow-hidden` 하나이며, 그것이 **비트맵이 잘리는
          바로 그 자리**다. 루트에도 `canvas-stage` 에도 `overflow` 가 없으므로 이 상자는
          여백까지 실제로 뻗는다(격자 상자가 이미 같은 방식으로 뻗어 그려지고 있다). */}
      <div
        data-testid="canvas-workspace-hit"
        aria-hidden="true"
        className="absolute touch-none"
        style={{
          left: -workspaceOrigin.x,
          top: -workspaceOrigin.y,
          width: workspaceSize.width,
          height: workspaceSize.height,
        }}
      />
      {/* 눈으로 보는 사람에게는 손잡이와 커서가 조작법을 알리지만, 스크린 리더에게는
          알릴 것이 없다 — 그래서 설명을 DOM 에 **항상** 두고 `aria-describedby` 로 잇는다
          (`FieldHelp.tsx` 가 세운 이 저장소의 관용구다). */}
      {/* 마키 사각형은 `aria-hidden` 인 장식이라 보조기기에게는 **존재하지 않는다** — 그
          몸짓이 있다는 사실이 닿는 유일한 통로가 이 문단이다(006 M11 이 흐림·경계를 두고
          세운 그 논리 그대로다). 그래서 키를 따로 두고 같은 문단에 잇는다: 두 문장은 모두
          "이 표면을 어떻게 조작하는가" 이고, 문단을 나누면 `aria-describedby` 가 둘을
          가리킬 수 없다. */}
      <p id={hintId} className="sr-only">
        {t('dashboard.canvas.edit.keyboardHint')} {t('dashboard.canvas.edit.marqueeHint')}{' '}
        {t('dashboard.canvas.edit.deleteHint')} {t('dashboard.canvas.edit.panHint')}
      </p>
      {/* 격자 — **신규 격자 컴포넌트를 만들지 않는다**(REQ-04). `PanelEditGrid` 는 선을
          DOM 요소가 아니라 `repeating-linear-gradient` 로 그리고 `absolute inset-0` +
          `pointer-events-none` 이라 좌표 변환이 아예 필요 없다. 캔버스에서는 이 층이
          칠해진 픽셀 **위**에 얹히지만(도형은 캔버스 안에 있다) 반투명 참조선이라 가리지
          않으며, 그 대가로 001 의 렌더 경로에 한 픽셀도 더하지 않는다.

          다만 **위에 얹힌다는 사실이 색을 정한다.** 다른 패널에서 요소 뒤에 깔릴 때 쓰는
          28% 짜리 선은 칠해진 도형 위에서는 사실상 보이지 않는다 — 그래서 같은 컴포넌트에
          진하기 한 벌(`strong`)만 더 두고 여기서 그것을 고른다.

          간격은 **`snapDelta` 가 쓰는 그 칸**이며 단위는 백분율이 아니라 **px** 다. 두 축에
          서로 다른 수가 가는 것은 캔버스와 스테이지의 종횡비가 다르면 한 칸의 화면 크기가
          축마다 다르기 때문이고, 두 값 모두 **같은 정수 간격**에서 나온다. 백분율이 아닌
          이유는 그것이 브라우저에서 소수 px 가 되어 선이 두 픽셀에 걸쳐 칠해지기 때문이다
          (사용 시험: "격자가 일정하지 않음" 세 번째 회차 — `PanelEditGrid` §단위).
          표면이 그리는 영역을 이 칸의 정수배로 맞춰 두므로 **자투리 칸도 없다**.
          그리는 값과 붙는 값을 각자 정하는 자리는 없다: 붙임은 이 px 를 아예 보지 않고
          같은 정수를 그대로 쓴다.

          한 칸이 1px 도 되지 않으면 그리지 않는다. 1px 선에 1px 미만의 주기는 격자가
          아니라 **꽉 찬 사각형**이며, 참조선이라고 내놓을 수 없는 그림이다. */}
      <div
        data-testid="canvas-workspace-grid"
        // 장식이다 — 보조기기에게 알릴 것이 없다.
        aria-hidden="true"
        // 이 상자는 오버레이 루트보다 **넓다.** 포인터를 받으면 그 위를 지나는 몸짓이
        // 이 층에서 끊기므로(위험 R8) 받지 않는다.
        className="pointer-events-none absolute"
        style={{
          left: -workspaceOrigin.x,
          top: -workspaceOrigin.y,
          width: workspaceSize.width,
          height: workspaceSize.height,
        }}
      >
        <PanelEditGrid
          enabled={snapToGrid && gridCell.x >= 1 && gridCell.y >= 1}
          step={gridCell.x}
          stepY={gridCell.y}
          strength="strong"
          unit="px"
          // 선은 **출력 영역의 원점**에서 시작한다(REQ-04 · 위험 R3). 이 상자의 왼쪽 위에서
          // 시작하면 저술 여백이 한 칸의 배수가 아닌 순간 그린 선과 붙은 자리가 갈라진다.
          offsetX={workspaceOrigin.x}
          offsetY={workspaceOrigin.y}
        />
      </div>
      {/*
        **어디가 패널에 나오는가** — 출력 영역 밖을 흐리게 덮고 경계를 두른다(REQ-02).
        둘 다 이 층의 루트 상자, 즉 **출력 영역 그 자체**에 걸린다: 오버레이 루트가 곧
        `canvas-stage` 이므로 `inset-0` 하나가 그 사각형이다. 상자를 다시 계산하지 않는
        것에 뜻이 있다 — 좌표로 두르는 순간 같은 상자가 두 벌이 된다(위험 R1).

        **흐리는 쪽은 바깥이다.** 도해 도구의 관용(아트보드는 밝고 그 밖은 어둡다)이자,
        이 기능이 답하려는 질문("어디가 패널에 나오나")에 직접 답하는 방향이다. 안쪽을
        흐리면 사용자가 실제로 보려는 것이 흐려진다.

        **칠하지 않고 DOM 으로 둔다**(REQ-02 · 불변식 I1). 캔버스에 칠하면 편집기 상태가
        001 이 유휴로 만든 rAF 루프 안으로 들어와, 선택·호버·격자 토글이 프레임을 0 건
        요청한다는 성질(AC-E4)이 무너진다.

        경계선만 두고 흐림을 빼는 길은 기각했다 — 선 하나는 격자선과 혼동되고, 격자를
        끄면 안팎을 가르는 단서가 그 한 줄뿐이 된다. 흐림의 **단차**는 선이 없어도 읽히는
        신호라, 둘을 함께 두면 격자가 켜지든 꺼지든 안팎이 갈린다.
      */}
      <div
        data-testid="canvas-region-scrim"
        aria-hidden="true"
        className="pointer-events-none absolute inset-0"
        style={{ boxShadow: `0 0 0 ${REGION_SCRIM_SPREAD_PX}px ${REGION_SCRIM_COLOR}` }}
      />
      <div
        data-testid="canvas-region-bounds"
        aria-hidden="true"
        className="pointer-events-none absolute inset-0 border"
        style={{ borderColor: REGION_BOUNDS_COLOR }}
      />
      {/* **떠 있는 배율 줄**(SPEC-CANVAS-006 M10 · REQ-10 · 불변식 I23).

          006 은 표시 층 셋(격자 · 흐림 · 경계)을 **조건 없이** 그리면서 컨트롤 전부를
          `dockHost !== null` 뒤에 두었고, 도크 자리를 펴는 곳은 설정 다이얼로그 한 곳뿐이다.
          그래서 대시보드에서 제자리 편집하는 사람은 **줄어든 출력 영역과 저술 여백을 보면서
          그 어느 것도 다스릴 수 없었다** — 층이 그려지는 자리에 그 층을 다스리는 손잡이가
          닿지 않았다. 이 줄이 그 자리를 메운다.

          **도크가 있으면 서지 않는다**(결정 1 · 불변식 I24). 한 값에 살아 있는 컨트롤이
          둘이면 같은 것을 두 자리에서 눌러야 하고 그중 하나는 **그림을 가린다** — 이
          저장소는 그 형상을 이미 한 번 걷어냈다(스테이지 위의 아이콘 띠).

          **자리는 작업 영역의 왼쪽 아래다** — 고른 것이 아니라 **남은 것**이다(결정 2).
          나머지 세 모서리에는 임자가 있다: 위쪽 띠 전부는 `DragHandle`(`inset-x-0 top-0 h-6`),
          오른쪽 위는 설정·삭제 단추(`right-1 top-1 z-20`), 오른쪽 아래 20×20 은
          `react-grid-layout` 의 `se` 리사이즈 손잡이이며 셋 다 대시보드 편집모드에서만 뜨는데
          **그 상태가 곧 캔버스 편집이 가능한 상태**다. 왼쪽 **위**는 걷어낸 아이콘 띠가
          살던 자리이고 M8 뒤 옛 그림이 몰리는 자리다(위험 R19).

          **상자는 손에 든 값 그대로 쓴다**(불변식 I10). 작업 영역의 아래 모서리는 오버레이
          좌표로 `-origin.y + box.height` 이므로 그 자리에 줄의 **아래 변**을 앉힌다
          (`translateY(-100%)`) — `outer` 와 축척으로 되짚으면 그것이 곧 두 번째 측정원이다.

          **포인터를 받는다** — 위험 R8 의 가드가 열거하는 세 이름에 **들어가지 않는다**
          (넣으면 그 가드가 곧 이 기능을 금지한다 · 위험 R16). 그 가드가 지키는 문장은
          "그리는 층은 포인터를 먹지 않는다" 가 아니라 **"장식은 포인터를 먹지 않는다"**
          이며, 줄은 이름을 가진 컨트롤이라 `aria-hidden` 이 아니다.

          **줄 위의 누름과 키를 제 자리에서 끊는다.** 끊지 않으면 배율을 적으려는 손짓이
          그 뒤 도형을 고르고 이동 드래그까지 시작하며(도크가 이미 같은 한 줄을 쓴다),
          방향키는 `handleKeyDown` 이 표적을 보지 않으므로 **수는 그대로이고 요소가
          움직인다**(가정 A23 · 위험 R26). `preventDefault` 는 쓰지 않는다 — 칸의 초점과
          캐럿이 살아 있어야 하고, `defaultPrevented` 는 `previewPan` 이 읽는 표시라 뜻이
          번진다. 끊는 자리를 `handleKeyDown` 안에 두지 않는 것도 그 함수가 REQ-08 이
          "한 줄도 바뀌지 않는다" 로 이름 적어 둔 경로이기 때문이다. */}
      {dockHost === null && (
        <div
          data-testid="canvas-workspace-zoom-bar"
          role="group"
          aria-label={t('dashboard.canvas.edit.workspaceZoomBar')}
          className={ZOOM_BAR_CLASS}
          style={{
            left: -workspaceOrigin.x + ZOOM_BAR_INSET_PX,
            top: -workspaceOrigin.y + workspaceSize.height - ZOOM_BAR_INSET_PX,
            transform: 'translateY(-100%)',
          }}
          onPointerDown={(event) => event.stopPropagation()}
          onKeyDown={(event) => event.stopPropagation()}
        >
          <CanvasWorkspaceZoomField
            zoom={workspaceZoom}
            onZoomChange={setWorkspaceZoom}
            className={ZOOM_BAR_INPUT_CLASS}
            // 도크가 쓰는 `FieldHelp` 의 클릭 팝오버를 여기 그대로 쓰면 **잘린다** —
            // 그 팝오버는 `absolute left-0 top-full w-64` 로 아래·오른쪽에 열리는데 줄은
            // 왼쪽 아래 모서리에 살고 표면 컨테이너에 `overflow-hidden` 이 있다. 열어도
            // 보이지 않는 `?` 는 화면이 지키지 못할 약속이다. 그래서 이 층이 이미 쓰는
            // 관용구(`sr-only` 문단 + `aria-describedby`)를 그대로 쓰고, 눈으로 보는
            // 사람에게는 칸의 `title` 이 같은 이름을 나른다 — 네이티브 툴팁은 DOM 이
            // 아니라 브라우저 크롬이라 `overflow-hidden` 에 잘리지 않는다.
            renderHelp={(describedById) => (
              <p id={describedById} data-testid="canvas-workspace-zoom-hint" className="sr-only">
                {t('dashboard.canvas.edit.workspaceZoomHint')}
              </p>
            )}
          />
          {/* **그룹 · 그룹 해제가 이 줄에도 선다**(SPEC-CANVAS-004 REQ-08 · 가정 A21 ·
              불변식 I23). 도크에만 두면 006 이 배달했던 그 결함을 같은 파일에 다시 심는
              것이다 — 그룹은 **두 표면 모두에서 그려지고 두 표면 모두에서 선택되는데**
              도크를 펴는 곳은 설정 다이얼로그 한 자리뿐이다.

              **도크와 같은 컴포넌트**를 그린다(불변식 I24 의 규율 — 한 도구의 두 표현이지
              두 도구가 아니다). 배율 칸이 006 M10 에서 같은 형상을 세웠고, 그래서 활성
              조건 · 확인 절차 · 거절 문구가 두 벌이 되지 않는다.

              줄 자신의 이름은 **바뀌지 않는다**(`workspaceZoomBar`). 이름을 고치면 그
              이름을 단언하는 006 의 시험이 빨개지고, 그것은 M6 가 006 의 가드를 걷어낸다는
              뜻이다. 대신 제 이름을 가진 묶음을 **안쪽에** 둔다 — 듣는 사람에게는 "보기
              배율 줄 → 그룹 → 단추 둘" 로 읽힌다. */}
          <div
            role="group"
            aria-label={t('dashboard.canvas.edit.dockGroup')}
            className="flex items-center gap-1"
          >
            {groupTools}
          </div>
          {/* **앵커 도구도 이 줄에 선다**(SPEC-CANVAS-011 AC-61 · 불변식 I23 · I24).

              그룹 묶음과 같은 자리·같은 이유이고, 근거는 한 걸음 더 곧다: 앵커 점은 도구가
              켜진 동안 **조건 없이** 그려지는 표시 층이므로, 켜고 끄는 손잡이가 그 층이
              그려지는 표면에 있어야 한다. 제 이름을 가진 묶음을 따로 두어 듣는 사람에게
              "보기 배율 줄 → 그룹 → …, 앵커 → …" 로 읽히게 한다. */}
          <div
            role="group"
            aria-label={t('dashboard.canvas.edit.dockAnchor')}
            className="flex items-center gap-1"
          >
            {anchorTools}
          </div>
          {/* **연결선 도구도 이 줄에 선다**(SPEC-CANVAS-011 AC-61 · 불변식 I23 · I24).

              앵커 묶음 바로 뒤이고, 근거는 그 묶음의 근거를 그대로 물려받는다: 이 단추들이
              켜는 것은 잇는 몸짓이자 **앵커 점이라는 표시 층**이므로(REQ-02-b), 켜고 끄는
              손잡이가 그 층이 그려지는 표면에 있어야 한다. */}
          <div
            role="group"
            aria-label={t('dashboard.canvas.edit.dockConnector')}
            className="flex items-center gap-1"
          >
            {connectorTools}
          </div>
        </div>
      )}
      {/*
          도형 팔레트를 비롯한 **편집 도구 한 벌은 이 층이 만들지만 이 층 안에 그려지지
          않는다**(사용 시험: "도형 팔레트 크기가 너무 작음"). 스테이지 위에 떠 있던 띠는
          그림을 가리고 스테이지 폭에 갇혀 이름을 달 수 없었다. 도구를 쓰는 곳은 패널
          설정뿐이므로, 미리보기 **옆**의 제 영역(`CanvasEditDockRegion`)으로 옮겨 이름과
          누를 면적을 되찾는다.

          왜 포털인가: 격자·정렬·순서는 스테이지 크기·선택·요소 배열을 모두 봐야 하고 그
          값들은 전부 이 층 안에 있다. 상태를 다이얼로그로 올리면 스테이지를 잴 때마다·
          요소가 끌릴 때마다 설정 화면이 통째로 다시 그려진다. 그래서 **자리만 받아**
          그곳에 그린다 — React 트리 상으로는 여전히 이 층의 자식이라 선택 컨텍스트도
          이벤트 전파도 종전과 같다(도구가 누름을 스스로 끊는 이유가 그것이다).

          자리가 없으면(대시보드에 놓인 패널) **아무 데도 그리지 않는다.** 도구가 스테이지
          위로 되돌아올 길을 남기지 않는 것이 "설정에서만 쓴다" 는 결정이다. */}
      {dockHost !== null &&
        createPortal(
          // 드롭 존 등록 채널을 **포털 안쪽**에 연다. 포털은 DOM 상으로만 패널 밖이고
          // React 트리 상으로는 이 층의 자식이므로 컨텍스트가 그대로 내려간다 — 도크가
          // 선택 컨텍스트를 이미 그렇게 받고 있다.
          <CanvasScratchpadDropContext value={setDropZone}>
            <CanvasEditDockBody
              onPlace={placeFromPalette}
              onPlaceShape={placeFromCatalog}
              snapToGrid={snapToGrid}
              onSnapToGridChange={setSnapToGrid}
              gridStep={gridStep}
              onGridStepChange={setGridStep}
              zoom={workspaceZoom}
              onZoomChange={setWorkspaceZoom}
              // 자투리 고지의 근거 — 간격이 이 두 축을 나누어떨어뜨리는가. 투영이 이미 들고
              // 있는 그 크기이므로 새 측정원이 되지 않는다(위험 R1).
              canvas={projection.canvas}
              canAlign={canAlign}
              canOrder={canOrder}
              onAlign={applyAlign}
              onOrder={applyZOrder}
              // 끌어 넣기와 **같은 함수**다(J11). 단추 쪽에는 되돌릴 기하가 없으므로 지금
              // 배열에서 그대로 뜬다 — 뒷줄 하나가 도는지 마는지만 다르다.
              onScratchpadSave={() => saveSelectionToScratchpad(elements)}
              canScratchpadSave={selection.size > 0}
              scratchpadDropActive={dropActive}
              onScratchpadPlace={placeFromScratchpad}
              // 가져오기도 같은 규칙이다 — 만드는 입구는 하나이고, 놓은 뒤의 선택은 이 층이
              // 소유한다(불변식 K9).
              onSvgImport={placeFromImport}
              // 그룹 묶음. 떠 있는 줄이 그리는 **그 컴포넌트**를 도크도 그린다 — 도크는
              // 자리를 주고 이름을 달 뿐이다(SPEC-CANVAS-004 REQ-08).
              groupTools={groupTools}
              // 앵커 묶음. 그룹과 **같은 규율**이다 — 짓는 자리는 이 층 하나이고 도크는
              // 자리와 이름만 더한다(SPEC-CANVAS-011 AC-61).
              anchorTools={anchorTools}
              // 연결선 묶음. 앞의 둘과 **같은 규율**이다 — 짓는 자리는 이 층 하나이고
              // 도크는 자리와 이름만 더한다(SPEC-CANVAS-011 AC-61).
              connectorTools={connectorTools}
            />
          </CanvasScratchpadDropContext>,
          dockHost,
        )}
      {/* 선택 윤곽 — **해석기를 지난 노드**를 두른다(SPEC-CANVAS-009 M3).

          `elements` 를 직접 돌면 부품 키는 어느 원소와도 만나지 못해 골라도 테두리가 서지
          않는다. `selectedNodes` 는 부품을 캔버스 단위 의사 노드로 풀어 주므로 `outlineBox`
          가 한 글자도 바뀌지 않고 걸리고, 판정하는 상자와 보이는 상자가 여전히 같은
          함수에서 나온다(위험 R1). */}
      {selectedNodes(elements, selection).map((el) => {
        const box = outlineBox(el, projection, textWidths);
        return (
          <div
            key={el.id}
            data-testid={`canvas-selection-${el.id}`}
            // 장식이다 — 무엇이 골라졌는지는 목록 편집기의 행이 알린다(T10).
            aria-hidden="true"
            className="pointer-events-none absolute border-2 border-dashed border-blue-500/80"
            style={{
              left: box.x,
              top: box.y,
              width: Math.max(box.w, MIN_OUTLINE_PX),
              height: Math.max(box.h, MIN_OUTLINE_PX),
            }}
          />
        );
      })}
      {/* **앵커 점** — 선이 붙는 자리(SPEC-CANVAS-011 REQ-02 · M3 · M3').

          손잡이 **앞에** 그린다. 둘은 겹칠 수 있고(모서리 앵커와 모서리 핸들은 같은
          자리다), 그때 위에 있어야 하는 것은 **잡히는 쪽**이다 — 점은 표식이라 가려져도
          잃는 것이 없지만, 손잡이가 가려지면 크기 조절이 죽는다.

          자리는 `anchorPoints` **한 함수**에서 나온다(REQ-02). 빼기 판정이 쓰는 것도 같은
          함수이므로(`applyAnchorGesture`), 보이는 점과 집히는 점이 갈릴 수 없다(위험 R1).

          좌표계는 선택 윤곽선·손잡이와 같다: 스테이지 로컬 px 을 `left`/`top` 에 그대로
          쓰고 `-translate-*-1/2` 로 중심을 맞춘다.

          **장식이다**(`aria-hidden`). 무엇이 어디에 붙을 수 있는지는 도구 단추의 이름이
          알리고(`toolAnchor`), 점 하나하나는 듣는 사람에게 읽을 것이 없는 좌표다 —
          아홉 × 요소 수만큼의 이름을 읽히면 그것은 알림이 아니라 소음이다. 그래서 위험
          R8 의 가드에 **걸린 채** `pointer-events-none` 을 갖는다. */}
      {anchorHosts.map((el) => {
        const points = anchorPoints(el, projection, textWidths);
        return [...points].map(([id, point]) => {
          const px = projectPoint(point, projection);
          return (
            <div
              key={`${el.id}/${id}`}
              data-testid={`canvas-anchor-dot-${el.id}-${id}`}
              aria-hidden="true"
              className={ANCHOR_DOT_CLASS}
              style={{ left: px.x, top: px.y }}
            />
          );
        });
      })}
      {handleHost !== undefined &&
        handlePositions(handleHost, projection, {
          measuredWidth: resolveMeasuredWidth(textWidths[handleHost.id]),
        }).map((handle) => (
          <button
            key={handle.id}
            type="button"
            data-testid={`canvas-handle-${handle.id}`}
            // 초점을 받는 진짜 요소다 — 칠한 손잡이였다면 이 한 줄이 통째로 사라진다(REQ-01).
            aria-label={t(HANDLE_ARIA_KEYS[handle.id])}
            className={cn(HANDLE_CLASS, HANDLE_CURSOR[handle.id])}
            style={{ left: handle.point.x, top: handle.point.y }}
            onPointerDown={(event) => startHandleDrag(handleHost, handle.id, event)}
          />
        ))}
      {/* **연결선 손잡이** — 끝점 둘 + 중간점마다 (SPEC-CANVAS-011 M9 · REQ-07).

          **8핸들과 이름 공간이 다르다**(`canvas-connector-handle-*`). 섞으면
          `canvas-handle-nw` 가 "상자 왼쪽 위" 인지 "연결선의 어떤 자리" 인지 이름만으로는
          답하지 못하고, 연결선에 8핸들이 서지 않았음을 재는 자리가 가려낼 것을 잃는다.

          갯수가 저술마다 다르므로 `CanvasHandleId` 를 넓히지 않고 **별도 갈래**로 둔다
          (plan §M9 2) — 넓히면 `HANDLE_ARIA_KEYS` · `HANDLE_CURSOR` 두 표가 뜻 없는 항목을
          하나씩 갖고, 그 항목에는 답할 문장이 없다.

          자리는 `resolveConnector` **한 함수**에서 온다 — 그리는 쪽·잡는 쪽이 보는 그
          목록이다. 끊긴 연결이면 목록 자체가 부재이므로 손잡이도 서지 않고 예외도 없다
          (REQ-08). `connectorHost` 와 `handleHost` 는 배타적이라 이 갈래와 위 갈래가
          함께 서는 일이 없다.

          좌표계는 8핸들과 같다: 투영된 점을 `left`/`top` 에 그대로 두고 `-translate-*-1/2`
          로 중심을 맞춘다. 반 칸을 미리 빼면 그 산술이 곧 두 번째 투영이 된다(AC-E2).

          **앵커 도구가 켜진 동안에는 포인터를 먹지 않는다.** 끝 손잡이는 제가 붙은 앵커와
          **정확히 같은 자리**에 서므로(그 점이 곧 그 끝이다), 먹으면 그 앵커를 겨눈 누름을
          손잡이가 가로챈다 — 위 §앵커가 잉크를 이긴다가 히트 층에서 막은 바로 그 구멍이
          DOM 층에서 되살아나는 형상이다. 도구가 몸짓의 뜻을 갈아 끼우는 동안 이 표면은
          앵커의 것이고, 그래서 앵커 점이 표식인 것과 **같은 이유로** 손잡이도 물러선다. */}
      {connectorHost !== undefined &&
        connectorPoints?.map((point, index) => {
          const handle = connectorHandleAt(index, connectorPoints.length);
          const name = connectorHandleName(handle);
          const px = projectPoint(point, projection);
          return (
            <button
              key={name}
              type="button"
              data-testid={`canvas-connector-handle-${name}`}
              aria-label={connectorHandleLabel(t, handle)}
              // **보는 사람에게도 같은 말을 한다.** `aria-label` 만 두면 "두 번 누르면
              // 없어진다" 는 사실이 보조기기를 쓰는 사람에게만 있고, 마우스를 쓰는 사람은
              // 그 길이 아예 없다고 읽는다 — 그것이 신고된 형상이다. 도구 단추들이 이미
              // 쓰는 관용구 그대로 **같은 키**를 두 자리에 건다(`CanvasConnectorTools`).
              title={connectorHandleLabel(t, handle)}
              className={cn(
                CONNECTOR_HANDLE_CLASS[handle.kind === 'end' ? handle.side : 'mid'],
                CONNECTOR_HANDLE_CURSOR,
                TOOL_ANCHOR_GESTURE[tool] && 'pointer-events-none',
              )}
              style={{ left: px.x, top: px.y }}
              onPointerDown={(event) => startConnectorHandleDrag(connectorHost, handle, event)}
            />
          );
        })}
      {/* **마키 사각형** — 지금 감싸고 있는 영역(SPEC-CANVAS-009 결정 5).

          **표시 층이 아니다**(불변식 I23). I23 이 막는 결함의 형상은 "**조건 없이** 그려지는
          층을 다스리는 컨트롤이 이 표면에는 없다" 이며, 006 에서 그것이 실제로 터진 자리는
          격자·흐림·경계였다 — 셋 다 손을 떼도 그대로 남고, 남아 있는 동안 사용자가 바꿀 수
          있어야 하는데 바꿀 손잡이가 도크에만 있었다.

          이 사각형은 그 어느 쪽도 아니다. **손이 눌려 있는 동안에만** 존재하고, 그리는 것도
          거두는 것도 **그 몸짓 자신**이다. 다스릴 지속 상태가 없으므로 다스릴 컨트롤도 없고,
          컨트롤을 하나 지어 붙인다면 그것은 누를 시간이 존재하지 않는 단추가 된다. 그래서
          `LAYER_CONTROLS` 표에 행을 더하지 않고, 선택 윤곽선과 **같은 면제**를 받는다 —
          그쪽의 근거("표시 층이 아니라 선택의 그림자다")가 여기서는 한 걸음 더 곧다:
          이것은 **몸짓 자신의 그림자**다.

          **위험 R8 의 가드는 그대로 통과한다.** 칠하고 `aria-hidden` 이므로 그 가드에
          **걸리며**, 걸린 채로 `pointer-events-none` 을 갖는다. 가드가 지키는 문장
          ("장식은 포인터를 먹지 않는다")을 우회하지 않고 만족시킨다 — 세 이름을 손으로 적은
          쪽 목록에는 더하지 않는다(그 셋은 006 의 표시 층이고 이것은 아니다).

          좌표계는 선택 윤곽선과 **같다**: 스테이지 로컬 px 을 `left`/`top` 에 그대로 쓴다.
          루트에 `overflow` 가 없으므로 저술 여백까지 음수 좌표로 뻗는다(닿는 면과 같은 방식). */}
      {/* **긋는 중인 선** — 누른 앵커에서 지금 손까지(SPEC-CANVAS-011 M8 · REQ-03).

          **표시 층이 아니다**(불변식 I23). 마키 사각형이 그 목록 밖인 근거가 여기에도
          한 글자도 다르지 않게 붙는다: 손이 눌려 있는 동안에만 존재하고, 그리는 것도
          거두는 것도 **그 몸짓 자신**이다. 다스릴 지속 상태가 없으므로 다스릴 컨트롤도
          없고, 컨트롤을 지어 붙인다면 누를 시간이 존재하지 않는 단추가 된다. 그래서
          `LAYER_CONTROLS` 표에 행을 더하지 않는다 — 이것은 **몸짓 자신의 그림자**다.

          **위험 R8 의 가드를 우회하지 않고 만족시킨다.** 칠하고 `aria-hidden` 이므로 그
          형상 가드에 **걸리며**, 걸린 채로 `pointer-events-none` 을 갖는다. `<div>` 인 것이
          그 사실의 절반이다 — `<svg>` 였다면 가드의 `instanceof HTMLElement` 에 잡히지 않아
          조용히 면제되었을 것이고, 면제를 얻으려고 원소 종류를 고르는 것은 가드를
          **우회하는** 일이다.

          좌표계는 선택 윤곽선·마키와 같다: 스테이지 로컬 px 을 `left`/`top` 에 그대로 쓴다. */}
      {connectorDraw !== null && (
        <div
          data-testid="canvas-connector-preview"
          aria-hidden="true"
          className={CONNECTOR_PREVIEW_CLASS}
          style={previewLineStyle(connectorDraw.origin, connectorDraw.point)}
        />
      )}
      {marquee !== null &&
        (() => {
          const box = marqueeRect(marquee.origin, marquee.point);
          return (
            <div
              data-testid="canvas-marquee"
              aria-hidden="true"
              className="pointer-events-none absolute border border-dashed border-blue-500 bg-blue-500/10"
              style={{ left: box.x, top: box.y, width: box.w, height: box.h }}
            />
          );
        })()}
    </div>
  );
}
