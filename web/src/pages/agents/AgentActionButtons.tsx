// 에이전트 라이프사이클 제어 버튼 그룹 (타깃 인지 — SPEC-REMOTE-001 M8, 그룹 J).
// 시작, 중지, 재시작, Enable/Disable, 내보내기, 삭제 액션을 인라인 아이콘 버튼으로
// 제공한다. 로컬 타깃은 기존 동작과 동일하며, 원격 타깃은 라이프사이클을 그룹 D
// 명령으로, 삭제를 M7 편집 경로로 라우팅한다. 노드가 지원하지 않는 enable/disable
// 은 원격에서 비활성+안내 툴팁으로 표시한다(가짜 동작 금지).
//
// SPEC-AUTH-006 E1 (M4): 각 컨트롤에 권한 키를 부착한다. 라이프사이클(시작·중지·
// 재시작·enable·disable)은 agent.execute 한 키로 묶이고, 삭제는 agent.delete,
// 내보내기는 읽기 성격이라 agent.read 다(서버의 GET /agents/{id}/export 도 같은
// 키로 보호된다). 권한 판정은 기존 원격 게이팅과 합성된다 — 어느 한쪽이라도
// 막으면 비활성이다.

import { Download, Play, Power, PowerOff, RotateCcw, Square, Trash2 } from 'lucide-react';

import {
  useEnableAgent,
  useStartAgent,
} from '@/hooks/useAgent';
import { useAgentActionsTarget, type AgentAction } from '@/hooks/useResourceActions';
import { useTargetGating } from '@/hooks/useTargetGating';
import { useTranslation } from '@/lib/i18n';
import { remoteEditErrorMessage } from '@/lib/remote/editError';
import { useTargetContext } from '@/lib/remote/TargetContext';
import PermissionButton from '@/components/common/PermissionButton';
import { downloadJSON } from '@/lib/utils/download';
import { exportAgent } from '@/services/api/agentService';
import { useUIStore } from '@/stores/uiStore';
import type { AgentInfo } from '@/types/agent';
import { cn } from '@/lib/utils/cn';

interface AgentActionButtonsProps {
  agent: AgentInfo;
  onAction?: () => void;
}

