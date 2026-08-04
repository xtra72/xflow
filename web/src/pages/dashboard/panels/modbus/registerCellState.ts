// SPEC-MODBUS-012 M4 (REQ-04-02 / REQ-05-02, §5.2): 레지스터 셀 상태 값 기반 판정 유틸.
//
// 레지스터 스냅샷(백엔드 미변경)의 "값"만으로 프론트에서 셀 상태를 판정한다.
// 공유 레지스터 맵(get_map unit 0)과 가상 디바이스 레지스터 맵(get_device_status unit N)이
// 동일 규칙을 재사용한다(AC-13, 중복 구현 없음).
//
// 판정 규칙(§5.2):
//   - bool 영역(coils / discrete_inputs): true → active, false → inactive
//   - 숫자 영역(holding_registers / input_registers): 값 != 0 → active, 0 → inactive
//   - degraded: 정의(register_counts>0)되었으나 스냅샷에 주소가 부재한 경우(조회 누락/에러).
//     register_counts 는 개수만 제공하므로, 영역별 degraded 수 = max(0, POINTS − 스냅샷 항목 수)로 집계한다.

import type { ModbusRegisterCounts, ModbusRegisterMap } from './useModbusData';

/** 셀 상태 3종. */
export type CellState = 'active' | 'inactive' | 'degraded';

/** bool 영역(ON/OFF) 여부. 나머지는 숫자(uint16) 영역이다. */
const BOOL_AREAS: ReadonlySet<string> = new Set(['coils', 'discrete_inputs']);

/** 영역 렌더 순서(레지스터 맵 그리드 공통, AgentDetailPanel 순서 재사용). */
export const REGISTER_AREA_ORDER = [
  'coils',
  'discrete_inputs',
  'input_registers',
  'holding_registers',
] as const;

export type RegisterArea = (typeof REGISTER_AREA_ORDER)[number];

/** 영역 라벨(기능 코드 포함, AgentDetailPanel REGISTER_AREA_LABELS 재사용). */
export const REGISTER_AREA_LABELS: Record<RegisterArea, string> = {
  coils: 'Coils (FC01/05/15)',
  discrete_inputs: 'Discrete Inputs (FC02)',
  input_registers: 'Input Registers (FC04)',
  holding_registers: 'Holding Registers (FC03/06/16)',
};

/** 그리드 렌더 안전 상한(영역당). register_counts 는 통상 작지만 폭주 방지용. */
export const MAX_RENDER_CELLS = 1024;

/** bool 영역인지 판정한다. */
export function isBoolArea(area: string): boolean {
  return BOOL_AREAS.has(area);
}

/**
 * 단일 값의 셀 상태를 값 기반으로 판정한다(present 주소 전용).
 * 값이 부재(undefined/null)면 degraded(조회 누락/에러)로 본다.
 */
export function valueCellState(area: string, value: boolean | number | undefined | null): CellState {
  if (value === undefined || value === null) return 'degraded';
  if (isBoolArea(area)) return value ? 'active' : 'inactive';
  return value !== 0 ? 'active' : 'inactive';
}

/**
 * 주소 기준 셀 상태 판정(design.md 시그니처, AC-13 공유 로직).
 * 스냅샷에 주소가 있으면 값 기반, 없으면 정의 여부(counts>0)에 따라 degraded/inactive.
 */
export function cellState(
  area: string,
  addr: number,
  snapshot: ModbusRegisterMap,
  counts: ModbusRegisterCounts,
): CellState {
  const areaMap = snapshot[area as keyof ModbusRegisterMap] as
    | Record<string, boolean | number>
    | undefined;
  const raw = areaMap ? areaMap[String(addr)] : undefined;
  if (raw !== undefined) return valueCellState(area, raw);
  const points = (counts[area as keyof ModbusRegisterCounts] as number | undefined) ?? 0;
  return points > 0 ? 'degraded' : 'inactive';
}

