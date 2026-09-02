// 테이블 열의 **범위 필터** 버튼 + 팝오버 (하한/상한).
//
// value(number) · timestamp(datetime) 처럼 값이 행마다 거의 다 다른 열은 값 목록으로
// 고를 수 없다 — 행 수만큼 선택지가 생긴다. 이런 열은 하한·상한으로 거른다.
//
// 시각적 언어(트리거 아이콘 · 활성 점 · fixed 앵커 팝오버)는 데이터 소스 시리즈
// 리스트의 값 선택 필터(`ColumnFilterButton`)와 맞춘다 — 같은 표 안에서 두 종류
// 필터가 서로 다르게 생기면 어느 쪽이 필터인지 알아보기 어렵다.

import { useEffect, useRef, useState } from 'react';
import { ListFilter } from 'lucide-react';

import type { TranslationFn } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import type { TableColumnFormat } from './chartChannelTypes';
import {
  emptyTableColumnFilter,
  epochToLocalInput,
  localInputToEpoch,
  type TableColumnFilter,
} from './tableColumns';

const POPOVER_WIDTH = 224;

export function TableRangeFilterButton({
  label,
  format,
  filter,
  onChange,
  t,
}: {
  label: string;
  format: TableColumnFormat | undefined;
  filter: TableColumnFilter | undefined;
  onChange: (next: TableColumnFilter) => void;
  t: TranslationFn;
}) {
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const btnRef = useRef<HTMLButtonElement>(null);
  const [coords, setCoords] = useState<{ left: number; top: number }>({ left: 0, top: 0 });

  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [open]);

  const current = filter ?? emptyTableColumnFilter();
  const active = current.min !== undefined || current.max !== undefined;
  const isTime = format === 'datetime';

  // 팝오버가 패널의 overflow 에 잘리지 않도록 fixed 로 버튼 아래에 앵커링하고
  // 뷰포트 안으로 클램프한다(값 선택 필터와 같은 방식).
  const toggleOpen = (): void => {
    setOpen((o) => {
      const next = !o;
      if (next && btnRef.current) {
        const r = btnRef.current.getBoundingClientRect();
        const vw = typeof window !== 'undefined' ? window.innerWidth : POPOVER_WIDTH + 16;
        setCoords({
          left: Math.max(8, Math.min(r.left, vw - POPOVER_WIDTH - 8)),
          top: r.bottom + 4,
        });
      }
      return next;
    });
  };

  const setBound = (which: 'min' | 'max', raw: string): void => {
    const parsed = isTime
      ? localInputToEpoch(raw)
      : raw.trim() === ''
        ? undefined
        : Number(raw);
    const value = parsed !== undefined && Number.isFinite(parsed) ? parsed : undefined;
    onChange({ ...current, values: new Set(current.values), [which]: value });
  };

  const inputClass = cn(
    'w-full rounded border border-(--color-border-default) bg-(--color-bg-elevated)',
    'px-1 py-0.5 text-xs text-(--color-text-primary) outline-none focus:border-blue-500',
  );

  return (
    <div ref={containerRef} className="relative inline-flex">
      <button
        ref={btnRef}
        type="button"
        onClick={toggleOpen}
        aria-haspopup="true"
        aria-expanded={open}
        className={cn(
          'inline-flex items-center rounded p-0.5 transition-colors',
          active
            ? 'text-blue-600 dark:text-blue-400'
            : 'text-(--color-text-muted) opacity-50 hover:opacity-100 hover:text-(--color-text-primary)',
        )}
        title={t('dashboard.chart.rangeFilterAria').replace('{label}', label)}
        aria-label={t('dashboard.chart.rangeFilterAria').replace('{label}', label)}
      >
        <ListFilter className="h-3 w-3" aria-hidden="true" />
        {active && (
          <span
            className="ml-0.5 inline-block h-1.5 w-1.5 rounded-full bg-blue-600 dark:bg-blue-400"
            aria-hidden="true"
          />
        )}
      </button>
      {open && (
        <div
          style={{ position: 'fixed', left: coords.left, top: coords.top, width: POPOVER_WIDTH }}
          className="z-50 rounded-md border border-(--color-border-default) bg-(--color-bg-primary) p-2 text-left shadow-lg"
        >
          <div className="space-y-1.5">
            <label className="block text-xs font-normal text-(--color-text-muted)">
              {t('dashboard.chart.rangeMin')}
              <input
                type={isTime ? 'datetime-local' : 'number'}
                value={isTime ? epochToLocalInput(current.min) : (current.min ?? '')}
                onChange={(e) => setBound('min', e.target.value)}
                aria-label={t('dashboard.chart.rangeMin')}
                className={inputClass}
              />
            </label>
            <label className="block text-xs font-normal text-(--color-text-muted)">
              {t('dashboard.chart.rangeMax')}
              <input
                type={isTime ? 'datetime-local' : 'number'}
                value={isTime ? epochToLocalInput(current.max) : (current.max ?? '')}
                onChange={(e) => setBound('max', e.target.value)}
                aria-label={t('dashboard.chart.rangeMax')}
                className={inputClass}
              />
            </label>
            <button
              type="button"
              onClick={() => onChange(emptyTableColumnFilter())}
              className="w-full rounded px-1 py-0.5 text-left text-xs font-normal text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-primary)"
            >
              {t('dashboard.chart.rangeClear')}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
