// 에이전트 라이프사이클 제어 버튼 그룹.
// 시작, 중지, 재시작, 삭제 액션을 인라인 아이콘 버튼으로 제공한다.

import { Play, RotateCcw, Square, Trash2 } from 'lucide-react';

import { useDeleteAgent, useRestartAgent, useStartAgent, useStopAgent } from '@/hooks/useAgent';
import type { AgentInfo } from '@/types/agent';
import { cn } from '@/lib/utils/cn';

interface AgentActionButtonsProps {
  agent: AgentInfo;
  onAction?: () => void;
}

export default function AgentActionButtons({ agent, onAction }: AgentActionButtonsProps) {
  const startAgent = useStartAgent();
  const stopAgent = useStopAgent();
  const restartAgent = useRestartAgent();
  const deleteAgent = useDeleteAgent();

  const isRunning = agent.connected === true;

  /** 에이전트 시작 */
  const handleStart = async (e: React.MouseEvent) => {
    e.stopPropagation();
    await startAgent.mutateAsync(agent.id);
    onAction?.();
  };

  /** 에이전트 중지 */
  const handleStop = async (e: React.MouseEvent) => {
    e.stopPropagation();
    await stopAgent.mutateAsync(agent.id);
    onAction?.();
  };

  /** 에이전트 재시작 */
  const handleRestart = async (e: React.MouseEvent) => {
    e.stopPropagation();
    await restartAgent.mutateAsync(agent.id);
    onAction?.();
  };

  /** 에이전트 삭제 (확인 다이얼로그 표시) */
  const handleDelete = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!window.confirm(`에이전트 "${agent.name}"을(를) 삭제하시겠습니까?`)) return;
    await deleteAgent.mutateAsync(agent.id);
    onAction?.();
  };

  const btnBase =
    'rounded-md p-1.5 text-gray-400 transition-colors hover:text-(--color-text-secondary) disabled:opacity-40 disabled:cursor-not-allowed';

  return (
    <div className="flex items-center justify-end gap-1">
      {/* 시작 */}
      <button
        type="button"
        title="시작"
        disabled={isRunning || startAgent.isPending}
        onClick={handleStart}
        className={cn(btnBase, 'hover:bg-green-50 dark:hover:bg-green-900/20')}
      >
        <Play className="h-4 w-4" />
      </button>

      {/* 중지 */}
      <button
        type="button"
        title="중지"
        disabled={!isRunning || stopAgent.isPending}
        onClick={handleStop}
        className={cn(btnBase, 'hover:bg-yellow-50 dark:hover:bg-yellow-900/20')}
      >
        <Square className="h-4 w-4" />
      </button>

      {/* 재시작 */}
      <button
        type="button"
        title="재시작"
        disabled={restartAgent.isPending}
        onClick={handleRestart}
        className={cn(btnBase, 'hover:bg-blue-50 dark:hover:bg-blue-900/20')}
      >
        <RotateCcw className="h-4 w-4" />
      </button>

      {/* 삭제 */}
      <button
        type="button"
        title="삭제"
        disabled={deleteAgent.isPending}
        onClick={handleDelete}
        className={cn(btnBase, 'hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400')}
      >
        <Trash2 className="h-4 w-4" />
      </button>
    </div>
  );
}
