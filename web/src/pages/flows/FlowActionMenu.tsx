// 플로우 라이프사이클 제어 버튼 그룹.
// 시작, 중지, 재시작, 배포, 삭제 액션을 인라인 아이콘 버튼으로 제공한다.

import { Cable, Download, Play, RotateCcw, Square, Trash2, Unplug } from 'lucide-react';

import {
  useStartFlow,
  useStopFlow,
  useRestartFlow,
  useDeployFlow,
  useUndeployFlow,
  useDeleteFlow,
} from '@/hooks';
import { cn } from '@/lib/utils/cn';
import { downloadJSON } from '@/lib/utils/download';
import { exportFlow } from '@/services/api/flowService';
import { useUIStore } from '@/stores/uiStore';
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
  const undeployFlow = useUndeployFlow();
  const deleteFlow = useDeleteFlow();
  const addNotification = useUIStore((s) => s.addNotification);

  const status = flow.status;
  const canStart = status === 'stored' || status === 'loaded' || status === 'stopped';
  const canStop = status === 'running' || status === 'paused';
  const canRestart = status === 'running' || status === 'paused';
  const canDeploy = status === 'stored';
  const canUndeploy = status === 'loaded' || status === 'stopped' || status === 'error';
  const canDelete = status === 'stored' || status === 'stopped' || status === 'error';
  const handleStart = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await startFlow.mutateAsync(flow.id);
      onAction?.();
    } catch (err) {
      addNotification({ type: 'error', message: `시작 실패: ${err instanceof Error ? err.message : '알 수 없는 오류'}` });
    }
  };

  const handleStop = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await stopFlow.mutateAsync(flow.id);
      onAction?.();
    } catch (err) {
      addNotification({ type: 'error', message: `중지 실패: ${err instanceof Error ? err.message : '알 수 없는 오류'}` });
    }
  };

  const handleRestart = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await restartFlow.mutateAsync(flow.id);
      onAction?.();
    } catch (err) {
      addNotification({ type: 'error', message: `재시작 실패: ${err instanceof Error ? err.message : '알 수 없는 오류'}` });
    }
  };

  const handleDeploy = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await deployFlow.mutateAsync(flow.id);
      onAction?.();
    } catch (err) {
      addNotification({ type: 'error', message: `배포 실패: ${err instanceof Error ? err.message : '알 수 없는 오류'}` });
    }
  };

  const handleUndeploy = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await undeployFlow.mutateAsync(flow.id);
      onAction?.();
    } catch (err) {
      addNotification({ type: 'error', message: `배포 해제 실패: ${err instanceof Error ? err.message : '알 수 없는 오류'}` });
    }
  };

  const handleExport = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      const data = await exportFlow(flow.id);
      downloadJSON(data, `${flow.name}.json`);
    } catch (err) {
      addNotification({ type: 'error', message: `내보내기 실패: ${err instanceof Error ? err.message : '알 수 없는 오류'}` });
    }
  };

  const handleDelete = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!canDelete) {
      addNotification({ type: 'error', message: '실행 중인 플로우는 중지 후 삭제할 수 있습니다' });
      return;
    }
    if (!window.confirm(`"${flow.name}" 플로우를 삭제하시겠습니까? 이 작업은 되돌릴 수 없습니다.`)) return;
    try {
      await deleteFlow.mutateAsync(flow.id);
      onAction?.();
    } catch (err) {
      addNotification({ type: 'error', message: `삭제 실패: ${err instanceof Error ? err.message : '알 수 없는 오류'}` });
    }
  };

  const btnBase =
    'rounded-md p-1.5 text-gray-400 transition-colors hover:text-(--color-text-secondary) disabled:opacity-40 disabled:cursor-not-allowed';

  return (
    <div className="inline-flex items-center gap-1">
      {/* 시작 */}
      <button
        type="button"
        title="시작"
        disabled={!canStart || startFlow.isPending}
        onClick={handleStart}
        className={cn(btnBase, 'hover:bg-green-50 dark:hover:bg-green-900/20')}
      >
        <Play className="h-4 w-4" />
      </button>

      {/* 중지 */}
      <button
        type="button"
        title="중지"
        disabled={!canStop || stopFlow.isPending}
        onClick={handleStop}
        className={cn(btnBase, 'hover:bg-yellow-50 dark:hover:bg-yellow-900/20')}
      >
        <Square className="h-4 w-4" />
      </button>

      {/* 재시작 */}
      <button
        type="button"
        title="재시작"
        disabled={!canRestart || restartFlow.isPending}
        onClick={handleRestart}
        className={cn(btnBase, 'hover:bg-blue-50 dark:hover:bg-blue-900/20')}
      >
        <RotateCcw className="h-4 w-4" />
      </button>

      {/* 배포 */}
      <button
        type="button"
        title="배포"
        disabled={!canDeploy || deployFlow.isPending}
        onClick={handleDeploy}
        className={cn(btnBase, 'hover:bg-purple-50 dark:hover:bg-purple-900/20')}
      >
        <Cable className="h-4 w-4" />
      </button>

      {/* 배포 해제 */}
      <button
        type="button"
        title="배포 해제"
        disabled={!canUndeploy || undeployFlow.isPending}
        onClick={handleUndeploy}
        className={cn(btnBase, 'hover:bg-orange-50 dark:hover:bg-orange-900/20')}
      >
        <Unplug className="h-4 w-4" />
      </button>

      {/* 내보내기 */}
      <button
        type="button"
        title="내보내기"
        onClick={handleExport}
        className={cn(btnBase, 'hover:bg-(--color-bg-elevated)')}
      >
        <Download className="h-4 w-4" />
      </button>

      {/* 삭제 */}
      <button
        type="button"
        title="삭제"
        disabled={!canDelete || deleteFlow.isPending}
        onClick={handleDelete}
        className={cn(btnBase, 'hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400')}
      >
        <Trash2 className="h-4 w-4" />
      </button>
    </div>
  );
}
