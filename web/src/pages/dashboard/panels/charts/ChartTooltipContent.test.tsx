// 툴팁 내용의 배치 계약.
//
// 보고된 결함: recharts 기본 내용은 한 줄을 `이름 : 값` 으로 이어 붙여, 시리즈 이름
// 길이가 제각각이면 값의 시작 위치가 줄마다 어긋났다. 여기서 고정하는 것은 픽셀이
// 아니라 **구조** 다 — 이름 칸과 값 칸이 나뉘고, 값 칸이 오른쪽 끝에 붙는가.

import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';

import { ChartTooltipContent } from './ChartTooltipContent';

type Props = Parameters<typeof ChartTooltipContent>[0];

function renderContent(over: Partial<Props> = {}): void {
  const props = {
    active: true,
    label: 0,
    payload: [
      { name: '아주긴시리즈이름', value: 1, color: '#f00', graphicalItemId: 'a' },
      { name: 'b', value: 22, color: '#0f0', graphicalItemId: 'b' },
    ],
    ...over,
  } as unknown as Props;
  render(<ChartTooltipContent {...props} />);
}

describe('ChartTooltipContent', () => {
  it('이름은 왼쪽, 값은 오른쪽 끝에 세운다', () => {
    renderContent();
    for (const el of screen.getAllByTestId('chart-tooltip-name')) {
      expect(el.className).toContain('text-left');
    }
    for (const el of screen.getAllByTestId('chart-tooltip-value')) {
      // ml-auto 가 값 칸을 오른쪽 끝으로 민다 — 이름 길이와 무관하게 자릿수가 세로로 맞는다.
      expect(el.className).toContain('ml-auto');
      expect(el.className).toContain('text-right');
    }
  });

  it('시리즈마다 한 줄을 그린다', () => {
    renderContent();
    expect(screen.getAllByTestId('chart-tooltip-row')).toHaveLength(2);
  });

  it('formatter 의 [값, 이름] 쌍을 그대로 따른다', () => {
    renderContent({
      formatter: ((value: number, name: string) => [`${value}℃`, `[${name}]`]) as Props['formatter'],
    });
    const values = screen.getAllByTestId('chart-tooltip-value').map((e) => e.textContent);
    const names = screen.getAllByTestId('chart-tooltip-name').map((e) => e.textContent);
    expect(values).toEqual(['1℃', '22℃']);
    expect(names).toEqual(['[아주긴시리즈이름]', '[b]']);
  });

  it('labelFormatter 결과를 머리에 놓는다', () => {
    renderContent({ labelFormatter: (() => '12:00:00') as Props['labelFormatter'] });
    expect(screen.getByTestId('chart-tooltip-content').textContent).toContain('12:00:00');
  });

  it('비활성이거나 그릴 줄이 없으면 상자를 만들지 않는다', () => {
    const { container } = render(
      <ChartTooltipContent {...({ active: false, payload: [] } as unknown as Props)} />,
    );
    expect(container.firstChild).toBeNull();
    const empty = render(
      <ChartTooltipContent {...({ active: true, payload: [] } as unknown as Props)} />,
    );
    expect(empty.container.firstChild).toBeNull();
  });
});
