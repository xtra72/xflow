import type { NodeTypeInfo } from '@/types/node';

import { get } from './client';

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
