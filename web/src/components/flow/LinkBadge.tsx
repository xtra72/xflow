// 가상(네임드) 링크 배지 (SPEC-LINK-001).
//
// `virtual=true` 인 와이어의 양 끝을 포트 옆에 작은 배지로 표시한다.
// - 출력(소스) 포트: "출력 링크 [name]"
// - 입력(타겟) 포트: "입력 링크 [name]"
// 클릭하면 같은 이름 그룹을 하이라이트(숨긴 선 일시 표시 + 상대 배지 강조)한다.
// 표시 보조는 이름 라벨만 사용한다(자동 색상 구분 없음).

import { useCallback } from 'react';

import { cn } from '@/lib/utils/cn';
import { useEditorStore } from '@/stores/editorStore';

interface LinkBadgeProps {
  /** 배지 방향: 출력(소스 포트) 또는 입력(타겟 포트). */
  direction: 'output' | 'input';
  /** 링크 이름(그룹 식별자). 빈 문자열이면 "(이름 없음)" 으로 표시한다. */
  name: string;
}

/** 빈 이름 표시용 폴백 라벨. */
const UNNAMED_LABEL = '(이름 없음)';

export function LinkBadge({ direction, name }: LinkBadgeProps) {
  const highlightedLinkName = useEditorStore((s) => s.highlightedLinkName);
  const setHighlightedLinkName = useEditorStore((s) => s.setHighlightedLinkName);

  // 같은 이름 그룹이 활성화되어 있으면 강조한다(빈 이름은 그룹 하이라이트 제외).
  const highlighted = name !== '' && highlightedLinkName === name;

  const handleClick = useCallback(
    (e: React.MouseEvent) => {
      // 노드 선택/드래그로 전파되지 않도록 막는다.
      e.stopPropagation();
      // 빈 이름은 그룹 식별이 불가하므로 하이라이트하지 않는다.
      if (name === '') return;
      setHighlightedLinkName(name);
    },
    [name, setHighlightedLinkName],
  );

  const prefix = direction === 'output' ? '출력 링크' : '입력 링크';
  const display = name === '' ? UNNAMED_LABEL : name;

  return (
    <button
      type="button"
      onClick={handleClick}
      onMouseDown={(e) => e.stopPropagation()}
      title={`${prefix} ${display}`}
      aria-label={`${prefix} ${display}`}
      data-link-direction={direction}
      data-link-name={name}
      className={cn(
        'max-w-[110px] truncate rounded-full border px-1.5 py-px',
        'text-[8px] font-medium leading-none transition-colors duration-150',
        'nodrag cursor-pointer',
        highlighted
          ? 'border-blue-400 bg-blue-100 text-blue-700 dark:border-blue-500 dark:bg-blue-900/40 dark:text-blue-300'
          : 'border-zinc-200 bg-zinc-50 text-zinc-500 hover:border-blue-300 hover:text-blue-600 dark:border-zinc-700 dark:bg-zinc-800 dark:text-zinc-400',
      )}
    >
      {prefix} {display}
    </button>
  );
}
