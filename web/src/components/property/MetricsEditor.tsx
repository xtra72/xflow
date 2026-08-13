// store-write 노드의 다중 메트릭(metrics) 테이블 에디터.
// 각 행은 하나의 메트릭 = 같은 키(key_template)에 metric_type 로 구분되는 시리즈이며,
// 자체 value_key / data_type / dead-band(min_interval, min_change, min_change_percent)를 갖는다.
// 추가/삭제를 지원하고, 배열(Array<object>) 형태로 직렬화한다.

import { useCallback, useEffect, useRef, useState } from 'react';
import { Plus, Trash2 } from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { useTranslation } from '@/lib/i18n';

// 'auto' 는 백엔드 sentinel 로, 첫 쓰기 값에서 타입을 추론해 시리즈에 고정한다.
// '' 는 data_type 을 아예 전송하지 않는 레거시 동작이며, 그 경우 store 가 해당
// 시리즈를 "동적 = string" 정책으로 등록해 숫자도 문자열로 저장한다
// (internal/agent/system/store.go 의 미등록 키 자동 등록 + stringifyValue).
// 즉 '' 는 auto 가 아니라 "문자열 저장" 이므로 라벨을 분리한다.
const DATA_TYPES = ['auto', '', 'int', 'float', 'string', 'boolean', 'bytes', 'json'] as const;

// ---- 내부 행 타입 (모든 필드 문자열로 보관; 직렬화 시 숫자 변환) ----

interface MetricRow {
  key: string;
  metric_type: string;
  value_key: string;
  data_type: string;
  min_interval: string;
  min_change: string;
  min_change_percent: string;
}

interface MetricsEditorProps {
  value: unknown;
  onChange: (value: unknown) => void;
  readOnly?: boolean;
}

let keyCounter = 0;
function nextKey(): string {
  return `metric-${++keyCounter}-${Date.now()}`;
}

function str(v: unknown): string {
  if (v === undefined || v === null) return '';
  return String(v);
}

/** 입력값(배열) → 내부 행 배열 */
function toRows(value: unknown): MetricRow[] {
  if (!Array.isArray(value)) return [];
  const rows: MetricRow[] = [];
  for (const item of value) {
    if (!item || typeof item !== 'object') continue;
    const m = item as Record<string, unknown>;
    rows.push({
      key: nextKey(),
      metric_type: str(m.metric_type),
      value_key: str(m.value_key),
      data_type: str(m.data_type),
      min_interval: str(m.min_interval),
      min_change: str(m.min_change),
      min_change_percent: str(m.min_change_percent),
    });
  }
  return rows;
}

/** 내부 행 배열 → 출력값(메트릭 객체 배열). 빈 선택 필드는 생략한다. */
function toMetrics(rows: MetricRow[]): Array<Record<string, unknown>> {
  return rows.map((r) => {
    const out: Record<string, unknown> = {};
    if (r.metric_type.trim() !== '') out.metric_type = r.metric_type.trim();
    if (r.value_key.trim() !== '') out.value_key = r.value_key.trim();
    if (r.data_type !== '') out.data_type = r.data_type;
    if (r.min_interval.trim() !== '') out.min_interval = r.min_interval.trim();
    if (r.min_change.trim() !== '') out.min_change = Number(r.min_change);
    if (r.min_change_percent.trim() !== '') out.min_change_percent = Number(r.min_change_percent);
    return out;
  });
}

const cellInput =
  'w-full rounded border border-(--color-border-default) bg-(--color-bg-primary) px-1.5 py-1 text-xs text-(--color-text-primary) focus:border-blue-400 focus:outline-none';
const readOnlyInput = 'cursor-not-allowed opacity-60';

