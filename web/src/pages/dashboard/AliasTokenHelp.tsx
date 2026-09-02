// 이름 토큰 도움말(물음표 팝오버) — 시리즈 세부 설정과 이름 형식 입력이 공유한다.
//
// `ChartPanelSections` 에 두면 `SeriesNameFormatField` 가 그것을 import 해야 하는데
// 반대 방향이 이미 있어 순환이 된다(`SeriesNameFormatField` 머리말 참조).
// 그래서 두 쪽이 함께 의존할 수 있는 잎 모듈로 뺀다.
//
// 삽입 로직은 여기 두지 않는다 — 커서 복원 방식이 호출부마다 다르고, 그 차이를
// 옵션으로 흡수하면 이 컴포넌트가 두 화면의 사정을 알게 된다. 여기는 "무엇을
// 보여줄지"만 알고, "누르면 무슨 일이 일어나는지"는 onInsert 가 정한다.
//
// @spec SPEC-WEB-005

import { useEffect, useRef, useState } from 'react';
import { HelpCircle } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';

import { makeAliasToken } from './panels/charts/aliasTemplate';

/**
 * 사용 가능한 이름 토큰을 물음표 뒤에 접어 두는 도움말.
 *
 * 토큰 목록은 시리즈 하나당 8개를 넘기도 한다. 상시 펼쳐 두면 정작 편집 대상인
 * 입력·색상·선 스타일이 토큰 더미에 밀려난다. 토큰은 "이름을 처음 조립할 때"만
 * 필요하므로 물음표 뒤로 접는다.
 *
 * - 목록의 토큰을 누르면 `onInsert` 가 호출된다. 연속 삽입을 위해 팝오버는 닫지 않는다.
 * - 바깥을 클릭하면 닫힌다(정보 팝오버와 동일한 규약).
 * - 제시할 토큰이 하나도 없으면 물음표 자체를 노출하지 않는다.
 */
export function AliasTokenHelp({
  tokenPaths,
  onInsert,
  testIdPrefix,
}: {
  /** 제시할 토큰 경로 목록(`availableAliasTokens` 결과). */
  tokenPaths: readonly string[];
  /** 토큰 클릭 시 호출. 커서 위치 삽입 등 실제 반영은 호출부 소관이다. */
  onInsert: (path: string) => void;
  /** 테스트 식별자 접두사. `-token-help` / `-tokens` / `-token-<path>` 로 파생한다. */
  testIdPrefix: string;
}): React.ReactElement | null {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLSpanElement>(null);

  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent): void => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [open]);

  if (tokenPaths.length === 0) return null;

  return (
    <span className="relative inline-flex" ref={containerRef}>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-haspopup="true"
        aria-expanded={open}
        data-testid={`${testIdPrefix}-token-help`}
        className="inline-flex items-center rounded p-0.5 text-(--color-text-muted) opacity-70 transition-colors hover:text-(--color-text-primary) hover:opacity-100"
        title={t('dashboard.chart.storeAliasTokenHelpAria')}
        aria-label={t('dashboard.chart.storeAliasTokenHelpAria')}
      >
        <HelpCircle className="h-3.5 w-3.5" aria-hidden="true" />
      </button>
      {open && (
        <div
          data-testid={`${testIdPrefix}-tokens`}
          className="absolute left-0 top-full z-30 mt-1 w-64 rounded-md border border-(--color-border-default) bg-(--color-bg-primary) p-2.5 text-left shadow-lg"
        >
          <p className="mb-1.5 text-xs font-semibold text-(--color-text-primary)">
            {t('dashboard.chart.storeAliasTokenHelpTitle')}
          </p>
          <div className="flex flex-wrap gap-1.5">
            {tokenPaths.map((k) => (
              <button
                key={k}
                type="button"
                onClick={() => onInsert(k)}
                data-testid={`${testIdPrefix}-token-${k}`}
                className="rounded border border-(--color-border-default) bg-(--color-bg-elevated) px-1.5 py-0.5 font-mono text-xs text-blue-600 transition-colors hover:bg-blue-50 dark:text-blue-400 dark:hover:bg-blue-900/30"
              >
                {makeAliasToken(k)}
              </button>
            ))}
          </div>
          <p className="mt-2 text-[10px] leading-snug text-(--color-text-muted)">
            {t('dashboard.chart.storeAliasTokenHelpHint')}
          </p>
        </div>
      )}
    </span>
  );
}
