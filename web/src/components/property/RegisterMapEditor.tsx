// Modbus TCP Server 레지스터 맵 테이블 에디터.
// register_map 의 4개 영역(coils, discrete_inputs, holding_registers, input_registers)을
// 플랫 테이블 행으로 표시하고, 추가/삭제를 지원한다.

import { useCallback, useMemo } from 'react';
import { Plus, Trash2 } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';

// ---- 상수 ----

// 각 영역 타입의 라벨은 i18n 키만 보관하고, 렌더 시 컴포넌트 내부에서 t(labelKey) 로 변환한다.
const AREA_TYPES = [
  { value: 'coils', labelKey: 'property.register.areaCoils' },
  { value: 'discrete_inputs', labelKey: 'property.register.areaDiscreteInputs' },
  { value: 'holding_registers', labelKey: 'property.register.areaHoldingRegisters' },
  { value: 'input_registers', labelKey: 'property.register.areaInputRegisters' },
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

// readOnly 스타일.
//
// 주의: number 등 readOnly attr 를 지원하는 input 에는 `disabled` 가 아닌
// `readOnly` 를 사용한다. `disabled` 는 다크모드에서 텍스트를 흐리게 렌더링해
// 값이 거의 보이지 않는 가시성 회귀를 일으킨다 (commit b4ad829 / 309966e 와 동일 패턴).
// select 는 readOnly attr 미지원이므로 `disabled={readOnly}` 를 그대로 사용한다.
const readOnlyInput = 'cursor-not-allowed bg-(--color-bg-elevated)';

// ---- 컴포넌트 ----

export function RegisterMapEditor({ value, onChange, readOnly }: RegisterMapEditorProps) {
  const { t } = useTranslation();
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
                {t('property.register.areaType')}
              </th>
              <th className="px-2 py-1.5 text-left text-xs font-medium text-(--color-text-muted)">
                {t('property.register.startAddress')}
              </th>
              <th className="px-2 py-1.5 text-left text-xs font-medium text-(--color-text-muted)">
                {t('property.register.count')}
              </th>
              <th className="px-2 py-1.5 text-left text-xs font-medium text-(--color-text-muted)">
                {t('property.register.dataType')}
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
                  {t('property.register.empty')}
                </td>
              </tr>
            )}
            {rows.map((row) => (
              <tr key={row.key} className="transition-colors hover:bg-(--color-bg-elevated)">
                {/* 영역 타입 */}
                <td className="px-2 py-1">
                  <select
                    value={row.areaType}
                    disabled={readOnly}
                    onChange={(e) => handleChange(row.key, 'areaType', e.target.value)}
                    className={cn(cellInput, readOnly && readOnlyInput)}
                  >
                    {AREA_TYPES.map((area) => (
                      <option key={area.value} value={area.value}>
                        {t(area.labelKey)}
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
                    // readOnly attr 사용 — disabled 는 다크모드에서 텍스트를 흐리게 렌더링한다 (commit b4ad829 참조).
                    readOnly={readOnly}
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
                    // readOnly attr 사용 — disabled 는 다크모드에서 텍스트를 흐리게 렌더링한다 (commit b4ad829 참조).
                    readOnly={readOnly}
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
                      className="rounded p-1 text-(--color-text-muted) transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                      aria-label={t('property.register.deleteAria')}
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
          {t('property.register.addArea')}
        </button>
      )}
    </div>
  );
}
