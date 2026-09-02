// sysmetrics 에이전트 스냅샷 폴링 훅.
//
// 에이전트 상태(`GET /agents/{id}?detail=summary` 의 state)를 주기적으로 읽어
// 스냅샷으로 정리하고, 누적 카운터 계열을 위해 직전 스냅샷을 함께 돌려준다.
//
// 쿼리 키는 `useAgent` 와 **같은 것**을 쓴다(`['agents', id, 'summary']`). 한
// 대시보드에 같은 에이전트를 보는 패널이 셋 놓여도 요청은 하나다 — 키를 달리하면
// 패널 수만큼 폴링이 늘어난다.

import { useContext, useEffect, useMemo, useRef, useState } from 'react';
import { QueryClientContext, useQuery } from '@tanstack/react-query';

import * as agentService from '@/services/api/agentService';
import { inertQueryClient } from '@/hooks/inertQueryClient';

import { isSameSample, toSnapshot, type SysMetricsSnapshot } from './sysMetricsSeries';

/** 패널이 화면을 가르는 데 쓰는 상태 */
export type SysMetricsPanelState =
  /** 첫 응답 대기 */
  | 'loading'
  /** 바인딩된 에이전트가 없거나 조회에 실패했다 (삭제·권한·네트워크) */
  | 'unavailable'
  /** 에이전트가 sysmetrics 가 아니어서 state 를 해석할 수 없다 */
  | 'incompatible'
  /** 에이전트는 있으나 아직 첫 표본을 뜨지 않았다 */
  | 'no_sample'
  /** 에이전트가 멈춰 있다 (마지막 표본은 남아 있을 수 있다) */
  | 'stopped'
  /** 정상 */
  | 'ready';

/** 훅 반환 형태 */
export interface UseSysMetricsSnapshotResult {
  /** 최신 스냅샷 (없으면 null) */
  snapshot: SysMetricsSnapshot | null;
  /**
   * 직전 표본. 누적 카운터의 증가량을 내려면 두 점이 필요하다.
   *
   * 같은 표본을 다시 받은 경우에는 갱신하지 않는다 — 갱신하면 경과 0 인 구간이
   * 생겨 rate 가 계단처럼 끊긴다.
   */
  previous: SysMetricsSnapshot | null;
  /** 화면 분기용 상태 */
  state: SysMetricsPanelState;
}

/**
 * 에이전트 스냅샷을 폴링한다.
 *
 * @param agentId 바인딩된 에이전트의 안정적 ID. 빈 문자열이면 조회하지 않는다.
 * @param refreshMs 폴링 주기
 */
export function useSysMetricsSnapshot(
  agentId: string,
  refreshMs: number,
): UseSysMetricsSnapshotResult {
  // Provider 가 없으면 비활성 클라이언트를 쓴다(`inertQueryClient` 주석 참조).
  //
  // 이 훅은 차트 패널의 sysmetrics 소스 경로에서도 쓰이는데, 그 경로는
  // `usePanelSeriesData` 가 소스 종류와 무관하게 **항상** 호출한다. Provider 를 요구하면
  // Provider 없이 렌더하던 패널 테스트가 소스와 무관하게 전부 깨진다.
  const client = useContext(QueryClientContext);
  const { data, isLoading, isError } = useQuery(
    {
      // useAgent 와 같은 키 — 같은 에이전트를 보는 패널들이 캐시를 나눠 쓴다.
      queryKey: ['agents', agentId, 'summary'],
      queryFn: () => agentService.getAgent(agentId, 'summary'),
      refetchInterval: refreshMs,
      // 탭이 뒤에 있을 때까지 폴링하면 돌아왔을 때 의미 없는 구간이 잔뜩 쌓인다.
      refetchIntervalInBackground: false,
      enabled: !!agentId && client !== undefined,
    },
    client ?? inertQueryClient(),
  );

  const snapshot = useMemo(
    () => (data ? toSnapshot((data as { state?: unknown }).state) : null),
    [data],
  );

  // 기준점. 렌더를 유발할 필요가 없어 ref 로 들고, 실제로 바뀐 경우에만 state 로 올린다.
  //
  // 어느 에이전트의 표본인지 함께 들고 다닌다. 에이전트 교체를 별도 effect 로 다루면
  // 마운트 시 두 effect 가 같은 ref 를 두고 경쟁해 기준점이 항상 비게 된다 — 실제로
  // 그렇게 짰다가 테스트가 잡았다.
  const baselineRef = useRef<{ agentId: string; snapshot: SysMetricsSnapshot } | null>(null);
  const [previous, setPrevious] = useState<SysMetricsSnapshot | null>(null);

  useEffect(() => {
    // 에이전트가 바뀌면 기준점을 버린다. 다른 호스트의 누적 카운터를 이어 붙이면
    // 첫 구간에 거대한 스파이크가 생긴다.
    if (baselineRef.current && baselineRef.current.agentId !== agentId) {
      baselineRef.current = null;
      setPrevious(null);
    }

    if (!snapshot || snapshot.collectedAt === null) return;

    const baseline = baselineRef.current;
    // 같은 표본을 다시 받은 것이면 기준점을 그대로 둔다.
    if (baseline && isSameSample(baseline.snapshot, snapshot)) return;

    setPrevious(baseline?.snapshot ?? null);
    baselineRef.current = { agentId, snapshot };
  }, [agentId, snapshot]);

  return { snapshot, previous, state: resolveState({ agentId, isLoading, isError, snapshot }) };
}

/** 화면 분기 상태를 정한다. */
function resolveState(input: {
  agentId: string;
  isLoading: boolean;
  isError: boolean;
  snapshot: SysMetricsSnapshot | null;
}): SysMetricsPanelState {
  const { agentId, isLoading, isError, snapshot } = input;

  if (!agentId || isError) return 'unavailable';
  if (isLoading) return 'loading';
  // 응답은 왔는데 state 를 해석할 수 없다 — sysmetrics 가 아닌 에이전트에 바인딩됐다.
  if (!snapshot) return 'incompatible';

  if (snapshot.status === 'no_sample') return 'no_sample';
  // 멈춘 에이전트라도 마지막 표본은 보여 준다. 값을 0 으로 덮으면 "멈췄다"와
  // "0 이 되었다"가 구별되지 않는다.
  if (snapshot.status === 'stopped') return 'stopped';
  return 'ready';
}
