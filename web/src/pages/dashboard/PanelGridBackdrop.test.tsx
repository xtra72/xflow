// 그리드 배경 검증.
//
// 이 레이어의 일은 둘이다: (1) 패널 뒤에 깔릴 것, (2) 칸 경계가 패널 상자에 맞을 것.
// 위에 깔리면 패널의 실제 모양이 가려지고, 경계가 어긋나면 패널이 몇 칸을 차지하는지
// 주변 칸으로 읽을 수 없다.

import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';

import { PanelGridBackdrop } from './PanelGridBackdrop';

const CELL = 100;
const GAP = 16;
const base = { cellW: CELL, cellH: CELL, gapX: GAP, gapY: GAP };

/** 첫 칸의 좌측 좌표(px). */
function gridLeft(): number {
  const inner = screen.getByTestId('panel-grid-backdrop').firstElementChild as HTMLElement;
  return Number.parseFloat(inner.style.left);
}

describe('PanelGridBackdrop', () => {
  it('패널 뒤에 깔린다 — 위에 그리면 패널 모양이 가려진다', () => {
    render(<PanelGridBackdrop areaW={800} areaH={600} boxW={448} boxH={332} {...base} />);

    const backdrop = screen.getByTestId('panel-grid-backdrop');
    expect(backdrop.className).toContain('z-0');
    expect(backdrop.className).toContain('pointer-events-none');
  });

  it('칸 경계가 패널 상자의 왼쪽 모서리에 맞는다', () => {
    // 영역 800, 상자 448 → 상자 왼쪽 = 176. 칸 간격 116 → 176 - 2·116 = -56.
    render(<PanelGridBackdrop areaW={800} areaH={600} boxW={448} boxH={332} {...base} />);

    const left = gridLeft();
    const boxLeft = (800 - 448) / 2;
    // 상자 왼쪽과의 거리가 칸 간격의 정수배여야 격자가 맞는다.
    expect(((boxLeft - left) / (CELL + GAP)) % 1).toBeCloseTo(0, 10);
    // 영역 밖에서 시작해 왼쪽 끝까지 덮는다.
    expect(left).toBeLessThanOrEqual(0);
  });

  it('영역 전체를 덮는다 — 패널 주변에서도 격자가 이어진다', () => {
    render(<PanelGridBackdrop areaW={800} areaH={600} boxW={448} boxH={332} {...base} />);

    const inner = screen.getByTestId('panel-grid-backdrop').firstElementChild as HTMLElement;
    const cols = inner.style.gridTemplateColumns.match(/repeat\((\d+),/)![1]!;
    const left = gridLeft();
    // 칸 수 × 간격이 시작점부터 영역 끝까지 닿는다.
    expect(left + Number(cols) * (CELL + GAP)).toBeGreaterThanOrEqual(800);
  });

  it('셀과 간격을 그대로 쓴다 — 대시보드와 같은 격자', () => {
    render(<PanelGridBackdrop areaW={800} areaH={600} boxW={448} boxH={332} {...base} />);

    const inner = screen.getByTestId('panel-grid-backdrop').firstElementChild as HTMLElement;
    expect(inner.style.gridTemplateColumns).toContain(`${CELL}px`);
    // row-gap column-gap — 채움 모드는 축별 배율이 달라 값이 갈릴 수 있다.
    expect(inner.style.gap).toBe(`${GAP}px ${GAP}px`);
  });

  it('칸 간격이 0 이면 그리지 않는다 — 무한 루프 대신 아무것도 그리지 않는다', () => {
    const { container } = render(
      <PanelGridBackdrop
        areaW={800}
        areaH={600}
        boxW={448}
        boxH={332}
        cellW={0}
        cellH={0}
        gapX={0}
        gapY={0}
      />,
    );
    expect(container.firstChild).toBeNull();
  });
});
