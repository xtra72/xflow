// SPEC-SCHEDULE-VIEW-001 M6: 스케줄 실행 로그 병합 로직.
//
// 평탄한 로그 레코드(fire/result)를 correlation_id 로 묶어 하나의 논리 행으로
// 병합한다(AC-16/AC-3). ScheduleLogTab 이 렌더 전에 사용하며, 순수 함수로
// 분리해 단위 테스트 및 컴포넌트 fast-refresh 경계를 지킨다.

import type {
  ScheduleLogRecord,
  ScheduleLogResult,
  ScheduleLogTargetResult,
} from '@/types/schedule';

/** correlation_id 로 병합된 논리 로그 행(fire + result 결합). */
export interface MergedScheduleLog {
  correlationId: string;
  // 발화(fire) 메타 — fire 우선, 없으면 result 에서 채운다.
  scheduleId: string;
  ruleName: string;
  declaredAgentId: string;
  triggerTime: number;
  // 실행 결과(result) 측 — result 레코드가 있을 때만 유효.
  hasResult: boolean;
  actorAgentId: string;
  target: string;
  action: string;
  result: ScheduleLogResult;
  targets: ScheduleLogTargetResult[];
  reason: string;
  // 그룹 최신 시각(정렬 기준).
  latestTimestamp: number;
}

/**
 * 평탄한 로그 레코드를 correlation_id 로 병합한다(AC-16/AC-3).
 *
 * 각 correlation_id 그룹은 하나의 행이 되며, fire 레코드에서 발화 메타를,
 * result 레코드에서 실행 결과를 취한다. result 가 없으면 fire-only 행으로
 * 결과 필드가 비어 있다(AC-15). 정렬은 그룹 최신 시각 내림차순(newest-first).
 */
export function mergeScheduleLogs(records: ScheduleLogRecord[]): MergedScheduleLog[] {
  const groups = new Map<string, ScheduleLogRecord[]>();
  for (const rec of records) {
    const arr = groups.get(rec.correlation_id);
    if (arr) arr.push(rec);
    else groups.set(rec.correlation_id, [rec]);
  }

  const merged: MergedScheduleLog[] = [];
  for (const [correlationId, recs] of groups) {
    const fire = recs.find((r) => r.record_kind === 'fire');
    const result = recs.find((r) => r.record_kind === 'result');
    // 발화 메타는 fire 우선, 없으면 result, 그래도 없으면 첫 레코드.
    const meta = fire ?? result ?? recs[0]!;
    const latestTimestamp = recs.reduce(
      (max, r) => Math.max(max, r.timestamp, r.trigger_time),
      0,
    );

    merged.push({
      correlationId,
      scheduleId: meta.schedule_id,
      ruleName: meta.rule_name,
      declaredAgentId: meta.declared_agent_id,
      triggerTime: meta.trigger_time,
      hasResult: result != null,
      actorAgentId: result?.actor_agent_id ?? '',
      target: result?.target ?? '',
      action: result?.action ?? '',
      result: result?.result ?? '',
      targets: result?.targets ?? [],
      reason: result?.reason ?? fire?.reason ?? '',
      latestTimestamp,
    });
  }

  merged.sort(
    (a, b) => b.latestTimestamp - a.latestTimestamp || b.triggerTime - a.triggerTime,
  );
  return merged;
}
