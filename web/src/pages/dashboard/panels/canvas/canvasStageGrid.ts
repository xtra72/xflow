// 스테이지 격자 컨텍스트 — 표면이 **그리는 영역을 맞춘 칸**을 오버레이에 알린다
// (SPEC-CANVAS-002 0.9.0 · 사용 시험 "격자가 일정하지 않음" 세 번째 회차).
//
// ## 왜 컨텍스트인가
//
// 그리는 영역은 격자 칸의 정수배여야 한다(`canvasGeometry.stageLattice`). 그래야 선이
// 정수 px 에서 시작해 두 장치 픽셀에 걸치지 않는다. 그런데 그 칸을 정하는 값(격자 간격)은
// **사용자가 편집 도크에서 고르고**, 상자를 짓는 것은 **표면**이다. 둘 사이에는
// `CanvasPanel` 이 있는데 그 파일은 오버레이 슬롯이 주는 값 가운데 둘(투영 · 글자 폭)만
// 골라 넘긴다 — 슬롯에 값을 하나 더 얹어도 오버레이까지 닿지 않는다. 그래서 표면이
// **제 슬롯 둘레에 직접** 이 컨텍스트를 편다. 도크 자리(`canvasEditDockHost`)를 아래로
// 내린 것과 같은 방향의 해법이다: 상태를 위로 올리는 대신 필요한 것만 아래로 내린다.
//
// ## 왜 상태의 주인이 표면인가
//
// 간격은 화면에서 "격자 간격" 으로 읽히지만 코드에서 하는 일은 **그리는 영역의 양자를
// 고르는 것**이고, 그 영역은 표면의 것이다. 오버레이가 들고 있으면 상자를 짓는 쪽이 남의
// 상태를 되물어야 하고, 그 되묻는 자리가 곧 두 번째 출처가 된다(위험 R1).
//
// 값이 `null` 이면 표면 밖이라는 뜻이다(오버레이만 세운 단위 시험이 그렇다). 그때 오버레이는
// 제 지역 상태로 떨어지고 칸도 같은 순수 함수로 직접 구한다 — `useCanvasEditSelection` 이
// provider 없이도 도는 것과 같은 규율이다.
//
// **이 파일은 컴포넌트를 하나도 내보내지 않는다.** 컨텍스트 객체와 훅만 있으므로
// `react-refresh/only-export-components` 가 울지 않는다(`canvasEditDockHost.ts` 와 같은 규율).
//
// @spec SPEC-CANVAS-002 REQ-04

import { createContext, useContext } from 'react';

import type { StageCell } from './canvasGeometry';

/** 표면이 그리는 영역을 맞춘 격자 한 벌. */
export interface CanvasStageGrid {
  /** 격자 간격(정수 캔버스 단위). 그리기 · 붙임 · Shift+방향키가 함께 쓰는 그 한 값이다. */
  step: number;
  /** 간격을 바꾼다. 표면은 그리는 영역을 새 칸에 다시 맞춘다. */
  setStep: (step: number) => void;
  /** 한 칸의 CSS px — 표면이 영역을 맞출 때 쓴 **그 값 그대로**다(다시 나누지 않는다). */
  cell: StageCell;
}

export const CanvasStageGridContext = createContext<CanvasStageGrid | null>(null);

/**
 * 표면이 편 격자 한 벌. **표면 밖에서는 `null` 이다.**
 *
 * `null` 을 숨기지 않는 것에 뜻이 있다 — 표면이 없으면 맞출 영역도 없으므로, 받는 쪽이
 * 그 사실을 알고 제 값으로 폴백해야 한다.
 */
export function useCanvasStageGrid(): CanvasStageGrid | null {
  return useContext(CanvasStageGridContext);
}
