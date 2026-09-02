// SPEC-MODBUS-012 M1/M4/M5 (REQ-01): renderDashboardPanel MODBUS Gateway 디스패치.
//
// switch(panel.type) 가 6종 신규 타입을 각 전용 컴포넌트로 디스패치하고 title/config props
// 를 전달하는지 검증한다(M4/M5 완성으로 플레이스홀더 제거, 6종 모두 실제 컴포넌트로 1:1).

import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

import type { PanelConfig, PanelType } from '@/stores/uiStore';

function stub(testid: string) {
  return {
    default: (props: { title: string; config: Record<string, unknown> }) => (
      <div data-testid={testid} data-title={props.title} data-cfg={JSON.stringify(props.config)} />
    ),
  };
}

vi.mock('./panels/modbus/ModbusRealDevicesPanel', () => stub('stub-modbus-real'));
vi.mock('./panels/modbus/ModbusVirtualDevicesPanel', () => stub('stub-modbus-virtual'));
vi.mock('./panels/modbus/ModbusSharedRegistersPanel', () => stub('stub-modbus-shared'));
vi.mock('./panels/modbus/ModbusDeviceRegistersPanel', () => stub('stub-modbus-device'));
vi.mock('./panels/modbus/ModbusBusStatsPanel', () => stub('stub-modbus-bus'));
vi.mock('./panels/modbus/ModbusSummaryStatsPanel', () => stub('stub-modbus-summary'));

import { renderDashboardPanel } from './renderDashboardPanel';

const handlers = () => ({ onConfigChange: vi.fn(), onTitleChange: vi.fn() });

function panel(type: PanelType, config: Record<string, unknown>): PanelConfig {
  return { id: `p-${type}`, type, title: `T-${type}`, config };
}

function renderPanel(p: PanelConfig) {
  return render(<>{renderDashboardPanel(p, [], undefined, 10, handlers)}</>);
}

describe('renderDashboardPanel MODBUS Gateway 디스패치 (SPEC-MODBUS-012)', () => {
  const cases: { type: PanelType; testid: string }[] = [
    { type: 'modbus-real-devices', testid: 'stub-modbus-real' },
    { type: 'modbus-virtual-devices', testid: 'stub-modbus-virtual' },
    { type: 'modbus-shared-registers', testid: 'stub-modbus-shared' },
    { type: 'modbus-device-registers', testid: 'stub-modbus-device' },
    { type: 'modbus-bus-stats', testid: 'stub-modbus-bus' },
    { type: 'modbus-summary-stats', testid: 'stub-modbus-summary' },
  ];

  it.each(cases)('$type → 전용 컴포넌트로 디스패치하고 title/config 를 전달한다', ({ type, testid }) => {
    const config = { agentId: 'gw-1' };
    renderPanel(panel(type, config));
    const el = screen.getByTestId(testid);
    expect(el).toBeInTheDocument();
    expect(el).toHaveAttribute('data-title', `T-${type}`);
    expect(el).toHaveAttribute('data-cfg', JSON.stringify(config));
  });

  it('6종은 서로 다른 컴포넌트로 1:1 디스패치된다(교차 렌더 없음)', () => {
    renderPanel(panel('modbus-shared-registers', { agentId: 'gw-1' }));
    expect(screen.getByTestId('stub-modbus-shared')).toBeInTheDocument();
    expect(screen.queryByTestId('stub-modbus-device')).toBeNull();
    expect(screen.queryByTestId('stub-modbus-summary')).toBeNull();
  });
});