export function MetricsEditor({ value, onChange, readOnly }: MetricsEditorProps) {
  const { t } = useTranslation();
  // rows 를 내부 상태로 유지하여 편집 중 행 key 가 안정적으로 보존되게 한다.
  // useMemo(toRows(value)) 로 매 렌더 재생성하면 nextKey() 가 새 key 를 발급해
  // input 이 리마운트되어 한 글자 입력 후 포커스가 빠지는 문제가 있었다.
  const [rows, setRows] = useState<MetricRow[]>(() => toRows(value));
  const internalUpdate = useRef(false);

  // 외부 value 변경 시에만 내부 동기화 (우리 emit 으로 인한 변경은 건너뜀).
  useEffect(() => {
    if (internalUpdate.current) {
      internalUpdate.current = false;
      return;
    }
    setRows(toRows(value));
  }, [value]);

  const emit = useCallback(
    (updated: MetricRow[]) => {
      setRows(updated);
      internalUpdate.current = true;
      onChange(toMetrics(updated));
    },
    [onChange],
  );

  const handleAdd = useCallback(() => {
    const newRow: MetricRow = {
      key: nextKey(),
      metric_type: '',
      value_key: '',
      // 신규 행 기본값은 auto — '' 로 두면 store 가 동적 string 으로 등록해
      // 숫자 측정값이 문자열로 저장되고 차트/집계에서 전부 제외된다.
      data_type: 'auto',
      min_interval: '',
      min_change: '',
      min_change_percent: '',
    };
    emit([...rows, newRow]);
  }, [rows, emit]);

  const handleRemove = useCallback(
    (key: string) => emit(rows.filter((r) => r.key !== key)),
    [rows, emit],
  );

  const handleChange = useCallback(
    (key: string, field: keyof MetricRow, val: string) => {
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
                {t('property.metrics.metric')}
              </th>
              <th className="px-2 py-1.5 text-left text-xs font-medium text-(--color-text-muted)">
                {t('property.metrics.valueKey')}
              </th>
              <th className="px-2 py-1.5 text-left text-xs font-medium text-(--color-text-muted)">
                {t('property.metrics.type')}
              </th>
              <th className="px-2 py-1.5 text-left text-xs font-medium text-(--color-text-muted)">
                {t('property.metrics.suppressInterval')}
              </th>
              <th className="px-2 py-1.5 text-left text-xs font-medium text-(--color-text-muted)">
                {t('property.metrics.changeAbsolute')}
              </th>
              <th className="px-2 py-1.5 text-left text-xs font-medium text-(--color-text-muted)">
                {t('property.metrics.changePercent')}
              </th>
              {!readOnly && <th className="w-10 px-2 py-1.5" />}
            </tr>
          </thead>
          <tbody className="divide-y divide-(--color-border-default)">
            {rows.length === 0 && (
              <tr>
                <td
                  colSpan={readOnly ? 6 : 7}
                  className="px-2 py-4 text-center text-xs text-(--color-text-muted)"
                >
                  {t('property.metrics.empty')}
                </td>
              </tr>
            )}
            {rows.map((row) => (
              <tr key={row.key} className="transition-colors hover:bg-(--color-bg-elevated)">
                <td className="px-2 py-1">
                  <input
                    type="text"
                    value={row.metric_type}
                    readOnly={readOnly}
                    placeholder="temperature / $.metadata.x"
                    onChange={(e) => handleChange(row.key, 'metric_type', e.target.value)}
                    className={cn(cellInput, 'w-28', readOnly && readOnlyInput)}
                  />
                </td>
                <td className="px-2 py-1">
                  <input
                    type="text"
                    value={row.value_key}
                    readOnly={readOnly}
                    placeholder="$.payload.value"
                    onChange={(e) => handleChange(row.key, 'value_key', e.target.value)}
                    className={cn(cellInput, 'w-36', readOnly && readOnlyInput)}
                  />
                </td>
                <td className="px-2 py-1">
                  <select
                    value={row.data_type}
                    disabled={readOnly}
                    onChange={(e) => handleChange(row.key, 'data_type', e.target.value)}
                    className={cn(cellInput, readOnly && readOnlyInput)}
                  >
                    {DATA_TYPES.map((dt) => (
                      <option key={dt} value={dt}>
                        {dt === '' ? t('property.metrics.unset') : dt === 'auto' ? t('property.metrics.auto') : dt}
                      </option>
                    ))}
                  </select>
                </td>
                <td className="px-2 py-1">
                  <input
                    type="text"
                    value={row.min_interval}
                    readOnly={readOnly}
                    placeholder="30s"
                    onChange={(e) => handleChange(row.key, 'min_interval', e.target.value)}
                    className={cn(cellInput, 'w-20', readOnly && readOnlyInput)}
                  />
                </td>
                <td className="px-2 py-1">
                  <input
                    type="number"
                    value={row.min_change}
                    readOnly={readOnly}
                    placeholder="0.5"
                    onChange={(e) => handleChange(row.key, 'min_change', e.target.value)}
                    className={cn(cellInput, 'w-20', readOnly && readOnlyInput)}
                  />
                </td>
                <td className="px-2 py-1">
                  <input
                    type="number"
                    value={row.min_change_percent}
                    readOnly={readOnly}
                    placeholder="10"
                    onChange={(e) => handleChange(row.key, 'min_change_percent', e.target.value)}
                    className={cn(cellInput, 'w-20', readOnly && readOnlyInput)}
                  />
                </td>
                {!readOnly && (
                  <td className="px-2 py-1">
                    <button
                      type="button"
                      onClick={() => handleRemove(row.key)}
                      className="rounded p-1 text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                      aria-label={t('property.metrics.deleteAria')}
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

      {!readOnly && (
        <button
          type="button"
          onClick={handleAdd}
          className="inline-flex items-center gap-1 rounded-md border border-dashed border-(--color-border-default) px-3 py-1.5 text-xs font-medium text-(--color-text-muted) transition-colors hover:border-blue-400 hover:text-blue-600 dark:hover:border-blue-500 dark:hover:text-blue-400"
        >
          <Plus className="h-3.5 w-3.5" />
          {t('property.metrics.add')}
        </button>
      )}
    </div>
  );
}
