// 컬러 스와치 + 팔레트 팝오버 — 패널 설정 다이얼로그 내부에서 공유.
// AcControlStyleSection / AcControlThresholdsSection 등 여러 곳에서 동일한 컬러 픽커
// UX 가 필요해 단일 위치로 추출했다.

import { useState } from 'react';
import { X } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

import { COLOR_PALETTE } from './colorPalette';

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
 * - 팝오버는 absolute 포지션 + z-20 으로 카드 경계 밖으로 노출 가능
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
  return (
    <span className="relative inline-flex">
      <button
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
          role="dialog"
          aria-label={t('dashboard.colorSwatch.paletteAria').replace('{label}', ariaLabel)}
          className="absolute right-0 top-full z-20 mt-1 flex w-[156px] flex-wrap gap-1 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) p-2 shadow-lg"
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
