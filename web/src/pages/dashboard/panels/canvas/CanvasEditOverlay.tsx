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
// **빈 지점 누름을 가로채지 않는다**(REQ-05 · AC-E3 · 위험 R2). 설정 미리보기에는 이미
// 휠 확대와 패널 크기 조절이 걸려 있고, 대시보드 패널은 편집모드에서 몸통을 잡아 옮긴다.
// 아무 데나 잡아도 끌리면 그것들과 부딪히므로 — `PanelDragLayer` 헤더가 같은 이유로 같은
// 결정을 했다 — **히트가 있을 때만** `preventDefault`/`stopPropagation` 한다. 히트가 없으면
// 선택만 비우고 이벤트를 그대로 흘려보낸다. 핸들은 스스로 이벤트를 소비하므로(자기
// `pointer-events-auto` 위에서 `stopPropagation`) 핸들을 잡은 포인터는 몸통 히트 테스트에
// 닿지 않는다.
//
// 포인터를 이 층의 루트에서 받는 이유: 캔버스는 **칠해진 픽셀**이라 잡을 DOM 노드가 없고,
// 핸들·팔레트(T9)는 어차피 이 층 안의 진짜 DOM 요소여서 같은 루트 아래 사는 편이
// 좌표 기준이 하나로 유지된다. spec.md §표면의 오버레이 슬롯은 처음에 `<canvas>` 가
// 포인터를 받는 형상을 그렸고 T3 이 그대로 통과 슬롯 넷을 달아 두었으나, 이 층이 서면서
// 그 넷은 **호출부가 하나도 없는 API** 가 되어 걷어냈다(문서도 함께 고쳤다 — spec.md
// HISTORY 0.2.0). AC-E3 이 약해지지는 않는다 — 그 인수 기준이 요구하는 것은 "어느 노드가
// 받는가" 가 아니라 **"소비하지 않는다"** 이며, 그 책임은 처음부터 이 층의 것이다.
//
// 좌표 변환은 이 파일에 **딱 하나** 있다: 포인터의 화면 좌표 → 스테이지 로컬 px → 정규화
// (축마다 스테이지 길이로 나눈다). 이것은 §히트 테스트가 금지한 "도형별 역산" 이
// 아니다 — 히트 **판정**은 여전히 순방향 투영 뒤 px 공간에서 이뤄지고(`canvasHitTest`),
// 여기서 나누는 것은 판정이 아니라 **이동량과 포인터 자리**다. 판정과 좌표는 축이 다르므로
// 둘이 모순되지 않는다. px 로 둘 수 없는 이유는 저장 좌표가 정규화이기 때문이다.
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
// 격자선" 으로 죄는 계산이라 1px 미세 이동을 0 으로 만들어 버린다 — 격자를 켜는 순간
// 방향키가 죽는 것은 접근성 후퇴다. 격자 칸 단위 이동은 Shift 가 이미 제공하고, 그 칸은
// **화면에 그려진 바로 그 칸**이므로 격자 어휘는 여전히 하나다 — 그리는 값 · 붙는 값 ·
// Shift 한 칸이 전부 `gridStep` 한 변수에서 `squareGridSteps` 로 파생된다(원 함수
// `snapOffsetToGrid` 는 이 파일에서 부르지 않는다 — 위험 R6).
//
// 그 칸은 **정사각형**이다. 백분율은 제 축 길이에 대한 값이라 두 축에 같은 수를 주면
// 정사각형이 아닌 패널에서 칸이 직사각형이 되는데, 여기는 그림을 그리는 자리다. 그래서
// 한 물리 칸에서 두 축의 백분율을 파생하며, 그 결과 Shift+→ 와 Shift+↓ 는 **서로 다른
// 백분율**을 쓰되 **같은 px** 를 옮긴다.
//
// @spec SPEC-CANVAS-002 REQ-01 / REQ-02 / REQ-03 / REQ-04 / REQ-05 / REQ-06

