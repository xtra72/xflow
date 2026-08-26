// `직전값 사용` 채우기의 사용 기간 제한 편집기.
//
// 직전값 채우기는 값이 끊긴 뒤에도 마지막 값을 계속 이어 그린다. 센서가 하루
// 죽어 있어도 하루치 직선이 "그렇게 측정된 값" 과 똑같이 보이므로, 언제까지
// 이어 쓸지 정할 수 있어야 한다.
//
// 미설정이 종전 동작이다 — 기간을 비우면 제한 없이 계속 이어 쓴다.
//
// @spec SPEC-TSDB-004 (빈 구간 처리)

import { useTranslation } from '@/lib/i18n';

import { formatWindowMs } from './panels/charts/seriesRange';

/** 기간 프리셋(ms). 상대 기간 입력과 같은 눈금을 쓴다. */
const PRESETS_MS = [
  60_000, 5 * 60_000, 10 * 60_000, 30 * 60_000, 60 * 60_000, 6 * 60 * 60_000,
  24 * 60 * 60_000,
] as const;

export interface FillPreviousLimitValue {
  maxMs?: number;
  overflow?: '' | 'value';
  overflowValue?: number;
}

function inputClass(): string {
  return 'w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-2 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-500';
}

/**
 * 사용 기간 + 초과 처리 편집기.
 *
 * `fill === 'previous'` 일 때만 렌더하는 것은 호출부 책임이다 — 다른 전략에는
 * 이 설정이 아무 뜻도 없으므로, 여기서 조건을 들고 있으면 호출부가 "왜 안 보이지"
 * 를 이 파일까지 들어와 찾아야 한다.
 */
export function FillPreviousLimitField({
  value,
  onChange,
  testIdPrefix,
}: {
  value: FillPreviousLimitValue;
  onChange: (next: FillPreviousLimitValue) => void;
  testIdPrefix: string;
}): React.ReactElement {
  const { t } = useTranslation();
  const maxMs = value.maxMs ?? 0;
  const isPreset = (PRESETS_MS as readonly number[]).includes(maxMs);

  return (
    <div className="space-y-1.5 rounded-md border border-(--color-border-default) p-2">
      <label className="block text-xs font-medium text-(--color-text-muted)">
        {t('dashboard.chart.fillPrevMaxLabel')}
      </label>
      <select
        value={maxMs === 0 ? '' : isPreset ? String(maxMs) : 'custom'}
        onChange={(e) => {
          const v = e.target.value;
          if (v === '') return onChange({ ...value, maxMs: undefined });
          if (v === 'custom') return onChange({ ...value, maxMs: maxMs || 60_000 });
          onChange({ ...value, maxMs: Number(v) });
        }}
        data-testid={`${testIdPrefix}-max`}
        className={inputClass()}
      >
        {/* 미설정이 기본이고, 그것이 종전 동작(계속 이어 씀)이다. */}
        <option value="">{t('dashboard.chart.fillPrevMaxUnlimited')}</option>
        {PRESETS_MS.map((ms) => (
          <option key={ms} value={ms}>
            {formatWindowMs(ms)}
          </option>
        ))}
        <option value="custom">{t('tsdb.intervalCustom')}</option>
      </select>

      {maxMs > 0 && !isPreset && (
        <label className="flex items-center gap-1 text-[11px] text-(--color-text-muted)">
          <input
            type="number"
            min={1}
            value={Math.round(maxMs / 1000)}
            onChange={(e) => {
              const n = parseInt(e.target.value, 10);
              if (!Number.isNaN(n) && n >= 1) onChange({ ...value, maxMs: n * 1000 });
            }}
            data-testid={`${testIdPrefix}-max-custom`}
            aria-label={t('dashboard.chart.fillPrevMaxLabel')}
            className="w-24 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1 text-right text-[11px] text-(--color-text-primary) outline-none focus:border-blue-500"
          />
          <span>{t('dashboard.chart.storeInfoSecUnit')}</span>
        </label>
      )}

      {/* 초과 처리는 기간을 정했을 때만 뜻이 있다. */}
      {maxMs > 0 && (
        <>
          <label className="block text-xs font-medium text-(--color-text-muted)">
            {t('dashboard.chart.fillPrevOverflowLabel')}
          </label>
          <div className="flex items-center gap-2">
            <select
              value={value.overflow === 'value' ? 'value' : ''}
              onChange={(e) =>
                onChange({
                  ...value,
                  overflow: e.target.value === 'value' ? 'value' : undefined,
                  // 비우기로 되돌리면 값도 같이 지운다 — 남겨 두면 다시 켰을 때
                  // 사용자가 넣은 적 없는 값이 되살아난다.
                  overflowValue: e.target.value === 'value' ? (value.overflowValue ?? 0) : undefined,
                })
              }
              data-testid={`${testIdPrefix}-overflow`}
              className={inputClass()}
            >
              <option value="">{t('dashboard.chart.fillPrevOverflowEmpty')}</option>
              <option value="value">{t('dashboard.chart.fillPrevOverflowValue')}</option>
            </select>
            {value.overflow === 'value' && (
              <input
                type="number"
                value={value.overflowValue ?? 0}
                onChange={(e) => {
                  const n = parseFloat(e.target.value);
                  onChange({ ...value, overflowValue: Number.isNaN(n) ? 0 : n });
                }}
                data-testid={`${testIdPrefix}-overflow-value`}
                aria-label={t('dashboard.chart.fillPrevOverflowValue')}
                className="w-24 shrink-0 rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-2 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-500"
              />
            )}
          </div>
        </>
      )}

      <p className="text-[11px] leading-snug text-(--color-text-muted)">
        {t('dashboard.chart.fillPrevHint')}
      </p>
    </div>
  );
}
