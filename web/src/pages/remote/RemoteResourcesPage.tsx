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

import { useMemo, useState } from 'react';
import { Boxes } from 'lucide-react';

import { MirrorResourceTable } from '@/components/remote/MirrorResourceTable';
import { RemoteNotServerNotice } from '@/components/remote/RemoteNotServerNotice';
import {
  useAllMirror,
  useManagedNodes,
  useNodeMirror,
  useRemoteMode,
} from '@/hooks/useRemote';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import type { MirroredResourceKind } from '@/types/remote';

/** 뷰 모드. */
type ViewMode = 'aggregated' | 'per-node';

/** 종류 탭 정의. */
const KIND_TABS: { kind: MirroredResourceKind; labelKey: string }[] = [
  { kind: 'flow', labelKey: 'remote.kind.flows' },
  { kind: 'agent', labelKey: 'remote.kind.agents' },
  { kind: 'device', labelKey: 'remote.kind.devices' },
];

export default function RemoteResourcesPage(): React.JSX.Element {
  const { t } = useTranslation();
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
        />
      </ResourceBody>
    </div>
  );
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
