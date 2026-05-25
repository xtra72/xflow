// React Flow 커스텀 노드 컴포넌트.
// 노드 유형에 따른 아이콘, 상태 표시 점, 입출력 핸들을 렌더링한다.
// 필수 설정이 누락된 노드는 좌측 상단에 경고 뱃지를 표시한다.

import { memo, useCallback, useMemo } from 'react';
import { Position, type NodeProps } from '@xyflow/react';
import {
  AlertTriangle,
  ArrowDownToLine,
  ArrowUpFromLine,
  Bug,
  Cable,
  Cog,
  Database,
  GitBranch,
  Power,
  PowerOff,
  ShieldAlert,
  Sparkles,
  type LucideIcon,
} from 'lucide-react';

import { getRequiredFieldErrors } from '@/config/nodeSchemas';
import { useNodeRuntimeStats } from '@/contexts/RuntimeStatsContext';
import { cn } from '@/lib/utils/cn';
import { useEditorStore } from '@/stores/editorStore';
import { NodeHandle } from './NodeHandle';

/** 카테고리별 아이콘 매핑 */
const CATEGORY_ICONS: Record<string, LucideIcon> = {
  processing: Cog,
  routing: GitBranch,
  io: Cable,
  error: ShieldAlert,
  debug: Bug,
  storage: Database,
  // 레거시 호환
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
  enabled?: boolean;
  ports?: { name: string; direction: 'input' | 'output' | 'error' }[];
  /** v0.18.9: output 노드 전용 — 출력 활성화 여부. 카드 우상단 ON/OFF 버튼과 연동. */
  output_enabled?: boolean;
  [key: string]: unknown;
}

/**
 * 커스텀 노드 컴포넌트.
 * 카테고리별 아이콘, 라벨, 상태 표시, 입출력 핸들을 포함하는 카드 형태로 표시한다.
 */
