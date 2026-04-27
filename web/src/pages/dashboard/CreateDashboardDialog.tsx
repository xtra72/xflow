// 대시보드 생성 다이얼로그.
// 새 대시보드 페이지의 이름을 입력받아 생성한다.

import { useCallback, useEffect, useRef, useState } from 'react';
import { X } from 'lucide-react';

import { useUIStore } from '@/stores/uiStore';

interface CreateDashboardDialogProps {
  open: boolean;
  onClose: () => void;
}

/** 대시보드 생성 모달 */
export default function CreateDashboardDialog({ open, onClose }: CreateDashboardDialogProps) {
  const addDashboardPage = useUIStore((s) => s.addDashboardPage);
  const [name, setName] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);

  // 모달이 열릴 때 상태 초기화 및 포커스
  useEffect(() => {
    if (open) {
      setName('');
      // requestAnimationFrame으로 포커스 보장
      requestAnimationFrame(() => {
        inputRef.current?.focus();
      });
    }
  }, [open]);

  // ESC 키로 닫기
  useEffect(() => {
    if (!open) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [open, onClose]);

  // 배경 클릭 시 닫기
  const handleBackdropClick = useCallback(
    (e: React.MouseEvent<HTMLDivElement>) => {
      if (e.target === e.currentTarget) onClose();
    },
    [onClose],
  );

  // 생성 처리
  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) return;
    addDashboardPage(trimmed);
    onClose();
  };

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={handleBackdropClick}
      role="dialog"
      aria-modal="true"
      aria-labelledby="create-dashboard-dialog-title"
    >
      <div className="mx-4 w-full max-w-sm rounded-lg bg-(--color-bg-surface) shadow-xl">
        {/* 헤더 */}
        <div className="flex items-center justify-between border-b border-(--color-border-default) px-6 py-4">
          <h2
            id="create-dashboard-dialog-title"
            className="text-lg font-semibold text-(--color-text-primary)"
          >
            대시보드 추가
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-gray-400 transition-colors hover:bg-(--color-bg-elevated) hover:text-gray-600 dark:hover:text-gray-300"
            aria-label="닫기"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* 본문 */}
        <form onSubmit={handleSubmit}>
          <div className="px-6 py-4">
            <label
              htmlFor="dashboard-name-input"
              className="mb-1.5 block text-sm font-medium text-(--color-text-secondary)"
            >
              대시보드 이름
            </label>
            <input
              ref={inputRef}
              id="dashboard-name-input"
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="새 대시보드"
              className="w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-2 text-sm text-(--color-text-primary) placeholder:text-(--color-text-muted) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            />
          </div>

          {/* 푸터 */}
          <div className="flex justify-end gap-2 border-t border-(--color-border-default) px-6 py-4">
            <button
              type="button"
              onClick={onClose}
              className="rounded-md border border-(--color-border-strong) px-4 py-2 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)"
            >
              취소
            </button>
            <button
              type="submit"
              disabled={!name.trim()}
              className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
            >
              생성
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
