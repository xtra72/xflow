// SPEC-MODBUS-012 M4 (REQ-05, AC-12/13 + AC-03 graceful): 가상 디바이스 레지스터 맵 패널.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';

import type { ModbusDeviceStatus } from './useModbusData';

const mockState = vi.hoisted(() => ({
  gate: { agentId: 'gw-1', remote: false, bound: true, enabled: true },
  status: undefined as ModbusDeviceStatus | undefined,
  isLoading: false,
  isError: false,
  lastUnitId: -1,
}));

vi.mock('./useModbusData', async () => {
  const actual = await vi.importActual<typeof import('./useModbusData')>('./useModbusData');
  return {
    ...actual,
    useModbusGate: () => mockState.gate,
    useModbusDeviceStatus: (_agentId: string, unitId: number) => {
      mockState.lastUnitId = unitId;
      return {
        status: mockState.status,
        isLoading: mockState.isLoading,
        isError: mockState.isError,
      };
    },
  };
});

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import ModbusDeviceRegistersPanel from './ModbusDeviceRegistersPanel';

function renderPanel(config: Record<string, unknown> = { agentId: 'gw-1', unitId: 2 }) {
  return render(<ModbusDeviceRegistersPanel title="가상 레지스터" config={config} />);
}

function status(unitId: number): ModbusDeviceStatus {
  return {
    unit_id: unitId,
    name: `U${unitId}`,
    register_counts: { coils: 0, discrete_inputs: 0, holding_registers: 3, input_registers: 0 },
    register_map: { holding_registers: { '0': 0, '1': 123 } },
    stats: { read_count: 0, write_count: 0, error_count: 0 },
    backing: null,
  };
}

describe('ModbusDeviceRegistersPanel (SPEC-MODBUS-012 REQ-05)', () => {
  beforeEach(() => {
    mockState.gate = { agentId: 'gw-1', remote: false, bound: true, enabled: true };
    mockState.status = undefined;
    mockState.isLoading = false;
    mockState.isError = false;
    mockState.lastUnitId = -1;
  });

  it('AC-03: agentId 미설정 시 안내', () => {
    mockState.gate = { agentId: '', remote: false, bound: false, enabled: false };
    renderPanel();
    expect(screen.getByText('dashboard.modbus.notConfigured')).toBeInTheDocument();
  });

  it('unitId=0(공유 컨테이너) 시 유닛 미선택 안내를 표시한다', () => {
    renderPanel({ agentId: 'gw-1', unitId: 0 });
    expect(screen.getByText('dashboard.modbus.unitNotSelected')).toBeInTheDocument();
  });

  it('unitId 누락 시 유닛 미선택 안내를 표시한다', () => {
    renderPanel({ agentId: 'gw-1' });
    expect(screen.getByText('dashboard.modbus.unitNotSelected')).toBeInTheDocument();
  });

  it('AC-12: config.unitId 로 get_device_status 를 조회한다', () => {
    mockState.status = status(2);
    renderPanel({ agentId: 'gw-1', unitId: 2 });
    expect(mockState.lastUnitId).toBe(2);
  });

  it('AC-12/13: 레지스터 맵을 4영역 그리드(공유 컴포넌트)로 렌더한다', () => {
    mockState.status = status(2);
    renderPanel({ agentId: 'gw-1', unitId: 2 });
    expect(screen.getByTestId('modbus-grid-area-holding_registers')).toBeInTheDocument();
    // register_counts=3, 스냅샷 2 → degraded 1(공유 로직 재사용, AC-13).
    expect(screen.getByTestId('modbus-grid-degraded-holding_registers')).toHaveTextContent('1');
    expect(screen.getByTestId('modbus-grid-active-holding_registers')).toHaveTextContent('1');
  });

  it('조회 에러 시 안내', () => {
    mockState.isError = true;
    renderPanel();
    expect(screen.getByText('dashboard.modbus.loadError')).toBeInTheDocument();
  });

  it('유닛 선택됨 + 레지스터 영역 없음 → 세그먼트 추가 안내(제네릭 empty 아님)', () => {
    mockState.status = {
      unit_id: 2,
      name: 'U2',
      register_counts: { coils: 0, discrete_inputs: 0, holding_registers: 0, input_registers: 0 },
      register_map: {},
      stats: { read_count: 0, write_count: 0, error_count: 0 },
      backing: null,
    };
    renderPanel({ agentId: 'gw-1', unitId: 2 });
    expect(screen.getByText('dashboard.modbus.deviceNoSegments')).toBeInTheDocument();
    expect(screen.queryByText('dashboard.modbus.empty')).toBeNull();
  });

  it('config.areaColumns 를 RegisterMapGrid areaColumns 로 전달한다(영역별 고정 열 그리드)', () => {
    mockState.status = status(2);
    renderPanel({ agentId: 'gw-1', unitId: 2, areaColumns: { holding_registers: 4 } });
    const cellContainer =
      screen.getByTestId('modbus-grid-cell-holding_registers-0').parentElement!;
    expect(cellContainer.className).toContain('grid');
    expect(cellContainer.style.gridTemplateColumns).toBe('repeat(4, minmax(0, 1fr))');
  });

  it('마이그레이션: 구 단일 config.columns 는 전 영역에 시드되어 적용된다', () => {
    mockState.status = status(2);
    // areaColumns 없이 구 columns 만 있는 기존 패널 → holding_registers 영역도 고정 열 그리드.
    renderPanel({ agentId: 'gw-1', unitId: 2, columns: 5 });
    const cellContainer =
      screen.getByTestId('modbus-grid-cell-holding_registers-0').parentElement!;
    expect(cellContainer.className).toContain('grid');
    expect(cellContainer.style.gridTemplateColumns).toBe('repeat(5, minmax(0, 1fr))');
  });
});
