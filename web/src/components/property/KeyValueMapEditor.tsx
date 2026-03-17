// Mapping 노드의 키-값 매핑 테이블 에디터.
// Record<string, string> 형태의 매핑 데이터를 테이블 행으로 표시하고,
// 추가/삭제를 지원한다.

import { useCallback, useMemo } from 'react';
import { Plus, Trash2 } from 'lucide-react';

import { cn } from '@/lib/utils/cn';

// ---- 내부 행 타입 ----

interface KvRow {
  key: string;
  mapKey: string;
  mapValue: string;
}

// ---- Props ----

interface KeyValueMapEditorProps {
  value: unknown;
  onChange: (value: unknown) => void;
  readOnly?: boolean;
}

// ---- 변환 유틸 ----

let keyCounter = 0;
function nextKey(): string {
  return `kv-${++keyCounter}-${Date.now()}`;
}

/** Record<string, string> → 플랫 행 배열 */
function toRows(val: unknown): KvRow[] {
  if (!val || typeof val !== 'object' || Array.isArray(val)) return [];
  const record = val as Record<string, string>;
  return Object.entries(record).map(([k, v]) => ({
    key: nextKey(),
    mapKey: k,
    mapValue: String(v ?? ''),
  }));
}

/** 플랫 행 배열 → Record<string, string> */
function toRecord(rows: KvRow[]): Record<string, string> {
  const result: Record<string, string> = {};
  for (const row of rows) {
    result[row.mapKey] = row.mapValue;
  }
  return result;
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

export function KeyValueMapEditor({ value, onChange, readOnly }: KeyValueMapEditorProps) {
  const rows = useMemo(() => toRows(value), [value]);

  const emit = useCallback(
    (updated: KvRow[]) => onChange(toRecord(updated)),
    [onChange],
  );

  const handleAdd = useCallback(() => {
    const newRow: KvRow = {
      key: nextKey(),
      mapKey: '',
      mapValue: '',
    };
    emit([...rows, newRow]);
  }, [rows, emit]);

  const handleRemove = useCallback(
    (key: string) => emit(rows.filter((r) => r.key !== key)),
    [rows, emit],
  );

  const handleChange = useCallback(
    (key: string, field: 'mapKey' | 'mapValue', val: string) => {
      emit(rows.map((r) => (r.key === key ? { ...r, [field]: val } : r)));
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
                키
              </th>
              <th className="px-2 py-1.5 text-left text-xs font-medium text-(--color-text-muted)">
                값
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
                  colSpan={readOnly ? 2 : 3}
                  className="px-2 py-4 text-center text-xs text-(--color-text-muted)"
                >
                  매핑 항목이 없습니다
                </td>
              </tr>
            )}
            {rows.map((row) => (
              <tr key={row.key} className="hover:bg-gray-50 dark:hover:bg-gray-800/50">
                {/* 키 */}
                <td className="px-2 py-1">
                  <input
                    type="text"
                    value={row.mapKey}
                    disabled={readOnly}
                    onChange={(e) => handleChange(row.key, 'mapKey', e.target.value)}
                    className={cn(cellInput, readOnly && readOnlyInput)}
                    placeholder="키"
                  />
                </td>

                {/* 값 */}
                <td className="px-2 py-1">
                  <input
                    type="text"
                    value={row.mapValue}
                    disabled={readOnly}
                    onChange={(e) => handleChange(row.key, 'mapValue', e.target.value)}
                    className={cn(cellInput, readOnly && readOnlyInput)}
                    placeholder="값"
                  />
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
          항목 추가
        </button>
      )}
    </div>
  );
}
