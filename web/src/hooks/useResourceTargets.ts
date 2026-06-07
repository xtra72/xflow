// useFlowsTarget / useAgentsTarget / useDevicesTarget — 목록 타깃 추상화
// (SPEC-REMOTE-001 M8, 그룹 J, REQ-J09/J10).
//
// `useEditorFlowTarget`(M7) 패턴을 목록 페이지로 확장한다. target 에 따라:
//   - 로컬: 기존 로컬 목록 훅(useFlows/useAgents/useDevicesRealtime)을 동일
//     인자로 위임한다 → 반환·쿼리키·동작이 회귀 없이 동일하다.
//   - 원격: 그룹 E 미러 목록(useNodeMirror)을 조회해 로컬 목록 아이템 형태로
//     매핑한다(REQ-J04 — 목록은 미러 우선). 페이지는 동일 코드로 렌더링한다.
//
// Rules of Hooks: 로컬/원격 분기 모두 항상 동일 순서로 두 훅을 호출하되, 비활성
// 분기는 enabled=false/빈 인자로 무력화한다(useEditorFlowTarget 와 동일 전략).

import { useMemo } from 'react';

import { useAgents } from '@/hooks/useAgent';
import { useDevicesRealtime } from '@/hooks/useDevice';
import { useFlows } from '@/hooks/useFlow';
import { useNodeMirror } from '@/hooks/useRemote';
import { isRemoteTarget, type ResourceTarget } from '@/lib/remote/target';
import type { AgentInfo } from '@/types/agent';
import type { DeviceInfo, DeviceListParams } from '@/types/device';
import type { FlowInfo } from '@/types/flow';
import type { MirroredResource } from '@/types/remote';

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
  mirror: ReturnType<typeof useNodeMirror>,
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

/** 플로우 목록 타깃 훅. */
export function useFlowsTarget(target: ResourceTarget): ListTargetResult<FlowInfo> {
  const remote = isRemoteTarget(target);
  const local = useFlows();
  const mirror = useNodeMirror(remote ? target.instanceId : '', 'flow', remote);
  const localResult = adaptLocal(local);
  const remoteResult = useMemo(
    () => adaptMirror(mirror, mirrorToFlow),
    [mirror.data, mirror.isLoading, mirror.error],
  );
  return remote ? remoteResult : localResult;
}

/** 에이전트 목록 타깃 훅. */
export function useAgentsTarget(target: ResourceTarget): ListTargetResult<AgentInfo> {
  const remote = isRemoteTarget(target);
  const local = useAgents();
  const mirror = useNodeMirror(remote ? target.instanceId : '', 'agent', remote);
  const localResult = adaptLocal(local);
  const remoteResult = useMemo(
    () => adaptMirror(mirror, mirrorToAgent),
    [mirror.data, mirror.isLoading, mirror.error],
  );
  return remote ? remoteResult : localResult;
}

/**
 * 디바이스 목록 타깃 훅.
 *
 * @param localParams - 로컬 분기에서 useDevicesRealtime 에 전달할 필터(원격은
 *   미러 전체를 받고 페이지가 클라이언트 필터링하므로 미사용).
 */
export function useDevicesTarget(
  target: ResourceTarget,
  localParams?: DeviceListParams,
): ListTargetResult<DeviceInfo> {
  const remote = isRemoteTarget(target);
  const local = useDevicesRealtime(remote ? undefined : localParams);
  const mirror = useNodeMirror(remote ? target.instanceId : '', 'device', remote);
  const localResult = adaptLocal(local);
  const remoteResult = useMemo(
    () => adaptMirror(mirror, mirrorToDevice),
    [mirror.data, mirror.isLoading, mirror.error],
  );
  return remote ? remoteResult : localResult;
}
