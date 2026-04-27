// Store API 클라이언트.
// GET /api/v1/store/{agent_name}/keys — 키 목록 조회.

import { get } from './client';

export interface StoreKeysResponse {
  keys: string[];
  count: number;
}

export async function listStoreKeys(
  agentName: string,
  namespace = 'default',
  pattern = '*',
): Promise<string[]> {
  const params = new URLSearchParams({ namespace, pattern });
  const data = await get<StoreKeysResponse>(
    `/store/${encodeURIComponent(agentName)}/keys?${params}`,
  );
  return data.keys ?? [];
}
