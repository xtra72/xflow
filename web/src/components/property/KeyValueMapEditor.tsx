// Mapping 노드의 키-값 매핑 테이블 에디터.
// Record<string, string> 형태의 매핑 데이터를 테이블 행으로 표시하고,
// 추가/삭제를 지원한다.

import { useCallback, useRef, useState } from 'react';
import { Plus, Trash2 } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';

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
  /** "키" 컬럼 헤더 오버라이드. 미지정 시 "키". */
  keyLabel?: string;
  /** "값" 컬럼 헤더 오버라이드. 미지정 시 "값". */
  valueLabel?: string;
  /** 키 입력 placeholder 오버라이드. 미지정 시 "키". */
  keyPlaceholder?: string;
  /** 값 입력 placeholder 오버라이드. 미지정 시 "값". */
  valuePlaceholder?: string;
  /** true 이면 값 입력 위에 `$.` JSONPath 빠른 삽입 칩을 표시한다(readOnly 아닐 때만). */
  pathHelper?: boolean;
}

// `$.` 빠른 삽입 칩 목록. storage-write 의 태그 값 JSONPath 참조 보조용.
const PATH_HELPER_CHIPS = ['$.payload.', '$.metadata.', '$.type', '$.timestamp'] as const;

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

export function KeyValueMapEditor({
  value,
  onChange,
  readOnly,
  keyLabel,
  valueLabel,
  keyPlaceholder,
  valuePlaceholder,
  pathHelper,
}: KeyValueMapEditorProps) {
  const { t } = useTranslation();
  // 내부 상태로 행을 관리하여 key 안정성을 보장한다.
  // 외부 value는 초기화 시에만 반영한다.
  const lastExternalRef = useRef<unknown>(undefined);
  const [rows, setRows] = useState<KvRow[]>(() => toRows(value));

  // pathHelper 칩 삽입 대상: 마지막으로 포커스된 값 셀의 row key 와 그 input 엘리먼트.
  const focusedValueKeyRef = useRef<string | null>(null);
  const valueInputRefs = useRef<Map<string, HTMLInputElement>>(new Map());

  // 라벨/placeholder 기본값 (미지정 시 기존 "키"/"값" 유지).
  const thKeyLabel = keyLabel ?? t('property.keyValue.keyLabel');
  const thValueLabel = valueLabel ?? t('property.keyValue.valueLabel');
  const phKey = keyPlaceholder ?? t('property.keyValue.keyLabel');
  const phValue = valuePlaceholder ?? t('property.keyValue.valueLabel');

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

  // `$.` 빠른 삽입 칩 클릭 처리.
  //
  // 삽입 대상 우선순위:
  //   1) 마지막으로 포커스된 값 셀 (caret 위치에 삽입, selectionStart 사용)
  //   2) 행이 있으면 마지막 행의 값에 append
  //   3) 행이 없으면 칩 텍스트를 값으로 갖는 새 행 추가
  const handleChipInsert = useCallback(
    (chip: string) => {
      const focusedKey = focusedValueKeyRef.current;
      const targetRow = focusedKey
        ? rows.find((r) => r.key === focusedKey)
        : rows.length > 0
          ? rows[rows.length - 1]
          : undefined;

      // 행이 없으면 새 행을 만들어 칩 텍스트를 값으로 사용.
      if (!targetRow) {
        const newRow: KvRow = { key: nextKey(), mapKey: '', mapValue: chip };
        emit([...rows, newRow]);
        return;
      }

      // caret 위치 결정: 포커스된 셀이면 selectionStart, 아니면 끝(append).
      const inputEl =
        focusedKey === targetRow.key ? valueInputRefs.current.get(targetRow.key) : undefined;
      const current = targetRow.mapValue;
      const caret =
        inputEl && inputEl.selectionStart != null ? inputEl.selectionStart : current.length;
      const nextValue = current.slice(0, caret) + chip + current.slice(caret);

      emit(rows.map((r) => (r.key === targetRow.key ? { ...r, mapValue: nextValue } : r)));

      // 삽입 후 포커스/caret 을 삽입 끝으로 복원한다.
      const restoreEl = valueInputRefs.current.get(targetRow.key);
      if (restoreEl) {
        const nextCaret = caret + chip.length;
        requestAnimationFrame(() => {
          restoreEl.focus();
          try {
            restoreEl.setSelectionRange(nextCaret, nextCaret);
          } catch {
            // setSelectionRange 미지원 환경(테스트 등)에서는 무시.
          }
        });
      }
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
                {thKeyLabel}
              </th>
              <th className="px-2 py-1.5 text-left text-xs font-medium text-(--color-text-muted)">
                {thValueLabel}
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
                  {t('property.keyValue.empty')}
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
                    placeholder={phKey}
                  />
                </td>

                {/* 값 */}
                <td className="px-2 py-1">
                  <input
                    type="text"
                    value={row.mapValue}
                    // pathHelper 칩 삽입 대상 추적을 위해 값 input 참조를 보관한다.
                    ref={(el) => {
                      if (el) valueInputRefs.current.set(row.key, el);
                      else valueInputRefs.current.delete(row.key);
                    }}
                    // 마지막으로 포커스된 값 셀을 기록 — 칩 삽입 대상 결정에 사용.
                    onFocus={() => {
                      focusedValueKeyRef.current = row.key;
                    }}
                    // readOnly attr 사용 — disabled 는 다크모드에서 텍스트를 흐리게 렌더링한다 (commit b4ad829 참조).
                    readOnly={readOnly}
                    onChange={(e) => handleChange(row.key, 'mapValue', e.target.value)}
                    className={cn(cellInput, readOnly && readOnlyInput)}
                    placeholder={phValue}
                  />
                </td>

                {/* 삭제 */}
                {!readOnly && (
                  <td className="px-2 py-1">
                    <button
                      type="button"
                      onClick={() => handleRemove(row.key)}
                      className="rounded p-1 text-(--color-text-muted) transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                      aria-label={t('property.common.deleteAria')}
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

      {/* `$.` JSONPath 빠른 삽입 칩 (pathHelper 이고 읽기 전용이 아닐 때만) */}
      {pathHelper && !readOnly && (
        <div className="flex flex-wrap items-center gap-1.5" data-testid="path-helper-chips">
          <span className="text-xs text-(--color-text-muted)">{t('property.keyValue.quickInsert')}</span>
          {PATH_HELPER_CHIPS.map((chip) => (
            <button
              key={chip}
              type="button"
              // mousedown 에서 preventDefault — 클릭으로 인한 input blur 를 막아
              // 포커스된 값 셀 정보를 유지한 채 삽입할 수 있게 한다.
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => handleChipInsert(chip)}
              className="rounded border border-(--color-border-default) px-1.5 py-0.5 font-mono text-xs text-(--color-text-secondary) transition-colors hover:border-blue-400 hover:text-blue-600 dark:hover:border-blue-500 dark:hover:text-blue-400"
            >
              {chip}
            </button>
          ))}
        </div>
      )}

      {/* 추가 버튼 */}
      {!readOnly && (
        <button
          type="button"
          onClick={handleAdd}
          className="inline-flex items-center gap-1 rounded-md border border-dashed border-(--color-border-default) px-3 py-1.5 text-xs font-medium text-(--color-text-muted) transition-colors hover:border-blue-400 hover:text-blue-600 dark:hover:border-blue-500 dark:hover:text-blue-400"
        >
          <Plus className="h-3.5 w-3.5" />
          {t('property.keyValue.addItem')}
        </button>
      )}
    </div>
  );
}
