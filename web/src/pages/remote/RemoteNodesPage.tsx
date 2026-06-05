// 원격 관리 — 관리 노드 목록 페이지 (SPEC-REMOTE-001 M5, G01 + G02).
//
// 책임:
//   - 전체 관리 노드를 테이블로 표시 (instance_id, hostname, version,
//     online/offline, 등록 상태 배지, last_seen).
//   - 상태별 행 액션:
//       pending  → 승인 / 거부
//       approved → 폐기
//   - 거부/폐기는 파괴적이므로 확인 다이얼로그를 거친다.
//   - 승인/거부/폐기 결과는 토스트로 피드백한다.
//
// 권한: admin 전용 (라우트 가드 + 백엔드 검증). 본 페이지는 UI 레벨 보조이다.

import { useMemo, useState } from 'react';
import { CheckCircle2, Network, XCircle } from 'lucide-react';

import { ConfirmDialog } from '@/components/remote/ConfirmDialog';
import { NodeOnlineIndicator } from '@/components/remote/NodeOnlineIndicator';
import { NodeStatusBadge } from '@/components/remote/NodeStatusBadge';
import {
  useApproveNode,
  useManagedNodes,
  useRejectNode,
  useRevokeNode,
} from '@/hooks/useRemote';
import { useTranslation } from '@/lib/i18n';
import { formatDate } from '@/lib/utils/format';
import { useUIStore } from '@/stores/uiStore';
import type { ManagedNode } from '@/types/remote';

/** 확인 다이얼로그 대상 상태. */
interface PendingConfirm {
  node: ManagedNode;
  kind: 'reject' | 'revoke';
}

