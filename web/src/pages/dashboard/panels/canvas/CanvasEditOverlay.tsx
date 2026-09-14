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
// **영역 선택은 빈 자리의 주 버튼 누름으로 시작한다**(SPEC-CANVAS-010). 맨손으로 끌면
// 사각형이 서고 그 안에 **온전히 든** 것들이 선택을 **갈아 끼우며**, Shift·Ctrl·Cmd 를
// 누른 채 끌면 같은 것들이 지금 선택에 **더해진다**. 009 는 이 몸짓을 modifier 로만 열어
// 두었고 그 근거는 "문턱(몇 px 움직이면 마키로 승격)으로 가르면 `previewPan` 이 누름에서
// 이미 팬을 시작했고 그것을 무를 신호가 없다" 였다 — 그 근거는 **문턱**에 대해서는 지금도
// 참이지만, 문턱을 쓰지 않고 **누르는 순간 곧장** 가져가면 무를 것이 없다. 그 층은
// `defaultPrevented` 로 임자를 가리므로 누름을 소비하면 팬은 시작조차 하지 않는다.
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
// 이다. 좁아진 만큼 위 세 조작 중 캔버스 안에서 잃는 것은 **주 버튼 끌기 팬 하나**이고,
// 휠 확대·크기 조절·오른쪽 버튼은 한 글자도 달라지지 않는다(누름을 쓰지 않거나 버튼이
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
import { isGroup, type CanvasNode } from './group/groupTypes';
import {
  DEFAULT_FONT_SIZE,
  type BoxGeometry,
  type CanvasElement,
  type CanvasPrimitiveKind,
  type Geometry,
  type LineGeometry,
} from './canvasConfig';
import {
  appendElement,
  appendImportedElements,
  appendPathElement,
  nextElementId,
  seedOffset,
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
  removeNodes,
  sendToBack,
  snapDelta,
  type AlignAxis,
  type AlignMode,
} from './canvasEditArrange';
import { useCanvasEditSelection, type CanvasSelection } from './canvasEditContext';
import { useCanvasEditDockHost } from './canvasEditDockHost';
import type { ImportedShapeSpec, ImportedTextSpec } from './svgimport/svgImportPlan';
import { useCanvasStageGrid } from './canvasStageGrid';
import { DEFAULT_WORKSPACE_ZOOM } from './canvasWorkspace';
import {
  projectBox,
  projectLine,
  projectPoint,
  resolveTextOrigin,
  stageLattice,
  unprojectBox,
  unprojectPoint,
  type CanvasBox,
  type CanvasDelta,
  type CanvasProjection,
  type PxBox,
  type PxPoint,
  type StageSize,
} from './canvasGeometry';
import { hitTest } from './canvasHitTest';

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

/** 진행 중인 드래그. `null` 이면 유휴. */
type DragState = MoveDrag | BoxResizeDrag | LineResizeDrag | FontResizeDrag;

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
 * 핸들의 겉모습. 크기·형상·흰 테두리는 `FloorPlanTransformOverlay` 의 모서리 핸들 어휘를
 * 그대로 따르고(같은 대시보드 안에서 손잡이가 두 모양이 되지 않게 한다), **색만** 캔버스
 * 선택 외곽선과 같은 파랑으로 둔다 — 한 선택 안에서 외곽선과 손잡이가 다른 색이면 둘이
 * 다른 것을 가리키는 것처럼 보인다.
 *
 * 자리는 `left`/`top` 에 **투영된 핸들 점 그대로**를 두고 변환으로 중심을 맞춘다. 반 칸을
 * 미리 빼서 넣으면 그 산술이 곧 두 번째 투영이 되어 AC-E2 가 지키려는 성질이 깨진다.
 */
const HANDLE_CLASS =
  'pointer-events-auto absolute h-2.5 w-2.5 -translate-x-1/2 -translate-y-1/2 rounded-sm ' +
  'border border-white bg-blue-500 shadow focus:outline-none focus:ring-2 focus:ring-blue-300';

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
 * 스크린 리더에 알리는 단축키 목록. 값은 W3C 가 정한 키 이름이라 **번역하지 않는다**
 * (번역하면 보조기기가 알아듣지 못한다). 사람이 읽는 설명은 `keyboardHint` ·
 * `deleteHint` 가 따로 낸다.
 */