/** 렌더용 개별 셀. degraded 플레이스홀더는 addr=null(부재 주소)이다. */
export interface AreaCell {
  addr: number | null;
  value: boolean | number | null;
  state: CellState;
}

/** 영역 그리드 렌더 모델 + 헤더 집계. */
export interface AreaGrid {
  area: string;
  /** POINTS = 정의된 레지스터 개수(register_counts). */
  points: number;
  /** ACTIVE 셀 수(값 기반). */
  active: number;
  /** inactive 셀 수. */
  inactive: number;
  /** DEGRADED 셀 수(정의되었으나 스냅샷 부재). */
  degraded: number;
  /** 렌더할 셀 목록(present 값 셀 + degraded 플레이스홀더). */
  cells: AreaCell[];
  /** present 주소 최소값(없으면 null). */
  addrMin: number | null;
  /** present 주소 최대값(없으면 null). */
  addrMax: number | null;
}

/**
 * 영역 스냅샷 + POINTS 로 그리드 렌더 모델을 계산한다.
 * present 주소는 값 기반 셀로, 부족분(POINTS − present)은 degraded 플레이스홀더로 채운다.
 */
export function computeAreaGrid(
  area: string,
  snapshot: Record<string, boolean | number> | undefined,
  points: number,
): AreaGrid {
  const entries = snapshot
    ? Object.entries(snapshot).sort(([a], [b]) => Number(a) - Number(b))
    : [];

  const cells: AreaCell[] = [];
  let active = 0;
  let inactive = 0;
  let addrMin: number | null = null;
  let addrMax: number | null = null;

  for (const [addrStr, value] of entries) {
    const addr = Number(addrStr);
    const state = valueCellState(area, value);
    if (state === 'active') active += 1;
    else inactive += 1;
    if (addrMin === null || addr < addrMin) addrMin = addr;
    if (addrMax === null || addr > addrMax) addrMax = addr;
    if (cells.length < MAX_RENDER_CELLS) {
      cells.push({ addr, value, state });
    }
  }

  // degraded = 정의 개수 대비 스냅샷 부족분(조회 누락/에러로 관측되지 않은 정의 레지스터).
  const degraded = Math.max(0, points - entries.length);
  for (let i = 0; i < degraded && cells.length < MAX_RENDER_CELLS; i += 1) {
    cells.push({ addr: null, value: null, state: 'degraded' });
  }

  return { area, points, active, inactive, degraded, cells, addrMin, addrMax };
}

/** 4자리 hex 주소 표기(0x0000). */
export function formatAddr(addr: number): string {
  return `0x${addr.toString(16).toUpperCase().padStart(4, '0')}`;
}

/** 4영역 모두 비었는지(정의 0 + 스냅샷 0) 판정 — 패널 빈 상태 안내용. */
export function isRegisterMapEmpty(
  registerMap: ModbusRegisterMap,
  registerCounts: ModbusRegisterCounts,
): boolean {
  return REGISTER_AREA_ORDER.every((area) => {
    const snapshot = registerMap[area] as Record<string, boolean | number> | undefined;
    const points = registerCounts[area] ?? 0;
    return points === 0 && (!snapshot || Object.keys(snapshot).length === 0);
  });
}

/**
 * 스냅샷 항목 수로 register_counts 를 유도한다.
 * get_map(공유 컨테이너 unit 0)은 register_counts 를 반환하지 않으므로(스냅샷만),
 * 정의 개수를 스냅샷 present 개수로 간주한다(→ 공유 맵은 degraded 0).
 */
export function countsFromSnapshot(snapshot: ModbusRegisterMap): ModbusRegisterCounts {
  const size = (m: Record<string, unknown> | undefined): number => (m ? Object.keys(m).length : 0);
  return {
    coils: size(snapshot.coils),
    discrete_inputs: size(snapshot.discrete_inputs),
    holding_registers: size(snapshot.holding_registers),
    input_registers: size(snapshot.input_registers),
  };
}
