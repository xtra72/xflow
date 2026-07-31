// SPEC-SCHEDULE-VIEW-001 M5: 스케줄 뷰 교차-플로우 집계(프런트엔드 팬아웃, RD-10).
//
// 백엔드 집계 엔드포인트 없이(RD-10) useFlows()로 플로우 목록을 받고 useQueries로
// 각 플로우 정의(getFlow)를 병렬 조회한다. 각 플로우 정의(config.nodes[].data)에서
// trigger 노드와 그 config.schedules[] 를 추출한다. 노드 런타임 목록(getFlowNodes)은
// 미배포 플로우에서 빈 배열을 반환(ListFlowNodes_미배포_미참조_빈결과)하므로 사용하지
// 않는다. 스케줄 SSOT 는 플로우 정의이며, dual-write PERSIST 경로(patchNodeConfigIn
// Definition)와 동일한 소스를 읽어 origin({flowId,nodeId,index})을 보존한다.
//
// 부수효과 없는 파싱/그룹화 함수 + 이를 react-query 로 배선한 useScheduleAggregation
// 훅으로 구성한다. 순수 함수는 UI 없이 vitest 로 검증한다(그룹키/미지정 버킷/교차 집계).

import { useCallback, useMemo } from 'react';
import { useQueries, useQueryClient } from '@tanstack/react-query';

import { getFlow } from '@/services/api/flowService';
import type { FlowInfo } from '@/types/flow';
import type { SerializedSchedule } from '@/pages/dashboard/panels/facilitySchedule/facilityScheduleUtils';

import { useFlows } from '@/hooks/useFlow';

/** 미지정(agent 미할당) 버킷 라벨(AC-8). */
export const UNASSIGNED_LABEL = '미지정';

/**
 * 한 trigger 노드의 스케줄 집합 + origin. CRUD 가 소유 노드의 FULL schedules 배열을
 * 재구성해 dual-write 할 수 있도록 nodeConfig(=노드 data, baseline)와 schedules 를 함께 담는다.
 */
export interface ScheduleNodeEntry {
  flowId: string;
  flowName: string;
  nodeId: string;
  nodeName: string;
  /** 노드 정의 config(reactflow data). dual-write baseline + node-level agentId 소스. */
  nodeConfig: Record<string, unknown>;
  /** 노드의 FULL schedules 배열(원본 인덱스 = CRUD origin). */
  schedules: SerializedSchedule[];
}

/** agent 그룹(AC-7). key=null 은 미지정 버킷(AC-8). */
export interface ScheduleAgentGroup {
  /** 해석된 agent id. null = 미지정. */
  key: string | null;
  /** 이 그룹에 속하는(≥1 스케줄 또는 빈 노드는 node-level agent 로) 노드 엔트리들. */
  nodes: ScheduleNodeEntry[];
}

/** 문자열을 안전하게 읽는다(빈 문자열/비문자열 → null). */
function readString(v: unknown): string | null {
  return typeof v === 'string' && v.length > 0 ? v : null;
}

/**
 * 스케줄의 그룹 키(AC-7). 명시 필드 우선: schedule.agent_id ?? nodeConfig.agentId ?? null.
 * 엣지 순회가 아니라 명시 필드로만 결정한다(RD-10).
 */
export function resolveScheduleAgent(
  schedule: SerializedSchedule,
  nodeConfig: Record<string, unknown>,
): string | null {
  return readString(schedule.agent_id) ?? readString(nodeConfig.agentId) ?? null;
}

/** 노드 레벨 agent(빈 노드의 그룹 배치용). nodeConfig.agentId ?? null. */
export function nodeLevelAgent(nodeConfig: Record<string, unknown>): string | null {
  return readString(nodeConfig.agentId);
}

/**
 * 한 플로우 정의에서 trigger 노드 엔트리를 추출한다. nodeType==='trigger' 또는
 * schedules 배열을 가진 노드를 스케줄 보유 노드로 본다. schedules 가 없으면 빈 배열.
 */
export function collectTriggerNodeEntries(flow: FlowInfo): ScheduleNodeEntry[] {
  const def = flow.config;
  if (!def || typeof def !== 'object') return [];
  const rawNodes = (def as Record<string, unknown>).nodes;
  const nodes = Array.isArray(rawNodes) ? (rawNodes as unknown[]) : [];

  const out: ScheduleNodeEntry[] = [];
  for (const raw of nodes) {
    if (!raw || typeof raw !== 'object') continue;
    const node = raw as Record<string, unknown>;
    const id = node.id;
    if (typeof id !== 'string' || !id) continue;

    const data =
      node.data && typeof node.data === 'object' && !Array.isArray(node.data)
        ? (node.data as Record<string, unknown>)
        : {};

    const hasSchedules = Array.isArray(data.schedules);
    const isTrigger = data.nodeType === 'trigger' || hasSchedules;
    if (!isTrigger) continue;

    const schedules = hasSchedules ? (data.schedules as SerializedSchedule[]) : [];
    const label = readString(data.label) ?? id;
    out.push({
      flowId: flow.id,
      flowName: flow.name,
      nodeId: id,
      nodeName: label,
      nodeConfig: data,
      schedules,
    });
  }
  return out;
}

