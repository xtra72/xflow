// 편집 중 패널 안에 까는 **배치 그리드와 중심 표식**. 통계·바·파이·게이지·선·캔버스가 함께 쓴다.
//
// 포인터를 받지 않는다 — 참조선일 뿐이라, 이것이 이벤트를 먹으면 그리드 위를 지나는
// 드래그가 끊긴다.
//
// 선은 DOM 요소가 아니라 `repeating-linear-gradient` 로 그린다. 10% 격자면 한 축에
// 선 9개씩 18개인데, 요소로 그리면 드래그 중 매 프레임 그만큼을 다시 그리게 된다.
//
// ## 진하기가 한 벌이 아닌 이유 (`strength`)
//
// 이 컴포넌트가 어디에 얹히는지는 부르는 패널마다 다르다. 통계·바·파이·게이지·선은
// 요소가 **DOM** 이라 격자가 그 **뒤**(`z-0`)에 깔리고, 그래서 옅어야 글자를 방해하지
// 않는다. 캔버스는 요소가 **칠해진 픽셀**이라 격자가 필연적으로 그 **위**에 얹히고,
// 28% 짜리 회색 선은 blue-500 도형 위에서 사실상 보이지 않는다.
//
// 그렇다고 격자 컴포넌트를 둘로 쪼개지 않는다(SPEC-CANVAS-002 REQ-04) — 격자 어휘가
// 둘이 되는 비용이 색 한 벌을 고르는 비용보다 크다. 그래서 **진하기만 한 벌 더** 둔다.
// 기본값(`subtle`)은 종전 값 그대로이므로 부르던 자리들의 그림은 한 픽셀도 바뀌지 않는다.
//
// ## 세로 간격을 따로 받는 이유 (`stepY`)
//
// 그라디언트의 백분율은 **제 축 길이에 대한** 값이다 — `to right` 의 10% 는 폭의 10%,
// `to bottom` 의 10% 는 높이의 10% 다. 그래서 두 축에 같은 백분율을 주면 정사각형이
// 아닌 상자에서는 칸이 직사각형이 된다(800×400 이면 80×40 px).
//
// 그것은 **원래 쓰던 다섯 패널에서는 옳다.** 그쪽 요소의 자리는 축마다의 백분율 오프셋
// 이라 격자도 같은 축을 그려야 오프셋 눈금과 격자선이 만난다. 그러나 캔버스는 그림을
// 그리는 자리라 사용자가 **정사각 칸**을 기대한다.
//
// 그래서 여기서도 컴포넌트를 쪼개지 않고 **세로 간격 하나를 더 받는다**(`strength` 와
// 같은 방식이다). 생략하면 `step` 과 같은 값이므로 다섯 패널의 그림은 한 글자도 바뀌지
// 않고, 캔버스만 "한 물리 칸에서 파생한 두 백분율"(`canvasEditArrange.squareGridSteps`)
// 을 넘겨 정사각 칸을 얻는다. 두 축의 값을 **부르는 쪽이 한 계산에서** 만들어 오므로,
// 그리는 간격과 붙는 간격이 갈라질 자리는 여기에도 생기지 않는다.
//
// @spec SPEC-CHART-004 §2.7 [U7] · SPEC-CANVAS-002 REQ-04

import { GRID_STEP_PERCENT } from './panels/charts/panelEditAlign';

/**
 * 격자의 진하기.
 *
 * - `subtle` — **요소 뒤에** 깔리는 패널용(기본). 배경과 요소 사이에서 읽히되 글자를
 *   방해하지 않을 만큼만 진하다.
 * - `strong` — **칠해진 픽셀 위에** 얹히는 캔버스용. 도형 위에서도 선이 남아야 무엇에
 *   붙는지 보인다.
 */
export type PanelEditGridStrength = 'subtle' | 'strong';

/** 한 벌의 색. 라이트/다크 어느 쪽에서도 보이도록 반투명 회색·파랑을 쓴다. */
interface GridPaint {
  /** 격자 선 색. */
  line: string;
  /** 중심 표식 색 — 격자선보다 진해야 "가운데" 라는 사실이 먼저 읽힌다. */
  center: string;
}

const PAINT: Record<PanelEditGridStrength, GridPaint> = {
  // 종전 값 그대로다. 이 두 문자열이 바뀌면 다섯 패널의 격자가 함께 바뀐다.
  subtle: { line: 'rgba(148, 163, 184, 0.28)', center: 'rgba(59, 130, 246, 0.55)' },
  // 색상은 같고 불투명도만 올린다 — 색을 바꾸면 같은 화면에 격자가 두 색으로 존재하게 된다.
  strong: { line: 'rgba(148, 163, 184, 0.6)', center: 'rgba(59, 130, 246, 0.9)' },
};

export function PanelEditGrid({
  enabled,
  step = GRID_STEP_PERCENT,
  stepY = step,
  strength = 'subtle',
}: {
  enabled: boolean;
  /** 가로 격자 간격(%) — 상자 **폭**에 대한 값. 기본은 `panelEditAlign` 이 소유하는 10%. */
  step?: number;
  /**
   * 세로 격자 간격(%) — 상자 **높이**에 대한 값. 생략하면 `step` 과 같다.
   *
   * 같은 값을 두 축에 주면 정사각형이 아닌 상자에서 칸이 직사각형이 된다. 그것을 바꾸려는
   * 부르는 쪽(캔버스)만 두 값을 함께 넘긴다(위 머리말 §세로 간격).
   */
  stepY?: number;
  /** 진하기. 기본은 요소 뒤에 깔리는 패널용 `subtle` 이다. */
  strength?: PanelEditGridStrength;
}): React.ReactElement | null {
  if (!enabled) return null;
  const paint = PAINT[strength];
  return (
    <div
      data-testid="panel-edit-grid"
      aria-hidden="true"
      className="pointer-events-none absolute inset-0 z-0"
      style={{
        backgroundImage: [
          `repeating-linear-gradient(to right, ${paint.line} 0 1px, transparent 1px ${step}%)`,
          `repeating-linear-gradient(to bottom, ${paint.line} 0 1px, transparent 1px ${stepY}%)`,
        ].join(', '),
      }}
    >
      {/* 중심 `+` — 가로/세로 짧은 선 두 개. 오프셋 0 이 곧 이 교점이므로,
          "가운데로 되돌렸다" 를 눈으로 확인하는 기준이 된다. */}
      <span
        data-testid="panel-edit-center"
        className="absolute left-1/2 top-1/2 h-px w-4 -translate-x-1/2 -translate-y-1/2"
        style={{ backgroundColor: paint.center }}
      />
      <span
        className="absolute left-1/2 top-1/2 h-4 w-px -translate-x-1/2 -translate-y-1/2"
        style={{ backgroundColor: paint.center }}
      />
    </div>
  );
}
