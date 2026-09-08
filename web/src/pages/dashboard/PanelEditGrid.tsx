// 편집 중 패널 안에 까는 **배치 그리드와 중심 표식**. 통계·바·파이가 함께 쓴다.
//
// 요소 뒤에 깔리고 포인터를 받지 않는다 — 참조선일 뿐이라, 이것이 이벤트를 먹으면
// 그리드 위를 지나는 드래그가 끊긴다.
//
// 선은 DOM 요소가 아니라 `repeating-linear-gradient` 로 그린다. 10% 격자면 한 축에
// 선 9개씩 18개인데, 요소로 그리면 드래그 중 매 프레임 그만큼을 다시 그리게 된다.
//
// @spec SPEC-CHART-004 §2.7 [U7]

import { GRID_STEP_PERCENT } from './panels/charts/panelEditAlign';

/**
 * 격자 선 색. 배경과 요소 사이에서 읽히되 글자를 방해하지 않을 만큼만 진하다.
 * 라이트/다크 어느 쪽에서도 보이도록 반투명 회색을 쓴다.
 */
const LINE = 'rgba(148, 163, 184, 0.28)';
/** 중심 표식 색 — 격자선보다 진해야 "가운데" 라는 사실이 먼저 읽힌다. */
const CENTER = 'rgba(59, 130, 246, 0.55)';

export function PanelEditGrid({
  enabled,
  step = GRID_STEP_PERCENT,
}: {
  enabled: boolean;
  /** 격자 간격(%). 기본은 `statAlign` 이 소유하는 10%. */
  step?: number;
}): React.ReactElement | null {
  if (!enabled) return null;
  return (
    <div
      data-testid="panel-edit-grid"
      aria-hidden="true"
      className="pointer-events-none absolute inset-0 z-0"
      style={{
        backgroundImage: [
          `repeating-linear-gradient(to right, ${LINE} 0 1px, transparent 1px ${step}%)`,
          `repeating-linear-gradient(to bottom, ${LINE} 0 1px, transparent 1px ${step}%)`,
        ].join(', '),
      }}
    >
      {/* 중심 `+` — 가로/세로 짧은 선 두 개. 오프셋 0 이 곧 이 교점이므로,
          "가운데로 되돌렸다" 를 눈으로 확인하는 기준이 된다. */}
      <span
        data-testid="panel-edit-center"
        className="absolute left-1/2 top-1/2 h-px w-4 -translate-x-1/2 -translate-y-1/2"
        style={{ backgroundColor: CENTER }}
      />
      <span
        className="absolute left-1/2 top-1/2 h-4 w-px -translate-x-1/2 -translate-y-1/2"
        style={{ backgroundColor: CENTER }}
      />
    </div>
  );
}
