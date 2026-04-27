// 재사용 가능한 확인 다이얼로그.
//
// 파괴적 작업(예: 저장소 초기화) 또는 일반 확인 흐름에서 사용한다.
// `variant='danger'` 면 빨간색 confirm 버튼으로 강조한다.
//
// PromoteToStaticDialog 와 동일한 모달 레이아웃 / 키보드·배경 클릭 패턴을 따른다.
//
// @spec SPEC-STORE-003

import { useCallback, useEffect, type ReactNode } from 'react';
import { Loader2, X } from 'lucide-react';

import { cn } from '@/lib/utils/cn';

// ---- Props ----

interface ConfirmDialogProps {
  /** 모달 표시 여부 */
  isOpen: boolean;
  /** 모달 닫기 핸들러 (취소·Esc·배경 클릭). */
  onClose: () => void;
  /** 확인 핸들러. async 핸들러도 지원 (await 됨). */
  onConfirm: () => void | Promise<void>;
  /** 헤더 제목. (예: "저장소 전체 초기화") */
  title: string;
  /**
   * 본문 메시지.
   * 문자열로 단순 텍스트를 전달하거나, ReactNode 로 더 풍부한 본문을 구성할 수 있다.
   */
  message: string | ReactNode;
  /** 확인 버튼 라벨. 기본값 '확인'. */
  confirmLabel?: string;
  /** 취소 버튼 라벨. 기본값 '취소'. */
  cancelLabel?: string;
  /**
   * 시각적 변형.
   * - `'danger'` : 빨간색 confirm 버튼 (파괴적 작업).
   * - `'default'` : 파란색 confirm 버튼 (일반 확인).
   * 기본값 `'default'`.
   */
  variant?: 'danger' | 'default';
  /**
   * 부모의 비동기 작업 진행 상태.
   * true 이면 양쪽 버튼이 비활성화되고 confirm 버튼에 스피너가 표시된다.
   * 또한 Esc / 배경 클릭이 무시된다.
   */
  isSubmitting?: boolean;
}

// ---- 컴포넌트 ----

export function ConfirmDialog({
  isOpen,
  onClose,
  onConfirm,
  title,
  message,
  confirmLabel = '확인',
  cancelLabel = '취소',
  variant = 'default',
  isSubmitting = false,
}: ConfirmDialogProps) {
  // Esc 키로 닫기 (제출 중에는 무시).
  useEffect(() => {
    if (!isOpen) return;
    const handleKeyDown = (e: globalThis.KeyboardEvent) => {
      if (e.key === 'Escape' && !isSubmitting) onClose();
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [isOpen, onClose, isSubmitting]);

  // 배경 클릭 시 닫기 (제출 중에는 무시).
  const handleBackdropClick = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (e.target === e.currentTarget && !isSubmitting) onClose();
    },
    [onClose, isSubmitting],
  );

  // 확인 클릭. 제출 중이면 무시.
  const handleConfirm = useCallback(async () => {
    if (isSubmitting) return;
    await onConfirm();
  }, [isSubmitting, onConfirm]);

  if (!isOpen) return null;

  // variant 별 confirm 버튼 색상.
  const confirmBtnCls =
    variant === 'danger'
      ? 'bg-red-500 text-white hover:bg-red-600 dark:bg-red-600 dark:hover:bg-red-500'
      : 'bg-blue-600 text-white hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600';

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={handleBackdropClick}
      role="dialog"
      aria-modal="true"
      aria-labelledby="confirm-dialog-title"
    >
      <div
        className="mx-4 flex w-full max-w-[480px] flex-col rounded-lg bg-(--color-bg-surface) shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* 헤더 */}
        <div className="flex items-center justify-between border-b border-(--color-border-default) px-5 py-3">
          <h2
            id="confirm-dialog-title"
            className="text-base font-semibold text-(--color-text-primary)"
          >
            {title}
          </h2>
          <button
            type="button"
            onClick={onClose}
            disabled={isSubmitting}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 disabled:opacity-50 dark:hover:text-gray-300"
            aria-label="닫기"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* 본문 */}
        <div className="px-5 py-4 text-sm text-(--color-text-primary)">
          {typeof message === 'string' ? <p>{message}</p> : message}
        </div>

        {/* 푸터 */}
        <div className="flex justify-end gap-2 border-t border-(--color-border-default) px-5 py-3">
          <button
            type="button"
            onClick={onClose}
            disabled={isSubmitting}
            className="rounded-md border border-(--color-border-strong) px-3 py-1.5 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated) disabled:opacity-50"
          >
            {cancelLabel}
          </button>
          <button
            type="button"
            onClick={() => void handleConfirm()}
            disabled={isSubmitting}
            className={cn(
              'inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50',
              confirmBtnCls,
            )}
          >
            {isSubmitting && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}
