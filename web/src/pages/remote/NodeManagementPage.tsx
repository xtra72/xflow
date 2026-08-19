// 관리자 뷰 — 노드 관리 페이지 (SPEC-REMOTE-001 M11, 그룹 M, REQ-M04~M06/M10).
//
// M9 좌측 디렉토리(20rem) + 우측 대시보드 master/detail 레이아웃을 **제자리 진화**
// (OQ-M5)시킨 전용 관리자 뷰이다. 노드 화면을 충실히 재현하기 위해:
//   - 관리 크롬(노드 피커·서브탭 네비·관리 액션·나가기)을 **상단 수평 바**
//     (ManagerViewTopBar)로 옮긴다(REQ-M04).
//   - 선택 노드의 화면이 상단 바 아래 **전체 너비/높이**를 점유한다 → 좌우 폭
//     축소(왜곡)를 방지한다(REQ-M04).
//   - 전역 좌측 Sidebar 는 사용자가 직접 접고 펼친다(자동 접기 안 함). 접힌
//     상태에서도 노드 관리/등록 관리 항목은 사이드바에서 개별 접근 가능하다.
//
// M9 기능은 전부 보존한다(REQ-M10): 노드 그룹핑·그룹 배정/해제·노드 대시보드
// 개요(시스템 정보+운영 요약)·Flow/Agent/Device/대시보드 서브탭·딥링크
// `?node=`/`?tab=`. 그룹/노드 데이터는 `/remote/groups`(전체 포함) + 단일
// `/remote/nodes` 클라이언트 묶기를 그대로 재사용한다(N+1 회피).
//
// 권한/모드: admin 전용 라우트 + server 모드에서만 쿼리를 발행한다.

import { useMemo } from 'react';
import { Link, useSearchParams } from 'react-router';
import { Network } from 'lucide-react';

