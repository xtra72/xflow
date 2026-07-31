// 스케줄 실행 로그 조회 서비스(SPEC-SCHEDULE-VIEW-001 M6).
//
// GET /schedules/logs?schedule_id=&rule_name=&agent_id=&limit=&offset= 를 호출한다.
// camelCase 인자를 snake_case 쿼리 파라미터로 매핑하고, 성공 envelope 를
// client 인터셉터가 해제한 payload(배열)를 그대로 반환한다. agent_id 는 백엔드에서
// declared_agent_id 또는 actor_agent_id 어느 쪽이든 매칭한다.

import type { ScheduleLogRecord } from '@/types/schedule';

import { get } from './client';

/** getScheduleLogs 쿼리 파라미터(모두 선택). */
export interface ScheduleLogQuery {
  scheduleId?: string;
  ruleName?: string;
  agentId?: string;
  limit?: number;
  offset?: number;
}

/**
 * 스케줄 실행 로그를 최신순으로 조회한다.
 *
 * 빈/미지정 필터는 쿼리에서 생략한다. 응답 data 가 null 이면 빈 배열로 정규화한다.
 */
export async function getScheduleLogs(
  params: ScheduleLogQuery = {},
): Promise<ScheduleLogRecord[]> {
  const query: Record<string, string | number> = {};
  if (params.scheduleId) query.schedule_id = params.scheduleId;
  if (params.ruleName) query.rule_name = params.ruleName;
  if (params.agentId) query.agent_id = params.agentId;
  if (params.limit != null) query.limit = params.limit;
  if (params.offset != null) query.offset = params.offset;

  const data = await get<ScheduleLogRecord[] | null>('/schedules/logs', { params: query });
  return data ?? [];
}
