// SPEC-MODBUS-012 M2 (REQ-02, AC-04/05/06): 실제 연결 디바이스 목록 패널.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';

const mockState = vi.hoisted(() => ({
  gate: { agentId: 'gw-1', remote: false, bound: true, enabled: true },
  devices: [] as Array<Record<string, unknown>>,
  isLoading: false,
  isError: false,
  statusByUnit: {} as Record<number, unknown>,
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
    useModbusDeviceStatus: (_agentId: string, unitId: number) => ({
      status: mockState.statusByUnit[unitId],
      isLoading: false,
      isError: false,
    }),
  };
});

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import ModbusRealDevicesPanel from './ModbusRealDevicesPanel';

function listItem(unit_id: number, name: string, backed: boolean, mode: string) {
  return {
    unit_id,
    name,
    register_counts: { coils: 0, discrete_inputs: 0, holding_registers: 0, input_registers: 0 },
    status: 'active',
    backed,
    mode,
    stats: { read_count: 0, write_count: 0, error_count: 0 },
  };
}

function backing(mode: string, connected: boolean, req: number, err: number, latMs: number, lastOk: number) {
  return {
    unit_id: 0,
    name: '',
    register_counts: { coils: 0, discrete_inputs: 0, holding_registers: 0, input_registers: 0 },
    register_map: {},
    stats: { read_count: 0, write_count: 0, error_count: 0 },
    backing: {
      mode,
      connected,
      request_count: req,
      error_count: err,
      avg_latency_ms: latMs,
      last_ok: lastOk,
    },
  };
}

function renderPanel() {
  return render(<ModbusRealDevicesPanel title="실제 디바이스" config={{ agentId: 'gw-1' }} />);
}

describe('ModbusRealDevicesPanel (SPEC-MODBUS-012 REQ-02)', () => {
  beforeEach(() => {
    mockState.gate = { agentId: 'gw-1', remote: false, bound: true, enabled: true };
    mockState.devices = [];
    mockState.isLoading = false;
    mockState.isError = false;
    mockState.statusByUnit = {};
  });

  it('AC-03: 미설정 시 안내 문구를 표시한다', () => {
    mockState.gate = { agentId: '', remote: false, bound: false, enabled: false };
    renderPanel();
    expect(screen.getByText('dashboard.modbus.notConfigured')).toBeInTheDocument();
  });

  it('AC-04: backed=true 항목만 표시하고 순수 slave 는 제외한다', () => {
    mockState.devices = [
      listItem(1, '실제-direct', true, 'direct'),
      listItem(2, '실제-indirect', true, 'indirect'),
      listItem(3, '순수-slave', false, ''),
    ];
    mockState.statusByUnit = {
      1: backing('direct', true, 10, 0, 3.5, 111),
      2: backing('indirect', true, 4, 1, 8.2, 222),
    };
    renderPanel();

    expect(screen.getByTestId('modbus-real-device-1')).toBeInTheDocument();
    expect(screen.getByTestId('modbus-real-device-2')).toBeInTheDocument();
    // 순수 slave(unit 3)는 렌더되지 않는다.
    expect(screen.queryByTestId('modbus-real-device-3')).toBeNull();
    expect(screen.queryByText('순수-slave')).toBeNull();
  });

  it('AC-05: mode/slave id/요청 수/에러율/레이턴시가 표시된다', () => {
    mockState.devices = [listItem(5, '실제-direct', true, 'direct')];
    mockState.statusByUnit = { 5: backing('direct', true, 10, 2, 4.0, 111) };
    renderPanel();

    // mode 배지
    expect(screen.getByText('direct')).toBeInTheDocument();
    // slave id = unit_id
    expect(screen.getByText(/dashboard\.modbus\.slaveId\s*5/)).toBeInTheDocument();
    // 레이턴시/요청/에러율 셀
    expect(screen.getByTestId('modbus-real-latency-5')).toHaveTextContent('4.0ms');
    expect(screen.getByTestId('modbus-real-requests-5')).toHaveTextContent('10');
    // 에러율 = 2/10 = 20%
    expect(screen.getByTestId('modbus-real-errrate-5')).toHaveTextContent('20%');
  });

  it('AC-06: connected=false 디바이스는 degraded 로 구분된다', () => {
    mockState.devices = [
      listItem(1, '정상', true, 'direct'),
      listItem(2, 'stale', true, 'indirect'),
    ];
    mockState.statusByUnit = {
      1: backing('direct', true, 10, 0, 3.0, 111),
      2: backing('indirect', false, 4, 0, 9.0, 0),
    };
    renderPanel();

    expect(screen.getByTestId('modbus-real-device-1')).toHaveAttribute('data-degraded', 'false');
    expect(screen.getByTestId('modbus-real-device-2')).toHaveAttribute('data-degraded', 'true');
    expect(screen.getByTestId('modbus-real-status-2')).toHaveAttribute('data-status', 'degraded');
    expect(screen.getByTestId('modbus-real-status-1')).toHaveAttribute('data-status', 'online');
  });

  it('백킹 디바이스가 없으면 안내 문구를 표시한다', () => {
    mockState.devices = [listItem(3, '순수-slave', false, '')];
    renderPanel();
    expect(screen.getByText('dashboard.modbus.noBackedDevices')).toBeInTheDocument();
  });
});
