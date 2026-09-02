// SPEC-MODBUS-012 M5 (REQ-06-04, AC-18 + AC-03 graceful): 버스 통계 패널(미니 차트).

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';

import type { ModbusDeviceListItem, ModbusStatus } from './useModbusData';

const mockState = vi.hoisted(() => ({
  gate: { agentId: 'gw-1', remote: false, bound: true, enabled: true },
  status: { active_connections: 0 } as unknown as ModbusStatus,
  devices: [] as ModbusDeviceListItem[],
  isLoading: false,
}));

vi.mock('./useModbusData', async () => {
  const actual = await vi.importActual<typeof import('./useModbusData')>('./useModbusData');
  return {
    ...actual,
    useModbusGate: () => mockState.gate,
    useModbusStatus: () => ({ status: mockState.status, isLoading: false, isError: false }),
    useModbusListDevices: () => ({
      devices: mockState.devices,
      isLoading: mockState.isLoading,
      isError: false,
    }),
  };
});

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import ModbusBusStatsPanel from './ModbusBusStatsPanel';

function dev(unit_id: number, r: number, w: number, e: number): ModbusDeviceListItem {
  return {
    unit_id,
    name: `U${unit_id}`,
    register_counts: { coils: 0, discrete_inputs: 0, holding_registers: 0, input_registers: 0 },
    status: 'active',
    backed: false,
    mode: '',
    stats: { read_count: r, write_count: w, error_count: e },
  };
}

function renderPanel(config: Record<string, unknown> = { agentId: 'gw-1' }) {
  return render(<ModbusBusStatsPanel title="버스 통계" config={config} />);
}

describe('ModbusBusStatsPanel (SPEC-MODBUS-012 REQ-06)', () => {
  beforeEach(() => {
    mockState.gate = { agentId: 'gw-1', remote: false, bound: true, enabled: true };
    mockState.status = { active_connections: 0 } as unknown as ModbusStatus;
    mockState.devices = [];
    mockState.isLoading = false;
  });

  it('AC-03: agentId 미설정 시 안내', () => {
    mockState.gate = { agentId: '', remote: false, bound: false, enabled: false };
    renderPanel();
    expect(screen.getByText('dashboard.modbus.notConfigured')).toBeInTheDocument();
  });

  it('AC-18: 4종 미니차트(reads/writes/errors/connections)를 렌더한다', () => {
    mockState.devices = [dev(1, 0, 0, 0)];
    renderPanel();
    expect(screen.getByTestId('modbus-bus-chart-reads')).toBeInTheDocument();
    expect(screen.getByTestId('modbus-bus-chart-writes')).toBeInTheDocument();
    expect(screen.getByTestId('modbus-bus-chart-errors')).toBeInTheDocument();
    expect(screen.getByTestId('modbus-bus-chart-connections')).toBeInTheDocument();
  });

  it('AC-18: 연속 폴 사이 read 델타를 per-min rate 로 누적한다', () => {
    // 1차 폴: 기준선(포인트 미생성 → reads 값 0).
    mockState.devices = [dev(1, 0, 0, 0)];
    const { rerender } = renderPanel();
    expect(screen.getByTestId('modbus-bus-value-reads')).toHaveTextContent('0');

    // 2차 폴: 총 read_count 60 증가 → reads/min 델타 누적(>0).
    mockState.devices = [dev(1, 60, 0, 0)];
    rerender(<ModbusBusStatsPanel title="버스 통계" config={{ agentId: 'gw-1' }} />);
    const readsVal = screen.getByTestId('modbus-bus-value-reads').textContent ?? '0';
    expect(Number(readsVal)).toBeGreaterThan(0);
  });

  it('columns 미설정 시 기본 2열(grid-cols-2)', () => {
    mockState.devices = [dev(1, 0, 0, 0)];
    renderPanel();
    const grid = screen.getByTestId('modbus-bus-grid');
    expect(grid.className).toContain('grid-cols-2');
    expect(grid.style.gridTemplateColumns).toBe('');
  });

  it('config.columns 설정 시 미니차트 그리드를 행당 고정 열로 배치한다', () => {
    mockState.devices = [dev(1, 0, 0, 0)];
    renderPanel({ agentId: 'gw-1', columns: 4 });
    const grid = screen.getByTestId('modbus-bus-grid');
    expect(grid.className).not.toContain('grid-cols-2');
    expect(grid.style.gridTemplateColumns).toBe('repeat(4, minmax(0, 1fr))');
  });
});
