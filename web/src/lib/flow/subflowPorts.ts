// SPEC-SUBFLOW-001 그룹 C: flow-node 의 참조 플로우 포트 비정규화(denormalize) 유틸.
//
// computePortsForNode 는 동기(sync) 이고 노드 config 만 보므로, flow-node 의 핸들을
// 렌더링하려면 참조 플로우의 플로우 레벨 포트(inputs/outputs)를 미리 config 에
// 비정규화해 둬야 한다. 사용자가 flow_id 를 선택(또는 "포트 갱신")할 때 참조 플로우
// 정의를 조회하여 포트 이름 배열(input_ports / output_ports)과 표시 이름(flow_name)을
// 추출해 flow-node config 에 저장한다.
//
// 백엔드는 배포 시점에 참조 플로우의 실제 정의에서 포트를 재해석하므로
// config 의 input_ports / output_ports / flow_name 은 에디터 표시 전용 캐시이다
// (REQ-SUBFLOW-C03 항상 최신 / REQ-SUBFLOW-D04 배포 시 재해석).

import { getFlow } from '@/services/api/flowService';
import type { FlowInfo } from '@/types/flow';

/** 참조 플로우에서 비정규화한 flow-node 표시용 포트 정보. */
export interface ResolvedFlowNodePorts {
  /** 참조 플로우 입력 포트 이름 배열 → flow-node 입력 핸들. */
  input_ports: string[];
  /** 참조 플로우 출력 포트 이름 배열 → flow-node 출력 핸들. */
  output_ports: string[];
  /** 참조 플로우 표시 이름 (노드 카드 표시 전용). */
  flow_name: string;
}

/** 플로우 정의 최상위 포트 항목 ({id,name,direction}). */
interface FlowDefinitionPort {
  id?: unknown;
  name?: unknown;
  direction?: unknown;
}

/**
 * 플로우 레벨 포트 맵 배열에서 (공백 제거 + 비어있지 않은) 이름 배열을 추출한다.
 * 비문자열/빈 이름/중복은 안전하게 걸러낸다.
 */
export function extractPortNames(ports: unknown): string[] {
  if (!Array.isArray(ports)) return [];
  const seen = new Set<string>();
  const names: string[] = [];
  for (const raw of ports) {
    const p = raw as FlowDefinitionPort;
    const name = typeof p?.name === 'string' ? p.name.trim() : '';
    if (name === '' || seen.has(name)) continue;
    seen.add(name);
    names.push(name);
  }
  return names;
}

/**
 * 플로우 상세 응답에서 flow-node 표시용 포트 정보를 추출한다.
 *
 * 플로우 상세(GET /flows/:id)는 reactflow 정의를 config 에 담아 반환하며,
 * 플로우 레벨 포트는 config.inputs / config.outputs (정의 최상위) 에 위치한다
 * (flowToReactFlowConfig, SPEC-SUBFLOW-001 REQ-SUBFLOW-A07).
 */
export function extractFlowNodePorts(flow: FlowInfo): ResolvedFlowNodePorts {
  const config = (flow.config ?? {}) as Record<string, unknown>;
  return {
    input_ports: extractPortNames(config.inputs),
    output_ports: extractPortNames(config.outputs),
    flow_name: flow.name ?? '',
  };
}

/**
 * 참조 플로우(flowId)의 정의를 조회하여 flow-node 표시용 포트 정보를 비정규화한다.
 *
 * flow_id 선택 시 또는 "포트 갱신" 시 호출하여, 반환값을 flow-node config 에 병합한다.
 */
export async function resolveFlowNodePorts(
  flowId: string,
): Promise<ResolvedFlowNodePorts> {
  const flow = await getFlow(flowId);
  return extractFlowNodePorts(flow);
}
