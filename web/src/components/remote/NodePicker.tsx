// 노드 피커 — 그룹 묶음 드롭다운 (SPEC-REMOTE-001 M11, 그룹 M, REQ-M05, OQ-M3).
//
// 관리자 뷰 상단 바에서 좌측 디렉토리(M9)를 대체하는 노드 선택 컨트롤이다.
// M9 노드 그룹핑(REQ-K01~K05, 단일 레벨 그룹·"전체" 기본 버킷)을 보존하며,
// 노드를 그룹 헤더 아래로 묶어 단일 드롭다운으로 제공한다(공간 절약·다수 노드
// 확장성). 노드를 전환하면 상단 컨텍스트와 하단 노드 화면이 즉시 전환된다.
//
// 그룹/노드 데이터는 NodeManagementPage 의 `sortedGroups`/`nodesByGroup`
// (= `/remote/groups` + `/remote/nodes` 클라이언트 묶기)를 prop 으로 재사용한다
// (신규 왕복 없음).

import { useEffect, useRef, useState } from 'react';
import { Check, ChevronDown, Network } from 'lucide-react';

import { NodeOnlineIndicator } from '@/components/remote/NodeOnlineIndicator';
import { NodeStatusBadge } from '@/components/remote/NodeStatusBadge';
import { useTranslation } from '@/lib/i18n';
import { cn } from '@/lib/utils/cn';
import type { ManagedNode, NodeGroup } from '@/types/remote';

interface NodePickerProps {
  /** 정렬된 그룹 목록("전체" 먼저). */
  groups: NodeGroup[];
  /** group_name → 노드 배열 매핑(빈 라벨 = "전체" 버킷). */
  nodesByGroup: Map<string, ManagedNode[]>;
  /** 현재 선택된 노드(없으면 null). */
  selectedNode: ManagedNode | null;
  /** 노드 선택 핸들러. */
  onSelect: (instanceId: string) => void;
}

/** 그룹 묶음 드롭다운 노드 피커. */
export function NodePicker({
  groups,
  nodesByGroup,
  selectedNode,
  onSelect,
}: NodePickerProps): React.JSX.Element {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  // 외부 클릭 시 닫기.
  useEffect(() => {
    if (!open) return;
    const handler = (e: MouseEvent): void => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [open]);

  const handleSelect = (instanceId: string): void => {
    onSelect(instanceId);
    setOpen(false);
  };

  const buttonLabel = selectedNode
    ? selectedNode.hostname || selectedNode.instance_id
    : t('remote.managerView.pickNode');

  return (
    <div ref={containerRef} className="relative">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-label={t('remote.managerView.nodePickerLabel')}
        data-testid="node-picker-button"
        className="flex min-w-56 max-w-80 items-center gap-2 rounded-md border border-(--color-border-default) bg-(--color-bg-surface) px-3 py-1.5 text-sm font-medium text-(--color-text-primary) transition-colors hover:bg-(--color-bg-elevated)"
      >
        {selectedNode ? (
          <NodeOnlineIndicator online={selectedNode.online} showLabel={false} />
        ) : (
          <Network className="h-4 w-4 shrink-0 text-(--color-text-muted)" aria-hidden="true" />
        )}
        <span className="min-w-0 flex-1 truncate text-left">{buttonLabel}</span>
        {selectedNode && (
          <NodeStatusBadge status={selectedNode.status} className="shrink-0" />
        )}
        <ChevronDown className="h-4 w-4 shrink-0 text-(--color-text-muted)" aria-hidden="true" />
      </button>

      {open && (
        <div
          role="listbox"
          aria-label={t('remote.managerView.nodePickerLabel')}
          data-testid="node-picker-menu"
          className="absolute left-0 z-30 mt-1 max-h-96 w-80 overflow-y-auto rounded-md border border-(--color-border-default) bg-(--color-bg-surface) p-1 shadow-lg"
        >
          {groups.map((group) => {
            const nodes = nodesByGroup.get(group.group_name) ?? [];
            return (
              <div
                key={group.group_name || '__all__'}
                data-testid="picker-group"
                data-group={group.group_name}
              >
                <p className="px-2 pb-0.5 pt-2 text-[10px] font-semibold uppercase tracking-wider text-(--color-text-muted)">
                  {group.group_name || t('remote.group.all')}
                  <span className="ml-1 font-normal">({group.node_count})</span>
                </p>
                {nodes.length === 0 ? (
                  <p className="px-2 py-1 text-xs text-(--color-text-muted)">
                    {t('remote.directory.emptyGroup')}
                  </p>
                ) : (
                  nodes.map((node) => (
                    <button
                      key={node.instance_id}
                      type="button"
                      role="option"
                      aria-selected={selectedNode?.instance_id === node.instance_id}
                      onClick={() => handleSelect(node.instance_id)}
                      data-testid="picker-node"
                      data-instance-id={node.instance_id}
                      className={cn(
                        'flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm transition-colors',
                        selectedNode?.instance_id === node.instance_id
                          ? 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
                          : 'text-(--color-text-primary) hover:bg-(--color-bg-elevated)',
                      )}
                    >
                      <NodeOnlineIndicator online={node.online} showLabel={false} />
                      <span className="min-w-0 flex-1 truncate">
                        {node.hostname || node.instance_id}
                      </span>
                      <NodeStatusBadge status={node.status} className="shrink-0" />
                      {selectedNode?.instance_id === node.instance_id && (
                        <Check className="h-3.5 w-3.5 shrink-0 text-blue-600" aria-hidden="true" />
                      )}
                    </button>
                  ))
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
