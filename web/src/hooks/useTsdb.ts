// TSDB 시리즈 조회 / 쿼리용 React Query 훅.
// @spec SPEC-WEB-005

import { useMutation, useQuery } from '@tanstack/react-query';

import {
  listTsdbSeries,
  queryTsdbMatrix,
  type TsdbSeriesListParams,
  type TsdbSeriesListResponse,
  type TsdbQueryRequest,
  type TsdbQueryResponse,
} from '@/services/api/tsdb';

/**
 * 페이지네이션된 TSDB 시리즈 목록을 조회한다.
 *
 * 같은 (agentId, page, size) 키로 캐싱되므로 페이지 이동 시 자동 캐시 활용된다.
 */
export function useTsdbSeries(params: TsdbSeriesListParams) {
  return useQuery<TsdbSeriesListResponse, Error>({
    queryKey: ['tsdb', 'series', params.agentId ?? null, params.page, params.size],
    queryFn: () => listTsdbSeries(params),
    // 시리즈 목록은 자주 바뀌지 않으므로 기본 staleTime 을 적용한다.
    staleTime: 5_000,
  });
}

/**
 * 매트릭스 쿼리(병렬 fan-out)를 실행하는 mutation 훅.
 */
export function useTsdbQuery() {
  return useMutation<TsdbQueryResponse, Error, TsdbQueryRequest>({
    mutationFn: (req) => queryTsdbMatrix(req),
  });
}
