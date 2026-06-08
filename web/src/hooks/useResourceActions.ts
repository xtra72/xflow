// useFlowActionsTarget / useAgentActionsTarget — 타깃 인지 라이프사이클/CRUD 액션
// 추상화 (SPEC-REMOTE-001 M8, 그룹 J, REQ-J03/J05/J12 확장).
//
// 목록/상세 페이지의 액션 어포던스(시작/중지/재시작/배포/삭제 등)를 타깃에 따라
// 동일 인터페이스로 라우팅한다.
//
//   - 로컬: 기존 로컬 뮤테이션 훅(useStartFlow 등)에 그대로 위임한다 → 회귀 없이
//     동일 동작·동일 per-action pending.
//   - 원격: 라이프사이클은 그룹 D 명령(useSendCommand: {domain, action, args:{id}})
//     으로, 자원 삭제는 M7 편집 훅(useDeleteRemoteFlow/useDeleteRemoteAgent)으로
//     라우팅한다. 노드 명령 어플라이어가 지원하지 않는 액션(플로우 restart/undeploy,
//     에이전트 enable/disable)은 supports(action)=false 로 표시하고, 호출 측이
//     해당 버튼만 비활성화한다(가짜 동작 금지 — 명시적 미지원).
//
// Rules of Hooks: 로컬/원격 뮤테이션 훅을 항상 동일 순서로 모두 호출하고, 반환
// 메서드에서만 타깃에 따라 분기한다(useResourceTargets 와 동일 전략).
//
// 노드 명령 어플라이어 지원 범위(cmd/xflowd/remote_commands.go SoT):
//   flow  : create, update, get, list, delete, deploy, start, stop  (pause/restart/undeploy 미지원)
//   agent : create, update, get, list, delete, start, stop, restart (enable/disable 미지원)
//   device: update(metadata), delete_metadata                       (런타임 명령 없음)

import { useMemo } from 'react';

import {
  useDeleteAgent,
  useDisableAgent,
  useEnableAgent,
  useRestartAgent,
  useStartAgent,
  useStopAgent,
} from '@/hooks/useAgent';
import {
  useDeleteFlow,
  useDeployFlow,
  useRestartFlow,
  useStartFlow,
  useStopFlow,
  useUndeployFlow,
} from '@/hooks/useFlow';
import {
  useDeleteRemoteAgent,
  useDeleteRemoteFlow,
  useSendCommand,
} from '@/hooks/useRemote';
import { isRemoteTarget, type ResourceTarget } from '@/lib/remote/target';

/** 플로우 라이프사이클/CRUD 액션 식별자. */
export type FlowAction = 'start' | 'stop' | 'restart' | 'deploy' | 'undeploy' | 'delete';

/** 에이전트 라이프사이클/CRUD 액션 식별자. */
export type AgentAction =
  | 'start'
  | 'stop'
  | 'restart'
  | 'enable'
  | 'disable'
  | 'delete';

/** 타깃 인지 액션 핸들의 공통 형태. */
export interface ActionHandle<A extends string> {
  /** 원격 타깃 여부. */
  isRemote: boolean;
  /** 액션을 적용한다(로컬: 로컬 훅, 원격: 명령/M7). 실패는 reject 로 전파한다. */
  perform: (action: A, id: string) => Promise<void>;
  /** 현재 타깃에서 액션이 지원되는지(원격 미지원 액션은 false). */
  supports: (action: A) => boolean;
  /** 액션별 진행 상태(per-action pending). */
  pending: Partial<Record<A, boolean>>;
}

// 원격에서 지원되지 않는 액션 집합(노드 명령 어플라이어 기준).
const FLOW_REMOTE_UNSUPPORTED: ReadonlySet<FlowAction> = new Set<FlowAction>([
  'restart',
  'undeploy',
]);
const AGENT_REMOTE_UNSUPPORTED: ReadonlySet<AgentAction> = new Set<AgentAction>([
  'enable',
  'disable',
]);

// 원격 라이프사이클 명령으로 매핑되는 액션 → 명령 action 문자열.
const FLOW_COMMAND_ACTION: Partial<Record<FlowAction, string>> = {
  start: 'start',
  stop: 'stop',
  deploy: 'deploy',
};
const AGENT_COMMAND_ACTION: Partial<Record<AgentAction, string>> = {
  start: 'start',
  stop: 'stop',
  restart: 'restart',
};

/**
 * 플로우 액션 타깃 훅.
 *
 * @param target - 로컬 또는 원격 노드 타깃.
 */
