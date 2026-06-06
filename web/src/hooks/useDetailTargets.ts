// 상세 타깃 추상화 — 상세 패널의 READ 데이터/라이브 데이터 소스를 target 으로
// 전환한다 (SPEC-REMOTE-001 M8, 그룹 J, REQ-J09/J11).
//
//   - 로컬: 기존 로컬 상세 훅(useFlowStatus/useFlowNodes/useAgent/useAgentStats/
//     useDeviceRealtime)을 동일 인자로 위임 → 회귀 없이 동일 동작.
//   - 원격: 정적/온디맨드 데이터는 READ/QUERY 프록시 GET(REQ-J04, 서버 TTL 캐시)
//     으로, 실시간 데이터(device.state / agent.stats / agent.series)는 SSE
//     스트림(REQ-J08)으로 취득하되 스트림 종료/실패 시 폴링 폴백한다.
//
// 모든 분기는 Rules of Hooks 를 위해 항상 동일 순서로 훅을 호출하되 비활성 분기는
// enabled=false 로 무력화한다.

import { useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';

import {
  useAgent as useLocalAgent,
  useAgentStats as useLocalAgentStats,
} from '@/hooks/useAgent';
import {
  useDeviceRealtime as useLocalDeviceRealtime,
} from '@/hooks/useDevice';
import {
  useFlowNodes as useLocalFlowNodes,
  useFlowStatus as useLocalFlowStatus,
} from '@/hooks/useFlow';
import { useRemoteStream } from '@/hooks/useRemoteStream';
import { isRemoteTarget, type ResourceTarget } from '@/lib/remote/target';
import * as remoteService from '@/services/api/remoteService';
import type { AgentInfo, AgentStatsInfo } from '@/types/agent';
import type { DeviceDetail } from '@/types/device';
import type { FlowNodeInfo, FlowStatusInfo } from '@/types/flow';

/** 단일 데이터 소스 결과(로컬 useQuery 와 호환되는 최소 형태). */
export interface DetailQueryResult<T> {
  data: T | undefined;
  isLoading: boolean;
  error: unknown;
}

// ---- 라이브 스트림 + 폴링 폴백 공통 헬퍼 ----
//
// 원격 라이브 데이터는 SSE 우선, 스트림 종료/실패(shouldFallback) 시 query-action
// 폴링으로 대체한다(REQ-J08). 스트림 프레임이 있으면 그것을, 없으면 폴링 결과를
// 노출한다. 스트림이 살아있는 동안에는 폴링을 비활성화해 노드 부하를 줄인다.

function useLiveWithFallback<T>(
  enabled: boolean,
  streamUrl: string | undefined,
  pollKey: unknown[],
  poll: () => Promise<T>,
  pollIntervalMs = 5000,
): DetailQueryResult<T> {
  const stream = useRemoteStream<T>(enabled ? streamUrl : undefined, {
    enabled,
  });
  // 스트림이 폴백을 요청할 때만 폴링한다(스트림 살아있는 동안 노드 부하 절감).
  const pollActive = enabled && stream.shouldFallback;
  const polled = useQuery({
    queryKey: pollKey,
    queryFn: poll,
    enabled: pollActive,
    refetchInterval: pollActive ? pollIntervalMs : false,
  });

  return useMemo(() => {
    if (!enabled) return { data: undefined, isLoading: false, error: null };
    if (stream.data !== undefined) {
      return { data: stream.data, isLoading: false, error: null };
    }
    if (stream.shouldFallback) {
      return {
        data: polled.data,
        isLoading: polled.isLoading,
        error: polled.error,
      };
    }
    // 스트림 연결 중(아직 프레임 없음).
    return {
      data: undefined,
      isLoading: stream.status === 'connecting',
      error: null,
    };
  }, [enabled, stream.data, stream.shouldFallback, stream.status, polled.data, polled.isLoading, polled.error]);
}

// ---- flow 상세 ----

/** 플로우 상태 타깃 훅(로컬 useFlowStatus 대체). */
export function useFlowStatusTarget(
  target: ResourceTarget,
  flowId: string,
): DetailQueryResult<FlowStatusInfo> {
  const remote = isRemoteTarget(target);
  const local = useLocalFlowStatus(remote ? '' : flowId);
  const remoteQuery = useQuery({
    queryKey: ['remote', 'detail', remote ? target.instanceId : '', 'flow', flowId, 'status'],
    queryFn: () =>
      remoteService.getRemoteFlowStatus<FlowStatusInfo>(
        (target as { instanceId: string }).instanceId,
        flowId,
      ),
    enabled: remote && !!flowId,
    refetchInterval: 5000,
  });
  return remote
    ? { data: remoteQuery.data, isLoading: remoteQuery.isLoading, error: remoteQuery.error }
    : { data: local.data, isLoading: local.isLoading, error: local.error };
}

/** 플로우 노드 목록 타깃 훅(로컬 useFlowNodes 대체). */
export function useFlowNodesTarget(
  target: ResourceTarget,
  flowId: string,
  refetchInterval?: number,
): DetailQueryResult<FlowNodeInfo[]> {
  const remote = isRemoteTarget(target);
  const local = useLocalFlowNodes(remote ? '' : flowId, refetchInterval);
  const remoteQuery = useQuery({
    queryKey: ['remote', 'detail', remote ? target.instanceId : '', 'flow', flowId, 'nodes'],
    queryFn: () =>
      remoteService.getRemoteFlowNodes<FlowNodeInfo[]>(
        (target as { instanceId: string }).instanceId,
        flowId,
      ),
    enabled: remote && !!flowId,
    refetchInterval,
  });
  return remote
    ? { data: remoteQuery.data, isLoading: remoteQuery.isLoading, error: remoteQuery.error }
    : { data: local.data, isLoading: local.isLoading, error: local.error };
}

// ---- agent 상세 ----

/** 에이전트 상세 타깃 훅(로컬 useAgent 대체). */
export function useAgentDetailTarget(
  target: ResourceTarget,
  agentId: string,
  detail: 'summary' | 'full' = 'summary',
): DetailQueryResult<AgentInfo> {
  const remote = isRemoteTarget(target);
  const local = useLocalAgent(remote ? '' : agentId, detail);
  const remoteQuery = useQuery({
    queryKey: ['remote', 'detail', remote ? target.instanceId : '', 'agent', agentId, detail],
    queryFn: () =>
      remoteService.getRemoteAgent<AgentInfo>(
        (target as { instanceId: string }).instanceId,
        agentId,
      ),
    enabled: remote && !!agentId,
  });
  return remote
    ? { data: remoteQuery.data, isLoading: remoteQuery.isLoading, error: remoteQuery.error }
    : { data: local.data, isLoading: local.isLoading, error: local.error };
}

/** 에이전트 라이브 통계 타깃 훅(로컬 useAgentStats 대체, 원격은 SSE+폴백). */
export function useAgentStatsTarget(
  target: ResourceTarget,
  agentId: string,
): DetailQueryResult<AgentStatsInfo> {
  const remote = isRemoteTarget(target);
  const local = useLocalAgentStats(remote ? '' : agentId);
  const instanceId = remote ? target.instanceId : '';
  const live = useLiveWithFallback<AgentStatsInfo>(
    remote && !!agentId,
    remote ? remoteService.remoteStreamUrl(instanceId, { domain: 'agent', action: 'stats' }, agentId) : undefined,
    ['remote', 'detail', instanceId, 'agent', agentId, 'stats'],
    () => remoteService.getRemoteAgentStats<AgentStatsInfo>(instanceId, agentId),
  );
  return remote ? live : { data: local.data, isLoading: local.isLoading, error: local.error };
}

// ---- device 상세 ----

/** 디바이스 상세+실시간 상태 타깃 훅(로컬 useDeviceRealtime 대체, 원격은 SSE+폴백). */
export function useDeviceDetailTarget(
  target: ResourceTarget,
  deviceId: string,
): DetailQueryResult<DeviceDetail> {
  const remote = isRemoteTarget(target);
  const local = useLocalDeviceRealtime(remote ? '' : deviceId);
  const instanceId = remote ? target.instanceId : '';

  // 정적 상세(get): 명령 스펙/메타데이터/식별 정보. 온디맨드 1회 + 완만 갱신.
  const base = useQuery({
    queryKey: ['remote', 'detail', instanceId, 'device', deviceId, 'get'],
    queryFn: () =>
      remoteService.getRemoteDevice<DeviceDetail>(instanceId, deviceId),
    enabled: remote && !!deviceId,
  });

  // 실시간 상태(state): SSE 우선, 폴백 폴링.
  const live = useLiveWithFallback<DeviceStatePayload>(
    remote && !!deviceId,
    remote ? remoteService.remoteStreamUrl(instanceId, { domain: 'device', action: 'state' }, deviceId) : undefined,
    ['remote', 'detail', instanceId, 'device', deviceId, 'state'],
    () => remoteService.getRemoteDeviceState<DeviceStatePayload>(instanceId, deviceId),
  );

  // 정적 상세 + 실시간 상태를 병합해 로컬 DeviceDetail 형태로 노출한다.
  // 실시간 프레임은 properties 만 갱신하고, 나머지 state 메타(online/ready 등)는
  // 정적 상세(base.state)를 보존한다(노드 redacted state 가 properties 위주이므로).
  const merged = useMemo<DeviceDetail | undefined>(() => {
    if (!remote) return undefined;
    if (!base.data) return undefined;
    if (!live.data) return base.data;
    const liveProps = extractProperties(live.data);
    if (!liveProps) return base.data;
    const baseState = base.data.state;
    return {
      ...base.data,
      state: {
        online: baseState?.online ?? base.data.online,
        ready: baseState?.ready ?? true,
        last_seen: baseState?.last_seen ?? base.data.last_seen,
        error_count: baseState?.error_count ?? 0,
        properties: liveProps,
      },
    };
  }, [remote, base.data, live.data]);

  if (!remote) {
    return { data: local.data, isLoading: local.isLoading, error: local.error };
  }
  return {
    data: merged,
    isLoading: base.isLoading,
    error: base.error ?? live.error,
  };
}

/** device.state 스트림/폴링 페이로드(노드 redacted state 형태). */
interface DeviceStatePayload {
  /** 일부 노드는 { state: { properties } } 형태, 일부는 { properties } 직접. */
  state?: { properties?: Record<string, unknown> } | Record<string, unknown>;
  properties?: Record<string, unknown>;
}

/** state 페이로드에서 properties 맵을 추출한다(없으면 undefined). */
function extractProperties(
  payload: DeviceStatePayload,
): Record<string, unknown> | undefined {
  if (payload.state && typeof payload.state === 'object') {
    const s = payload.state as { properties?: Record<string, unknown> };
    if (s.properties) return s.properties;
    return payload.state as Record<string, unknown>;
  }
  return payload.properties;
}
