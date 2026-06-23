// 가상(네임드) 링크 컴팩트 인디케이터 (SPEC-LINK-001).
//
// 인라인 "출력 링크 [name]" / "입력 링크 [name]" 배지(노드 폭을 넓히던 원인)를
// 대체한다. 가상 링크가 있는 포트 옆에 "링크 아이콘 + 개수" 칩만 표시하고,
// 클릭하면 노드 바깥에 뜨는 팝오버 목록(LinkListPopover)을 연다.
// 노드 폭에 영향을 주지 않도록 폭을 최소화한다.

import { Link2 } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

interface LinkIndicatorProps {
  /** 인디케이터 방향: 출력(소스 포트) 또는 입력(타겟 포트). */
  direction: 'output' | 'input';
  /** 이 포트의 가상 링크(와이어) 개수. 칩에 숫자로 표시한다. */
  count: number;
  /** 같은 포트의 팝오버가 현재 열려 있는지(강조 표시용). */
  active: boolean;
  /** 클릭 핸들러 — 팝오버 열기/닫기 토글. */
  onClick: (e: React.MouseEvent) => void;
}

/**
 * 포트 옆 컴팩트 가상 링크 인디케이터.
 * 링크 아이콘 + 개수만 노출하며, 클릭으로 팝오버를 토글한다.
 */
export function LinkIndicator({
  direction,
  count,
  active,
  onClick,
}: LinkIndicatorProps) {
  const { t } = useTranslation();
  const tooltip = t('editor.link.indicatorTooltip').replace(
    '{count}',
    String(count),
  );

  return (
    <button
      type="button"
      onClick={onClick}
      onMouseDown={(e) => e.stopPropagation()}
      title={tooltip}
      aria-label={tooltip}
      aria-expanded={active}
      data-link-indicator={direction}
      className={cn(
        // 폭 최소화: 아이콘 + 한두 자리 숫자만. 노드 폭에 영향 없도록 px 작게.
        'inline-flex flex-shrink-0 items-center gap-0.5 rounded-full border px-1 py-px',
        'text-[8px] font-medium leading-none transition-colors duration-150',
        'nodrag cursor-pointer',
        active
          ? 'border-blue-400 bg-blue-100 text-blue-700 dark:border-blue-500 dark:bg-blue-900/40 dark:text-blue-300'
          : 'border-zinc-200 bg-zinc-50 text-zinc-500 hover:border-blue-300 hover:text-blue-600 dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-400',
      )}
    >
      <Link2 className="h-2 w-2" />
      <span className="tabular-nums">{count}</span>
    </button>
  );
}
