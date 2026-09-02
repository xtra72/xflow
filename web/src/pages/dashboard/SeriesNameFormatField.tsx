// 시리즈 표시 이름 형식 입력 — Store · TSDB 두 소스가 공유한다.
//
// `ChartPanelSections` 에 두면 `TsdbSourceSection` 이 그것을 import 해야 하는데,
// 반대 방향(`ChartPanelSections -> TsdbSourceSection`) 이 이미 있어 순환이 된다.
// 그래서 공용 모듈로 뺀다.
//
// @spec SPEC-WEB-005 (이름 형식) · SPEC-TSDB-004

import { useRef } from 'react';

import { useTranslation } from '@/lib/i18n';

import { AliasTokenHelp } from './AliasTokenHelp';
import {
  availableAliasTokens,
  makeAliasToken,
  resolveSeriesAlias,
  unknownAliasTokens,
} from './panels/charts/aliasTemplate';

/** 이름 형식 해석에 쓰는 표본 시리즈. 두 소스의 참조 타입이 이 형상을 만족한다. */
export interface NameFormatSample {
  key: string;
  field?: string;
  tags?: Record<string, string>;
}

// 입력은 폭을 고정한다. w-full 이면 미리보기가 옆에 설 자리가 없어 아랫줄로 밀린다.
// 형식 문자열은 보통 토큰 두세 개라 이 폭이면 대개 한눈에 들어온다.
function inputClass(): string {
  return 'w-56 max-w-full rounded-md border border-(--color-border-default) bg-(--color-bg-surface) px-2 py-1 text-xs text-(--color-text-primary)';
}

/**
 * 시리즈 이름 형식 입력 + 토큰 도움말(물음표) + 미리보기.
 *
 * **미지정이 정상 상태다.** 비워 두면 내장 서술 표기(measurement + 필드 + 태그)가
 * 쓰이며, 그것이 대부분의 경우에 읽기 좋다. 형식은 그 기본이 부족할 때만 쓴다.
 *
 * 레이아웃: `[입력] (?) 미리보기: …` 한 줄. 폭이 모자라면 접힌다. 토큰 목록은
 * 시리즈 세부 설정과 같은 규약으로 물음표 뒤에 접어 둔다 — 상시 펼쳐 두면 토큰
 * 여덟 줄이 정작 편집 대상인 입력과 미리보기를 밀어냈다.
 */
export function SeriesNameFormatField({
  value,
  onChange,
  sample,
  testIdPrefix = 'chart-store-series-name-format',
  label,
  placeholder,
  hint,
}: {
  value: string | undefined;
  onChange: (next: string | undefined) => void;
  sample: NameFormatSample | undefined;
  /** 테스트 식별자 접두사. Store 의 기존 식별자를 그대로 유지하려고 기본값을 둔다. */
  testIdPrefix?: string;
  label?: string;
  placeholder?: string;
  /** 미지정 시 기본 표기 설명. */
  hint?: string;
}): React.ReactElement {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLInputElement | null>(null);
  const ctx = sample
    ? { measurement: sample.key, field: sample.field ?? '', tags: sample.tags ?? {} }
    : undefined;
  const tokenPaths = ctx ? availableAliasTokens(ctx) : [];

  const insertToken = (path: string): void => {
    const token = makeAliasToken(path);
    const current = value ?? '';
    const el = inputRef.current;
    let next: string;
    if (el && el.selectionStart != null && el.selectionEnd != null) {
      next = current.slice(0, el.selectionStart) + token + current.slice(el.selectionEnd);
    } else {
      next = current + token;
    }
    onChange(next.trim() === '' ? undefined : next);
  };

  const preview =
    value && value.trim() !== '' && ctx ? resolveSeriesAlias(value, ctx) : undefined;
  // 실재하지 않는 토큰은 조용히 빈 문자열이 되어 "형식이 안 먹는다" 로 보인다.
  // 결과만 보여 주면 오타인지 값이 빈 것인지 구분할 수 없으므로 따로 알린다.
  const unknown = value && ctx ? unknownAliasTokens(value, ctx) : [];

  return (
    <div className="space-y-1" data-testid={testIdPrefix}>
      <label className="mb-1.5 block text-xs font-medium text-(--color-text-muted)">
        {label ?? t('dashboard.chart.storeSeriesNameFormat')}
      </label>
      {/* 입력 | 토큰 도움말 | 미리보기 — 한 줄, 폭이 모자라면 접힌다. */}
      <div className="flex flex-wrap items-center gap-2">
        <input
          type="text"
          value={value ?? ''}
          ref={inputRef}
          placeholder={placeholder ?? t('dashboard.chart.storeSeriesNameFormatPlaceholder')}
          onChange={(e) => onChange(e.target.value.trim() === '' ? undefined : e.target.value)}
          className={inputClass()}
          data-testid={`${testIdPrefix}-input`}
        />
        <AliasTokenHelp
          tokenPaths={tokenPaths}
          onInsert={insertToken}
          testIdPrefix={testIdPrefix}
        />
        {preview !== undefined && (
          <span
            data-testid={`${testIdPrefix}-preview`}
            className="inline-flex min-w-0 max-w-full items-center gap-0.5 text-xs text-(--color-text-muted)"
          >
            <span>{t('dashboard.chart.storeAliasPreview')}</span>
            <span className="truncate font-mono text-(--color-text-primary)">{preview}</span>
          </span>
        )}
      </div>
      {hint !== undefined && (
        <p data-testid={`${testIdPrefix}-hint`} className="px-0.5 text-[11px] text-(--color-text-muted)">
          {hint}
        </p>
      )}
      {unknown.length > 0 && (
        <p
          data-testid={`${testIdPrefix}-unknown`}
          className="px-0.5 text-[11px] text-amber-500"
        >
          {t('dashboard.chart.seriesNameFormatUnknownToken')}{' '}
          <span className="font-mono">{unknown.map((k) => makeAliasToken(k)).join(', ')}</span>
        </p>
      )}
    </div>
  );
}
