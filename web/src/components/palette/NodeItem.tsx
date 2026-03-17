// 드래그 가능한 노드 팔레트 항목 컴포넌트.
// HTML5 Drag API를 사용하여 에디터 캔버스에 노드를 드롭할 수 있다.

import {
  ArrowDownToLine,
  ArrowUpFromLine,
  Cable,
  Cog,
  Sparkles,
  type LucideIcon,
} from 'lucide-react';

import type { NodeTypeInfo } from '@/types/node';

/** 카테고리별 아이콘 매핑 */
const categoryIcons: Record<string, LucideIcon> = {
  input: ArrowDownToLine,
  output: ArrowUpFromLine,
  process: Cog,
  bridge: Cable,
  special: Sparkles,
};

interface NodeItemProps {
  nodeType: NodeTypeInfo;
}

export function NodeItem({ nodeType }: NodeItemProps) {
  const Icon = categoryIcons[nodeType.category] ?? Cog;

  /** 드래그 시작 시 노드 타입 정보를 dataTransfer에 저장 */
  function handleDragStart(event: React.DragEvent) {
    event.dataTransfer.setData(
      'application/xflow-node',
      JSON.stringify(nodeType),
    );
    event.dataTransfer.effectAllowed = 'move';
  }

  return (
    <div
      draggable
      onDragStart={handleDragStart}
      className="flex items-center gap-2 rounded px-2 py-1.5 cursor-grab
        hover:bg-gray-100 dark:hover:bg-gray-800
        active:cursor-grabbing transition-colors"
    >
      <Icon className="h-3.5 w-3.5 shrink-0 text-(--color-text-muted)" />
      <div className="min-w-0">
        <p className="truncate text-sm font-medium text-(--color-text-secondary)">
          {nodeType.type}
        </p>
        {nodeType.description && (
          <p className="truncate text-xs text-(--color-text-muted)">
            {nodeType.description}
          </p>
        )}
      </div>
    </div>
  );
}
