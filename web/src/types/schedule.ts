// Schedule log domain types matching the Go DTO returned by
// GET /schedules/logs (SPEC-SCHEDULE-VIEW-001 M3/M6).
//
// 각 레코드는 fire(발화) 또는 result(실행 결과) 이벤트 하나를 나타내며,
// correlation_id(= schedule_id + ":" + trigger_time)로 fire/result 를 짝짓는다.
// 응답은 최신순(newest-first)이며 성공 envelope({ data: [...] })로 감싸진다.

/** 로그 레코드 종류: 발화(fire) 또는 실행 결과(result). */
export type ScheduleLogKind = 'fire' | 'result';

/** 집계 실행 결과. result 레코드에서만 채워지며 fire 는 빈 문자열. */
export type ScheduleLogResult = 'ok' | 'error' | '';

/**
 * 대상(멤버)별 실행 결과 임베드.
 * set_multiple 등 다중 대상 동작에서 대상 단위 성공/실패를 담는다(없으면 빈 배열).
 */
export interface ScheduleLogTargetResult {
  target: string;
  result: string;
  reason: string;
}

/**
 * 스케줄 실행 로그 레코드(fire 또는 result 단건).
 * Go DTO(json snake_case)를 그대로 미러링한다.
 */
export interface ScheduleLogRecord {
  id: number;
  /** fire/result 조인 키(= schedule_id + ":" + trigger_time). */
  correlation_id: string;
  record_kind: ScheduleLogKind;
  schedule_id: string;
  rule_name: string;
  /** 규칙에 선언된 에이전트 ID. */
  declared_agent_id: string;
  /** 실제 실행 에이전트 ID. fire 에서는 빈 문자열. */
  actor_agent_id: string;
  /** 발화 시각(epoch ms). */
  trigger_time: number;
  /** result 전용: 실행 대상(e.g. "group_id=station:0150"). */
  target: string;
  /** result 전용: 동작(set_power/set_fan_speed/set_multiple). */
  action: string;
  /** result 전용: 집계 결과. fire 에서는 빈 문자열. */
  result: ScheduleLogResult;
  /** 대상별 결과 임베드(없으면 빈 배열). */
  targets: ScheduleLogTargetResult[];
  reason: string;
  /** 기록 시각(epoch ms). */
  timestamp: number;
}
