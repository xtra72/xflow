// useDashboardConfigTarget — 대시보드 config 소스를 target 으로 전환한다
// (SPEC-REMOTE-001 M10, 그룹 L, REQ-L01/L02/L03/L10).
//
//   - 로컬: useUIStore 에 동기화된 config(useDashboardSync 가 서버와 PUT/GET
//     양방향 동기). 본 훅은 로컬 분기에서 config 를 읽지 않는다 — 로컬 경로는
//     기존 DashboardPage(useUIStore + useDashboardSync)를 그대로 사용하므로,
//     본 훅은 원격 분기에서만 의미를 가진다(원격 read-only config 취득).
//   - 원격: GET /remote/nodes/{id}/dashboards/{shared|mine} 로 노드의 대시보드
//     config 를 READ-ONLY 취득한다(REQ-L01). 편집/PUT/sync 없음(v1.5 비목표 —
//     REQ-L12). config 내부 deviceId 는 그 노드 기준 해석(노드-로컬 — REQ-L03).
//
// 원격 config 는 단기 변동이 적으므로 적당한 polling 주기를 둔다(서버 TTL 캐시가
// 노드 부하를 완화 — REQ-J16). 실패(503/504/502/404)는 error 로 노출되어 호출
// 측이 editError 매핑으로 표시한다(REQ-L11).

import { useQuery } from '@tanstack/react-query';

import { isRemoteTarget, type ResourceTarget } from '@/lib/remote/target';
import * as remoteService from '@/services/api/remoteService';
import type { RemoteDashboardScope } from '@/services/api/remoteService';
import type { DashboardPayload, DashboardSnapshot } from '@/types/dashboard';

/** 원격 대시보드 config 갱신 주기(ms). config 는 완만 변동이므로 길게 둔다. */
const REMOTE_CONFIG_REFETCH_MS = 15000;

/** useDashboardConfigTarget 반환 형태. */
export interface DashboardConfigTargetResult {
  /** 취득한 READ-ONLY config payload(원격, 미수신 시 undefined). */
  payload: DashboardPayload | undefined;
  isLoading: boolean;
  error: unknown;
  refetch: () => void;
}

/**
 * 원격 노드의 대시보드 config 를 스코프별로 READ-ONLY 취득한다(REQ-L01).
 *
 * @param target - 원격 노드 타깃(로컬이면 비활성 — payload undefined).
 * @param scope - 'shared' | 'mine'(로컬 탭과 동일 의미).
 * @param enabled - 쿼리 활성 여부(노드 ready 아닐 때 false 로 발행 차단).
 */
export function useDashboardConfigTarget(
  target: ResourceTarget,
  scope: RemoteDashboardScope,
  enabled = true,
): DashboardConfigTargetResult {
  const remote = isRemoteTarget(target);
  const instanceId = remote ? target.instanceId : '';

  const query = useQuery<DashboardSnapshot>({
    queryKey: ['remote', 'dashboard', instanceId, scope],
    queryFn: () => remoteService.getRemoteDashboard(instanceId, scope),
    enabled: remote && enabled && !!instanceId,
    refetchInterval: REMOTE_CONFIG_REFETCH_MS,
  });

  return {
    payload: remote ? query.data?.payload : undefined,
    isLoading: remote ? query.isLoading : false,
    error: remote ? query.error : null,
    refetch: () => void query.refetch(),
  };
}
