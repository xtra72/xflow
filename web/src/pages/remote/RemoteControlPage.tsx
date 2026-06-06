// 원격 노드 제어 셀렉터 페이지 (SPEC-REMOTE-001 M8, 그룹 J, REQ-J13/J14).
//
// 승인된 원격 노드를 나열하고, 각 노드를 로컬과 동일한 목록/제어 화면으로
// 라우팅한다(`/flows?target=remote:{id}` 등). 별도 원격 전용 자원 화면 대신
// 로컬 페이지를 재사용하므로(REQ-J10/J14), 본 페이지는 "진입점(셀렉터)" 역할만
// 한다. 기존 RemoteResourcesPage(미러 뷰)는 그대로 두되, 사이드바는 본 셀렉터를
// 원격 제어의 1차 진입점으로 노출한다.
//
// 권한/모드: admin 전용 라우트 + server 모드에서만 노드 쿼리를 발행한다.

import { Bot, HardDrive, Network, Workflow } from 'lucide-react';
import { Link } from 'react-router';

import { RemoteNotServerNotice } from '@/components/remote/RemoteNotServerNotice';
import { useManagedNodes, useRemoteMode } from '@/hooks/useRemote';
import { useTranslation } from '@/lib/i18n';
import { serializeTargetParam } from '@/lib/remote/target';
import { cn } from '@/lib/utils/cn';
import type { ManagedNode } from '@/types/remote';

/** 한 노드의 자원 화면으로 가는 target 쿼리를 만든다. */
function targetQuery(instanceId: string): string {
  return `?target=${serializeTargetParam({ type: 'remote', instanceId })}`;
}

export default function RemoteControlPage(): React.JSX.Element {
  const { t } = useTranslation();
  const { data: remoteMode } = useRemoteMode();
  const isServer = remoteMode?.mode === 'server';
  const { data: nodes } = useManagedNodes(undefined, isServer);

  const approved = (nodes ?? []).filter((n) => n.status === 'approved');

  if (remoteMode && !isServer) {
    return (
      <div className="space-y-6">
        <SelectorHeader />
        <RemoteNotServerNotice />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <SelectorHeader />

      {approved.length === 0 ? (
        <div
          data-testid="remote-control-empty"
          className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) py-16 text-center"
        >
          <Network className="mx-auto h-12 w-12 text-gray-300 dark:text-gray-600" aria-hidden="true" />
          <p className="mt-4 text-sm text-(--color-text-muted)">{t('remote.selector.empty')}</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {approved.map((node) => (
            <NodeCard key={node.instance_id} node={node} />
          ))}
        </div>
      )}
    </div>
  );
}

function SelectorHeader(): React.JSX.Element {
  const { t } = useTranslation();
  return (
    <header data-testid="remote-control-header">
      <h1 className="text-2xl font-semibold text-(--color-text-primary)">
        {t('remote.selector.title')}
      </h1>
      <p className="mt-1 text-sm text-(--color-text-muted)">{t('remote.selector.subtitle')}</p>
    </header>
  );
}

/** 노드 1개 카드 — 플로우/에이전트/디바이스 화면으로의 링크 3종. */
function NodeCard({ node }: { node: ManagedNode }): React.JSX.Element {
  const { t } = useTranslation();
  const q = targetQuery(node.instance_id);
  const links = [
    { to: `/flows${q}`, label: t('remote.selector.openFlows'), Icon: Workflow },
    { to: `/agents${q}`, label: t('remote.selector.openAgents'), Icon: Bot },
    { to: `/devices${q}`, label: t('remote.selector.openDevices'), Icon: HardDrive },
  ];

  return (
    <div
      data-testid={`remote-control-node-${node.instance_id}`}
      className="rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-4"
    >
      <div className="mb-3 flex items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <Network className="h-5 w-5 shrink-0 text-(--color-text-muted)" aria-hidden="true" />
          <span className="truncate text-sm font-semibold text-(--color-text-primary)">
            {node.hostname || node.instance_id}
          </span>
        </div>
        <span
          className={cn(
            'inline-flex shrink-0 items-center gap-1 rounded-full px-2 py-0.5 text-[10px] font-medium',
            node.online
              ? 'bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-300'
              : 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400',
          )}
        >
          <span className={cn('h-1.5 w-1.5 rounded-full', node.online ? 'bg-green-500' : 'bg-gray-400')} />
          {node.online ? t('remote.online') : t('remote.selector.nodeOffline')}
        </span>
      </div>
      <div className="grid grid-cols-3 gap-2">
        {links.map(({ to, label, Icon }) => (
          <Link
            key={to}
            to={to}
            className="inline-flex flex-col items-center gap-1 rounded-md border border-(--color-border-strong) px-2 py-2.5 text-xs font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)"
          >
            <Icon className="h-4 w-4" aria-hidden="true" />
            {label}
          </Link>
        ))}
      </div>
    </div>
  );
}
