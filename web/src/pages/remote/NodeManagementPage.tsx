// 노드 관리 페이지 (SPEC-REMOTE-001 M9, 그룹 K, REQ-K11~K14).
//
// 사이드바 `원격 관리`의 운영 진입점이다. master/detail 레이아웃:
//   LEFT  — 디렉토리 뷰: 단일 레벨 그룹 트리(전체 먼저, 그룹별 노드 수). 그룹을
//           펼치면 소속 노드 목록(online/status 표식)을 보여준다. 노드별 그룹
//           배정/해제 affordance 를 제공한다(REQ-K02/K12).
//   RIGHT — 노드 선택 시 노드 대시보드(시스템 정보 + 운영 요약 + Flow/Agent/Device
//           서브탭이 M8 통합 제어 재사용 — REQ-K13/K14).
//
// 그룹 목록은 `GET /remote/groups`(전체 가상 버킷 포함)에서, 노드는 단일
// `GET /remote/nodes` 응답을 group_name 으로 클라이언트 묶기 한다(N+1 회피).
//
// 권한/모드: admin 전용 라우트 + server 모드에서만 쿼리를 발행한다.

import { useMemo, useState } from 'react';
import { ChevronDown, ChevronRight, FolderOpen, Network } from 'lucide-react';

import { NodeOnlineIndicator } from '@/components/remote/NodeOnlineIndicator';
import { NodeStatusBadge } from '@/components/remote/NodeStatusBadge';
import { NodeDashboard } from '@/components/remote/NodeDashboard';
import { NodeGroupMenu } from '@/components/remote/NodeGroupMenu';
import { RemoteNotServerNotice } from '@/components/remote/RemoteNotServerNotice';
import {
  useClearNodeGroup,
  useManagedNodes,
  useRemoteGroups,
  useRemoteMode,
  useSetNodeGroup,
} from '@/hooks/useRemote';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import { useUIStore } from '@/stores/uiStore';
import type { ManagedNode, NodeGroup } from '@/types/remote';

/** 빈 라벨("전체") 정렬 시 항상 맨 앞으로 보내기 위한 정렬 비교자. */
function compareGroups(a: NodeGroup, b: NodeGroup): number {
  if (a.group_name === b.group_name) return 0;
  if (a.group_name === '') return -1; // "전체" 먼저.
  if (b.group_name === '') return 1;
  return a.group_name.localeCompare(b.group_name);
}

