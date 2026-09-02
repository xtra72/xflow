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

  it('areaColumns 미설정 시 셀 컨테이너는 flex-wrap(기본)', () => {
    const map: ModbusRegisterMap = { holding_registers: { '0': 1, '1': 2 } };
    render(<RegisterMapGrid registerMap={map} registerCounts={counts({ holding_registers: 2 })} />);
    const cellContainer =
      screen.getByTestId('modbus-grid-cell-holding_registers-0').parentElement!;
    expect(cellContainer.className).toContain('flex');
    expect(cellContainer.style.gridTemplateColumns).toBe('');
  });

  it('areaColumns 설정 시 해당 영역만 고정 열 CSS grid 로 렌더한다', () => {
    const map: ModbusRegisterMap = { holding_registers: { '0': 1, '1': 2 } };
    render(
      <RegisterMapGrid
        registerMap={map}
        registerCounts={counts({ holding_registers: 2 })}
        areaColumns={{ holding_registers: 4 }}
      />,
    );
    const cellContainer =
      screen.getByTestId('modbus-grid-cell-holding_registers-0').parentElement!;
    expect(cellContainer.className).toContain('grid');
    expect(cellContainer.style.gridTemplateColumns).toBe('repeat(4, minmax(0, 1fr))');
  });

  it('layoutColumns 미설정 시 외곽 컨테이너는 반응형 기본(grid-cols-1 sm:grid-cols-2)을 유지한다', () => {
    const map: ModbusRegisterMap = { holding_registers: { '0': 1, '1': 2 } };
    render(<RegisterMapGrid registerMap={map} registerCounts={counts({ holding_registers: 2 })} />);
    // 외곽 컨테이너 = 영역 카드(modbus-grid-area-*)의 부모.
    const outer = screen.getByTestId('modbus-grid-area-holding_registers').parentElement!;
    expect(outer.className).toContain('grid-cols-1');
    expect(outer.className).toContain('sm:grid-cols-2');
    expect(outer.style.gridTemplateColumns).toBe('');
  });

  it('layoutColumns 설정 시 외곽 컨테이너는 고정 열 CSS grid(영역 카드 배치)로 렌더한다', () => {
    const map: ModbusRegisterMap = {
      coils: { '0': true },
      holding_registers: { '0': 1, '1': 2 },
    };
    render(
      <RegisterMapGrid
        registerMap={map}
        registerCounts={counts({ coils: 1, holding_registers: 2 })}
        layoutColumns={2}
      />,
    );
    const outer = screen.getByTestId('modbus-grid-area-holding_registers').parentElement!;
    expect(outer.style.gridTemplateColumns).toBe('repeat(2, minmax(0, 1fr))');
    // 고정 열 사용 시 반응형 기본 클래스는 적용하지 않는다.
    expect(outer.className).not.toContain('sm:grid-cols-2');
  });

  it('layoutColumns(외곽 카드 배치)와 areaColumns(영역 내부 셀 열)는 독립적으로 적용된다', () => {
    const map: ModbusRegisterMap = { holding_registers: { '0': 1, '1': 2 } };
    render(
      <RegisterMapGrid
        registerMap={map}
        registerCounts={counts({ holding_registers: 2 })}
        areaColumns={{ holding_registers: 4 }}
        layoutColumns={1}
      />,
    );
    const outer = screen.getByTestId('modbus-grid-area-holding_registers').parentElement!;
    expect(outer.style.gridTemplateColumns).toBe('repeat(1, minmax(0, 1fr))');
    // 영역 내부 셀 그리드는 areaColumns 값(4)을 그대로 유지한다.
    const cellContainer =
      screen.getByTestId('modbus-grid-cell-holding_registers-0').parentElement!;
    expect(cellContainer.style.gridTemplateColumns).toBe('repeat(4, minmax(0, 1fr))');
  });

  it('areaColumns 는 영역별로 개별 적용된다(설정 영역=grid, 미설정 영역=flex-wrap)', () => {
    // coils 만 열 수 설정, holding_registers 는 미설정 → 각 영역 배치가 독립적으로 결정된다.
    const map: ModbusRegisterMap = {
      coils: { '0': true, '1': false, '2': true },
      holding_registers: { '0': 1, '1': 2 },
    };
    render(
      <RegisterMapGrid
        registerMap={map}
        registerCounts={counts({ coils: 3, holding_registers: 2 })}
        areaColumns={{ coils: 2 }}
      />,
    );
    const coilsContainer = screen.getByTestId('modbus-grid-cell-coils-0').parentElement!;
    expect(coilsContainer.className).toContain('grid');
    expect(coilsContainer.style.gridTemplateColumns).toBe('repeat(2, minmax(0, 1fr))');

    const holdingContainer =
      screen.getByTestId('modbus-grid-cell-holding_registers-0').parentElement!;
    expect(holdingContainer.className).toContain('flex');
    expect(holdingContainer.style.gridTemplateColumns).toBe('');
  });
});
