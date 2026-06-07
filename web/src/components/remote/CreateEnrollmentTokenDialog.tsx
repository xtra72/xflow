// Enrollment 토큰 발급 모달 (수동 enrollment).
//
// label(선택) + expires_in(선택 select) + max_uses(선택 number)를 입력받아
// 토큰을 발급한다. expires_in 은 사람이 읽는 옵션을 Go duration 문자열로 매핑한다
// (미사용="" / 1h / 24h / 7d=168h / 30d=720h).
//
// 접근성: role=dialog, aria-modal, 라벨 연결, 포커스 트랩/Escape.

import { useCallback, useEffect, useRef, useState } from 'react';
import { Loader2 } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import type { EnrollmentTokenCreateRequest } from '@/types/remote';

interface CreateEnrollmentTokenDialogProps {
  /** 열림 여부. */
  open: boolean;
  /** 발급 진행 중 여부. */
  pending: boolean;
  /**
   * 발급 제출 콜백. 부모가 mutation 을 호출한다.
   * 실패 시 본 컴포넌트의 error 로 표시할 수 있도록 reject 된 에러를 전파한다.
   */
  onSubmit: (req: EnrollmentTokenCreateRequest) => Promise<void>;
  /** 취소/닫기 콜백. */
  onCancel: () => void;
}

/** 만료 옵션 → Go duration 문자열 매핑. value 가 expires_in 으로 전송된다. */
const EXPIRES_OPTIONS: ReadonlyArray<{ value: string; labelKey: string }> = [
  { value: '', labelKey: 'remote.enrollment.expires.none' },
  { value: '1h', labelKey: 'remote.enrollment.expires.1h' },
  { value: '24h', labelKey: 'remote.enrollment.expires.24h' },
  { value: '168h', labelKey: 'remote.enrollment.expires.7d' },
  { value: '720h', labelKey: 'remote.enrollment.expires.30d' },
];

/** 포커스 가능한 요소 셀렉터 (포커스 트랩용). */
const FOCUSABLE =
  'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

/**
 * Enrollment 토큰 발급 모달.
 */
export function CreateEnrollmentTokenDialog({
  open,
  pending,
  onSubmit,
  onCancel,
}: CreateEnrollmentTokenDialogProps): React.JSX.Element | null {
  const { t } = useTranslation();
  const dialogRef = useRef<HTMLDivElement>(null);
  const firstInputRef = useRef<HTMLInputElement>(null);

  const [label, setLabel] = useState('');
  const [expiresIn, setExpiresIn] = useState('');
  const [maxUses, setMaxUses] = useState('');
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setLabel('');
      setExpiresIn('');
      setMaxUses('');
      setError(null);
      const id = window.setTimeout(() => firstInputRef.current?.focus(), 0);
      return () => window.clearTimeout(id);
    }
    return undefined;
  }, [open]);

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
    setError(null);

    // 선택적 필드만 채워 요청 본문을 구성한다.
    const req: EnrollmentTokenCreateRequest = {};
    const trimmedLabel = label.trim();
    if (trimmedLabel) req.label = trimmedLabel;
    if (expiresIn) req.expires_in = expiresIn;
    const parsedMaxUses = Number.parseInt(maxUses, 10);
    if (maxUses.trim() && Number.isFinite(parsedMaxUses) && parsedMaxUses > 0) {
      req.max_uses = parsedMaxUses;
    }

    void onSubmit(req).catch(() => {
      setError(t('remote.enrollment.create.errorGeneric'));
    });
  };

  if (!open) return null;

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      data-testid="create-enrollment-token-dialog"
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
        aria-labelledby="create-token-title"
        aria-describedby="create-token-desc"
        onKeyDown={handleKeyDown}
        className="relative z-10 w-full max-w-md rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-6 shadow-xl"
      >
        <h2
          id="create-token-title"
          className="text-lg font-semibold text-(--color-text-primary)"
        >
          {t('remote.enrollment.create.title')}
        </h2>
        <p id="create-token-desc" className="mt-2 text-sm text-(--color-text-muted)">
          {t('remote.enrollment.create.desc')}
        </p>

        <form className="mt-4 space-y-4" onSubmit={handleSubmit}>
          <div>
            <label
              htmlFor="create-token-label"
              className="block text-sm font-medium text-(--color-text-secondary)"
            >
              {t('remote.enrollment.create.labelLabel')}
            </label>
            <input
              ref={firstInputRef}
              id="create-token-label"
              type="text"
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              disabled={pending}
              placeholder={t('remote.enrollment.create.labelPlaceholder')}
              data-testid="create-token-label"
              className="mt-1 w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-primary) px-3 py-2 text-sm text-(--color-text-primary) placeholder:text-(--color-text-muted) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-50"
            />
          </div>

          <div>
            <label
              htmlFor="create-token-expires"
              className="block text-sm font-medium text-(--color-text-secondary)"
            >
              {t('remote.enrollment.create.expiresLabel')}
            </label>
            <select
              id="create-token-expires"
              value={expiresIn}
              onChange={(e) => setExpiresIn(e.target.value)}
              disabled={pending}
              data-testid="create-token-expires"
              className="mt-1 w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-primary) px-3 py-2 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-50"
            >
              {EXPIRES_OPTIONS.map((opt) => (
                <option key={opt.value} value={opt.value}>
                  {t(opt.labelKey)}
                </option>
              ))}
            </select>
          </div>

          <div>
            <label
              htmlFor="create-token-max-uses"
              className="block text-sm font-medium text-(--color-text-secondary)"
            >
              {t('remote.enrollment.create.maxUsesLabel')}
            </label>
            <input
              id="create-token-max-uses"
              type="number"
              min={1}
              value={maxUses}
              onChange={(e) => setMaxUses(e.target.value)}
              disabled={pending}
              placeholder={t('remote.enrollment.create.maxUsesPlaceholder')}
              data-testid="create-token-max-uses"
              className="mt-1 w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-primary) px-3 py-2 text-sm text-(--color-text-primary) placeholder:text-(--color-text-muted) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 disabled:opacity-50"
            />
          </div>

          {error && (
            <p
              className="text-sm text-red-600 dark:text-red-400"
              data-testid="create-token-error"
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
              data-testid="create-token-submit"
              className="inline-flex items-center gap-2 rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-60 dark:bg-blue-500 dark:hover:bg-blue-600"
            >
              {pending && <Loader2 className="h-4 w-4 animate-spin" aria-hidden="true" />}
              {pending
                ? t('remote.enrollment.create.submitting')
                : t('remote.enrollment.create.submit')}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