export default function RemoteNodesPage(): React.JSX.Element {
  const { t } = useTranslation();
  const { data: nodes, isLoading, error, refetch } = useManagedNodes();
  const addNotification = useUIStore((s) => s.addNotification);

  const approve = useApproveNode();
  const reject = useRejectNode();
  const revoke = useRevokeNode();

  const [confirm, setConfirm] = useState<PendingConfirm | null>(null);

  const allNodes = useMemo<ManagedNode[]>(() => nodes ?? [], [nodes]);

  // 승인 핸들러 (확인 불필요).
  const handleApprove = (node: ManagedNode): void => {
    approve.mutate(node.instance_id, {
      onSuccess: () =>
        addNotification({ type: 'success', message: t('remote.toast.approved') }),
      onError: () =>
        addNotification({ type: 'error', message: t('remote.toast.actionFailed') }),
    });
  };

  // 확인 다이얼로그 확정 핸들러.
  const handleConfirm = (): void => {
    if (!confirm) return;
    const { node, kind } = confirm;
    const onSuccess = (): void => {
      addNotification({
        type: 'success',
        message: kind === 'reject' ? t('remote.toast.rejected') : t('remote.toast.revoked'),
      });
      setConfirm(null);
    };
    const onError = (): void => {
      addNotification({ type: 'error', message: t('remote.toast.actionFailed') });
      setConfirm(null);
    };

    if (kind === 'reject') {
      reject.mutate({ instanceID: node.instance_id }, { onSuccess, onError });
    } else {
      revoke.mutate(node.instance_id, { onSuccess, onError });
    }
  };

  const confirmPending = reject.isPending || revoke.isPending;

  // --- 로딩 상태 ---
  if (isLoading && !nodes) {
    return (
      <div className="space-y-6" data-testid="remote-nodes-loading">
        <PageHeader />
        <div className="space-y-2">
          {Array.from({ length: 4 }).map((_, i) => (
            <div
              key={i}
              className="h-14 animate-pulse rounded bg-(--color-bg-elevated)"
            />
          ))}
        </div>
      </div>
    );
  }

  // --- 에러 상태 ---
  if (error) {
    return (
      <div className="space-y-6">
        <PageHeader />
        <div
          className="rounded-md border border-red-200 bg-red-50 p-6 text-center dark:border-red-800 dark:bg-red-900/20"
          data-testid="remote-nodes-error"
        >
          <p className="text-sm text-red-700 dark:text-red-400">
            {t('remote.loadError')}
          </p>
          <button
            type="button"
            onClick={() => refetch()}
            className="mt-3 rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700 dark:bg-red-500 dark:hover:bg-red-600"
          >
            {t('common.retry')}
          </button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <PageHeader />

      {allNodes.length === 0 ? (
        <div
          className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) py-16 text-center"
          data-testid="remote-nodes-empty"
        >
          <Network className="mx-auto h-12 w-12 text-gray-300 dark:text-gray-600" aria-hidden="true" />
          <p className="mt-4 text-sm text-(--color-text-muted)">
            {t('remote.noNodes')}
          </p>
        </div>
      ) : (
        <div className="overflow-x-auto rounded-lg border border-(--color-border-default)">
          <table
            className="min-w-full divide-y divide-(--color-border-default)"
            aria-label={t('remote.nodesTableLabel')}
          >
            <thead className="bg-(--color-bg-primary)">
              <tr>
                <Th>{t('remote.col.hostname')}</Th>
                <Th>{t('remote.col.instanceId')}</Th>
                <Th>{t('remote.col.version')}</Th>
                <Th>{t('remote.col.online')}</Th>
                <Th>{t('remote.col.status')}</Th>
                <Th>{t('remote.col.lastSeen')}</Th>
                <th className="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-(--color-text-muted)" scope="col">
                  {t('remote.col.actions')}
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-(--color-border-default) bg-(--color-bg-surface)">
              {allNodes.map((node) => (
                <NodeRow
                  key={node.instance_id}
                  node={node}
                  onApprove={() => handleApprove(node)}
                  onReject={() => setConfirm({ node, kind: 'reject' })}
                  onRevoke={() => setConfirm({ node, kind: 'revoke' })}
                  approvePending={
                    approve.isPending && approve.variables === node.instance_id
                  }
                />
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* 파괴적 작업 확인 다이얼로그 */}
      <ConfirmDialog
        open={confirm !== null}
        title={
          confirm?.kind === 'reject'
            ? t('remote.confirm.rejectTitle')
            : t('remote.confirm.revokeTitle')
        }
        description={
          confirm
            ? (confirm.kind === 'reject'
                ? t('remote.confirm.rejectDesc')
                : t('remote.confirm.revokeDesc')
              ).replace('{node}', confirm.node.hostname || confirm.node.instance_id)
            : ''
        }
        confirmLabel={
          confirm?.kind === 'reject' ? t('remote.action.reject') : t('remote.action.revoke')
        }
        pending={confirmPending}
        onConfirm={handleConfirm}
        onCancel={() => {
          if (!confirmPending) setConfirm(null);
        }}
      />
    </div>
  );
}

// ---- 페이지 헤더 ----

function PageHeader(): React.JSX.Element {
  const { t } = useTranslation();
  return (
    <header data-testid="remote-nodes-header">
      <h1 className="text-2xl font-semibold text-(--color-text-primary)">
        {t('remote.title')}
      </h1>
      <p className="mt-1 text-sm text-(--color-text-muted)">{t('remote.subtitle')}</p>
    </header>
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

// ---- 노드 행 ----

interface NodeRowProps {
  node: ManagedNode;
  onApprove: () => void;
  onReject: () => void;
  onRevoke: () => void;
  approvePending: boolean;
}

/** 단일 노드 테이블 행 + 상태별 액션. */
function NodeRow({
  node,
  onApprove,
  onReject,
  onRevoke,
  approvePending,
}: NodeRowProps): React.JSX.Element {
  const { t } = useTranslation();
  const lastSeen =
    node.last_seen > 0 ? formatDate(new Date(node.last_seen), 'long') : '-';

  return (
    <tr data-testid="remote-node-row" data-instance-id={node.instance_id}>
      <td className="whitespace-nowrap px-4 py-3 text-sm font-medium text-(--color-text-primary)">
        {node.hostname || '-'}
      </td>
      <td className="whitespace-nowrap px-4 py-3 font-mono text-xs text-(--color-text-muted)">
        {node.instance_id}
      </td>
      <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
        {node.version || '-'}
      </td>
      <td className="whitespace-nowrap px-4 py-3">
        <NodeOnlineIndicator online={node.online} />
      </td>
      <td className="whitespace-nowrap px-4 py-3">
        <NodeStatusBadge status={node.status} />
      </td>
      <td className="whitespace-nowrap px-4 py-3 text-sm text-(--color-text-muted)">
        {lastSeen}
      </td>
      <td className="whitespace-nowrap px-4 py-3 text-right">
        <div className="inline-flex items-center justify-end gap-2">
          {node.status === 'pending' && (
            <>
              <button
                type="button"
                onClick={onApprove}
                disabled={approvePending}
                data-testid="node-approve-button"
                className="inline-flex items-center gap-1 rounded-md bg-emerald-600 px-3 py-1.5 text-xs font-medium text-white transition-colors hover:bg-emerald-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-emerald-500 dark:hover:bg-emerald-600"
              >
                <CheckCircle2 className="h-3.5 w-3.5" aria-hidden="true" />
                {t('remote.action.approve')}
              </button>
              <button
                type="button"
                onClick={onReject}
                data-testid="node-reject-button"
                className="inline-flex items-center gap-1 rounded-md border border-red-200 px-3 py-1.5 text-xs font-medium text-red-700 transition-colors hover:bg-red-50 dark:border-red-800 dark:text-red-400 dark:hover:bg-red-950"
              >
                <XCircle className="h-3.5 w-3.5" aria-hidden="true" />
                {t('remote.action.reject')}
              </button>
            </>
          )}
          {node.status === 'approved' && (
            <button
              type="button"
              onClick={onRevoke}
              data-testid="node-revoke-button"
              className="inline-flex items-center gap-1 rounded-md border border-red-200 px-3 py-1.5 text-xs font-medium text-red-700 transition-colors hover:bg-red-50 dark:border-red-800 dark:text-red-400 dark:hover:bg-red-950"
            >
              {t('remote.action.revoke')}
            </button>
          )}
          {(node.status === 'rejected' || node.status === 'revoked') && (
            <span className="text-xs text-(--color-text-muted)">—</span>
          )}
        </div>
      </td>
    </tr>
  );
}
