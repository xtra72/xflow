// 컬러 스와치 + 팔레트 팝오버 — 패널 설정 다이얼로그 내부에서 공유.
// AcControlStyleSection / AcControlThresholdsSection 등 여러 곳에서 동일한 컬러 픽커
// UX 가 필요해 단일 위치로 추출했다.

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react';
import { X } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import { COLOR_PALETTE } from './colorPalette';

/** 팝오버 너비(px) — 클래스의 `w-[156px]` 와 같아야 우측 정렬이 맞는다. */
const POPOVER_WIDTH = 156;
/** 트리거와의 간격(px). */
const POPOVER_GAP = 4;
/** 화면 가장자리 최소 여백(px). */
const POPOVER_MARGIN = 8;
/** 첫 계산에 쓰는 추정 높이(px) — 렌더 뒤 실제 높이로 다시 잡는다. */
const POPOVER_FALLBACK_HEIGHT = 96;

interface ColorSwatchButtonProps {
  color: string | undefined;
  onChange: (next: string | undefined) => void;
  ariaLabel: string;
  /** 트리거 버튼의 test id(선택). 목록 안에서 개별 스와치를 집을 때 쓴다. */
  testId?: string;
}

/**
 * 작은 컬러 스와치. 클릭 시 팔레트 팝오버를 열어 색상을 선택한다.
 *
 * - 팝오버는 **fixed** 포지션으로 띄운다. absolute 로 두면 스크롤 컨테이너
 *   (예: 시리즈 선택 표의 `max-h-[45vh] overflow-auto`)에 잘려, 아래쪽 행에서
 *   팔레트가 보이지 않는다. fixed 는 그 컨테이너를 벗어난다.
 * - 아래 공간이 부족하면 위로 뒤집고, 좌우는 화면 안에 가둔다.
 * - fixed 는 스크롤을 따라가지 않으므로 스크롤·리사이즈에는 닫는다.
 * - "미설정" 옵션(X 아이콘) 으로 기본값(undefined) 으로 되돌릴 수 있음
 */
export default function ColorSwatchButton({
  color,
  onChange,
  ariaLabel,
  testId,
}: ColorSwatchButtonProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const popRef = useRef<HTMLSpanElement>(null);
  const [pos, setPos] = useState<{ top: number; left: number }>({ top: 0, left: 0 });

  /** 트리거 기준으로 팝오버 좌표를 정한다. 실제 높이를 잰 뒤 필요하면 뒤집는다. */
  const place = useCallback((): void => {
    const trigger = triggerRef.current;
    if (!trigger) return;
    const r = trigger.getBoundingClientRect();
    // 0 은 "재지 못했다" 는 뜻이므로(레이아웃 없는 환경) 추정값으로 대체한다.
    const h = popRef.current?.offsetHeight || POPOVER_FALLBACK_HEIGHT;
    const below = r.bottom + POPOVER_GAP;
    // 아래로 열면 화면을 넘는 경우에만 위로 뒤집는다.
    const top = below + h > window.innerHeight - POPOVER_MARGIN ? r.top - h - POPOVER_GAP : below;
    // 우측 정렬을 기본으로 하되 왼쪽으로 넘치지 않게 가둔다.
    const left = Math.max(POPOVER_MARGIN, r.right - POPOVER_WIDTH);
    setPos({ top: Math.max(POPOVER_MARGIN, top), left });
  }, []);

  // 렌더 뒤 실제 높이로 한 번 더 잡는다 — 첫 계산은 추정 높이를 쓴다.
  useLayoutEffect(() => {
    if (open) place();
  }, [open, place]);

  // fixed 는 스크롤을 따라가지 않는다. 어긋난 자리에 떠 있느니 닫는다.
  useEffect(() => {
    if (!open) return;
    const close = (): void => setOpen(false);
    document.addEventListener('scroll', close, true);
    window.addEventListener('resize', close);
    return () => {
      document.removeEventListener('scroll', close, true);
      window.removeEventListener('resize', close);
    };
  }, [open]);

  return (
    <span className="relative inline-flex">
      <button
        ref={triggerRef}
        type="button"
        onClick={() => setOpen((v) => !v)}
        className={cn(
          'h-3.5 w-3.5 rounded-full border border-(--color-border-default) transition-transform hover:scale-110',
          !color && 'bg-(--color-bg-surface)',
        )}
        style={color ? { backgroundColor: color } : undefined}
        aria-label={ariaLabel}
        {...(testId ? { 'data-testid': testId } : {})}
      />
      {open && (
        <span
          ref={popRef}
          role="dialog"
          aria-label={t('dashboard.colorSwatch.paletteAria').replace('{label}', ariaLabel)}
          style={{ position: 'fixed', top: pos.top, left: pos.left }}
          className="z-50 flex w-[156px] flex-wrap gap-1 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) p-2 shadow-lg"
        >
          {COLOR_PALETTE.map((c) => (
            <button
              key={c}
              type="button"
              onClick={() => {
                onChange(c);
                setOpen(false);
              }}
              className={cn(
                'h-5 w-5 rounded-full border transition-transform hover:scale-110',
                color === c
                  ? 'border-blue-500 ring-1 ring-blue-500'
                  : 'border-(--color-border-default)',
              )}
              style={{ backgroundColor: c }}
              aria-label={c}
            />
          ))}
          <button
            type="button"
            onClick={() => {
              onChange(undefined);
              setOpen(false);
            }}
            className="flex h-5 w-5 items-center justify-center rounded-full border border-(--color-border-default) bg-(--color-bg-surface) text-(--color-text-muted) hover:bg-(--color-bg-elevated)"
            aria-label={t('dashboard.colorSwatch.defaultAria')}
            title={t('dashboard.colorSwatch.unsetTitle')}
          >
            <X className="h-3 w-3" />
          </button>
        </span>
      )}
    </span>
  );
}
