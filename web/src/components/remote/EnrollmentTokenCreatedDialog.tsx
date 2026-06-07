// Enrollment 토큰 발급 결과 모달 — raw 토큰 1회 표시.
//
// 발급된 raw 토큰은 다시 표시되지 않으므로, 복사 가능한 필드 + 명확한 경고와
// 함께 1회만 노출한다. 클라이언트 설정 스니펫도 함께 제공하여 운영자가 바로
// 적용할 수 있게 한다.
//
// 접근성: role=dialog, aria-modal, 복사 버튼 aria-label, 열릴 때 완료 버튼 포커스.

import { useCallback, useEffect, useRef, useState } from 'react';
import { Check, Copy } from 'lucide-react';

import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import type { EnrollmentTokenCreated } from '@/types/remote';

interface EnrollmentTokenCreatedDialogProps {
  /** 발급된 토큰 (null 이면 닫힘). */
  created: EnrollmentTokenCreated | null;
  /** 닫기 콜백. */
  onClose: () => void;
}

/** 복사 표시가 사라질 때까지의 시간 (ms). */
const COPIED_TTL_MS = 3000;

/** 포커스 가능한 요소 셀렉터 (포커스 트랩용). */
const FOCUSABLE =
  'button:not([disabled]), [href], input:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

/** 클라이언트 설정 스니펫을 생성한다. */
function buildClientConfig(token: string): string {
  return [
    'remote_management:',
    '  mode: client',
    '  server_url: "wss://..."',
    `  enrollment_token: "${token}"`,
  ].join('\n');
}

/**
 * 발급된 raw 토큰을 1회 표시하는 모달.
 */
export function EnrollmentTokenCreatedDialog({
  created,
  onClose,
}: EnrollmentTokenCreatedDialogProps): React.JSX.Element | null {
  const { t } = useTranslation();
  const dialogRef = useRef<HTMLDivElement>(null);
  const doneRef = useRef<HTMLButtonElement>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // 어떤 항목을 방금 복사했는지 추적 ('token' | 'snippet').
  const [copied, setCopied] = useState<string | null>(null);

  const open = created !== null;

  useEffect(() => {
    if (open) {
      const id = window.setTimeout(() => doneRef.current?.focus(), 0);
      return () => window.clearTimeout(id);
    }
    setCopied(null);
    return undefined;
  }, [open]);

  // unmount 시 타이머 정리.
  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, []);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLDivElement>) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        onClose();
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
    [onClose],
  );

  const handleCopy = useCallback(async (key: string, value: string) => {
    try {
      await navigator.clipboard.writeText(value);
    } catch {
      // 복사 실패는 silent — 값은 화면에 그대로 노출되어 수동 선택 가능.
      return;
    }
    if (timerRef.current) clearTimeout(timerRef.current);
    setCopied(key);
    timerRef.current = setTimeout(() => setCopied(null), COPIED_TTL_MS);
  }, []);

  if (!created) return null;

  const snippet = buildClientConfig(created.token);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      data-testid="enrollment-token-created-dialog"
    >
      <div className="absolute inset-0 bg-black/40" aria-hidden="true" onClick={onClose} />

      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="token-created-title"
        aria-describedby="token-created-warning"
        onKeyDown={handleKeyDown}
        className="relative z-10 w-full max-w-lg rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-6 shadow-xl"
      >
        <h2
          id="token-created-title"
          className="text-lg font-semibold text-(--color-text-primary)"
        >
          {t('remote.enrollment.created.title')}
        </h2>

        <p
          id="token-created-warning"
          className="mt-2 rounded-md border border-yellow-300 bg-yellow-50 px-3 py-2 text-sm text-yellow-900 dark:border-yellow-700 dark:bg-yellow-950 dark:text-yellow-200"
          role="alert"
        >
          {t('remote.enrollment.created.warning')}
        </p>

        {/* raw 토큰 (복사 가능) */}
        <div className="mt-4">
          <label
            htmlFor="token-created-value"
            className="block text-sm font-medium text-(--color-text-secondary)"
          >
            {t('remote.enrollment.created.tokenLabel')}
          </label>
          <div className="mt-1 flex items-stretch gap-2">
            <input
              id="token-created-value"
              type="text"
              readOnly
              value={created.token}
              data-testid="enrollment-token-value"
              onFocus={(e) => e.currentTarget.select()}
              className="w-full rounded-md border border-(--color-border-strong) bg-(--color-bg-primary) px-3 py-2 font-mono text-sm text-(--color-text-primary)"
            />
            <CopyButton
              copied={copied === 'token'}
              label={t('remote.enrollment.created.copy')}
              copiedLabel={t('remote.enrollment.created.copied')}
              onClick={() => void handleCopy('token', created.token)}
              testId="enrollment-token-copy"
            />
          </div>
        </div>

        {/* 클라이언트 설정 스니펫 */}
        <div className="mt-4">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium text-(--color-text-secondary)">
              {t('remote.enrollment.created.snippetLabel')}
            </span>
            <CopyButton
              copied={copied === 'snippet'}
              label={t('remote.enrollment.created.copySnippet')}
              copiedLabel={t('remote.enrollment.created.copied')}
              onClick={() => void handleCopy('snippet', snippet)}
              testId="enrollment-snippet-copy"
            />
          </div>
          <pre
            data-testid="enrollment-snippet"
            className="mt-1 overflow-x-auto rounded-md border border-(--color-border-strong) bg-(--color-bg-primary) p-3 font-mono text-xs text-(--color-text-primary)"
          >
            {snippet}
          </pre>
        </div>

        <div className="mt-6 flex justify-end">
          <button
            ref={doneRef}
            type="button"
            onClick={onClose}
            data-testid="enrollment-token-created-done"
            className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            {t('remote.enrollment.created.done')}
          </button>
        </div>
      </div>
    </div>
  );
}

interface CopyButtonProps {
  copied: boolean;
  label: string;
  copiedLabel: string;
  onClick: () => void;
  testId: string;
}

/** 복사 버튼 — 복사 후 체크 표시로 전환. */
function CopyButton({
  copied,
  label,
  copiedLabel,
  onClick,
  testId,
}: CopyButtonProps): React.JSX.Element {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={label}
      data-testid={testId}
      className={cn(
        'inline-flex shrink-0 items-center gap-1 rounded-md border px-3 py-2 text-xs font-medium transition-colors',
        copied
          ? 'border-emerald-300 text-emerald-700 dark:border-emerald-700 dark:text-emerald-400'
          : 'border-(--color-border-strong) text-(--color-text-secondary) hover:bg-(--color-bg-elevated)',
      )}
    >
      {copied ? (
        <Check className="h-3.5 w-3.5" aria-hidden="true" />
      ) : (
        <Copy className="h-3.5 w-3.5" aria-hidden="true" />
      )}
      {copied ? copiedLabel : label}
    </button>
  );
}
