// useFlowsTarget / useAgentsTarget / useDevicesTarget — 목록 타깃 추상화
// (SPEC-REMOTE-001 M8, 그룹 J, REQ-J04/J09/J10).
//
// `useEditorFlowTarget`(M7) 패턴을 목록 페이지로 확장한다. target 에 따라:
//   - 로컬: 기존 로컬 목록 훅(useFlows/useAgents/useDevicesRealtime)을 동일
//     인자로 위임한다 → 반환·쿼리키·동작이 회귀 없이 동일하다.
//   - 원격: 노드의 라이브 목록(useNodeLiveList — M8 query 프록시)을 조회해
//     로컬과 동형의 FULL 타입 목록(connected/uptime/stats 등 런타임 필드 포함)을
//     반환한다. 따라서 목록 행이 로컬과 동일하게 type/status/uptime/messages 를
//     모두 렌더링한다. 라이브 쿼리가 실패하면(구버전 노드의 502/404 등) 그룹 E
//     미러 목록(useNodeMirror) 매핑으로 폴백하여 graceful 하게 degrade 한다.
//
// Rules of Hooks: 로컬/원격 분기 모두 항상 동일 순서로 모든 훅을 호출하되, 비활성
// 분기는 enabled=false/빈 인자로 무력화한다(useEditorFlowTarget 와 동일 전략).

import { useMemo } from 'react';

import { useAgents } from '@/hooks/useAgent';
import { useDevicesRealtime } from '@/hooks/useDevice';
import { useFlows } from '@/hooks/useFlow';
import { useNodeLiveList, useNodeMirror } from '@/hooks/useRemote';
import { useTargetGating } from '@/hooks/useTargetGating';
import { isRemoteTarget, type ResourceTarget } from '@/lib/remote/target';
import type { AgentInfo } from '@/types/agent';
import type { DeviceInfo, DeviceListParams } from '@/types/device';
import type { FlowInfo } from '@/types/flow';
import type { MirroredResource, MirroredResourceKind } from '@/types/remote';

/** 목록 타깃 훅 공통 반환 형태(로컬 목록 훅과 호환되는 최소 형태). */
export interface ListTargetResult<T> {
  data: { data: T[]; total: number } | undefined;
  isLoading: boolean;
  error: unknown;
  refetch: () => void;
  /** 원격 타깃 여부(페이지 배지/게이팅). */
  isRemote: boolean;
}

/** 미러 정의(JSON 문자열)를 파싱한다(손상 시 빈 객체). */
function parseDefinition(raw: string | undefined): Record<string, unknown> {
  if (!raw) return {};
  try {
    const parsed: unknown = JSON.parse(raw);
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>;
    }
  } catch {
    // 무시 — 빈 객체 폴백.
  }
  return {};
}

/** 미러 행 → FlowInfo(목록 렌더링에 필요한 최소 필드). */
function mirrorToFlow(r: MirroredResource): FlowInfo {
  const def = parseDefinition(r.definition);
  const nodes = Array.isArray(def.nodes) ? (def.nodes as unknown[]) : [];
  return {
    id: r.id,
    name: r.name,
    status: r.status ?? (r.online ? 'running' : 'stopped'),
    node_count: nodes.length,
    config: def,
    updated_at: r.updated_at ? new Date(r.updated_at).toISOString() : undefined,
  };
}

/** 미러 행 → AgentInfo(목록 렌더링에 필요한 최소 필드). */
function mirrorToAgent(r: MirroredResource): AgentInfo {
  const def = parseDefinition(r.definition);
  const type = typeof def.type === 'string' ? def.type : '';
  return {
    id: r.id,
    name: r.name,
    type,
    status: r.status ?? (r.online ? 'running' : 'stopped'),
    config: def,
  };
}

/** 미러 행 → DeviceInfo(목록 렌더링에 필요한 최소 필드). */
function mirrorToDevice(r: MirroredResource): DeviceInfo {
  const def = parseDefinition(r.definition);
  return {
    id: r.id,
    uid: r.id,
    name: r.name,
    type: typeof def.type === 'string' ? def.type : '',
    protocol: typeof def.protocol === 'string' ? def.protocol : '',
    agent_name: typeof def.agent_name === 'string' ? def.agent_name : '',
    source: typeof def.source === 'string' ? def.source : '',
    online: r.online,
    last_seen: r.updated_at ? new Date(r.updated_at).toISOString() : '',
    capabilities: [],
  };
}

/** 미러 쿼리 결과를 목록 타깃 결과로 어댑트한다. */
function adaptMirror<T>(
  mirror: { data?: MirroredResource[]; isLoading: boolean; error: unknown; refetch: () => unknown },
  map: (r: MirroredResource) => T,
): ListTargetResult<T> {
  const rows = mirror.data ?? [];
  return {
    data: { data: rows.map(map), total: rows.length },
    isLoading: mirror.isLoading,
    error: mirror.error,
    refetch: () => void mirror.refetch(),
    isRemote: true,
  };
}

/** 로컬 목록 쿼리 결과를 목록 타깃 결과로 어댑트한다. */
function adaptLocal<T>(
  query: { data?: { data: T[]; total: number }; isLoading: boolean; error: unknown; refetch: () => unknown },
): ListTargetResult<T> {
  return {
    data: query.data,
    isLoading: query.isLoading,
    error: query.error,
    refetch: () => void query.refetch(),
    isRemote: false,
  };
}