import {
  ManagerViewTopBar,
  parseManagerViewTab,
  type ManagerViewTab,
} from '@/components/remote/ManagerViewTopBar';
import { NodeDashboard } from '@/components/remote/NodeDashboard';
import { RemoteNotServerNotice } from '@/components/remote/RemoteNotServerNotice';
import {
  useClearNodeGroup,
  useManagedNodes,
  useRemoteGroups,
  useRemoteMode,
  useSetNodeGroup,
} from '@/hooks/useRemote';
import { useTranslation } from '@/lib/i18n';
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

  // 선택 노드와 활성 서브탭을 URL(`?node=`/`?tab=`)에 동기화한다(딥링크/뒤로가기
  // 지원, M9 보존 — REQ-M10). 상단 바와 NodeDashboard 콘텐츠가 각자 이 파라미터를
  // 읽어 단일 출처로 동기화된다.
  const [searchParams, setSearchParams] = useSearchParams();
  const selectedId = searchParams.get('node');
  const activeTab = parseManagerViewTab(searchParams.get('tab'));

  const setSelectedId = (instanceId: string): void => {
    setSearchParams((prev) => {
      const params = new URLSearchParams(prev);
      params.set('node', instanceId);
      // 노드 전환 시 직전 노드의 탭 잔존을 막기 위해 탭을 초기화(개요)한다.
      params.delete('tab');
      return params;
    });
  };

  const exitSelection = (): void => {
    setSearchParams((prev) => {
      const params = new URLSearchParams(prev);
      params.delete('node');
      params.delete('tab');
      return params;
    });
  };

  const setActiveTab = (tab: ManagerViewTab): void => {
    setSearchParams(
      (prev) => {
        const params = new URLSearchParams(prev);
        // overview(기본)는 URL 을 깔끔히 유지하기 위해 파라미터를 제거한다.
        if (tab === 'overview') params.delete('tab');
        else params.set('tab', tab);
        return params;
      },
      { replace: true },
    );
  };

  // group_name → 노드 배열 매핑(빈 라벨 = "전체" 버킷). M9 로직 재사용.
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
    const counts = new Map<string, number>([['', 0]]);
    for (const node of nodes ?? []) {
      const key = node.group_name ?? '';
      counts.set(key, (counts.get(key) ?? 0) + 1);
    }
    return Array.from(counts, ([group_name, node_count]) => ({ group_name, node_count })).sort(
      compareGroups,
    );
  }, [groups, nodes]);

  // 선택된 노드.
  const selectedNode = useMemo<ManagedNode | null>(
    () => (nodes ?? []).find((n) => n.instance_id === selectedId) ?? null,
    [nodes, selectedId],
  );

  const groupNames = useMemo<string[]>(
    () => sortedGroups.map((g) => g.group_name).filter((n) => n !== ''),
    [sortedGroups],
  );

  // 그룹 배정/해제 mutation(선택 노드 대상). M9 동작 보존(REQ-K02/M10).
  const setGroup = useSetNodeGroup();
  const clearGroup = useClearNodeGroup();
  const addNotification = useUIStore((s) => s.addNotification);
  const groupPending = setGroup.isPending || clearGroup.isPending;

  const handleAssignGroup = (groupName: string): void => {
    if (!selectedNode) return;
    setGroup.mutate(
      { instanceID: selectedNode.instance_id, groupName },
      {
        onSuccess: () =>
          addNotification({ type: 'success', message: t('remote.group.toast.assigned') }),
        onError: () =>
          addNotification({ type: 'error', message: t('remote.group.toast.failed') }),
      },
    );
  };

  const handleClearGroup = (): void => {
    if (!selectedNode) return;
    clearGroup.mutate(selectedNode.instance_id, {
      onSuccess: () =>
        addNotification({ type: 'success', message: t('remote.group.toast.cleared') }),
      onError: () =>
        addNotification({ type: 'error', message: t('remote.group.toast.failed') }),
    });
  };

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
        <div className="h-96 animate-pulse rounded bg-(--color-bg-elevated)" />
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

  // --- 관리자 뷰: 상단 바 + 풀폭 노드 화면 ---
  // `-m-6` 으로 AppLayout main 의 p-6 패딩을 상쇄해 상단 바/노드 화면이 콘텐츠
  // 영역을 가장자리까지 풀폭/풀하이트로 점유하게 한다(REQ-M04 — 좌우 폭 축소 방지).
  return (
    <div
      data-testid="manager-view"
      className="-m-6 flex h-[calc(100vh-var(--header-height))] flex-col"
    >
      <ManagerViewTopBar
        groups={sortedGroups}
        nodesByGroup={nodesByGroup}
        selectedNode={selectedNode}
        activeTab={activeTab}
        groupNames={groupNames}
        onSelect={setSelectedId}
        onTabChange={setActiveTab}
        onExit={exitSelection}
        onAssignGroup={handleAssignGroup}
        onClearGroup={handleClearGroup}
        groupPending={groupPending}
      />

      {/* 노드 화면(풀폭/풀하이트) */}
      <div className="min-h-0 flex-1 overflow-auto p-6" data-testid="manager-view-screen">
        {selectedNode ? (
          <NodeDashboard
            key={selectedNode.instance_id}
            instanceId={selectedNode.instance_id}
            enabled={isServer}
            hideTabNav
          />
        ) : (
          <div
            data-testid="node-management-no-selection"
            className="flex h-full min-h-64 flex-col items-center justify-center gap-3 p-8 text-center"
          >
            <Network className="h-10 w-10 text-gray-300 dark:text-gray-600" aria-hidden="true" />
            <p className="text-sm text-(--color-text-muted)">
              {t('remote.managerView.selectNodeHint')}
            </p>
            {/* 그룹 관리는 전용 서브 페이지로 일원화되었다(트리뷰 + 그룹 제어). */}
            <Link
              to="/admin/remote/groups"
              className="text-sm font-medium text-blue-600 hover:underline dark:text-blue-400"
            >
              {t('nav.groupManagement')}
            </Link>
          </div>
        )}
      </div>
    </div>
  );
}

// ---- 페이지 설명(비-server/로딩/에러 상태 전용) ----
//
// 페이지 제목은 앱 헤더(Header 의 PAGE_TITLE_KEYS)가 그린다. 여기 남은 것은
// 제목이 아니라 설명이므로 헤더 래퍼 없이 문단 하나로 둔다.

function PageHeader(): React.JSX.Element {
  const { t } = useTranslation();
  return (
    <p data-testid="node-management-header" className="text-sm text-(--color-text-muted)">
      {t('remote.nodeManagement.subtitle')}
    </p>
  );
}
