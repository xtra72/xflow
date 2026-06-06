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
import { CheckCircle2, Network, Plus, Trash2, XCircle } from 'lucide-react';

import { ConfirmDialog } from '@/components/remote/ConfirmDialog';
import { EnrollmentTokenSection } from '@/components/remote/EnrollmentTokenSection';
import { NodeOnlineIndicator } from '@/components/remote/NodeOnlineIndicator';
import { NodeStatusBadge } from '@/components/remote/NodeStatusBadge';
import { PreRegisterNodeDialog } from '@/components/remote/PreRegisterNodeDialog';
import { RemoteNotServerNotice } from '@/components/remote/RemoteNotServerNotice';
import {
  useApproveNode,
  useDeleteNode,
  useManagedNodes,
  usePreRegisterNode,
  useRejectNode,
  useRemoteMode,
  useRevokeNode,
} from '@/hooks/useRemote';
import { useTranslation } from '@/lib/i18n';
import { formatDate } from '@/lib/utils/format';
import { useUIStore } from '@/stores/uiStore';
import type { ManagedNode } from '@/types/remote';

/** 확인 다이얼로그 대상 상태. */
interface PendingConfirm {
  node: ManagedNode;
  kind: 'reject' | 'revoke' | 'delete';
}

export default function RemoteNodesPage(): React.JSX.Element {
  const { t } = useTranslation();
  // server 모드가 아니면 노드 쿼리를 막아 404 노이즈를 방지하고 안내를 표시한다.
  const { data: remoteMode } = useRemoteMode();
  const isServer = remoteMode?.mode === 'server';
  const {
    data: nodes,
    isLoading,
    error,
    refetch,
  } = useManagedNodes(undefined, isServer);
  const addNotification = useUIStore((s) => s.addNotification);

  const approve = useApproveNode();
  const reject = useRejectNode();
  const revoke = useRevokeNode();
  const deleteNode = useDeleteNode();
  const preRegister = usePreRegisterNode();

  const [confirm, setConfirm] = useState<PendingConfirm | null>(null);
  const [preRegisterOpen, setPreRegisterOpen] = useState(false);

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
    const successMessage: Record<PendingConfirm['kind'], string> = {
      reject: t('remote.toast.rejected'),
      revoke: t('remote.toast.revoked'),
      delete: t('remote.toast.deleted'),
    };
    const onSuccess = (): void => {
      addNotification({ type: 'success', message: successMessage[kind] });
      setConfirm(null);
    };
    const onError = (): void => {
      addNotification({ type: 'error', message: t('remote.toast.actionFailed') });
      setConfirm(null);
    };

    if (kind === 'reject') {
      reject.mutate({ instanceID: node.instance_id }, { onSuccess, onError });
    } else if (kind === 'revoke') {
      revoke.mutate(node.instance_id, { onSuccess, onError });
    } else {
      deleteNode.mutate(node.instance_id, { onSuccess, onError });
    }
  };

  // 사전 등록 제출 핸들러. 실패 시 모달이 에러를 표시할 수 있도록 에러를 전파한다.
  const handlePreRegister = async (instanceId: string, name: string): Promise<void> => {
    await preRegister.mutateAsync({
      instance_id: instanceId,
      ...(name ? { name } : {}),
    });
    addNotification({ type: 'success', message: t('remote.toast.preRegistered') });
    setPreRegisterOpen(false);
  };

  const confirmPending = reject.isPending || revoke.isPending || deleteNode.isPending;

  // --- 비-server 모드: 안내만 표시하고 쿼리는 발행하지 않는다 ---
  if (remoteMode && !isServer) {
    return (
      <div className="space-y-6">
        <PageHeader />
        <RemoteNotServerNotice />
      </div>
    );
  }

  // --- 로딩 상태 (모드 미확정 또는 노드 로딩 중) ---
  if (!remoteMode || (isLoading && !nodes)) {
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
      <PageHeader onPreRegister={() => setPreRegisterOpen(true)} />

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
                  onDelete={() => setConfirm({ node, kind: 'delete' })}
                  approvePending={
                    approve.isPending && approve.variables === node.instance_id
                  }
                />
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Enrollment 토큰 관리 섹션 */}
      <div className="border-t border-(--color-border-default) pt-6">
        <EnrollmentTokenSection enabled={isServer} />
      </div>

      {/* 노드 사전 등록 모달 */}
      <PreRegisterNodeDialog
        open={preRegisterOpen}
        pending={preRegister.isPending}
        onSubmit={handlePreRegister}
        onCancel={() => {
          if (!preRegister.isPending) setPreRegisterOpen(false);
        }}
      />

      {/* 파괴적 작업 확인 다이얼로그 */}
      <ConfirmDialog
        open={confirm !== null}
        title={confirm ? t(CONFIRM_TITLE_KEY[confirm.kind]) : ''}
        description={
          confirm
            ? t(CONFIRM_DESC_KEY[confirm.kind]).replace(
                '{node}',
                confirm.node.hostname || confirm.node.instance_id,
              )
            : ''
        }
        confirmLabel={confirm ? t(CONFIRM_ACTION_KEY[confirm.kind]) : ''}
        pending={confirmPending}
        onConfirm={handleConfirm}
        onCancel={() => {
          if (!confirmPending) setConfirm(null);
        }}
      />
    </div>
  );
}

