// 파괴적 작업 확인 다이얼로그 (SPEC-REMOTE-001 M5, G02).
//
// 거부(reject)/폐기(revoke) 처럼 되돌릴 수 없는 작업 전에 사용자 확인을 받는다.
// 접근성:
//   - role="dialog" + aria-modal + aria-labelledby/aria-describedby
//   - 열릴 때 확인 버튼에 포커스, Escape 로 취소, Tab 포커스 트랩
//   - 백드롭 클릭 시 취소

import { useCallback, useEffect, useRef } from 'react';
import { AlertTriangle, Loader2 } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';

interface ConfirmDialogProps {
  /** 열림 여부. */
  open: boolean;
  /** 다이얼로그 제목. */
  title: string;
  /** 본문 설명 (작업 영향 안내). */
  description: string;
  /** 확인 버튼 라벨. 미지정 시 공통 "확인". */
  confirmLabel?: string;
  /** 취소 버튼 라벨. 미지정 시 공통 "취소". */
  cancelLabel?: string;
  /** 파괴적(빨강) 스타일 적용 여부. 기본 true. */
  destructive?: boolean;
  /** 확인 작업 진행 중 여부 (버튼 비활성 + 스피너). */
  pending?: boolean;
  /** 확인 콜백. */
  onConfirm: () => void;
  /** 취소/닫기 콜백. */
  onCancel: () => void;
}

/** 포커스 가능한 요소 셀렉터. */
const FOCUSABLE =
  'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

/**
 * 파괴적 작업 확인 모달. 포커스 트랩 + Escape 취소를 지원한다.
 */
export function ConfirmDialog({
  open,
  title,
  description,
  confirmLabel,
  cancelLabel,
  destructive = true,
  pending = false,
  onConfirm,
  onCancel,
}: ConfirmDialogProps): React.JSX.Element | null {
  const { t } = useTranslation();
  const dialogRef = useRef<HTMLDivElement>(null);
  const confirmRef = useRef<HTMLButtonElement>(null);

  // 열릴 때 확인 버튼으로 포커스 이동.
  useEffect(() => {
    if (open) {
      // 다음 틱에 포커스 (렌더 후).
      const id = window.setTimeout(() => confirmRef.current?.focus(), 0);
      return () => window.clearTimeout(id);
    }
    return undefined;
  }, [open]);

  // 키보드 핸들러: Escape 취소 + Tab 포커스 트랩.
  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        if (!pending) onCancel();
        return;
      }
      if (e.key === 'Tab') {
        const root = dialogRef.current;
        if (!root) return;
        const focusable = Array.from(
          root.querySelectorAll<HTMLElement>(FOCUSABLE),
        );
        if (focusable.length === 0) return;
        const first = focusable[0]!;
        const last = focusable[focusable.length - 1]!;
        const active = document.activeElement as HTMLElement | null;

        if (e.shiftKey && active === first) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && active === last) {
          e.preventDefault();
          first.focus();
        }
      }
    },
    [onCancel, pending],
  );

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      data-testid="confirm-dialog"
    >
      {/* 백드롭 */}
      <div
        className="absolute inset-0 bg-black/40"
        aria-hidden="true"
        onClick={() => {
          if (!pending) onCancel();
        }}
      />

      {/* 다이얼로그 */}
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="confirm-dialog-title"
        aria-describedby="confirm-dialog-desc"
        onKeyDown={handleKeyDown}
        className="relative z-10 w-full max-w-md rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-6 shadow-xl"
      >
        <div className="flex items-start gap-3">
          {destructive && (
            <span className="mt-0.5 inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-red-100 text-red-600 dark:bg-red-900/30 dark:text-red-400">
              <AlertTriangle className="h-5 w-5" aria-hidden="true" />
            </span>
          )}
          <div className="flex-1">
            <h2
              id="confirm-dialog-title"
              className="text-lg font-semibold text-(--color-text-primary)"
            >
              {title}
            </h2>
            <p
              id="confirm-dialog-desc"
              className="mt-2 text-sm text-(--color-text-muted)"
            >
              {description}
            </p>
          </div>
        </div>

        <div className="mt-6 flex justify-end gap-2">
          <button
            type="button"
            onClick={onCancel}
            disabled={pending}
            className="rounded-md border border-(--color-border-strong) px-4 py-2 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-50"
          >
            {cancelLabel ?? t('common.cancel')}
          </button>
          <button
            ref={confirmRef}
            type="button"
            onClick={onConfirm}
            disabled={pending}
            data-testid="confirm-dialog-confirm"
            className={cn(
              'inline-flex items-center gap-2 rounded-md px-4 py-2 text-sm font-medium text-white transition-colors disabled:cursor-not-allowed disabled:opacity-60',
              destructive
                ? 'bg-red-600 hover:bg-red-700 dark:bg-red-500 dark:hover:bg-red-600'
                : 'bg-blue-600 hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600',
            )}
          >
            {pending && <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />}
            {confirmLabel ?? t('common.confirm')}
          </button>
        </div>
      </div>
    </div>
  );
}
