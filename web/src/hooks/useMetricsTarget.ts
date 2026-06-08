// useMetricsTarget — ResourceWidget 의 메트릭 소스를 target 으로 전환한다
// (SPEC-REMOTE-001 M10, 그룹 L, REQ-L05).
//
//   - 로컬: 기존 로컬 메트릭(GET /monitor/metrics)을 동일 주기로 폴링한다 →
//     회귀 없이 동일 동작.
//   - 원격: GET /remote/nodes/{id}/metrics(monitor/metrics query-action)로
//     스냅샷을 온디맨드 취득한다(REQ-L05). 서버 단기 TTL 캐시가 노드 부하를
//     완화한다(REQ-J16). 장기 시계열/상시 폴링은 신설하지 않으며, 패널 표시
//     목적의 주기적 스냅샷만 취득한다(K BASIC 비목표 일관).
//
// 두 분기 모두 항상 동일 순서로 호출하되 비활성 분기는 enabled=false 로 무력화한다
// (Rules of Hooks — useDetailTargets 와 동일 전략).

import { useQuery } from '@tanstack/react-query';

import { isRemoteTarget, type ResourceTarget } from '@/lib/remote/target';
import { getMetrics } from '@/services/api/monitorService';
import * as remoteService from '@/services/api/remoteService';

/** useMetricsTarget 반환 형태(ResourceWidget 가 소비하는 최소 형태). */
export interface MetricsTargetResult {
  metrics: Record<string, unknown> | undefined;
  isLoading: boolean;
  error: unknown;
}

/**
 * 메트릭 스냅샷을 target 에 따라 로컬/원격 소스로 취득한다(REQ-L05).
 *
 * @param target - 로컬 또는 원격 노드 타깃.
 * @param refetchMs - 폴링 주기(ms). 대시보드 refresh 간격과 일치시킨다.
 * @param enabled - 원격 쿼리 활성 여부(노드 ready 아닐 때 false).
 */
export function useMetricsTarget(
  target: ResourceTarget,
  refetchMs: number,
  enabled = true,
): MetricsTargetResult {
  const remote = isRemoteTarget(target);
  const instanceId = remote ? target.instanceId : '';

  const local = useQuery({
    queryKey: ['monitor', 'metrics'],
    queryFn: getMetrics,
    refetchInterval: refetchMs,
    enabled: !remote,
  });

  const remoteQuery = useQuery({
    queryKey: ['remote', 'metrics', instanceId],
    queryFn: () => remoteService.getRemoteMetrics(instanceId),
    refetchInterval: refetchMs,
    enabled: remote && enabled && !!instanceId,
  });

  return remote
    ? { metrics: remoteQuery.data, isLoading: remoteQuery.isLoading, error: remoteQuery.error }
    : { metrics: local.data, isLoading: local.isLoading, error: local.error };
}
