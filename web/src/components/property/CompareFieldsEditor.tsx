// 비교 필드 테이블 에디터 컴포넌트 (v0.18.4).
// deduplicate 노드의 compare_fields 설정을 테이블 형식으로 편집.
// 각 행: { name: string, tolerance?: number }.
// 결과: Array<{ name: string, tolerance?: number }>.
// 레거시 문자열 ("a:0.5, b") 도 초기값으로 받아 파싱한다.

import { useCallback, useEffect, useRef, useState } from 'react';
import { Plus, Trash2 } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';

interface CompareFieldRow {
  key: string;
  name: string;
  /** 빈 문자열이면 tolerance 미지정 (=완전 일치). */
  tolerance: string;
}

interface CompareFieldsEditorProps {
  value: unknown;
  onChange: (value: unknown) => void;
  readOnly?: boolean;
}

let keyCounter = 0;
function nextKey(): string {
  return `cf-${++keyCounter}-${Date.now()}`;
}

/** 레거시 문자열 형식 "name:tol, name2" 를 행으로 변환. */
function parseLegacyString(s: string): CompareFieldRow[] {
  return s
    .split(',')
    .map((p) => p.trim())
    .filter((p) => p.length > 0)
    .map((p) => {
      const idx = p.indexOf(':');
      if (idx >= 0) {
        return { key: nextKey(), name: p.slice(0, idx).trim(), tolerance: p.slice(idx + 1).trim() };
      }
      return { key: nextKey(), name: p, tolerance: '' };
    });
}

/** unknown → CompareFieldRow[]. 배열 / 레거시 string / 그 외(빈 배열) 모두 지원. */
function toRows(val: unknown): CompareFieldRow[] {
  if (typeof val === 'string') {
    return parseLegacyString(val);
  }
  if (!Array.isArray(val)) return [];
  return val.map((item) => {
    if (item && typeof item === 'object') {
      const obj = item as { name?: unknown; tolerance?: unknown };
      const name = typeof obj.name === 'string' ? obj.name : '';
      let tolerance = '';
      if (typeof obj.tolerance === 'number' && Number.isFinite(obj.tolerance)) {
        tolerance = String(obj.tolerance);
      } else if (typeof obj.tolerance === 'string') {
        tolerance = obj.tolerance;
      }
      return { key: nextKey(), name, tolerance };
    }
    return { key: nextKey(), name: String(item ?? ''), tolerance: '' };
  });
}

/** 행 배열 → backend 가 받는 객체 배열. 빈 name 은 제외, 유효한 tolerance 만 number 로. */
function toEmitValue(rows: CompareFieldRow[]): Array<{ name: string; tolerance?: number }> {
  const out: Array<{ name: string; tolerance?: number }> = [];
  for (const r of rows) {
    const name = r.name.trim();
    if (name === '') continue;
    const entry: { name: string; tolerance?: number } = { name };
    if (r.tolerance.trim() !== '') {
      const n = Number(r.tolerance);
      if (Number.isFinite(n)) entry.tolerance = Math.abs(n);
    }
    out.push(entry);
  }
  return out;
}

const cellInput = cn(
  'w-full rounded border px-2 py-1 text-sm',
  'border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-primary)',
  'focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400',
  'dark:focus:border-blue-500',
);

const readOnlyInput = 'cursor-not-allowed bg-(--color-bg-elevated)';

export function CompareFieldsEditor({ value, onChange, readOnly }: CompareFieldsEditorProps) {
  const { t } = useTranslation();
  const [rows, setRows] = useState<CompareFieldRow[]>(() => toRows(value));
  const internalUpdate = useRef(false);

  // 외부 value 변경 시 내부 동기화 (내부 업데이트가 아닌 경우만).
  useEffect(() => {
    if (internalUpdate.current) {
      internalUpdate.current = false;
      return;
    }
    setRows(toRows(value));
  }, [value]);

  const emit = useCallback(
    (updated: CompareFieldRow[]) => {
      setRows(updated);
      internalUpdate.current = true;
      onChange(toEmitValue(updated));
    },
    [onChange],
  );

  const handleAdd = useCallback(() => {
    emit([...rows, { key: nextKey(), name: '', tolerance: '' }]);
  }, [rows, emit]);

  const handleRemove = useCallback(
    (key: string) => emit(rows.filter((r) => r.key !== key)),
    [rows, emit],
  );

  const handleNameChange = useCallback(
    (key: string, name: string) => {
      emit(rows.map((r) => (r.key === key ? { ...r, name } : r)));
    },
    [rows, emit],
  );

  const handleToleranceChange = useCallback(
    (key: string, tolerance: string) => {
      emit(rows.map((r) => (r.key === key ? { ...r, tolerance } : r)));
    },
    [rows, emit],
  );

  return (
    <div className="space-y-1.5">
      {rows.length > 0 && (
        <>
          {/* 헤더 */}
          <div className="flex items-center gap-1 px-1 text-[10px] font-medium uppercase tracking-wide text-(--color-text-muted)">
            <span className="flex-1">{t('property.compareFields.fieldName')}</span>
            <span className="w-24">{t('property.compareFields.tolerance')}</span>
            {!readOnly && <span className="w-6" />}
          </div>
          <div className="space-y-1">
            {rows.map((row) => (
              <div key={row.key} className="flex items-center gap-1">
                <input
                  type="text"
                  value={row.name}
                  readOnly={readOnly}
                  onChange={(e) => handleNameChange(row.key, e.target.value)}
                  className={cn(cellInput, 'flex-1', readOnly && readOnlyInput)}
                  placeholder={t('property.compareFields.namePlaceholder')}
                />
                <input
                  type="number"
                  step="any"
                  min="0"
                  value={row.tolerance}
                  readOnly={readOnly}
                  onChange={(e) => handleToleranceChange(row.key, e.target.value)}
                  className={cn(cellInput, 'w-24', readOnly && readOnlyInput)}
                  placeholder={t('property.compareFields.tolerancePlaceholder')}
                />
                {!readOnly && (
                  <button
                    type="button"
                    onClick={() => handleRemove(row.key)}
                    className="shrink-0 rounded p-1 text-(--color-text-muted) transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                    aria-label={t('property.common.deleteAria')}
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                )}
              </div>
            ))}
          </div>
        </>
      )}

      {rows.length === 0 && (
        <p className="py-2 text-center text-xs text-(--color-text-muted)">
          {t('property.compareFields.empty')}
        </p>
      )}

      {!readOnly && (
        <button
          type="button"
          onClick={handleAdd}
          className="inline-flex items-center gap-1 rounded-md border border-dashed border-(--color-border-default) px-3 py-1.5 text-xs font-medium text-(--color-text-muted) transition-colors hover:border-blue-400 hover:text-blue-600 dark:hover:border-blue-500 dark:hover:text-blue-400"
        >
          <Plus className="h-3.5 w-3.5" />
          {t('property.compareFields.addField')}
        </button>
      )}
    </div>
  );
}