/** 라이브 목록 쿼리 결과를 목록 타깃 결과로 어댑트한다(로컬과 동형의 FULL 타입). */
function adaptLive<T>(
  query: { data?: T[]; isLoading: boolean; error: unknown; refetch: () => unknown },
): ListTargetResult<T> {
  const rows = query.data ?? [];
  return {
    data: { data: rows, total: rows.length },
    isLoading: query.isLoading,
    error: query.error,
    refetch: () => void query.refetch(),
    isRemote: true,
  };
}

/**
 * 원격 분기 결과를 합성한다(라이브 우선, 실패 시 미러 폴백 — REQ-J04 + graceful
 * degradation).
 *
 * 라이브 쿼리가 에러(구버전 노드의 502/404, 또는 노드 ready 가 아니라 비활성)면
 * 미러 매핑 결과로 폴백한다. 라이브가 성공/로딩이면 그 FULL 결과를 사용한다.
 *
 * @param liveErrored - 라이브 쿼리가 실패했는지(폴백 트리거).
 */
function combineRemote<T>(
  live: ListTargetResult<T>,
  mirror: ListTargetResult<T>,
  liveErrored: boolean,
): ListTargetResult<T> {
  return liveErrored ? mirror : live;
}

/**
 * 원격 라이브 목록 + 미러 폴백을 한 묶음으로 호출하는 내부 헬퍼.
 *
 * Rules of Hooks 를 위해 두 쿼리(라이브/미러)를 항상 동일 순서로 호출한다. 라이브는
 * 노드 ready(승인+온라인)일 때만 발행하고, 미러는 폴백 경로(라이브 비-ready/에러)에서만
 * 발행하여 정상 경로에서 불필요한 노드 부하를 줄인다.
 */
function useRemoteListSources<T>(
  instanceID: string,
  kind: MirroredResourceKind,
  remote: boolean,
  nodeReady: boolean,
  mapMirror: (r: MirroredResource) => T,
): { result: ListTargetResult<T>; isRemote: boolean } {
  const id = remote ? instanceID : '';
  // 라이브: remote ∧ nodeReady 일 때만 발행(미관리/오프라인 노드의 503 노이즈 방지).
  const live = useNodeLiveList(id, kind, remote && nodeReady);
  const liveErrored = !!live.error;
  // 미러 폴백: 라이브가 비활성(노드 미-ready)이거나 실패했을 때만 발행한다.
  const mirror = useNodeMirror(id, kind, remote && (!nodeReady || liveErrored));

  // 쿼리 객체 자체는 렌더마다 새 참조라 필드 단위로 분해해 메모 의존성을 안정화한다
  // (refetch 는 react-query 옵저버에 바인딩된 안정 참조).
  const { data: liveData, isLoading: liveLoading, error: liveError, refetch: liveRefetch } = live;
  const { data: mirrorData, isLoading: mirrorLoading, error: mirrorError, refetch: mirrorRefetch } = mirror;
  const liveResult = useMemo(
    () =>
      adaptLive<T>({
        data: liveData as T[] | undefined,
        isLoading: liveLoading,
        error: liveError,
        refetch: liveRefetch,
      }),
    [liveData, liveLoading, liveError, liveRefetch],
  );
  const mirrorResult = useMemo(
    () =>
      adaptMirror(
        { data: mirrorData, isLoading: mirrorLoading, error: mirrorError, refetch: mirrorRefetch },
        mapMirror,
      ),
    [mirrorData, mirrorLoading, mirrorError, mirrorRefetch, mapMirror],
  );
  // 폴백 조건: 라이브 에러 OR 노드 미-ready(라이브 비활성).
  const result = combineRemote(liveResult, mirrorResult, liveErrored || !nodeReady);
  return { result, isRemote: true };
}

/** 플로우 목록 타깃 훅. */
export function useFlowsTarget(target: ResourceTarget): ListTargetResult<FlowInfo> {
  const remote = isRemoteTarget(target);
  const local = useFlows();
  const gating = useTargetGating(target);
  const { result } = useRemoteListSources<FlowInfo>(
    remote ? target.instanceId : '',
    'flow',
    remote,
    gating.nodeReady,
    mirrorToFlow,
  );
  return remote ? result : adaptLocal(local);
}

/** 에이전트 목록 타깃 훅. */
export function useAgentsTarget(target: ResourceTarget): ListTargetResult<AgentInfo> {
  const remote = isRemoteTarget(target);
  const local = useAgents();
  const gating = useTargetGating(target);
  const { result } = useRemoteListSources<AgentInfo>(
    remote ? target.instanceId : '',
    'agent',
    remote,
    gating.nodeReady,
    mirrorToAgent,
  );
  return remote ? result : adaptLocal(local);
}

/**
 * 디바이스 목록 타깃 훅.
 *
 * @param localParams - 로컬 분기에서 useDevicesRealtime 에 전달할 필터(원격은
 *   라이브/미러 전체를 받고 페이지가 클라이언트 필터링하므로 미사용).
 */
export function useDevicesTarget(
  target: ResourceTarget,
  localParams?: DeviceListParams,
): ListTargetResult<DeviceInfo> {
  const remote = isRemoteTarget(target);
  const local = useDevicesRealtime(remote ? undefined : localParams);
  const gating = useTargetGating(target);
  const { result } = useRemoteListSources<DeviceInfo>(
    remote ? target.instanceId : '',
    'device',
    remote,
    gating.nodeReady,
    mirrorToDevice,
  );
  return remote ? result : adaptLocal(local);
}
