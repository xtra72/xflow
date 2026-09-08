// 글자 스타일 편집 칸 — 글꼴 · 크기 · 색 · 굵기 · 정렬.
//
// `ChartPanelSections`(설정 다이얼로그)에서 떼어 낸다. 통계 패널의 요소 스타일
// 팝오버(SPEC-CHART-004)가 이 컴포넌트를 쓰는데, 패널 쪽에서 설정 모듈을 import 하면
// 3,900줄짜리 모듈이 패널 번들 청크로 딸려 온다 — 순환 참조는 아니지만(설정 모듈은
// 패널을 import 하지 않는다) 번들 방향이 뒤집혀, 대시보드만 여는 사용자가 설정
// 다이얼로그 코드를 내려받게 된다.
//
// 함께 옮긴 `LabeledField` / `inputClass` 는 이 컴포넌트가 쓰는 두 헬퍼다. 설정 모듈은
// 여기서 다시 import 해 종전 그대로 쓴다.
//
// @spec SPEC-CHART-004 §5 D5

import React from 'react';

import { useTranslation } from '@/lib/i18n';

import {
  FONT_FAMILY_OPTIONS,
  TEXT_ALIGN_OPTIONS,
  type ChartFontFamily,
  type ChartTextAlign,
} from './panels/charts/textStyle';

/**
 * 글자 크기 칸의 허용 범위(px).
 *
 * 상한이 160 인 이유: 통계 패널의 본값은 기본 36px 이고 배율 3까지 커진다(108px).
 * 종전 상한 40 은 축 라벨·범례를 전제한 값이라 통계 본값을 담지 못했다.
 * 상한은 **입력 가능 범위**일 뿐 기본값이 아니므로 저장된 값은 영향을 받지 않는다.
 */
export const FONT_SIZE_MIN = 6;
export const FONT_SIZE_MAX = 160;

// --- 공용 입력 헬퍼 ---

export function LabeledField(props: { label: string; children: React.ReactNode; hint?: string }): React.ReactElement {
  return (
    <div>
      <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
        {props.label}
      </label>
      {props.children}
      {props.hint && (
        <p className="mt-1 text-[10px] leading-snug text-(--color-text-muted)">{props.hint}</p>
      )}
    </div>
  );
}

export function inputClass(): string {
  return 'w-full rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-1.5 text-sm text-(--color-text-primary) outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500';
}

