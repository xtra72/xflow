// 패널이 잰 **바깥 상자**를 편집기로 흘려보내는 채널 (SPEC-CANVAS-002 0.10.0).
//
// ## 왜 채널이 필요한가
//
// 축척이 하나가 된 뒤로 캔버스 비율과 패널 비율이 다르면 한 축에 여백이 남는다. 그 여백을
// 없애는 조작("패널 비율에 맞춤")은 **캔버스 크기 두 정수를 고치는 일**이라 그 칸이 있는
// 요소 목록 편집기에 속한다. 그런데 패널 비율을 아는 것은 `ResizeObserver` 를 든 표면뿐이고,
// 편집기는 설정 다이얼로그의 **다른 자리**에 마운트된다. 그래서 둘을 잇는 통로가 필요하다.
//
// 새로 지어낸 형태가 아니다 — `canvasEditContext` 의 라이브 시리즈 채널과 **같은 형상**이다:
// 살아 있는 패널이 제가 실제로 잰 값을 내놓고, 편집기가 그것을 그대로 쓴다. 편집기가 패널
// 크기를 **추측**하면(예: 저장된 캔버스 비율로 역산) 그 추측이 곧 두 번째 출처가 되고,
// 두 값이 갈라지는 날 "맞췄는데 여백이 남는다" 가 된다(위험 R1 과 같은 부류다).
//
// ## 무엇을 나르는가 — **잰 바깥 상자**다
//
// 맞춘 영역(`projection.stage`)이 아니라 표면이 `ResizeObserver` 로 잰 **바깥** 상자를
// 나른다. 맞춘 영역은 이미 캔버스 비율이므로 그것에 맞추면 언제나 무동작이다 — 제 꼬리를
// 무는 값이다. 여백을 없애려면 여백을 만든 그 상자를 봐야 한다.
//
// **이 파일은 컴포넌트를 하나도 내보내지 않는다.** 컨텍스트 객체와 훅만 있으므로
// `react-refresh/only-export-components` 가 울지 않는다(`canvasStageGrid.ts` 와 같은 규율).
//
// @spec SPEC-CANVAS-002 REQ-04

import { createContext, useCallback, useContext, useMemo, useState } from 'react';

import type { StageSize } from './canvasGeometry';

/** 아직 아무도 재지 않은 상태. 참조가 고정이라 효과 의존성에 그대로 들어간다. */
const NOT_MEASURED: StageSize | null = null;

/** 아무도 듣고 있지 않을 때의 발행(대시보드에 놓인 패널). 참조가 고정이다. */
const noopPublish = (_next: StageSize): void => {};

/** 채널이 나르는 것 전부. */
export interface CanvasStageAspectValue {
  /** 패널이 마지막으로 잰 바깥 상자(CSS px). 아무도 재지 않았으면 `null`. */
  outer: StageSize | null;
  /** 패널이 제가 잰 상자를 내놓는다. **값이 같으면 아무 일도 일어나지 않는다.** */
  publish: (next: StageSize) => void;
}

/**
 * 공유 컨텍스트. 기본값이 `null` 인 것이 **provider 없이도 동작한다**는 계약이다 —
 * 대시보드에 놓인 패널 곁에는 편집기가 없으므로 발행은 무동작이고, 편집기 단독 렌더
 * (단위 시험)에서는 상자가 `null` 이라 맞춤 단추가 잠긴다.
 */
export const CanvasStageAspectContext = createContext<CanvasStageAspectValue | null>(null);

/**
 * 채널 한 벌을 만든다. 감싸는 쪽(설정 다이얼로그)이 부른다.
 *
 * 값이 같으면 `setState` 가 **이전 값 그대로**를 돌려주므로 React 가 렌더를 건너뛴다 —
 * 리사이즈가 1px 씩 흔들려도 편집기가 다시 그려지지 않는 근거가 이 한 줄이다.
 */
export function useCanvasStageAspectState(): CanvasStageAspectValue {
  const [outer, setOuter] = useState<StageSize | null>(NOT_MEASURED);
  const publish = useCallback((next: StageSize) => {
    setOuter((prev) =>
      prev !== null && prev.width === next.width && prev.height === next.height ? prev : next,
    );
  }, []);
  return useMemo(() => ({ outer, publish }), [outer, publish]);
}

/** 지금 잰 바깥 상자. provider 가 없으면 `null` 이다(맞춤 단추가 잠긴다). */
export function useCanvasStageAspect(): StageSize | null {
  return useContext(CanvasStageAspectContext)?.outer ?? NOT_MEASURED;
}

/** 잰 상자를 내놓는 통로. provider 가 없으면 무동작이다(대시보드에 놓인 패널). */
export function useCanvasStageAspectPublisher(): (next: StageSize) => void {
  return useContext(CanvasStageAspectContext)?.publish ?? noopPublish;
}
