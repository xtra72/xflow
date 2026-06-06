// 원격 관리 — 미러 자원 뷰 페이지 (SPEC-REMOTE-001 M5, G03 + G04).
//
// 두 가지 뷰 모드를 제공한다.
//   - 통합(aggregated): 전 노드의 자원을 출처 노드 태그와 함께 표시.
//   - 노드별(per-node): 선택한 노드의 자원만 표시.
// 종류 탭(flows/agents/devices)으로 자원 종류를 전환한다.
// 각 행의 명령 버튼(G04)으로 deploy/start/stop 등을 발행하며, 결과는
// 토스트로 피드백한다 (RemoteCommandButtons).
//
// 권한: admin 전용 (라우트 가드 + 백엔드 검증).

import { useCallback, useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { Boxes, Plus } from 'lucide-react';

import { ConfirmDialog } from '@/components/remote/ConfirmDialog';
import { MirrorResourceTable } from '@/components/remote/MirrorResourceTable';
import {
  RemoteAgentEditDialog,
  type RemoteAgentEditMode,
  type RemoteAgentEditValue,
} from '@/components/remote/RemoteAgentEditDialog';
import { RemoteNotServerNotice } from '@/components/remote/RemoteNotServerNotice';
import {
  useAllMirror,
  useCreateRemoteAgent,
  useDeleteRemoteAgent,
  useDeleteRemoteFlow,
  useManagedNodes,
  useNodeMirror,
  useRemoteMode,
  useUpdateRemoteAgent,
} from '@/hooks/useRemote';
import { useTranslation } from '@/lib/i18n';
import { remoteEditErrorMessage } from '@/lib/remote/editError';
import { omitMaskedSecrets } from '@/lib/remote/secretOmission';
import { cn } from '@/lib/utils/cn';
import { useUIStore } from '@/stores/uiStore';
import type { MirroredResource, MirroredResourceKind } from '@/types/remote';

/** 뷰 모드. */
type ViewMode = 'aggregated' | 'per-node';

/** 종류 탭 정의. */
const KIND_TABS: { kind: MirroredResourceKind; labelKey: string }[] = [
  { kind: 'flow', labelKey: 'remote.kind.flows' },
  { kind: 'agent', labelKey: 'remote.kind.agents' },
  { kind: 'device', labelKey: 'remote.kind.devices' },
];

/** 편집(수정/삭제) 게이팅에 쓰는 노드 상태 조회 결과. */
interface NodeEditState {
  approved: boolean;
  online: boolean;
}

/** 삭제 확인 다이얼로그 대상. */
interface DeleteTarget {
  resource: MirroredResource;
}

/** 에이전트 편집 다이얼로그 상태. */
interface AgentEditState {
  mode: RemoteAgentEditMode;
  /** 수정 대상 자원 (create 면 undefined). */
  resource?: MirroredResource;
  /** 대상 노드. */
  instanceID: string;
}

export default function RemoteResourcesPage(): React.JSX.Element {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const addNotification = useUIStore((s) => s.addNotification);
  const [mode, setMode] = useState<ViewMode>('aggregated');
  const [kind, setKind] = useState<MirroredResourceKind>('flow');
  const [selectedNode, setSelectedNode] = useState<string>('');

  // server 모드가 아니면 모든 미러/노드 쿼리를 막아 404 노이즈를 방지한다.
  const { data: remoteMode } = useRemoteMode();
  const isServer = remoteMode?.mode === 'server';

  const { data: nodes } = useManagedNodes(undefined, isServer);

  // 노드별 뷰에서 선택 가능한 노드는 승인된(approved) 노드로 제한한다.
  const approvedNodes = useMemo(
    () => (nodes ?? []).filter((n) => n.status === 'approved'),
    [nodes],
  );

  // instance_id → {approved, online} 매핑 (편집 게이팅용 — REQ-I05/I10).
  const nodeStateById = useMemo<Map<string, NodeEditState>>(() => {
    const m = new Map<string, NodeEditState>();
    for (const n of nodes ?? []) {
      m.set(n.instance_id, {
        approved: n.status === 'approved',
        online: n.online,
      });
    }
    return m;
  }, [nodes]);

  // per-node 모드일 때 유효한 선택 노드 (없으면 첫 승인 노드).
  const effectiveNode =
    selectedNode || (approvedNodes.length > 0 ? approvedNodes[0]!.instance_id : '');

  // 통합/노드별 쿼리. 활성 모드의 쿼리만 의미 있는 데이터를 가진다.
  const aggregated = useAllMirror(kind, isServer);
  const perNode = useNodeMirror(
    mode === 'per-node' ? effectiveNode : '',
    kind,
    isServer,
  );

  const activeQuery = mode === 'aggregated' ? aggregated : perNode;
  const resources = activeQuery.data ?? [];
  const isLoading = activeQuery.isLoading;
  const error = activeQuery.error;

  // --- 편집 뮤테이션 (M7, 그룹 I) ---
  const createAgent = useCreateRemoteAgent();
  const updateAgent = useUpdateRemoteAgent();
  const deleteFlow = useDeleteRemoteFlow();
  const deleteAgent = useDeleteRemoteAgent();

  const [agentEdit, setAgentEdit] = useState<AgentEditState | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<DeleteTarget | null>(null);

  // 편집/삭제 게이팅: 승인+온라인 노드의 노출 자원만 편집 가능(REQ-I05/I10).
  // (미러에 존재한다는 것 자체가 노출 범위 내임을 의미한다 — REQ-E07.)
  const canEdit = useCallback(
    (res: MirroredResource): boolean => {
      const state = nodeStateById.get(res.source_instance_id);
      return !!state && state.approved && state.online && res.online;
    },
    [nodeStateById],
  );

  // flow 편집 → 시각 편집기 라우트로 이동(REQ-I08). agent 편집 → 다이얼로그.
  const handleEdit = useCallback(
    (res: MirroredResource): void => {
      if (res.kind === 'flow') {
        navigate(
          `/admin/remote/nodes/${res.source_instance_id}/flows/${res.id}/edit`,
        );
        return;
      }
      if (res.kind === 'agent') {
        setAgentEdit({
          mode: 'update',
          resource: res,
          instanceID: res.source_instance_id,
        });
      }
    },
    [navigate],
  );

  // 신규 생성: flow → 빈 편집기, agent → 빈 다이얼로그. 대상 노드 필요.
  const handleCreate = useCallback(
    (targetNode: string): void => {
      if (!targetNode) return;
      if (kind === 'flow') {
        navigate(`/admin/remote/nodes/${targetNode}/flows/new`);
        return;
      }
      if (kind === 'agent') {
        setAgentEdit({ mode: 'create', instanceID: targetNode });
      }
    },
    [kind, navigate],
  );

  // 에이전트 편집 제출 — create/update 경로로 전파(시크릿 생략 — REQ-I07).
  const handleAgentSubmit = useCallback(
    async (value: RemoteAgentEditValue): Promise<void> => {
      if (!agentEdit) return;
      // 수정 config 의 마스킹/미변경 시크릿 필드를 생략한다(REQ-I07).
      const cleanedConfig = omitMaskedSecrets(value.config);
      if (agentEdit.mode === 'create') {
        await createAgent.mutateAsync({
          instanceID: agentEdit.instanceID,
          req: { name: value.name, type: value.type, config: cleanedConfig },
        });
      } else {
        await updateAgent.mutateAsync({
          instanceID: agentEdit.instanceID,
          agentID: agentEdit.resource!.id,
          req: { name: value.name, config: cleanedConfig },
        });
      }
      addNotification({ type: 'success', message: t('remote.edit.saveSuccess') });
      setAgentEdit(null);
    },
    [agentEdit, createAgent, updateAgent, addNotification, t],
  );

  // 삭제 확정 — flow/agent 삭제 명령 전파.
  const handleDeleteConfirm = useCallback((): void => {
    if (!deleteTarget) return;
    const res = deleteTarget.resource;
    const onSuccess = (): void => {
      addNotification({ type: 'success', message: t('remote.edit.deleteSuccess') });
      setDeleteTarget(null);
    };
    const onError = (err: unknown): void => {
      addNotification({ type: 'error', message: remoteEditErrorMessage(err, t) });
      setDeleteTarget(null);
    };
    if (res.kind === 'flow') {
      deleteFlow.mutate(
        { instanceID: res.source_instance_id, flowID: res.id },
        { onSuccess, onError },
      );
    } else if (res.kind === 'agent') {
      deleteAgent.mutate(
        { instanceID: res.source_instance_id, agentID: res.id },
        { onSuccess, onError },
      );
    } else {
      setDeleteTarget(null);
    }
  }, [deleteTarget, deleteFlow, deleteAgent, addNotification, t]);

  // 생성 액션 게이팅: 대상 노드가 승인+온라인이어야 한다(REQ-I05/I10).
  // 통합 뷰는 단일 대상 노드가 없으므로, per-node 뷰에서만 생성을 노출한다.
  const createTargetNode =
    mode === 'per-node' && effectiveNode ? effectiveNode : '';
  const createTargetState = createTargetNode
    ? nodeStateById.get(createTargetNode)
    : undefined;
  const canCreate =
    kind !== 'device' &&
    !!createTargetState &&
    createTargetState.approved &&
    createTargetState.online;

  const deletePending = deleteFlow.isPending || deleteAgent.isPending;
  const agentPending = createAgent.isPending || updateAgent.isPending;

  // 비-server 모드: 안내만 표시하고 미러 쿼리는 발행하지 않는다.
  if (remoteMode && !isServer) {
    return (
      <div className="space-y-6">
        <ResourcesHeader />
        <RemoteNotServerNotice />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <ResourcesHeader />

      {/* 뷰 모드 토글 */}
      <div
        className="inline-flex rounded-md border border-(--color-border-strong) p-0.5"
        role="tablist"
        aria-label={t('remote.viewModeLabel')}
      >
        {(['aggregated', 'per-node'] as const).map((m) => (
          <button
            key={m}
            type="button"
            role="tab"
            aria-selected={mode === m}
            data-testid={`remote-mode-${m}`}
            onClick={() => setMode(m)}
            className={cn(
              'rounded px-3 py-1.5 text-sm font-medium transition-colors',
              mode === m
                ? 'bg-blue-600 text-white dark:bg-blue-500'
                : 'text-(--color-text-secondary) hover:bg-(--color-bg-elevated)',
            )}
          >
            {m === 'aggregated' ? t('remote.mode.aggregated') : t('remote.mode.perNode')}
          </button>
        ))}
      </div>

      {/* 노드별 모드: 노드 선택 */}
      {mode === 'per-node' && (
        <div className="flex items-center gap-2">
          <label
            htmlFor="remote-node-select"
            className="text-sm text-(--color-text-secondary)"
          >
            {t('remote.selectNode')}
          </label>
          <select
            id="remote-node-select"
            data-testid="remote-node-select"
            value={effectiveNode}
            onChange={(e) => setSelectedNode(e.target.value)}
            className="rounded-md border border-(--color-border-strong) bg-(--color-bg-surface) px-3 py-1.5 text-sm text-(--color-text-primary) focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
          >
            {approvedNodes.length === 0 && (
              <option value="">{t('remote.noApprovedNodes')}</option>
            )}
            {approvedNodes.map((n) => (
              <option key={n.instance_id} value={n.instance_id}>
                {n.hostname || n.instance_id}
                {n.online ? '' : ` (${t('remote.offline')})`}
              </option>
            ))}
          </select>
        </div>
      )}

      {/* 종류 탭 */}
      <div
        className="flex gap-1 border-b border-(--color-border-default)"
        role="tablist"
        aria-label={t('remote.kindTabsLabel')}
      >
        {KIND_TABS.map(({ kind: k, labelKey }) => (
          <button
            key={k}
            type="button"
            role="tab"
            aria-selected={kind === k}
            data-testid={`remote-kind-${k}`}
            onClick={() => setKind(k)}
            className={cn(
              '-mb-px border-b-2 px-4 py-2 text-sm font-medium transition-colors',
              kind === k
                ? 'border-blue-600 text-blue-700 dark:border-blue-400 dark:text-blue-400'
                : 'border-transparent text-(--color-text-muted) hover:text-(--color-text-secondary)',
            )}
          >
            {t(labelKey)}
          </button>
        ))}
      </div>

      {/* M7: 생성 액션 (REQ-I10). per-node 뷰에서 대상 노드가 승인+온라인일
          때만 활성화한다(게이팅 — REQ-I05). device 는 편집 대상이 아니다. */}
      {mode === 'per-node' && kind !== 'device' && (
        <div className="flex items-center justify-end">
          <button
            type="button"
            onClick={() => handleCreate(createTargetNode)}
            disabled={!canCreate}
            data-testid="remote-resource-create"
            title={canCreate ? undefined : t('remote.edit.createGateHint')}
            className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50 dark:bg-blue-500 dark:hover:bg-blue-600"
          >
            <Plus className="h-4 w-4" aria-hidden="true" />
            {kind === 'flow'
              ? t('remote.edit.createFlow')
              : t('remote.edit.createAgent')}
          </button>
        </div>
      )}

      {/* 본문 */}
      <ResourceBody
        isLoading={isLoading}
        hasError={!!error}
        onRetry={() => activeQuery.refetch()}
        isEmpty={resources.length === 0}
        needsNodeSelection={mode === 'per-node' && !effectiveNode}
      >
        <MirrorResourceTable
          resources={resources}
          showSource={mode === 'aggregated'}
          nodes={nodes}
          // flow/agent 만 편집 가능(device 는 테이블 내부에서 숨김).
          onEdit={kind === 'device' ? undefined : handleEdit}
          onDelete={kind === 'device' ? undefined : (res) => setDeleteTarget({ resource: res })}
          canEdit={canEdit}
        />
      </ResourceBody>

      {/* 에이전트 편집/설정 다이얼로그 (REQ-I09) */}
      <RemoteAgentEditDialog
        open={agentEdit !== null}
        mode={agentEdit?.mode ?? 'create'}
        pending={agentPending}
        initialName={agentEdit?.resource?.name}
        initialType={resourceType(agentEdit?.resource)}
        initialConfig={parseConfig(agentEdit?.resource?.definition)}
        onSubmit={handleAgentSubmit}
        onCancel={() => {
          if (!agentPending) setAgentEdit(null);
        }}
      />

      {/* 삭제 확인 다이얼로그 (REQ-I03/I04) */}
      <ConfirmDialog
        open={deleteTarget !== null}
        title={t('remote.edit.deleteTitle')}
        description={
          deleteTarget
            ? t('remote.edit.deleteDesc').replace(
                '{name}',
                deleteTarget.resource.name || deleteTarget.resource.id,
              )
            : ''
        }
        confirmLabel={t('remote.action.delete')}
        pending={deletePending}
        onConfirm={handleDeleteConfirm}
        onCancel={() => {
          if (!deletePending) setDeleteTarget(null);
        }}
      />
    </div>
  );
}

/** 미러 자원의 종류(type)를 추출한다 (없으면 빈 문자열). */
function resourceType(res: MirroredResource | undefined): string {
  if (!res) return '';
  // 미러 정의에 type 이 있으면 사용한다(에이전트 redacted config 의 type).
  const config = parseConfig(res.definition);
  const t = config['type'];
  return typeof t === 'string' ? t : '';
}

/** 미러 자원의 redacted 정의(JSON 문자열)를 파싱한다 (실패 시 빈 객체). */
function parseConfig(raw: string | undefined): Record<string, unknown> {
  if (!raw) return {};
  try {
    const parsed: unknown = JSON.parse(raw);
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
  } catch {
    // 손상된 정의는 빈 객체로 폴백.
  }
  return {};
}

// ---- 페이지 헤더 ----

function ResourcesHeader(): React.JSX.Element {
  const { t } = useTranslation();
  return (
    <header data-testid="remote-resources-header">
      <h1 className="text-2xl font-semibold text-(--color-text-primary)">
        {t('remote.resourcesTitle')}
      </h1>
      <p className="mt-1 text-sm text-(--color-text-muted)">
        {t('remote.resourcesSubtitle')}
      </p>
    </header>
  );
}

// ---- 본문 상태 래퍼 ----

interface ResourceBodyProps {
  isLoading: boolean;
  hasError: boolean;
  onRetry: () => void;
  isEmpty: boolean;
  needsNodeSelection: boolean;
  children: React.ReactNode;
}

/** 로딩/에러/빈 상태/정상을 분기 렌더한다. */
function ResourceBody({
  isLoading,
  hasError,
  onRetry,
  isEmpty,
  needsNodeSelection,
  children,
}: ResourceBodyProps): React.JSX.Element {
  const { t } = useTranslation();

  if (needsNodeSelection) {
    return (
      <div
        className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) py-12 text-center"
        data-testid="remote-resources-need-node"
      >
        <p className="text-sm text-(--color-text-muted)">{t('remote.noApprovedNodes')}</p>
      </div>
    );
  }

  if (isLoading) {
    return (
      <div className="space-y-2" data-testid="remote-resources-loading">
        {Array.from({ length: 3 }).map((_, i) => (
          <div key={i} className="h-12 animate-pulse rounded bg-(--color-bg-elevated)" />
        ))}
      </div>
    );
  }

  if (hasError) {
    return (
      <div
        className="rounded-md border border-red-200 bg-red-50 p-6 text-center dark:border-red-800 dark:bg-red-900/20"
        data-testid="remote-resources-error"
      >
        <p className="text-sm text-red-700 dark:text-red-400">{t('remote.loadError')}</p>
        <button
          type="button"
          onClick={onRetry}
          className="mt-3 rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-red-700 dark:bg-red-500 dark:hover:bg-red-600"
        >
          {t('common.retry')}
        </button>
      </div>
    );
  }

  if (isEmpty) {
    return (
      <div
        className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) py-16 text-center"
        data-testid="remote-resources-empty"
      >
        <Boxes className="mx-auto h-12 w-12 text-gray-300 dark:text-gray-600" aria-hidden="true" />
        <p className="mt-4 text-sm text-(--color-text-muted)">{t('remote.noResources')}</p>
      </div>
    );
  }

  return <>{children}</>;
}
