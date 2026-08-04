// SPEC-MODBUS-012 M4 (REQ-04/05, AC-09/10/11/13): 레지스터 맵 그리드 공유 컴포넌트.

import { describe, it, expect, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';

vi.mock('@/lib/i18n', () => ({ useTranslation: () => ({ t: (k: string) => k }) }));

import RegisterMapGrid from './RegisterMapGrid';
import { isRegisterMapEmpty } from './registerCellState';
import type { ModbusRegisterCounts, ModbusRegisterMap } from './useModbusData';

const counts = (c: Partial<ModbusRegisterCounts>): ModbusRegisterCounts => ({
  coils: c.coils ?? 0,
  discrete_inputs: c.discrete_inputs ?? 0,
  holding_registers: c.holding_registers ?? 0,
  input_registers: c.input_registers ?? 0,
});

describe('RegisterMapGrid (SPEC-MODBUS-012 REQ-04/05)', () => {
  it('AC-09: 4영역이 스냅샷 값으로 렌더된다', () => {
    const map: ModbusRegisterMap = {
      coils: { '0': true, '1': false },
      discrete_inputs: { '0': true },
      input_registers: { '0': 10 },
      holding_registers: { '0': 0, '1': 123 },
    };
    render(
      <RegisterMapGrid
        registerMap={map}
        registerCounts={counts({
          coils: 2,
          discrete_inputs: 1,
          input_registers: 1,
          holding_registers: 2,
        })}
      />,
    );
    expect(screen.getByTestId('modbus-grid-area-coils')).toBeInTheDocument();
    expect(screen.getByTestId('modbus-grid-area-discrete_inputs')).toBeInTheDocument();
    expect(screen.getByTestId('modbus-grid-area-input_registers')).toBeInTheDocument();
    expect(screen.getByTestId('modbus-grid-area-holding_registers')).toBeInTheDocument();
  });

  it('AC-10: 값 기반 셀 색상(active/inactive/degraded)', () => {
    // holding: 0(inactive), 123(active), count=3 → 1개 부재(degraded).
    const map: ModbusRegisterMap = { holding_registers: { '0': 0, '1': 123 } };
    render(<RegisterMapGrid registerMap={map} registerCounts={counts({ holding_registers: 3 })} />);
    const area = screen.getByTestId('modbus-grid-area-holding_registers');
    const cells = within(area).getAllByTestId(/modbus-grid-cell-holding_registers-/);
    const states = cells.map((c) => c.getAttribute('data-state'));
    expect(states).toContain('inactive');
    expect(states).toContain('active');
    expect(states).toContain('degraded');
  });

  it('AC-11: 영역별 POINTS/ACTIVE/DEGRADED 집계가 헤더에 표시된다', () => {
    const map: ModbusRegisterMap = { holding_registers: { '0': 0, '1': 123 } };
    render(<RegisterMapGrid registerMap={map} registerCounts={counts({ holding_registers: 3 })} />);
    const points = screen.getByTestId('modbus-grid-points-holding_registers');
    const active = screen.getByTestId('modbus-grid-active-holding_registers');
    const degraded = screen.getByTestId('modbus-grid-degraded-holding_registers');
    expect(points).toHaveTextContent('3');
    expect(active).toHaveTextContent('1');
    expect(degraded).toHaveTextContent('1');
  });

  it('빈 영역(정의 0 + 스냅샷 0)은 렌더하지 않는다', () => {
    const map: ModbusRegisterMap = { holding_registers: { '0': 1 } };
    render(<RegisterMapGrid registerMap={map} registerCounts={counts({ holding_registers: 1 })} />);
    expect(screen.getByTestId('modbus-grid-area-holding_registers')).toBeInTheDocument();
    expect(screen.queryByTestId('modbus-grid-area-coils')).toBeNull();
  });

  it('isRegisterMapEmpty: 전 영역 공백이면 true', () => {
    expect(isRegisterMapEmpty({}, counts({}))).toBe(true);
    expect(isRegisterMapEmpty({ coils: { '0': false } }, counts({ coils: 1 }))).toBe(false);
  });
});
