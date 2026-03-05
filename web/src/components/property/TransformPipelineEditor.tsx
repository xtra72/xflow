// Transform 노드 페이로드 필드 구성 에디터.
// 모드(select/merge/exclude)는 상단 별도 선택,
// 각 필드를 key-value 테이블 행으로 편집한다.
// dot notation 지원: params.area → { params: { area: ... } } 자동 중첩.
// 백엔드 parseExpressionSteps() 와 호환.

import { useCallback, useEffect, useRef, useState } from 'react';
import { Plus, Trash2 } from 'lucide-react';

import { cn } from '@/lib/utils/cn';

// ---- 상수 ----

const MODES = [
  { value: 'select', label: 'Select (필드 선택)' },
  { value: 'merge', label: 'Merge (필드 병합)' },
  { value: 'exclude', label: 'Exclude (필드 제외)' },
] as const;

type PipelineMode = (typeof MODES)[number]['value'];

// ---- 내부 행 타입 ----

interface FieldRow {
  key: string;
  fieldName: string;
  fieldValue: string;
}

// ---- Props ----

interface TransformPipelineEditorProps {
  value: unknown;
  onChange: (value: unknown) => void;
  readOnly?: boolean;
}

// ---- 키 생성 ----

let keyCounter = 0;
function nextKey(): string {
  return `tf-${++keyCounter}-${Date.now()}`;
}

// ---- 파싱 유틸 ----

/** 중첩({}, [], 문자열)을 고려하여 최상위 레벨에서 delimiter 로 분할 */
function splitAtTopLevel(str: string, delim: string): string[] {
  const parts: string[] = [];
  let current = '';
  let depth = 0;
  let inStr = false;
  let strChar = '';

  for (let i = 0; i < str.length; i++) {
    const ch = str[i];

    if (inStr) {
      current += ch;
      if (ch === strChar && str[i - 1] !== '\\') inStr = false;
      continue;
    }
    if (ch === '"' || ch === "'") {
      inStr = true;
      strChar = ch;
      current += ch;
      continue;
    }
    if (ch === '{' || ch === '[' || ch === '(') { depth++; current += ch; continue; }
    if (ch === '}' || ch === ']' || ch === ')') { depth--; current += ch; continue; }
    if (depth === 0 && ch === delim) {
      parts.push(current);
      current = '';
      continue;
    }
    current += ch;
  }

  if (current.trim()) parts.push(current);
  return parts;
}

/** 최상위 레벨에서 첫 번째 해당 문자의 인덱스 반환 (-1 = 없음) */
function indexOfTopLevel(str: string, char: string): number {
  let depth = 0;
  let inStr = false;
  let strChar = '';

  for (let i = 0; i < str.length; i++) {
    const ch = str[i];
    if (inStr) { if (ch === strChar && str[i - 1] !== '\\') inStr = false; continue; }
    if (ch === '"' || ch === "'") { inStr = true; strChar = ch; continue; }
    if (ch === '{' || ch === '[' || ch === '(') { depth++; continue; }
    if (ch === '}' || ch === ']' || ch === ')') { depth--; continue; }
    if (depth === 0 && ch === char) return i;
  }
  return -1;
}

/** { key: value, nested: { subKey: val } } 형태를 플랫 필드 행으로 파싱 */
function parseObjectExpr(expr: string, prefix: string): FieldRow[] {
  const t = expr.trim();
  if (!t.startsWith('{') || !t.endsWith('}')) return [];

  const inner = t.slice(1, -1).trim();
  if (!inner) return [];

  const pairs = splitAtTopLevel(inner, ',');
  const rows: FieldRow[] = [];

  for (const pair of pairs) {
    const p = pair.trim();
    if (!p) continue;

    const ci = indexOfTopLevel(p, ':');
    if (ci === -1) continue;

    const key = p.substring(0, ci).trim();
    const val = p.substring(ci + 1).trim();
    const fullKey = prefix ? `${prefix}.${key}` : key;

    // 중첩 객체이면 재귀로 플래트닝
    if (val.startsWith('{') && val.endsWith('}')) {
      rows.push(...parseObjectExpr(val, fullKey));
    } else {
      rows.push({ key: nextKey(), fieldName: fullKey, fieldValue: val });
    }
  }

  return rows;
}

/** 쉼표로 구분된 제외 필드 목록 파싱 */
function parseExcludeExpr(expr: string): FieldRow[] {
  return expr
    .split(',')
    .map((f) => f.trim())
    .filter(Boolean)
    .map((f) => ({ key: nextKey(), fieldName: f, fieldValue: '' }));
}

