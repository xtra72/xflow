// Enrollment 토큰 관리 섹션 (수동 enrollment).
//
// 책임:
//   - 토큰 발급 버튼 + 발급 모달 (CreateEnrollmentTokenDialog)
//   - 발급 직후 raw 토큰 1회 표시 모달 (EnrollmentTokenCreatedDialog)
//   - 토큰 목록 테이블 (id 짧게, label, created/expires, max_uses, uses, 상태)
//   - 행별 폐기(revoke) 액션 → ConfirmDialog → revoke mutation
//
// server 모드에서만 렌더된다 (부모 페이지가 게이팅). admin 전용.

import { useState } from 'react';
import { KeyRound, Plus } from 'lucide-react';

import { ConfirmDialog } from '@/components/remote/ConfirmDialog';
import { CreateEnrollmentTokenDialog } from '@/components/remote/CreateEnrollmentTokenDialog';
import { EnrollmentTokenCreatedDialog } from '@/components/remote/EnrollmentTokenCreatedDialog';
import {
  useCreateEnrollmentToken,
  useEnrollmentTokens,
  useRevokeEnrollmentToken,
} from '@/hooks/useRemote';
import { useTranslation } from '@/lib/i18n';
import { formatDate } from '@/lib/utils/format';
import { useUIStore } from '@/stores/uiStore';
import type {
  EnrollmentToken,
  EnrollmentTokenCreated,
  EnrollmentTokenCreateRequest,
} from '@/types/remote';

interface EnrollmentTokenSectionProps {
  /** 쿼리 활성 여부. server 모드가 아니면 false. */
  enabled: boolean;
}

/** 토큰의 표시 상태(활성/폐기/만료)를 계산한다. */
function tokenState(token: EnrollmentToken): 'active' | 'revoked' | 'expired' {
  if (token.revoked) return 'revoked';
  if (token.expires_at !== undefined && token.expires_at > 0 && token.expires_at < Date.now()) {
    return 'expired';
  }
  return 'active';
}

/**
 * Enrollment 토큰 관리 섹션.
 */
