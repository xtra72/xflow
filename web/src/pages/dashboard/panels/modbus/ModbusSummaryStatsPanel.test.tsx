// SPEC-MODBUS-012 M5 (REQ-06-03, AC-17 + AC-03 graceful): 종합 통계 패널.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, within } from '@testing-library/react';

import type { ModbusClient, ModbusDeviceListItem, ModbusStatus } from './useModbusData';

const mockState = vi.hoisted(() => ({
  gate: { agentId: 'gw-1', remote: false, bound: true, enabled: true },
  status: undefined as ModbusStatus | undefined,
  devices: [] as ModbusDeviceListItem[],
  clients: [] as ModbusClient[],
}));

vi.mock('./useModbusData', async () => {
  const actual = await vi.importActual<typeof import('./useModbusData')>('./useModbusData');
  return {
    ...actual,
    useModbusGate: () => mockState.gate,
    useModbusStatus: () => ({ status: mockState.status, isLoading: false, isError: false }),
    useModbusListDevices: () => ({ devices: mockState.devices, isLoading: false, isError: false }),
    useModbusListClients: () => ({ clients: mockState.clients, isLoading: false, isError: false }),
  };
});

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (sel: (s: { dashboardRefreshInterval: number }) => unknown) =>
    sel({ dashboardRefreshInterval: 5 }),
}));

import ModbusSummaryStatsPanel from './ModbusSummaryStatsPanel';
import { formatUptime } from './useModbusData';

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

const st = (active: number, uptime: number): ModbusStatus => ({
  listen_address: '0.0.0.0',
  listen_port: 502,
  unit_id: 1,
  active_connections: active,
  max_connections: 10,
  uptime_seconds: uptime,
});

const client = (addr: string): ModbusClient => ({
  remote_addr: addr,
  connected_at: '',
  unit_ids: [1],
  request_count: 0,
  last_seen: '',
});

function renderPanel() {
  return render(<ModbusSummaryStatsPanel title="종합 통계" config={{ agentId: 'gw-1' }} />);
}

describe('ModbusSummaryStatsPanel (SPEC-MODBUS-012 REQ-06)', () => {
  beforeEach(() => {
    mockState.gate = { agentId: 'gw-1', remote: false, bound: true, enabled: true };
    mockState.status = undefined;
    mockState.devices = [];
    mockState.clients = [];
  });

  it('formatUptime: 초→사람이 읽는 형식', () => {
    expect(formatUptime(45)).toBe('45s');
    expect(formatUptime(125)).toBe('2m 5s');
    expect(formatUptime(3661)).toBe('1h 1m');
  });

  it('AC-03: agentId 미설정 시 안내', () => {
    mockState.gate = { agentId: '', remote: false, bound: false, enabled: false };
    renderPanel();
    expect(screen.getByText('dashboard.modbus.notConfigured')).toBeInTheDocument();
  });

  it('AC-17: active_connections/clients/uptime/총 디바이스/총 reads·writes·errors 표시', () => {
    mockState.status = st(3, 3661);
    mockState.devices = [dev(1, 10, 2, 1), dev(2, 20, 3, 0), dev(3, 5, 0, 2), dev(4, 0, 0, 0)];
    mockState.clients = [client('a'), client('b')];
    renderPanel();

    expect(within(screen.getByTestId('modbus-summary-connections')).getByText('3')).toBeInTheDocument();
    expect(within(screen.getByTestId('modbus-summary-clients')).getByText('2')).toBeInTheDocument();
    expect(within(screen.getByTestId('modbus-summary-devices')).getByText('4')).toBeInTheDocument();
    // 합계: reads=35, writes=5, errors=3.
    expect(within(screen.getByTestId('modbus-summary-reads')).getByText('35')).toBeInTheDocument();
    expect(within(screen.getByTestId('modbus-summary-writes')).getByText('5')).toBeInTheDocument();
    expect(screen.getByTestId('modbus-summary-errors')).toHaveTextContent('3');
    expect(screen.getByTestId('modbus-summary-uptime')).toHaveTextContent('1h 1m');
    // poll cycle = 5s(uiStore mock).
    expect(screen.getByTestId('modbus-summary-pollCycle')).toHaveTextContent('5s');
  });

  it('타일 컨테이너가 반응형 auto-fit 그리드로 패널 폭에 맞춰 리플로우한다', () => {
    mockState.status = st(1, 10);
    mockState.devices = [dev(1, 0, 0, 0)];
    renderPanel();
    const bar = screen.getByTestId('modbus-summary-bar');
    // 고정폭 flex-wrap 이 아닌 grid + auto-fit minmax(1fr) 여야 타일이 균등 신축·리플로우한다.
    expect(bar.className).toContain('grid');
    expect(bar.style.gridTemplateColumns).toBe('repeat(auto-fit, minmax(92px, 1fr))');
  });
});
