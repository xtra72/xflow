// 타입 선택이 가능한 키-값 맵 에디터.
// 각 행에 키(string) + 값 타입(string/number/boolean/array/json) + 값 입력.
// 결과는 Record<string, unknown> (타입에 따라 변환된 값).

import { useCallback, useState } from 'react';
import { Plus, Trash2 } from 'lucide-react';

import { cn } from '@/lib/utils/cn';

type ValueType = 'string' | 'number' | 'boolean' | 'array' | 'json';

interface TypedRow {
  id: string;
  key: string;
  valueType: ValueType;
  rawValue: string;
}

interface TypedKeyValueMapEditorProps {
  value: unknown;
  onChange: (value: unknown) => void;
  readOnly?: boolean;
}

let idCounter = 0;
function nextId(): string {
  return `tkv-${++idCounter}-${Date.now()}`;
}

function detectType(v: unknown): ValueType {
  if (v === null || v === undefined) return 'string';
  if (typeof v === 'number') return 'number';
  if (typeof v === 'boolean') return 'boolean';
  if (Array.isArray(v)) return 'array';
  if (typeof v === 'object') return 'json';
  return 'string';
}

function toRows(val: unknown): TypedRow[] {
  if (!val || typeof val !== 'object' || Array.isArray(val)) return [];
  const record = val as Record<string, unknown>;
  return Object.entries(record).map(([k, v]) => ({
    id: nextId(),
    key: k,
    valueType: detectType(v),
    rawValue: typeof v === 'string' ? v : JSON.stringify(v),
  }));
}

function parseValue(raw: string, type: ValueType): unknown {
  switch (type) {
    case 'number': {
      const n = Number(raw);
      return Number.isFinite(n) ? n : 0;
    }
    case 'boolean':
      return raw === 'true' || raw === '1';
    case 'array':
    case 'json':
      try {
        return JSON.parse(raw);
      } catch {
        return type === 'array' ? [] : {};
      }
    default:
      return raw;
  }
}

function toRecord(rows: TypedRow[]): Record<string, unknown> {
  const result: Record<string, unknown> = {};
  for (const row of rows) {
    if (!row.key) continue;
    result[row.key] = parseValue(row.rawValue, row.valueType);
  }
  return result;
}

const inputClass = cn(
  'rounded border px-2 py-1 text-sm',
  'border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-primary)',
  'focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400',
);

export function TypedKeyValueMapEditor({ value, onChange, readOnly }: TypedKeyValueMapEditorProps) {
  const [rows, setRows] = useState<TypedRow[]>(() => toRows(value));

  const commit = useCallback(
    (next: TypedRow[]) => {
      setRows(next);
      onChange(toRecord(next));
    },
    [onChange],
  );

  const addRow = () => {
    commit([...rows, { id: nextId(), key: '', valueType: 'string', rawValue: '' }]);
  };

  const removeRow = (id: string) => {
    commit(rows.filter((r) => r.id !== id));
  };

  const patchRow = (id: string, patch: Partial<TypedRow>) => {
    commit(rows.map((r) => (r.id === id ? { ...r, ...patch } : r)));
  };

  return (
    <div className="space-y-1.5">
      {/* 헤더 */}
      <div className="flex items-center gap-1.5 text-[10px] font-medium text-(--color-text-muted)">
        <span className="flex-1">키</span>
        <span className="w-20">타입</span>
        <span className="flex-[2]">값</span>
        <span className="w-6" />
      </div>

      {rows.map((row) => (
        <div key={row.id} className="flex items-center gap-1.5">
          {/* 키 */}
          <input
            type="text"
            value={row.key}
            onChange={(e) => patchRow(row.id, { key: e.target.value })}
            placeholder="key"
            readOnly={readOnly}
            className={cn(inputClass, 'flex-1')}
          />

          {/* 타입 선택 */}
          <select
            value={row.valueType}
            onChange={(e) => patchRow(row.id, { valueType: e.target.value as ValueType })}
            disabled={readOnly}
            className={cn(inputClass, 'w-20')}
          >
            <option value="string">문자열</option>
            <option value="number">숫자</option>
            <option value="boolean">참/거짓</option>
            <option value="array">배열</option>
            <option value="json">JSON</option>
          </select>

          {/* 값 입력 — 타입별 UI */}
          {row.valueType === 'boolean' ? (
            <select
              value={row.rawValue === 'true' || row.rawValue === '1' ? 'true' : 'false'}
              onChange={(e) => patchRow(row.id, { rawValue: e.target.value })}
              disabled={readOnly}
              className={cn(inputClass, 'flex-[2]')}
            >
              <option value="true">true</option>
              <option value="false">false</option>
            </select>
          ) : row.valueType === 'array' || row.valueType === 'json' ? (
            <textarea
              value={row.rawValue}
              onChange={(e) => patchRow(row.id, { rawValue: e.target.value })}
              readOnly={readOnly}
              placeholder={row.valueType === 'array' ? '["a", "b"]' : '{"k": "v"}'}
              rows={1}
              className={cn(inputClass, 'flex-[2] resize-y')}
            />
          ) : (
            <input
              type={row.valueType === 'number' ? 'number' : 'text'}
              value={row.rawValue}
              onChange={(e) => patchRow(row.id, { rawValue: e.target.value })}
              readOnly={readOnly}
              placeholder={row.valueType === 'number' ? '0' : '값'}
              className={cn(inputClass, 'flex-[2]')}
            />
          )}

          {/* 삭제 */}
          <button
            type="button"
            onClick={() => removeRow(row.id)}
            disabled={readOnly}
            className="flex h-6 w-6 shrink-0 items-center justify-center rounded text-(--color-text-muted) hover:bg-red-50 hover:text-red-600"
          >
            <Trash2 className="h-3 w-3" />
          </button>
        </div>
      ))}

      {/* 추가 버튼 */}
      {!readOnly && (
        <button
          type="button"
          onClick={addRow}
          className="flex items-center gap-1 rounded px-2 py-0.5 text-xs text-blue-600 hover:bg-blue-50"
        >
          <Plus className="h-3 w-3" /> 추가
        </button>
      )}
    </div>
  );
}
