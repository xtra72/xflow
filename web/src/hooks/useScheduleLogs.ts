// 스케줄 실행 로그 조회/초기화 React Query 훅(SPEC-SCHEDULE-VIEW-001 M6).
//
// useScheduleLogs: scheduleLogService.getScheduleLogs 를 래핑한다. queryKey 에
// 필터/페이지네이션 파라미터를 포함해 필터·페이지 변경 시 자동 재조회한다.
// data 는 { items, total } 형태이며 기본 limit 25, offset 지원.
//
// useClearScheduleLogs: clearScheduleLogs 뮤테이션. 성공 시 로그 쿼리를 invalidate 해
// 테이블이 빈 상태로 갱신된다.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import * as scheduleLogService from '@/services/api/scheduleLogService';
import type { ScheduleLogQuery } from '@/services/api/scheduleLogService';

/** 기본 조회 개수(페이지네이션 미지정 시). */
export const SCHEDULE_LOGS_DEFAULT_LIMIT = 25;

/** 로그 쿼리 캐시 키 접두어(뮤테이션 invalidate 대상). */
export const SCHEDULE_LOGS_QUERY_KEY = 'schedule-logs';

/**
 * 스케줄 실행 로그를 조회한다.
 *
 * @param params 필터(scheduleId/ruleName/agentId) + 페이지네이션(limit/offset).
 *               limit 미지정 시 SCHEDULE_LOGS_DEFAULT_LIMIT 를 사용한다.
 * @returns react-query 결과. `data` 는 `{ items, total }`.
 */
export function useScheduleLogs(params: ScheduleLogQuery = {}) {
  const query: ScheduleLogQuery = { limit: SCHEDULE_LOGS_DEFAULT_LIMIT, ...params };
  return useQuery({
    queryKey: [SCHEDULE_LOGS_QUERY_KEY, query],
    queryFn: () => scheduleLogService.getScheduleLogs(query),
  });
}

/**
 * 저장된 모든 실행 로그를 삭제하는 뮤테이션.
 *
 * 성공 시 모든 로그 쿼리를 invalidate 해 현재 필터/페이지의 테이블을 재조회한다.
 */
export function useClearScheduleLogs() {
  const queryClient = useQueryClient();
  return useMutation<void, Error, void>({
    mutationFn: () => scheduleLogService.clearScheduleLogs(),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: [SCHEDULE_LOGS_QUERY_KEY] });
    },
  });
}
