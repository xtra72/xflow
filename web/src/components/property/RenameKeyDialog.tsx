// 스토어 키 이름 변경 모달 (SPEC-STORE-004).
//
// 동적 키(그 키의 모든 시리즈)를 새 키로 이동한다. 값 + 히스토리 + 메타는 백엔드에서
// 보존된다. 디바이스와 스토어 시리즈는 직접 관계가 없으므로 순수한 키 이름 변경이다.
//
// 이 다이얼로그는 새 키 이름을 입력받아 onConfirm(newKey) 로 전달한다. 실제 API 호출
// (POST /store/{agent}/keys/{key}/rename)과 알림/캐시 무효화는 부모가 처리한다.

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
} from 'react';
import { Loader2, X } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';

interface RenameKeyDialogProps {
  /** 모달 표시 여부. */
  isOpen: boolean;
  /** 모달 닫기(취소/외부 클릭/ESC). */
  onClose: () => void;
  /** 현재 키 이름(사용자 관점 key). */
  currentKey: string;
  /** 확인 시 새 키 이름을 전달한다. 부모가 rename API 를 호출한다. */
  onConfirm: (newKey: string) => void | Promise<void>;
  /** 부모의 rename 진행 중 상태. true 이면 입력/버튼 비활성 + 스피너. */
  isSubmitting?: boolean;
}

/**
 * 스토어 키 이름 변경 모달.
 *
 * - 현재 키를 사전 채워 편집하도록 한다.
 * - 새 키가 비어있거나 현재 키와 동일하면 확인 버튼을 비활성화한다.
 */
export default function RenameKeyDialog({
  isOpen,
  onClose,
  currentKey,
  onConfirm,
  isSubmitting = false,
}: RenameKeyDialogProps): React.ReactElement | null {
  const { t } = useTranslation();
  const [value, setValue] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);

  // 열릴 때 현재 키로 사전 채우고 포커스/전체 선택.
  useEffect(() => {
    if (!isOpen) return;
    setValue(currentKey);
    const id = window.setTimeout(() => {
      inputRef.current?.focus();
      inputRef.current?.select();
    }, 0);
    return () => window.clearTimeout(id);
  }, [isOpen, currentKey]);

  // ESC 로 닫기(진행 중이 아닐 때).
  useEffect(() => {
    if (!isOpen) return;
    const onKey = (e: globalThis.KeyboardEvent): void => {
      if (e.key === 'Escape' && !isSubmitting) onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [isOpen, isSubmitting, onClose]);

  const trimmed = value.trim();
  const canConfirm = trimmed !== '' && trimmed !== currentKey && !isSubmitting;

  const handleConfirm = useCallback(() => {
    if (trimmed === '' || trimmed === currentKey || isSubmitting) return;
    void onConfirm(trimmed);
  }, [trimmed, currentKey, isSubmitting, onConfirm]);

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLInputElement>) => {
      if (e.key === 'Enter') {
        e.preventDefault();
        handleConfirm();
      }
    },
    [handleConfirm],
  );

  if (!isOpen) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={() => {
        if (!isSubmitting) onClose();
      }}
      role="dialog"
      aria-modal="true"
      aria-labelledby="rename-key-title"
    >
      <div
        className="mx-4 flex w-full max-w-[440px] flex-col rounded-lg bg-(--color-bg-surface) shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* 헤더 */}
        <div className="flex items-center justify-between border-b border-(--color-border-default) px-5 py-3">
          <h2
            id="rename-key-title"
            className="text-base font-semibold text-(--color-text-primary)"
          >
            {t('agents.detail.store.renameTitle')}
          </h2>
          <button
            type="button"
            onClick={onClose}
            disabled={isSubmitting}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 disabled:opacity-50 dark:hover:text-gray-300"
            aria-label={t('property.meta.closeAria')}
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {/* 본문 */}
        <div className="space-y-4 px-5 py-4">
          <p className="text-xs text-(--color-text-muted)">
            {t('agents.detail.store.renameDesc')}
          </p>

          {/* 현재 키(읽기 전용) */}
          <div>
            <p className="mb-1 text-xs font-medium text-(--color-text-muted)">
              {t('agents.detail.store.renameCurrentLabel')}
            </p>
            <p className="break-all rounded-md border border-(--color-border-default) bg-(--color-bg-elevated) px-3 py-2 font-mono text-sm text-(--color-text-primary)">
              {currentKey}
            </p>
          </div>

          {/* 새 키 입력 */}
          <div>
            <label
              htmlFor="rename-key-input"
              className="mb-1 block text-xs font-medium text-(--color-text-secondary)"
            >
              {t('agents.detail.store.renameNewLabel')}
            </label>
            <input
              ref={inputRef}
              id="rename-key-input"
              type="text"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              onKeyDown={handleKeyDown}
              disabled={isSubmitting}
              data-testid="rename-key-input"
              className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-primary) px-3 py-2 font-mono text-sm text-(--color-text-primary) placeholder:text-(--color-text-muted) focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-60"
            />
          </div>
        </div>

        {/* 푸터 */}
        <div className="flex justify-end gap-2 border-t border-(--color-border-default) px-5 py-3">
          <button
            type="button"
            onClick={onClose}
            disabled={isSubmitting}
            className="rounded-md border border-(--color-border-default) bg-(--color-bg-primary) px-3 py-1.5 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-secondary) disabled:opacity-50"
          >
            {t('property.promote.cancel')}
          </button>
          <button
            type="button"
            onClick={handleConfirm}
            disabled={!canConfirm}
            data-testid="rename-key-confirm"
            className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {isSubmitting && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
            {t('agents.detail.store.renameConfirm')}
          </button>
        </div>
      </div>
    </div>
  );
}
