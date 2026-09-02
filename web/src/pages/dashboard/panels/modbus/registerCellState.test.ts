// SPEC-MODBUS-012 M4 (REQ-04-02 / REQ-05-02 / AC-10 / AC-13): 값 기반 셀 상태 판정 유틸.

import { describe, it, expect } from 'vitest';

import type { ModbusRegisterCounts, ModbusRegisterMap } from './useModbusData';
import {
  cellState,
  computeAreaGrid,
  isBoolArea,
  valueCellState,
  REGISTER_AREA_ORDER,
  formatAddr,
} from './registerCellState';

const counts = (c: Partial<ModbusRegisterCounts>): ModbusRegisterCounts => ({
  coils: c.coils ?? 0,
  discrete_inputs: c.discrete_inputs ?? 0,
  holding_registers: c.holding_registers ?? 0,
  input_registers: c.input_registers ?? 0,
});

describe('registerCellState util (SPEC-MODBUS-012 §5.2)', () => {
  it('isBoolArea: coils/discrete_inputs 만 bool 영역', () => {
    expect(isBoolArea('coils')).toBe(true);
    expect(isBoolArea('discrete_inputs')).toBe(true);
    expect(isBoolArea('holding_registers')).toBe(false);
    expect(isBoolArea('input_registers')).toBe(false);
  });

  describe('valueCellState (값 기반 규칙)', () => {
    it('bool 영역: true→active, false→inactive', () => {
      expect(valueCellState('coils', true)).toBe('active');
      expect(valueCellState('coils', false)).toBe('inactive');
      expect(valueCellState('discrete_inputs', true)).toBe('active');
    });

    it('숫자 영역: !=0→active, 0→inactive', () => {
      expect(valueCellState('holding_registers', 123)).toBe('active');
      expect(valueCellState('holding_registers', 0)).toBe('inactive');
      expect(valueCellState('input_registers', 65535)).toBe('active');
    });

    it('값 부재(undefined/null)는 degraded', () => {
      expect(valueCellState('holding_registers', undefined)).toBe('degraded');
      expect(valueCellState('coils', null)).toBe('degraded');
    });
  });

  describe('cellState (주소 기준, AC-13 공유 로직)', () => {
    const snapshot: ModbusRegisterMap = {
      holding_registers: { '0': 0, '1': 123 },
      coils: { '0': true },
    };
    it('스냅샷에 있는 주소는 값 기반 판정', () => {
      expect(cellState('holding_registers', 0, snapshot, counts({ holding_registers: 3 }))).toBe(
        'inactive',
      );
      expect(cellState('holding_registers', 1, snapshot, counts({ holding_registers: 3 }))).toBe(
        'active',
      );
      expect(cellState('coils', 0, snapshot, counts({ coils: 1 }))).toBe('active');
    });

    it('정의(count>0)되었으나 스냅샷 부재 주소는 degraded (AC-10)', () => {
      expect(cellState('holding_registers', 2, snapshot, counts({ holding_registers: 3 }))).toBe(
        'degraded',
      );
    });

    it('정의되지 않은(count=0) 부재 주소는 inactive', () => {
      expect(cellState('input_registers', 5, snapshot, counts({}))).toBe('inactive');
    });
  });

  describe('computeAreaGrid (그리드 렌더 모델 + 집계, AC-11)', () => {
    it('POINTS/ACTIVE/DEGRADED 집계 (AC-10/AC-11 시나리오)', () => {
      // holding: 값 0(inactive), 123(active), count=3 이므로 1개 부재 → degraded 1.
      const grid = computeAreaGrid('holding_registers', { '0': 0, '1': 123 }, 3);
      expect(grid.points).toBe(3);
      expect(grid.active).toBe(1);
      expect(grid.inactive).toBe(1);
      expect(grid.degraded).toBe(1);
      // 렌더 셀 = present 2 + degraded 1.
      expect(grid.cells).toHaveLength(3);
      expect(grid.cells.filter((c) => c.state === 'degraded')).toHaveLength(1);
      expect(grid.cells.find((c) => c.state === 'degraded')?.addr).toBeNull();
      expect(grid.addrMin).toBe(0);
      expect(grid.addrMax).toBe(1);
    });

    it('bool 영역 그리드', () => {
      const grid = computeAreaGrid('coils', { '0': true, '1': false, '2': true }, 3);
      expect(grid.active).toBe(2);
      expect(grid.inactive).toBe(1);
      expect(grid.degraded).toBe(0);
      expect(grid.cells).toHaveLength(3);
    });

    it('스냅샷 부재(빈 영역)면 POINTS 만큼 전부 degraded', () => {
      const grid = computeAreaGrid('input_registers', undefined, 2);
      expect(grid.degraded).toBe(2);
      expect(grid.active).toBe(0);
      expect(grid.cells.every((c) => c.state === 'degraded')).toBe(true);
      expect(grid.addrMin).toBeNull();
    });

    it('주소 정렬(숫자 오름차순)', () => {
      const grid = computeAreaGrid('holding_registers', { '10': 1, '2': 1, '100': 1 }, 3);
      expect(grid.cells.map((c) => c.addr)).toEqual([2, 10, 100]);
    });
  });

  it('REGISTER_AREA_ORDER 는 4영역', () => {
    expect(REGISTER_AREA_ORDER).toEqual([
      'coils',
      'discrete_inputs',
      'input_registers',
      'holding_registers',
    ]);
  });

  it('formatAddr: 4자리 hex', () => {
    expect(formatAddr(0)).toBe('0x0000');
    expect(formatAddr(255)).toBe('0x00FF');
    expect(formatAddr(597)).toBe('0x0255');
  });
});
