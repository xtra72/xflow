// 노드 사전 등록(수동 등록) 모달 (수동 enrollment).
//
// instance_id(필수) + name(선택)을 입력받아 서버에서 직접 노드를 등록한다.
// 성공 시 토스트 + 노드 목록 무효화로 신규 노드가 나타난다 (status=approved).
// 중복(409)은 명확한 한글 에러로, 그 외 실패는 일반 에러로 표시한다.
//
// 접근성: ConfirmDialog 와 동일한 패턴(role=dialog, aria-modal, 포커스/Escape).
// 입력 필드에 라벨을 연결하고, 열릴 때 첫 입력에 포커스한다.

import { useCallback, useEffect, useRef, useState } from 'react';
import { Loader2 } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { APIError } from '@/types/api';

interface PreRegisterNodeDialogProps {
  /** 열림 여부. */
  open: boolean;
  /** 등록 진행 중 여부. */
  pending: boolean;
  /**
   * 등록 제출 콜백. 부모가 mutation 을 호출하고 성공/실패를 처리한다.
   * 실패 시 본 컴포넌트의 `error` 로 표시할 수 있도록 reject 된 에러를 전파한다.
   */
  onSubmit: (instanceId: string, name: string) => Promise<void>;
  /** 취소/닫기 콜백. */
  onCancel: () => void;
}

/** 포커스 가능한 요소 셀렉터 (포커스 트랩용). */
const FOCUSABLE =
  'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

/**
 * 노드 사전 등록 모달.
 */
export function PreRegisterNodeDialog({
  open,
  pending,
  onSubmit,
  onCancel,
}: PreRegisterNodeDialogProps): React.JSX.Element | null {
  const { t } = useTranslation();
  const dialogRef = useRef<HTMLDivElement>(null);
  const firstInputRef = useRef<HTMLInputElement>(null);

  const [instanceId, setInstanceId] = useState('');
  const [name, setName] = useState('');
  const [error, setError] = useState<string | null>(null);

  // 열릴 때마다 폼을 초기화하고 첫 입력에 포커스한다.
  useEffect(() => {
    if (open) {
      setInstanceId('');
      setName('');
      setError(null);
      const id = window.setTimeout(() => firstInputRef.current?.focus(), 0);
      return () => window.clearTimeout(id);
    }
    return undefined;
  }, [open]);

  // Escape 취소 + Tab 포커스 트랩.
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
        const focusable = Array.from(root.querySelectorAll<HTMLElement>(FOCUSABLE));
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

  const handleSubmit = (e: React.FormEvent): void => {
    e.preventDefault();
    const trimmedId = instanceId.trim();
    if (!trimmedId) {
      setError(t('remote.preRegister.instanceIdRequired'));
      return;
    }
    setError(null);
    void onSubmit(trimmedId, name.trim()).catch((err: unknown) => {
      setError(preRegisterErrorMessage(err, t));
    });
  };

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      data-testid="pre-register-dialog"
    >
      <div
        className="absolute inset-0 bg-black/40"
        aria-hidden="true"
        onClick={() => {
          if (!pending) onCancel();
        }}
      />

      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="pre-register-title"
        aria-describedby="pre-register-desc"
        onKeyDown={handleKeyDown}
        className="relative z-10 w-full max-w-md rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-6 shadow-xl"
      >
        <h2
          id="pre-register-title"
          className="text-lg font-semibold text-(--color-text-primary)"
        >
          {t('remote.preRegister.title')}
        </h2>
        <p
          id="pre-register-desc"
          className="mt-2 text-sm text-(--color-text-muted)"
        >
          {t('remote.preRegister.desc')}
        </p>

        <form className="mt-4 space-y-4" onSubmit={handleSubmit}>
          <div>
            <label
              htmlFor="pre-register-instance-id"
              className="block text-sm font-medium text-(--color-text-secondary)"
            >
              {t('remote.preRegister.instanceIdLabel')}
            </label>
            <input
              ref={firstInputRef}
              id="pre-register-instance-id"
              type="text"
              value={instanceId}
              onChange={(e) => setInstanceId(e.target.value)}
              disabled={pending}
              placeholder={t('remote.preRegister.instanceIdPlaceholder')}
              data-testid="pre-register-instance-id"
              className="mt-1 w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-primary) px-3 py-2 text-sm text-(--color-text-primary) placeholder:text-(--color-text-muted) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-50"
            />
          </div>

          <div>
            <label
              htmlFor="pre-register-name"
              className="block text-sm font-medium text-(--color-text-secondary)"
            >
              {t('remote.preRegister.nameLabel')}
            </label>
            <input
              id="pre-register-name"
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={pending}
              placeholder={t('remote.preRegister.namePlaceholder')}
              data-testid="pre-register-name"
              className="mt-1 w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-primary) px-3 py-2 text-sm text-(--color-text-primary) placeholder:text-(--color-text-muted) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-50"
            />
          </div>

          {error && (
            <p
              className="text-sm text-red-600 dark:text-red-400"
              data-testid="pre-register-error"
              role="alert"
            >
              {error}
            </p>
          )}

          <div className="flex justify-end gap-2 pt-2">
            <button
              type="button"
              onClick={onCancel}
              disabled={pending}
              className="rounded-md border border-(--color-border-strong) px-4 py-2 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated) disabled:cursor-not-allowed disabled:opacity-50"
            >
              {t('common.cancel')}
            </button>
            <button
              type="submit"
              disabled={pending}
              data-testid="pre-register-submit"
              className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-blue-500 dark:hover:bg-blue-600"
            >
              {pending && <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />}
              {pending
                ? t('remote.preRegister.submitting')
                : t('remote.preRegister.submit')}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}

/**
 * 사전 등록 에러를 사용자 친화 메시지로 변환한다.
 * 409 → 중복 인스턴스 ID.
 */
function preRegisterErrorMessage(err: unknown, t: (k: string) => string): string {
  if (err instanceof APIError && err.status === 409) {
    return t('remote.preRegister.errorDuplicate');
  }
  return t('remote.preRegister.errorGeneric');
}
