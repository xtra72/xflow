// renderDashboardPanel 설비 디스패치 테스트 (SPEC-FACILITY-DASHBOARD-001 M6, REQ-04-03).
//
// switch(panel.type) 가 3종 설비 타입을 각각의 컴포넌트로 디스패치하는지 검증한다.
// 설비 패널 컴포넌트를 경량 스텁으로 mock 하여, 타입별로 올바른 컴포넌트만 렌더되고
// panelId/title/config props 가 전달되는지 확인한다.

import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import type { PanelConfig, PanelType } from '@/stores/uiStore';

// 3종 설비 패널을 각각 구별 가능한 testid 스텁으로 대체한다.
vi.mock('./panels/FacilityDevicePanel', () => ({
  default: (props: { panelId: string; title: string; config: Record<string, unknown> }) => (
    <div data-testid="stub-facility-device" data-title={props.title} data-cfg={JSON.stringify(props.config)} />
  ),
}));
vi.mock('./panels/FacilityStationPanel', () => ({
  default: (props: { panelId: string; title: string; config: Record<string, unknown> }) => (
    <div data-testid="stub-facility-station" data-title={props.title} data-cfg={JSON.stringify(props.config)} />
  ),
}));
vi.mock('./panels/FacilityLinePanel', () => ({
  default: (props: { panelId: string; title: string; config: Record<string, unknown> }) => (
    <div data-testid="stub-facility-line" data-title={props.title} data-cfg={JSON.stringify(props.config)} />
  ),
}));

import { renderDashboardPanel } from './renderDashboardPanel';

/** config/title 변경 핸들러(디스패치 검증에는 no-op). */
const handlers = () => ({ onConfigChange: vi.fn(), onTitleChange: vi.fn() });

function panel(type: PanelType, config: Record<string, unknown>): PanelConfig {
  return { id: `p-${type}`, type, title: `T-${type}`, config };
}

function renderPanel(p: PanelConfig) {
  return render(<>{renderDashboardPanel(p, [], undefined, 10, handlers)}</>);
}

describe('renderDashboardPanel 설비 디스패치 (SPEC-FACILITY-DASHBOARD-001 REQ-04-03)', () => {
  const cases: { type: PanelType; testid: string; config: Record<string, unknown> }[] = [
    { type: 'facility-device', testid: 'stub-facility-device', config: { agentId: 'a1', deviceId: 'd1' } },
    { type: 'facility-station', testid: 'stub-facility-station', config: { agentId: 'a1', station: 'ST-1' } },
    { type: 'facility-line', testid: 'stub-facility-line', config: { agentId: 'a1', line: '2호선' } },
  ];

  it.each(cases)('$type → 해당 컴포넌트로 디스패치하고 props 를 전달한다', ({ type, testid, config }) => {
    renderPanel(panel(type, config));

    const el = screen.getByTestId(testid);
    expect(el).toBeInTheDocument();
    // title/config props 가 그대로 전달된다.
    expect(el).toHaveAttribute('data-title', `T-${type}`);
    expect(el).toHaveAttribute('data-cfg', JSON.stringify(config));

    // 다른 두 설비 컴포넌트는 렌더되지 않는다(정확한 1:1 디스패치).
    for (const other of cases.filter((c) => c.type !== type)) {
      expect(screen.queryByTestId(other.testid)).toBeNull();
    }
  });
});
