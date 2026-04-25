// Store 에이전트의 정적 키 + 태그 목록 에디터.
//
// 각 행은 `{ key: string, tags: Record<string, string> }` 형상이며,
// 태그는 칩(chip) 스타일로 표시된다. 사용자는 행 단위로 추가/삭제하고,
// 태그는 행 내에서 개별적으로 추가/삭제한다.
//
// 유효성 검증은 "소프트 경고" 수준으로만 표시하고 부모의 저장 로직을 막지 않는다.
// 중복 키와 잘못된 태그 키 문자를 경고한다.
//
// @spec SPEC-STORE-003

import { useCallback, useMemo, useRef, useState, type KeyboardEvent } from 'react';
import { Plus, Trash2, X } from 'lucide-react';

import { cn } from '@/lib/utils/cn';

// ---- 외부 값 타입 ----

/**
 * 저장되는 단일 엔트리. 백엔드 스키마와 동일.
 *
 * @spec SPEC-STORE-003
 */
export interface StoreKeyEntry {
  key: string;
  tags: Record<string, string>;
}

// ---- Props ----

interface StoreKeysEditorProps {
  /** 배열이 아니거나 undefined 이면 빈 목록으로 취급한다 (설정이 비어있는 초기 상태 대응). */
  value: unknown;
  onChange: (next: StoreKeyEntry[]) => void;
  readOnly?: boolean;
}

// ---- 내부 행 타입 ----
// React 리스트 렌더링에서 안정적 `key` 를 보장하기 위해 내부 행마다 id 를 부여한다.
// 외부 값(StoreKeyEntry[]) 로 변환 시 id 는 제거된다.

interface InternalRow {
  id: string;
  key: string;
  tags: { tagId: string; k: string; v: string }[];
}

// ---- 변환 유틸 ----

let rowCounter = 0;
function nextRowId(): string {
  return `row-${++rowCounter}-${Date.now()}`;
}

let tagCounter = 0;
function nextTagId(): string {
  return `tag-${++tagCounter}-${Date.now()}`;
}

/** 외부 value → 내부 rows 로 변환. 알 수 없는 형상은 빈 목록으로 처리한다. */
function toRows(value: unknown): InternalRow[] {
  if (!Array.isArray(value)) return [];
  const rows: InternalRow[] = [];
  for (const item of value) {
    if (!item || typeof item !== 'object') continue;
    const rec = item as Record<string, unknown>;
    const key = typeof rec.key === 'string' ? rec.key : '';
    const rawTags =
      rec.tags && typeof rec.tags === 'object' && !Array.isArray(rec.tags)
        ? (rec.tags as Record<string, unknown>)
        : {};
    const tags = Object.entries(rawTags).map(([k, v]) => ({
      tagId: nextTagId(),
      k,
      v: typeof v === 'string' ? v : String(v ?? ''),
    }));
    rows.push({ id: nextRowId(), key, tags });
  }
  return rows;
}

/** 내부 rows → 외부 StoreKeyEntry[] (id/tagId 제거). */
function toEntries(rows: InternalRow[]): StoreKeyEntry[] {
  return rows.map((row) => {
    const tagsObj: Record<string, string> = {};
    for (const t of row.tags) {
      // 빈 태그 키는 생략 (부분 입력 중 상태 대응).
      if (t.k === '') continue;
      tagsObj[t.k] = t.v;
    }
    return { key: row.key, tags: tagsObj };
  });
}

// ---- 검증 ----

/** 태그 키 허용 문자: 영문/숫자/언더스코어/하이픈. */
const TAG_KEY_REGEX = /^[a-zA-Z0-9_-]+$/;

function isValidTagKey(k: string): boolean {
  return k === '' || TAG_KEY_REGEX.test(k);
}

/** 중복된 Store 키 목록을 반환한다 (비어있는 키는 제외). */
function findDuplicateKeys(rows: InternalRow[]): Set<string> {
  const seen = new Map<string, number>();
  for (const row of rows) {
    if (row.key === '') continue;
    seen.set(row.key, (seen.get(row.key) ?? 0) + 1);
  }
  const dupes = new Set<string>();
  for (const [k, count] of seen) {
    if (count > 1) dupes.add(k);
  }
  return dupes;
}

// ---- 스타일 ----

const inputCls = cn(
  'w-full rounded border px-2 py-1 text-sm',
  'border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-primary)',
  'focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400',
  'dark:focus:border-blue-500',
);

// readOnly 스타일.
//
// 주의: input 의 readOnly attr 와 함께 사용한다. disabled 를 적용하면
// 브라우저가 텍스트를 흐리게(회색조로) 렌더링하여 다크모드에서 값이
// 거의 보이지 않는 가시성 회귀가 발생한다 (commit b4ad829 와 동일한 패턴).
// opacity 는 낮추지 않고 배경만 살짝 다르게 표시한다.
const readOnlyCls = 'cursor-not-allowed bg-(--color-bg-elevated)';