export default function AgentActionButtons({ agent, onAction }: AgentActionButtonsProps) {
  const { t } = useTranslation();
  const target = useTargetContext();
  const actions = useAgentActionsTarget(target);
  const gating = useTargetGating(target);
  const addNotification = useUIStore((s) => s.addNotification);

  // 로컬 전용 편의 동작(enable → start)을 위해 로컬 훅을 직접 보유한다(원격은 미사용).
  const enableAgentLocal = useEnableAgent();
  const startAgentLocal = useStartAgent();

  const remote = actions.isRemote;
  const nodeControllable = gating.canControl();

  const isRunning = agent.connected === true;
  // enabled 필드가 응답에 없으면 구버전 서버로 간주하여 활성화 상태로 취급한다 (하위 호환).
  const isEnabled = agent.enabled !== false;

  // 원격 타깃에서 액션 버튼의 disabled/툴팁을 계산한다.
  const remoteState = (action: AgentAction, baseTitle: string): { disabled: boolean; title: string } => {
    if (!remote) return { disabled: false, title: baseTitle };
    if (!actions.supports(action)) {
      return { disabled: true, title: t('remote.edit.unsupportedOnRemote') };
    }
    if (!nodeControllable) {
      return { disabled: true, title: t('remote.edit.actionGateHint') };
    }
    return { disabled: false, title: baseTitle };
  };

  const run = async (action: AgentAction) => {
    try {
      await actions.perform(action, agent.id);
      onAction?.();
    } catch (err) {
      if (remote) {
        addNotification({ type: 'error', message: remoteEditErrorMessage(err, t) });
      }
      // 로컬 실패는 기존 동작과 동일하게 React Query 에러로만 노출(토스트 없음).
    }
  };

  /** 에이전트 시작 (disabled 상태도 수동 시작 허용: SPEC-AGENT-005 R4.1) */
  const handleStart = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!remote && !isEnabled) {
      // disabled 에이전트의 일시 시작 안내 (R6.6)
      if (!window.confirm(t('agents.action.confirmTempStart'))) {
        return;
      }
    }
    await run('start');
  };

  /**
   * 에이전트 영속 활성화 (로컬 전용 — 원격 노드는 enable 명령을 지원하지 않음).
   *
   * 백엔드 동작(SPEC-AGENT-005 R3.8): enable API 는 정지된 에이전트를 자동으로
   * Start 하지 않는다. UI 편의상 enable 성공 후 정지 상태면 Start 도 호출한다.
   */
  const handleEnable = async (e: React.MouseEvent) => {
    e.stopPropagation();
    await enableAgentLocal.mutateAsync(agent.id);
    if (!isRunning) {
      try {
        await startAgentLocal.mutateAsync(agent.id);
      } catch {
        // Start 실패는 enable 성공을 되돌리지 않는다.
      }
    }
    onAction?.();
  };

  /** 에이전트 영속 비활성화 (로컬 전용, 런타임 상태 영향 없음: R3.7) */
  const handleDisable = async (e: React.MouseEvent) => {
    e.stopPropagation();
    await run('disable');
  };

  /** 에이전트 중지 */
  const handleStop = async (e: React.MouseEvent) => {
    e.stopPropagation();
    await run('stop');
  };

  /** 에이전트 재시작 */
  const handleRestart = async (e: React.MouseEvent) => {
    e.stopPropagation();
    await run('restart');
  };

  /** 에이전트 내보내기 (로컬 전용) */
  const handleExport = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      const data = await exportAgent(agent.id);
      downloadJSON(data, `${agent.name}.json`);
    } catch {
      // 내보내기 실패 시 조용히 무시 (콘솔에 에러 출력은 fetch 레벨에서 처리)
    }
  };

  /** 에이전트 삭제 (확인 다이얼로그 표시) */
  const handleDelete = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!window.confirm(t('agents.action.confirmDelete').replace('{name}', agent.name))) return;
    await run('delete');
  };

  const btnBase =
    'rounded-md p-1.5 text-gray-400 transition-colors hover:text-(--color-text-secondary) disabled:opacity-40 disabled:cursor-not-allowed';

  const start = remoteState('start', t('agents.start'));
  const stop = remoteState('stop', t('agents.stop'));
  const restart = remoteState('restart', t('agents.restart'));
  const enable = remoteState('enable', t('agents.action.enableTooltip'));
  const disable = remoteState('disable', t('agents.action.disableTooltip'));
  const del = remoteState('delete', t('agents.delete'));

  return (
    <div className="flex items-center justify-end gap-1">
      {/* 시작 */}
      <PermissionButton
        type="button"
        permission="agent.execute"
        title={start.title}
        disabled={isRunning || actions.pending.start || start.disabled}
        onClick={handleStart}
        className={cn(btnBase, 'hover:bg-green-50 dark:hover:bg-green-900/20')}
      >
        <Play className="h-4 w-4" />
      </PermissionButton>

      {/* 중지 */}
      <PermissionButton
        type="button"
        permission="agent.execute"
        title={stop.title}
        disabled={!isRunning || actions.pending.stop || stop.disabled}
        onClick={handleStop}
        className={cn(btnBase, 'hover:bg-yellow-50 dark:hover:bg-yellow-900/20')}
      >
        <Square className="h-4 w-4" />
      </PermissionButton>

      {/* 재시작 */}
      <PermissionButton
        type="button"
        permission="agent.execute"
        title={restart.title}
        disabled={actions.pending.restart || restart.disabled}
        onClick={handleRestart}
        className={cn(btnBase, 'hover:bg-blue-50 dark:hover:bg-blue-900/20')}
      >
        <RotateCcw className="h-4 w-4" />
      </PermissionButton>

      {/* Enable/Disable 토글 (원격 미지원 — 비활성). 로컬은 SPEC-AGENT-005 동작. */}
      {isEnabled ? (
        <PermissionButton
          type="button"
          permission="agent.execute"
          title={disable.title}
          disabled={enableAgentLocal.isPending || disable.disabled}
          onClick={handleDisable}
          className={cn(btnBase, 'hover:bg-gray-100 dark:hover:bg-gray-800')}
        >
          <PowerOff className="h-4 w-4" />
        </PermissionButton>
      ) : (
        <PermissionButton
          type="button"
          permission="agent.execute"
          title={enable.title}
          disabled={enableAgentLocal.isPending || enable.disabled}
          onClick={handleEnable}
          className={cn(btnBase, 'hover:bg-emerald-50 dark:hover:bg-emerald-900/20')}
        >
          <Power className="h-4 w-4" />
        </PermissionButton>
      )}

      {/* 내보내기 (로컬 전용) */}
      {!remote && (
        <PermissionButton
          type="button"
          permission="agent.read"
          title={t('agents.action.export')}
          onClick={handleExport}
          className={cn(btnBase, 'hover:bg-(--color-bg-elevated)')}
        >
          <Download className="h-4 w-4" />
        </PermissionButton>
      )}

      {/* 삭제 */}
      <PermissionButton
        type="button"
        permission="agent.delete"
        title={del.title}
        disabled={actions.pending.delete || del.disabled}
        onClick={handleDelete}
        className={cn(btnBase, 'hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400')}
      >
        <Trash2 className="h-4 w-4" />
      </PermissionButton>
    </div>
  );
}