const EDIT_KEY_SHORTCUTS =
  'Delete Backspace ' +
  'ArrowUp ArrowDown ArrowLeft ArrowRight ' +
  'Shift+ArrowUp Shift+ArrowDown Shift+ArrowLeft Shift+ArrowRight';

// --- 순수 도우미 ---------------------------------------------------------

/** 유효한 글자 크기(px). `canvasHitTest.resolveFontSize` 와 같은 판정이다. */
function resolveFontSize(size: number | undefined): number {
  if (size === undefined) return DEFAULT_FONT_SIZE;
  return Number.isFinite(size) && size > 0 ? size : DEFAULT_FONT_SIZE;
}

/** 실측 글자 폭. 아직 한 프레임도 그리지 않았으면 0 이다(AC-E7). */
function resolveMeasuredWidth(width: number | undefined): number {
  return width !== undefined && Number.isFinite(width) && width > 0 ? width : 0;
}

/**
 * 선택 외곽선이 두를 상자(스테이지 로컬 CSS px, **양수 범위로 정규화**).
 *
 * 투영은 `canvasGeometry` 의 같은 함수를 지난다 — 렌더·히트·외곽선이 한 투영을 공유하면
 * 셋이 갈라질 수 없다(위험 R1).
 *
 * 문구 상자의 **세로 기준이 이 함수에서 가장 틀리기 쉬운 지점이다**(위험 R8).
 * `drawElement.TEXT_BASELINE` 이 `'middle'` 이므로 기준점 y 는 상자의 **세로 중심**이고,
 * 따라서 상단은 `기준점 y - fontSize/2` 다. 상단으로 착각하면 외곽선이 글자 아래로 반 줄
 * 내려가 앉는다. `canvasHitTest` 의 문구 상자와 같은 식이어야 **보이는 대로 잡힌다**.
 *
 * **경로는 rect 와 같은 상자다.** 종전의 `default:` 는 경로의 상자 기하를 문구 기준점으로
 * 읽어(구조적으로 대입된다 — 가정 A6) 외곽선을 실측 글자 폭 0 · 기본 글자 크기의 작은
 * 상자로 그렸다. 컴파일러가 울지 않던 자리이므로 갈래를 이름으로 적는다.
 */
function outlineBox(
  el: CanvasNode,
  proj: CanvasProjection,
  textWidths: Readonly<Record<string, number>>,
): PxBox {
  switch (el.kind) {
    // 그룹의 윤곽은 **제 상자**다(REQ-08). 부품의 합집합을 여기서 다시 재지 않는다 —
    // 상자는 묶는 순간 적힌 **저장된 값**이고(가정 A16), 파생으로 되재면 8핸들이
    // 잡는 상자와 윤곽이 두는 상자가 갈라진다. 그 갈라짐은 "늘리면 테두리만 안 따라온다"
    // 로만 보고되는 부류다.
    case 'group':
    case 'rect':
    case 'ellipse':
    case 'path': {
      const box = projectBox(el.geometry, proj);
      return normalizeBox(box);
    }
    case 'line': {
      const line = projectLine(el.geometry, proj);
      return normalizeBox({
        x: line.x1,
        y: line.y1,
        w: line.x2 - line.x1,
        h: line.y2 - line.y1,
      });
    }
    case 'text': {
      const width = resolveMeasuredWidth(textWidths[el.id]);
      const fontSize = resolveFontSize(el.style.fontSize);
      const origin = resolveTextOrigin(
        projectPoint(el.geometry, proj),
        el.style.align ?? 'left',
        width,
      );
      return normalizeBox({ x: origin.x, y: origin.y - fontSize / 2, w: width, h: fontSize });
    }
  }
}

/** 음수 크기를 양수 범위로 편다. 001 은 음수 크기 박스를 그릴 수 있게 해 두었다. */
function normalizeBox(box: PxBox): PxBox {
  return {
    x: Math.min(box.x, box.x + box.w),
    y: Math.min(box.y, box.y + box.h),
    w: Math.abs(box.w),
    h: Math.abs(box.h),
  };
}

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
 */
function nodeForKey(elements: readonly CanvasNode[], key: string): CanvasNode | undefined {
  const { nodeId, partId } = parseFrameKey(key);
  if (partId === undefined) return elements.find((el) => el.id === nodeId);
  return partInCanvasUnits(elements, nodeId, partId);
}