const chipCls = cn(
  'inline-flex items-center gap-1 rounded-full px-2 py-0.5',
  'bg-(--color-bg-elevated) text-xs font-medium text-(--color-text-secondary)',
);

const chipWarnCls = cn(
  'inline-flex items-center gap-1 rounded-full px-2 py-0.5',
  'bg-amber-100 text-xs font-medium text-amber-800',
  'dark:bg-amber-900/40 dark:text-amber-300',
);

// ---- 컴포넌트 ----

export function StoreKeysEditor({ value, onChange, readOnly }: StoreKeysEditorProps) {
  // 내부 상태로 행을 관리해 안정적 key 를 보장한다.
  // 외부 value 가 완전히 다른 객체로 교체되면(새 에이전트 로드, 취소 등) 동기화한다.
  const lastExternalRef = useRef<unknown>(undefined);
  const [rows, setRows] = useState<InternalRow[]>(() => toRows(value));

  if (value !== lastExternalRef.current) {
    // 얕은 비교: 외부 엔트리 개수나 키 집합이 다르면 전체 리셋.
    const externalEntries = toEntries(toRows(value));
    const internalEntries = toEntries(rows);
    const serialize = (list: StoreKeyEntry[]) =>
      JSON.stringify(
        list
          .map((e) => ({ key: e.key, tags: { ...e.tags } }))
          .sort((a, b) => a.key.localeCompare(b.key)),
      );
    if (serialize(externalEntries) !== serialize(internalEntries)) {
      setRows(toRows(value));
    }
    lastExternalRef.current = value;
  }

  const emit = useCallback(
    (next: InternalRow[]) => {
      setRows(next);
      const entries = toEntries(next);
      lastExternalRef.current = value; // ref 는 동일한 외부 참조를 유지하여 루프 방지.
      onChange(entries);
    },
    [onChange, value],
  );

  // --- 행 추가/삭제 ---

  const handleAddRow = useCallback(() => {
    emit([...rows, { id: nextRowId(), key: '', tags: [] }]);
  }, [rows, emit]);

  const handleRemoveRow = useCallback(
    (rowId: string) => {
      emit(rows.filter((r) => r.id !== rowId));
    },
    [rows, emit],
  );

  // --- 키 편집 ---

  const handleKeyChange = useCallback(
    (rowId: string, nextKey: string) => {
      emit(rows.map((r) => (r.id === rowId ? { ...r, key: nextKey } : r)));
    },
    [rows, emit],
  );

  // --- 태그 추가/삭제/편집 ---

  const handleRemoveTag = useCallback(
    (rowId: string, tagId: string) => {
      emit(
        rows.map((r) =>
          r.id === rowId
            ? { ...r, tags: r.tags.filter((t) => t.tagId !== tagId) }
            : r,
        ),
      );
    },
    [rows, emit],
  );

  const handleAddTagFromInput = useCallback(
    (rowId: string, keyInput: string, valInput: string) => {
      const k = keyInput.trim();
      const v = valInput.trim();
      if (k === '') return; // 키가 비어있으면 무시 (부모에 emit 하지 않음)
      emit(
        rows.map((r) =>
          r.id === rowId
            ? {
                ...r,
                tags: [...r.tags, { tagId: nextTagId(), k, v }],
              }
            : r,
        ),
      );
    },
    [rows, emit],
  );

  // --- 검증 결과 (소프트 경고) ---

  const duplicateKeys = useMemo(() => findDuplicateKeys(rows), [rows]);

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
                태그
              </th>
              {!readOnly && <th className="w-10 px-2 py-1.5" />}
            </tr>
          </thead>
          <tbody className="divide-y divide-(--color-border-default)">
            {rows.length === 0 && (
              <tr>
                <td
                  colSpan={readOnly ? 2 : 3}
                  className="px-2 py-4 text-center text-xs text-(--color-text-muted)"
                >
                  정적 키가 없습니다
                </td>
              </tr>
            )}
            {rows.map((row) => {
              const isDuplicate = duplicateKeys.has(row.key);
              return (
                <tr
                  key={row.id}
                  className="transition-colors hover:bg-(--color-bg-elevated)"
                >
                  {/* 키 입력 */}
                  <td className="px-2 py-1.5 align-top" style={{ minWidth: '12rem' }}>
                    <input
                      type="text"
                      value={row.key}
                      // readOnly attr 사용 — disabled 는 다크모드에서 텍스트를
                      // 흐리게 렌더링해 값이 보이지 않게 만든다 (commit b4ad829 참조).
                      readOnly={readOnly}
                      onChange={(e) => handleKeyChange(row.id, e.target.value)}
                      className={cn(
                        inputCls,
                        readOnly && readOnlyCls,
                        isDuplicate &&
                          'border-amber-400 focus:border-amber-500 focus:ring-amber-400',
                      )}
                      placeholder="indoor/1/temperature"
                      aria-invalid={isDuplicate || undefined}
                    />
                    {isDuplicate && (
                      <p className="mt-1 text-[10px] text-amber-600 dark:text-amber-400">
                        중복된 키입니다
                      </p>
                    )}
                  </td>

                  {/* 태그 칩 + 추가 폼 */}
                  <td className="px-2 py-1.5 align-top">
                    <TagChipsEditor
                      tags={row.tags}
                      readOnly={readOnly}
                      onRemoveTag={(tagId) => handleRemoveTag(row.id, tagId)}
                      onAddTag={(k, v) => handleAddTagFromInput(row.id, k, v)}
                    />
                  </td>

                  {/* 행 삭제 */}
                  {!readOnly && (
                    <td className="px-2 py-1.5 align-top">
                      <button
                        type="button"
                        onClick={() => handleRemoveRow(row.id)}
                        className="rounded p-1 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                        aria-label="행 삭제"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </td>
                  )}
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {/* 행 추가 버튼 */}
      {!readOnly && (
        <button
          type="button"
          onClick={handleAddRow}
          className="inline-flex items-center gap-1 rounded-md border border-dashed border-(--color-border-default) px-3 py-1.5 text-xs font-medium text-(--color-text-muted) transition-colors hover:border-blue-400 hover:text-blue-600 dark:hover:border-blue-500 dark:hover:text-blue-400"
        >
          <Plus className="h-3.5 w-3.5" />
          행 추가
        </button>
      )}
    </div>
  );
}

// ---- 태그 칩 에디터 (행 내부) ----

/**
 * 단일 행의 태그 목록을 칩으로 표시하고, 하단에 "키=값" 입력 + 추가 버튼을 둔다.
 *
 * 추가 동작: 키 input 에서 Enter 키 또는 "추가" 버튼 클릭 시 emit.
 *   - 태그 키 비어있으면 no-op.
 *   - 유효하지 않은 키 문자는 chip 을 추가하되 경고 스타일(amber) 로 표시.
 */
function TagChipsEditor({
  tags,
  readOnly,
  onRemoveTag,
  onAddTag,
}: {
  tags: { tagId: string; k: string; v: string }[];
  readOnly?: boolean;
  onRemoveTag: (tagId: string) => void;
  onAddTag: (k: string, v: string) => void;
}) {
  const [keyInput, setKeyInput] = useState('');
  const [valInput, setValInput] = useState('');

  const handleAddClick = useCallback(() => {
    if (keyInput.trim() === '') return;
    onAddTag(keyInput, valInput);
    setKeyInput('');
    setValInput('');
  }, [keyInput, valInput, onAddTag]);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLInputElement>) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        handleAddClick();
      }
    },
    [handleAddClick],
  );

  return (
    <div className="space-y-1.5">
      {/* 태그 칩 리스트 */}
      <div className="flex flex-wrap gap-1">
        {tags.length === 0 && (
          <span className="text-[11px] text-(--color-text-muted)">태그 없음</span>
        )}
        {tags.map((t) => {
          const valid = isValidTagKey(t.k);
          return (
            <span key={t.tagId} className={valid ? chipCls : chipWarnCls}>
              <span className="font-mono">
                {t.k}={t.v}
              </span>
              {!readOnly && (
                <button
                  type="button"
                  onClick={() => onRemoveTag(t.tagId)}
                  className="rounded-full p-0.5 hover:bg-black/10 dark:hover:bg-white/10"
                  aria-label={`${t.k} 태그 삭제`}
                >
                  <X className="h-3 w-3" />
                </button>
              )}
            </span>
          );
        })}
      </div>

      {/* 태그 추가 폼 */}
      {!readOnly && (
        <div className="flex items-center gap-1.5">
          <input
            type="text"
            value={keyInput}
            onChange={(e) => setKeyInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="태그 키 (예: room)"
            className={cn(inputCls, 'max-w-[9rem]')}
            aria-label="태그 키"
          />
          <span className="text-(--color-text-muted)">=</span>
          <input
            type="text"
            value={valInput}
            onChange={(e) => setValInput(e.target.value)}
            onKeyDown={handleKeyDown}
            placeholder="태그 값"
            className={cn(inputCls, 'max-w-[9rem]')}
            aria-label="태그 값"
          />
          <button
            type="button"
            onClick={handleAddClick}
            disabled={keyInput.trim() === ''}
            className={cn(
              'inline-flex items-center gap-0.5 rounded-md border border-(--color-border-default) px-2 py-1 text-[11px] font-medium',
              'text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)',
              'disabled:cursor-not-allowed disabled:opacity-50',
            )}
          >
            <Plus className="h-3 w-3" />
            태그 추가
          </button>
        </div>
      )}
    </div>
  );
}
