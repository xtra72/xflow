// 드롭 존 노드의 자리와 놓임 판정 (SPEC-CANVAS-008 M9 · REQ-03 · 불변식 J7).
//
// **왜 드롭 존이 제 손으로 놓임을 받지 못하는가** — 이 SPEC 에서 실측으로 확인한 사실이다.
// 오버레이는 누름을 받으면 **루트에 포인터를 잡는다**(`CanvasEditOverlay.handlePointerDown`
// 의 `host.setPointerCapture?.(event.pointerId)`, 그리고 `host` 는 `event.currentTarget`
// = 오버레이 루트다). 잡힌 뒤에는 손이 스테이지 밖 도크 위로 가도 `pointermove`/`pointerup`
// 이 **여전히 오버레이로 온다** — 도크 안의 노드는 그 사건을 받지 못한다.
//
// 그래서 드롭 존에 `onDrop` · `onPointerEnter` · `onPointerUp` 을 달아도 **한 번도 불리지
// 않는다.** 놓임 판정은 오버레이의 `pointerup` 안에서 일어나야 하고, 드롭 존이 내놓는 것은
// **제 노드 하나**뿐이다.
//
// **판정은 클라이언트 좌표 containment 다**(불변식 J7). 스테이지 산술이 아니다 —
// `projection` 을 보지 않고, 스테이지 축 길이로 나누지 않는다. 그래서 좌표 공간을 넘는
// 자리는 여전히 `stagePoint` 한 함수뿐이다. 아래 `pointInRect` 의 본문이 그 사실을
// **형상으로** 말한다: 인자에 상자와 두 수만 있고 투영이 들어올 문이 없다.
//
// **컨텍스트가 나르는 것은 노드가 아니라 콜백 ref 다.** 방향이 `canvasEditDockHost` 와
// 반대이기 때문이다 — 도크 자리는 오버레이의 **조상**이 내주지만, 드롭 존은 오버레이가
// 포털로 그리는 **자손** 안에 있다. 자손이 조상에게 값을 올릴 길은 컨텍스트에 없으므로,
// 내려보내는 것을 콜백 ref 로 두고 노드는 그 호출로 올라온다. provider 가 없으면 `null`
// 이고, 그때 드롭 존은 아무 데도 등록되지 않는다(= 도크가 없는 표면).
//
// **이 파일은 컴포넌트를 하나도 내보내지 않는다** — `canvasEditDockHost.ts` 와 같은 규율
// 이라 `react-refresh/only-export-components` 가 울지 않는다.
//
// @spec SPEC-CANVAS-008 REQ-03 · REQ-07 · AC-05 · AC-E7

import { createContext, useContext } from 'react';

/** 드롭 존이 제 노드를 알리는 콜백. 언마운트에서는 `null` 로 불린다. */
export type ScratchpadDropRef = (node: HTMLElement | null) => void;

/**
 * 항목을 캔버스 위에 놓은 **클라이언트 자리**.
 *
 * 스테이지 좌표가 아닌 것에 뜻이 있다: 환산은 오버레이의 `stagePoint` 한 함수가 하며
 * (불변식 J7), 서랍은 그 함수를 알지 못한다. 서랍이 스테이지 좌표를 만들려면 오버레이의
 * 상자와 축척을 알아야 하고, 그 순간 좌표 공간을 넘는 자리가 둘이 된다.
 */
export interface ScratchpadDropPoint {
  clientX: number;
  clientY: number;
}

/**
 * 드롭 존 등록 채널. `null` 이면 등록할 곳이 없다는 뜻이고, 그때 스크래치패드는
 * 놓임 판정에 참여하지 않는다.
 */
export const CanvasScratchpadDropContext = createContext<ScratchpadDropRef | null>(null);

/** 드롭 존이 제 노드를 알릴 콜백. 없으면 `null`. */
export function useScratchpadDropRef(): ScratchpadDropRef | null {
  return useContext(CanvasScratchpadDropContext);
}

/**
 * 클라이언트 좌표가 상자 안인가 — **놓임 판정의 전부**다.
 *
 * 상자가 `null` 이면 거짓이다. 도크가 없는 표면(대시보드)에서 호출부가 분기를 하나 더
 * 두지 않게 하려는 것이며, 그래서 이 판정은 호출부에 **한 자리**만 갖는다.
 *
 * 경계는 **포함**이다(`>=`/`<=`). 상자의 테두리 위에서 손을 뗐을 때 "안도 밖도 아니다"
 * 가 되면 그 한 픽셀에서 저장이 조용히 실패한다.
 */
export function pointInRect(rect: DOMRect | null, clientX: number, clientY: number): boolean {
  if (rect === null) return false;
  return (
    clientX >= rect.left && clientX <= rect.right && clientY >= rect.top && clientY <= rect.bottom
  );
}