/** 외부 value → 편집기 상태 변환 */
function parseInput(value: unknown): { mode: PipelineMode; fields: FieldRow[] } {
  // 파이프라인 배열: [{ select: "{ ... }" }]
  if (Array.isArray(value) && value.length > 0) {
    const step = value[0] as Record<string, unknown>;
    if (step && typeof step === 'object') {
      for (const m of MODES) {
        if (m.value in step) {
          const s = String(step[m.value] ?? '');
          return {
            mode: m.value,
            fields: m.value === 'exclude' ? parseExcludeExpr(s) : parseObjectExpr(s, ''),
          };
        }
      }
    }
  }

  // 단일 문자열 (하위 호환, select 모드로 간주)
  if (typeof value === 'string' && value.trim()) {
    const t = value.trim();
    if (t.startsWith('{')) return { mode: 'select', fields: parseObjectExpr(t, '') };
    return { mode: 'select', fields: [{ key: nextKey(), fieldName: t, fieldValue: '' }] };
  }

  return { mode: 'select', fields: [] };
}

// ---- 직렬화 유틸 ----

interface TreeNode {
  value?: string;
  children: Map<string, TreeNode>;
  order: number;
}

/** 플랫 행을 dot notation 기반 중첩 트리로 구성 */
function buildTree(rows: FieldRow[]): TreeNode {
  const root: TreeNode = { children: new Map(), order: 0 };
  let ord = 0;

  for (const row of rows) {
    if (!row.fieldName.trim()) continue;
    const parts = row.fieldName.split('.');
    let cur = root;

    for (let i = 0; i < parts.length - 1; i++) {
      const part = parts[i] as string;
      let child = cur.children.get(part);
      if (!child) {
        child = { children: new Map(), order: ord++ };
        cur.children.set(part, child);
      }
      cur = child;
    }

    const leaf = parts[parts.length - 1] as string;
    let leafNode = cur.children.get(leaf);
    if (!leafNode) {
      leafNode = { children: new Map(), order: ord++ };
      cur.children.set(leaf, leafNode);
    }
    leafNode.value = row.fieldValue;
  }

  return root;
}

/** 트리를 { key: value, ... } 형태의 표현식 문자열로 직렬화 */
function serializeNode(node: TreeNode): string {
  const entries = [...node.children.entries()].sort((a, b) => a[1].order - b[1].order);
  const parts: string[] = [];

  for (const [key, child] of entries) {
    if (child.children.size > 0) {
      parts.push(`${key}: ${serializeNode(child)}`);
    } else if (child.value !== undefined) {
      parts.push(`${key}: ${child.value}`);
    }
  }

  return `{ ${parts.join(', ')} }`;
}

/** 편집기 상태 → 표현식 문자열 */
function buildExprString(mode: PipelineMode, rows: FieldRow[]): string {
  if (mode === 'exclude') {
    return rows.map((r) => r.fieldName).filter(Boolean).join(', ');
  }
  const valid = rows.filter((r) => r.fieldName.trim());
  if (valid.length === 0) return '{}';
  return serializeNode(buildTree(valid));
}

/** 편집기 상태 → 백엔드 호환 파이프라인 배열 */
function serializeOutput(mode: PipelineMode, rows: FieldRow[]): Record<string, string>[] {
  const hasContent = rows.some((r) => r.fieldName.trim());
  if (!hasContent) return [];
  return [{ [mode]: buildExprString(mode, rows) }];
}

// ---- 스타일 ----

const cellInput = cn(
  'w-full rounded border px-2 py-1 text-sm',
  'border-gray-200 bg-white text-gray-900',
  'focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400',
  'dark:border-gray-600 dark:bg-gray-800 dark:text-gray-100',
  'dark:focus:border-blue-500',
);

const readOnlyInput = 'opacity-60 cursor-not-allowed bg-gray-50 dark:bg-gray-900';

// ---- 컴포넌트 ----