import { useCallback, useEffect, useId, useRef, useState } from 'react';
import { createPortal } from 'react-dom';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import { PanelEditGrid } from '../../PanelEditGrid';
import { EMPTY_SELECTION, nextSelection } from '../charts/panelEditSelection';
import { CanvasEditDockBody } from './CanvasEditDock';
import {
  DEFAULT_FONT_SIZE,
  type BoxGeometry,
  type CanvasElement,
  type CanvasElementKind,
  type Geometry,
  type LineGeometry,
} from './canvasConfig';
import { appendElement } from './canvasElementFactory';
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
  type NormalizedDelta,
} from './canvasEditGeometry';
import {
  CANVAS_GRID_STEP_PERCENT,
  alignDeltas,
  bringToFront,
  sendToBack,
  snapDelta,
  squareGridSteps,
  type AlignAxis,
  type AlignMode,
} from './canvasEditArrange';
import { useCanvasEditSelection } from './canvasEditContext';
import { useCanvasEditDockHost } from './canvasEditDockHost';
import {
  projectBox,
  projectLine,
  projectPoint,
  resolveTextOrigin,
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
  /** 배열 순서 = 그리기 순서(뒤가 위). 히트 순회의 z-order 이기도 하다. */
  elements: readonly CanvasElement[];
  /** 표면이 잰 스테이지 CSS px 크기. 이 층은 크기를 **스스로 재지 않는다**. */
  stage: StageSize;
  /** 직전 프레임이 잰 글자 폭(요소 id → px). 없으면 기준점 둘레로 폴백한다(AC-E7). */
  textWidths: Readonly<Record<string, number>>;
  /**
   * 드래그가 만든 새 요소 배열. **끄는 동안 계속** 호출되어 그림이 손을 따라온다
   * (`PanelDragLayer` 의 "미리보기가 즉시 따라와야 어디에 놓일지 보인다" 와 같은 계약).
   */
  onElementsChange: (next: CanvasElement[]) => void;
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
   * 격자 붙임의 **기준 상자**(잡는 순간의 스테이지 로컬 px, 양수 범위).
   *
   * 잡은 요소 하나가 기준이며 무리는 같은 델타로 따라온다 — 무리의 각 요소를 저마다
   * 격자에 붙이면 끌려가는 동안 무리가 서로 흩어진다. 격자 붙임이 꺼져 있으면 쓰이지
   * 않으며, **잡는 순간의 값**이라 드래그 도중에 다시 재지 않는다(`frame` 과 같은 규율).
   */
  anchor: PxBox;
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
 * 방향키 한 번의 **미세 이동**(CSS px). 화면 양이지 정규화 양이 아니다 — 집기 여유
 * (`HIT_TOLERANCE_PX`)를 화면 양으로 둔 것과 같은 판단이며(가정 A2), 정규화 공간에서
 * 재면 같은 숫자가 큰 패널에서는 성큼 뛰고 작은 패널에서는 꿈쩍하지 않는다.
 *
 * 1px 인 것은 이 조작의 뜻이 "손으로는 낼 수 없는 정밀도" 이기 때문이다. 성큼 옮기고
 * 싶은 사람에게는 Shift 가 한 격자 칸을 준다.
 */
const NUDGE_PX = 1;

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
 * 스크린 리더에 알리는 단축키 목록. 값은 W3C 가 정한 키 이름이라 **번역하지 않는다**
 * (번역하면 보조기기가 알아듣지 못한다). 사람이 읽는 설명은 `keyboardHint` 가 따로 낸다.
 */
const NUDGE_KEY_SHORTCUTS =
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
 */
function outlineBox(
  el: CanvasElement,
  stage: StageSize,
  textWidths: Readonly<Record<string, number>>,
): PxBox {
  switch (el.kind) {
    case 'rect':
    case 'ellipse': {
      const box = projectBox(el.geometry, stage);
      return normalizeBox(box);
    }
    case 'line': {
      const line = projectLine(el.geometry, stage);
      return normalizeBox({
        x: line.x1,
        y: line.y1,
        w: line.x2 - line.x1,
        h: line.y2 - line.y1,
      });
    }
    default: {
      const width = resolveMeasuredWidth(textWidths[el.id]);
      const fontSize = resolveFontSize(el.style.fontSize);
      const origin = resolveTextOrigin(
        projectPoint(el.geometry, stage),
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
 */
function handleDragState(
  el: CanvasElement,
  handle: CanvasHandleId,
  common: DragCommon,
): DragState | null {
  // 글자 크기 핸들은 기하를 보지 않으므로 종류와 맞춰 볼 것이 없다.
  if (handle === 'font') {
    return { ...common, mode: 'font', nodeId: el.id, fontSize: resolveFontSize(el.style.fontSize) };
  }
  if (handle === 'p1' || handle === 'p2') {
    if (el.kind !== 'line') return null;
    return { ...common, mode: 'line', nodeId: el.id, endpoint: handle, geometry: el.geometry };
  }
  if (el.kind !== 'rect' && el.kind !== 'ellipse') return null;
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
  elements: readonly CanvasElement[],
  nodeId: string,
  fontSize: number,
): CanvasElement[] {
  return elements.map((el): CanvasElement =>
    el.kind === 'text' && el.id === nodeId ? { ...el, style: { ...el.style, fontSize } } : el,
  );
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
  stage,
  textWidths,
  onElementsChange,
}: CanvasEditOverlayProps) {
  const { t } = useTranslation();
  // provider 가 없으면 로컬 선택이다 — 대시보드에 놓인 패널에는 목록 편집기가 없다.
  const { selection, setSelection } = useCanvasEditSelection();
  /**
   * 편집 도구를 그릴 자리(패널 밖). `null` 이면 도크가 없다는 뜻이고, 그때 도구는
   * **아무 데도 그려지지 않는다** — 대시보드에 놓인 패널이 그 경우다.
   */
  const dockHost = useCanvasEditDockHost();

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
   * 격자 간격(%). **이 값 하나가 그려지는 격자와 붙는 눈금을 함께 정한다** — 아래에서
   * `PanelEditGrid` 로 가는 축 쌍과 `snapDelta` 의 `step` 이 모두 이 변수에서 파생되고
   * (`squareGridSteps`), 그것이 이 값을 두 곳에 나누어 두지 않는 유일한 이유다. 보이는
   * 간격과 붙는 간격이 갈라지면 화면이 거짓말을 하며, 그 거짓말은 "붙긴 붙는데 선하고
   * 안 맞는다" 로만 보고된다.
   *
   * 고를 수 있는 값은 `canvasEditArrange` 가 소유한다(격자 어휘의 주인은 여전히 그 모듈
   * 하나다). 격자 토글과 같이 **저장하지 않는 표시 상태**다(가정 A4) — config 스키마를
   * 넓히지 않으며, 이 값을 바꿔도 캔버스 props 는 그대로라 프레임을 예약하지 않는다(AC-E4).
   */
  const [gridStep, setGridStep] = useState<number>(CANVAS_GRID_STEP_PERCENT);

  /**
   * 지금 격자의 **두 축 간격**(%) — 고른 값 하나와 스테이지의 종횡비에서 파생된 정사각 칸.
   *
   * 백분율이 축마다 다른 것이 칸이 두 종류라는 뜻은 아니다. 칸은 px 하나이고, 그것을
   * 그리는 자리와 붙이는 자리가 둘 다 **제 축 길이에 대한 백분율**로만 말할 수 있어서 두
   * 번 옮겨 적을 뿐이다(`squareGridSteps`).
   *
   * 이 값을 그림(`PanelEditGrid`)과 Shift+방향키가 함께 본다. 드래그 붙임은 `snapDelta`
   * **안에서 같은 함수로 다시 파생**하므로 세 자리가 한 계산을 공유한다 — 축 쌍을 손으로
   * 조립해 넘기는 자리를 만들지 않는 것이 요점이다.
   */
  const gridCell = squareGridSteps(gridStep, stage);

  /**
   * 오버레이 루트. 핸들에서 시작한 드래그도 **루트의 상자**로 좌표를 옮기고 **루트에서**
   * 포인터를 잡는다 — 기준이 둘이 되면 몸통 드래그와 핸들 드래그가 서로 다른 원점을 믿게
   * 되고, 그 어긋남은 "핸들로 잡으면 조금 밀린다" 로만 보인다.
   */
  const rootRef = useRef<HTMLDivElement>(null);

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
  const latestRef = useRef({ elements, stage, onElementsChange, snapToGrid, gridStep });
  useEffect(() => {
    latestRef.current = { elements, stage, onElementsChange, snapToGrid, gridStep };
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
   * **clamp 하지 않는다**(가정 A5 · AC-E5). 스테이지 밖으로 나간 배치도 뜻이 있는
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
      stage: size,
      onElementsChange: emit,
      snapToGrid: snap,
      gridStep: step,
    } = latestRef.current;
    // 0 으로 나눈 이동량은 요소를 화면 밖으로 날린다.
    if (!(size.width > 0) || !(size.height > 0)) return;

    const { point } = pending;
    const px = { dx: point.x - drag.origin.x, dy: point.y - drag.origin.y };
    const pointer = { x: point.x / size.width, y: point.y / size.height };

    switch (drag.mode) {
      case 'move': {
        const raw = { dx: px.dx / size.width, dy: px.dy / size.height };
        // 켜져 있을 때만 죈다. 상한은 래퍼(`canvasEditArrange.snapDelta`)가 무한대로
        // 넘기므로 스테이지의 40% 지점에 조용히 붙잡히지 않는다(위험 R6 · AC-E5).
        // 간격은 **화면에 그려진 그 칸**이다 — 같은 `gridStep`·같은 `stage` 에서
        // `squareGridSteps` 가 파생하는 축 쌍이 `PanelEditGrid` 로도 간다.
        const delta = snap ? snapDelta(raw, drag.anchor, size, step) : raw;
        let next: CanvasElement[] = [...els];
        for (const base of drag.bases) {
          next = patchNodeGeometry(next, base.nodeId, moveGeometry(base.geometry, delta));
        }
        emit(next);
        return;
      }
      case 'box': {
        const box = resizeBox(drag.geometry, drag.handle, pointer, {
          preserveAspect: pending.shift,
        });
        emit(patchNodeGeometry(els, drag.nodeId, box));
        return;
      }
      case 'line': {
        const line = resizeLine(drag.geometry, drag.endpoint, pointer, {
          constrainAngle: pending.shift,
        });
        emit(patchNodeGeometry(els, drag.nodeId, line));
        return;
      }
      default: {
        // 유일하게 기하 통로를 지나지 않는 쓰기다. 델타가 px 인 것은 글자 크기가 **화면
        // 양**이기 때문이다 — 정규화 공간에서 재면 패널이 클수록 손이 더 가야 같은 크기가 된다.
        emit(patchNodeFontSize(els, drag.nodeId, resizeFontSize(drag.fontSize, px)));
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
    setSelection(EMPTY_SELECTION);
  }, [enabled, setSelection]);

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

    const hit = hitTest(elements, point, stage, textWidths);
    if (hit === undefined) {
      // 빈 지점: 선택만 비우고 **이벤트를 소비하지 않는다**(AC-E3). 상위의 휠 확대·
      // 패널 크기 조절·패널 끌기가 종전대로 동작해야 한다.
      if (selection.size > 0) setSelection(EMPTY_SELECTION);
      return;
    }

    // 여기부터는 우리 조작이다 — 상위가 함께 반응하지 않도록 이 이벤트만 소비한다.
    event.preventDefault();
    event.stopPropagation();

    // **초점을 손으로 옮긴다.** 바로 위 `preventDefault` 가 브라우저의 기본 초점 이동까지
    // 함께 막으므로, 이 한 줄이 없으면 방금 고른 것을 방향키로 옮길 수 없다 — 포인터로
    // 고른 사람과 키보드로 옮기려는 사람이 같은 사람이다(T15 · REQ-01).
    host.focus();

    // Shift·Ctrl·Cmd 는 **고르기 전용** 조작이다(`PanelDragLayer` 와 같은 규칙) —
    // 그 상태로 끌리면 무리에 넣으려다 배치가 흐트러진다.
    const additive = event.shiftKey || event.ctrlKey || event.metaKey;
    const picked = nextSelection(selection, hit.nodeId, additive);
    if (picked !== selection) setSelection(picked);
    if (additive) return;

    if (!(stage.width > 0) || !(stage.height > 0)) return;

    // 무리 이동: 같은 정규화 델타를 선택된 **모든** 요소에 더한다. 상한이 없으므로
    // `clampGroupDelta` 는 쓰지 않는다 — 아무 일도 하지 않는 호출은 읽는 사람에게
    // 상한이 있다고 거짓말한다(spec.md §드래그 기구).
    const bases: DragBase[] = elements
      .filter((el) => picked.has(el.id))
      .map((el) => ({ nodeId: el.id, geometry: el.geometry }));
    if (bases.length === 0) return;

    // 히트는 `elements` 를 훑어 나온 id 이므로 그 요소는 반드시 있다(`canvasHitTest` 는
    // 배열 밖의 id 를 만들지 않는다). 없을 수 없는 경우에 가드를 두면 그 가드는 검증되지
    // 않은 채 남아 읽는 사람에게 "없을 수도 있다" 고 거짓말한다.
    const anchorEl = elements.find((el) => el.id === hit.nodeId)!;

    dragRef.current = {
      mode: 'move',
      pointerId: event.pointerId,
      origin: point,
      frame,
      bases,
      // 선택 외곽선이 두르는 **그 상자**다 — 화면에 보이는 테두리와 격자에 붙는 중심이
      // 다른 상자에서 나오면 "보이는 것과 다른 곳에 붙는다" 가 된다.
      anchor: outlineBox(anchorEl, stage, textWidths),
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
    el: CanvasElement,
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

  const handlePointerMove = (event: React.PointerEvent<HTMLDivElement>): void => {
    const drag = dragRef.current;
    if (drag === null || event.pointerId !== drag.pointerId) return;
    event.preventDefault();
    pendingRef.current = {
      point: stagePoint(event.clientX, event.clientY, drag.frame),
      shift: event.shiftKey,
    };
    // 한 프레임 사이에 이벤트가 열 번 와도 쓰기는 한 번이다(AC-E4).
    if (frameRef.current === null) frameRef.current = requestAnimationFrame(flush);
  };

  /** 잡고 있던 포인터를 놓아준다. 잡은 적이 없으면 브라우저가 예외를 던지므로 확인한다. */
  const releaseCapture = (host: HTMLDivElement, pointerId: number): void => {
    if (host.hasPointerCapture?.(pointerId)) host.releasePointerCapture(pointerId);
  };

  const handlePointerUp = (event: React.PointerEvent<HTMLDivElement>): void => {
    const drag = dragRef.current;
    if (drag === null || event.pointerId !== drag.pointerId) return;
    event.preventDefault();
    releaseCapture(event.currentTarget, event.pointerId);
    finishDrag({
      point: stagePoint(event.clientX, event.clientY, drag.frame),
      shift: event.shiftKey,
    });
  };

  const handlePointerCancel = (event: React.PointerEvent<HTMLDivElement>): void => {
    const drag = dragRef.current;
    if (drag === null || event.pointerId !== drag.pointerId) return;
    releaseCapture(event.currentTarget, event.pointerId);
    // 취소는 좌표를 주지 않는다 — **마지막 유효 위치**를 그대로 확정한다.
    finishDrag(null);
  };

  /**
   * 고른 것들을 정규화 델타만큼 옮긴다 — **드래그의 키보드 등가물**이다(T15 · REQ-04).
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
  const nudgeSelection = (delta: NormalizedDelta): boolean => {
    let next: CanvasElement[] = [...elements];
    let moved = false;
    for (const el of elements) {
      if (!selection.has(el.id)) continue;
      next = patchNodeGeometry(next, el.id, moveGeometry(el.geometry, delta));
      moved = true;
    }
    if (!moved) return false;
    onElementsChange(next);
    return true;
  };

  /**
   * 방향키 미세 이동 · Shift+방향키 한 격자 칸(T15 · AC-08).
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
    const step = ARROW_STEPS[event.key];
    if (step === undefined) return;
    if (dragRef.current !== null) return;
    if (selection.size === 0) return;

    let delta: NormalizedDelta;
    if (event.shiftKey) {
      // 격자 칸은 스테이지 축 길이에 대한 **분수**라 나눌 것이 없다 — 그래서 아직 크기를
      // 재지 못한 순간에도 뜻이 성립한다. 간격은 **지금 그려진 그 칸**이다 — "한 격자 칸"
      // 이 화면에 그려진 칸과 다르면 그 이름이 거짓이 되므로, 격자 표시·드래그 붙임과
      // 같은 파생(`squareGridSteps`)을 본다.
      //
      // 축마다 백분율이 다른 것이 요점이다. 칸은 정사각이므로 **가로 한 칸과 세로 한 칸은
      // 같은 px** 이고, 그것을 각 축의 분수로 옮기면 두 수가 갈린다 — 여기서 한 값을 두
      // 축에 쓰면 세로 한 칸이 그려진 칸을 건너뛰거나 못 미친다.
      delta = { dx: (step.x * gridCell.x) / 100, dy: (step.y * gridCell.y) / 100 };
    } else {
      // 미세 이동은 화면 양이므로 축 길이로 나눈다. 0 으로 나눈 이동량은 요소를 화면 밖으로
      // 날린다 — 드래그가 같은 자리에서 같은 판정을 한다.
      if (!(stage.width > 0) || !(stage.height > 0)) return;
      delta = { dx: (step.x * NUDGE_PX) / stage.width, dy: (step.y * NUDGE_PX) / stage.height };
    }

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
  const placeFromPalette = (kind: CanvasElementKind): void => {
    const { next, created } = appendElement(elements, kind);
    onElementsChange(next);
    setSelection(new Set([created.id]));
  };

  /**
   * 고른 것들을 서로 맞춘다(T13 · REQ-04).
   *
   * 변환 사슬(**투영 px 상자 → 백분율 오프셋 → ÷100 → 정규화 델타**)은 순수 모듈
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
      picked.map((el) => ({ nodeId: el.id, box: outlineBox(el, stage, textWidths) })),
      stage,
      axis,
      mode,
    );
    if (deltas.length === 0) return;

    const geometryById = new Map(picked.map((el) => [el.id, el.geometry] as const));
    let next: CanvasElement[] = [...elements];
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

  // 편집이 꺼져 있으면 DOM 에 아무것도 남기지 않는다(표시 전용).
  if (!enabled) return null;

  /**
   * 핸들을 달 요소. **하나만 골랐을 때만** 있다(파일 머리말의 다중 선택 규칙).
   * `find` 가 비는 것은 실제로 일어난다 — 골라 둔 요소를 목록 편집기에서 지우면 선택에는
   * 그 id 가 남고 배열에는 없다.
   */
  const handleHost =
    selection.size === 1 ? elements.find((el) => selection.has(el.id)) : undefined;

  /** 정렬은 **맞출 상대가 있어야** 뜻이 있다 — 하나만 골라 놓고 맞출 곳은 없다. */
  const canAlign = selection.size >= 2;
  /** 순서 이동은 하나만 골라도 뜻이 있다. */
  const canOrder = selection.size >= 1;

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
      aria-keyshortcuts={NUDGE_KEY_SHORTCUTS}
      // 캔버스 위 전면 층. 터치 스크롤이 드래그를 가로채지 않게 `touch-none` 을 둔다.
      className="absolute inset-0 z-20 touch-none"
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerUp}
      onPointerCancel={handlePointerCancel}
      onKeyDown={handleKeyDown}
    >
      {/* 눈으로 보는 사람에게는 손잡이와 커서가 조작법을 알리지만, 스크린 리더에게는
          알릴 것이 없다 — 그래서 설명을 DOM 에 **항상** 두고 `aria-describedby` 로 잇는다
          (`FieldHelp.tsx` 가 세운 이 저장소의 관용구다). */}
      <p id={hintId} className="sr-only">
        {t('dashboard.canvas.edit.keyboardHint')}
      </p>
      {/* 격자 — **신규 격자 컴포넌트를 만들지 않는다**(REQ-04). `PanelEditGrid` 는 선을
          DOM 요소가 아니라 `repeating-linear-gradient` 로 그리고 `absolute inset-0` +
          `pointer-events-none` 이라 좌표 변환이 아예 필요 없다. 캔버스에서는 이 층이
          칠해진 픽셀 **위**에 얹히지만(도형은 캔버스 안에 있다) 반투명 참조선이라 가리지
          않으며, 그 대가로 001 의 렌더 경로에 한 픽셀도 더하지 않는다.

          다만 **위에 얹힌다는 사실이 색을 정한다.** 다른 패널에서 요소 뒤에 깔릴 때 쓰는
          28% 짜리 선은 칠해진 도형 위에서는 사실상 보이지 않는다 — 그래서 같은 컴포넌트에
          진하기 한 벌(`strong`)만 더 두고 여기서 그것을 고른다.

          간격은 **`snapDelta` 가 쓰는 그 칸**이다. 두 축에 서로 다른 백분율이 가는 것은
          칸을 **정사각**으로 두기 위해서다 — 백분율은 제 축 길이에 대한 값이라 같은 수를
          두 축에 주면 정사각형이 아닌 패널에서 칸이 직사각형이 된다. 그리는 값과 붙는
          값을 각자 정하는 자리는 여전히 없다: 둘 다 `squareGridSteps` 한 계산에서 온다. */}
      <PanelEditGrid
        enabled={snapToGrid}
        step={gridCell.x}
        stepY={gridCell.y}
        strength="strong"
      />
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
          <CanvasEditDockBody
            onPlace={placeFromPalette}
            snapToGrid={snapToGrid}
            onSnapToGridChange={setSnapToGrid}
            gridStep={gridStep}
            onGridStepChange={setGridStep}
            canAlign={canAlign}
            canOrder={canOrder}
            onAlign={applyAlign}
            onOrder={applyZOrder}
          />,
          dockHost,
        )}
      {elements.map((el) => {
        if (!selection.has(el.id)) return null;
        const box = outlineBox(el, stage, textWidths);
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
        handlePositions(handleHost, stage, {
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
    </div>
  );
}
