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
import { getRemoteFlow } from '@/services/api/remoteService';
import type { FlowInfo } from '@/types/flow';

/**
 * 원격 플로우 참조의 정규화 스킴 접두사.
 *
 * 로컬 플로우 편집(target=local)에서 원격 노드의 플로우를 서브플로우로 참조할 때
 * flow-node 의 flow_id 는 `remote://{instance_id}/{flow_id}` 로 정규화(qualify)된다
 * (SPEC-SUBFLOW-001 v1.2 그룹 RU). 백엔드는 배포 시점에 이 참조를 query 프록시
 * (flow/get)로 해석하여 인라인 확장한다. 스킴이 없는 평문(bare) id 는 로컬 참조다.
 */
const REMOTE_FLOW_REF_PREFIX = 'remote://';

/** 파싱된 원격 플로우 참조(노드 인스턴스 + 노드 로컬 플로우 id). */
export interface RemoteFlowRef {
  /** 참조 대상 노드 인스턴스 식별자. */
  instanceId: string;
  /** 그 노드에 존재하는 플로우 id(노드 로컬 채번). */
  flowId: string;
}

/**
 * `remote://{instance_id}/{flow_id}` 정규화 참조를 빌드한다.
 *
 * 백엔드 규약과 동형이다(스킴 + instanceId + '/' + flowId). instanceId/flowId 는
 * 노드-로컬 식별자(보통 영숫자/대시)이므로 별도 인코딩 없이 그대로 결합한다.
 */
export function buildRemoteFlowRef(instanceId: string, flowId: string): string {
  return `${REMOTE_FLOW_REF_PREFIX}${instanceId}/${flowId}`;
}

/**
 * 정규화된 원격 플로우 참조 문자열을 파싱한다(순수 함수).
 *
 * 백엔드 `remote://{instance_id}/{flow_id}` 와 동형으로, 스킴 제거 후 첫 '/' 에서
 * 한 번만 분할한다(flowId 에 '/' 가 포함될 수 있으므로 첫 구분자 기준). 스킴이
 * 없거나(평문 로컬 id), instanceId/flowId 중 하나라도 비면 null 을 반환한다.
 *
 * @param value - flow-node 의 flow_id 값(정규화 원격 참조 또는 평문 로컬 id).
 * @returns 원격 참조면 { instanceId, flowId }, 아니면 null(로컬 참조로 처리).
 */
export function parseRemoteFlowRef(value: string): RemoteFlowRef | null {
  if (!value.startsWith(REMOTE_FLOW_REF_PREFIX)) return null;
  const rest = value.slice(REMOTE_FLOW_REF_PREFIX.length);
  const slash = rest.indexOf('/');
  if (slash <= 0) return null;
  const instanceId = rest.slice(0, slash);
  const flowId = rest.slice(slash + 1);
  if (instanceId === '' || flowId === '') return null;
  return { instanceId, flowId };
}

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

/**
 * 원격 노드의 참조 플로우(flowId)의 정의를 조회하여 flow-node 표시용 포트 정보를
 * 비정규화한다 (SPEC-REMOTE-001 — 타깃 인지 서브플로우 picker / "포트 갱신").
 *
 * 원격 노드의 플로우를 서브플로우로 참조할 때, 포트 갱신은 로컬 매니저의 GET
 * /flows/:id 가 아니라 대상 노드의 flow READ 프록시(getRemoteFlow → flow/get)에서
 * 정의를 가져와야 한다. 그렇지 않으면 매니저 로컬 저장소의 플로우(또는 404)를
 * 잘못 해석하게 된다. 반환 정의의 config 최상위 inputs/outputs 가 포트 소스다
 * (로컬 getFlow 와 동형).
 */
export async function resolveRemoteFlowNodePorts(
  instanceId: string,
  flowId: string,
): Promise<ResolvedFlowNodePorts> {
  const flow = await getRemoteFlow<FlowInfo>(instanceId, flowId);
  return extractFlowNodePorts(flow);
}
