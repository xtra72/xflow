// Mapping 노드의 키-값 매핑 테이블 에디터.
// Record<string, string> 형태의 매핑 데이터를 테이블 행으로 표시하고,
// 추가/삭제를 지원한다.

import { useCallback, useRef, useState } from 'react';
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

/** Record<string, string> → 플랫 행 배열 (안정적 key 유지) */
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

// readOnly 스타일.
//
// 주의: text 등 readOnly attr 를 지원하는 input 에는 `disabled` 가 아닌
// `readOnly` 를 사용한다. `disabled` 는 다크모드에서 텍스트를 흐리게 렌더링해
// 값이 거의 보이지 않는 가시성 회귀를 일으킨다 (commit b4ad829 / 309966e 와 동일 패턴).
const readOnlyInput = 'cursor-not-allowed bg-(--color-bg-elevated)';

// ---- 컴포넌트 ----

export function KeyValueMapEditor({ value, onChange, readOnly }: KeyValueMapEditorProps) {
  // 내부 상태로 행을 관리하여 key 안정성을 보장한다.
  // 외부 value는 초기화 시에만 반영한다.
  const lastExternalRef = useRef<unknown>(undefined);
  const [rows, setRows] = useState<KvRow[]>(() => toRows(value));

  // 외부 value가 완전히 다른 객체로 교체되면 내부 상태를 동기화한다.
  // (단, 자체 emit으로 인한 변경은 무시)
  if (value !== lastExternalRef.current) {
    const externalRecord = (value && typeof value === 'object' && !Array.isArray(value))
      ? value as Record<string, string> : {};
    const internalRecord = toRecord(rows);
    const externalKeys = Object.keys(externalRecord).sort().join(',');
    const internalKeys = Object.keys(internalRecord).sort().join(',');
    const externalVals = Object.values(externalRecord).sort().join(',');
    const internalVals = Object.values(internalRecord).sort().join(',');
    if (externalKeys !== internalKeys || externalVals !== internalVals) {
      const newRows = toRows(value);
      setRows(newRows);
    }
    lastExternalRef.current = value;
  }

  const emit = useCallback(
    (updated: KvRow[]) => {
      setRows(updated);
      const record = toRecord(updated);
      lastExternalRef.current = record;
      onChange(record);
    },
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
              <tr key={row.key} className="transition-colors hover:bg-(--color-bg-elevated)">
                {/* 키 */}
                <td className="px-2 py-1">
                  <input
                    type="text"
                    value={row.mapKey}
                    // readOnly attr 사용 — disabled 는 다크모드에서 텍스트를 흐리게 렌더링한다 (commit b4ad829 참조).
                    readOnly={readOnly}
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
                    // readOnly attr 사용 — disabled 는 다크모드에서 텍스트를 흐리게 렌더링한다 (commit b4ad829 참조).
                    readOnly={readOnly}
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
