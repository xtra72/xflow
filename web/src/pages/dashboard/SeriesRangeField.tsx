// 가져올 데이터 범위 편집기 — 기간(상대·절대) 또는 갯수.
//
// Store · TSDB 데이터 소스가 **같은 컴포넌트**를 쓴다. 소스마다 따로 만들면 같은
// 개념이 화면마다 다르게 생겨, 소스를 바꿀 때 설정을 옮겨 적기 어렵다.

import { useTranslation } from '@/lib/i18n';

import {
  epochToLocalInput,
  localInputToEpoch,
} from './panels/charts/tableColumns';
import {
  DEFAULT_RANGE_COUNT,
  formatWindowMs,
  MAX_RANGE_COUNT,
  type SeriesRange,
  type SeriesRangeMode,
} from './panels/charts/seriesRange';

const MODES: SeriesRangeMode[] = ['relative', 'absolute', 'count'];

const MODE_LABEL_KEY: Record<SeriesRangeMode, string> = {
  relative: 'dashboard.chart.rangeModeRelative',
  absolute: 'dashboard.chart.rangeModeAbsolute',
  count: 'dashboard.chart.rangeModeCount',
};

/** 상대 기간 프리셋(ms). 인터벌 프리셋과 달리 "지난 N" 감각의 눈금이다. */
const WINDOW_PRESETS_MS = [
  5 * 60_000, 15 * 60_000, 30 * 60_000, 60 * 60_000, 6 * 60 * 60_000,
  12 * 60 * 60_000, 24 * 60 * 60_000, 7 * 24 * 60 * 60_000,
] as const;

function inputClass(): string {
  return 'w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500';
}