export function EnrollmentTokenSection({
  enabled,
}: EnrollmentTokenSectionProps): React.JSX.Element {
  const { t } = useTranslation();
  const addNotification = useUIStore((s) => s.addNotification);

  const { data: tokens, isLoading, error } = useEnrollmentTokens(enabled);
  const createToken = useCreateEnrollmentToken();
  const revokeToken = useRevokeEnrollmentToken();

  // 발급 모달 열림 여부.
  const [createOpen, setCreateOpen] = useState(false);
  // 발급된 raw 토큰 (1회 표시 모달). null 이면 닫힘.
  const [createdToken, setCreatedToken] = useState<EnrollmentTokenCreated | null>(null);
  // 폐기 확인 대상.
  const [revokeTarget, setRevokeTarget] = useState<EnrollmentToken | null>(null);

  const handleCreate = async (req: EnrollmentTokenCreateRequest): Promise<void> => {
    const created = await createToken.mutateAsync(req);
    setCreateOpen(false);
    setCreatedToken(created);
  };

  const handleRevokeConfirm = (): void => {
    if (!revokeTarget) return;
    revokeToken.mutate(revokeTarget.id, {
      onSuccess: () => {
        addNotification({ type: 'success', message: t('remote.enrollment.toast.revoked') });
        setRevokeTarget(null);
      },
      onError: () => {
        addNotification({ type: 'error', message: t('remote.enrollment.toast.actionFailed') });
        setRevokeTarget(null);
      },
    });
  };

  const list = tokens ?? [];

  return (
    <section className="space-y-4" data-testid="enrollment-token-section">
      <header className="flex items-start justify-between gap-4">
        <div>
          <h2 className="flex items-center gap-2 text-lg font-semibold text-(--color-text-primary)">
            <KeyRound className="h-5 w-5" aria-hidden="true" />
            {t('remote.enrollment.title')}
          </h2>
          <p className="mt-1 text-sm text-(--color-text-muted)">
            {t('remote.enrollment.subtitle')}
          </p>
        </div>
        <button
          type="button"
          onClick={() => setCreateOpen(true)}
          data-testid="enrollment-create-button"
          className="inline-flex shrink-0 items-center gap-1.5 rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
        >
          <Plus className="h-4 w-4" aria-hidden="true" />
          {t('remote.enrollment.createButton')}
        </button>
      </header>

      {error ? (
        <div
          className="rounded-md border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-400"
          data-testid="enrollment-tokens-error"
        >
          {t('remote.enrollment.loadError')}
        </div>
      ) : isLoading && !tokens ? (
        <div className="space-y-2" data-testid="enrollment-tokens-loading">
          {Array.from({ length: 2 }).map((_, i) => (
            <div key={i} className="h-12 animate-pulse rounded bg-(--color-bg-elevated)" />
          ))}
        </div>
      ) : list.length === 0 ? (
        <div
          className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) py-10 text-center"
          data-testid="enrollment-tokens-empty"
        >
          <p className="text-sm text-(--color-text-muted)">
            {t('remote.enrollment.noTokens')}
          </p>
        </div>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
          <table
            className="min-w-full divide-y divide-(--color-border-default)"
            aria-label={t('remote.enrollment.tableLabel')}
          >
            <thead className="bg-(--color-bg-primary)">
              <tr>
                <Th>{t('remote.enrollment.col.id')}</Th>
                <Th>{t('remote.enrollment.col.label')}</Th>
                <Th>{t('remote.enrollment.col.createdAt')}</Th>
                <Th>{t('remote.enrollment.col.expiresAt')}</Th>
                <Th>{t('remote.enrollment.col.maxUses')}</Th>
                <Th>{t('remote.enrollment.col.uses')}</Th>
                <Th>{t('remote.enrollment.col.state')}</Th>
                <th
                  scope="col"
                  className="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-(--color-text-muted)"
                >
                  {t('remote.enrollment.col.actions')}
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-(--color-border-default) bg-(--color-bg-surface)">
              {list.map((token) => (
                <TokenRow
                  key={token.id}
                  token={token}
                  onRevoke={() => setRevokeTarget(token)}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* 발급 모달 */}
      <CreateEnrollmentTokenDialog
        open={createOpen}
        pending={createToken.isPending}
        onSubmit={handleCreate}
        onCancel={() => {
          if (!createToken.isPending) setCreateOpen(false);
        }}
      />

      {/* 발급 결과(raw 토큰 1회 표시) 모달 */}
      <EnrollmentTokenCreatedDialog
        created={createdToken}
        onClose={() => setCreatedToken(null)}
      />

      {/* 폐기 확인 다이얼로그 */}
      <ConfirmDialog
        open={revokeTarget !== null}
        title={t('remote.enrollment.confirm.revokeTitle')}
        description={
          revokeTarget
            ? t('remote.enrollment.confirm.revokeDesc').replace(
                '{label}',
                revokeTarget.label || revokeTarget.id,
              )
            : ''
        }
        confirmLabel={t('remote.action.revoke')}
        pending={revokeToken.isPending}
        onConfirm={handleRevokeConfirm}
        onCancel={() => {
          if (!revokeToken.isPending) setRevokeTarget(null);
        }}
      />
    </section>
  );
}

// ---- 테이블 헤더 셀 ----

function Th({ children }: { children: React.ReactNode }): React.JSX.Element {
  return (
    <th
      scope="col"
      className="px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-(--color-text-muted)"
    >
      {children}
    </th>
  );
}

// ---- 토큰 행 ----

interface TokenRowProps {
  token: EnrollmentToken;
  onRevoke: () => void;
}

/** id 앞 8자만 짧게 표시한다. */
function shortId(id: string): string {
  return id.length > 8 ? `${id.slice(0, 8)}…` : id;
}

function TokenRow({ token, onRevoke }: TokenRowProps): React.JSX.Element {
  const { t } = useTranslation();
  const state = tokenState(token);
  const created = token.created_at > 0 ? formatDate(new Date(token.created_at), 'long') : '-';
  const expires =
    token.expires_at !== undefined && token.expires_at > 0
      ? formatDate(new Date(token.expires_at), 'long')
      : t('remote.enrollment.never');
  const maxUses =
    token.max_uses !== undefined && token.max_uses > 0
      ? String(token.max_uses)
      : t('remote.enrollment.unlimited');

  return (
    <tr data-testid="enrollment-token-row" data-token-id={token.id}>
      <td className="whitespace-nowrap px-4 py-3 font-mono text-xs text-(--color-text-muted)" title={token.id}>
        {shortId(token.id)}
      </td>
      <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-primary)">
        {token.label || '-'}
      </td>
      <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
        {created}
      </td>
      <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
        {expires}
      </td>
      <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
        {maxUses}
      </td>
      <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
        {token.uses}
      </td>
      <td className="whitespace-nowrap px-4 py-3">
        <StateBadge state={state} />
      </td>
      <td className="whitespace-nowrap px-4 py-3 text-right">
        {state === 'active' ? (
          <button
            type="button"
            onClick={onRevoke}
            data-testid="enrollment-token-revoke-button"
            className="inline-flex items-center gap-1 rounded-md border border-red-200 px-3 py-1.5 text-xs font-medium text-red-700 transition-colors hover:bg-red-50 dark:border-red-800 dark:text-red-400 dark:hover:bg-red-950"
          >
            {t('remote.action.revoke')}
          </button>
        ) : (
          <span className="text-xs text-(--color-text-muted)">—</span>
        )}
      </td>
    </tr>
  );
}

// ---- 상태 배지 ----

function StateBadge({
  state,
}: {
  state: 'active' | 'revoked' | 'expired';
}): React.JSX.Element {
  const { t } = useTranslation();
  const cls =
    state === 'active'
      ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
      : 'bg-(--color-bg-sunken) text-(--color-text-secondary)';
  return (
    <span
      data-testid="enrollment-token-state"
      data-state={state}
      className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${cls}`}
    >
      {t(`remote.enrollment.state.${state}`)}
    </span>
  );
}
