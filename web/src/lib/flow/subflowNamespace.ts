// 서브플로우 런타임 통계 네임스페이스 유틸 (WEB 측 Fix 1 / Fix 2 공용).
//
// 배포 시 flow-node 가 참조하는 서브플로우의 내부 노드는 부모 플로우 안에서
// 네임스페이스 ID `subflow_<flowNodeID>_<originalID>` 로 실행된다(백엔드
// subflow_expand.go 참조). 따라서:
//   - 부모 뷰: flow-node 자신의 id 로 키된 엔진 노드가 없어 통계가 0 으로 보인다.
//   - 서브플로우 단독 뷰: 내부 노드가 원본 id 가 아닌 네임스페이스 id 로 돌아가 0 으로 보인다.
//
// 이 모듈은 백엔드 SubflowNodeIDPrefix 와 동일한 접두 문자열 규약을 복제하고,
// 부모 뷰 집계(Fix 1)와 서브플로우 단독 뷰 병합(Fix 2)의 순수 로직을 제공한다.

import type {
  NodeRuntimeStats,
  PortRuntimeStat,
  StatSource,
} from '@/contexts/RuntimeStatsContext';
import type { FlowNodeInfo, SubflowNodeStat } from '@/types/flow';

/**
 * FlowNodeInfo.extra["stat_source"] 를 StatSource 로 정규화한다(SPEC-SUBFLOW-002 S03).
 *
 * 백엔드가 넣는 값은 'direct' | 'embedded' | 'direct+embedded' 셋 중 하나다. 그 외
 * 값/부재는 undefined 로 처리해 출처 배지를 표시하지 않는다(과해석 방지).
 */
export function parseStatSource(
  extra: Record<string, unknown> | undefined,
): StatSource | undefined {
  const raw = extra?.['stat_source'];
  if (raw === 'direct' || raw === 'embedded' || raw === 'direct+embedded') {
    return raw;
  }
  return undefined;
}

/**
 * 두 출처 표식을 합집합으로 결합한다(Fix 1 집계 보조).
 *
 * direct/embedded 둘 중 하나라도 양쪽에서 관측되면 'direct+embedded'(혼합)로 승급한다.
 * 한쪽만 있으면 그 값을, 둘 다 없으면 undefined 를 반환한다.
 */
export function combineStatSource(
  a: StatSource | undefined,
  b: StatSource | undefined,
): StatSource | undefined {
  if (a === undefined) return b;
  if (b === undefined) return a;
  if (a === b) return a;
  // 서로 다르거나 한쪽이 이미 혼합이면 혼합으로 승급한다.
  return 'direct+embedded';
}

/**
 * 서브플로우 네임스페이스 접두사를 만든다: `subflow_<flowNodeId>_`.
 *
 * 백엔드 service.SubflowNodeIDPrefix 와 1:1 동형이다(고정 토큰 "subflow_" +
 * flowNodeId + "_"). 부모 플로우에 평탄화된 모든 내부 노드 id 는 이 접두사로
 * 시작한다. 부모 뷰 집계(Fix 1)는 이 접두사로 시작하는 엔진 노드를 모아 합산한다.
 *
 * @param flowNodeId 부모 플로우 안의 flow-node(서브플로우 참조 노드) id.
 */
export function subflowNodeIdPrefix(flowNodeId: string): string {
  return `subflow_${flowNodeId}_`;
}

/** FlowNodeInfo 의 포트 합산값을 NodeRuntimeStats 로 정규화한다(공용 헬퍼). */
function portsToRuntimeStats(
  ports: ReadonlyArray<{ name: string; direction: string; messages: number; delivered: number }>,
  state: string,
  statSource?: StatSource,
): NodeRuntimeStats {
  const inMessages = ports
    .filter((p) => p.direction === 'input')
    .reduce((sum, p) => sum + p.messages, 0);
  const outMessages = ports
    .filter((p) => p.direction === 'output')
    .reduce((sum, p) => sum + p.messages, 0);
  const portStats: PortRuntimeStat[] = ports.map((p) => ({
    name: p.name,
    direction: p.direction,
    messages: p.messages,
    delivered: p.delivered,
  }));
  return { inMessages, outMessages, state, ports: portStats, statSource };
}

/**
 * 단일 runtime 노드(FlowNodeInfo)를 NodeRuntimeStats 로 변환한다.
 *
 * 기존 EditorPage 의 인라인 매핑과 동일한 규칙(input/output 포트 messages 합산,
 * 포트별 messages/delivered 보존)을 순수 함수로 추출한 것이다. SPEC-SUBFLOW-002 S03:
 * extra["stat_source"] 표식도 NodeRuntimeStats.statSource 로 함께 운반한다.
 */
export function nodeInfoToRuntimeStats(node: FlowNodeInfo): NodeRuntimeStats {
  return portsToRuntimeStats(node.ports ?? [], node.state, parseStatSource(node.extra));
}

/**
 * runtime 노드 목록(getFlowNodes 결과)으로부터 exact-id 키 통계 맵을 만든다.
 *
 * 일반 노드는 자신의 엔진 id 로 그대로 매핑된다(네임스페이스 노드 포함 — 이후
 * Fix 1 집계가 flow-node 위로 합산한다).
 */
export function buildRuntimeStatsMap(
  nodes: ReadonlyArray<FlowNodeInfo>,
): Record<string, NodeRuntimeStats> {
  const map: Record<string, NodeRuntimeStats> = {};
  for (const node of nodes) {
    map[node.node_id] = nodeInfoToRuntimeStats(node);
  }
  return map;
}

