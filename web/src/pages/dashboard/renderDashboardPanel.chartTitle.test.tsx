// renderDashboardPanel — 차트 5종에 panel.title 을 전달하는지 검증.
//
// 회귀 배경: line-chart 만 title 을 넘기고 stat/bar/pie/table 은 빠져 있었다. 각 패널 헤더는
// `title || channel_name || '채널 미지정'` 으로 폴백하므로, 넘기지 않으면 사용자가 지정한
// 패널 이름 대신 채널 이름이 뜨고, store/tsdb 소스 패널은 channel_name 이 비어 있어
// "채널 미지정" 까지 내려간다.

import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import type { PanelConfig } from '@/stores/uiStore';

/** 차트 패널 stub — 받은 props 를 data-* 로 노출한다. */
function stub(testid: string) {
  return {
    default: (props: { panelId: string; title?: string; config: Record<string, unknown> }) => (
      <div data-testid={testid} data-panelid={props.panelId} data-title={props.title ?? ''} />
    ),
  };
}

vi.mock('./panels/charts/StatPanel', () => stub('stub-stat'));
vi.mock('./panels/charts/LineChartPanel', () => stub('stub-line'));
vi.mock('./panels/charts/BarChartPanel', () => stub('stub-bar'));
vi.mock('./panels/charts/PieChartPanel', () => stub('stub-pie'));
vi.mock('./panels/charts/TablePanel', () => stub('stub-table'));

import { renderDashboardPanel } from './renderDashboardPanel';

const handlers = () => ({ onConfigChange: vi.fn(), onTitleChange: vi.fn() });

const CASES = [
  { type: 'stat', testid: 'stub-stat' },
  { type: 'line-chart', testid: 'stub-line' },
  { type: 'bar-chart', testid: 'stub-bar' },
  { type: 'pie-chart', testid: 'stub-pie' },
  { type: 'table', testid: 'stub-table' },
] as const;

describe('renderDashboardPanel — 차트 패널 타이틀 전달', () => {
  for (const { type, testid } of CASES) {
    it(`${type}: panel.title 을 그대로 전달한다`, () => {
      const panel: PanelConfig = {
        id: `p-${type}`,
        type,
        title: '실외기 온도',
        // store 소스 패널은 channel_name 이 비어 있다 — 폴백이 "채널 미지정" 으로 내려가는
        // 조건을 그대로 재현한다.
        config: { channel_name: '', data_source: 'store' },
      } as unknown as PanelConfig;
      render(<>{renderDashboardPanel(panel, [], undefined, 10, handlers)}</>);

      const el = screen.getByTestId(testid);
      expect(el).toHaveAttribute('data-panelid', `p-${type}`);
      expect(el).toHaveAttribute('data-title', '실외기 온도');
    });
  }
});
