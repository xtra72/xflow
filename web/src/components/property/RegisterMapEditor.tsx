// Modbus TCP Server 레지스터 맵 테이블 에디터.
// register_map 의 4개 영역(coils, discrete_inputs, holding_registers, input_registers)을
// 플랫 테이블 행으로 표시하고, 추가/삭제를 지원한다.

import { useCallback, useMemo } from 'react';
import { Plus, Trash2 } from 'lucide-react';

import { cn } from '@/lib/utils/cn';

// ---- 상수 ----

const AREA_TYPES = [
  { value: 'coils', label: '코일 (FC01/05/15)' },
  { value: 'discrete_inputs', label: '이산 입력 (FC02)' },
  { value: 'holding_registers', label: '보유 레지스터 (FC03/06/16)' },
  { value: 'input_registers', label: '입력 레지스터 (FC04)' },
] as const;

type AreaType = (typeof AREA_TYPES)[number]['value'];

const DATA_TYPES = ['uint16', 'int16', 'float32', 'uint32', 'int32'] as const;

// ---- 내부 행 타입 ----

interface RegisterRow {
  key: string;
  areaType: AreaType;
  startAddress: number;
  count: number;
  dataType: string;
}

// ---- Props ----

interface RegisterMapEditorProps {
  value: unknown;
  onChange: (value: unknown) => void;
  readOnly?: boolean;
}

// ---- 변환 유틸 ----

let keyCounter = 0;
function nextKey(): string {
  return `reg-${++keyCounter}-${Date.now()}`;
}

/** register_map 객체 → 플랫 행 배열 */
function toRows(registerMap: unknown): RegisterRow[] {
  if (!registerMap || typeof registerMap !== 'object') return [];
  const map = registerMap as Record<string, unknown>;
  const rows: RegisterRow[] = [];

  for (const area of AREA_TYPES) {
    const raw = map[area.value];
    if (!raw) continue;

    const segments = Array.isArray(raw) ? raw : [raw];
    for (const seg of segments) {
      if (!seg || typeof seg !== 'object') continue;
      const s = seg as Record<string, unknown>;
      rows.push({
        key: nextKey(),
        areaType: area.value,
        startAddress: Number(s.start_address ?? 0),
        count: Number(s.count ?? 1),
        dataType: String(s.data_type ?? 'uint16'),
      });
    }
  }

  return rows;
}

/** 플랫 행 배열 → register_map 객체 */
function toRegisterMap(rows: RegisterRow[]): Record<string, unknown> {
  const map: Record<string, Record<string, unknown>[]> = {};

  for (const row of rows) {
    const segments = map[row.areaType] ?? (map[row.areaType] = []);
    const seg: Record<string, unknown> = {
      start_address: row.startAddress,
      count: row.count,
    };
    if (row.dataType && row.dataType !== 'uint16') {
      seg.data_type = row.dataType;
    }
    segments.push(seg);
  }

  return map;
}

// ---- 스타일 ----

const cellInput = cn(
  'w-full rounded border px-2 py-1 text-sm',
  'border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-primary)',
  'focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400',
  'dark:focus:border-blue-500',
);

const readOnlyInput = 'opacity-60 cursor-not-allowed bg-gray-50 dark:bg-gray-900';

// ---- 컴포넌트 ----

