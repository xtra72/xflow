// SPEC-MODBUS-012 M4 (REQ-04 / REQ-05, AC-09~13): 레지스터 맵 그리드 공유 컴포넌트.
//
// 4영역(Coils / Discrete Inputs / Input Registers / Holding Registers)을 컴팩트한 색상 셀
// 그리드로 렌더한다. 각 영역 헤더에 POINTS / ACTIVE / DEGRADED 집계와 주소 범위를 표시한다.
// 셀 색상은 값 기반 판정(registerCellState)을 재사용한다. 공유(get_map unit 0)와
// 가상 디바이스(get_device_status unit N) 맵이 동일 컴포넌트를 사용한다(AC-13, 중복 없음).

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import {
  computeAreaGrid,
  formatAddr,
  REGISTER_AREA_LABELS,
  REGISTER_AREA_ORDER,
  type AreaGrid,
  type CellState,
  type RegisterArea,
} from './registerCellState';
import type { ModbusRegisterCounts, ModbusRegisterMap } from './useModbusData';

interface RegisterMapGridProps {
  registerMap: ModbusRegisterMap;
  registerCounts: ModbusRegisterCounts;
}

/** 셀 상태별 색상 토큰(값 기반). */
const CELL_CLASS: Record<CellState, string> = {
  active: 'bg-green-500 dark:bg-green-400',
  inactive: 'bg-(--color-bg-elevated) border border-(--color-border-default)',
  degraded: 'bg-amber-400 dark:bg-amber-500',
};

/** 셀 값 툴팁 텍스트. */
function cellTitle(addr: number | null, value: boolean | number | null): string {
  if (addr === null) return 'degraded';
  const v = typeof value === 'boolean' ? (value ? 'ON' : 'OFF') : String(value);
  return `${formatAddr(addr)} = ${v}`;
}

/** 개별 영역 섹션(헤더 집계 + 셀 그리드). */
function AreaSection({ grid }: { grid: AreaGrid }) {
  const { t } = useTranslation();
  const label = REGISTER_AREA_LABELS[grid.area as RegisterArea] ?? grid.area;
  const range =
    grid.addrMin !== null && grid.addrMax !== null
      ? `${formatAddr(grid.addrMin)}→${formatAddr(grid.addrMax)}`
      : '—';

  return (
    <div
      data-testid={`modbus-grid-area-${grid.area}`}
      className="rounded-md border border-(--color-border-default) p-2"
    >
      {/* 헤더: 라벨 + 주소 범위 */}
      <div className="mb-1.5 flex items-center justify-between gap-2">
        <span className="truncate text-xs font-semibold text-(--color-text-secondary)">
          {label}
        </span>
        <span className="shrink-0 font-mono text-[10px] text-(--color-text-muted)">{range}</span>
      </div>
      {/* 집계 배지: POINTS / ACTIVE / DEGRADED */}
      <div className="mb-2 flex gap-2 text-[10px]">
        <span data-testid={`modbus-grid-points-${grid.area}`} className="text-(--color-text-muted)">
          {t('dashboard.modbus.points')} <b className="text-(--color-text-primary)">{grid.points}</b>
        </span>
        <span
          data-testid={`modbus-grid-active-${grid.area}`}
          className="text-green-600 dark:text-green-400"
        >
          {t('dashboard.modbus.gridActive')} <b>{grid.active}</b>
        </span>
        <span
          data-testid={`modbus-grid-degraded-${grid.area}`}
          className="text-amber-600 dark:text-amber-400"
        >
          {t('dashboard.modbus.degradedShort')} <b>{grid.degraded}</b>
        </span>
      </div>
      {/* 셀 그리드 */}
      <div className="flex flex-wrap gap-0.5">
        {grid.cells.map((cell, idx) => (
          <span
            key={cell.addr ?? `deg-${idx}`}
            data-testid={`modbus-grid-cell-${grid.area}-${idx}`}
            data-state={cell.state}
            title={cellTitle(cell.addr, cell.value)}
            className={cn('h-3 w-3 shrink-0 rounded-[2px]', CELL_CLASS[cell.state])}
          />
        ))}
      </div>
    </div>
  );
}

/** 레지스터 맵 그리드(4영역). points>0 또는 스냅샷 값이 있는 영역만 렌더한다. */
export default function RegisterMapGrid({ registerMap, registerCounts }: RegisterMapGridProps) {
  const grids = REGISTER_AREA_ORDER.map((area) => {
    const snapshot = registerMap[area] as Record<string, boolean | number> | undefined;
    const points = registerCounts[area] ?? 0;
    return computeAreaGrid(area, snapshot, points);
  }).filter((g) => g.points > 0 || g.cells.length > 0);

  return (
    <div className="grid min-h-0 flex-1 grid-cols-1 gap-2 overflow-y-auto sm:grid-cols-2">
      {grids.map((grid) => (
        <AreaSection key={grid.area} grid={grid} />
      ))}
    </div>
  );
}