export function useFlowActionsTarget(target: ResourceTarget): ActionHandle<FlowAction> {
  const remote = isRemoteTarget(target);
  const instanceID = remote ? target.instanceId : '';

  // 로컬 뮤테이션(항상 호출 — Rules of Hooks).
  const startFlow = useStartFlow();
  const stopFlow = useStopFlow();
  const restartFlow = useRestartFlow();
  const deployFlow = useDeployFlow();
  const undeployFlow = useUndeployFlow();
  const deleteFlow = useDeleteFlow();

  // 원격 뮤테이션(항상 호출).
  const sendCommand = useSendCommand();
  const deleteRemoteFlow = useDeleteRemoteFlow();

  return useMemo<ActionHandle<FlowAction>>(() => {
    const supports = (action: FlowAction): boolean =>
      remote ? !FLOW_REMOTE_UNSUPPORTED.has(action) : true;

    const performLocal = async (action: FlowAction, id: string): Promise<void> => {
      switch (action) {
        case 'start':
          await startFlow.mutateAsync(id);
          return;
        case 'stop':
          await stopFlow.mutateAsync(id);
          return;
        case 'restart':
          await restartFlow.mutateAsync(id);
          return;
        case 'deploy':
          await deployFlow.mutateAsync(id);
          return;
        case 'undeploy':
          await undeployFlow.mutateAsync(id);
          return;
        case 'delete':
          await deleteFlow.mutateAsync(id);
          return;
      }
    };

    const performRemote = async (action: FlowAction, id: string): Promise<void> => {
      if (!supports(action)) {
        throw new Error(`flow action not supported on remote: ${action}`);
      }
      if (action === 'delete') {
        await deleteRemoteFlow.mutateAsync({ instanceID, flowID: id });
        return;
      }
      const cmdAction = FLOW_COMMAND_ACTION[action];
      if (!cmdAction) {
        throw new Error(`flow action not mapped to remote command: ${action}`);
      }
      await sendCommand.mutateAsync({
        instanceID,
        req: { domain: 'flow', action: cmdAction, args: { id } },
      });
    };

    const pending: Partial<Record<FlowAction, boolean>> = remote
      ? {
          start: sendCommand.isPending,
          stop: sendCommand.isPending,
          deploy: sendCommand.isPending,
          delete: deleteRemoteFlow.isPending,
        }
      : {
          start: startFlow.isPending,
          stop: stopFlow.isPending,
          restart: restartFlow.isPending,
          deploy: deployFlow.isPending,
          undeploy: undeployFlow.isPending,
          delete: deleteFlow.isPending,
        };

    return {
      isRemote: remote,
      supports,
      perform: (action, id) => (remote ? performRemote(action, id) : performLocal(action, id)),
      pending,
    };
  }, [
    remote,
    instanceID,
    startFlow,
    stopFlow,
    restartFlow,
    deployFlow,
    undeployFlow,
    deleteFlow,
    sendCommand,
    deleteRemoteFlow,
  ]);
}

/**
 * 에이전트 액션 타깃 훅.
 *
 * @param target - 로컬 또는 원격 노드 타깃.
 */
export function useAgentActionsTarget(target: ResourceTarget): ActionHandle<AgentAction> {
  const remote = isRemoteTarget(target);
  const instanceID = remote ? target.instanceId : '';

  // 로컬 뮤테이션(항상 호출).
  const startAgent = useStartAgent();
  const stopAgent = useStopAgent();
  const restartAgent = useRestartAgent();
  const enableAgent = useEnableAgent();
  const disableAgent = useDisableAgent();
  const deleteAgent = useDeleteAgent();

  // 원격 뮤테이션(항상 호출).
  const sendCommand = useSendCommand();
  const deleteRemoteAgent = useDeleteRemoteAgent();

  return useMemo<ActionHandle<AgentAction>>(() => {
    const supports = (action: AgentAction): boolean =>
      remote ? !AGENT_REMOTE_UNSUPPORTED.has(action) : true;

    const performLocal = async (action: AgentAction, id: string): Promise<void> => {
      switch (action) {
        case 'start':
          await startAgent.mutateAsync(id);
          return;
        case 'stop':
          await stopAgent.mutateAsync(id);
          return;
        case 'restart':
          await restartAgent.mutateAsync(id);
          return;
        case 'enable':
          await enableAgent.mutateAsync(id);
          return;
        case 'disable':
          await disableAgent.mutateAsync(id);
          return;
        case 'delete':
          await deleteAgent.mutateAsync(id);
          return;
      }
    };

    const performRemote = async (action: AgentAction, id: string): Promise<void> => {
      if (!supports(action)) {
        throw new Error(`agent action not supported on remote: ${action}`);
      }
      if (action === 'delete') {
        await deleteRemoteAgent.mutateAsync({ instanceID, agentID: id });
        return;
      }
      const cmdAction = AGENT_COMMAND_ACTION[action];
      if (!cmdAction) {
        throw new Error(`agent action not mapped to remote command: ${action}`);
      }
      await sendCommand.mutateAsync({
        instanceID,
        req: { domain: 'agent', action: cmdAction, args: { id } },
      });
    };

    const pending: Partial<Record<AgentAction, boolean>> = remote
      ? {
          start: sendCommand.isPending,
          stop: sendCommand.isPending,
          restart: sendCommand.isPending,
          delete: deleteRemoteAgent.isPending,
        }
      : {
          start: startAgent.isPending,
          stop: stopAgent.isPending,
          restart: restartAgent.isPending,
          enable: enableAgent.isPending,
          disable: disableAgent.isPending,
          delete: deleteAgent.isPending,
        };

    return {
      isRemote: remote,
      supports,
      perform: (action, id) => (remote ? performRemote(action, id) : performLocal(action, id)),
      pending,
    };
  }, [
    remote,
    instanceID,
    startAgent,
    stopAgent,
    restartAgent,
    enableAgent,
    disableAgent,
    deleteAgent,
    sendCommand,
    deleteRemoteAgent,
  ]);
}