/**
 * 노드 엔트리들을 agent 그룹으로 묶는다(AC-7/AC-8). 스케줄이 있는 노드는 스케줄별
 * 해석 agent 마다 해당 그룹에 편입되고(엔트리 중복 편입 = 혼합 agent 노드 지원),
 * 스케줄이 없는 노드는 node-level agent(또는 미지정)로 배치해 최초 규칙 추가 진입점을 남긴다.
 * 어떤 것도 드롭하지 않는다(AC-8).
 */
export function groupEntriesByAgent(entries: ScheduleNodeEntry[]): ScheduleAgentGroup[] {
  const map = new Map<string, ScheduleAgentGroup>();
  const bucketId = (k: string | null) => (k === null ? '__unassigned__' : `a:${k}`);
  const add = (k: string | null, entry: ScheduleNodeEntry) => {
    const id = bucketId(k);
    let g = map.get(id);
    if (!g) {
      g = { key: k, nodes: [] };
      map.set(id, g);
    }
    if (!g.nodes.some((n) => n.flowId === entry.flowId && n.nodeId === entry.nodeId)) {
      g.nodes.push(entry);
    }
  };

  for (const entry of entries) {
    if (entry.schedules.length === 0) {
      add(nodeLevelAgent(entry.nodeConfig), entry);
      continue;
    }
    const keys = new Set<string | null>();
    for (const s of entry.schedules) keys.add(resolveScheduleAgent(s, entry.nodeConfig));
    for (const k of keys) add(k, entry);
  }

  return Array.from(map.values());
}

/** {@link useScheduleAggregation} 반환 형태. */
export interface UseScheduleAggregation {
  /** 모든 플로우에서 수집한 trigger 노드 엔트리(플랫). */
  nodeEntries: ScheduleNodeEntry[];
  /** 팬아웃 진행 중(초기 로딩 표시용, AC-18). */
  isLoading: boolean;
  /** 조회 실패한 플로우 수(부분 실패 통지용, AC-18/REQ-07-05). */
  failedCount: number;
  /** 전체 플로우 수. */
  totalFlows: number;
  /** 목록 조회 실패 또는 모든 플로우 조회 실패(전면 에러). */
  hasError: boolean;
  /** 목록 + 정의 팬아웃을 재조회(dual-write 반영). */
  refetch: () => void;
}

/**
 * 교차-플로우 스케줄 집계 훅(RD-10 프런트엔드 팬아웃). useFlows() 로 목록을 받아
 * useQueries 로 각 플로우 정의(getFlow)를 병렬 조회하고 trigger 노드 엔트리를 수집한다.
 * 부분 실패 시 성공한 플로우만으로 계속 진행하고 failedCount 로 통지한다(AC-18).
 */
export function useScheduleAggregation(): UseScheduleAggregation {
  const queryClient = useQueryClient();
  const flowsQ = useFlows();
  const flows = useMemo(() => flowsQ.data?.data ?? [], [flowsQ.data]);

  const flowQueries = useQueries({
    queries: flows.map((f) => ({
      queryKey: ['flows', f.id, 'definition'] as const,
      queryFn: () => getFlow(f.id),
      staleTime: 30_000,
    })),
  });

  const nodeEntries = useMemo(() => {
    const out: ScheduleNodeEntry[] = [];
    for (let i = 0; i < flows.length; i++) {
      const data = flowQueries[i]?.data;
      if (!data) continue;
      out.push(...collectTriggerNodeEntries(data));
    }
    return out;
  }, [flows, flowQueries]);

  const failedCount = flowQueries.filter((q) => q.isError).length;
  const isLoading = flowsQ.isLoading || flowQueries.some((q) => q.isLoading);
  const hasError =
    flowsQ.isError || (flows.length > 0 && flowQueries.length > 0 && flowQueries.every((q) => q.isError));

  const refetch = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: ['flows'] });
  }, [queryClient]);

  return {
    nodeEntries,
    isLoading,
    failedCount,
    totalFlows: flows.length,
    hasError,
    refetch,
  };
}
