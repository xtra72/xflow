// 설정 필드 라벨 옆의 도움말(?) 아이콘 + 클릭 팝오버.
//
// 노드/에이전트 설정 폼은 필드마다 설명이 길어 화면이 복잡해진다. 설명을 인라인
// 으로 항상 노출하는 대신, 라벨 뒤의 "?" 아이콘을 클릭했을 때만 팝오버로 보여
// 준다. 접근성을 위해 설명 텍스트는 sr-only 로 항상 DOM 에 두어 aria-describedby
// 가 참조할 수 있게 한다.

import { useEffect, useRef, useState } from 'react';
import { HelpCircle } from 'lucide-react';

import { cn } from '@/lib/utils/cn';

interface FieldHelpProps {
  /** 표시할 설명 텍스트 */
  text: string;
  /** aria-describedby 가 참조하는 id (sr-only 설명에 부여) */
  describedById?: string;
}

export function FieldHelp({ text, describedById }: FieldHelpProps) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLSpanElement>(null);

  // 팝오버가 열려 있을 때 외부 클릭 / Escape 로 닫는다.
  useEffect(() => {
    if (!open) return;
    const onPointerDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setOpen(false);
    };
    document.addEventListener('mousedown', onPointerDown);
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('mousedown', onPointerDown);
      document.removeEventListener('keydown', onKeyDown);
    };
  }, [open]);

  return (
    <span ref={ref} className="relative inline-flex">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        aria-label="설명 보기"
        aria-expanded={open}
        className={cn(
          'inline-flex h-4 w-4 items-center justify-center rounded-full',
          'text-(--color-text-muted) transition-colors',
          'hover:text-blue-600 hover:bg-(--color-bg-elevated)',
          'focus:outline-none focus:ring-1 focus:ring-blue-400',
          open && 'text-blue-600',
        )}
      >
        <HelpCircle className="h-3.5 w-3.5" />
      </button>

      {/* 스크린리더용 — aria-describedby 가 항상 참조 가능하도록 DOM 에 유지 */}
      <span id={describedById} className="sr-only">
        {text}
      </span>

      {open && (
        <div
          role="tooltip"
          className={cn(
            'absolute left-0 top-full z-50 mt-1 w-64 max-w-[18rem]',
            'rounded-md border border-(--color-border-default) bg-(--color-bg-elevated)',
            'px-2.5 py-1.5 text-xs font-normal leading-relaxed text-(--color-text-secondary) shadow-lg',
            'whitespace-pre-wrap break-words',
          )}
        >
          {text}
        </div>
      )}
    </span>
  );
}