export default function NodeManagementPage(): React.JSX.Element {
  const { t } = useTranslation();
  const { data: remoteMode } = useRemoteMode();
  const isServer = remoteMode?.mode === 'server';

  const { data: groups } = useRemoteGroups(isServer);
  const { data: nodes, isLoading, error, refetch } = useManagedNodes(undefined, isServer);

  const [selectedId, setSelectedId] = useState<string | null>(null);

  // group_name → 노드 배열 매핑(빈 라벨 = "전체" 버킷).
  const nodesByGroup = useMemo<Map<string, ManagedNode[]>>(() => {
    const map = new Map<string, ManagedNode[]>();
    for (const node of nodes ?? []) {
      const key = node.group_name ?? '';
      const list = map.get(key);
      if (list) list.push(node);
      else map.set(key, [node]);
    }
    return map;
  }, [nodes]);

  // 표시할 그룹 목록: `/remote/groups`(전체 포함) 우선, 폴백으로 노드에서 파생.
  const sortedGroups = useMemo<NodeGroup[]>(() => {
    if (groups && groups.length > 0) return [...groups].sort(compareGroups);
    // 폴백: 그룹 API 미응답 시 노드의 group_name 으로 distinct 집계("전체" 포함).
    const counts = new Map<string, number>([['', 0]]);
    for (const node of nodes ?? []) {
      const key = node.group_name ?? '';
      counts.set(key, (counts.get(key) ?? 0) + 1);
    }
    return Array.from(counts, ([group_name, node_count]) => ({ group_name, node_count })).sort(
      compareGroups,
    );
  }, [groups, nodes]);

  // 선택된 노드(상세 패널 표시용).
  const selectedNode = useMemo<ManagedNode | null>(
    () => (nodes ?? []).find((n) => n.instance_id === selectedId) ?? null,
    [nodes, selectedId],
  );

  // --- 비-server 모드 ---
  if (remoteMode && !isServer) {
    return (
      <div className="space-y-6">
        <PageHeader />
        <RemoteNotServerNotice />
      </div>
    );
  }

  // --- 로딩 ---
  if (!remoteMode || (isLoading && !nodes)) {
    return (
      <div className="space-y-6" data-testid="node-management-loading">
        <PageHeader />
        <div className="grid grid-cols-1 gap-4 lg:grid-cols-[20rem_1fr]">
          <div className="h-96 animate-pulse rounded bg-(--color-bg-elevated)" />
          <div className="h-96 animate-pulse rounded bg-(--color-bg-elevated)" />
        </div>
      </div>
    );
  }

  // --- 에러 ---
  if (error) {
    return (
      <div className="space-y-6">
        <PageHeader />
        <div
          data-testid="node-management-error"
          className="rounded-md border border-red-200 bg-red-50 p-6 text-center dark:border-red-800 dark:bg-red-900/20"
        >
          <p className="text-sm text-red-700 dark:text-red-400">{t('remote.loadError')}</p>
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

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-[20rem_1fr]">
        {/* LEFT: 디렉토리 뷰 */}
        <NodeDirectory
          groups={sortedGroups}
          nodesByGroup={nodesByGroup}
          selectedId={selectedId}
          onSelect={setSelectedId}
          allGroupNames={sortedGroups.map((g) => g.group_name).filter((n) => n !== '')}
        />

        {/* RIGHT: 노드 대시보드 또는 빈 안내 */}
        <div className="min-w-0">
          {selectedNode ? (
            <NodeDashboard
              key={selectedNode.instance_id}
              instanceId={selectedNode.instance_id}
              enabled={isServer}
            />
          ) : (
            <div
              data-testid="node-management-no-selection"
              className="flex h-full min-h-64 flex-col items-center justify-center rounded-lg border border-dashed border-(--color-border-default) bg-(--color-bg-surface) p-8 text-center"
            >
              <Network className="h-10 w-10 text-gray-300 dark:text-gray-600" aria-hidden="true" />
              <p className="mt-3 text-sm text-(--color-text-muted)">
                {t('remote.directory.selectNodeHint')}
              </p>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

// ---- 디렉토리 뷰(좌측) ----

interface NodeDirectoryProps {
  groups: NodeGroup[];
  nodesByGroup: Map<string, ManagedNode[]>;
  selectedId: string | null;
  onSelect: (instanceId: string) => void;
  /** 그룹 배정 메뉴에 제안할 기존 그룹 라벨 목록("전체" 제외). */
  allGroupNames: string[];
}

/** 단일 레벨 그룹 트리(전체 먼저) + 그룹별 노드 목록. */
function NodeDirectory({
  groups,
  nodesByGroup,
  selectedId,
  onSelect,
  allGroupNames,
}: NodeDirectoryProps): React.JSX.Element {
  const { t } = useTranslation();

  return (
    <nav
      data-testid="node-directory"
      aria-label={t('remote.directory.label')}
      className="space-y-1 rounded-lg border border-(--color-border-default) bg-(--color-bg-surface) p-2"
    >
      {groups.map((group) => (
        <GroupNode
          key={group.group_name || '__all__'}
          group={group}
          nodes={nodesByGroup.get(group.group_name) ?? []}
          selectedId={selectedId}
          onSelect={onSelect}
          allGroupNames={allGroupNames}
        />
      ))}
    </nav>
  );
}

// ---- 그룹 노드(접기/펼치기 + 노드 목록) ----

interface GroupNodeProps {
  group: NodeGroup;
  nodes: ManagedNode[];
  selectedId: string | null;
  onSelect: (instanceId: string) => void;
  allGroupNames: string[];
}

function GroupNode({
  group,
  nodes,
  selectedId,
  onSelect,
  allGroupNames,
}: GroupNodeProps): React.JSX.Element {
  const { t } = useTranslation();
  const [open, setOpen] = useState(true);
  const label = group.group_name || t('remote.group.all');

  return (
    <div data-testid="directory-group" data-group={group.group_name}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm font-medium text-(--color-text-secondary) transition-colors hover:bg-(--color-bg-elevated)"
      >
        {open ? (
          <ChevronDown className="h-4 w-4 shrink-0" aria-hidden="true" />
        ) : (
          <ChevronRight className="h-4 w-4 shrink-0" aria-hidden="true" />
        )}
        <FolderOpen className="h-4 w-4 shrink-0 text-(--color-text-muted)" aria-hidden="true" />
        <span className="flex-1 truncate text-left">{label}</span>
        <span className="shrink-0 rounded-full bg-(--color-bg-elevated) px-1.5 py-0.5 text-[10px] font-medium text-(--color-text-muted)">
          {group.node_count}
        </span>
      </button>

      {open && (
        <ul className="ml-3 mt-0.5 space-y-0.5 border-l border-(--color-border-default) pl-2">
          {nodes.length === 0 ? (
            <li className="px-2 py-1.5 text-xs text-(--color-text-muted)">
              {t('remote.directory.emptyGroup')}
            </li>
          ) : (
            nodes.map((node) => (
              <DirectoryNodeRow
                key={node.instance_id}
                node={node}
                selected={selectedId === node.instance_id}
                onSelect={() => onSelect(node.instance_id)}
                allGroupNames={allGroupNames}
              />
            ))
          )}
        </ul>
      )}
    </div>
  );
}

// ---- 노드 행(선택 + 그룹 배정 메뉴) ----

interface DirectoryNodeRowProps {
  node: ManagedNode;
  selected: boolean;
  onSelect: () => void;
  allGroupNames: string[];
}

function DirectoryNodeRow({
  node,
  selected,
  onSelect,
  allGroupNames,
}: DirectoryNodeRowProps): React.JSX.Element {
  const setGroup = useSetNodeGroup();
  const clearGroup = useClearNodeGroup();
  const addNotification = useUIStore((s) => s.addNotification);
  const { t } = useTranslation();

  const handleAssign = (groupName: string): void => {
    setGroup.mutate(
      { instanceID: node.instance_id, groupName },
      {
        onSuccess: () =>
          addNotification({ type: 'success', message: t('remote.group.toast.assigned') }),
        onError: () =>
          addNotification({ type: 'error', message: t('remote.group.toast.failed') }),
      },
    );
  };

  const handleClear = (): void => {
    clearGroup.mutate(node.instance_id, {
      onSuccess: () =>
        addNotification({ type: 'success', message: t('remote.group.toast.cleared') }),
      onError: () =>
        addNotification({ type: 'error', message: t('remote.group.toast.failed') }),
    });
  };

  return (
    <li
      data-testid="directory-node"
      data-instance-id={node.instance_id}
      className={cn(
        'group flex items-center gap-2 rounded-md px-2 py-1.5',
        selected ? 'bg-blue-50 dark:bg-blue-900/30' : 'hover:bg-(--color-bg-elevated)',
      )}
    >
      <button
        type="button"
        onClick={onSelect}
        className="flex min-w-0 flex-1 items-center gap-2 text-left"
      >
        <NodeOnlineIndicator online={node.online} showLabel={false} />
        <span
          className={cn(
            'truncate text-sm font-medium',
            selected ? 'text-blue-700 dark:text-blue-300' : 'text-(--color-text-primary)',
          )}
        >
          {node.hostname || node.instance_id}
        </span>
        <NodeStatusBadge status={node.status} className="ml-auto shrink-0" />
      </button>

      <NodeGroupMenu
        currentGroup={node.group_name ?? ''}
        groupNames={allGroupNames}
        pending={setGroup.isPending || clearGroup.isPending}
        onAssign={handleAssign}
        onClear={handleClear}
      />
    </li>
  );
}

// ---- 페이지 헤더 ----

function PageHeader(): React.JSX.Element {
  const { t } = useTranslation();
  return (
    <header data-testid="node-management-header">
      <h1 className="text-2xl font-semibold text-(--color-text-primary)">
        {t('remote.nodeManagement.title')}
      </h1>
      <p className="mt-1 text-sm text-(--color-text-muted)">
        {t('remote.nodeManagement.subtitle')}
      </p>
    </header>
  );
}
