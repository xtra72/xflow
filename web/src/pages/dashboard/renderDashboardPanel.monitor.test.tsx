// renderDashboardPanel 모니터링 패널 4종 디스패치 + uiStore 기본값 검증.
//
// switch(panel.type) 가 네 타입을 각 패널로 보내고 panelId/title/config 를 전달하는지,
// 그리고 추가 시 기본 표시 항목이 채워지는지 확인한다.

import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import type { PanelConfig } from '@/stores/uiStore';

vi.mock('./panels/monitor/MonitorStatsPanel', () => ({
  default: (p: { panelId: string; title: string; config: Record<string, unknown> }) => (
    <div data-testid="stub-monitor-stats" data-panelid={p.panelId} data-title={p.title} data-cfg={JSON.stringify(p.config)} />
  ),
}));
vi.mock('./panels/monitor/MonitorMetricsPanel', () => ({
  default: (p: { panelId: string; title: string }) => (
    <div data-testid="stub-monitor-metrics" data-panelid={p.panelId} data-title={p.title} />
  ),
}));
vi.mock('./panels/monitor/MonitorNetworkPanel', () => ({
  default: (p: { panelId: string; title: string }) => (
    <div data-testid="stub-monitor-network" data-panelid={p.panelId} data-title={p.title} />
  ),
}));
vi.mock('./panels/monitor/MonitorLogsPanel', () => ({
  default: (p: { panelId: string; title: string }) => (
    <div data-testid="stub-monitor-logs" data-panelid={p.panelId} data-title={p.title} />
  ),
}));
vi.mock('./panels/monitor/MonitorEventsPanel', () => ({
  default: (p: { panelId: string; title: string }) => (
    <div data-testid="stub-monitor-events" data-panelid={p.panelId} data-title={p.title} />
  ),
}));

import { renderDashboardPanel } from './renderDashboardPanel';
import { useUIStore } from '@/stores/uiStore';
import { DEFAULT_LAYOUT } from '@/pages/monitoring/monitoringLayout';

const handlers = () => ({ onConfigChange: vi.fn(), onTitleChange: vi.fn() });

const CASES: Array<[PanelConfig['type'], string]> = [
  ['monitor-stats', 'stub-monitor-stats'],
  ['monitor-metrics', 'stub-monitor-metrics'],
  ['monitor-network', 'stub-monitor-network'],
  ['monitor-logs', 'stub-monitor-logs'],
  ['monitor-events', 'stub-monitor-events'],
];

describe('renderDashboardPanel 모니터링 패널 디스패치', () => {
  it.each(CASES)('%s → %s 로 디스패치한다', (type, testId) => {
    const panel: PanelConfig = {
      id: `p-${type}`,
      type,
      title: `제목 ${type}`,
      config: { items: [] },
    };
    render(<>{renderDashboardPanel(panel, [], undefined, 10, handlers)}</>);

    const el = screen.getByTestId(testId);
    expect(el).toHaveAttribute('data-panelid', `p-${type}`);
    expect(el).toHaveAttribute('data-title', `제목 ${type}`);
  });

  it('config 를 그대로 패널에 전달한다', () => {
    const panel: PanelConfig = {
      id: 'p1',
      type: 'monitor-stats',
      title: '통계',
      config: { items: ['uptime'] },
    };
    render(<>{renderDashboardPanel(panel, [], undefined, 10, handlers)}</>);

    expect(screen.getByTestId('stub-monitor-stats')).toHaveAttribute(
      'data-cfg',
      JSON.stringify({ items: ['uptime'] }),
    );
  });
});

describe('uiStore 모니터링 패널 기본값', () => {
  /** 현재 활성 대시보드 페이지. */
  function activePage() {
    const s = useUIStore.getState();
    return s.dashboardPages.find((p) => p.id === s.activeDashboardId)!;
  }

  const cases: Array<{ type: PanelConfig['type']; items: readonly string[]; size: { w: number; h: number } }> = [
    { type: 'monitor-stats', items: DEFAULT_LAYOUT.stats, size: { w: 4, h: 3 } },
    { type: 'monitor-metrics', items: DEFAULT_LAYOUT.metrics, size: { w: 6, h: 4 } },
    { type: 'monitor-network', items: DEFAULT_LAYOUT.network, size: { w: 6, h: 5 } },
    { type: 'monitor-logs', items: DEFAULT_LAYOUT.logs, size: { w: 8, h: 5 } },
    { type: 'monitor-events', items: DEFAULT_LAYOUT.events, size: { w: 4, h: 5 } },
  ];

  it.each(cases)('addPanel($type) → 섹션 기본 항목 + 그리드 크기', ({ type, items, size }) => {
    useUIStore.getState().addPanel(type);

    const page = activePage();
    const panel = page.panels[page.panels.length - 1]!;
    expect(panel.type).toBe(type);
    expect(panel.config.items).toEqual([...items]);
    expect(panel.title).toBeTruthy();

    const layout = page.layout.find((l) => l.i === panel.id)!;
    expect(layout.w).toBe(size.w);
    expect(layout.h).toBe(size.h);
  });
});
