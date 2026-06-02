import type { NodeTypeInfo } from '@/types/node';

import { get, post } from './client';

/**
 * List all available node types (input, output, transform, etc.).
 */
export async function getNodeTypes(): Promise<NodeTypeInfo[]> {
  return get<NodeTypeInfo[]>('/nodes');
}

/**
 * Get detailed information about a specific node type.
 */
export async function getNodeType(type: string): Promise<NodeTypeInfo> {
  return get<NodeTypeInfo>(`/nodes/${type}`);
}

/**
 * 실행 중인 플로우의 특정 노드 설정을 라이브로 적용한다.
 *
 * 저장/재배포 없이 현재 배포된 런타임 노드에 즉시 반영된다 (예: output 노드의
 * `output_enabled` ON/OFF). 플로우가 실행 중이 아니거나 노드가 없으면 백엔드는
 * 404 를 반환하며, 호출자는 이를 "실행 중 아님" 으로 간주해야 한다 (에디터
 * 상태 변경은 다음 배포에서 반영되므로 에러로 취급하지 않는다).
 *
 * @param flowId 대상 플로우 ID
 * @param nodeId 대상 노드 ID
 * @param config 적용할 설정 (예: `{ output_enabled: false }`)
 */
export async function configureNode(
  flowId: string,
  nodeId: string,
  config: Record<string, unknown>,
): Promise<void> {
  await post<void>(`/flows/${flowId}/nodes/${nodeId}/configure`, { config });
}
