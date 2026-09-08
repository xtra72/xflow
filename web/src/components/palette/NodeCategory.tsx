// 아코디언 스타일의 노드 카테고리 그룹 컴포넌트.
// 카테고리 헤더 클릭으로 펼치기/접기를 토글한다.

import { useState } from 'react';
import {
  ArrowDownToLine,
  ArrowUpFromLine,
  Cable,
  ChevronRight,
  Cog,
  Database,
  GitBranch,
  ShieldAlert,
  Sparkles,
  type LucideIcon,
} from 'lucide-react';

import type { NodeTypeInfo } from '@/types/node';
import { cn } from '@/lib/utils/cn';

import { NodeItem } from './NodeItem';

/** 카테고리별 아이콘 매핑 */
const categoryIcons: Record<string, LucideIcon> = {
  processing: Cog,
  routing: GitBranch,
  io: Cable,
  error: ShieldAlert,
  storage: Database,
  // 레거시 호환
  input: ArrowDownToLine,
  output: ArrowUpFromLine,
  process: Cog,
  bridge: Cable,
  special: Sparkles,
};

interface NodeCategoryProps {
  category: string;
  nodes: NodeTypeInfo[];
  defaultOpen?: boolean;
}

export function NodeCategory({
  category,
  nodes,
  defaultOpen = false,
}: NodeCategoryProps) {
  const [isOpen, setIsOpen] = useState(defaultOpen);
  const Icon = categoryIcons[category] ?? Cog;

  return (
    <div>
      {/* 카테고리 헤더 - 클릭하면 접기/펼치기 토글 */}
      <button
        type="button"
        onClick={() => setIsOpen((prev) => !prev)}
        className="flex w-full items-center gap-1.5 px-2 py-1.5 text-xs font-semibold
          uppercase tracking-wider text-(--color-text-muted)
          hover:bg-(--color-bg-secondary) transition-colors"
      >
        <ChevronRight
          className={cn(
            'h-3.5 w-3.5 shrink-0 transition-transform',
            isOpen && 'rotate-90',
          )}
        />
        <Icon className="h-3.5 w-3.5 shrink-0" />
        <span>{category}</span>
        <span className="ml-auto text-(--color-text-muted)">
          {nodes.length}
        </span>
      </button>

      {/* 카테고리 내 노드 목록 */}
      {isOpen && (
        <div className="pl-3">
          {nodes.map((node) => (
            <NodeItem key={node.type} nodeType={node} />
          ))}
        </div>
      )}
    </div>
  );
}