/** 고른 것들을 노드로 푼다. 가리킬 것이 없는 키는 조용히 빠진다. */
function selectedNodes(
  elements: readonly CanvasNode[],
  selection: CanvasSelection,
): CanvasNode[] {
  const out: CanvasNode[] = [];
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

  const rootRef = useRef<HTMLDivElement>(null);

  /**
   * 직전 누름 — **더블클릭 판정에만** 쓴다(SPEC-CANVAS-009 0.3.0 §그룹 진입).
   *
   * `useRef` 인 것에 뜻이 있다. 이 값은 화면에 아무것도 그리지 않으므로 상태로 들면
   * 누를 때마다 렌더가 한 번씩 더 돌고, 001 이 지은 유휴 정지가 그만큼 흔들린다.
   */
  const lastPressRef = useRef<LastPress | null>(null);

  const dragRef = useRef<DragState | null>(null);
  /** 아직 반영하지 않은 마지막 포인터 상태(스테이지 로컬 px + 보조키). */
  const pendingRef = useRef<PendingPointer | null>(null);
  /** 예약된 합류 프레임. `null` 이면 예약 없음. */
  const frameRef = useRef<number | null>(null);

  /**
   * 프레임 콜백이 늦게 실행될 때 **최신** props 를 보게 한다. 드래그 중에는 매 프레임
   * `elements` 가 새로 오므로, 예약 시점의 클로저를 그대로 쓰면 한 프레임 뒤처진 배열에
   * 기하를 써 넣게 된다.
   */
  const latestRef = useRef({ elements, projection, onElementsChange, snapToGrid, gridStep });
  useEffect(() => {
    latestRef.current = { elements, projection, onElementsChange, snapToGrid, gridStep };
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

  // 언마운트 정리 — 예약된 합류 프레임을 남기지 않는다.
  useEffect(
    () => () => {
      if (frameRef.current !== null) cancelAnimationFrame(frameRef.current);
      frameRef.current = null;
      dragRef.current = null;
      pendingRef.current = null;
    },
    [],
  );

  const handlePointerDown = (event: React.PointerEvent<HTMLDivElement>): void => {
    if (!enabled) return;
    const host = event.currentTarget;
    const frame = pointerFrameOf(host.getBoundingClientRect(), stage);
    const point = stagePoint(event.clientX, event.clientY, frame);

    // Shift·Ctrl·Cmd 는 **고르기 전용** 조작이다(`PanelDragLayer` 와 같은 규칙). 히트가
    // 있을 때의 뜻("이것을 선택에 더하거나 뺀다")과 빈 지점에서의 뜻("사각형으로 감싼
    // 것들을 선택에 더한다")은 **같은 한 낱말**이다 — 둘 다 "더한다" 이고, 어느 쪽도
    // 배치를 건드리지 않는다.
    const additive = event.shiftKey || event.ctrlKey || event.metaKey;

    const hit = hitTest(elements, point, projection, textWidths);
    if (hit === undefined) {
      // **주 버튼이 아니면 종전 그대로다.** 선택만 비우고 이벤트를 소비하지 않는다 —
      // 오른쪽 버튼은 상황 메뉴의 것이고, 가운데 버튼은 애초에 여기까지 오지 않는다
      // (`previewPan` 이 캡처 단계에서 끊는다). 여기서 소비하면 캔버스 위에서만
      // 상황 메뉴가 사라지는 화면이 된다.
      if (event.button !== PRIMARY_BUTTON) {
        if (!additive && selection.size > 0) setSelection(EMPTY_SELECTION);
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
      // 그 대가로 **캔버스 안에서 주 버튼 끌기로 화면을 옮기는 길은 닫힌다.** 남는 길은
      // 둘이며 둘 다 이미 서 있다: 가운데 버튼(그 파일의 명시적 우회로)과, 고른 것이
      // 없을 때의 방향키(아래 `handleKeyDown` 이 흘려보내면 그 층이 받는다). 가운데
      // 버튼이 없는 손도 빈 자리를 한 번 누르면 선택이 비고 초점이 이 탭 정거장에 앉으므로
      // 곧바로 방향키를 쓸 수 있다. 캔버스 **밖**(게이지·차트·히트맵 미리보기)은 아무도
      // 몸짓을 가져가지 않으므로 주 버튼 끌기가 종전 그대로 팬이다.
      event.preventDefault();
      event.stopPropagation();
      // 히트 때와 같은 이유다 — 바로 위 `preventDefault` 가 브라우저의 기본 초점 이동을
      // 막으므로, 이 한 줄이 없으면 방금 감싸 고른 것을 방향키로 옮길 수 없다(T15).
      host.focus();

      // **맨손은 갈아 끼우고 modifier 는 더한다.** 그것이 게이트를 걷어낸 뒤 modifier 에
      // 남은 뜻이며, 히트 경로의 `additive`("이것을 선택에 더하거나 뺀다")와 여전히 한
      // 낱말이다. 맨손을 합집합으로 두면 modifier 는 아무것도 가르지 않는 장식이 된다.
      //
      // 비우는 일을 **누르는 순간** 한다. 사각형이 고르는 일은 이동에서만 일어나므로,
      // 여기서 비우지 않으면 "빈 자리를 눌러 선택을 푼다" 는 009 이전부터의 뜻이 움직이지
      // 않은 몸짓에서 조용히 사라진다. 비운 그 값이 곧 합집합의 좌변이므로 규칙도 하나다.
      const base = additive ? selection : EMPTY_SELECTION;
      if (base !== selection) setSelection(base);
      // 빈 자리를 누르면 **연타 사슬이 끊긴다**(SPEC-CANVAS-009 0.3.0). 끊지 않으면 빈
      // 곳을 거쳐 같은 부품을 다시 누른 것이 더블클릭으로 읽혀, 사용자가 한 번도 겹쳐
      // 누르지 않았는데 그룹 안으로 들어간다.
      lastPressRef.current = null;
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
    const hitKey = frameKey(hit.nodeId, hit.partId);
    const second = isSecondPress(lastPressRef.current, event.timeStamp, point, hitKey);
    lastPressRef.current = { at: event.timeStamp, point, key: hitKey };

    const key = pressTargetKey(hit, selection, second);
    const enteredPart = isPartKey(key);

    // **부품 선택은 언제나 하나이고, 그 그룹과 함께 서지 않는다**(REQ-01-a · REQ-01-b).
    // modifier 를 부품 경로에서 흘려보내는 것에 뜻이 있다: 부품을 무리에 더할 수 있게 하면
    // "서로 다른 그룹의 부품 둘" 이라는 뜻이 정의되지 않은 상태가 만들어지고(A1), 8핸들이
    // 두 상자에 서는 화면이 그 뒤를 따른다. 그룹을 고르는 경로는 최상위 원소와 같으므로
    // modifier 가 종전 그대로 산다.
    const picked = enteredPart ? new Set([key]) : nextSelection(selection, key, additive);
    if (picked !== selection) setSelection(picked);
    if (additive && !enteredPart) return;

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
   * 드롭 존 강조를 **바뀔 때만** 갈아 끼운다. 같은 값을 다시 넣으면 React 가 렌더를
   * 건너뛰기는 하나, 그 전에 갱신을 예약하는 비용은 매 이동마다 든다(AC-E4 의 규율).
   */
  const setDropHighlight = (next: boolean): void => {
    if (dropActiveRef.current === next) return;
    dropActiveRef.current = next;
    setDropActive(next);
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
    // **영역 선택이 먼저다.** 둘은 배타적이므로(마키는 히트가 없을 때만, 이동 드래그는
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
   * 고른 것들을 지운다 — **규칙은 `canvasEditArrange.removeNodes` 한 곳에 있다**
   * (SPEC-CANVAS-010). 목록 편집기의 휴지통이 같은 함수를 지나므로 "목록에서 지웠는가
   * 캔버스에서 지웠는가" 에 따라 결과가 갈릴 수 없다.
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
    const next = removeNodes(elements, selection);
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
    const picked = elements.filter((el) => selection.has(el.id));
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
  const handleHost =
    selection.size === 1 ? nodeForKey(elements, [...selection][0]!) : undefined;

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
      className="absolute inset-0 z-20 touch-none"
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerUp}
      onPointerCancel={handlePointerCancel}
      onKeyDown={handleKeyDown}
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
        {t('dashboard.canvas.edit.deleteHint')}
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
