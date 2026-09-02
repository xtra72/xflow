// SPEC-TRIGGER-SCHED-001 M2 / AC-13 — 신규 facility-schedule 패널이 범용 trigger-config 과 공존.
//
// renderDashboardPanel 의 switch 가 두 패널 타입을 각각의 컴포넌트로 디스패치하는지 검증한다.
// 두 패널을 경량 스텁으로 mock 하여 1:1 디스패치 + props 전달을 확인한다(대체 아님, 공존).

import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import type { PanelConfig, PanelType } from '@/stores/uiStore';

vi.mock('./panels/TriggerConfigPanel', () => ({
  default: (props: { title: string; config: Record<string, unknown> }) => (
    <div data-testid="stub-trigger-config" data-title={props.title} data-cfg={JSON.stringify(props.config)} />
  ),
}));
vi.mock('./panels/facilitySchedule/FacilitySchedulePanel', () => ({
  default: (props: { title: string; config: Record<string, unknown> }) => (
    <div data-testid="stub-facility-schedule" data-title={props.title} data-cfg={JSON.stringify(props.config)} />
  ),
}));

import { renderDashboardPanel } from './renderDashboardPanel';

const handlers = () => ({ onConfigChange: vi.fn(), onTitleChange: vi.fn() });
function panel(type: PanelType, config: Record<string, unknown>): PanelConfig {
  return { id: `p-${type}`, type, title: `T-${type}`, config };
}
function renderPanel(p: PanelConfig) {
  return render(<>{renderDashboardPanel(p, [], undefined, 10, handlers)}</>);
}

describe('renderDashboardPanel 트리거/설비예약 공존 디스패치 (AC-13)', () => {
  it('facility-schedule → FacilitySchedulePanel 로 디스패치하고 config 를 전달한다', () => {
    const cfg = { flowId: 'f1', nodeId: 'n1', agentId: 'a1' };
    renderPanel(panel('facility-schedule', cfg));
    const el = screen.getByTestId('stub-facility-schedule');
    expect(el).toBeInTheDocument();
    expect(el).toHaveAttribute('data-cfg', JSON.stringify(cfg));
    // 범용 트리거 패널은 렌더되지 않는다.
    expect(screen.queryByTestId('stub-trigger-config')).toBeNull();
  });

  it('trigger-config → TriggerConfigPanel 로 디스패치한다(신규 패널이 대체하지 않음)', () => {
    const cfg = { flowId: 'f1', nodeId: 'n1', payloadCatalog: {} };
    renderPanel(panel('trigger-config', cfg));
    expect(screen.getByTestId('stub-trigger-config')).toBeInTheDocument();
    expect(screen.queryByTestId('stub-facility-schedule')).toBeNull();
  });

  it('두 패널 타입이 동시에 렌더될 수 있다(공존)', () => {
    render(
      <>
        {renderDashboardPanel(panel('trigger-config', { flowId: 'f1', nodeId: 'n1', payloadCatalog: {} }), [], undefined, 10, handlers)}
        {renderDashboardPanel(panel('facility-schedule', { flowId: 'f2', nodeId: 'n2', agentId: 'a1' }), [], undefined, 10, handlers)}
      </>,
    );
    expect(screen.getByTestId('stub-trigger-config')).toBeInTheDocument();
    expect(screen.getByTestId('stub-facility-schedule')).toBeInTheDocument();
  });
});
