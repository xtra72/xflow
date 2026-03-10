// 플로우 라이프사이클 제어 버튼 그룹.
// 시작, 중지, 재시작, 배포, 삭제 액션을 인라인 아이콘 버튼으로 제공한다.

import { Download, Play, RotateCcw, Square, Trash2, Upload } from 'lucide-react';

import {
  useStartFlow,
  useStopFlow,
  useRestartFlow,
  useDeployFlow,
  useDeleteFlow,
} from '@/hooks';
import { cn } from '@/lib/utils/cn';
import { downloadJSON } from '@/lib/utils/download';
import { exportFlow } from '@/services/api/flowService';
import type { FlowInfo } from '@/types/flow';

interface FlowActionMenuProps {
  flow: FlowInfo;
  onAction?: () => void;
}

/**
 * 플로우 액션 버튼 그룹.
 * 플로우 상태에 따라 활성화/비활성화되는 lifecycle 동작을 인라인 아이콘 버튼으로 제공한다.
 */
export default function FlowActionMenu({ flow, onAction }: FlowActionMenuProps) {
  const startFlow = useStartFlow();
  const stopFlow = useStopFlow();
  const restartFlow = useRestartFlow();
  const deployFlow = useDeployFlow();
  const deleteFlow = useDeleteFlow();

  const isRunning = flow.status === 'Running';

  const handleStart = async (e: React.MouseEvent) => {
    e.stopPropagation();
    await startFlow.mutateAsync(flow.id);
    onAction?.();
  };

  const handleStop = async (e: React.MouseEvent) => {
    e.stopPropagation();
    await stopFlow.mutateAsync(flow.id);
    onAction?.();
  };

  const handleRestart = async (e: React.MouseEvent) => {
    e.stopPropagation();
    await restartFlow.mutateAsync(flow.id);
    onAction?.();
  };

  const handleDeploy = async (e: React.MouseEvent) => {
    e.stopPropagation();
    await deployFlow.mutateAsync(flow.id);
    onAction?.();
  };

  const handleExport = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      const data = await exportFlow(flow.id);
      downloadJSON(data, `${flow.name}.json`);
    } catch {
      // 내보내기 실패 시 무시 (콘솔에 에러 출력)
    }
  };

  const handleDelete = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!window.confirm(`"${flow.name}" 플로우를 삭제하시겠습니까? 이 작업은 되돌릴 수 없습니다.`)) return;
    await deleteFlow.mutateAsync(flow.id);
    onAction?.();
  };

  const btnBase =
    'rounded-md p-1.5 text-gray-400 transition-colors hover:text-gray-600 dark:hover:text-gray-300 disabled:opacity-40 disabled:cursor-not-allowed';

  return (
    <div className="inline-flex items-center gap-1">
      {/* 시작 */}
      <button
        type="button"
        title="시작"
        disabled={isRunning || startFlow.isPending}
        onClick={handleStart}
        className={cn(btnBase, 'hover:bg-green-50 dark:hover:bg-green-900/20')}
      >
        <Play className="h-4 w-4" />
      </button>

      {/* 중지 */}
      <button
        type="button"
        title="중지"
        disabled={!isRunning || stopFlow.isPending}
        onClick={handleStop}
        className={cn(btnBase, 'hover:bg-yellow-50 dark:hover:bg-yellow-900/20')}
      >
        <Square className="h-4 w-4" />
      </button>

      {/* 재시작 */}
      <button
        type="button"
        title="재시작"
        disabled={restartFlow.isPending}
        onClick={handleRestart}
        className={cn(btnBase, 'hover:bg-blue-50 dark:hover:bg-blue-900/20')}
      >
        <RotateCcw className="h-4 w-4" />
      </button>

      {/* 배포 */}
      <button
        type="button"
        title="배포"
        disabled={isRunning || deployFlow.isPending}
        onClick={handleDeploy}
        className={cn(btnBase, 'hover:bg-purple-50 dark:hover:bg-purple-900/20')}
      >
        <Upload className="h-4 w-4" />
      </button>

      {/* 내보내기 */}
      <button
        type="button"
        title="내보내기"
        onClick={handleExport}
        className={cn(btnBase, 'hover:bg-gray-100 dark:hover:bg-gray-700')}
      >
        <Download className="h-4 w-4" />
      </button>

      {/* 삭제 */}
      <button
        type="button"
        title="삭제"
        disabled={isRunning || deleteFlow.isPending}
        onClick={handleDelete}
        className={cn(btnBase, 'hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400')}
      >
        <Trash2 className="h-4 w-4" />
      </button>
    </div>
  );
}
