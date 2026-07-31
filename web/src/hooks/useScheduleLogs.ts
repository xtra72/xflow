// 스케줄 실행 로그 조회 React Query 훅(SPEC-SCHEDULE-VIEW-001 M6).
//
// scheduleLogService.getScheduleLogs 를 래핑한다. queryKey 에 필터/페이지네이션
// 파라미터를 포함해 필터 변경 시 자동 재조회한다. 기본 limit 100, offset 지원.

import { useQuery } from '@tanstack/react-query';

import * as scheduleLogService from '@/services/api/scheduleLogService';
import type { ScheduleLogQuery } from '@/services/api/scheduleLogService';

/** 기본 조회 개수(페이지네이션 미지정 시). */
export const SCHEDULE_LOGS_DEFAULT_LIMIT = 100;

/**
 * 스케줄 실행 로그를 조회한다.
 *
 * @param params 필터(scheduleId/ruleName/agentId) + 페이지네이션(limit/offset).
 *               limit 미지정 시 SCHEDULE_LOGS_DEFAULT_LIMIT 를 사용한다.
 */
export function useScheduleLogs(params: ScheduleLogQuery = {}) {
  const query: ScheduleLogQuery = { limit: SCHEDULE_LOGS_DEFAULT_LIMIT, ...params };
  return useQuery({
    queryKey: ['schedule-logs', query],
    queryFn: () => scheduleLogService.getScheduleLogs(query),
  });
}
