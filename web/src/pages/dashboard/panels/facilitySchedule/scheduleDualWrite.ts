// SPEC-SCHEDULE-VIEW-001: 스케줄 dual-write 코어(순수 async 추출).
//
// useScheduleDualWrite 훅 내부의 LIVE→404→PERSIST→통지 로직을 훅에서 분리해 재사용
// 가능한 순수 async 로 추출한다. 훅은 이 코어를 감싸 saving/baselineRef 만 관리한다
// (behavior-preserving). 스케줄 뷰의 "규칙 추가"는 저장 시점에 선택된 노드로 dual-write
// 해야 하는데, 대상 노드가 저장 시점에 결정되므로 (flowId,nodeId) 별 훅 대신 이 one-shot
// async 로 각인/지속화한다(Rules of Hooks 준수).

import { configureNode } from '@/services/api/nodeService';
import { getFlow, updateFlow } from '@/services/api/flowService';
import type { Notification } from '@/stores/uiStore';
import { APIError } from '@/types/api';
import {
  buildFullTriggerConfig,
  detectConflict,
  findNodeConfigInDefinition,
  patchNodeConfigInDefinition,
  type SerializedSchedule,
} from '../triggerPanelUtils';

/** dual-write 통지 콜백 인자(uiStore.addNotification 과 동형). */
export type ScheduleNotify = (n: Omit<Notification, 'id' | 'timestamp'>) => void;

/** {@link persistScheduleDualWrite} 인자. */
export interface PersistScheduleDualWriteArgs {
  flowId: string;
  nodeId: string;
  /** dual-write baseline(노드 config 스냅샷). FULL config 조립/충돌 감지 기준. */
  baseline: Record<string, unknown>;
  /** 각인 대상 노드에 지속화할 FULL schedules 배열. */
  nextSchedules: SerializedSchedule[];
  /** 통지 발행(성공/경고/오류). */
  notify: ScheduleNotify;
}

/**
 * 스케줄 dual-write 코어(순수 async). LIVE(configureNode) + PERSIST(getFlow→patch→
 * updateFlow), 404→persist-only+통지, last-write-wins(detectConflict) 를 그대로 담는다.
 * 전체 성공 시 성공/경고 통지를 발행하고 `true` 를, 조기 반환 오류 경로에서는 오류 통지를
 * 발행하고 `false` 를 반환한다(소비자 상태 반영은 성공 시에만).
 *
 * baseline 갱신(성공 후 baselineRef=fullConfig)은 이 코어가 아니라 호출자(훅)가
 * 담당한다 — 코어는 상태를 소유하지 않는다.
 */
export async function persistScheduleDualWrite({
  flowId,
  nodeId,
  baseline,
  nextSchedules,
  notify,
}: PersistScheduleDualWriteArgs): Promise<boolean> {
  if (!flowId || !nodeId) return false;
  const fullConfig = buildFullTriggerConfig(baseline, nextSchedules);

  // 1) LIVE
  let persistOnly = false;
  try {
    await configureNode(flowId, nodeId, fullConfig);
  } catch (err) {
    if (err instanceof APIError && err.status === 404) {
      persistOnly = true;
    } else {
      notify({ type: 'error', message: '저장 실패 — 라이브 반영 중 오류가 발생했습니다.' });
      return false;
    }
  }

  // 2) PERSIST (patch-then-PUT)
  let conflict = false;
  try {
    const flow = await getFlow(flowId);
    const def = (flow.config ?? {}) as Record<string, unknown>;
    conflict = detectConflict(findNodeConfigInDefinition(def, nodeId), baseline);
    const patched = patchNodeConfigInDefinition(def, nodeId, fullConfig);
    await updateFlow(flowId, { definition: patched });
  } catch {
    notify({ type: 'error', message: '저장 실패 — 플로우 정의 지속화 중 오류가 발생했습니다.' });
    return false;
  }

  notify({
    type: persistOnly ? 'warning' : 'success',
    message: persistOnly ? '노드 미실행 — 저장만 적용, 재배포 시 반영' : '저장 완료',
  });
  if (conflict) {
    notify({ type: 'warning', message: '다른 편집이 감지되어 덮어썼습니다 (last-write-wins)' });
  }
  return true;
}
