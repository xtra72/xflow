// 원격 관리 로그 조회 훅 (@SPEC:SPEC-REMOTE-LOG-001).
//
// 전체 로그 페이지와 노드 화면의 로그 탭이 같은 훅을 쓴다 — 두 화면이 각자 조회를
// 만들면 같은 데이터가 두 규칙으로 갈라진다.
//
// 정렬·필터·쪽 나누기는 모두 서버가 맡는다. 화면에서 하면 받아 온 쪽 안에서만
// 적용되어, 정렬한 사람이 보는 결과가 전체를 대표하지 않는다.

import { useQuery } from '@tanstack/react-query';

import * as remoteService from '@/services/api/remoteService';
import type { RemoteLogQuery } from '@/types/remote';

/** 쪽당 줄 수 기본값과 고를 수 있는 값들. */
export const REMOTE_LOG_PAGE_SIZE = 50;
export const REMOTE_LOG_PAGE_SIZE_OPTIONS = [25, 50, 100, 200, 500];

/** 로그는 사건이 생길 때만 늘어나므로 짧게 폴링할 이유가 없다. */
const REFETCH_MS = 15_000;

/**
 * 원격 관리 로그 조회.
 *
 * @param query - 노드·사건·수행자 필터, 정렬, 쪽 나누기.
 * @param enabled - server 모드가 아니면 false 로 발행을 막는다(404 소음 방지).
 */
export function useRemoteLogs(query: RemoteLogQuery = {}, enabled = true) {
  const {
    instanceId = '',
    action = '',
    actor = '',
    sort = 'ts',
    asc = false,
    limit = REMOTE_LOG_PAGE_SIZE,
    offset = 0,
  } = query;
  return useQuery({
    // 조건이 키에 모두 들어가야 조건을 바꿨을 때 이전 결과가 남지 않는다.
    queryKey: ['remote', 'logs', instanceId, action, actor, sort, asc, limit, offset],
    queryFn: () => remoteService.listRemoteLogs({ instanceId, action, actor, sort, asc, limit, offset }),
    refetchInterval: REFETCH_MS,
    enabled,
  });
}
