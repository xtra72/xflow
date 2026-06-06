// 플로우 라이프사이클 제어 버튼 그룹 (타깃 인지 — SPEC-REMOTE-001 M8, 그룹 J).
// 시작, 중지, 재시작, 배포, 배포 해제, 내보내기, 삭제 액션을 인라인 아이콘 버튼으로
// 제공한다. 로컬 타깃은 기존 동작과 동일하며, 원격 타깃은 동일 어포던스를 그룹 D
// 명령/M7 편집 경로로 라우팅한다(노드 미지원 액션은 비활성+안내 툴팁).

import { Cable, Download, Play, RotateCcw, Square, Trash2, Unplug } from 'lucide-react';

import { useFlowActionsTarget, type FlowAction } from '@/hooks/useResourceActions';
import { useTargetGating } from '@/hooks/useTargetGating';
import { useTranslation } from '@/lib/i18n';
import { remoteEditErrorMessage } from '@/lib/remote/editError';
import { useTargetContext } from '@/lib/remote/TargetContext';
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
 * 타깃(로컬|원격)에 따라 동일 어포던스가 적절한 백엔드 경로로 라우팅된다.
 */
export default function FlowActionMenu({ flow, onAction }: FlowActionMenuProps) {
  const { t } = useTranslation();
  const target = useTargetContext();
  const actions = useFlowActionsTarget(target);
  const gating = useTargetGating(target);
  const addNotification = useUIStore((s) => s.addNotification);

  const remote = actions.isRemote;
  // 원격 제어 가능 여부: 노드가 승인+온라인일 때만 명령을 적용할 수 있다(REQ-J05).
  // 플로우 자원의 online 은 미러 상태(status)로 별도 노출되지 않으므로 노드 ready 로
  // 판정한다(로컬은 항상 true).
  const nodeControllable = gating.canControl();

  const status = flow.status;
  const canStart = status === 'stored' || status === 'loaded' || status === 'stopped';
  const canStop = status === 'running' || status === 'paused';
  const canRestart = status === 'running' || status === 'paused';
  const canDeploy = status === 'stored';
  const canUndeploy = status === 'loaded' || status === 'stopped' || status === 'error';
  const canDelete = status === 'stored' || status === 'stopped' || status === 'error';

  // 원격 타깃에서 액션 버튼의 disabled/툴팁을 계산한다.
  // - 원격 미지원 액션(restart/undeploy): 항상 비활성 + 미지원 안내.
  // - 노드 미제어(오프라인/미승인): 비활성 + 게이트 안내.
  const remoteState = (action: FlowAction, baseTitle: string): { disabled: boolean; title: string } => {
    if (!remote) return { disabled: false, title: baseTitle };
    if (!actions.supports(action)) {
      return { disabled: true, title: t('remote.edit.unsupportedOnRemote') };
    }
    if (!nodeControllable) {
      return { disabled: true, title: t('remote.edit.actionGateHint') };
    }
    return { disabled: false, title: baseTitle };
  };

  const run = async (action: FlowAction, errLabel: string) => {
    try {
      await actions.perform(action, flow.id);
      onAction?.();
    } catch (err) {
      const message = remote
        ? remoteEditErrorMessage(err, t)
        : `${errLabel}: ${err instanceof Error ? err.message : '알 수 없는 오류'}`;
      addNotification({ type: 'error', message });
    }
  };

  const handleStart = (e: React.MouseEvent) => {
    e.stopPropagation();
    void run('start', '시작 실패');
  };
  const handleStop = (e: React.MouseEvent) => {
    e.stopPropagation();
    void run('stop', '중지 실패');
  };
  const handleRestart = (e: React.MouseEvent) => {
    e.stopPropagation();
    void run('restart', '재시작 실패');
  };
  const handleDeploy = (e: React.MouseEvent) => {
    e.stopPropagation();
    void run('deploy', '배포 실패');
  };
  const handleUndeploy = (e: React.MouseEvent) => {
    e.stopPropagation();
    void run('undeploy', '배포 해제 실패');
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
    await run('delete', '삭제 실패');
  };

  const btnBase =
    'rounded-md p-1.5 text-gray-400 transition-colors hover:text-(--color-text-secondary) disabled:opacity-40 disabled:cursor-not-allowed';

  const start = remoteState('start', '시작');
  const stop = remoteState('stop', '중지');
  const restart = remoteState('restart', '재시작');
  const deploy = remoteState('deploy', '배포');
  const undeploy = remoteState('undeploy', '배포 해제');
  const del = remoteState('delete', '삭제');

  return (
    <div className="inline-flex items-center gap-1">
      {/* 시작 */}
      <button
        type="button"
        title={start.title}
        disabled={!canStart || actions.pending.start || start.disabled}
        onClick={handleStart}
        className={cn(btnBase, 'hover:bg-green-50 dark:hover:bg-green-900/20')}
      >
        <Play className="h-4 w-4" />
      </button>

      {/* 중지 */}
      <button
        type="button"
        title={stop.title}
        disabled={!canStop || actions.pending.stop || stop.disabled}
        onClick={handleStop}
        className={cn(btnBase, 'hover:bg-yellow-50 dark:hover:bg-yellow-900/20')}
      >
        <Square className="h-4 w-4" />
      </button>

      {/* 재시작 (원격 미지원 — 비활성) */}
      <button
        type="button"
        title={restart.title}
        disabled={!canRestart || actions.pending.restart || restart.disabled}
        onClick={handleRestart}
        className={cn(btnBase, 'hover:bg-blue-50 dark:hover:bg-blue-900/20')}
      >
        <RotateCcw className="h-4 w-4" />
      </button>

      {/* 배포 */}
      <button
        type="button"
        title={deploy.title}
        disabled={!canDeploy || actions.pending.deploy || deploy.disabled}
        onClick={handleDeploy}
        className={cn(btnBase, 'hover:bg-purple-50 dark:hover:bg-purple-900/20')}
      >
        <Cable className="h-4 w-4" />
      </button>

      {/* 배포 해제 (원격 미지원 — 비활성) */}
      <button
        type="button"
        title={undeploy.title}
        disabled={!canUndeploy || actions.pending.undeploy || undeploy.disabled}
        onClick={handleUndeploy}
        className={cn(btnBase, 'hover:bg-orange-50 dark:hover:bg-orange-900/20')}
      >
        <Unplug className="h-4 w-4" />
      </button>

      {/* 내보내기 (로컬 전용 — 원격 미러는 redaction 정의만 보유) */}
      {!remote && (
        <button
          type="button"
          title="내보내기"
          onClick={handleExport}
          className={cn(btnBase, 'hover:bg-(--color-bg-elevated)')}
        >
          <Download className="h-4 w-4" />
        </button>
      )}

      {/* 삭제 */}
      <button
        type="button"
        title={del.title}
        disabled={!canDelete || actions.pending.delete || del.disabled}
        onClick={handleDelete}
        className={cn(btnBase, 'hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400')}
      >
        <Trash2 className="h-4 w-4" />
      </button>
    </div>
  );
}
