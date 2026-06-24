// 문자열 목록 에디터 컴포넌트.
// string[] 형태의 데이터를 행별로 표시하고 추가/삭제를 지원한다.
// mqtt-subscriber의 토픽 목록 등에 사용한다.

import { useCallback, useEffect, useRef, useState } from 'react';
import { Plus, Trash2 } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';

interface StringListRow {
  key: string;
  value: string;
}

interface StringListEditorProps {
  value: unknown;
  onChange: (value: unknown) => void;
  readOnly?: boolean;
  placeholder?: string;
}

let keyCounter = 0;
function nextKey(): string {
  return `sl-${++keyCounter}-${Date.now()}`;
}

/** unknown → StringListRow[] */
function toRows(val: unknown): StringListRow[] {
  if (!Array.isArray(val)) return [];
  return (val as unknown[]).map((item) => ({
    key: nextKey(),
    value: String(item ?? ''),
  }));
}

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

export function StringListEditor({ value, onChange, readOnly, placeholder }: StringListEditorProps) {
  const { t } = useTranslation();
  // 내부 행 상태 (빈 행 포함)
  const [rows, setRows] = useState<StringListRow[]>(() => toRows(value));
  const internalUpdate = useRef(false);

  // 외부 value 변경 시 내부 동기화 (내부 업데이트가 아닌 경우만)
  useEffect(() => {
    if (internalUpdate.current) {
      internalUpdate.current = false;
      return;
    }
    setRows(toRows(value));
  }, [value]);

  // 내부 상태 변경 → 외부 onChange (빈 문자열 제외)
  const emit = useCallback(
    (updated: StringListRow[]) => {
      setRows(updated);
      internalUpdate.current = true;
      onChange(updated.map((r) => r.value).filter((v) => v !== ''));
    },
    [onChange],
  );

  const handleAdd = useCallback(() => {
    emit([...rows, { key: nextKey(), value: '' }]);
  }, [rows, emit]);

  const handleRemove = useCallback(
    (key: string) => emit(rows.filter((r) => r.key !== key)),
    [rows, emit],
  );

  const handleChange = useCallback(
    (key: string, val: string) => {
      emit(rows.map((r) => (r.key === key ? { ...r, value: val } : r)));
    },
    [rows, emit],
  );

  return (
    <div className="space-y-1.5">
      {rows.length > 0 && (
        <div className="space-y-1">
          {rows.map((row) => (
            <div key={row.key} className="flex items-center gap-1">
              <input
                type="text"
                value={row.value}
                // readOnly attr 사용 — disabled 는 다크모드에서 텍스트를 흐리게 렌더링한다 (commit b4ad829 참조).
                readOnly={readOnly}
                onChange={(e) => handleChange(row.key, e.target.value)}
                className={cn(cellInput, readOnly && readOnlyInput)}
                placeholder={placeholder ?? t('property.stringList.placeholder')}
              />
              {!readOnly && (
                <button
                  type="button"
                  onClick={() => handleRemove(row.key)}
                  className="shrink-0 rounded p-1 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                  aria-label={t('property.common.deleteAria')}
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              )}
            </div>
          ))}
        </div>
      )}

      {rows.length === 0 && (
        <p className="py-2 text-center text-xs text-(--color-text-muted)">
          {t('property.stringList.empty')}
        </p>
      )}

      {!readOnly && (
        <button
          type="button"
          onClick={handleAdd}
          className="inline-flex items-center gap-1 rounded-md border border-dashed border-(--color-border-default) px-3 py-1.5 text-xs font-medium text-(--color-text-muted) transition-colors hover:border-blue-400 hover:text-blue-600 dark:hover:border-blue-500 dark:hover:text-blue-400"
        >
          <Plus className="h-3.5 w-3.5" />
          {t('property.stringList.add')}
        </button>
      )}
    </div>
  );
}
