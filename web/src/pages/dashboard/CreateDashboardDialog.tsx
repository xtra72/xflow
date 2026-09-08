// 대시보드 생성 다이얼로그.
// 새 대시보드의 이름을 입력받아 **서버에 생성**한다(SPEC-DASHBOARD-004 M6 6.4).
//
// 구 모델은 `addDashboardPage(name)` 으로 클라이언트 스토어만 바꿨다 — 서버는
// 그 대시보드의 존재를 몰랐고, uid 도 클라이언트가 지어냈다. 지금은
// `POST /api/v1/dashboards` 로 만들고 서버가 발급한 uid 를 그대로 쓴다.
//
// 생성 권한은 대시보드 단위가 아니라 전역 `dashboard.create` 다(spec.md §2.7 E1).
// viewer 는 이를 보유하지 않으므로 제출 버튼이 **비활성 + 사유 툴팁** 으로 남는다
// (숨기지 않는다 — SPEC-AUTH-006 §4.2).
//
// @spec SPEC-DASHBOARD-004 v0.1.0 (§2.7 E1, §2.11 S2)

import { useCallback, useEffect, useRef, useState } from 'react';
import { X } from 'lucide-react';

import { usePermission } from '@/hooks/usePermission';
import { useTranslation } from '@/lib/i18n';

import { useCreateDashboard } from './useCreateDashboard';

interface CreateDashboardDialogProps {
  open: boolean;
  onClose: () => void;
}

/** 대시보드 생성 모달 */
export default function CreateDashboardDialog({ open, onClose }: CreateDashboardDialogProps) {
  const { t } = useTranslation();
  const { hasPermission } = usePermission();
  const canCreate = hasPermission('dashboard.create');
  const { create, isCreating } = useCreateDashboard();
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

  // 생성 처리 — 서버 응답을 받은 뒤에만 닫는다. 실패하면 열어둔 채 사유를
  // 알린다(토스트는 useCreateDashboard 가 발화). 낙관적으로 닫으면 403 을
  // 받은 사용자가 "만들어졌다" 고 오해한다.
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed || !canCreate || isCreating) return;
    const created = await create(trimmed);
    if (created) onClose();
  };

  if (!open) return null;

  const submitDisabled = !name.trim() || !canCreate || isCreating;

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
            {t('dashboard.createDashboard.title')}
          </h2>
          <button
            type="button"
            onClick={onClose}
            className="rounded-md p-1 text-(--color-text-muted) transition-colors hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary)"
            aria-label={t('dashboard.createDashboard.closeAria')}
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
              {t('dashboard.createDashboard.nameLabel')}
            </label>
            <input
              ref={inputRef}
              id="dashboard-name-input"
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('dashboard.createDashboard.namePlaceholder')}
              className="w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-2 text-sm text-(--color-text-primary) placeholder:text-(--color-text-muted) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
            />
            {/* 권한 부족 사유와 완화책은 화면에도 남긴다 — 툴팁만으로는
                키보드·터치 사용자에게 전달되지 않는다(spec.md §5 접근성). */}
            {!canCreate && (
              <p
                role="status"
                data-testid="create-dashboard-denied"
                className="mt-2 text-xs text-amber-700 dark:text-amber-400"
              >
                {t('dashboard.gate.createDenied')} {t('dashboard.gate.mitigation')}
              </p>
            )}
          </div>

          {/* 푸터 */}
          <div className="flex justify-end gap-2 border-t border-(--color-border-default) px-6 py-4">
            <button
              type="button"
              onClick={onClose}
              className="rounded-md border border-(--color-border-strong) px-4 py-2 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)"
            >
              {t('dashboard.cancel')}
            </button>
            <button
              type="submit"
              disabled={submitDisabled}
              aria-disabled={submitDisabled}
              title={canCreate ? undefined : t('dashboard.gate.createDenied')}
              className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
            >
              {t('dashboard.create')}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
