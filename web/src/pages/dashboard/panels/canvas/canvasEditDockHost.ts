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
// ## 자리가 **둘**인 이유 (SPEC-CANVAS-011 후속 — 도구 띠)
//
// 도구 한 벌이 두 자리로 갈라졌다. 캔버스에 **재료를 놓는** 절 셋(도형 · 가져오기 · 서랍)은
// 세로 목록을 원하므로 왼쪽 도크에 남고, 캔버스와 **고른 것에 작용하는** 절 일곱(작업 영역 ·
// 격자 · 정렬 · 순서 · 그룹 · 앵커 · 연결선)은 미리보기 제목 바로 아래의 가로 띠로 간다.
//
// 그래서 호스트가 둘이다 — **하나로는 안 된다.** 포털의 목적지는 DOM 노드이고, 한 노드는
// 두 자리에 동시에 있을 수 없다. 자리가 둘이면 노드도 둘이고, 노드가 둘이면 컨텍스트도 둘이다.
//
// **둘은 언제나 함께 태어나고 함께 사라진다** — 자리를 내는 것은 `CanvasEditDockRegion`
// 한 컴포넌트뿐이고, 그것이 두 노드를 같은 커밋에서 붙인다. 그래서 오버레이는 "도크가
// 있는가" 를 **여전히 `dockHost === null` 한 줄로** 묻는다(떠 있는 줄이 서는 조건이 그
// 한 줄이며 006 · 011 의 시험이 그것을 이름으로 단언한다). 띠만 있고 도크가 없는 상태는
// 만들 수 있는 길이 없다.
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

/**
 * 도구 띠의 포털 호스트 DOM 요소. 없으면 `null`.
 *
 * 도크의 그것과 **한 글자도 다르지 않은 규율**이다 — 값은 노드이고, 자리를 내는 쪽이
 * 콜백 ref → state 로 한 번 다시 그려 붙는 순간을 알린다. 다른 것은 **자리** 하나뿐이다:
 * 이 노드는 미리보기 제목 바로 아래, 도크와 미리보기를 함께 덮는 폭으로 선다.
 */
export const CanvasEditToolbarHostContext = createContext<HTMLElement | null>(null);

/** 도구 띠가 있으면 그 호스트 요소, 없으면 `null`. */
export function useCanvasEditToolbarHost(): HTMLElement | null {
  return useContext(CanvasEditToolbarHostContext);
}
