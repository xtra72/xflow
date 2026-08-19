// GroupManagementPage 는 원격 관리 하위의 "그룹 관리" 서브 페이지이다.
//
// 2분할 레이아웃:
//   - 좌측: 그룹 트리(명명 그룹 → 멤버 노드, 미분류, 신규 그룹).
//   - 우측: 선택 컨텍스트
//       · 그룹 선택   → 그룹 제어(이름변경/삭제/일괄 업데이트/명령 + 멤버 목록)
//       · 노드 선택   → 노드 상세(시스템 정보 + 버전 관리 + 운영 요약, NodeDashboard 재사용)
//       · 신규 그룹   → 그룹 생성(이름 + 노드 다중선택)

import { useEffect, useMemo, useState } from 'react';

import { GroupControlPanel } from '@/components/remote/GroupControlPanel';
import { GroupCreatePanel } from '@/components/remote/GroupCreatePanel';
import { GroupTree } from '@/components/remote/GroupTree';
import { NodeDashboard } from '@/components/remote/NodeDashboard';
import { RemoteNotServerNotice } from '@/components/remote/RemoteNotServerNotice';
import { UpdateSourceSettings } from '@/components/remote/UpdateSourceSettings';
import {
  useManagedNodes,
  useRemoteGroups,
  useRemoteMode,
} from '@/hooks/useRemote';
import { useTranslation } from '@/lib/i18n';
import type { ManagedNode, NodeGroup } from '@/types/remote';

type Selection =
  | { kind: 'group'; name: string }
  | { kind: 'node'; id: string }
  | { kind: 'create' };

// 페이지 제목은 앱 헤더(Header 의 PAGE_TITLE_KEYS)가 그린다. 여기 남은 설명
// 밴드는 아래 2분할(트리 | 상세) 영역과 맞닿으므로 구분선(border-b)은 유지한다 —
// 선을 빼면 설명 문구가 좌측 트리 첫 항목에 붙어 보인다.
function PageHeader(): React.JSX.Element {
  const { t } = useTranslation();
  return (
    <div className="border-b border-(--color-border-default) px-6 py-3">
      <p className="text-xs text-(--color-text-muted)">
        {t('remote.groupManagement.subtitle')}
      </p>
    </div>
  );
}

export default function GroupManagementPage(): React.JSX.Element {
  const { t } = useTranslation();
  const { data: remoteMode } = useRemoteMode();
  const isServer = remoteMode?.mode === 'server';

  const { data: groups } = useRemoteGroups(isServer);
  const { data: nodes } = useManagedNodes(undefined, isServer);

  const [selection, setSelection] = useState<Selection | null>(null);

  // group_name → 멤버 노드 매핑("" = 미분류).
  const nodesByGroup = useMemo<Map<string, ManagedNode[]>>(() => {
    const map = new Map<string, ManagedNode[]>();
    for (const n of nodes ?? []) {
      const key = n.group_name ?? '';
      const arr = map.get(key) ?? [];
      arr.push(n);
      map.set(key, arr);
    }
    return map;
  }, [nodes]);

  // 명명된 그룹(빈 라벨 제외) 정렬. /remote/groups 우선, 폴백으로 노드에서 파생.
  const namedGroups = useMemo<NodeGroup[]>(() => {
    const source: NodeGroup[] =
      groups && groups.length > 0
        ? groups.filter((g) => g.group_name !== '')
        : [...nodesByGroup.keys()]
            .filter((k) => k !== '')
            .map((k) => ({ group_name: k, node_count: nodesByGroup.get(k)?.length ?? 0 }));
    return [...source].sort((a, b) => a.group_name.localeCompare(b.group_name));
  }, [groups, nodesByGroup]);

  // 기본 선택: 첫 명명 그룹 → 없으면 미분류.
  useEffect(() => {
    if (selection !== null || !nodes) return;
    if (namedGroups.length > 0) {
      setSelection({ kind: 'group', name: namedGroups[0]!.group_name });
    } else {
      setSelection({ kind: 'group', name: '' });
    }
  }, [selection, nodes, namedGroups]);

  // 선택된 노드/그룹이 사라지면(삭제·이동) 안전하게 미분류로 환원.
  useEffect(() => {
    if (!nodes || selection === null) return;
    if (selection.kind === 'node') {
      const exists = (nodes ?? []).some((n) => n.instance_id === selection.id);
      if (!exists) setSelection({ kind: 'group', name: '' });
    }
  }, [nodes, selection]);

  if (remoteMode && !isServer) {
    return (
      <div className="space-y-6">
        <PageHeader />
        <RemoteNotServerNotice />
      </div>
    );
  }
  if (!remoteMode || !nodes) {
    return (
      <div>
        <PageHeader />
        <div className="m-6 h-96 animate-pulse rounded bg-(--color-bg-elevated)" data-testid="group-page-loading" />
      </div>
    );
  }

  const selectedGroup =
    selection?.kind === 'group' ? selection.name : null;
  const selectedNodeId = selection?.kind === 'node' ? selection.id : null;
  const createSelected = selection?.kind === 'create';

  return (
    <div
      className="-m-6 flex h-[calc(100vh-var(--header-height))] flex-col"
      data-testid="group-management-page"
    >
      <PageHeader />
      <div className="flex min-h-0 flex-1">
        {/* 좌측: 업데이트 소스(전역 설정) + 그룹 트리 */}
        <aside className="w-72 shrink-0 space-y-3 overflow-y-auto border-r border-(--color-border-default) p-3">
          <UpdateSourceSettings />
          <GroupTree
            namedGroups={namedGroups}
            nodesByGroup={nodesByGroup}
            selectedGroup={selectedGroup}
            selectedNodeId={selectedNodeId}
            createSelected={createSelected}
            onSelectGroup={(name) => setSelection({ kind: 'group', name })}
            onSelectNode={(id) => setSelection({ kind: 'node', id })}
            onSelectCreate={() => setSelection({ kind: 'create' })}
          />
        </aside>

        {/* 우측: 선택 컨텍스트 */}
        <div className="min-h-0 flex-1 overflow-y-auto p-6" data-testid="group-detail-pane">
          {createSelected ? (
            <GroupCreatePanel
              nodes={nodes}
              onCreated={(name) => setSelection({ kind: 'group', name })}
            />
          ) : selectedNodeId !== null ? (
            <NodeDashboard instanceId={selectedNodeId} enabled={isServer} hideTabNav />
          ) : selection?.kind === 'group' ? (
            <GroupControlPanel
              groupName={selection.name}
              members={nodesByGroup.get(selection.name) ?? []}
              onSelectNode={(id) => setSelection({ kind: 'node', id })}
              onMutated={(next) =>
                setSelection(next === null ? { kind: 'group', name: '' } : { kind: 'group', name: next })
              }
            />
          ) : (
            <p className="text-sm text-(--color-text-muted)">
              {t('remote.groupManagement.selectHint')}
            </p>
          )}
        </div>
      </div>
    </div>
  );
}
