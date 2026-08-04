// SPEC-MODBUS-012 M2 (REQ-03, AC-07/08 + AC-03 graceful): 가상 디바이스 목록 패널.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';

import { AREA_BADGES, formatUnitLabel } from './useModbusData';

// mock 이 참조하는 가변 상태(gate/devices)를 테스트에서 조정한다.
const mockState = vi.hoisted(() => ({
  gate: { agentId: 'gw-1', remote: false, bound: true, enabled: true },
  devices: [] as unknown[],
  isLoading: false,
  isError: false,
}));

vi.mock('./useModbusData', async () => {
  const actual = await vi.importActual<typeof import('./useModbusData')>('./useModbusData');
  return {
    ...actual,
    useModbusGate: () => mockState.gate,
    useModbusListDevices: () => ({
      devices: mockState.devices,
      isLoading: mockState.isLoading,
      isError: mockState.isError,
    }),
  };
});

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import ModbusVirtualDevicesPanel from './ModbusVirtualDevicesPanel';

function dev(unit_id: number, name: string, counts: Partial<Record<string, number>>, stats: { r?: number; w?: number; e?: number }) {
  return {
    unit_id,
    name,
    register_counts: {
      coils: counts.coils ?? 0,
      discrete_inputs: counts.discrete_inputs ?? 0,
      holding_registers: counts.holding_registers ?? 0,
      input_registers: counts.input_registers ?? 0,
    },
    status: 'active',
    backed: false,
    mode: '',
    stats: { read_count: stats.r ?? 0, write_count: stats.w ?? 0, error_count: stats.e ?? 0 },
  };
}

function renderPanel() {
  return render(<ModbusVirtualDevicesPanel title="가상 디바이스" config={{ agentId: 'gw-1' }} />);
}

describe('ModbusVirtualDevicesPanel (SPEC-MODBUS-012 REQ-03)', () => {
  beforeEach(() => {
    mockState.gate = { agentId: 'gw-1', remote: false, bound: true, enabled: true };
    mockState.devices = [];
    mockState.isLoading = false;
    mockState.isError = false;
  });

  it('AC-03: agentId 미설정 시 안내 문구를 표시한다', () => {
    mockState.gate = { agentId: '', remote: false, bound: false, enabled: false };
    renderPanel();
    expect(screen.getByText('dashboard.modbus.notConfigured')).toBeInTheDocument();
  });

  it('AC-03: 원격 타깃이면 사용 불가 안내를 표시한다', () => {
    mockState.gate = { agentId: 'gw-1', remote: true, bound: true, enabled: false };
    renderPanel();
    expect(screen.getByText('dashboard.modbus.remoteUnavailable')).toBeInTheDocument();
  });

  it('빈 목록이면 empty 안내를 표시한다', () => {
    mockState.devices = [];
    renderPanel();
    expect(screen.getByText('dashboard.modbus.empty')).toBeInTheDocument();
  });

  it('AC-07: U01~ 목록에 unit 라벨·이름·영역 배지가 표시된다', () => {
    mockState.devices = [
      dev(1, '펌프', { coils: 2, holding_registers: 5 }, { r: 0 }),
      dev(2, '밸브', { discrete_inputs: 3, input_registers: 1 }, { r: 0 }),
    ];
    renderPanel();

    expect(screen.getByTestId('modbus-virtual-device-1')).toBeInTheDocument();
    expect(screen.getByText('펌프')).toBeInTheDocument();
    expect(screen.getByText(formatUnitLabel(1))).toBeInTheDocument();

    // U01: coils(CO) + holding(HR) 배지만, DI/IR 없음.
    expect(screen.getByTestId('modbus-area-badge-1-coils')).toBeInTheDocument();
    expect(screen.getByTestId('modbus-area-badge-1-holding_registers')).toBeInTheDocument();
    expect(screen.queryByTestId('modbus-area-badge-1-discrete_inputs')).toBeNull();
    expect(screen.queryByTestId('modbus-area-badge-1-input_registers')).toBeNull();
    // U02: DI + IR 배지.
    expect(screen.getByTestId('modbus-area-badge-2-discrete_inputs')).toBeInTheDocument();
    expect(screen.getByTestId('modbus-area-badge-2-input_registers')).toBeInTheDocument();

    // 배지 라벨 매핑 검증(CO/DI/IR/HR).
    const labels = AREA_BADGES.map((b) => b.label);
    expect(labels).toEqual(['CO', 'DI', 'IR', 'HR']);
  });

  it('AC-08: 폴링 델타로 접근 있는 디바이스는 active, 무접근은 stale', () => {
    // 1차 폴: 기준선. 최초 관측이라 둘 다 stale.
    mockState.devices = [dev(1, 'U1', { coils: 1 }, { r: 5 }), dev(2, 'U2', { coils: 1 }, { r: 0 })];
    const { rerender } = renderPanel();
    expect(screen.getByTestId('modbus-virtual-status-1')).toHaveAttribute('data-status', 'stale');
    expect(screen.getByTestId('modbus-virtual-status-2')).toHaveAttribute('data-status', 'stale');

    // 2차 폴: U01 은 read 증가(접근), U02 는 그대로.
    mockState.devices = [dev(1, 'U1', { coils: 1 }, { r: 12 }), dev(2, 'U2', { coils: 1 }, { r: 0 })];
    rerender(<ModbusVirtualDevicesPanel title="가상 디바이스" config={{ agentId: 'gw-1' }} />);

    expect(screen.getByTestId('modbus-virtual-status-1')).toHaveAttribute('data-status', 'active');
    expect(screen.getByTestId('modbus-virtual-status-2')).toHaveAttribute('data-status', 'stale');
  });
});
