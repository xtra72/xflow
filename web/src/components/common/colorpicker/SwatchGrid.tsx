// 색 칸 격자 — 프리셋과 최근색이 같은 부품을 쓴다.
//
// **격자 전체가 탭 정지 하나다**(roving tabindex). 28칸이 각각 탭 정지면 팝오버 하나를
// 지나가는 데 Tab 을 서른 번 눌러야 한다. 안에서는 방향키로 옮기고 Enter/Space 로 고른다.
//
// 칸을 `<button>` 이 아니라 `role="option"` 으로 두는 데에는 이유가 있다. 목록상자
// (`role="listbox"`)의 자식은 `option` 이어야 하고, `<button>` 에 `role="option"` 을
// 덧씌우면 **Enter 가 두 번 먹는다** — 브라우저가 단추의 기본 동작으로 `click` 을 내고,
// 격자의 `keydown` 처리가 한 번 더 고른다. 두 번 고르면 값이 두 번 나가고 팝오버가 닫힌
// 뒤 한 번 더 나간다.
//
// 칸의 접근성 이름은 색 이름이 아니라 **hex 문자열**이다. 오늘 `ColorSwatchButton` 과
// `PanelColorRow` 가 그렇게 하고 있고, 28색에 사람 말 이름을 붙이면 i18n 항목이 56개
// 늘면서 "짙은 청록" 류의 번역 품질을 보증할 수 없다. 이연 항목으로 남긴다.
//
// @spec SPEC-COLOR-001 §결정 7 (M5)

import { useEffect, useRef, useState } from 'react';

import { cn } from '@/lib/utils/cn';

/** 격자 안의 자리. 평평한 차례가 아니라 (행, 칸) 인 것은 행마다 길이가 다르기 때문이다. */
interface Cell {
  readonly row: number;
  readonly col: number;
}

export interface SwatchGridProps {
  /** 행마다 길이가 다를 수 있다 — 무채색 8칸, 색상환 10칸. */
  rows: readonly (readonly string[])[];
  /** 현재 값. 이 색의 칸이 `aria-selected` 이고, 커서가 처음 놓이는 자리다. */
  value: string | undefined;
  onPick: (color: string) => void;
  ariaLabel: string;
  testId: string;
}

/**
 * 그 행의 칸 수. **없는 행은 0** 이다.
 *
 * 인덱스 접근(`rows[row]`) 대신 훑는 것은 `noUncheckedIndexedAccess` 아래에서
 * `undefined` 갈래가 하나 생기고 그 갈래를 시험으로 지날 길이 없기 때문이다. 여기서는
 * "범위 밖" 이 0 으로 접혀 뒤의 가둠 산수가 그대로 처리한다 — 최근색 목록이 줄어들어
 * 커서가 사라진 행을 가리키는 경우가 실제로 있다.
 */
function colsIn(rows: readonly (readonly string[])[], row: number): number {
  let i = 0;
  for (const line of rows) {
    if (i === row) return line.length;
    i += 1;
  }
  return 0;
}

/** 격자 안으로 가둔 자리. 갈 칸이 없으면 `null`. */
function clampCell(
  rows: readonly (readonly string[])[],
  row: number,
  col: number,
): Cell | null {
  const r = Math.min(Math.max(row, 0), rows.length - 1);
  const n = colsIn(rows, r);
  if (n === 0) return null;
  return { row: r, col: Math.min(Math.max(col, 0), n - 1) };
}

/** 방향키 한 번. 격자 밖으로는 넘어가지 않는다(끝에서 멈춘다). */
function move(
  rows: readonly (readonly string[])[],
  at: Cell,
  key: string,
): Cell | null {
  switch (key) {
    case 'ArrowRight':
      return clampCell(rows, at.row, at.col + 1);
    case 'ArrowLeft':
      return clampCell(rows, at.row, at.col - 1);
    case 'ArrowDown':
      return clampCell(rows, at.row + 1, at.col);
    case 'ArrowUp':
      return clampCell(rows, at.row - 1, at.col);
    default:
      return null;
  }
}

/** 현재 값이 놓인 칸. 없으면 `null`. */
function findValue(rows: readonly (readonly string[])[], value: string | undefined): Cell | null {
  if (value === undefined) return null;
  let row = 0;
  for (const line of rows) {
    const col = line.indexOf(value);
    if (col >= 0) return { row, col };
    row += 1;
  }
  return null;
}

export default function SwatchGrid({ rows, value, onPick, ariaLabel, testId }: SwatchGridProps) {
  const gridRef = useRef<HTMLDivElement>(null);
  /** 사용자가 방향키로 옮긴 자리. 옮기기 전에는 현재 값의 칸을 따라간다. */
  const [cursor, setCursor] = useState<Cell | null>(null);

  const active = cursor ?? findValue(rows, value) ?? { row: 0, col: 0 };
  const cells = rows.flatMap((line, row) => line.map((color, col) => ({ row, col, color })));
  const activeColor = cells.find((c) => c.row === active.row && c.col === active.col)?.color;

  // 방향키로 옮긴 뒤에는 포커스도 따라가야 한다 — 따라가지 않으면 `keydown` 이 계속
  // 옛 칸에서 나고, 화면 낭독기가 읽는 칸과 선택될 칸이 어긋난다.
  useEffect(() => {
    if (cursor === null) return;
    gridRef.current
      ?.querySelector<HTMLElement>(`[data-swatch="${cursor.row}-${cursor.col}"]`)
      ?.focus();
  }, [cursor]);

  return (
    <div
      ref={gridRef}
      role="listbox"
      aria-label={ariaLabel}
      data-testid={testId}
      className="flex flex-col gap-1"
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          if (activeColor !== undefined) onPick(activeColor);
          return;
        }
        const next = move(rows, active, e.key);
        if (next === null) return;
        e.preventDefault();
        setCursor(next);
      }}
    >
      {rows.map((line, row) => (
        // 행 묶음은 배치를 위한 것일 뿐이라 접근성 트리에서 지운다 — 목록상자의
        // 자식은 `option` 이어야 한다.
        <div key={row} role="presentation" className="flex gap-1">
          {line.map((color, col) => (
            <span
              key={color}
              role="option"
              aria-selected={color === value}
              aria-label={color}
              data-swatch={`${row}-${col}`}
              tabIndex={row === active.row && col === active.col ? 0 : -1}
              onClick={() => onPick(color)}
              className={cn(
                'h-5 w-5 cursor-pointer rounded-md border transition-transform hover:scale-110',
                color === value
                  ? 'border-(--color-interactive-primary) ring-1 ring-(--color-interactive-primary)'
                  : 'border-(--color-border-default)',
              )}
              style={{ backgroundColor: color }}
            />
          ))}
        </div>
      ))}
    </div>
  );
}
