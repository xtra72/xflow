// 에이전트 라이프사이클 제어 버튼 그룹.
// 시작, 중지, 재시작, Enable/Disable, 내보내기, 삭제 액션을 인라인 아이콘 버튼으로 제공한다.

import { Download, Play, Power, PowerOff, RotateCcw, Square, Trash2 } from 'lucide-react';

import {
  useDeleteAgent,
  useDisableAgent,
  useEnableAgent,
  useRestartAgent,
  useStartAgent,
  useStopAgent,
} from '@/hooks/useAgent';
import { downloadJSON } from '@/lib/utils/download';
import { exportAgent } from '@/services/api/agentService';
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
  const enableAgent = useEnableAgent();
  const disableAgent = useDisableAgent();
  const deleteAgent = useDeleteAgent();

  const isRunning = agent.connected === true;
  // enabled 필드가 응답에 없으면 구버전 서버로 간주하여 활성화 상태로 취급한다 (하위 호환).
  const isEnabled = agent.enabled !== false;

  /** 에이전트 시작 (disabled 상태도 수동 시작 허용: SPEC-AGENT-005 R4.1) */
  const handleStart = async (e: React.MouseEvent) => {
    e.stopPropagation();
    if (!isEnabled) {
      // disabled 에이전트의 일시 시작 안내 (R6.6)
      if (!window.confirm('비활성화된 에이전트를 일시 시작합니다. 다음 데몬 재시작 시에는 자동으로 시작되지 않습니다.\n계속하시겠습니까?')) {
        return;
      }
    }
    await startAgent.mutateAsync(agent.id);
    onAction?.();
  };

  /**
   * 에이전트 영속 활성화.
   *
   * 백엔드 동작(SPEC-AGENT-005 R3.8): enable API 는 정지된 에이전트를
   * 자동으로 Start 하지 않는다. 그러나 UI 관점에서는 사용자가 "지금 당장
   * 활성화하고 사용하겠다" 는 의도로 버튼을 누르는 것이 일반적이므로,
   * enable 호출이 성공하면 에이전트가 정지 상태일 때 한해 연속으로
   * Start 를 호출하여 한 번의 클릭으로 "활성화 + 시작" 을 완료한다.
   * 이 편의 로직은 UI 레이어에만 존재하며 백엔드 API 시맨틱스는
   * 그대로 유지된다 (스크립트/CLI/직접 API 호출 시에는 enable 만 실행됨).
   */
  const handleEnable = async (e: React.MouseEvent) => {
    e.stopPropagation();
    await enableAgent.mutateAsync(agent.id);
    // enable 성공 후 에이전트가 정지 상태면 바로 Start 도 호출한다.
    // agent.connected 는 이 시점의 캐시된 값을 반영하며, enable 은
    // 런타임 상태를 변경하지 않으므로 그대로 신뢰해도 무방하다.
    if (!isRunning) {
      try {
        await startAgent.mutateAsync(agent.id);
      } catch {
        // Start 실패는 enable 성공을 되돌리지 않는다. 사용자는 목록의
        // 상태 표시로 실패를 인지할 수 있으며, 수동 Start 버튼으로
        // 재시도할 수 있다.
      }
    }
    onAction?.();
  };

  /** 에이전트 영속 비활성화 (런타임 상태 영향 없음: R3.7) */
  const handleDisable = async (e: React.MouseEvent) => {
    e.stopPropagation();
    await disableAgent.mutateAsync(agent.id);
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

  /** 에이전트 내보내기 */
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

      {/* Enable/Disable 토글 (SPEC-AGENT-005) */}
      {isEnabled ? (
        <button
          type="button"
          title="비활성화 (자동 시작 제외)"
          disabled={disableAgent.isPending}
          onClick={handleDisable}
          className={cn(btnBase, 'hover:bg-gray-100 dark:hover:bg-gray-800')}
        >
          <PowerOff className="h-4 w-4" />
        </button>
      ) : (
        <button
          type="button"
          title="활성화 (자동 시작 복원)"
          disabled={enableAgent.isPending}
          onClick={handleEnable}
          className={cn(btnBase, 'hover:bg-emerald-50 dark:hover:bg-emerald-900/20')}
        >
          <Power className="h-4 w-4" />
        </button>
      )}

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
        disabled={deleteAgent.isPending}
        onClick={handleDelete}
        className={cn(btnBase, 'hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400')}
      >
        <Trash2 className="h-4 w-4" />
      </button>
    </div>
  );
}
