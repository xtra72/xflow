// GroupTree 는 그룹 관리 화면의 좌측 트리뷰이다.
//
// 구성(위→아래):
//   1. 명명된 그룹들(정렬) — 펼치면 멤버 노드가 자식으로 표시된다.
//   2. "미분류"(group_name="") — 그룹 미지정 노드. 기본 그룹이라 삭제 불가. 마지막에서 두 번째.
//   3. "신규 그룹" 리프 — 클릭 시 그룹 생성 패널을 연다. 항상 마지막.
//
// 그룹/노드/신규추가 선택을 상위로 위임하고, 펼침 상태만 내부에서 관리한다.

import { useState } from 'react';
import { ChevronDown, ChevronRight, FolderPlus, Layers } from 'lucide-react';

import { NodeOnlineIndicator } from '@/components/remote/NodeOnlineIndicator';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import type { ManagedNode, NodeGroup } from '@/types/remote';

interface GroupTreeProps {
  /** 명명된 그룹(group_name != ""), 정렬됨. */
  namedGroups: NodeGroup[];
  /** group_name → 멤버 노드. "" 키는 미분류. */
  nodesByGroup: Map<string, ManagedNode[]>;
  selectedGroup: string | null; // 선택된 그룹명("" = 미분류). 그룹 미선택이면 null.
  selectedNodeId: string | null;
  createSelected: boolean;
  onSelectGroup: (name: string) => void;
  onSelectNode: (id: string) => void;
  onSelectCreate: () => void;
}

export function GroupTree({
  namedGroups,
  nodesByGroup,
  selectedGroup,
  selectedNodeId,
  createSelected,
  onSelectGroup,
  onSelectNode,
  onSelectCreate,
}: GroupTreeProps): React.JSX.Element {
  const { t } = useTranslation();
  // 선택된 그룹은 기본 펼침. 나머지는 토글.
  const [expanded, setExpanded] = useState<Set<string>>(
    () => new Set(selectedGroup !== null ? [selectedGroup] : []),
  );

  const toggle = (name: string): void => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  };

  const renderGroup = (
    groupName: string,
    label: string,
    isUngrouped: boolean,
  ): React.JSX.Element => {
    const members = nodesByGroup.get(groupName) ?? [];
    const isOpen = expanded.has(groupName);
    const groupActive = selectedGroup === groupName && selectedNodeId === null;
    return (
      <li key={groupName || '__ungrouped__'}>
        <div className="flex items-center">
          <button
            type="button"
            onClick={() => toggle(groupName)}
            aria-label={isOpen ? 'collapse' : 'expand'}
            className="flex h-7 w-6 items-center justify-center text-(--color-text-muted) hover:text-(--color-text-primary)"
          >
            {isOpen ? (
              <ChevronDown className="h-4 w-4" aria-hidden="true" />
            ) : (
              <ChevronRight className="h-4 w-4" aria-hidden="true" />
            )}
          </button>
          <button
            type="button"
            onClick={() => onSelectGroup(groupName)}
            data-testid={`tree-group-${groupName || 'ungrouped'}`}
            className={cn(
              'flex flex-1 items-center gap-1.5 rounded px-1.5 py-1 text-left text-sm',
              groupActive
                ? 'bg-blue-50 font-medium text-blue-700 dark:bg-blue-950 dark:text-blue-200'
                : 'text-(--color-text-primary) hover:bg-(--color-bg-elevated)',
            )}
          >
            <Layers className="h-3.5 w-3.5 shrink-0 text-(--color-text-muted)" aria-hidden="true" />
            <span className="flex-1 truncate">
              {isUngrouped ? t('remote.groupManagement.ungrouped') : label}
            </span>
            <span className="shrink-0 text-xs text-(--color-text-muted)">{members.length}</span>
          </button>
        </div>
        {isOpen && (
          <ul className="ml-6 border-l border-(--color-border-default) pl-1" data-testid={`tree-members-${groupName || 'ungrouped'}`}>
            {members.length === 0 ? (
              <li className="px-2 py-1 text-xs text-(--color-text-muted)">
                {t('remote.groupManagement.noMembers')}
              </li>
            ) : (
              members.map((n) => (
                <li key={n.instance_id}>
                  <button
                    type="button"
                    onClick={() => onSelectNode(n.instance_id)}
                    data-testid={`tree-node-${n.instance_id}`}
                    className={cn(
                      'flex w-full items-center gap-1.5 rounded px-2 py-1 text-left text-xs',
                      selectedNodeId === n.instance_id
                        ? 'bg-blue-50 font-medium text-blue-700 dark:bg-blue-950 dark:text-blue-200'
                        : 'text-(--color-text-secondary) hover:bg-(--color-bg-elevated)',
                    )}
                  >
                    <NodeOnlineIndicator online={n.online} showLabel={false} />
                    <span className="flex-1 truncate">{n.hostname || n.instance_id}</span>
                  </button>
                </li>
              ))
            )}
          </ul>
        )}
      </li>
    );
  };

  return (
    <nav aria-label={t('remote.groupManagement.tree')} data-testid="group-tree">
      <ul className="space-y-0.5">
        {/* 1. 명명된 그룹 */}
        {namedGroups.map((g) => renderGroup(g.group_name, g.group_name, false))}
        {/* 2. 미분류(마지막에서 두 번째, 삭제 불가) */}
        {renderGroup('', t('remote.groupManagement.ungrouped'), true)}
        {/* 3. 신규 그룹(마지막) */}
        <li>
          <button
            type="button"
            onClick={onSelectCreate}
            data-testid="tree-add-group"
            className={cn(
              'mt-1 flex w-full items-center gap-1.5 rounded px-2 py-1.5 text-left text-sm',
              createSelected
                ? 'bg-blue-50 font-medium text-blue-700 dark:bg-blue-950 dark:text-blue-200'
                : 'text-(--color-text-secondary) hover:bg-(--color-bg-elevated)',
            )}
          >
            <FolderPlus className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
            {t('remote.groupManagement.addNew')}
          </button>
        </li>
      </ul>
    </nav>
  );
}