export function TransformPipelineEditor({ value, onChange, readOnly }: TransformPipelineEditorProps) {
  // 로컬 상태 (커서 위치 보존을 위해 ref 기반 동기화 사용)
  const [mode, setMode] = useState<PipelineMode>(() => parseInput(value).mode);
  const [rows, setRows] = useState<FieldRow[]>(() => parseInput(value).fields);
  const serializedRef = useRef(JSON.stringify(value));

  // 외부 값 변경 시 동기화 (다른 노드 선택 등)
  useEffect(() => {
    const s = JSON.stringify(value);
    if (s !== serializedRef.current) {
      const parsed = parseInput(value);
      setMode(parsed.mode);
      setRows(parsed.fields);
      serializedRef.current = s;
    }
  }, [value]);

  // 변경 방출
  const emit = useCallback(
    (newMode: PipelineMode, newRows: FieldRow[]) => {
      setMode(newMode);
      setRows(newRows);
      const output = serializeOutput(newMode, newRows);
      serializedRef.current = JSON.stringify(output);
      onChange(output);
    },
    [onChange],
  );

  const handleModeChange = useCallback(
    (newMode: PipelineMode) => emit(newMode, rows),
    [rows, emit],
  );

  const handleAdd = useCallback(() => {
    emit(mode, [...rows, { key: nextKey(), fieldName: '', fieldValue: '' }]);
  }, [mode, rows, emit]);

  const handleRemove = useCallback(
    (key: string) => emit(mode, rows.filter((r) => r.key !== key)),
    [mode, rows, emit],
  );

  const handleFieldChange = useCallback(
    (key: string, field: 'fieldName' | 'fieldValue', val: string) => {
      emit(mode, rows.map((r) => (r.key === key ? { ...r, [field]: val } : r)));
    },
    [mode, rows, emit],
  );

  const isExclude = mode === 'exclude';

  return (
    <div className="space-y-2">
      {/* 모드 선택 */}
      <select
        value={mode}
        disabled={readOnly}
        onChange={(e) => handleModeChange(e.target.value as PipelineMode)}
        className={cn(cellInput, 'w-auto', readOnly && readOnlyInput)}
      >
        {MODES.map((m) => (
          <option key={m.value} value={m.value}>
            {m.label}
          </option>
        ))}
      </select>

      {/* 필드 테이블 */}
      <div className="overflow-x-auto rounded-md border border-gray-200 dark:border-gray-700">
        <table className="min-w-full text-sm">
          <thead>
            <tr className="bg-gray-50 dark:bg-gray-800">
              <th className="px-2 py-1.5 text-left text-xs font-medium text-gray-500 dark:text-gray-400">
                {isExclude ? '제외 필드' : '필드명'}
              </th>
              {!isExclude && (
                <th className="px-2 py-1.5 text-left text-xs font-medium text-gray-500 dark:text-gray-400">
                  값 / 표현식
                </th>
              )}
              {!readOnly && <th className="w-10 px-2 py-1.5" />}
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-100 dark:divide-gray-700">
            {rows.length === 0 && (
              <tr>
                <td
                  colSpan={readOnly ? (isExclude ? 1 : 2) : (isExclude ? 2 : 3)}
                  className="px-2 py-4 text-center text-xs text-gray-400 dark:text-gray-500"
                >
                  {isExclude ? '제외할 필드가 없습니다' : '변환 필드가 없습니다'}
                </td>
              </tr>
            )}
            {rows.map((row) => (
              <tr key={row.key} className="hover:bg-gray-50 dark:hover:bg-gray-800/50">
                {/* 필드명 */}
                <td className="px-2 py-1">
                  <input
                    type="text"
                    value={row.fieldName}
                    disabled={readOnly}
                    onChange={(e) => handleFieldChange(row.key, 'fieldName', e.target.value)}
                    placeholder={isExclude ? 'field_name' : 'params.temperature'}
                    className={cn(cellInput, 'font-mono', readOnly && readOnlyInput)}
                  />
                </td>

                {/* 값/표현식 (exclude 모드에서는 숨김) */}
                {!isExclude && (
                  <td className="px-2 py-1">
                    <input
                      type="text"
                      value={row.fieldValue}
                      disabled={readOnly}
                      onChange={(e) => handleFieldChange(row.key, 'fieldValue', e.target.value)}
                      placeholder="$.payload.temperature"
                      className={cn(cellInput, 'font-mono', readOnly && readOnlyInput)}
                    />
                  </td>
                )}

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
          className="inline-flex items-center gap-1 rounded-md border border-dashed border-gray-300 px-3 py-1.5 text-xs font-medium text-gray-600 transition-colors hover:border-blue-400 hover:text-blue-600 dark:border-gray-600 dark:text-gray-400 dark:hover:border-blue-500 dark:hover:text-blue-400"
        >
          <Plus className="h-3.5 w-3.5" />
          필드 추가
        </button>
      )}
    </div>
  );
}
