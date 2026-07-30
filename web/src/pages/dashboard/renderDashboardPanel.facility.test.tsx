// renderDashboardPanel 설비 디스패치 테스트 (SPEC-FACILITY-DASHBOARD-001 M6 / SPEC-XSFM-GROUP-001 M7).
//
// switch(panel.type) 가 설비 타입을 각각의 컴포넌트로 디스패치하는지 검증한다. 설비 패널 컴포넌트를
// 경량 스텁으로 mock 하여 타입별로 올바른 컴포넌트만 렌더되고 panelId/title/config props 가 전달되는지
// 확인한다. 레거시 facility-station 은 FacilityGroupPanel 별칭으로 디스패치된다(하위호환).

import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import type { PanelConfig, PanelType } from '@/stores/uiStore';

// 설비 패널을 각각 구별 가능한 testid 스텁으로 대체한다.
// FacilityStationPanel 은 그룹 패널로 흡수되어 제거되었으므로 mock 하지 않는다.
vi.mock('./panels/FacilityDevicePanel', () => ({
  default: (props: { panelId: string; title: string; config: Record<string, unknown> }) => (
    <div data-testid="stub-facility-device" data-title={props.title} data-cfg={JSON.stringify(props.config)} />
  ),
}));
vi.mock('./panels/FacilityLinePanel', () => ({
  default: (props: { panelId: string; title: string; config: Record<string, unknown> }) => (
    <div data-testid="stub-facility-line" data-title={props.title} data-cfg={JSON.stringify(props.config)} />
  ),
}));
// SPEC-XSFM-GROUP-001 M7: 설비 그룹 패널 스텁(facility-group + 레거시 facility-station 별칭 공용).
vi.mock('./panels/FacilityGroupPanel', () => ({
  default: (props: { panelId: string; title: string; config: Record<string, unknown> }) => (
    <div data-testid="stub-facility-group" data-title={props.title} data-cfg={JSON.stringify(props.config)} />
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

describe('renderDashboardPanel 설비 디스패치', () => {
  const cases: { type: PanelType; testid: string; config: Record<string, unknown> }[] = [
    { type: 'facility-device', testid: 'stub-facility-device', config: { agentId: 'a1', deviceId: 'd1' } },
    { type: 'facility-line', testid: 'stub-facility-line', config: { agentId: 'a1', line: '2호선' } },
    { type: 'facility-group', testid: 'stub-facility-group', config: { agentId: 'a1', groupId: 'custom:floor2', showStats: true } },
  ];

  it.each(cases)('$type → 해당 컴포넌트로 디스패치하고 props 를 전달한다', ({ type, testid, config }) => {
    renderPanel(panel(type, config));

    const el = screen.getByTestId(testid);
    expect(el).toBeInTheDocument();
    expect(el).toHaveAttribute('data-title', `T-${type}`);
    expect(el).toHaveAttribute('data-cfg', JSON.stringify(config));

    // 다른 설비 컴포넌트는 렌더되지 않는다(정확한 1:1 디스패치).
    for (const other of cases.filter((c) => c.testid !== testid)) {
      expect(screen.queryByTestId(other.testid)).toBeNull();
    }
  });

  // ---- 하위호환: 레거시 facility-station → FacilityGroupPanel 별칭 ----

  it('레거시 facility-station 을 FacilityGroupPanel 로 디스패치하고 config(station)를 그대로 전달한다', () => {
    const cfg = { agentId: 'a1', station: 'st01' };
    renderPanel(panel('facility-station', cfg));

    // 별칭: 그룹 패널 스텁이 렌더되고, config.station 이 그대로 전달되어 패널이 groupId 를 파생한다.
    const el = screen.getByTestId('stub-facility-group');
    expect(el).toBeInTheDocument();
    expect(el).toHaveAttribute('data-title', 'T-facility-station');
    expect(el).toHaveAttribute('data-cfg', JSON.stringify(cfg));
    // 라인/기기 스텁은 렌더되지 않는다.
    expect(screen.queryByTestId('stub-facility-line')).toBeNull();
    expect(screen.queryByTestId('stub-facility-device')).toBeNull();
  });
});
