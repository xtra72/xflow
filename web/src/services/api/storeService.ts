// Store API 클라이언트.
// GET /api/v1/store/{agent_name}/keys — 키 목록 조회.
//
// v0.7.0 (M11) BREAKING CHANGE: 백엔드 응답이 객체 배열로 진화함에 따라
// 이 모듈은 응답을 string[] 형태로 derived 해 반환한다. 신규 코드는
// `services/api/store.ts` 의 `fetchStoreKeyObjects` 또는 `fetchStoreKeys` 를
// 사용해야 한다. 이 모듈은 대시보드 PanelSettingsDialog 호환을 위해 유지된다.
//
// @spec SPEC-WEB-005 v0.7.0 (M11)

import { get } from './client';
import type { StoreKeyObject } from './store';

/**
 * v0.3.0 (M11) 응답 형상.
 * 이전 v0.2.0 의 `keys: string[]` 는 폐기되었다.
 *
 * @spec SPEC-WEB-005 v0.7.0 (M11)
 */
export interface StoreKeysResponse {
  keys: StoreKeyObject[];
  count: number;
}

/**
 * 키 이름 배열만 받아오는 alternate 클라이언트 (PanelSettingsDialog 전용).
 *
 * v0.7.0 (M11): 백엔드 객체 배열에서 `key` 필드만 추출해 string[] 으로 derived.
 * 신규 코드는 `fetchStoreKeyObjects` 사용 권장.
 *
 * @deprecated 신규 코드는 `services/api/store.ts` 의 `fetchStoreKeys` 또는
 *   `fetchStoreKeyObjects` 를 사용하라. 이 함수는 대시보드 호환을 위해 유지된다.
 * @spec SPEC-WEB-005 v0.7.0 (M11)
 */
export async function listStoreKeys(
  agentName: string,
  namespace = 'default',
  pattern = '*',
): Promise<string[]> {
  const params = new URLSearchParams({ namespace, pattern });
  const data = await get<StoreKeysResponse>(
    `/store/${encodeURIComponent(agentName)}/keys?${params}`,
  );
  return (data.keys ?? []).map((obj) => obj.key);
}
