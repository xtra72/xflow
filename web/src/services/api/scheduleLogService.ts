// 스케줄 실행 로그 조회/초기화 서비스(SPEC-SCHEDULE-VIEW-001 M6).
//
// GET /schedules/logs?schedule_id=&rule_name=&agent_id=&limit=&offset= 를 호출한다.
// camelCase 인자를 snake_case 쿼리 파라미터로 매핑하고, 성공 envelope 를 client
// 인터셉터가 해제한 payload({ items, total })를 정규화해 반환한다. total 은 필터에
// 매칭되는 전체 레코드 수(limit/offset 무관)이다. agent_id 는 백엔드에서
// declared_agent_id 또는 actor_agent_id 어느 쪽이든 매칭한다.
//
// DELETE /schedules/logs 는 저장된 모든 실행 로그를 삭제한다(full-auth).

import type { ScheduleLogRecord } from '@/types/schedule';

import { del, get } from './client';

/** getScheduleLogs 쿼리 파라미터(모두 선택). */
export interface ScheduleLogQuery {
  scheduleId?: string;
  ruleName?: string;
  agentId?: string;
  limit?: number;
  offset?: number;
}

/** 페이지네이션 응답: 현재 페이지 레코드 + 필터 매칭 전체 수. */
export interface ScheduleLogPage {
  items: ScheduleLogRecord[];
  total: number;
}

/** 응답 data 의 원시 형태(누락 필드 대비). */
interface RawScheduleLogPage {
  items?: ScheduleLogRecord[] | null;
  total?: number | null;
}

/**
 * 스케줄 실행 로그를 최신순으로 조회한다.
 *
 * 빈/미지정 필터는 쿼리에서 생략한다. 응답 data 가 null 이거나 필드가 누락되면
 * items → 빈 배열, total → 0 으로 정규화한다.
 */
export async function getScheduleLogs(
  params: ScheduleLogQuery = {},
): Promise<ScheduleLogPage> {
  const query: Record<string, string | number> = {};
  if (params.scheduleId) query.schedule_id = params.scheduleId;
  if (params.ruleName) query.rule_name = params.ruleName;
  if (params.agentId) query.agent_id = params.agentId;
  if (params.limit != null) query.limit = params.limit;
  if (params.offset != null) query.offset = params.offset;

  const data = await get<RawScheduleLogPage | null>('/schedules/logs', { params: query });
  return {
    items: data?.items ?? [],
    total: data?.total ?? 0,
  };
}

/**
 * 저장된 모든 실행 로그를 삭제한다(DELETE /schedules/logs).
 *
 * 파괴적 작업이므로 호출부에서 사용자 확인 절차를 거쳐야 한다. nil repo 인
 * 백엔드는 성공 no-op 으로 응답한다.
 */
export async function clearScheduleLogs(): Promise<void> {
  await del('/schedules/logs');
}
