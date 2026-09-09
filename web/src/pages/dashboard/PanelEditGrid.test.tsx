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
  it('간격 하나만 넘기면 두 축 모두에 쓰인다 (종전 그대로다)', () => {
    render(<PanelEditGrid enabled step={20} />);

    const image = grid().style.backgroundImage;
    expect(image).toContain('transparent 1px 20%');
    // 세로를 따로 주지 않으면 가로 값을 그대로 쓴다 — 다섯 패널이 지나는 그 길이다.
    expect(image.match(/transparent 1px 20%/g)).toHaveLength(2);
    expect(image).not.toContain('10%');
  });
});

// --- 세로 간격을 따로 받는다 (사용 시험: "격자가 일정하지 않음") -------------
//
// 백분율은 제 축 길이에 대한 값이라, 두 축에 같은 수를 주면 정사각형이 아닌 상자에서 칸이
// 직사각형이 된다. 그것을 바꾸려는 부르는 쪽(캔버스)만 두 값을 넘기고, 나머지 다섯 패널은
// 종전 그대로여야 한다 — 그 "종전 그대로" 는 위 §기본값 절이 글자 그대로 못 박는다.

describe('세로 간격은 따로 받되 기본은 가로와 같다 (`stepY`)', () => {
  it('세로 간격을 주면 아래 방향 그라디언트만 그 값을 쓴다', () => {
    render(<PanelEditGrid enabled step={10} stepY={20} />);

    const image = grid().style.backgroundImage;
    expect(image).toContain('to right, rgba(148, 163, 184, 0.28) 0 1px, transparent 1px 10%');
    expect(image).toContain('to bottom, rgba(148, 163, 184, 0.28) 0 1px, transparent 1px 20%');
  });

  it('세로를 가로와 같은 값으로 주는 것과 생략하는 것은 같은 그림이다', () => {
    const { container: implicit } = render(<PanelEditGrid enabled step={20} />);
    const implicitHtml = implicit.innerHTML;
    cleanup();
    const { container: explicit } = render(<PanelEditGrid enabled step={20} stepY={20} />);

    expect(explicit.innerHTML).toBe(implicitHtml);
  });

  it('진하기와 함께 넘겨도 서로를 밀어내지 않는다', () => {
    render(<PanelEditGrid enabled step={5} stepY={10} strength="strong" />);

    const image = grid().style.backgroundImage;
    expect(image).toContain('rgba(148, 163, 184, 0.6)');
    expect(image).toContain('transparent 1px 5%)');
    expect(image).toContain('transparent 1px 10%)');
  });
});

// --- 단위를 따로 받는다 (사용 시험: "격자가 일정하지 않음" 세 번째 회차) -----
//
// 백분율은 브라우저가 상자 폭에 곱하는 순간 소수 px 가 되고(1749px 에 5% = 87.45px),
// 소수 자리에서 시작하는 1px 선은 두 장치 픽셀에 나뉘어 칠해져 선마다 굵기가 달라 보인다.
// 캔버스는 그리는 영역을 칸의 정수배로 맞춰 두므로 칸을 px 로 곧장 말할 수 있다.
//
// 여기서 지킬 것은 위 §기본값 절과 같은 하나다: **넘기지 않으면 종전 그대로**여야 한다.

describe('간격의 단위를 고를 수 있다 — 기본은 종전대로 % 다 (`unit`)', () => {
  it('`px` 를 주면 두 축의 주기가 px 로 적힌다', () => {
    render(<PanelEditGrid enabled step={87} stepY={49} unit="px" strength="strong" />);

    const image = grid().style.backgroundImage;
    expect(image).toContain('to right, rgba(148, 163, 184, 0.6) 0 1px, transparent 1px 87px');
    expect(image).toContain('to bottom, rgba(148, 163, 184, 0.6) 0 1px, transparent 1px 49px');
    // 백분율이 한 글자도 남으면 브라우저가 다시 곱해 소수를 만든다.
    expect(image).not.toContain('%');
  });

  it('단위를 생략하는 것과 `%` 를 명시하는 것은 같은 그림이다 (다섯 패널의 길)', () => {
    const { container: implicit } = render(<PanelEditGrid enabled step={10} stepY={20} />);
    const implicitHtml = implicit.innerHTML;
    cleanup();
    const { container: explicit } = render(
      <PanelEditGrid enabled step={10} stepY={20} unit="%" />,
    );

    expect(explicit.innerHTML).toBe(implicitHtml);
  });
});
