// useDeviceCommandTarget — control 패널의 디바이스 명령 쓰기를 target 으로 라우팅한다
// (SPEC-REMOTE-001 M10, 그룹 L, REQ-L08/D04, REQ-J03/J12).
//
//   - 로컬: 기존 useExecuteCommand(POST /devices/{id}/execute) 그대로 → 회귀 없음.
//   - 원격: 그룹 D 명령(useSendCommand: {domain:'device', action:'execute',
//     args:{id, command, params}})으로 라우팅한다. 디바이스 쓰기를 프록시(query/
//     스트림) 경유로 수행하지 않는다(READ-ONLY 강제 — REQ-J03/J12). config 의 bare
//     deviceId 는 원격 target 하에서 그 노드의 디바이스로 해석된다(REQ-L03).
//
// 게이팅·실패 의미는 호출 측(control 패널)이 useTargetGating + editError 로 처리한다
// (오프라인=503/타임아웃=504/노드 적용 실패=502/노출 범위 밖=404 — REQ-L11).
//
// 두 뮤테이션을 항상 호출하고 반환 메서드에서만 분기한다(Rules of Hooks).

import { useMemo } from 'react';

import { useSendCommand } from '@/hooks/useRemote';
import { useExecuteCommand } from '@/hooks/useDevice';
import { isRemoteTarget, type ResourceTarget } from '@/lib/remote/target';

/** useDeviceCommandTarget 반환 형태. */
export interface DeviceCommandTargetResult {
  /** 원격 타깃 여부. */
  isRemote: boolean;
  /** 명령 적용 중 여부(per-target pending). */
  isPending: boolean;
  /** 마지막 에러(로컬/원격 공통). null = 정상. */
  error: unknown;
  /**
   * 디바이스 명령을 실행한다(로컬: /execute, 원격: 그룹 D execute). 실패는 reject 로
   * 전파한다(호출 측이 editError 매핑으로 표시). deviceId 가 비면 무시한다.
   */
  execute: (
    deviceId: string | undefined,
    command: string,
    params: Record<string, unknown>,
  ) => void;
}

/**
 * 디바이스 명령 타깃 훅.
 *
 * @param target - 로컬 또는 원격 노드 타깃.
 */
export function useDeviceCommandTarget(target: ResourceTarget): DeviceCommandTargetResult {
  const remote = isRemoteTarget(target);
  const instanceID = remote ? target.instanceId : '';

  const local = useExecuteCommand();
  const sendCommand = useSendCommand();

  return useMemo<DeviceCommandTargetResult>(() => {
    const execute = (
      deviceId: string | undefined,
      command: string,
      params: Record<string, unknown>,
    ): void => {
      if (!deviceId) return;
      if (remote) {
        // 그룹 D 명령: {domain:'device', action:'execute', args:{id, command, params}}.
        sendCommand.mutate({
          instanceID,
          req: {
            domain: 'device',
            action: 'execute',
            args: { id: deviceId, command, params },
          },
        });
        return;
      }
      local.mutate({ id: deviceId, req: { command, params } });
    };

    return {
      isRemote: remote,
      isPending: remote ? sendCommand.isPending : local.isPending,
      error: remote ? sendCommand.error : local.error,
      execute,
    };
  }, [remote, instanceID, local, sendCommand]);
}
