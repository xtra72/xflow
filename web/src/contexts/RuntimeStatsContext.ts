// 런타임 노드 통계 컨텍스트.
// 에디터 캔버스의 CustomNode가 런타임 메시지 카운트를 표시할 때 사용한다.

import { createContext, useContext } from 'react';

/** 노드별 런타임 통계 */
export interface NodeRuntimeStats {
  inMessages: number;
  outMessages: number;
  state: string;
}

/**
 * 노드 ID를 키로 한 런타임 통계 맵.
 * EditorPage에서 플로우 실행 중일 때 폴링하여 제공한다.
 */
export const RuntimeStatsContext = createContext<Record<string, NodeRuntimeStats>>({});

/** 특정 노드의 런타임 통계를 조회하는 훅 */
export function useNodeRuntimeStats(nodeId: string): NodeRuntimeStats | undefined {
  const stats = useContext(RuntimeStatsContext);
  return stats[nodeId];
}
