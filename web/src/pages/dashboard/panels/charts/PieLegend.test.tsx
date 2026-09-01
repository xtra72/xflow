// PieLegend 테스트 — 세로 나열에서만 칸을 맞추는 규칙과 표시 옵션.

import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';

import { PieLegend, type PieLegendItem } from './PieLegend';

const items: PieLegendItem[] = [
  { name: 'A', value: 3, percent: 0.75, color: '#f00' },
  { name: 'B', value: 1, percent: 0.25, color: '#0f0' },
];

function renderLegend(props: Partial<React.ComponentProps<typeof PieLegend>> = {}) {
  return render(
    <PieLegend
      items={items}
      position="bottom"
      showPercentage={false}
      showValue={false}
      fontSize={12}
      decimals={1}
      offsetX={0}
      offsetY={0}
      {...props}
    />,
  );
}

describe('PieLegend', () => {
  it('항목이 없으면 아무것도 그리지 않는다', () => {
    const { container } = renderLegend({ items: [] });
    expect(container.firstChild).toBeNull();
  });

  it('이름은 언제나 있다 — 색 점과 함께', () => {
    renderLegend();
    expect(screen.getAllByTestId('pie-legend-name').map((e) => e.textContent)).toEqual(['A', 'B']);
  });

  describe('칸 맞추기는 세로 나열에서만', () => {
    it('left / right 는 grid 로 열을 맞춘다', () => {
      for (const position of ['left', 'right'] as const) {
        const { unmount } = renderLegend({ position, showPercentage: true, showValue: true });
        const legend = screen.getByTestId('pie-chart-legend');
        expect(legend.getAttribute('data-aligned')).toBe('true');
        // 이름 · 비율 · 값 세 칸.
        expect(legend.getAttribute('style')).toContain('grid-template-columns: auto auto auto');
        unmount();
      }
    });

    it('행이 패널 높이만큼 늘어나지 않는다 — 항목 사이가 벌어지지 않게', () => {
      renderLegend({ position: 'right' });
      // content-center 가 없으면 grid 행이 stretch 되어 항목 3개가 화면 높이로 흩어진다.
      expect(screen.getByTestId('pie-chart-legend').className).toContain('content-center');
    });

    it('켠 칸 수만큼만 열을 만든다', () => {
      renderLegend({ position: 'right', showPercentage: true, showValue: false });
      const style = screen.getByTestId('pie-chart-legend').getAttribute('style') ?? '';
      expect(style).toContain('grid-template-columns: auto auto;');
    });

    it('하단(가로)은 맞출 기준선이 없어 grid 를 쓰지 않는다', () => {
      renderLegend({ position: 'bottom', showPercentage: true, showValue: true });
      const legend = screen.getByTestId('pie-chart-legend');
      expect(legend.getAttribute('data-aligned')).toBe('false');
      expect(legend.getAttribute('style')).not.toContain('grid-template-columns');
    });
  });

  it('비율·값은 켠 것만 칸을 만든다', () => {
    renderLegend({ showPercentage: true });
    expect(screen.getAllByTestId('pie-legend-percent').map((e) => e.textContent)).toEqual([
      '75%',
      '25%',
    ]);
    expect(screen.queryAllByTestId('pie-legend-value')).toHaveLength(0);
  });

  it('값은 자릿수·단위 규칙을 따른다', () => {
    renderLegend({ showValue: true, decimals: 2, unit: 'kW' });
    expect(screen.getAllByTestId('pie-legend-value').map((e) => e.textContent)).toEqual([
      '3.00kW',
      '1.00kW',
    ]);
  });

  it('글자 크기와 오프셋이 스타일로 실린다', () => {
    renderLegend({ fontSize: 18, offsetX: 10, offsetY: -6 });
    const style = screen.getByTestId('pie-chart-legend').getAttribute('style') ?? '';
    expect(style).toContain('font-size: 18px');
    expect(style).toContain('translate(calc(-50% + 10px), -6px)');
  });

  it('글꼴은 지정했을 때만 싣는다 — 미지정은 상속', () => {
    const { unmount } = renderLegend();
    expect(screen.getByTestId('pie-chart-legend').getAttribute('style') ?? '').not.toContain(
      'font-family',
    );
    unmount();

    renderLegend({ fontFamily: 'serif' });
    expect(screen.getByTestId('pie-chart-legend').getAttribute('style') ?? '').toContain('Georgia');
  });

  it('글자색을 지정하면 이름·비율·값이 함께 따라간다', () => {
    renderLegend({ fontColor: '#ff0000', showPercentage: true });
    const legend = screen.getByTestId('pie-chart-legend');
    expect(legend.getAttribute('style') ?? '').toContain('color: rgb(255, 0, 0)');
    // 지정색일 때 비율 칸은 muted 토큰 대신 불투명도로 톤을 준다 — 토큰을 그대로 두면
    // 지정한 색이 그 칸에만 먹지 않는다.
    expect(screen.getAllByTestId('pie-legend-percent')[0]!.className).toContain('opacity-70');
    expect(screen.getAllByTestId('pie-legend-percent')[0]!.className).not.toContain(
      'text-(--color-text-muted)',
    );
  });

  it('파이 위에 겹쳐 뜬다 — 자리를 나눠 가지지 않는다', () => {
    renderLegend({ position: 'right' });
    const cls = screen.getByTestId('pie-chart-legend').className;
    expect(cls).toContain('absolute');
    expect(cls).toContain('z-10');
    // 겹친 자리에서 글자가 읽히도록 옅은 판을 깐다.
    expect(cls).toContain('bg-(--color-bg-surface)/80');
  });

  it('드래그 레이어가 잡을 수 있도록 표식을 남긴다', () => {
    renderLegend();
    expect(screen.getByTestId('pie-chart-legend').hasAttribute('data-pie-legend')).toBe(true);
  });
});
