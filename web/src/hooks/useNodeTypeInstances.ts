// 특정 노드 타입의 플로우별 인스턴스를 집계하는 훅.
// running 플로우들의 상태에서 node_stats를 조회하여 해당 타입의 노드 인스턴스를 추출한다.

import { useMemo } from 'react';
import { useQueries } from '@tanstack/react-query';

import * as flowService from '@/services/api/flowService';
import { useFlows } from './useFlow';

export interface NodeInstance {
  flowId: string;
  flowName: string;
  nodeId: string;
  nodeName: string;
  processed: number;
  errors: number;
}

/**
 * 특정 노드 타입의 인스턴스를 모든 running 플로우에서 집계한다.
 * enabled가 false이면 쿼리를 실행하지 않는다 (패널 미확장 시).
 */
export function useNodeTypeInstances(nodeType: string, enabled = true) {
  const { data: flowsData, isLoading: flowsLoading } = useFlows();
  const flows = useMemo(() => flowsData?.data ?? [], [flowsData?.data]);

  // running 플로우만 상태 조회
  const runningFlows = useMemo(
    () => flows.filter((f) => f.status === 'running'),
    [flows],
  );

  // 각 running 플로우의 상태를 병렬 조회
  const statusQueries = useQueries({
    queries: runningFlows.map((flow) => ({
      queryKey: ['flows', flow.id, 'status', 'instances'],
      queryFn: () => flowService.getFlowStatus(flow.id),
      staleTime: 30_000,
      enabled: enabled && runningFlows.length > 0,
    })),
  });

  const isLoading = flowsLoading || statusQueries.some((q) => q.isLoading);

  // node_stats에서 타입별 인스턴스 추출
  const instances = useMemo<NodeInstance[]>(() => {
    const result: NodeInstance[] = [];
    for (let i = 0; i < runningFlows.length; i++) {
      const flow = runningFlows[i]!;
      const status = statusQueries[i]?.data;
      if (!status?.node_stats) continue;

      for (const stat of status.node_stats) {
        if (stat.node_type === nodeType) {
          result.push({
            flowId: flow.id,
            flowName: flow.name,
            nodeId: stat.node_id,
            nodeName: stat.node_name,
            processed: stat.processed,
            errors: stat.errors,
          });
        }
      }
    }
    return result;
  }, [runningFlows, statusQueries, nodeType]);

  return { instances, isLoading };
}