export function SeriesRangeField({
  range,
  onChange,
  testIdPrefix,
}: {
  range: SeriesRange;
  onChange: (next: SeriesRange) => void;
  /** `chart-store` / `chart-tsdb` — 두 소스의 컨트롤을 구분하기 위한 접두사. */
  testIdPrefix: string;
}): React.ReactElement {
  const { t } = useTranslation();

  // 방식을 바꿔도 다른 방식의 값은 지우지 않는다 — 되돌렸을 때 이전 설정이 살아난다.
  const setMode = (mode: SeriesRangeMode): void => {
    if (mode === range.mode) return;
    const next: SeriesRange = { ...range, mode };
    // 처음 그 방식으로 갈 때만 기본값을 채운다.
    if (mode === 'count' && !(next.count && next.count > 0)) next.count = DEFAULT_RANGE_COUNT;
    if (mode === 'relative' && !(next.window_ms && next.window_ms > 0)) {
      next.window_ms = 60 * 60_000;
    }
    onChange(next);
  };

  const windowMs = range.window_ms ?? 0;
  const windowIsPreset = (WINDOW_PRESETS_MS as readonly number[]).includes(windowMs);

  return (
    <div className="space-y-1.5">
      <label className="block text-xs font-medium text-(--color-text-muted)">
        {t('dashboard.chart.rangeLabel')}
      </label>

      {/* 방식 선택 — 데이터소스 토글과 같은 모양의 세그먼트 컨트롤. */}
      <div
        className="inline-flex rounded-md border border-(--color-border-default) bg-(--color-bg-surface) p-0.5"
        role="tablist"
        aria-label={t('dashboard.chart.rangeLabel')}
      >
        {MODES.map((m) => {
          const selected = range.mode === m;
          return (
            <button
              key={m}
              type="button"
              role="tab"
              aria-selected={selected}
              data-testid={`${testIdPrefix}-range-mode-${m}`}
              onClick={() => setMode(m)}
              className={`rounded px-2.5 py-1 text-xs font-medium transition-colors ${
                selected
                  ? 'bg-blue-600 text-white'
                  : 'text-(--color-text-secondary) hover:bg-(--color-bg-elevated)'
              }`}
            >
              {t(MODE_LABEL_KEY[m])}
            </button>
          );
        })}
      </div>

      {range.mode === 'relative' && (
        <div className="space-y-1">
          <select
            data-testid={`${testIdPrefix}-range-window`}
            value={windowIsPreset ? String(windowMs) : 'custom'}
            onChange={(e) => {
              const v = e.target.value;
              if (v === 'custom') return;
              onChange({ ...range, window_ms: Number(v) });
            }}
            aria-label={t('dashboard.chart.rangeModeRelative')}
            className={inputClass()}
          >
            {WINDOW_PRESETS_MS.map((ms) => (
              <option key={ms} value={ms}>
                {formatWindowMs(ms)}
              </option>
            ))}
            <option value="custom">{t('tsdb.intervalCustom')}</option>
          </select>
          {!windowIsPreset && (
            <label className="flex items-center gap-1 text-[11px] text-(--color-text-muted)">
              <input
                type="number"
                min={1}
                data-testid={`${testIdPrefix}-range-window-custom`}
                value={Math.round(windowMs / 1000)}
                aria-label={t('dashboard.chart.rangeWindowCustomAria')}
                onChange={(e) => {
                  const sec = Number(e.target.value);
                  if (!Number.isFinite(sec) || sec <= 0) return;
                  onChange({ ...range, window_ms: Math.round(sec) * 1000 });
                }}
                className="w-20 rounded border border-(--color-border-default) bg-(--color-bg-surface) px-1 py-0.5 text-[11px]"
              />
              <span>{t('dashboard.chart.storeInfoSecUnit')}</span>
            </label>
          )}
        </div>
      )}

      {range.mode === 'absolute' && (
        <div className="space-y-1">
          <label className="block text-[11px] text-(--color-text-muted)">
            {t('dashboard.chart.rangeFrom')}
            <input
              type="datetime-local"
              data-testid={`${testIdPrefix}-range-start`}
              value={epochToLocalInput(range.start_ms)}
              aria-label={t('dashboard.chart.rangeFrom')}
              onChange={(e) => onChange({ ...range, start_ms: localInputToEpoch(e.target.value) })}
              className={inputClass()}
            />
          </label>
          <label className="block text-[11px] text-(--color-text-muted)">
            {t('dashboard.chart.rangeTo')}
            <input
              type="datetime-local"
              data-testid={`${testIdPrefix}-range-end`}
              value={epochToLocalInput(range.end_ms)}
              aria-label={t('dashboard.chart.rangeTo')}
              onChange={(e) => onChange({ ...range, end_ms: localInputToEpoch(e.target.value) })}
              className={inputClass()}
            />
          </label>
          {/* 앞뒤가 뒤집혔거나 비어 있으면 조회가 서지 않는다 — 왜 비었는지 알려준다. */}
          {!(
            (range.start_ms ?? 0) > 0 &&
            (range.end_ms ?? 0) > 0 &&
            (range.end_ms ?? 0) >= (range.start_ms ?? 0)
          ) && (
            <p
              data-testid={`${testIdPrefix}-range-absolute-warning`}
              className="text-[11px] leading-snug text-amber-600 dark:text-amber-400"
            >
              {t('dashboard.chart.rangeAbsoluteInvalid')}
            </p>
          )}
        </div>
      )}

      {range.mode === 'count' && (
        <div className="space-y-1">
          <label className="flex items-center gap-1 text-[11px] text-(--color-text-muted)">
            <input
              type="number"
              min={1}
              max={MAX_RANGE_COUNT}
              data-testid={`${testIdPrefix}-range-count`}
              value={range.count ?? ''}
              aria-label={t('dashboard.chart.rangeModeCount')}
              onChange={(e) => {
                const n = Number(e.target.value);
                if (!Number.isFinite(n) || n <= 0) return;
                onChange({ ...range, count: Math.min(Math.round(n), MAX_RANGE_COUNT) });
              }}
              className="w-24 rounded border border-(--color-border-default) bg-(--color-bg-surface) px-1 py-0.5 text-[11px]"
            />
            <span>{t('dashboard.chart.rangeCountUnit')}</span>
          </label>
          {/* 갯수는 인터벌 × N 으로 구간을 역산해 조회한다 — 인터벌이 함께 읽혀야 뜻이 선다. */}
          <p className="text-[11px] leading-snug text-(--color-text-muted)">
            {t('dashboard.chart.rangeCountHint')}
          </p>
        </div>
      )}
    </div>
  );
}
