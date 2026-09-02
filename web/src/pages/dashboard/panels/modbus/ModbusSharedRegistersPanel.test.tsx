// SPEC-MODBUS-012 M4 (REQ-04, AC-09/11b + AC-03 graceful): 공유 레지스터 맵 패널.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';

import type { ModbusRegisterMap } from './useModbusData';

const mockState = vi.hoisted(() => ({
  gate: { agentId: 'gw-1', remote: false, bound: true, enabled: true },
  registerMap: undefined as ModbusRegisterMap | undefined,
  isLoading: false,
  isError: false,
}));

vi.mock('./useModbusData', async () => {
  const actual = await vi.importActual<typeof import('./useModbusData')>('./useModbusData');
  return {
    ...actual,
    useModbusGate: () => mockState.gate,
    useModbusRegisterMap: () => ({
      registerMap: mockState.registerMap,
      isLoading: mockState.isLoading,
      isError: mockState.isError,
    }),
  };
});

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import ModbusSharedRegistersPanel from './ModbusSharedRegistersPanel';

function renderPanel() {
  return render(<ModbusSharedRegistersPanel title="공유 레지스터" config={{ agentId: 'gw-1' }} />);
}

describe('ModbusSharedRegistersPanel (SPEC-MODBUS-012 REQ-04)', () => {
  beforeEach(() => {
    mockState.gate = { agentId: 'gw-1', remote: false, bound: true, enabled: true };
    mockState.registerMap = undefined;
    mockState.isLoading = false;
    mockState.isError = false;
  });

  it('AC-03: agentId 미설정 시 안내', () => {
    mockState.gate = { agentId: '', remote: false, bound: false, enabled: false };
    renderPanel();
    expect(screen.getByText('dashboard.modbus.notConfigured')).toBeInTheDocument();
  });

  it('AC-11b: 공유 컨테이너 미구성(에러/빈 스냅샷) 시 안내, 크래시 없음', () => {
    mockState.isError = true;
    renderPanel();
    expect(screen.getByText('dashboard.modbus.noSharedContainer')).toBeInTheDocument();
  });

  it('AC-11b: 빈 스냅샷도 안내 상태', () => {
    mockState.registerMap = {};
    renderPanel();
    expect(screen.getByText('dashboard.modbus.noSharedContainer')).toBeInTheDocument();
  });

  it('AC-09: unit 0 스냅샷으로 그리드를 렌더한다', () => {
    mockState.registerMap = {
      holding_registers: { '0': 123, '1': 0 },
      coils: { '0': true },
    };
    renderPanel();
    expect(screen.getByTestId('modbus-grid-area-holding_registers')).toBeInTheDocument();
    expect(screen.getByTestId('modbus-grid-area-coils')).toBeInTheDocument();
    // get_map 은 register_counts 부재 → 스냅샷 present 개수로 유도(degraded 0).
    expect(screen.getByTestId('modbus-grid-degraded-holding_registers')).toHaveTextContent('0');
  });
});