// ---- 확인 다이얼로그 i18n 키 매핑 ----

const CONFIRM_TITLE_KEY: Record<PendingConfirm['kind'], string> = {
  reject: 'remote.confirm.rejectTitle',
  revoke: 'remote.confirm.revokeTitle',
  delete: 'remote.confirm.deleteTitle',
};

const CONFIRM_DESC_KEY: Record<PendingConfirm['kind'], string> = {
  reject: 'remote.confirm.rejectDesc',
  revoke: 'remote.confirm.revokeDesc',
  delete: 'remote.confirm.deleteDesc',
};

const CONFIRM_ACTION_KEY: Record<PendingConfirm['kind'], string> = {
  reject: 'remote.action.reject',
  revoke: 'remote.action.revoke',
  delete: 'remote.action.delete',
};

// ---- 페이지 헤더 ----

interface PageHeaderProps {
  /** 헤더 우측 액션 영역 (사전 등록 버튼 등). server 모드에서만 노출. */
  onPreRegister?: () => void;
}

function PageHeader({ onPreRegister }: PageHeaderProps): React.JSX.Element {
  const { t } = useTranslation();
  return (
    <header
      data-testid="remote-nodes-header"
      className="flex items-start justify-between gap-4"
    >
      <div>
        <h1 className="text-2xl font-semibold text-(--color-text-primary)">
          {t('remote.title')}
        </h1>
        <p className="mt-1 text-sm text-(--color-text-muted)">{t('remote.subtitle')}</p>
      </div>
      {onPreRegister && (
        <button
          type="button"
          onClick={onPreRegister}
          data-testid="node-pre-register-button"
          className="inline-flex shrink-0 items-center gap-1.5 rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 dark:bg-blue-500 dark:hover:bg-blue-600"
        >
          <Plus className="h-4 w-4" aria-hidden="true" />
          {t('remote.preRegister.button')}
        </button>
      )}
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
  onDelete: () => void;
  approvePending: boolean;
}

/** 단일 노드 테이블 행 + 상태별 액션. */
function NodeRow({
  node,
  onApprove,
  onReject,
  onRevoke,
  onDelete,
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
          {/* 삭제는 폐기와 달리 항목 자체를 제거한다 — 모든 상태에서 제공. */}
          <button
            type="button"
            onClick={onDelete}
            data-testid="node-delete-button"
            aria-label={t('remote.action.delete')}
            title={t('remote.action.delete')}
            className="inline-flex items-center gap-1 rounded-md border border-(--color-border-strong) px-2 py-1.5 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)"
          >
            <Trash2 className="h-3.5 w-3.5" aria-hidden="true" />
          </button>
        </div>
      </td>
    </tr>
  );
}
