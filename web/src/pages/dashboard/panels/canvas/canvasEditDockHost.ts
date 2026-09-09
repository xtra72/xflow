// 도크 자리(포털 호스트) 컨텍스트 — 편집 도구가 **패널 밖 어디에 붙는가**만 알린다
// (SPEC-CANVAS-002 T9 후속).
//
// 왜 컨텍스트인가: 도구를 만들어 내는 쪽(`CanvasEditOverlay`)은 스테이지 크기·선택·격자
// 상태를 이미 손에 들고 있고, 도구를 **놓을 자리**는 패널 바깥(설정 다이얼로그의 미리보기
// 영역)에 있다. 상태를 위로 올리면 스테이지·요소가 바뀔 때마다 바깥이 다시 그려지므로,
// 반대로 **자리만 아래로 내린다** — 오버레이가 그 자리에 포털로 그린다.
//
// 값이 `null` 이면 도크가 없다는 뜻이고, 그때 오버레이는 도구를 **아무 데도 그리지 않는다**.
// 그것이 "패널 설정에서만 쓴다" 는 결정이 코드에 남는 자리다 — 대시보드에 놓인 패널에는
// 이 provider 가 없으므로 도구가 스테이지 위로 되돌아올 길이 없다.
//
// **이 파일은 컴포넌트를 하나도 내보내지 않는다.** 컨텍스트 객체와 훅만 있으므로
// `react-refresh/only-export-components` 가 울지 않는다 — `canvasEditContext.tsx` 가
// provider 컴포넌트를 두지 않은 것과 같은 규율이다(React 19 는 `<Context value>` 를
// 그대로 쓴다).
//
// @spec SPEC-CANVAS-002 REQ-01

import { createContext, useContext } from 'react';

/**
 * 도크의 포털 호스트 DOM 요소. 없으면 `null`.
 *
 * 요소 자체를 값으로 두는 이유: 포털의 목적지는 DOM 노드이고, ref 를 통째로 넘기면
 * 그 노드가 붙는 순간을 구독하는 쪽이 알 수 없다(ref 변경은 렌더를 부르지 않는다).
 * 자리를 내는 쪽이 콜백 ref → state 로 한 번 다시 그리면, 구독하는 쪽은 평범한 값 변경
 * 으로 그 순간을 본다.
 */
export const CanvasEditDockHostContext = createContext<HTMLElement | null>(null);

/** 도크가 있으면 그 호스트 요소, 없으면 `null`. */
export function useCanvasEditDockHost(): HTMLElement | null {
  return useContext(CanvasEditDockHostContext);
}
