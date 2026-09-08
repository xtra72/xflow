// 공용 편집 격자 테스트 (SPEC-CHART-004 §2.7 · SPEC-CANVAS-002 REQ-04).
//
// 이 컴포넌트는 여섯 패널(통계·바·파이·게이지·선·캔버스)이 함께 쓴다. 그래서 여기서
// 가장 중요한 것은 **기본값이 종전 그림 그대로인가** 하나다 — 캔버스가 자기 진하기를
// 얻겠다고 기본 색을 건드리면 다섯 패널의 격자가 조용히 함께 바뀐다. 그 변화는 어느
// 패널 테스트에도 걸리지 않는다(그 테스트들은 격자가 **있는지**만 본다). 그래서 색
// 문자열을 여기에 **글자 그대로** 못 박는다: 값을 바꾸려면 이 파일을 함께 고쳐야 하고,
// 그 순간 "다섯 패널이 함께 바뀐다" 를 읽게 된다.
//
// @spec SPEC-CHART-004 §2.7 [U7] · SPEC-CANVAS-002 REQ-04

import { describe, expect, it, afterEach } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';

import { PanelEditGrid } from './PanelEditGrid';
import { GRID_STEP_PERCENT } from './panels/charts/panelEditAlign';

afterEach(cleanup);

/** 이 변경 **이전**의 값 그대로다. 한 글자라도 다르면 다섯 패널의 격자가 바뀐 것이다. */
const LEGACY_LINE = 'rgba(148, 163, 184, 0.28)';
const LEGACY_CENTER = 'rgba(59, 130, 246, 0.55)';

function grid(): HTMLElement {
  return screen.getByTestId('panel-edit-grid');
}

function center(): HTMLElement {
  return screen.getByTestId('panel-edit-center');
}

describe('기본값은 종전 그림 그대로다 (다섯 패널이 함께 쓴다)', () => {
  it('선 색·중심 색·간격이 종전 값이다', () => {
    render(<PanelEditGrid enabled />);

    const image = grid().style.backgroundImage;
    expect(image).toBe(
      [
        `repeating-linear-gradient(to right, ${LEGACY_LINE} 0 1px, transparent 1px ${GRID_STEP_PERCENT}%)`,
        `repeating-linear-gradient(to bottom, ${LEGACY_LINE} 0 1px, transparent 1px ${GRID_STEP_PERCENT}%)`,
      ].join(', '),
    );
    expect(center().style.backgroundColor).toBe(LEGACY_CENTER);
  });

  it('`strength` 를 명시하지 않은 것과 `subtle` 은 같은 그림이다', () => {
    const { container: implicit } = render(<PanelEditGrid enabled />);
    const implicitHtml = implicit.innerHTML;
    cleanup();
    const { container: explicit } = render(<PanelEditGrid enabled strength="subtle" />);

    expect(explicit.innerHTML).toBe(implicitHtml);
  });

  it('꺼져 있으면 DOM 에 아무것도 남기지 않는다', () => {
    const { container } = render(<PanelEditGrid enabled={false} strength="strong" step={5} />);

    expect(container.innerHTML).toBe('');
  });
});

describe('진하기 한 벌을 더 둔다 — 컴포넌트를 쪼개지 않는다 (REQ-04)', () => {
  it('`strong` 은 기본보다 **진한** 선을 쓴다 (칠해진 픽셀 위에서도 남아야 한다)', () => {
    render(<PanelEditGrid enabled strength="strong" />);

    const image = grid().style.backgroundImage;
    // 종전 값이 그대로 새어 나오면 캔버스에서 여전히 보이지 않는다 — 그것이 이 갈래의 이유다.
    expect(image).not.toContain(LEGACY_LINE);
    // 색상은 같고 불투명도만 오른다. 색을 바꾸면 한 화면에 격자가 두 색으로 존재하게 된다.
    expect(image).toContain('rgba(148, 163, 184, 0.6)');
    expect(center().style.backgroundColor).toBe('rgba(59, 130, 246, 0.9)');
  });
});

describe('간격은 그린 선 자리에 그대로 나타난다', () => {
  it('넘긴 간격이 두 축 모두에 쓰인다', () => {
    render(<PanelEditGrid enabled step={20} />);

    const image = grid().style.backgroundImage;
    expect(image).toContain('transparent 1px 20%');
    // 가로·세로 두 그라디언트가 같은 간격을 쓴다 — 한 축만 바뀌면 격자가 직사각형이 된다.
    expect(image.match(/transparent 1px 20%/g)).toHaveLength(2);
    expect(image).not.toContain('10%');
  });
});