export function TextStyleFields({
  label,
  family,
  size,
  color,
  showColor = true,
  weight,
  align,
  sizePlaceholder,
  testIdPrefix,
  onChange,
}: {
  label: string;
  family: ChartFontFamily | undefined;
  size: number | undefined;
  color: string | undefined;
  /**
   * 색 칸을 그릴지. 기본은 그린다.
   *
   * `color === undefined` 로는 감출 수 없다 — 그 값은 이미 **상속**을 뜻하기 때문이다
   * (미지정과 "칸 자체가 없음" 은 다른 상태다). 색이 다른 축에 속한 대상에서 쓴다:
   * 통계 패널의 변화량은 색을 방향(증가/감소/변화없음)이 정하므로 정적 글자색 칸을
   * 두면 두 축이 싸운다(SPEC-CHART-004 §5 D4).
   */
  showColor?: boolean;
  /**
   * 굵기. **`undefined` 를 넘기면 굵기 칸 자체를 그리지 않는다** — 범례·라벨처럼 굵기를
   * 고르지 않는 대상에 빈 칸이 생기면 무엇을 고르는 자리인지 읽히지 않는다.
   */
  weight?: 'normal' | 'bold' | 'inherit';
  /**
   * 가로 정렬. 굵기와 같은 규칙 — **`undefined` 를 넘기면 정렬 칸을 그리지 않는다.**
   * 정렬이 먹지 않는 자리(SVG 텍스트 등)에 칸만 생기면 고른 대로 되지 않는다.
   */
  align?: ChartTextAlign | 'inherit';
  sizePlaceholder: string;
  testIdPrefix: string;
  onChange: (patch: {
    family?: ChartFontFamily | undefined;
    size?: number | undefined;
    color?: string | undefined;
    weight?: 'normal' | 'bold' | undefined;
    align?: ChartTextAlign | undefined;
  }) => void;
}): React.ReactElement {
  const { t } = useTranslation();
  return (
    <LabeledField label={label}>
      {/*
        두 줄로 나눈다. 다섯 칸을 한 줄에 두면 고정폭(크기 64 + 굵기 80 + 색 28 + 되돌리기
        28 + 간격)만 220px 을 넘어, 디자인 팝오버(w-72) 안에서 글꼴 칸이 짜부라지고
        마지막 칸이 상자 밖으로 밀려난다.
      */}
      <div className="space-y-1.5">
        <select
          value={family ?? ''}
          data-testid={`${testIdPrefix}-family`}
          aria-label={`${label} ${t('dashboard.chart.fontFamily')}`}
          onChange={(e) =>
            onChange({ family: (e.target.value || undefined) as ChartFontFamily | undefined })
          }
          className={`${inputClass()} w-full`}
        >
          <option value="">{t('dashboard.chart.inherit')}</option>
          {FONT_FAMILY_OPTIONS.map((o) => (
            <option key={o.value} value={o.value}>
              {t(o.labelKey)}
            </option>
          ))}
        </select>
        <div className="flex items-center gap-1.5">
        <input
          type="number"
          min={FONT_SIZE_MIN}
          max={FONT_SIZE_MAX}
          value={size ?? ''}
          placeholder={sizePlaceholder}
          data-testid={`${testIdPrefix}-size`}
          aria-label={`${label} ${t('dashboard.chart.fontSize')}`}
          onChange={(e) => {
            const v = e.target.value;
            onChange({ size: v === '' ? undefined : parseInt(v, 10) || undefined });
          }}
          className="w-16 shrink-0 rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-1 text-center text-xs text-(--color-text-primary) outline-none focus:border-blue-500"
        />
        {showColor && (
          <input
            type="color"
            value={color ?? '#9ca3af'}
            data-testid={`${testIdPrefix}-color`}
            aria-label={`${label} ${t('dashboard.chart.fontColor')}`}
            onChange={(e) => onChange({ color: e.target.value })}
            className="h-7 w-7 shrink-0 cursor-pointer rounded border border-(--color-border-default) bg-transparent p-0"
          />
        )}
        {weight !== undefined && (
          <select
            value={weight === 'inherit' ? '' : weight}
            data-testid={`${testIdPrefix}-weight`}
            aria-label={`${label} ${t('dashboard.chart.fontWeight')}`}
            onChange={(e) =>
              onChange({ weight: (e.target.value || undefined) as 'normal' | 'bold' | undefined })
            }
            className={`${inputClass()} min-w-0 flex-1`}
          >
            <option value="">{t('dashboard.chart.inherit')}</option>
            <option value="normal">{t('dashboard.chart.fontWeightNormal')}</option>
            <option value="bold">{t('dashboard.chart.fontWeightBold')}</option>
          </select>
        )}
        {align !== undefined && (
          <select
            value={align === 'inherit' ? '' : align}
            data-testid={`${testIdPrefix}-align`}
            aria-label={`${label} ${t('dashboard.chart.textAlign')}`}
            onChange={(e) =>
              onChange({ align: (e.target.value || undefined) as ChartTextAlign | undefined })
            }
            className={`${inputClass()} min-w-0 flex-1`}
          >
            <option value="">{t('dashboard.chart.inherit')}</option>
            {TEXT_ALIGN_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>
                {t(o.labelKey)}
              </option>
            ))}
          </select>
        )}
        {showColor && color !== undefined && (
          <button
            type="button"
            data-testid={`${testIdPrefix}-color-reset`}
            aria-label={`${label} ${t('dashboard.chart.fontColorReset')}`}
            title={t('dashboard.chart.fontColorReset')}
            onClick={() => onChange({ color: undefined })}
            className="h-7 w-7 shrink-0 rounded border border-(--color-border-default) text-xs text-(--color-text-muted) hover:bg-(--color-bg-elevated)"
          >
            x
          </button>
        )}
        </div>
      </div>
    </LabeledField>
  );
}