export function RegisterMapEditor({ value, onChange, readOnly }: RegisterMapEditorProps) {
  const rows = useMemo(() => toRows(value), [value]);

  const emit = useCallback(
    (updated: RegisterRow[]) => onChange(toRegisterMap(updated)),
    [onChange],
  );

  const handleAdd = useCallback(() => {
    const newRow: RegisterRow = {
      key: nextKey(),
      areaType: 'holding_registers',
      startAddress: 0,
      count: 10,
      dataType: 'uint16',
    };
    emit([...rows, newRow]);
  }, [rows, emit]);

  const handleRemove = useCallback(
    (key: string) => emit(rows.filter((r) => r.key !== key)),
    [rows, emit],
  );

  const handleChange = useCallback(
    (key: string, field: keyof RegisterRow, val: unknown) => {
      emit(
        rows.map((r) =>
          r.key === key ? { ...r, [field]: field === 'startAddress' || field === 'count' ? Number(val) : val } : r,
        ),
      );
    },
    [rows, emit],
  );

  return (
    <div className="space-y-2">
      <div className="overflow-x-auto rounded-md border border-(--color-border-default)">
        <table className="min-w-full text-sm">
          <thead>
            <tr className="bg-(--color-bg-primary)">
              <th className="px-2 py-1.5 text-left text-xs font-medium text-(--color-text-muted)">
                영역 타입
              </th>
              <th className="px-2 py-1.5 text-left text-xs font-medium text-(--color-text-muted)">
                시작 주소
              </th>
              <th className="px-2 py-1.5 text-left text-xs font-medium text-(--color-text-muted)">
                개수
              </th>
              <th className="px-2 py-1.5 text-left text-xs font-medium text-(--color-text-muted)">
                데이터 타입
              </th>
              {!readOnly && (
                <th className="w-10 px-2 py-1.5" />
              )}
            </tr>
          </thead>
          <tbody className="divide-y divide-(--color-border-default)">
            {rows.length === 0 && (
              <tr>
                <td
                  colSpan={readOnly ? 4 : 5}
                  className="px-2 py-4 text-center text-xs text-(--color-text-muted)"
                >
                  레지스터 영역이 없습니다
                </td>
              </tr>
            )}
            {rows.map((row) => (
              <tr key={row.key} className="hover:bg-gray-50 dark:hover:bg-gray-800/50">
                {/* 영역 타입 */}
                <td className="px-2 py-1">
                  <select
                    value={row.areaType}
                    disabled={readOnly}
                    onChange={(e) => handleChange(row.key, 'areaType', e.target.value)}
                    className={cn(cellInput, readOnly && readOnlyInput)}
                  >
                    {AREA_TYPES.map((t) => (
                      <option key={t.value} value={t.value}>
                        {t.label}
                      </option>
                    ))}
                  </select>
                </td>

                {/* 시작 주소 */}
                <td className="px-2 py-1">
                  <input
                    type="number"
                    min={0}
                    max={65535}
                    value={row.startAddress}
                    disabled={readOnly}
                    onChange={(e) => handleChange(row.key, 'startAddress', e.target.value)}
                    className={cn(cellInput, 'w-24', readOnly && readOnlyInput)}
                  />
                </td>

                {/* 개수 */}
                <td className="px-2 py-1">
                  <input
                    type="number"
                    min={1}
                    max={65535}
                    value={row.count}
                    disabled={readOnly}
                    onChange={(e) => handleChange(row.key, 'count', e.target.value)}
                    className={cn(cellInput, 'w-20', readOnly && readOnlyInput)}
                  />
                </td>

                {/* 데이터 타입 */}
                <td className="px-2 py-1">
                  <select
                    value={row.dataType}
                    disabled={readOnly}
                    onChange={(e) => handleChange(row.key, 'dataType', e.target.value)}
                    className={cn(cellInput, readOnly && readOnlyInput)}
                  >
                    {DATA_TYPES.map((dt) => (
                      <option key={dt} value={dt}>
                        {dt}
                      </option>
                    ))}
                  </select>
                </td>

                {/* 삭제 */}
                {!readOnly && (
                  <td className="px-2 py-1">
                    <button
                      type="button"
                      onClick={() => handleRemove(row.key)}
                      className="rounded p-1 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                      aria-label="삭제"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </td>
                )}
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* 추가 버튼 */}
      {!readOnly && (
        <button
          type="button"
          onClick={handleAdd}
          className="inline-flex items-center gap-1 rounded-md border border-dashed border-(--color-border-default) px-3 py-1.5 text-xs font-medium text-(--color-text-muted) transition-colors hover:border-blue-400 hover:text-blue-600 dark:hover:border-blue-500 dark:hover:text-blue-400"
        >
          <Plus className="h-3.5 w-3.5" />
          영역 추가
        </button>
      )}
    </div>
  );
}