/**
 * 여러 포트 통계를 포트 이름 단위로 합산한다(Fix 1 보조).
 * 같은 이름 포트의 messages/delivered 를 더하고, 방향은 최초 등장 기준으로 보존한다.
 */
function sumPorts(
  accumulator: Map<string, PortRuntimeStat>,
  order: string[],
  ports: ReadonlyArray<PortRuntimeStat>,
): void {
  for (const p of ports) {
    const existing = accumulator.get(p.name);
    if (existing === undefined) {
      accumulator.set(p.name, {
        name: p.name,
        direction: p.direction,
        messages: p.messages,
        delivered: p.delivered,
      });
      order.push(p.name);
    } else {
      existing.messages += p.messages;
      existing.delivered += p.delivered;
    }
  }
}

/** 자식 런타임 상태 집합에서 대표 상태를 고른다(하나라도 running 이면 running). */
function pickAggregateState(states: ReadonlyArray<string>): string {
  if (states.some((s) => s === 'running')) return 'running';
  // running 이 없으면 비어있지 않은 첫 상태를, 전부 비면 '' 를 반환한다.
  return states.find((s) => s !== '') ?? '';
}

/**
 * 특정 flow-node 의 직속 내부 노드(`subflow_<flowNodeId>_*`) 통계를 합산한다(Fix 1).
 *
 * 부모 뷰에서 flow-node 자신의 id 로는 엔진 노드가 없으므로, 접두사로 시작하는
 * 모든 엔진 노드의 processed/errors(= 포트 messages)와 포트별 messages/delivered 를
 * 더해 flow-node 한 칸에 표시할 NodeRuntimeStats 를 만든다. 일치하는 자식이 하나도
 * 없으면 undefined 를 반환한다(통계 미표시 유지).
 *
 * @param flowNodeId 집계 대상 flow-node id.
 * @param nodes      부모 플로우의 전체 runtime 노드 목록(getFlowNodes 결과).
 */
export function aggregateFlowNodeStats(
  flowNodeId: string,
  nodes: ReadonlyArray<FlowNodeInfo>,
): NodeRuntimeStats | undefined {
  const prefix = subflowNodeIdPrefix(flowNodeId);
  const portAcc = new Map<string, PortRuntimeStat>();
  const portOrder: string[] = [];
  const states: string[] = [];
  // SPEC-SUBFLOW-002 S03: 자식 네임스페이스 노드들의 stat_source 를 합집합으로
  // 결합한다(하나라도 embedded 면 flow-node 집계에 embedded 가 섞인 것으로 본다).
  let aggSource: StatSource | undefined;
  let matched = false;

  for (const node of nodes) {
    if (!node.node_id.startsWith(prefix)) continue;
    matched = true;
    states.push(node.state);
    aggSource = combineStatSource(aggSource, parseStatSource(node.extra));
    const ports = (node.ports ?? []).map((p) => ({
      name: p.name,
      direction: p.direction,
      messages: p.messages,
      delivered: p.delivered,
    }));
    sumPorts(portAcc, portOrder, ports);
  }

  if (!matched) return undefined;

  const mergedPorts = portOrder.map((name) => {
    const port = portAcc.get(name);
    // portOrder 에 들어간 이름은 항상 accumulator 에 존재한다.
    return port as PortRuntimeStat;
  });
  return portsToRuntimeStats(mergedPorts, pickAggregateState(states), aggSource);
}

/**
 * 서브플로우 단독 뷰에서 subflow-stats 응답 노드를 NodeRuntimeStats 로 변환한다(Fix 2).
 *
 * subflow-stats 의 node_id 는 ORIGINAL 노드 id 이며, ports 는 getFlowNodes 와 동일
 * 형태이다. 따라서 일반 runtime 노드와 동일한 포트 합산 규칙을 적용한다.
 */
export function subflowStatToRuntimeStats(stat: SubflowNodeStat): NodeRuntimeStats {
  return portsToRuntimeStats(stat.ports ?? [], stat.state ?? '');
}

/**
 * 서브플로우 단독 뷰의 통계 맵을 만든다: 자기 자신의 getFlowNodes(own) 우선,
 * 없으면 subflow-stats(부모에서 역집계) 로 채운다(Fix 2 병합 규칙).
 *
 * 같은 노드 id 가 양쪽에 모두 있으면(서브플로우가 단독 배포되면서 부모에도 참조)
 * own 을 우선한다(own takes precedence). own 에 없는 노드만 subflow-stats 로 채운다.
 *
 * @param ownMap         getFlowNodes(현재 플로우) 로 만든 exact-id 통계 맵.
 * @param subflowStats   subflow-stats 응답 노드 목록(원본 노드 id 키).
 */
export function mergeSubflowStats(
  ownMap: Record<string, NodeRuntimeStats>,
  subflowStats: ReadonlyArray<SubflowNodeStat>,
): Record<string, NodeRuntimeStats> {
  const merged: Record<string, NodeRuntimeStats> = { ...ownMap };
  for (const stat of subflowStats) {
    if (Object.prototype.hasOwnProperty.call(merged, stat.node_id)) {
      continue; // own 우선 — 이미 자기 getFlowNodes 통계가 있으면 유지한다.
    }
    merged[stat.node_id] = subflowStatToRuntimeStats(stat);
  }
  return merged;
}
