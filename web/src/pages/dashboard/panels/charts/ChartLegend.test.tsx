// ChartLegend — 실제 라인 차트 패널과 설정 미리보기가 공유하는 범례.
//
// 보고된 결함: 미리보기가 recharts 내장 <Legend> 를 쓰던 시절, 범례 옵션(이름/선/마지막 값)이
// 적용되지 않고 차트와 범례 사이 구분선·여백도 없었다.

import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';

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
      isMultiMode={false}
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
        isMultiMode={false}
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

describe('ChartLegend 연결 상태 점', () => {
  it('상태를 주입하지 않으면(미리보기) 점을 그리지 않는다', () => {
    const { container } = renderLegend();
    expect(container.querySelector('[title]')).toBeNull();
  });

  it('상태를 주입하면(실제 패널) 점을 그린다', () => {
    render(
      <ChartLegend
        seriesKeys={['LAI']}
        seriesColors={['#c1']}
        isMultiMode={false}
        channelStates={[{ ref: { name: 'LAI' }, state: { status: 'connected' } }]}
        legendCfg={{}}
        chartData={DATA}
        formatValue={(_k, v) => v.toFixed(1)}
      />,
    );
    expect(screen.getByTitle('connected')).toBeInTheDocument();
  });

  it('시리즈가 없으면 아무것도 렌더하지 않는다', () => {
    const { container } = render(
      <ChartLegend
        seriesKeys={[]}
        seriesColors={[]}
        isMultiMode={false}
        legendCfg={{}}
        chartData={[]}
        formatValue={(_k, v) => String(v)}
      />,
    );
    expect(container.firstChild).toBeNull();
  });
});