function CustomNodeComponent({ id, data, selected }: NodeProps) {
  const nodeData = data as CustomNodeData;
  const disabled = nodeData.enabled === false;
  const stats = useNodeRuntimeStats(id);
  const Icon = CATEGORY_ICONS[nodeData.category] ?? Cog;
  const statusColor = stats
    ? (STATUS_COLORS[stats.state] ?? STATUS_COLORS.draft)
    : (STATUS_COLORS[nodeData.status ?? 'draft'] ?? STATUS_COLORS.draft);

  // v0.18.9: output 노드 전용 — 출력 ON/OFF 토글. 패널을 펼치지 않아도
  // 노드 카드에서 직접 토글 가능.
  const isOutputNode = nodeData.nodeType === 'output';
  const outputEnabled = nodeData.output_enabled !== false; // default true
  const updateNodeData = useEditorStore((s) => s.updateNodeData);
  const toggleOutput = useCallback(
    (e: React.MouseEvent) => {
      e.stopPropagation();
      updateNodeData(id, { output_enabled: !outputEnabled });
    },
    [id, outputEnabled, updateNodeData],
  );

  // 필수 필드 누락 검사 (노드 카드에 경고 뱃지 표시용).
  // bridge 처럼 agent_type 에 따라 스키마가 달라지는 노드는 nodeData.agent_type 을 그대로 사용한다.
  const missingRequired = useMemo(() => {
    return getRequiredFieldErrors(
      nodeData.nodeType,
      nodeData as unknown as Record<string, unknown>,
      nodeData.agent_type as string | undefined,
    );
  }, [nodeData]);
  const hasValidationError = missingRequired.length > 0;
  const validationTooltip = hasValidationError
    ? `설정 필요:\n${missingRequired.map((e) => `• ${e.label}`).join('\n')}`
    : undefined;

  // 입력/출력/에러 포트 분리
  const inputPorts = nodeData.ports?.filter((p) => p.direction === 'input') ?? [];
  const outputPorts = nodeData.ports?.filter((p) => p.direction === 'output') ?? [];
  const errorPorts = nodeData.ports?.filter((p) => p.direction === 'error') ?? [];
  // 오른쪽 면에 배치되는 전체 포트 수 (출력 + 에러)
  const rightPorts = [...outputPorts, ...errorPorts];

  return (
    <div
      className={cn(
        'relative rounded-lg border bg-white px-3 py-2 shadow-sm',
        'dark:bg-zinc-900 dark:border-zinc-700',
        'min-w-[140px] transition-shadow duration-150',
        selected
          ? 'ring-2 ring-blue-500 border-blue-500 shadow-md'
          : 'border-zinc-200 hover:shadow-md',
        // 필수 설정이 누락된 경우 호박색 테두리로 시각화 (선택 상태가 우선)
        !selected && hasValidationError && 'border-amber-400 dark:border-amber-600',
        disabled && 'opacity-45',
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

      {/* 필수 설정 누락 경고 뱃지 (좌측 상단) */}
      {hasValidationError && (
        <div
          className={cn(
            'absolute -top-1.5 -left-1.5 flex h-4 w-4 items-center justify-center rounded-full',
            'bg-amber-400 text-white shadow-sm dark:bg-amber-500',
            'ring-1 ring-white dark:ring-zinc-800',
          )}
          title={validationTooltip}
          aria-label={validationTooltip}
        >
          <AlertTriangle className="h-2.5 w-2.5" />
        </div>
      )}

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
        {/* v0.18.9: output 노드 ON/OFF 토글 버튼 */}
        {isOutputNode && (
          <button
            type="button"
            onClick={toggleOutput}
            onMouseDown={(e) => e.stopPropagation()}
            className={cn(
              'flex-shrink-0 rounded-md p-1 transition-colors',
              outputEnabled
                ? 'bg-emerald-100 text-emerald-700 hover:bg-emerald-200 dark:bg-emerald-900/40 dark:text-emerald-400 dark:hover:bg-emerald-900/60'
                : 'bg-zinc-100 text-zinc-400 hover:bg-zinc-200 dark:bg-zinc-800 dark:text-zinc-500 dark:hover:bg-zinc-700',
            )}
            title={outputEnabled ? '출력 ON — 클릭하여 OFF' : '출력 OFF — 클릭하여 ON'}
            aria-label={outputEnabled ? '출력 비활성화' : '출력 활성화'}
          >
            {outputEnabled ? (
              <Power className="h-3 w-3" />
            ) : (
              <PowerOff className="h-3 w-3" />
            )}
          </button>
        )}
      </div>

      {/* 런타임 메시지 통계 (플로우 실행 중일 때만 표시) */}
      {stats && (
        <div className="mt-1.5 flex items-center gap-2 border-t border-zinc-100 pt-1.5 text-[10px] text-zinc-400 dark:border-zinc-700">
          <span className="inline-flex items-center gap-0.5" title="입력">
            <ArrowDownToLine className="h-2.5 w-2.5" />
            {stats.inMessages.toLocaleString()}
          </span>
          <span className="inline-flex items-center gap-0.5" title="출력">
            <ArrowUpFromLine className="h-2.5 w-2.5" />
            {stats.outMessages.toLocaleString()}
          </span>
        </div>
      )}

      {/* 입력 핸들 (왼쪽) */}
      {inputPorts.map((port, i) => (
        <NodeHandle
          key={port.name}
          type="target"
          position={Position.Left}
          id={port.name}
          label={port.name}
          offset={inputPorts.length > 1 ? `${((i + 1) / (inputPorts.length + 1)) * 100}%` : undefined}
        />
      ))}

      {/* 출력 핸들 (오른쪽) */}
      {outputPorts.map((port, i) => (
        <NodeHandle
          key={port.name}
          type="source"
          position={Position.Right}
          id={port.name}
          label={port.name}
          offset={rightPorts.length > 1 ? `${((i + 1) / (rightPorts.length + 1)) * 100}%` : undefined}
        />
      ))}

      {/* 에러 핸들 (오른쪽, 출력 포트 아래) */}
      {errorPorts.map((port, i) => (
        <NodeHandle
          key={port.name}
          type="source"
          position={Position.Right}
          id={port.name}
          label={port.name}
          isError
          offset={rightPorts.length > 1 ? `${((outputPorts.length + i + 1) / (rightPorts.length + 1)) * 100}%` : undefined}
        />
      ))}
    </div>
  );
}

export const CustomNode = memo(CustomNodeComponent);
