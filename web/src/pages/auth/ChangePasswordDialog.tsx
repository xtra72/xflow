// 비밀번호 변경 다이얼로그 컴포넌트.
// 현재 비밀번호, 새 비밀번호, 확인 입력을 받아 서버에 변경 요청을 보낸다.

import { useState } from 'react';
import { X } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { changePassword } from '@/services/api/authService';

interface ChangePasswordDialogProps {
  open: boolean;
  onClose: () => void;
}

/** 최소 비밀번호 길이 */
const MIN_PASSWORD_LENGTH = 4;

/**
 * 비밀번호 변경 모달 다이얼로그.
 * 현재 비밀번호 확인 후 새 비밀번호로 변경한다.
 */
export default function ChangePasswordDialog({ open, onClose }: ChangePasswordDialogProps) {
  const { t } = useTranslation();

  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  if (!open) return null;

  /** 폼 초기화 */
  const resetForm = () => {
    setCurrentPassword('');
    setNewPassword('');
    setConfirmPassword('');
    setError('');
    setSuccess('');
    setIsSubmitting(false);
  };

  /** 다이얼로그 닫기 */
  const handleClose = () => {
    resetForm();
    onClose();
  };

  /** 폼 제출 핸들러 */
  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setSuccess('');

    // 클라이언트 유효성 검사
    if (newPassword.length < MIN_PASSWORD_LENGTH) {
      setError(t('auth.passwordTooShort'));
      return;
    }
    if (newPassword !== confirmPassword) {
      setError(t('auth.passwordMismatch'));
      return;
    }

    setIsSubmitting(true);
    try {
      await changePassword({
        current_password: currentPassword,
        new_password: newPassword,
      });
      setSuccess(t('auth.passwordChanged'));
      // 성공 후 잠시 대기 후 닫기
      setTimeout(() => {
        handleClose();
      }, 1500);
    } catch {
      setError(t('auth.passwordChangeError'));
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      {/* 오버레이 */}
      <div
        className="absolute inset-0 bg-black/50"
        onClick={handleClose}
        onKeyDown={(e) => e.key === 'Escape' && handleClose()}
        role="presentation"
      />

      {/* 다이얼로그 */}
      <div className="relative w-full max-w-md rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-6 shadow-lg">
        {/* 헤더 */}
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-lg font-semibold text-(--color-text-primary)">
            {t('auth.changePassword')}
          </h2>
          <button
            type="button"
            onClick={handleClose}
            className="rounded-md p-1 text-(--color-text-muted) hover:bg-(--color-bg-elevated) hover:text-(--color-text-secondary)"
            aria-label={t('common.close')}
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* 폼 */}
        <form onSubmit={(e) => void handleSubmit(e)} className="space-y-4">
          {/* 에러/성공 메시지 */}
          {error && (
            <div
              className="rounded-md border border-red-300 bg-red-50 px-4 py-2 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/20 dark:text-red-400"
              role="alert"
            >
              {error}
            </div>
          )}
          {success && (
            <div
              className="rounded-md border border-green-300 bg-green-50 px-4 py-2 text-sm text-green-700 dark:border-green-700 dark:bg-green-900/20 dark:text-green-400"
              role="status"
            >
              {success}
            </div>
          )}

          {/* 현재 비밀번호 */}
          <div>
            <label
              htmlFor="current-password"
              className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
            >
              {t('auth.currentPassword')}
            </label>
            <input
              id="current-password"
              type="password"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              disabled={isSubmitting}
              autoComplete="current-password"
              required
              className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-surface) px-3 py-2 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-50"
            />
          </div>

          {/* 새 비밀번호 */}
          <div>
            <label
              htmlFor="new-password"
              className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
            >
              {t('auth.newPassword')}
            </label>
            <input
              id="new-password"
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              disabled={isSubmitting}
              autoComplete="new-password"
              required
              minLength={MIN_PASSWORD_LENGTH}
              className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-surface) px-3 py-2 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-50"
            />
          </div>

          {/* 비밀번호 확인 */}
          <div>
            <label
              htmlFor="confirm-password"
              className="mb-1 block text-sm font-medium text-(--color-text-secondary)"
            >
              {t('auth.confirmPassword')}
            </label>
            <input
              id="confirm-password"
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              disabled={isSubmitting}
              autoComplete="new-password"
              required
              minLength={MIN_PASSWORD_LENGTH}
              className="w-full rounded-md border border-(--color-border-default) bg-(--color-bg-surface) px-3 py-2 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-50"
            />
          </div>

          {/* 버튼 */}
          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={handleClose}
              disabled={isSubmitting}
              className="rounded-md border border-(--color-border-default) bg-(--color-bg-surface) px-4 py-2 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated) disabled:opacity-50"
            >
              {t('common.cancel')}
            </button>
            <button
              type="submit"
              disabled={isSubmitting}
              className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {isSubmitting ? t('common.loading') : t('common.confirm')}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
