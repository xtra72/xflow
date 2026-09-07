// 라우트(switch) 테이블 에디터 컴포넌트 (SPEC-SWITCH-001).
// switch 노드의 routes 설정을 (조건식, 출력 포트명) 행 단위로 편집한다.
// 각 행: { condition: string, name: string }.
// 결과: Array<{ name: string, condition: string }> — 순서 보존(백엔드 first-match 의존).
//
// 백엔드 계약(SPEC-SWITCH-001 §5.2):
//   { name: "hot", condition: "$.payload.temp >= 30" }
//   name = 출력 포트 이름(= 와이어 SourcePort = _target_port).
//   condition = compileCondition 표현식.
// 순서가 의미를 가지므로 key→value 맵이 아닌 순서 있는 배열 에디터를 사용한다.

import { useCallback, useEffect, useRef, useState } from 'react';
import { Plus, Trash2 } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';

interface RouteRow {
  /** React 렌더링용 안정 키 (config 에는 포함되지 않음). */
  key: string;
  /** 조건식 (compileCondition 표현식). */
  condition: string;
  /** 출력 포트 이름. */
  name: string;
}

interface RoutesEditorProps {
  value: unknown;
  onChange: (value: unknown) => void;
  readOnly?: boolean;
}

let keyCounter = 0;
function nextKey(): string {
  return `rt-${++keyCounter}-${Date.now()}`;
}

/** unknown → RouteRow[]. 객체 배열만 행으로 변환하고, 순서를 그대로 보존한다. */
function toRows(val: unknown): RouteRow[] {
  if (!Array.isArray(val)) return [];
  return val.map((item) => {
    if (item && typeof item === 'object') {
      const obj = item as { name?: unknown; condition?: unknown };
      const name = typeof obj.name === 'string' ? obj.name : '';
      const condition = typeof obj.condition === 'string' ? obj.condition : '';
      return { key: nextKey(), name, condition };
    }
    return { key: nextKey(), name: '', condition: '' };
  });
}

/**
 * 행 배열 → 백엔드가 받는 객체 배열.
 * name/condition 중 하나라도 비어있지 않은 행을 순서대로 내보낸다.
 * (완전히 빈 행은 편집 중 임시 행이므로 emit 에서 제외한다.)
 * 키 순서는 {name, condition} 으로 — 계약 §5.2 와 동일.
 */
function toEmitValue(rows: RouteRow[]): Array<{ name: string; condition: string }> {
  const out: Array<{ name: string; condition: string }> = [];
  for (const r of rows) {
    const name = r.name.trim();
    const condition = r.condition.trim();
    if (name === '' && condition === '') continue;
    out.push({ name, condition });
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

export function RoutesEditor({ value, onChange, readOnly }: RoutesEditorProps) {
  const { t } = useTranslation();
  const [rows, setRows] = useState<RouteRow[]>(() => toRows(value));
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
    (updated: RouteRow[]) => {
      setRows(updated);
      internalUpdate.current = true;
      onChange(toEmitValue(updated));
    },
    [onChange],
  );

  const handleAdd = useCallback(() => {
    emit([...rows, { key: nextKey(), condition: '', name: '' }]);
  }, [rows, emit]);

  const handleRemove = useCallback(
    (key: string) => emit(rows.filter((r) => r.key !== key)),
    [rows, emit],
  );

  const handleConditionChange = useCallback(
    (key: string, condition: string) => {
      emit(rows.map((r) => (r.key === key ? { ...r, condition } : r)));
    },
    [rows, emit],
  );

  const handleNameChange = useCallback(
    (key: string, name: string) => {
      emit(rows.map((r) => (r.key === key ? { ...r, name } : r)));
    },
    [rows, emit],
  );

  return (
    <div className="space-y-1.5">
      {rows.length > 0 && (
        <>
          {/* 헤더 */}
          <div className="flex items-center gap-1 px-1 text-[10px] font-medium uppercase tracking-wide text-(--color-text-muted)">
            <span className="flex-1">{t('property.routes.condition')}</span>
            <span className="w-28">{t('property.routes.outputPort')}</span>
            {!readOnly && <span className="w-6" />}
          </div>
          <div className="space-y-1">
            {rows.map((row) => (
              <div key={row.key} className="flex items-center gap-1">
                <input
                  type="text"
                  value={row.condition}
                  readOnly={readOnly}
                  onChange={(e) => handleConditionChange(row.key, e.target.value)}
                  className={cn(cellInput, 'flex-1 font-mono text-xs', readOnly && readOnlyInput)}
                  placeholder="$.payload.temp >= 30"
                  aria-label={t('property.routes.condition')}
                />
                <input
                  type="text"
                  value={row.name}
                  readOnly={readOnly}
                  onChange={(e) => handleNameChange(row.key, e.target.value)}
                  className={cn(cellInput, 'w-28', readOnly && readOnlyInput)}
                  placeholder="hot"
                  aria-label={t('property.routes.outputPort')}
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
          {t('property.routes.empty')}
        </p>
      )}

      {!readOnly && (
        <button
          type="button"
          onClick={handleAdd}
          className="inline-flex items-center gap-1 rounded-md border border-dashed border-(--color-border-default) px-3 py-1.5 text-xs font-medium text-(--color-text-muted) transition-colors hover:border-blue-400 hover:text-blue-600 dark:hover:border-blue-500 dark:hover:text-blue-400"
        >
          <Plus className="h-3.5 w-3.5" />
          {t('property.routes.addRoute')}
        </button>
      )}
    </div>
  );
}
