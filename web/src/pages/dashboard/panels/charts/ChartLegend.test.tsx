// ChartLegend — 실제 라인 차트 패널과 설정 미리보기가 공유하는 범례.
//
// 보고된 결함: 미리보기가 recharts 내장 <Legend> 를 쓰던 시절, 범례 옵션(이름/선/마지막 값)이
// 적용되지 않고 차트와 범례 사이 구분선·여백도 없었다.

import { describe, it, expect } from 'vitest';
import { cleanup, render, screen } from '@testing-library/react';

import { ChartLegend } from './ChartLegend';

const DATA = [
  { t: 0, LAI: 10, LAeq: 20 },
  { t: 1, LAI: 38.4, LAeq: 28.5 },
];

function renderLegend(legendCfg: Record<string, unknown> = {}) {
  return render(
    <ChartLegend
      seriesKeys={['LAI', 'LAeq']}
      seriesColors={['#c1', '#c2']}
      legendCfg={legendCfg}
      chartData={DATA}
      formatValue={(_k, v) => v.toFixed(1)}
    />,
  );
}

describe('ChartLegend 표시 옵션', () => {
  it('기본값은 이름·선 표시, 마지막 값 숨김', () => {
    renderLegend();
    expect(screen.getByText('LAI')).toBeInTheDocument();
    expect(screen.queryByText('38.4')).not.toBeInTheDocument();
  });

  it('show_last_value=true 면 시리즈별 마지막 유효 값을 표시한다', () => {
    renderLegend({ show_last_value: true });
    expect(screen.getByText('38.4')).toBeInTheDocument();
    expect(screen.getByText('28.5')).toBeInTheDocument();
  });

  it('show_name=false 면 이름을 숨긴다', () => {
    renderLegend({ show_name: false, show_last_value: true });
    expect(screen.queryByText('LAI')).not.toBeInTheDocument();
    expect(screen.getByText('38.4')).toBeInTheDocument();
  });

  it('마지막 값이 없는 시리즈는 —(대시)로 표기한다', () => {
    render(
      <ChartLegend
        seriesKeys={['LAI']}
        seriesColors={['#c1']}
          legendCfg={{ show_last_value: true }}
        chartData={[]}
        formatValue={(_k, v) => v.toFixed(1)}
      />,
    );
    expect(screen.getByText('—')).toBeInTheDocument();
  });
});

describe('ChartLegend 배치 — 차트와의 구분', () => {
  it('가로 배치(기본)는 상단 구분선 + 세로 여백을 갖는다', () => {
    renderLegend();
    const legend = screen.getByTestId('line-chart-legend');
    expect(legend.className).toContain('border-t');
    expect(legend.className).toContain('py-1.5');
  });

  it('세로 배치(left/right)는 좌측 구분선 + 가로 여백을 갖는다', () => {
    renderLegend({ position: 'right' });
    const legend = screen.getByTestId('line-chart-legend');
    expect(legend.className).toContain('border-l');
    expect(legend.className).toContain('pl-3');
  });
});


describe('ChartLegend 글자 스타일', () => {
  function style(): string {
    return screen.getByTestId('line-chart-legend').getAttribute('style') ?? '';
  }

  it('미지정이면 종전 크기(11px)를 쓴다 — 저장된 범례가 그대로 보인다', () => {
    renderLegend();
    expect(style()).toContain('font-size: 11px');
    expect(style()).not.toContain('font-family');
    expect(style()).not.toContain('position: relative');
  });

  it('크기·글꼴·색을 얹는다', () => {
    renderLegend({ font_size: 16, font_family: 'mono', font_color: '#ff0000' });
    expect(style()).toContain('font-size: 16px');
    expect(style()).toContain('ui-monospace');
    expect(style()).toContain('color: rgb(255, 0, 0)');
  });

  it('인식 불가 값은 무시한다 — 임의 색으로 떨어뜨리면 어두운 배경에서 글자가 사라진다', () => {
    renderLegend({ font_size: 0, font_color: 'red', font_family: 'comic' });
    expect(style()).toContain('font-size: 11px');
    expect(style()).not.toContain('color:');
  });

  it('색을 지정하면 이름 칸이 상속받는다 — 클래스가 남으면 지정색이 먹지 않는다', () => {
    renderLegend({ font_color: '#00ff00' });
    const name = screen.getByText('LAI');
    expect(name.className).not.toContain('--color-text-primary');
  });
});

describe('ChartLegend 위치 변위(드래그)', () => {
  // 변위는 **바깥 상자**에 붙는다. `position: relative` 의 left·top 퍼센트는 담는 블록의
  // 폭·높이를 기준으로 풀리는데, 바깥 상자의 담는 블록이 본문이라 두 축의 기준이 같아진다.
  // 안쪽에 붙이면 기준이 바깥 상자가 되어, 좌·우 배치에서 가로 기준이 범례 폭으로 쪼그라든다.
  function boxStyle(): string {
    return screen.getByTestId('line-chart-legend').getAttribute('style') ?? '';
  }

  it('변위가 0이면 상대 배치를 붙이지 않는다 — 저장된 대시보드의 그림이 변하지 않는다', () => {
    renderLegend({ offset_x: 0, offset_y: 0 });
    expect(boxStyle()).not.toContain('position: relative');
  });

  it('변위를 바깥 상자에 상대 배치로 얹는다 — 두 축이 같은 기준(본문)을 쓴다', () => {
    renderLegend({ offset_x: 12, offset_y: -8 });
    expect(boxStyle()).toContain('position: relative');
    expect(boxStyle()).toContain('left: 12%');
    expect(boxStyle()).toContain('top: -8%');
    // 안쪽 상자는 재는 자리일 뿐 스스로 움직이지 않는다.
    expect(
      screen.getByTestId('line-chart-legend-items').getAttribute('style') ?? '',
    ).not.toContain('position: relative');
  });

  it('저장된 변위는 성긴 상한으로 죈다 — 손으로 고쳐도 범례가 사라지지 않는다', () => {
    renderLegend({ offset_x: 9999, offset_y: -9999 });
    expect(boxStyle()).toContain('left: 50%');
    expect(boxStyle()).toContain('top: -50%');
  });

  it('잡는 표식은 바깥, 재는 표식은 안쪽에 둔다 — 늘어난 상자를 재면 한 축이 얼어붙는다', () => {
    renderLegend({ position: 'right' });
    const box = screen.getByTestId('line-chart-legend');
    expect(box.hasAttribute('data-chart-legend')).toBe(true);
    expect(box.getAttribute('data-position')).toBe('right');
    expect(
      screen.getByTestId('line-chart-legend-items').hasAttribute('data-chart-legend-content'),
    ).toBe(true);
  });

  it('구분선은 제자리일 때만 그린다 — 항목이 떠난 자리에 선만 남으면 뜻이 없다', () => {
    renderLegend({ position: 'right' });
    expect(screen.getByTestId('line-chart-legend').className).toContain('border-l');

    cleanup();
    renderLegend({ position: 'right', offset_x: 10 });
    expect(screen.getByTestId('line-chart-legend').className).not.toContain('border-l');
  });

  it('자리(여백)는 옮겨도 그대로다 — 차트 크기가 변하지 않는다', () => {
    renderLegend({ position: 'right', offset_x: 10, offset_y: 20 });
    const box = screen.getByTestId('line-chart-legend').className;
    expect(box).toContain('py-2');
    expect(box).toContain('pl-3');
    expect(box).toContain('shrink-0');
  });
});
