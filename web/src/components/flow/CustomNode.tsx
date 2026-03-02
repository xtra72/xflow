// React Flow 커스텀 노드 컴포넌트.
// 노드 유형에 따른 아이콘, 상태 표시 점, 입출력 핸들을 렌더링한다.

import { memo } from 'react';
import { Position, type NodeProps } from '@xyflow/react';
import {
  ArrowDownToLine,
  ArrowUpFromLine,
  Cable,
  Cog,
  Sparkles,
  type LucideIcon,
} from 'lucide-react';

import { cn } from '@/lib/utils/cn';
import { NodeHandle } from './NodeHandle';

/** 카테고리별 아이콘 매핑 */
const CATEGORY_ICONS: Record<string, LucideIcon> = {
  input: ArrowDownToLine,
  output: ArrowUpFromLine,
  process: Cog,
  bridge: Cable,
  special: Sparkles,
};

/** 상태별 색상 매핑 */
const STATUS_COLORS: Record<string, string> = {
  running: 'bg-emerald-500',
  starting: 'bg-yellow-400',
  error: 'bg-red-500',
  stopped: 'bg-zinc-400',
  draft: 'bg-zinc-400',
};

/** 노드 data에 전달되는 속성 */
interface CustomNodeData {
  label: string;
  nodeType: string;
  category: string;
  icon?: string;
  status?: string;
  ports?: { name: string; direction: 'input' | 'output' }[];
  [key: string]: unknown;
}

/**
 * 커스텀 노드 컴포넌트.
 * 카테고리별 아이콘, 라벨, 상태 표시, 입출력 핸들을 포함하는 카드 형태로 표시한다.
 */
function CustomNodeComponent({ data, selected }: NodeProps) {
  const nodeData = data as CustomNodeData;
  const Icon = CATEGORY_ICONS[nodeData.category] ?? Cog;
  const statusColor = STATUS_COLORS[nodeData.status ?? 'draft'] ?? STATUS_COLORS.draft;

  // 입력/출력 포트 분리
  const inputPorts = nodeData.ports?.filter((p) => p.direction === 'input') ?? [];
  const outputPorts = nodeData.ports?.filter((p) => p.direction === 'output') ?? [];

  return (
    <div
      className={cn(
        'relative rounded-lg border bg-white px-3 py-2 shadow-sm',
        'dark:bg-zinc-900 dark:border-zinc-700',
        'min-w-[140px] transition-shadow duration-150',
        selected
          ? 'ring-2 ring-blue-500 border-blue-500 shadow-md'
          : 'border-zinc-200 hover:shadow-md',
      )}
    >
      {/* 상태 표시 점 */}
      <div
        className={cn(
          'absolute -top-1 -right-1 h-2.5 w-2.5 rounded-full border border-white dark:border-zinc-800',
          statusColor,
        )}
        title={nodeData.status ?? 'draft'}
      />

      {/* 아이콘 + 라벨 */}
      <div className="flex items-center gap-2">
        <div className="flex-shrink-0 rounded-md bg-zinc-100 p-1.5 dark:bg-zinc-800">
          <Icon className="h-4 w-4 text-zinc-600 dark:text-zinc-300" />
        </div>
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-medium text-zinc-900 dark:text-zinc-100">
            {nodeData.label}
          </p>
          <p className="truncate text-[10px] text-zinc-400">
            {nodeData.nodeType}
          </p>
        </div>
      </div>

      {/* 입력 핸들 (왼쪽) */}
      {inputPorts.map((port) => (
        <NodeHandle
          key={`in-${port.name}`}
          type="target"
          position={Position.Left}
          id={`in-${port.name}`}
          label={port.name}
        />
      ))}

      {/* 출력 핸들 (오른쪽) */}
      {outputPorts.map((port) => (
        <NodeHandle
          key={`out-${port.name}`}
          type="source"
          position={Position.Right}
          id={`out-${port.name}`}
          label={port.name}
        />
      ))}
    </div>
  );
}

export const CustomNode = memo(CustomNodeComponent);
