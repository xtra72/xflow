// 런타임 노드 통계 컨텍스트.
// 에디터 캔버스의 CustomNode가 런타임 메시지 카운트를 표시할 때 사용한다.

import { createContext, useContext } from 'react';

/** 포트별 런타임 통계 (포트 이름 + 방향으로 식별). */
export interface PortRuntimeStat {
  name: string;
  direction: string;
  /** 노드가 이 포트로 생성(emit)한 메시지 누계. */
  messages: number;
  /** 연결된 와이어로 실제 전달된 메시지 누계 (output 포트에서 의미 있음). */
  delivered: number;
}

/**
 * 노드 통계의 출처(SPEC-SUBFLOW-002 S03).
 * 백엔드 FlowNodeInfo.Extra["stat_source"] 값과 1:1 매핑된다.
 *   - 'direct'          : shared(공유) 또는 단독 실행 통계.
 *   - 'embedded'        : instance(인라인 복제본) 임베디드 병합 통계.
 *   - 'direct+embedded' : 위 둘이 혼합된 통계.
 */
export type StatSource = 'direct' | 'embedded' | 'direct+embedded';

/** 노드별 런타임 통계 */
export interface NodeRuntimeStats {
  inMessages: number;
  outMessages: number;
  state: string;
  /**
   * 포트별 상세 통계. output 포트의 delivered / 큐 적체량(messages - delivered)
   * 표시에 사용한다. 노드 카드는 포트 이름으로 조회한다.
   */
  ports: PortRuntimeStat[];
  /**
   * SPEC-SUBFLOW-002 S03: 통계 출처 표식. 백엔드 Extra["stat_source"] 를 그대로
   * 운반한다(없으면 undefined). 노드 카드는 'embedded' / 'direct+embedded' 일 때만
   * 출처 배지를 표시한다(direct/미표식은 기본이라 배지 없음).
   */
  statSource?: StatSource;
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
