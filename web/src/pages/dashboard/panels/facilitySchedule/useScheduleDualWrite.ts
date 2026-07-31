// SPEC-SCHEDULE-VIEW-001 M4: 스케줄 dual-write 훅 (behavior-preserving 추출).
//
// FacilitySchedulePanel 인라인 persist(선행 SPEC-TRIGGER-PANEL-001 dual-write 재사용)를
// 재사용 가능한 훅으로 추출한다. configureNode(LIVE) + getFlow→patch→updateFlow(PERSIST),
// 404→persist-only+통지, last-write-wins(detectConflict) 를 그대로 담는다. 신규 저장소/
// API 도입 없음(RD-9). M5 의 스케줄 뷰 CRUD 가 (flowId,nodeId) 별로 동일 dual-write 에
// 재사용한다(REQ-06-04, AC-12). 패널의 관측 가능한 동작은 불변.

import { useCallback, useRef, useState } from 'react';

import { useUIStore } from '@/stores/uiStore';
import { buildFullTriggerConfig, type SerializedSchedule } from '../triggerPanelUtils';
import { persistScheduleDualWrite } from './scheduleDualWrite';

/** 훅 대상 노드 좌표. */
export interface UseScheduleDualWriteOptions {
  flowId: string;
  nodeId: string;
}

/** {@link useScheduleDualWrite} 반환 형태. */
export interface UseScheduleDualWrite {
  /** dual-write 진행 중 여부(재진입 가드/UI 비활성화용). */
  saving: boolean;
  /**
   * baseline(노드 config 스냅샷) 을 시드한다. 하이드레이션 시점에 호출하며,
   * FULL config 조립/동시편집 감지의 기준이 된다. dual-write 성공 시 내부에서 갱신된다.
   */
  setBaseline: (config: Record<string, unknown>) => void;
  /**
   * dual-write 실행. LIVE(configureNode) + PERSIST(getFlow→patch→updateFlow).
   * 전체 성공 시에만 baseline 을 갱신하고 `onApplied(nextSchedules)` 를 호출한다
   * (조기 반환 오류 경로에서는 호출하지 않음 — 소비자 상태는 성공 시에만 반영).
   */
  persist: (
    nextSchedules: SerializedSchedule[],
    onApplied?: (applied: SerializedSchedule[]) => void,
  ) => Promise<void>;
}

/**
 * 스케줄 dual-write 훅 (M6 선행 재사용, M4 추출).
 *
 * `saving` 상태와 `baselineRef` 를 소유하고 통지를 발행한다. 패널/스케줄 뷰가
 * `{flowId,nodeId}` 만으로 구성해 CRUD dual-write 에 재사용한다.
 */
export function useScheduleDualWrite({
  flowId,
  nodeId,
}: UseScheduleDualWriteOptions): UseScheduleDualWrite {
  const addNotification = useUIStore((s) => s.addNotification);

  const [saving, setSaving] = useState(false);
  const baselineRef = useRef<Record<string, unknown>>({});

  const setBaseline = useCallback((config: Record<string, unknown>) => {
    baselineRef.current = config;
  }, []);

  const persist = useCallback(
    async (
      nextSchedules: SerializedSchedule[],
      onApplied?: (applied: SerializedSchedule[]) => void,
    ) => {
      if (!flowId || !nodeId || saving) return;
      setSaving(true);
      const baseline = baselineRef.current;
      const ok = await persistScheduleDualWrite({
        flowId,
        nodeId,
        baseline,
        nextSchedules,
        notify: addNotification,
      });
      if (ok) {
        // 전체 성공 시에만 baseline 을 이번 fullConfig 로 갱신하고 소비자 상태를 반영한다
        // (조기 반환 오류 경로에서는 미갱신 — 코어가 상태를 소유하지 않으므로 훅이 담당).
        baselineRef.current = buildFullTriggerConfig(baseline, nextSchedules);
        onApplied?.(nextSchedules);
      }
      setSaving(false);
    },
    [flowId, nodeId, saving, addNotification],
  );

  return { saving, setBaseline, persist };
}
